import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router"

import { getLocalSession, logoutTantan } from "~/lib/tantan-api/client"

import { TantanShellPage } from "./TantanAppShell"

type SettingsItem = {
  to: string
  icon: string
  label: string
  description?: string
}

const SettingsGroup = ({ title, items }: { title: string; items: SettingsItem[] }) => (
  <section className="mt-5" data-settings-group>
    <h2 className="mb-2 px-1 text-xs font-medium text-zinc-500">{title}</h2>
    <div className="overflow-hidden rounded-2xl bg-white shadow-sm ring-1 ring-zinc-200/70 dark:bg-[#17181b] dark:ring-white/[0.07]">
      {items.map((item) => (
        <Link
          key={item.to}
          to={item.to}
          className="flex min-h-16 items-center gap-3 border-b border-zinc-100 px-4 py-3 outline-none last:border-b-0 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-orange-500 dark:border-white/[0.06]"
        >
          <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-orange-500/10 text-orange-500">
            <i className={`${item.icon} size-5`} aria-hidden />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium">{item.label}</span>
            {item.description && (
              <span className="mt-0.5 block truncate text-xs text-zinc-500">
                {item.description}
              </span>
            )}
          </span>
          <i className="i-mgc-right-cute-re size-5 text-zinc-400" aria-hidden />
        </Link>
      ))}
    </div>
  </section>
)

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
      <section className="mt-5 flex items-center gap-3 rounded-2xl bg-white p-4 shadow-sm ring-1 ring-zinc-200/70 dark:bg-[#17181b] dark:ring-white/[0.07]">
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
      <SettingsGroup
        title="偏好设置"
        items={[
          {
            to: "/settings/general",
            icon: "i-mgc-settings-7-cute-re",
            label: "通用",
            description: "阅读、订阅和服务端 AI 行为",
          },
          {
            to: "/settings/appearance",
            icon: "i-mgc-palette-cute-re",
            label: "外观",
            description: "主题、字号和动态效果",
          },
        ]}
      />
      <SettingsGroup
        title="Tantan 服务"
        items={[
          {
            to: "/settings/ai",
            icon: "i-mgc-sparkles-2-cute-re",
            label: "服务端 AI",
            description: "Gemini 预设与服务端密钥状态",
          },
          {
            to: "/settings/topics",
            icon: "i-mgc-tag-cute-re",
            label: "频道管理",
            description: "固定、隐藏和调整首页 Topic",
          },
          {
            to: "/settings/about",
            icon: "i-mgc-information-cute-re",
            label: "关于 Tantan",
          },
        ]}
      />
    </TantanShellPage>
  )
}
