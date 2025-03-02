# frozen_string_literal: true

class HealthController < Sinatra::Base
  get '/health' do
    content_type :json
    { status: 'ok', message: 'Ollama K8s Manager is running' }.to_json
  end

  get '/ready' do
    content_type :json
    { status: 'ok', message: 'Ollama K8s Manager is ready to accept requests' }.to_json
  end
end
