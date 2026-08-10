import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import type { TopicPatchRequest, TopicsResponse } from "~/lib/tantan-api/gen/types"
import { getTopics } from "~/modules/tantan-home/api"
import { homeQueryKeys } from "~/modules/tantan-home/query-keys"
import { patchTopics } from "~/modules/tantan-settings/api"

export function TopicManager() {
  const queryClient = useQueryClient()
  const topicsQuery = useQuery({
    queryKey: homeQueryKeys.topics,
    queryFn: ({ signal }) => getTopics(signal),
  })
  const patchMutation = useMutation({
    mutationFn: (request: TopicPatchRequest) => patchTopics(request),
    onSuccess: (response) => {
      queryClient.setQueryData<TopicsResponse>(homeQueryKeys.topics, response)
      queryClient.invalidateQueries({ queryKey: homeQueryKeys.all })
    },
  })
  const response = topicsQuery.data
  const topics = response?.topics ?? []

  const mutate = (operations: TopicPatchRequest["operations"]) => {
    if (!response || patchMutation.isPending) return
    patchMutation.mutate({ version: response.version, operations })
  }

  if (topicsQuery.isPending)
    return <div aria-busy="true" className="h-64 animate-pulse rounded-xl bg-[#17181b]" />
  if (topicsQuery.isError) {
    return (
      <div role="alert" className="rounded-xl bg-red-500/10 p-4 text-sm text-red-300">
        {topicsQuery.error.message}
      </div>
    )
  }

  return (
    <div className="space-y-2" aria-busy={patchMutation.isPending}>
      {topics.map((topic, index) => {
        const immutable = topic.id === "recommend"
        return (
          <article
            key={topic.id}
            className="flex min-h-16 items-center gap-3 rounded-xl border border-white/[0.06] bg-[#17181b] px-3 py-2"
          >
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-sm text-zinc-400">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <h2 className="truncate text-sm font-medium text-zinc-100">{topic.name}</h2>
              <p className="text-xs text-zinc-500">
                {immutable
                  ? "系统推荐 · 不可隐藏或移动"
                  : `${topic.kind} · ${topic.unreadCount} 未读`}
              </p>
            </div>
            {!immutable && (
              <div className="flex shrink-0 items-center">
                <button
                  type="button"
                  aria-label={`上移 ${topic.name}`}
                  disabled={index <= 1 || patchMutation.isPending}
                  onClick={() =>
                    mutate([
                      {
                        op: "move",
                        topicId: topic.id,
                        afterTopicId: topics[index - 2]?.id ?? "recommend",
                      },
                    ])
                  }
                  className="flex size-11 items-center justify-center rounded-lg text-zinc-400 outline-none hover:bg-white/10 hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-30"
                >
                  <i className="i-mgc-up-cute-re size-4" aria-hidden />
                </button>
                <button
                  type="button"
                  aria-label={`下移 ${topic.name}`}
                  disabled={index >= topics.length - 1 || patchMutation.isPending}
                  onClick={() =>
                    mutate([
                      {
                        op: "move",
                        topicId: topic.id,
                        afterTopicId: topics[index + 1]?.id ?? null,
                      },
                    ])
                  }
                  className="flex size-11 items-center justify-center rounded-lg text-zinc-400 outline-none hover:bg-white/10 hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-30"
                >
                  <i className="i-mgc-sort-descending-cute-re size-4" aria-hidden />
                </button>
                <button
                  type="button"
                  aria-label={`${topic.pinned ? "取消固定" : "固定"} ${topic.name}`}
                  disabled={patchMutation.isPending}
                  onClick={() =>
                    mutate([{ op: topic.pinned ? "unpin" : "pin", topicId: topic.id }])
                  }
                  className="flex size-11 items-center justify-center rounded-lg text-zinc-400 outline-none hover:bg-white/10 hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-30"
                >
                  <i className="i-mgc-bookmark-cute-re size-4" aria-hidden />
                </button>
                <button
                  type="button"
                  aria-label={`${topic.hidden ? "显示" : "隐藏"} ${topic.name}`}
                  disabled={patchMutation.isPending}
                  onClick={() =>
                    mutate([{ op: topic.hidden ? "show" : "hide", topicId: topic.id }])
                  }
                  className="flex size-11 items-center justify-center rounded-lg text-zinc-400 outline-none hover:bg-white/10 hover:text-zinc-100 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-30"
                >
                  <i
                    className={
                      topic.hidden ? "i-mgc-eye-2-cute-re size-4" : "i-mgc-eye-close-cute-re size-4"
                    }
                    aria-hidden
                  />
                </button>
              </div>
            )}
          </article>
        )
      })}
      {patchMutation.isError && (
        <p role="alert" className="text-sm text-red-300">
          {patchMutation.error.message}
        </p>
      )}
    </div>
  )
}
