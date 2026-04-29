package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================================
// Chapter 8: Status Conditions (Modular Implementation)
// FILE: api/v1/database_types.go
// ============================================================================

var GroupVersion = schema.GroupVersion{Group: "database.myorg.com", Version: "v1"}
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &MockResource{}, &MockResourceList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// MockStatus contains the extremely important Conditions array.
type MockStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type MockResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            MockStatus `json:"status,omitempty"`
}

func (in *MockResource) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	out := new(MockResource)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	// Properly clone the conditions array
	if in.Status.Conditions != nil {
		in, out := &in.Status.Conditions, &out.Status.Conditions
		*out = make([]metav1.Condition, len(*in))
		copy(*out, *in)
	}
	return out
}

type MockResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MockResource `json:"items"`
}

func (in *MockResourceList) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	out := new(MockResourceList)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MockResource, len(*in))
		for i := range *in {
			(*in)[i] = *(*out)[i].DeepCopyObject().(*MockResource)
		}
	}
	return out
}
