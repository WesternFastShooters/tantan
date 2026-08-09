import type { InfiniteData } from "@tanstack/react-query"
import { QueryClient } from "@tanstack/react-query"
import { describe, expect, test } from "vitest"

import type { HomeResponse } from "~/lib/tantan-api/gen/types"

import { removeEntryFromAllHomeQueries } from "./home-cache"

const page = (ids: string[]): HomeResponse => ({
  items: ids.map((entryId) => ({
    entryId,
    type: "article",
    title: entryId,
    excerpt: null,
    cover: null,
    source: { id: "source", name: "Source", avatar: null },
    publishedAt: "2026-08-09T12:00:00Z",
    topics: [],
    translated: false,
  })),
  nextCursor: null,
  queue: {
    id: "queue",
    version: 1,
    total: ids.length,
    unread: ids.length,
    finished: false,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
})

describe("Tantan Home cache", () => {
  test("REQ:FE-03 removes a read entry from every Home query without reordering siblings", () => {
    const queryClient = new QueryClient()
    const recommend: InfiniteData<HomeResponse> = {
      pages: [page(["entry-1", "entry-2"]), page(["entry-2", "entry-3"])],
      pageParams: [null, "cursor-2"],
    }
    const topic: InfiniteData<HomeResponse> = {
      pages: [page(["entry-4", "entry-2", "entry-5"])],
      pageParams: [null],
    }
    queryClient.setQueryData(["home", "recommend", null], recommend)
    queryClient.setQueryData(["home", "topic-ai", "filter-1"], topic)
    queryClient.setQueryData(["search", "entry-2"], { untouched: true })

    removeEntryFromAllHomeQueries(queryClient, "entry-2")

    const recommendAfter = queryClient.getQueryData<InfiniteData<HomeResponse>>([
      "home",
      "recommend",
      null,
    ])
    const topicAfter = queryClient.getQueryData<InfiniteData<HomeResponse>>([
      "home",
      "topic-ai",
      "filter-1",
    ])
    expect(
      recommendAfter?.pages.flatMap((item) => item.items.map(({ entryId }) => entryId)),
    ).toEqual(["entry-1", "entry-3"])
    expect(topicAfter?.pages[0]?.items.map(({ entryId }) => entryId)).toEqual([
      "entry-4",
      "entry-5",
    ])
    expect(queryClient.getQueryData(["search", "entry-2"])).toEqual({ untouched: true })
  })
})
