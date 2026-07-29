package istio

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) serviceEntryExists(ctx context.Context, hostLabel string) (bool, error) {
	log := logf.FromContext(ctx)
	seList := &istionetworking.ServiceEntryList{}
	err := r.List(ctx, seList, inEgressNamespace(), matchingHostLabel(hostLabel))
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to list ServiceEntries in %s namespace", EgressNamespace))
		return false, err
	}

	return len(seList.Items) > 0, nil
}

func (r *Reconciler) createServiceEntry(ctx context.Context, se *istionetworking.ServiceEntry) error {
	return r.Create(ctx, se)
}

func newHTTPSServiceEntry(host datanavnov1.Host) *istionetworking.ServiceEntry {
	return &istionetworking.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      host.Name(),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"host":         host.Host,
				managedByLabel: managedByValue,
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

func newCloudSQLServiceEntry(cloudSQLInstance datanavnov1.CloudSQLInstance) *istionetworking.ServiceEntry {
	return &istionetworking.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      cloudSQLInstance.Name(),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"host":         cloudSQLInstance.Host(),
				managedByLabel: managedByValue,
			},
		},
		Spec: istionetworkingmodels.ServiceEntry{
			Hosts:     []string{cloudSQLInstance.Host()},
			Addresses: []string{cloudSQLInstance.IP},
			Endpoints: []*istionetworkingmodels.WorkloadEntry{
				{
					Address: cloudSQLInstance.IP,
				},
			},
			Ports: []*istionetworkingmodels.ServicePort{
				{
					Number:   3307,
					Name:     "tcp-3307",
					Protocol: "TCP",
				},
			},
			Resolution: istionetworkingmodels.ServiceEntry_STATIC,
			ExportTo:   []string{"*"},
		},
	}
}
