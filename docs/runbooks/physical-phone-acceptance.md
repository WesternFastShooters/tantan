# Physical-phone HTTPS acceptance

Run this only against the final production build behind the intended HTTPS reverse proxy. The Go
process remains on `127.0.0.1:3000`; do not expose that port directly. Use a newly rotated Gemini
Key in the server Secret file/manager, never the value posted in chat.

Record device, OS/browser version, HTTPS URL, build commit and time. Then complete every row on a
physical iPhone Safari or Android Chrome equivalent.

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
