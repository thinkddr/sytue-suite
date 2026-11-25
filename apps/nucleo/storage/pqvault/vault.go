// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pqvault es la bóveda post-cuántica directa (ADR 0086/0087) para las apps
// nativas: el mismo almacén (`services/drive`, `/api/v1/pq-vault/{product}/*`), el
// mismo envoltorio (`recoverywrap.PQVaultWrapper`), el mismo sobre
// (`org.sytue.storage_pq`) y la misma prueba de propietario
// (`org.sytue.storage.auth.v1`) que la web (`packages/offline/src/vault/pq-vault.ts`),
// así que lo que guarda una lo abre la otra.
//
// La diferencia es dónde vive la clave de desbloqueo de Sytue ID: en la web, en el
// puente de `id.<dominio>`; en nativo, en el dispositivo, con la sesión que comparten
// sus apps (ADR 0084). Aquí se sella y se abre el envoltorio directamente con ella,
// sin puente.
package pqvault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

var (
	// ErrSignin: este dispositivo no tiene la clave de desbloqueo. La salida es
	// volver a entrar en Sytue ID.
	ErrSignin = errors.New("pqvault: hace falta entrar en Sytue ID para desbloquear")
	// ErrWrong: la clave de desbloqueo no abre la bóveda, y la bóveda guarda datos.
	// La persona elige: volver a entrar en Sytue ID o vaciarla (Reset).
	ErrWrong = errors.New("pqvault: la clave de desbloqueo no abre esta bóveda")
	// ErrIdentity: lo que devolvió el servidor no corresponde a esta bóveda.
	ErrIdentity = errors.New("pqvault: la bóveda no coincide con su identidad")
	// ErrClosed: la bóveda ya se cerró.
	ErrClosed = errors.New("pqvault: la bóveda está cerrada")
	// ErrChanged: se pidió vaciar la bóveda tal como se vio y ya no es esa (otra
	// bóveda, o registros nuevos: quizá el recibo de una importación). No se vació.
	ErrChanged = errors.New("pqvault: la bóveda cambió desde que se vio; no se vació")
	// ErrCopyInProgress: otro dispositivo está pasando a la bóveda el Drive de
	// Matrix (o la marca con la que se escribía ya no vale). No se tocó nada.
	ErrCopyInProgress = errors.New("pqvault: otro dispositivo está copiando en la bóveda")
)

// Server es lo que la bóveda necesita de services/drive (*api.Drive).
type Server interface {
	PQVaultKeys(context.Context, storage.Scope) (api.PQVaultState, error)
	PutPQVaultKey(context.Context, storage.Scope, recoverywrap.PQVaultWrapper, bool, *api.PQVaultExpect) (api.PQVaultState, error)
	AppendPQVaultRecord(context.Context, storage.Scope, string, string, json.RawMessage) (api.PQVaultRecord, error)
	PQVaultRecord(context.Context, storage.Scope, string, string) (api.PQVaultRecord, error)
	PQVaultRecords(context.Context, storage.Scope, string, string, int) (api.PQVaultRecordPage, error)
}

// Registry es el registro de claves de firma de Sytue ID (*api.Account).
type Registry interface {
	VaultKeys(context.Context, string) ([]api.VaultKey, error)
	PutVaultKey(context.Context, string, api.VaultKeyRegistration) error
}

// Config abre la bóveda de un `(tenant, subject, product)`.
type Config struct {
	Scope  storage.Scope
	Server Server
	// Registry es opcional: sin él, la clave de firma se crea pero no se registra
	// (SigningUnavailable). Ninguna app de almacenamiento la necesita para abrir.
	Registry Registry
	// UnlockKey es la clave de desbloqueo de Sytue ID (32 bytes). Open la copia; el
	// llamante borra la suya.
	UnlockKey []byte
	// Expect, solo con Reset: lo que se vio de la bóveda al decidir vaciarla. El
	// servidor solo la vacía si sigue así (ErrChanged si no). Sin él, vale lo que
	// lea Reset al empezar.
	Expect *api.PQVaultExpect
	// Handover registra la clave de firma con la marca de traspaso del correo de
	// Matrix a la bóveda (ADR 0087), que Sytue ID solo acepta en `mail` y con
	// identificación reciente. Solo cuando la persona lo confirma («vaciar y pasar
	// a la bóveda»), y normalmente con Reset: el traspaso llega con una bóveda nueva.
	Handover bool
}

// SigningStatus dice si la clave de firma de la bóveda es la activa en Sytue ID.
type SigningStatus string

const (
	SigningRegistered SigningStatus = "registered"
	// SigningReauth: hay otra activa y sustituirla pide identificarse otra vez. La
	// bóveda funciona igual; solo espera lo que se firma con ella (el correo).
	SigningReauth      SigningStatus = "reauth"
	SigningMissing     SigningStatus = "missing"
	SigningUnavailable SigningStatus = "unavailable"
)

// Vault es una bóveda abierta. Guarda en memoria la identidad híbrida, la clave
// de propietario y la de firma hasta Close.
type Vault struct {
	scope     storage.Scope
	server    Server
	vaultID   string
	keyID     string
	recipient pqc.Recipient

	mu       sync.RWMutex
	identity *pqc.Identity
	owner    []byte
	// signingKey es la clave de firma (ADR 0087) en la forma del secreto; la usa
	// el correo para firmar sus acciones de entrada. Se borra al cerrar.
	signingKey []byte
	signing    SigningStatus
	// signingCheck dice por qué la clave de firma no quedó registrada (vacío si lo
	// está): la misma etiqueta que la web (`PqVaultError.check`), para el código de
	// fallo. No se borra al cerrar: no es secreta.
	signingCheck string
	closed       bool
}

// Open abre la bóveda con la clave de desbloqueo, o la crea si no existe. Si la
// clave no la abre y la bóveda no guarda ni un registro, la reinicia sola: no hay
// nada que perder. Si guarda datos, devuelve ErrWrong y no toca nada.
func Open(ctx context.Context, cfg Config) (*Vault, error) { return open(ctx, cfg, false) }

// Reset vacía la bóveda (borra sus registros y claves en el servidor) y la estrena
// con una identidad nueva. Solo tras la confirmación explícita de la persona, o
// cuando se sabe que la bóveda no guarda nada de nadie. Vacía solo si sigue como
// se vio (Config.Expect, o lo que lee al empezar): si alguien publicó algo
// después, ErrChanged y no se toca nada.
func Reset(ctx context.Context, cfg Config) (*Vault, error) { return open(ctx, cfg, true) }

//nolint:gocyclo // el mismo orden de pasos que PqVault.openInternal de la web.
func open(ctx context.Context, cfg Config, reset bool) (*Vault, error) {
	if cfg.Server == nil || !cfg.Scope.Valid() {
		return nil, ErrIdentity
	}
	if len(cfg.UnlockKey) != recoverywrap.UnlockKeyLen {
		return nil, ErrSignin
	}
	auk := append([]byte(nil), cfg.UnlockKey...)
	defer clear(auk)
	scope := cfg.Scope

	state, err := cfg.Server.PQVaultKeys(ctx, scope)
	if err != nil {
		return nil, err
	}
	var (
		secret  recoverywrap.PQVaultSecret
		opened  bool
		vaultID = state.VaultID
	)
	expect, err := resetExpectation(cfg, state, reset)
	if err != nil {
		return nil, err
	}
	if !reset && state.VaultID != "" && len(state.Keys) > 0 {
		var openErr error
		secret, openErr = unwrap(auk, scope, state)
		switch {
		case openErr == nil:
			opened = true
		case errors.Is(openErr, recoverywrap.ErrWrongSecret):
			// Envuelta con otra clave de desbloqueo (la de antes de la ADR 0086 o
			// una que ya no existe). Sin registros, no hay nada que perder.
			if !vaultIsEmpty(ctx, cfg.Server, scope, state.VaultID) {
				return nil, ErrWrong
			}
			reset = true
		default:
			return nil, ErrIdentity
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if !opened {
		var vaciar *api.PQVaultExpect
		if reset {
			vaciar = expect
		}
		secret, vaultID, err = create(ctx, cfg.Server, auk, scope, vaciar)
		if err != nil {
			return nil, serverError(err)
		}
	}
	defer func() {
		secret.Identity, secret.OwnerKey, secret.SigningKey = "", "", ""
	}()
	identity, err := secret.Validate()
	if err != nil {
		return nil, ErrIdentity
	}
	keyID, err := pqc.RecipientID(identity.Recipient())
	if err != nil {
		return nil, ErrIdentity
	}
	// Una bóveda de antes de la ADR 0087 gana su clave de firma. Si falla, se
	// intenta en la siguiente apertura: nada de esto impide abrirla.
	if secret.SigningKey == "" {
		if next, err := addSigningKey(ctx, cfg.Server, auk, scope, secret); err == nil {
			secret = next
		}
	}
	signing, signingCheck := SigningMissing, "signing_missing"
	if secret.SigningKey != "" {
		signing, signingCheck = registerSigningKey(ctx, cfg.Registry, scope, secret.SigningKey, cfg.Handover)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := base64.StdEncoding.DecodeString(secret.OwnerKey)
	if err != nil || len(owner) != ownerKeyLen {
		clear(owner)
		return nil, ErrIdentity
	}
	if !validVaultID(vaultID) {
		clear(owner)
		return nil, ErrIdentity
	}
	return &Vault{
		scope: scope, server: cfg.Server, vaultID: vaultID, keyID: keyID,
		recipient: identity.Recipient(), signing: signing, signingCheck: signingCheck, identity: identity, owner: owner,
		signingKey: []byte(secret.SigningKey),
	}, nil
}

// unwrap prueba los envoltorios, la clave activa primero. ErrWrongSecret solo si
// TODOS fallan por la clave: un envoltorio roto no se confunde con otra clave.
func unwrap(auk []byte, scope storage.Scope, state api.PQVaultState) (recoverywrap.PQVaultSecret, error) {
	ordered := make([]recoverywrap.PQVaultWrapper, 0, len(state.Keys))
	for _, w := range state.Keys {
		if w.KeyID == state.ActiveKeyID {
			ordered = append([]recoverywrap.PQVaultWrapper{w}, ordered...)
		} else {
			ordered = append(ordered, w)
		}
	}
	wrong := false
	for _, w := range ordered {
		secret, err := recoverywrap.OpenPQVault(auk, scope.Tenant, scope.Product, scope.Subject, w)
		if err == nil {
			return secret, nil
		}
		if errors.Is(err, recoverywrap.ErrWrongSecret) {
			wrong = true
		}
	}
	if wrong {
		return recoverywrap.PQVaultSecret{}, recoverywrap.ErrWrongSecret
	}
	return recoverywrap.PQVaultSecret{}, recoverywrap.ErrInvalid
}

// vaultIsEmpty: ¿la bóveda no guarda ni un registro? Ante cualquier duda —error de
// red, respuesta rara— contesta que no: un reinicio silencioso solo vale si no hay
// nada que perder.
func vaultIsEmpty(ctx context.Context, server Server, scope storage.Scope, vaultID string) bool {
	page, err := server.PQVaultRecords(ctx, scope, vaultID, "", 1)
	return err == nil && page.Entries != nil && len(page.Entries) == 0 && page.NextCursor == ""
}

const ownerKeyLen = 32

// resetExpectation es lo que se vacía, si se vacía: lo que vio quien decidió
// vaciar (Config.Expect), que tiene que ser lo que hay ahora, o lo que se acaba
// de leer.
func resetExpectation(cfg Config, state api.PQVaultState, reset bool) (*api.PQVaultExpect, error) {
	expect := &api.PQVaultExpect{VaultID: state.VaultID, Version: state.Version}
	if !reset || cfg.Expect == nil {
		return expect, nil
	}
	if *cfg.Expect != *expect {
		return nil, ErrChanged
	}
	return cfg.Expect, nil
}

// serverError traduce los rechazos de la bóveda que el llamante distingue.
func serverError(err error) error {
	switch {
	case api.IsPQVaultCode(err, api.CodePQVaultChanged):
		return errors.Join(ErrChanged, err)
	case api.IsPQVaultCode(err, api.CodePQVaultCopyInProgress):
		return errors.Join(ErrCopyInProgress, err)
	default:
		return err
	}
}

// create genera una identidad nueva, su clave de propietario y su clave de firma,
// las sella con la clave de desbloqueo y las guarda. Con `reset`, el servidor vacía
// antes la bóveda (si sigue como se vio) y le da un identificador nuevo.
func create(ctx context.Context, server Server, auk []byte, scope storage.Scope, reset *api.PQVaultExpect) (recoverywrap.PQVaultSecret, string, error) {
	identity, err := pqc.GenerateIdentity()
	if err != nil {
		return recoverywrap.PQVaultSecret{}, "", err
	}
	owner := make([]byte, ownerKeyLen)
	defer clear(owner)
	if _, err := io.ReadFull(rand.Reader, owner); err != nil {
		return recoverywrap.PQVaultSecret{}, "", err
	}
	secret := recoverywrap.PQVaultSecret{
		V: recoverywrap.PQVaultVersion, Profile: pqc.HybridProfile,
		Identity: identity.String(), OwnerKey: base64.StdEncoding.EncodeToString(owner),
	}
	// La clave de firma nace con la bóveda (ADR 0087). Sin ella se crea igual.
	if signing, err := recoverywrap.NewPQVaultSigningKey(); err == nil {
		secret.SigningKey = signing
	}
	wrapper, err := recoverywrap.WrapPQVault(auk, scope.Tenant, scope.Product, scope.Subject, secret)
	if err != nil {
		return recoverywrap.PQVaultSecret{}, "", err
	}
	saved, err := server.PutPQVaultKey(ctx, scope, wrapper, true, reset)
	if err != nil {
		return recoverywrap.PQVaultSecret{}, "", err
	}
	if !validVaultID(saved.VaultID) || saved.ActiveKeyID != wrapper.KeyID {
		return recoverywrap.PQVaultSecret{}, "", ErrIdentity
	}
	return secret, saved.VaultID, nil
}

func addSigningKey(ctx context.Context, server Server, auk []byte, scope storage.Scope, secret recoverywrap.PQVaultSecret) (recoverywrap.PQVaultSecret, error) {
	signing, err := recoverywrap.NewPQVaultSigningKey()
	if err != nil {
		return secret, err
	}
	next := secret
	next.SigningKey = signing
	wrapper, err := recoverywrap.WrapPQVault(auk, scope.Tenant, scope.Product, scope.Subject, next)
	if err != nil {
		return secret, err
	}
	if _, err := server.PutPQVaultKey(ctx, scope, wrapper, true, nil); err != nil {
		return secret, err
	}
	return next, nil
}

// registerSigningKey registra en Sytue ID la clave de firma si no es ya la activa.
// Sin activa es un alta; con otra activa, que esta bóveda ya no puede aprobar
// porque no la tiene, es un vaciado, y Sytue ID pide identificarse otra vez. Con
// `handover` (solo `mail`) se pide siempre, con la marca de traspaso: Sytue ID
// dice si esa clave ya lo tiene, y si no, exige identificación reciente.
//
// Si otro dispositivo registró entre la lectura y el registro (`409
// vault_key_stale`), se vuelve a leer la activa y se repite una vez, como la web.
// Devuelve también por qué no quedó registrada (signingCheck), vacío si lo está.
func registerSigningKey(ctx context.Context, registry Registry, scope storage.Scope, encoded string, handover bool) (SigningStatus, string) {
	if registry == nil {
		return SigningUnavailable, "signing_registry"
	}
	if handover && scope.Product != "mail" {
		return SigningUnavailable, "signing_handover"
	}
	signer, err := recoverywrap.ParsePQVaultSigningKey(encoded)
	if err != nil {
		return SigningUnavailable, "signing_key"
	}
	for intento := 0; ; intento++ {
		status, check, rancia := registerSigningKeyOnce(ctx, registry, scope, signer, handover)
		if !rancia {
			return status, check
		}
		if intento > 0 {
			return SigningUnavailable, check
		}
	}
}

// registerSigningKeyOnce es un intento de registerSigningKey; `rancia` dice que
// Sytue ID contestó vault_key_stale y vale la pena repetir.
func registerSigningKeyOnce(ctx context.Context, registry Registry, scope storage.Scope, signer *recoverywrap.PQVaultSigner, handover bool) (status SigningStatus, check string, rancia bool) {
	public := signer.Public()
	keyID, err := public.ID()
	if err != nil {
		return SigningUnavailable, "signing_key", false
	}
	keys, err := registry.VaultKeys(ctx, scope.Product)
	if err != nil {
		return SigningUnavailable, signingCheck("signing_get", err), false
	}
	var active *api.VaultKey
	for i := range keys {
		if keys[i].RetiredAt == nil {
			active = &keys[i]
			break
		}
	}
	generation, previous := int64(1), ""
	if active != nil {
		if active.KeyID == keyID {
			// La misma clave otra vez. Con traspaso, Sytue ID solo da por bueno el
			// que ya se hizo (repetirlo devuelve el hecho), nunca uno colgado de una
			// clave que ya estaba sin él; y una clave no se sucede a sí misma en la
			// declaración, así que ni se pide.
			if !handover || active.Reason == ReasonHandover {
				return SigningRegistered, "", false
			}
			return SigningUnavailable, "signing_handover_taken", false
		}
		generation, previous = active.Generation+1, active.KeyID
	}
	statement, err := recoverywrap.PQVaultKeyStatement(scope.Subject, scope.Tenant, scope.Product, generation, keyID, previous)
	if err != nil {
		return SigningUnavailable, "signing_statement", false
	}
	proof, err := signer.Sign(recoverywrap.VaultKeyEnrollContext, statement)
	if err != nil {
		return SigningUnavailable, "signing_sign", false
	}
	err = registry.PutVaultKey(ctx, scope.Product, api.VaultKeyRegistration{PublicKey: public, PreviousKeyID: previous, Proof: proof, Handover: handover})
	if err == nil {
		return SigningRegistered, "", false
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 403 && apiErr.Code() == api.CodeVaultKeyReauth {
		return SigningReauth, "", false
	}
	return SigningUnavailable, signingCheck("signing_put", err), errors.As(err, &apiErr) && apiErr.Status == 409 && apiErr.Code() == codeVaultKeyStale
}

// codeVaultKeyStale es el 409 de Sytue ID cuando la declaración se firmó para otra
// generación: otro dispositivo registró antes.
const codeVaultKeyStale = "vault_key_stale"

var checkToken = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// signingCheck es la etiqueta de un fallo de Sytue ID: la petición, el estado HTTP
// y el código del servicio (`signing_put_400_vault_key_proof`), lo mismo que la web.
func signingCheck(peticion string, err error) string {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return peticion + "_failed"
	}
	check := peticion + "_" + strconv.Itoa(apiErr.Status)
	if code := apiErr.Code(); checkToken.MatchString(code) {
		check += "_" + code
	}
	if len(check) > 64 {
		check = check[:64]
	}
	return check
}

func validVaultID(id string) bool {
	return storage.Reference{RoomID: id, EventID: "$x"}.Valid()
}

// Scope es el ámbito de la bóveda.
func (v *Vault) Scope() storage.Scope { return v.scope }

// VaultID es el identificador público de la bóveda (`!pqv_…`).
func (v *Vault) VaultID() string { return v.vaultID }

// KeyID identifica la identidad híbrida de la bóveda.
func (v *Vault) KeyID() string { return v.keyID }

// SigningStatus dice si su clave de firma es la activa en Sytue ID.
func (v *Vault) SigningStatus() SigningStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.signing
}

// SigningError es ErrSigningKey con el porqué de que la clave de firma no sea la
// activa (SigningKeyError.Check), para el código de fallo; nil si lo es.
func (v *Vault) SigningError() error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.signing == SigningRegistered {
		return nil
	}
	check := v.signingCheck
	if check == "" {
		check = "signing_" + string(v.signing)
	}
	return &SigningKeyError{Check: check}
}

// SigningKeyError es ErrSigningKey con la etiqueta de qué contestó Sytue ID
// (`signing_put_400_vault_key_proof`, `signing_missing`…). errors.Is(err,
// ErrSigningKey) sigue valiendo.
type SigningKeyError struct{ Check string }

func (e *SigningKeyError) Error() string { return ErrSigningKey.Error() + " (" + e.Check + ")" }
func (e *SigningKeyError) Unwrap() error { return ErrSigningKey }

// PublicationTarget es el destino de las publicaciones, el mismo que usa la web:
// la bóveda y un dispositivo `pq-` derivado de la clave.
func (v *Vault) PublicationTarget() storage.PublicationTarget {
	return storage.PQVaultTarget(v.vaultID, v.keyID)
}

// Close borra la clave de propietario y suelta la identidad.
func (v *Vault) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return
	}
	v.closed = true
	clear(v.owner)
	clear(v.signingKey)
	v.owner, v.identity, v.signingKey = nil, nil, nil
}

// keys devuelve la identidad y una copia de la clave de propietario, o ErrClosed.
func (v *Vault) keys() (*pqc.Identity, []byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.closed {
		return nil, nil, ErrClosed
	}
	return v.identity, append([]byte(nil), v.owner...), nil
}
