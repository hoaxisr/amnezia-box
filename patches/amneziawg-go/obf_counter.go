package device

import (
	"encoding/binary"
	"sync/atomic"
)

// newCounterObf creates a packet counter obfuscator.
// The <c> tag outputs a 4-byte counter in network byte order (big-endian).
// Counter starts at 0 and increments with each Obfuscate() call.
// This matches the AWG kernel module implementation.
func newCounterObf(_ string) (obf, error) {
	return &counterObf{}, nil
}

type counterObf struct {
	counter uint32
}

// Obfuscate writes the current counter value (4 bytes, big-endian)
// and increments it atomically.
func (o *counterObf) Obfuscate(dst, src []byte) {
	// Get current value and increment atomically
	val := atomic.AddUint32(&o.counter, 1) - 1
	binary.BigEndian.PutUint32(dst, val)
}

// Deobfuscate always returns true - counter values are not validated.
// The receiving side doesn't know the expected counter value.
func (o *counterObf) Deobfuscate(dst, src []byte) bool {
	return true
}

// ObfuscatedLen returns 4 (counter is always 4 bytes).
func (o *counterObf) ObfuscatedLen(n int) int {
	return 4
}

// DeobfuscatedLen returns 0 (counter doesn't contribute to original data).
func (o *counterObf) DeobfuscatedLen(n int) int {
	return 0
}
