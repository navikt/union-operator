package istio

import (
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
)

func TestNewVirtualServiceForExternalHost(t *testing.T) {
	host := datanavnov1.Host{Host: "vg.no"}
	vs := newVirtualServiceForExternalHost(&host)

	if vs.Name != host.Name() {
		t.Errorf("expected name %q, got %q", host.Name(), vs.Name)
	}
	if vs.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, vs.Namespace)
	}
	if vs.Labels["host"] != host.Host {
		t.Errorf("expected label host=%q, got %q", host.Host, vs.Labels["host"])
	}
	if vs.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, vs.Labels[managedByLabel])
	}
	if len(vs.Spec.Hosts) != 1 || vs.Spec.Hosts[0] != host.Host {
		t.Errorf("expected Hosts=[%q], got %v", host.Host, vs.Spec.Hosts)
	}
	if len(vs.Spec.Gateways) != 2 || vs.Spec.Gateways[0] != meshGatewayName || vs.Spec.Gateways[1] != externalGatewayName {
		t.Errorf("expected Gateways=[%q, %q], got %v", meshGatewayName, externalGatewayName, vs.Spec.Gateways)
	}
	if len(vs.Spec.Http) != 2 {
		t.Fatalf("expected 2 HTTP routes, got %d", len(vs.Spec.Http))
	}

	t.Run("mesh to gateway route uses subset", func(t *testing.T) {
		route := vs.Spec.Http[0]
		if len(route.Match) != 1 || route.Match[0].Gateways[0] != meshGatewayName {
			t.Errorf("expected match on %q, got %v", meshGatewayName, route.Match)
		}
		if len(route.Route) != 1 {
			t.Fatalf("expected 1 route destination, got %d", len(route.Route))
		}
		dest := route.Route[0].Destination
		if dest.Host != externalGatewayHost {
			t.Errorf("expected Destination.Host=%q, got %q", externalGatewayHost, dest.Host)
		}
		if dest.Port.Number != 80 {
			t.Errorf("expected Destination.Port=80, got %d", dest.Port.Number)
		}
		if dest.Subset != host.Name() {
			t.Errorf("expected Destination.Subset=%q, got %q", host.Name(), dest.Subset)
		}
	})

	t.Run("gateway to external route has no subset", func(t *testing.T) {
		route := vs.Spec.Http[1]
		if len(route.Match) != 1 || route.Match[0].Gateways[0] != externalGatewayName {
			t.Errorf("expected match on %q, got %v", externalGatewayName, route.Match)
		}
		if len(route.Route) != 1 {
			t.Fatalf("expected 1 route destination, got %d", len(route.Route))
		}
		dest := route.Route[0].Destination
		if dest.Host != host.Host {
			t.Errorf("expected Destination.Host=%q, got %q", host.Host, dest.Host)
		}
		if dest.Port.Number != 443 {
			t.Errorf("expected Destination.Port=443, got %d", dest.Port.Number)
		}
		if dest.Subset != "" {
			t.Errorf("expected no Destination.Subset, got %q", dest.Subset)
		}
	})
}

// TestSubsetNameConsistency guards against the VirtualService subset reference
// and the to-gateway DestinationRule subset definition drifting apart. If they
// diverge, Istio will silently drop traffic.
func TestSubsetNameConsistency(t *testing.T) {
	host := datanavnov1.Host{Host: "vg.no"}
	dr := newExternalHostDestinationRuleToGateway(host)
	vs := newVirtualServiceForExternalHost(&host)

	drSubset := dr.Spec.Subsets[0].Name
	vsSubset := vs.Spec.Http[0].Route[0].Destination.Subset

	if drSubset != vsSubset {
		t.Errorf("subset name mismatch: to-gateway DR defines %q but VirtualService references %q", drSubset, vsSubset)
	}
}

func TestNewVirtualServiceForCloudSQLHost(t *testing.T) {
	cloudSQL := datanavnov1.CloudSQLInstance{IP: "34.32.34.32"}
	vs := newVirtualServiceForCloudSQLHost(cloudSQL)

	if vs.Name != cloudSQL.Name() {
		t.Errorf("expected name %q, got %q", cloudSQL.Name(), vs.Name)
	}
	if vs.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, vs.Namespace)
	}
	if vs.Labels["host"] != cloudSQL.Host() {
		t.Errorf("expected label host=%q, got %q", cloudSQL.Host(), vs.Labels["host"])
	}
	if vs.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, vs.Labels[managedByLabel])
	}
	if len(vs.Spec.Hosts) != 1 || vs.Spec.Hosts[0] != cloudSQL.Host() {
		t.Errorf("expected Hosts=[%q], got %v", cloudSQL.Host(), vs.Spec.Hosts)
	}
	if len(vs.Spec.Gateways) != 2 || vs.Spec.Gateways[0] != meshGatewayName || vs.Spec.Gateways[1] != cloudSQLGatewayName {
		t.Errorf("expected Gateways=[%q, %q], got %v", meshGatewayName, cloudSQLGatewayName, vs.Spec.Gateways)
	}
	if len(vs.Spec.Tcp) != 2 {
		t.Fatalf("expected 2 TCP routes, got %d", len(vs.Spec.Tcp))
	}

	t.Run("mesh to gateway route uses subset", func(t *testing.T) {
		route := vs.Spec.Tcp[0]
		if len(route.Match) != 1 || route.Match[0].Gateways[0] != meshGatewayName {
			t.Errorf("expected match on %q, got %v", meshGatewayName, route.Match)
		}
		if len(route.Route) != 1 {
			t.Fatalf("expected 1 route destination, got %d", len(route.Route))
		}
		dest := route.Route[0].Destination
		if dest.Host != cloudSQLGatewayHost {
			t.Errorf("expected Destination.Host=%q, got %q", cloudSQLGatewayHost, dest.Host)
		}
		if dest.Port.Number != cloudSQLPort {
			t.Errorf("expected Destination.Port=%d, got %d", cloudSQLPort, dest.Port.Number)
		}
		if dest.Subset != cloudSQL.Name() {
			t.Errorf("expected Destination.Subset=%q, got %q", cloudSQL.Name(), dest.Subset)
		}
	})

	t.Run("gateway to external route has no subset", func(t *testing.T) {
		route := vs.Spec.Tcp[1]
		if len(route.Match) != 1 || route.Match[0].Gateways[0] != cloudSQLGatewayName {
			t.Errorf("expected match on %q, got %v", cloudSQLGatewayName, route.Match)
		}
		if len(route.Route) != 1 {
			t.Fatalf("expected 1 route destination, got %d", len(route.Route))
		}
		dest := route.Route[0].Destination
		if dest.Host != cloudSQL.Host() {
			t.Errorf("expected Destination.Host=%q, got %q", cloudSQL.Host(), dest.Host)
		}
		if dest.Port.Number != cloudSQLPort {
			t.Errorf("expected Destination.Port=%d, got %d", cloudSQLPort, dest.Port.Number)
		}
		if dest.Subset != "" {
			t.Errorf("expected no Destination.Subset, got %q", dest.Subset)
		}
	})
}
