package istio

import (
	"fmt"
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	istionetworkingmodels "istio.io/api/networking/v1beta1"
)

func TestNewExternalHostDestinationRuleToGateway(t *testing.T) {
	host := datanavnov1.Host{Host: "vg.no"}
	dr := newExternalHostDestinationRuleToGateway(host)

	if dr.Name != "vg-no-to-gateway" {
		t.Errorf("expected name %q, got %q", "vg-no-to-gateway", dr.Name)
	}
	if dr.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, dr.Namespace)
	}
	if dr.Labels["host"] != host.Host {
		t.Errorf("expected label host=%q, got %q", host.Host, dr.Labels["host"])
	}
	if dr.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, dr.Labels[managedByLabel])
	}
	if dr.Labels["type"] != egressToGatewayLabel {
		t.Errorf("expected label type=%q, got %q", egressToGatewayLabel, dr.Labels["type"])
	}

	// Target is the egress gateway service, not the external host.
	if dr.Spec.Host != externalGatewayHost {
		t.Errorf("expected Spec.Host %q, got %q", externalGatewayHost, dr.Spec.Host)
	}

	// Traffic policy must be inside the subset, not at top level.
	if dr.Spec.TrafficPolicy != nil {
		t.Error("expected no top-level TrafficPolicy, got one")
	}
	if len(dr.Spec.Subsets) != 1 {
		t.Fatalf("expected 1 subset, got %d", len(dr.Spec.Subsets))
	}

	subset := dr.Spec.Subsets[0]
	if subset.Name != host.Name() {
		t.Errorf("expected subset name %q, got %q", host.Name(), subset.Name)
	}
	if subset.TrafficPolicy == nil {
		t.Fatal("expected subset TrafficPolicy, got nil")
	}
	if len(subset.TrafficPolicy.PortLevelSettings) != 1 {
		t.Fatalf("expected 1 PortLevelSetting, got %d", len(subset.TrafficPolicy.PortLevelSettings))
	}

	ps := subset.TrafficPolicy.PortLevelSettings[0]
	if ps.Port.Number != 80 {
		t.Errorf("expected port 80, got %d", ps.Port.Number)
	}
	if ps.Tls.Mode != istionetworkingmodels.ClientTLSSettings_ISTIO_MUTUAL {
		t.Errorf("expected ISTIO_MUTUAL, got %v", ps.Tls.Mode)
	}
	if ps.Tls.Sni != host.Host {
		t.Errorf("expected SNI %q, got %q", host.Host, ps.Tls.Sni)
	}
}

func TestNewExternalHostDestinationRuleFromGateway(t *testing.T) {
	host := datanavnov1.Host{Host: "vg.no"}
	dr := newExternalHostDestinationRuleFromGateway(host)

	if dr.Name != "vg-no-from-gateway" {
		t.Errorf("expected name %q, got %q", "vg-no-from-gateway", dr.Name)
	}
	if dr.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, dr.Namespace)
	}
	if dr.Labels["host"] != host.Host {
		t.Errorf("expected label host=%q, got %q", host.Host, dr.Labels["host"])
	}
	if dr.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, dr.Labels[managedByLabel])
	}
	if dr.Labels["type"] != egressFromGatewayLabel {
		t.Errorf("expected label type=%q, got %q", egressFromGatewayLabel, dr.Labels["type"])
	}

	// Target is the external host, not the gateway service.
	if dr.Spec.Host != host.Host {
		t.Errorf("expected Spec.Host %q, got %q", host.Host, dr.Spec.Host)
	}

	// No subsets; traffic policy lives at the top level.
	if len(dr.Spec.Subsets) != 0 {
		t.Errorf("expected no subsets, got %d", len(dr.Spec.Subsets))
	}
	if dr.Spec.TrafficPolicy == nil {
		t.Fatal("expected top-level TrafficPolicy, got nil")
	}
	if len(dr.Spec.TrafficPolicy.PortLevelSettings) != 1 {
		t.Fatalf("expected 1 PortLevelSetting, got %d", len(dr.Spec.TrafficPolicy.PortLevelSettings))
	}

	ps := dr.Spec.TrafficPolicy.PortLevelSettings[0]
	if ps.Port.Number != 443 {
		t.Errorf("expected port 443, got %d", ps.Port.Number)
	}
	if ps.Tls.Mode != istionetworkingmodels.ClientTLSSettings_SIMPLE {
		t.Errorf("expected SIMPLE, got %v", ps.Tls.Mode)
	}
	if ps.Tls.Sni != host.Host {
		t.Errorf("expected SNI %q, got %q", host.Host, ps.Tls.Sni)
	}
}

func TestNewCloudSQLHostDestinationRuleToGateway(t *testing.T) {
	cloudSQL := datanavnov1.CloudSQLInstance{IP: "34.32.34.32"}
	dr := newCloudSQLDestinationRuleToGateway(cloudSQL)

	if dr.Name != fmt.Sprintf("%s-to-gateway", cloudSQL.Name()) {
		t.Errorf("expected name %q, got %q", fmt.Sprintf("%s-to-gateway", cloudSQL.Name()), dr.Name)
	}
	if dr.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, dr.Namespace)
	}
	if dr.Labels["host"] != cloudSQL.Host() {
		t.Errorf("expected label host=%q, got %q", cloudSQL.Host(), dr.Labels["host"])
	}
	if dr.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, dr.Labels[managedByLabel])
	}
	if dr.Labels["type"] != egressToGatewayLabel {
		t.Errorf("expected label type=%q, got %q", egressToGatewayLabel, dr.Labels["type"])
	}

	// Target is the egress gateway service, not the external host.
	if dr.Spec.Host != cloudSQLGatewayHost {
		t.Errorf("expected Spec.Host %q, got %q", cloudSQLGatewayHost, dr.Spec.Host)
	}

	// Traffic policy must be inside the subset, not at top level.
	if dr.Spec.TrafficPolicy != nil {
		t.Error("expected no top-level TrafficPolicy, got one")
	}
	if len(dr.Spec.Subsets) != 1 {
		t.Fatalf("expected 1 subset, got %d", len(dr.Spec.Subsets))
	}

	subset := dr.Spec.Subsets[0]
	if subset.Name != cloudSQL.Name() {
		t.Errorf("expected subset name %q, got %q", cloudSQL.Name(), subset.Name)
	}
	if subset.TrafficPolicy == nil {
		t.Fatal("expected subset TrafficPolicy, got nil")
	}
	if len(subset.TrafficPolicy.PortLevelSettings) != 1 {
		t.Fatalf("expected 1 PortLevelSetting, got %d", len(subset.TrafficPolicy.PortLevelSettings))
	}

	ps := subset.TrafficPolicy.PortLevelSettings[0]
	if ps.Port.Number != cloudSQLPort {
		t.Errorf("expected port %d, got %d", cloudSQLPort, ps.Port.Number)
	}
	if ps.Tls.Mode != istionetworkingmodels.ClientTLSSettings_ISTIO_MUTUAL {
		t.Errorf("expected ISTIO_MUTUAL, got %v", ps.Tls.Mode)
	}
	if ps.Tls.Sni != cloudSQL.IP {
		t.Errorf("expected SNI %q, got %q", cloudSQL.IP, ps.Tls.Sni)
	}
}

func TestNewCloudSQLHostDestinationRuleFromGateway(t *testing.T) {
	cloudSQL := datanavnov1.CloudSQLInstance{IP: "34.32.34.32"}
	dr := newCloudSQLDestinationRuleFromGateway(cloudSQL)

	if dr.Name != fmt.Sprintf("%s-from-gateway", cloudSQL.Name()) {
		t.Errorf("expected name %q, got %q", fmt.Sprintf("%s-from-gateway", cloudSQL.Name()), dr.Name)
	}
	if dr.Namespace != EgressNamespace {
		t.Errorf("expected namespace %q, got %q", EgressNamespace, dr.Namespace)
	}
	if dr.Labels["host"] != cloudSQL.Host() {
		t.Errorf("expected label host=%q, got %q", cloudSQL.Host(), dr.Labels["host"])
	}
	if dr.Labels[managedByLabel] != managedByValue {
		t.Errorf("expected label %q=%q, got %q", managedByLabel, managedByValue, dr.Labels[managedByLabel])
	}
	if dr.Labels["type"] != egressFromGatewayLabel {
		t.Errorf("expected label type=%q, got %q", egressFromGatewayLabel, dr.Labels["type"])
	}

	// Target is the external host, not the gateway service.
	if dr.Spec.Host != cloudSQL.Host() {
		t.Errorf("expected Spec.Host %q, got %q", cloudSQL.Host(), dr.Spec.Host)
	}

	// No subsets; traffic policy lives at the top level.
	if len(dr.Spec.Subsets) != 0 {
		t.Errorf("expected no subsets, got %d", len(dr.Spec.Subsets))
	}
	if dr.Spec.TrafficPolicy == nil {
		t.Fatal("expected top-level TrafficPolicy, got nil")
	}
	if len(dr.Spec.TrafficPolicy.PortLevelSettings) != 1 {
		t.Fatalf("expected 1 PortLevelSetting, got %d", len(dr.Spec.TrafficPolicy.PortLevelSettings))
	}

	ps := dr.Spec.TrafficPolicy.PortLevelSettings[0]
	if ps.Port.Number != cloudSQLPort {
		t.Errorf("expected port %d, got %d", cloudSQLPort, ps.Port.Number)
	}
	if ps.Tls.Mode != istionetworkingmodels.ClientTLSSettings_DISABLE {
		t.Errorf("expected DISABLE, got %v", ps.Tls.Mode)
	}
}
