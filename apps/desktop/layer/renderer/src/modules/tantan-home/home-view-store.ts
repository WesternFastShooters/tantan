import { useStore } from "zustand"
import { createStore } from "zustand/vanilla"

interface HomeViewState {
  activeTopicId: string
  activeFilterId: string | null
  activeFilterPrompt: string | null
  scrollY: Record<string, number>
  saveScroll: (topicId: string, scrollY: number) => void
  setActiveTopic: (topicId: string) => void
  activateFilter: (filterId: string, topicId: string, prompt?: string | null) => void
  clearFilter: (topicId?: string) => void
  reset: () => void
}

const initialData = {
  activeTopicId: "recommend",
  activeFilterId: null,
  activeFilterPrompt: null,
  scrollY: {},
} satisfies Pick<
  HomeViewState,
  "activeTopicId" | "activeFilterId" | "activeFilterPrompt" | "scrollY"
>

export const homeViewStore = createStore<HomeViewState>((set) => ({
  ...initialData,
  saveScroll: (topicId, scrollY) =>
    set((state) => ({ scrollY: { ...state.scrollY, [topicId]: Math.max(0, scrollY) } })),
  setActiveTopic: (activeTopicId) => set({ activeTopicId }),
  activateFilter: (activeFilterId, activeTopicId, activeFilterPrompt = null) =>
    set({ activeFilterId, activeTopicId, activeFilterPrompt }),
  clearFilter: (activeTopicId = "recommend") =>
    set({ activeFilterId: null, activeTopicId, activeFilterPrompt: null }),
  reset: () => set({ ...initialData, scrollY: {} }),
}))

export const useHomeViewStore = <T>(selector: (value: HomeViewState) => T) =>
  useStore(homeViewStore, selector)
