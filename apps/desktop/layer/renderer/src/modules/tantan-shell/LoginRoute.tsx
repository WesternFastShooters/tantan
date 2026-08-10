import { useEffect, useRef, useState } from "react"
import { useLocation, useNavigate } from "react-router"

import {
  getFoloAuthProviders,
  getLocalSession,
  isFoloTwoFactorChallenge,
  signInWithFoloEmail,
  signInWithFoloToken,
  startFoloSocialLogin,
  verifyFoloTwoFactor,
} from "~/lib/tantan-api/client"
import type { FoloAuthProvider, SessionResponse } from "~/lib/tantan-api/gen/types"

type LoginMode = "providers" | "email" | "token" | "two-factor"
type SocialProvider = Extract<FoloAuthProvider, "google" | "github" | "apple">

const providerLabels: Record<FoloAuthProvider, string> = {
  google: "使用 Google 继续",
  github: "使用 GitHub 继续",
  apple: "使用 Apple 继续",
  credential: "使用 Email 继续",
  token: "输入授权令牌继续",
}

const providerIcons: Record<FoloAuthProvider, string> = {
  google: "i-mgc-google-cute-fi",
  github: "i-mgc-github-cute-fi",
  apple: "i-mgc-apple-cute-fi",
  credential: "i-mgc-mail-cute-re",
  token: "i-mgc-key-2-cute-re",
}

const safeReturnTo = (search: string) => {
  const candidate = new URLSearchParams(search).get("returnTo") ?? "/"
  if (
    !candidate.startsWith("/") ||
    candidate.startsWith("//") ||
    candidate.includes("\\") ||
    candidate.includes("://") ||
    /[\r\n\0]/u.test(candidate)
  ) {
    return "/"
  }
  return candidate
}

const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : "登录暂时失败，请重试"

export function LoginRoute() {
  const location = useLocation()
  const navigate = useNavigate()
  const returnTo = safeReturnTo(location.search)
  const setupRequested = new URLSearchParams(location.search).get("setup") === "1"
  const [providers, setProviders] = useState<FoloAuthProvider[]>([])
  const [mode, setMode] = useState<LoginMode>("providers")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [token, setToken] = useState("")
  const [code, setCode] = useState("")
  const [flowId, setFlowId] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const passwordRef = useRef<HTMLInputElement>(null)
  const tokenRef = useRef<HTMLInputElement>(null)
  const codeRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const controller = new AbortController()
    const initialize = async () => {
      if (!setupRequested) {
        try {
          await getLocalSession(controller.signal)
          if (!controller.signal.aborted) void navigate(returnTo, { replace: true })
          return
        } catch {
          if (controller.signal.aborted) return
        }
      }
      try {
        const response = await getFoloAuthProviders(controller.signal)
        if (controller.signal.aborted) return
        setProviders(response.providers)
        if (response.providers.length === 1 && response.providers[0] === "token") {
          setMode("token")
        }
      } catch (reason) {
        if (!controller.signal.aborted) setError(errorMessage(reason))
      }
    }
    void initialize()
    return () => controller.abort()
  }, [navigate, returnTo, setupRequested])

  const finish = (_session: SessionResponse) => {
    setError(null)
    void navigate(returnTo, { replace: true })
  }

  const clearPassword = () => {
    setPassword("")
    if (passwordRef.current) passwordRef.current.value = ""
  }

  const selectProvider = async (provider: FoloAuthProvider) => {
    setError(null)
    if (provider === "credential") {
      setMode("email")
      return
    }
    if (provider === "token") {
      setMode("token")
      return
    }

    setPending(true)
    try {
      const result = await startFoloSocialLogin({ provider: provider as SocialProvider })
      window.open(result.authorizeUrl, "_blank", "noopener,noreferrer")
      setMode("token")
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setPending(false)
    }
  }

  const submitEmail = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPending(true)
    setError(null)
    try {
      finish(await signInWithFoloEmail({ email: email.trim(), password, returnTo }))
    } catch (reason) {
      if (isFoloTwoFactorChallenge(reason)) {
        setFlowId(reason.response.challenge.flowId)
        setMode("two-factor")
      } else {
        setError(errorMessage(reason))
      }
    } finally {
      clearPassword()
      setPending(false)
    }
  }

  const submitTwoFactor = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!flowId) return
    setPending(true)
    setError(null)
    try {
      finish(await verifyFoloTwoFactor({ flowId, code: code.trim() }))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setCode("")
      if (codeRef.current) codeRef.current.value = ""
      setPending(false)
    }
  }

  const submitToken = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPending(true)
    setError(null)
    try {
      finish(await signInWithFoloToken({ token: token.trim(), returnTo }))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setToken("")
      if (tokenRef.current) tokenRef.current.value = ""
      setPending(false)
    }
  }

  const reset = () => {
    clearPassword()
    setToken("")
    setCode("")
    setFlowId(null)
    setError(null)
    setMode("providers")
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-[#121212] px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-[max(1.25rem,env(safe-area-inset-top))] text-zinc-100">
      <section className="w-full max-w-sm">
        <div className="mb-7 text-center">
          <div className="mx-auto mb-5 flex size-16 items-center justify-center rounded-[20px] bg-orange-500 shadow-lg shadow-orange-500/20">
            <i className="i-mgc-rada-cute-fi size-9 text-white" aria-hidden />
          </div>
          <h1 className="text-[28px] font-bold tracking-tight">欢迎使用 Tantan</h1>
          <p className="mt-2 text-sm leading-6 text-zinc-400">使用 Folo 账号同步订阅与阅读状态</p>
        </div>

        {error && (
          <div
            role="alert"
            className="mb-3 rounded-xl bg-red-500/10 px-4 py-3 text-sm text-red-300"
          >
            {error}
          </div>
        )}

        {mode === "providers" && (
          <div className="space-y-3" aria-label="Folo 登录方式">
            {providers.map((provider) => (
              <button
                key={provider}
                type="button"
                disabled={pending}
                className="flex min-h-12 w-full items-center justify-center gap-3 rounded-xl border border-white/10 bg-zinc-800/80 px-4 font-semibold text-zinc-100 outline-none transition-colors hover:bg-zinc-700 focus-visible:ring-2 focus-visible:ring-orange-500 disabled:opacity-50"
                onClick={() => void selectProvider(provider)}
              >
                <i className={`${providerIcons[provider]} size-5`} aria-hidden />
                {providerLabels[provider]}
              </button>
            ))}
            {providers.length === 0 && !error && (
              <div className="space-y-3" aria-label="正在加载登录方式">
                {[0, 1, 2, 3].map((item) => (
                  <div key={item} className="h-12 animate-pulse rounded-xl bg-zinc-800" />
                ))}
              </div>
            )}
          </div>
        )}

        {mode === "email" && (
          <form className="space-y-3" onSubmit={submitEmail}>
            <label className="block text-sm text-zinc-300">
              Email
              <input
                name="email"
                type="email"
                autoComplete="username"
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                className="mt-2 min-h-12 w-full rounded-xl border border-white/10 bg-zinc-900 px-4 text-base outline-none focus:border-orange-500 focus:ring-2 focus:ring-orange-500/30"
              />
            </label>
            <label className="block text-sm text-zinc-300">
              密码
              <input
                ref={passwordRef}
                name="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                className="mt-2 min-h-12 w-full rounded-xl border border-white/10 bg-zinc-900 px-4 text-base outline-none focus:border-orange-500 focus:ring-2 focus:ring-orange-500/30"
              />
            </label>
            <button
              type="submit"
              disabled={pending}
              className="min-h-12 w-full rounded-xl bg-orange-500 px-4 font-semibold text-white disabled:opacity-50"
            >
              {pending ? "登录中…" : "登录"}
            </button>
            <button type="button" className="min-h-11 w-full text-sm text-zinc-400" onClick={reset}>
              返回其他登录方式
            </button>
          </form>
        )}

        {mode === "two-factor" && (
          <form className="space-y-3" onSubmit={submitTwoFactor}>
            <div className="mb-4 text-center">
              <h2 className="text-lg font-semibold">输入两步验证码</h2>
              <p className="mt-1 text-sm text-zinc-400">请输入 Folo 身份验证器中的 6 位验证码</p>
            </div>
            <input
              ref={codeRef}
              name="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              required
              value={code}
              onChange={(event) => setCode(event.target.value.replace(/\D/gu, ""))}
              aria-label="Folo 两步验证码"
              className="min-h-12 w-full rounded-xl border border-white/10 bg-zinc-900 px-4 text-center text-xl tracking-[0.45em] outline-none focus:border-orange-500 focus:ring-2 focus:ring-orange-500/30"
            />
            <button
              type="submit"
              disabled={pending}
              className="min-h-12 w-full rounded-xl bg-orange-500 px-4 font-semibold text-white disabled:opacity-50"
            >
              {pending ? "验证中…" : "验证并登录"}
            </button>
          </form>
        )}

        {mode === "token" && (
          <form className="space-y-3" onSubmit={submitToken}>
            <div className="mb-4 text-center">
              <h2 className="text-lg font-semibold">管理员初始化 / 重新连接</h2>
              <p className="mt-1 text-sm leading-6 text-zinc-400">
                仅在电脑上首次绑定或 Folo
                会话失效时粘贴一次性令牌。绑定完成后，手机会自动进入，不需要令牌。
              </p>
            </div>
            <input
              ref={tokenRef}
              name="token"
              type="password"
              autoComplete="off"
              required
              value={token}
              onChange={(event) => setToken(event.target.value)}
              aria-label="Folo 一次性授权令牌"
              className="min-h-12 w-full rounded-xl border border-white/10 bg-zinc-900 px-4 text-base outline-none focus:border-orange-500 focus:ring-2 focus:ring-orange-500/30"
            />
            <button
              type="submit"
              disabled={pending}
              className="min-h-12 w-full rounded-xl bg-orange-500 px-4 font-semibold text-white disabled:opacity-50"
            >
              {pending ? "连接中…" : "绑定并进入"}
            </button>
            {providers.length > 1 && (
              <button
                type="button"
                className="min-h-11 w-full text-sm text-zinc-400"
                onClick={reset}
              >
                返回其他登录方式
              </button>
            )}
          </form>
        )}
      </section>
    </main>
  )
}
