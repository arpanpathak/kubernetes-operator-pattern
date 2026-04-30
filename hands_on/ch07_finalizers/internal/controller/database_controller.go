package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbv1 "hands_on/ch07_finalizers/api/v1"
)

// ============================================================================
// Chapter 7: Finalizers (Modular Implementation)
// FILE: internal/controller/database_controller.go
// ============================================================================
// This file contains the Reconciler for managing external infrastructure.
// Because the Operator provisions resources outside of the Kubernetes cluster
// (like an AWS RDS Database), we cannot rely on native OwnerReferences for
// garbage collection.
//
// We must implement a strict Finalizer lifecycle to ensure the AWS Database
// is cleanly destroyed before the Kubernetes Custom Resource is deleted from etcd.
// ============================================================================

// finalizerString is the unique identifier we will inject into the metadata.finalizers array.
// It acts as a lock. As long as this string exists on the object, the API Server
// will refuse to delete it from etcd.
const finalizerString = "database.myorg.com/cleanup-finalizer"

type DatabaseReconciler struct {
	// client.Client is injected by the Manager. It allows us to perform CRUD
	// operations against the local cache (for Reads) and the API Server (for Writes).
	client.Client
}

// Reconcile is the master control loop.
// We strictly avoid nested if-else statements here. Instead, we use linear Guard Clauses
// to handle the different phases of the finalizer lifecycle.
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// ------------------------------------------------------------------------
	// Phase 1: Fetch the Resource
	// ------------------------------------------------------------------------
	// We retrieve the absolute latest state of the Custom Resource from the local cache.
	resource := &dbv1.MockDatabase{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		// If the error is "NotFound", it means the resource was fully deleted
		// from etcd. Since we use finalizers, this will only happen AFTER our
		// cleanup logic successfully finishes. We return nil to stop reconciling.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// ------------------------------------------------------------------------
	// Phase 2: The Deletion Intercept (Guard Clause)
	// ------------------------------------------------------------------------
	// When a user runs `kubectl delete`, the API server sees our finalizer string.
	// It aborts the true deletion and instead populates the `deletionTimestamp`.
	// We check for that timestamp here. If it exists, the user WANTS to delete this object!
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("Resource is marked for deletion. Executing cleanup sequence.")
		return r.handleDeletion(ctx, resource)
	}

	// ------------------------------------------------------------------------
	// Phase 3: Finalizer Injection (Guard Clause)
	// ------------------------------------------------------------------------
	// If the object is NOT being deleted, we must ensure our finalizer lock is present.
	// If it is missing (which happens immediately after creation), we inject it.
	// We MUST do this BEFORE we provision the external AWS database, otherwise a
	// crash right here would leave an un-trackable database running in AWS forever.
	if err := r.ensureFinalizer(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}

	// ------------------------------------------------------------------------
	// Phase 4: Normal Provisioning Logic
	// ------------------------------------------------------------------------
	// We only reach this point if the object is alive, not being deleted, and safely
	// locked by our finalizer.
	logger.Info("Provisioning the external AWS database...")
	
	// (Mock AWS SDK call goes here)
	
	return ctrl.Result{}, nil
}

// handleDeletion executes the highly dangerous external API cleanup.
func (r *DatabaseReconciler) handleDeletion(ctx context.Context, resource *dbv1.MockDatabase) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Safety Check: Make sure our specific finalizer is actually on the object.
	// If it's already gone, we let Kubernetes proceed with deleting the resource.
	if !controllerutil.ContainsFinalizer(resource, finalizerString) {
		return ctrl.Result{}, nil
	}

	logger.Info("Executing external API call to terminate Cloud Database...")
	
	// --------------------------------------------------------------------
	// CRITICAL FAILURE POINT
	// --------------------------------------------------------------------
	// Imagine we call `aws.DeleteRDSInstance()` here.
	// What happens if the AWS API is down and returns an HTTP 500 error?
	// We MUST return that error back to the Workqueue.
	//
	// Because we return the error, the Workqueue will apply exponential backoff
	// and retry this function in 5 minutes.
	// Because we DID NOT remove the finalizer string, the Kubernetes object remains
	// safely stuck in the "Terminating" phase in etcd. It will not disappear.
	
	// Assuming the AWS API call succeeded...
	logger.Info("External cleanup successful. Removing finalizer lock.")
	
	// We remove the lock string from the array.
	controllerutil.RemoveFinalizer(resource, finalizerString)
	
	// We push the updated array back to the API Server.
	// Once the API Server receives this, it sees the finalizers array is empty,
	// and it instantly deletes the object from etcd. Our job is done.
	if err := r.Update(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureFinalizer safely injects the string into metadata.finalizers.
func (r *DatabaseReconciler) ensureFinalizer(ctx context.Context, resource *dbv1.MockDatabase) error {
	// If it already has the string, do nothing to avoid unnecessary API Server PUTs.
	if controllerutil.ContainsFinalizer(resource, finalizerString) {
		return nil
	}

	log.FromContext(ctx).Info("Adding Finalizer lock to resource")
	controllerutil.AddFinalizer(resource, finalizerString)
	
	// Push the updated object to the API Server. This causes a new Reconcile loop to trigger.
	return r.Update(ctx, resource)
}

// SetupWithManager registers this Reconciler with the main Controller Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.MockDatabase{}). // This tells the Manager to watch MockDatabase resources.
		Complete(r)
}
