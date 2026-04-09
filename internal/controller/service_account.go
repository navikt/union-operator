package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/go-logr/logr"
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
	UnionProjectLabel = "union.nav.no/project"
	UnionDomainLabel  = "union.nav.no/domain"
)

type UnionEnv struct {
	Project string
	Domain string
	ServiceAccounts []datanavnov1.UnionServiceAccount
}

func (u *UnionEnv) Namespace() string {
	return fmt.Sprintf("%s-%s", u.Project, u.Domain)
}

func (u *UnionEnv) googleServiceAccountName(serviceAccountName string) string {
	name := fmt.Sprintf("%s-%s-%s", serviceAccountName, u.Domain, u.Project)
	hash := sha256.Sum256([]byte(name))

	prefixLength := 23
	if len(name) < 23 {
		prefixLength = len(name)
	}
	return fmt.Sprintf("%s-%s", name[:prefixLength], hex.EncodeToString(hash[:])[:5])
}

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



	if err := r.createOrUpdateServiceAccounts(ctx, unionEnv, existing.Items, log); err != nil {
		log.Error(err, "Failed to create or update service accounts for project and domain", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	if err := r.cleanupRemovedServiceAccounts(ctx, unionEnv, existing.Items, log); err != nil {
		log.Error(err, "Failed to cleanup removed service accounts for project and domain", "project", unionEnv.Project, "domain", unionEnv.Domain)
		return err
	}

	return nil
}

func (r *UnionTeamServiceAccountsReconciler) createOrUpdateServiceAccounts(ctx context.Context, unionEnv *UnionEnv, existing []v1.ServiceAccount, log logr.Logger) error {
		for _, sa := range unionEnv.ServiceAccounts {
			existing := findByField(existing, sa.Name, func(sa v1.ServiceAccount) string { return sa.Name }) 
			if existing != nil {
				if err := r.updateServiceAccountForDomain(ctx, unionEnv, sa); err != nil {
					log.Error(err, "Failed to cleanup Google service account", "name", sa.Name)
					return err
				}
			} else {
				err := r.createServiceAccountForDomain(ctx, unionEnv, sa)
				if err != nil {
					log.Error(err, "Failed to create service account for domain", "project", unionEnv.Project, "domain", unionEnv.Domain, "serviceAccount", sa.Name)
					return err
				}
			}
		}
		return nil
}

func (r *UnionTeamServiceAccountsReconciler) updateServiceAccountForDomain(ctx context.Context, unionEnv *UnionEnv, serviceAccount datanavnov1.UnionServiceAccount) error {
	log := logf.FromContext(ctx)
	existing := &iam.IAMServiceAccount{}

	err := r.Get(
		ctx, 
		types.NamespacedName{
			Name: unionEnv.googleServiceAccountName(serviceAccount.Name), 
			Namespace: unionEnv.Namespace(),
		}, 
		existing,
	)
	if err != nil {
		return err
	}

	// err = r.Update(ctx, existing)
	// if err != nil {
	// 	log.Error(err, "Failed to update IAM service account for domain", "project", unionEnv.Project, "domain", unionEnv.Domain, "serviceAccount", serviceAccount.Name)
	// 	return err
	// }

	sa := &v1.ServiceAccount{}
	err = r.Get(
		ctx, 
		types.NamespacedName{
			Name:      serviceAccount.Name,
			Namespace: unionEnv.Namespace(),
		}, 
		sa,
	)
	if err != nil {
		log.Error(err, "Failed to create service account")
		return err
	}

	err = r.Update(ctx, sa)
	if err != nil {
		log.Error(err, "Failed to update service account")
		return err
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

func (r *UnionTeamServiceAccountsReconciler) cleanupRemovedServiceAccounts(ctx context.Context, unionEnv *UnionEnv, existingServiceAccounts []v1.ServiceAccount, log logr.Logger) error {
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

func (r *UnionTeamServiceAccountsReconciler) cleanupGoogleServiceAccount(ctx context.Context, unionEnv *UnionEnv, serviceAccountName string) error {
	log := logf.FromContext(ctx)

	googleServiceAccount := &iam.IAMServiceAccount{}
	err := r.Get(
		ctx, 
		types.NamespacedName{
			Name: unionEnv.googleServiceAccountName(serviceAccountName),
			Namespace: unionEnv.Namespace(),
		},
		googleServiceAccount,
	)
	if err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to get IAM service account for deletion", "name", serviceAccountName)
				return err
			}
		log.Info(fmt.Sprintf("Google service account for domain %s in project %s not found.", unionEnv.Project, unionEnv.Domain))
	} 

	return nil
}

func findByField[T datanavnov1.UnionServiceAccount | v1.ServiceAccount](serviceAccounts []T, name string, fieldFunc func(T) string) (*T) {
	for _, sa := range serviceAccounts {
		if fieldFunc(sa) == name {
			return &sa
		}
	}

	return nil
}

// func (r *UnionTeamServiceAccountsReconciler) cleanupRemovedServiceAccounts(ctx context.Context, project, domain string, serviceAccounts []datanavnov1.UnionServiceAccount, existingServiceAccounts []v1.ServiceAccount, log logr.Logger) {
// 	for _, existing := range existingServiceAccounts {
// 		if _, found := findServiceAccount(existing.Name, serviceAccounts); !found {
// 			err := r.Delete(ctx, &existing)
// 			if err != nil {
// 				log.Error(err, "Failed to delete service account", "name", existing.Name)
// 			}
// 		}

// 	}
// }