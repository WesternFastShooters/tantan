import { tantanRequest } from "~/lib/tantan-api/client"
import type {
  AIProviderPutRequest,
  AIProviderResponse,
  AIProviderTestRequest,
  AIProviderTestResponse,
  SourceBlocksResponse,
  TopicPatchRequest,
  TopicsResponse,
} from "~/lib/tantan-api/gen/types"

const idempotencyKey = () =>
  globalThis.crypto?.randomUUID?.() ?? `tantan-${Date.now()}-${Math.random().toString(36).slice(2)}`

export const getAIProvider = (signal?: AbortSignal) =>
  tantanRequest<AIProviderResponse>("/tantan/v1/settings/ai-provider", { signal })

export const saveAIProvider = (body: AIProviderPutRequest) =>
  tantanRequest<AIProviderResponse>("/tantan/v1/settings/ai-provider", {
    method: "PUT",
    body: JSON.stringify(body),
  })

export const testAIProvider = (body: AIProviderTestRequest) =>
  tantanRequest<AIProviderTestResponse>("/tantan/v1/settings/ai-provider/test", {
    method: "POST",
    body: JSON.stringify(body),
  })

export const deleteAIProvider = () =>
  tantanRequest<void>("/tantan/v1/settings/ai-provider", { method: "DELETE" })

export const patchTopics = (body: TopicPatchRequest) =>
  tantanRequest<TopicsResponse>("/tantan/v1/topics", {
    method: "PATCH",
    body: JSON.stringify(body),
  })

export const getBlockedSources = (signal?: AbortSignal) =>
  tantanRequest<SourceBlocksResponse>("/tantan/v1/recommendation/blocks/sources", { signal })

export const restoreBlockedSource = (sourceId: string) =>
  tantanRequest<{ applied: true }>(
    `/tantan/v1/recommendation/blocks/sources/${encodeURIComponent(sourceId)}`,
    {
      method: "DELETE",
      headers: { "Idempotency-Key": idempotencyKey() },
    },
  )
