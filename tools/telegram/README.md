# Telegram bot

The official mikan bot as an addon: the subscription menu, purchases, devices and notices.
Protocol: [PROTOCOL-v2.md](../../PROTOCOL-v2.md), kind `bot`, mode `client`. Image:
`ghcr.io/getmikan/tool-telegram`.

The code is not here: the image builds `cmd/mikan-telegram` of
[mikan](https://github.com/drillex100-beep/scilla) at the tag `v<version>`, where `<version>` is
this tool's version in [adapter.json](adapter.json) (CI passes it as the `VERSION` build
argument). So the tool's version is always the mikan release it comes from, and a new
release of the bot is a new version here.

The panel keeps the bot token, the polling, Telegram Stars and the Mini App; the addon only
draws the screens. It talks to the panel through the Bot API proxy (`MIKAN_BOT_API_URL`,
`MIKAN_BOT_TOKEN`) and the Panel API (`MIKAN_PANEL_URL`, `MIKAN_PANEL_TOKEN`), keeps its
state in the panel's KV and serves nothing itself. It has no settings of its own.

| Permission | What for |
|---|---|
| `bot` | the bot slot: updates, messages, the bot's data in the panel |
| `users:read` | the subscription, devices and traffic of a user who writes to the bot |
| `users:write` | unbinding a device, a new subscription link |
| `billing:invoice` | tariffs, packages, trials and invoices |
| `events` | payments and the notices the panel schedules |
| `kv` | menus and transfers between taps |

Releasing a new bot: set `version` (and `min_panel` when the bot needs a newer panel) to the
mikan release after its tag is pushed, then merge. Building the image by hand:

```sh
docker buildx build --platform linux/amd64 \
  --build-arg VERSION="$(jq -r .version tools/telegram/adapter.json)" \
  -f tools/telegram/Dockerfile -t tool-telegram:dev --load .
```
