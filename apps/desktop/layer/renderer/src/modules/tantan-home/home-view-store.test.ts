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

  test("ST-03 remembers queue generations independently by topic and filter scope", () => {
    homeViewStore.getState().rememberQueueGeneration("recommend", null, "generation-default")
    homeViewStore.getState().rememberQueueGeneration("topic-ai", null, "generation-ai")
    homeViewStore.getState().rememberQueueGeneration("recommend", "filter-1", "generation-filter")

    expect(homeViewStore.getState().queueGenerations).toEqual({
      "recommend\u0000": "generation-default",
      "topic-ai\u0000": "generation-ai",
      "recommend\u0000filter-1": "generation-filter",
    })

    homeViewStore.getState().forgetQueueGeneration("recommend", "filter-1")
    expect(homeViewStore.getState().queueGenerations["recommend\u0000filter-1"]).toBeUndefined()
    expect(homeViewStore.getState().queueGenerations["recommend\u0000"]).toBe("generation-default")
  })
})
