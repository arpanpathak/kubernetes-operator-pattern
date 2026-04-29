package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================================
// Chapter 7: Finalizers (Modular Implementation)
// FILE: api/v1/database_types.go
// ============================================================================

var GroupVersion = schema.GroupVersion{Group: "database.myorg.com", Version: "v1"}
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &MockDatabase{}, &MockDatabaseList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// MockDatabase is our API type representing an external Cloud Database.
type MockDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec and Status are omitted for brevity in this finalizer-focused example.
}

func (in *MockDatabase) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	out := new(MockDatabase)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	return out
}

type MockDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MockDatabase `json:"items"`
}

func (in *MockDatabaseList) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	out := new(MockDatabaseList)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MockDatabase, len(*in))
		for i := range *in {
			(*in)[i] = *(*out)[i].DeepCopyObject().(*MockDatabase)
		}
	}
	return out
}
