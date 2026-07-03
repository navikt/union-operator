package istio

import (
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	istiosecuritymodels "istio.io/api/security/v1beta1"
)

const testHostVG = "vg.no"

func testServiceAccount() uniontypes.ServiceAccount {
	return uniontypes.ServiceAccount{
		UnionServiceAccount: datanavnov1.UnionServiceAccount{Name: "my-sa"},
		UnionEnv: &uniontypes.UnionEnv{
			Project: "my-project",
			Domain:  "development",
		},
	}
}

func TestNewAuthorizationPolicyHTTPS(t *testing.T) {
	sa := testServiceAccount()
	host := &datanavnov1.Host{Host: testHostVG}

	ap, err := newAuthorizationPolicy(sa, host, host.Host, "HTTPS", hostTypeLabelExternal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ap.Name != "my-project-development-my-sa-vg-no" {
		t.Errorf("expected name %q, got %q", "my-project-development-my-sa-vg-no", ap.Name)
	}
	if ap.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, ap.Namespace)
	}
	if ap.Labels["project"] != "my-project" {
		t.Errorf("expected label project=my-project, got %q", ap.Labels["project"])
	}
	if ap.Labels["domain"] != "development" {
		t.Errorf("expected label domain=development, got %q", ap.Labels["domain"])
	}
	if ap.Labels["service-account"] != "my-sa" {
		t.Errorf("expected label service-account=my-sa, got %q", ap.Labels["service-account"])
	}
	if ap.Labels["host"] != testHostVG {
		t.Errorf("expected label host=vg.no, got %q", ap.Labels["host"])
	}
	if ap.Labels["parent-host"] != testHostVG {
		t.Errorf("expected label parent-host=vg.no, got %q", ap.Labels["parent-host"])
	}
	if ap.Labels["host-type"] != hostTypeLabelExternal {
		t.Errorf("expected label host-type=%q, got %q", hostTypeLabelExternal, ap.Labels["host-type"])
	}
	if ap.Spec.Action != istiosecuritymodels.AuthorizationPolicy_ALLOW {
		t.Errorf("expected action ALLOW, got %v", ap.Spec.Action)
	}

	if len(ap.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ap.Spec.Rules))
	}
	rule := ap.Spec.Rules[0]

	expectedPrincipal := "cluster.local/ns/my-project-development/sa/my-sa"
	if len(rule.From) != 1 || len(rule.From[0].Source.Principals) != 1 || rule.From[0].Source.Principals[0] != expectedPrincipal {
		t.Errorf("expected principal %q, got %v", expectedPrincipal, rule.From)
	}

	// HTTPS: To is set with the host, When is nil.
	if len(rule.To) != 1 {
		t.Fatalf("expected 1 To rule, got %d", len(rule.To))
	}
	if len(rule.To[0].Operation.Hosts) != 1 || rule.To[0].Operation.Hosts[0] != testHostVG {
		t.Errorf("expected To.Hosts=[vg.no], got %v", rule.To[0].Operation.Hosts)
	}
	if len(rule.To[0].Operation.Paths) != 0 {
		t.Errorf("expected no To.Paths, got %v", rule.To[0].Operation.Paths)
	}
	if len(rule.When) != 0 {
		t.Errorf("expected no When conditions for HTTPS, got %v", rule.When)
	}
}

func TestNewAuthorizationPolicyHTTPSWithPaths(t *testing.T) {
	sa := testServiceAccount()
	host := &datanavnov1.Host{Host: "example.com", Paths: []string{"/api", "/v2"}}

	ap, err := newAuthorizationPolicy(sa, host, host.Host, "HTTPS", hostTypeLabelExternal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := ap.Spec.Rules[0]
	if len(rule.To) != 1 {
		t.Fatalf("expected 1 To rule, got %d", len(rule.To))
	}
	paths := rule.To[0].Operation.Paths
	if len(paths) != 2 || paths[0] != "/api" || paths[1] != "/v2" {
		t.Errorf("expected paths [/api /v2], got %v", paths)
	}
}

func TestNewAuthorizationPolicyTCP(t *testing.T) {
	sa := testServiceAccount()
	host := &datanavnov1.Host{Host: "a01dbfl039.adeo.no"}

	ap, err := newAuthorizationPolicy(sa, host, host.Host, "TCP", hostTypeLabelInternal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := ap.Spec.Rules[0]

	// TCP: When is set with connection.sni, To is nil.
	if len(rule.To) != 0 {
		t.Errorf("expected no To rules for TCP, got %d", len(rule.To))
	}
	if len(rule.When) != 1 {
		t.Fatalf("expected 1 When condition, got %d", len(rule.When))
	}
	if rule.When[0].Key != "connection.sni" {
		t.Errorf("expected When[0].Key=connection.sni, got %q", rule.When[0].Key)
	}
	if len(rule.When[0].Values) != 1 || rule.When[0].Values[0] != host.Host {
		t.Errorf("expected When[0].Values=[%q], got %v", host.Host, rule.When[0].Values)
	}
}

// TestNewAuthorizationPolicyProtocolCaseInsensitive verifies that protocol
// matching is case-insensitive, since the switch uses strings.ToUpper.
func TestNewAuthorizationPolicyProtocolCaseInsensitive(t *testing.T) {
	sa := testServiceAccount()
	host := &datanavnov1.Host{Host: testHostVG}

	for _, proto := range []string{"https", "Https", "HTTPS"} {
		_, err := newAuthorizationPolicy(sa, host, host.Host, proto, hostTypeLabelExternal)
		if err != nil {
			t.Errorf("protocol %q: unexpected error: %v", proto, err)
		}
	}
}

func TestNewAuthorizationPolicyUnknownProtocolReturnsError(t *testing.T) {
	sa := testServiceAccount()
	host := &datanavnov1.Host{Host: "example.com"}

	_, err := newAuthorizationPolicy(sa, host, host.Host, "UDP", hostTypeLabelExternal)
	if err == nil {
		t.Error("expected error for unknown protocol, got nil")
	}
}

// TestNewAuthorizationPolicyVIPHost covers the on-prem VIP case: the host is a
// VIP IP address, but the parent-host label should point to the real hostname.
// This is how the reconciler calls the function for on-prem VIPs.
func TestNewAuthorizationPolicyVIPHost(t *testing.T) {
	sa := testServiceAccount()
	vipHost := &datanavnov1.Host{Host: "10.53.20.91"}
	realHostname := "a01dbfl039.adeo.no"

	ap, err := newAuthorizationPolicy(sa, vipHost, realHostname, "TCP", hostTypeLabelInternal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ap.Labels["host"] != "10.53.20.91" {
		t.Errorf("expected label host=10.53.20.91, got %q", ap.Labels["host"])
	}
	if ap.Labels["parent-host"] != realHostname {
		t.Errorf("expected label parent-host=%q, got %q", realHostname, ap.Labels["parent-host"])
	}

	// SNI match uses the VIP address, not the parent hostname.
	rule := ap.Spec.Rules[0]
	if len(rule.When) != 1 || rule.When[0].Values[0] != "10.53.20.91" {
		t.Errorf("expected SNI=10.53.20.91, got %v", rule.When)
	}
}
