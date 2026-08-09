import type { AIProviderId, AIProviderPutRequest } from "~/lib/tantan-api/gen/types"

export const PROVIDER_PRESETS: Record<AIProviderId, { label: string; baseUrl: string }> = {
  openai: { label: "OpenAI", baseUrl: "https://api.openai.com/v1" },
  anthropic: { label: "Anthropic", baseUrl: "https://api.anthropic.com" },
  google: { label: "Google", baseUrl: "https://generativelanguage.googleapis.com" },
  deepseek: { label: "DeepSeek", baseUrl: "https://api.deepseek.com" },
  openrouter: { label: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1" },
}

export interface ProviderDraft {
  providerId: AIProviderId
  model: string
  apiKey: string
}

export const validateProviderDraft = (draft: ProviderDraft, hasSavedKey: boolean) => {
  if (!draft.model.trim()) return "请输入模型名称"
  if (draft.model.trim().length > 100) return "模型名称不能超过 100 个字符"
  if (!hasSavedKey && draft.apiKey.length < 8) return "API Key 至少需要 8 个字符"
  if (draft.apiKey && draft.apiKey.length < 8) return "API Key 至少需要 8 个字符"
  if (draft.apiKey.length > 4096) return "API Key 不能超过 4096 个字符"
  return null
}

export const buildProviderSaveRequest = (draft: ProviderDraft): AIProviderPutRequest => {
  const request: AIProviderPutRequest = {
    providerId: draft.providerId,
    model: draft.model.trim(),
  }
  if (draft.apiKey) request.apiKey = draft.apiKey
  return request
}
