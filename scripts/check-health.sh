#!/usr/bin/env bash
# Health check for all homelab-observability services
# Run after docker compose up -d to verify everything is working

set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

check_service() {
    local name="$1"
    local url="$2"
    local expected="${3:-200}"

    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$url" 2>/dev/null || echo "000")

    if [ "$code" = "$expected" ]; then
        echo -e "  ${GREEN}[ok]${NC}   $name ($url) → $code"
    else
        echo -e "  ${RED}[fail]${NC} $name ($url) → $code (expected $expected)"
    fi
}

echo "=== Service Health Check ==="
echo ""

# Check containers are running
echo "Container status:"
docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null || echo "  [warn] Could not get container status"
echo ""

echo "Endpoint checks:"
check_service "Prometheus"      "http://127.0.0.1:9090/-/healthy"
check_service "Alertmanager"    "http://127.0.0.1:9093/-/healthy"
check_service "Grafana"         "http://127.0.0.1:3003/api/health"
check_service "Netdata"         "http://127.0.0.1:19999/api/v1/info"
check_service "ntopng"          "http://127.0.0.1:3002"          "302"
check_service "Throttle Engine" "http://127.0.0.1:8090/healthz"

echo ""
echo "Prometheus targets:"
curl -s "http://127.0.0.1:9090/api/v1/targets" 2>/dev/null | \
    python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    for t in data.get('data',{}).get('activeTargets',[]):
        status = t.get('health','unknown')
        job = t.get('labels',{}).get('job','?')
        color = '\033[0;32m' if status == 'up' else '\033[0;31m'
        print(f'  {color}[{status}]\033[0m {job} → {t.get(\"scrapeUrl\",\"?\")}')
except:
    print('  [warn] Could not parse Prometheus targets')
" 2>/dev/null || echo "  [warn] Could not reach Prometheus"

echo ""
echo "=== Done ==="
