// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package vaultproof verifica las acciones de entrada (`key.enroll`, `task.*`)
// firmadas por la clave de una bóveda post-cuántica (ADR 0087), en lugar de por
// un dispositivo de Matrix.
//
// El formato de la acción no cambia (accion.Action, con el nonce y el
// transcript que retiene la autoridad): cambia quién la firma y contra qué se
// comprueba. La firma es la híbrida de la bóveda (Ed25519 + ML-DSA-65) sobre el
// JSON canónico de la acción, con el contexto `ingress.<kind>`, y la clave tiene
// que ser la ACTIVA que Sytue ID tiene registrada para esa persona, organización
// y producto. El resultado es una accion.Authorization como la de Matrix, así
// que el resto del correo (cola, handoff, ACK) no distingue de dónde vino.
package vaultproof

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/accion"
)

// Algorithm es el algoritmo de la firma dentro de `signatures`: la entrada es
// `signatures[UserID]["sytue-pq-vault:" + KeyID]`.
const Algorithm = "sytue-pq-vault"

// ErrUnverified es lo que se devuelve ante cualquier cosa que no verifica.
var ErrUnverified = accion.ErrUnverified

// Registered es la clave activa de la bóveda según Sytue ID.
type Registered struct {
	Generation int64
	KeyID      string
	Public     recoverywrap.PQVaultVerifyKey
}

// UserID es la identidad con la que la bóveda firma: el prefijo aislado del
// ámbito, como una cuenta de Matrix, seguido de un servidor que no existe en
// Matrix. Así accion.Authority.Validate la acepta y nunca se confunde con una.
func UserID(scope storage.Scope) string {
	prefix := storage.IsolatedMatrixUserPrefix(scope)
	if prefix == "" {
		return ""
	}
	return prefix + "sytue-vault"
}

// Context es el contexto de firma de una acción de ese tipo.
func Context(kind string) string { return "ingress." + kind }

// Payload son los bytes que se firman: el JSON canónico de la acción.
func Payload(action accion.Action) ([]byte, error) {
	return accion.CanonicalBytes(action)
}

// Sign firma la acción con la clave de la bóveda (para el cliente y las pruebas).
func Sign(signer *recoverywrap.PQVaultSigner, action accion.Action) (accion.SignedAction, error) {
	public := signer.Public()
	keyID, err := public.ID()
	if err != nil {
		return accion.SignedAction{}, err
	}
	payload, err := Payload(action)
	if err != nil {
		return accion.SignedAction{}, err
	}
	sig, err := signer.Sign(Context(action.Kind), payload)
	if err != nil {
		return accion.SignedAction{}, err
	}
	return accion.SignedAction{Action: action, Signatures: signatures.Signatures{
		id.UserID(UserID(action.Scope)): {id.KeyID(Algorithm + ":" + keyID): base64.StdEncoding.EncodeToString(sig)},
	}}, nil
}

// RootKey es la raíz con la que la bóveda aparece ante el correo: la mitad
// Ed25519 de su clave de firma, en el formato de Matrix (base64 sin relleno).
func RootKey(public recoverywrap.PQVaultVerifyKey) string {
	return base64.RawStdEncoding.EncodeToString(public.Ed25519)
}

// Authority es la autoridad de almacenamiento de una bóveda: la que Rooms sirve
// para un ámbito que vive en la bóveda y no en Matrix (ADR 0087). La generación y
// la raíz son las de la clave activa en Sytue ID.
func Authority(scope storage.Scope, reg Registered) (accion.Authority, error) {
	out := accion.Authority{
		Version: 1, Scope: scope, UserID: UserID(scope), RootKey: RootKey(reg.Public),
		Generation: reg.Generation, Status: "active",
	}
	keyID, err := reg.Public.ID()
	if err != nil || keyID != reg.KeyID || out.Validate(scope) != nil {
		return accion.Authority{}, ErrUnverified
	}
	return out, nil
}

// IsAuthority dice si una autoridad de almacenamiento es la de una bóveda (y no
// la de una cuenta de Matrix): su identidad es la de UserID.
func IsAuthority(a accion.Authority) bool {
	return a.Scope.Valid() && a.UserID == UserID(a.Scope)
}

// Signable son las acciones de entrada que una bóveda puede firmar. Las de la
// raíz (`root.*`) no: la raíz de una bóveda la ancla Sytue ID, no una firma.
func Signable(kind string) bool {
	switch kind {
	case "key.enroll", "task.claim", "task.ack", "task.handoff":
		return true
	default:
		return false
	}
}

// SignIntent firma `action` solo si es exactamente lo que el cliente pidió
// (`expected`, fijado antes de pedir el reto), si está vigente, si es una acción
// de entrada y si nombra la raíz de ESTA clave. Es lo que hace el núcleo Wasm
// por el navegador: el reto del servidor nunca elige qué se firma.
func SignIntent(signer *recoverywrap.PQVaultSigner, expected accion.Intent, action accion.Action, now time.Time) (accion.SignedAction, error) {
	if signer == nil || !expected.Valid() || action.Intent() != expected || !Signable(action.Kind) ||
		!action.Valid(now) || action.RootKey != RootKey(signer.Public()) {
		return accion.SignedAction{}, ErrUnverified
	}
	return Sign(signer, action)
}

// Signed dice si la prueba viene firmada por una bóveda (y no por Matrix).
func Signed(proof accion.SignedAction) bool {
	for _, keys := range proof.Signatures {
		for k := range keys {
			if strings.HasPrefix(string(k), Algorithm+":") {
				return true
			}
		}
	}
	return false
}

// verifySignature comprueba que la prueba trae una sola firma, la de esta clave
// con el nombre de la bóveda, y que verifica sobre la acción esperada.
func verifySignature(expected accion.Action, proof accion.SignedAction, public recoverywrap.PQVaultVerifyKey, keyID string) error {
	user := id.UserID(UserID(expected.Scope))
	if user == "" || len(proof.Signatures) != 1 || len(proof.Signatures[user]) != 1 {
		return ErrUnverified
	}
	raw, ok := proof.Signatures[user][id.KeyID(Algorithm+":"+keyID)]
	if !ok {
		return ErrUnverified
	}
	sig, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return ErrUnverified
	}
	payload, err := Payload(expected)
	if err != nil {
		return ErrUnverified
	}
	if err := public.Verify(Context(expected.Kind), payload, sig); err != nil {
		if errors.Is(err, recoverywrap.ErrWrongSecret) || errors.Is(err, recoverywrap.ErrInvalid) {
			return ErrUnverified
		}
		return err
	}
	return nil
}

// Verify comprueba la prueba contra la acción esperada y la clave registrada, y
// devuelve la autorización. `expected` es el transcript que retuvo la autoridad.
func Verify(expected accion.Action, proof accion.SignedAction, reg Registered, now time.Time) (accion.Authorization, error) {
	var nada accion.Authorization
	if proof.Action != expected || !expected.Intent().Valid() || reg.Generation < 1 {
		return nada, ErrUnverified
	}
	if now.UnixMilli() < expected.IssuedAt-30_000 || now.UnixMilli() >= expected.ExpiresAt {
		return nada, ErrUnverified
	}
	keyID, err := reg.Public.ID()
	if err != nil || keyID != reg.KeyID {
		return nada, ErrUnverified
	}
	// Las claves raíz y de firma que ve el resto del correo son la mitad Ed25519,
	// en el formato de Matrix: la acción tiene que nombrar ESA raíz y ESA generación.
	root := RootKey(reg.Public)
	if expected.RootKey != root || expected.Generation != reg.Generation {
		return nada, ErrUnverified
	}
	if err := verifySignature(expected, proof, reg.Public, keyID); err != nil {
		return nada, err
	}
	authority, err := Authority(expected.Scope, reg)
	if err != nil {
		return nada, ErrUnverified
	}
	out := accion.Authorization{
		Version: 1, Authority: authority,
		Target: expected.Target, SigningKey: root, Nonce: expected.Nonce, RequestSHA256: expected.RequestSHA256,
	}
	if out.Validate(expected) != nil {
		return nada, ErrUnverified
	}
	return out, nil
}
