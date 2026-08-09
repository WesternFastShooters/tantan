import type { InfiniteData, QueryKey } from "@tanstack/react-query"
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { TantanAPIError } from "~/lib/tantan-api/client"
import type {
  FeedbackRequest,
  HomeCard,
  HomeResponse,
  TopicsResponse,
} from "~/lib/tantan-api/gen/types"

import {
  deleteActiveFilter,
  getHome,
  getTopics,
  postRecommendationFeedback,
  putActiveFilter,
} from "./api"
import { removeEntryFromAllHomeQueries } from "./home-cache"
import {
  assertHomePageGeneration,
  dedupeHomeCards,
  HomeQueueGenerationChangedError,
  nextHomePageParam,
} from "./home-model"
import { homeQueueScope, homeViewStore, useHomeViewStore } from "./home-view-store"
import { homeQueryKeys } from "./query-keys"

const fallbackTopic = {
  id: "recommend",
  name: "推荐",
  kind: "core",
  fixed: true,
  pinned: true,
  hidden: false,
  unreadCount: 0,
} as const

type FeedbackCommand = {
  card: HomeCard
  action: FeedbackRequest["action"]
  topicId?: string
}

type HomeSnapshot = [QueryKey, InfiniteData<HomeResponse> | undefined]

type UndoFeedback = {
  entryId: string
  label: string
  snapshots: HomeSnapshot[]
  expiresAt: number
}

const isQueueGenerationError = (error: unknown) =>
  error instanceof HomeQueueGenerationChangedError ||
  (error instanceof TantanAPIError && error.code === "QUEUE_VERSION_CHANGED")

export function useHomeController(scrollRef: React.RefObject<HTMLDivElement | null>) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const activeTopicId = useHomeViewStore((state) => state.activeTopicId)
  const activeFilterId = useHomeViewStore((state) => state.activeFilterId)
  const activeFilterPrompt = useHomeViewStore((state) => state.activeFilterPrompt)
  const scrollY = useHomeViewStore((state) => state.scrollY)
  const queueGeneration = useHomeViewStore(
    (state) => state.queueGenerations[homeQueueScope(activeTopicId, activeFilterId)] ?? null,
  )
  const [filterSheetOpen, setFilterSheetOpen] = useState(false)
  const [undoFeedback, setUndoFeedback] = useState<UndoFeedback | null>(null)
  const [feedbackError, setFeedbackError] = useState<string | null>(null)
  const [queueRefreshNotice, setQueueRefreshNotice] = useState<string | null>(null)
  const filterTriggerRef = useRef<HTMLElement | null>(null)

  const topicsQuery = useQuery({
    queryKey: homeQueryKeys.topics,
    queryFn: ({ signal }) => getTopics(signal),
    staleTime: 30_000,
  })

  useEffect(() => {
    const serverFilterId = topicsQuery.data?.activeFilterId
    if (serverFilterId && serverFilterId !== homeViewStore.getState().activeFilterId) {
      homeViewStore.getState().activateFilter(serverFilterId, "recommend", "AI 智能筛选已启用")
    }
  }, [topicsQuery.data?.activeFilterId])

  const homeQueryKey = homeQueryKeys.feed(activeTopicId, activeFilterId, queueGeneration)
  const homeQuery = useInfiniteQuery({
    queryKey: homeQueryKey,
    queryFn: async ({ pageParam, signal }) => {
      const page = await getHome({
        topicId: activeTopicId,
        filterId: activeFilterId,
        cursor: pageParam.cursor,
        signal,
      })
      return assertHomePageGeneration(page, pageParam.generation)
    },
    initialPageParam: { cursor: null, generation: null } as const,
    getNextPageParam: nextHomePageParam,
    staleTime: 30_000,
  })

  useEffect(() => {
    const data = homeQuery.data
    const returnedGeneration = data?.pages[0]?.queueGeneration
    if (!data || !returnedGeneration || returnedGeneration === queueGeneration) return
    queryClient.setQueryData(
      homeQueryKeys.feed(activeTopicId, activeFilterId, returnedGeneration),
      data,
    )
    homeViewStore
      .getState()
      .rememberQueueGeneration(activeTopicId, activeFilterId, returnedGeneration)
  }, [activeFilterId, activeTopicId, homeQuery.data, queryClient, queueGeneration])

  const cards = useMemo(
    () => dedupeHomeCards(homeQuery.data?.pages.flatMap((page) => page.items) ?? []),
    [homeQuery.data?.pages],
  )
  const queue = homeQuery.data?.pages.at(-1)?.queue ?? null

  const saveCurrentScroll = useCallback(() => {
    homeViewStore.getState().saveScroll(activeTopicId, scrollRef.current?.scrollTop ?? 0)
  }, [activeTopicId, scrollRef])

  const changeTopic = useCallback(
    (topicId: string) => {
      if (topicId === activeTopicId) return
      saveCurrentScroll()
      homeViewStore.getState().setActiveTopic(topicId)
    },
    [activeTopicId, saveCurrentScroll],
  )

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      if (scrollRef.current) scrollRef.current.scrollTop = scrollY[activeTopicId] ?? 0
    })
    return () => cancelAnimationFrame(frame)
  }, [activeTopicId, scrollRef, scrollY])

  const openSearch = useCallback(() => {
    saveCurrentScroll()
    navigate("/search", {
      state: {
        returnTopicId: activeTopicId,
        returnFilterId: activeFilterId,
        returnScrollY: scrollRef.current?.scrollTop ?? 0,
      },
    })
  }, [activeFilterId, activeTopicId, navigate, saveCurrentScroll, scrollRef])

  const openFilterSheet = useCallback(() => {
    filterTriggerRef.current = document.activeElement as HTMLElement | null
    setFilterSheetOpen(true)
  }, [])

  const closeFilterSheet = useCallback(() => {
    setFilterSheetOpen(false)
    requestAnimationFrame(() => filterTriggerRef.current?.focus())
  }, [])

  const filterMutation = useMutation({
    mutationFn: putActiveFilter,
    onSuccess: (result, prompt) => {
      queryClient.setQueryData<TopicsResponse>(homeQueryKeys.topics, (current) => ({
        version: current?.version ?? 1,
        topicsRevision: result.topicsRevision,
        activeFilterId: result.filter?.id ?? null,
        topics: result.topics,
      }))
      if (result.filter) {
        homeViewStore
          .getState()
          .rememberQueueGeneration("recommend", result.filter.id, result.queueGeneration)
        homeViewStore
          .getState()
          .activateFilter(result.filter.id, "recommend", result.filter.prompt || prompt)
      }
      setFilterSheetOpen(false)
      queryClient.invalidateQueries({ queryKey: homeQueryKeys.all })
      requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = 0
        filterTriggerRef.current?.focus()
      })
    },
  })

  const resetFilterMutation = useMutation({
    mutationFn: deleteActiveFilter,
    onSuccess: (result) => {
      queryClient.setQueryData<TopicsResponse>(homeQueryKeys.topics, (current) => ({
        version: current?.version ?? 1,
        topicsRevision: result.topicsRevision,
        activeFilterId: null,
        topics: result.topics,
      }))
      homeViewStore.getState().clearFilter("recommend")
      homeViewStore.getState().rememberQueueGeneration("recommend", null, result.queueGeneration)
      queryClient.invalidateQueries({ queryKey: homeQueryKeys.all })
      requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = 0
      })
    },
  })

  const feedbackMutation = useMutation({
    mutationFn: ({ card, action, topicId }: FeedbackCommand) =>
      postRecommendationFeedback({
        entryId: card.entryId,
        action,
        ...(action === "block_topic" && topicId ? { topicId } : {}),
      }),
    onMutate: async ({ card }) => {
      setFeedbackError(null)
      setUndoFeedback(null)
      await queryClient.cancelQueries({ queryKey: homeQueryKeys.all })
      const snapshots = queryClient.getQueriesData<InfiniteData<HomeResponse>>({
        queryKey: homeQueryKeys.all,
      })
      removeEntryFromAllHomeQueries(queryClient, card.entryId)
      return { snapshots }
    },
    onSuccess: (_result, command, context) => {
      const label =
        command.action === "block_source"
          ? `已屏蔽 ${command.card.source.name}`
          : command.action === "block_topic"
            ? `已减少 ${command.card.topics[0]?.name ?? "该 Topic"}`
            : "已减少此类推荐"
      setUndoFeedback({
        entryId: command.card.entryId,
        label,
        snapshots: context?.snapshots ?? [],
        expiresAt: Date.now() + 5_000,
      })
    },
    onError: (error, _command, context) => {
      context?.snapshots.forEach(([queryKey, data]) => queryClient.setQueryData(queryKey, data))
      setFeedbackError(error instanceof Error ? error.message : "推荐反馈失败，卡片已恢复")
    },
  })

  const undoFeedbackMutation = useMutation({
    mutationFn: (state: UndoFeedback) =>
      postRecommendationFeedback({ entryId: state.entryId, action: "undo" }),
    onMutate: () => setFeedbackError(null),
    onSuccess: (_result, state) => {
      state.snapshots.forEach(([queryKey, data]) => queryClient.setQueryData(queryKey, data))
      setUndoFeedback(null)
    },
    onError: (error) => {
      setFeedbackError(error instanceof Error ? error.message : "撤销失败")
    },
  })

  useEffect(() => {
    if (!undoFeedback) return
    const timeout = window.setTimeout(
      () => setUndoFeedback((current) => (current === undoFeedback ? null : current)),
      Math.max(0, undoFeedback.expiresAt - Date.now()),
    )
    return () => window.clearTimeout(timeout)
  }, [undoFeedback])

  const sendFeedback = useCallback(
    (card: HomeCard, action: FeedbackRequest["action"], topicId?: string) => {
      if (action === "undo") return
      feedbackMutation.mutate({ card, action, topicId })
    },
    [feedbackMutation],
  )

  const fetchNext = useCallback(async () => {
    try {
      const result = await homeQuery.fetchNextPage()
      if (result.error && isQueueGenerationError(result.error)) {
        queryClient.removeQueries({
          queryKey: homeQueryKeys.scope(activeTopicId, activeFilterId),
        })
        homeViewStore.getState().forgetQueueGeneration(activeTopicId, activeFilterId)
        setQueueRefreshNotice("推荐已更新")
      }
    } catch (error) {
      if (!isQueueGenerationError(error)) throw error
      queryClient.removeQueries({
        queryKey: homeQueryKeys.scope(activeTopicId, activeFilterId),
      })
      homeViewStore.getState().forgetQueueGeneration(activeTopicId, activeFilterId)
      setQueueRefreshNotice("推荐已更新")
    }
  }, [activeFilterId, activeTopicId, homeQuery, queryClient])

  useEffect(() => {
    if (!queueRefreshNotice) return
    const timeout = window.setTimeout(() => setQueueRefreshNotice(null), 3_000)
    return () => window.clearTimeout(timeout)
  }, [queueRefreshNotice])

  return {
    activeTopicId,
    activeFilterId,
    activeFilterPrompt,
    topics: topicsQuery.data?.topics ?? [fallbackTopic],
    topicsLoading: topicsQuery.isPending,
    cards,
    queue,
    homeLoading: homeQuery.isPending,
    homeError:
      homeQuery.error instanceof Error && !isQueueGenerationError(homeQuery.error)
        ? homeQuery.error.message
        : null,
    refetchHome: homeQuery.refetch,
    hasNextPage: Boolean(homeQuery.hasNextPage),
    fetchingNext: homeQuery.isFetchingNextPage,
    fetchNext,
    queueRefreshNotice,
    changeTopic,
    openSearch,
    openFilterSheet,
    closeFilterSheet,
    filterSheetOpen,
    submitFilter: (prompt: string) => filterMutation.mutate(prompt),
    filterPending: filterMutation.isPending,
    filterError: filterMutation.error instanceof Error ? filterMutation.error.message : null,
    resetFilter: () => resetFilterMutation.mutate(),
    resetFilterPending: resetFilterMutation.isPending,
    saveCurrentScroll,
    sendFeedback,
    undoFeedback,
    undoFeedbackPending: undoFeedbackMutation.isPending,
    undoLastFeedback: () => {
      if (undoFeedback) undoFeedbackMutation.mutate(undoFeedback)
    },
    feedbackError,
  }
}
