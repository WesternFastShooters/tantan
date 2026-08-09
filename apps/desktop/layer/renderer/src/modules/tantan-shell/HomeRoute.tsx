import { TantanShellPage } from "./TantanAppShell"

export function HomeRoute() {
  return (
    <TantanShellPage>
      <header className="mb-5">
        <h1 className="text-2xl font-bold tracking-tight">今日推荐</h1>
        <p className="mt-1 text-sm text-zinc-400">最近 7 天未读内容将在这里生成每日推荐队列。</p>
      </header>
      <div className="rounded-2xl border border-white/10 bg-zinc-900/60 p-6 text-sm text-zinc-400">
        首页信息流正在初始化；已缓存的订阅内容仍可从订阅页阅读。
      </div>
    </TantanShellPage>
  )
}
