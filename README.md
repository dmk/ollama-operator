# Ollama Operator

A Kubernetes operator that manages Ollama models declaratively through custom resources.

## Examples

Example custom resources can be found in the [manifests](./manifests) directory.

## Current Status

This operator has only been tested on a local k3d cluster. The included Ollama deployment does not configure GPU access - users should review and modify the deployment configuration for production use cases, especially when GPU acceleration is required.

## License

Apache License 2.0
