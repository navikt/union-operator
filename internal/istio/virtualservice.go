package istio

import (
	"context"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *Reconciler) virtualServiceExists(ctx context.Context, hostName string) (bool, error) {
	vs := &istionetworking.VirtualService{}
	err := r.Get(ctx, types.NamespacedName{Name: hostName, Namespace: EgressNamespace}, vs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func newVirtualServiceForExternalHost(host *datanavnov1.Host) *istionetworking.VirtualService {
	return &istionetworking.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"host":         host.Host,
				managedByLabel: managedByValue,
			},
		},
		Spec: istionetworkingmodels.VirtualService{
			Hosts: []string{host.Host},
			Gateways: []string{
				meshGatewayName,
				externalGatewayName,
			},
			Http: []*istionetworkingmodels.HTTPRoute{
				{
					Match: []*istionetworkingmodels.HTTPMatchRequest{
						{
							Gateways: []string{meshGatewayName},
							Port:     80,
						},
					},
					Route: []*istionetworkingmodels.HTTPRouteDestination{
						{
							Destination: &istionetworkingmodels.Destination{
								Host: externalGatewayHost,
								Port: &istionetworkingmodels.PortSelector{
									Number: 80,
								},
								Subset: host.Name(),
							},
						},
					},
				},
				{
					Match: []*istionetworkingmodels.HTTPMatchRequest{
						{
							Gateways: []string{externalGatewayName},
							Port:     80,
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

func newVirtualServiceForCloudSQLHost(cloudSQL datanavnov1.CloudSQLInstance) *istionetworking.VirtualService {
	return &istionetworking.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      cloudSQL.Name(),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"host":         cloudSQL.Host(),
				managedByLabel: managedByValue,
			},
		},
		Spec: istionetworkingmodels.VirtualService{
			Hosts: []string{cloudSQL.Host()},
			Gateways: []string{
				meshGatewayName,
				cloudSQLGatewayName,
			},
			Tcp: []*istionetworkingmodels.TCPRoute{
				{
					Match: []*istionetworkingmodels.L4MatchAttributes{
						{
							Gateways: []string{meshGatewayName},
							Port:     3307,
						},
					},
					Route: []*istionetworkingmodels.RouteDestination{
						{
							Destination: &istionetworkingmodels.Destination{
								Host: cloudSQLGatewayHost,
								Port: &istionetworkingmodels.PortSelector{
									Number: 3307,
								},
								Subset: cloudSQL.Name(),
							},
						},
					},
				},
				{
					Match: []*istionetworkingmodels.L4MatchAttributes{
						{
							Gateways: []string{cloudSQLGatewayName},
							Port:     3307,
						},
					},
					Route: []*istionetworkingmodels.RouteDestination{
						{
							Destination: &istionetworkingmodels.Destination{
								Host: cloudSQL.Host(),
								Port: &istionetworkingmodels.PortSelector{
									Number: 3307,
								},
							},
						},
					},
				},
			},
		},
	}
}
