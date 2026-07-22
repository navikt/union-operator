package istio

import (
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
)

func TestNewServiceEntryForExternalHost(t *testing.T) {
	host := datanavnov1.Host{Host: "data.nav.no"}

	se := newHTTPSServiceEntry(host)

	if se.Name != host.Name() {
		t.Errorf("expected name %q, got %q", host.Name(), se.Name)
	}
	if se.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, se.Namespace)
	}
	if se.Labels["host"] != host.Host {
		t.Errorf("expected label host=%q, got %q", host.Host, se.Labels["host"])
	}
	if se.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, se.Labels[managedByLabel])
	}

	if len(se.Spec.Hosts) != 1 || se.Spec.Hosts[0] != host.Host {
		t.Errorf("expected Hosts=[%q], got %v", host.Host, se.Spec.Hosts)
	}
	if len(se.Spec.Ports) != 2 || se.Spec.Ports[0].Number != httpPort || se.Spec.Ports[1].Number != httpsPort {
		t.Errorf("expected Ports=[%d, %d], got %v", httpPort, httpsPort, se.Spec.Ports)
	}
	if len(se.Spec.ExportTo) != 1 || se.Spec.ExportTo[0] != "*" {
		t.Errorf("expected ExportTo=[*], got %v", se.Spec.ExportTo)
	}
	if se.Spec.Resolution != istionetworkingmodels.ServiceEntry_DNS {
		t.Errorf("expected Resolution=DNS, got %v", se.Spec.Resolution)
	}
}

func TestNewServiceEntryForCloudSQLHost(t *testing.T) {
	cloudSQLHost := datanavnov1.CloudSQLInstance{IP: "34.32.34.32"}

	se := newCloudSQLServiceEntry(cloudSQLHost)

	if se.Name != cloudSQLHost.Name() {
		t.Errorf("expected name %q, got %q", cloudSQLHost.Name(), se.Name)
	}
	if se.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, se.Namespace)
	}
	if se.Labels["host"] != cloudSQLHost.Host() {
		t.Errorf("expected label host=%q, got %q", cloudSQLHost.Host(), se.Labels["host"])
	}
	if se.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, se.Labels[managedByLabel])
	}

	if len(se.Spec.Hosts) != 1 || se.Spec.Hosts[0] != cloudSQLHost.Host() {
		t.Errorf("expected Hosts=[%q], got %v", cloudSQLHost.Host(), se.Spec.Hosts)
	}
	if len(se.Spec.Addresses) != 1 || se.Spec.Addresses[0] != cloudSQLHost.IP {
		t.Errorf("expected Addresses=[%q], got %v", cloudSQLHost.IP, se.Spec.Addresses)
	}
	if len(se.Spec.Endpoints) != 1 || se.Spec.Endpoints[0].Address != cloudSQLHost.IP {
		t.Errorf("expected Endpoints=[%q], got %v", cloudSQLHost.IP, se.Spec.Endpoints)
	}

	if len(se.Spec.Ports) != 1 || se.Spec.Ports[0].Number != cloudSQLPort {
		t.Errorf("expected Ports=[%d], got %v", cloudSQLPort, se.Spec.Ports)
	}
	if len(se.Spec.ExportTo) != 1 || se.Spec.ExportTo[0] != "*" {
		t.Errorf("expected ExportTo=[*], got %v", se.Spec.ExportTo)
	}
	if se.Spec.Resolution != istionetworkingmodels.ServiceEntry_STATIC {
		t.Errorf("expected Resolution=%s, got %v", istionetworkingmodels.ServiceEntry_STATIC, se.Spec.Resolution)
	}
}
