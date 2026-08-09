const removedFoloRoute =
  /^\/(?:ai|wallets|payments|referrals|trending)(?:\/|$)|^\/better-auth\/(?:subscription|stripe)(?:\/|$)|^\/rsshub\/use$/

export const isRemovedFoloRoute = (url: URL, apiOrigin: string) =>
  url.origin === apiOrigin && removedFoloRoute.test(url.pathname)

export const removedFoloResponse = () =>
  new Response(
    JSON.stringify({
      error: {
        code: "FOLO_FEATURE_REMOVED",
        message: "该 Folo 产品功能已从 Tantan 移除",
      },
    }),
    {
      status: 410,
      headers: {
        "Cache-Control": "no-store",
        "Content-Type": "application/json",
      },
    },
  )
