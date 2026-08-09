/// <reference lib="webworker" />
import { CacheableResponsePlugin } from "workbox-cacheable-response"
import { ExpirationPlugin } from "workbox-expiration"
import { createHandlerBoundToURL, precacheAndRoute } from "workbox-precaching"
import { NavigationRoute, registerRoute } from "workbox-routing"
import { CacheFirst } from "workbox-strategies"

declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<{ url: string; revision?: string }>
}

precacheAndRoute(self.__WB_MANIFEST)

const isSensitiveRequest = (url: URL) =>
  url.pathname === "/auth" ||
  url.pathname.startsWith("/auth/") ||
  url.pathname === "/tantan/v1" ||
  url.pathname.startsWith("/tantan/v1/")

registerRoute(
  new NavigationRoute(createHandlerBoundToURL("index.html"), {
    denylist: [/^\/auth(?:\/|$)/, /^\/tantan\/v1(?:\/|$)/],
  }),
)

registerRoute(
  ({ request, url }) => request.destination === "image" && !isSensitiveRequest(url),
  new CacheFirst({
    cacheName: "image-assets",
    plugins: [
      new CacheableResponsePlugin({
        statuses: [0, 200],
      }),
      new ExpirationPlugin({
        maxEntries: 100,
        maxAgeSeconds: 10 * 24 * 60 * 60,
        purgeOnQuotaError: true,
      }),
    ],
  }),
)
