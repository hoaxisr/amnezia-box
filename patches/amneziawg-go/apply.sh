#!/bin/bash
# Apply AWG counter tag patch to vendored amneziawg-go
# This script adds support for <c> (packet counter) obfuscation tag

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_AWG="vendor/github.com/amnezia-vpn/amneziawg-go/device"

# Check if vendor directory exists
if [ ! -d "$VENDOR_AWG" ]; then
    echo "Error: vendor directory not found. Run 'go mod vendor' first."
    exit 1
fi

# Copy obf_counter.go
echo "Adding obf_counter.go..."
cp "$SCRIPT_DIR/obf_counter.go" "$VENDOR_AWG/obf_counter.go"

# Patch obf.go to register the counter tag
OBF_GO="$VENDOR_AWG/obf.go"

if grep -q '"c":' "$OBF_GO"; then
    echo "Counter tag already registered in obf.go, skipping..."
else
    echo "Patching obf.go to register counter tag..."
    # Insert "c": newCounterObf after "b": newBytesObf
    sed -i '/"b":.*newBytesObf/a\	"c":  newCounterObf,' "$OBF_GO"
fi

echo "Patch applied successfully!"
echo ""
echo "Supported tags now include:"
echo "  <b HEX>  - static bytes"
echo "  <c>      - packet counter (4 bytes, big-endian)"
echo "  <t>      - unix timestamp (4 bytes)"
echo "  <r N>    - random bytes"
echo "  <rc N>   - random chars"
echo "  <rd N>   - random digits"
