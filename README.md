# chore-chart

Annual chore calendar. Displays a year-view schedule of recurring household chores. Go backend + React/Vite frontend, deployed on a Raspberry Pi 4B.

---

## Repo layout

```
chore-chart/
  server/
    main.go           — Go HTTP server, schedule generation, JSON API
    go.mod
    data/
      chores.json     — chore definitions (name, person, cadence in weeks)
    internal/
      auth/           — JWT verification, JWKS key provider, auth middleware
      logging/        — HTTP request logging middleware
  client/
    src/              — React/Vite frontend
    vite.config.ts
    package.json
  Dockerfile          — multi-stage Go build (GOARCH=arm64), scratch runtime
```

---

## Chore data

Chores are defined in `server/data/chores.json`. This file is baked into the Docker image at build time — it is not a volume. To update the chore schedule, edit the JSON and redeploy.

```json
[
  { "name": "Kitchen Trash", "person": "Alison", "cadence_weeks": 0.5 },
  { "name": "Bathroom Vanity", "person": "Kevina", "cadence_weeks": 2 }
]
```

`cadence_weeks` can be a decimal (e.g. `0.5` = twice a week). Must be greater than zero.

---

## API

All API endpoints require a valid `cc_token` cookie (RS256 JWT issued by the account-manager OAuth server).

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/chores` | Returns the full list of chore definitions |
| `GET` | `/api/schedule` | Returns the computed year schedule keyed by ISO date (`"2026-01-05": [...]`) |
| `GET` | `/oauth/start` | Initiates the PKCE OAuth 2.0 login flow |
| `GET` | `/oauth/callback` | Handles the OAuth callback, sets `cc_token` and `cc_refresh` cookies |
| `POST` | `/api/auth/logout` | Clears auth cookies |

The schedule is computed once at startup and held in memory — no database.

The server listens on `:8080` by default. Set the `PORT` environment variable to override.

---

## Auth

Authentication uses OAuth 2.0 with PKCE via an external account-manager service. On login, the server exchanges an auth code for tokens and sets two `HttpOnly; Secure; SameSite=Strict` cookies:

- `cc_token` — RS256 JWT access token (expires per `expires_in` from the token response)
- `cc_refresh` — refresh token (30-day expiry)

The server fetches its RS256 public key from the JWKS endpoint at startup and refreshes it hourly to handle key rotation. Keys are matched by `kid` header claim.

### Required environment variables

| Variable | Description |
|---|---|
| `ACCOUNT_MANAGER_URL` | Base URL of the OAuth/account-manager service |
| `OAUTH_CLIENT_ID` | OAuth client ID |
| `OAUTH_CLIENT_SECRET` | OAuth client secret |
| `OAUTH_REDIRECT_URI` | Redirect URI registered with the OAuth server |
| `PORT` | Port to listen on (default: `8080`) |

---

## Development

### Backend

```bash
cd server
ACCOUNT_MANAGER_URL=... OAUTH_CLIENT_ID=... OAUTH_CLIENT_SECRET=... OAUTH_REDIRECT_URI=... go run .
```

The backend serves on `http://localhost:8080`.

### Frontend

```bash
cd client
npm install
npm run dev    # Vite dev server with proxy to :8080
```

### Tests

```bash
# Go
cd server && go test ./...

# Frontend
cd client && npm test
```

---

## Deployment

Deployed via the infra repo's `deploy.sh`. The frontend is built and copied to `infra/static/chore-chart/` by the deploy script; nginx serves it as static files under `/chores/`. The Go server handles only the API and OAuth flow.

See [homelab-infra/README.md](../homelab-infra/README.md) for the full deployment guide.
