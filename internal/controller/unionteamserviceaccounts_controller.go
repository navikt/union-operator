/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
)

const unionFinalizer = "data.nav.no/finalizer"

// UnionTeamServiceAccountsReconciler reconciles a UnionTeamServiceAccounts object
type UnionTeamServiceAccountsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=data.nav.no,resources=unionteamserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=data.nav.no,resources=unionteamserviceaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=data.nav.no,resources=unionteamserviceaccounts/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iamserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iampolicymembers,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the current state of the cluster closer to the desired state
// by managing ServiceAccounts, IAMServiceAccounts, and IAMPolicyMembers.
// It uses a finalizer to ensure cross-namespace resources are cleaned up on deletion.

func (r *UnionTeamServiceAccountsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	utsa := &datanavnov1.UnionTeamServiceAccounts{}
	err := r.Get(ctx, req.NamespacedName, utsa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("UnionTeamServiceAccounts resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get UnionTeamServiceAccounts")
		return ctrl.Result{}, err
	}

	unionEnv := &UnionEnv{
		Project:         utsa.Spec.Project,
		Domain:          utsa.Spec.Domain,
		ServiceAccounts: utsa.Spec.ServiceAccounts,
	}

	// Handle deletion: clean up all cross-namespace resources before allowing CR removal.
	if !utsa.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(utsa, unionFinalizer) {
			log.Info("Running finalizer cleanup", "project", utsa.Spec.Project, "domain", utsa.Spec.Domain)

			if err := r.cleanupAllResources(ctx, unionEnv); err != nil {
				log.Error(err, "Failed to run finalizer cleanup")
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(utsa, unionFinalizer)
			if err := r.Update(ctx, utsa); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
			log.Info("Finalizer cleanup completed")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if !controllerutil.ContainsFinalizer(utsa, unionFinalizer) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(utsa, unionFinalizer)
		if err := r.Update(ctx, utsa); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Normal reconciliation.
	if err := r.updateServiceAccountsForDomain(ctx, unionEnv); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnionTeamServiceAccountsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datanavnov1.UnionTeamServiceAccounts{}).
		Named("unionteamserviceaccounts").
		Complete(r)
}
