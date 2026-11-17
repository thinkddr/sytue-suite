// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrMetadataProof = errors.New("httpx: invalid authenticated metadata proof")

func validMetadataHex(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}
func ValidMetadataNonce(value string) bool  { return validMetadataHex(value) }
func ValidMetadataSecret(value string) bool { return validMetadataHex(value) }
func FreshMetadataNonce() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// MetadataMAC authenticates only bounded PUBLIC service metadata. Callers own
// the typed protocol: use a fixed domain, clear the response's MAC field and
// verify its fresh nonce, scope, binding and original authority separately.
// Both sides serialize the same Go types. This helper grants no SDK identity,
// consent, source access or publication authority and never accepts private keys.
func MetadataMAC(secret, domain string, request, response any) (string, error) {
	if !ValidMetadataSecret(secret) || len(domain) < 1 || len(domain) > 128 || strings.ContainsAny(domain, "\x00\r\n\t ") {
		return "", ErrMetadataProof
	}
	raw, err := json.Marshal(struct {
		Domain            string
		Request, Response any
	}{domain, request, response})
	if err != nil || len(raw) > 256<<10 {
		return "", ErrMetadataProof
	}
	key, _ := hex.DecodeString(secret)
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func VerifyMetadataMAC(secret, domain string, request, response any, actual string) error {
	if !validMetadataHex(actual) {
		return ErrMetadataProof
	}
	expected, err := MetadataMAC(secret, domain, request, response)
	if err != nil {
		return err
	}
	expectedBytes, _ := hex.DecodeString(expected)
	actualBytes, _ := hex.DecodeString(actual)
	if !hmac.Equal(expectedBytes, actualBytes) {
		return ErrMetadataProof
	}
	return nil
}
