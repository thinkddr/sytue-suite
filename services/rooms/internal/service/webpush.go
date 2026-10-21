// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strings"
	"time"
)

// Web Push CLEAN-ROOM (Ola 18) — cifrado RFC 8291 (ECDH P-256 + HKDF-SHA256 + AES-128-GCM,
// content-encoding aes128gcm de RFC 8188) y firma VAPID (RFC 8292: JWT ES256). Implementado
// SOLO con la stdlib de Go 1.26: crypto/ecdh, crypto/ecdsa, crypto/hkdf, crypto/aes,
// crypto/cipher, crypto/sha256. Sin dependencias externas (no toca go.mod).
//
// Genera un par de claves VAPID así (una vez, guárdalas fuera de git como SYTUE_VAPID_*):
//
//	# clave privada P-256 (escalar de 32 bytes) y pública (punto sin comprimir de 65 bytes),
//	# ambas en base64url SIN relleno — el formato que espera el navegador (applicationServerKey).
//	openssl ecparam -name prime256v1 -genkey -noout -out vapid.pem
//	# privada (32 bytes → base64url):
//	openssl ec -in vapid.pem -outform DER 2>/dev/null | tail -c 32 | basenc --base64url | tr -d '='
//	# pública (65 bytes → base64url):
//	openssl ec -in vapid.pem -pubout -outform DER 2>/dev/null | tail -c 65 | basenc --base64url | tr -d '='

// vapidKeys es el par VAPID ya parseado: la privada P-256 (para firmar el JWT ES256), la
// pública en base64url (parámetro `k` de la cabecera y applicationServerKey del navegador)
// y el `sub` de contacto (mailto:).
type vapidKeys struct {
	priv    *ecdsa.PrivateKey
	pubB64  string
	subject string
}

// b64urlDecode decodifica base64url tolerando relleno ausente (las herramientas de VAPID
// emiten sin `=`).
func b64urlDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(s), "="))
}

// b64urlEncode codifica en base64url sin relleno (formato de JWT y de las claves VAPID).
func b64urlEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// parseVAPIDKeys construye el par VAPID desde las claves en base64url. La privada es el
// escalar P-256 (32 bytes); la pública, el punto sin comprimir (65 bytes, 0x04||X||Y).
func parseVAPIDKeys(pubB64, privB64, subject string) (*vapidKeys, error) {
	scalar, err := b64urlDecode(privB64)
	if err != nil {
		return nil, fmt.Errorf("vapid: clave privada base64url inválida: %w", err)
	}
	pub, err := b64urlDecode(pubB64)
	if err != nil {
		return nil, fmt.Errorf("vapid: clave pública base64url inválida: %w", err)
	}
	if len(pub) != 65 || pub[0] != 0x04 {
		return nil, errors.New("vapid: clave pública debe ser un punto P-256 sin comprimir de 65 bytes")
	}
	priv, err := ecdsaFromScalar(scalar)
	if err != nil {
		return nil, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "mailto:ops@sytue.eu"
	}
	return &vapidKeys{priv: priv, pubB64: b64urlEncode(pub), subject: subject}, nil
}

// ecdsaFromScalar reconstruye la clave privada ECDSA P-256 desde su escalar de 32 bytes.
func ecdsaFromScalar(scalar []byte) (*ecdsa.PrivateKey, error) {
	if len(scalar) != 32 {
		return nil, fmt.Errorf("vapid: la clave privada P-256 debe ser de 32 bytes (son %d)", len(scalar))
	}
	curve := elliptic.P256()
	d := new(big.Int).SetBytes(scalar)
	if d.Sign() == 0 || d.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("vapid: escalar de clave privada fuera de rango")
	}
	priv := new(ecdsa.PrivateKey)
	priv.Curve = curve
	priv.D = d                                    //nolint:staticcheck // SA1019: ECDSA P256 manual para VAPID; migrar a crypto/ecdh es tarea aparte de esta feature
	priv.X, priv.Y = curve.ScalarBaseMult(scalar) //nolint:staticcheck // SA1019: idem (punto base P256)
	return priv, nil
}

// authorizationHeader construye la cabecera Authorization VAPID para un endpoint: firma un
// JWT ES256 con `aud`=origen del endpoint, `exp`=+12h y `sub`=contacto, y devuelve
// «vapid t=<jwt>, k=<pública base64url>» (RFC 8292).
func (v *vapidKeys) authorizationHeader(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("vapid: endpoint inválido")
	}
	aud := u.Scheme + "://" + u.Host
	claims := map[string]any{"aud": aud, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": v.subject}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64urlEncode([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + b64urlEncode(cb)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, v.priv, digest[:])
	if err != nil {
		return "", err
	}
	// La firma JWS ES256 es R||S de 32 bytes cada uno (NO ASN.1).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	jwt := signingInput + "." + b64urlEncode(sig)
	return "vapid t=" + jwt + ", k=" + v.pubB64, nil
}

// encryptPush cifra `plaintext` para una suscripción (clave pública del cliente p256dh + su
// secreto auth) según RFC 8291 (aes128gcm de RFC 8188) y devuelve el CUERPO listo para POST:
//
//	header = salt(16) || rs(4, big-endian) || idlen(1)=65 || keyid(clave pública efímera, 65)
//	body   = header || AES128GCM( plaintext || 0x02 )
//
// `rng` inyecta la aleatoriedad (clave efímera + salt) para poder testear con vectores
// deterministas; producción pasa crypto/rand.Reader.
func encryptPush(rng io.Reader, p256dh, authSecret, plaintext []byte) ([]byte, error) {
	if len(authSecret) != 16 {
		return nil, fmt.Errorf("webpush: auth debe ser de 16 bytes (son %d)", len(authSecret))
	}
	curve := ecdh.P256()
	uaPub, err := curve.NewPublicKey(p256dh) // valida que sea un punto P-256 válido
	if err != nil {
		return nil, fmt.Errorf("webpush: p256dh inválida: %w", err)
	}
	asPriv, err := curve.GenerateKey(rng)
	if err != nil {
		return nil, err
	}
	asPub := asPriv.PublicKey().Bytes() // 65 bytes, sin comprimir
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rng, salt); err != nil {
		return nil, err
	}
	shared, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}

	// RFC 8291 §3.4: combina el secreto ECDH con el `auth` del cliente para obtener el IKM.
	prkKey, err := hkdf.Extract(sha256.New, shared, authSecret)
	if err != nil {
		return nil, err
	}
	keyInfo := append([]byte("WebPush: info\x00"), p256dh...)
	keyInfo = append(keyInfo, asPub...)
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}

	// RFC 8188 (aes128gcm): deriva CEK y NONCE del IKM con el salt del registro.
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// Registro único = ÚLTIMO registro: delimitador de relleno 0x02 (RFC 8188 §2), sin más padding.
	record := append(append([]byte{}, plaintext...), 0x02)
	ciphertext := aead.Seal(nil, nonce, record, nil)

	body := make([]byte, 0, 16+4+1+len(asPub)+len(ciphertext))
	body = append(body, salt...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], 4096) // record size; nuestros payloads son pequeños
	body = append(body, rs[:]...)
	body = append(body, byte(len(asPub))) //nolint:gosec // G115: len(asPub)=65 (clave P256 sin comprimir), cabe en byte
	body = append(body, asPub...)
	body = append(body, ciphertext...)
	return body, nil
}
