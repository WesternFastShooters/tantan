import { describe, expect, test } from "vitest"

import { homeQueryKeys } from "./query-keys"

describe("Tantan Home query keys", () => {
  test("ST-03 scopes pages by topic, filter, and queue generation", () => {
    expect(homeQueryKeys.scope("topic-ai", "filter-1")).toEqual(["home", "topic-ai", "filter-1"])
    expect(homeQueryKeys.feed("topic-ai", "filter-1", "generation-2")).toEqual([
      "home",
      "topic-ai",
      "filter-1",
      "generation-2",
    ])
  })
})
