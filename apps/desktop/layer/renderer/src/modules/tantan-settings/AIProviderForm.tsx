import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"

import type { AIProviderId, AIProviderTestRequest } from "~/lib/tantan-api/gen/types"

import { deleteAIProvider, getAIProvider, saveAIProvider, testAIProvider } from "./api"
import type { ProviderDraft } from "./provider-form"
import { buildProviderSaveRequest, PROVIDER_PRESETS, validateProviderDraft } from "./provider-form"

const initialDraft: ProviderDraft = { providerId: "openai", model: "", apiKey: "" }

export function AIProviderForm() {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ProviderDraft>(initialDraft)
  const [formError, setFormError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const providerQuery = useQuery({
    queryKey: ["settings", "ai-provider"],
    queryFn: ({ signal }) => getAIProvider(signal),
  })

  useEffect(() => {
    const data = providerQuery.data
    if (!data) return
    setDraft((current) => ({
      providerId: data.providerId ?? current.providerId,
      model: data.model ?? current.model,
      apiKey: "",
    }))
  }, [providerQuery.data])

  const saveMutation = useMutation({
    mutationFn: saveAIProvider,
    onSuccess: (data) => {
      queryClient.setQueryData(["settings", "ai-provider"], data)
      setDraft((current) => ({ ...current, apiKey: "" }))
      setStatus("配置已保存到本机 Keychain")
    },
  })
  const testMutation = useMutation({
    mutationFn: testAIProvider,
    onSuccess: (data) => setStatus(`连接成功 · ${data.latencyMs}ms`),
  })
  const deleteMutation = useMutation({
    mutationFn: deleteAIProvider,
    onSuccess: () => {
      queryClient.setQueryData(["settings", "ai-provider"], {
        configured: false,
        providerId: null,
        model: null,
        baseUrl: null,
        hasApiKey: false,
        keyFingerprint: null,
      })
      setDraft(initialDraft)
      setStatus("本机 AI 配置已删除")
    },
  })
  const pending = saveMutation.isPending || testMutation.isPending || deleteMutation.isPending
  const requestError = saveMutation.error || testMutation.error || deleteMutation.error

  const validate = (forTest: boolean) => {
    const error = validateProviderDraft(
      draft,
      forTest ? false : Boolean(providerQuery.data?.hasApiKey),
    )
    setFormError(error)
    setStatus(null)
    return !error
  }

  const save = () => {
    if (!validate(false)) return
    saveMutation.mutate(buildProviderSaveRequest(draft))
  }

  const testConnection = () => {
    if (!validate(true)) return
    testMutation.mutate({
      providerId: draft.providerId,
      model: draft.model.trim(),
      apiKey: draft.apiKey,
    } satisfies AIProviderTestRequest)
  }

  if (providerQuery.isPending)
    return <div aria-busy="true" className="h-56 animate-pulse rounded-xl bg-[#17181b]" />

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        save()
      }}
      autoComplete="off"
      className="space-y-5 rounded-2xl border border-white/[0.06] bg-[#17181b] p-4 sm:p-6"
    >
      <div>
        <label htmlFor="ai-provider" className="mb-1.5 block text-sm font-medium text-zinc-200">
          Provider
        </label>
        <select
          id="ai-provider"
          value={draft.providerId}
          disabled={pending}
          onChange={(event) =>
            setDraft((current) => ({ ...current, providerId: event.target.value as AIProviderId }))
          }
          className="h-11 w-full rounded-xl border border-white/10 bg-zinc-950 px-3 text-sm text-zinc-100 outline-none focus:border-orange-500 focus:ring-1 focus:ring-orange-500"
        >
          {Object.entries(PROVIDER_PRESETS).map(([id, preset]) => (
            <option key={id} value={id}>
              {preset.label}
            </option>
          ))}
        </select>
      </div>
      <div>
        <label htmlFor="ai-endpoint" className="mb-1.5 block text-sm font-medium text-zinc-200">
          内置 Endpoint
        </label>
        <input
          id="ai-endpoint"
          readOnly
          value={PROVIDER_PRESETS[draft.providerId].baseUrl}
          className="h-11 w-full rounded-xl border border-white/5 bg-black/20 px-3 text-sm text-zinc-500 outline-none"
        />
        <p className="mt-1 text-xs text-zinc-500">为防止密钥被转发到未知地址，此项不可编辑。</p>
      </div>
      <div>
        <label htmlFor="ai-model" className="mb-1.5 block text-sm font-medium text-zinc-200">
          模型
        </label>
        <input
          id="ai-model"
          value={draft.model}
          disabled={pending}
          maxLength={100}
          onChange={(event) => setDraft((current) => ({ ...current, model: event.target.value }))}
          placeholder="例如 gpt-5-mini"
          className="h-11 w-full rounded-xl border border-white/10 bg-zinc-950 px-3 text-sm text-zinc-100 outline-none focus:border-orange-500 focus:ring-1 focus:ring-orange-500"
        />
      </div>
      <div>
        <label htmlFor="ai-api-key" className="mb-1.5 block text-sm font-medium text-zinc-200">
          API Key
        </label>
        <input
          id="ai-api-key"
          name="tantan-provider-key"
          type="password"
          value={draft.apiKey}
          disabled={pending}
          autoComplete="new-password"
          maxLength={4096}
          onChange={(event) => setDraft((current) => ({ ...current, apiKey: event.target.value }))}
          placeholder={
            providerQuery.data?.hasApiKey ? "已保存；留空表示保留" : "只会发送给本机 Go 服务"
          }
          className="h-11 w-full rounded-xl border border-white/10 bg-zinc-950 px-3 text-sm text-zinc-100 outline-none focus:border-orange-500 focus:ring-1 focus:ring-orange-500"
        />
        {providerQuery.data?.keyFingerprint && (
          <p className="mt-1 text-xs text-zinc-500">
            已保存密钥指纹：{providerQuery.data.keyFingerprint}
          </p>
        )}
      </div>

      {(formError || requestError) && (
        <p role="alert" className="text-sm text-red-300">
          {formError ?? (requestError instanceof Error ? requestError.message : "请求失败")}
        </p>
      )}
      {status && (
        <p role="status" className="text-sm text-emerald-400">
          {status}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={pending}
          onClick={testConnection}
          className="min-h-11 rounded-xl bg-white/10 px-4 text-sm font-medium outline-none hover:bg-white/15 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
        >
          {testMutation.isPending ? "测试中…" : "测试连接"}
        </button>
        <button
          type="submit"
          disabled={pending}
          className="min-h-11 rounded-xl bg-orange-500 px-4 text-sm font-semibold text-white outline-none hover:bg-orange-400 focus-visible:ring-2 focus-visible:ring-orange-300 disabled:opacity-50"
        >
          {saveMutation.isPending ? "保存中…" : "保存配置"}
        </button>
        {providerQuery.data?.configured && (
          <button
            type="button"
            disabled={pending}
            onClick={() => deleteMutation.mutate()}
            className="min-h-11 rounded-xl px-4 text-sm text-red-300 outline-none hover:bg-red-500/10 focus-visible:ring-2 focus-visible:ring-red-400 disabled:opacity-50"
          >
            删除配置
          </button>
        )}
      </div>
    </form>
  )
}
