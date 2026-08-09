import { useQuery, useQueryClient } from "@tanstack/react-query"
import type { PropsWithChildren } from "react"
import { useEffect } from "react"
import { useNavigate } from "react-router"

import { getLocalSession, getReadiness, TantanAPIError } from "~/lib/tantan-api/client"

const readinessQueryKey = ["tantan", "readiness"] as const
const sessionQueryKey = ["tantan", "session"] as const

export function LocalServiceGuard({ children }: PropsWithChildren) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const readiness = useQuery({
    queryKey: readinessQueryKey,
    queryFn: ({ signal }) => getReadiness(signal),
    retry: 1,
    retryDelay: 500,
    refetchInterval: 30_000,
    staleTime: 10_000,
  })
  const session = useQuery({
    queryKey: sessionQueryKey,
    queryFn: ({ signal }) => getLocalSession(signal),
    enabled: readiness.data?.ready === true,
    retry: false,
    staleTime: 30_000,
  })

  useEffect(() => {
    if (!(session.error instanceof TantanAPIError) || session.error.status !== 401) return
    queryClient.clear()
    void navigate("/login", { replace: true })
  }, [navigate, queryClient, session.error])

  const unavailable = readiness.isError || readiness.data?.ready === false

  return (
    <>
      {unavailable && (
        <div
          role="alert"
          className="absolute inset-x-3 top-3 z-50 flex min-h-11 items-center justify-between gap-3 rounded-xl border border-orange-400/40 bg-zinc-950/95 px-4 py-2 text-sm text-orange-100 shadow-xl md:left-auto md:max-w-md"
        >
          <span>本地服务未启动，正在展示已有内容</span>
          <button
            type="button"
            className="min-h-11 shrink-0 rounded-lg px-3 font-medium text-orange-200 outline-none hover:bg-orange-400/10 focus-visible:ring-2 focus-visible:ring-orange-300"
            onClick={() => void readiness.refetch()}
          >
            重试
          </button>
        </div>
      )}
      {children}
    </>
  )
}
