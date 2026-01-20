#!/bin/bash
# Apply AWG patches to vendored amneziawg-go
# Patches:
#   1. Counter tag support (<c> obfuscation tag)
#   2. S4 keepalive padding fix (AWG 2.0 compatibility)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_AWG="vendor/github.com/amnezia-vpn/amneziawg-go/device"

# Check if vendor directory exists
if [ ! -d "$VENDOR_AWG" ]; then
    echo "Error: vendor directory not found. Run 'go mod vendor' first."
    exit 1
fi

echo "=== Applying AWG patches ==="

# Patch 1: Counter tag support
echo ""
echo "1. Adding counter tag support..."
# Copy obf_counter.go but remove the //go:build ignore lines (they're only for patches dir)
sed '/^\/\/go:build ignore/d; /^\/\/ +build ignore/d; /^\/\/ This file is a source template/d; /^\/\/ It gets copied to vendor/d; /^\/\/ by apply.sh script/d' \
    "$SCRIPT_DIR/obf_counter.go" > "$VENDOR_AWG/obf_counter.go"

OBF_GO="$VENDOR_AWG/obf.go"
if grep -q '"c":' "$OBF_GO"; then
    echo "   Counter tag already registered in obf.go, skipping..."
else
    echo "   Patching obf.go to register counter tag..."
    sed -i '/"b":.*newBytesObf/a\	"c":  newCounterObf,' "$OBF_GO"
fi

# Patch 2: S4 keepalive padding fix
# The send side only adds S4 padding to data packets, not keepalives.
# The receive side needs to handle both cases.
echo ""
echo "2. Applying S4 keepalive padding fix..."
RECEIVE_GO="$VENDOR_AWG/receive.go"

if grep -q "try without padding (for keepalives" "$RECEIVE_GO"; then
    echo "   S4 keepalive fix already applied, skipping..."
else
    echo "   Patching receive.go for S4 keepalive handling..."
    # Replace the transport detection block with fixed version
    # Original:
    #   if size >= padding+MessageTransportHeaderSize {
    #       data := packet[padding:]
    #       if header.Validate(binary.LittleEndian.Uint32(data)) {
    #           return MessageTransportType, padding
    #       }
    #   }
    # Fixed version adds fallback for keepalives without padding

    python3 - "$RECEIVE_GO" << 'PYTHON_SCRIPT'
import sys
import re

filename = sys.argv[1]
with open(filename, 'r') as f:
    content = f.read()

# Find and replace the transport detection block
old_block = '''	if expectedType == MessageUnknownType || expectedType == MessageTransportType {
		padding := device.paddings.transport
		header := device.headers.transport

		if size >= padding+MessageTransportHeaderSize {
			data := packet[padding:]
			if header.Validate(binary.LittleEndian.Uint32(data)) {
				return MessageTransportType, padding
			}
		}
	}'''

new_block = '''	if expectedType == MessageUnknownType || expectedType == MessageTransportType {
		padding := device.paddings.transport
		header := device.headers.transport

		// First try with S4 padding (for data packets)
		if size >= padding+MessageTransportHeaderSize {
			data := packet[padding:]
			if header.Validate(binary.LittleEndian.Uint32(data)) {
				return MessageTransportType, padding
			}
		}

		// If that fails, try without padding (for keepalives - see send.go line 575)
		// Keepalives don't get S4 padding, so check offset 0 as well
		if padding > 0 && size >= MessageTransportHeaderSize {
			if header.Validate(binary.LittleEndian.Uint32(packet)) {
				return MessageTransportType, 0
			}
		}
	}'''

if old_block in content:
    content = content.replace(old_block, new_block)
    with open(filename, 'w') as f:
        f.write(content)
    print("   Patch applied successfully")
else:
    print("   Warning: Could not find exact block to patch")
    print("   The file may have already been patched or modified")
PYTHON_SCRIPT
fi

echo ""
echo "=== All patches applied successfully! ==="
echo ""
echo "Supported obfuscation tags:"
echo "  <b HEX>  - static bytes"
echo "  <c>      - packet counter (4 bytes, big-endian)"
echo "  <t>      - unix timestamp (4 bytes)"
echo "  <r N>    - random bytes"
echo "  <rc N>   - random chars"
echo "  <rd N>   - random digits"
echo ""
echo "AWG 2.0 fixes:"
echo "  - S4 keepalive padding handled correctly"
