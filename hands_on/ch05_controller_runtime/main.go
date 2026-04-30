package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ============================================================================
// Chapter 5 Hands-On: The Controller-Runtime Manager and Reconciler
// ============================================================================
// This file demonstrates the absolute bare-minimum required to start a Manager
// and register a Reconciler using the high-level `controller-runtime` library.
// Notice how much simpler this is compared to the raw Informers in Chapter 3!
// ============================================================================

// MockReconciler satisfies the reconcile.Reconciler interface.
type MockReconciler struct {
	// client.Client is injected by the Manager. It allows us to perform CRUD
	// operations against the local cache (for Reads) and the API Server (for Writes).
	client.Client
}

// Reconcile is the heart of the operator.
// It receives a Request containing ONLY the Name and Namespace of the object.
// The object itself is not passed, forcing you to fetch the latest state from cache.
func (r *MockReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 1. Log the event
	fmt.Printf("[RECONCILE LOOP TRIGGERED] Resource: %s/%s\n", req.Namespace, req.Name)

	// In a real operator, you would:
	// 1. Fetch the object using r.Get(ctx, req.NamespacedName, &myObject)
	// 2. Perform logic
	// 3. Return ctrl.Result{}

	// For this skeleton, we just pretend everything was successful.
	// Returning an empty Result{} and nil error tells the Workqueue to remove
	// this item and go back to sleep.
	return ctrl.Result{}, nil
}

// SetupWithManager registers our Reconciler with the Controller Manager.
// We tell the Manager which object type should trigger our Reconcile loop.
func (r *MockReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Note: Because we haven't defined a real CRD in this skeleton, we are using
	// corev1.Pod just to prove the compiler accepts it.
	// In a real project, this would be `.For(&api.AppService{})`
	
	// return ctrl.NewControllerManagedBy(mgr).
	// 	For(&corev1.Pod{}).
	// 	Complete(r)
	
	return nil
}

func main() {
	// ------------------------------------------------------------------------
	// STEP 1: Configure Logging
	// ------------------------------------------------------------------------
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	fmt.Println("[INFO] Initializing Controller Manager...")

	// ------------------------------------------------------------------------
	// STEP 2: Instantiate the Manager
	// ------------------------------------------------------------------------
	// The Manager orchestrates the API connection, Informers, Webhooks, and Metrics.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: runtime.NewScheme(), // In a real project, we populate this Scheme first
	})
	if err != nil {
		fmt.Printf("[CRITICAL ERROR] Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// STEP 3: Register the Reconciler
	// ------------------------------------------------------------------------
	reconciler := &MockReconciler{
		Client: mgr.GetClient(),
	}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		fmt.Printf("[CRITICAL ERROR] Unable to wire up controller: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// STEP 4: Start the Manager
	// ------------------------------------------------------------------------
	// Start blocks forever. SetupSignalHandler ensures it listens for SIGTERM/SIGINT
	// and cleanly drains the Workqueues before exiting.
	fmt.Println("[SUCCESS] Manager fully initialized. Starting loop.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("[CRITICAL ERROR] Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
