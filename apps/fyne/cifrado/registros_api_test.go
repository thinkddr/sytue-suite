// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	personaA = auth.IdentitySnapshot{Issuer: "https://id", Client: "contacts", Subject: "persona-a", Organization: "acme"}
	personaB = auth.IdentitySnapshot{Issuer: "https://id", Client: "contacts", Subject: "persona-b", Organization: "acme"}
)

type matrixFalsa struct {
	objeto   fyne.CanvasObject
	retries  int
	retiros  int
	arrancar func() bool
	arranco  []bool
}

func (m *matrixFalsa) Retire() { m.retiros++ }

func (m *matrixFalsa) Objeto() fyne.CanvasObject { return m.objeto }

// Retry es lo que haría PrivateApplication: preguntar a CanStart antes de abrir Matrix.
func (m *matrixFalsa) Retry() {
	m.retries++
	m.arranco = append(m.arranco, m.arrancar())
}

type bancoRegistros struct {
	t        *testing.T
	host     *registros
	matrix   *matrixFalsa
	ui       chan func()
	owner    auth.IdentitySnapshot
	inside   bool
	modos    map[string]string
	fallo    error
	mu       sync.Mutex
	pedidos  []storage.Scope
	montajes []*CuentaAPI
	soltadas int
	apiVista fyne.CanvasObject
}

func nuevoBancoRegistros(t *testing.T) *bancoRegistros {
	t.Helper()
	fynetest.NewTempApp(t)
	b := &bancoRegistros{t: t, ui: make(chan func(), 8), owner: personaA, inside: true, modos: map[string]string{}, apiVista: widget.NewLabel("api")}
	b.matrix = &matrixFalsa{objeto: widget.NewLabel("matrix")}
	b.host = &registros{
		product:   "contacts",
		translate: func(k string) string { return k },
		identity:  func() (auth.IdentitySnapshot, bool) { return b.owner, b.inside },
		session:   func(auth.IdentitySnapshot) api.Session { return nil },
		modo: func(_ context.Context, _ api.Session, scope storage.Scope) (string, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.pedidos = append(b.pedidos, scope)
			if b.fallo != nil {
				return "", b.fallo
			}
			return b.modos[scope.Subject], nil
		},
		montar: func(c Cuenta) (ApplicationView, error) {
			cuenta, _ := c.(*CuentaAPI)
			b.montajes = append(b.montajes, cuenta)
			return ApplicationView{Object: b.apiVista, Detach: func() { b.soltadas++ }}, nil
		},
		dispatch: func(f func()) { b.ui <- f },
		slot:     container.NewStack(),
	}
	b.host.matrix = b.matrix
	b.matrix.arrancar = b.host.matrixPuedeEmpezar
	return b
}

// esperar corre en el «hilo de la interfaz» la vuelta de leer el modo.
func (b *bancoRegistros) esperar() {
	b.t.Helper()
	select {
	case f := <-b.ui:
		f()
	case <-time.After(5 * time.Second):
		b.t.Fatal("la decisión no volvió")
	}
}

func (b *bancoRegistros) enseña() fyne.CanvasObject {
	if len(b.host.slot.Objects) != 1 {
		b.t.Fatalf("la app enseña %d objetos", len(b.host.slot.Objects))
	}
	return b.host.slot.Objects[0]
}

// Una cuenta en `legacy` va por la API, como la web, y Matrix no arranca: abrirlo
// importaría los datos y congelaría la fuente que la web sigue usando.
func TestRegistrosLegacyVaPorLaAPIYNoArrancaMatrix(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = FuenteLegacy
	b.host.adoptar()
	if b.host.matrixPuedeEmpezar() {
		t.Fatal("Matrix puede arrancar antes de decidir")
	}
	b.esperar()
	if len(b.montajes) != 1 || b.enseña() != b.apiVista {
		t.Fatalf("montajes %d; enseña %v", len(b.montajes), b.enseña())
	}
	if b.matrix.retries != 0 || b.host.matrixPuedeEmpezar() {
		t.Fatalf("Matrix se arrancó para una cuenta legacy (retries %d)", b.matrix.retries)
	}
	want := storage.Scope{Tenant: "acme", Subject: "persona-a", Product: "contacts"}
	if len(b.pedidos) != 1 || b.pedidos[0] != want || b.montajes[0].PrivateStorageScope() != want {
		t.Fatalf("ámbito %v", b.pedidos)
	}
	if err := b.montajes[0].Check(context.Background()); err != nil {
		t.Fatalf("la cuenta de la vista no vale: %v", err)
	}
}

// Una fuente ya migrada (sus datos están en Matrix) sigue por Matrix, y solo para
// esa cuenta.
func TestRegistrosMigradaSigueEnMatrix(t *testing.T) {
	for _, modo := range []string{"frozen", "private", "building"} {
		t.Run(modo, func(t *testing.T) {
			b := nuevoBancoRegistros(t)
			b.modos["persona-a"] = modo
			b.host.adoptar()
			b.esperar()
			if len(b.montajes) != 0 || b.enseña() != b.matrix.objeto {
				t.Fatalf("montó la API para una fuente %s", modo)
			}
			if b.matrix.retries != 1 || !b.matrix.arranco[0] {
				t.Fatalf("Matrix no arrancó: %v", b.matrix.arranco)
			}
			b.owner = personaB
			if b.host.matrixPuedeEmpezar() {
				t.Fatal("Matrix puede arrancar para otra cuenta sin decidir")
			}
		})
	}
}

// Cambiar de cuenta suelta la vista de la API, su cuenta deja de valer y se vuelve
// a decidir para la nueva; mientras tanto Matrix no arranca.
func TestRegistrosOtraCuentaSueltaLaVistaYVuelveADecidir(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = FuenteLegacy
	b.modos["persona-b"] = "private"
	b.host.adoptar()
	b.esperar()
	vieja := b.montajes[0]
	b.owner = personaB
	b.host.adoptar()
	if b.soltadas != 1 {
		t.Fatalf("la vista de la cuenta anterior no se soltó (%d)", b.soltadas)
	}
	if err := vieja.Check(context.Background()); err == nil {
		t.Fatal("la cuenta de la vista anterior sigue valiendo")
	}
	if b.host.matrixPuedeEmpezar() {
		t.Fatal("Matrix puede arrancar antes de decidir la cuenta nueva")
	}
	b.esperar()
	if b.matrix.retries != 1 || !b.matrix.arranco[0] || b.enseña() != b.matrix.objeto {
		t.Fatal("la cuenta nueva, migrada, no fue a Matrix")
	}
	// Refrescar el token (misma cuenta) no vuelve a decidir.
	b.host.adoptar()
	if len(b.pedidos) != 2 {
		t.Fatalf("pedidos %d", len(b.pedidos))
	}
}

// Una respuesta que llega tarde, de la cuenta anterior, no monta nada.
func TestRegistrosRespuestaDeOtraCuentaSeDescarta(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = FuenteLegacy
	b.modos["persona-b"] = FuenteLegacy
	b.host.adoptar()
	b.owner = personaB
	b.host.adoptar()
	// Las dos vueltas, en el orden en que lleguen.
	b.esperar()
	b.esperar()
	if len(b.montajes) != 1 || b.montajes[0].PrivateStorageScope().Subject != "persona-b" {
		t.Fatalf("montajes %d", len(b.montajes))
	}
}

// Sin poder leer el modo no se elige nada: ni API ni Matrix, y se puede reintentar.
func TestRegistrosSinModoNoEligeNada(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.fallo = errors.New("sin red")
	b.host.adoptar()
	b.esperar()
	if len(b.montajes) != 0 || b.matrix.retries != 0 || b.host.matrixPuedeEmpezar() {
		t.Fatal("eligió destino sin saber el modo")
	}
	b.fallo = nil
	b.modos["persona-a"] = FuenteLegacy
	b.host.decidir()
	b.esperar()
	if len(b.montajes) != 1 {
		t.Fatal("reintentar no montó la API")
	}
}

// Fuera de la sesión no se pregunta nada: la de Matrix enseña «Iniciar sesión».
func TestRegistrosFueraNoPregunta(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.inside = false
	b.host.adoptar()
	if len(b.pedidos) != 0 || b.enseña() != b.matrix.objeto {
		t.Fatal("preguntó el modo sin sesión")
	}
}

type sesionFalsa struct {
	actual     auth.IdentitySnapshot
	tokens     int
	invalidada int
}

func (s *sesionFalsa) AccessToken(context.Context) (string, error) { s.tokens++; return "t", nil }
func (s *sesionFalsa) Invalidate(string)                           { s.invalidada++ }
func (s *sesionFalsa) SnapshotIdentity() auth.IdentitySnapshot     { return s.actual }

// La sesión de una vista no da el token de otra cuenta.
func TestSesionDeCuentaNoDaElTokenDeOtra(t *testing.T) {
	falsa := &sesionFalsa{actual: personaA}
	s := sesionDeCuenta{session: falsa, owner: personaA, capturar: func(ctx context.Context) (context.Context, auth.IdentitySnapshot) { return ctx, falsa.actual }}
	if _, err := s.AccessToken(context.Background()); err != nil || falsa.tokens != 1 {
		t.Fatalf("la dueña no obtiene su token: %v", err)
	}
	falsa.actual = personaB
	if _, err := s.AccessToken(context.Background()); !errors.Is(err, auth.ErrAuthorizationChanged) || falsa.tokens != 1 {
		t.Fatalf("dio el token de otra cuenta: %v", err)
	}
	s.Invalidate("x")
	if falsa.invalidada != 0 {
		t.Fatal("invalidó la sesión de otra cuenta")
	}
}

// Con la sesión real de la suite: la vista de alice no recibe el token de bob.
func TestSesionDeCuentaRealSigueALaDueña(t *testing.T) {
	session := auth.NewSession(factoryTokens("alice"), nil, nil)
	alice := session.SnapshotIdentity()
	cuenta := nuevaSesionDeCuenta(session, alice)
	if token, err := cuenta.AccessToken(t.Context()); err != nil || token != "synthetic-factory-access" {
		t.Fatalf("la dueña no obtiene su token: %q %v", token, err)
	}
	session.Adoptar(factoryTokens("bob"))
	if _, err := cuenta.AccessToken(t.Context()); !errors.Is(err, auth.ErrAuthorizationChanged) {
		t.Fatalf("dio el token de bob a la vista de alice: %v", err)
	}
}

// Salir y volver a entrar con la misma cuenta (A→fuera→A) no revive la vista vieja.
func TestRegistrosVolverAEntrarNoReviveLaVistaVieja(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = FuenteLegacy
	b.host.adoptar()
	b.esperar()
	vieja := b.montajes[0]
	b.inside = false
	b.host.adoptar()
	b.inside = true
	b.host.adoptar()
	b.esperar()
	if err := vieja.Check(context.Background()); err == nil {
		t.Fatal("la vista de antes de salir sigue valiendo")
	}
	if len(b.montajes) != 2 || b.montajes[1].Check(context.Background()) != nil {
		t.Fatal("la vista nueva no se montó")
	}
}

// «Soltar la fuente» desde el panel de Matrix: solo para la cuenta que va por
// Matrix, con una reidentificación si el servicio la pide y para la MISMA cuenta, y
// otro intento si la época cambió entre leerla y soltarla; después Matrix se suelta
// y la cuenta pasa a la API.
func TestRegistrosSoltarLaFuenteVuelveALaAPI(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = "frozen"
	var llamadas []storage.Scope
	respuestas := []error{api.ErrReautenticacion, api.ErrFuenteCambiada, nil}
	b.host.devolver = func(_ context.Context, _ api.Session, scope storage.Scope) error {
		llamadas = append(llamadas, scope)
		err := respuestas[0]
		respuestas = respuestas[1:]
		if err == nil {
			b.mu.Lock()
			b.modos["persona-a"] = FuenteLegacy
			b.mu.Unlock()
		}
		return err
	}
	reidentificaciones := 0
	b.host.reidentificar = func(context.Context) error { reidentificaciones++; return nil }
	b.host.adoptar()
	b.esperar()
	if b.enseña() != b.matrix.objeto {
		t.Fatal("una fuente congelada no fue a Matrix")
	}
	if err := b.host.soltarFuente(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := storage.Scope{Tenant: "acme", Subject: "persona-a", Product: "contacts"}
	if reidentificaciones != 1 || len(llamadas) != 3 || llamadas[2] != want {
		t.Fatalf("reidentificaciones %d, llamadas %v", reidentificaciones, llamadas)
	}
	b.esperar() // soltada
	b.esperar() // el modo nuevo
	if b.matrix.retiros != 1 || len(b.montajes) != 1 || b.enseña() != b.apiVista || b.host.matrixPuedeEmpezar() {
		t.Fatalf("retiros %d, montajes %d", b.matrix.retiros, len(b.montajes))
	}
	// Ya por la API: no hay nada que soltar desde Matrix.
	if err := b.host.soltarFuente(context.Background()); !errors.Is(err, auth.ErrAuthorizationChanged) || len(llamadas) != 3 {
		t.Fatal("soltó una cuenta que no va por Matrix", err)
	}
}

// Si al volver de Sytue ID la cuenta es otra, no se suelta nada de nadie.
func TestRegistrosSoltarLaFuenteNoCruzaDeCuenta(t *testing.T) {
	b := nuevoBancoRegistros(t)
	b.modos["persona-a"] = "frozen"
	llamadas := 0
	b.host.devolver = func(context.Context, api.Session, storage.Scope) error {
		llamadas++
		return api.ErrReautenticacion
	}
	b.host.reidentificar = func(context.Context) error { b.owner = personaB; return nil }
	b.host.adoptar()
	b.esperar()
	if err := b.host.soltarFuente(context.Background()); !errors.Is(err, auth.ErrAuthorizationChanged) || llamadas != 1 {
		t.Fatalf("err %v, llamadas %d", err, llamadas)
	}
	if b.matrix.retiros != 0 {
		t.Fatal("soltó Matrix sin soltar la fuente")
	}
}
