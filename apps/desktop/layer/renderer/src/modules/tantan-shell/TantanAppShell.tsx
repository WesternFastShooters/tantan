import { cn } from "@follow/utils/utils"
import type { PropsWithChildren } from "react"
import { NavLink, Outlet, useLocation } from "react-router"

import { setMainContainerElement, setRootContainerElement } from "~/atoms/dom"
import { EntriesProvider } from "~/modules/entry-column/context/EntriesContext"
import { LocalServiceGuard } from "~/modules/tantan-service-status/LocalServiceGuard"

import { primaryRoutes } from "./primary-routes"

const handleRootRef = (element: HTMLDivElement | null) => setRootContainerElement(element)

const MobileTab = ({
  route,
  active,
}: {
  route: (typeof primaryRoutes)[number]
  active: boolean
}) => (
  <NavLink
    to={route.path}
    end={route.end}
    role="tab"
    aria-selected={active}
    className="flex min-h-11 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-xl px-1 py-1 text-[10px] font-medium text-zinc-500 outline-none transition-transform focus-visible:ring-2 focus-visible:ring-orange-500 active:scale-90 aria-selected:text-orange-500"
  >
    {({ isActive }) => (
      <>
        <i className={cn(route.icon, "size-[25px]", isActive && "text-orange-500")} aria-hidden />
        <span>{route.label}</span>
        <span className="sr-only" aria-live="polite">
          {isActive ? "当前页面" : ""}
        </span>
      </>
    )}
  </NavLink>
)

const MobileTabBar = () => {
  const location = useLocation()
  return (
    <nav
      role="tablist"
      aria-label="主导航"
      className="fixed inset-x-0 bottom-0 z-40 mx-auto grid min-h-16 max-w-[560px] grid-cols-4 gap-1 border-t border-zinc-200/70 bg-white/90 px-[max(0.5rem,env(safe-area-inset-left))] pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-1 shadow-[0_-6px_24px_rgba(0,0,0,0.04)] backdrop-blur-xl dark:border-white/10 dark:bg-zinc-950/90"
    >
      {primaryRoutes.map((route) => (
        <MobileTab key={route.path} route={route} active={route.path === location.pathname} />
      ))}
    </nav>
  )
}

const RouteContent = () => {
  const location = useLocation()
  const content = <Outlet />
  return location.pathname.startsWith("/timeline") ? (
    <EntriesProvider>{content}</EntriesProvider>
  ) : (
    content
  )
}

const primaryPaths = new Set(primaryRoutes.map((route) => route.path))

export function TantanAppShell() {
  const location = useLocation()
  const showTabs = primaryPaths.has(location.pathname as (typeof primaryRoutes)[number]["path"])

  return (
    <div
      ref={handleRootRef}
      data-testid="tantan-mobile-shell"
      className="relative mx-auto flex h-dvh min-h-0 w-full max-w-[560px] overflow-hidden bg-zinc-50 text-zinc-950 shadow-2xl dark:bg-[#08090b] dark:text-zinc-100"
    >
      <LocalServiceGuard>
        <main
          ref={setMainContainerElement}
          tabIndex={-1}
          className={cn(
            "min-w-0 flex-1 overflow-auto bg-zinc-50 outline-none dark:bg-[#08090b]",
            showTabs && "pb-[calc(4rem+max(0.5rem,env(safe-area-inset-bottom)))]",
          )}
        >
          <RouteContent />
        </main>
      </LocalServiceGuard>
      {showTabs && <MobileTabBar />}
    </div>
  )
}

export const TantanShellPage = ({ children }: PropsWithChildren) => (
  <section className="mx-auto min-h-full w-full px-4 pb-6 pt-[max(1.25rem,env(safe-area-inset-top))]">
    {children}
  </section>
)
