package controller

import (
	"context"
	"fmt"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	iam "github.com/nais/liberator/pkg/apis/iam.cnrm.cloud.google.com/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	UnionProjectLabel      = "union.nav.no/project"
	UnionDomainLabel       = "union.nav.no/domain"
	GCPProjectName         = "nav-data-union-restricted-dev"
	DataBucket             = "restricted-dev-data"
	FastRegistrationBucket = "restricted-dev-fast-registration"
)

func (r *UnionTeamServiceAccountsReconciler) updateServiceAccountsForDomain(ctx context.Context, unionEnv *UnionEnv) error {
	log := logf.FromContext(ctx)
	existing := &v1.ServiceAccountList{}

	err := r.List(ctx, existing, client.MatchingLabels{
		UnionProjectLabel: unionEnv.Project,
		UnionDomainLabel:  unionEnv.Domain,
	})
	if err != nil {
		log.Error(err, "Failed to list service accounts for project and domain", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	if err := r.createOrUpdateServiceAccounts(ctx, unionEnv, existing.Items); err != nil {
		log.Error(err, "Failed to create or update service accounts for project and domain", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	if err := r.cleanupRemovedServiceAccounts(ctx, unionEnv, existing.Items); err != nil {
		log.Error(err, "Failed to cleanup removed service accounts for project and domain", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createOrUpdateServiceAccounts(
	ctx context.Context,
	unionEnv *UnionEnv,
	existing []v1.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	for _, sa := range unionEnv.ServiceAccounts {
		existing := findByField(existing, sa.Name, func(sa v1.ServiceAccount) string { return sa.Name })
		if existing == nil {
			err := r.createServiceAccountForDomain(ctx, unionEnv, sa)
			if err != nil {
				log.Error(err, "Failed to create service account for domain", "project", unionEnv.Project, "domain", unionEnv.Domain, "serviceAccount", sa.Name)
				return err
			}
		}
		err := r.createIAMPolicyMembers(ctx, unionEnv, sa)
		if err != nil {
			log.Error(err, "Failed to create iam policy members for project", "project", unionEnv.Project, "domain", unionEnv.Domain, "serviceAccount", sa.Name)
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
	ApiVersion string
}

func (r *UnionTeamServiceAccountsReconciler) createIAMPolicyMembers(ctx context.Context, unionEnv *UnionEnv, sa datanavnov1.UnionServiceAccount) error {
	log := logf.FromContext(ctx)

	workloadIdentity := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-workload-identity-user", sa.Name),
		Role:       "roles/iam.workloadIdentityUser",
		Kind:       "IAMServiceAccount",
		External:   fmt.Sprintf("projects/%s/serviceAccounts/%s@%s.iam.gserviceaccount.com", GCPProjectName, unionEnv.googleServiceAccountName(sa.Name), GCPProjectName),
		ApiVersion: "iam.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s.svc.id.goog[%s/%s]", GCPProjectName, unionEnv.Namespace(), sa.Name),
	}

	dataBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-data-bucket-object-admin", sa.Name),
		Role:       "roles/storage.objectAdmin",
		Kind:       "StorageBucket",
		External:   DataBucket,
		ApiVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s@%s.iam.gserviceaccount.com", unionEnv.googleServiceAccountName(sa.Name), GCPProjectName),
	}
	fastRegistrationBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-fast-registration-bucket-viewer", sa.Name),
		Role:       "roles/storage.objectViewer",
		Kind:       "StorageBucket",
		External:   FastRegistrationBucket,
		ApiVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s@%s.iam.gserviceaccount.com", unionEnv.googleServiceAccountName(sa.Name), GCPProjectName),
	}

	policyMembers := []IAMPolicyMemberOpts{
		workloadIdentity,
		dataBucket,
		fastRegistrationBucket,
	}

	for _, member := range policyMembers {
		err := r.createIAMPolicyMember(
			ctx,
			unionEnv,
			sa,
			member,
		)
		if err != nil {
			log.Error(err, "Failed to create IAM policy member", "name", member.Name)
			return err
		}
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createIAMPolicyMember(
	ctx context.Context,
	unionEnv *UnionEnv,
	sa datanavnov1.UnionServiceAccount,
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
						"cnrm.cloud.google.com/project-id": GCPProjectName,
					},
				},
				Spec: iam.IAMPolicyMemberSpec{
					Member: fmt.Sprintf("serviceAccount:%s", opts.Member),
					Role:   opts.Role,
					ResourceRef: iam.ResourceRef{
						ApiVersion: opts.ApiVersion,
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

func (r *UnionTeamServiceAccountsReconciler) createServiceAccountForDomain(ctx context.Context, unionEnv *UnionEnv, serviceAccount datanavnov1.UnionServiceAccount) error {
	log := logf.FromContext(ctx)
	googleServiceAccountName := unionEnv.googleServiceAccountName(serviceAccount.Name)

	iamServiceAccount := &iam.IAMServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IAMServiceAccount",
			APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      googleServiceAccountName,
			Namespace: unionEnv.Namespace(),
			Annotations: map[string]string{
				"cnrm.cloud.google.com/project-id": "nav-data-union-restricted-dev",
			},
		},
		Spec: iam.IAMServiceAccountSpec{
			DisplayName: fmt.Sprintf("Union service account %s for domain %s in project %s", serviceAccount.Name, unionEnv.Domain, unionEnv.Project),
		},
	}

	err := r.Create(ctx, iamServiceAccount)
	if err != nil {
		log.Error(err, "Failed to create IAM service account")
		return err
	}

	sa := &v1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount.Name,
			Namespace: unionEnv.Namespace(),
			Annotations: map[string]string{
				"iam.gke.io/gcp-service-account": googleServiceAccountName + "@nav-data-union-restricted-dev.iam.gserviceaccount.com",
			},
		},
	}

	err = r.Create(ctx, sa)
	if err != nil {
		log.Error(err, "Failed to create service account")
		return err
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) cleanupRemovedServiceAccounts(
	ctx context.Context,
	unionEnv *UnionEnv,
	existingServiceAccounts []v1.ServiceAccount,
) error {
	log := logf.FromContext(ctx)
	for _, existing := range existingServiceAccounts {
		if findByField(unionEnv.ServiceAccounts, existing.Name, func(sa datanavnov1.UnionServiceAccount) string { return sa.Name }) == nil {
			if err := r.cleanupGoogleServiceAccount(ctx, unionEnv, existing.Name); err != nil {
				log.Error(err, "Failed to cleanup Google service account", "name", existing.Name)
			}

			err := r.Delete(ctx, &existing)
			if err != nil {
				log.Error(err, "Failed to delete k8s service account", "name", existing.Name)
			}
		}
	}
	return nil
}

func (r *UnionTeamServiceAccountsReconciler) cleanupGoogleServiceAccount(
	ctx context.Context,
	unionEnv *UnionEnv,
	serviceAccountName string,
) error {
	log := logf.FromContext(ctx)

	googleServiceAccount := &iam.IAMServiceAccount{}
	err := r.Get(
		ctx,
		types.NamespacedName{
			Name:      unionEnv.googleServiceAccountName(serviceAccountName),
			Namespace: unionEnv.Namespace(),
		},
		googleServiceAccount,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Google service account for domain %s in project %s not found.", unionEnv.Project, unionEnv.Domain))
			return nil
		}
		log.Error(err, "Failed to get IAM service account for deletion", "name", serviceAccountName)
		return err
	}

	err = r.Delete(ctx, googleServiceAccount)
	if err != nil {
		log.Error(err, "Failed to delete IAM service account", "name", serviceAccountName)
		return err
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
