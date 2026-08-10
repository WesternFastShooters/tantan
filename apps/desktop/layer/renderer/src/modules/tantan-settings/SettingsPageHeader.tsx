import type { PropsWithChildren } from "react"
import { Link } from "react-router"

export function SettingsPageHeader({ children }: PropsWithChildren) {
  return (
    <header className="mb-5 flex min-h-14 items-center gap-2">
      <Link
        to="/settings"
        aria-label="返回设置"
        className="flex size-11 items-center justify-center rounded-full text-zinc-300 outline-none hover:bg-white/10 focus-visible:ring-2 focus-visible:ring-orange-500"
      >
        <i className="i-mgc-arrow-left-cute-re size-5" aria-hidden />
      </Link>
      <h1 className="text-xl font-bold tracking-tight text-zinc-50">{children}</h1>
    </header>
  )
}
