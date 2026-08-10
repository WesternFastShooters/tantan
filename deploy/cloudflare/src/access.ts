import type { JWTPayload } from "jose"

const protectedHeaders = [
  "cf-access-jwt-assertion",
  "forwarded",
  "x-forwarded-for",
  "x-forwarded-host",
  "x-forwarded-proto",
  "x-real-ip",
  "x-tantan-authenticated-owner",
  "x-tantan-gateway-secret",
]

export function isOwner(payload: JWTPayload, ownerEmail: string): boolean {
  return (
    typeof payload.email === "string" &&
    payload.email.trim().toLowerCase() === ownerEmail.trim().toLowerCase()
  )
}

export function createContainerRequest(
  request: Request,
  ownerAccessID: string,
  gatewaySecret: string,
): Request {
  const headers = new Headers(request.headers)
  for (const name of [...headers.keys()]) {
    if (name.startsWith("cf-access-") || name.startsWith("x-forwarded-")) {
      headers.delete(name)
    }
  }
  for (const name of protectedHeaders) {
    headers.delete(name)
  }
  headers.set("X-Tantan-Authenticated-Owner", ownerAccessID)
  headers.set("X-Tantan-Gateway-Secret", gatewaySecret)
  return new Request(request, { headers })
}
