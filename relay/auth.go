package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
)

// verifyAuth validates the ECDSA P-256 signature a host sends when connecting.
//
// The signed payload is "ts.serverId.role.connectionId" and the signature is
// the raw concatenation of r and s (each 32 bytes for P-256). The public key
// travels alongside the signature as a base64url uncompressed point. This is
// an optional defense-in-depth layer: enable with RELAY_VERIFY_AUTH=true. The
// application traffic itself is always end-to-end encrypted by the peers.
func verifyAuth(serverId, role, connectionId, tsStr, sigStr, pkStr string) bool {
	if tsStr == "" || sigStr == "" || pkStr == "" {
		return false
	}
	pkBytes, err := base64.RawURLEncoding.DecodeString(pkStr)
	if err != nil {
		return false
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return false
	}
	x, y, err := parseUncompressedPubKey(pkBytes)
	if err != nil {
		return false
	}
	payload := fmt.Sprintf("%s.%s.%s.%s", tsStr, serverId, role, connectionId)
	hash := sha256.Sum256([]byte(payload))
	sigLen := len(sigBytes)
	if sigLen%2 != 0 || sigLen < 64 {
		return false
	}
	half := sigLen / 2
	rVal := new(big.Int).SetBytes(sigBytes[:half])
	sVal := new(big.Int).SetBytes(sigBytes[half:])
	return ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, hash[:], rVal, sVal)
}

// parseUncompressedPubKey decodes a 65-byte SEC 1 uncompressed P-256 public
// key (0x04 || X || Y) and verifies the point is on the curve.
func parseUncompressedPubKey(raw []byte) (*big.Int, *big.Int, error) {
	if len(raw) != 65 || raw[0] != 0x04 {
		return nil, nil, fmt.Errorf("invalid uncompressed public key")
	}
	x := new(big.Int).SetBytes(raw[1:33])
	y := new(big.Int).SetBytes(raw[33:65])
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, nil, fmt.Errorf("point not on curve")
	}
	return x, y, nil
}
