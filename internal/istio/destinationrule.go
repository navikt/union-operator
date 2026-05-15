package istio

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	"istio.io/api/networking/v1alpha3"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) ensureDestinationRule(ctx context.Context, sa uniontypes.ServiceAccount, host datanavnov1.Host) error {
	dr := &istionetworking.DestinationRuleList{}
	err := r.List(ctx, dr, inEgressNamespace(), client.MatchingLabels{
		"host": host.Host,
		"type": egressToGatewayLabel,
	})
	if err != nil {
		return err
	}

	if len(dr.Items) < 1 {
		if err = r.Create(ctx, newDestinationRuleToGateway(sa, host)); err != nil {
			return err
		}
	}

	err = r.List(ctx, dr, inEgressNamespace(), client.MatchingLabels{
		"host": host.Host,
		"type": egressFromGatewayLabel,
	})
	if err != nil {
		return err
	}

	if len(dr.Items) < 1 {
		if err = r.Create(ctx, newDestinationRuleFromGateway(sa, host)); err != nil {
			return err
		}
	}

	return nil
}

func newDestinationRuleToGateway(sa uniontypes.ServiceAccount, host datanavnov1.Host) *istionetworking.DestinationRule {
	return newDestinationRule(
		sa,
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

func newDestinationRuleFromGateway(sa uniontypes.ServiceAccount, host datanavnov1.Host) *istionetworking.DestinationRule {
	return newDestinationRule(
		sa,
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
				Sni:  host.Host,
			},
		})
}

func newDestinationRule(sa uniontypes.ServiceAccount, name, gatewayLabel, targetHost string, host datanavnov1.Host, portTrafficPolicy *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworking.DestinationRule {
	return &istionetworking.DestinationRule{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: EgressNamespace,
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
