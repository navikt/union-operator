package serviceaccount

import (
	"context"
	"errors"
	"fmt"

	iam "github.com/nais/liberator/pkg/apis/iam.cnrm.cloud.google.com/v1beta1"
	datanavnov1 "github.com/navikt/union-operator/api/v1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// cleanupAllResources deletes all ServiceAccounts, IAMServiceAccounts, and
// IAMPolicyMembers managed by this controller for the given UnionEnv.
// This is called by the finalizer when the CR is being deleted.
func (r *Reconciler) CleanupAllResources(ctx context.Context, unionEnv *uniontypes.UnionEnv) error {
	log := logf.FromContext(ctx)
	existing := &v1.ServiceAccountList{}

	err := r.List(ctx, existing, client.MatchingLabels{
		UnionProjectLabel: unionEnv.Project,
		UnionDomainLabel:  unionEnv.Domain,
	})
	if err != nil {
		log.Error(err, "Failed to list service accounts during cleanup", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	var errs []error
	for _, sa := range existing.Items {
		if err := r.cleanupServiceAccount(ctx, unionEnv, sa.Name); err != nil {
			log.Error(err, "Failed to cleanup service account resources during finalizer", "name", sa.Name)
			errs = append(errs, err)
			continue
		}

		if err := r.Delete(ctx, &sa); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete Kubernetes ServiceAccount during finalizer", "name", sa.Name)
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// cleanupServiceAccount deletes the IAMServiceAccount and all associated
// IAMPolicyMembers for a given service account name.
func (r *Reconciler) cleanupServiceAccount(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	serviceAccountName string,
) error {
	log := logf.FromContext(ctx)
	var errs []error

	if err := r.cleanupIAMPolicyMembers(ctx, unionEnv, serviceAccountName); err != nil {
		log.Error(err, "Failed to cleanup IAMPolicyMembers", "name", serviceAccountName)
		errs = append(errs, err)
	}

	googleServiceAccount := &iam.IAMServiceAccount{}
	err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      unionEnv.GoogleServiceAccountName(serviceAccountName),
			Namespace: unionEnv.Namespace(),
		},
		googleServiceAccount,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("IAMServiceAccount not found, skipping deletion", "name", serviceAccountName, "project", unionEnv.Project, "domain", unionEnv.Domain)
			return errors.Join(errs...)
		}
		log.Error(err, "Failed to get IAMServiceAccount for deletion", "name", serviceAccountName)
		errs = append(errs, err)
		return errors.Join(errs...)
	}

	if err := r.Delete(ctx, googleServiceAccount); err != nil {
		log.Error(err, "Failed to delete IAMServiceAccount", "name", serviceAccountName)
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// cleanupIAMPolicyMembers deletes the three IAMPolicyMember resources
// (workload identity, data bucket, fast registration bucket) for a service account.
func (r *Reconciler) cleanupIAMPolicyMembers(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	serviceAccountName string,
) error {
	log := logf.FromContext(ctx)

	policyMemberNames := []string{
		fmt.Sprintf("%s-workload-identity-user", serviceAccountName),
		fmt.Sprintf("%s-union-data-bucket-object-admin", serviceAccountName),
		fmt.Sprintf("%s-union-fast-registration-bucket-viewer", serviceAccountName),
	}

	var errs []error
	for _, name := range policyMemberNames {
		existing := &iam.IAMPolicyMember{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: unionEnv.Namespace(),
		}, existing)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, "Failed to get IAMPolicyMember for deletion", "name", name)
			errs = append(errs, err)
			continue
		}

		if err := r.Delete(ctx, existing); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete IAMPolicyMember", "name", name)
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func (r *Reconciler) cleanupRemovedServiceAccounts(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	existingServiceAccounts []v1.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	var errs []error
	for _, existing := range existingServiceAccounts {
		if findByField(unionEnv.ServiceAccounts, existing.Name, func(sa datanavnov1.UnionServiceAccount) string { return sa.Name }) == nil {
			if err := r.cleanupServiceAccount(ctx, unionEnv, existing.Name); err != nil {
				log.Error(err, "Failed to cleanup service account resources", "name", existing.Name)
				errs = append(errs, err)
				continue
			}

			if err := r.Delete(ctx, &existing); err != nil {
				if !apierrors.IsNotFound(err) {
					log.Error(err, "Failed to delete Kubernetes ServiceAccount", "name", existing.Name)
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}
