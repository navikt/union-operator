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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datanavnov1 "github.com/navikt/union-operator/api/v1alpha1"
	"github.com/navikt/union-operator/internal/persist"
	uniontypes "github.com/navikt/union-operator/internal/types"
)

// noOpPersister satisfies persist.AllowlistPersister without making any real
// external calls, making it safe to use in unit and integration tests.
type noOpPersister struct{}

func (noOpPersister) PersistAllowlist(_ context.Context, _ *datanavnov1.UnionTeamServiceAccounts) error {
	return nil
}

// Compile-time check.
var _ persist.AllowlistPersister = noOpPersister{}

var _ = Describe("UnionTeamServiceAccounts Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		unionteamserviceaccounts := &datanavnov1.UnionTeamServiceAccounts{}

		BeforeEach(func() {
			By("creating the target namespace for reconciled resources")
			targetNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-project-test-domain",
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: targetNs.Name}, targetNs)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, targetNs)).To(Succeed())
			}

			By("creating the custom resource for the Kind UnionTeamServiceAccounts")
			err = k8sClient.Get(ctx, typeNamespacedName, unionteamserviceaccounts)
			if err != nil && errors.IsNotFound(err) {
				resource := &datanavnov1.UnionTeamServiceAccounts{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: datanavnov1.UnionTeamServiceAccountsSpec{
						Project: "test-project",
						Domain:  "test-domain",
						ServiceAccounts: []datanavnov1.UnionServiceAccount{
							{Name: "test-sa"},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &datanavnov1.UnionTeamServiceAccounts{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance UnionTeamServiceAccounts")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &UnionTeamServiceAccountsReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				UnionConfig: &uniontypes.UnionDataplaneConfig{},
				Persister:   noOpPersister{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
