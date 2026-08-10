import { Container } from "@cloudflare/containers"
import type { JWTPayload } from "jose"
import { createRemoteJWKSet, jwtVerify } from "jose"

import { createContainerRequest, isOwner } from "./access"

interface Env {
  TANTAN_CONTAINER: DurableObjectNamespace<TantanContainer>
  TEAM_DOMAIN: string
  POLICY_AUD: string
  OWNER_EMAIL: string
  TANTAN_PUBLIC_ORIGIN: string
  R2_ACCOUNT_ID: string
  R2_BUCKET_NAME: string
  TANTAN_MASTER_KEY_B64: string
  TANTAN_GEMINI_API_KEY: string
  TANTAN_GATEWAY_SECRET: string
  R2_ACCESS_KEY_ID: string
  R2_SECRET_ACCESS_KEY: string
}

const ownerAccessID = "cloudflare-access-owner"
const containerName = "tantan-owner"
const remoteKeySets = new Map<string, ReturnType<typeof createRemoteJWKSet>>()

export class TantanContainer extends Container<Env> {
  defaultPort = 8080

  envVars = {
    HOME: "/var/lib/tantan",
    TZ: "Asia/Shanghai",
    TANTAN_DATA_DIR: "/var/lib/tantan",
    TANTAN_STATIC_DIR: "/app/static",
    TANTAN_LISTEN_ADDR: "0.0.0.0:8080",
    TANTAN_CLOUDFLARE_CONTAINER: "true",
    TANTAN_SINGLE_USER: "true",
    TANTAN_OWNER_ACCESS_ID: ownerAccessID,
    TANTAN_PUBLIC_ORIGIN: this.env.TANTAN_PUBLIC_ORIGIN,
    TANTAN_GATEWAY_SECRET: this.env.TANTAN_GATEWAY_SECRET,
    TANTAN_MASTER_KEY_B64: this.env.TANTAN_MASTER_KEY_B64,
    TANTAN_GEMINI_API_KEY: this.env.TANTAN_GEMINI_API_KEY,
    TANTAN_REPLICA_BUCKET: this.env.R2_BUCKET_NAME,
    TANTAN_REPLICA_PATH: "single-user/tantan.sqlite",
    TANTAN_REPLICA_ENDPOINT: `https://${this.env.R2_ACCOUNT_ID}.r2.cloudflarestorage.com`,
    AWS_ACCESS_KEY_ID: this.env.R2_ACCESS_KEY_ID,
    AWS_SECRET_ACCESS_KEY: this.env.R2_SECRET_ACCESS_KEY,
  }

  override onError(error: unknown) {
    console.error("tantan_container_error", error instanceof Error ? error.name : "unknown")
  }
}

async function verifyAccess(request: Request, env: Env): Promise<JWTPayload | null> {
  if (!env.TEAM_DOMAIN.startsWith("https://") || !env.POLICY_AUD || !env.OWNER_EMAIL) {
    return null
  }
  const assertion = request.headers.get("Cf-Access-Jwt-Assertion")
  if (!assertion) {
    return null
  }
  let keySet = remoteKeySets.get(env.TEAM_DOMAIN)
  if (!keySet) {
    keySet = createRemoteJWKSet(new URL(`${env.TEAM_DOMAIN}/cdn-cgi/access/certs`))
    remoteKeySets.set(env.TEAM_DOMAIN, keySet)
  }
  try {
    const { payload } = await jwtVerify(assertion, keySet, {
      issuer: env.TEAM_DOMAIN,
      audience: env.POLICY_AUD,
      algorithms: ["RS256"],
    })
    return payload
  } catch {
    return null
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const payload = await verifyAccess(request, env)
    if (!payload || !isOwner(payload, env.OWNER_EMAIL)) {
      return new Response("Forbidden", {
        status: 403,
        headers: { "Cache-Control": "no-store", "Content-Type": "text/plain; charset=utf-8" },
      })
    }
    const container = env.TANTAN_CONTAINER.getByName(containerName)
    return container.fetch(
      createContainerRequest(request, ownerAccessID, env.TANTAN_GATEWAY_SECRET),
    )
  },
} satisfies ExportedHandler<Env>
