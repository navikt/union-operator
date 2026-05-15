package istio

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) ensureVirtualServiceForHost(ctx context.Context, sa uniontypes.ServiceAccount, host *datanavnov1.Host) error {
	log := logf.FromContext(ctx)

	vs := &istionetworking.VirtualService{}
	err := r.Get(ctx, types.NamespacedName{Name: host.Name(), Namespace: EgressNamespace}, vs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			vs = newVirtualServiceForHost(sa, host)
			if err = r.Create(ctx, vs); err != nil {
				log.Error(err, fmt.Sprintf("Failed to create VirtualService %s in %s namespace", host.Name(), EgressNamespace))
				return err
			}
			return nil
		}
		log.Error(err, fmt.Sprintf("Failed to get VirtualService %s in %s namespace", host.Name(), EgressNamespace))
		return err
	}

	return nil
}

func newVirtualServiceForHost(sa uniontypes.ServiceAccount, host *datanavnov1.Host) *istionetworking.VirtualService {
	return &istionetworking.VirtualService{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: EgressNamespace,
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
							Gateways: []string{meshGatewayName},
							Port:     80,
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
							Gateways: []string{gatewayName},
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
