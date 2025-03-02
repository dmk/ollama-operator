require 'net/http'
require 'json'
require 'uri'

class OllamaClient
  attr_reader :base_url

  def initialize(base_url = 'http://localhost:11434', logger)
    @base_url = base_url
    @logger = logger
  end

  def list_models
    response = get('/api/tags')
    JSON.parse(response.body)
  end

  def show_model(model_name, verbose = false)
    data = { model: model_name }
    data[:verbose] = verbose if verbose

    response = post('/api/show', data)
    JSON.parse(response.body)
  end

  def delete_model(model_name)
    response = delete('/api/delete', { model: model_name })
    response.code == '200'
  end

  def pull_model(model_name, insecure: false, stream: true)
    @logger.info("Pulling model: #{model_name}")
    data = { model: model_name, insecure: insecure, stream: stream }

    if stream
      uri = URI.parse("#{@base_url}/api/pull")
      http = Net::HTTP.new(uri.host, uri.port)
      request = Net::HTTP::Post.new(uri.request_uri)
      request.body = data.to_json
      request['Content-Type'] = 'application/json'

      response_stream = []
      http.request(request) do |response|
        response.read_body do |chunk|
          chunk.split("\n").each do |line|
            next if line.empty?
            begin
              json_response = JSON.parse(line)
              response_stream << json_response
              yield json_response if block_given?
            rescue JSON::ParserError => e
              puts "Error parsing JSON: #{e.message}"
            end
          end
        end
      end
      response_stream
    else
      response = post('/api/pull', data)
      JSON.parse(response.body)
    end
  end

  def version
    response = get('/api/version')
    JSON.parse(response.body)
  end

  private

  def get(path)
    uri = URI.parse("#{@base_url}#{path}")
    http = Net::HTTP.new(uri.host, uri.port)
    request = Net::HTTP::Get.new(uri.request_uri)
    http.request(request)
  end

  def post(path, data)
    uri = URI.parse("#{@base_url}#{path}")
    http = Net::HTTP.new(uri.host, uri.port)
    request = Net::HTTP::Post.new(uri.request_uri)
    request.body = data.to_json
    request['Content-Type'] = 'application/json'
    http.request(request)
  end

  def delete(path, data)
    uri = URI.parse("#{@base_url}#{path}")
    http = Net::HTTP.new(uri.host, uri.port)
    request = Net::HTTP::Delete.new(uri.request_uri)
    request.body = data.to_json
    request['Content-Type'] = 'application/json'
    http.request(request)
  end
end