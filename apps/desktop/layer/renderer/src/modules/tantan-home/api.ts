import { tantanRequest } from "~/lib/tantan-api/client"
import type {
  FeedbackRequest,
  FilterMutationResponse,
  HomeResponse,
  TopicsResponse,
} from "~/lib/tantan-api/gen/types"

const idempotencyKey = () =>
  globalThis.crypto?.randomUUID?.() ?? `tantan-${Date.now()}-${Math.random().toString(36).slice(2)}`

export const getHome = ({
  topicId,
  filterId,
  cursor,
  limit = 20,
  signal,
}: {
  topicId: string
  filterId: string | null
  cursor: string | null
  limit?: number
  signal?: AbortSignal
}) => {
  const search = new URLSearchParams({ topicId, limit: String(limit) })
  if (filterId) search.set("filterId", filterId)
  if (cursor) search.set("cursor", cursor)
  return tantanRequest<HomeResponse>(`/tantan/v1/home?${search.toString()}`, { signal })
}

export const getTopics = (signal?: AbortSignal) =>
  tantanRequest<TopicsResponse>("/tantan/v1/topics", { signal })

export const putActiveFilter = (prompt: string) =>
  tantanRequest<FilterMutationResponse>("/tantan/v1/filter", {
    method: "PUT",
    headers: { "Idempotency-Key": idempotencyKey() },
    body: JSON.stringify({ prompt }),
  })

export const deleteActiveFilter = () =>
  tantanRequest<FilterMutationResponse>("/tantan/v1/filter", { method: "DELETE" })

export const postRecommendationFeedback = (body: FeedbackRequest) =>
  tantanRequest<{ applied: true }>("/tantan/v1/recommendation/feedback", {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey() },
    body: JSON.stringify(body),
  })
