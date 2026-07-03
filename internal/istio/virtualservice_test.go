package istio

import (
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
)

func TestNewVirtualServiceForHost(t *testing.T) {
	host := datanavnov1.Host{Host: "vg.no"}
	vs := newVirtualServiceForHost(&host)

	if vs.Name != host.Name() {
		t.Errorf("expected name %q, got %q", host.Name(), vs.Name)
	}
	if vs.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, vs.Namespace)
	}
	if vs.Labels["host"] != host.Host {
		t.Errorf("expected label host=%q, got %q", host.Host, vs.Labels["host"])
	}
	if len(vs.Spec.Hosts) != 1 || vs.Spec.Hosts[0] != host.Host {
		t.Errorf("expected Hosts=[%q], got %v", host.Host, vs.Spec.Hosts)
	}
	if len(vs.Spec.Gateways) != 2 || vs.Spec.Gateways[0] != meshGatewayName || vs.Spec.Gateways[1] != gatewayName {
		t.Errorf("expected Gateways=[%q, %q], got %v", meshGatewayName, gatewayName, vs.Spec.Gateways)
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
		if dest.Host != gatewayHost {
			t.Errorf("expected Destination.Host=%q, got %q", gatewayHost, dest.Host)
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
		if len(route.Match) != 1 || route.Match[0].Gateways[0] != gatewayName {
			t.Errorf("expected match on %q, got %v", gatewayName, route.Match)
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
	dr := newDestinationRuleToGateway(host)
	vs := newVirtualServiceForHost(&host)

	drSubset := dr.Spec.Subsets[0].Name
	vsSubset := vs.Spec.Http[0].Route[0].Destination.Subset

	if drSubset != vsSubset {
		t.Errorf("subset name mismatch: to-gateway DR defines %q but VirtualService references %q", drSubset, vsSubset)
	}
}
