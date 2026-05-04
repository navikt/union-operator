package istio

import (
	"context"
	"fmt"
	"slices"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) ensureGatewayContainsHost(ctx context.Context, host datanavnov1.Host) error {
	log := logf.FromContext(ctx)
	gateway := &istionetworking.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: EgressNamespace}, gateway)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, fmt.Sprintf("Failed to get Gateway %s in %s namespace", gatewayName, EgressNamespace))
			return err
		}

		gateway = newGateway(host.Host)
		if err = r.Create(ctx, gateway); err != nil {
			log.Error(err, fmt.Sprintf("Failed to create Gateway %s in %s namespace", gatewayName, EgressNamespace))
			return err
		}

		return nil
	}

	gateway.Spec.Servers[0].Hosts = appendSortedCompact(gateway.Spec.Servers[0].Hosts, host.Host)

	if err := r.Update(ctx, gateway); err != nil {
		log.Error(err, "Failed to patch Gateway", "name", gatewayName)
		return err
	}

	return nil
}

func newGateway(host string) *istionetworking.Gateway {
	return &istionetworking.Gateway{
		ObjectMeta: v1.ObjectMeta{
			Name:      gatewayName,
			Namespace: EgressNamespace,
		},
		Spec: istionetworkingmodels.Gateway{
			Selector: map[string]string{
				"istio": istioGatewaySelector,
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

func inEgressNamespace() client.InNamespace {
	return client.InNamespace(EgressNamespace)
}

func matchingHostLabel(host string) client.MatchingLabels {
	return client.MatchingLabels{"host": host}
}

func appendSortedCompact(slice []string, val string) []string {
	slice = append(slice, val)
	slices.Sort(slice)
	return slices.Compact(slice)
}
