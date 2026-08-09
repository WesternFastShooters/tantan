import { Spring } from "@follow/components/constants/spring.js"
import { useMobile } from "@follow/components/hooks/useMobile.js"
import { AnimatePresence } from "motion/react"
import { memo, useEffect, useState } from "react"

import { m } from "~/components/common/Motion"
import { ROUTE_ENTRY_PENDING } from "~/constants"
import { useNavigateEntry } from "~/hooks/biz/useNavigateEntry"
import { useRouteParamsSelector } from "~/hooks/biz/useRouteParams"
import { useShowEntryDetailsColumn } from "~/hooks/biz/useShowEntryDetailsColumn"
import { EntryContentPlaceholder } from "~/modules/app-layout/entry-content/EntryContentPlaceholder"
import { EntryColumn } from "~/modules/entry-column"
import { EntryContent } from "~/modules/entry-content/components/entry-content"
import { AIEntryHeader as EntryHeader } from "~/modules/entry-content/components/entry-header"
import { AppLayoutGridContainerProvider } from "~/providers/app-grid-layout-container-provider"

const DesktopTimelineLayout = ({ entryId }: { entryId: string }) => {
  const showEntryDetailsColumn = useShowEntryDetailsColumn()
  const hasEntry = Boolean(entryId)

  return (
    <AppLayoutGridContainerProvider>
      <div className="relative flex size-full min-w-0 overflow-hidden">
        <section
          data-hide-in-print={showEntryDetailsColumn ? true : undefined}
          className={
            showEntryDetailsColumn
              ? "relative flex h-full w-[min(44%,560px)] min-w-[300px] flex-none flex-col overflow-hidden border-r"
              : "relative flex h-full min-w-0 flex-1 flex-col overflow-hidden"
          }
        >
          <EntryColumn />
          {!showEntryDetailsColumn && hasEntry && (
            <div className="absolute inset-0 z-[9] flex flex-col overflow-hidden bg-theme-background">
              <EntryHeader entryId={entryId} />
              <EntryContent entryId={entryId} className="h-0 flex-1" />
            </div>
          )}
        </section>

        {showEntryDetailsColumn && (
          <section className="relative flex h-full min-w-0 flex-1 flex-col overflow-hidden bg-theme-background print:w-full">
            {hasEntry ? (
              <>
                <EntryHeader entryId={entryId} />
                <EntryContent entryId={entryId} className="h-0 flex-1" />
              </>
            ) : (
              <div className="flex flex-1 items-center justify-center px-8">
                <EntryContentPlaceholder />
              </div>
            )}
          </section>
        )}
      </div>
    </AppLayoutGridContainerProvider>
  )
}

const MobileTimelineLayout = ({ entryId }: { entryId: string }) => {
  const [view, setView] = useState<"list" | "entry">(entryId ? "entry" : "list")
  const navigate = useNavigateEntry()
  const routeView = useRouteParamsSelector((state) => state.view)

  useEffect(() => {
    setView(entryId ? "entry" : "list")
  }, [entryId])

  return (
    <AppLayoutGridContainerProvider>
      <div className="relative size-full overflow-hidden">
        <AnimatePresence mode="wait">
          {view === "list" ? (
            <m.div
              key="timeline-list"
              className="absolute inset-0 flex flex-col overflow-hidden"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={Spring.smooth(0.2)}
            >
              <EntryColumn />
            </m.div>
          ) : (
            entryId && (
              <m.div
                key="timeline-entry"
                className="absolute inset-0 flex flex-col overflow-hidden bg-theme-background"
                initial={{ opacity: 0, x: 20 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: 20 }}
                transition={Spring.smooth(0.2)}
              >
                <div className="flex items-center border-b bg-background">
                  <button
                    type="button"
                    aria-label="Back to entries"
                    className="m-1 inline-flex shrink-0 items-center rounded-full p-2 text-text-secondary hover:bg-fill/50"
                    onClick={() => navigate({ entryId: null, view: routeView })}
                  >
                    <i className="i-mingcute-arrow-left-line size-5" />
                  </button>
                  <div className="min-w-0 flex-1 overflow-hidden">
                    <EntryHeader entryId={entryId} />
                  </div>
                </div>
                <EntryContent entryId={entryId} className="h-0 flex-1" />
              </m.div>
            )
          )}
        </AnimatePresence>
      </div>
    </AppLayoutGridContainerProvider>
  )
}

export const TimelineLayout = memo(function TimelineLayout() {
  const entryId = useRouteParamsSelector((state) => state.entryId)
  const isMobile = useMobile()
  const realEntryId = entryId === ROUTE_ENTRY_PENDING ? "" : (entryId ?? "")

  return isMobile ? (
    <MobileTimelineLayout entryId={realEntryId} />
  ) : (
    <DesktopTimelineLayout entryId={realEntryId} />
  )
})
