package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestHostMetadataIdentifiers(t *testing.T) {
	for _, value := range []string{"api.example.com", strings.Repeat("a", 51) + ".example.com"} {
		host := Host{Host: value}
		if host.Name() != strings.ReplaceAll(value, ".", "-") || host.LabelValue() != value {
			t.Fatalf("changed legacy identifiers for %q", value)
		}
	}
	for _, value := range []string{
		strings.Repeat("a", 52) + ".example.com",
		"tenant-with-a-deliberately-long-identifier.environment.api.example.com",
		strings.Join([]string{strings.Repeat("a", 63), strings.Repeat("b", 63), strings.Repeat("c", 63), strings.Repeat("d", 61)}, "."),
	} {
		host := Host{Host: value}
		hash := sha256.Sum256([]byte(value))
		id := hex.EncodeToString(hash[:16])
		if host.Name() != "host-"+id || host.LabelValue() != "host_"+id {
			t.Fatalf("unexpected long-host identifiers for %q: %q, %q", value, host.Name(), host.LabelValue())
		}
		if errs := validation.IsDNS1123Label(host.Name()); len(errs) != 0 {
			t.Fatalf("invalid name %q: %v", host.Name(), errs)
		}
		if errs := validation.IsValidLabelValue(host.LabelValue()); len(errs) != 0 {
			t.Fatalf("invalid label %q: %v", host.LabelValue(), errs)
		}
		if host.Name() != (Host{Host: value}).Name() {
			t.Fatal("name is not deterministic")
		}
		other := Host{Host: strings.Replace(value, "example.com", "example.net", 1) + ".different"}
		if host.Name() == other.Name() || host.LabelValue() == other.LabelValue() {
			t.Fatal("different long hosts have identical identifiers")
		}
	}
}
