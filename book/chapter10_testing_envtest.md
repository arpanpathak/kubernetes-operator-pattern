# Chapter 10: The Crucible: Integration Testing with EnvTest

[← Previous: Chapter 9](./chapter09_webhooks.md) | [Back to Index](./index.md)

---

Writing a Kubernetes Operator is only half the battle. The other half is proving that it actually works.

Testing an Operator is notoriously difficult. If you try to write standard Go unit tests, you will quickly find yourself mocking the `client.Client` interface. You will write a test that passes a fake object into your Reconcile loop, and asserts that a mock `Create` function was called.

This approach is practically useless. 

The hardest bugs in Operator development do not happen in your business logic. They happen in the interactions with the Kubernetes API Server. Did you forget to set the `OwnerReference` correctly? Did your OpenAPI schema validation reject a valid float value? Did the API Server reject your `Status` update because you didn't configure the subresource? Mock clients cannot test these behaviors.

To solve this, the Kubernetes SIG-Testing team built a phenomenal framework called **EnvTest**.

## 10.1 The Magic of EnvTest

EnvTest does not mock the Kubernetes API. Instead, it downloads and spins up a *real, local, ephemeral instance* of the `kube-apiserver` and `etcd` binaries directly on your laptop or CI server. 

```text
+-------------------------------------------------------+
|                 YOUR GO TEST SUITE                    |
|                                                       |
|  1. Start EnvTest ---> Boots up real `etcd`           |
|                        Boots up real `kube-apiserver` |
|                                                       |
|  2. Start Manager ---> Connects to ephemeral API      |
|                        Starts Reconcile Loop in bkgd  |
|                                                       |
|  3. Run Tests -------> `k8sClient.Create(obj)`        |
|                        Wait for Operator to react     |
+-------------------------------------------------------+
```

It takes approximately 1.5 seconds to boot. It does **not** spin up a Kubelet or a Scheduler. This means if you use EnvTest to create a `Pod`, it will stay in the `Pending` state forever, because there are no worker nodes to schedule it on.

However, since our Operator logic usually just watches for resources and creates other resources in the API Server (which we demonstrated in Chapter 6), this is perfectly fine! We are testing the *Control Plane logic*, not the runtime execution.

## 10.2 The Integration Test Pattern (The Eventually Block)

Once the suite is running, you write integration tests that act like a human user typing `kubectl` commands.

### Step 1: The Input (Desired State)
You create a Custom Resource struct in memory and call `k8sClient.Create(ctx, cr)`.
At this moment, the API server accepts the object. Your background Operator's Informer immediately catches the event and triggers your Reconcile loop on another thread.

### Step 2: The Polling Loop (Actual State)
Because the Reconcile loop is running asynchronously, you cannot immediately assert that the child resources exist. If you check instantly, the test will fail because the Reconciler hasn't finished.

Instead, we use a polling loop. In the Ginkgo testing framework, this is the `Eventually` block.

Here is the exact, complete, flawless implementation of an EnvTest Integration test using `Ginkgo` and `Gomega`:

```go
package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	webappv1 "hands_on/ch06_building_operator/api/v1"
)

var _ = Describe("AppService Controller", func() {
	Context("When creating an AppService", func() {
		It("Should automatically provision a Deployment with the correct replicas", func() {
			ctx := context.Background()

			// 1. The Input (Desired State)
			appService := &webappv1.AppService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: webappv1.AppServiceSpec{
					Replicas: 3,
					Image:    "nginx:latest",
					Port:     8080,
				},
			}
			Expect(k8sClient.Create(ctx, appService)).Should(Succeed())

			// 2. The Polling Loop (Actual State)
			lookupKey := types.NamespacedName{Name: "test-app", Namespace: "default"}
			createdDeployment := &appsv1.Deployment{}

			// We poll the API Server every 250ms for up to 10 seconds.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdDeployment)
				if err != nil {
					return false // Keep trying, it hasn't been created yet
				}
				
				// Assert the internal state is exactly what we commanded
				return *createdDeployment.Spec.Replicas == 3
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})
})
```

This block will query the real API server every 250 milliseconds. If your operator works correctly, it will create the Deployment, the `Get` call will succeed, the replica count will match, and the test will pass. If your operator crashes or encounters an RBAC error, the 10-second timeout will hit, and the test will fail.

## 10.4 The Ultimate Proof of Reliability

By writing tests using EnvTest, you are guaranteeing that your Operator interacts perfectly with a true Kubernetes API Server. You are verifying your JSON schemas, your webhooks, your RBAC permissions, and your Reconcile loops in a single, lightning-fast test suite.

This marks the end of our journey. You now possess the theoretical knowledge, the architectural mental models, and the practical code patterns to build enterprise-grade, highly reliable Kubernetes Operators in Go.
