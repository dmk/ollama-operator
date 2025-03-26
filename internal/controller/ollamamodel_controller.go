/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ollamamodel "github.com/dmk/ollama-operator/api/v1alpha1"
	"github.com/ollama/ollama/api"
)

// OllamaClient defines the interface for interacting with the Ollama API
type OllamaClient interface {
	Delete(ctx context.Context, req *api.DeleteRequest) error
	Show(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error)
	Pull(ctx context.Context, req *api.PullRequest, fn api.PullProgressFunc) error
	List(ctx context.Context) (*api.ListResponse, error)
}

// OllamaModelReconciler reconciles a OllamaModel object
type OllamaModelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Ollama   OllamaClient
	Recorder record.EventRecorder
}

const ollamaModelFinalizer = "ollama.smithforge.dev/finalizer"

// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OllamaModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	ollamaModel := &ollamamodel.OllamaModel{}

	if err := r.Get(ctx, req.NamespacedName, ollamaModel); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Construct the full model name (e.g., "llama2:7b")
	modelName := fmt.Sprintf("%s:%s", ollamaModel.Spec.Name, ollamaModel.Spec.Tag)

	// Check if the model is being deleted
	if !ollamaModel.DeletionTimestamp.IsZero() {
		log.Info("handling deletion of model", "name", ollamaModel.Name, "model", modelName)
		return r.handleDeletion(ctx, ollamaModel, modelName)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(ollamaModel, ollamaModelFinalizer) {
		log.Info("adding finalizer", "name", ollamaModel.Name)
		controllerutil.AddFinalizer(ollamaModel, ollamaModelFinalizer)
		if err := r.Update(ctx, ollamaModel); err != nil {
			// If update fails, retry after a short delay
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
		return ctrl.Result{}, nil
	}

	log.Info("reconciling OllamaModel", "name", ollamaModel.Name, "model", modelName)

	// Check for refresh annotation
	if val, exists := ollamaModel.Annotations["ollama.smithforge.dev/refresh"]; exists && val == "true" {
		log.Info("refresh annotation detected, forcing model refresh", "name", ollamaModel.Name, "model", modelName)
		return r.refreshModel(ctx, ollamaModel, modelName)
	}

	// Initialize status if needed
	if ollamaModel.Status.State == "" {
		log.Info("initializing model status", "name", ollamaModel.Name)
		ollamaModel.Status.State = ollamamodel.StatePending
		if err := r.Status().Update(ctx, ollamaModel); err != nil {
			// If update fails, retry after a short delay
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
		return ctrl.Result{}, nil
	}

	// Check if model exists in Ollama
	showReq := &api.ShowRequest{Name: modelName}
	_, err := r.Ollama.Show(ctx, showReq)
	if err != nil {
		// Model doesn't exist, start pulling
		if ollamaModel.Status.State == ollamamodel.StatePending {
			log.Info("starting model pull", "name", ollamaModel.Name, "model", modelName)
			ollamaModel.Status.State = ollamamodel.StatePulling
			if err := r.Status().Update(ctx, ollamaModel); err != nil {
				// If update fails, retry after a short delay
				return ctrl.Result{RequeueAfter: time.Second * 5}, err
			}

			// Actually pull the model
			pullReq := &api.PullRequest{Name: modelName}
			err := r.Ollama.Pull(ctx, pullReq, func(resp api.ProgressResponse) error {
				log.Info("pull progress", "model", modelName, "status", resp.Status, "completed", resp.Completed)
				return nil
			})
			if err != nil {
				log.Error(err, "failed to pull model", "model", modelName)
				ollamaModel.Status.State = ollamamodel.StateFailed
				ollamaModel.Status.Error = err.Error()
				if updateErr := r.Status().Update(ctx, ollamaModel); updateErr != nil {
					// If update fails, retry after a short delay
					return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
				}
				// Return error to trigger retry
				return ctrl.Result{RequeueAfter: time.Second * 30}, err
			}

			log.Info("model pull completed successfully", "name", ollamaModel.Name, "model", modelName)
			return r.updateModelDetails(ctx, ollamaModel, modelName)
		}
	} else {
		// Model exists, update to ready if not already
		if ollamaModel.Status.State != ollamamodel.StateReady {
			log.Info("model already exists, marking as ready", "name", ollamaModel.Name, "model", modelName)
			return r.updateModelDetails(ctx, ollamaModel, modelName)
		}
	}

	return ctrl.Result{}, nil
}

// updateModelDetails updates the OllamaModel details including state, digest, and size
func (r *OllamaModelReconciler) updateModelDetails(ctx context.Context, ollamaModel *ollamamodel.OllamaModel, modelName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Update state to ready
	now := metav1.Now()
	ollamaModel.Status.State = ollamamodel.StateReady
	ollamaModel.Status.LastPullTime = &now

	// Get model details
	showReq := &api.ShowRequest{Name: modelName}
	showResp, err := r.Ollama.Show(ctx, showReq)
	if err == nil && showResp != nil {
		// Get digest from show response
		if showResp.Modelfile != "" {
			// Use first 64 chars of the modelfile hash as digest
			digest := fmt.Sprintf("%064x", showResp.Modelfile)
			if len(digest) > 64 {
				digest = digest[:64]
			}
			ollamaModel.Status.Digest = digest
		}

		// Get the model size by listing models
		listResp, listErr := r.Ollama.List(ctx)
		if listErr == nil {
			// Find the model in the list
			for _, model := range listResp.Models {
				// Check if this is our model
				if model.Name == modelName {
					// Update the size from the list response
					ollamaModel.Status.Size = model.Size
					// Set the formatted size
					ollamaModel.Status.FormattedSize = formatBytes(model.Size)
					log.Info("updated model size", "model", modelName, "size", model.Size, "formattedSize", ollamaModel.Status.FormattedSize)
					break
				}
			}
		} else {
			log.Error(listErr, "failed to list models to get size", "model", modelName)
		}
	}

	// Use exponential backoff for status updates
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := r.Status().Update(ctx, ollamaModel); err != nil {
			if i == maxRetries-1 {
				return ctrl.Result{}, err
			}
			// Wait with exponential backoff before retrying
			time.Sleep(time.Second * time.Duration(1<<uint(i)))
			continue
		}
		break
	}

	return ctrl.Result{}, nil
}

// formatBytes converts bytes to a human-readable string (e.g., "4.2 GiB")
func formatBytes(bytes int64) string {
	const (
		_          = iota
		KB float64 = 1 << (10 * iota)
		MB
		GB
		TB
		PB
	)

	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	var value float64
	var unit string

	switch {
	case bytes >= int64(PB):
		value = float64(bytes) / PB
		unit = "PiB"
	case bytes >= int64(TB):
		value = float64(bytes) / TB
		unit = "TiB"
	case bytes >= int64(GB):
		value = float64(bytes) / GB
		unit = "GiB"
	case bytes >= int64(MB):
		value = float64(bytes) / MB
		unit = "MiB"
	case bytes >= int64(KB):
		value = float64(bytes) / KB
		unit = "KiB"
	}

	return fmt.Sprintf("%.1f %s", value, unit)
}

// handleDeletion handles the deletion of a model when the OllamaModel resource is deleted
func (r *OllamaModelReconciler) handleDeletion(ctx context.Context, ollamaModel *ollamamodel.OllamaModel, modelName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if the finalizer exists
	if controllerutil.ContainsFinalizer(ollamaModel, ollamaModelFinalizer) {
		// Delete the model from Ollama with retries
		maxRetries := 3
		var deleteErr error
		for i := 0; i < maxRetries; i++ {
			deleteReq := &api.DeleteRequest{Name: modelName}
			deleteErr = r.Ollama.Delete(ctx, deleteReq)
			if deleteErr == nil {
				break
			}
			// If model not found, that's fine - it's already deleted
			if strings.Contains(deleteErr.Error(), "model not found") {
				deleteErr = nil
				break
			}
			// Wait with exponential backoff before retrying
			time.Sleep(time.Second * time.Duration(1<<uint(i)))
		}

		if deleteErr != nil {
			log.Error(deleteErr, "failed to delete model from Ollama after retries", "model", modelName)
			// We don't return an error here as we still want to allow deletion of the resource
			// even if the model deletion fails
		} else {
			log.Info("successfully deleted model from Ollama", "model", modelName)
		}

		// Remove the finalizer to allow the resource to be deleted
		controllerutil.RemoveFinalizer(ollamaModel, ollamaModelFinalizer)
		if err := r.Update(ctx, ollamaModel); err != nil {
			// If update fails, retry after a short delay
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	return ctrl.Result{}, nil
}

// refreshModel forces a model to be re-pulled and updates its status
func (r *OllamaModelReconciler) refreshModel(ctx context.Context, ollamaModel *ollamamodel.OllamaModel, modelName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Record event for refresh start
	r.Recorder.Event(ollamaModel, "Normal", "RefreshStarted", fmt.Sprintf("Starting refresh of model %s", modelName))

	// Set state to pulling to indicate a refresh is in progress
	ollamaModel.Status.State = ollamamodel.StatePulling
	if err := r.Status().Update(ctx, ollamaModel); err != nil {
		// If update fails, retry after a short delay
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Pull the model with retries
	maxRetries := 3
	var pullErr error
	for i := 0; i < maxRetries; i++ {
		pullReq := &api.PullRequest{Name: modelName}
		pullErr = r.Ollama.Pull(ctx, pullReq, func(resp api.ProgressResponse) error {
			log.Info("refresh progress", "model", modelName, "status", resp.Status, "completed", resp.Completed)
			return nil
		})
		if pullErr == nil {
			break
		}
		// Wait with exponential backoff before retrying
		time.Sleep(time.Second * time.Duration(1<<uint(i)))
	}

	if pullErr != nil {
		log.Error(pullErr, "failed to refresh model after retries", "model", modelName)
		ollamaModel.Status.State = ollamamodel.StateFailed
		ollamaModel.Status.Error = pullErr.Error()

		// Record event for refresh failure
		r.Recorder.Event(ollamaModel, "Warning", "RefreshFailed",
			fmt.Sprintf("Failed to refresh model %s: %v", modelName, pullErr))

		if updateErr := r.Status().Update(ctx, ollamaModel); updateErr != nil {
			// If update fails, retry after a short delay
			return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, pullErr
	}

	// Update the model details
	result, err := r.updateModelDetails(ctx, ollamaModel, modelName)
	if err != nil {
		return result, err
	}

	// Update the annotation to indicate the refresh is complete
	if ollamaModel.Annotations == nil {
		ollamaModel.Annotations = make(map[string]string)
	}
	ollamaModel.Annotations["ollama.smithforge.dev/refresh"] = fmt.Sprintf("completed-%s", time.Now().Format(time.RFC3339))
	if err := r.Update(ctx, ollamaModel); err != nil {
		// If update fails, retry after a short delay
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Record event for successful refresh
	r.Recorder.Event(ollamaModel, "Normal", "RefreshCompleted",
		fmt.Sprintf("Successfully refreshed model %s (size: %s)", modelName, ollamaModel.Status.FormattedSize))

	log.Info("model refresh completed successfully", "name", ollamaModel.Name, "model", modelName)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OllamaModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamamodel.OllamaModel{}).
		Named("ollamamodel").
		Complete(r)
}
