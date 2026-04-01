# Homelab Infrastructure Documentation

Full documentation for a split homelab + cloud infrastructure. All services are accessible via Cloudflare Tunnel (homelab) or direct A records (cloud VPS).

> **Security note:** All real IPs, tokens, passwords, UUIDs, and credentials have been replaced with placeholders. The LAN range `192.168.1.0/24` is a private range and safe to reference.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                  Cloudflare (DNS + Tunnel)                    │
│           *.yourdomain → CNAME or A records                  │
└──────────┬──────────────────────────────────┬────────────────┘
           │ Tunnel (outbound)                │ A record
    ┌──────▼──────┐                    ┌──────▼──────┐
    │  Homelab     │    Tailscale      │  Cloud VPS  │
    │  LAN server  │◄════════════════►│  Public IP  │
    │  Behind CGNAT │    Mesh VPN      │             │
    │               │                  │  Coolify    │
    │  Monitoring   │                  │  Critical   │
    │  Dev tools    │                  │  Business   │
    │  K3s cluster  │                  │  Services   │
    │  Downloads    │                  │             │
    └──────────────┘                  └─────────────┘
```

The infrastructure is split by purpose:

- **Homelab** — experiments, monitoring, dev tools, media. Behind ISP CGNAT so no public IP. All access goes through a Cloudflare Tunnel (outbound connection from server to Cloudflare) or Tailscale VPN.
- **Cloud VPS** — production/critical services deployed via Coolify. Has a public IP with Traefik reverse proxy, automatic SSL, CrowdSec + Fail2ban.

Both servers are connected via Tailscale mesh VPN for private communication.

## Homelab Server

**OS:** Ubuntu 24.04 LTS | **CPU:** 4 cores | **RAM:** 7.6 GB | **Disk:** 914 GB

### Monitoring Stack

Purpose: Answer "is something stressing my infra?" and build toward automated network control.

| Service | What it does | Port |
|---------|-------------|------|
| **Prometheus** | Scrapes metrics from all services every 15s. Evaluates alert rules. Retains 30 days of data. | 9090 |
| **Alertmanager** | Receives alerts from Prometheus, deduplicates, groups, and routes them. Sends bandwidth alerts to the throttle engine webhook. | 9093 |
| **Grafana** | Pre-loaded dashboard showing CPU, memory, disk IO, network bandwidth per interface, disk space, and TCP connections. Prometheus is auto-configured as the default datasource. | 3003 |
| **Netdata** | Real-time system monitoring with 1-second granularity. Monitors all Docker containers via docker.sock. | 19999 |
| **ntopng** | Deep packet inspection and network traffic flow analysis. Runs on host network to see real traffic on the primary interface. Shows active connections, protocol breakdown, top talkers. | 3002 |
| **Node Exporter** | Exposes host-level metrics (CPU, memory, disk, network) to Prometheus. Runs with PID namespace and filesystem access for accurate readings. | 9100 |
| **Throttle Engine** | Custom Go service. Receives Alertmanager webhooks, identifies devices from inventory, logs alerts. Web dashboard with live system gauges, bandwidth charts, network topology map, device management (CRUD), and nmap network scanner. Basic auth protected. | 8090 |

**Alert rules defined:**
- `HighBandwidthReceive` — interface RX > 100MB/s for 2 minutes
- `HighBandwidthTransmit` — interface TX > 100MB/s for 2 minutes
- `HighCPUUsage` — CPU > 85% for 5 minutes
- `HighMemoryUsage` — memory > 90% for 5 minutes
- `DiskSpaceLow` — filesystem < 15% free for 10 minutes
- `HighDiskIO` — IO saturation > 90% for 5 minutes

**Alert flow:**
```
Prometheus → Alert Rules → Alertmanager → Webhook POST → Throttle Engine → Log
                                                          (future: OPNsense API)
```

**Compose file:** `~/homelab-observability/docker-compose.yml`

### Services Stack

| Service | What it does | Port |
|---------|-------------|------|
| **Homepage** | Central dashboard linking all services across both servers. Shows container status via docker.sock. Organized by category: Monitoring, Network, Media, Dev, Tools, Security, Cloud. | 3010 |
| **Forgejo** | Self-hosted Git server (Gitea fork). Forgejo Actions enabled for CI/CD — compatible with GitHub Actions workflow syntax. Uses PostgreSQL backend. SSH on port 2222 for git push. Registration disabled. | 3016 |
| **Forgejo DB** | PostgreSQL 16 database for Forgejo. | internal |
| **Forgejo Runner** | Executes CI/CD jobs for Forgejo Actions. Connects to Docker socket to run jobs in containers. | internal |
| **Vaultwarden** | Bitwarden-compatible password manager. Self-hosted, end-to-end encrypted. Signups disabled — manage users via admin panel. Works with all Bitwarden browser extensions and mobile apps. | 3017 |
| **IT-Tools** | 30+ developer and sysadmin utilities in one web UI. Base64 encode/decode, JWT debugger, hash generators, cron expression builder, regex tester, UUID generator, IP subnet calculator, and more. | 3018 |
| **Ntfy** | Push notification server. Send notifications from any script with a simple curl to your phone. Auth set to deny-all by default. | 3019 |
| **Upsnap** | Wake-on-LAN dashboard. Power on machines remotely from a web UI. Runs on host network for LAN access. | 3020 |
| **Authelia** | Single sign-on and authentication gateway. Provides 2FA and one-factor auth for services. Users defined in a YAML file with argon2id password hashes. | 9091 |
| **Stirling PDF** | PDF toolkit — merge, split, rotate, convert, compress, OCR, watermark, and more. No external API calls, everything processed locally. | 3012 |
| **Changedetection.io** | Monitors websites for changes. Tracks price drops, job postings, page content. Uses a headless Chrome browser (Playwright) for JavaScript-heavy sites. | 3013 |
| **Playwright Chrome** | Headless browser engine used by Changedetection to render JavaScript-heavy pages. | internal |
| **Speedtest Tracker** | Runs automated ISP speed tests every 2 hours and graphs the results over time. Useful for documenting ISP performance issues. SQLite backend. | 3014 |
| **Dozzle** | Real-time Docker container log viewer. Read-only access to docker.sock. No persistent storage — just streams live logs from all containers. | 3015 |
| **Trivy** | Container vulnerability scanner by Aqua Security. Runs as a server for faster repeated scans (caches vulnerability DB). Scans images for CVEs, misconfigurations, secrets, and license issues. | 4954 |
| **Watchtower** | Automatically checks for Docker image updates daily at 4:00 AM. Pulls new images, restarts containers, and cleans up old images. Monitors all containers on the host. | — |
| **CrowdSec** | Community-powered threat detection. Parses logs (SSH, nginx, system) and blocks known attackers. Shares threat data across all CrowdSec users — if someone attacks another user, your server blocks them preemptively. | 8080 |
| **Actual Budget** | Self-hosted budgeting app (YNAB alternative). Zero-based budgeting with envelope method. Sync across devices, import bank transactions. | 3021 |
| **Firefly III** | Personal finance manager. Track income, expenses, budgets, categories, piggy banks. PostgreSQL backend. Full reporting with charts and graphs. | 3022 |
| **Firefly DB** | PostgreSQL 16 database for Firefly III. | internal |

**Compose file:** `~/homelab-services/docker-compose.yml`

### Pre-existing Services

| Service | What it does | Port |
|---------|-------------|------|
| **Uptime Kuma** | Service uptime monitoring. Checks all services every 60 seconds. Monitors both homelab and cloud VPS services (cross-site). | 3001 |
| **qBittorrent** | Torrent client with web UI. | 8081 |
| **JDownloader** | Direct download manager with browser-based VNC UI. | 5800 |
| **File Browser** | Web-based file manager for browsing and managing server files. | 8088 |
| **Docker Registry** | Private container image registry for storing Docker images. | 5000 |
| **Registry UI** | Web UI for browsing the private Docker registry. | 5001 |
| **Aria2** | Download daemon with RPC interface. | 6800 |

### Kubernetes (K3s)

K3s is installed as a single-node lightweight Kubernetes cluster running alongside Docker. Existing Docker Compose services are not migrated — K3s is for new workloads and learning.

**Installed tools:**
- **kubectl** — Kubernetes CLI, kubeconfig at `~/.kube/config`
- **Helm v3** — Kubernetes package manager
- **k9s** — Terminal UI for Kubernetes cluster management

**Quick reference:**
```bash
kubectl get nodes              # Check cluster status
kubectl get pods -A            # All pods across namespaces
kubectl get svc -A             # All services
helm repo add <name> <url>     # Add a Helm chart repo
helm install <name> <chart>    # Deploy via Helm
k9s                            # Interactive terminal dashboard
```

K3s includes Traefik as its ingress controller and local-path-provisioner for persistent volumes.

## Cloud VPS (via Coolify)

**OS:** Ubuntu 24.04 LTS | **CPU:** 4 cores | **RAM:** 7.8 GB | **Disk:** 145 GB

All services deployed and managed via Coolify with automatic SSL via Traefik.

| Service | What it does |
|---------|-------------|
| **Coolify** | Self-hosted deployment platform (Heroku/Vercel alternative). Manages all VPS services, handles SSL, builds, and deployments. |
| **HashiCorp Vault** | Secrets management — stores and controls access to API keys, credentials, certificates. |
| **Invoice Ninja** | Invoicing, billing, and payment tracking for freelance/business use. |
| **Odoo** | Full ERP and business suite — CRM, accounting, inventory, project management. PostgreSQL backend. |
| **Remark42** | Self-hosted comment system for websites. Privacy-focused, no tracking. |
| **Paperless-ngx** | Document management system. Upload/scan documents, automatic OCR makes everything searchable. |
| **Uptime Kuma** | Cross-site monitoring instance. Watches all homelab services from outside to detect outages. |
| **CrowdSec** | Same threat detection as homelab — community intelligence, log parsing, attacker blocking. |
| **Fail2ban** | Brute force protection for SSH and exposed services. Active and enabled. |

## Cross-Site Monitoring

Each server runs an Uptime Kuma instance that monitors the other. If either goes down, the surviving instance knows.

**Homelab Kuma** monitors all cloud VPS services:
- Vault, Coolify, Invoice Ninja, Odoo, Paperless, Cloud Monitor, main domain, subdomains

**Cloud VPS Kuma** monitors all homelab services:
- Homepage, Grafana, Throttle Engine, PDF, Changedetection, Speedtest, File Browser, qBittorrent, JDownloader, Homelab Status, main domain, subdomains

Monitors check every 60 seconds with 3 retries before alerting.

## Cloudflare Tunnel

The homelab is behind ISP CGNAT — no public IP, no port forwarding possible. All public access goes through a Cloudflare Tunnel.

**How it works:** The server makes an outbound connection to Cloudflare's edge network. Cloudflare routes incoming requests for configured hostnames through this tunnel to the appropriate localhost port. No inbound firewall rules needed.

**Config location:** `/etc/cloudflared/config.yml` (system service)
**Local copy:** `~/homelab-observability/cloudflared/config.yml`

**Important notes:**
- Free Cloudflare SSL covers `*.yourdomain` but NOT `*.subdomain.yourdomain` (two levels deep). Use flat subdomains like `grafana-monitor.yourdomain` instead of `grafana.monitor.yourdomain`.
- Cloudflared runs as a systemd service, not a Docker container. This avoids conflicts with Docker networking.
- The tunnel is an outbound connection so UFW doesn't block it.

### Managing tunnel routes

```bash
# Add a new route
cloudflared tunnel route dns YOUR_TUNNEL_NAME subdomain.yourdomain

# Update the config
nano ~/homelab-observability/cloudflared/config.yml

# Deploy to system
sudo cp ~/homelab-observability/cloudflared/config.yml /etc/cloudflared/config.yml
sudo systemctl restart cloudflared
```

### DNS record types

- **Tunnel services (homelab):** CNAME records pointing to `YOUR_TUNNEL_UUID.cfargotunnel.com`
- **Direct services (cloud VPS):** A records pointing to the VPS public IP
- **Creating tunnel CNAMEs:** `cloudflared tunnel route dns YOUR_TUNNEL_NAME hostname`
- **Creating A records:** Done in Cloudflare dashboard or via API

## Firewall (UFW)

The homelab runs UFW with a deny-all-incoming default:

```
Default: deny incoming, allow outgoing

Rules:
- Allow all from 192.168.1.0/24     (LAN devices)
- Allow SSH (22) from 192.168.1.0/24
- Allow Forgejo SSH (2222) from 192.168.1.0/24
- Allow all on tailscale0            (Tailscale VPN)
```

**Why this works:**
- Cloudflare Tunnel is an outbound connection — firewall doesn't block it
- LAN devices can access all services directly
- Tailscale provides access from anywhere without opening ports
- Even if someone bypasses CGNAT, they hit a deny-all firewall

## Security Layers

| Layer | Tool | Where | What it does |
|-------|------|-------|-------------|
| Network | UFW | Homelab | Deny all except LAN + Tailscale |
| Network | Cloudflare Tunnel | Homelab | No exposed ports, outbound-only |
| Network | CGNAT | Homelab | ISP-level NAT, no public IP |
| VPN | Tailscale | Both | Private mesh network between servers and devices |
| Threat Intel | CrowdSec | Both | Community threat database, auto-blocking |
| Brute Force | Fail2ban | Cloud VPS | SSH and service protection |
| Auth | Authelia | Homelab | SSO gateway with 2FA support |
| Auth | Throttle Engine | Homelab | Basic auth on monitoring UI |
| Passwords | Vaultwarden | Homelab | Self-hosted Bitwarden password vault |
| Scanning | Trivy | Homelab | Container image CVE scanning |
| Updates | Watchtower | Homelab | Auto-patches containers daily |

## Throttle Engine (Custom Go Service)

A purpose-built Go service that serves as the brain of the monitoring stack.

**Source:** `~/homelab-observability/throttle-engine/`

### Architecture
```
throttle-engine/
├── main.go                          # HTTP server, route registration, auth wrapping
├── Dockerfile                       # Multi-stage build, includes nmap
├── config.yaml                      # Thresholds, server settings
└── internal/
    ├── config/config.go             # YAML config + env var overrides
    ├── handler/
    │   ├── auth.go                  # Basic auth middleware (skips healthz + webhook)
    │   ├── metrics.go               # Prometheus proxy for bandwidth/system charts
    │   ├── ui.go                    # Dashboard HTML, device CRUD, network scan
    │   └── webhook.go               # Alertmanager payload processing, alert history
    └── inventory/inventory.go       # Thread-safe device store with YAML persistence
```

### API Endpoints
| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/` | Yes | Web dashboard |
| POST | `/api/v1/alert` | No | Alertmanager webhook receiver |
| GET | `/api/v1/alerts` | Yes | Alert history (JSON) |
| GET | `/api/v1/stats` | Yes | Engine statistics |
| GET | `/api/v1/devices` | Yes | Full device inventory |
| PUT | `/api/v1/devices/update` | Yes | Add or update a device |
| DELETE | `/api/v1/devices/update` | Yes | Remove a device |
| GET | `/api/v1/scan` | Yes | Trigger nmap network scan |
| GET | `/api/v1/metrics/bandwidth` | Yes | Bandwidth data from Prometheus |
| GET | `/api/v1/metrics/system` | Yes | CPU/memory/disk/connections from Prometheus |
| GET | `/healthz` | No | Health check |

### Dashboard Features
- Live system gauges (CPU, memory, disk, TCP connections) with color-coded rings
- Bandwidth RX/TX line charts over the last hour from Prometheus
- Network topology map: router → server → all devices with online/offline status
- Device inventory management (add, edit, delete — persisted to YAML)
- Network scanner (nmap) with known vs unknown device identification
- Alert history table with device identification
- Auto-refresh every 10 seconds, auto-scan on page load

### Current vs Future Behavior
**Now:** Logs alerts, identifies devices, prints warnings. All action points marked with `FUTURE:` comments.
**Later (with OPNsense):** Call OPNsense API to apply traffic shaping rules, auto-remove after timeout.

## Container Scanning (Trivy)

Trivy runs as a server to cache the vulnerability database for faster repeated scans.

```bash
# Scan a specific image
~/homelab-services/scripts/trivy-scan.sh nginx:latest

# Scan all running containers
~/homelab-services/scripts/trivy-scan.sh

# Only critical and high severity
~/homelab-services/scripts/trivy-scan.sh --critical-only
```

A sample Forgejo Actions CI workflow is provided at `~/homelab-services/forgejo/sample-ci-workflow.yml`. It builds a Docker image, scans it with Trivy, and pushes to the private registry if clean.

## Device Inventory

The throttle engine maintains a YAML-based device inventory at `~/homelab-observability/inventory/devices.yaml`. Devices can be managed via the web UI or by editing the file directly.

```yaml
"192.168.1.1":
  name: "Router"
  owner: "ISP"
  type: "router"
  mac: "aa:bb:cc:dd:ee:ff"
  notes: "ISP gateway"
```

**Types:** laptop, phone, desktop, server, smart_tv, iot, game_console, router, printer, camera, guest, unknown

**Why this matters:**
- Alerts show device names instead of raw IPs
- Policy decisions based on device type (throttle guests, not servers)
- Anomaly detection (IoT device making outbound SSH = suspicious)
- Future OPNsense rules can be generated per device category

## Startup & Maintenance

### Starting everything

```bash
# Monitoring stack
cd ~/homelab-observability && docker compose up -d

# Services stack
cd ~/homelab-services && docker compose up -d

# Cloudflare tunnel (system service)
sudo systemctl start cloudflared

# K3s (system service)
sudo systemctl start k3s
```

### Automatic maintenance
- **Watchtower** — updates containers daily at 4:00 AM
- **Speedtest Tracker** — ISP speed test every 2 hours
- **CrowdSec** — continuous log monitoring on both servers
- **Prometheus** — 30-day metric retention
- **Cross-site monitoring** — 60-second checks on all services

### Useful commands

```bash
# All containers with status
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'

# Service logs
docker compose logs -f <service>

# Reload Prometheus without restart
curl -X POST http://127.0.0.1:9090/-/reload

# Check Prometheus scrape targets
curl -s http://127.0.0.1:9090/api/v1/targets | python3 -m json.tool

# Test throttle engine webhook
curl -X POST http://127.0.0.1:8090/api/v1/alert \
  -H "Content-Type: application/json" \
  -d '{"version":"4","status":"firing","alerts":[{"status":"firing","labels":{"alertname":"HighBandwidthReceive","device":"eno1","severity":"warning"},"annotations":{"summary":"Test alert"}}]}'

# Network scan
curl -s -u USER:PASS http://127.0.0.1:8090/api/v1/scan | python3 -m json.tool

# Trivy scan all containers
~/homelab-services/scripts/trivy-scan.sh --critical-only

# UFW status
sudo ufw status verbose

# K3s cluster
kubectl get nodes && kubectl get pods -A

# CrowdSec decisions
docker exec crowdsec cscli decisions list

# Tunnel status
cloudflared tunnel info YOUR_TUNNEL_NAME
```

## Troubleshooting

### Service not reachable via tunnel
1. Check the service responds locally: `curl http://localhost:PORT`
2. Check cloudflared config has the route: `grep HOSTNAME ~/homelab-observability/cloudflared/config.yml`
3. Check system config matches: `sudo cp ~/homelab-observability/cloudflared/config.yml /etc/cloudflared/config.yml && sudo systemctl restart cloudflared`
4. Check DNS: `dig +short HOSTNAME @1.1.1.1`

### Cloudflare 524 timeout
The origin server is too slow to respond. Usually happens during database migrations or heavy operations. Access the service directly via localhost instead.

### ntopng PF_RING crash
If ntopng crash-loops with PF_RING version mismatch:
```bash
sudo systemctl stop ntopng       # stop system ntopng
sudo systemctl disable ntopng    # prevent auto-start
sudo rmmod pf_ring               # unload kernel module
docker compose restart ntopng    # restart Docker container
```

### Docker pulls failing with TLS timeout
ISP congestion or throttling. Pull images one at a time or wait for off-peak hours:
```bash
docker pull IMAGE:TAG            # one at a time
docker compose pull              # all at once (may timeout)
```

## Future Plans

### Phase 2 — Network Control
- OPNsense integration (VM or dedicated hardware)
- Throttle engine calls OPNsense API for real traffic shaping
- Managed switch with port mirroring for full traffic visibility
- AdGuard Home for DNS-level ad/tracker blocking

### Phase 3 — Second Server
- K3s multi-node cluster across both machines
- Longhorn distributed storage
- Dedicate second server to heavy workloads (media, compute)
- Keep monitoring and networking on primary server

### Phase 4 — Automation & GitOps
- Ansible playbooks for full server provisioning from scratch
- Terraform for Cloudflare DNS and tunnel management as code
- ArgoCD for GitOps deployment to K3s
- Full pipeline: Forgejo → Forgejo Actions → Registry → ArgoCD → K3s
