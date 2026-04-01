# BrewOps

### Enterprise-Grade Beverage Infrastructure Observability

> *"There is coffee all over the world. Increasingly, in a world in which computing is ubiquitous, the computists want to make coffee."*
> — RFC 2324, Section 1

**BrewOps** is the world's first production-grade implementation of the [Hyper Text Coffee Pot Control Protocol (HTCPCP/1.0)](https://datatracker.ietf.org/doc/html/rfc2324) with full [RFC 7168 TEA extension](https://datatracker.ietf.org/doc/html/rfc7168) support, real-time observability, and Denial of Coffee Service (DoCS) attack detection.

We identified a critical gap in modern cloud infrastructure: despite HTCPCP being standardized since 1998, there are **zero production-ready implementations** with SLA monitoring, incident timelines, and real-time telemetry. BrewOps closes this gap.

---

## What Is This

A fully functional HTCPCP server that:

- Responds to `BREW`, `GET`, `WHEN`, and `PROPFIND` HTTP methods per the RFC
- Returns proper `418 I'm a Teapot` when you try to brew coffee in a teapot
- Auto-creates new pots on each request (every 5th pot is a surprise teapot)
- Streams live events to a retro-brutalist web dashboard via SSE
- Detects Denial of Coffee Service (DoCS) attacks when you brew too fast
- Cleans up idle pots automatically because even coffee infrastructure needs a janitor

Built in Go with zero external dependencies. Pure standard library.

---

## Quick Start

```bash
# Clone and run
git clone https://github.com/turazashvili/brewops.git
cd brewops
go run ./cmd/brewopsd
```

Then open http://localhost:8418/dashboard or start brewing:

```bash
# Brew coffee (auto-creates a pot)
curl -X BREW http://localhost:8418/pot -d 'start'

# Try to brew coffee in the teapot
curl -X BREW http://localhost:8418/pot-2 -d 'start'

# Brew with additions
curl -X BREW http://localhost:8418/pot \
  -H 'Accept-Additions: Cream, Whisky' -d 'start'

# Brew tea (RFC 7168)
curl -X BREW http://localhost:8418/tea/earl-grey \
  -H 'Content-Type: message/teapot' -d 'start'

# Check pot status
curl http://localhost:8418/pot-0

# Say "when" for milk (RFC 2324 Section 2.1.4)
curl -X WHEN http://localhost:8418/pot-0

# Launch a DoCS attack
for i in $(seq 1 15); do
  curl -s -X BREW http://localhost:8418/pot -d 'start' > /dev/null
done

# Fleet status
curl http://localhost:8418/status
```

Every response is pretty-printed plain text with ASCII art. The dashboard updates in real time.

---

## Docker

```bash
docker build -f Brewfile -t brewops .
docker run -p 8418:8418 brewops
```

Yes, it's called a `Brewfile`. Yes, the build stage is named `barista`. Yes, the working directory is `/coffeeshop`.

---

## The Dashboard

A retro-brutalist real-time observability dashboard that looks like a 90s web forum had a baby with Grafana.

- **Global Operations** panel: Total Brews, 418s Served, 418 Rate, Caffeine Dispensed, DoCS Attacks
- **Fleet Status**: Live pot cards showing state, temperature, fill level, brew time
- **Incident Timeline**: Live-scrolling event log with severity levels (CRITICAL for 418s and DoCS)
- **SLA Panel**: Brew Uptime (always 99.97%), Spills This Quarter (always 3), Data Retention (until next deploy)
- **Try It Yourself**: Copy-paste curl commands

The dashboard uses Server-Sent Events (SSE) for real-time updates. Every curl command from any user appears in the timeline instantly.

---

## Protocol Compliance

### RFC 2324 — HTCPCP/1.0

| Feature | Status |
|---------|--------|
| `BREW` method | Implemented |
| `POST` method (equivalent to BREW) | Implemented |
| `GET` method | Implemented |
| `WHEN` method ("say when" for milk) | Implemented |
| `PROPFIND` method (brew metadata) | Implemented |
| `Accept-Additions` header | 16 additions (milk, syrup, alcohol, sugar) |
| `Safe: if-user-awake` header | Always returned |
| `418 I'm a Teapot` | The crown jewel |
| `406 Not Acceptable` (bad additions) | Implemented |
| `coffee:` URI scheme | Implemented |
| `Content-Type: message/coffeepot` | Implemented |
| Security Considerations (Section 7) | DoCS detection |

### RFC 7168 — HTCPCP-TEA

| Feature | Status |
|---------|--------|
| `Content-Type: message/teapot` | Implemented |
| `300 Multiple Options` (tea menu) | Implemented with Alternates header |
| Tea varieties (Darjeeling, Earl Grey, etc.) | 6 varieties |
| Sugar additions (Sugar, Xylitol, Stevia) | Implemented |

---

## Fleet Configuration

The server starts with 4 permanent pots. New pots are created dynamically via `BREW /pot`.

| Pot | Type | Name | Notes |
|-----|------|------|-------|
| pot-0 | coffee-pot | Primary Brew Unit | Always available |
| pot-1 | coffee-pot | Secondary Brew Unit | Always available |
| pot-2 | **teapot** | Her Majesty's Teapot | **Will 418 on coffee** |
| pot-3 | coffee-pot | Emergency Backup Unit | Always available |
| pot-N | dynamic | Auto-created | Every 5th is a surprise teapot |

Dynamic pots auto-expire after cooling down. The 4 original pots are permanent infrastructure.

---

## Architecture

```
                    ┌─────────────────┐
                    │   brew-ctl CLI  │
                    └────────┬────────┘
                             │ BREW / GET / WHEN / PROPFIND
                             ▼
┌──────────┐        ┌────────────────┐        ┌──────────────┐
│ Dashboard │◄─SSE─│   brewopsd     │───────►│   Metrics    │
│ (Web UI)  │       │  HTCPCP/1.0   │        │  Collector   │
└──────────┘        │   Server      │        │  + DoCS Det. │
                    └───────┬───────┘        └──────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
    ┌─────────┐       ┌──────────┐       ┌─────────┐
    │  pot-0  │       │  pot-2   │       │  pot-N  │
    │ coffee  │       │ TEAPOT   │       │ dynamic │
    └─────────┘       └──────────┘       └─────────┘
```

## CLI Client

```bash
go run ./cmd/brew-ctl status                              # Fleet overview
go run ./cmd/brew-ctl brew coffee --pot 0                 # Brew in specific pot
go run ./cmd/brew-ctl brew coffee --pot 2                 # 418 with ASCII art
go run ./cmd/brew-ctl brew espresso --additions Cream     # With additions
go run ./cmd/brew-ctl tea earl-grey                       # Brew tea
go run ./cmd/brew-ctl when 0                              # Stop milk
go run ./cmd/brew-ctl props 0                             # PROPFIND metadata
go run ./cmd/brew-ctl logs                                # Stream live events
```

---

## Denial of Coffee Service (DoCS)

> *"Unmoderated access to unprotected coffee pots from Internet users might lead to several kinds of denial of coffee service attacks."*
> — RFC 2324, Section 7

If you send more than 10 BREW requests in 30 seconds, BrewOps classifies it as a DoCS attack. The brew still succeeds (this is a coffee pot, not a firewall), but:

- The DoCS counter increments on the dashboard
- A CRITICAL incident appears in the timeline
- Your response includes a DoCS warning banner
- The SRE on-call is paged (not really)

---

## Pot Lifecycle

Each pot goes through a state machine:

```
idle → grinding (3s) → brewing (12s) → pouring (2s) → ready (15s) → cooling (30s) → idle
```

Dynamic pots are cleaned up ~60 seconds after going idle.

---

## SLA

| Metric | Value |
|--------|-------|
| Brew Uptime | 99.97% |
| Spills This Quarter | 3 |
| DoCS Attacks | Live counter |
| Data Retention Policy | Until next deploy |
| Mean Time To Brew | ~15 seconds |
| Supported Additions | 16 |

---

## Project Structure

```
brewops/
├── cmd/
│   ├── brewopsd/main.go        # Server entry point
│   └── brew-ctl/main.go        # CLI client
├── internal/
│   ├── htcpcp/
│   │   ├── types.go            # Protocol types, additions, temperature labels
│   │   ├── pot.go              # Pot state machine, fleet management, janitor
│   │   └── server.go           # HTTP handler, routing, all HTCPCP methods
│   ├── metrics/
│   │   └── metrics.go          # Counters, SSE, DoCS detection, event messages
│   └── dashboard/
│       └── handler.go          # Static file serving
├── web/
│   ├── index.html              # Dashboard (retro-brutalist theme)
│   ├── style.css               # Teal background, black borders, orange headings
│   └── dashboard.js            # SSE client, live updates
├── Brewfile                    # Dockerfile (but for coffee)
├── fly.toml                    # Fly.io deployment config
└── README.md
```

Zero external dependencies. Pure Go standard library.

---

## Security Considerations

> *"Anyone who gets in between me and my morning coffee should be insecure."*
>
> *"The improper use of filtration devices might admit trojan grounds."*
>
> *"Putting coffee grounds into Internet plumbing may result in clogged plumbing, which would entail the services of an Internet Plumber."*
>
> — RFC 2324, Section 7

---

## Credits

- [RFC 2324](https://datatracker.ietf.org/doc/html/rfc2324) — Larry Masinter (1 April 1998)
- [RFC 7168](https://datatracker.ietf.org/doc/html/rfc7168) — Imran Nazar (1 April 2014)
- Built for the [DEV April Fools Challenge 2026](https://dev.to/challenges/aprilfools-2026)

## License

MIT — Brew freely.
