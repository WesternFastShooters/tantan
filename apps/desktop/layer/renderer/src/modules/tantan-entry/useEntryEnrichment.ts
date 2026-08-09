import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { ensureEntryEnrichment, getEntryEnrichment } from "./enrichment-api"

export const useEntryEnrichment = (entryId: string) => {
  const queryClient = useQueryClient()
  const queryKey = ["entry-enrichment", entryId, "zh-CN"] as const
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getEntryEnrichment(entryId, signal),
    enabled: Boolean(entryId),
    staleTime: 30_000,
    refetchInterval: ({ state }) => {
      const status = state.data?.state
      return status === "queued" || status === "processing" ? 2_000 : false
    },
  })
  const ensureMutation = useMutation({
    mutationFn: () => ensureEntryEnrichment(entryId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  })
  return {
    ...query,
    ensure: () => ensureMutation.mutate(),
    ensuring: ensureMutation.isPending,
    ensureError: ensureMutation.error,
  }
}
