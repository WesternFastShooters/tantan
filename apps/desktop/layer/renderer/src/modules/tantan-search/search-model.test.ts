import { describe, expect, test } from "vitest"

import { buildSearchRequest, mergeSearchPages, SEARCH_DEBOUNCE_MS } from "./search-model"

describe("Tantan search model", () => {
  test("REQ:FE-04 keeps the approved debounce and isolated query contract", () => {
    expect(SEARCH_DEBOUNCE_MS).toBe(250)
    expect(buildSearchRequest("  Claude Code  ", null)).toEqual({
      q: "Claude Code",
      cursor: null,
      limit: 20,
    })
  })

  test("REQ:FE-04 deduplicates overlapping search pages without changing first results", () => {
    const item = (entryId: string, title = entryId) => ({
      entryId,
      type: "article" as const,
      title,
      excerpt: null,
      cover: null,
      source: { id: "source", name: "Source", avatar: null },
      publishedAt: "2026-08-09T12:00:00Z",
      topics: [],
      translated: false,
    })
    const first = item("entry-1")
    expect(mergeSearchPages([[first, item("entry-2")], [item("entry-1", "duplicate")]])).toEqual([
      first,
      item("entry-2"),
    ])
  })
})
