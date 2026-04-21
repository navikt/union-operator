package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	iam "github.com/nais/liberator/pkg/apis/iam.cnrm.cloud.google.com/v1beta1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	UnionProjectLabel      = "union.nav.no/project"
	UnionDomainLabel       = "union.nav.no/domain"
	DataBucket             = "restricted-dev-data"
	FastRegistrationBucket = "restricted-dev-fast-registration"
)

func (r *UnionTeamServiceAccountsReconciler) updateServiceAccountsForDomain(ctx context.Context, serviceAccounts []uniontypes.ServiceAccount, unionEnv *uniontypes.UnionEnv) error {

	if err := r.createOrUpdateServiceAccounts(ctx, serviceAccounts); err != nil {
		return err
	}

	if err := r.cleanupRemovedServiceAccounts(ctx, unionEnv, serviceAccounts); err != nil {
		return err
	}

	return nil
}

// cleanupAllResources deletes all ServiceAccounts, IAMServiceAccounts, and
// IAMPolicyMembers managed by this controller for the given UnionEnv.
// This is called by the finalizer when the CR is being deleted.
func (r *UnionTeamServiceAccountsReconciler) cleanupAllResources(ctx context.Context, unionEnv *uniontypes.UnionEnv) error {
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

func (r *UnionTeamServiceAccountsReconciler) createOrUpdateServiceAccounts(
	ctx context.Context,
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
	return nil
}

type IAMPolicyMemberOpts struct {
	Name     string
	Role     string
	Kind     string
	External string

	Member     string
	APIVersion string
}

func (r *UnionTeamServiceAccountsReconciler) createIAMPolicyMembers(ctx context.Context, sa uniontypes.ServiceAccount) error {

	workloadIdentity := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-workload-identity-user", sa.Name),
		Role:       "roles/iam.workloadIdentityUser",
		Kind:       "IAMServiceAccount",
		External:   fmt.Sprintf("projects/%s/serviceAccounts/%s", sa.GCPProjectName, sa.GoogleServiceAccountEmail()),
		APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s.svc.id.goog[%s/%s]", sa.GCPProjectName, sa.Namespace(), sa.Name),
	}

	dataBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-data-bucket-object-admin", sa.Name),
		Role:       "roles/storage.objectAdmin",
		Kind:       "StorageBucket",
		External:   DataBucket,
		APIVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     sa.GoogleServiceAccountEmail(),
	}
	fastRegistrationBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-fast-registration-bucket-viewer", sa.Name),
		Role:       "roles/storage.objectViewer",
		Kind:       "StorageBucket",
		External:   FastRegistrationBucket,
		APIVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     sa.GoogleServiceAccountEmail(),
	}

	policyMembers := []IAMPolicyMemberOpts{
		workloadIdentity,
		dataBucket,
		fastRegistrationBucket,
	}

	for _, member := range policyMembers {
		if err := r.createIAMPolicyMember(ctx, sa.UnionEnv, member); err != nil {
			return err
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createIAMPolicyMember(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	opts IAMPolicyMemberOpts,
) error {
	log := logf.FromContext(ctx)
	existing := &iam.IAMPolicyMember{}
	err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      opts.Name,
			Namespace: unionEnv.Namespace(),
		},
		existing,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			member := &iam.IAMPolicyMember{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opts.Name,
					Namespace: unionEnv.Namespace(),
					Annotations: map[string]string{
						"cnrm.cloud.google.com/project-id": unionEnv.GCPProjectName,
					},
				},
				Spec: iam.IAMPolicyMemberSpec{
					Member: fmt.Sprintf("serviceAccount:%s", opts.Member),
					Role:   opts.Role,
					ResourceRef: iam.ResourceRef{
						ApiVersion: opts.APIVersion,
						Kind:       opts.Kind,
						External:   &opts.External,
					},
				},
			}
			err = r.Create(ctx, member)
			if err != nil {
				log.Error(err, "Failed to create IAM policy member", "name", opts.Name)
				return err
			}
		} else {
			log.Error(err, "Failed to get IAM policy member", "name", opts.Name)
			return err
		}
	}
	return nil
}

func (r *UnionTeamServiceAccountsReconciler) reconcileServiceAccountForDomain(ctx context.Context, sa uniontypes.ServiceAccount) error {
	if err := r.reconcileIAMServiceAccount(ctx, sa); err != nil {
		return err
	}
	return r.reconcileServiceAccount(ctx, sa)
}

func (r *UnionTeamServiceAccountsReconciler) reconcileIAMServiceAccount(ctx context.Context, sa uniontypes.ServiceAccount) error {
	log := logf.FromContext(ctx)

	iamServiceAccount := &iam.IAMServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.GoogleServiceAccountName(),
			Namespace: sa.Namespace(),
		},
	}

	err := r.Get(ctx, types.NamespacedName{Name: sa.GoogleServiceAccountName(), Namespace: sa.Namespace()}, iamServiceAccount)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get IAMServiceAccount", "name", sa.GoogleServiceAccountName())
			return err
		}
		// Create new IAMServiceAccount.
		setUnionMetadata(iamServiceAccount, sa.UnionEnv, map[string]string{
			"cnrm.cloud.google.com/project-id": sa.GCPProjectName,
		})
		iamServiceAccount.Spec.DisplayName = fmt.Sprintf("Union service account %s for domain %s in project %s", sa.Name, sa.Domain, sa.Project)
		if err := r.Create(ctx, iamServiceAccount); err != nil {
			log.Error(err, "Failed to create IAMServiceAccount", "name", sa.GoogleServiceAccountName())
			return err
		}
		log.Info("Created IAMServiceAccount", "name", sa.GoogleServiceAccountName())
		return nil
	}

	// Patch labels/annotations on existing IAMServiceAccount (avoids touching immutable spec fields).
	patch := client.MergeFrom(iamServiceAccount.DeepCopy())
	setUnionMetadata(iamServiceAccount, sa.UnionEnv, map[string]string{
		"cnrm.cloud.google.com/project-id": sa.GCPProjectName,
	})
	if err := r.Patch(ctx, iamServiceAccount, patch); err != nil {
		log.Error(err, "Failed to patch IAMServiceAccount", "name", sa.GoogleServiceAccountName())
		return err
	}
	return nil
}

func (r *UnionTeamServiceAccountsReconciler) reconcileServiceAccount(ctx context.Context, sa uniontypes.ServiceAccount) error {
	log := logf.FromContext(ctx)

	k8sSa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sa.Name,
			Namespace: sa.Namespace(),
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, k8sSa, func() error {
		setUnionMetadata(k8sSa, sa.UnionEnv, map[string]string{
			"iam.gke.io/gcp-service-account": sa.GoogleServiceAccountEmail(),
		})
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to create or update ServiceAccount", "name", sa.Name)
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.Info("Reconciled ServiceAccount", "name", sa.Name, "result", result)
	}
	return nil
}

func (r *UnionTeamServiceAccountsReconciler) cleanupRemovedServiceAccounts(
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
		if findByField(serviceAccounts, k8sSa.Name, func(s uniontypes.ServiceAccount) string { return s.Name }) == nil {
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
	}
	return errors.Join(errs...)
}

// cleanupServiceAccount deletes the IAMServiceAccount and all associated
// IAMPolicyMembers for a given service account name.
func (r *UnionTeamServiceAccountsReconciler) cleanupServiceAccount(
	ctx context.Context,
	sa uniontypes.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	var errs []error

	if err := r.cleanupIAMPolicyMembers(ctx, sa); err != nil {
		log.Error(err, "Failed to cleanup IAMPolicyMembers", "name", sa.Name)
		errs = append(errs, err)
	}

	googleServiceAccount := &iam.IAMServiceAccount{}
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
func (r *UnionTeamServiceAccountsReconciler) cleanupIAMPolicyMembers(
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
		existing := &iam.IAMPolicyMember{}
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

func findByField[T uniontypes.ServiceAccount | v1.ServiceAccount](serviceAccounts []T, name string, fieldFunc func(T) string) *T {
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
