// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

type puertaFalsa struct {
	objeto   fyne.CanvasObject
	lista    func(context.Context, *pqvault.Vault)
	vida     context.Context
	vaciando bool
	cerrada  bool
}

func (p *puertaFalsa) Objeto() fyne.CanvasObject   { return p.objeto }
func (p *puertaFalsa) Iniciar(ctx context.Context) { p.vida = ctx }
func (p *puertaFalsa) IniciarVaciando(ctx context.Context) {
	p.vida, p.vaciando = ctx, true
}
func (p *puertaFalsa) Cerrar() { p.cerrada = true }

type repoBovedaFalso struct {
	inicializar error
	cierres     atomic.Int32
}

func (r *repoBovedaFalso) Initialize(context.Context) error          { return r.inicializar }
func (r *repoBovedaFalso) Close()                                    { r.cierres.Add(1) }
func (r *repoBovedaFalso) DriveRepository() *privateDrive.Repository { return nil }

type bancoAnfitrionDrive struct {
	t        *testing.T
	banco    *bancoDriveBoveda
	host     *anfitrionDrive
	matrix   *matrixFalsa
	ui       chan func()
	owner    auth.IdentitySnapshot
	elige    map[string]AlmacenDrive
	fallo    error
	puertas  []*puertaFalsa
	repos    []*repoBovedaFalso
	recursos []*RecursosBoveda
	montajes int
	detaches int
	vista    fyne.CanvasObject
	estados  []ApplicationState
}

func nuevoBancoAnfitrionDrive(t *testing.T) *bancoAnfitrionDrive {
	t.Helper()
	fynetest.NewTempApp(t)
	alice := auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "alice", Organization: "acme"}
	b := &bancoAnfitrionDrive{t: t, banco: nuevoBancoDriveBoveda(t), ui: make(chan func(), 8), owner: alice, elige: map[string]AlmacenDrive{}, vista: widget.NewLabel("drive")}
	b.matrix = &matrixFalsa{objeto: widget.NewLabel("matrix")}
	b.host = &anfitrionDrive{
		translate: func(k string) string { return k },
		identity:  func() (auth.IdentitySnapshot, bool) { return b.owner, b.owner.Subject != "" },
		elegir: func(_ context.Context, owner auth.IdentitySnapshot) (AlmacenDrive, error) {
			return b.elige[owner.Subject], b.fallo
		},
		puerta: func(lista func(context.Context, *pqvault.Vault)) (puertaDrive, error) {
			p := &puertaFalsa{objeto: widget.NewLabel("puerta"), lista: lista}
			b.puertas = append(b.puertas, p)
			return p, nil
		},
		abrir: func(ctx context.Context, vault *pqvault.Vault) (*RecursosBoveda, repositorioBoveda, error) {
			recursos, err := b.banco.fabrica.Abrir(ctx, vault)
			if err != nil {
				return nil, nil, err
			}
			repo := &repoBovedaFalso{}
			b.repos = append(b.repos, repo)
			b.recursos = append(b.recursos, recursos)
			return recursos, repo, nil
		},
		montar: func(_ context.Context, c Cuenta, _ *privateDrive.Repository) (ApplicationView, error) {
			if c.PrivateStorageScope() != scopeDriveBoveda {
				return ApplicationView{}, ErrPrivateResources
			}
			b.montajes++
			return ApplicationView{Object: b.vista, Detach: func() { b.detaches++ }}, nil
		},
		estado:   func(s ApplicationState, _ error) { b.estados = append(b.estados, s) },
		dispatch: func(f func()) { b.ui <- f },
		slot:     container.NewStack(),
	}
	b.host.matrix = b.matrix
	b.matrix.arrancar = b.host.matrixPuedeEmpezar
	t.Cleanup(func() {
		b.host.cerrar()
		for _, r := range b.recursos {
			r.Close()
		}
	})
	return b
}

func (b *bancoAnfitrionDrive) esperar() {
	b.t.Helper()
	select {
	case f := <-b.ui:
		f()
	case <-time.After(10 * time.Second):
		b.t.Fatal("no volvió al hilo de la interfaz")
	}
}

func (b *bancoAnfitrionDrive) enseña() fyne.CanvasObject {
	if len(b.host.slot.Objects) != 1 {
		b.t.Fatalf("enseña %d objetos", len(b.host.slot.Objects))
	}
	return b.host.slot.Objects[0]
}

// Una cuenta cuya bóveda ya tiene datos va por la bóveda: puerta, repositorio
// sobre la bóveda y vista; Matrix no arranca. Cambiar de cuenta lo suelta todo
// (la bóveda se cierra) y la cuenta nueva, sin bóveda, va por Matrix.
func TestAnfitrionDriveVaPorLaBovedaYSueltaAlCambiarDeCuenta(t *testing.T) {
	b := nuevoBancoAnfitrionDrive(t)
	b.elige["alice"] = DriveEnBoveda
	b.elige["bob"] = DriveEnMatrix
	b.host.adoptar()
	if b.host.matrixPuedeEmpezar() {
		t.Fatal("Matrix puede arrancar antes de decidir")
	}
	b.esperar()
	if len(b.puertas) != 1 || b.enseña() != b.puertas[0].objeto || b.puertas[0].vida == nil {
		t.Fatal("la cuenta de la bóveda no enseña la puerta iniciada")
	}
	if b.matrix.retries != 0 || b.host.matrixPuedeEmpezar() {
		t.Fatal("Matrix arrancó para una cuenta de la bóveda")
	}
	// La puerta abre con la clave de Sytue ID y entrega la bóveda.
	vault := b.banco.abrirBoveda(t)
	b.puertas[0].lista(b.banco.ctx, vault)
	b.esperar()
	if b.montajes != 1 || b.enseña() != b.vista || len(b.estados) != 1 || b.estados[0] != ApplicationReady {
		t.Fatal("la vista sobre la bóveda no se montó", b.montajes, b.estados)
	}
	// Una segunda entrega (puerta vieja o repetida) no monta otra vez y no deja
	// la bóveda abierta.
	otra := b.banco.abrirBoveda(t)
	b.puertas[0].lista(b.banco.ctx, otra)
	if !otra.Closed() || b.montajes != 1 {
		t.Fatal("una entrega repetida se montó o dejó la bóveda abierta")
	}

	b.owner = auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "bob", Organization: "acme"}
	b.host.adoptar()
	if !b.puertas[0].cerrada || b.detaches != 1 {
		t.Fatal("la puerta o la vista de la cuenta anterior siguen")
	}
	deadline := time.Now().Add(5 * time.Second)
	for (b.repos[0].cierres.Load() == 0 || !vault.Closed()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if b.repos[0].cierres.Load() != 1 || !vault.Closed() {
		t.Fatal("el repositorio o la bóveda de la cuenta anterior siguen abiertos")
	}
	b.esperar()
	if b.enseña() != b.matrix.objeto || b.matrix.retries != 1 || !b.matrix.arranco[0] {
		t.Fatal("la cuenta sin bóveda no va por Matrix", b.matrix.arranco)
	}
	if len(b.puertas) != 1 {
		t.Fatal("se abrió una puerta para una cuenta de Matrix")
	}
}

// Sin poder decidir no se abre ni Matrix ni la bóveda; reintentar vuelve a decidir.
func TestAnfitrionDriveSinDecidirNoAbreNada(t *testing.T) {
	b := nuevoBancoAnfitrionDrive(t)
	b.fallo = errors.New("sin red")
	b.host.adoptar()
	b.esperar()
	if b.host.matrixPuedeEmpezar() || b.matrix.retries != 0 || len(b.puertas) != 0 {
		t.Fatal("se abrió algo sin saber dónde viven los datos")
	}
	if len(b.estados) != 1 || b.estados[0] != ApplicationFailed {
		t.Fatal("el fallo no se avisó", b.estados)
	}
	b.fallo = nil
	b.elige["alice"] = DriveEnMatrix
	b.host.reintentar()
	b.esperar()
	if !b.host.matrixPuedeEmpezar() || b.matrix.retries != 1 {
		t.Fatal("reintentar no volvió a decidir")
	}
}

// Si el repositorio de la bóveda no inicializa (p. ej. sin recibo de migración en
// la bóveda), no se monta nada y la bóveda se cierra.
func TestAnfitrionDriveFalloAlInicializarCierraLaBoveda(t *testing.T) {
	b := nuevoBancoAnfitrionDrive(t)
	b.elige["alice"] = DriveEnBoveda
	abrir := b.host.abrir
	b.host.abrir = func(ctx context.Context, vault *pqvault.Vault) (*RecursosBoveda, repositorioBoveda, error) {
		recursos, repo, err := abrir(ctx, vault)
		if repo != nil {
			repo.(*repoBovedaFalso).inicializar = privateDrive.ErrNotReady
		}
		return recursos, repo, err
	}
	b.host.adoptar()
	b.esperar()
	vault := b.banco.abrirBoveda(t)
	b.puertas[0].lista(b.banco.ctx, vault)
	b.esperar()
	if b.montajes != 0 || b.host.matrixPuedeEmpezar() {
		t.Fatal("se montó una bóveda sin inicializar, o se pasó a Matrix")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !vault.Closed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !vault.Closed() || b.repos[0].cierres.Load() != 1 {
		t.Fatal("la bóveda o el repositorio quedaron abiertos tras el fallo")
	}
}

// Una bóveda que abre pero cuyo Drive no carga (recién vaciada, con la fuente ya
// `private`) no se queda en «reintentar»: deja vaciar, con casilla, y vacía con una
// puerta nueva (que suelta antes la fuente, AntesDeVaciar).
func TestAnfitrionDriveFalloAlInicializarDejaVaciarConConfirmacion(t *testing.T) {
	b := nuevoBancoAnfitrionDrive(t)
	b.elige["alice"] = DriveEnBoveda
	abrir := b.host.abrir
	b.host.abrir = func(ctx context.Context, vault *pqvault.Vault) (*RecursosBoveda, repositorioBoveda, error) {
		recursos, repo, err := abrir(ctx, vault)
		if repo != nil {
			repo.(*repoBovedaFalso).inicializar = privateDrive.ErrSource
		}
		return recursos, repo, err
	}
	b.host.adoptar()
	b.esperar()
	b.puertas[0].lista(b.banco.ctx, b.banco.abrirBoveda(t))
	b.esperar()
	vacio, ok := b.enseña().(*ui.Vacio)
	if !ok || vacio.AccionSecundaria == nil {
		t.Fatal("el fallo de carga no ofrece vaciar")
	}
	vaciar := vacio.AccionSecundaria.(*ui.Boton)
	if vaciar.Texto != "vault.empty" {
		t.Fatal("la salida no es vaciar:", vaciar.Texto)
	}
	vaciar.AlPulsar()
	confirmacion, ok := b.enseña().(*fyne.Container)
	if !ok {
		t.Fatal("vaciar no pidió confirmación")
	}
	var casilla *ui.Eleccion
	var peligro *ui.Boton
	for _, o := range confirmacion.Objects {
		switch v := o.(type) {
		case *ui.Eleccion:
			casilla = v
		case *fyne.Container:
			for _, b := range v.Objects {
				if boton, ok := b.(*ui.Boton); ok && boton.Variante == ui.Peligro {
					peligro = boton
				}
			}
		}
	}
	if casilla == nil || peligro == nil || !peligro.Inerte {
		t.Fatal("la confirmación no tiene casilla y botón de peligro inerte")
	}
	// Sin la casilla no se vacía nada.
	peligro.Tapped(nil)
	if len(b.puertas) != 1 {
		t.Fatal("vació sin la casilla")
	}
	casilla.Marcada = true
	casilla.AlCambiar(true)
	if peligro.Inerte {
		t.Fatal("la casilla no habilita vaciar")
	}
	peligro.Tapped(nil)
	if len(b.puertas) != 2 || !b.puertas[1].vaciando || b.puertas[1].vida == nil {
		t.Fatal("vaciar no abrió una puerta nueva vaciando")
	}
	if !b.puertas[0].cerrada || b.host.matrixPuedeEmpezar() {
		t.Fatal("la puerta anterior sigue o se pasó a Matrix")
	}
}

// La puerta (ADR 0086) es la de Sytue ID: su servidor está atado a la cuenta.
func TestSesionBovedaDeCuentaNoLlevaElTokenDeOtraCuenta(t *testing.T) {
	session := auth.NewSession(factoryTokens("alice"), nil, nil)
	alice := session.SnapshotIdentity()
	if _, err := (sesionBovedaDeCuenta{session: session, owner: alice}).AccessToken(t.Context()); err != nil {
		t.Fatal(err)
	}
	bob := alice
	bob.Subject = "bob"
	if _, err := (sesionBovedaDeCuenta{session: session, owner: bob}).AccessToken(t.Context()); !errors.Is(err, auth.ErrAuthorizationChanged) {
		t.Fatal("la sesión de alice dio su token a la bóveda de bob", err)
	}
}
