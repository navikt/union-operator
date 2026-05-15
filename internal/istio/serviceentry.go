package istio

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) serviceEntryExists(ctx context.Context, host datanavnov1.Host) (bool, error) {
	log := logf.FromContext(ctx)
	seList := &istionetworking.ServiceEntryList{}
	err := r.List(ctx, seList, inEgressNamespace(), matchingHostLabel(host.Host))
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list ServiceEntries in %s namespace", EgressNamespace))
		return false, err
	}

	return len(seList.Items) > 0, nil
}

func (r *Reconciler) createServiceEntry(ctx context.Context, sa uniontypes.ServiceAccount, host datanavnov1.Host) error {
	se := newHTTPSServiceEntry(sa, host)
	return r.Create(ctx, se)
}

func newHTTPSServiceEntry(sa uniontypes.ServiceAccount, host datanavnov1.Host) *istionetworking.ServiceEntry {
	return &istionetworking.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: EgressNamespace,
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
