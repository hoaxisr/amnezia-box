# amnezia-box

Fork of [sing-box](https://github.com/SagerNet/sing-box) with [AmneziaWG](https://docs.amnezia.org/documentation/amnezia-wg/) (AWG) support.

> This is a fork of a fork: sing-box → [amnezia-vpn/sing-box](https://github.com/amnezia-vpn/sing-box) → amnezia-box

## Features

- Full sing-box functionality
- AmneziaWG protocol support via [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go)
- AWG 2.0 features: H1-H4 ranges, S3/S4 padding, I1-I5 obfuscation chains
- Proper FakeIP and DNS routing support for AWG endpoint

## AWG Endpoint DNS Resolution Fix

The AWG endpoint implementation includes a fix for proper domain name resolution when used with sing-box's FakeIP or standard DNS routing.

### Problem Solved

The original AWG endpoint did not properly handle domain resolution:
- FakeIP addresses (198.18.x.x) were not being resolved back to real IPs
- Domain resolution failed inside netstack mode
- Clash API delay tests returned errors

### Solution

AWG endpoint now overrides `DialContext()` and `ListenPacket()` methods to:
1. Check if destination is a domain (FQDN)
2. Use `dnsRouter.Lookup()` to resolve domains to real IPs
3. Connect through the tunnel using resolved addresses

This aligns AWG endpoint behavior with the standard WireGuard endpoint implementation.

## Build

```bash
go build -tags "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_awg" ./cmd/sing-box
```

## Branch Strategy

- `alpha` → syncs with upstream `dev-next` (development)
- `main` → syncs with upstream `stable-next` (stable releases)

## Documentation

https://sing-box.sagernet.org

## License

```
Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
```