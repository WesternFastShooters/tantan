import { useMutation, useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { getAIProvider, testAIProvider } from "./api"

const fixedProvider = "Google Gemini"
const fixedModel = "gemini-3.5-flash-lite"
const fixedEndpoint = "https://generativelanguage.googleapis.com/v1beta/openai"

export function AIProviderForm() {
  const [status, setStatus] = useState<string | null>(null)
  const providerQuery = useQuery({
    queryKey: ["settings", "ai-provider"],
    queryFn: ({ signal }) => getAIProvider(signal),
  })
  const testMutation = useMutation({
    mutationFn: testAIProvider,
    onMutate: () => setStatus(null),
    onSuccess: (data) => setStatus(`连接成功 · ${data.latencyMs}ms · ${data.model}`),
  })

  if (providerQuery.isPending) {
    return <div aria-busy="true" className="h-56 animate-pulse rounded-xl bg-[#17181b]" />
  }

  const configuration = providerQuery.data
  const requestError = providerQuery.error || testMutation.error

  return (
    <section className="space-y-5 rounded-2xl border border-white/[0.06] bg-[#17181b] p-4 sm:p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="font-semibold text-zinc-100">服务端 AI</h2>
          <p className="mt-1 text-sm leading-6 text-zinc-400">
            Provider、Endpoint、模型和 API Key 均由 Go 服务端私密配置。
          </p>
        </div>
        <span
          className={
            configuration?.configured
              ? "shrink-0 rounded-full bg-emerald-500/10 px-2.5 py-1 text-xs text-emerald-300"
              : "shrink-0 rounded-full bg-amber-500/10 px-2.5 py-1 text-xs text-amber-200"
          }
        >
          {configuration?.configured ? "已配置" : "未配置"}
        </span>
      </div>

      <dl className="divide-y divide-white/[0.06] rounded-xl border border-white/[0.06] bg-black/15 px-4">
        <div className="flex min-h-12 items-center justify-between gap-4 py-2">
          <dt className="text-sm text-zinc-500">Provider</dt>
          <dd className="text-right text-sm text-zinc-200">{fixedProvider}</dd>
        </div>
        <div className="flex min-h-12 items-center justify-between gap-4 py-2">
          <dt className="text-sm text-zinc-500">模型</dt>
          <dd className="break-all text-right text-sm text-zinc-200">
            {configuration?.model || fixedModel}
          </dd>
        </div>
        <div className="flex min-h-12 items-center justify-between gap-4 py-2">
          <dt className="text-sm text-zinc-500">Endpoint</dt>
          <dd className="max-w-[70%] break-all text-right text-xs text-zinc-400">
            {configuration?.baseUrl || fixedEndpoint}
          </dd>
        </div>
        <div className="flex min-h-12 items-center justify-between gap-4 py-2">
          <dt className="text-sm text-zinc-500">API Key</dt>
          <dd className="text-right text-sm text-zinc-300">
            {configuration?.hasApiKey ? "已在服务端安全加载" : "尚未加载"}
          </dd>
        </div>
      </dl>

      {requestError && (
        <p role="alert" className="text-sm text-red-300">
          {requestError instanceof Error ? requestError.message : "请求失败"}
        </p>
      )}
      {status && (
        <p role="status" className="text-sm text-emerald-400">
          {status}
        </p>
      )}
      <button
        type="button"
        disabled={!configuration?.configured || testMutation.isPending}
        onClick={() => testMutation.mutate()}
        className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
      >
        {testMutation.isPending ? "测试中…" : "测试连接"}
      </button>
      <p className="text-xs leading-5 text-zinc-500">
        浏览器不能查看、提交、修改或删除 API Key；更换密钥请更新 Go 服务的私密配置后重启。
      </p>
    </section>
  )
}
