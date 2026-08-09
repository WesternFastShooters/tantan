import { TANTAN_API_ORIGIN } from "~/lib/tantan-api/client"

export function LoginRoute() {
  const startLogin = () => window.location.assign(new URL("/auth/folo/start", TANTAN_API_ORIGIN))

  return (
    <main className="flex min-h-dvh items-center justify-center bg-zinc-950 px-5 text-zinc-100">
      <section className="w-full max-w-sm rounded-3xl border border-white/10 bg-zinc-900 p-7 shadow-2xl">
        <div className="mb-6 text-center">
          <h1 className="text-2xl font-bold">登录 Tantan</h1>
          <p className="mt-2 text-sm text-zinc-400">继续使用现有 Folo 账号同步订阅与阅读状态。</p>
        </div>
        <button
          type="button"
          className="flex min-h-11 w-full items-center justify-center rounded-xl bg-red-500 px-4 font-semibold text-white outline-none hover:bg-red-400 focus-visible:ring-2 focus-visible:ring-red-300"
          onClick={startLogin}
        >
          使用 Folo 账号登录
        </button>
      </section>
    </main>
  )
}
