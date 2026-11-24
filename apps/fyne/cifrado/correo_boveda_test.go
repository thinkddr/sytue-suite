// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultingress"
)

// roomsAutoridad es Rooms sirviendo solo la autoridad del correo.
type roomsAutoridad struct {
	mu        sync.Mutex
	autoridad matrixproof.Authority
	lecturas  int
}

func (r *roomsAutoridad) poner(a matrixproof.Authority) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoridad = a
}

func (r *roomsAutoridad) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !strings.HasSuffix(req.URL.Path, "/matrix/storage/mail/authority") {
		http.NotFound(w, req)
		return
	}
	if got, err := storage.DecodeScopeHeader(req.Header.Get(storage.ScopeHeader)); err != nil || got != scopeCorreo {
		http.Error(w, "scope", http.StatusConflict)
		return
	}
	r.lecturas++
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(r.autoridad)
}

// bancoCorreoBoveda: la sesión de la app de verdad (testigo, arrendamiento y
// HTTP), el servidor falso de la bóveda del correo, su diario y Rooms.
type bancoCorreoBoveda struct {
	fake    *pqvaulttest.Fake
	drive   *api.Drive
	account *api.Account
	fabrica FabricaBoveda
	ctx     context.Context
	rooms   *roomsAutoridad
	config  CorreoBovedaConfig
}

func nuevoBancoCorreoBoveda(t *testing.T) *bancoCorreoBoveda {
	t.Helper()
	session := auth.NewSession(factoryTokens("alice"), nil, nil)
	ctx := session.WithAuthorizationIntent(t.Context())
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("APPDATA", configDir)
	t.Setenv("HOME", configDir)
	ports, err := nativestoragetest.NewSessionPorts(ctx, session, "sesion-cccccccccccccccccccccccccccccccc", func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	factory := Factory[*factoryDevice]{Product: "mail", CaptureWork: ports.CaptureWork, RegisterWork: ports.RegisterWork, CurrentIdentity: ports.CurrentIdentity, BindHTTP: ports.BindHTTP}
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	b := &bancoCorreoBoveda{ctx: ctx, rooms: &roomsAutoridad{}}
	b.fake, b.drive, b.account = pqvaulttest.New(t, scopeCorreo)
	b.fabrica = FabricaBoveda{
		Capturar: factory.Capture,
		Diario: func(ctx context.Context, witness auth.TestigoTrabajo) (*offline.Store, error) {
			if err := witness.Validar(); err != nil {
				return nil, err
			}
			return journal, ctx.Err()
		},
	}
	rooms := httptest.NewServer(b.rooms)
	t.Cleanup(rooms.Close)
	b.config = CorreoBovedaConfig{MailURL: "http://127.0.0.1:1", DriveURL: "http://127.0.0.1:1", RoomsURL: rooms.URL}
	return b
}

func (b *bancoCorreoBoveda) abrirBoveda(t *testing.T) *pqvault.Vault {
	t.Helper()
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scopeCorreo, Server: b.drive, Registry: b.account, UnlockKey: claveBoveda(5)})
	if err != nil {
		t.Fatal(err)
	}
	return vault
}

func autoridadDeBoveda(t *testing.T, vault *pqvault.Vault, generation int64) matrixproof.Authority {
	t.Helper()
	root, err := vault.IngressRootKey()
	if err != nil {
		t.Fatal(err)
	}
	return matrixproof.Authority{Version: 1, Scope: scopeCorreo, UserID: vaultproof.UserID(scopeCorreo), RootKey: root, Generation: generation, Status: "active"}
}

// El correo de la bóveda solo se monta si la autoridad del correo que da Rooms es
// la de ESTA bóveda; si es la de Matrix (o la de otra clave), no se abre nada y la
// bóveda se cierra.
func TestCorreoBovedaExigeLaAutoridadDeEstaBoveda(t *testing.T) {
	b := nuevoBancoCorreoBoveda(t)
	vault := b.abrirBoveda(t)
	matrix := autoridadDeBoveda(t, vault, 5)
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scopeCorreo) + "matrix.sytue.eu"
	b.rooms.poner(matrix)
	if _, _, err := AbrirCorreoBoveda(b.ctx, b.fabrica, b.config, vault); !errors.Is(err, vaultingress.ErrAuthority) {
		t.Fatalf("con la autoridad de Matrix se abrió el correo en la bóveda: %v", err)
	}
	if !vault.Closed() {
		t.Fatal("la bóveda rechazada quedó abierta")
	}

	vault = b.abrirBoveda(t)
	b.rooms.poner(autoridadDeBoveda(t, vault, 6))
	recursos, repository, err := AbrirCorreoBoveda(b.ctx, b.fabrica, b.config, vault)
	if err != nil {
		t.Fatal(err)
	}
	defer recursos.Close()
	if repository.Application() == nil || repository.Application().PrivateStorageScope() != scopeCorreo || recursos.PrivateStorageScope() != scopeCorreo {
		t.Fatal("el correo de la bóveda no es el de la cuenta")
	}
	if _, err := repository.Application().ReadyRepository(recursos.Context()); err == nil {
		t.Fatal("el correo se da por listo sin preparar")
	}
	repository.Close()
	recursos.Close()
	if !vault.Closed() {
		t.Fatal("cerrar los recursos no cerró la bóveda")
	}
}

// Sin la clave de firma activa en Sytue ID, el correo no se monta: el servidor no
// aceptaría nada de lo que firmara.
func TestCorreoBovedaSinFirmaActivaNoAbre(t *testing.T) {
	b := nuevoBancoCorreoBoveda(t)
	first := b.abrirBoveda(t)
	first.Close()
	// Otra clave de desbloqueo y la bóveda vacía: se reinicia sola, y sustituir la
	// clave de firma pide identificarse otra vez.
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scopeCorreo, Server: b.drive, Registry: b.account, UnlockKey: claveBoveda(6)})
	if err != nil {
		t.Fatal(err)
	}
	if vault.SigningStatus() != pqvault.SigningReauth {
		t.Fatalf("estado: %s", vault.SigningStatus())
	}
	if _, _, err := AbrirCorreoBoveda(b.ctx, b.fabrica, b.config, vault); !errors.Is(err, pqvault.ErrSigningKey) || !vault.Closed() {
		t.Fatalf("se abrió el correo sin clave de firma activa: %v", err)
	}
	if b.rooms.lecturas != 0 {
		t.Fatal("se preguntó a Rooms sin clave de firma activa")
	}
}
