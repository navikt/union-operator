package istio

import (
	"context"
	"strings"
	"testing"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const longTestHost = "tenant-with-a-deliberately-long-identifier.environment.api.example.com"

func TestHostResourceMetadataAndRouting(t *testing.T) {
	for _, value := range []string{"api.example.com", strings.Repeat("a", 51) + ".example.com", longTestHost} {
		t.Run(value, func(t *testing.T) {
			host := datanavnov1.Host{Host: value, Paths: []string{"/api/*"}}
			se := newHTTPSServiceEntry(host)
			vs := newVirtualServiceForExternalHost(&host)
			to := newExternalHostDestinationRuleToGateway(host)
			from := newExternalHostDestinationRuleFromGateway(host)
			ap, err := newAuthorizationPolicy(testServiceAccount(), &authPolicyData{Host: value, HostName: value, Paths: host.Paths}, value, httpsProtocol, hostTypeLabelExternal)
			if err != nil {
				t.Fatal(err)
			}
			for _, obj := range []client.Object{se, vs, to, from, ap} {
				if errs := validation.IsDNS1123Subdomain(obj.GetName()); len(errs) != 0 {
					t.Fatalf("invalid name %q: %v", obj.GetName(), errs)
				}
				for key, label := range obj.GetLabels() {
					if errs := validation.IsValidLabelValue(label); len(errs) != 0 {
						t.Fatalf("invalid %s label %q: %v", key, label, errs)
					}
				}
				if obj.GetLabels()["host"] != host.LabelValue() {
					t.Fatalf("incorrect host label on %q", obj.GetName())
				}
				if len(value) > 63 {
					if obj.GetAnnotations()[hostAnnotation] != value {
						t.Fatalf("full hostname missing on %q", obj.GetName())
					}
				} else if len(obj.GetAnnotations()) != 0 {
					t.Fatalf("changed short-host annotations on %q", obj.GetName())
				}
			}
			if len(value) <= 63 {
				legacyName := strings.ReplaceAll(value, ".", "-")
				if se.Name != legacyName || vs.Name != legacyName || to.Name != legacyName+"-to-gateway" || from.Name != legacyName+"-from-gateway" {
					t.Fatal("changed short-host resource names")
				}
			}
			if se.Spec.Hosts[0] != value || vs.Spec.Hosts[0] != value || from.Spec.Host != value || vs.Spec.Http[1].Route[0].Destination.Host != value {
				t.Fatal("replaced a routing hostname with an identifier")
			}
			if to.Spec.Subsets[0].Name != host.Name() || vs.Spec.Http[0].Route[0].Destination.Subset != to.Spec.Subsets[0].Name {
				t.Fatal("subset names do not agree")
			}
			if errs := validation.IsDNS1123Label(to.Spec.Subsets[0].Name); len(errs) != 0 {
				t.Fatalf("invalid subset name: %v", errs)
			}
			if to.Spec.Subsets[0].TrafficPolicy.PortLevelSettings[0].Tls.Sni != value || from.Spec.TrafficPolicy.PortLevelSettings[0].Tls.Sni != value {
				t.Fatal("replaced the TLS SNI hostname")
			}
			operation := ap.Spec.Rules[0].To[0].Operation
			if operation.Hosts[0] != value || operation.Paths[0] != "/api/*" || ap.Labels["parent-host"] != host.LabelValue() {
				t.Fatal("incorrect authorization policy host, paths or parent label")
			}
		})
	}
}

func hostTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{datanavnov1.AddToScheme, istionetworking.AddToScheme, istiosecurity.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func assertHostResourceCount(t *testing.T, c client.Client, host string, want int) {
	t.Helper()
	for _, list := range []client.ObjectList{&istionetworking.ServiceEntryList{}, &istionetworking.VirtualServiceList{}, &istionetworking.DestinationRuleList{}, &istiosecurity.AuthorizationPolicyList{}} {
		if err := c.List(context.Background(), list, inEgressNamespace(), matchingHostLabel(host)); err != nil {
			t.Fatal(err)
		}
		count := 0
		switch list := list.(type) {
		case *istionetworking.ServiceEntryList:
			count = len(list.Items)
		case *istionetworking.VirtualServiceList:
			count = len(list.Items)
		case *istionetworking.DestinationRuleList:
			count = len(list.Items)
		case *istiosecurity.AuthorizationPolicyList:
			count = len(list.Items)
		}
		expected := want
		if _, ok := list.(*istionetworking.DestinationRuleList); ok {
			expected *= 2
		}
		if count != expected {
			t.Fatalf("%T for %q: got %d, want %d", list, host, count, expected)
		}
	}
}

func TestLongExternalHostLookupAndCleanup(t *testing.T) {
	ctx := context.Background()
	sa := testServiceAccount()
	sa.ExternalAllowlist = []datanavnov1.Host{{Host: "api.example.com"}, {Host: longTestHost}}
	utsa := &datanavnov1.UnionTeamServiceAccounts{
		ObjectMeta: metav1.ObjectMeta{Name: "host-test", Namespace: "default"},
		Spec:       datanavnov1.UnionTeamServiceAccountsSpec{Project: sa.Project, Domain: sa.Domain, ServiceAccounts: []datanavnov1.UnionServiceAccount{sa.UnionServiceAccount}},
	}
	c := hostTestClient(t, utsa)
	r := Reconciler{Client: c}
	for range 2 {
		if err := r.EnsureExternalHosts(ctx, []uniontypes.ServiceAccount{sa}); err != nil {
			t.Fatal(err)
		}
		if err := r.EnsureAuthorizationPolicies(ctx, []uniontypes.ServiceAccount{sa}); err != nil {
			t.Fatal(err)
		}
	}
	assertHostResourceCount(t, c, longTestHost, 1)
	assertHostResourceCount(t, c, "api.example.com", 1)

	// A second UTSA still using the long host must keep the shared resources alive.
	shared := utsa.DeepCopy()
	shared.Name = "shared-host-test"
	shared.ResourceVersion = ""
	shared.Spec.Project = "other-project"
	shared.Spec.ServiceAccounts[0].ExternalAllowlist = []datanavnov1.Host{{Host: longTestHost}}
	if err := c.Create(ctx, shared); err != nil {
		t.Fatal(err)
	}
	utsa.Spec.ServiceAccounts[0].ExternalAllowlist = []datanavnov1.Host{{Host: "api.example.com"}}
	if err := c.Update(ctx, utsa); err != nil {
		t.Fatal(err)
	}
	if err := r.CleanupUnusedHosts(ctx); err != nil {
		t.Fatal(err)
	}
	assertHostResourceCount(t, c, longTestHost, 1)
	if err := c.Delete(ctx, shared); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := r.CleanupUnusedHosts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertHostResourceCount(t, c, longTestHost, 0)
	assertHostResourceCount(t, c, "api.example.com", 1)
	gateway := &istionetworking.Gateway{}
	if err := c.Get(ctx, types.NamespacedName{Name: externalGatewayName, Namespace: EgressNamespace}, gateway); err != nil {
		t.Fatal(err)
	}
	if hosts := gateway.Spec.Servers[0].Hosts; len(hosts) != 1 || hosts[0] != "api.example.com" {
		t.Fatalf("incorrect remaining gateway hosts: %v", hosts)
	}
}

func TestLongInternalParentHostCleanup(t *testing.T) {
	ctx := context.Background()
	sa := testServiceAccount()
	sa.InternalAllowlist = []datanavnov1.Host{{Host: longTestHost}, {Host: "api.example.com"}}
	r := Reconciler{Client: hostTestClient(t), OnpremHosts: uniontypes.OnpremHostMap{
		longTestHost:      {Protocol: tcpProtocol, VIP: []string{"192.0.2.10"}},
		"api.example.com": {Protocol: httpsProtocol},
	}}
	for range 2 {
		if err := r.EnsureAuthorizationPolicies(ctx, []uniontypes.ServiceAccount{sa}); err != nil {
			t.Fatal(err)
		}
	}
	list := &istiosecurity.AuthorizationPolicyList{}
	if err := r.List(ctx, list, inEgressNamespace()); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 3 {
		t.Fatalf("expected host, VIP and short-host policies; got %d", len(list.Items))
	}
	for _, policy := range list.Items {
		if policy.Labels["parent-host"] == hostLabelValue(longTestHost) && policy.Annotations[parentHostAnnotation] != longTestHost {
			t.Fatal("missing long parent-host annotation")
		}
	}
	utsas := datanavnov1.UnionTeamServiceAccountsList{Items: []datanavnov1.UnionTeamServiceAccounts{{Spec: datanavnov1.UnionTeamServiceAccountsSpec{
		Project: sa.Project, Domain: sa.Domain, ServiceAccounts: []datanavnov1.UnionServiceAccount{sa.UnionServiceAccount},
	}}}}
	if err := r.cleanupInternalHosts(ctx, utsas); err != nil {
		t.Fatal(err)
	}
	for _, policy := range list.Items {
		if err := r.Get(ctx, client.ObjectKeyFromObject(policy), &istiosecurity.AuthorizationPolicy{}); err != nil {
			t.Fatalf("deleted an internal host still in use: %v", err)
		}
	}
	utsas.Items[0].Spec.ServiceAccounts[0].InternalAllowlist = []datanavnov1.Host{{Host: "api.example.com"}}
	for range 2 {
		if err := r.cleanupInternalHosts(ctx, utsas); err != nil {
			t.Fatal(err)
		}
	}
	for _, policy := range list.Items {
		err := r.Get(ctx, client.ObjectKeyFromObject(policy), &istiosecurity.AuthorizationPolicy{})
		if policy.Labels["parent-host"] == hostLabelValue(longTestHost) {
			if !apierrors.IsNotFound(err) {
				t.Fatalf("long-host or VIP policy was not removed: %v", err)
			}
		} else if err != nil {
			t.Fatalf("short-host policy was removed: %v", err)
		}
	}
}
