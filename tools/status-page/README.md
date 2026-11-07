# status-page

The official **network status page** for Scilla panels — an addon of
[protocol 3](../../PROTOCOL-v3.md) (`ui:public`). It installs and removes like any other
addon; while installed, the panel serves its page to every visitor at

```
https://<panel>/<sub path>/addons-public/status-page/
```

and links it from every subscriber's page, so clients find it without being told the
address.

## What the page shows

- **A banner for the whole network**: Всё работает / Частичные проблемы / Серьёзные
  проблемы / Нет данных — the first thing a client checks when their connection fails.
- **A row per protocol** (inbound): the title as the client sees it, its node, a status
  badge (Работает / Проблемы / Выключен / Нет данных), uptime over 24 h / 7 d / 30 d and
  the time of the last incident.
- **An explanation block** that tells the client where the problem likely is: one
  protocol failing while the rest work (the provider may block it — try another), one
  node failing (a server problem), everything failing (the panel or the network), or
  everything working (the problem is on the client's device or provider).
- **A hint in the footer** for the "everything is green but it does not work" case.

Russian and English, chosen by `?lang=` or `Accept-Language`. The page refreshes itself
every 60 seconds (`<meta http-equiv="refresh">` — the CSP of public pages forbids
scripts, so there is no JavaScript).

No addresses, ports or keys ever reach the page: the addon reads them through
`nodes:read` and `inbounds:read`, which do not include them.

## How it decides

- Every 30 seconds it polls the panel's `/panel/v2/nodes` and `/panel/v2/inbounds`.
- A protocol that reports `error` is shown as **Проблемы** only when the error persists
  for two polls in a row (hysteresis): a single failed check counts as **Нет данных**,
  so the page does not flicker.
- History is kept as hourly counters (ok / error / off / unknown per protocol) in the
  panel's KV store — the addon keeps no files, so its state survives container updates
  and is backed up with the panel. A snapshot of the counters' totals and the incident
  log is written every 5 minutes and on shutdown, and restored on boot; hour buckets
  older than 35 days are dropped.

## Permissions

`nodes:read`, `inbounds:read`, `kv`, `ui:public` — nothing else. The addon cannot touch
users, settings or other addons.

## Configuration

Standard protocol-2 environment, set by the panel on install: `SCILLA_ADAPTER_LISTEN`,
`SCILLA_ADAPTER_TOKEN`, `SCILLA_PANEL_URL`, `SCILLA_PANEL_TOKEN`. No settings of its own.

## Building

```sh
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/drillex100-beep/tool-status-page:0.5.2.2 -f tools/status-page/Dockerfile .
```

Static Go binary (stdlib only) on a distroless image; runs as uid 65532, read-only root
filesystem is fine.
