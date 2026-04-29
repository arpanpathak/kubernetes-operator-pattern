# Chapter 7: The Nuances of Garbage Collection: OwnerReferences and Custom Finalizers

[← Previous: Chapter 6](./chapter06_building_operator.md) | [Back to Index](./index.md) | [Next: Chapter 8 →](./chapter08_status_conditions.md)

---

A developer builds a highly complex Operator that provisions a Custom Resource called `ManagedDatabase`. When this resource is applied, the Operator creates three Deployments, two Services, a PersistentVolumeClaim, and a Secret containing the database credentials.

Everything works perfectly. The developer proudly ships the code to production.

One week later, an administrator decides they no longer need the database. They run:
`kubectl delete manageddatabase prod-db`

The Custom Resource immediately disappears from the cluster. However, the administrator notices that the Pods are still running. The Persistent Volumes are still consuming costly SAN storage. The cluster is now polluted with "orphaned" resources, and the company is bleeding money.

What happened? The developer failed to implement Kubernetes Garbage Collection.

## 7.1 Understanding the Kubernetes Garbage Collector

Kubernetes does not automatically know that a Deployment was created *because* of a Custom Resource. To the API Server, they are just two independent records in the `etcd` database.

To solve this, Kubernetes runs a background daemon called the **Garbage Collector Controller**. This controller scans the entire cluster looking for specific metadata tags that establish parent-child relationships. These tags are called **OwnerReferences**.

```text
+-----------------------------------+
|  PARENT: ManagedDatabase          |
|  Name: prod-db                    |
|  UID: a1b2c3d4-e5f6...            |
+-----------------------------------+
          |                |
          v                v
+------------------+ +------------------+
| CHILD: Deployment| | CHILD: Secret    |
| OwnerReference:  | | OwnerReference:  |
|  UID: a1b2c3d4   | |  UID: a1b2c3d4   |
+------------------+ +------------------+
```

When you are writing your Operator in Go, you inject this relationship right before creating the child:

```go
// From the Chapter 6 example:
ctrl.SetControllerReference(parentResource, childDeployment, r.Scheme)
```

## 7.2 The Limit of OwnerReferences: External State

OwnerReferences are a miracle worker for resources *inside* the Kubernetes cluster. But what if your Operator manages infrastructure *outside* the cluster?

Imagine you are building an `AWSBucket` operator. Your Reconcile loop uses the AWS SDK to provision an S3 Bucket.

An OwnerReference cannot help you here. The Garbage Collector does not have AWS API keys. If a user deletes the `AWSBucket` CR, Kubernetes will instantly delete it from `etcd`, and that S3 Bucket will live in AWS forever, accumulating charges.

To prevent this, you must intercept the deletion command using a **Finalizer**.

## 7.3 The Deep Mechanics of Finalizers

A finalizer is simply an arbitrary string added to the `metadata.finalizers` array of an object. (e.g., `s3.myorg.com/cleanup-finalizer`).

**The Golden Rule:** If an object has at least one string in its `finalizers` array, the API Server is strictly forbidden from deleting it from `etcd`.

### The Finalizer Lifecycle: A Step-by-Step Architecture

Here is the exact architectural flow of a Finalizer intercepting a deletion command:

```text
1. User types `kubectl delete AWSBucket/my-images`
                        |
                        v
+-------------------------------------------------------+
|                 API SERVER (etcd)                     |
|                                                       |
| - Looks at `metadata.finalizers` array.               |
| - Sees "s3.myorg.com/cleanup-finalizer".              |
| - ABORTS Deletion!                                    |
| - Sets `metadata.deletionTimestamp = "2023-10-27..."` |
+-------------------------------------------------------+
                        |
                        v (Fires Update Event)
+-------------------------------------------------------+
|              OPERATOR RECONCILE LOOP                  |
|                                                       |
| 1. if !deletionTimestamp.IsZero() {                   |
| 2.   Call AWS API: DeleteBucket("my-images")          |
| 3.   Wait for HTTP 200 OK from AWS.                   |
| 4.   Remove string from `metadata.finalizers`         |
| 5.   Call r.Update(ctx, obj)                          |
| }                                                     |
+-------------------------------------------------------+
                        |
                        v (Sends Payload back)
+-------------------------------------------------------+
|                 API SERVER (etcd)                     |
|                                                       |
| - Sees `deletionTimestamp` is set.                    |
| - Sees `finalizers` array is now EMPTY.               |
| - INSTANTLY DELETES object from etcd.                 |
+-------------------------------------------------------+
```

### The Code Implementation

Here is the perfect, fully-fleshed-out Go implementation of the Finalizer pattern using Guard Clauses:

```go
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbv1 "hands_on/ch07_finalizers/api/v1"
)

const finalizerString = "database.myorg.com/cleanup-finalizer"

type DatabaseReconciler struct {
	client.Client
}

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Phase 1: Fetch Resource
	resource := &dbv1.MockDatabase{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Phase 2: The Deletion Intercept (Guard Clause)
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("Resource is marked for deletion. Executing cleanup sequence.")
		return r.handleDeletion(ctx, resource)
	}

	// Phase 3: Finalizer Injection (Guard Clause)
	if err := r.ensureFinalizer(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}

	// Phase 4: Normal Provisioning Logic
	logger.Info("Provisioning the external AWS database...")
	
	// (Mock AWS SDK call goes here)
	
	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) handleDeletion(ctx context.Context, resource *dbv1.MockDatabase) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(resource, finalizerString) {
		return ctrl.Result{}, nil
	}

	logger.Info("Executing external API call to terminate Cloud Database...")
	
	// Assuming the AWS API call succeeded...
	logger.Info("External cleanup successful. Removing finalizer lock.")
	
	controllerutil.RemoveFinalizer(resource, finalizerString)
	if err := r.Update(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) ensureFinalizer(ctx context.Context, resource *dbv1.MockDatabase) error {
	if controllerutil.ContainsFinalizer(resource, finalizerString) {
		return nil
	}

	log.FromContext(ctx).Info("Adding Finalizer lock to resource")
	controllerutil.AddFinalizer(resource, finalizerString)
	return r.Update(ctx, resource)
}
```

### The Danger of the Infinite Retry Loop

What happens if the AWS API is down when `handleDeletion` executes?
Your operator must return the error: `return ctrl.Result{}, err`. 
The Workqueue will apply exponential backoff and retry. Because the finalizer string is *still* on the object, the Custom Resource remains safely stuck in the "Terminating" state in Kubernetes. It will never disappear until the AWS API finally succeeds.

---
*Open `hands_on/ch07_finalizers/internal/controller/database_controller.go` to see this heavily documented, bulletproof implementation of the Finalizer lifecycle running locally.*
