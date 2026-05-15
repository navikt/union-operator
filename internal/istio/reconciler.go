package istio

import (
	"context"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler manages Istio resources (ServiceEntries, VirtualServices,
// DestinationRules, AuthorizationPolicies, and Gateways) in the egress namespace.
type Reconciler struct {
	client.Client
	OnpremHosts uniontypes.OnpremHostMap
}

func (r *Reconciler) EnsureExternalHosts(ctx context.Context, serviceAccounts []uniontypes.ServiceAccount) error {
	for _, sa := range serviceAccounts {
		for _, host := range sa.ExternalAllowlist {
			if err := r.ensureGatewayContainsHost(ctx, host); err != nil {
				return err
			}

			if err := r.ensureVirtualServiceForHost(ctx, sa, &host); err != nil {
				return err
			}

			if err := r.ensureDestinationRule(ctx, sa, host); err != nil {
				return err
			}

			exists, err := r.serviceEntryExists(ctx, host)
			if err != nil {
				return err
			}
			if exists {
				continue
			}

			if err := r.createServiceEntry(ctx, sa, host); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler) EnsureAuthorizationPolicies(ctx context.Context, serviceAccounts []uniontypes.ServiceAccount) error {
	for _, sa := range serviceAccounts {
		for _, host := range sa.ExternalAllowlist {
			if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &host, httpsProtocol, hostTypeLabelExternal); err != nil {
				return err
			}
		}

		for _, host := range sa.InternalAllowlist {
			if hostData, ok := r.OnpremHosts[host.Host]; ok {
				if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &host, hostData.Protocol, hostTypeLabelInternal); err != nil {
					return err
				}
				for _, vip := range hostData.VIP {
					vipHost := datanavnov1.Host{Host: vip}
					if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &vipHost, hostData.Protocol, hostTypeLabelInternal); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
