# Ocheverse Homelab — Infrastructure Documentation

Comprehensive documentation for the homelab and cloud infrastructure running under the `ocheverse.ng` domain.

## Architecture Overview

The infrastructure is split across two servers with distinct responsibilities:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Cloudflare (DNS + Tunnel)                     │
│         *.ocheverse.ng → Tunnel or A records                     │
└──────────┬──────────────────────────────────┬───────────────────┘
           │                                  │
    ┌──────▼──────┐                    ┌──────▼──────┐
    │  Homelab     │    Tailscale      │  Contabo    │
    │  192.168.1.200│◄════════════════►│  VPS        │
    │  Behind CGNAT │    Mesh VPN      │  Public IP  │
    │               │                  │             │
    │  Observability│                  │  Coolify    │
    │  Services     │                  │  Critical   │
    │  Dev Tools    │                  │  Services   │
    │  K3s Cluster  │                  │             │
    └──────────────┘                  └─────────────┘
```

**Homelab** (ochehomelab) — Ubuntu 24.04, 4 CPU, 7.6GB RAM, 914GB disk
- Monitoring, network analysis, dev tools, experiments
- Behind ISP CGNAT — accessed via Cloudflare Tunnel + Tailscale
- UFW firewall: LAN + Tailscale only

**Contabo VPS** — Ubuntu 24.04, 4 CPU, 7.8GB RAM, 145GB disk
- Production/critical services via Coolify
- Public IP with Traefik reverse proxy
- CrowdSec + Fail2ban for security

## Services — Homelab

### Monitoring Stack (homelab-observability)

| Service | Port | URL | Purpose |
|---------|------|-----|---------|
| Prometheus | 9090 | `prometheus-monitor.ocheverse.ng` | Metrics collection & alerting |
| Alertmanager | 9093 | Internal | Alert routing to webhooks |
| Grafana | 3003 | `grafana-monitor.ocheverse.ng` | Dashboards & visualization |
| Netdata | 19999 | `netdata-monitor.ocheverse.ng` | Real-time system monitoring |
| ntopng | 3002 | `ntopng-monitor.ocheverse.ng` | Network traffic analysis & DPI |
| Node Exporter | 9100 | Internal | Host metrics for Prometheus |
| Throttle Engine | 8090 | `throttle-monitor.ocheverse.ng` | Custom Go service: alerts, device inventory, network scanning |

**Compose file:** `~/homelab-observability/docker-compose.yml`

### Services Stack (homelab-services)

| Service | Port | URL | Purpose |
|---------|------|-----|---------|
| Homepage | 3010 | `home.ocheverse.ng` | Central dashboard |
| Forgejo | 3016 | `git.ocheverse.ng` | Git server + CI/CD (Forgejo Actions) |
| Forgejo DB | — | Internal | PostgreSQL for Forgejo |
| Vaultwarden | 3017 | `passwords.ocheverse.ng` | Password manager (Bitwarden-compatible) |
| IT-Tools | 3018 | `tools.ocheverse.ng` | Developer/sysadmin utilities |
| Ntfy | 3019 | `notify.ocheverse.ng` | Push notifications |
| Upsnap | 3020 | `wol.ocheverse.ng` | Wake-on-LAN dashboard |
| Authelia | 9091 | `auth.ocheverse.ng` | SSO & authentication gateway |
| Stirling PDF | 3012 | `pdf.ocheverse.ng` | PDF toolkit |
| Changedetection | 3013 | `changes.ocheverse.ng` | Website change monitoring |
| Speedtest Tracker | 3014 | `speedtest.ocheverse.ng` | ISP speed monitoring (every 2h) |
| Dozzle | 3015 | `logs.ocheverse.ng` | Real-time Docker log viewer |
| Trivy | 4954 | Internal | Container vulnerability scanner |
| Watchtower | — | — | Auto-updates containers daily at 4am |
| CrowdSec | 8080 | Internal | Community threat intelligence |

**Compose file:** `~/homelab-services/docker-compose.yml`

### Pre-existing Services

| Service | Port | URL |
|---------|------|-----|
| Uptime Kuma | 3001 | `status.ocheverse.ng` |
| qBittorrent | 8081 | `downloads.ocheverse.ng` |
| JDownloader | 5800 | `jdl.ocheverse.ng` |
| File Browser | 8088 | `files.ocheverse.ng` |
| Docker Registry | 5000 | `registry.ocheverse.ng` |
| Registry UI | 5001 | `registry-ui.ocheverse.ng` |
| Aria2 | 6800 | Internal |

## Services — Contabo (via Coolify)

| Service | URL | Purpose |
|---------|-----|---------|
| Coolify | `coolify.ocheverse.ng` | Deployment platform |
| Vault | `vault.ocheverse.ng` | HashiCorp Vault (secrets management) |
| Invoice Ninja | `invoice.ocheverse.ng` | Invoicing & billing |
| Odoo | `odoo.ocheverse.ng` | ERP & business suite |
| Remark42 | `comments.ocheverse.ng` | Comment system |
| Paperless-ngx | `docs.ocheverse.ng` | Document management |
| Uptime Kuma | `monitor.ocheverse.ng` | Cross-site monitoring (watches homelab) |
| CrowdSec | — | Threat detection |

All Contabo services are deployed and managed via Coolify with automatic SSL.

## Cross-Site Monitoring

Each server's Uptime Kuma monitors the other:

**Homelab Kuma** (`status.ocheverse.ng`) monitors:
- Vault, Coolify, Invoice Ninja, Odoo, Paperless, Contabo Monitor, Ocheverse, Kaima

**Contabo Kuma** (`monitor.ocheverse.ng`) monitors:
- Homepage, Grafana, Throttle Engine, Stirling PDF, Changedetection, Speedtest Tracker, File Browser, qBittorrent, JDownloader, Homelab Status, Ocheverse, Kaima

## Cloudflare Tunnel

The homelab is behind CGNAT — all public access goes through a Cloudflare Tunnel.

**Tunnel config:** `/etc/cloudflared/config.yml` (system service)
**Local copy:** `~/homelab-observability/cloudflared/config.yml`

```yaml
tunnel: YOUR_TUNNEL_UUID
credentials-file: /home/bpur/.cloudflared/YOUR_TUNNEL_UUID.json

ingress:
  - hostname: status.ocheverse.ng      → localhost:3001
  - hostname: registry.ocheverse.ng    → localhost:5000
  - hostname: registry-ui.ocheverse.ng → localhost:5001
  - hostname: jdl.ocheverse.ng         → localhost:5800
  - hostname: files.ocheverse.ng       → localhost:8088
  - hostname: downloads.ocheverse.ng   → localhost:8081
  - hostname: grafana-monitor          → localhost:3003
  - hostname: prometheus-monitor       → localhost:9090
  - hostname: netdata-monitor          → localhost:19999
  - hostname: ntopng-monitor           → localhost:3002
  - hostname: throttle-monitor         → localhost:8090
  - hostname: home                     → localhost:3010
  - hostname: pdf                      → localhost:3012
  - hostname: changes                  → localhost:3013
  - hostname: speedtest                → localhost:3014
  - hostname: logs                     → localhost:3015
  - hostname: git                      → localhost:3016
  - hostname: passwords                → localhost:3017
  - hostname: tools                    → localhost:3018
  - hostname: notify                   → localhost:3019
  - hostname: wol                      → localhost:3020
  - hostname: auth                     → localhost:9091
  - service: http_status:404
```

**Important:** Free Cloudflare SSL only covers `*.ocheverse.ng` (one level). Use flat subdomains like `grafana-monitor.ocheverse.ng`, not `grafana.monitor.ocheverse.ng`.

### Updating tunnel config

```bash
# Edit the local copy
nano ~/homelab-observability/cloudflared/config.yml

# Copy to system and restart
sudo cp ~/homelab-observability/cloudflared/config.yml /etc/cloudflared/config.yml
sudo systemctl restart cloudflared

# Create DNS record for new subdomain
cloudflared tunnel route dns oche-homelab SUBDOMAIN.ocheverse.ng
```

## UFW Firewall

```
Default: deny incoming, allow outgoing
22     ALLOW IN    192.168.1.0/24    # SSH from LAN
2222   ALLOW IN    192.168.1.0/24    # Forgejo SSH from LAN
*      ALLOW IN    192.168.1.0/24    # All LAN traffic
*      ALLOW IN    on tailscale0     # Tailscale VPN
```

Cloudflare Tunnel works because it's an outgoing connection — firewall doesn't block it.

## Throttle Engine

Custom Go service at `~/homelab-observability/throttle-engine/`.

**Features:**
- Web dashboard with live system gauges (CPU, memory, disk, connections)
- Bandwidth charts from Prometheus
- Network topology map
- Device inventory (add/edit/delete, persisted to YAML)
- Network scanner (nmap)
- Alertmanager webhook receiver
- Basic auth protected

**Auth:** Set via `THROTTLE_AUTH_USER` and `THROTTLE_AUTH_PASS` env vars in `.env`

**Alert flow:** Prometheus → Alertmanager → POST /api/v1/alert → Throttle Engine → log (future: OPNsense API)

## Trivy — Container Scanning

Runs as a server on port 4954. Scan images:

```bash
# Scan specific image
~/homelab-services/scripts/trivy-scan.sh nginx:latest

# Scan all running containers
~/homelab-services/scripts/trivy-scan.sh

# Critical/high only
~/homelab-services/scripts/trivy-scan.sh --critical-only
```

Sample CI workflow at `~/homelab-services/forgejo/sample-ci-workflow.yml` — copy to `.forgejo/workflows/ci.yml` in any Forgejo repo.

## Kubernetes (K3s)

K3s is installed as a single-node cluster running alongside Docker. Existing Docker Compose services are NOT migrated — K3s is for new workloads and learning.

```bash
# Check cluster
sudo k3s kubectl get nodes
sudo k3s kubectl get pods -A

# Copy kubeconfig for regular kubectl
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $(id -u):$(id -g) ~/.kube/config
```

## Startup Commands

```bash
# Monitoring stack
cd ~/homelab-observability && docker compose up -d

# Services stack
cd ~/homelab-services && docker compose up -d

# Cloudflare tunnel (system service)
sudo systemctl start cloudflared
```

## Useful Commands

```bash
# View all running containers
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'

# Live logs
docker compose logs -f <service>

# Reload Prometheus config
curl -X POST http://127.0.0.1:9090/-/reload

# Check Prometheus targets
curl -s http://127.0.0.1:9090/api/v1/targets | python3 -m json.tool

# Test throttle engine webhook
curl -X POST http://127.0.0.1:8090/api/v1/alert \
  -H "Content-Type: application/json" \
  -d '{"version":"4","status":"firing","alerts":[{"status":"firing","labels":{"alertname":"HighBandwidthReceive","device":"eno1","severity":"warning"},"annotations":{"summary":"Test alert"}}]}'

# Network scan via throttle engine
curl -s -u USER:PASS http://127.0.0.1:8090/api/v1/scan | python3 -m json.tool

# Trivy scan all containers
~/homelab-services/scripts/trivy-scan.sh --critical-only

# Check UFW status
sudo ufw status verbose

# Cloudflare tunnel status
cloudflared tunnel info oche-homelab
```

## Maintenance

### Automatic
- **Watchtower** — checks for container updates daily at 4:00 AM, auto-pulls and restarts
- **Speedtest Tracker** — runs ISP speed test every 2 hours
- **CrowdSec** — continuously monitors logs for threats on both servers
- **Prometheus** — retains 30 days of metrics data

### Manual periodic tasks
- Review CrowdSec decisions: `docker exec crowdsec cscli decisions list`
- Review Trivy scan results for new CVEs
- Update device inventory via throttle engine UI
- Check Uptime Kuma dashboards on both servers
- Review Grafana dashboards for anomalies

## Security Measures

| Layer | Tool | Scope |
|-------|------|-------|
| Firewall | UFW | Homelab — deny all except LAN + Tailscale |
| Threat Detection | CrowdSec | Both servers — community threat intelligence |
| Brute Force | Fail2ban | Contabo — SSH + service protection |
| Auth Gateway | Authelia | Homelab — SSO for services |
| Service Auth | Throttle Engine basic auth | Homelab — protects monitoring UI |
| Network Access | Cloudflare Tunnel | Homelab — no exposed ports |
| VPN | Tailscale | Both — private mesh network |
| Container Scanning | Trivy | Homelab — CVE detection in images |
| Password Management | Vaultwarden | Homelab — Bitwarden-compatible vault |
| Auto-updates | Watchtower | Homelab — keeps containers patched |

## DNS Records Overview

### Tunnel CNAMEs (→ YOUR_TUNNEL_UUID.cfargotunnel.com)
`status`, `registry`, `registry-ui`, `jdl`, `files`, `downloads`, `grafana-monitor`, `prometheus-monitor`, `netdata-monitor`, `ntopng-monitor`, `throttle-monitor`, `home`, `pdf`, `changes`, `speedtest`, `logs`, `git`, `passwords`, `tools`, `notify`, `wol`, `auth`

### A Records (→ YOUR_CONTABO_IP)
`vault`, `coolify`, `invoice`, `odoo`, `comments`, `docs`, `monitor`

## Network Topology

```
ISP (CGNAT)
    │
    ▼
ZTE Router (192.168.1.1)
    │
    ├── ochehomelab (192.168.1.200)
    │     ├── Docker: 25+ containers
    │     ├── K3s: single-node cluster
    │     └── Cloudflare Tunnel → public access
    │
    ├── Various devices (192.168.1.11-20)
    │     └── Tracked in throttle engine inventory
    │
    └── (Future: managed switch for port mirroring)
```

## Future Plans

### Phase 2 — Network Control
- OPNsense integration (VM or dedicated box)
- Throttle engine → real API calls for traffic shaping
- Managed switch with port mirroring for full traffic visibility
- AdGuard Home for DNS-level ad blocking

### Phase 3 — Second Server
- K3s multi-node cluster
- Longhorn distributed storage
- Move heavy workloads (media, LLMs) to second box
- Keep observability on primary server

### Phase 4 — Automation
- Ansible playbooks for full server provisioning
- Terraform for Cloudflare DNS/tunnel management
- ArgoCD for GitOps deployment to K3s
- Forgejo Actions → Registry → ArgoCD → K3s pipeline
