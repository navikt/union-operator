package istio

import (
	"context"
	"fmt"
	"strings"

	datanavnov1 "github.com/navikt/union-operator/api/v1"
	uniontypes "github.com/navikt/union-operator/internal/types"
	istiosecuritymodels "istio.io/api/security/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) ensureAuthorizationPolicyForHost(ctx context.Context, unionEnv *uniontypes.UnionEnv, serviceAccount string, host *datanavnov1.Host, protocol, hostTypeLabel string) error {
	_ = logf.FromContext(ctx)

	apList := &istiosecurity.AuthorizationPolicyList{}
	err := r.List(ctx, apList, inEgressNamespace(), client.MatchingLabels{
		"project":         unionEnv.Project,
		"domain":          unionEnv.Domain,
		"service-account": serviceAccount,
		"host":            host.Host,
	})
	if err != nil {
		return err
	}

	if len(apList.Items) < 1 {
		ap, err := newAuthorizationPolicy(unionEnv, serviceAccount, host, protocol, hostTypeLabel)
		if err != nil {
			return err
		}
		if err = r.Create(ctx, ap); err != nil {
			return err
		}
	}

	return nil
}

func newAuthorizationPolicy(unionEnv *uniontypes.UnionEnv, serviceAccount string, host *datanavnov1.Host, protocol, hostTypeLabel string) (*istiosecurity.AuthorizationPolicy, error) {
	var when []*istiosecuritymodels.Condition
	var to []*istiosecuritymodels.Rule_To
	switch strings.ToUpper(protocol) {
	case httpsProtocol:
		to = []*istiosecuritymodels.Rule_To{
			{
				Operation: &istiosecuritymodels.Operation{
					Hosts: []string{host.Host},
					Paths: host.Paths,
				},
			},
		}
	case tcpProtocol:
		when = []*istiosecuritymodels.Condition{
			{
				Key:    "connection.sni",
				Values: []string{host.Host},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return &istiosecurity.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s-%s", unionEnv.Project, unionEnv.Domain, serviceAccount, host.Name()),
			Namespace: EgressNamespace,
			Labels: map[string]string{
				"project":         unionEnv.Project,
				"domain":          unionEnv.Domain,
				"service-account": serviceAccount,
				"host":            host.Host,
				"host-type":       hostTypeLabel,
			},
		},
		Spec: istiosecuritymodels.AuthorizationPolicy{
			Action: istiosecuritymodels.AuthorizationPolicy_ALLOW,
			Rules: []*istiosecuritymodels.Rule{
				{
					From: []*istiosecuritymodels.Rule_From{
						{
							Source: &istiosecuritymodels.Source{
								Principals: []string{fmt.Sprintf("cluster.local/ns/%s/sa/%s", unionEnv.Namespace(), serviceAccount)},
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
