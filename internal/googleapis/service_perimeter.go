package googleapis

import (
	"context"
	"errors"
	"fmt"
	"slices"

	v1 "github.com/navikt/union-operator/api/v1"
	"github.com/navikt/union-operator/internal/types"
	accesscontextmanager "google.golang.org/api/accesscontextmanager/v1"
)

const servicePerimeterFQN = "accessPolicies/756121543316/servicePerimeters/dataplattform_perimeter_dev"

func EnsureServicePerimeter(ctx context.Context, unionEnv *types.UnionEnv) error {
	var errs []error
	for _, sa := range unionEnv.ServiceAccounts {
		for _, api := range sa.PrivateGoogleAPIs {
			err := ensureEgressPolicy(ctx, unionEnv.GoogleServiceAccountEmail(sa.Name), api)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func CleanupUnusedEgressPolicies(ctx context.Context) error {
}

func ensureEgressPolicy(ctx context.Context, sa string, api v1.GoogleAPI) error {
	accesscontextmanagerService, err := accesscontextmanager.NewService(ctx)
	if err != nil {
		return err
	}

	servicePerimeter, err := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Get(servicePerimeterFQN).Do()
	if err != nil {
		return err
	}

	if egressPolicyExists(servicePerimeter, sa) {
		return nil
	}

	perimeterUpdateRequest := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Patch(servicePerimeterFQN, &accesscontextmanager.ServicePerimeter{
		Status: &accesscontextmanager.ServicePerimeterConfig{
			EgressPolicies: append(servicePerimeter.Status.EgressPolicies, &accesscontextmanager.EgressPolicy{
				Title: sa,
				EgressFrom: &accesscontextmanager.EgressFrom{
					Identities: []string{fmt.Sprintf("serviceAccount:%s", sa)},
				},
				EgressTo: &accesscontextmanager.EgressTo{
					Resources: []string{fmt.Sprintf("projects/%d", api.ProjectNumber)},
					Operations: []*accesscontextmanager.ApiOperation{
						{
							ServiceName: api.Name,
							MethodSelectors: []*accesscontextmanager.MethodSelector{
								{
									Method: "*",
								},
							},
						},
					},
				},
			}),
		},
	})

	perimeterUpdateRequest.UpdateMask("status.restricted_services,status.egress_policies")
	_, err = perimeterUpdateRequest.Do()
	return err
}

func egressPolicyExists(servicePerimeter *accesscontextmanager.ServicePerimeter, sa string) bool {
	for _, p := range servicePerimeter.Status.EgressPolicies {
		if slices.Contains(p.EgressFrom.Identities, fmt.Sprintf("serviceAccount:%s", sa)) {
			fmt.Println("found existsing rule")
			return true
		}
	}
	return false
}
