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

	webappv1 "hands_on/ch06_building_operator/api/v1"
	"hands_on/ch06_building_operator/internal/controller"
)

// ============================================================================
// Chapter 6 Hands-On: End-to-End Implementation
// FILE: cmd/main.go
// ============================================================================
// This is the bootstrap file. It brings together the API types (from api/v1)
// and the Reconciler (from internal/controller), configures the Scheme,
// and starts the controller-runtime Manager.
//
// Every line here is critical for establishing the underlying Informer cache
// and the Reconcile event loop.
// ============================================================================

func main() {
	// ------------------------------------------------------------------------
	// 1. Logger Initialization
	// ------------------------------------------------------------------------
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ------------------------------------------------------------------------
	// 2. Scheme Registration
	// ------------------------------------------------------------------------
	// The Scheme maps JSON payloads from the API server into our Go structs.
	scheme := runtime.NewScheme()
	
	// Register Native Kubernetes Types (Pods, Deployments, Services, etc.)
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register Custom Types (AppService)
	utilruntime.Must(webappv1.AddToScheme(scheme))

	fmt.Println("[BOOTSTRAP] Initializing Controller Manager...")

	// ------------------------------------------------------------------------
	// 3. Manager Instantiation
	// ------------------------------------------------------------------------
	// The Manager orchestrates the API connection and the Cache.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Port:   9443,
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 4. Register the Controller
	// ------------------------------------------------------------------------
	// We inject the client and the scheme into the Reconciler.
	reconciler := &controller.AppServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	
	fmt.Println("[BOOTSTRAP] Wiring up AppService Reconciler...")
	// SetupWithManager registers the Reconciler with the Manager's Informers.
	if err = reconciler.SetupWithManager(mgr); err != nil {
		fmt.Printf("CRITICAL: Unable to wire up controller: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 5. Start the Event Loop
	// ------------------------------------------------------------------------
	// Start blocks forever. SetupSignalHandler ensures it listens for SIGTERM/SIGINT
	// and cleanly drains the Workqueues before exiting.
	fmt.Println("[BOOTSTRAP] Starting manager event loop...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
