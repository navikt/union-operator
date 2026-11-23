package istio

import datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"

const (
	hostAnnotation       = "union.nav.no/host"
	parentHostAnnotation = "union.nav.no/parent-host"
)

func hostLabelValue(host string) string {
	return (datanavnov1.Host{Host: host}).LabelValue()
}

// Only new long-host resources need annotations; preserve short-host metadata.
func hostAnnotations(host, parent string) map[string]string {
	var annotations map[string]string
	if len(host) > 63 {
		annotations = map[string]string{hostAnnotation: host}
	}
	if len(parent) > 63 {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[parentHostAnnotation] = parent
	}
	return annotations
}
