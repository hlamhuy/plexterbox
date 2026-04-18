# plexterbox

A self-hosted web app that keeps your [Plex](https://www.plex.tv) and [Letterboxd](https://letterboxd.com) movie watch histories in sync.

Connect both accounts, import your existing history, and let auto sync handle the rest — all running locally on your machine or in a Docker container.

---

## Features

- **Plex OAuth** — connect with your Plex account, no password stored
- **Letterboxd login** — supports accounts with 2FA (TOTP)
- **Unified watch table** — view both platforms' history side by side, deduplicated
- **Manual import** — import Letterboxd diary → Plex, or Plex history → Letterboxd
- **Auto Sync** — polls both platforms on a configurable interval and merges new entries automatically

---

## Getting Started

### Docker (recommended)

```bash
docker compose up -d
```

Then open [http://localhost:12349](http://localhost:12349).

Your session and database are stored in a named Docker volume and persist across restarts and image updates.

To use a different port:

```yaml
# docker-compose.yml
ports:
  - "8080:12349"
```

---

### Windows executable

Download `plexterbox.exe` from the [Releases](../../releases) page and run it. Open [http://localhost:12349](http://localhost:12349) in your browser.

The app stores its data in `%APPDATA%\plexterbox\` (session and database).

> Windows Defender may flag the executable as a false positive on first run. You can submit it for review at [Microsoft's intelligence portal](https://www.microsoft.com/en-us/wdsi/filesubmission) or build from source.

---

### Build from source

**Prerequisites:** Go 1.25+, Node.js 22+

```bash
# Clone
git clone https://github.com/hlamhuy/plexterbox.git
cd plexterbox

# Build for current OS (outputs ./plexterbox)
make build

# Build Windows executable (outputs ./plexterbox.exe)
make build-windows

# Development mode (backend + Vite dev server with HMR)
go run ./cmd/server/ &
cd web && npm install && npm run dev
```

---

## Configuration

| Method | Example |
|---|---|
| Default | `./plexterbox` → listens on `:12349` |
| Flag | `./plexterbox -port 8080` |
| Env var | `PORT=8080 ./plexterbox` |

---

## Tech Stack

- **Backend:** Go, SQLite (`modernc.org/sqlite` — pure Go, no CGo)
- **Frontend:** React, TypeScript, Vite, Tailwind CSS
- **Auth:** Plex OAuth (pin-based), Letterboxd cookie session with Chrome TLS fingerprint bypass

---

## License

[MIT](LICENSE)
