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

	dbv1 "hands_on/ch07_finalizers/api/v1"
	"hands_on/ch07_finalizers/internal/controller"
)

// ============================================================================
// Chapter 7: Finalizers (Modular Implementation)
// FILE: cmd/main.go
// ============================================================================
// The main.go file is the entry point of our Operator binary. Its sole
// responsibility is to bootstrap the environment, configure the connections
// to the Kubernetes API Server, and wire together the strongly-typed API
// schemas with the Reconciler logic.
//
// We keep this file strictly focused on initialization. No business logic
// or Reconcile loops should ever be placed here.
// ============================================================================

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
	// (like `corev1.Pod` or our `dbv1.MockDatabase`) to their corresponding
	// Kubernetes API Group, Version, and Kind (GVK) strings.
	// 
	// Why is this critical? When the API Server sends a stream of JSON bytes
	// to our Operator over the network, the underlying REST client needs to
	// know exactly which Go struct to unmarshal those bytes into. Without the
	// Scheme, the client would crash, having no idea what a "MockDatabase" is.
	scheme := runtime.NewScheme()
	
	// First, we register all native Kubernetes types (Pods, Services, etc.)
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	
	// Next, we register our Custom Resource types defined in the api/v1 folder.
	utilruntime.Must(dbv1.AddToScheme(scheme))

	// ------------------------------------------------------------------------
	// 3. Manager Instantiation
	// ------------------------------------------------------------------------
	// The Manager is the orchestrator of the entire controller-runtime framework.
	// When we call NewManager, it automatically finds our Kubeconfig (if running
	// locally) or the injected ServiceAccount token (if running inside a Pod),
	// and establishes a connection pool to the API Server.
	// 
	// It also initializes the SharedIndexInformer cache, the RateLimiting Workqueue,
	// and the Leader Election mechanisms.
	fmt.Println("[BOOTSTRAP] Initializing Finalizer Manager...")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Port:   9443, // The default port for the embedded Admission Webhook server.
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager. This usually means the API server is unreachable: %v\n", err)
		os.Exit(1) // We must crash the binary if we cannot connect to the cluster.
	}

	// ------------------------------------------------------------------------
	// 4. Controller Wiring
	// ------------------------------------------------------------------------
	// We instantiate our custom DatabaseReconciler.
	// Notice that we inject the `mgr.GetClient()` into it. This client is fully
	// cache-backed. When the Reconciler calls `r.Get()`, this client will route
	// the request to the local Informer memory cache, rather than hitting the API Server.
	fmt.Println("[BOOTSTRAP] Registering DatabaseReconciler...")
	reconciler := &controller.DatabaseReconciler{
		Client: mgr.GetClient(),
	}
	
	// We register the Reconciler with the Manager. This tells the Manager's
	// Informer to start watching `MockDatabase` objects and route their events
	// to this specific Reconciler's Workqueue.
	if err = reconciler.SetupWithManager(mgr); err != nil {
		fmt.Printf("CRITICAL: Unable to wire up the controller to the manager: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 5. Starting the Event Loop
	// ------------------------------------------------------------------------
	// mgr.Start blocks the main thread permanently. It boots up the Informers,
	// performs the initial `LIST` to populate the local cache, and then spawns
	// the worker goroutines that execute our Reconcile loops.
	//
	// SetupSignalHandler() listens for OS interrupts (SIGTERM, SIGINT). When the
	// Kubernetes Kubelet tries to restart this Pod, the handler catches the signal
	// and cleanly drains the Workqueue before allowing the binary to exit.
	fmt.Println("[BOOTSTRAP] Configuration successful. Starting the Event Loop...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: The manager encountered a fatal error while running: %v\n", err)
		os.Exit(1)
	}
}
