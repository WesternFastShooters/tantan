#!/usr/bin/env ruby
# frozen_string_literal: true

require "digest"
require "json"
require "pathname"
require "time"
require "uri"

PACKAGE_ROOT = File.expand_path("..", __dir__)
REPO_ROOT = File.expand_path("..", PACKAGE_ROOT)
ERRORS = []

def fail_check(message)
  ERRORS << message
end

def load_json(path)
  JSON.parse(File.read(path, encoding: "UTF-8"))
rescue JSON::ParserError => e
  fail_check("invalid JSON #{path}: #{e.message}")
  {}
end

def json_pointer(document, pointer)
  return document if pointer.nil? || pointer.empty? || pointer == "#"

  value = document
  pointer.delete_prefix("#/").split("/").each do |part|
    key = part.gsub("~1", "/").gsub("~0", "~")
    value = value.fetch(key)
  end
  value
rescue KeyError, TypeError
  nil
end

def json_type?(value, expected)
  case expected
  when "object" then value.is_a?(Hash)
  when "array" then value.is_a?(Array)
  when "string" then value.is_a?(String)
  when "integer" then value.is_a?(Integer)
  when "number" then value.is_a?(Numeric)
  when "boolean" then value == true || value == false
  when "null" then value.nil?
  else false
  end
end

def validate_format(value, format)
  case format
  when "date-time"
    Time.iso8601(value)
    true
  when "uri"
    uri = URI.parse(value)
    !uri.scheme.nil?
  when "email"
    value.match?(/\A[^\s@]+@[^\s@]+\z/)
  else
    true
  end
rescue ArgumentError, URI::InvalidURIError
  false
end

def validate_instance(value, schema, root_schema, location = "$")
  return [] unless schema.is_a?(Hash)

  errors = []
  if schema["$ref"]
    reference = schema["$ref"]
    unless reference.start_with?("#")
      return ["#{location}: example validator only accepts internal refs, got #{reference}"]
    end
    target = json_pointer(root_schema, reference)
    return ["#{location}: unresolved ref #{reference}"] unless target

    return validate_instance(value, target, root_schema, location)
  end

  if schema["oneOf"]
    matches = schema["oneOf"].count { |branch| validate_instance(value, branch, root_schema, location).empty? }
    errors << "#{location}: expected exactly one oneOf branch, got #{matches}" unless matches == 1
    return errors
  end

  errors << "#{location}: const mismatch" if schema.key?("const") && value != schema["const"]
  errors << "#{location}: value is outside enum" if schema["enum"] && !schema["enum"].include?(value)

  if schema["type"]
    types = Array(schema["type"])
    unless types.any? { |expected| json_type?(value, expected) }
      return errors + ["#{location}: expected type #{types.join("|")}, got #{value.class}"]
    end
  end

  if value.is_a?(Hash)
    Array(schema["required"]).each do |key|
      errors << "#{location}: missing required property #{key}" unless value.key?(key)
    end
    properties = schema.fetch("properties", {})
    value.each do |key, child|
      if properties.key?(key)
        errors.concat(validate_instance(child, properties[key], root_schema, "#{location}.#{key}"))
      elsif schema["additionalProperties"] == false
        errors << "#{location}: unexpected property #{key}"
      elsif schema["additionalProperties"].is_a?(Hash)
        errors.concat(validate_instance(child, schema["additionalProperties"], root_schema, "#{location}.#{key}"))
      end
    end
  elsif value.is_a?(Array)
    errors << "#{location}: fewer than minItems" if schema["minItems"] && value.length < schema["minItems"]
    errors << "#{location}: more than maxItems" if schema["maxItems"] && value.length > schema["maxItems"]
    if schema["uniqueItems"] && value.map { |item| JSON.generate(item) }.uniq.length != value.length
      errors << "#{location}: array items are not unique"
    end
    value.each_with_index do |child, index|
      errors.concat(validate_instance(child, schema["items"], root_schema, "#{location}[#{index}]")) if schema["items"]
    end
  elsif value.is_a?(String)
    errors << "#{location}: shorter than minLength" if schema["minLength"] && value.length < schema["minLength"]
    errors << "#{location}: longer than maxLength" if schema["maxLength"] && value.length > schema["maxLength"]
    errors << "#{location}: pattern mismatch" if schema["pattern"] && !Regexp.new(schema["pattern"]).match?(value)
    errors << "#{location}: invalid #{schema["format"]}" if schema["format"] && !validate_format(value, schema["format"])
  elsif value.is_a?(Numeric)
    errors << "#{location}: below minimum" if schema["minimum"] && value < schema["minimum"]
    errors << "#{location}: above maximum" if schema["maximum"] && value > schema["maximum"]
  end

  errors
end

def walk_refs(value, document, base_dir, location = "$")
  case value
  when Hash
    if value["$ref"]
      ref = value["$ref"]
      if ref.start_with?("#")
        fail_check("#{location}: unresolved internal ref #{ref}") unless json_pointer(document, ref)
      else
        file_ref, fragment = ref.split("#", 2)
        target_path = File.expand_path(file_ref, base_dir)
        unless Pathname.new(target_path).to_s.start_with?(Pathname.new(PACKAGE_ROOT).to_s + File::SEPARATOR)
          fail_check("#{location}: external ref escapes package: #{ref}")
          return
        end
        unless File.file?(target_path)
          fail_check("#{location}: missing external ref file #{ref}")
          return
        end
        target = load_json(target_path)
        fail_check("#{location}: unresolved external ref fragment #{ref}") if fragment && !json_pointer(target, "##{fragment}")
      end
    end
    value.each { |key, child| walk_refs(child, document, base_dir, "#{location}.#{key}") }
  when Array
    value.each_with_index { |child, index| walk_refs(child, document, base_dir, "#{location}[#{index}]") }
  end
end

def validate_manifest
  path = File.join(PACKAGE_ROOT, "manifest.json")
  manifest = load_json(path)
  fail_check("manifest status must be approved") unless manifest["status"] == "approved"
  fail_check("manifest packageVersion must be 2.0.0") unless manifest["packageVersion"] == "2.0.0"
  fail_check("manifest phase clients mismatch") unless manifest.dig("phase", "clients") == ["mobile-web-pwa"]
  fail_check("native mobile must be excluded") unless manifest.dig("phase", "excluded").to_a.include?("ios-android-native")
  fail_check("PC Web must be excluded") unless manifest.dig("phase", "excluded").to_a.include?("pc-web")
  fail_check("Electron must be excluded") unless manifest.dig("phase", "excluded").to_a.include?("electron")

  entries = manifest.fetch("files", [])
  listed = entries.map { |entry| entry["path"] }
  actual = Dir.glob(File.join(PACKAGE_ROOT, "**", "*"))
              .select { |file| File.file?(file) }
              .map { |file| Pathname.new(file).relative_path_from(Pathname.new(PACKAGE_ROOT)).to_s }
              .reject { |file| file == "manifest.json" }
              .sort
  fail_check("manifest file list differs; listed=#{listed.sort.inspect}, actual=#{actual.inspect}") unless listed.sort == actual
  fail_check("manifest contains duplicate file paths") unless listed.uniq.length == listed.length

  entries.each do |entry|
    file = File.join(PACKAGE_ROOT, entry.fetch("path", ""))
    next unless File.file?(file)

    actual_hash = Digest::SHA256.file(file).hexdigest
    fail_check("hash mismatch for #{entry["path"]}") unless actual_hash == entry["sha256"]
  end

  manifest.fetch("inputs", []).each do |entry|
    file = File.expand_path(entry.fetch("path"), PACKAGE_ROOT)
    unless File.file?(file)
      fail_check("missing input #{entry["path"]}")
      next
    end
    fail_check("input hash mismatch for #{entry["path"]}") unless Digest::SHA256.file(file).hexdigest == entry["sha256"]
  end
end

def validate_json_schemas
  mappings = {
    "home-response.schema.json" => "home-response.json",
    "filter-spec-v1.schema.json" => "filter-spec-v1.json",
    "ai-enrichment-v1.schema.json" => "ai-enrichment-v1.json",
    "topic-classification-v1.schema.json" => "topic-classification-v1.json"
  }
  mappings.each do |schema_name, example_name|
    schema = load_json(File.join(PACKAGE_ROOT, "schemas", schema_name))
    example = load_json(File.join(PACKAGE_ROOT, "examples", example_name))
    fail_check("#{schema_name}: missing draft 2020-12 marker") unless schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
    fail_check("#{schema_name}: additionalProperties must be false") unless schema["additionalProperties"] == false
    validate_instance(example, schema, schema).each { |error| fail_check("#{example_name}: #{error}") }
  end
end

def validate_openapi
  path = File.join(PACKAGE_ROOT, "api", "openapi.json")
  api = load_json(path)
  fail_check("OpenAPI version must be 3.1.0") unless api["openapi"] == "3.1.0"
  fail_check("OpenAPI server must be same-origin") unless api["servers"] == [{ "url" => "/", "description" => "Current Tantan origin" }]
  fail_check("OpenAPI global LocalSession security missing") unless api["security"] == [{ "LocalSession" => [] }]

  expected_paths = %w[
    /api/healthz /api/readyz /api/auth/folo/providers /api/auth/folo/social-start
    /api/auth/folo/email /api/auth/folo/token /api/auth/folo/two-factor /api/auth/logout
    /api/tantan/v1/session /api/tantan/v1/home /api/tantan/v1/topics /api/tantan/v1/filter
    /api/tantan/v1/recommendation/feedback /api/tantan/v1/recommendation/blocks/sources
    /api/tantan/v1/recommendation/blocks/sources/{sourceId} /api/tantan/v1/search
    /api/tantan/v1/entries/{entryId}/enrichment /api/tantan/v1/settings/ai-provider
    /api/tantan/v1/settings/ai-provider/test /api/tantan/v1/sync/status /api/tantan/v1/sync
    /api/tantan/v1/diagnostics
  ].sort
  fail_check("OpenAPI public path set differs") unless api.fetch("paths", {}).keys.sort == expected_paths

  operation_ids = []
  http_methods = %w[get post put patch delete options head]
  api.fetch("paths", {}).each do |route, path_item|
    path_item.each do |method, operation|
      next unless http_methods.include?(method)
      op_id = operation["operationId"]
      fail_check("#{method.upcase} #{route}: missing operationId") unless op_id.is_a?(String) && !op_id.empty?
      operation_ids << op_id if op_id
      fail_check("#{method.upcase} #{route}: responses missing") unless operation["responses"].is_a?(Hash) && !operation["responses"].empty?

      if route.start_with?("/api/tantan/v1/") && %w[post put patch delete].include?(method)
        refs = Array(operation["parameters"]).map { |param| param["$ref"] if param.is_a?(Hash) }.compact
        fail_check("#{method.upcase} #{route}: mutation missing Origin") unless refs.include?("#/components/parameters/Origin")
        fail_check("#{method.upcase} #{route}: mutation missing CSRF token") unless refs.include?("#/components/parameters/CsrfToken")
      end
    end
  end
  fail_check("OpenAPI operationId values are not unique") unless operation_ids.uniq.length == operation_ids.length

  idempotent_ops = [
    ["/api/tantan/v1/filter", "put"],
    ["/api/tantan/v1/recommendation/feedback", "post"],
    ["/api/tantan/v1/recommendation/blocks/sources/{sourceId}", "delete"],
    ["/api/tantan/v1/entries/{entryId}/enrichment", "post"]
  ]
  idempotent_ops.each do |route, method|
    refs = api.dig("paths", route, method, "parameters").to_a.map { |param| param["$ref"] }.compact
    fail_check("#{method.upcase} #{route}: missing Idempotency-Key") unless refs.include?("#/components/parameters/IdempotencyKey")
  end

  provider_enum = api.dig("components", "schemas", "FoloAuthProvider", "enum")
  fail_check("Folo auth methods must match Google/GitHub/Apple/Email/token") unless provider_enum == %w[google github apple credential token]
  social_pattern = api.dig("components", "schemas", "FoloSocialStartResponse", "properties", "authorizeUrl", "pattern").to_s
  fail_check("social login URL must be fixed to app.folo.is") unless social_pattern.include?("app\\.folo\\.is/login")
  fail_check("AI provider must be the locked Gemini preset") unless api.dig("components", "schemas", "AIProviderId", "enum") == ["google-gemini-openai"]
  provider_path = api.dig("paths", "/api/tantan/v1/settings/ai-provider") || {}
  fail_check("AI provider settings must be read-only") unless provider_path.keys == ["get"]
  test_operation = api.dig("paths", "/api/tantan/v1/settings/ai-provider/test", "post") || {}
  fail_check("AI provider test must not accept a browser request body") if test_operation.key?("requestBody")
  provider_response = api.dig("components", "schemas", "AIProviderResponse", "properties") || {}
  model_branches = provider_response.dig("model", "oneOf").to_a
  fail_check("AI model must be gemini-3.5-flash-lite") unless model_branches.any? { |branch| branch["const"] == "gemini-3.5-flash-lite" }
  base_branches = provider_response.dig("baseUrl", "oneOf").to_a
  fail_check("AI endpoint must be the locked Gemini endpoint") unless base_branches.any? { |branch| branch["const"] == "https://generativelanguage.googleapis.com/v1beta/openai" }
  fail_check("browser-facing schemas must not contain an API key field") if api.fetch("components", {}).fetch("schemas", {}).values.any? do |schema|
    schema.is_a?(Hash) && schema.fetch("properties", {}).keys.any? { |key| key.casecmp("apiKey").zero? }
  end
  walk_refs(api, api, File.dirname(path))
end

def validate_route_policy
  policy = load_json(File.join(PACKAGE_ROOT, "api", "folo-route-policy.json"))
  fail_check("route policy must default deny") unless policy["defaultAction"] == "deny"
  fail_check("route policy SDK version mismatch") unless policy["sdkVersion"] == "0.3.95"
  enabled = policy.fetch("enabled", [])
  ids = enabled.map { |route| route["id"] }
  fail_check("enabled route ids are not unique") unless ids.uniq.length == ids.length
  pairs = []
  enabled.each do |route|
    pattern = route["pathPattern"].to_s
    fail_check("route #{route["id"]} is not fully anchored") unless pattern.start_with?("^") && pattern.end_with?("$")
    begin
      Regexp.new(pattern)
    rescue RegexpError => e
      fail_check("route #{route["id"]} has invalid regex: #{e.message}")
    end
    Array(route["methods"]).each do |method|
      fail_check("route #{route["id"]} method is not uppercase") unless method == method.upcase
      pairs << [method, pattern]
    end
    mutation_methods = route["mutation"] == true ? Array(route["methods"]) : Array(route["mutationMethods"])
    Array(route["methods"]).grep(/\A(?:POST|PUT|PATCH|DELETE)\z/).each do |method|
      fail_check("route #{route["id"]} does not mark #{method} as mutation") unless mutation_methods.include?(method)
    end
  end
  fail_check("enabled method/path pairs are duplicated") unless pairs.uniq.length == pairs.length

  disabled_ids = policy.fetch("disabledByDefault", []).map { |route| route["id"] }
  required_disabled = %w[discover-rsshub-analytics entries-transcription feeds-reset feeds-analytics feeds-claim-challenge feeds-claim-list feeds-claim-message inboxes-email inboxes-webhook]
  fail_check("disabled-by-default route set is incomplete") unless (required_disabled - disabled_ids).empty?

  fail_check("Folo settings must not be publicly proxied") if enabled.any? { |route| route["id"].to_s.start_with?("settings") }
  internal_auth_ids = policy.fetch("internalAuthRoutes", []).map { |route| route["id"] }
  %w[auth-sign-in-email auth-token-apply auth-token-verify auth-two-factor-verify-totp auth-session auth-sign-out].each do |id|
    fail_check("internal auth route missing #{id}") unless internal_auth_ids.include?(id)
  end

  removed = policy.fetch("removed", [])
  representative = %w[/ai/config /wallets/balance /payments/create /better-auth/subscription/upgrade /better-auth/stripe/webhook /referrals/list /trending/topics /rsshub/use]
  representative.each do |path|
    matches = removed.select do |rule|
      Array(rule["methods"] || ["GET", "POST"]).any? && Regexp.new(rule["pathPattern"]).match?(path)
    end
    fail_check("removed policy misses #{path}") if matches.empty?
    enabled_match = enabled.any? { |rule| Regexp.new(rule["pathPattern"]).match?(path) }
    fail_check("removed path is also enabled: #{path}") if enabled_match
  end
end

def validate_task_manifest
  manifest = load_json(File.join(PACKAGE_ROOT, "agent", "task-manifest.json"))
  policy = manifest.fetch("executionPolicy", {})
  %w[oneActiveTask dependencyOrderRequired contractChangeRequiresSpecAmendment redEvidenceRequired verifyEvidenceRequired].each do |key|
    fail_check("task execution policy #{key} must be true") unless policy[key] == true
  end
  fail_check("task default write action must be deny") unless policy["defaultWriteAction"] == "deny"

  tasks = manifest.fetch("tasks", [])
  ids = tasks.map { |task| task["id"] }
  expected = %w[TASK-01 TASK-02 TASK-03 TASK-04 TASK-05 TASK-06 TASK-07 TASK-08]
  fail_check("task set differs") unless ids.sort == expected.sort
  fail_check("task ids are not unique") unless ids.uniq.length == ids.length

  task_by_id = tasks.to_h { |task| [task["id"], task] }
  tasks.each do |task|
    fail_check("#{task["id"]}: allowedWrites empty") if Array(task["allowedWrites"]).empty?
    fail_check("#{task["id"]}: forbiddenWrites empty") if Array(task["forbiddenWrites"]).empty?
    fail_check("#{task["id"]}: requiredGates empty") if Array(task["requiredGates"]).empty?
    Array(task["dependsOn"]).each { |dep| fail_check("#{task["id"]}: unknown dependency #{dep}") unless task_by_id.key?(dep) }
  end

  visiting = {}
  visited = {}
  visit = lambda do |id|
    if visiting[id]
      fail_check("task dependency cycle at #{id}")
      return
    end
    return if visited[id]
    visiting[id] = true
    Array(task_by_id.dig(id, "dependsOn")).each { |dep| visit.call(dep) }
    visiting.delete(id)
    visited[id] = true
  end
  ids.each { |id| visit.call(id) }

  global_forbidden = manifest.fetch("globalForbiddenWrites", [])
  fail_check("Folo source must be globally read-only") unless global_forbidden.include?("/Users/mingrui/Project/Folo/**")
  fail_check("native app must be globally read-only") unless global_forbidden.include?("apps/mobile/**")
end

def validate_acceptance_and_specs
  matrix = File.read(File.join(PACKAGE_ROOT, "tests", "acceptance-matrix.md"), encoding: "UTF-8")
  fe_ids = matrix.scan(/\| FE:TC-(\d{3}) \|/).flatten
  be_ids = matrix.scan(/\| BE:TC-(\d{3}) \|/).flatten
  expected = (1..23).map { |number| format("%03d", number) }
  fail_check("frontend acceptance IDs differ") unless fe_ids == expected
  fail_check("backend acceptance IDs differ") unless be_ids == expected

  scanned = Dir.glob(File.join(PACKAGE_ROOT, "**", "*")).select do |path|
    File.file?(path) && !path.end_with?("manifest.json") && !path.end_with?("scripts/validate-package.rb")
  end
  placeholder = /(?:待确认|待定|\bTODO\b|\bTBD\b|等待用户决定|状态：草案)/
  scanned.each do |path|
    text = File.read(path, encoding: "UTF-8")
    fail_check("unresolved placeholder in #{path}") if text.match?(placeholder)
  end
end

validate_manifest
validate_json_schemas
validate_openapi
validate_route_policy
validate_task_manifest
validate_acceptance_and_specs

if ERRORS.empty?
  puts "PASS: spec package JSON/contracts/tasks/acceptance checks"
  exit 0
end

ERRORS.each { |error| warn "ERROR: #{error}" }
warn "FAIL: #{ERRORS.length} package validation error(s)"
exit 1
