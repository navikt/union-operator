package serviceaccount

import (
	"context"
	"fmt"

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
	Name      string
	Role      string
	Kind      string
	External  string
	Condition *uniontypes.Condition

	Member     string
	APIVersion string
}

func (r *Reconciler) createIAMPolicyMembers(ctx context.Context, sa uniontypes.ServiceAccount) error {
	workloadIdentity := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-workload-identity-user", sa.Name),
		Role:       "roles/iam.workloadIdentityUser",
		Kind:       "IAMServiceAccount",
		External:   fmt.Sprintf("projects/%s/serviceAccounts/%s", sa.GCPProjectName, sa.GoogleServiceAccountEmail()),
		APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
		Member:     fmt.Sprintf("%s.svc.id.goog[%s/%s]", sa.GCPProjectName, sa.Namespace(), sa.Name),
	}

	dataBucket := IAMPolicyMemberOpts{
		Name:     fmt.Sprintf("%s-union-data-bucket-object-admin", sa.Name),
		Role:     "roles/storage.objectAdmin",
		Kind:     "StorageBucket",
		External: r.DataBucket,
		Condition: &uniontypes.Condition{
			Title:       "UnionDataBucketAccess",
			Description: fmt.Sprintf("Allow access to project %s, domain %s in Union data bucket", sa.Project, sa.Domain),
			Expression:  fmt.Sprintf("resource.name.startsWith(\"projects/_/buckets/%s/objects/metadata/v2/union-nav/%s/%s/\") ||\nresource.name == \"projects/_/buckets/%s\"", r.DataBucket, sa.Project, sa.Domain, r.DataBucket),
		},
		APIVersion: "storage.cnrm.cloud.google.com/v1beta1",
		Member:     sa.GoogleServiceAccountEmail(),
	}
	fastRegistrationBucket := IAMPolicyMemberOpts{
		Name:       fmt.Sprintf("%s-union-fast-registration-bucket-viewer", sa.Name),
		Role:       "roles/storage.objectViewer",
		Kind:       "StorageBucket",
		External:   r.FastRegistrationBucket,
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

func (r *Reconciler) createIAMPolicyMember(
	ctx context.Context,
	unionEnv *uniontypes.UnionEnv,
	opts IAMPolicyMemberOpts,
) error {
	log := logf.FromContext(ctx)
	existing := &uniontypes.IAMPolicyMember{}
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
			member := &uniontypes.IAMPolicyMember{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opts.Name,
					Namespace: unionEnv.Namespace(),
					Annotations: map[string]string{
						"cnrm.cloud.google.com/project-id": unionEnv.GCPProjectName,
					},
				},
				Spec: uniontypes.IAMPolicyMemberSpec{
					Member:    fmt.Sprintf("serviceAccount:%s", opts.Member),
					Role:      opts.Role,
					Condition: opts.Condition,
					ResourceRef: uniontypes.ResourceRef{
						ApiVersion: opts.APIVersion,
						Kind:       opts.Kind,
						External:   &opts.External,
					},
				},
			}
			setUnionMetadata(member, unionEnv, map[string]string{})
			err = r.Create(ctx, member)
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					log.V(1).Info("IAMPolicyMember already exists (stale cache)", "name", opts.Name)
					return nil
				}
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

func (r *Reconciler) reconcileServiceAccountForDomain(ctx context.Context, sa uniontypes.ServiceAccount) error {
	if err := r.reconcileIAMServiceAccount(ctx, sa); err != nil {
		return err
	}
	return r.reconcileServiceAccount(ctx, sa)
}

func (r *Reconciler) reconcileIAMServiceAccount(ctx context.Context, sa uniontypes.ServiceAccount) error {
	log := logf.FromContext(ctx)

	iamServiceAccount := &uniontypes.IAMServiceAccount{
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
			if apierrors.IsAlreadyExists(err) {
				// Stale cache: the object exists but the informer hasn't observed it yet.
				// It will be picked up on the next reconcile.
				log.V(1).Info("IAMServiceAccount already exists (stale cache), will reconcile on next pass", "name", sa.GoogleServiceAccountName())
				return nil
			}
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

func (r *Reconciler) reconcileServiceAccount(ctx context.Context, sa uniontypes.ServiceAccount) error {
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
