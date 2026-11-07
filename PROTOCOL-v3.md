# Addon protocol 3: public pages

Protocol 2 gives an addon an admin UI: the panel proxies its `/ui/` files into a sandboxed
iframe of the admin app, behind the admin's session. Protocol 3 adds one permission,
`ui:public`, for the pages an addon wants **every visitor** to open — a network status
page, a knowledge base — without a session.

## The grant

`ui:public` is approved by the admin like every permission of protocol 2 (the panel lists
it in the install dialog and in the addon's settings; a grant is stored per addon). An
addon without the grant has no public pages: the panel answers 404 for every address of
them, exactly like an unknown path, so the address itself tells a visitor nothing.

## Where the pages live

Below the panel's subscription path, next to the subscription page itself:

```
https://panel.example/<sub path>/addons-public/<id>/<file>
```

- `<sub path>` is the panel's secret subscription path — the same one the subscription
  page uses. The pages work on the subscription port too.
- `<id>` is the addon's id, `<file>` a path under the addon's `/ui/` as protocol 2 serves
  it (`index.html`, `style.css`, …). An empty `<file>` serves the entry named in
  `/v2/info`'s `ui.entry`.
- Only `GET` (and `HEAD`) is answered; anything else is 405.

## What the panel guarantees

- **Static content only.** The response carries a Content-Security-Policy that forbids
  scripts, objects, forms, frames and base URLs: `default-src 'none'`, inline styles and
  images from the panel's origin or `data:` allowed. A public page cannot run code in a
  visitor's browser, so it cannot act for them or reach the panel's cookies.
- **No session, no API.** The pages are fetched from the addon over its own `/ui/` like
  the admin UI, but the visitor's request carries nothing of the panel's: no cookies, no
  tokens. Anything a page needs (live data) the addon must collect itself through its
  granted permissions and bake into the page.
- **Caching.** `Cache-Control: public, max-age=30` unless the addon says otherwise.
- **Failure looks like absence.** An addon that is not installed, not running, not
  granted `ui:public`, or names no `ui.entry` answers the panel's bare 404. An addon
  that is granted but does not answer gives a plain 502 page.

## What an addon provides

Nothing new on the wire: the same `/v2/info` (`ui.entry`, `ui.title`), the same `/ui/`
files. The only addition is the permission in `adapter.json`:

```json
"permissions": ["nodes:read", "inbounds:read", "kv", "ui:public"]
```

`min_panel` must be at least the panel version that introduced `ui:public`, so older
panels do not offer the addon pages they cannot serve.

## Discovery

The subscription page lists the public pages of the installed addons: `/<sub>/info`
answers `public_pages` with `{id, title, url}` for every running addon that has the
`ui:public` grant and a UI entry, and the page shows them as buttons. An addon needs no
code for this; the panel derives the list itself.

## The reference implementation

`tools/status-page` is the official status page: it polls `/panel/v2/nodes` and
`/panel/v2/inbounds` every 30 seconds, keeps hourly history in the panel's KV store, and
serves a self-refreshing static page (no scripts, per the CSP) in Russian and English.
