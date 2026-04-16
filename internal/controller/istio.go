package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	"istio.io/api/networking/v1alpha3"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istiosecuritymodels "istio.io/api/security/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	istioEgressNamespace   = "istio-egress"
	gatewayHost            = "istio-egressgateway.istio-egress.svc.cluster.local"
	gatewayName            = "istio-egressgateway"
	meshGatewayName        = "mesh"
	istioGatewaySelector   = "istioegressgateway"
	egressToGatewayLabel   = "egress-to-gateway"
	egressFromGatewayLabel = "egress-from-gateway"
	httpsProtocol          = "HTTPS"
	tcpProtocol            = "TCP"
	hostTypeLabelExternal  = "external"
	hostTypeLabelInternal  = "internal"
)

func (r *UnionTeamServiceAccountsReconciler) cleanupUnusedHosts(ctx context.Context) error {
	var utsas datanavnov1.UnionTeamServiceAccountsList
	err := r.List(ctx, &utsas)
	if err != nil {
		return err
	}

	err = r.cleanupInternalHosts(ctx, utsas)
	if err != nil {
		return err
	}

	err = r.cleanupExternalHosts(ctx, utsas)
	if err != nil {
		return err
	}

	return nil

}

func (r *UnionTeamServiceAccountsReconciler) cleanupInternalHosts(ctx context.Context, utsas datanavnov1.UnionTeamServiceAccountsList) error {
	for _, utsa := range utsas.Items {
		for _, sa := range utsa.Spec.ServiceAccounts {
			hosts := &istiosecurity.AuthorizationPolicyList{}
			err := r.List(
				ctx,
				hosts,
				client.InNamespace(istioEgressNamespace),
				client.MatchingLabels{
					"project":         utsa.Spec.Project,
					"domain":          utsa.Spec.Domain,
					"service-account": sa.Name,
					"host-type":       hostTypeLabelInternal,
				},
			)
			if err != nil {
				return err
			}

			for _, host := range hosts.Items {
				if !r.containsHost(host, sa.InternalAllowlist) {
					err := r.Delete(ctx, host)
					if err != nil {
						return err
					}
				}
			}
		}

	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) containsHost(existing *istiosecurity.AuthorizationPolicy, internalAllowlist []datanavnov1.Host) bool {
	for _, host := range internalAllowlist {
		if existing.Labels["host"] == host.Host {
			return true
		}
	}

	return false
}

func (r *UnionTeamServiceAccountsReconciler) cleanupExternalHosts(ctx context.Context, utsas datanavnov1.UnionTeamServiceAccountsList) error {
	log := logf.FromContext(ctx)

	externalHostsInUse := make(map[string]bool)
	for _, utsa := range utsas.Items {
		for _, sa := range utsa.Spec.ServiceAccounts {
			for _, host := range sa.ExternalAllowlist {
				externalHostsInUse[host.Host] = true
			}
		}
	}

	gateway := &istionetworking.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: istioEgressNamespace}, gateway)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Gateway not found, skipping cleanup")
			return nil
		}

		return err
	}

	for _, server := range gateway.Spec.Servers {
		if server.Port.Protocol == httpsProtocol {
			for _, host := range server.Hosts {
				_, ok := externalHostsInUse[host]
				if !ok {
					err := r.removeExternalHost(ctx, host)
					if err != nil {
						return err
					}
				}
			}
			hosts := slices.Collect(maps.Keys(externalHostsInUse))
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

func (r *UnionTeamServiceAccountsReconciler) removeExternalHost(ctx context.Context, host string) error {
	var errs []error
	for _, list := range []client.ObjectList{
		&istionetworking.ServiceEntryList{},
		&istionetworking.VirtualServiceList{},
		&istionetworking.DestinationRuleList{},
		&istiosecurity.AuthorizationPolicyList{},
	} {
		if err := r.removeByHostLabel(ctx, host, list); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *UnionTeamServiceAccountsReconciler) removeByHostLabel(ctx context.Context, host string, list client.ObjectList) error {
	err := r.List(ctx, list, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host})
	if err != nil {
		return err
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		return err
	}

	var errs []error
	for _, item := range items {
		if obj, ok := item.(client.Object); ok {
			if err := r.Delete(ctx, obj); err != nil {
				errs = append(errs, err)
			}
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

func (r *UnionTeamServiceAccountsReconciler) serviceEntryExists(ctx context.Context, host datanavnov1.Host) (error, bool) {
	log := logf.FromContext(ctx)
	seList := &istionetworking.ServiceEntryList{}
	err := r.List(ctx, seList, client.InNamespace(istioEgressNamespace), client.MatchingLabels{"host": host.Host})
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list ServiceEntries in %s namespace", istioEgressNamespace))
		return err, false
	}

	return nil, len(seList.Items) > 0
}

func (r *UnionTeamServiceAccountsReconciler) createIstioServiceEntry(ctx context.Context, host datanavnov1.Host) error {
	se := createHTTPSServiceEntry(host)
	err := r.Create(ctx, se)
	if err != nil {
		return err
	}

	return nil
}

func createHTTPSServiceEntry(host datanavnov1.Host) *istionetworking.ServiceEntry {
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

func (r *UnionTeamServiceAccountsReconciler) ensureGatewayContainsHost(ctx context.Context, host datanavnov1.Host) error {
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

func (r *UnionTeamServiceAccountsReconciler) ensureVirtualServiceForHost(ctx context.Context, host *datanavnov1.Host) error {
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

func (r *UnionTeamServiceAccountsReconciler) createVirtualServiceForHost(host *datanavnov1.Host) *istionetworking.VirtualService {
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
			err := r.ensureAuthorizationPolicyForHost(ctx, unionEnv, sa.Name, &host, httpsProtocol, hostTypeLabelExternal)
			if err != nil {
				return err
			}
		}

		for _, host := range sa.InternalAllowlist {
			if hostData, ok := r.OnpremHosts[host.Host]; ok {
				err := r.ensureAuthorizationPolicyForHost(ctx, unionEnv, sa.Name, &host, hostData.Protocol, hostTypeLabelInternal)
				if err != nil {
					return err
				}
				for _, vip := range hostData.VIP {
					vipHost := datanavnov1.Host{
						Host: vip,
					}
					err := r.ensureAuthorizationPolicyForHost(ctx, unionEnv, sa.Name, &vipHost, hostData.Protocol, hostTypeLabelInternal)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) ensureAuthorizationPolicyForHost(ctx context.Context, unionEnv *UnionEnv, serviceAccount string, host *datanavnov1.Host, protocol, hostTypeLabel string) error {
	_ = logf.FromContext(ctx)

	apList := &istiosecurity.AuthorizationPolicyList{}
	err := r.List(
		ctx,
		apList,
		client.InNamespace(istioEgressNamespace),
		client.MatchingLabels{
			"project":         unionEnv.Project,
			"domain":          unionEnv.Domain,
			"service-account": serviceAccount,
			"host":            host.Host,
		},
	)
	if err != nil {
		return err
	}

	if len(apList.Items) < 1 {
		ap, err := r.createAuthorizationPolicyForHost(unionEnv, serviceAccount, host, protocol, hostTypeLabel)
		if err != nil {
			return err
		}
		err = r.Create(ctx, ap)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createAuthorizationPolicyForHost(unionEnv *UnionEnv, serviceAccount string, host *datanavnov1.Host, protocol, hostTypeLabel string) (*istiosecurity.AuthorizationPolicy, error) {
	var when []*istiosecuritymodels.Condition
	var to []*istiosecuritymodels.Rule_To
	switch strings.ToUpper(protocol) {
	case httpsProtocol:
		to = []*istiosecuritymodels.Rule_To{
			{
				Operation: &istiosecuritymodels.Operation{
					Hosts: []string{host.Host},
					Paths: host.Paths,
				},
			},
		}
	case tcpProtocol:
		when = []*istiosecuritymodels.Condition{
			{
				Key: "connection.sni",
				Values: []string{
					host.Host,
				},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return &istiosecurity.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s-%s", unionEnv.Project, unionEnv.Domain, serviceAccount, host.Name()),
			Namespace: istioEgressNamespace,
			Labels: map[string]string{
				"project":         unionEnv.Project,
				"domain":          unionEnv.Domain,
				"service-account": serviceAccount,
				"host":            host.Host,
				"host-type":       hostTypeLabel,
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
					When: when,
					To:   to,
				},
			},
		},
	}, nil
}

func (r *UnionTeamServiceAccountsReconciler) ensureIstioDestinationRule(ctx context.Context, host datanavnov1.Host) error {
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

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHostToGateway(host datanavnov1.Host) *istionetworking.DestinationRule {
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
				Sni:  host.Host,
			},
		})
}

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHostFromGateway(host datanavnov1.Host) *istionetworking.DestinationRule {
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

func (r *UnionTeamServiceAccountsReconciler) createDestinationRuleForHost(name, gatewayLabel, targetHost string, host datanavnov1.Host, portTrafficPolicy *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworking.DestinationRule {
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
