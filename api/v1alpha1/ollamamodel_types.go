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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OllamaModelSpec defines the desired state of OllamaModel.
type OllamaModelSpec struct {
	// Name is the name of the Ollama model (e.g., "llama3.2", "gemma3")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Tag is the version/tag of the model (e.g., "7b", "1b")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Tag string `json:"tag"`
}

// OllamaModelStatus defines the observed state of OllamaModel.
// +kubebuilder:default=pending
type OllamaModelStatus struct {
	// State represents the current state of the model (pending, pulling, ready, failed)
	// +kubebuilder:validation:Enum=pending;pulling;ready;failed
	State string `json:"state,omitempty"`

	// LastPullTime is the timestamp of the last successful model pull
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastPullTime *metav1.Time `json:"lastPullTime,omitempty"`

	// Digest is the SHA256 digest of the model file
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{64}$`
	Digest string `json:"digest,omitempty"`

	// Size is the size of the model in bytes
	// +kubebuilder:validation:Minimum=0
	Size int64 `json:"size,omitempty"`

	// Error message if the model is in failed state
	// +kubebuilder:validation:MaxLength=1024
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Tag",type="string",JSONPath=".spec.tag"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Size",type="integer",JSONPath=".status.size"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OllamaModel is the Schema for the ollamamodels API.
type OllamaModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OllamaModelSpec   `json:"spec,omitempty"`
	Status OllamaModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OllamaModelList contains a list of OllamaModel.
type OllamaModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OllamaModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OllamaModel{}, &OllamaModelList{})
}
