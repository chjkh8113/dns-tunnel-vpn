# Troubleshooting Guide

## Common Issues

### 1. DNS Queries Not Reaching Server

**Symptoms:**
- `nslookup test.t.example.com YOUR_SERVER_IP` times out
- dnstt-client shows no session

**Solutions:**

1. Check if dnstt-server is running:
   ```bash
   systemctl status dnstt
   ```

2. Check iptables rules:
   ```bash
   iptables -t nat -L -n | grep 53
   ```

3. Verify DNS records propagated:
   ```bash
   dig NS t.example.com
   dig A tns.example.com
   ```

4. Test direct UDP to server:
   ```bash
   nc -u YOUR_SERVER_IP 53
   ```

---

### 2. WireGuard Handshake Fails

**Symptoms:**
- `sudo wg show` shows no handshake
- "Handshake did not complete" errors

**Solutions:**

1. Check wstunnel is running:
   ```bash
   systemctl status wstunnel
   ```

2. Verify keys match:
   - Server's `[Peer] PublicKey` should be client's public key
   - Client's `[Peer] PublicKey` should be server's public key

3. Check if port 51820 is listening locally:
   ```bash
   ss -ulnp | grep 51820
   ```

4. Test wstunnel connection:
   ```bash
   curl -v http://127.0.0.1:7000
   ```

---

### 3. Connected But No Internet

**Symptoms:**
- WireGuard shows successful handshake
- Cannot browse websites
- `curl ifconfig.me` fails

**Solutions:**

1. Check IP forwarding on server:
   ```bash
   cat /proc/sys/net/ipv4/ip_forward
   # Should be 1
   ```

2. Check NAT rules:
   ```bash
   iptables -t nat -L POSTROUTING -n
   ```

3. Check WireGuard interface:
   ```bash
   ip addr show wg0
   ```

4. Test connectivity from server:
   ```bash
   ping -I wg0 8.8.8.8
   ```

---

### 4. Very Slow Speeds

**Expected:** DNS tunnels are slow (~50-200 KB/s)

**Optimizations:**

1. Try different DoH resolvers:
   ```bash
   # Cloudflare
   -doh https://cloudflare-dns.com/dns-query

   # Google
   -doh https://dns.google/dns-query

   # Quad9
   -doh https://dns.quad9.net/dns-query
   ```

2. Reduce WireGuard MTU:
   ```ini
   [Interface]
   MTU = 1280
   ```

3. Use UDP mode instead of DoH (faster but less covert):
   ```bash
   dnstt-client -udp YOUR_SERVER_IP:53 ...
   ```

---

### 5. Connection Drops Frequently

**Solutions:**

1. Add keepalive to WireGuard:
   ```ini
   [Peer]
   PersistentKeepalive = 25
   ```

2. Increase dnstt timeout:
   - This requires modifying dnstt source code

3. Use a more reliable DoH resolver

4. Check server resources:
   ```bash
   htop
   journalctl -u dnstt -f
   ```

---

### 6. "Permission Denied" for WireGuard

**Solutions:**

1. Run with sudo:
   ```bash
   sudo wg-quick up wg-tunnel.conf
   ```

2. Fix config file permissions:
   ```bash
   chmod 600 ~/wireguard/wg-tunnel.conf
   ```

---

## Diagnostic Commands

### Server Side

```bash
# Check all services
systemctl status wg-quick@wg0 wstunnel dnstt

# View logs
journalctl -u dnstt -f
journalctl -u wstunnel -f

# Check ports
ss -tulnp | grep -E '53|5300|5555|51820'

# Check WireGuard
wg show

# Check iptables
iptables -L -n
iptables -t nat -L -n
```

### Client Side

```bash
# Check processes
ps aux | grep -E 'dnstt|wstunnel'

# Check local ports
lsof -i :7000
lsof -i :51820

# Check WireGuard
sudo wg show

# Test DNS tunnel
nslookup test.t.example.com 8.8.8.8
```

---

## Getting Help

1. Check logs first - they usually explain the issue
2. Test each component separately (dnstt, wstunnel, WireGuard)
3. Open an issue with:
   - Your OS version
   - Component versions
   - Relevant log output
   - Steps to reproduce
