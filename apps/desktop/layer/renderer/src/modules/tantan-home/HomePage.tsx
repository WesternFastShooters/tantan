import { ScrollArea } from "@follow/components/ui/scroll-area/index.js"
import { useRef } from "react"

import { ActiveAIFilterBar } from "./ActiveAIFilterBar"
import { AIFilterSheet } from "./AIFilterSheet"
import { HomeHeader } from "./HomeHeader"
import { MasonryFeed } from "./MasonryFeed"
import { TopicTabs } from "./TopicTabs"
import { useHomeController } from "./useHomeController"

export function HomePage() {
  const scrollRef = useRef<HTMLDivElement>(null)
  const controller = useHomeController(scrollRef)

  return (
    <ScrollArea.ScrollArea
      ref={scrollRef}
      rootClassName="h-full bg-[#08090b]"
      viewportClassName="h-full"
      viewportProps={{ "data-testid": "home-scroll-viewport" }}
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
          onEdit={controller.openFilterSheet}
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
            columns={2}
            loading={controller.homeLoading || controller.topicsLoading}
            fetchingNext={controller.fetchingNext}
            hasNextPage={controller.hasNextPage}
            onFetchNext={controller.fetchNext}
            onOpenCard={controller.saveCurrentScroll}
            onFeedback={controller.sendFeedback}
          />
        )}
        <AIFilterSheet
          open={controller.filterSheetOpen}
          initialPrompt={controller.activeFilterPrompt ?? ""}
          pending={controller.filterPending}
          error={controller.filterError}
          onClose={controller.closeFilterSheet}
          onSubmit={controller.submitFilter}
        />
        {controller.undoFeedback && (
          <aside
            role="status"
            className="fixed bottom-20 left-1/2 z-40 flex min-h-12 -translate-x-1/2 items-center gap-3 rounded-xl border border-white/10 bg-zinc-900 px-4 text-sm text-zinc-100 shadow-xl md:bottom-6"
          >
            <span>{controller.undoFeedback.label}</span>
            <button
              type="button"
              aria-label="撤销推荐反馈"
              disabled={controller.undoFeedbackPending}
              onClick={controller.undoLastFeedback}
              className="min-h-10 rounded-lg px-2 font-semibold text-orange-400 outline-none focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
            >
              撤销
            </button>
          </aside>
        )}
        {controller.feedbackError && (
          <p
            role="alert"
            className="fixed bottom-20 left-1/2 z-40 -translate-x-1/2 rounded-xl bg-red-950 px-4 py-3 text-sm text-red-200 shadow-xl md:bottom-6"
          >
            {controller.feedbackError}
          </p>
        )}
        {controller.queueRefreshNotice && (
          <p
            role="status"
            className="fixed bottom-20 left-1/2 z-40 -translate-x-1/2 rounded-xl bg-zinc-900 px-4 py-3 text-sm text-zinc-100 shadow-xl md:bottom-6"
          >
            {controller.queueRefreshNotice}
          </p>
        )}
      </section>
    </ScrollArea.ScrollArea>
  )
}
