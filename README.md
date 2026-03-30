# homelab-observability

Network observability, traffic analysis, and automated control stack for a single Linux homelab server. Built to provide full infrastructure visibility and lay the groundwork for automated network management via OPNsense.

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        Cloudflare Tunnel                         │
│  grafana-monitor.ocheverse.ng    prometheus-monitor.ocheverse.ng │
│  netdata-monitor.ocheverse.ng    ntopng-monitor.ocheverse.ng     │
│  throttle-monitor.ocheverse.ng                                   │
└──────────┬───────────────────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────────────────┐
│                      ochehomelab (Ubuntu 24.04)                  │
│                                                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │ Prometheus   │  │ Alertmanager │  │ Throttle Engine (Go)    │ │
│  │ :9090        │──│ :9093        │──│ :8090                   │ │
│  │ metrics +    │  │ routes alerts│  │ webhook receiver + UI   │ │
│  │ alert rules  │  │ to webhook   │  │ device inventory        │ │
│  └──────┬───────┘  └──────────────┘  │ network scanner (nmap)  │ │
│         │                            └─────────────────────────┘ │
│  ┌──────┴───────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │ Node Exporter│  │ Grafana      │  │ ntopng                  │ │
│  │ :9100        │  │ :3003        │  │ :3002                   │ │
│  │ CPU/mem/disk │  │ dashboards   │  │ deep packet inspection  │ │
│  │ net/IO       │  │ pre-loaded   │  │ traffic flow analysis   │ │
│  └──────────────┘  └──────────────┘  └─────────────────────────┘ │
│                                                                  │
│  ┌──────────────┐                                                │
│  │ Netdata      │                                                │
│  │ :19999       │                                                │
│  │ real-time    │                                                │
│  │ system stats │                                                │
│  └──────────────┘                                                │
└──────────────────────────────────────────────────────────────────┘
```

## Stack

| Service | Purpose | Port | Public URL |
|---------|---------|------|------------|
| **Prometheus** | Metrics collection, alerting rules, scraping | 9090 | `prometheus-monitor.ocheverse.ng` |
| **Alertmanager** | Alert routing, webhook dispatch | 9093 | Internal only |
| **Grafana** | Dashboards and visualization | 3003 | `grafana-monitor.ocheverse.ng` |
| **Netdata** | Real-time system monitoring with 1s granularity | 19999 | `netdata-monitor.ocheverse.ng` |
| **ntopng** | Network traffic analysis, DPI, flow tracking | 3002 | `ntopng-monitor.ocheverse.ng` |
| **Node Exporter** | Host-level metrics (CPU, memory, disk, network) | 9100 | Internal only |
| **Throttle Engine** | Go service: alert webhooks, device inventory, network scanning, UI dashboard | 8090 | `throttle-monitor.ocheverse.ng` |
| **Cloudflared** | System service routing all subdomains via Cloudflare Tunnel | — | Manages all above |

## What It Monitors

- **CPU and memory** — usage percentage, pressure alerts at 85% CPU / 90% memory
- **Disk IO** — read/write throughput per device, saturation detection
- **Disk space** — alerts when any filesystem drops below 15% free
- **Network bandwidth** — per-interface receive/transmit rates, spike detection at configurable thresholds
- **Active connections** — established TCP connection count
- **Network traffic flows** — via ntopng: protocol breakdown, top talkers, connection patterns
- **Container health** — Netdata monitors all Docker containers on the host

## Alert Pipeline

```
Prometheus (scrapes metrics every 15s)
    │
    ▼
Alert Rules (bandwidth.yml)
    │  HighBandwidthReceive  — interface RX > 100MB/s for 2min
    │  HighBandwidthTransmit — interface TX > 100MB/s for 2min
    │  HighCPUUsage          — CPU > 85% for 5min
    │  HighMemoryUsage       — memory > 90% for 5min
    │  DiskSpaceLow          — filesystem < 15% free for 10min
    │  HighDiskIO            — IO saturation > 90% for 5min
    ▼
Alertmanager (groups, deduplicates, routes)
    │
    ▼
Throttle Engine (POST /api/v1/alert)
    │  Parses alert payload
    │  Looks up source IP in device inventory
    │  Identifies device name and owner
    │  Logs structured JSON with full context
    │
    ▼
[FUTURE] OPNsense API → apply traffic shaping / firewall rules
```

## Throttle Engine

The throttle engine is a custom Go service that acts as the brain of the observability stack. It does more than just receive webhooks.

### Features

- **Web Dashboard** — dark-themed UI at the root URL showing:
  - Real-time alert stats (total, firing, resolved)
  - Alert history table with device identification
  - Device inventory management (add, edit, delete — persisted to YAML)
  - Network scanner (runs nmap, identifies known vs unknown devices, one-click add to inventory)
- **Alertmanager Webhook Receiver** — processes Prometheus alerts, enriches with device info
- **Device Inventory** — YAML-backed device database with IP, name, owner, type, MAC, notes
- **Network Discovery** — nmap-powered LAN scanning with known device correlation
- **Structured Logging** — JSON logs with full alert context for later analysis
- **REST API**:
  - `GET /` — dashboard UI
  - `POST /api/v1/alert` — Alertmanager webhook endpoint
  - `GET /api/v1/alerts` — alert history (JSON)
  - `GET /api/v1/stats` — engine statistics
  - `GET /api/v1/devices` — full device inventory
  - `PUT /api/v1/devices/update` — add or update a device
  - `DELETE /api/v1/devices/update` — remove a device
  - `GET /api/v1/scan` — trigger network scan
  - `GET /healthz` — health check

### Current Behavior (Phase 1)

The engine logs alerts and identifies devices. All action points where OPNsense API calls would happen are marked with `FUTURE:` comments in the source code.

### Planned Behavior (Phase 2 — OPNsense)

When OPNsense integration is ready, the engine will:
1. Receive bandwidth alerts
2. Look up the offending device in the inventory
3. Decide whether to throttle, deprioritize, or block based on device type/owner
4. Call the OPNsense API to apply traffic shaping rules
5. Auto-remove rules after a configurable timeout
6. Log all actions for audit trail

## Device Inventory

The inventory at `inventory/devices.yaml` maps IP addresses to device metadata. Devices are discovered via nmap scans and can be managed through the throttle engine UI.

```yaml
"192.168.1.1":
  name: "ZTE-Router"
  owner: "ISP"
  type: "router"
  mac: "30:d3:86:b4:40:25"
  notes: "ISP gateway"

"192.168.1.200":
  name: "ochehomelab"
  owner: "Oche"
  type: "server"
```

The inventory enables:
- **Alert enrichment** — "Oche's MacBook is spiking" instead of "192.168.1.10 is spiking"
- **Policy decisions** — throttle guest devices but not the server
- **Anomaly detection** — flag unexpected behavior by device type (e.g. IoT device making outbound SSH)
- **Automation** — generate OPNsense rules per device category

## ntopng Limitations

ntopng runs on the server's network interface (`eno1`), not on a router or mirror port. This means:

| What you see | What you don't see |
|---|---|
| All traffic to/from this server | Traffic between other LAN devices |
| Docker container traffic | Full WAN traffic (unless server is gateway) |
| LAN broadcasts (ARP, mDNS, SSDP) | Traffic on other subnets/VLANs |
| DNS queries from this server | Other devices' DNS queries |

**To get full network visibility later:**
1. Configure your router to send NetFlow/sFlow to ntopng
2. Set up a SPAN/mirror port on your switch
3. Deploy OPNsense as gateway with ntopng plugin

## Project Structure

```
homelab-observability/
├── docker-compose.yml              # All services
├── .env.example                    # Configuration template
├── .gitignore
├── README.md
│
├── alertmanager/
│   └── alertmanager.yml            # Routes bandwidth alerts → throttle engine
│
├── cloudflared/
│   └── config.yml                  # Tunnel routes (merged with existing services)
│
├── grafana/
│   └── provisioning/
│       ├── dashboards/
│       │   ├── dashboards.yml      # Auto-load dashboard JSONs
│       │   └── node-metrics.json   # Pre-built: CPU, mem, disk, network, connections
│       └── datasources/
│           └── datasources.yml     # Prometheus auto-configured as default
│
├── inventory/
│   └── devices.yaml                # IP → device mapping (editable via UI)
│
├── prometheus/
│   ├── prometheus.yml              # Scrape configs for all exporters
│   └── alerts/
│       └── bandwidth.yml           # Alert rules: bandwidth, CPU, memory, disk
│
├── scripts/
│   ├── setup.sh                    # Pre-flight checks (ports, Docker, .env)
│   └── check-health.sh             # Post-deploy verification
│
└── throttle-engine/
    ├── Dockerfile                  # Multi-stage Go build + nmap
    ├── go.mod / go.sum
    ├── main.go                     # HTTP server, route registration
    ├── config.yaml                 # Thresholds, server settings
    └── internal/
        ├── config/config.go        # YAML + env var config loading
        ├── handler/
        │   ├── webhook.go          # Alertmanager webhook processing + alert history
        │   └── ui.go               # Dashboard HTML, device CRUD, network scan
        └── inventory/
            └── inventory.go        # Thread-safe device inventory with persistence
```

## Setup

### Prerequisites

- Docker and Docker Compose V2
- An existing Cloudflare Tunnel (or remove cloudflared config and access services on localhost)
- Ubuntu/Debian server (tested on Ubuntu 24.04)

### Quick Start

```bash
# 1. Clone and enter the project
git clone <this-repo>
cd homelab-observability

# 2. Run setup checks
./scripts/setup.sh

# 3. Configure environment
cp .env.example .env
nano .env  # Set passwords, interface name, tunnel UUID

# 4. Configure Cloudflare Tunnel
# Edit cloudflared/config.yml with your tunnel UUID
# Copy credentials: cp ~/.cloudflared/<UUID>.json cloudflared/credentials.json
# Or merge routes into your existing /etc/cloudflared/config.yml

# 5. Start everything
docker compose up -d

# 6. Verify
./scripts/check-health.sh
```

### Port Reference

| Port | Service | Binding |
|------|---------|---------|
| 3002 | ntopng | 0.0.0.0 (host network) |
| 3003 | Grafana | 127.0.0.1 |
| 8090 | Throttle Engine | 0.0.0.0 (host network) |
| 9090 | Prometheus | 127.0.0.1 |
| 9093 | Alertmanager | 127.0.0.1 |
| 9100 | Node Exporter | Docker network only |
| 19999 | Netdata | 127.0.0.1 |

### Cloudflare Tunnel Notes

- Free Cloudflare SSL covers `*.ocheverse.ng` but **not** `*.subdomain.ocheverse.ng` (two levels deep)
- All monitoring hostnames use the flat format: `grafana-monitor.ocheverse.ng` (not `grafana.monitor.ocheverse.ng`)
- If you already run cloudflared as a system service, merge the ingress routes into your existing `/etc/cloudflared/config.yml` instead of running a separate container
- DNS CNAME records are created with: `cloudflared tunnel route dns <tunnel-name> <hostname>`

## Useful Commands

```bash
# View logs for a specific service
docker compose logs -f throttle-engine
docker compose logs -f prometheus

# Hot-reload Prometheus config (no restart needed)
curl -X POST http://127.0.0.1:9090/-/reload

# Check Prometheus scrape targets
curl -s http://127.0.0.1:9090/api/v1/targets | python3 -m json.tool

# Test the throttle engine webhook manually
curl -X POST http://127.0.0.1:8090/api/v1/alert \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "status": "firing",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "HighBandwidthReceive",
        "device": "eno1",
        "instance": "192.168.1.200:9100",
        "severity": "warning"
      },
      "annotations": {
        "summary": "High inbound bandwidth on eno1",
        "description": "Interface eno1 receiving 150MB/s"
      },
      "startsAt": "2026-01-01T00:00:00Z",
      "endsAt": "0001-01-01T00:00:00Z",
      "generatorURL": "http://prometheus:9090",
      "fingerprint": "test123"
    }]
  }'

# View device inventory via API
curl -s http://127.0.0.1:8090/api/v1/devices | python3 -m json.tool

# Trigger a network scan via API
curl -s http://127.0.0.1:8090/api/v1/scan | python3 -m json.tool

# Restart a single service
docker compose restart grafana
```

## Troubleshooting

### ntopng crash-looping with PF_RING error

If you see `PF_RING version mismatch` in ntopng logs, the PF_RING kernel module is loaded and conflicts with the Docker image version. Fix:

```bash
sudo systemctl stop ntopng          # stop system ntopng if installed
sudo systemctl disable ntopng       # prevent it from starting on boot
sudo rmmod pf_ring                  # unload the kernel module
docker compose restart ntopng       # restart the Docker container
```

### Grafana not accessible via tunnel

Check that the port mapping is correct (3003 on host maps to 3000 in container). Verify with:
```bash
curl -s http://127.0.0.1:3003/api/health
```

### Cloudflare Tunnel DNS not resolving

- Free Cloudflare SSL only covers one level of subdomain (`*.ocheverse.ng`)
- Use flat format: `grafana-monitor.ocheverse.ng` not `grafana.monitor.ocheverse.ng`
- Flush local DNS: `sudo resolvectl flush-caches`
- New DNS records can take 1-5 minutes to propagate globally

### Throttle engine can't scan network

The throttle engine container runs with `network_mode: host` so nmap can see the LAN. If scans fail, ensure nmap is installed in the container (it's in the Dockerfile).

## Roadmap

### Phase 2 — OPNsense Integration
- Replace simulated throttle actions with real OPNsense API calls
- Automated traffic shaping based on device type and alert severity
- Rule lifecycle management (auto-apply, auto-expire)

### Phase 3 — Extended Observability
- **Loki + Promtail** — centralized log aggregation
- **cAdvisor** — per-container CPU/memory/network metrics
- **Blackbox Exporter** — probe uptime of external services and endpoints
- **SNMP Exporter** — pull metrics from router and managed switches

### Phase 4 — Automation & Intelligence
- **Automated device discovery** — scheduled ARP scans to populate inventory
- **Slack/Discord alerts** — notification channels in Alertmanager
- **Cloudflare Access** — zero-trust auth in front of all tunnel hostnames
- **Baseline learning** — establish normal traffic patterns per device, alert on anomalies

## Tech Stack

- **Go 1.22** — throttle engine
- **Docker Compose** — container orchestration
- **Prometheus + Alertmanager** — metrics and alerting
- **Grafana 11** — dashboards
- **Netdata** — real-time system monitoring
- **ntopng** — network traffic analysis
- **Cloudflare Tunnel** — secure public access without port forwarding
- **nmap** — network discovery
