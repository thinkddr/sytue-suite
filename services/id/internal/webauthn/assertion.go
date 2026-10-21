// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// Etiquetas de COSE_Key (RFC 9052 §7 / RFC 8152). Las claves EC2/RSA comparten
// las etiquetas -1/-2 con distinto significado según kty.
const (
	coseKty    = 1  // tipo de clave
	coseAlg    = 3  // algoritmo
	coseEC2Crv = -1 // curva (EC2)
	coseEC2X   = -2 // coordenada x (EC2)
	coseEC2Y   = -3 // coordenada y (EC2)
	coseRSAN   = -1 // módulo (RSA)
	coseRSAE   = -2 // exponente (RSA)

	ktyEC2  = 2 // clave de curva elíptica (dos coordenadas)
	ktyRSA  = 3 // clave RSA
	crvP256 = 1 // P-256 (secp256r1)

	// algES256 (-7) y algRS256 (-257) son los algoritmos que anuncia el registro
	// (pubKeyCredParams en webauthn.go); solo estos dos se aceptan en el login.
	algES256 = -7
	algRS256 = -257
)

// Errores de verificación de la assertion (WebAuthn L2 §7.2). Estables para que
// el transporte los distinga si hace falta; al usuario se le da un mensaje genérico.
var (
	ErrAssertionType       = errors.New("webauthn: la operación no es webauthn.get")
	ErrChallengeMismatch   = errors.New("webauthn: el reto no coincide")
	ErrOriginNotAllowed    = errors.New("webauthn: origen no permitido")
	ErrRPIDMismatch        = errors.New("webauthn: rpIdHash no coincide")
	ErrUserNotPresent      = errors.New("webauthn: falta el flag User Present")
	ErrUserNotVerified     = errors.New("webauthn: se requería verificación de usuario")
	ErrSignCountRegression = errors.New("webauthn: el signCount retrocede (posible clonación del autenticador)")
	ErrBadSignature        = errors.New("webauthn: firma de la assertion inválida")
	ErrAlgMismatch         = errors.New("webauthn: el algoritmo no coincide con la clave")
)

// AssertionInput agrupa todo lo necesario para verificar una assertion WebAuthn
// (navigator.credentials.get) contra una credencial ya registrada.
type AssertionInput struct {
	// COSEPublicKey es la clave pública COSE almacenada en el registro.
	COSEPublicKey []byte
	// AuthenticatorData, ClientDataJSON y Signature son la respuesta del autenticador
	// (ya decodificados de base64url).
	AuthenticatorData []byte
	ClientDataJSON    []byte
	Signature         []byte
	// StoredSignCount es el contador de firmas persistido (anti-clonación).
	StoredSignCount uint32
	// ExpectedChallenge es el reto emitido por el servidor (ya consumido del store).
	ExpectedChallenge []byte
	// RPID y AllowedOrigins son la configuración del Relying Party.
	RPID           string
	AllowedOrigins []string
	// RequireUserVerification exige el bit UV (login sin contraseña de alto valor).
	RequireUserVerification bool
}

// AssertionResult es el veredicto de una assertion verificada.
type AssertionResult struct {
	// SignCount es el nuevo contador a persistir (> StoredSignCount salvo el caso
	// "ambos 0", en que el autenticador no lleva contador).
	SignCount uint32
	// UserVerified refleja el bit UV (passkey verificada ⇒ factor fuerte, amr=mfa).
	UserVerified bool
}

// VerifyAssertion realiza la ceremonia de verificación de una assertion WebAuthn
// (L2 §7.2): valida clientDataJSON (tipo webauthn.get, reto, origen), el
// authenticatorData (rpIdHash, flag UP), la regla anti-clonación del signCount y,
// finalmente, la firma criptográfica sobre authData || SHA-256(clientDataJSON) con
// la clave pública COSE registrada. Es una función pura (sin BD ni HTTP): todo el
// núcleo de seguridad es testeable de forma aislada.
func VerifyAssertion(in AssertionInput) (AssertionResult, error) {
	var cd ClientData
	if err := json.Unmarshal(in.ClientDataJSON, &cd); err != nil {
		return AssertionResult{}, fmt.Errorf("webauthn: clientData no parseable: %w", err)
	}
	if cd.Type != "webauthn.get" {
		return AssertionResult{}, ErrAssertionType
	}
	// Reto: comparación en tiempo constante contra el emitido por el servidor.
	gotChallenge, err := base64.RawURLEncoding.DecodeString(cd.Challenge)
	if err != nil || subtle.ConstantTimeCompare(gotChallenge, in.ExpectedChallenge) != 1 {
		return AssertionResult{}, ErrChallengeMismatch
	}
	if !originInList(cd.Origin, in.AllowedOrigins) {
		return AssertionResult{}, ErrOriginNotAllowed
	}

	ad, err := ParseAuthenticatorData(in.AuthenticatorData)
	if err != nil {
		return AssertionResult{}, err
	}
	rpHash := sha256.Sum256([]byte(in.RPID))
	if !ad.RPIDHashEquals(rpHash[:]) {
		return AssertionResult{}, ErrRPIDMismatch
	}
	if !ad.UserPresent() {
		return AssertionResult{}, ErrUserNotPresent
	}
	if in.RequireUserVerification && !ad.UserVerified() {
		return AssertionResult{}, ErrUserNotVerified
	}

	// Anti-clonación (L2 §7.2 paso 21): si el autenticador lleva contador (alguno de
	// los dos != 0), el nuevo DEBE ser estrictamente mayor; si retrocede o se estanca,
	// se sospecha un autenticador clonado y se rechaza. "Ambos 0" ⇒ sin contador, OK.
	if ad.SignCount != 0 || in.StoredSignCount != 0 {
		if ad.SignCount <= in.StoredSignCount {
			return AssertionResult{}, ErrSignCountRegression
		}
	}

	pub, alg, err := ParseCOSEPublicKey(in.COSEPublicKey)
	if err != nil {
		return AssertionResult{}, err
	}
	// Datos firmados = authenticatorData || SHA-256(clientDataJSON) (L2 §7.2 paso 23).
	cdHash := sha256.Sum256(in.ClientDataJSON)
	signed := make([]byte, 0, len(in.AuthenticatorData)+len(cdHash))
	signed = append(signed, in.AuthenticatorData...)
	signed = append(signed, cdHash[:]...)
	if err := verifySignature(pub, alg, signed, in.Signature); err != nil {
		return AssertionResult{}, err
	}
	return AssertionResult{SignCount: ad.SignCount, UserVerified: ad.UserVerified()}, nil
}

// ParseCOSEPublicKey decodifica una clave pública en formato COSE_Key (CBOR con
// claves enteras) a una clave de crypto de la stdlib. Solo EC2/P-256 (ES256) y RSA
// (RS256), los algoritmos que el registro anuncia. Valida que el punto EC esté en
// la curva (vía crypto/ecdh, sin APIs deprecadas).
func ParseCOSEPublicKey(coseKey []byte) (crypto.PublicKey, int64, error) {
	d := &cborDecoder{buf: coseKey}
	m, err := d.readIntMap()
	if err != nil {
		return nil, 0, fmt.Errorf("webauthn: COSE_Key: %w", err)
	}
	kty, _ := m[coseKty].(int64)
	alg, _ := m[coseAlg].(int64)
	switch kty {
	case ktyEC2:
		crv, _ := m[coseEC2Crv].(int64)
		if crv != crvP256 {
			return nil, 0, fmt.Errorf("webauthn: curva EC no soportada: %d", crv)
		}
		x, xok := m[coseEC2X].([]byte)
		y, yok := m[coseEC2Y].([]byte)
		if !xok || !yok || len(x) != 32 || len(y) != 32 {
			return nil, 0, errors.New("webauthn: coordenadas EC inválidas")
		}
		// UNA sola llamada valida el punto Y construye la clave. Antes eran dos
		// pasos —`ecdh.P256().NewPublicKey` para comprobar que el punto está en la
		// curva, y luego un `ecdsa.PublicKey` armado a mano con X e Y— y ese
		// segundo paso usa campos que Go 1.26 marcó obsoletos: manipular las
		// coordenadas en crudo permite construir claves inválidas y las
		// operaciones de `big.Int` no son de tiempo constante.
		//
		// `ParseUncompressedPublicKey` rechaza igual lo que no esté en P-256, así
		// que no se pierde ninguna comprobación: se deja de hacer dos veces.
		point := make([]byte, 0, 65)
		point = append(point, 0x04) // formato sin comprimir (SEC1)
		point = append(point, x...)
		point = append(point, y...)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
		if err != nil {
			return nil, 0, fmt.Errorf("webauthn: punto EC inválido: %w", err)
		}
		return pub, alg, nil
	case ktyRSA:
		n, nok := m[coseRSAN].([]byte)
		e, eok := m[coseRSAE].([]byte)
		if !nok || !eok || len(n) == 0 || len(e) == 0 {
			return nil, 0, errors.New("webauthn: parámetros RSA inválidos")
		}
		exp := new(big.Int).SetBytes(e)
		if !exp.IsInt64() || exp.Int64() <= 0 {
			return nil, 0, errors.New("webauthn: exponente RSA inválido")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exp.Int64())}, alg, nil
	default:
		return nil, 0, fmt.Errorf("webauthn: tipo de clave COSE no soportado: %d", kty)
	}
}

// verifySignature verifica la firma con el algoritmo declarado en la clave COSE,
// exigiendo que el algoritmo case con el tipo de clave (ES256↔EC, RS256↔RSA).
func verifySignature(pub crypto.PublicKey, alg int64, signed, sig []byte) error {
	digest := sha256.Sum256(signed)
	switch p := pub.(type) {
	case *ecdsa.PublicKey:
		if alg != algES256 {
			return ErrAlgMismatch
		}
		// La firma ES256 de WebAuthn viene en DER (secuencia r,s).
		if !ecdsa.VerifyASN1(p, digest[:], sig) {
			return ErrBadSignature
		}
		return nil
	case *rsa.PublicKey:
		if alg != algRS256 {
			return ErrAlgMismatch
		}
		if err := rsa.VerifyPKCS1v15(p, crypto.SHA256, digest[:], sig); err != nil {
			return ErrBadSignature
		}
		return nil
	default:
		return errors.New("webauthn: tipo de clave no soportado")
	}
}

// originInList indica si el origen de clientDataJSON está entre los permitidos
// (coincidencia exacta, sin comodines).
func originInList(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}
