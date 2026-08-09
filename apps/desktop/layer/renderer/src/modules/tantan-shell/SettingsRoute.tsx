import { Link } from "react-router"

import { TantanShellPage } from "./TantanAppShell"

export function SettingsRoute() {
  return (
    <TantanShellPage>
      <h1 className="text-2xl font-bold tracking-tight">设置</h1>
      <p className="mt-2 text-sm text-zinc-400">AI Provider、Topic 和阅读偏好将在本地保存。</p>
      <div className="mt-6 grid gap-3 sm:grid-cols-2">
        <Link
          to="/settings/ai"
          className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4 outline-none hover:border-orange-500/40 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-sparkles-2-cute-re size-5 text-orange-400" aria-hidden />
          <h2 className="mt-3 font-semibold text-zinc-100">本地 AI</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500">
            配置自己的 Provider、模型和 Keychain 密钥。
          </p>
        </Link>
        <Link
          to="/settings/topics"
          className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4 outline-none hover:border-orange-500/40 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-tag-cute-re size-5 text-orange-400" aria-hidden />
          <h2 className="mt-3 font-semibold text-zinc-100">频道管理</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500">固定、隐藏和调整首页 Topic 顺序。</p>
        </Link>
        <Link
          to="/settings/appearance"
          className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4 outline-none hover:border-orange-500/40 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-palette-cute-re size-5 text-zinc-400" aria-hidden />
          <h2 className="mt-3 font-semibold text-zinc-100">外观</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500">沿用 Folo 的本地外观设置。</p>
        </Link>
        <Link
          to="/settings/general"
          className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4 outline-none hover:border-orange-500/40 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-settings-7-cute-re size-5 text-zinc-400" aria-hidden />
          <h2 className="mt-3 font-semibold text-zinc-100">通用</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500">阅读、启动与本地数据偏好。</p>
        </Link>
      </div>
    </TantanShellPage>
  )
}
