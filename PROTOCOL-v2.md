# mikan addon protocol, version 2

Every addon that is not a payment method speaks protocol 2: a Telegram bot, admin pages,
anything else that works with the [mikan](https://github.com/drillex100-beep/scilla) panel. Payment
adapters keep [PROTOCOL.md](PROTOCOL.md) (protocol 1) unchanged. Protocol 2 needs panel and
host 0.5.1.0 or later.

- **The panel** keeps everything that touches money, the bot token and access: Telegram
  polling, Stars payments, the Mini App, identities. It gives an addon only what the admin
  approved (permissions) through its **Panel API**.
- **The addon** is a container next to the panel. It keeps no files: its state goes to the
  panel's KV store, which is backed up with the panel.
- **The host** (`mikan`) installs, updates and removes the container and generates its tokens.

## Catalog entry (`adapter.json`)

An addon of this repository lives in `tools/<id>/` and is published as
`ghcr.io/getmikan/tool-<id>`. Its `adapter.json` is the catalog entry without the digest:

```json
{
  "id": "telegram",
  "category": "tools",
  "name": {"ru": "Telegram-бот", "en": "Telegram bot"},
  "description": {"ru": "…", "en": "…"},
  "version": "0.5.1.0-rc.6",
  "protocol": 2,
  "kind": "bot",
  "mode": "client",
  "permissions": ["bot", "users:read", "users:write", "billing:invoice", "events", "kv"],
  "resources": {"mem_mb": 128},
  "image": "ghcr.io/getmikan/tool-telegram",
  "min_panel": "0.5.1.0-rc.6",
  "homepage": "https://github.com/drillex100-beep/scilla"
}
```

| Field | |
|---|---|
| `protocol` | `2`. A tool never speaks 1: older panels would offer it as a payment method. |
| `kind` | `bot`, `ui` or `generic` (required). `payment` is protocol 1 only. |
| `mode` | `http` (the default when absent): the addon serves `/v2/*` to the panel. `client`: it serves nothing and only calls the panel (a bot on a framework); the host checks that its container keeps running 5 s. |
| `permissions` | what the addon asks for, each at most once, from the list below. |
| `resources.mem_mb` | 64 to 256 MiB; absent or 0 is the host's default, 128. |
| `version`, `min_panel` | `x.y.z`, optionally a fourth number (mikan's releases, `0.5.1.0`) and a prerelease (`-rc.6`). A panel older than `min_panel` does not offer the addon; mind that `0.5.1.0` is newer than `0.5.1.0-rc.6`. |

`go run ./cmd/index list` refuses what the panel and the host would drop: an unknown kind,
mode or permission, a permission twice, memory out of range, another protocol, or any of
these fields on a protocol 1 entry. In `index.json` they are written under the same names,
after `protocol`; a payment adapter's entry has none of them.

The catalog entry is the truth for the permissions: the panel shows them to the admin at
install and keeps the approval bound to the version and the set. An update that asks for
more keeps the old version until the admin approves the new set.

### Permissions

| Permission | Gives |
|---|---|
| `bot` | the bot slot: the Bot API proxy and the bot's own calls to the panel. One addon holds the bot, the one the admin picks. |
| `users:read` | users, subscriptions, devices, traffic of the users this addon talks to |
| `users:write` | grant days and traffic, unbind devices, reissue links |
| `billing:invoice` | invoices, trials and free plans through the panel's payment methods |
| `events` | the panel's events |
| `kv` | its own key-value store |
| `ui` | its own pages in the admin panel |

`bot` and `users:write` make an addon nearly as strong as an admin; the panel says so in the
approval dialog.

## Environment

The host gives the container these variables and nothing else:

| Variable | |
|---|---|
| `MIKAN_ADAPTER_LISTEN` | where to listen (mode `http`): a loopback `ip:port` on the host network |
| `MIKAN_ADAPTER_TOKEN` | the panel calls the addon with `Authorization: Bearer <it>` |
| `MIKAN_PANEL_URL` | the Panel API base, e.g. `http://127.0.0.1:41990/panel/v2` |
| `MIKAN_PANEL_TOKEN` | the addon calls the Panel API with `Authorization: Bearer <it>` |
| `MIKAN_BOT_API_URL` | the Bot API proxy, e.g. `http://127.0.0.1:41990` |
| `MIKAN_BOT_TOKEN` | `0:<MIKAN_PANEL_TOKEN>`, a token in Telegram's shape so frameworks accept it |

Both tokens are 48 random alphanumeric characters, kept across updates of the same addon.
The container runs as 65532 with a read-only root file system, `/tmp` on tmpfs (16 MiB), no
capabilities, at most 64 processes, and the memory from `resources.mem_mb`.

## Panel → addon (mode `http`)

Every request carries `Authorization: Bearer <MIKAN_ADAPTER_TOKEN>`; JSON; errors are
`{"code", "message"}` as in protocol 1; the panel waits 40 s.

- `GET /v2/info`: the addon describes itself. The host waits for it after install and checks
  `id` and `protocol`. Fields: `id`, `protocol` (2), `version`, `kind`, `name`,
  `description`, `permissions`, `settings` (the form the panel draws, as in protocol 1 plus
  the types `text`, `code`, `bool`, `select`, `number`), `ui`, `help`. The panel reads it for
  texts, settings and pages; the permissions it trusts are the catalog's.
- `ui: {"entry": "/ui/index.html", "title": {"ru", "en"}}`: the addon's admin pages. With the
  `ui` permission the panel shows them in a sandboxed frame (scripts only, no cookies, no
  storage, no access to the admin API), fetching `GET /ui/<file>` (only GET, at most 8 MiB, no
  redirects; the addon gets `Accept`, `Accept-Language`, `If-None-Match`,
  `If-Modified-Since` and its own token, never the admin's cookie). Only `Content-Type`,
  `Cache-Control`, `ETag` and `Last-Modified` come back.
- `POST /v2/ui/rpc`: a call of the pages. The frame posts `{type: "rpc", id, method, path,
  body}` to its parent; the panel forwards `{"method": "GET|POST|PUT|DELETE", "path": "/…",
  "body": …, "admin": {"lang"}}` and hands the addon's status and JSON (at most 1 MiB) back
  to the frame as they are. Every call but GET goes to the audit log, without its body.

The contract also has `GET /v2/health`, `POST /v2/check` (settings) and `POST /v2/events`
(push delivery); the panel does not call them yet.

## Addon → panel (Panel API)

A listener of its own on loopback (custom images: the bridge gateway), never the public
port. `Authorization: Bearer <MIKAN_PANEL_TOKEN>`; a request with an `Origin` or a foreign
`Host` is refused; bodies up to 1 MiB. Errors are `{"code", "message"}`: `unauthorized`
(401), `not_approved` (403, no approval yet), `forbidden` (403, the permission is missing),
`not_found` (404), `conflict` (409), `quota_exceeded` (507).

| Method | Permission | |
|---|---|---|
| `GET /panel/v2/self` | any | `{"id", "version", "permissions"}`: who the panel takes the addon for |
| `GET /panel/v2/kv/{key}` | `kv` | the value as bytes, `ETag: "<version>"`, `X-Expires-At` when it expires |
| `PUT /panel/v2/kv/{key}` | `kv` | the body is the value (≤ 64 KiB); `?ttl=<seconds>` (≤ 366 days), `?if_absent=1`, `If-Match: "<version>"` → `{"version"}` or 409 |
| `DELETE /panel/v2/kv/{key}` | `kv` | `If-Match` as above |
| `GET /panel/v2/kv?prefix=&after=&limit=` | `kv` | `{"keys": […], "next"}`, up to 1000 per page |
| `GET /panel/v2/events?after=&types=&limit=&wait=` | `events` | `{"events": [{"id", "type", "at", "user_id", "tg_id", "data"}]}` after the cursor (or `after`), up to 500, waiting up to 50 s |
| `POST /panel/v2/events/ack` `{"id"}` | `events` | moves the cursor forward to the last event handled |
| `POST /panel/v2/bot/{method}` | `bot` | the bot backend of the panel, for the addon that holds the bot (the official bot's calls; not a stable API for others) |

Keys are 1 to 256 of `A-Z a-z 0-9 . _ : / -`; an addon keeps at most 5 MiB. Delivery of
events is at least once: dedupe by `id` (a KV key with `if_absent`). Event types:
`payment.paid`, `payment.refunded`, `user.expiring`, `user.expired`, `traffic.90`,
`traffic.100`. The users, offers and billing calls of the contract are not served yet.

## Bot API proxy

The addon that holds the bot runs any Telegram framework with the API base
`MIKAN_BOT_API_URL` and the token `MIKAN_BOT_TOKEN`: `/bot<token>/<method>`. The panel keeps
the real token and the polling; the bot's id comes from `getMe`.

- `getUpdates` reads what the panel buffered for the addon: `offset` acknowledges what is
  below it (0 or absent acknowledges nothing), `limit` 1 to 100, `timeout` up to 50 s (0 or
  absent answers at once). The buffer holds at most 2000 updates for 24 h. Payments
  (`pre_checkout_query`, `successful_payment`, refunds) never reach it: the panel handles
  them and emits `payment.paid` and `payment.refunded`.
- `deleteWebhook` is a no-op answering `true`, so frameworks start; with
  `drop_pending_updates` it clears the buffer.
- Allowed, with Telegram's own answer passed back: `getMe`, `getChat`, `getChatMember`,
  `answerCallbackQuery`, `setMyCommands`, `getMyCommands`, `deleteMyCommands`,
  `setChatMenuButton`, `getChatMenuButton` at once; through the panel's outbox (Telegram's
  limits) `sendMessage`, `editMessageText`, `editMessageReplyMarkup`, `editMessageCaption`,
  `editMessageMedia`, `deleteMessage`, `deleteMessages`, `sendPhoto`, `sendDocument`,
  `sendMediaGroup`, `sendChatAction`, `copyMessage`, `forwardMessage`, `sendSticker`,
  `sendAnimation`, `sendVideo`, `sendAudio`, `sendVoice`, `sendLocation`, `pinChatMessage`,
  `unpinChatMessage`. `X-Mikan-Priority: notice | bulk` picks the outbox lane.
- Everything else, `setWebhook`, `sendInvoice`, `createInvoiceLink`, `answerPreCheckoutQuery`,
  `refundStarPayment`, `logOut` and `close` among them, is refused with 403 `not allowed for
  addons`.
- Parameters come as JSON, a form, multipart (files up to the panel's limit) or the query.
- `web_app.url` and `login_url` anywhere in `reply_markup` or `menu_button` must open the
  panel's own pages (its public subscription address), else 400: a foreign page would get
  signed Mini App data.

## Sources

The catalog's addons come from `ghcr.io/getmikan/`, pinned by the multi-arch digest of the
signed catalog, and run on the host network on loopback. An admin may also run an addon of
their own, protocol 2 only: a custom image by digest (on the bridge `mikan-addons`, its port
published on loopback) or an external process on `http://127.0.0.1:<port>`. The panel marks
those "not verified" and shows the permissions the admin gives them.
