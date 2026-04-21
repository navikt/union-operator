package serviceaccount

import (
	"context"
	"maps"

	uniontypes "github.com/navikt/union-operator/internal/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	UnionProjectLabel = "union.nav.no/project"
	UnionDomainLabel  = "union.nav.no/domain"
)

// Reconciler creates and manages Google service accounts and their associated
// Kubernetes service accounts. It reconciles ServiceAccounts, IAMServiceAccounts
// and IAMPolicyMembers for a Union project and domain.
type Reconciler struct {
	client.Client
	FastRegistrationBucket string
	DataBucket             string
}

// CreateOrUpdateServiceAccounts reconciles the given service accounts and cleans
// up any existing ServiceAccounts that are no longer present in the spec.
func (r *Reconciler) CreateOrUpdateServiceAccounts(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	serviceAccounts []uniontypes.ServiceAccount,
) error {
	for _, sa := range serviceAccounts {
		if err := r.reconcileServiceAccountForDomain(ctx, sa); err != nil {
			return err
		}
		if err := r.createIAMPolicyMembers(ctx, sa); err != nil {
			return err
		}
	}

	return r.cleanupRemovedServiceAccounts(ctx, unionEnv, serviceAccounts)
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
