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

	dbv1 "hands_on/ch08_status/api/v1"
	"hands_on/ch08_status/internal/controller"
)

// ============================================================================
// Chapter 8: Status Conditions (Modular Implementation)
// FILE: cmd/main.go
// ============================================================================
// The main.go file is the entry point of our Operator binary. Its sole
// responsibility is to bootstrap the environment, configure the connections
// to the Kubernetes API Server, and wire together the strongly-typed API
// schemas with the Reconciler logic.
// ============================================================================

func main() {
	// ------------------------------------------------------------------------
	// 1. Logger Initialization
	// ------------------------------------------------------------------------
	// We use the high-performance Zap logger. We bind the standard flags so
	// cluster administrators can dynamically change log levels via CLI arguments.
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ------------------------------------------------------------------------
	// 2. Scheme Registration
	// ------------------------------------------------------------------------
	// The Scheme is a thread-safe registry that maps Go structs to their JSON
	// counterparts. We must register the native Kubernetes structs AND our
	// custom database structs, otherwise the API client will panic when it
	// tries to unmarshal a response.
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dbv1.AddToScheme(scheme))

	// ------------------------------------------------------------------------
	// 3. Manager Instantiation
	// ------------------------------------------------------------------------
	// The Manager automatically connects to the cluster (using Kubeconfig or
	// an injected Pod token) and spins up the highly-optimized Informer Cache.
	fmt.Println("[BOOTSTRAP] Initializing Status Manager...")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 4. Controller Wiring
	// ------------------------------------------------------------------------
	// We instantiate our StatusReconciler and inject the Manager's cache-backed
	// client into it.
	fmt.Println("[BOOTSTRAP] Registering StatusReconciler...")
	reconciler := &controller.StatusReconciler{Client: mgr.GetClient()}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		fmt.Printf("CRITICAL: Unable to create controller: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 5. Starting the Event Loop
	// ------------------------------------------------------------------------
	// mgr.Start blocks permanently, listening for Kubernetes events and routing
	// them to our Reconciler's Workqueue.
	fmt.Println("[BOOTSTRAP] Starting Event Loop...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
