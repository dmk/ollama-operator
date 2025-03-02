# frozen_string_literal: true

module JsonHelpers
  def parse_json_body
    request_body = request.body.read
    return Hashie::Mash.new if request_body.empty?

    json = JSON.parse(request_body)
    Hashie::Mash.new(json)
  end

  def safe_parse_json(json_string)
    return Hashie::Mash.new if json_string.nil? || json_string.empty?

    begin
      data = JSON.parse(json_string)
      Hashie::Mash.new(data)
    rescue JSON::ParserError => e
      logger.error("JSON parsing error: #{e.message}")
      halt 400, json(error: "Invalid JSON format: #{e.message}")
    end
  end
end
