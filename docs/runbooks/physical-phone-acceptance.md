# Physical-phone HTTPS acceptance

Run this only against the final production build behind the intended HTTPS reverse proxy. The Go
process remains on `127.0.0.1:3000`; do not expose that port directly. Use a newly rotated Gemini
Key in the server Secret file/manager, never the value posted in chat.

Record device, OS/browser version, HTTPS URL, build commit and time. Then complete every row on a
physical iPhone Safari or Android Chrome equivalent.

## Local-LAN HTTPS path

Use this only while the Mac and phone are on the same trusted Wi-Fi. It does not expose Tantan to
the public Internet: Vite terminates local TLS and proxies same-origin `/api`; Go still binds only
`127.0.0.1:3000`.

1. Run `pnpm build:web`.
2. Start Go with `TANTAN_PUBLIC_ORIGIN=https://<LAN-IP>:2443` and the production
   `TANTAN_STATIC_DIR`.
3. Start `SSL=1 WEB_BUILD=1 pnpm --dir apps/desktop exec vite preview --host 0.0.0.0 --port 2443 --strictPort`.
4. Transfer only `$(mkcert -CAROOT)/rootCA.pem` to the phone. Never transfer
   `rootCA-key.pem`. Install and explicitly trust the temporary CA, then open
   `https://<LAN-IP>:2443`.
5. After acceptance, stop both processes and remove the temporary CA profile/trust from the phone.

The operator must not use a public tunnel for this checklist. If the LAN IP changes, restart both
processes with the new origin and regenerate/retrust the certificate if necessary.

| Step | Action                                              | Pass observation                                             | Status  |
| ---- | --------------------------------------------------- | ------------------------------------------------------------ | ------- |
| 1    | Open HTTPS URL and add to Home Screen               | Tantan icon/name; standalone launch works                    | PENDING |
| 2    | Open protected route while logged out               | Google/GitHub/Apple/Email/token methods; returnTo kept       | PENDING |
| 3    | Complete one real Folo login                        | Only Tantan session is browser-visible; Home opens           | PENDING |
| 4    | Scroll Home and switch Topic                        | Fixed two columns; no duplicate/jump; Topic state restores   | PENDING |
| 5    | Use normal search and return                        | `/search`; Home Filter/Topic/scroll unchanged                | PENDING |
| 6    | Open/cancel/apply AI Filter                         | Cancel has no effect; apply atomically changes Home/Topics   | PENDING |
| 7    | Open detail, translate/summary, favorite and return | Original remains readable; read removes Home card            | PENDING |
| 8    | View/add/remove RSS subscription                    | State matches Folo after success/failure                     | PENDING |
| 9    | Discover a Source and subscribe                     | Result appears and subscription is usable                    | PENDING |
| 10   | Visit every Settings destination                    | No Plan/Power/Wallet/Upgrade/Folo AI Chat                    | PENDING |
| 11   | Close and relaunch installed PWA                    | Session and server-backed state recover                      | PENDING |
| 12   | Temporarily interrupt network                       | Cached shell remains; no authenticated API response in cache | PENDING |

Capture screenshots for steps 1, 3, 4, 6, 7, 10 and 11. Export a HAR with response bodies omitted
and run the release security scanner before retaining it. Any direct browser request to Folo or
Gemini, any paid entry, or any Secret match is a release blocker.
