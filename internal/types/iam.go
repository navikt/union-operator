package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "iam.cnrm.cloud.google.com", Version: "v1beta1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion} //nolint:staticcheck
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&IAMPolicyMember{}, &IAMPolicyMemberList{}, &IAMServiceAccount{})
}

// Condition is a GCP IAM Condition (see
// https://cloud.google.com/config-connector/docs/reference/resource-docs/iam/iampolicymember#condition).
type Condition struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Expression  string `json:"expression"`
}
type ResourceRef struct {
	ApiVersion string  `json:"apiVersion"`
	External   *string `json:"external,omitempty"`
	Kind       string  `json:"kind"`
	Name       *string `json:"name,omitempty"`
}
type IAMPolicyMemberSpec struct {
	Member      string      `json:"member"`
	Role        string      `json:"role"`
	Condition   *Condition  `json:"condition,omitempty"`
	ResourceRef ResourceRef `json:"resourceRef"`
}
type IAMPolicyMemberStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

type IAMPolicyMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IAMPolicyMemberSpec   `json:"spec"`
	Status            IAMPolicyMemberStatus `json:"status,omitempty"`
}

type IAMPolicyMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMPolicyMember `json:"items"`
}

type IAMServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IAMServiceAccountSpec `json:"spec"`
}

type IAMServiceAccountSpec struct {
	DisplayName string `json:"displayName"`
}

func (in *IAMPolicyMemberList) DeepCopyInto(out *IAMPolicyMemberList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]IAMPolicyMember, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *IAMPolicyMemberList) DeepCopy() *IAMPolicyMemberList {
	if in == nil {
		return nil
	}
	out := new(IAMPolicyMemberList)
	in.DeepCopyInto(out)
	return out
}

func (in *IAMPolicyMemberList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *IAMPolicyMember) DeepCopyInto(out *IAMPolicyMember) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *IAMPolicyMember) DeepCopy() *IAMPolicyMember {
	if in == nil {
		return nil
	}
	out := new(IAMPolicyMember)
	in.DeepCopyInto(out)
	return out
}

func (in *IAMPolicyMember) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *IAMPolicyMemberSpec) DeepCopyInto(out *IAMPolicyMemberSpec) {
	*out = *in
	if in.Condition != nil {
		in, out := &in.Condition, &out.Condition
		*out = new(Condition)
		**out = **in
	}
	in.ResourceRef.DeepCopyInto(&out.ResourceRef)
}

func (in *IAMPolicyMemberSpec) DeepCopy() *IAMPolicyMemberSpec {
	if in == nil {
		return nil
	}
	out := new(IAMPolicyMemberSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ResourceRef) DeepCopyInto(out *ResourceRef) {
	*out = *in
	if in.External != nil {
		in, out := &in.External, &out.External
		*out = new(string)
		**out = **in
	}
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
}

func (in *ResourceRef) DeepCopy() *ResourceRef {
	if in == nil {
		return nil
	}
	out := new(ResourceRef)
	in.DeepCopyInto(out)
	return out
}

func (in *IAMPolicyMemberStatus) DeepCopyInto(out *IAMPolicyMemberStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *IAMPolicyMemberStatus) DeepCopy() *IAMPolicyMemberStatus {
	if in == nil {
		return nil
	}
	out := new(IAMPolicyMemberStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *IAMServiceAccount) DeepCopyInto(out *IAMServiceAccount) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
}

func (in *IAMServiceAccount) DeepCopy() *IAMServiceAccount {
	if in == nil {
		return nil
	}
	out := new(IAMServiceAccount)
	in.DeepCopyInto(out)
	return out
}

func (in *IAMServiceAccount) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
