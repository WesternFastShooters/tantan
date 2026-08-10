import { describe, expect, test } from "vitest"

import { createContainerRequest, isOwner } from "./access"

describe("Cloudflare Access gateway", () => {
  test("allows only the configured owner email", () => {
    expect(isOwner({ email: "Owner@Example.com" }, "owner@example.com")).toBe(true)
    expect(isOwner({ email: "attacker@example.com" }, "owner@example.com")).toBe(false)
    expect(isOwner({}, "owner@example.com")).toBe(false)
  })

  test("overwrites spoofable gateway headers before container dispatch", async () => {
    const request = new Request("https://tantan.example.com/api/tantan/v1/session", {
      headers: {
        "Cf-Access-Authenticated-User-Email": "attacker@example.com",
        "Cf-Access-Jwt-Assertion": "access-jwt-CANARY",
        "X-Forwarded-For": "attacker-CANARY",
        "X-Tantan-Authenticated-Owner": "attacker",
        "X-Tantan-Gateway-Secret": "attacker-secret",
      },
    })
    const proxied = createContainerRequest(request, "cloudflare-owner", "server-secret")

    expect(proxied.headers.get("Cf-Access-Jwt-Assertion")).toBeNull()
    expect(proxied.headers.get("Cf-Access-Authenticated-User-Email")).toBeNull()
    expect(proxied.headers.get("X-Forwarded-For")).toBeNull()
    expect(proxied.headers.get("X-Tantan-Authenticated-Owner")).toBe("cloudflare-owner")
    expect(proxied.headers.get("X-Tantan-Gateway-Secret")).toBe("server-secret")
  })
})
