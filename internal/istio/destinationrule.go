package istio

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	"istio.io/api/networking/v1alpha3"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) destinationRuleToGatewayExists(ctx context.Context, hostName string) (bool, error) {
	dr := &istionetworking.DestinationRuleList{}
	err := r.List(ctx, dr, inEgressNamespace(), client.MatchingLabels{
		"host":         hostName,
		"type":         egressToGatewayLabel,
		managedByLabel: managedByValue,
	})
	if err != nil {
		return false, err
	}

	return len(dr.Items) > 0, nil
}

func (r *Reconciler) destinationRuleFromGatewayExists(ctx context.Context, hostName string) (bool, error) {
	dr := &istionetworking.DestinationRuleList{}
	err := r.List(ctx, dr, inEgressNamespace(), client.MatchingLabels{
		"host":         hostName,
		"type":         egressFromGatewayLabel,
		managedByLabel: managedByValue,
	})
	if err != nil {
		return false, err
	}

	return len(dr.Items) > 0, nil
}

func newExternalHostDestinationRuleToGateway(host datanavnov1.Host) *istionetworking.DestinationRule {
	return newDestinationRuleWithSubset(
		fmt.Sprintf("%s-to-gateway", host.Name()),
		egressToGatewayLabel,
		externalGatewayHost,
		host.Host,
		host.Name(),
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: httpPort,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_ISTIO_MUTUAL,
				Sni:  host.Host,
			},
		})
}

func newExternalHostDestinationRuleFromGateway(host datanavnov1.Host) *istionetworking.DestinationRule {
	return newDestinationRule(
		fmt.Sprintf("%s-from-gateway", host.Name()),
		egressFromGatewayLabel,
		host.Host,
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: httpsPort,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_SIMPLE,
				Sni:  host.Host,
			},
		})
}

func newCloudSQLDestinationRuleToGateway(cloudSQL datanavnov1.CloudSQLInstance) *istionetworking.DestinationRule {
	return newDestinationRuleWithSubset(
		fmt.Sprintf("%s-to-gateway", cloudSQL.Name()),
		egressToGatewayLabel,
		cloudSQLGatewayHost,
		cloudSQL.Host(),
		cloudSQL.Name(),
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: cloudSQLPort,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_ISTIO_MUTUAL,
				Sni:  cloudSQL.IP,
			},
		})
}

func newCloudSQLDestinationRuleFromGateway(cloudSQL datanavnov1.CloudSQLInstance) *istionetworking.DestinationRule {
	return newDestinationRule(
		fmt.Sprintf("%s-from-gateway", cloudSQL.Name()),
		egressFromGatewayLabel,
		cloudSQL.Host(),
		&v1alpha3.TrafficPolicy_PortTrafficPolicy{
			Port: &istionetworkingmodels.PortSelector{
				Number: cloudSQLPort,
			},
			Tls: &istionetworkingmodels.ClientTLSSettings{
				Mode: istionetworkingmodels.ClientTLSSettings_DISABLE,
			},
		})
}

func newDestinationRule(name, gatewayLabel, host string, portTrafficPolicy *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworking.DestinationRule {
	return &istionetworking.DestinationRule{
		ObjectMeta: destinationRuleObjectMeta(name, gatewayLabel, host),
		Spec: istionetworkingmodels.DestinationRule{
			Host:          host,
			TrafficPolicy: portLevelTrafficPolicy(portTrafficPolicy),
		},
	}
}

func newDestinationRuleWithSubset(name, gatewayLabel, targetHost, host, hostName string, portTrafficPolicy *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworking.DestinationRule {
	return &istionetworking.DestinationRule{
		ObjectMeta: destinationRuleObjectMeta(name, gatewayLabel, host),
		Spec: istionetworkingmodels.DestinationRule{
			Host: targetHost,
			Subsets: []*istionetworkingmodels.Subset{
				{
					Name:          hostName,
					TrafficPolicy: portLevelTrafficPolicy(portTrafficPolicy),
				},
			},
		},
	}
}

func destinationRuleObjectMeta(name, gatewayLabel string, hostName string) v1.ObjectMeta {
	return v1.ObjectMeta{
		Name:      name,
		Namespace: EgressNamespace,
		Labels:    map[string]string{"host": hostName, "type": gatewayLabel, managedByLabel: managedByValue},
	}
}

func portLevelTrafficPolicy(p *v1alpha3.TrafficPolicy_PortTrafficPolicy) *istionetworkingmodels.TrafficPolicy {
	return &istionetworkingmodels.TrafficPolicy{
		PortLevelSettings: []*v1alpha3.TrafficPolicy_PortTrafficPolicy{p},
	}
}
