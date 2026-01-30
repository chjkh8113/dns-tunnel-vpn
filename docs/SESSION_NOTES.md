# DNS Tunnel VPN - Session Notes

## Project Summary

Successfully set up a DNS tunnel using dnstt that forwards to SSH, allowing SOCKS5 proxy access through DNS queries. This bypasses network restrictions where only DNS traffic is allowed.

---

## What Worked

### 1. dnstt + SSH SOCKS Proxy
- **Status**: WORKING
- dnstt-server forwards to SSH port 22
- Client connects via dnstt-client, then uses `ssh -D 1080` for SOCKS proxy
- Tested successfully from both Windows and Mac

### 2. Direct UDP Mode
- **Status**: WORKING
- `-udp 188.40.147.153:53` works when direct connection to server is possible
- Best for testing and when DNS resolvers are unreliable

### 3. DNS Configuration (Cloudflare)
- **Status**: WORKING
- A record: `tns.rmdashrf.com` → `188.40.147.153`
- NS record: `t.rmdashrf.com` → `tns.rmdashrf.com`

---

## What Failed

### 1. DoH (DNS over HTTPS)
- **Status**: BLOCKED in restricted networks
- Error: `dial tcp 10.10.34.36:443` - Cloudflare being hijacked to private IP
- DoH resolvers (Cloudflare, Google, etc.) are blocked/hijacked

### 2. WireGuard over dnstt + wstunnel
- **Status**: FAILED - Protocol mismatch
- **Issue**: dnstt forwards raw TCP, but wstunnel expects WebSocket protocol
- **Error**: `Error while upgrading cnx to websocket: hyper::Error(Parse(Method))`
- **Lesson**: wstunnel and dnstt are incompatible without additional protocol translation

### 3. VPS2 Connection (176.65.243.222)
- **Status**: FAILED - Permission denied
- Tried: root@, ubuntu@
- SSH key not authorized on this server

### 4. Some DNS Resolvers
- Many resolvers return corrupted data
- "base32 decoding" errors indicate resolver modifying queries
- Need to use dnscan to find working resolvers

---

## Architecture

### Working Setup (SSH SOCKS)
```
[Client App] → [SOCKS 127.0.0.1:1080]
                      ↓
              [SSH -D 1080]
                      ↓
           [dnstt-client :7000]
                      ↓
              [DNS Queries]
                      ↓
            [DNS Resolver]
                      ↓
         [dnstt-server :5300]
                      ↓
              [SSH :22]
                      ↓
              [Internet]
```

### Failed Setup (WireGuard)
```
[WireGuard UDP] → [wstunnel] → [dnstt] ← PROTOCOL MISMATCH
```
- wstunnel expects WebSocket, dnstt sends raw TCP
- Would need udp2raw or similar for proper UDP-over-TCP

---

## Server Configuration

### Services Running
```bash
# dnstt-server (forwards to SSH)
/root/dnstt-server -udp :5300 -privkey-file /root/server.key t.rmdashrf.com 127.0.0.1:22

# iptables (redirect port 53 to 5300)
iptables -t nat -I PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-ports 5300
```

### WireGuard (installed but not used with dnstt)
- Installed and configured at `/etc/wireguard/wg0.conf`
- Works standalone, but incompatible with dnstt+wstunnel architecture

---

## Client Commands

### Mac/Linux
```bash
# Terminal 1: dnstt tunnel
./dnstt-client -udp WORKING_DNS_IP:53 \
  -pubkey 7eb6bd9d446c54ee03640f21c827bbca41e93aaabd09c74d28c8990d4472bf4c \
  t.rmdashrf.com 127.0.0.1:7000

# Terminal 2: SSH SOCKS proxy
ssh -i ~/.ssh/snowflake_key -p 7000 -D 1080 -N root@127.0.0.1

# Use proxy
curl --proxy socks5://127.0.0.1:1080 https://ifconfig.me
```

### Direct Test (bypass DNS resolver)
```bash
./dnstt-client -udp 188.40.147.153:53 \
  -pubkey 7eb6bd9d446c54ee03640f21c827bbca41e93aaabd09c74d28c8990d4472bf4c \
  t.rmdashrf.com 127.0.0.1:7000
```

---

## Tools & Resources

### Required Tools
| Tool | Purpose | URL |
|------|---------|-----|
| dnstt | DNS tunnel | https://www.bamsoftware.com/software/dnstt/ |
| WireGuard | VPN (optional) | https://www.wireguard.com/ |
| wstunnel | UDP over TCP (didn't work with dnstt) | https://github.com/erebe/wstunnel |

### DNS Scanner Tools
| Tool | Purpose | URL |
|------|---------|-----|
| dnscan | Find working DNS resolvers | https://github.com/nightowlnerd/dnscan |
| dnstt-resolver-probe | Test DNS for dnstt compatibility | https://github.com/FarazFe/dnstt-resolver-probe |

### Build Commands
```bash
# Build dnstt-server for Linux
cd dnstt/dnstt-server
GOOS=linux GOARCH=amd64 go build -o dnstt-server-linux

# Build dnstt-client for Windows
cd dnstt/dnstt-client
go build -o dnstt-client.exe

# Build dnstt-client for Mac
GOOS=darwin GOARCH=amd64 go build -o dnstt-client-macos
```

---

## Troubleshooting

### "base32 decoding" errors
- **Cause**: DNS resolver modifying/corrupting queries
- **Fix**: Use different DNS resolver, find working ones with dnscan

### Session starts but no streams
- **Cause**: DNS queries corrupted, SSH not reachable
- **Fix**: Test with direct UDP mode first, verify iptables

### DoH connection timeout
- **Cause**: DoH being blocked/hijacked in network
- **Fix**: Use UDP mode with working DNS resolver instead

### SSH permission denied
- **Cause**: Wrong SSH key or key not authorized
- **Fix**: Use correct key from .env file

---

## File Locations

### Server (188.40.147.153)
```
/root/dnstt-server          # dnstt binary
/root/server.key            # dnstt private key
/root/server.pub            # dnstt public key
/root/dnstt.log            # dnstt logs
/etc/wireguard/wg0.conf    # WireGuard config (not used)
/etc/wireguard/*.key       # WireGuard keys
```

### Local (Windows)
```
C:/Users/Administrator/snowflake/.env              # All credentials
C:/Users/Administrator/snowflake/snowflake_key     # SSH key
C:/Users/Administrator/dnstt/                      # dnstt source code
C:/Users/Administrator/dnstt/dnstt-client/         # Client binary
C:/Users/Administrator/dnstt/dnstt-server/         # Server binary
```

---

## GitHub Repository

- **URL**: https://github.com/chjkh8113/dns-tunnel-vpn
- **Collaborator**: ars1364
- **Contents**: Public documentation (no secrets)

---

## Next Steps / TODO

1. [ ] Find more working DNS resolvers for Iran using dnscan
2. [ ] Test DoT (DNS over TLS) as alternative to DoH
3. [ ] Consider alternative UDP-over-TCP solutions for WireGuard
4. [ ] Set up systemd services for auto-start on server reboot
5. [ ] Connect to VPS2 (need correct SSH key)
