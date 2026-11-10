//go:build !badlinkname

package vless

// Без badlinkname реестр Vision недоступен, и Vision поверх VLESS Encryption
// не собрать (см. vision_encryption.go).
const visionOverEncryption = false
