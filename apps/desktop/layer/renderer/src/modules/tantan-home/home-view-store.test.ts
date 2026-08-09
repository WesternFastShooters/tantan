import { beforeEach, describe, expect, test } from "vitest"

import { homeViewStore } from "./home-view-store"

describe("Tantan Home view state", () => {
  beforeEach(() => homeViewStore.getState().reset())

  test("REQ:FE-03 preserves independent Topic scroll and atomically activates a filter", () => {
    homeViewStore.getState().saveScroll("recommend", 320)
    homeViewStore.getState().setActiveTopic("topic-ai")
    homeViewStore.getState().saveScroll("topic-ai", 120)

    expect(homeViewStore.getState().scrollY).toEqual({ recommend: 320, "topic-ai": 120 })
    homeViewStore.getState().activateFilter("filter-1", "recommend")
    expect(homeViewStore.getState()).toMatchObject({
      activeFilterId: "filter-1",
      activeTopicId: "recommend",
    })
  })
})
