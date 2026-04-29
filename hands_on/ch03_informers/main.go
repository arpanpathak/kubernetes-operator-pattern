package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

// ============================================================================
// Chapter 3 Hands-On: The Informer & Workqueue Architecture
// ============================================================================
// This is the absolute core pattern of Kubernetes controllers. 
// We combine a SharedIndexInformer (for caching and event listening) with a 
// RateLimitingQueue (for safe, parallel, retryable event processing).
// ============================================================================

// PodController orchestrates the cache and the worker queue.
type PodController struct {
	// clientset allows us to make modifying API calls (e.g., Update Status) if needed.
	clientset *kubernetes.Clientset
	
	// informer is our thread-safe local cache of the cluster state.
	informer cache.SharedIndexInformer
	
	// queue ensures events are processed serially per-key, and handles exponential backoff on errors.
	queue workqueue.RateLimitingInterface
}

// NewPodController wires the Informer's event stream into the Workqueue.
func NewPodController(clientset *kubernetes.Clientset, informer cache.SharedIndexInformer) *PodController {
	// The DefaultControllerRateLimiter provides exponential backoff (e.g., 5ms, 10ms, 20ms... up to 1000s)
	// which prevents our controller from taking down the API server if it hits a persistent error.
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// We attach Event Handlers. Notice that the ONLY thing these handlers do is extract
	// the string key ("namespace/name") and push it to the queue. They do NOT contain business logic.
	// This ensures the Informer thread is never blocked.
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil { queue.Add(key) }
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil { queue.Add(key) }
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamespaceKeyFunc handles the edge case where the object was deleted 
			// while the controller was disconnected, and it only exists as a "Tombstone" in the cache.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil { queue.Add(key) }
		},
	})

	return &PodController{
		clientset: clientset,
		informer:  informer,
		queue:     queue,
	}
}

// Run starts the controller threads and blocks until stopCh is closed.
func (c *PodController) Run(workers int, stopCh <-chan struct{}) {
	// HandleCrash catches panics in our goroutines, preventing the whole binary from dying.
	defer runtime.HandleCrash()
	// ShutDown ensures the queue is drained and closed cleanly when the controller stops.
	defer c.queue.ShutDown()

	fmt.Println("[INFO] Booting Pod Controller...")
	
	// CRITICAL STEP: We must wait for the Informer to perform its initial "List" operation
	// and fully populate the local memory cache. If we start workers before this, they will
	// think the cluster is empty!
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		fmt.Println("[CRITICAL] Failed to synchronize caches from API Server. Exiting.")
		return
	}
	fmt.Println("[SUCCESS] Local cache synchronized. Starting worker threads.")

	// Spin up the requested number of parallel worker goroutines.
	// wait.Until will restart the runWorker function every 1 second if it unexpectedly exits,
	// until stopCh is closed.
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh // Block forever until termination signal.
	fmt.Println("[INFO] Shutting down Pod Controller gracefully.")
}

// runWorker is a simple infinite loop that pulls keys from the queue.
func (c *PodController) runWorker() {
	// processNextItem returns false only when the queue is shut down.
	for c.processNextItem() {}
}

// processNextItem is the safety wrapper around our business logic.
func (c *PodController) processNextItem() bool {
	// 1. Pop a key from the queue. This blocks if the queue is empty.
	keyInterface, quit := c.queue.Get()
	if quit {
		return false
	}
	key := keyInterface.(string)

	// 2. ALWAYS call Done() when finished processing, or the queue will assume the item is still in-flight
	// and will never process another event for this specific key.
	defer c.queue.Done(key)

	// 3. Execute the actual business logic (Reconciliation).
	err := c.reconcile(key)
	
	// 4. Error Handling
	if err != nil {
		fmt.Printf("[ERROR] Reconciliation failed for %s: %v. Re-queueing with rate limit penalty.\n", key, err)
		c.queue.AddRateLimited(key)
		return true
	}

	// 5. Success: Tell the queue to forget the rate-limit history for this key.
	c.queue.Forget(key)
	return true
}

// reconcile contains the actual business logic. Notice the early-return guard clauses!
func (c *PodController) reconcile(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// If the key is malformed, returning an error here would just cause an infinite retry loop.
		// Instead, we return nil to drop it.
		fmt.Printf("[WARNING] Invalid resource key format: %s\n", key)
		return nil
	}

	// CRITICAL: Fetch the object from the Informer's LOCAL CACHE (GetIndexer), NOT the API Server.
	// This makes our operator lightning fast and gentle on the cluster.
	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("local cache lookup failed: %v", err)
	}

	// Guard Clause: Handle Deletions
	if !exists {
		fmt.Printf("[EVENT] Pod %s/%s was deleted. (Cleanup logic would go here)\n", namespace, name)
		return nil
	}

	// The object exists. Cast it safely.
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("object retrieved from cache is not a Pod")
	}

	// Perform your business logic here!
	fmt.Printf("[EVENT] Reconciled Pod %s/%s | Phase: %-10s | IP: %s\n", namespace, name, pod.Status.Phase, pod.Status.PodIP)
	
	return nil
}

// main sets up the dependencies and runs the controller.
func main() {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Create a SharedInformerFactory. "Shared" means if we create 5 different controllers
	// that all need to watch Pods, they will all share the single underlying Cache and Watch connection.
	// The 10*time.Minute parameter is the Resync Period (forces a synthetic Update event for all items periodically).
	factory := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	
	// Request an Informer specifically for Core V1 Pods.
	podInformer := factory.Core().V1().Pods().Informer()

	// Instantiate our custom controller.
	controller := NewPodController(clientset, podInformer)

	stopCh := make(chan struct{})
	
	// Start all informers requested from the factory. This opens the HTTP Watch connection.
	factory.Start(stopCh)
	
	// Block and run the controller with 2 parallel workers.
	controller.Run(2, stopCh)
}
