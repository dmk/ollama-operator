# frozen_string_literal: true

require 'dotenv'
Dotenv.load

ENV['RACK_ENV'] ||= 'development'

if ENV['RACK_ENV'] == 'production'
  $stdout.sync = true
else
  require 'pry' if ENV['RACK_ENV'] == 'development'
end
