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

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

type autoridadFalsa func(context.Context, storage.Scope) (matrixproof.Authority, error)

func (f autoridadFalsa) MatrixStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	return f(ctx, scope)
}

var scopeCorreo = storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}

// La misma regla que `elegirAlmacenCorreo` de la web, caso por caso.
func TestElegirAlmacenCorreo(t *testing.T) {
	boveda := matrixproof.Authority{Version: 1, Scope: scopeCorreo, UserID: vaultproof.UserID(scopeCorreo), RootKey: "raiz", Generation: 6, Status: "active"}
	matrix := boveda
	matrix.UserID = storage.IsolatedMatrixUserPrefix(scopeCorreo) + "matrix.sytue.eu"
	responde := func(a matrixproof.Authority, err error) autoridadFalsa {
		return func(context.Context, storage.Scope) (matrixproof.Authority, error) { return a, err }
	}
	conflicto := func(body string) error { return &api.Error{Status: 409, Body: body} }
	casos := []struct {
		nombre string
		rooms  LectorAutoridad
		scope  storage.Scope
		quiero AlmacenCorreo
	}{
		{"autoridad de la bóveda", responde(boveda, nil), scopeCorreo, CorreoEnBoveda},
		{"cuenta nueva (409 en el sobre de httpx)", responde(matrixproof.Authority{}, conflicto(`{"error":{"code":"storage_root_enrollment_required"}}`)), scopeCorreo, CorreoEnBoveda},
		{"cuenta nueva (409 suelto)", responde(matrixproof.Authority{}, conflicto(`{"code":"storage_root_enrollment_required"}`)), scopeCorreo, CorreoEnBoveda},
		{"autoridad de Matrix confirmada", responde(matrix, nil), scopeCorreo, CorreoEnMatrix},
		{"Matrix retirada (409 en el sobre de httpx)", responde(matrixproof.Authority{}, conflicto(`{"error":{"code":"storage_matrix_retired"}}`)), scopeCorreo, CorreoRetirado},
		{"Matrix retirada (409 suelto)", responde(matrixproof.Authority{}, conflicto(`{"code":"storage_matrix_retired"}`)), scopeCorreo, CorreoRetirado},
		{"el código de la retirada sin 409: la duda", responde(matrixproof.Authority{}, &api.Error{Status: 503, Body: `{"code":"storage_matrix_retired"}`}), scopeCorreo, CorreoDudoso},
		{"otro 409: la duda", responde(matrixproof.Authority{}, conflicto(`{"code":"otra_cosa"}`)), scopeCorreo, CorreoDudoso},
		{"5xx: la duda", responde(matrixproof.Authority{}, &api.Error{Status: 503}), scopeCorreo, CorreoDudoso},
		{"sin red: la duda", responde(matrixproof.Authority{}, errors.New("sin red")), scopeCorreo, CorreoDudoso},
		{"sin organización: la duda", responde(boveda, nil), storage.Scope{Subject: "alice", Product: "mail"}, CorreoDudoso},
		{"sin Rooms: la duda", nil, scopeCorreo, CorreoDudoso},
		{"otro producto: la duda", responde(boveda, nil), storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}, CorreoDudoso},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := ElegirAlmacenCorreo(t.Context(), caso.rooms, caso.scope); got != caso.quiero {
				t.Fatalf("eligió %d, quería %d", got, caso.quiero)
			}
		})
	}
}

// `destinoCorreo` de la web: la bóveda gana siempre; después, un traspaso pedido;
// con Matrix retirada, solo pasar, y la duda sabiéndolo no abre Matrix.
func TestDestinoCorreo(t *testing.T) {
	casos := []struct {
		almacen                   AlmacenCorreo
		traspaso, sigue, retirada bool
		quiero                    destinoCorreo
	}{
		{CorreoEnBoveda, false, false, false, correoBoveda},
		{CorreoEnBoveda, true, true, false, correoBoveda},
		{CorreoEnBoveda, false, false, true, correoBoveda},
		{CorreoEnMatrix, false, false, false, correoElegir},
		{CorreoEnMatrix, false, true, false, correoMatrix},
		{CorreoEnMatrix, true, true, false, correoTraspaso},
		{CorreoDudoso, false, false, false, correoMatrix},
		{CorreoDudoso, true, false, false, correoTraspaso},
		// Matrix retirada (entrega 5).
		{CorreoRetirado, false, false, true, correoPasar},
		{CorreoRetirado, false, true, true, correoPasar},
		{CorreoRetirado, true, false, true, correoTraspaso},
		{CorreoDudoso, false, false, true, correoSinComprobar},
		{CorreoDudoso, false, true, true, correoSinComprobar},
		{CorreoDudoso, true, false, true, correoTraspaso},
	}
	for _, c := range casos {
		if got := elegirDestinoCorreo(c.almacen, c.traspaso, c.sigue, c.retirada); got != c.quiero {
			t.Errorf("%d/%v/%v/%v: %d, quería %d", c.almacen, c.traspaso, c.sigue, c.retirada, got, c.quiero)
		}
	}
}

// Los modos de la puerta abren como dicen: el traspaso vacía y lleva la marca; el
// traspaso ya vaciado abre lo que hay con la marca; el normal, lo de siempre.
func TestModosDeLaPuertaDelCorreo(t *testing.T) {
	fake, drive, account := pqvaulttest.New(t, scopeCorreo)
	cfg := pqvault.Config{Scope: scopeCorreo, Server: drive, Registry: account, UnlockKey: claveBoveda(3)}
	abrir, _ := puertaTraspaso.aperturas()
	nueva, err := abrir(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nueva.Close()
	if fake.Handovers != 1 || len(fake.Resets()) != 1 || !fake.Resets()[0] || nueva.SigningStatus() != pqvault.SigningReauth {
		t.Fatalf("el traspaso vacía y pide la marca: %v %d %s", fake.Resets(), fake.Handovers, nueva.SigningStatus())
	}
	fake.RecentLogin = true
	abrir, _ = puertaTraspasoVaciada.aperturas()
	misma, err := abrir(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer misma.Close()
	if misma.VaultID() != nueva.VaultID() || len(fake.Resets()) != 1 || fake.Handovers != 2 || misma.SigningStatus() != pqvault.SigningRegistered {
		t.Fatalf("el traspaso ya vaciado no vuelve a vaciar: %v %d %s", fake.Resets(), fake.Handovers, misma.SigningStatus())
	}
	abrir, _ = puertaNormal.aperturas()
	normal, err := abrir(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer normal.Close()
	if normal.VaultID() != nueva.VaultID() || fake.Handovers != 2 {
		t.Fatal("la apertura normal pidió traspaso")
	}
}

type puertaCorreoFalsa struct {
	modo     modoPuerta
	objeto   fyne.CanvasObject
	lista    func(context.Context, *pqvault.Vault)
	vida     context.Context
	vaciando bool
	cerrada  bool
}

func (p *puertaCorreoFalsa) Objeto() fyne.CanvasObject   { return p.objeto }
func (p *puertaCorreoFalsa) Iniciar(ctx context.Context) { p.vida = ctx }
func (p *puertaCorreoFalsa) IniciarVaciando(ctx context.Context) {
	p.vida, p.vaciando = ctx, true
}
func (p *puertaCorreoFalsa) Cerrar() { p.cerrada = true }

type recursosCorreoFalsos struct {
	life    context.Context
	cierres atomic.Int32
}

func (r *recursosCorreoFalsos) Context() context.Context           { return r.life }
func (r *recursosCorreoFalsos) PrivateStorageScope() storage.Scope { return scopeCorreo }
func (r *recursosCorreoFalsos) HTTP() api.Session                  { return nil }
func (r *recursosCorreoFalsos) Check(ctx context.Context) error    { return ctx.Err() }
func (r *recursosCorreoFalsos) Close()                             { r.cierres.Add(1) }

type repoCorreoFalso struct {
	inicializar error
	cierres     atomic.Int32
}

func (r *repoCorreoFalso) Initialize(context.Context) error        { return r.inicializar }
func (r *repoCorreoFalso) Close()                                  { r.cierres.Add(1) }
func (r *repoCorreoFalso) Application() *MailApplicationRepository { return nil }

type bancoCorreo struct {
	t          *testing.T
	host       *anfitrionCorreo
	matrix     *matrixFalsa
	ui         chan func()
	owner      auth.IdentitySnapshot
	elige      AlmacenCorreo
	fake       *pqvaulttest.Fake
	drive      *api.Drive
	account    *api.Account
	puertas    []*puertaCorreoFalsa
	entradas   int
	entrar     func() error
	abiertas   []*pqvault.Vault
	repos      []*repoCorreoFalso
	fallaAbrir error
	montajes   int
	vista      fyne.CanvasObject
	estados    []ApplicationState
	// Las pantallas de Mail, falsas: guardan lo que harían sus botones.
	pasar                *widget.Label
	confirmar, seguir    func()
	oferta               *widget.Label
	confirmarDesdeMatrix func()
	sinComprobar         *widget.Label
	reintentar           func()
}

func nuevoBancoCorreo(t *testing.T) *bancoCorreo {
	t.Helper()
	fynetest.NewTempApp(t)
	alice := auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "alice", Organization: "acme"}
	b := &bancoCorreo{t: t, ui: make(chan func(), 8), owner: alice, vista: widget.NewLabel("buzón")}
	b.fake, b.drive, b.account = pqvaulttest.New(t, scopeCorreo)
	b.matrix = &matrixFalsa{objeto: widget.NewLabel("matrix")}
	b.host = &anfitrionCorreo{
		translate: func(k string) string { return k },
		identity:  func() (auth.IdentitySnapshot, bool) { return b.owner, b.owner.Subject != "" },
		elegir:    func(context.Context, auth.IdentitySnapshot) AlmacenCorreo { return b.elige },
		puerta: func(modo modoPuerta, lista func(context.Context, *pqvault.Vault)) (puertaDrive, error) {
			p := &puertaCorreoFalsa{modo: modo, objeto: widget.NewLabel("puerta"), lista: lista}
			b.puertas = append(b.puertas, p)
			return p, nil
		},
		reidentificar: func(context.Context) error {
			b.entradas++
			if b.entrar != nil {
				return b.entrar()
			}
			return nil
		},
		intencion: func(ctx context.Context, owner auth.IdentitySnapshot) (context.Context, error) {
			if owner != b.owner {
				return nil, auth.ErrAuthorizationChanged
			}
			return ctx, nil
		},
		registrar: func(ctx context.Context, _ auth.IdentitySnapshot, vault *pqvault.Vault, handover bool) pqvault.SigningStatus {
			return vault.RegisterSigningKey(ctx, b.account, handover)
		},
		abrir: func(ctx context.Context, vault *pqvault.Vault) (recursosCorreo, repositorioCorreo, error) {
			b.abiertas = append(b.abiertas, vault)
			if b.fallaAbrir != nil {
				vault.Close()
				return nil, nil, b.fallaAbrir
			}
			repo := &repoCorreoFalso{}
			b.repos = append(b.repos, repo)
			return &recursosCorreoFalsos{life: context.WithoutCancel(ctx)}, repo, nil
		},
		montar: func(_ context.Context, c Cuenta, _ *MailApplicationRepository) (ApplicationView, error) {
			if c.PrivateStorageScope() != scopeCorreo {
				return ApplicationView{}, ErrPrivateResources
			}
			b.montajes++
			return ApplicationView{Object: b.vista}, nil
		},
		vistaPasar: func(confirmar, seguir func()) fyne.CanvasObject {
			b.pasar, b.confirmar, b.seguir = widget.NewLabel("pasar"), confirmar, seguir
			return b.pasar
		},
		vistaOferta: func(confirmar func()) fyne.CanvasObject {
			b.oferta, b.confirmarDesdeMatrix = widget.NewLabel("oferta"), confirmar
			return b.oferta
		},
		vistaSinComprobar: func(reintentar func()) fyne.CanvasObject {
			b.sinComprobar, b.reintentar = widget.NewLabel("sin comprobar"), reintentar
			return b.sinComprobar
		},
		retirada:  retiradaEnPreferencias(fyne.CurrentApp()),
		estado:    func(s ApplicationState, _ error) { b.estados = append(b.estados, s) },
		dispatch:  func(f func()) { b.ui <- f },
		slot:      container.NewStack(),
		traspasos: map[auth.IdentitySnapshot]*marcaTraspaso{},
		seguir:    map[auth.IdentitySnapshot]bool{},
	}
	b.host.matrix = b.matrix
	b.matrix.arrancar = b.host.matrixPuedeEmpezar
	t.Cleanup(func() {
		b.host.cerrar()
		for _, v := range b.abiertas {
			v.Close()
		}
	})
	return b
}

func (b *bancoCorreo) esperar() {
	b.t.Helper()
	select {
	case f := <-b.ui:
		f()
	case <-time.After(10 * time.Second):
		b.t.Fatal("no volvió al hilo de la interfaz")
	}
}

func (b *bancoCorreo) enseña() fyne.CanvasObject {
	b.t.Helper()
	if len(b.host.slot.Objects) != 1 {
		b.t.Fatalf("enseña %d objetos", len(b.host.slot.Objects))
	}
	return b.host.slot.Objects[0]
}

// entregar hace lo que haría la puerta de verdad con su modo: abrir (o vaciar) la
// bóveda del correo contra el servidor falso y entregarla.
func (b *bancoCorreo) entregar(p *puertaCorreoFalsa) *pqvault.Vault {
	b.t.Helper()
	abrir, _ := p.modo.aperturas()
	vault, err := abrir(b.t.Context(), pqvault.Config{Scope: scopeCorreo, Server: b.drive, Registry: b.account, UnlockKey: claveBoveda(7)})
	if err != nil {
		b.t.Fatal(err)
	}
	p.lista(p.vida, vault)
	return vault
}

func (b *bancoCorreo) ultimaPuerta() *puertaCorreoFalsa {
	b.t.Helper()
	if len(b.puertas) == 0 {
		b.t.Fatal("no hay puerta")
	}
	return b.puertas[len(b.puertas)-1]
}

// Una cuenta nueva (o con la autoridad de la bóveda) abre la bóveda y monta el
// correo sobre ella; Matrix no arranca.
func TestAnfitrionCorreoCuentaNuevaVaPorLaBoveda(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoEnBoveda
	b.host.adoptar()
	if b.host.matrixPuedeEmpezar() {
		t.Fatal("Matrix puede arrancar antes de decidir")
	}
	b.esperar()
	p := b.ultimaPuerta()
	if p.modo != puertaNormal || b.enseña() != p.objeto || p.vida == nil {
		t.Fatal("la cuenta de la bóveda no enseña la puerta normal iniciada")
	}
	b.entregar(p)
	b.esperar()
	if b.montajes != 1 || b.enseña() != b.vista || b.host.matrixPuedeEmpezar() || len(b.matrix.arranco) != 0 {
		t.Fatal("el correo de la bóveda no se montó, o Matrix arrancó")
	}
	if b.fake.Handovers != 0 || len(b.estados) != 1 || b.estados[0] != ApplicationReady {
		t.Fatalf("abrir la bóveda normal pidió traspaso o no avisó: %d %v", b.fake.Handovers, b.estados)
	}
	// Cambiar de cuenta lo suelta todo.
	b.owner = auth.IdentitySnapshot{}
	b.host.adoptar()
	if !p.cerrada || !esperarCierre(&b.repos[0].cierres) {
		t.Fatal("cambiar de cuenta no soltó el correo de la bóveda")
	}
}

func esperarCierre(n *atomic.Int32) bool {
	for range 200 {
		if n.Load() > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Ante la duda, Matrix sin proponer vaciar; la salida «¿No puedes abrir tu
// correo?» está debajo mientras Matrix no abre, y desaparece al abrir.
func TestAnfitrionCorreoAnteLaDudaVaPorMatrix(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoDudoso
	b.host.adoptar()
	b.esperar()
	if len(b.puertas) != 0 || len(b.matrix.arranco) != 1 || !b.matrix.arranco[0] {
		t.Fatal("ante la duda no va por Matrix")
	}
	borde, ok := b.enseña().(*fyne.Container)
	if !ok || b.oferta == nil || !contiene(borde, b.matrix.objeto) || !contiene(borde, b.oferta) {
		t.Fatal("Matrix sin la salida del traspaso mientras no abre")
	}
	b.host.estadoMatrix(ApplicationReady)
	if b.enseña() != b.matrix.objeto {
		t.Fatal("con el correo de Matrix abierto se sigue ofreciendo vaciar")
	}
}

func contiene(c *fyne.Container, o fyne.CanvasObject) bool {
	for _, x := range c.Objects {
		if x == o {
			return true
		}
	}
	return false
}

// Con autoridad de Matrix confirmada se ofrece pasar a la bóveda; «seguir por
// ahora» va por Matrix y no se vuelve a proponer en esta ejecución.
func TestAnfitrionCorreoMatrixConfirmadaOfreceElegirYSeguir(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoEnMatrix
	b.host.adoptar()
	b.esperar()
	if !b.host.va(correoElegir) || len(b.puertas) != 0 || len(b.matrix.arranco) != 0 {
		t.Fatal("con autoridad de Matrix no se ofrece elegir antes de abrir nada")
	}
	if b.pasar == nil || b.enseña() != b.pasar {
		t.Fatal("no enseña la pantalla de pasar a la bóveda")
	}
	b.seguir()
	if !b.host.va(correoMatrix) || len(b.matrix.arranco) != 1 || !b.matrix.arranco[0] {
		t.Fatal("seguir por ahora no va por Matrix")
	}
	b.host.decidir()
	b.esperar()
	if !b.host.va(correoMatrix) || len(b.puertas) != 0 {
		t.Fatal("seguir por ahora se olvidó en la misma ejecución")
	}
}

// Vaciar y pasar: una bóveda NUEVA con la marca de traspaso. Si Sytue ID pide
// volver a entrar, se entra y se registra la MISMA bóveda, sin vaciar otra vez.
func TestAnfitrionCorreoTraspasoConReidentificacionNoVaciaDosVeces(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoEnMatrix
	b.host.adoptar()
	b.esperar()
	if b.pasar == nil || b.enseña() != b.pasar {
		t.Fatal("no enseña la pantalla de pasar a la bóveda")
	}
	b.confirmar()
	p := b.ultimaPuerta()
	if p.modo != puertaTraspaso || b.host.matrixPuedeEmpezar() {
		t.Fatal("el traspaso no abre una bóveda nueva con la marca")
	}
	vault := b.entregar(p)
	if vault.SigningStatus() != pqvault.SigningReauth || b.host.pendiente != vault {
		t.Fatalf("sin identificación reciente la bóveda espera: %s", vault.SigningStatus())
	}
	resets := len(b.fake.Resets())

	// La persona cancela el inicio de sesión: se sigue pidiendo, con la misma bóveda.
	b.entrar = func() error { return errors.New("cancelado") }
	b.host.registrarFirma(true)
	b.esperar()
	if b.host.pendiente != vault || vault.Closed() || len(b.abiertas) != 0 {
		t.Fatal("cancelar el inicio de sesión soltó la bóveda o abrió el correo")
	}

	// Vuelve a entrar: Sytue ID ya la ve reciente y el traspaso queda registrado.
	b.entrar = func() error { b.fake.RecentLogin = true; return nil }
	b.host.registrarFirma(true)
	b.esperar() // registrada
	b.esperar() // inicializado
	if b.entradas != 2 || len(b.abiertas) != 1 || b.abiertas[0] != vault || b.montajes != 1 || b.enseña() != b.vista {
		t.Fatal("al volver no se abrió el correo sobre la misma bóveda")
	}
	if len(b.fake.Resets()) != resets || len(b.fake.Signing) != 1 || b.fake.Signing[0].Reason != pqvault.ReasonHandover {
		t.Fatalf("se vació otra vez o no quedó el traspaso: %v %+v", b.fake.Resets(), b.fake.Signing)
	}
	if b.host.traspasos[b.owner] != nil {
		t.Fatal("con el correo abierto en la bóveda, la marca del traspaso sigue")
	}
}

// Si el correo no llega a abrir tras vaciar (Rooms aún no ve el traspaso), el
// siguiente intento abre esa misma bóveda con la marca: no vacía otra vez.
func TestAnfitrionCorreoTraspasoTrasUnFalloNoVaciaOtraVez(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.fake.RecentLogin = true
	b.elige = CorreoEnMatrix
	b.host.adoptar()
	b.esperar()
	b.host.pedirTraspaso()
	b.fallaAbrir = errors.New("la autoridad aún es de Matrix")
	first := b.entregar(b.ultimaPuerta())
	b.esperar()
	if b.montajes != 0 || len(b.estados) != 1 || b.estados[0] != ApplicationFailed {
		t.Fatal("el fallo al abrir no se enseñó")
	}
	resets := len(b.fake.Resets())

	b.fallaAbrir = nil
	b.host.reintentar()
	b.esperar() // decidido: la autoridad sigue siendo de Matrix, con el traspaso pedido
	p := b.ultimaPuerta()
	if p.modo != puertaTraspasoVaciada {
		t.Fatalf("el reintento vuelve a vaciar: modo %d", p.modo)
	}
	second := b.entregar(p)
	b.esperar()
	if second.VaultID() != first.VaultID() || len(b.fake.Resets()) != resets || b.montajes != 1 {
		t.Fatalf("el reintento no abrió la misma bóveda: %v", b.fake.Resets())
	}

	// Y en cuanto la autoridad es de la bóveda, la puerta es la normal.
	b.owner = auth.IdentitySnapshot{}
	b.host.adoptar()
	b.owner = auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "alice", Organization: "acme"}
	b.elige = CorreoEnBoveda
	b.host.adoptar()
	b.esperar()
	if b.ultimaPuerta().modo != puertaNormal {
		t.Fatal("con la autoridad de la bóveda no se abre la puerta normal")
	}
}

// La salida debajo de Matrix suelta Matrix y abre la bóveda nueva del traspaso.
func TestAnfitrionCorreoSalidaDesdeMatrix(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoDudoso
	b.host.adoptar()
	b.esperar()
	if b.confirmarDesdeMatrix == nil {
		t.Fatal("sin salida debajo de Matrix")
	}
	b.confirmarDesdeMatrix()
	if b.matrix.retiros != 1 || len(b.puertas) != 1 || b.ultimaPuerta().modo != puertaTraspaso || b.host.matrixPuedeEmpezar() {
		t.Fatal("la salida no soltó Matrix ni abrió la bóveda del traspaso")
	}
}

// Cambiar de cuenta con la bóveda esperando a volver a entrar la cierra.
func TestAnfitrionCorreoCambiarDeCuentaCierraLaBovedaPendiente(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoEnMatrix
	b.host.adoptar()
	b.esperar()
	b.host.pedirTraspaso()
	vault := b.entregar(b.ultimaPuerta())
	if b.host.pendiente != vault {
		t.Fatal("la bóveda no espera")
	}
	b.owner = auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "bob", Organization: "acme"}
	b.host.adoptar()
	if !vault.Closed() || b.host.pendiente != nil {
		t.Fatal("la bóveda de la cuenta anterior quedó abierta")
	}
	if b.host.traspasos[auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "suite", Subject: "alice", Organization: "acme"}] == nil {
		t.Fatal("el traspaso pedido por la cuenta anterior se perdió en la ejecución")
	}
}

// Con Matrix retirada en Rooms (entrega 5) solo se ofrece pasar a la bóveda: la
// pantalla sin «seguir», sin abrir Matrix; confirmar abre la bóveda del traspaso.
func TestAnfitrionCorreoMatrixRetiradaSoloOfrecePasar(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoRetirado
	b.host.adoptar()
	b.esperar()
	if !b.host.va(correoPasar) || b.pasar == nil || b.enseña() != b.pasar || b.seguir != nil {
		t.Fatal("con Matrix retirada no se enseña solo «pasar a la bóveda»")
	}
	if len(b.puertas) != 0 || len(b.matrix.arranco) != 0 || b.host.matrixPuedeEmpezar() || b.oferta != nil {
		t.Fatal("con Matrix retirada se abrió algo antes de confirmar")
	}
	// «Seguir con Matrix» no tiene efecto aquí aunque alguien lo llame.
	b.host.seguirConMatrix()
	if !b.host.va(correoPasar) || len(b.matrix.arranco) != 0 {
		t.Fatal("con Matrix retirada se pudo seguir con Matrix")
	}
	b.confirmar()
	if p := b.ultimaPuerta(); p.modo != puertaTraspaso || b.host.matrixPuedeEmpezar() {
		t.Fatal("confirmar con Matrix retirada no abre la bóveda nueva del traspaso")
	}
}

// Desde que Rooms dijo que Matrix está retirada, la duda no abre Matrix sino
// «Reintentar»; se recuerda en las preferencias de la app y una respuesta de
// Matrix (el conmutador de Rooms apagado) lo olvida.
func TestAnfitrionCorreoDudaConMatrixRetiradaNoAbreMatrix(t *testing.T) {
	b := nuevoBancoCorreo(t)
	b.elige = CorreoRetirado
	b.host.adoptar()
	b.esperar()
	if !b.host.va(correoPasar) {
		t.Fatal("no se aprendió la retirada")
	}

	// La siguiente lectura no sabe (red, 5xx): ni Matrix ni la salida de vaciar.
	b.elige = CorreoDudoso
	b.host.reintentar()
	b.esperar()
	if !b.host.va(correoSinComprobar) || b.sinComprobar == nil || b.enseña() != b.sinComprobar {
		t.Fatal("la duda con Matrix retirada no enseña «Reintentar»")
	}
	if len(b.matrix.arranco) != 0 || b.host.matrixPuedeEmpezar() || b.oferta != nil || len(b.puertas) != 0 {
		t.Fatal("la duda con Matrix retirada abrió Matrix o la bóveda")
	}

	// Lo recuerda otra ejecución (las preferencias de la app).
	otro := &anfitrionCorreo{retirada: retiradaEnPreferencias(fyne.CurrentApp())}
	if !otro.retirada.sabe(b.owner) || otro.retirada.sabe(auth.IdentitySnapshot{Subject: "bob"}) {
		t.Fatal("la retirada no quedó en las preferencias de esta cuenta")
	}

	// «Reintentar» vuelve a preguntar.
	b.elige = CorreoRetirado
	b.reintentar()
	b.esperar()
	if !b.host.va(correoPasar) || b.enseña() != b.pasar {
		t.Fatal("«Reintentar» no volvió a preguntar a Rooms")
	}

	// Una respuesta de Matrix olvida la retirada: la duda vuelve a ir por Matrix.
	b.elige = CorreoEnMatrix
	b.host.reintentar()
	b.esperar()
	if !b.host.va(correoElegir) || b.seguir == nil {
		t.Fatal("con Matrix confirmada no se ofrece elegir")
	}
	b.elige = CorreoDudoso
	b.host.decidir()
	b.esperar()
	if !b.host.va(correoMatrix) || len(b.matrix.arranco) != 1 || !b.matrix.arranco[0] {
		t.Fatal("olvidada la retirada, la duda no va por Matrix como antes")
	}
}
