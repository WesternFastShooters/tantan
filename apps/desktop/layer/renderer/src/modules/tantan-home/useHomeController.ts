import type { InfiniteData } from "@tanstack/react-query"
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

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
import { dedupeHomeCards } from "./home-model"
import { homeViewStore, useHomeViewStore } from "./home-view-store"
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

export function useHomeController(scrollRef: React.RefObject<HTMLDivElement | null>) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const activeTopicId = useHomeViewStore((state) => state.activeTopicId)
  const activeFilterId = useHomeViewStore((state) => state.activeFilterId)
  const activeFilterPrompt = useHomeViewStore((state) => state.activeFilterPrompt)
  const scrollY = useHomeViewStore((state) => state.scrollY)
  const [filterSheetOpen, setFilterSheetOpen] = useState(false)
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

  const homeQuery = useInfiniteQuery({
    queryKey: homeQueryKeys.feed(activeTopicId, activeFilterId),
    queryFn: ({ pageParam, signal }) =>
      getHome({
        topicId: activeTopicId,
        filterId: activeFilterId,
        cursor: pageParam,
        signal,
      }),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })

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
        activeFilterId: result.filter?.id ?? null,
        topics: result.topics,
      }))
      if (result.filter) {
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
        activeFilterId: null,
        topics: result.topics,
      }))
      homeViewStore.getState().clearFilter("recommend")
      queryClient.invalidateQueries({ queryKey: homeQueryKeys.all })
      requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = 0
      })
    },
  })

  const feedbackMutation = useMutation({
    mutationFn: (body: FeedbackRequest) => postRecommendationFeedback(body),
    onMutate: async (body) => {
      await queryClient.cancelQueries({ queryKey: homeQueryKeys.all })
      const snapshots = queryClient.getQueriesData<InfiniteData<HomeResponse>>({
        queryKey: homeQueryKeys.all,
      })
      removeEntryFromAllHomeQueries(queryClient, body.entryId)
      return { snapshots }
    },
    onError: (_error, _body, context) => {
      context?.snapshots.forEach(([queryKey, data]) => queryClient.setQueryData(queryKey, data))
    },
  })

  const notInterested = useCallback(
    (card: HomeCard) => {
      feedbackMutation.mutate({
        entryId: card.entryId,
        action: "not_interested",
        topicId: activeTopicId,
      })
    },
    [activeTopicId, feedbackMutation],
  )

  return {
    activeTopicId,
    activeFilterId,
    activeFilterPrompt,
    topics: topicsQuery.data?.topics ?? [fallbackTopic],
    topicsLoading: topicsQuery.isPending,
    cards,
    queue,
    homeLoading: homeQuery.isPending,
    homeError: homeQuery.error instanceof Error ? homeQuery.error.message : null,
    refetchHome: homeQuery.refetch,
    hasNextPage: Boolean(homeQuery.hasNextPage),
    fetchingNext: homeQuery.isFetchingNextPage,
    fetchNext: () => homeQuery.fetchNextPage(),
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
    notInterested,
  }
}
