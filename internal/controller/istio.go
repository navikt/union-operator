package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	"istio.io/api/networking/v1alpha3"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istiosecuritymodels "istio.io/api/security/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	istioEgressNamespace = "istio-egress"
	gatewayHost          = "istio-egressgateway.istio-egress.svc.cluster.local"
	gatewayName          = "istio-egressgateway"
	meshGatewayName      = "mesh"
	istioGatewaySelector = "istioegressgateway"
	egressToGatewayLabel = "egress-to-gateway"
	egressFromGatewayLabel = "egress-from-gateway"
)

func (r *UnionTeamServiceAccountsReconciler) cleanupUnusedHosts(ctx context.Context) error {
	log := logf.FromContext(ctx)

	var utsas datanavnov1.UnionTeamServiceAccountsList
	err := r.List(ctx, &utsas)
	if err != nil {
		return err
	}

	hostsInUse := make(map[string]bool)
	for _, utsa := range utsas.Items {
		for _, sa := range utsa.Spec.ServiceAccounts {
			for _, host := range sa.ExternalAllowlist {
				hostsInUse[host.Host] = true
			}
		}
	}

	gateway := &istionetworking.Gateway{}
	err = r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: istioEgressNamespace}, gateway)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Gateway not found, skipping cleanup")
			return nil
		}

		return err
	}

	for _, server := range gateway.Spec.Servers {
		if server.Port.Protocol == "HTTPS" {
			for _, host := range server.Hosts {
				_, ok := hostsInUse[host]
				if !ok {
					err := r.removeHost(ctx, host)
					if err != nil {
						return err
					}
				}
			}
			hosts := slices.Collect(maps.Keys(hostsInUse))
			slices.Sort(hosts)
			server.Hosts = hosts

			if len(hosts) == 0 {
				err := r.Delete(ctx, gateway)
				if err != nil {
					return err
				}

				return nil
			}

			if err := r.Update(ctx, gateway); err != nil {
				return err
			}

		}
	}

	return nil

}

func (r *UnionTeamServiceAccountsReconciler) removeHost(ctx context.Context, host string) error {
	var errs []error
	err := r.removeServiceEntry(ctx, host)
	if err != nil {
		errs = append(errs, err)
	}

	err = r.removeVirtualService(ctx, host)
	if err != nil {
		errs = append(errs, err)
	}

	err = r.removeDestinationRules(ctx, host)
	if err != nil {
		errs = append(errs, err)
	}

	err = r.removeAuthorizationPolicy(ctx, host)
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) removeServiceEntry(ctx context.Context, host string) error {
	seList := &istionetworking.ServiceEntryList{}
	err := r.List(ctx, seList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host})
	if err != nil {
		return err
	}

	var errs []error
	for _, se := range seList.Items {
		err = r.Delete(ctx, se)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) removeVirtualService(ctx context.Context, host string) error {
	vsList := &istionetworking.VirtualServiceList{}
	err := r.List(ctx, vsList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host})
	if err != nil {
		return err
	}

	var errs []error
	for _, vs := range vsList.Items {
		err = r.Delete(ctx, vs)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) removeDestinationRules(ctx context.Context, host string) error {
 	drList := &istionetworking.DestinationRuleList{}
	err := r.List(ctx, drList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host})
	if err != nil {
		return err
	}

	var errs []error
	for _, vs := range drList.Items {
		err = r.Delete(ctx, vs)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) removeAuthorizationPolicy(ctx context.Context, host string) error {
	apList := &istiosecurity.AuthorizationPolicyList{}
	err := r.List(ctx, apList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host})
	if err != nil {
		return err
	}

	var errs []error
	for _, authPol := range apList.Items {
		err = r.Delete(ctx, authPol)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) ensureIstioExternalHost(ctx context.Context, unionEnv *UnionEnv) error {

	for _, sa := range unionEnv.ServiceAccounts {
		for _, host := range sa.ExternalAllowlist {
			err := r.ensureGatewayContainsHost(ctx, host)
			if err != nil {
				return err
			}

			err = r.ensureVirtualServiceForHost(ctx, &host)
			if err != nil {
				return err
			}

			err = r.ensureIstioDestinationRule(ctx, host)
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

func (r *UnionTeamServiceAccountsReconciler) serviceEntryExists(ctx context.Context, host datanavnov1.ExternalHost) (error, bool) {
	log := logf.FromContext(ctx)
	seList := &istionetworking.ServiceEntryList{}
	err := r.List(ctx, seList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host.Host})
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list ServiceEntries in %s namespace", istioEgressNamespace))
		return err, false
	}

	return nil, len(seList.Items) > 0
}

func (r *UnionTeamServiceAccountsReconciler) createIstioServiceEntry(ctx context.Context, host datanavnov1.ExternalHost) error {
	se := createHTTPSServiceEntry(host)
	err := r.Create(ctx, se)
	if err != nil {
		return err
	}

	return nil
}

func createHTTPSServiceEntry(host datanavnov1.ExternalHost) *istionetworking.ServiceEntry {
	return &istionetworking.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"host": host.Host,
			},
		},
		Spec: istionetworkingmodels.ServiceEntry{
			Hosts: []string{host.Host},
			Ports: []*istionetworkingmodels.ServicePort{
				{
					Number:   80,
					Name:     "http",
					Protocol: "HTTP",
				},
				{
					Number:   443,
					Name:     "https",
					Protocol: "HTTPS",
				},
			},
			Resolution: istionetworkingmodels.ServiceEntry_DNS,
			Location:   istionetworkingmodels.ServiceEntry_MESH_EXTERNAL,
			ExportTo:   []string{"*"},
		},
	}
}

func (r *UnionTeamServiceAccountsReconciler) ensureGatewayContainsHost(ctx context.Context, host datanavnov1.ExternalHost) error {
	log := logf.FromContext(ctx)
	gateway := &istionetworking.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: istioEgressNamespace}, gateway)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, fmt.Sprintf("Failed to get Gateway %s in %s namespace", gatewayName, istioEgressNamespace))
			return err
		}

		gateway = r.createGateway(host.Host)
		err = r.Create(ctx, gateway)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to create Gateway %s in %s namespace", gatewayName, istioEgressNamespace))
			return err
		}

		return nil
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

func (r *UnionTeamServiceAccountsReconciler) ensureVirtualServiceForHost(ctx context.Context, host *datanavnov1.ExternalHost) error {
	log := logf.FromContext(ctx)

	vs := &istionetworking.VirtualService{}
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

func (r *UnionTeamServiceAccountsReconciler) createGateway(host string) *istionetworking.Gateway {
	return &istionetworking.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Name:      gatewayName,
					Namespace: istioEgressNamespace,
				},
				Spec: istionetworkingmodels.Gateway{
					Selector: map[string]string{
						"app": istioGatewaySelector,
					},
					Servers: []*istionetworkingmodels.Server{
						{
							Port: &istionetworkingmodels.Port{
								Number:   80,
								Name:     "http-port-for-tls-origination",
								Protocol: "HTTPS",
							},
							Hosts: []string{host},
							Tls: &istionetworkingmodels.ServerTLSSettings{
								Mode: istionetworkingmodels.ServerTLSSettings_ISTIO_MUTUAL,
							},
						},
					},
				},
			}
}

func (r *UnionTeamServiceAccountsReconciler) createVirtualServiceForHost(host *datanavnov1.ExternalHost) *istionetworking.VirtualService {
	return &istionetworking.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"host": host.Host,
			},
		},
		Spec: istionetworkingmodels.VirtualService{
			Hosts: []string{host.Host},
			Gateways: []string{
				meshGatewayName,
				gatewayName,
			},
			Http: []*istionetworkingmodels.HTTPRoute{
				{
					Match: []*istionetworkingmodels.HTTPMatchRequest{
						{
							Gateways: []string{
								meshGatewayName,
							},
							Port: 80,
						},
					},
					Route: []*istionetworkingmodels.HTTPRouteDestination{
						{
							Destination: &istionetworkingmodels.Destination{
								Host: gatewayHost,
								Port: &istionetworkingmodels.PortSelector{
									Number: 80,
								},
							},
						},
					},
				},
				{
					Match: []*istionetworkingmodels.HTTPMatchRequest{
						{
							Gateways: []string{
								gatewayName,
							},
							Port: 80,
						},
					},
					Route: []*istionetworkingmodels.HTTPRouteDestination{
						{
							Destination: &istionetworkingmodels.Destination{
								Host: host.Host,
								Port: &istionetworkingmodels.PortSelector{
									Number: 443, 
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *UnionTeamServiceAccountsReconciler) ensureAuthorizationPolicies(ctx context.Context, unionEnv *UnionEnv) error {
	for _, sa := range unionEnv.ServiceAccounts {
		for _, host := range sa.ExternalAllowlist {
			err := r.ensureAuthorizationPolicyForHost(ctx, unionEnv, sa.Name, &host)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) ensureAuthorizationPolicyForHost(ctx context.Context, unionEnv *UnionEnv, serviceAccount string, host *datanavnov1.ExternalHost) error {
	_ = logf.FromContext(ctx)

	apList := &istiosecurity.AuthorizationPolicyList{}
	err := r.List(
		ctx, 
		apList, 
		client.InNamespace(istioEgressNamespace), 
		client.MatchingLabels{
			"project": unionEnv.Project,
			"domain": unionEnv.Domain,
			"service-account": serviceAccount,
			"host": host.Host,
		},
	)
	if err != nil {
		return err
	}

	if len(apList.Items) < 1 {
		ap := r.createAuthorizationPolicyForHost(unionEnv, serviceAccount, host)
		err = r.Create(ctx, ap)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createAuthorizationPolicyForHost(unionEnv *UnionEnv, serviceAccount string, host *datanavnov1.ExternalHost) *istiosecurity.AuthorizationPolicy {
	return &istiosecurity.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s-%s", unionEnv.Project, unionEnv.Domain, serviceAccount, host.Name()),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"project": unionEnv.Project,
				"domain": unionEnv.Domain,
				"service-account": serviceAccount,
				"host": host.Host,
			},
		},
		Spec: istiosecuritymodels.AuthorizationPolicy{
			Action: istiosecuritymodels.AuthorizationPolicy_ALLOW,
			Rules: []*istiosecuritymodels.Rule{
				{
					From: []*istiosecuritymodels.Rule_From{
						{
							Source: &istiosecuritymodels.Source{
								Principals: []string{fmt.Sprintf("cluster.local/ns/%s/sa/%s", unionEnv.Namespace(), serviceAccount)},
							},
						},
					},
					To: []*istiosecuritymodels.Rule_To{
						{
							Operation: &istiosecuritymodels.Operation{
								Hosts: []string{host.Host},
								Paths: host.Paths,
							},
						},
					},
				},
			},
		},
	}
}

func (r *UnionTeamServiceAccountsReconciler) ensureIstioDestinationRule(ctx context.Context, host datanavnov1.ExternalHost) error {
	dr := &istionetworking.DestinationRuleList{}
	err := r.List(
		ctx, 
		dr, 
		client.InNamespace(istioEgressNamespace), 
		client.MatchingLabels{
			"host": host.Host,
			"type": egressToGatewayLabel,
		},
	)
	if err != nil {
		return err
	}
	
	if len(dr.Items) < 1 {
		destinationRule := r.createDestinationRuleForHostToGateway(host)
		err = r.Create(ctx, destinationRule)
		if err != nil {
			return err
		}
	}

	err = r.List(
		ctx, 
		dr, 
		client.InNamespace(istioEgressNamespace), 
		client.MatchingLabels{
			"host": host.Host,
			"type": egressFromGatewayLabel,
		},
	)
	if err != nil {
		return err
	}
	
	if len(dr.Items) < 1 {
		destinationRule := r.createDestinationRuleForHostFromGateway(host)
		err = r.Create(ctx, destinationRule)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHostToGateway(host datanavnov1.ExternalHost) *istionetworking.DestinationRule {
	return r.createDestinationRuleForHost(
		fmt.Sprintf("%s-to-gateway", host.Name()),
		egressToGatewayLabel, 
		gatewayHost, 
		host,
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: 80,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_ISTIO_MUTUAL,
				Sni: host.Host,
			},
	})
}

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHostFromGateway(host datanavnov1.ExternalHost) *istionetworking.DestinationRule {
	return r.createDestinationRuleForHost(
		fmt.Sprintf("%s-from-gateway", host.Name()),
		egressFromGatewayLabel, 
		host.Host, 
		host,
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: 443,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_SIMPLE,
			},
	})
}

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHost(name, gatewayLabel, targetHost string, host datanavnov1.ExternalHost, portTrafficPolicy *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworking.DestinationRule {
	return &istionetworking.DestinationRule{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: istioEgressNamespace,
				Labels: map[string]string{
					"host": host.Host,
					"type": gatewayLabel,
				},
			},
			Spec: istionetworkingmodels.DestinationRule{
				Host: targetHost,
				TrafficPolicy: &istionetworkingmodels.TrafficPolicy{
					PortLevelSettings: []*v1alpha3.TrafficPolicy_PortTrafficPolicy{
						portTrafficPolicy,
					},
				},
			},
	}	
}