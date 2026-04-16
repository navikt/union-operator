package istio

import (
	"context"
	"errors"
	"maps"
	"slices"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) CleanupUnusedHosts(ctx context.Context) error {
	var utsas datanavnov1.UnionTeamServiceAccountsList
	if err := r.List(ctx, &utsas); err != nil {
		return err
	}

	if err := r.cleanupInternalHosts(ctx, utsas); err != nil {
		return err
	}

	if err := r.cleanupExternalHosts(ctx, utsas); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) cleanupInternalHosts(ctx context.Context, utsas datanavnov1.UnionTeamServiceAccountsList) error {
	for _, utsa := range utsas.Items {
		for _, sa := range utsa.Spec.ServiceAccounts {
			hosts := &istiosecurity.AuthorizationPolicyList{}
			err := r.List(ctx, hosts, inEgressNamespace(), client.MatchingLabels{
				"project":         utsa.Spec.Project,
				"domain":          utsa.Spec.Domain,
				"service-account": sa.Name,
				"host-type":       hostTypeLabelInternal,
			})
			if err != nil {
				return err
			}

			for _, host := range hosts.Items {
				if !containsHost(host, sa.InternalAllowlist) {
					if err := r.Delete(ctx, host); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func containsHost(existing *istiosecurity.AuthorizationPolicy, internalAllowlist []datanavnov1.Host) bool {
	for _, host := range internalAllowlist {
		if existing.Labels["host"] == host.Host {
			return true
		}
	}
	return false
}

func (r *Reconciler) cleanupExternalHosts(ctx context.Context, utsas datanavnov1.UnionTeamServiceAccountsList) error {
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
	err := r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: EgressNamespace}, gateway)
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
				if _, ok := externalHostsInUse[host]; !ok {
					if err := r.removeExternalHost(ctx, host); err != nil {
						return err
					}
				}
			}
			hosts := slices.Collect(maps.Keys(externalHostsInUse))
			slices.Sort(hosts)
			server.Hosts = hosts

			if len(hosts) == 0 {
				return r.Delete(ctx, gateway)
			}

			if err := r.Update(ctx, gateway); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler) removeExternalHost(ctx context.Context, host string) error {
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

func (r *Reconciler) removeByHostLabel(ctx context.Context, host string, list client.ObjectList) error {
	if err := r.List(ctx, list, inEgressNamespace(), matchingHostLabel(host)); err != nil {
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
