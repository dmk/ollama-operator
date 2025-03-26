# Ollama Operator API Usage Examples

This document provides examples of how to use the Ollama Operator's HTTP API to manage Ollama models.

## Prerequisites

- Ollama Operator installed in your Kubernetes cluster
- API server enabled with the `--enable-api-server` flag
- API key configured (if using authentication)

## API Endpoints

The API provides the following endpoints:

- `GET /api/v1/models` - List all models
- `GET /api/v1/models/{name}` - Get details of a specific model
- `POST /api/v1/models` - Create a new model
- `DELETE /api/v1/models/{name}` - Delete a model
- `POST /api/v1/models/{name}/refresh` - Refresh a model

## Authentication

If API key authentication is enabled, include the API key in the `X-API-Key` header:

```bash
curl -H "X-API-Key: your-api-key" http://localhost:8082/api/v1/models
```

## Examples

### List all models

```bash
curl -s -H "X-API-Key: your-api-key" http://localhost:8082/api/v1/models | jq
```

Example response:

```json
{
  "items": [
    {
      "name": "llama3.2-1b",
      "namespace": "default",
      "modelName": "llama3.2",
      "tag": "1b",
      "state": "Ready",
      "size": 1815319791,
      "formattedSize": "1.7 GiB",
      "lastPullTime": "2025-03-25T12:00:00Z"
    },
    {
      "name": "gemma3-1b",
      "namespace": "default",
      "modelName": "gemma3",
      "tag": "1b",
      "state": "Ready",
      "size": 815319791,
      "formattedSize": "777.5 MiB",
      "lastPullTime": "2025-03-25T19:04:53Z"
    }
  ]
}
```

### Get a specific model

```bash
curl -s -H "X-API-Key: your-api-key" http://localhost:8082/api/v1/models/gemma3-1b | jq
```

Example response:

```json
{
  "name": "gemma3-1b",
  "namespace": "default",
  "modelName": "gemma3",
  "tag": "1b",
  "state": "Ready",
  "size": 815319791,
  "formattedSize": "777.5 MiB",
  "lastPullTime": "2025-03-25T19:04:53Z"
}
```

### Create a new model

```bash
curl -s -X POST -H "Content-Type: application/json" -H "X-API-Key: your-api-key" \
  -d '{"name": "phi3", "tag": "mini"}' \
  http://localhost:8082/api/v1/models | jq
```

Example response:

```json
{
  "name": "phi3-mini",
  "namespace": "default",
  "modelName": "phi3",
  "tag": "mini",
  "state": "Pending"
}
```

### Delete a model

```bash
curl -s -X DELETE -H "X-API-Key: your-api-key" http://localhost:8082/api/v1/models/phi3-mini
```

This returns no content with a 204 status code if successful.

### Refresh a model

```bash
curl -s -X POST -H "X-API-Key: your-api-key" http://localhost:8082/api/v1/models/gemma3-1b/refresh | jq
```

Example response:

```json
{
  "name": "gemma3-1b",
  "namespace": "default",
  "modelName": "gemma3",
  "tag": "1b",
  "state": "Ready",
  "size": 815319791,
  "formattedSize": "777.5 MiB",
  "lastPullTime": "2025-03-25T19:04:53Z"
}
```

## Integration with Rails Applications

For Ruby on Rails applications, you can create a simple client to interact with the API:

```ruby
# app/services/ollama_api_client.rb
require 'net/http'
require 'json'

class OllamaApiClient
  def initialize(base_url: 'http://localhost:8082', api_key: nil)
    @base_url = base_url
    @api_key = api_key
  end

  def list_models
    request(:get, '/api/v1/models')
  end

  def get_model(name)
    request(:get, "/api/v1/models/#{name}")
  end

  def create_model(name, tag)
    request(:post, '/api/v1/models', { name: name, tag: tag })
  end

  def delete_model(name)
    request(:delete, "/api/v1/models/#{name}")
  end

  def refresh_model(name)
    request(:post, "/api/v1/models/#{name}/refresh")
  end

  private

  def request(method, path, payload = nil)
    uri = URI.parse("#{@base_url}#{path}")

    http = Net::HTTP.new(uri.host, uri.port)

    request = case method
    when :get
      Net::HTTP::Get.new(uri.request_uri)
    when :post
      req = Net::HTTP::Post.new(uri.request_uri)
      req.body = payload.to_json if payload
      req
    when :delete
      Net::HTTP::Delete.new(uri.request_uri)
    end

    request['Content-Type'] = 'application/json' if payload
    request['X-API-Key'] = @api_key if @api_key

    response = http.request(request)

    return nil if response.code.to_i == 204

    if response.body && !response.body.empty?
      JSON.parse(response.body)
    end
  end
end
```

Usage in a Rails application:

```ruby
# Initialize the client
client = OllamaApiClient.new(api_key: 'your-api-key')

# List all models
models = client.list_models
puts "Available models: #{models['items'].map { |m| m['name'] }.join(', ')}"

# Create a new model
client.create_model('phi3', 'mini')

# Get model details
model = client.get_model('phi3-mini')
puts "Model state: #{model['state']}"

# Refresh a model
client.refresh_model('phi3-mini')

# Delete a model
client.delete_model('phi3-mini')
```