package googleapis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/navikt/union-operator/api/v1"
	"github.com/navikt/union-operator/internal/types"
	accesscontextmanager "google.golang.org/api/accesscontextmanager/v1"
)

const servicePerimeterFQN = "accessPolicies/756121543316/servicePerimeters/dataplattform_perimeter_dev"

func EnsureServicePerimeter(ctx context.Context, serviceAccounts []types.ServiceAccount) error {
	var errs []error
	for _, sa := range serviceAccounts {
		for _, api := range sa.PrivateGoogleAPIs {
			err := ensureEgressPolicy(ctx, sa, api)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func CleanupUnusedEgressPolicies(ctx context.Context, utsas []v1.UnionServiceAccount) error {
	//	policies := make(map[string]bool)
	//	for _, utsa := range utsas {
	//		if len(utsa.PrivateGoogleAPIs) < 0 {
	//			policies[utsa.Name] = true
	//		}
	//
	//	}
	//
	//	accesscontextmanagerService, err := accesscontextmanager.NewService(ctx)
	//	if err != nil {
	//		return err
	//	}
	//
	//	servicePerimeter, err := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Get(servicePerimeterFQN).Do()
	//	if err != nil {
	//		return err
	//	}

	return nil
}

func ensureEgressPolicy(ctx context.Context, sa types.ServiceAccount, api v1.GoogleAPI) error {
	accesscontextmanagerService, err := accesscontextmanager.NewService(ctx)
	if err != nil {
		return err
	}

	servicePerimeter, err := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Get(servicePerimeterFQN).Do()
	if err != nil {
		return err
	}

	serviceAccountEmail := sa.GoogleServiceAccountEmail()

	// if egressPolicyExists(servicePerimeter, sa) {
	// 	return nil
	// }

	// dataplattform-development-dataplattform-biguery-googleapis-com

	var egressPolicies []*accesscontextmanager.EgressPolicy

	if egressPolicyExists(servicePerimeter, sa, api) {
		for _, ep := range servicePerimeter.Status.EgressPolicies {
			if ep.Title == egressPolicyName(sa, api) {
				ep.EgressFrom.Identities = []string{fmt.Sprintf("serviceAccount:%s", serviceAccountEmail)}
				for _, identity := range api.ImpersonatedAccounts {
					ep.EgressFrom.Identities = append(ep.EgressFrom.Identities, fmt.Sprintf("serviceAccount:%s", identity))
				}
			}

			egressPolicies = append(egressPolicies, ep)
		}
	} else {
		egressPolicies = servicePerimeter.Status.EgressPolicies
		egressPolicies = append(egressPolicies, &accesscontextmanager.EgressPolicy{
			Title: egressPolicyName(sa, api),
			EgressFrom: &accesscontextmanager.EgressFrom{
				Identities: []string{fmt.Sprintf("serviceAccount:%s", serviceAccountEmail)},
			},
			EgressTo: &accesscontextmanager.EgressTo{
				Resources: []string{fmt.Sprintf("projects/%d", api.ProjectNumber)},
				Operations: []*accesscontextmanager.ApiOperation{
					{
						ServiceName: api.ServiceName,
						MethodSelectors: []*accesscontextmanager.MethodSelector{
							{
								Method: "*",
							},
						},
					},
				},
			},
		},
		)
	}

	perimeterUpdateRequest := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Patch(servicePerimeterFQN, &accesscontextmanager.ServicePerimeter{
		Status: &accesscontextmanager.ServicePerimeterConfig{
			EgressPolicies: egressPolicies,
		},
	})

	perimeterUpdateRequest.UpdateMask("status.restricted_services,status.egress_policies")
	_, err = perimeterUpdateRequest.Do()
	return err
}

func egressPolicyExists(servicePerimeter *accesscontextmanager.ServicePerimeter, sa types.ServiceAccount, api v1.GoogleAPI) bool {
	for _, p := range servicePerimeter.Status.EgressPolicies {
		if p.Title == egressPolicyName(sa, api) {
			return true
		}
	}
	return false
}

func egressPolicyName(sa types.ServiceAccount, api v1.GoogleAPI) string {
	return fmt.Sprintf("%s-%s-%s-%s-%d",
		sa.Project, sa.Domain[:3], sa.Name,
		strings.Split(api.ServiceName, ".")[0], api.ProjectNumber)
}
