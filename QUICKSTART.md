# Quick Start Reference

## Server Info
- **IP**: 188.40.147.153
- **Domain**: t.rmdashrf.com
- **dnstt pubkey**: `7eb6bd9d446c54ee03640f21c827bbca41e93aaabd09c74d28c8990d4472bf4c`

## Connect (Mac/Linux)

### Terminal 1 - Tunnel
```bash
./dnstt-client -udp WORKING_DNS:53 \
  -pubkey 7eb6bd9d446c54ee03640f21c827bbca41e93aaabd09c74d28c8990d4472bf4c \
  t.rmdashrf.com 127.0.0.1:7000
```

### Terminal 2 - SSH SOCKS
```bash
ssh -i ~/.ssh/snowflake_key -p 7000 -D 1080 -N root@127.0.0.1
```

### Use Proxy
- SOCKS5: `127.0.0.1:1080`
- Test: `curl --proxy socks5://127.0.0.1:1080 https://ifconfig.me`

## Find Working DNS
```bash
./dnscan --country ir --domain t.rmdashrf.com --mode list
```

## Direct Test (bypass resolver)
```bash
./dnstt-client -udp 188.40.147.153:53 -pubkey 7eb6bd9d446c54ee03640f21c827bbca41e93aaabd09c74d28c8990d4472bf4c t.rmdashrf.com 127.0.0.1:7000
```

## Server Commands
```bash
# Check dnstt status
ssh root@188.40.147.153 "ps aux | grep dnstt"

# View logs
ssh root@188.40.147.153 "tail -50 /root/dnstt.log"

# Restart dnstt
ssh root@188.40.147.153 "pkill dnstt-server; nohup /root/dnstt-server -udp :5300 -privkey-file /root/server.key t.rmdashrf.com 127.0.0.1:22 > /root/dnstt.log 2>&1 &"
```

## Important Files
- `.env` - All credentials (KEEP SECURE)
- `docs/SESSION_NOTES.md` - Detailed documentation
- SSH key: See .env file
