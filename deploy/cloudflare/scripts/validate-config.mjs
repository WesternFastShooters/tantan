import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"

import { parse, printParseErrorCode } from "jsonc-parser"

const configPath = fileURLToPath(new URL("../wrangler.jsonc", import.meta.url))
const parseErrors = []
const config = parse(readFileSync(configPath, "utf8"), parseErrors, {
  allowTrailingComma: true,
  disallowComments: false,
})

if (parseErrors.length > 0) {
  const errors = parseErrors.map(({ error, offset }) => `${printParseErrorCode(error)}@${offset}`)
  throw new Error(`wrangler.jsonc: JSONC 格式错误（${errors.join(", ")}）`)
}

const route = config.routes?.[0]?.pattern
const vars = config.vars ?? {}

const required = {
  route,
  TEAM_DOMAIN: vars.TEAM_DOMAIN,
  POLICY_AUD: vars.POLICY_AUD,
  OWNER_EMAIL: vars.OWNER_EMAIL,
  TANTAN_PUBLIC_ORIGIN: vars.TANTAN_PUBLIC_ORIGIN,
  R2_ACCOUNT_ID: vars.R2_ACCOUNT_ID,
  R2_BUCKET_NAME: vars.R2_BUCKET_NAME,
}

for (const [name, value] of Object.entries(required)) {
  if (
    typeof value !== "string" ||
    value.trim() === "" ||
    /REPLACE|YOUR-|example\.com/i.test(value)
  ) {
    throw new Error(`wrangler.jsonc: ${name} 仍是占位值`)
  }
}

if (vars.TANTAN_PUBLIC_ORIGIN !== `https://${route}`) {
  throw new Error("wrangler.jsonc: TANTAN_PUBLIC_ORIGIN 必须与自定义域名完全一致")
}

if (!/^https:\/\/[^/]+\.cloudflareaccess\.com$/.test(vars.TEAM_DOMAIN)) {
  throw new Error("wrangler.jsonc: TEAM_DOMAIN 必须是 Cloudflare Access 团队域名")
}

if (!/^[^@\s]+@[^\s@][^\s.@]*\.[^\s@]+$/.test(vars.OWNER_EMAIL)) {
  throw new Error("wrangler.jsonc: OWNER_EMAIL 必须是有效邮箱")
}

process.stdout.write("Cloudflare 非敏感配置校验通过\n")
