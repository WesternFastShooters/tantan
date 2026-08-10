import { tantanRequest } from "~/lib/tantan-api/client"
import type { EnrichmentResponse, JobAcceptedResponse } from "~/lib/tantan-api/gen/types"

const idempotencyKey = () =>
  globalThis.crypto?.randomUUID?.() ??
  `entry-ai-${Date.now()}-${Math.random().toString(36).slice(2)}`

export const getEntryEnrichment = (entryId: string, signal?: AbortSignal) =>
  tantanRequest<EnrichmentResponse>(
    `/api/tantan/v1/entries/${encodeURIComponent(entryId)}/enrichment?language=zh-CN`,
    { signal },
  )

export const ensureEntryEnrichment = (entryId: string) =>
  tantanRequest<JobAcceptedResponse>(
    `/api/tantan/v1/entries/${encodeURIComponent(entryId)}/enrichment`,
    {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey() },
      body: JSON.stringify({
        fields: ["translation", "summary", "keyPoints"],
        language: "zh-CN",
      }),
    },
  )
