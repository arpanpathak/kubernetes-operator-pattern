package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webappv1 "hands_on/ch06_building_operator/api/v1"
)

// ============================================================================
// Chapter 6 Hands-On: End-to-End Implementation
// FILE: internal/controller/appservice_controller.go
// ============================================================================
// This file contains the absolute gold-standard of Operator Reconcile loops.
// It is completely flattened. There are NO nested if/else blocks.
// Everything delegates to strictly-typed helper methods using guard clauses.
//
// By reading this file top-to-bottom, you will understand exactly how 
// declarative reconciliation is supposed to be written in Go.
// ============================================================================

// AppServiceReconciler reconciles an AppService object.
type AppServiceReconciler struct {
	// client.Client is injected by the Manager. It provides cache-backed read
	// operations and direct-to-api write operations.
	client.Client
	
	// Scheme is injected by the Manager. We need this to establish the 
	// OwnerReference between the AppService and the child Deployment.
	Scheme *runtime.Scheme
}

// Reconcile is the master control loop.
// Notice how it reads like an instruction manual. Linear, predictable, and testable.
func (r *AppServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// ------------------------------------------------------------------------
	// Phase 1: Fetch the Primary Resource from the Local Cache
	// ------------------------------------------------------------------------
	// The `req` only contains a Name and Namespace. We must fetch the actual
	// struct from the Informer's memory cache.
	appService := &webappv1.AppService{}
	if err := r.Get(ctx, req.NamespacedName, appService); err != nil {
		if errors.IsNotFound(err) {
			// The CR was deleted. OwnerReferences will garbage collect the child
			// deployment automatically. We just go back to sleep.
			return ctrl.Result{}, nil
		}
		// A real error occurred (e.g., API server down). Return it to the queue for retry.
		return ctrl.Result{}, err
	}

	// ------------------------------------------------------------------------
	// Phase 2: Fetch the corresponding child Deployment
	// ------------------------------------------------------------------------
	// We expect the child deployment to have the exact same name as the parent.
	foundDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: appService.Name, Namespace: appService.Namespace}, foundDeployment)

	// ------------------------------------------------------------------------
	// Phase 3: Guard Clause - Handle Creation
	// ------------------------------------------------------------------------
	// If the deployment doesn't exist, this is a brand new CR.
	if err != nil {
		if errors.IsNotFound(err) {
			// Delegate the heavy lifting to a helper function, then return early.
			return r.createDeployment(ctx, appService)
		}
		return ctrl.Result{}, err
	}

	// ------------------------------------------------------------------------
	// Phase 4: Guard Clause - Handle Updates (Drift Correction)
	// ------------------------------------------------------------------------
	// If the deployment exists, we must check if someone manually edited it 
	// (state drift) or if the CR spec changed (desired state changed).
	if r.deploymentNeedsUpdate(appService, foundDeployment) {
		// Delegate to helper, return early.
		return r.updateDeployment(ctx, appService, foundDeployment)
	}

	// ------------------------------------------------------------------------
	// Phase 5: Final Step - Sync Status Subresource
	// ------------------------------------------------------------------------
	// If we reach this line, the actual state perfectly matches desired state.
	// We simply need to tell the user exactly how many pods are actually running right now.
	return r.updateStatus(ctx, appService, foundDeployment)
}

// createDeployment builds and persists a new Deployment, establishing the OwnerReference.
func (r *AppServiceReconciler) createDeployment(ctx context.Context, a *webappv1.AppService) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	
	// Call our struct builder
	desiredDeployment := r.buildDeployment(a)

	logger.Info("Creating child Deployment", "Name", desiredDeployment.Name)
	if err := r.Create(ctx, desiredDeployment); err != nil {
		return ctrl.Result{}, err
	}
	
	// We return Requeue: true. This forces the loop to run again immediately.
	// Why? Because we just created a Deployment, and we want to drop down to 
	// Phase 5 on the next tick so we can update the Status!
	return ctrl.Result{Requeue: true}, nil
}

// deploymentNeedsUpdate explicitly checks for drift between Desired and Actual state.
func (r *AppServiceReconciler) deploymentNeedsUpdate(a *webappv1.AppService, d *appsv1.Deployment) bool {
	if *d.Spec.Replicas != a.Spec.Replicas {
		return true
	}
	if d.Spec.Template.Spec.Containers[0].Image != a.Spec.Image {
		return true
	}
	return false
}

// updateDeployment mutates the existing Deployment to match the desired state.
func (r *AppServiceReconciler) updateDeployment(ctx context.Context, a *webappv1.AppService, d *appsv1.Deployment) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Updating Deployment spec to correct state drift.")

	// Mutate the struct we pulled from the cache
	d.Spec.Replicas = &a.Spec.Replicas
	d.Spec.Template.Spec.Containers[0].Image = a.Spec.Image
	
	// Push the mutated struct to the API Server
	if err := r.Update(ctx, d); err != nil {
		return ctrl.Result{}, err
	}
	
	return ctrl.Result{Requeue: true}, nil
}

// updateStatus reflects the actual cluster state (Running Pods) into the Custom Resource.
func (r *AppServiceReconciler) updateStatus(ctx context.Context, a *webappv1.AppService, d *appsv1.Deployment) (ctrl.Result, error) {
	// If the numbers already match, do nothing. Avoid unnecessary API Server PUTs.
	if a.Status.AvailableReplicas == d.Status.AvailableReplicas {
		return ctrl.Result{}, nil
	}

	a.Status.AvailableReplicas = d.Status.AvailableReplicas
	if a.Status.AvailableReplicas == a.Spec.Replicas {
		a.Status.Phase = "Running"
	} else {
		a.Status.Phase = "Scaling"
	}

	// Always use r.Status().Update() to ensure we only touch the subresource.
	if err := r.Status().Update(ctx, a); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildDeployment constructs the complex nested Deployment struct based on the simple AppService struct.
func (r *AppServiceReconciler) buildDeployment(a *webappv1.AppService) *appsv1.Deployment {
	labels := map[string]string{"app": a.Name, "managed-by": "appservice-operator"}
	replicas := a.Spec.Replicas

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name,
			Namespace: a.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: a.Spec.Image,
						Name:  "app-container",
						Ports: []corev1.ContainerPort{{ContainerPort: a.Spec.Port}},
					}},
				},
			},
		},
	}
	
	// IMPORTANT: Setting this establishes a parent-child relationship for Garbage Collection.
	// If the AppService is deleted, the Kubernetes Garbage Collector daemon will automatically
	// delete this Deployment.
	ctrl.SetControllerReference(a, dep, r.Scheme)
	return dep
}

// SetupWithManager registers the Reconciler with the main Controller Manager.
func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.AppService{}).           // This triggers the loop when AppService changes
		Owns(&appsv1.Deployment{}).            // This triggers the loop when the Child Deployment changes (e.g. a Pod dies)
		Complete(r)
}
