import { useStore } from "zustand"
import { createStore } from "zustand/vanilla"

interface HomeViewState {
  activeTopicId: string
  activeFilterId: string | null
  activeFilterPrompt: string | null
  scrollY: Record<string, number>
  queueGenerations: Record<string, string>
  saveScroll: (topicId: string, scrollY: number) => void
  setActiveTopic: (topicId: string) => void
  rememberQueueGeneration: (topicId: string, filterId: string | null, generation: string) => void
  forgetQueueGeneration: (topicId: string, filterId: string | null) => void
  activateFilter: (filterId: string, topicId: string, prompt?: string | null) => void
  clearFilter: (topicId?: string) => void
  reset: () => void
}

const initialData = {
  activeTopicId: "recommend",
  activeFilterId: null,
  activeFilterPrompt: null,
  scrollY: {},
  queueGenerations: {},
} satisfies Pick<
  HomeViewState,
  "activeTopicId" | "activeFilterId" | "activeFilterPrompt" | "scrollY" | "queueGenerations"
>

export const homeQueueScope = (topicId: string, filterId: string | null) =>
  `${topicId}\u0000${filterId ?? ""}`

export const homeViewStore = createStore<HomeViewState>((set) => ({
  ...initialData,
  saveScroll: (topicId, scrollY) =>
    set((state) => ({ scrollY: { ...state.scrollY, [topicId]: Math.max(0, scrollY) } })),
  setActiveTopic: (activeTopicId) => set({ activeTopicId }),
  rememberQueueGeneration: (topicId, filterId, generation) =>
    set((state) => ({
      queueGenerations: {
        ...state.queueGenerations,
        [homeQueueScope(topicId, filterId)]: generation,
      },
    })),
  forgetQueueGeneration: (topicId, filterId) =>
    set((state) => {
      const scope = homeQueueScope(topicId, filterId)
      const queueGenerations = { ...state.queueGenerations }
      delete queueGenerations[scope]
      return { queueGenerations }
    }),
  activateFilter: (activeFilterId, activeTopicId, activeFilterPrompt = null) =>
    set({ activeFilterId, activeTopicId, activeFilterPrompt }),
  clearFilter: (activeTopicId = "recommend") =>
    set({ activeFilterId: null, activeTopicId, activeFilterPrompt: null }),
  reset: () => set({ ...initialData, scrollY: {}, queueGenerations: {} }),
}))

export const useHomeViewStore = <T>(selector: (value: HomeViewState) => T) =>
  useStore(homeViewStore, selector)
