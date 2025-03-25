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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ollamav1alpha1 "github.com/dmk/ollama-operator/api/v1alpha1"
	"github.com/ollama/ollama/api"
)

// OllamaModelReconciler reconciles a OllamaModel object
type OllamaModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Ollama *api.Client
}

const ollamaModelFinalizer = "ollama.smithforge.dev/finalizer"

// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ollama.smithforge.dev,resources=ollamamodels/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OllamaModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	ollamaModel := &ollamav1alpha1.OllamaModel{}

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
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.Info("reconciling OllamaModel", "name", ollamaModel.Name, "model", modelName)

	// Initialize status if needed
	if ollamaModel.Status.State == "" {
		log.Info("initializing model status", "name", ollamaModel.Name)
		ollamaModel.Status.State = "pending"
		if err := r.Status().Update(ctx, ollamaModel); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check if model exists in Ollama
	showReq := &api.ShowRequest{Name: modelName}
	_, err := r.Ollama.Show(ctx, showReq)
	if err != nil {
		// Model doesn't exist, start pulling
		if ollamaModel.Status.State == "pending" {
			log.Info("starting model pull", "name", ollamaModel.Name, "model", modelName)
			ollamaModel.Status.State = "pulling"
			if err := r.Status().Update(ctx, ollamaModel); err != nil {
				return ctrl.Result{}, err
			}

			// Actually pull the model
			pullReq := &api.PullRequest{Name: modelName}
			err := r.Ollama.Pull(ctx, pullReq, func(resp api.ProgressResponse) error {
				log.Info("pull progress", "model", modelName, "status", resp.Status, "completed", resp.Completed)
				return nil
			})
			if err != nil {
				log.Error(err, "failed to pull model", "model", modelName)
				ollamaModel.Status.State = "failed"
				ollamaModel.Status.Error = err.Error()
				if updateErr := r.Status().Update(ctx, ollamaModel); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, err
			}

			log.Info("model pull completed successfully", "name", ollamaModel.Name, "model", modelName)
			return r.updateModelDetails(ctx, ollamaModel, modelName)
		}
	} else {
		// Model exists, update to ready if not already
		if ollamaModel.Status.State != "ready" {
			log.Info("model already exists, marking as ready", "name", ollamaModel.Name, "model", modelName)
			return r.updateModelDetails(ctx, ollamaModel, modelName)
		}
	}

	return ctrl.Result{}, nil
}

// updateModelDetails updates the OllamaModel details including state, digest, and size
func (r *OllamaModelReconciler) updateModelDetails(ctx context.Context, ollamaModel *ollamav1alpha1.OllamaModel, modelName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Update state to ready
	now := metav1.Now()
	ollamaModel.Status.State = "ready"
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

	if err := r.Status().Update(ctx, ollamaModel); err != nil {
		return ctrl.Result{}, err
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
func (r *OllamaModelReconciler) handleDeletion(ctx context.Context, ollamaModel *ollamav1alpha1.OllamaModel, modelName string) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if the finalizer exists
	if controllerutil.ContainsFinalizer(ollamaModel, ollamaModelFinalizer) {
		// Delete the model from Ollama
		deleteReq := &api.DeleteRequest{Name: modelName}
		if err := r.Ollama.Delete(ctx, deleteReq); err != nil {
			log.Error(err, "failed to delete model from Ollama", "model", modelName)
			// We don't return an error here as we still want to allow deletion of the resource
			// even if the model deletion fails (e.g., if the model doesn't exist)
		} else {
			log.Info("successfully deleted model from Ollama", "model", modelName)
		}

		// Remove the finalizer to allow the resource to be deleted
		controllerutil.RemoveFinalizer(ollamaModel, ollamaModelFinalizer)
		if err := r.Update(ctx, ollamaModel); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OllamaModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.OllamaModel{}).
		Named("ollamamodel").
		Complete(r)
}
