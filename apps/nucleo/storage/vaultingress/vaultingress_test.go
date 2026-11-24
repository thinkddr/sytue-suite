// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultingress_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/application"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultingress"
)

// La bóveda del correo es un dispositivo completo para el modo de bóveda de
// mail/application, y su capa de ficheros, la de filesdk.
var (
	_ application.Device = (*vaultingress.Device)(nil)
	_ filesdk.SDK        = (*vaultingress.Device)(nil)
)

var scope = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "mail"}

const epoch = "11111111-1111-4111-8111-111111111111"

type banco struct {
	fake   *pqvaulttest.Fake
	cfg    pqvault.Config
	vault  *pqvault.Vault
	device *vaultingress.Device
}

func nuevoBanco(t *testing.T, s storage.Scope) *banco {
	t.Helper()
	fake, drive, account := pqvaulttest.New(t, s)
	cfg := pqvault.Config{Scope: s, Server: drive, Registry: account, UnlockKey: bytes.Repeat([]byte{1}, recoverywrap.UnlockKeyLen)}
	vault, err := pqvault.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := vaultfiles.New(vault)
	if err != nil {
		t.Fatal(err)
	}
	device, err := vaultingress.New(store)
	if err != nil {
		t.Fatal(err)
	}
	return &banco{fake: fake, cfg: cfg, vault: vault, device: device}
}

func lease(context.Context) (func(), error) { return func() {}, nil }

func TestSoloParaProductosQueRecibenYConBoveda(t *testing.T) {
	if _, err := vaultingress.New(nil); !errors.Is(err, pqvault.ErrIdentity) {
		t.Fatal("sin bóveda", err)
	}
	drive := storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "drive"}
	_, server, account := pqvaulttest.New(t, drive)
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: drive, Server: server, Registry: account, UnlockKey: bytes.Repeat([]byte{1}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	store, err := vaultfiles.New(vault)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vaultingress.New(store); !errors.Is(err, pqvault.ErrIdentity) {
		t.Fatal("una bóveda que no recibe correo", err)
	}
}

// Un plan de entrada se guarda en la bóveda y se recupera desde ella; el correo
// que llega cifrado para él se abre con su identidad, que no sale de la bóveda.
func TestPlanDeEntradaYCorreoEntranteSobreLaBoveda(t *testing.T) {
	b := nuevoBanco(t, scope)
	plan, err := b.device.CreateIngressKey(t.Context(), scope, epoch, 1)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := b.device.RecoverIngressEnrollment(t.Context(), plan)
	if err != nil || recovered != plan {
		t.Fatalf("el plan no se recupera tal cual: %v", err)
	}
	// Lo guardado es un registro de la bóveda, con la transacción de la web.
	name := keyring.SecretName(scope, epoch, 1, plan.KeyID)
	raw, found, err := b.vault.ReadIngressSecret(t.Context(), name)
	if err != nil || !found || !json.Valid(raw) {
		t.Fatalf("el secreto no está en la bóveda: %v", err)
	}
	clear(raw)

	body := []byte("From: alguien@example.test\r\n\r\nHola")
	metadata := []byte(`{"from":"alguien@example.test"}`)
	var cipher bytes.Buffer
	task, err := agecipher.Seal(t.Context(), &cipher, plan, "delivery", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	gotMeta, reader, err := b.device.OpenIngressBody(t.Context(), task, bytes.NewReader(cipher.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	gotBody, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(gotMeta, metadata) || !bytes.Equal(gotBody, body) {
		t.Fatalf("el correo entrante cambió: %v", err)
	}

	// Un plan cuyo secreto no está (el de Matrix antes de un traspaso): el correo
	// sabe que tiene que proponer otro, nunca se inventa uno.
	missing := plan
	missing.Generation = 2
	if _, err := b.device.RecoverIngressEnrollment(t.Context(), missing); !errors.Is(err, keyring.ErrRecoveryRequired) {
		t.Fatalf("sin secreto: %v", err)
	}
	other := plan
	other.Scope.Subject = "otra"
	if _, err := b.device.RecoverIngressEnrollment(t.Context(), other); err == nil {
		t.Fatal("recupera el plan de otro ámbito")
	}
}

func accion(t *testing.T, b *banco, kind string, generation int64, now time.Time) matrixproof.Action {
	t.Helper()
	root, err := b.vault.IngressRootKey()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := matrixproof.RequestDigest(map[string]string{"recipient": "age1pq1…"})
	if err != nil {
		t.Fatal(err)
	}
	return matrixproof.Action{
		Version: 1, Generation: generation, Scope: scope, Epoch: epoch, Kind: kind,
		Target: b.vault.PublicationTarget(), RootKey: root, RequestSHA256: digest,
		Nonce: "server_challenge", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli(),
	}
}

func TestFirmaDeEntradaConLaClaveDeLaBoveda(t *testing.T) {
	b := nuevoBanco(t, scope)
	var public recoverywrap.PQVaultVerifyKey
	if len(b.fake.Signing) != 1 || json.Unmarshal(b.fake.Signing[0].PublicKey, &public) != nil {
		t.Fatal("la clave de firma no está registrada")
	}
	reg := vaultproof.Registered{Generation: 1, KeyID: b.fake.Signing[0].KeyID, Public: public}
	now := time.Now()
	action := accion(t, b, "key.enroll", 1, now)
	proof, err := b.device.SignIngressAction(t.Context(), lease, action.Intent(), action)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vaultproof.Verify(action, proof, reg, now); err != nil {
		t.Fatalf("no verifica contra Sytue ID: %v", err)
	}
	if _, err := b.device.SignIngressAction(t.Context(), nil, action.Intent(), action); !errors.Is(err, offline.ErrWorkLease) {
		t.Fatalf("firma sin arrendamiento: %v", err)
	}
	moved := action
	moved.Target = storage.PublicationTarget{RoomID: "!otra:sytue-vault", DeviceID: "pq-otra"}
	if _, err := b.device.SignIngressAction(t.Context(), lease, moved.Intent(), moved); err == nil {
		t.Fatal("firma para otro destino")
	}
	changed := action
	changed.RequestSHA256 = strings.Repeat("0", 64)
	if _, err := b.device.SignIngressAction(t.Context(), lease, action.Intent(), changed); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("firma algo distinto de lo pedido: %v", err)
	}
	root := accion(t, b, "root.rotate", 1, now)
	if _, err := b.device.SignIngressAction(t.Context(), lease, root.Intent(), root); !errors.Is(err, matrixproof.ErrUnverified) {
		t.Fatalf("firma una acción de raíz: %v", err)
	}
}

// La autoridad del correo tiene que ser la de esta bóveda: su identidad, su raíz
// y activa. La de Matrix no abre aquí.
func TestLaAutoridadTieneQueSerLaDeEstaBoveda(t *testing.T) {
	b := nuevoBanco(t, scope)
	root, err := b.vault.IngressRootKey()
	if err != nil {
		t.Fatal(err)
	}
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: vaultproof.UserID(scope), RootKey: root, Generation: 3, Status: "active"}
	if err := b.device.RequireStorageAuthority(authority); err != nil {
		t.Fatalf("la autoridad de esta bóveda: %v", err)
	}
	matrix := authority
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scope) + "matrix.sytue.eu"
	otra := authority
	otra.RootKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	revocada := authority
	revocada.Status = "revoked"
	for name, a := range map[string]matrixproof.Authority{"de Matrix": matrix, "de otra clave": otra, "revocada": revocada} {
		if err := b.device.RequireStorageAuthority(a); !errors.Is(err, vaultingress.ErrAuthority) {
			t.Fatalf("autoridad %s aceptada: %v", name, err)
		}
	}
}

// Una entrega reclamada publica solo mientras su reclamación vale.
func TestPublicacionAutorizadaComprobadaAntesDeEnviar(t *testing.T) {
	b := nuevoBanco(t, scope)
	target := b.vault.PublicationTarget()
	caducada := errors.New("reclamación caducada")
	ctx, err := b.device.RequireAuthorizedStoragePublication(t.Context(), scope, target, func(context.Context) error { return caducada })
	if err != nil {
		t.Fatal(err)
	}
	before := b.fake.RecordCount()
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "message_1", RevisionID: "rev_1", Role: "content", Payload: []byte(`{}`)}
	if _, err := b.device.PutStorageRecordAt(ctx, target, "txn_1", record); !errors.Is(err, caducada) || b.fake.RecordCount() != before {
		t.Fatalf("publica con la reclamación caducada: %v", err)
	}
	if _, err := b.device.RequireAuthorizedStoragePublication(t.Context(), storage.Scope{Tenant: "acme", Subject: "otra", Product: "mail"}, target, func(context.Context) error { return nil }); err == nil {
		t.Fatal("autoriza otro ámbito")
	}
	if _, err := b.device.RequireRecoverableIngress(t.Context(), scope, storage.PublicationTarget{RoomID: "!otra:sytue-vault", DeviceID: "pq-otra"}); err == nil {
		t.Fatal("recupera para otro destino")
	}
}
