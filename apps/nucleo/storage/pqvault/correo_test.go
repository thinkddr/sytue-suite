// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

var mailScope = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "mail"}

// El traspaso del correo de Matrix a la bóveda (ADR 0087): la bóveda nueva
// registra su clave con la marca de traspaso. Sin identificación reciente queda
// en SigningReauth; al volver de identificarse, se registra la MISMA bóveda, sin
// vaciarla otra vez.
func TestTraspasoSinIdentificacionRecienteYAlVolverNoVaciaOtraVez(t *testing.T) {
	server, cfg := newFake(t, mailScope)
	cfg.Handover = true
	vault, err := Reset(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if vault.SigningStatus() != SigningReauth || server.Handovers != 1 || len(server.Signing) != 0 {
		t.Fatalf("un traspaso sin identificación reciente no queda registrado: %s %+v", vault.SigningStatus(), server.Signing)
	}
	if _, err := vault.IngressRootKey(); err != nil {
		t.Fatalf("la bóveda conserva su clave de firma: %v", err)
	}
	resets := len(server.Resets())
	server.RecentLogin = true
	if got := vault.RegisterSigningKey(t.Context(), cfg.Registry, true); got != SigningRegistered || vault.SigningStatus() != SigningRegistered {
		t.Fatalf("al volver de identificarse, el traspaso se registra: %s", got)
	}
	if len(server.Signing) != 1 || server.Signing[0].Reason != "traspaso" || len(server.Resets()) != resets {
		t.Fatalf("el traspaso se registra una vez y sin vaciar otra vez: %+v %v", server.Signing, server.Resets())
	}
	// Repetirlo (una respuesta perdida) devuelve el ya hecho.
	if got := vault.RegisterSigningKey(t.Context(), cfg.Registry, true); got != SigningRegistered || len(server.Signing) != 1 {
		t.Fatalf("repetir el traspaso cambió algo: %s %+v", got, server.Signing)
	}
	// Abrirla otra vez sin marca: la misma clave, ya activa.
	again, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.VaultID() != vault.VaultID() || again.SigningStatus() != SigningRegistered {
		t.Fatal("la bóveda del traspaso no se vuelve a abrir tal cual")
	}
}

// Si otro dispositivo registró entre la lectura y el registro, Sytue ID contesta
// `409 vault_key_stale`: se vuelve a leer la activa y se repite una vez, como la web.
func TestClaveDeFirmaRanciaSeRepiteUnaVez(t *testing.T) {
	server, cfg := newFake(t, mailScope)
	server.StaleOnce = true
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if vault.SigningStatus() != SigningRegistered || vault.SigningError() != nil || len(server.Signing) != 1 {
		t.Fatalf("tras un vault_key_stale se repite y queda registrada: %s %v", vault.SigningStatus(), vault.SigningError())
	}
}

// Un rechazo de Sytue ID queda dicho: qué petición, qué estado y qué código, la
// misma etiqueta que enseña la web en «Más información».
func TestClaveDeFirmaRechazadaDiceElPorque(t *testing.T) {
	server, cfg := newFake(t, mailScope)
	server.BadProof = true
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if vault.SigningStatus() != SigningUnavailable {
		t.Fatalf("estado = %s", vault.SigningStatus())
	}
	err = vault.SigningError()
	var detalle *SigningKeyError
	if !errors.Is(err, ErrSigningKey) || !errors.As(err, &detalle) || detalle.Check != "signing_put_400_vault_key_proof" {
		t.Fatalf("el porqué del rechazo: %v", err)
	}
}

// El traspaso es solo del correo, y nunca se cuelga de una clave que ya estaba
// registrada sin él.
func TestTraspasoSoloEnElCorreoYConClaveNueva(t *testing.T) {
	server, cfg := newFake(t, testScope)
	server.RecentLogin = true
	cfg.Handover = true
	other, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if other.SigningStatus() != SigningUnavailable || server.Handovers != 0 || len(server.Signing) != 0 {
		t.Fatalf("un traspaso fuera del correo ni se pide: %s", other.SigningStatus())
	}

	server, cfg = newFake(t, mailScope)
	server.RecentLogin = true
	plain, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if plain.SigningStatus() != SigningRegistered || server.Signing[0].Reason != "alta" {
		t.Fatalf("alta: %s %+v", plain.SigningStatus(), server.Signing)
	}
	if got := plain.RegisterSigningKey(t.Context(), cfg.Registry, true); got != SigningUnavailable || len(server.Signing) != 1 {
		t.Fatalf("un traspaso colgado de la clave de siempre se acepta: %s", got)
	}
}

func TestSecretosDeEntradaEnLaBoveda(t *testing.T) {
	_, cfg := newFake(t, mailScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	name := ingressSecretPrefix + strings.Repeat("ab", 32)
	if _, found, err := vault.ReadIngressSecret(t.Context(), name); err != nil || found {
		t.Fatalf("un secreto que no está: %v %v", found, err)
	}
	value := []byte(`{"v":1,"identity":"secreto"}`)
	if err := vault.WriteIngressSecret(t.Context(), name, value); err != nil {
		t.Fatal(err)
	}
	got, found, err := vault.ReadIngressSecret(t.Context(), name)
	if err != nil || !found || !bytes.Equal(got, value) {
		t.Fatalf("se lee lo escrito: %q %v %v", got, found, err)
	}
	if err := vault.WriteIngressSecret(t.Context(), name, value); err != nil {
		t.Fatalf("repetir la misma escritura vale: %v", err)
	}
	if err := vault.WriteIngressSecret(t.Context(), name, []byte(`{"v":1,"identity":"otro"}`)); err == nil {
		t.Fatal("un secreto de entrada se sustituye")
	}
	for _, bad := range []string{"org.sytue.ingress.key.v1.ab", "otro." + strings.Repeat("ab", 32), ingressSecretPrefix + strings.Repeat("AB", 32)} {
		if err := vault.WriteIngressSecret(t.Context(), bad, value); !errors.Is(err, storage.ErrRecord) {
			t.Fatalf("nombre %q aceptado: %v", bad, err)
		}
	}
	if err := vault.WriteIngressSecret(t.Context(), name, []byte("no es json")); !errors.Is(err, storage.ErrRecord) {
		t.Fatalf("un secreto que no es JSON se guarda: %v", err)
	}
	vault.Close()
	if _, _, err := vault.ReadIngressSecret(t.Context(), name); !errors.Is(err, ErrClosed) {
		t.Fatalf("una bóveda cerrada lee secretos: %v", err)
	}
}

// Lo que la web guardó lo lee el nativo: la misma transacción, el mismo objeto.
func TestSecretoDeEntradaSinBuscadorFalla(t *testing.T) {
	_, cfg := newFake(t, mailScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	vault.server = sinBuscador{vault.server}
	if _, _, err := vault.ReadIngressSecret(t.Context(), ingressSecretPrefix+strings.Repeat("ab", 32)); !errors.Is(err, ErrNoFinder) {
		t.Fatalf("sin búsqueda por transacción no se adivina: %v", err)
	}
}

type sinBuscador struct{ Server }

func registered(t *testing.T, server *pqvaulttest.Fake) vaultproof.Registered {
	t.Helper()
	var public recoverywrap.PQVaultVerifyKey
	if len(server.Signing) == 0 || json.Unmarshal(server.Signing[0].PublicKey, &public) != nil {
		t.Fatal("sin clave registrada")
	}
	return vaultproof.Registered{Generation: server.Signing[0].Generation, KeyID: server.Signing[0].KeyID, Public: public}
}

func accionDeEntrada(t *testing.T, vault *Vault, generation int64, kind string, now time.Time) matrixproof.Action {
	t.Helper()
	root, err := vault.IngressRootKey()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := matrixproof.RequestDigest(map[string]string{"recipient": "age1pq1…"})
	if err != nil {
		t.Fatal(err)
	}
	return matrixproof.Action{
		Version: 1, Generation: generation, Scope: vault.Scope(), Epoch: "11111111-1111-4111-8111-111111111111",
		Kind: kind, Target: vault.PublicationTarget(), RootKey: root, RequestSHA256: digest,
		Nonce: "server_challenge", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli(),
	}
}

// Las acciones de entrada se firman con la clave de la bóveda y el servidor las
// verifica contra la activa en Sytue ID; sin ella activa, no se firma nada.
func TestFirmaDeAccionesDeEntrada(t *testing.T) {
	server, cfg := newFake(t, mailScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	reg := registered(t, server)
	now := time.Now()
	action := accionDeEntrada(t, vault, reg.Generation, "task.claim", now)
	proof, err := vault.SignIngressAction(action.Intent(), action, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vaultproof.Verify(action, proof, reg, now); err != nil {
		t.Fatalf("la firma de la bóveda no verifica contra Sytue ID: %v", err)
	}
	if vault.IngressUserID() != vaultproof.UserID(mailScope) {
		t.Fatal("identidad ante el correo distinta de la de la web")
	}
	other := action
	other.Scope.Subject = "otra"
	if _, err := vault.SignIngressAction(other.Intent(), other, now); !errors.Is(err, ErrIdentity) {
		t.Fatalf("firma una acción de otro ámbito: %v", err)
	}
	root := action
	root.Kind = "root.enroll"
	if _, err := vault.SignIngressAction(root.Intent(), root, now); err == nil {
		t.Fatal("firma una acción de raíz")
	}

	// Tras un vaciado sin identificación reciente, no firma.
	reset, err := Reset(t.Context(), withKey(cfg, unlockKey(2)))
	if err != nil {
		t.Fatal(err)
	}
	defer reset.Close()
	if reset.SigningStatus() != SigningReauth {
		t.Fatalf("estado: %s", reset.SigningStatus())
	}
	pending := accionDeEntrada(t, reset, reg.Generation+1, "task.claim", now)
	if _, err := reset.SignIngressAction(pending.Intent(), pending, now); !errors.Is(err, ErrSigningKey) {
		t.Fatalf("firma sin clave activa: %v", err)
	}
	reset.Close()
	if _, err := reset.IngressRootKey(); !errors.Is(err, ErrClosed) {
		t.Fatalf("una bóveda cerrada da su raíz: %v", err)
	}
}

// La política de una publicación autorizada se comprueba antes de cada envío.
func TestPoliticaDePublicacionAntesDeCadaEnvio(t *testing.T) {
	server, cfg := newFake(t, mailScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	denied := errors.New("reclamación caducada")
	allow := true
	calls := 0
	ctx, err := vault.WithPublicationPolicy(t.Context(), vault.PublicationTarget(), func(context.Context) error {
		calls++
		if !allow {
			return denied
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Put(ctx, "txn_1", record(mailScope, "message_1", "rev_1", []byte(`{}`))); err != nil || calls != 1 {
		t.Fatalf("con la política en vigor se publica: %v (%d)", err, calls)
	}
	allow = false
	before := server.RecordCount()
	if _, err := vault.Put(ctx, "txn_2", record(mailScope, "message_2", "rev_1", []byte(`{}`))); !errors.Is(err, denied) || server.RecordCount() != before {
		t.Fatalf("con la política caducada no sale nada: %v", err)
	}
	if _, err := vault.WithPublicationPolicy(ctx, vault.PublicationTarget(), func(context.Context) error { return nil }); !errors.Is(err, ErrIdentity) {
		t.Fatalf("una política sustituye a otra: %v", err)
	}
	if _, err := vault.WithPublicationPolicy(t.Context(), storage.PublicationTarget{RoomID: "!otra:sytue-vault", DeviceID: "pq-x"}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("una política para otro destino")
	}
	if _, err := vault.WithPublicationPolicy(t.Context(), vault.PublicationTarget(), nil); !errors.Is(err, ErrIdentity) {
		t.Fatalf("una política vacía: %v", err)
	}
}
