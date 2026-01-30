#!/bin/bash
# Client Connection Script for DNS Tunnel VPN
# For macOS and Linux

set -e

# Configuration - EDIT THESE
DNSTT_PUBKEY="<DNSTT_PUBLIC_KEY>"
TUNNEL_DOMAIN="t.example.com"
DOH_RESOLVER="https://cloudflare-dns.com/dns-query"
WG_CONFIG="$HOME/wireguard/wg-tunnel.conf"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== DNS Tunnel VPN Client ===${NC}"
echo ""

# Check dependencies
check_command() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}Error: $1 is not installed${NC}"
        exit 1
    fi
}

check_command dnstt-client
check_command wstunnel
check_command wg-quick

# Check WireGuard config exists
if [ ! -f "$WG_CONFIG" ]; then
    echo -e "${RED}Error: WireGuard config not found at $WG_CONFIG${NC}"
    echo "Create it first using the example config."
    exit 1
fi

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Disconnecting...${NC}"
    sudo wg-quick down "$WG_CONFIG" 2>/dev/null || true
    kill $WSTUNNEL_PID 2>/dev/null || true
    kill $DNSTT_PID 2>/dev/null || true
    echo -e "${GREEN}Disconnected.${NC}"
}

trap cleanup EXIT INT TERM

echo -e "${YELLOW}[1/3] Starting dnstt-client...${NC}"
dnstt-client -doh "$DOH_RESOLVER" -pubkey "$DNSTT_PUBKEY" "$TUNNEL_DOMAIN" 127.0.0.1:7000 &
DNSTT_PID=$!
sleep 3

# Check if dnstt started successfully
if ! kill -0 $DNSTT_PID 2>/dev/null; then
    echo -e "${RED}Error: dnstt-client failed to start${NC}"
    exit 1
fi

echo -e "${YELLOW}[2/3] Starting wstunnel...${NC}"
wstunnel client -L udp://127.0.0.1:51820:127.0.0.1:51820 ws://127.0.0.1:7000 &
WSTUNNEL_PID=$!
sleep 2

# Check if wstunnel started successfully
if ! kill -0 $WSTUNNEL_PID 2>/dev/null; then
    echo -e "${RED}Error: wstunnel failed to start${NC}"
    exit 1
fi

echo -e "${YELLOW}[3/3] Starting WireGuard...${NC}"
sudo wg-quick up "$WG_CONFIG"

echo ""
echo -e "${GREEN}=== Tunnel is UP ===${NC}"
echo ""
echo "Your traffic is now routed through the DNS tunnel."
echo ""
echo -e "Press ${YELLOW}Ctrl+C${NC} to disconnect..."
echo ""

# Wait for user interrupt
wait
