// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

// Desbloqueo con el inicio de sesión de Sytue ID (ADR 0084).
//
// La persona solo teclea su contraseña (o usa su passkey) en la página de inicio
// de sesión de Sytue ID. Allí, EN EL NAVEGADOR, se deriva una clave (la KEK del
// sobre) que abre la CLAVE DE DESBLOQUEO de la cuenta: 32 bytes al azar que nacen
// en un navegador y que Sytue ID solo guarda dentro de un sobre (Envelope) que no
// puede abrir. Con esa clave cada app abre su envoltorio `sytue-id` de la clave de
// recuperación de Matrix sin pedir nada a la persona.
//
// Lo que guarda cada servidor no se abre con nada que tenga ese servidor: Sytue ID
// tiene sobres (sin la contraseña derivada ni la salida PRF), y Rooms envoltorios
// (sin la clave de desbloqueo). La salvedad honesta, la misma de la ADR 0082: la
// contraseña la ve Sytue ID al comprobarla, así que un operador ACTIVO que capture
// inicios de sesión podría derivar la KEK. La passkey no tiene ese punto débil.
//
// El formato lo fija testdata/desbloqueo.json, que comprueban también los guiones
// de la página de Sytue ID (services/id/internal/http/e2e) y la web
// (@sytue/offline/matrix).

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// LabelDesbloqueo separa estas derivaciones de cualquier otro uso de la misma
// contraseña, passkey o clave.
const LabelDesbloqueo = "sytue-e2e-unlock-v1"

// UnlockKeyLen es el tamaño de la clave de desbloqueo; HintLen, el de su huella.
const (
	UnlockKeyLen = 32
	HintLen      = 16
)

// EnvelopeVersion es el campo `v` de un sobre.
const EnvelopeVersion = 1

// MaxLegacy es cuántos envoltorios de la ADR 0082 abre como mucho un inicio de
// sesión: cada uno de contraseña cuesta un Argon2id de 64 MiB en el navegador.
const MaxLegacy = 4

// PasskeyPRFInput es la entrada `first` de la extensión PRF con la que la página de
// inicio de sesión evalúa CUALQUIER passkey. Es fija a propósito: el inicio de
// sesión con passkey puede ser sin usuario (el autenticador elige la credencial), y
// entonces no hay forma de mandar una entrada distinta por credencial. La salida
// sigue siendo distinta en cada passkey: la PRF va con la clave de esa credencial.
var PasskeyPRFInput = func() []byte {
	sum := sha256.Sum256([]byte(LabelDesbloqueo + "/passkey-prf"))
	return sum[:]
}()

// ErrDelivery es una entrega a una app nativa que no es para ella o está rota.
var ErrDelivery = errors.New("recoverywrap: invalid unlock delivery")

// NewUnlockKey es una clave de desbloqueo nueva. En producción la crea el navegador
// en la página de Sytue ID; aquí sirve a las pruebas y a los vectores.
func NewUnlockKey() ([]byte, error) {
	key := make([]byte, UnlockKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// Hint es la huella pública de una clave de desbloqueo: dice si un sobre o un
// envoltorio se cerró con ESTA clave sin intentar abrirlo, y no revela nada de ella.
func Hint(unlockKey []byte) ([]byte, error) {
	if len(unlockKey) != UnlockKeyLen {
		return nil, ErrInvalid
	}
	return hkdf.Key(sha256.New, unlockKey, nil, LabelDesbloqueo+"/hint", HintLen)
}

// SytueIDKey es la clave de envoltorio `sytue-id` de la clave de recuperación.
func SytueIDKey(unlockKey []byte) ([]byte, error) {
	if len(unlockKey) != UnlockKeyLen {
		return nil, ErrInvalid
	}
	return hkdf.Key(sha256.New, unlockKey, nil, Label+"/"+KindSytueID, keyLen)
}

// WrapSytueID envuelve la clave de recuperación de `account` con la clave de
// desbloqueo de la persona.
func WrapSytueID(unlockKey []byte, account, keyID, recoveryKey string) (Wrapper, error) {
	return SealSytueID(unlockKey, account, keyID, recoveryKey, nil)
}

// SealSytueID es WrapSytueID con el nonce elegido (solo los vectores lo eligen).
func SealSytueID(unlockKey []byte, account, keyID, recoveryKey string, nonce []byte) (Wrapper, error) {
	hint, err := Hint(unlockKey)
	if err != nil {
		return Wrapper{}, err
	}
	key, err := SytueIDKey(unlockKey)
	if err != nil {
		return Wrapper{}, err
	}
	defer clear(key)
	return Seal(key, KindSytueID, account, keyID, "", KDF{Alg: AlgSytueID, Hint: hint}, recoveryKey, nonce)
}

// UnwrapSytueID abre un envoltorio `sytue-id`. Uno cerrado con otra clave de
// desbloqueo (otra huella) es ErrWrongSecret sin gastar nada en intentarlo.
func UnwrapSytueID(unlockKey []byte, account string, w Wrapper) (string, error) {
	if err := w.Validate(); err != nil || w.Kind != KindSytueID {
		return "", ErrInvalid
	}
	hint, err := Hint(unlockKey)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(hint, w.KDF.Hint) {
		return "", ErrWrongSecret
	}
	key, err := SytueIDKey(unlockKey)
	if err != nil {
		return "", err
	}
	defer clear(key)
	return Open(key, account, w)
}

// Envelope es la clave de desbloqueo cerrada con un factor de la persona: lo que
// Sytue ID guarda y devuelve. Ninguna parte es secreta. Uno de contraseña por
// persona y uno por passkey; todos llevan la misma clave (la misma Hint).
type Envelope struct {
	V            int    `json:"v"`
	Kind         string `json:"kind"`
	CredentialID string `json:"credential_id"`
	KDF          KDF    `json:"kdf"`
	Hint         []byte `json:"hint"`
	Nonce        []byte `json:"nonce"`
	Ciphertext   []byte `json:"ciphertext"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// Validate comprueba la forma del sobre sin abrirlo: lo usa Sytue ID antes de
// guardarlo. La entrada PRF de un sobre de passkey tiene que ser PasskeyPRFInput:
// es la única que la página de inicio de sesión sabe evaluar.
func (e Envelope) Validate() error {
	if e.V != EnvelopeVersion || len(e.Hint) != HintLen || len(e.Nonce) != nonceLen ||
		len(e.Ciphertext) != UnlockKeyLen+16 {
		return ErrInvalid
	}
	switch e.Kind {
	case KindPassword:
		if e.CredentialID != "" || !e.KDF.validPassword() {
			return ErrInvalid
		}
	case KindPasskey:
		if !validCredential(e.CredentialID) || !e.KDF.validPasskey() || !bytes.Equal(e.KDF.PRFSalt, PasskeyPRFInput) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// EnvelopeAAD ata el sobre a su persona (el `sub` de Sytue ID), a su tipo y a su
// passkey: un sobre copiado a otra fila no se abre.
func EnvelopeAAD(kind, userID, credentialID string) []byte {
	return []byte(LabelDesbloqueo + "\x00" + kind + "\x00" + userID + "\x00" + credentialID)
}

// EnvelopePasswordKEK deriva la KEK de un sobre de contraseña. En producción la
// calcula la página de inicio de sesión, en el navegador.
func EnvelopePasswordKEK(password string, kdf KDF) ([]byte, error) {
	ikm, err := PasswordIKM(password, kdf)
	if err != nil {
		return nil, err
	}
	defer clear(ikm)
	return hkdf.Key(sha256.New, ikm, nil, LabelDesbloqueo+"/"+KindPassword, keyLen)
}

// EnvelopePasskeyKEK deriva la KEK de un sobre de passkey de su salida PRF.
func EnvelopePasskeyKEK(prf []byte) ([]byte, error) {
	if len(prf) != prfOutputLen {
		return nil, ErrInvalid
	}
	return hkdf.Key(sha256.New, prf, nil, LabelDesbloqueo+"/"+KindPasskey, keyLen)
}

// SealEnvelope cierra la clave de desbloqueo con una KEK ya derivada. nonce es nil
// salvo en los vectores.
func SealEnvelope(kek []byte, kind, userID, credentialID string, kdf KDF, unlockKey, nonce []byte) (Envelope, error) {
	if userID == "" {
		return Envelope{}, ErrInvalid
	}
	hint, err := Hint(unlockKey)
	if err != nil {
		return Envelope{}, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return Envelope{}, err
	}
	if nonce == nil {
		nonce = make([]byte, nonceLen)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return Envelope{}, err
		}
	}
	if len(nonce) != nonceLen {
		return Envelope{}, ErrInvalid
	}
	e := Envelope{V: EnvelopeVersion, Kind: kind, CredentialID: credentialID, KDF: kdf, Hint: hint, Nonce: bytes.Clone(nonce)}
	e.Ciphertext = aead.Seal(nil, e.Nonce, unlockKey, EnvelopeAAD(kind, userID, credentialID))
	if err := e.Validate(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

// OpenEnvelope abre un sobre con una KEK ya derivada y comprueba que lo que sale
// es la clave de su huella.
func OpenEnvelope(kek []byte, userID string, e Envelope) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	key, err := aead.Open(nil, e.Nonce, e.Ciphertext, EnvelopeAAD(e.Kind, userID, e.CredentialID))
	if err != nil {
		return nil, ErrWrongSecret
	}
	if hint, err := Hint(key); err != nil || !bytes.Equal(hint, e.Hint) {
		clear(key)
		return nil, ErrWrongSecret
	}
	return key, nil
}

// LegacyIKM es lo que la página de inicio de sesión calcula UNA vez para abrir un
// envoltorio de la ADR 0082 (contraseña: el Argon2id con su sal; passkey: la salida
// PRF con su entrada) y pasarlo al tipo `sytue-id`.
type LegacyIKM struct {
	Kind         string `json:"kind"`
	Salt         []byte `json:"salt,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
	IKM          []byte `json:"ikm"`
}

// OpenLegacy abre, entre `wrappers`, el de la ADR 0082 al que corresponde `ikm`.
func OpenLegacy(ikm LegacyIKM, account string, wrappers []Wrapper) (string, error) {
	key, err := KeyFromIKM(ikm.Kind, ikm.IKM)
	if err != nil {
		return "", err
	}
	defer clear(key)
	for _, w := range wrappers {
		if w.Kind != ikm.Kind || w.Validate() != nil {
			continue
		}
		if (w.Kind == KindPassword && !bytes.Equal(w.KDF.Salt, ikm.Salt)) ||
			(w.Kind == KindPasskey && w.CredentialID != ikm.CredentialID) {
			continue
		}
		if got, err := Open(key, account, w); err == nil {
			return got, nil
		}
	}
	return "", ErrWrongSecret
}

// Delivery es lo que la página de Sytue ID entrega a una app NATIVA al volver del
// inicio de sesión: la clave de desbloqueo de `Sub` y, en la transición, lo que
// abre sus envoltorios de la 0082. Viaja cifrado a una clave efímera que la app
// genera para ese inicio de sesión (SealDelivery), así que da igual por dónde
// pase: el loopback del escritorio o el esquema propio del móvil.
type Delivery struct {
	V      int         `json:"v"`
	Sub    string      `json:"sub"`
	Key    []byte      `json:"key"`
	Legacy []LegacyIKM `json:"legacy,omitempty"`
}

const (
	deliveryInfo   = LabelDesbloqueo + "/native"
	p256PublicLen  = 65
	maxDeliveryLen = 4096
)

// NewDeliveryKey es la clave efímera de un inicio de sesión nativo. Vive en
// memoria hasta que vuelve el navegador; su parte pública va en /authorize.
func NewDeliveryKey() (*ecdh.PrivateKey, error) { return ecdh.P256().GenerateKey(rand.Reader) }

// DeliveryPublic es la parte pública en el formato del parámetro
// `sytue_unlock_key`: el punto P-256 sin comprimir, en base64url sin relleno.
func DeliveryPublic(priv *ecdh.PrivateKey) string {
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
}

func deliveryAEAD(shared, ephPublic, appPublic []byte) (cipher.AEAD, error) {
	salt := append(bytes.Clone(ephPublic), appPublic...)
	key, err := hkdf.Key(sha256.New, shared, salt, deliveryInfo, keyLen)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealDelivery cifra una entrega para la clave pública de la app (ECDH P-256,
// HKDF-SHA256 y AES-256-GCM). En producción lo hace el navegador; aquí sirve a los
// vectores y a las pruebas. eph y nonce son nil salvo en los vectores.
func SealDelivery(appPublic string, d Delivery, eph *ecdh.PrivateKey, nonce []byte) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(appPublic)
	if err != nil || len(raw) != p256PublicLen {
		return "", ErrDelivery
	}
	pub, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return "", ErrDelivery
	}
	if eph == nil {
		if eph, err = NewDeliveryKey(); err != nil {
			return "", err
		}
	}
	shared, err := eph.ECDH(pub)
	if err != nil {
		return "", ErrDelivery
	}
	defer clear(shared)
	aead, err := deliveryAEAD(shared, eph.PublicKey().Bytes(), raw)
	if err != nil {
		return "", err
	}
	if nonce == nil {
		nonce = make([]byte, nonceLen)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return "", err
		}
	}
	plain, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	out := append(eph.PublicKey().Bytes(), nonce...)
	out = aead.Seal(out, nonce, plain, []byte(deliveryInfo))
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// OpenDelivery descifra la entrega con la clave efímera de este inicio de sesión
// y comprueba su forma. Quien la recibe compara después Sub con el `sub` de los
// tokens que acaba de canjear: una entrega de otra persona no se acepta.
func OpenDelivery(priv *ecdh.PrivateKey, blob string) (Delivery, error) {
	if priv == nil || len(blob) > maxDeliveryLen {
		return Delivery{}, ErrDelivery
	}
	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil || len(raw) < p256PublicLen+nonceLen+16 {
		return Delivery{}, ErrDelivery
	}
	ephRaw, nonce, sealed := raw[:p256PublicLen], raw[p256PublicLen:p256PublicLen+nonceLen], raw[p256PublicLen+nonceLen:]
	eph, err := ecdh.P256().NewPublicKey(ephRaw)
	if err != nil {
		return Delivery{}, ErrDelivery
	}
	shared, err := priv.ECDH(eph)
	if err != nil {
		return Delivery{}, ErrDelivery
	}
	defer clear(shared)
	aead, err := deliveryAEAD(shared, ephRaw, priv.PublicKey().Bytes())
	if err != nil {
		return Delivery{}, err
	}
	plain, err := aead.Open(nil, nonce, sealed, []byte(deliveryInfo))
	if err != nil {
		return Delivery{}, ErrDelivery
	}
	defer clear(plain)
	var d Delivery
	if json.Unmarshal(plain, &d) != nil || d.V != 1 || d.Sub == "" || len(d.Key) != UnlockKeyLen || len(d.Legacy) > MaxLegacy {
		return Delivery{}, ErrDelivery
	}
	for _, l := range d.Legacy {
		if len(l.IKM) != keyLen || (l.Kind != KindPassword && l.Kind != KindPasskey) {
			return Delivery{}, ErrDelivery
		}
	}
	return d, nil
}

// LegacyRequest es el parámetro `sytue_legacy` con el que una app pide a la página de
// inicio de sesión de Sytue ID que abra también sus envoltorios de la ADR 0082: solo
// sus parámetros públicos (la sal de Argon2id con los valores por defecto, que son los
// únicos que la página calcula, o la passkey y su entrada PRF), como mucho MaxLegacy.
// "" si no hay ninguno. La web lo compone igual (custody-browser.ts).
func LegacyRequest(wrappers []Wrapper) string {
	b := base64.RawURLEncoding.EncodeToString
	var items []string
	for _, w := range wrappers {
		if len(items) == MaxLegacy {
			break
		}
		switch {
		case w.Kind == KindPassword && w.KDF.validPassword() && w.KDF.Memory == DefaultMemoryKiB &&
			w.KDF.Time == DefaultTime && w.KDF.Threads == DefaultThreads:
			items = append(items, "p."+b(w.KDF.Salt))
		case w.Kind == KindPasskey && w.KDF.validPasskey() && validCredential(w.CredentialID):
			items = append(items, "k."+w.CredentialID+"."+b(w.KDF.PRFSalt))
		}
	}
	out := ""
	for i, item := range items {
		if i > 0 {
			out += "~"
		}
		out += item
	}
	return out
}
