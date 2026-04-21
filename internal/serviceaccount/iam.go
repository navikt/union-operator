package serviceaccount

import (
	"context"
	"fmt"

	iam "github.com/nais/liberator/pkg/apis/iam.cnrm.cloud.google.com/v1beta1"
	datanavnov1 "github.com/navikt/union-operator/api/v1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type IAMPolicyMemberOpts struct {
	Name     string
	Role     string
	Kind     string
	External string

	Member     string
	APIVersion string
}

func (r *Reconciler) createIAMPolicyMembers(ctx context.Context, unionEnv *uniontypes.UnionEnv, sa datanavnov1.UnionServiceAccount) error {
	workloadIdentity := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-workload-identity-user", sa.Name),
		Role:       "roles/iam.workloadIdentityUser",
		Kind:       "IAMServiceAccount",
		External:   fmt.Sprintf("projects/%s/serviceAccounts/%s", unionEnv.GCPProjectName, unionEnv.GoogleServiceAccountEmail(sa.Name)),
		APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s.svc.id.goog[%s/%s]", r.GCPProjectName, unionEnv.Namespace(), sa.Name),
	}

	dataBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-data-bucket-object-admin", sa.Name),
		Role:       "roles/storage.objectAdmin",
		Kind:       "StorageBucket",
		External:   r.DataBucket,
		APIVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     unionEnv.GoogleServiceAccountEmail(sa.Name),
	}
	fastRegistrationBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-fast-registration-bucket-viewer", sa.Name),
		Role:       "roles/storage.objectViewer",
		Kind:       "StorageBucket",
		External:   r.FastRegistrationBucket,
		APIVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     unionEnv.GoogleServiceAccountEmail(sa.Name),
	}

	policyMembers := []IAMPolicyMemberOpts{
		workloadIdentity,
		dataBucket,
		fastRegistrationBucket,
	}

	for _, member := range policyMembers {
		if err := r.createIAMPolicyMember(ctx, unionEnv, member); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) createIAMPolicyMember(
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
						"cnrm.cloud.google.com/project-id": r.GCPProjectName,
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

func (r *Reconciler) reconcileServiceAccountForDomain(ctx context.Context, unionEnv *uniontypes.UnionEnv, serviceAccount datanavnov1.UnionServiceAccount) error {
	if err := r.reconcileIAMServiceAccount(ctx, unionEnv, serviceAccount); err != nil {
		return err
	}
	return r.reconcileServiceAccount(ctx, unionEnv, serviceAccount)
}

func (r *Reconciler) reconcileIAMServiceAccount(ctx context.Context, unionEnv *uniontypes.UnionEnv, serviceAccount datanavnov1.UnionServiceAccount) error {
	log := logf.FromContext(ctx)
	googleServiceAccountName := unionEnv.GoogleServiceAccountName(serviceAccount.Name)

	iamServiceAccount := &iam.IAMServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      googleServiceAccountName,
			Namespace: unionEnv.Namespace(),
		},
	}

	err := r.Get(ctx, types.NamespacedName{Name: googleServiceAccountName, Namespace: unionEnv.Namespace()}, iamServiceAccount)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get IAMServiceAccount", "name", googleServiceAccountName)
			return err
		}

		// Create new IAMServiceAccount.
		setUnionMetadata(iamServiceAccount, unionEnv, map[string]string{
			"cnrm.cloud.google.com/project-id": r.GCPProjectName,
		})
		iamServiceAccount.Spec.DisplayName = fmt.Sprintf("Union service account %s for domain %s in project %s", serviceAccount.Name, unionEnv.Domain, unionEnv.Project)
		if err := r.Create(ctx, iamServiceAccount); err != nil {
			log.Error(err, "Failed to create IAMServiceAccount", "name", googleServiceAccountName)
			return err
		}
		log.Info("Created IAMServiceAccount", "name", googleServiceAccountName)
		return nil
	}

	// Patch labels/annotations on existing IAMServiceAccount (avoids touching immutable spec fields).
	patch := client.MergeFrom(iamServiceAccount.DeepCopy())
	setUnionMetadata(iamServiceAccount, unionEnv, map[string]string{
		"cnrm.cloud.google.com/project-id": r.GCPProjectName,
	})
	if err := r.Patch(ctx, iamServiceAccount, patch); err != nil {
		log.Error(err, "Failed to patch IAMServiceAccount", "name", googleServiceAccountName)
		return err
	}
	return nil
}

func (r *Reconciler) reconcileServiceAccount(ctx context.Context, unionEnv *uniontypes.UnionEnv, serviceAccount datanavnov1.UnionServiceAccount) error {
	log := logf.FromContext(ctx)
	googleServiceAccountName := unionEnv.GoogleServiceAccountName(serviceAccount.Name)

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount.Name,
			Namespace: unionEnv.Namespace(),
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		setUnionMetadata(sa, unionEnv, map[string]string{
			"iam.gke.io/gcp-service-account": googleServiceAccountName + "@" + r.GCPProjectName + ".iam.gserviceaccount.com",
		})
		return nil
	})
	if err != nil {
		log.Error(err, "Failed to create or update ServiceAccount", "name", serviceAccount.Name)
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.Info("Reconciled ServiceAccount", "name", serviceAccount.Name, "result", result)
	}
	return nil
}
