package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ollamav1alpha1 "github.com/dmk/ollama-operator/api/v1alpha1"
)

// ModelRequest represents the payload for creating a model
type ModelRequest struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

// ModelResponse represents the API response for a model
type ModelResponse struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ModelName     string `json:"modelName"`
	Tag           string `json:"tag"`
	State         string `json:"state"`
	Size          int64  `json:"size,omitempty"`
	FormattedSize string `json:"formattedSize,omitempty"`
	LastPullTime  string `json:"lastPullTime,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ModelListResponse represents the API response for listing models
type ModelListResponse struct {
	Items []ModelResponse `json:"items"`
}

// listModels handles the GET /api/v1/models endpoint
func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("api-listModels")

	// List all OllamaModel resources in the configured namespace
	var modelList ollamav1alpha1.OllamaModelList
	if err := s.client.List(ctx, &modelList, client.InNamespace(s.config.Namespace)); err != nil {
		logger.Error(err, "failed to list models")
		sendError(w, err, http.StatusInternalServerError)
		return
	}

	// Convert to API response
	response := ModelListResponse{
		Items: make([]ModelResponse, len(modelList.Items)),
	}

	for i, model := range modelList.Items {
		response.Items[i] = convertModelToResponse(model)
	}

	sendJSON(w, response, http.StatusOK)
}

// getModel handles the GET /api/v1/models/{name} endpoint
func (s *Server) getModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("api-getModel")
	vars := mux.Vars(r)
	name := vars["name"]

	// Get the model by name
	model := &ollamav1alpha1.OllamaModel{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.config.Namespace, Name: name}, model); err != nil {
		if apierrors.IsNotFound(err) {
			sendError(w, fmt.Errorf("model not found: %s", name), http.StatusNotFound)
		} else {
			logger.Error(err, "failed to get model", "name", name)
			sendError(w, err, http.StatusInternalServerError)
		}
		return
	}

	response := convertModelToResponse(*model)
	sendJSON(w, response, http.StatusOK)
}

// createModel handles the POST /api/v1/models endpoint
func (s *Server) createModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("api-createModel")

	// Parse request body
	var req ModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, fmt.Errorf("invalid request: %w", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Tag == "" {
		sendError(w, fmt.Errorf("name and tag are required"), http.StatusBadRequest)
		return
	}

	// Check if model already exists
	modelName := fmt.Sprintf("%s-%s", req.Name, req.Tag)
	existing := &ollamav1alpha1.OllamaModel{}
	err := s.client.Get(ctx, types.NamespacedName{Namespace: s.config.Namespace, Name: modelName}, existing)
	if err == nil {
		// Model already exists
		sendError(w, fmt.Errorf("model already exists: %s", modelName), http.StatusConflict)
		return
	} else if !apierrors.IsNotFound(err) {
		// Unexpected error
		logger.Error(err, "failed to check if model exists", "name", modelName)
		sendError(w, err, http.StatusInternalServerError)
		return
	}

	// Create new model
	model := &ollamav1alpha1.OllamaModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelName,
			Namespace: s.config.Namespace,
		},
		Spec: ollamav1alpha1.OllamaModelSpec{
			Name: req.Name,
			Tag:  req.Tag,
		},
	}

	if err := s.client.Create(ctx, model); err != nil {
		logger.Error(err, "failed to create model", "name", modelName)
		sendError(w, err, http.StatusInternalServerError)
		return
	}

	response := convertModelToResponse(*model)
	sendJSON(w, response, http.StatusCreated)
}

// deleteModel handles the DELETE /api/v1/models/{name} endpoint
func (s *Server) deleteModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("api-deleteModel")
	vars := mux.Vars(r)
	name := vars["name"]

	// Get the model to ensure it exists
	model := &ollamav1alpha1.OllamaModel{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.config.Namespace, Name: name}, model); err != nil {
		if apierrors.IsNotFound(err) {
			sendError(w, fmt.Errorf("model not found: %s", name), http.StatusNotFound)
		} else {
			logger.Error(err, "failed to get model", "name", name)
			sendError(w, err, http.StatusInternalServerError)
		}
		return
	}

	// Delete the model
	if err := s.client.Delete(ctx, model); err != nil {
		logger.Error(err, "failed to delete model", "name", name)
		sendError(w, err, http.StatusInternalServerError)
		return
	}

	// Return success with no content
	w.WriteHeader(http.StatusNoContent)
}

// refreshModel handles the POST /api/v1/models/{name}/refresh endpoint
func (s *Server) refreshModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithName("api-refreshModel")
	vars := mux.Vars(r)
	name := vars["name"]

	// Get the model
	model := &ollamav1alpha1.OllamaModel{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.config.Namespace, Name: name}, model); err != nil {
		if apierrors.IsNotFound(err) {
			sendError(w, fmt.Errorf("model not found: %s", name), http.StatusNotFound)
		} else {
			logger.Error(err, "failed to get model", "name", name)
			sendError(w, err, http.StatusInternalServerError)
		}
		return
	}

	// Add the refresh annotation
	if model.Annotations == nil {
		model.Annotations = make(map[string]string)
	}
	model.Annotations["ollama.smithforge.dev/refresh"] = "true"

	// Update the model
	if err := s.client.Update(ctx, model); err != nil {
		logger.Error(err, "failed to update model with refresh annotation", "name", name)
		sendError(w, err, http.StatusInternalServerError)
		return
	}

	response := convertModelToResponse(*model)
	sendJSON(w, response, http.StatusAccepted)
}

// convertModelToResponse converts an OllamaModel to a ModelResponse
func convertModelToResponse(model ollamav1alpha1.OllamaModel) ModelResponse {
	response := ModelResponse{
		Name:          model.Name,
		Namespace:     model.Namespace,
		ModelName:     model.Spec.Name,
		Tag:           model.Spec.Tag,
		State:         string(model.Status.State),
		Size:          model.Status.Size,
		FormattedSize: model.Status.FormattedSize,
		Error:         model.Status.Error,
	}

	if model.Status.LastPullTime != nil {
		response.LastPullTime = model.Status.LastPullTime.Format(time.RFC3339)
	}

	return response
}
