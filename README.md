# ollama-operator

A Kubernetes operator for declarative management of Ollama models through Kubernetes custom resources.

## Description

The Ollama Operator provides a Kubernetes-native way to manage [Ollama](https://ollama.com/) models in your cluster. It allows you to:

- Declaratively specify which Ollama models should be available
- Automatically pull models when they are added as resources
- Track the state of models (pending, pulling, ready, failed)
- Automatically remove models when the resources are deleted

This operator makes it easy to integrate Ollama's large language models into your Kubernetes infrastructure using GitOps principles and standard Kubernetes tooling.

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- [Ollama](https://ollama.com/) running in your cluster or accessible to the operator

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/ollama-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/ollama-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

### Create Ollama Model Resources

Once the operator is running, you can create OllamaModel custom resources to manage models. For example:

```yaml
apiVersion: ollama.smithforge.dev/v1alpha1
kind: OllamaModel
metadata:
  name: llama3.2-1b
spec:
  name: llama3.2  # Model name (as recognized by Ollama)
  tag: 1b         # Model tag/version
```

The operator will ensure that the specified model is pulled and ready in your Ollama instance. You can check the status using:

```sh
kubectl get ollamamodels
```

Sample resources can be found in the `config/samples/` directory.

You can apply the samples with:

```sh
kubectl apply -k config/samples/
```

### Current Status and Limitations

The operator has been tested with a standard Kubernetes setup. Note that:

- The operator does not deploy Ollama itself - it manages models for an existing Ollama installation
- GPU acceleration requires proper configuration of your Ollama deployment
- For production use cases, you may need to customize resource limits

## Technical Details

### Custom Resource Definition

The operator introduces a new Custom Resource Definition (CRD) called `OllamaModel` with the following specification:

```yaml
spec:
  name: <model-name>   # Name of the Ollama model (e.g., llama3.2, gemma3)
  tag: <model-tag>     # Version/tag of the model (e.g., 7b, 1b)
```

The resource reports the following status fields:

```yaml
status:
  state: <pending|pulling|ready|failed>  # Current state of the model
  lastPullTime: <timestamp>              # When the model was last pulled
  digest: <sha256>                       # Model file SHA256 digest
  size: <bytes>                          # Size of the model in bytes
  error: <message>                       # Error message if in failed state
```

### Architecture

The operator connects to the Ollama API to:
1. Check if requested models exist
2. Pull models that don't exist
3. Delete models when resources are removed
4. Update status information about each model

## Uninstalling

**Delete all model instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

**Undeploy the controller from the cluster:**

```sh
make undeploy
```

## License

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

