#!/bin/bash
# Server Setup Script for DNS Tunnel VPN
# Run as root on your VPS

set -e

echo "=== DNS Tunnel VPN Server Setup ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root"
    exit 1
fi

# Configuration - EDIT THESE
TUNNEL_DOMAIN="t.example.com"  # Your tunnel domain

echo "[1/7] Installing dependencies..."
apt update
apt install -y wireguard wget

echo "[2/7] Installing wstunnel..."
cd /tmp
wget -q https://github.com/erebe/wstunnel/releases/download/v10.1.7/wstunnel_10.1.7_linux_amd64.tar.gz
tar xzf wstunnel_10.1.7_linux_amd64.tar.gz
mv wstunnel /usr/local/bin/
chmod +x /usr/local/bin/wstunnel

echo "[3/7] Building dnstt-server..."
apt install -y golang-go git
cd /tmp
git clone https://www.bamsoftware.com/git/dnstt.git
cd dnstt/dnstt-server
go build
mv dnstt-server /usr/local/bin/

echo "[4/7] Generating keys..."
cd /root

# dnstt keys
/usr/local/bin/dnstt-server -gen-key -privkey-file dnstt-server.key -pubkey-file dnstt-server.pub

# WireGuard keys
mkdir -p /etc/wireguard
cd /etc/wireguard
umask 077
wg genkey | tee server_private.key | wg pubkey > server_public.key
wg genkey | tee client_private.key | wg pubkey > client_public.key

echo "[5/7] Configuring WireGuard..."
SERVER_PRIVKEY=$(cat /etc/wireguard/server_private.key)
CLIENT_PUBKEY=$(cat /etc/wireguard/client_public.key)

cat > /etc/wireguard/wg0.conf << EOF
[Interface]
Address = 10.66.66.1/24
ListenPort = 51820
PrivateKey = ${SERVER_PRIVKEY}
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE

[Peer]
PublicKey = ${CLIENT_PUBKEY}
AllowedIPs = 10.66.66.2/32
EOF

# Enable IP forwarding
echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
sysctl -p

echo "[6/7] Configuring iptables..."
iptables -I INPUT -p udp --dport 5300 -j ACCEPT
iptables -t nat -I PREROUTING -i eth0 -p udp --dport 53 -j REDIRECT --to-ports 5300

echo "[7/7] Creating systemd services..."

cat > /etc/systemd/system/wstunnel.service << 'EOF'
[Unit]
Description=wstunnel server
After=network.target

[Service]
ExecStart=/usr/local/bin/wstunnel server ws://0.0.0.0:5555 --restrict-to 127.0.0.1:51820
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/dnstt.service << EOF
[Unit]
Description=dnstt server
After=network.target wstunnel.service

[Service]
ExecStart=/usr/local/bin/dnstt-server -udp :5300 -privkey-file /root/dnstt-server.key ${TUNNEL_DOMAIN} 127.0.0.1:5555
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable wg-quick@wg0 wstunnel dnstt
systemctl start wg-quick@wg0 wstunnel dnstt

echo ""
echo "=== Setup Complete ==="
echo ""
echo "IMPORTANT - Save these values for client configuration:"
echo ""
echo "DNSTT Public Key:"
cat /root/dnstt-server.pub
echo ""
echo "WireGuard Server Public Key:"
cat /etc/wireguard/server_public.key
echo ""
echo "WireGuard Client Private Key:"
cat /etc/wireguard/client_private.key
echo ""
echo "Tunnel Domain: ${TUNNEL_DOMAIN}"
echo ""
echo "Don't forget to configure DNS records!"
echo "  A    tns.yourdomain.com  ->  YOUR_SERVER_IP"
echo "  NS   t.yourdomain.com    ->  tns.yourdomain.com"
