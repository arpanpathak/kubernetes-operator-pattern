package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1 "hands_on/ch08_status/api/v1"
)

// ============================================================================
// Chapter 8: Status Conditions (Modular Implementation)
// FILE: internal/controller/status_controller.go
// ============================================================================
// This file demonstrates the industry-standard pattern for managing the 
// Status subresource and updating metav1.Condition arrays.
// 
// If your Operator does not properly report its status, cluster administrators
// are essentially flying blind. They will not know if your database is 
// provisioning, failing, or healthy without manually tailing the Pod logs.
// ============================================================================

// These constants define the strict, machine-readable vocabulary of our Operator.
// 1. ConditionType: The overarching question being asked.
// 2. Reasons: The specific, CamelCase answers to that question.
const (
	ConditionTypeDatabaseReady = "DatabaseReady"
	ReasonProvisioning         = "ProvisioningStarted"
	ReasonProvisioned          = "ProvisionedSuccessfully"
	ReasonFailed               = "APIFailure"
)

type StatusReconciler struct {
	// client.Client is injected by the Manager. It allows us to perform CRUD
	// operations against the local cache (for Reads) and the API Server (for Writes).
	client.Client
}

// Reconcile is the master loop. Notice how we use a helper function to mutate
// the Status array, keeping this function incredibly clean and linear.
func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// ------------------------------------------------------------------------
	// Phase 1: Fetch the Resource
	// ------------------------------------------------------------------------
	resource := &dbv1.MockResource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// ------------------------------------------------------------------------
	// Phase 2: Report Initial State
	// ------------------------------------------------------------------------
	// Before we do any heavy lifting, we immediately update the Status to tell
	// the human user that we have seen their request and are working on it.
	// This prevents the user from wondering if the Operator is broken.
	r.setStatus(ctx, resource, metav1.ConditionFalse, ReasonProvisioning, "Reaching out to AWS API to provision resources. This may take up to 5 minutes.")

	// ------------------------------------------------------------------------
	// Phase 3: Perform the Work (Mocked)
	// ------------------------------------------------------------------------
	// Imagine this function reaches out to the AWS Go SDK.
	err := func() error { return nil }()

	// ------------------------------------------------------------------------
	// Phase 4: Handle Errors & Report Failure State
	// ------------------------------------------------------------------------
	// If the AWS API returns a 500 Error, we MUST report it to the user.
	// We set the Status to False, use the `ReasonFailed` constant, and crucially,
	// we inject the raw `err.Error()` string into the Message field so the
	// administrator can debug it without reading logs.
	if err != nil {
		r.setStatus(ctx, resource, metav1.ConditionFalse, ReasonFailed, err.Error())
		// We return the error so the Workqueue applies exponential backoff.
		return ctrl.Result{}, err
	}

	// ------------------------------------------------------------------------
	// Phase 5: Report Success State
	// ------------------------------------------------------------------------
	// The database is successfully running. We flip the Status to True!
	r.setStatus(ctx, resource, metav1.ConditionTrue, ReasonProvisioned, "Database is online and accepting connections.")
	
	// We return nil. The Reconciler goes to sleep.
	return ctrl.Result{}, nil
}

// setStatus is an essential helper function. Managing the metav1.Condition array
// manually is incredibly error-prone. You have to iterate the slice, find if the
// condition already exists, and manually manage the LastTransitionTime.
//
// By wrapping `meta.SetStatusCondition`, we guarantee that timestamps are only
// updated when the status actually changes, preventing false-positive drift alarms.
func (r *StatusReconciler) setStatus(ctx context.Context, resource *dbv1.MockResource, status metav1.ConditionStatus, reason, message string) {
	
	condition := metav1.Condition{
		Type:    ConditionTypeDatabaseReady,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	// 1. Mutate the array safely.
	meta.SetStatusCondition(&resource.Status.Conditions, condition)
	
	// 2. Push the change to the API Server.
	// CRITICAL: Notice that we do not call `r.Update()`.
	// We call `r.Status().Update()`. This strictly targets the `/status` REST endpoint.
	// This ensures that we do not accidentally overwrite any changes the user might
	// have made to the `Spec` while we were reconciling.
	_ = r.Status().Update(ctx, resource)
}

// SetupWithManager registers this Reconciler with the main Controller Manager.
func (r *StatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.MockResource{}).
		Complete(r)
}
