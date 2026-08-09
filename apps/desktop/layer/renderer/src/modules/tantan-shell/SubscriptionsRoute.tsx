import { Link } from "react-router"

import { TantanShellPage } from "./TantanAppShell"

export function SubscriptionsRoute() {
  return (
    <TantanShellPage>
      <h1 className="text-2xl font-bold tracking-tight">订阅</h1>
      <p className="mt-2 text-sm text-zinc-400">管理 RSS Source，并阅读历史内容。</p>
      <Link
        className="mt-6 inline-flex min-h-11 items-center rounded-xl bg-red-500 px-4 text-sm font-semibold text-white outline-none hover:bg-red-400 focus-visible:ring-2 focus-visible:ring-red-300"
        to="/timeline/view-0/pending/pending"
      >
        打开订阅阅读器
      </Link>
    </TantanShellPage>
  )
}
