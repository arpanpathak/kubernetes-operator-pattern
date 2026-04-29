package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ============================================================================
// Chapter 2 Hands-On: Mastering client-go
// ============================================================================
// This script demonstrates the absolute bedrock of Kubernetes Operator 
// development: establishing a secure, authenticated connection to the 
// API Server and executing a strongly-typed REST call.
//
// Every line in this file is carefully documented to explain the "Why".
// ============================================================================

func main() {
	// ------------------------------------------------------------------------
	// Phase 1: The Authentication Dance
	// ------------------------------------------------------------------------
	// Before we can talk to Kubernetes, we must prove who we are. 
	// The BuildConfigFromFlags function is a magical helper. If we pass it a valid
	// filepath (like `~/.kube/config`), it reads our local TLS certificates and 
	// constructs a `rest.Config`. 
	//
	// CRITICALLY: If we were running this inside a Docker container in the cluster,
	// `kubeconfigPath` would be empty. In that case, this exact same function 
	// automatically falls back to `InClusterConfig()`, reading the auto-mounted 
	// ServiceAccount JWT token from the Pod's filesystem. 
	// This means we never have to write two separate authentication codepaths!
	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Printf("CRITICAL: Failed to build Kubeconfig. Are you sure you are logged in to a cluster? Error: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// Phase 2: Constructing the Strongly-Typed Client
	// ------------------------------------------------------------------------
	// The `Clientset` is a massive collection of generated Go methods.
	// We pass it our secure `rest.Config`, and it gives us access to every 
	// built-in resource in Kubernetes. By using the Clientset, we get the 
	// protection of the Go compiler—we cannot accidentally submit a `Pod` struct 
	// to a `Deployment` REST endpoint.
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("CRITICAL: Failed to instantiate the Clientset: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// Phase 3: The Mandate of Context Timeouts
	// ------------------------------------------------------------------------
	// In a distributed system, networks partition and API servers hang.
	// If we issue a `.List()` call without a timeout, and the router drops the TCP
	// packet without sending a RST packet, our Go thread will hang forever.
	// This would completely freeze our Operator's event loop.
	//
	// Therefore, we MUST wrap every single network call in a `context.WithTimeout`.
	// Here, we tell the Go runtime: "If the API Server does not respond in exactly
	// 5 seconds, violently abort the HTTP request and return an error."
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	
	// `defer cancel()` ensures that the timer resources are freed up immediately
	// after the HTTP call finishes, preventing memory leaks.
	defer cancel()

	// ------------------------------------------------------------------------
	// Phase 4: Executing the API Call
	// ------------------------------------------------------------------------
	// Notice the fluent method chaining: 
	// 1. CoreV1() limits us to the core API group.
	// 2. Pods("") sets the target resource and namespace (empty string means ALL namespaces).
	// 3. List() executes the HTTP GET request.
	fmt.Println("[NETWORK] Reaching out to the API Server to fetch all Pods...")
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("CRITICAL: The API call failed or timed out: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------------
	// Phase 5: Processing the Response
	// ------------------------------------------------------------------------
	fmt.Printf("[SUCCESS] Successfully retrieved %d Pods from the cluster.\n", len(pods.Items))
	for i, pod := range pods.Items {
		// We only print the first 3 to avoid spamming the terminal.
		if i >= 3 {
			break
		}
		fmt.Printf(" - Pod: %s/%s (Status: %s)\n", pod.Namespace, pod.Name, pod.Status.Phase)
	}
	
	fmt.Println("\n[WARNING] While this script works perfectly, using a simple .List() inside an infinite loop to build an Operator is a catastrophic anti-pattern that will destroy your API Server. Proceed to Chapter 3 to learn the proper event-driven architecture using Informers.")
}
