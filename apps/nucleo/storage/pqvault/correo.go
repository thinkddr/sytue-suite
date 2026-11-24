// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

// Lo que Sytue Mail necesita de la bóveda (ADR 0087, entrega 4), lo mismo que
// `PqVault` de la web (`signIngressAction`, `readIngressSecret`,
// `writeIngressSecret`, `ingressRootKey`, `ingressUserID`):
//
//   - la clave de firma, que firma las acciones de entrada del correo y que el
//     servidor verifica contra la activa en Sytue ID (vaultproof);
//   - los secretos de entrada, guardados como registros `content` cifrados de la
//     propia bóveda, de una sola escritura, y leídos por su transacción;
//   - la política de publicación de una entrega reclamada: se comprueba justo antes
//     de cada envío al servidor, como `putAuthorized` en la web.

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

var (
	// ErrSigningKey: la bóveda no tiene clave de firma, o no es la activa en Sytue
	// ID. Lo que firmaría no lo aceptaría el servidor; tras un vaciado o un
	// traspaso hay que identificarse otra vez (RegisterSigningKey).
	ErrSigningKey = errors.New("pqvault: la clave de firma de la bóveda no es la activa en Sytue ID")
	// ErrNoFinder: el servidor no sabe buscar un registro por su transacción.
	ErrNoFinder = errors.New("pqvault: el servidor no busca registros por transacción")
)

// ReasonHandover es el motivo con el que Sytue ID guarda una clave registrada
// con la marca de traspaso.
const ReasonHandover = "traspaso"

// RecordFinder busca un registro por la transacción con la que se guardó
// (*api.Drive). Es opcional en Server: solo lo usa el correo.
type RecordFinder interface {
	FindPQVaultRecord(context.Context, storage.Scope, string, string) (api.PQVaultRecord, bool, error)
}

// RegisterSigningKey vuelve a registrar la clave de firma de esta bóveda en Sytue
// ID y guarda el resultado. Es lo que se hace al volver de identificarse otra vez
// tras un vaciado (SigningReauth): la bóveda ya está abierta y no se vuelve a
// vaciar. Con `handover`, con la marca de traspaso (solo `mail`).
func (v *Vault) RegisterSigningKey(ctx context.Context, registry Registry, handover bool) SigningStatus {
	v.mu.RLock()
	closed, encoded := v.closed, string(v.signingKey)
	v.mu.RUnlock()
	if closed {
		return SigningUnavailable
	}
	if encoded == "" {
		return SigningMissing
	}
	status := registerSigningKey(ctx, registry, v.scope, encoded, handover)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return SigningUnavailable
	}
	v.signing = status
	return status
}

func (v *Vault) signer() (*recoverywrap.PQVaultSigner, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.closed {
		return nil, ErrClosed
	}
	if len(v.signingKey) == 0 {
		return nil, ErrSigningKey
	}
	return recoverywrap.ParsePQVaultSigningKey(string(v.signingKey))
}

// IngressUserID es la identidad de la bóveda ante el correo (vaultproof.UserID).
func (v *Vault) IngressUserID() string { return vaultproof.UserID(v.scope) }

// IngressRootKey es la raíz con la que la bóveda aparece ante el correo: la
// mitad Ed25519 de su clave de firma (vaultproof.RootKey).
func (v *Vault) IngressRootKey() (string, error) {
	signer, err := v.signer()
	if err != nil {
		return "", err
	}
	return vaultproof.RootKey(signer.Public()), nil
}

// SignIngressAction firma una acción de entrada del correo con la clave de la
// bóveda, solo si es exactamente la que pidió el cliente (vaultproof.SignIntent)
// y de este ámbito. Sin clave activa en Sytue ID no firma: el servidor no la
// aceptaría.
func (v *Vault) SignIngressAction(expected matrixproof.Intent, action matrixproof.Action, now time.Time) (matrixproof.SignedAction, error) {
	if v.SigningStatus() != SigningRegistered {
		return matrixproof.SignedAction{}, ErrSigningKey
	}
	if action.Scope != v.scope {
		return matrixproof.SignedAction{}, ErrIdentity
	}
	signer, err := v.signer()
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	proof, err := vaultproof.SignIntent(signer, expected, action, now)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	if v.Closed() {
		return matrixproof.SignedAction{}, ErrClosed
	}
	return proof, nil
}

// Find lee y autentica el registro guardado con `transaction`. `found` es falso
// si no hay ninguno.
func (v *Vault) Find(ctx context.Context, transaction string) (record storage.Record, found bool, err error) {
	if !storage.Identifier(transaction) {
		return storage.Record{}, false, storage.ErrRecord
	}
	if _, _, err := v.keys(); err != nil {
		return storage.Record{}, false, err
	}
	finder, ok := v.server.(RecordFinder)
	if !ok {
		return storage.Record{}, false, ErrNoFinder
	}
	saved, found, err := finder.FindPQVaultRecord(ctx, v.scope, v.vaultID, transaction)
	if err != nil || !found {
		return storage.Record{}, false, err
	}
	ref := storage.Reference{RoomID: saved.VaultID, EventID: saved.EventID}
	if saved.VaultID != v.vaultID || !ref.Valid() {
		return storage.Record{}, false, ErrIdentity
	}
	wire, decoded, err := pqc.DecodeStorageMessage(saved.Payload)
	if err != nil || !decoded || wire.Kind == "fragment" {
		return storage.Record{}, false, storage.ErrRecord
	}
	record, err = v.open(ctx, ref, wire)
	if err != nil {
		return storage.Record{}, false, err
	}
	return record, true, nil
}

// Los secretos de entrada del correo (keyring.SecretName): su nombre lleva el
// resumen que los identifica, y el registro y la transacción salen de él, como
// en la web.
const (
	ingressSecretPrefix   = "org.sytue.ingress.key.v1." //nolint:gosec // G101: el prefijo público del nombre, no una credencial.
	maxIngressSecretBytes = 2048
)

func ingressSecretID(name string) (string, error) {
	id, ok := strings.CutPrefix(name, ingressSecretPrefix)
	if !ok || len(id) != 64 {
		return "", storage.ErrRecord
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", storage.ErrRecord
		}
	}
	return id, nil
}

// ReadIngressSecret lee un secreto de entrada del correo. `found` es falso si no
// está en la bóveda: con él, el correo sabe que hace falta otro plan de entrada.
func (v *Vault) ReadIngressSecret(ctx context.Context, name string) (secret []byte, found bool, err error) {
	id, err := ingressSecretID(name)
	if err != nil {
		return nil, false, err
	}
	record, found, err := v.Find(ctx, "ingress_secret_"+id)
	if err != nil || !found {
		return nil, false, err
	}
	if record.ObjectID != "ingress_"+id || record.RevisionID != "v1" || record.Role != "content" || len(record.Previous) != 0 ||
		len(record.Payload) > maxIngressSecretBytes || !json.Valid(record.Payload) {
		clear(record.Payload)
		return nil, false, ErrIdentity
	}
	return record.Payload, true, nil
}

// WriteIngressSecret guarda un secreto de entrada del correo. Es de una sola
// escritura: repetirla con los mismos bytes no hace nada; con otros, falla (nunca
// se sustituye una clave con la que ya se ha podido cifrar correo).
func (v *Vault) WriteIngressSecret(ctx context.Context, name string, value []byte) error {
	id, err := ingressSecretID(name)
	if err != nil {
		return err
	}
	if len(value) > maxIngressSecretBytes || !json.Valid(value) {
		return storage.ErrRecord
	}
	record := storage.Record{Version: 1, Scope: v.scope, ObjectID: "ingress_" + id, RevisionID: "v1", Role: "content", Payload: bytes.Clone(value)}
	defer clear(record.Payload)
	_, err = v.PutStorageRecord(ctx, "ingress_secret_"+id, record)
	if err == nil {
		return nil
	}
	// Un reintento tras una escritura que sí llegó choca con su propio cifrado:
	// vale si lo guardado es lo mismo.
	saved, found, readErr := v.ReadIngressSecret(ctx, name)
	if readErr != nil || !found {
		return err
	}
	defer clear(saved)
	if subtle.ConstantTimeCompare(saved, value) != 1 {
		return err
	}
	return nil
}

type publicationKey struct{}

type publication struct {
	vault  *Vault
	target storage.PublicationTarget
	policy storage.PublicationPolicy
}

// WithPublicationPolicy ata a `ctx` la política de una publicación autorizada (la
// de una entrega reclamada): cada envío a la bóveda hecho con ese contexto la
// comprueba justo antes y, si falla, no envía nada. Una política no sustituye a
// otra.
func (v *Vault) WithPublicationPolicy(ctx context.Context, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if policy == nil {
		return nil, ErrIdentity
	}
	if err := v.CheckStoragePublicationTarget(target); err != nil {
		return nil, err
	}
	if _, found := ctx.Value(publicationKey{}).(publication); found {
		return nil, ErrIdentity
	}
	return context.WithValue(ctx, publicationKey{}, publication{vault: v, target: target, policy: policy}), nil
}

// checkPublication aplica la política de `ctx`, si la hay, antes de un envío.
func (v *Vault) checkPublication(ctx context.Context) error {
	p, found := ctx.Value(publicationKey{}).(publication)
	if !found {
		return nil
	}
	if p.vault != v || p.target != v.PublicationTarget() {
		return ErrIdentity
	}
	return p.policy(ctx)
}
