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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
)

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the UnionTeamServiceAccounts object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile

func (r *UnionTeamServiceAccountsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	utsa := &datanavnov1.UnionTeamServiceAccounts{}
	err := r.Get(ctx, req.NamespacedName, utsa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("UnionTeamServiceAccounts resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the obbject - requeue the request.
		log.Error(err, "Failed to get UnionTeamServiceAccounts")
		return ctrl.Result{}, err
	}

	err = r.updateServiceAccountsForDomain(ctx, 
		&UnionEnv{
			Project: utsa.Spec.Project,
			Domain: utsa.Spec.Domain,
			ServiceAccounts: utsa.Spec.ServiceAccounts,
		},
	)
	if err != nil {
		log.Error(err, "Failed to update service accounts for domain", "project", utsa.Spec.Project, "domain", utsa.Spec.Domain)
		return ctrl.Result{}, err
	}

	// iamPolicyMember := &iam.GCPIAMPolicyMember{
	// 	TypeMeta: metav1.TypeMeta{
	// 		Kind:       "IAMPolicyMember",
	// 		APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
	// 	},
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      utsa.Spec.Name,
	// 		Namespace: utsa.Namespace,
	// 	},
	// 	Spec: iam.GCPIAMPolicyMemberSpec{
	// 		Member: "serviceAccount:" + utsa.Spec.Name + "@" + utsa.Namespace + ".iam.gserviceaccount.com",
	// 		Role:   "roles/viewer",
	// 	},
	// }

	//config, err := rest.InClusterConfig()
	//if err != nil {
	//	log.Error(err, "Failed to get cluster config")
	//	return ctrl.Result{}, err
	//}

	//c, err := client.New(config, client.Options{})
	//if err != nil {
	//	log.Error(err, "Failed to create k8s client")
	//	return ctrl.Result{}, err
	//}

	//err = c.Create(ctx, sa)
	//if err != nil {
	//	log.Error(err, "Failed to create service account")
	//	return ctrl.Result{}, err
	//}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnionTeamServiceAccountsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datanavnov1.UnionTeamServiceAccounts{}).
		Named("unionteamserviceaccounts").
		Complete(r)
}
