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
			if err := r.ensureGatewayExists(ctx, externalGatewayName, host.Host); err != nil {
				return err
			}

			if err := r.ensureGatewayContainsHost(ctx, externalGatewayName, host.Host); err != nil {
				return err
			}

			vsExists, err := r.virtualServiceExists(ctx, host.Name())
			if err != nil {
				return err
			}

			if !vsExists {
				if err = r.Create(ctx, newVirtualServiceForExternalHost(&host)); err != nil {
					return err
				}
			}

			drToGWExists, err := r.destinationRuleToGatewayExists(ctx, host.Host)
			if err != nil {
				return err
			}
			if !drToGWExists {
				if err = r.Create(ctx, newExternalHostDestinationRuleToGateway(host)); err != nil {
					return err
				}
			}

			drFromGWExists, err := r.destinationRuleFromGatewayExists(ctx, host.Host)
			if err != nil {
				return err
			}
			if !drFromGWExists {
				if err = r.Create(ctx, newExternalHostDestinationRuleFromGateway(host)); err != nil {
					return err
				}
			}

			seExists, err := r.serviceEntryExists(ctx, host.Host)
			if err != nil {
				return err
			}
			if !seExists {
				if err := r.createServiceEntry(ctx, newHTTPSServiceEntry(host)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) EnsureCloudSQLHosts(ctx context.Context, serviceAccounts []uniontypes.ServiceAccount) error {
	for _, sa := range serviceAccounts {
		for _, cloudSQL := range sa.CloudSQL {
			if err := r.ensureGatewayContainsHost(ctx, cloudSQLGatewayName, cloudSQL.Host()); err != nil {
				return err
			}

			vsExists, err := r.virtualServiceExists(ctx, cloudSQL.Name())
			if err != nil {
				return err
			}

			if !vsExists {
				if err = r.Create(ctx, newVirtualServiceForCloudSQLHost(cloudSQL)); err != nil {
					return err
				}
			}

			drToGWExists, err := r.destinationRuleToGatewayExists(ctx, cloudSQL.Host())
			if err != nil {
				return err
			}
			if !drToGWExists {
				if err = r.Create(ctx, newCloudSQLDestinationRuleToGateway(cloudSQL)); err != nil {
					return err
				}
			}

			drFromGWExists, err := r.destinationRuleFromGatewayExists(ctx, cloudSQL.Host())
			if err != nil {
				return err
			}
			if !drFromGWExists {
				if err = r.Create(ctx, newCloudSQLDestinationRuleFromGateway(cloudSQL)); err != nil {
					return err
				}
			}

			seExists, err := r.serviceEntryExists(ctx, cloudSQL.Host())
			if err != nil {
				return err
			}
			if !seExists {
				if err := r.createServiceEntry(ctx, newCloudSQLServiceEntry(cloudSQL)); err != nil {
					return err
				}
			}

		}
	}

	return nil
}

func (r *Reconciler) EnsureAuthorizationPolicies(ctx context.Context, serviceAccounts []uniontypes.ServiceAccount) error {
	for _, sa := range serviceAccounts {
		for _, host := range sa.ExternalAllowlist {
			if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &authPolicyData{Host: host.Host, HostName: host.Host, Paths: host.Paths}, nil, httpsProtocol, hostTypeLabelExternal); err != nil {
				return err
			}
		}

		for _, host := range sa.CloudSQL {
			if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &authPolicyData{Host: host.IP, HostName: host.Host()}, nil, tcpProtocol, hostTypeLabelExternal); err != nil {
				return err
			}
		}

		for _, host := range sa.InternalAllowlist {
			if hostData, ok := r.OnpremHosts[host.Host]; ok {
				if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &authPolicyData{Host: host.Host, HostName: host.Host, Paths: host.Paths}, nil, hostData.Protocol, hostTypeLabelInternal); err != nil {
					return err
				}
				for _, vip := range hostData.VIP {
					vipHost := datanavnov1.Host{Host: vip}
					if err := r.ensureAuthorizationPolicyForHost(ctx, sa, &authPolicyData{Host: vipHost.Host, HostName: vipHost.Host, Paths: vipHost.Paths}, &host.Host, hostData.Protocol, hostTypeLabelInternal); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
