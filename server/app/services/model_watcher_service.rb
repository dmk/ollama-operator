# frozen_string_literal: true

require 'pp'

class ModelWatcherService
  def initialize(ollama_client, logger)
    @ollama_client = ollama_client
    @logger = logger
  end

  def start
    Thread.new do
      begin
        @logger.info("Starting Kubernetes OllamaModel watcher")
        k8s_client = KubernetesClient.new(ENV['KUBERNETES_NAMESPACE'] || 'default', @logger)

        k8s_client.watch_ollama_models do |event|
          process_model_event(event)
        end
      rescue => e
        @logger.error("Error in Kubernetes watcher: #{e.message}")
        @logger.error(e.backtrace.join("\n"))
        sleep 10
        retry
      end
    end
  end

  private

  def process_model_event(event)
    model = Hashie::Mash.new(event.object)
    name = model.spec.modelName

    @logger.info("Processing model: #{name}, event type: #{event.type}")

    case event.type
    when 'ADDED', 'MODIFIED'
      handle_model_added_or_modified(model)
    when 'DELETED'
      handle_model_deleted(model, name)
    end
  end

  def handle_model_added_or_modified(model)
    model_name = model.spec.modelName
    @logger.info("Ensuring model exists: #{model_name}")

    begin
      existing_models = @ollama_client.list_models
      @logger.debug("Existing models: #{existing_models}")
      model_exists = existing_models["models"]&.any? { |m| m["name"] == model_name }

      unless model_exists
        @logger.info("Pulling model: #{model_name}")

        @ollama_client.pull_model(model_name) do |progress|
          # @logger.info("Pull progress: #{progress}")
          next
        end
      end
    rescue => e
      @logger.error("Error processing model #{model_name}: #{e.message}")
    end
  end

  def handle_model_deleted(model, name)
    @logger.info("Model deleted: #{name}")
    if model.spec.respond_to?(:modelName)
      model_name = model.spec.modelName
      @ollama_client.delete_model(model_name)
    end
  end
end