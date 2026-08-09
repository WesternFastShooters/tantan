import { TantanShellPage } from "./TantanAppShell"

export function DiscoverRoute() {
  return (
    <TantanShellPage>
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-[28px] font-bold tracking-tight">发现</h1>
          <p className="mt-1 text-sm text-zinc-500">搜索网站、RSS 和你感兴趣的 Source。</p>
        </div>
        <i className="i-mgc-search-3-cute-re size-7 text-orange-500" aria-hidden />
      </header>
    </TantanShellPage>
  )
}
