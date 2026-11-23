// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

func newFake(t *testing.T, scope storage.Scope) (*pqvaulttest.Fake, Config) {
	t.Helper()
	fake, drive, account := pqvaulttest.New(t, scope)
	return fake, Config{Scope: scope, Server: drive, Registry: account}
}

var testScope = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "contacts"}

func unlockKey(b byte) []byte { return bytes.Repeat([]byte{b}, recoverywrap.UnlockKeyLen) }

func withKey(cfg Config, key []byte) Config {
	cfg.UnlockKey = key
	return cfg
}

func record(scope storage.Scope, object, revision string, payload []byte, previous ...string) storage.Record {
	return storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: revision, Role: "content", Previous: previous, Payload: payload}
}

func TestOpenCreatesVaultOnceAndRegistersSigningKey(t *testing.T) {
	server, cfg := newFake(t, testScope)
	first, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if first.SigningStatus() != SigningRegistered || len(server.Signing) != 1 || server.Signing[0].Generation != 1 {
		t.Fatalf("la clave de firma no quedó registrada como alta: %s %+v", first.SigningStatus(), server.Signing)
	}
	if got := server.Resets(); len(got) != 1 || got[0] {
		t.Fatalf("crear una bóveda nueva no debe vaciar nada: %v", got)
	}
	second, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.VaultID() != first.VaultID() || second.KeyID() != first.KeyID() {
		t.Fatal("volver a abrir con la misma clave cambió de bóveda o de identidad")
	}
	if len(server.Resets()) != 1 || len(server.Signing) != 1 || second.SigningStatus() != SigningRegistered {
		t.Fatal("abrir una bóveda existente no debe volver a crearla ni a registrar su firma")
	}
	if target := second.PublicationTarget(); !target.Valid() || target.RoomID != second.VaultID() || !strings.HasPrefix(target.DeviceID, "pq-") {
		t.Fatalf("destino de publicación distinto del de la web: %+v", target)
	}
}

func TestOpenWithoutUnlockKeyAsksToSignIn(t *testing.T) {
	_, cfg := newFake(t, testScope)
	for _, key := range [][]byte{nil, unlockKey(1)[:16]} {
		if _, err := Open(t.Context(), withKey(cfg, key)); !errors.Is(err, ErrSignin) {
			t.Fatalf("sin clave de desbloqueo hay que volver a entrar, no %v", err)
		}
	}
}

func TestRecordsRoundTripIncludingFragments(t *testing.T) {
	_, cfg := newFake(t, testScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	small := record(testScope, "contact_1", "rev_1", []byte(`{"name":"Ana"}`))
	ref, err := vault.Put(t.Context(), "txn_small", small)
	if err != nil {
		t.Fatal(err)
	}
	got, err := vault.Read(t.Context(), ref)
	if err != nil || !small.MatchesExact(got) {
		t.Fatalf("el registro no vuelve igual: %v", err)
	}
	// Repetir la transacción devuelve el registro ya aceptado, no uno nuevo.
	again, err := vault.Put(t.Context(), "txn_small", small)
	if err != nil || again != ref {
		t.Fatalf("la transacción no es idempotente: %v %v", again, err)
	}
	big := record(testScope, "contact_2", "rev_1", []byte(`{"note":"`+strings.Repeat("x", 29<<10)+`"}`), ref.EventID)
	bigRef, err := vault.Put(t.Context(), "txn_big", big)
	if err != nil {
		t.Fatal(err)
	}
	page, err := vault.History(t.Context(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Entries[0].Reference != bigRef || !big.MatchesExact(page.Entries[0].Record) || page.Entries[1].Reference != ref {
		t.Fatalf("el historial no devuelve los dos registros, el último primero: %+v", page.Entries)
	}
	if page.Scanned <= 2 {
		t.Fatal("el registro grande debía ir en fragmentos")
	}
}

func TestWrongKeyOnEmptyVaultResetsSilently(t *testing.T) {
	server, cfg := newFake(t, testScope)
	old, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	fresh, err := Open(t.Context(), withKey(cfg, unlockKey(2)))
	if err != nil {
		t.Fatalf("una bóveda sin registros se reinicia sola: %v", err)
	}
	defer fresh.Close()
	if fresh.VaultID() == old.VaultID() || fresh.KeyID() == old.KeyID() {
		t.Fatal("el reinicio no estrenó bóveda ni identidad")
	}
	if got := server.Resets(); len(got) != 2 || !got[1] {
		t.Fatalf("el reinicio silencioso debe pedir reset al servidor: %v", got)
	}
	// Había otra clave de firma activa: sustituirla pide identificarse otra vez,
	// pero la bóveda se abre igual.
	if fresh.SigningStatus() != SigningReauth {
		t.Fatalf("estado de la clave de firma: %s", fresh.SigningStatus())
	}
}

func TestWrongKeyWithDataNeedsAChoice(t *testing.T) {
	server, cfg := newFake(t, testScope)
	old, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Put(t.Context(), "txn_1", record(testScope, "contact_1", "rev_1", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	old.Close()
	if _, err := Open(t.Context(), withKey(cfg, unlockKey(2))); !errors.Is(err, ErrWrong) {
		t.Fatalf("con datos, una clave que no abre no reinicia nada: %v", err)
	}
	if server.RecordCount() != 1 || len(server.Resets()) != 1 || server.VaultID != old.VaultID() {
		t.Fatal("abrir con otra clave tocó la bóveda")
	}
	// Ante la duda (el servidor no contesta a «¿está vacía?»), tampoco.
	server.FailList = true
	server.Records = nil
	if _, err := Open(t.Context(), withKey(cfg, unlockKey(2))); !errors.Is(err, ErrWrong) {
		t.Fatalf("sin saber si está vacía, no se reinicia: %v", err)
	}
	if len(server.Resets()) != 1 {
		t.Fatal("se reinició sin saber si había datos")
	}
}

func TestConfirmedResetEmptiesTheVault(t *testing.T) {
	server, cfg := newFake(t, testScope)
	server.RecentLogin = true
	old, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Put(t.Context(), "txn_1", record(testScope, "contact_1", "rev_1", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	old.Close()
	fresh, err := Reset(t.Context(), withKey(cfg, unlockKey(2)))
	if err != nil {
		t.Fatal(err)
	}
	if server.RecordCount() != 0 || fresh.VaultID() == old.VaultID() {
		t.Fatal("vaciar debe borrar los registros y estrenar la bóveda")
	}
	if _, err := fresh.Put(t.Context(), "txn_2", record(testScope, "contact_2", "rev_1", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	fresh.Close()
	if fresh.SigningStatus() != SigningRegistered || len(server.Signing) != 2 || server.Signing[0].Generation != 2 {
		t.Fatalf("tras vaciar con identificación reciente la firma nueva queda activa: %s", fresh.SigningStatus())
	}
	reopened, err := Open(t.Context(), withKey(cfg, unlockKey(2)))
	if err != nil || reopened.VaultID() != fresh.VaultID() {
		t.Fatalf("la bóveda nueva se abre con la clave nueva: %v", err)
	}
	reopened.Close()
	if _, err := Open(t.Context(), withKey(cfg, unlockKey(1))); !errors.Is(err, ErrWrong) {
		t.Fatalf("la clave vieja no abre la bóveda nueva: %v", err)
	}
}

// Vaciar es condicional: si entre ver la bóveda y vaciarla alguien publica (la
// web termina su importación y deja el recibo), el servidor lo rechaza y no se
// borra nada. Con lo visto ya viejo, ni se pregunta al servidor.
func TestResetRefusesWhenTheVaultChangedSinceItWasSeen(t *testing.T) {
	server, cfg := newFake(t, testScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Put(t.Context(), "copia_1", record(testScope, "contact_1", "rev_1", []byte(`{}`))); err != nil {
		t.Fatal(err)
	}
	vault.Close()
	seen, err := cfg.Server.PQVaultKeys(t.Context(), testScope)
	if err != nil {
		t.Fatal(err)
	}
	expect := api.PQVaultExpect{VaultID: seen.VaultID, Version: seen.Version}
	// La otra mitad de la carrera: el recibo llega justo antes de vaciar.
	server.BeforeReset = func(f *pqvaulttest.Fake) {
		if !f.AppendOther("recibo_web", json.RawMessage(`{"org.sytue.storage_pq":{}}`)) {
			t.Error("la web no pudo publicar su recibo")
		}
	}
	vaciar := withKey(cfg, unlockKey(1))
	vaciar.Expect = &expect
	if _, err := Reset(t.Context(), vaciar); !errors.Is(err, ErrChanged) {
		t.Fatalf("se vació una bóveda con un recibo nuevo: %v", err)
	}
	if server.RecordCount() != 2 {
		t.Fatal("el vaciado rechazado borró registros", server.RecordCount())
	}
	// Con lo visto ya viejo, Reset ni lo intenta.
	resets := len(server.Resets())
	if _, err := Reset(t.Context(), vaciar); !errors.Is(err, ErrChanged) || len(server.Resets()) != resets {
		t.Fatalf("vaciar con lo visto antes del recibo: %v", err)
	}
	if server.RecordCount() != 2 {
		t.Fatal("se borró el recibo")
	}
}

func TestTamperedProofIsRejected(t *testing.T) {
	server, cfg := newFake(t, testScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	ref, err := vault.Put(t.Context(), "txn_1", record(testScope, "contact_1", "rev_1", []byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	// Otra clave de propietario: el sobre abre, la prueba no.
	vault.mu.Lock()
	vault.owner[0] ^= 1
	vault.mu.Unlock()
	if _, err := vault.Read(t.Context(), ref); !errors.Is(err, ErrIdentity) {
		t.Fatalf("una prueba que no verifica no se acepta: %v", err)
	}
	if _, err := vault.History(t.Context(), "", 10); !errors.Is(err, ErrIdentity) {
		t.Fatalf("ni en el historial: %v", err)
	}
	_ = server
}

type webVector struct {
	UnlockKey []byte                      `json:"unlock_key"`
	Tenant    string                      `json:"tenant"`
	Product   string                      `json:"product"`
	Subject   string                      `json:"subject"`
	Secret    recoverywrap.PQVaultSecret  `json:"secret"`
	Wrapper   recoverywrap.PQVaultWrapper `json:"wrapper"`
}

type recordVector struct {
	VaultID string          `json:"vault_id"`
	EventID string          `json:"event_id"`
	Record  storage.Record  `json:"record"`
	Digest  string          `json:"digest"`
	MAC     string          `json:"mac"`
	Proof   json.RawMessage `json:"proof"`
	Payload json.RawMessage `json:"payload"`
}

func loadVectors(t *testing.T) (webVector, recordVector) {
	t.Helper()
	var wrap webVector
	var rec recordVector
	for name, out := range map[string]any{"boveda_pq.json": &wrap, "boveda_pq_registro.json": &rec} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "recoverywrap", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
	}
	return wrap, rec
}

// La prueba de propietario es la de pq-vault.ts: el vector se comprobó con una
// copia literal de computeRecordDigest/signProof en JavaScript.
func TestOwnerProofMatchesWebVector(t *testing.T) {
	wrap, rec := loadVectors(t)
	digest, err := RecordDigest(rec.VaultID, rec.Record)
	if err != nil || digest != rec.Digest {
		t.Fatalf("resumen distinto del de la web: %s %v", digest, err)
	}
	owner, _ := base64.StdEncoding.DecodeString(wrap.Secret.OwnerKey)
	if got := base64.StdEncoding.EncodeToString(RecordMAC(owner, digest)); got != rec.MAC {
		t.Fatalf("HMAC distinto del de la web: %s", got)
	}
}

// Lo que dejó la web —su envoltorio sellado por el puente y un registro sellado por
// su núcleo Wasm con su prueba— lo abre la app nativa con la misma clave.
func TestNativeOpensWhatTheWebStored(t *testing.T) {
	wrap, rec := loadVectors(t)
	scope := storage.Scope{Tenant: wrap.Tenant, Subject: wrap.Subject, Product: wrap.Product}
	server, cfg := newFake(t, scope)
	server.VaultID, server.ActiveKeyID = rec.VaultID, wrap.Wrapper.KeyID
	server.Keys = []recoverywrap.PQVaultWrapper{wrap.Wrapper}
	server.Seq = 1
	server.Records = []api.PQVaultRecord{{EventID: rec.EventID, VaultID: rec.VaultID, Seq: 1, Payload: rec.Payload}}
	vault, err := Open(t.Context(), withKey(cfg, wrap.UnlockKey))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if vault.VaultID() != rec.VaultID || vault.KeyID() != wrap.Wrapper.KeyID {
		t.Fatal("la app nativa abrió otra bóveda")
	}
	got, err := vault.Read(t.Context(), storage.Reference{RoomID: rec.VaultID, EventID: rec.EventID})
	if err != nil || !rec.Record.MatchesExact(got) {
		t.Fatalf("el registro de la web no se abre igual: %v", err)
	}
	// La bóveda de la web no tenía clave de firma: la gana al abrirse, sin reiniciar.
	resets := server.Resets()
	if len(resets) != 1 || resets[0] || len(server.Keys) != 1 {
		t.Fatalf("añadir la clave de firma no debe vaciar ni duplicar la bóveda: %v", resets)
	}
	opened, err := recoverywrap.OpenPQVault(wrap.UnlockKey, scope.Tenant, scope.Product, scope.Subject, server.Keys[0])
	if err != nil || opened.SigningKey == "" || opened.OwnerKey != wrap.Secret.OwnerKey || opened.Identity != wrap.Secret.Identity {
		t.Fatal("el envoltorio nuevo debe conservar la identidad y la clave de propietario y sumar la de firma")
	}
	// Y lo que guarda la app nativa lo verifica la prueba de la web.
	own := record(scope, "file_native", "rev_1", []byte(`{"from":"fyne"}`))
	ref, err := vault.Put(t.Context(), "txn_native", own)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Read(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

// Una transacción que ya pertenece a otro registro aceptado no se da por buena: lo
// que se lee de vuelta tiene que ser exactamente lo que se quería guardar.
func TestPutChecksTheAcceptedRecord(t *testing.T) {
	_, cfg := newFake(t, testScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	if _, err := vault.Put(t.Context(), "txn_1", record(testScope, "contact_1", "rev_1", []byte(`{"a":1}`))); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Put(t.Context(), "txn_1", record(testScope, "contact_1", "rev_2", []byte(`{"a":2}`))); !errors.Is(err, ErrIdentity) {
		t.Fatalf("otro registro bajo la misma transacción: %v", err)
	}
}
