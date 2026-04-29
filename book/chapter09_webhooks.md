# Chapter 9: The Pre-Flight Check: Mutating and Validating Admission Webhooks

[← Previous: Chapter 8](./chapter08_status_conditions.md) | [Back to Index](./index.md) | [Next: Chapter 10 →](./chapter10_testing_envtest.md)

---

OpenAPI validation markers (the `+kubebuilder:validation` comments we discussed in Chapter 4) are incredibly powerful. They protect your database from users submitting `replicas: -5` or strings that exceed character limits.

However, OpenAPI validation is purely syntactic. It evaluates a single field in isolation.

What if your Custom Resource has deeply intertwined, interdependent fields? 
Imagine a `Database` resource with two fields: `StorageClass` and `DiskType`. You want to enforce a strict business rule: **"If `DiskType` is set to `SSD`, then `StorageClass` MUST be set to `gp3`. If it is set to `HDD`, it must be `st1`."**

OpenAPI cannot evaluate cross-field logic. 

Furthermore, what if you want to automatically intercept the user's YAML payload and silently inject default values or sidecar containers before it is saved? OpenAPI cannot mutate payloads; it can only reject them.

By the time your Reconcile loop runs, the flawed payload has already been accepted into the `etcd` database. Your Reconciler would have to constantly log errors, and the user's terminal would misleadingly show that the resource was successfully "created".

To intercept the request *before* it hits the database, Kubernetes uses **Admission Webhooks**.

## 9.1 The Admission Flow: A Microsecond Journey

When a human or a CI/CD pipeline runs `kubectl apply`, the JSON payload embarks on a complex, multi-stage journey through the API Server before it is ever committed to disk.

```text
+----------------+      +------------------+      +-------------------+
|                |      | Authentication & |      | Mutating Admission|
| kubectl apply  |----->| Authorization    |----->| Webhooks          |
| (JSON Payload) |      | (RBAC Check)     |      | (Modifies JSON)   |
+----------------+      +------------------+      +-------------------+
                                                            |
                                                            v
+----------------+      +------------------+      +-------------------+
|                |      | Validating       |      | Object Schema     |
|      etcd      |<-----| Admission Webhook|<-----| Validation        |
|                |      | (Final Yes/No)   |      | (OpenAPI limits)  |
+----------------+      +------------------+      +-------------------+
```

1. **Authentication/Authorization**: The API Server checks the user's certificates and RBAC roles.
2. **Mutating Admission Webhooks**: The API Server pauses the request and POSTs the JSON payload to external webhooks registered in the cluster. These webhooks can *modify* the JSON (e.g., inject an Istio sidecar proxy container).
3. **Object Schema Validation**: The API Server checks the mutated JSON against the OpenAPI schema to ensure types and limits are respected.
4. **Validating Admission Webhooks**: The API Server POSTs the mutated, structurally sound JSON to external webhooks for a final yes/no vote. If a webhook replies "No" (with an error message), the entire `kubectl apply` request is rejected. The user sees the error message instantly in their terminal (HTTP 400 Bad Request).
5. **Persist to etcd**: Only if all steps succeed is the object finally saved to the database.

## 9.2 The Complexity of the AdmissionReview Object

Building webhooks manually is a notoriously painful experience. The API Server expects your webhook to be a highly available HTTPS server. When it calls your webhook, it does not send the raw Custom Resource JSON. It sends a deeply nested meta-object called an `AdmissionReview`.

Your HTTP server must:
1. Decode the `AdmissionReview` JSON.
2. Extract the embedded raw bytes of the `OldObject` and the `NewObject`.
3. Unmarshal those bytes into your Go structs.
4. Run your business logic.
5. Create a JSON Patch representing your mutations.
6. Package that JSON Patch back into an `AdmissionResponse` struct and serve it over HTTPS.

## 9.3 The Controller-Runtime Savior

Fortunately, `controller-runtime` reduces this multi-week nightmare into a single Go interface implementation.

The Manager object we introduced in Chapter 5 has a built-in HTTPS webhook server listening on port 9443. 

To create a Validating Webhook, you simply take your Go struct (e.g., `AppService`) and implement the `webhook.Validator` interface.

Here is the exact, complete, flawless implementation of a cross-field Validating Webhook:

```go
package v1

import (
	"fmt"
	
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// We force the compiler to ensure AppService implements Validator
var _ webhook.Validator = &AppService{}

// ValidateCreate handles new objects
func (r *AppService) ValidateCreate() (admission.Warnings, error) {
	if r.Spec.DiskType == "SSD" && r.Spec.StorageClass != "gp3" && r.Spec.StorageClass != "io1" {
		return nil, fmt.Errorf("validation failed: SSD disks strictly require storageClass 'gp3' or 'io1', but got '%s'", r.Spec.StorageClass)
	}
	return nil, nil
}

// ValidateUpdate handles mutated objects and compares them against the old state
func (r *AppService) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	oldObj, ok := old.(*AppService)
	if !ok { 
		return nil, fmt.Errorf("failed to cast old object") 
	}

	// Prevent illegal downgrades
	if oldObj.Spec.DiskType == "SSD" && r.Spec.DiskType == "HDD" {
		return nil, fmt.Errorf("validation failed: downgrading diskType from SSD to HDD is not supported without data loss")
	}

	// Always re-run the creation validation rules
	return r.ValidateCreate()
}

// ValidateDelete handles kubectl delete
func (r *AppService) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

// SetupWebhookWithManager registers the webhook server.
func (r *AppService) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}
```

That is it. The Manager automatically spins up the HTTPS server, automatically unwraps the `AdmissionReview` JSON, dynamically calls your `ValidateCreate` function, takes your returned `error`, wraps it back into an `AdmissionResponse`, and serves it back to the API Server.

If you return an `error`, the user running `kubectl apply` will instantly see:
`Error from server: admission webhook "vappservice.kb.io" denied the request: SSD disks strictly require storageClass 'gp3' or 'io1', but got 'standard'`

---
*Open `hands_on/ch09_webhooks/api/v1/webhook_types.go` to see this complete, deeply documented implementation of a Validating Admission Webhook verifying complex cross-field dependencies.*
