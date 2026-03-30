#!/usr/bin/env bash
# homelab-observability setup script
# Run this once before docker compose up

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=== homelab-observability setup ==="

# 1. Create .env from example if it doesn't exist
if [ ! -f "$PROJECT_DIR/.env" ]; then
    cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"
    echo "[!] Created .env from .env.example"
    echo "    EDIT .env NOW before starting services"
    echo "    At minimum, set:"
    echo "      - CF_TUNNEL_UUID"
    echo "      - GF_ADMIN_PASSWORD"
    echo "      - HOST_NETWORK_INTERFACE (run 'ip link' to find it)"
else
    echo "[ok] .env already exists"
fi

# 2. Check for cloudflared credentials
CREDS_FILE="$PROJECT_DIR/cloudflared/credentials.json"
if [ ! -f "$CREDS_FILE" ]; then
    echo ""
    echo "[!] Missing: cloudflared/credentials.json"
    echo "    Copy your tunnel credentials file here:"
    echo "    cp ~/.cloudflared/<TUNNEL_UUID>.json $CREDS_FILE"
    echo ""
    echo "    To find your tunnel UUID:"
    echo "    cloudflared tunnel list"
else
    echo "[ok] cloudflared credentials found"
fi

# 3. Detect the primary network interface
PRIMARY_IF=$(ip route | grep default | awk '{print $5}' | head -1)
echo ""
echo "[info] Detected primary network interface: $PRIMARY_IF"
echo "       Make sure HOST_NETWORK_INTERFACE=$PRIMARY_IF in your .env"

# 4. Check Docker
if ! command -v docker &>/dev/null; then
    echo "[error] Docker not found. Install Docker first."
    exit 1
fi

if ! docker compose version &>/dev/null; then
    echo "[error] Docker Compose V2 not found."
    exit 1
fi

echo "[ok] Docker and Compose found"

# 5. Check if required ports are free
echo ""
echo "[info] Checking ports..."
for port in 3000 3001 8090 9090 9093 19999; do
    if ss -tlnp | grep -q ":$port "; then
        echo "  [warn] Port $port is already in use"
    else
        echo "  [ok]   Port $port is free"
    fi
done

echo ""
echo "=== Setup complete ==="
echo "Next steps:"
echo "  1. Edit .env with your values"
echo "  2. Place cloudflared credentials.json"
echo "  3. Run: docker compose up -d"
echo "  4. Run: ./scripts/check-health.sh"
