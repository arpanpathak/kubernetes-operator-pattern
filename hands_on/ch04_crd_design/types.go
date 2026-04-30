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
//
// Every line below, especially the `main()` bootstrap, is exhaustively documented
// to explain exactly what the controller-runtime engine is doing under the hood.
// ============================================================================

// ----------------------------------------------------------------------------
// API Registration (The Scheme Builder)
// ----------------------------------------------------------------------------
// GroupVersion defines the REST API path that Kubernetes will dynamically
// generate for us. In this case, our API will be reachable at:
// /apis/webapp.mydomain.com/v1/...
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
// The List object is required by the API server so that `kubectl get appservices`
// has a properly formatted JSON array to return.
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
		
		// CRITICAL: We MUST allocate a brand new underlying array for the slice in memory. 
		// If we simply wrote `out.Items = in.Items`, Go would copy the slice header, but both 
		// the new and old slice would point to the exact same underlying array in RAM. 
		// Any mutation to `out` would mutate `in`, completely defeating the purpose of a DeepCopy!
		*outItems = make([]AppService, len(*inItems))
		
		for i := range *inItems {
			// We iterate through the original array (inItems), call DeepCopyObject on each individual item,
			// safely cast it back to an AppService struct, and store it in our newly allocated array (outItems).
			(*outItems)[i] = *(*inItems)[i].DeepCopyObject().(*AppService)
		}
	}
	return out 
}

// ----------------------------------------------------------------------------
// Main Bootstrap
// ----------------------------------------------------------------------------
func main() {
	// ------------------------------------------------------------------------
	// 1. Logger Initialization
	// ------------------------------------------------------------------------
	// We use the high-performance Zap logger provided by controller-runtime.
	// Development mode enables human-readable stack traces and debug-level logs,
	// whereas production mode outputs structured JSON for log aggregators (like ELK).
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ------------------------------------------------------------------------
	// 2. Scheme Registration
	// ------------------------------------------------------------------------
	// The Scheme is a massive, thread-safe registry that maps Go structs
	// (like `corev1.Pod` or our `AppService`) to their corresponding
	// Kubernetes API Group, Version, and Kind (GVK) strings.
	scheme := runtime.NewScheme()
	
	// First, we register all native Kubernetes types (Pods, Services, etc.)
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	
	// CRITICAL: Next, we register our Custom Resource types. 
	// If we forget this line, the Manager will panic and crash when it receives
	// an `AppService` event from the API Server because it won't know how to decode it.
	utilruntime.Must(SchemeBuilder.AddToScheme(scheme)) 

	fmt.Println("[BOOTSTRAP] Validating Schema Registration...")

	// ------------------------------------------------------------------------
	// 3. Manager Instantiation
	// ------------------------------------------------------------------------
	// The Manager is the orchestrator of the entire controller-runtime framework.
	// When we call NewManager, it automatically finds our Kubeconfig (if running
	// locally) or the injected ServiceAccount token (if running inside a Pod),
	// and establishes a connection pool to the API Server.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager. Is your cluster running?: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[SUCCESS] API Types successfully registered with the Scheme.")
	fmt.Println("[BOOTSTRAP] Starting empty Manager (No controllers attached yet)...")

	// ------------------------------------------------------------------------
	// 4. Starting the Event Loop
	// ------------------------------------------------------------------------
	// mgr.Start blocks the main thread permanently. It boots up the Informers
	// and metrics servers. 
	// Note: In this specific chapter, we haven't attached a Reconciler yet, 
	// so the Manager just sits idly, proving that our Scheme and API Types 
	// are perfectly valid and the binary compiles.
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
