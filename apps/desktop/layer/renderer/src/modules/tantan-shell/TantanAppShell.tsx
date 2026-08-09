import { cn } from "@follow/utils/utils"
import type { PropsWithChildren } from "react"
import { NavLink, Outlet, useLocation } from "react-router"

import { setMainContainerElement, setRootContainerElement } from "~/atoms/dom"
import { EntriesProvider } from "~/modules/entry-column/context/EntriesContext"
import { LocalServiceGuard } from "~/modules/tantan-service-status/LocalServiceGuard"

import { primaryRoutes } from "./primary-routes"
import { useTantanMobile } from "./useTantanMobile"

const handleRootRef = (element: HTMLDivElement | null) => setRootContainerElement(element)

const PrimaryLink = ({
  route,
  compact,
}: {
  route: (typeof primaryRoutes)[number]
  compact: boolean
}) => (
  <NavLink
    to={route.path}
    end={route.end}
    className={({ isActive }) =>
      cn(
        "flex min-h-11 items-center rounded-xl text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-red-400",
        compact ? "min-w-16 flex-1 flex-col justify-center gap-0.5 px-2 py-1" : "gap-3 px-3",
        isActive
          ? "bg-red-500/15 text-red-400"
          : "text-zinc-400 hover:bg-white/5 hover:text-zinc-100",
      )
    }
  >
    <i className={cn(route.icon, "size-5 shrink-0")} aria-hidden />
    <span>{route.label}</span>
  </NavLink>
)

const DesktopNavigation = () => (
  <aside className="hidden h-full w-[72px] shrink-0 border-r border-white/10 bg-zinc-950 md:flex md:flex-col lg:w-60">
    <div className="flex h-16 items-center px-4 text-lg font-bold text-zinc-50 lg:px-6">
      <span className="text-red-500">T</span>
      <span className="hidden lg:inline">antan</span>
    </div>
    <nav aria-label="Primary navigation" className="flex flex-1 flex-col gap-1 px-2 lg:px-3">
      {primaryRoutes.map((route) => (
        <PrimaryLink key={route.path} route={route} compact={false} />
      ))}
    </nav>
  </aside>
)

const MobileNavigation = () => (
  <nav
    aria-label="Mobile navigation"
    className="fixed inset-x-0 bottom-0 z-40 flex min-h-16 border-t border-white/10 bg-zinc-950/95 px-[max(0.5rem,env(safe-area-inset-left))] pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
  >
    {primaryRoutes.map((route) => (
      <PrimaryLink key={route.path} route={route} compact />
    ))}
  </nav>
)

const RouteContent = () => {
  const location = useLocation()
  const content = <Outlet />
  return location.pathname.startsWith("/timeline") ? (
    <EntriesProvider>{content}</EntriesProvider>
  ) : (
    content
  )
}

export function TantanAppShell() {
  const mobile = useTantanMobile()
  const location = useLocation()
  const detailRoute =
    location.pathname.startsWith("/entries/") || location.pathname.startsWith("/sources/")

  return (
    <div ref={handleRootRef} className="relative flex h-dvh min-h-0 bg-zinc-950 text-zinc-100">
      {!mobile && <DesktopNavigation />}
      <LocalServiceGuard>
        <main
          ref={setMainContainerElement}
          tabIndex={-1}
          className={cn(
            "min-w-0 flex-1 overflow-auto bg-zinc-950 outline-none md:pb-0",
            !detailRoute && "pb-[calc(4rem+env(safe-area-inset-bottom))]",
          )}
        >
          <RouteContent />
        </main>
      </LocalServiceGuard>
      {mobile && !detailRoute && <MobileNavigation />}
    </div>
  )
}

export const TantanShellPage = ({ children }: PropsWithChildren) => (
  <section className="mx-auto min-h-full w-full max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
    {children}
  </section>
)
