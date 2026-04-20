package googleapis

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/navikt/union-operator/api/v1"
	"github.com/navikt/union-operator/internal/types"
	accesscontextmanager "google.golang.org/api/accesscontextmanager/v1"
)

const servicePerimeterFQN = "accessPolicies/756121543316/servicePerimeters/dataplattform_perimeter_dev"

func EnsureServicePerimeter(ctx context.Context, unionEnv *types.UnionEnv) error {
	var errs []error
	for _, sa := range unionEnv.ServiceAccounts {
		for _, api := range sa.PrivateGoogleAPIs {
			err := ensureEgressPolicy(ctx, unionEnv, sa.Name, api)
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

func ensureEgressPolicy(ctx context.Context, unionEnv *types.UnionEnv, sa string, api v1.GoogleAPI) error {
	accesscontextmanagerService, err := accesscontextmanager.NewService(ctx)
	if err != nil {
		return err
	}

	servicePerimeter, err := accesscontextmanagerService.AccessPolicies.ServicePerimeters.Get(servicePerimeterFQN).Do()
	if err != nil {
		return err
	}

	serviceAccountEmail := unionEnv.GoogleServiceAccountEmail(sa)

	// if egressPolicyExists(servicePerimeter, sa) {
	// 	return nil
	// }


// dataplattform-development-dataplattform-biguery-googleapis-com

	var egressPolicies []*accesscontextmanager.EgressPolicy

	if egressPolicyExists(servicePerimeter, unionEnv.Project, unionEnv.Domain, sa, api) {
		for _, ep := range servicePerimeter.Status.EgressPolicies {
			if ep.Title == api.EgressPolicyName(unionEnv.Project, unionEnv.Domain, sa) {
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
				Title: api.EgressPolicyName(unionEnv.Project, unionEnv.Domain, sa),
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

func egressPolicyExists(servicePerimeter *accesscontextmanager.ServicePerimeter, project, domain, sa string, api v1.GoogleAPI) bool {
	for _, p := range servicePerimeter.Status.EgressPolicies {
		if p.Title == api.EgressPolicyName(project, domain, sa) {
			return true
		}
	}
	return false
}
