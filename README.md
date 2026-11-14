# awgm-box

A proxy platform for routers, built for [awg-manager](https://github.com/hoaxisr/awg-manager).

awgm-box is an independent fork of [sing-box](https://github.com/SagerNet/sing-box)
by nekohasekai and contributors. It is not affiliated with or endorsed by the
sing-box project. Changes from upstream are merged as needed.

## What is different

- AmneziaWG (AWG 3.x) endpoint and outbound
- mieru, TrustTunnel and MASQUE outbounds, XHTTP transport with Xray parity
- VLESS Encryption (mlkem768x25519plus) client, Vision over it
- finalmask, uTLS fingerprints and REALITY options matching Xray
- tun stack and memory fixes for MIPS and ARM routers (Entware builds)

The configuration format follows sing-box; its documentation at
https://sing-box.sagernet.org applies unless noted. For compatibility with
existing tooling some identifiers keep the upstream name: the Go module path,
the Clash API version string and log lines.

## Builds

Entware binaries for `mipsel-3.4`, `mips-3.4` and `aarch64-3.10` are published
as release assets `awgm-box-<version>-<arch>` by the `release-entware` workflow.
The version is a build counter (`41`); `awgm-box version` prints the upstream
sing-box release the build is based on on its second line.

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