# mikan marketplace

Addons for the [mikan](https://github.com/drillex100-beep/scilla) VPN panel, and the signed
catalog the panel installs them from. They come in packages:

- [`payments/`](payments): payment methods. Each is a small stateless HTTP service that
  connects the panel to one payment provider. The panel lists them under Payments →
  «Добавить способ оплаты»; the host pulls the adapter's image by digest and runs it next
  to the panel, on loopback. The contract between them is [PROTOCOL.md](PROTOCOL.md).
- [`tools/`](tools): everything else, such as the Telegram bot. They speak protocol 2,
  [PROTOCOL-v2.md](PROTOCOL-v2.md): they call the panel's API for addons with the
  permissions the admin approved, and a bot works through the panel's Bot API proxy.

## Tools

| id | | Kind |
|---|---|---|
| [`telegram`](tools/telegram) | the official mikan bot: subscription menu, purchases, devices, notices. Built from mikan's source at the release its version names | `bot`, mode `client` |

Each needs panel 0.5.1.0 or later (the bot: the release it is built from).

## Payment methods

| id | Provider | Currencies | Capabilities |
|---|---|---|---|
| [`yookassa`](payments/yookassa) | ЮKassa: bank cards and SBP | RUB | webhook, refund |
| [`cardlink`](payments/cardlink) | Cardlink: bank cards and SBP | RUB, USD, EUR | webhook |
| [`cryptobot`](payments/cryptobot) | @CryptoBot (Crypto Pay): cryptocurrency, priced in fiat | RUB, USD, EUR | webhook |
| [`platega`](payments/platega) | Platega: SBP, cards, Sberpay, crypto | RUB | webhook |
| [`rollypay`](payments/rollypay) | RollyPay: SBP, cards, crypto | RUB | webhook |

Each needs panel 0.4.3 or later.

## Layout

```
payments/<id>/       one payment adapter: main.go, the provider client, tests, adapter.json, Dockerfile
tools/<id>/          one tool (protocol 2): adapter.json, Dockerfile, its code when it lives here
internal/adapter/    the shared server of protocol 1: env, loopback check, Bearer auth, limits, errors, settings validation
internal/catalog/    adapter.json and index.json: types, checks, signing
cmd/index/           builds and signs dist/index.json; verifies it
internal/community/  the registry of community addons: snapshots, checks, the sync plan, the GitHub API
cmd/community/       the indexer of getmikan/marketplace-community: sync, build, verify
PROTOCOL.md          protocol 1, for payment adapter authors
PROTOCOL-v2.md       protocol 2, for the authors of every other addon
```

Go 1.27, standard library only.

```sh
gofmt -l . && go vet ./... && go test ./...
docker buildx build --platform linux/amd64 -f payments/yookassa/Dockerfile -t adapter-yookassa:dev --load .
docker run --rm --network host -e MIKAN_ADAPTER_LISTEN=127.0.0.1:41873 -e MIKAN_ADAPTER_TOKEN=test adapter-yookassa:dev
curl -H 'Authorization: Bearer test' http://127.0.0.1:41873/v1/info
```

## Writing a payment adapter

1. Read [PROTOCOL.md](PROTOCOL.md), especially the security section.
2. Create `payments/<id>/` (lowercase letters, digits, dashes; an id is unique across the
   packages):
   - `adapter.json`, the catalog entry without the digest:
     ```json
     {
       "id": "examplepay",
       "category": "payments",
       "name": {"ru": "Example Pay", "en": "Example Pay"},
       "description": {"ru": "Карты", "en": "Cards"},
       "version": "1.0.0",
       "protocol": 1,
       "image": "ghcr.io/getmikan/adapter-examplepay",
       "min_panel": "0.4.3",
       "homepage": "https://github.com/getmikan/marketplace/tree/main/payments/examplepay"
     }
     ```
     `category` is the package, the directory it is in. `version` belongs to this
     adapter. Increase it when its code, Dockerfile, embedded manifest, or shared server
     code changes.
   - `main.go` embeds `adapter.json` and calls `adapter.Run` with your provider, which
     implements `adapter.Provider` (and `adapter.Refunder` with the `refund` capability).
     The server validates the settings against your `Info().Settings`, checks invoice
     requests, maps errors and logs requests; your code only talks to the provider. Return
     `*adapter.Error` (`adapter.BadCredentials`, `adapter.NotFound`, `adapter.Refused`, …)
     for answers the admin should see; any other error becomes `provider_unavailable`
     without its text. Use `adapter.HTTPClient()` (15 s timeout) for provider calls.
   - Tests with an `httptest` fake of the provider, through `adapter.NewHandler`: info,
     check with good and bad credentials, the idempotency key and the amount of a new
     invoice, the status mapping with amount and currency, a valid, a forged and an
     ignored webhook, and that the secrets never reach the log. The adapters here show how.
   - `Dockerfile`: copy one of the existing ones and change the id.
   - `README.md`: the settings, where to find them, the webhook setup.
3. Open a pull request. CI runs gofmt, vet, the tests and builds the image for amd64 and
   arm64; `go run ./cmd/index list` checks every `adapter.json`.

## Releases and signing

Each adapter has its own version. To release a changed adapter, increase only its
`adapter.json` version and merge the change to `main`. [release.yml](.github/workflows/release.yml)
then:

1. Runs tests and verifies the previous release's `index.json.sig` with the trusted
   release public key. It compares the previous release tag with the new commit.
2. Requires a newer adapter version for changes to its image inputs. Documentation and
   tests alone need no bump. A change to shared server code or Go dependencies requires
   a version increase for every adapter whose Dockerfile copies them (not the telegram
   tool, which builds mikan's source). Versions are `x.y.z`, optionally with a fourth
   number and a prerelease, as mikan's (`0.5.1.0-rc.6`).
3. Builds and pushes only changed images for amd64 and arm64, each tagged with its own
   adapter version (also passed to the build as `VERSION`). Unchanged image digests come
   from the verified previous catalog.
4. Signs a new complete catalog and publishes it under an automatic `catalog-<run>-<attempt>`
   tag. The panel reads `releases/latest/download/index.json`.

Unrelated documentation and workflow changes do not publish a new catalog. An unavailable
previous release, invalid signature, unknown image input, or missing digest fails the
release. Adapter removal needs a separate explicit process.

`index.json.sig` is `base64(ed25519.Sign(key, <exact bytes of index.json>))`, the same
format as the panel's release manifest, and the panel and the host check it with the same
public key, `Z3wSIPBSaJxh5CsGO8eINI0aM0kyrQ46EcJSNeH85W8=`.

Before the first release the maintainer must:

- add the repository secret **`MARKETPLACE_SIGNING_KEY`**: the PEM (PKCS#8) Ed25519
  private key, **the same key as the panel's `RELEASE_SIGNING_KEY`**. The workflow refuses
  to publish a catalog that this public key does not verify;
- after the first push of each image, make the package `adapter-<id>` (`tool-<id>` for a
  tool) **public** in the
  organization's packages settings (GitHub creates new packages private, and the host pulls
  without credentials), and check that it is linked to this repository.

Check a published catalog by hand:

```sh
curl -fsSLO https://github.com/getmikan/marketplace/releases/latest/download/index.json
curl -fsSLO https://github.com/getmikan/marketplace/releases/latest/download/index.json.sig
go run ./cmd/index verify index.json
```

## ⛔ Forks and third-party addons

> [!CAUTION]
> Official addons are only the ones in this repository's signed catalog, with images under `ghcr.io/getmikan/`. **The mikan project considers every fork of an addon, every third-party catalog and every "unofficial" addon potentially unsafe.** They are not ours: we do not review or support them and are not responsible for how they work. The panel marks them as not verified by mikan.

Third-party addons are listed in [getmikan/marketplace-community](https://github.com/getmikan/marketplace-community):
snapshots of the authors' releases, synced every hour by `cmd/community` (its README says
how an addon gets there and how it is revoked). Being listed there is not a review either.

## License

[GPL-3.0-only](LICENSE), as the panel.
