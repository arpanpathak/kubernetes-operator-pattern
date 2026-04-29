package v1

import (
	"fmt"
	
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ============================================================================
// Chapter 9: Admission Webhooks (Modular Implementation)
// FILE: api/v1/webhook_types.go
// ============================================================================
// In Kubebuilder projects, Webhook implementation lives directly alongside
// the API Types in the api/v1 directory.
//
// Webhooks are the absolute last line of defense before a malicious or 
// corrupted payload is saved into the etcd database. If you do not write
// a Validating Webhook, you force your Reconcile loop to handle impossible
// states, which leads to panic crashes and infinite retry loops.
// ============================================================================

// ----------------------------------------------------------------------------
// API Struct Definitions
// ----------------------------------------------------------------------------
var GroupVersion = schema.GroupVersion{Group: "webapp.mydomain.com", Version: "v1"}
var SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &AppService{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
})
var AddToScheme = SchemeBuilder.AddToScheme

// AppService is our mock resource. Notice that we don't have OpenAPI validation
// markers here. We are going to handle the validation strictly via the Webhook
// to demonstrate complex cross-field validation.
type AppService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec struct {
		DiskType     string `json:"diskType"`
		StorageClass string `json:"storageClass"`
	} `json:"spec,omitempty"`
}

// DeepCopyObject is the strict mandate from the runtime.Object interface to prevent
// Informer cache corruption.
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
// Webhook Validator Implementation
// ----------------------------------------------------------------------------

// SetupWebhookWithManager registers the webhook server.
// When called, the Manager will spin up an HTTPS server on port 9443 and automatically
// route incoming `AdmissionReview` JSON payloads to the functions defined below.
func (r *AppService) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}

// This line is a clever Go trick. It forces the compiler to verify that our
// `AppService` struct actually implements all the required methods of the
// `webhook.Validator` interface. If we misspelled a function name, the build fails here.
var _ webhook.Validator = &AppService{}

// ValidateCreate is called by the API Server when `kubectl apply` creates a NEW resource.
// We intercept the payload BEFORE it hits etcd.
func (r *AppService) ValidateCreate() (admission.Warnings, error) {
	fmt.Println("[WEBHOOK] Validating Creation Request...")

	// Complex Business Logic: Cross-field validation.
	// OpenAPI cannot evaluate two fields against each other. Webhooks can.
	// If the user requests an SSD, we MUST ensure the storage class is set to "gp3" or "io1".
	if r.Spec.DiskType == "SSD" && r.Spec.StorageClass != "gp3" && r.Spec.StorageClass != "io1" {
		fmt.Println("[WEBHOOK] Validation Failed: Rejecting request.")
		
		// Returning an error here completely rejects the HTTP request.
		// The API Server will immediately abort the transaction.
		// The user typing `kubectl apply` will see this exact error string printed
		// directly in their terminal in red text.
		return nil, fmt.Errorf("validation failed: SSD disks strictly require storageClass 'gp3' or 'io1', but got '%s'", r.Spec.StorageClass)
	}

	// Returning nil means the resource is semantically valid and can be safely saved to etcd.
	fmt.Println("[WEBHOOK] Validation Passed.")
	return nil, nil
}

// ValidateUpdate is called when a user modifies an EXISTING resource.
// Unlike OpenAPI, which only sees the new payload, the Webhook is incredibly powerful:
// It gives us both the OLD state and the NEW state to compare against!
func (r *AppService) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	fmt.Println("[WEBHOOK] Validating Update Request...")

	// We safely cast the generic `runtime.Object` back into our strongly typed struct.
	oldObj, ok := old.(*AppService)
	if !ok { 
		return nil, fmt.Errorf("failed to cast old object") 
	}

	// Complex Business Logic: State-transition validation.
	// We want to prevent users from downgrading an existing database from an SSD to an HDD,
	// because doing so in AWS requires a total cluster rebuild.
	// This type of temporal logic is utterly impossible to do with OpenAPI markers.
	if oldObj.Spec.DiskType == "SSD" && r.Spec.DiskType == "HDD" {
		fmt.Println("[WEBHOOK] Validation Failed: Rejecting disk downgrade.")
		return nil, fmt.Errorf("validation failed: downgrading diskType from SSD to HDD is not supported without data loss")
	}

	// Even if it is an update, we still run the standard creation checks on the new object 
	// to ensure it remains semantically valid.
	return r.ValidateCreate()
}

// ValidateDelete is called when a user runs `kubectl delete`.
// This is rarely used, but can be helpful to prevent accidental deletions of critical resources.
// For example, you could check if `r.Spec.DeletionProtection == true` and return an error here.
func (r *AppService) ValidateDelete() (admission.Warnings, error) {
	fmt.Println("[WEBHOOK] Validating Deletion Request...")
	return nil, nil
}
