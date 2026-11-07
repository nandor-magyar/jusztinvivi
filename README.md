# jusztin vivien

Portfolio + shop for painter Jusztin Vivien. Hungarian (default, at `/`) and English (at `/en/`).

- `site/`: Hugo site. Noir theme, self-hosted fonts. The only third-party request is Cloudflare Turnstile on the contact page.
- `main.go`: Go server (stdlib only). Serves the embedded site, `GET /healthz`, and `POST /api/contact`.

## Requirements

- Hugo 0.168
- Go 1.26
- Docker for the image

## Local

```sh
make serve          # hugo only, http://localhost:1313
make server         # build + Go server, http://localhost:8080
make run            # same, in the image
```

The contact page shows the e-mail address after a Cloudflare Turnstile check. The address is never in the HTML or the repo. The server reads `CONTACT_EMAIL` and returns it from `POST /api/contact` only after Cloudflare's siteverify accepts the token (`TURNSTILE_SECRET`). Without these the endpoint returns 503 and the rest of the site works. `turnstileSiteKey` in `site/hugo.toml` is Cloudflare's always-pass test key until the real widget exists. Locally, `TURNSTILE_SECRET=1x0000000000000000000000000000000AA` is the matching always-pass secret.

## Content

Everything editable is plain text under `site/`.

| what | where |
|---|---|
| a work (original) | `site/content/works/<name>/index.hu.md` + `index.en.md` + `image.jpg` |
| a fan-art piece | `site/content/fanart/<name>/…` (same shape) |
| story page with music | `site/content/journeys/<name>/…`; put the audio file in that folder and set `audio:` |
| exhibitions, education, press | `site/data/cv.yaml` |
| email, Instagram, Facebook | `site/hugo.toml` → `[params]` |
| interface wording | `site/i18n/hu.toml`, `site/i18n/en.toml` |
| about text | body of `site/content/about.hu.md` / `about.en.md` |
| portrait | `site/assets/img/portrait.jpg` |

Work front matter:

```yaml
title: "olívia"          # empty -> pencil note
slug: "olivia"           # url part; can differ per language
art: original            # original | fanart
medium: "akril"
size: "40 × 50 cm"
year: ""
status: available        # available | sold | nfs
price: ""                # empty -> "price on request"
journey: "olivia"        # optional: links to journeys/olivia
quote: ""                # optional line under the facts
weight: 1                # order in the grid
```

Missing facts render as lilac handwritten notes `[…]`, so gaps are visible on the page. To list a work without a photo yet, add `placeholder: true` and `build: {render: never}`.

### Rules the build enforces

- Fan art is never offered for sale: a fan-art page with `price`, `prints`, `handFinished` or `status: available` fails the build. Fan-art pages never get an enquire button.
- Paintings are scaled down (`Fit`), never cropped. Re-encoding drops camera metadata.

## Deploy

The image is `ghcr.io/nandor-magyar/jusztinvivien`, distroless, nonroot, port 8080 (`PORT`). Env: `TURNSTILE_SECRET`, `CONTACT_EMAIL`.

- `make release BUMP=patch|minor|major` (default patch) tags the next `vX.Y.Z`, pushes the tag and creates a GitHub release. The version from `git describe` is baked into `/version.txt` and the image label.
- CI (`.github/workflows/image.yaml`) builds amd64 and arm64. Pull requests build only. `main` pushes `:main` and `:sha-<short>`. A `vX.Y.Z` tag pushes `:X.Y.Z`, `:X.Y`, `:X` and `:latest`.
- Hosting follows itsnandi in `rf-apps`: a compose in `srv/jusztinvivien/` (hostname `jusztinvivien.rf.internal`, secrets in `secrets.env`) and a `vivi.rocketfuel.pw` block in the shared Caddyfile that reverse-proxies to port 8080. Renovate bumps the tag.

Before go-live: set `baseURL` in `site/hugo.toml` (now `vivi.rocketfuel.pw`, final `jusztinvivien.com`), set the real `turnstileSiteKey`, and replace the reference photos with straight, true-colour shots.

## Fonts

EB Garamond, Cormorant Garamond, Caveat (SIL Open Font License, licences in `site/static/fonts/`), latin + latin-ext subsets for full Hungarian (ő, ű).
