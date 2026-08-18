package serviceaccount

import (
	"context"
	"errors"
	"fmt"
	"slices"

	uniontypes "github.com/navikt/union-operator/internal/types"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// CleanupAllResources deletes all ServiceAccounts, IAMServiceAccounts, and
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
		if err := r.cleanupServiceAccount(ctx, unionEnv.ServiceAccountByName(sa.Name)); err != nil {
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

func (r *Reconciler) CleanupRemovedServiceAccounts(ctx context.Context, unionEnv *uniontypes.UnionEnv, serviceAccounts []uniontypes.ServiceAccount) error {
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
		if !containsSA(sa.Name, serviceAccounts) {
			if err := r.cleanupServiceAccount(ctx, unionEnv.ServiceAccountByName(sa.Name)); err != nil {
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
	}
	return errors.Join(errs...)
}

func containsSA(saName string, serviceAccounts []uniontypes.ServiceAccount) bool {
	for _, s := range serviceAccounts {
		if s.Name == saName {
			return true
		}
	}

	return false
}

// cleanupServiceAccount deletes the IAMServiceAccount and all associated
// IAMPolicyMembers for a given service account.
func (r *Reconciler) cleanupServiceAccount(
	ctx context.Context,
	sa uniontypes.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	var errs []error

	if err := r.cleanupIAMPolicyMembers(ctx, sa); err != nil {
		log.Error(err, "Failed to cleanup IAMPolicyMembers", "name", sa.Name)
		errs = append(errs, err)
	}

	googleServiceAccount := &uniontypes.IAMServiceAccount{}
	err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      sa.GoogleServiceAccountName(),
			Namespace: sa.Namespace(),
		},
		googleServiceAccount,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("IAMServiceAccount not found, skipping deletion", "name", sa.Name, "project", sa.Project, "domain", sa.Domain)
			return errors.Join(errs...)
		}
		log.Error(err, "Failed to get IAMServiceAccount for deletion", "name", sa.Name)
		errs = append(errs, err)
		return errors.Join(errs...)
	}

	if err := r.Delete(ctx, googleServiceAccount); err != nil {
		log.Error(err, "Failed to delete IAMServiceAccount", "name", sa.Name)
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// cleanupIAMPolicyMembers deletes the three IAMPolicyMember resources
// (workload identity, data bucket, fast registration bucket) for a service account.
func (r *Reconciler) cleanupIAMPolicyMembers(
	ctx context.Context,
	sa uniontypes.ServiceAccount,
) error {
	log := logf.FromContext(ctx)

	policyMemberNames := []string{
		fmt.Sprintf("%s-workload-identity-user", sa.Name),
		fmt.Sprintf("%s-union-data-bucket-object-admin", sa.Name),
		fmt.Sprintf("%s-union-fast-registration-bucket-viewer", sa.Name),
	}

	var errs []error
	for _, name := range policyMemberNames {
		existing := &uniontypes.IAMPolicyMember{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: sa.Namespace(),
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

// cleanupRemovedServiceAccounts deletes ServiceAccounts, IAMServiceAccounts and
// IAMPolicyMembers for service accounts that are no longer present in the spec.
func (r *Reconciler) cleanupRemovedServiceAccounts(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	serviceAccounts []uniontypes.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	existing := &v1.ServiceAccountList{}

	err := r.List(ctx, existing, client.MatchingLabels{
		UnionProjectLabel: unionEnv.Project,
		UnionDomainLabel:  unionEnv.Domain,
	})
	if err != nil {
		log.Error(err, "Failed to list ServiceAccounts", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	var errs []error
	for _, k8sSa := range existing.Items {
		if slices.ContainsFunc(serviceAccounts, func(s uniontypes.ServiceAccount) bool { return s.Name == k8sSa.Name }) {
			continue
		}

		if err := r.cleanupServiceAccount(ctx, unionEnv.ServiceAccountByName(k8sSa.Name)); err != nil {
			log.Error(err, "Failed to cleanup service account resources", "name", k8sSa.Name)
			errs = append(errs, err)
			continue
		}

		if err := r.Delete(ctx, &k8sSa); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete Kubernetes ServiceAccount", "name", k8sSa.Name)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
