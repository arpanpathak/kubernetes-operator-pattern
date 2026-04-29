# Chapter 8: The Language of the Cluster: Status Subresources and Conditions

[← Previous: Chapter 7](./chapter07_finalizers_gc.md) | [Back to Index](./index.md) | [Next: Chapter 9 →](./chapter09_webhooks.md)

---

Imagine driving a car without a dashboard. You press the accelerator, but you have no speedometer to tell you how fast you are going. You turn the key, but you have no check-engine light to tell you if the motor is failing.

An Operator that does not properly implement the `Status` subresource is exactly like that car.

You might have written a flawless Reconcile loop that perfectly manages thousands of pods, but if your Custom Resource does not constantly broadcast its internal state, the cluster administrators are driving blind.

In this chapter, we will exhaustively cover how to design, protect, and update the `Status` subresource so your Operator behaves like a true, professional Kubernetes citizen.

## 8.1 The Strict Separation of Spec and Status

In Kubernetes API design, there is a fundamental law:
- **`Spec` is the Input.** It is the Desired State. It represents the hopes and dreams of the user. Only the user should modify the Spec.
- **`Status` is the Output.** It is the Actual State. It represents the cold, hard reality of the cluster. **Only the Operator should ever modify the Status.**

### The RBAC Vulnerability

If you simply add a `Status` struct to your Custom Resource Definition without taking further action, you introduce a severe architectural vulnerability. By default, Kubernetes treats the entire object as a single JSON blob. If a user has RBAC permissions to update the `Spec` (which they must have, to use your tool), they implicitly have permission to update the `Status`.

A malicious or confused user could run `kubectl edit` and manually change the `Status` to say "Healthy", even if the pods are crashing.

### The Subresource Solution

To solve this, Kubernetes introduced the concept of **Subresources**. By adding a specific OpenAPI marker to your Go code:

```go
// +kubebuilder:subresource:status
```

You instruct the API Server to physically split your Custom Resource into two distinct REST endpoints:

```text
                                +-------------------+
                                |                   |
                      +-------->|  PUT /.../spec    | (RBAC: Users Allowed)
                      |         |                   |
+----------------+    |         +-------------------+
|  API Server    |----+
|  (Split Router)|    |         +-------------------+
+----------------+    |         |                   |
                      +-------->|  PUT /.../status  | (RBAC: ONLY Operator Allowed)
                                |                   |
                                +-------------------+
```

Furthermore, when your Operator code updates the status using `client.Status().Update()`, the API Server will completely ignore any changes made to the `Spec` in that same payload, preventing race conditions.

## 8.2 Mastering `metav1.Condition`

In the early days of Kubernetes, developers used a simple string field to communicate state, often called `Phase`. This turned out to be horribly inadequate. What if the `Phase` is "Running", but performance is severely degraded?

To solve this, the Kubernetes Architecture Special Interest Group (SIG-Architecture) standardized a deeply detailed structure known as `metav1.Condition`.

A Condition is a highly structured array of boolean flags that provides a granular, multi-dimensional view of the system.

Let's dissect the fields of a `metav1.Condition`:

```json
{
  "type": "DatabaseReady",
  "status": "False",
  "reason": "VolumeProvisioningFailed",
  "message": "AWS EBS volume could not be attached due to quota limits.",
  "lastTransitionTime": "2023-10-27T10:00:00Z",
  "observedGeneration": 4
}
```

### 1. `Type` (The Question)
The `Type` is the specific condition being evaluated. 
**Crucial Best Practice:** The `Type` should always represent an abnormal or positive state, like `Ready`, `Healthy`, or `Degraded`. Never use negative state types like `Failed`. If it failed, the `Type` is `Ready` and the `Status` is `False`.

### 2. `Reason` (The Machine-Readable Explanation)
The `Reason` is a short, PascalCase string intended for machines to read. It allows automation tools (like Flux or ArgoCD) to switch/case on specific failures without parsing human text. Examples: `InvalidCredentials`, `NetworkTimeout`.

### 3. `lastTransitionTime` (The Timestamp)
This is arguably the most important field for debugging. It records the exact microsecond the `Status` flipped from True to False (or vice versa). If a database goes down at 3:00 AM, the cluster admin can look at the `lastTransitionTime` to correlate it with network logs from the exact same minute.

### 4. `observedGeneration` (The Drift Detector)
Every time a user updates the `Spec` of a resource, the API server increments the `metadata.generation` integer. By copying that integer into the `observedGeneration` of your Condition, you are proving to the user: *"I am evaluating Condition X based on Generation 4 of your Spec."* If the user's Spec is at Generation 5, but the Status shows `observedGeneration: 4`, the user instantly knows the Operator is lagging behind or stuck.

## 8.3 Implementing Conditions Safely in Go

Manually managing an array of Conditions is tedious. You have to iterate through the slice, check if a Condition of the same `Type` already exists, and carefully update the `lastTransitionTime` *only* if the `Status` boolean actually flipped.

Thankfully, the `k8s.io/apimachinery` package provides a magical helper function: `meta.SetStatusCondition()`.

Here is the exact, perfect Go implementation of a Reconcile loop utilizing this helper:

```go
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1 "hands_on/ch08_status/api/v1"
)

const (
	ConditionTypeDatabaseReady = "DatabaseReady"
	ReasonProvisioning         = "ProvisioningStarted"
	ReasonProvisioned          = "ProvisionedSuccessfully"
	ReasonFailed               = "APIFailure"
)

type StatusReconciler struct {
	client.Client
}

func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	resource := &dbv1.MockResource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. Initial State
	r.setStatus(ctx, resource, metav1.ConditionFalse, ReasonProvisioning, "Reaching out to AWS API")

	// 2. Perform work (Mocked)
	err := func() error { return nil }()

	// 3. Error State
	if err != nil {
		r.setStatus(ctx, resource, metav1.ConditionFalse, ReasonFailed, err.Error())
		return ctrl.Result{}, err
	}

	// 4. Success State
	r.setStatus(ctx, resource, metav1.ConditionTrue, ReasonProvisioned, "Database is online and accepting connections")
	return ctrl.Result{}, nil
}

func (r *StatusReconciler) setStatus(ctx context.Context, resource *dbv1.MockResource, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    ConditionTypeDatabaseReady,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	// 1. Mutate the array safely.
	meta.SetStatusCondition(&resource.Status.Conditions, condition)
	
	// 2. Push the change to the API Server using the specific Status() endpoint.
	_ = r.Status().Update(ctx, resource)
}
```

---
*Understanding the theory of Conditions is vital, but seeing them implemented in a complete, runnable Go binary is where the knowledge solidifies. Open `hands_on/ch08_status/internal/controller/status_controller.go` to see this implementation locally.*
