# chore-chart

Annual chore calendar with minimal CRUD. Displays a year-view schedule of recurring household chores. Go backend + React/Vite frontend.

---

## Repo layout

```
chore-chart/
  server/
    main.go           — Go HTTP server, schedule generation, JSON API
    go.mod
    data/
      chores.json     — chore definitions (name, person, cadence in weeks)
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

`cadence_weeks` can be a decimal (e.g. `0.5` = twice a week).

---

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/chores` | Returns the full list of chore definitions |
| `GET` | `/api/schedule` | Returns the computed year schedule keyed by ISO date (`"2026-01-05": [...]`) |

The schedule is computed once at startup and held in memory — no database.

The server listens on `:8080` (hardcoded; `PORT` env var is not yet supported).

---

## Development

### Backend

```bash
cd server
go run .
```

The backend serves on `http://localhost:8080`.

### Frontend

```bash
cd client
npm install
npm run dev    # Vite dev server with proxy to :8080
```

---

## Deployment

Deployed via the infra repo's `deploy.sh`. The frontend is built and copied to `infra/static/chore-chart/` by the deploy script; nginx serves it as static files.

See [homelab-infra/README.md](../homelab-infra/README.md) for the full deployment guide.

> **Note:** Auth integration (OAuth 2.0 via account-manager) is planned for Step 4 of the infra build. Currently the API has no authentication.
# chore-chart
