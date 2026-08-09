import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { getBlockedSources, restoreBlockedSource } from "./api"

const queryKey = ["settings", "blocked-sources"] as const

export function BlockedSources() {
  const queryClient = useQueryClient()
  const blocksQuery = useQuery({
    queryKey,
    queryFn: ({ signal }) => getBlockedSources(signal),
  })
  const restoreMutation = useMutation({
    mutationFn: restoreBlockedSource,
    onSuccess: (_result, sourceId) => {
      queryClient.setQueryData(queryKey, (current: typeof blocksQuery.data) =>
        current
          ? { ...current, items: current.items.filter((item) => item.sourceId !== sourceId) }
          : current,
      )
    },
  })

  return (
    <section
      className="mt-8 border-t border-white/[0.06] pt-6"
      aria-labelledby="blocked-sources-title"
    >
      <h2 id="blocked-sources-title" className="text-base font-semibold text-zinc-100">
        已屏蔽 Source
      </h2>
      <p className="mt-1 text-sm text-zinc-500">恢复后，该 Source 可进入下一次生成的推荐队列。</p>
      {blocksQuery.isPending && (
        <div aria-busy="true" className="mt-3 h-16 animate-pulse rounded-xl bg-[#17181b]" />
      )}
      {blocksQuery.isError && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {blocksQuery.error.message}
        </p>
      )}
      {blocksQuery.data?.items.length === 0 && (
        <p className="mt-3 rounded-xl bg-[#17181b] p-4 text-sm text-zinc-500">
          暂无已屏蔽 Source。
        </p>
      )}
      <div className="mt-3 space-y-2">
        {blocksQuery.data?.items.map((item) => (
          <article
            key={item.sourceId}
            className="flex min-h-16 items-center gap-3 rounded-xl border border-white/[0.06] bg-[#17181b] px-3 py-2"
          >
            <div className="min-w-0 flex-1">
              <h3 className="truncate text-sm font-medium text-zinc-100">{item.name}</h3>
              <p className="text-xs text-zinc-500">
                {new Date(item.createdAt).toLocaleString("zh-CN")}
              </p>
            </div>
            <button
              type="button"
              disabled={restoreMutation.isPending}
              onClick={() => restoreMutation.mutate(item.sourceId)}
              className="min-h-11 rounded-xl px-3 text-sm font-medium text-orange-400 outline-none hover:bg-orange-500/10 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
            >
              恢复 {item.name}
            </button>
          </article>
        ))}
      </div>
      {restoreMutation.isError && (
        <p role="alert" className="mt-3 text-sm text-red-300">
          {restoreMutation.error.message}
        </p>
      )}
    </section>
  )
}
