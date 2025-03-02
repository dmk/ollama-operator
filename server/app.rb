# frozen_string_literal: true

require 'sinatra/base'
require 'sinatra/json'
require 'json'
require 'hashie'
require 'logger'

require_relative 'config/environment'

require_relative 'app/helpers/json_helpers'

require_relative 'lib/clients/ollama_client'
require_relative 'lib/clients/kubernetes_client'

require_relative 'app/services/model_watcher_service'

require_relative 'app/controllers/health_controller'

class OllamaManagerApp < Sinatra::Base
  helpers Sinatra::JSON
  helpers JsonHelpers

  configure do
    set :logger, Logger.new(STDOUT)

    set :ollama_client, OllamaClient.new(
      ENV['OLLAMA_API_URL'] || 'http://localhost:11434',
      settings.logger
    )

    watcher = ModelWatcherService.new(settings.ollama_client, settings.logger)
    watcher.start
  end

  use HealthController
end
