import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router"

import { getLocalSession, logoutTantan } from "~/lib/tantan-api/client"

import { TantanShellPage } from "./TantanAppShell"

export function SettingsRoute() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const session = useQuery({
    queryKey: ["tantan", "session"],
    queryFn: ({ signal }) => getLocalSession(signal),
    staleTime: 30_000,
  })
  const logout = useMutation({
    mutationFn: logoutTantan,
    onSuccess: () => {
      queryClient.clear()
      void navigate("/login", { replace: true })
    },
  })

  return (
    <TantanShellPage>
      <h1 className="text-2xl font-bold tracking-tight">设置</h1>
      <p className="mt-2 text-sm text-zinc-400">管理服务端 AI 状态、Topic 和阅读偏好。</p>
      <section className="mt-5 flex items-center gap-3 rounded-2xl border border-white/[0.06] bg-[#17181b] p-4">
        {session.data?.user.image ? (
          <img
            src={session.data.user.image}
            alt=""
            className="size-11 shrink-0 rounded-full object-cover"
            referrerPolicy="no-referrer"
          />
        ) : (
          <span className="flex size-11 shrink-0 items-center justify-center rounded-full bg-orange-500 font-semibold text-white">
            {(session.data?.user.name || "F").slice(0, 1)}
          </span>
        )}
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-semibold text-zinc-100">
            {session.data?.user.name || "Folo 账号"}
          </h2>
          <p className="truncate text-xs text-zinc-500">
            {session.data?.user.email || (session.isPending ? "正在加载账号…" : "已连接 Folo")}
          </p>
        </div>
        <button
          type="button"
          disabled={logout.isPending}
          onClick={() => logout.mutate()}
          className="min-h-11 shrink-0 rounded-xl px-3 text-sm text-red-300 outline-none hover:bg-red-500/10 focus-visible:ring-2 focus-visible:ring-red-400 disabled:opacity-50"
        >
          {logout.isPending ? "退出中…" : "退出登录"}
        </button>
      </section>
      {(session.isError || logout.isError) && (
        <p role="alert" className="mt-3 rounded-xl bg-red-500/10 p-3 text-sm text-red-300">
          {logout.error instanceof Error
            ? logout.error.message
            : session.error instanceof Error
              ? session.error.message
              : "账号操作失败"}
        </p>
      )}
      <div className="mt-6 grid gap-3 sm:grid-cols-2">
        <Link
          to="/settings/ai"
          className="rounded-xl border border-white/[0.06] bg-[#17181b] p-4 outline-none hover:border-orange-500/40 focus-visible:ring-2 focus-visible:ring-orange-500"
        >
          <i className="i-mgc-sparkles-2-cute-re size-5 text-orange-400" aria-hidden />
          <h2 className="mt-3 font-semibold text-zinc-100">服务端 AI</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-500">
            查看固定 Gemini 预设和服务端密钥状态。
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
