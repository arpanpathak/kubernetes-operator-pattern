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

	webv1 "hands_on/ch09_webhooks/api/v1"
)

// ============================================================================
// Chapter 9: Admission Webhooks (Modular Implementation)
// FILE: cmd/main.go
// ============================================================================
// The main.go file is the entry point of our Operator binary.
// Note that in this specific chapter, we do not instantiate a Reconciler.
// We are only instantiating the Manager to run the HTTPS Webhook Server.
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
	// We must register the native Kubernetes structs AND our custom structs,
	// otherwise the API client will panic when it tries to unmarshal a response.
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(webv1.AddToScheme(scheme))

	// ------------------------------------------------------------------------
	// 3. Manager Instantiation
	// ------------------------------------------------------------------------
	// The Manager automatically connects to the cluster and spins up the Cache.
	// Crucially for this chapter, it also prepares the internal HTTPS server
	// on port 9443 to receive traffic from the Kubernetes API Server.
	fmt.Println("[BOOTSTRAP] Initializing Webhook Manager...")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("CRITICAL: Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 4. Webhook Wiring
	// ------------------------------------------------------------------------
	// We call SetupWebhookWithManager, which tells the Manager to route incoming
	// HTTPS AdmissionReviews for "AppService" resources directly to the
	// ValidateCreate/Update/Delete functions we defined in api/v1.
	fmt.Println("[BOOTSTRAP] Registering Validating Webhook...")
	appService := &webv1.AppService{}
	if err := appService.SetupWebhookWithManager(mgr); err != nil {
		fmt.Printf("CRITICAL: Unable to create webhook: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// 5. Starting the HTTPS Server
	// ------------------------------------------------------------------------
	// IMPORTANT: For this to actually work in a live cluster, you must inject
	// TLS Certificates into the Pod. Webhooks STRICTLY require HTTPS.
	// You cannot use HTTP. Tools like `cert-manager` handle this injection
	// automatically in a real production environment.
	fmt.Println("[BOOTSTRAP] Starting HTTPS Server on port 9443...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("CRITICAL: Problem running manager: %v\n", err)
		os.Exit(1)
	}
}
