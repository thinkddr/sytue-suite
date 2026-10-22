// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package keycrypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifyHMACSHA256 valida una firma hexadecimal del cuerpo crudo en tiempo constante.
func VerifyHMACSHA256(secret string, body []byte, signature string) bool {
	if secret == "" || len(signature) != sha256.Size*2 {
		return false
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
