package controller

import (
	"context"
	"fmt"
	"slices"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	istiov1beta1 "istio.io/api/networking/v1beta1"
	istio "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	istioEgressNamespace = "istio-egress"
	gatewayHost = "istio-egressgateway.istio-egress.svc.cluster.local"
	gatewayName = "istio-egressgateway"
	meshGatewayName = "mesh"
)

func (r *UnionTeamServiceAccountsReconciler) ensureIstioServiceEntry(ctx context.Context, unionEnv *UnionEnv) error {

	for _, sa := range unionEnv.ServiceAccounts {
		for _, host := range sa.Allowlist {
			err := r.ensureGatewayContainsHost(ctx, host)
			if err != nil {
				return err
			}

			err = r.ensureVirtualServiceForHost(ctx, &host)
			if err != nil {
				return err
			}

			err, exists := r.serviceEntryExists(ctx, host)
			if err != nil {
				return err
			}
			if exists {
				continue
			}


			err = r.createIstioServiceEntry(ctx, host)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) serviceEntryExists(ctx context.Context, host datanavnov1.AllowedHost) (error, bool) {
	log := logf.FromContext(ctx)
	seList := &istio.ServiceEntryList{}
	err := r.List(ctx, seList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host.Host})
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list ServiceEntries in %s namespace", istioEgressNamespace))
		return err, false
	}

	return nil, len(seList.Items) > 0
}

func (r *UnionTeamServiceAccountsReconciler) createIstioServiceEntry(ctx context.Context, host datanavnov1.AllowedHost) error {
	se := &istio.ServiceEntry{}
	switch host.Protocol {
	case "HTTPS":
		se = createHTTPSServiceEntry(host)
	case "TCP":
		panic("uninplemented")
	default:
		return fmt.Errorf("unsupported protocol: %s", host.Protocol)
	}

	err := r.Create(ctx, se)
	if err != nil {
		return err
	}

	return nil
}

func createHTTPSServiceEntry(host datanavnov1.AllowedHost) *istio.ServiceEntry {
	return &istio.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"host": host.Host,
			},
		},
		Spec: istiov1beta1.ServiceEntry{
			Hosts:      []string{host.Host},
			Ports:      []*istiov1beta1.ServicePort{
				{
					Number: 80,
					Name: "http",
					Protocol: "HTTP",
				},
				{
					Number: uint32(host.Port), 
					Name: host.Protocol, 
					Protocol: host.Protocol,
				},
			},
			Resolution:  istiov1beta1.ServiceEntry_DNS,
			Location:    istiov1beta1.ServiceEntry_MESH_EXTERNAL,
			ExportTo:   []string{"*"},
		},
	}
}

func (r *UnionTeamServiceAccountsReconciler) ensureGatewayContainsHost(ctx context.Context, host datanavnov1.AllowedHost) error {
	log := logf.FromContext(ctx)
	gateway := &istio.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: istioEgressNamespace}, gateway)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get Gateway %s in %s namespace", gatewayName, istioEgressNamespace))
		return err
	}

	gateway.Spec.Servers[0].Hosts = append(gateway.Spec.Servers[0].Hosts, host.Host)
	slices.Sort(gateway.Spec.Servers[0].Hosts)
	gateway.Spec.Servers[0].Hosts = slices.Compact(gateway.Spec.Servers[0].Hosts)

	if err := r.Update(ctx, gateway); err != nil {
		log.Error(err, "Failed to patch Gateway", "name", gatewayName)
		return err
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) ensureVirtualServiceForHost(ctx context.Context, host *datanavnov1.AllowedHost) error {
	log := logf.FromContext(ctx)

	vs := &istio.VirtualService{}
	err := r.Get(ctx, types.NamespacedName{Name: host.Name(), Namespace: istioEgressNamespace}, vs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			vs = r.createVirtualServiceForHost(host)
			err = r.Create(ctx, vs)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to create VirtualService %s in %s namespace", host.Name(), istioEgressNamespace))
				return err
			}
			return nil
		}
		log.Error(err, fmt.Sprintf("Failed to get VirtualService %s in %s namespace", host.Name(), istioEgressNamespace))
		return err
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createVirtualServiceForHost(host *datanavnov1.AllowedHost) *istio.VirtualService {
	return &istio.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"host": host.Host,
			},
		},
		Spec: istiov1beta1.VirtualService{
			Hosts:    []string{host.Host},
			Gateways: []string{
				meshGatewayName,
				gatewayName,
			},
			Http: []*istiov1beta1.HTTPRoute{
				{
					Match: []*istiov1beta1.HTTPMatchRequest{
						{
							Gateways: []string{
								meshGatewayName,
							},
							Port: 80,
						},
					},
					Route: []*istiov1beta1.HTTPRouteDestination{
						{
							Destination: &istiov1beta1.Destination{
								Host: gatewayHost, 
								Port: &istiov1beta1.PortSelector{
									Number: 80,
								},
							},
						},
					},
				},
				{
					Match: []*istiov1beta1.HTTPMatchRequest{
						{
							Gateways: []string{
								gatewayName,
							},
							Port: 80,
						},
					},
					Route: []*istiov1beta1.HTTPRouteDestination{
						{
							Destination: &istiov1beta1.Destination{
								Host: host.Host, 
								Port: &istiov1beta1.PortSelector{
									Number: uint32(host.Port),
								},
							},
						},
					},
				},
			},
		},
	}
}
