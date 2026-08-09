import { tantanRequest } from "~/lib/tantan-api/client"
import type {
  AIProviderResponse,
  AIProviderTestResponse,
  SourceBlocksResponse,
  TopicPatchRequest,
  TopicsResponse,
} from "~/lib/tantan-api/gen/types"

const idempotencyKey = () =>
  globalThis.crypto?.randomUUID?.() ?? `tantan-${Date.now()}-${Math.random().toString(36).slice(2)}`

export const getAIProvider = (signal?: AbortSignal) =>
  tantanRequest<AIProviderResponse>("/api/tantan/v1/settings/ai-provider", { signal })

export const testAIProvider = () =>
  tantanRequest<AIProviderTestResponse>("/api/tantan/v1/settings/ai-provider/test", {
    method: "POST",
  })

export const patchTopics = (body: TopicPatchRequest) =>
  tantanRequest<TopicsResponse>("/api/tantan/v1/topics", {
    method: "PATCH",
    body: JSON.stringify(body),
  })

export const getBlockedSources = (signal?: AbortSignal) =>
  tantanRequest<SourceBlocksResponse>("/api/tantan/v1/recommendation/blocks/sources", { signal })

export const restoreBlockedSource = (sourceId: string) =>
  tantanRequest<{ applied: true }>(
    `/api/tantan/v1/recommendation/blocks/sources/${encodeURIComponent(sourceId)}`,
    {
      method: "DELETE",
      headers: { "Idempotency-Key": idempotencyKey() },
    },
  )
