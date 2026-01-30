# DNS Tunnel VPN

A complete guide to setting up a **full VPN tunnel over DNS** using dnstt + WireGuard + wstunnel. This allows you to bypass network restrictions by encapsulating VPN traffic inside DNS queries.

## How It Works

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENT (Mac/Linux)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│  [Apps] → [WireGuard] → [wstunnel] → [dnstt-client] → [DoH Resolver]       │
│              UDP:51820      TCP:7000                    (Cloudflare)         │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      │ DNS Queries (encrypted)
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                                   SERVER (VPS)                               │
├─────────────────────────────────────────────────────────────────────────────┤
│  [dnstt-server] → [wstunnel] → [WireGuard] → [Internet]                    │
│      UDP:53         TCP:5555     UDP:51820                                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Features

- **Full VPN**: Routes ALL traffic (not just TCP like SOCKS5)
- **DNS Covert Channel**: Traffic looks like normal DNS queries
- **DoH Support**: Uses DNS-over-HTTPS for additional encryption
- **Bypasses Firewalls**: Works even when only DNS is allowed

## Components

| Component | Purpose |
|-----------|---------|
| [dnstt](https://www.bamsoftware.com/software/dnstt/) | DNS tunnel (encodes data in DNS queries) |
| [wstunnel](https://github.com/erebe/wstunnel) | UDP-over-TCP tunnel (bridges WireGuard to dnstt) |
| [WireGuard](https://www.wireguard.com/) | Fast, modern VPN |

## Prerequisites

- A VPS with root access
- A domain name with DNS control (Cloudflare recommended)
- Go 1.21+ (for building dnstt)

---

## Server Setup

### 1. DNS Configuration

Add these records to your DNS provider (e.g., Cloudflare):

| Type | Name | Value |
|------|------|-------|
| A | `tns.example.com` | `YOUR_SERVER_IP` |
| NS | `t.example.com` | `tns.example.com` |

> **Important**: The NS record delegates `t.example.com` to your server, making it the authoritative nameserver for that subdomain.

### 2. Install Dependencies

```bash
# Install WireGuard
apt update && apt install -y wireguard

# Install wstunnel
cd /tmp
wget https://github.com/erebe/wstunnel/releases/download/v10.1.7/wstunnel_10.1.7_linux_amd64.tar.gz
tar xzf wstunnel_10.1.7_linux_amd64.tar.gz
mv wstunnel /usr/local/bin/
chmod +x /usr/local/bin/wstunnel
```

### 3. Build dnstt-server

```bash
git clone https://www.bamsoftware.com/git/dnstt.git
cd dnstt/dnstt-server
go build
mv dnstt-server /usr/local/bin/
```

### 4. Generate Keys

```bash
# dnstt keys
cd /root
dnstt-server -gen-key -privkey-file dnstt-server.key -pubkey-file dnstt-server.pub
cat dnstt-server.pub  # Save this for clients

# WireGuard keys
cd /etc/wireguard
umask 077
wg genkey | tee server_private.key | wg pubkey > server_public.key
wg genkey | tee client_private.key | wg pubkey > client_public.key
```

### 5. Configure WireGuard Server

Create `/etc/wireguard/wg0.conf`:

```ini
[Interface]
Address = 10.66.66.1/24
ListenPort = 51820
PrivateKey = <SERVER_PRIVATE_KEY>
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE

[Peer]
PublicKey = <CLIENT_PUBLIC_KEY>
AllowedIPs = 10.66.66.2/32
```

Enable IP forwarding:
```bash
echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
sysctl -p
```

Start WireGuard:
```bash
systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0
```

### 6. Configure iptables for DNS

```bash
# Allow dnstt port
iptables -I INPUT -p udp --dport 5300 -j ACCEPT

# Redirect port 53 to dnstt
iptables -t nat -I PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-ports 5300
```

### 7. Start Services

Create `/etc/systemd/system/wstunnel.service`:

```ini
[Unit]
Description=wstunnel server
After=network.target

[Service]
ExecStart=/usr/local/bin/wstunnel server ws://0.0.0.0:5555 --restrict-to 127.0.0.1:51820
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/dnstt.service`:

```ini
[Unit]
Description=dnstt server
After=network.target wstunnel.service

[Service]
ExecStart=/usr/local/bin/dnstt-server -udp :5300 -privkey-file /root/dnstt-server.key t.example.com 127.0.0.1:5555
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Start services:
```bash
systemctl daemon-reload
systemctl enable wstunnel dnstt
systemctl start wstunnel dnstt
```

---

## Client Setup (macOS)

### 1. Install Dependencies

```bash
# Install WireGuard
brew install wireguard-tools

# Install wstunnel
brew install wstunnel

# Download dnstt-client (or build from source)
git clone https://www.bamsoftware.com/git/dnstt.git
cd dnstt/dnstt-client
go build
sudo mv dnstt-client /usr/local/bin/
```

### 2. Create WireGuard Config

Save as `~/wireguard/wg-tunnel.conf`:

```ini
[Interface]
PrivateKey = <CLIENT_PRIVATE_KEY>
Address = 10.66.66.2/24
DNS = 8.8.8.8

[Peer]
PublicKey = <SERVER_PUBLIC_KEY>
Endpoint = 127.0.0.1:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
```

### 3. Create Start Script

Save as `~/wireguard/start-tunnel.sh`:

```bash
#!/bin/bash

DNSTT_PUBKEY="<DNSTT_PUBLIC_KEY>"
TUNNEL_DOMAIN="t.example.com"
DOH_RESOLVER="https://cloudflare-dns.com/dns-query"

echo "[1/3] Starting dnstt-client..."
dnstt-client -doh "$DOH_RESOLVER" -pubkey "$DNSTT_PUBKEY" "$TUNNEL_DOMAIN" 127.0.0.1:7000 &
DNSTT_PID=$!
sleep 3

echo "[2/3] Starting wstunnel..."
wstunnel client -L udp://127.0.0.1:51820:127.0.0.1:51820 ws://127.0.0.1:7000 &
WSTUNNEL_PID=$!
sleep 2

echo "[3/3] Starting WireGuard..."
sudo wg-quick up ~/wireguard/wg-tunnel.conf

echo ""
echo "Tunnel is UP!"
echo "Press Ctrl+C to disconnect..."

trap "sudo wg-quick down ~/wireguard/wg-tunnel.conf; kill $WSTUNNEL_PID $DNSTT_PID 2>/dev/null; echo 'Disconnected.'" EXIT
wait
```

Make it executable:
```bash
chmod +x ~/wireguard/start-tunnel.sh
```

### 4. Connect

```bash
~/wireguard/start-tunnel.sh
```

### 5. Verify Connection

```bash
# Check WireGuard status
sudo wg show

# Check your public IP (should be VPS IP)
curl ifconfig.me
```

---

## Client Setup (Linux)

Same as macOS, but install dependencies with:

```bash
# Debian/Ubuntu
sudo apt install wireguard-tools

# Download wstunnel
wget https://github.com/erebe/wstunnel/releases/download/v10.1.7/wstunnel_10.1.7_linux_amd64.tar.gz
tar xzf wstunnel_10.1.7_linux_amd64.tar.gz
sudo mv wstunnel /usr/local/bin/
```

---

## Client Setup (Android)

For Android, use the WireGuard app with a proxy:

1. Install [WireGuard](https://play.google.com/store/apps/details?id=com.wireguard.android)
2. Use a DNS tunnel app like [Intra](https://getintra.org/) or similar
3. Configure WireGuard to use `127.0.0.1:51820` as endpoint

> Note: Full Android setup requires additional apps for the DNS tunnel layer.

---

## Troubleshooting

### DNS not resolving
```bash
# Test DNS directly to server
nslookup test.t.example.com YOUR_SERVER_IP

# Check if dnstt is running
systemctl status dnstt
```

### WireGuard handshake fails
```bash
# Check wstunnel is running
systemctl status wstunnel

# Check WireGuard logs
journalctl -u wg-quick@wg0 -f
```

### Slow speeds
- DNS tunnels are inherently slow (~50-200 KB/s)
- Try different DoH resolvers
- Reduce MTU in WireGuard config

### Connection drops
- Add `PersistentKeepalive = 25` to WireGuard config
- Check server logs for errors

---

## Alternative DoH Resolvers

| Provider | URL |
|----------|-----|
| Cloudflare | `https://cloudflare-dns.com/dns-query` |
| Google | `https://dns.google/dns-query` |
| Quad9 | `https://dns.quad9.net/dns-query` |
| AdGuard | `https://dns.adguard.com/dns-query` |

---

## Security Notes

- Keep your private keys secure
- Use strong, unique keys for each client
- The DNS tunnel encrypts data, but metadata (query patterns) may be visible
- For maximum security, combine with Tor or additional encryption

---

## Related Tools

- [dnscan](https://github.com/nightowlnerd/dnscan) - Find working DNS resolvers
- [dnstt-resolver-probe](https://github.com/FarazFe/dnstt-resolver-probe) - Test DNS resolvers for dnstt compatibility

---

## License

MIT License - See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request.

## Acknowledgments

- [dnstt](https://www.bamsoftware.com/software/dnstt/) by David Fifield
- [wstunnel](https://github.com/erebe/wstunnel) by erebe
- [WireGuard](https://www.wireguard.com/) by Jason Donenfeld
