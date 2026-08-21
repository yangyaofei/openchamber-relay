package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"time"
)

// authTolerance is how far a signed timestamp may drift from relay clock
// time. Replay-protection window for the host handshake.
const authTolerance = 5 * time.Minute

// verifyAuth validates the relay-layer auth a host sends when connecting and
// enforces a replay window. See verifyAuthCrypto for the protocol details.
func verifyAuth(serverId, role, connectionId, tsStr, sigStr, pkStr string) bool {
	var ts int64
	if _, err := fmt.Sscanf(tsStr, "%d", &ts); err != nil {
		return false
	}
	drift := time.Since(time.UnixMilli(ts))
	if drift < 0 {
		drift = -drift
	}
	if drift > authTolerance {
		log.Printf("[relay] auth ts out of tolerance: %s (drift %v)", tsStr, drift)
		return false
	}
	return verifyAuthCrypto(serverId, role, connectionId, tsStr, sigStr, pkStr)
}

// verifyAuthCrypto validates the relay-layer auth a host sends when connecting,
// matching the upstream OpenChamber protocol exactly
// (packages/web/server/lib/relay/{identity,signing-key,host-client}.js):
//
//   - pk  = base64url(UTF-8 JSON string of the canonical public JWK,
//     i.e. `{"crv":...,"kty":...,"x":...,"y":...}` with fixed key order)
//   - serverId = base64url(SHA-256(canonical public JWK string))
//   - sig  = base64url(ECDSA-SHA256 over "{ts}.{serverId}.{role}.{connectionId}",
//     IEEE P1363 raw r||s encoding)
//
// The check proves the peer holds the private key belonging to serverId, so
// a stranger cannot squat a serverId on this relay. Application traffic is
// always end-to-end encrypted regardless; this is an anti-impersonation /
// anti-squat layer only. Enable with RELAY_VERIFY_AUTH=true.
func verifyAuthCrypto(serverId, role, connectionId, tsStr, sigStr, pkStr string) bool {
	// 1. Decode and parse the canonical public JWK.
	pkBytes, err := base64.RawURLEncoding.DecodeString(pkStr)
	if err != nil {
		return false
	}
	var jwk struct {
		Crv string `json:"crv"`
		Kty string `json:"kty"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal(pkBytes, &jwk); err != nil {
		return false
	}
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return false
	}
	xb, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil || len(xb) != 32 {
		return false
	}
	yb, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil || len(yb) != 32 {
		return false
	}
	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)
	if !elliptic.P256().IsOnCurve(x, y) {
		return false
	}

	// 2. serverId binding: SHA-256(canonical JWK string) must equal serverId.
	// The decoded pkBytes ARE the canonical JWK string bytes, so this is both
	// the identity check and protection against squatted serverIds.
	sum := sha256.Sum256(pkBytes)
	expectedId := base64.RawURLEncoding.EncodeToString(sum[:])
	if expectedId != serverId {
		return false
	}

	// 3. Signature over "{ts}.{serverId}.{role}.{connectionId}", P1363 r||s.
	sigBytes, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil || len(sigBytes) != 64 {
		return false
	}
	payload := fmt.Sprintf("%s.%s.%s.%s", tsStr, serverId, role, connectionId)
	digest := sha256.Sum256([]byte(payload))
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	rVal := new(big.Int).SetBytes(sigBytes[:32])
	sVal := new(big.Int).SetBytes(sigBytes[32:])
	if !ecdsa.Verify(pub, digest[:], rVal, sVal) {
		return false
	}

	return true
}
