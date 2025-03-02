require 'k8s-ruby'
require 'logger'

class KubernetesClient
  attr_reader :client, :logger

  def initialize(namespace = 'default', logger = Logger.new(STDOUT))
    @logger = logger
    @namespace = namespace
    initialize_client
  end

  def initialize_client
    # Auto-configure from within the cluster or use kubeconfig
    if File.exist?('/var/run/secrets/kubernetes.io/serviceaccount/token')
      # In-cluster configuration
      @logger.info("Using in-cluster Kubernetes configuration")
      @client = K8s::Client.in_cluster_config
    else
      # Local development with kubeconfig
      @logger.info("Using local Kubernetes configuration")
      kubeconfig_path = ENV['KUBECONFIG'] || File.join(Dir.home, '.kube', 'config')
      @client = K8s::Client.config(K8s::Config.load_file(kubeconfig_path))
    end
  end

  # Check if the CRD exists
  def crd_exists?
    begin
      @client.api('apiextensions.k8s.io/v1').resource('customresourcedefinitions').get('ollamamodels.ollama.io')
      @logger.info("OllamaModel CRD found in the cluster")
      true
    rescue K8s::Error::NotFound
      @logger.warn("OllamaModel CRD not found in the cluster")
      false
    rescue => e
      @logger.error("Error checking CRD: #{e.message}")
      false
    end
  end

  # Watch for changes to OllamaModel custom resources
  def watch_ollama_models(&block)
    begin
      unless crd_exists?
        @logger.error("OllamaModel CRD not found in the cluster. Please apply the CRD first.")
        exit 1
      end

      @logger.info("Starting to watch OllamaModel resources in namespace: #{@namespace}")

      # Get the API resource for OllamaModels
      api_resource =
        @client
          .api('ollama.io/v1alpha1')
          .resource('ollamamodels', namespace: @namespace)

      # Watch for changes
      api_resource.watch do |watch_event|
        resource = watch_event.resource
        @logger.info("Received event: #{watch_event.type} for model: #{resource.metadata.name}")
        block.call(watch_event) if block
      end
    rescue => e
      @logger.error("Error watching OllamaModel resources: #{e.message}")
      @logger.error(e.backtrace.join("\n"))
      sleep 5
      retry
    end
  end

  # Get all OllamaModel resources
  def get_ollama_models
    @client.api('ollama.io/v1alpha1')
           .resource('ollamamodels', namespace: @namespace)
           .list
  end

  # Get a specific OllamaModel resource
  def get_ollama_model(name)
    @client.api('ollama.io/v1alpha1')
           .resource('ollamamodels', namespace: @namespace)
           .get(name)
  end

  # Update the status of an OllamaModel resource
  def update_ollama_model_status(name, status)
    api_resource = @client.api('ollama.io/v1alpha1')
                          .resource('ollamamodels', namespace: @namespace)

    model = api_resource.get(name)

    # Create a status subresource update
    model.status = status

    # Use the status subresource if available, otherwise update the whole resource
    begin
      api_resource.subresource('status').update_resource(model)
    rescue K8s::Error::UnsupportedMediaType
      # Fall back to updating the whole resource if status subresource is not supported
      api_resource.update_resource(model)
    end
  end

  # Helper method to check if the Ollama operator is installed and running
  def check_operator_status
    begin
      deployments = @client.api('apps/v1')
                           .resource('deployments', namespace: @namespace)
                           .list(labelSelector: 'app=ollama-operator')

      if deployments.empty?
        @logger.warn("Ollama operator deployment not found")
        return false
      end

      deployment = deployments.first
      ready_replicas = deployment.status.readyReplicas || 0
      desired_replicas = deployment.spec.replicas

      if ready_replicas == desired_replicas
        @logger.info("Ollama operator is running (#{ready_replicas}/#{desired_replicas} replicas ready)")
        return true
      else
        @logger.warn("Ollama operator is not fully ready (#{ready_replicas}/#{desired_replicas} replicas ready)")
        return false
      end
    rescue => e
      @logger.error("Error checking operator status: #{e.message}")
      return false
    end
  end
end