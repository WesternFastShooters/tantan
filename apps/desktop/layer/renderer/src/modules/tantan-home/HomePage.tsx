import { ScrollArea } from "@follow/components/ui/scroll-area/index.js"
import { useRef } from "react"
import { useWindowSize } from "usehooks-ts"

import { ActiveAIFilterBar } from "./ActiveAIFilterBar"
import { AIFilterSheet } from "./AIFilterSheet"
import { homeColumnCount } from "./home-model"
import { HomeHeader } from "./HomeHeader"
import { MasonryFeed } from "./MasonryFeed"
import { TopicTabs } from "./TopicTabs"
import { useHomeController } from "./useHomeController"

export function HomePage() {
  const scrollRef = useRef<HTMLDivElement>(null)
  const { width } = useWindowSize({ initializeWithValue: true })
  const columns = homeColumnCount(width)
  const controller = useHomeController(scrollRef)

  return (
    <ScrollArea.ScrollArea
      ref={scrollRef}
      rootClassName="h-full bg-[#08090b]"
      viewportClassName="h-full"
      scrollbarClassName="z-20"
    >
      <section className="mx-auto min-h-full w-full max-w-[1280px]">
        <div className="sticky top-0 z-20 border-b border-white/[0.06] bg-[#08090b]/95 backdrop-blur">
          <HomeHeader
            onSearch={controller.openSearch}
            onOpenAIFilter={controller.openFilterSheet}
          />
          <TopicTabs
            topics={controller.topics}
            activeTopicId={controller.activeTopicId}
            onChange={controller.changeTopic}
          />
        </div>
        <ActiveAIFilterBar
          prompt={controller.activeFilterPrompt}
          onReset={controller.resetFilter}
          resetting={controller.resetFilterPending}
        />
        {controller.homeError ? (
          <div
            role="alert"
            className="flex min-h-80 flex-col items-center justify-center px-6 text-center"
          >
            <p className="text-sm text-red-300">{controller.homeError}</p>
            <button
              type="button"
              onClick={() => controller.refetchHome()}
              className="mt-3 min-h-11 rounded-xl bg-white/10 px-4 text-sm text-zinc-100 outline-none hover:bg-white/15 focus-visible:ring-2 focus-visible:ring-orange-500"
            >
              重试
            </button>
          </div>
        ) : (
          <MasonryFeed
            cards={controller.cards}
            queue={controller.queue}
            columns={columns}
            loading={controller.homeLoading || controller.topicsLoading}
            fetchingNext={controller.fetchingNext}
            hasNextPage={controller.hasNextPage}
            onFetchNext={controller.fetchNext}
            onOpenCard={controller.saveCurrentScroll}
            onNotInterested={controller.notInterested}
          />
        )}
        <AIFilterSheet
          open={controller.filterSheetOpen}
          pending={controller.filterPending}
          error={controller.filterError}
          onClose={controller.closeFilterSheet}
          onSubmit={controller.submitFilter}
        />
      </section>
    </ScrollArea.ScrollArea>
  )
}
