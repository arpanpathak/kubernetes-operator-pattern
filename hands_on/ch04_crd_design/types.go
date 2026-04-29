package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================================
// Chapter 4 Hands-On: CRDs and API Design (End-to-End Bootstrap)
// ============================================================================
// This file demonstrates the exact architecture of a Custom Resource Definition 
// implemented in Go, complete with OpenAPI Validation Markers and the 
// mandatory DeepCopy implementations.
// ============================================================================

// ----------------------------------------------------------------------------
// API Registration (The Scheme Builder)
// ----------------------------------------------------------------------------
// We must declare our API Group ("webapp.mydomain.com") and Version ("v1").
// The API Server will dynamically generate REST endpoints at this path.
var GroupVersion = schema.GroupVersion{Group: "webapp.mydomain.com", Version: "v1"}

// SchemeBuilder is a helper that adds our custom Go structs to the global Scheme.
// Without this, the underlying REST client would not know how to unmarshal 
// the JSON bytes coming from the API Server into our `AppService` struct.
var SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &AppService{}, &AppServiceList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
})

// ----------------------------------------------------------------------------
// The Desired State (The Spec)
// ----------------------------------------------------------------------------
// AppServiceSpec is where the human user defines what they want.
// We use `+kubebuilder` markers to push data validation to the API Server.
// If a user submits `replicas: -5`, the API Server will instantly reject it
// with an HTTP 400, protecting our Reconcile loop from handling corrupt data.
type AppServiceSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32  `json:"replicas"`
	
	// +kubebuilder:validation:MinLength=1
	Image    string `json:"image"`
	
	// +kubebuilder:validation:Minimum=80
	// +kubebuilder:validation:Maximum=65535
	Port     int32  `json:"port"`
}

// ----------------------------------------------------------------------------
// The Root Object (The Structural Trinity)
// ----------------------------------------------------------------------------
// +kubebuilder:object:root=true
type AppService struct {
	// TypeMeta handles the JSON APIVersion and Kind
	metav1.TypeMeta   `json:",inline"`
	// ObjectMeta handles Name, Namespace, Labels, and Finalizers
	metav1.ObjectMeta `json:"metadata,omitempty"`
	
	// Spec is the human's desired state.
	Spec              AppServiceSpec `json:"spec,omitempty"`
}

// ----------------------------------------------------------------------------
// The DeepCopy Mandate
// ----------------------------------------------------------------------------
// This is the most critical part of this chapter.
// To prevent developers from accidentally mutating pointers fetched from the 
// Informer's Local Cache (which would cause global state corruption), 
// Kubernetes demands that every struct implements `runtime.Object`.
// This interface requires a `DeepCopyObject()` method.
func (in *AppService) DeepCopyObject() runtime.Object { 
	if in == nil { return nil }
	out := new(AppService)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	return out
}

// ----------------------------------------------------------------------------
// The List Object
// ----------------------------------------------------------------------------
// +kubebuilder:object:root=true
type AppServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppService `json:"items"`
}

func (in *AppServiceList) DeepCopyObject() runtime.Object { 
	if in == nil { return nil }
	out := new(AppServiceList)
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		inItems, outItems := &in.Items, &out.Items
		*outItems = make([]AppService, len(*inItems))
		for i := range *inItems {
			(*inItems)[i] = *(*outItems)[i].DeepCopyObject().(*AppService)
		}
	}
	return out 
}

// ----------------------------------------------------------------------------
// Main Bootstrap
// ----------------------------------------------------------------------------
func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(SchemeBuilder.AddToScheme(scheme)) // CRITICAL: Register our Custom Types!

	fmt.Println("[BOOTSTRAP] Validating Schema Registration...")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Port:   9443,
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[SUCCESS] API Types successfully registered with the Scheme.")
	fmt.Println("[BOOTSTRAP] Starting empty Manager (No controllers attached yet)...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
