package serviceaccount

import (
	"context"
	"maps"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	UnionProjectLabel = "union.nav.no/project"
	UnionDomainLabel  = "union.nav.no/domain"
)

// Reconciler creates and manages Google service accounts and their associated Kubernetes service accounts.
// ServiceAccounts, IAMServiceAccounts, IAMPolicyMembers for Union project and domains
type Reconciler struct {
	client.Client
	GCPProjectName         string
	FastRegistrationBucket string
	DataBucket             string
}

func (r *Reconciler) CreateOrUpdateServiceAccounts(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
) error {
	for _, sa := range unionEnv.ServiceAccounts {
		if err := r.reconcileServiceAccountForDomain(ctx, unionEnv, sa); err != nil {
			return err
		}
		if err := r.createIAMPolicyMembers(ctx, unionEnv, sa); err != nil {
			return err
		}
	}
	return nil
}

func findByField[T datanavnov1.UnionServiceAccount | v1.ServiceAccount](serviceAccounts []T, name string, fieldFunc func(T) string) *T {
	for _, sa := range serviceAccounts {
		if fieldFunc(sa) == name {
			return &sa
		}
	}

	return nil
}

// setUnionMetadata ensures the standard union labels and the provided annotations
// are set on the given object. Existing labels and annotations are preserved.
func setUnionMetadata(obj metav1.Object, unionEnv *uniontypes.UnionEnv, annotations map[string]string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[UnionProjectLabel] = unionEnv.Project
	labels[UnionDomainLabel] = unionEnv.Domain
	obj.SetLabels(labels)

	existing := obj.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}
	maps.Copy(existing, annotations)
	obj.SetAnnotations(existing)
}
