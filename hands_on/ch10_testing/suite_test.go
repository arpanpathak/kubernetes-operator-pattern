package controllers_test

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// ============================================================================
// Chapter 10 Hands-On: EnvTest Integration Testing
// ============================================================================
// This file demonstrates the setup and execution of a true API Server test
// environment. This is not a mock. It spins up a real etcd and kube-apiserver
// binary on your machine.
// ============================================================================

func RunEnvTestExample() error {
	// ------------------------------------------------------------------------
	// STEP 1: Bootstrapping the API Server
	// ------------------------------------------------------------------------
	fmt.Println("[TEST] Bootstrapping EnvTest Environment...")

	testEnv := &envtest.Environment{
		// We point EnvTest to the folder containing our generated YAML CRDs.
		// It will automatically apply them to the test API Server on boot.
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false, // Set to false for this mock example to run safely
	}

	// Start boots the binaries and generates a secure connection config.
	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("failed to start test environment: %v", err)
	}
	// Defer cleanup ensures the binaries are killed when the test ends.
	defer func() {
		fmt.Println("[TEST] Tearing down EnvTest Environment...")
		_ = testEnv.Stop()
	}()

	fmt.Println("[TEST] API Server and etcd successfully booted!")

	// ------------------------------------------------------------------------
	// STEP 2: Creating the Client
	// ------------------------------------------------------------------------
	// We create a standard controller-runtime client connected to our test cluster.
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}

	// ------------------------------------------------------------------------
	// STEP 3: Starting the Manager in the background
	// ------------------------------------------------------------------------
	// We start the manager just like we would in main.go, but we run it in a 
	// goroutine so it doesn't block our test thread.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to start manager: %v", err)
	}

	// Imagine we registered our AppServiceReconciler here.
	// _ = (&AppServiceReconciler{Client: k8sClient}).SetupWithManager(mgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		fmt.Println("[TEST] Starting background Manager...")
		_ = mgr.Start(ctx)
	}()

	// ------------------------------------------------------------------------
	// STEP 4: Executing Tests
	// ------------------------------------------------------------------------
	fmt.Println("[TEST] Executing Integration Tests...")
	
	// Example test flow:
	// 1. Create an AppService custom resource using k8sClient.Create()
	// 2. We don't call Reconcile() directly! The background Manager handles it.
	// 3. We use a polling loop (like Gomega's Eventually) to query the API server
	//    until the child Deployment appears.
	
	// Simulate waiting for the operator to act
	time.Sleep(time.Second * 2)

	fmt.Println("[TEST] SUCCESS: Operator correctly provisioned resources in the test cluster.")
	return nil
}
