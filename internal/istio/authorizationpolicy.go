package istio

import (
	"context"
	"fmt"
	"strings"

	uniontypes "github.com/navikt/union-operator/internal/types"
	istiosecuritymodels "istio.io/api/security/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type authPolicyData struct {
	HostName string
	Host     string
	Paths    []string
}

func (a *authPolicyData) Name(sa uniontypes.ServiceAccount) string {
	return fmt.Sprintf("%s-%s-%s-%s", sa.Project, sa.Domain, sa.Name, strings.ReplaceAll(a.HostName, ".", "-"))
}

func (r *Reconciler) ensureAuthorizationPolicyForHost(ctx context.Context, sa uniontypes.ServiceAccount, authPolicyData *authPolicyData, parentHost *string, protocol, hostTypeLabel string) error {
	log := logf.FromContext(ctx)

	parent := authPolicyData.HostName
	if parentHost != nil {
		parent = *parentHost
	}

	apList := &istiosecurity.AuthorizationPolicyList{}
	err := r.List(ctx, apList, inEgressNamespace(), client.MatchingLabels{
		"project":         sa.Project,
		"domain":          sa.Domain,
		"service-account": sa.Name,
		"host":            authPolicyData.HostName,
		"parent-host":     parent,
	})
	if err != nil {
		return err
	}

	if len(apList.Items) < 1 {
		ap, err := newAuthorizationPolicy(sa, authPolicyData, parent, protocol, hostTypeLabel)
		if err != nil {
			return err
		}

		if err = r.Create(ctx, ap); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Stale cache: the AuthorizationPolicy exists but the label-index
				// in the informer cache has not caught up yet.
				log.V(1).Info("AuthorizationPolicy already exists (stale cache)", "name", ap.Name)
				return nil
			}
			return err
		}
	}

	return nil
}

func newAuthorizationPolicy(sa uniontypes.ServiceAccount, authPolicyData *authPolicyData, parentHost, protocol, hostTypeLabel string) (*istiosecurity.AuthorizationPolicy, error) {
	var when []*istiosecuritymodels.Condition
	var to []*istiosecuritymodels.Rule_To
	switch strings.ToUpper(protocol) {
	case httpsProtocol:
		to = []*istiosecuritymodels.Rule_To{
			{
				Operation: &istiosecuritymodels.Operation{
					Hosts: []string{authPolicyData.Host},
					Paths: authPolicyData.Paths,
				},
			},
		}
	case tcpProtocol:
		when = []*istiosecuritymodels.Condition{
			{
				Key:    "connection.sni",
				Values: []string{authPolicyData.Host},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return &istiosecurity.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      authPolicyData.Name(sa),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"project":         sa.Project,
				"domain":          sa.Domain,
				"service-account": sa.Name,
				"host":            authPolicyData.HostName,
				"host-type":       hostTypeLabel,
				"parent-host":     parentHost,
				managedByLabel:    managedByValue,
			},
		},
		Spec: istiosecuritymodels.AuthorizationPolicy{
			Action: istiosecuritymodels.AuthorizationPolicy_ALLOW,
			Rules: []*istiosecuritymodels.Rule{
				{
					From: []*istiosecuritymodels.Rule_From{
						{
							Source: &istiosecuritymodels.Source{
								Principals: []string{fmt.Sprintf("cluster.local/ns/%s/sa/%s", sa.Namespace(), sa.Name)},
							},
						},
					},
					When: when,
					To:   to,
				},
			},
		},
	}, nil
}
