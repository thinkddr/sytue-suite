// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cifrado owns the shared native device verification and recovery panel.
package cifrado

// Reuses the maintained Matrix SDK and the existing suite dialog components.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// Panel owns the native device lifecycle. All UI state stays on Fyne's thread;
// network/crypto operations run outside it. Access tokens and recovery secrets
// never enter application metadata services or persistent UI preferences.
type Panel struct {
	nativeRecovery bool
	Lienzo         func() fyne.Canvas
	Traducir       func(string) string
	Copiar         func(string)
	Avisar         func(string)
	Bootstrap      func(context.Context) (mx.Bootstrap, error)
	AlContexto     func(context.Context)
	// AlCerrar detaches application views on the UI thread and returns optional
	// background cleanup. SDK key teardown and the next open wait for that cleanup.
	AlCerrar func() func()
	Personas func() []Persona
	// Custodio guarda la clave de recuperación envuelta con la clave de desbloqueo de
	// la persona (ADR 0084). La cuenta la elige el bootstrap de este panel.
	Custodio CustodioClave
	// Desbloqueo da la clave de desbloqueo que trajo el inicio de sesión en Sytue ID
	// (con la sesión, compartida entre las apps nativas) y lo que abre los envoltorios
	// de la ADR 0082; vacía si este dispositivo no la tiene. ConSesion la conecta.
	Desbloqueo func() Material
	// OlvidarLegado retira de la sesión lo que abría los envoltorios de la ADR 0082 una
	// vez pasados al tipo nuevo.
	OlvidarLegado func(context.Context)
	// IniciarSesion vuelve a entrar en Sytue ID pidiendo la clave (prompt=login), y con
	// `legado` los envoltorios de la ADR 0082 que abrir. Es la ÚNICA forma de
	// desbloquear que ofrece este panel: aquí no se teclea nada.
	IniciarSesion func(ctx context.Context, legado string) error
	// SoltarFuente es «Vaciar y volver a los datos del servidor» (ADR 0086): la
	// salida de una app de registros que tiene sus datos en Matrix, abra o no. Devuelve la
	// fuente del servidor a como estaba al congelarse; lo escrito después en
	// Matrix se pierde. Nil: sin esa salida (Rooms, Drive, Mail).
	SoltarFuente func(context.Context) error
	// SoltarSinIdentificar: la salida no pasa por Sytue ID (Drive vuelve a la
	// copia del servidor dentro de su bóveda, sin tocar el servidor), así que el
	// paso no avisa de que habrá que volver a iniciar sesión.
	SoltarSinIdentificar bool
	Licencias            string
	Base                 mx.Config
	Login                func(context.Context, mx.Config) (*mx.Client, error)
	Abrir                func(string) error
	AlLista              func(context.Context)
	AlEvento             func(mx.DecryptedEvent, error)
	mu                   sync.RWMutex
	client               *mx.Client
	closing              <-chan struct{}
	ctx                  context.Context
	cancel               context.CancelFunc
	generation           uint64
	body, footer         *fyne.Container
	dialog               *ui.Dialogo
	busy                 bool
	status               mx.VerificationStatus
	txn                  id.VerificationTransactionID
	verifyUser           string
	diag                 Diagnostico
	comparison           string
	verificationWaiting  bool
	authWait             chan bool
	workers              atomic.Pointer[PanelWorkers]
	localWork            *offline.WorkGate
	clients              map[*mx.Client]bool

	// entroSolo: el panel ya fue solo a Sytue ID a por la clave de desbloqueo (ver
	// irSoloAEntrar). Una vez por inicio: si a la vuelta sigue sin ella, queda el botón.
	entroSolo bool
}

func (c *Panel) Cliente(ctx context.Context) (*mx.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.client == nil {
		return nil, mx.ErrLoginRequired
	}
	return c.client, nil
}

func (c *Panel) Iniciar(ctx context.Context, subject, tenant string) {
	c.Cerrar() //nolint:contextcheck // Old-device shutdown is detached before the new lifecycle begins.
	ctx, c.cancel = context.WithCancel(ctx)
	c.ctx = ctx
	c.mu.Lock()
	if c.workers.Load() == nil {
		c.localWork = offline.NewWorkGate(ctx)
	}
	c.mu.Unlock()
	if c.AlContexto != nil {
		c.AlContexto(ctx)
	}
	c.Base.Subject, c.Base.Tenant = subject, tenant
	c.Base.VerificationCallbacks = c
	c.entroSolo = false
	c.generation++
	generation := c.generation
	c.busy = true
	c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	cfg := c.Base
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		if err := c.esperarCierre(job); err != nil {
			return
		}
		bootstrap, err := c.Bootstrap(job)
		cfg.Bootstrap = bootstrap
		var client *mx.Client
		if err == nil {
			client, err = mx.Open(job, cfg)
		}
		c.remember(client, worker)
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				if client != nil {
					go c.releaseClient(client, worker)
				}
				return
			}
			c.Base = cfg
			c.busy = false
			if err != nil {
				if !errors.Is(err, mx.ErrLoginRequired) {
					c.fallo()
				}
				c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
				return
			}
			c.instalar(client) //nolint:contextcheck // The UI installs future actions bound to the retained device lifetime.
		})
	})
}

func (c *Panel) instalar(client *mx.Client) {
	c.mu.Lock()
	c.client = client
	c.mu.Unlock()
	ctx, generation := c.ctx, c.generation
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	c.runWork(ctx, generation, true, func(job context.Context, worker *PanelWorkers) {
		_ = client.Sync(job, func(value mx.DecryptedEvent, err error) {
			fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
				if generation == c.generation && c.workCurrent(ctx, worker) && c.AlEvento != nil {
					c.AlEvento(value, err)
				}
			})
		})
	})
	c.revisar(ctx, generation, client, true)
}

// custodia es la cuenta de Matrix de este panel y dónde van sus envoltorios: la
// de Rooms, o la cuenta (heredada o aislada) del producto de almacenamiento.
func (c *Panel) custodia() Custodia {
	if c.Custodio == nil {
		return Custodia{}
	}
	b := c.Base.Bootstrap
	var account api.MatrixRecoveryAccount
	if b.StorageProduct != "" {
		account = api.MatrixRecoveryAccount{Product: b.StorageProduct, IdentityVersion: b.StorageIdentityVersion}
	}
	return Custodia{Servicio: c.Custodio, Cuenta: account}
}

// resolver hace lo que pide el diagnóstico con la clave de desbloqueo (Resolver) y
// lo que eso arrastra en este panel: la recuperación nativa de Drive tras crear y
// retirar lo que abría los envoltorios de la ADR 0082 tras pasarlos al tipo nuevo.
func (c *Panel) resolver(job context.Context, client *mx.Client, custodia Custodia, diag Diagnostico, native bool) (Diagnostico, error) {
	creating := diag.Paso == PasoCrear
	m := c.material()
	defer m.Borrar()
	diag, err := Resolver(job, client, custodia, diag, m, c.uia(job, client))
	if err != nil {
		return diag, err
	}
	// ADR 0085: con el `sytue-id` comprobado aquí, los envoltorios de la 0082 sobran.
	if diag.Paso == PasoListo && custodia.Servicio != nil {
		if _, err := RetirarViejos(job, client, custodia, m); err != nil {
			slog.Warn("cifrado: no se pudieron retirar los envoltorios de la ADR 0082", "error", err)
		}
	}
	if creating && native && diag.Paso != PasoIniciarSesion {
		if err := client.SyncNativeRecovery(job); err != nil {
			return diag, err
		}
	}
	if diag.migrado && c.OlvidarLegado != nil {
		c.OlvidarLegado(job)
	}
	diag.Status, err = c.recoveryStatus(job, client)
	return diag, err
}

// material es la clave de desbloqueo de la sesión, si este dispositivo la tiene.
func (c *Panel) material() Material {
	if c.Desbloqueo == nil {
		return Material{}
	}
	return c.Desbloqueo()
}

// revisar decide el paso de este dispositivo y lo hace sin pedir nada (ADR 0084):
// recuperarlo con la clave de desbloqueo de Sytue ID (o con la copia de la ADR 0081),
// crear el cifrado, o, si falta la clave, ofrecer volver a entrar en Sytue ID.
func (c *Panel) revisar(ctx context.Context, generation uint64, client *mx.Client, wait bool) {
	custodia := c.custodia()
	native := c.nativeRecovery
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		var err error
		if wait {
			err = client.WaitReady(job)
		}
		var diag Diagnostico
		if err == nil {
			diag.Status, err = c.recoveryStatus(job, client)
		}
		if err == nil && (!diag.Status.Verified || diag.Status.RecoverySetupPending || custodia.Servicio != nil) {
			diag, err = Diagnosticar(job, client, custodia)
		}
		if err == nil && custodia.Servicio != nil {
			diag, err = c.resolver(job, client, custodia, diag, native)
		}
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				return
			}
			c.busy = false
			c.status, c.diag = diag.Status, diag
			if err != nil {
				c.fallo()
			}
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.lista()
			c.irSoloAEntrar() //nolint:contextcheck // Como iniciarSesion: vive con el ciclo del dispositivo (c.ctx).
		})
	})
}

// irSoloAEntrar: sin la clave de desbloqueo en este dispositivo, el panel va solo a
// Sytue ID a por ella, sin esperar a que la persona pulse «Iniciar sesión», como la
// puerta de la bóveda (PuertaBoveda) y la web (PqVaultGate): con iniciar sesión
// tiene que bastar. Una vez por inicio; si a la vuelta sigue sin ella, queda el botón.
func (c *Panel) irSoloAEntrar() {
	if c.entroSolo || c.busy || c.client == nil || c.diag.Paso != PasoIniciarSesion || c.IniciarSesion == nil {
		return
	}
	c.entroSolo = true
	c.iniciarSesion()
}

// listo: verificado y sin nada pendiente de la custodia. Una clave por envolver
// no abre los datos hasta que la persona la protege o lo deja para otro momento.
func (c *Panel) listo() bool {
	return c.status.Verified && !c.status.RecoverySetupPending && c.diag.Paso != PasoProteger
}

func (c *Panel) lista() {
	if c.listo() && c.AlLista != nil {
		c.AlLista(c.ctx)
	}
}

// Cerrar invalidates pending work before clearing private data or switching account.
func (c *Panel) Salir()  { c.cerrar(true) }
func (c *Panel) Cerrar() { c.cerrar(false) }

// WaitClosed joins the most recent Cerrar/Salir before another presenter opens
// the same native SDK store. Call it off the UI thread, after detaching the view.
func (c *Panel) WaitClosed(ctx context.Context) error {
	c.mu.RLock()
	done := c.closing
	c.mu.RUnlock()
	if done == nil {
		return ctx.Err()
	}
	select {
	case <-done:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Retire releases a permanently detached panel's work registration. Device keys
// remain persisted. The caller must not start this panel again while retiring.
func (c *Panel) Retire(ctx context.Context) error {
	if err := c.WaitClosed(ctx); err != nil {
		return err
	}
	worker := c.workers.Load()
	if worker != nil {
		if err := worker.Drain(ctx); err != nil {
			return err
		}
		worker.Close() //nolint:contextcheck // Drain joined all jobs; final local cleanup outlives cancelled view contexts.
		c.workers.CompareAndSwap(worker, nil)
	}
	return nil
}

func (c *Panel) cerrar(revoke bool) {
	oldContext := c.ctx
	c.generation++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	var drain func()
	if c.AlCerrar != nil {
		drain = c.AlCerrar()
	}
	if c.dialog != nil {
		c.dialog.Cerrar()
		c.dialog = nil
	}
	c.mu.Lock()
	client := c.client
	localWork := c.localWork
	worker := c.workers.Load()
	c.client = nil
	previous := c.closing
	if client != nil || len(c.clients) > 0 || drain != nil || oldContext != nil && (localWork != nil || worker != nil) {
		done := make(chan struct{})
		c.closing = done
		go func() {
			defer close(done)
			c.closeDevice(previous, localWork, worker, client, drain, oldContext != nil, revoke)
		}()
	}
	c.mu.Unlock()
	if c.body != nil {
		c.body.Objects = nil
	}
	if c.footer != nil {
		c.footer.Objects = nil
	}
	c.diag.olvidar()
	c.diag = Diagnostico{}
	c.comparison = ""
	c.verificationWaiting = false
	c.status = mx.VerificationStatus{}
	c.txn = ""
	c.verifyUser = ""
	c.authWait = nil
	c.busy = false
}

// Cancelled UI lifetimes cannot bound cleanup; the next open awaits its completion.
//
//nolint:contextcheck // Teardown deliberately owns a detached lifetime after parent cancellation.
func (c *Panel) closeDevice(previous <-chan struct{}, localWork *offline.WorkGate, worker *PanelWorkers, client *mx.Client, drain func(), hadContext, revoke bool) {
	if previous != nil {
		<-previous
	}
	if localWork != nil {
		_ = localWork.Drain(context.Background())
	}
	if worker != nil && hadContext {
		_ = worker.Drain(context.Background())
	}
	if drain != nil {
		drain()
	}
	// Open/Login can finish before its UI callback installs c.client. Those SDKs
	// were already remembered by the joined job and must close before store reuse.
	defer c.closeRetainedClients(worker)
	if client == nil {
		return
	}
	if revoke {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = client.Logout(ctx)
		c.mu.Lock()
		delete(c.clients, client)
		c.mu.Unlock()
		if worker != nil {
			worker.release(client)
		}
	} else {
		c.releaseClient(client, worker)
	}
}

func (c *Panel) closeRetainedClients(worker *PanelWorkers) {
	c.mu.RLock()
	clients := make([]*mx.Client, 0, len(c.clients))
	for client := range c.clients {
		clients = append(clients, client)
	}
	c.mu.RUnlock()
	for _, client := range clients {
		c.releaseClient(client, worker)
	}
}

func (c *Panel) lienzo() fyne.Canvas {
	if c.Lienzo != nil {
		return c.Lienzo()
	}
	return nil
}

func (c *Panel) AbrirPanel() {
	if c.dialog != nil {
		c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
		return
	}
	canvas := c.lienzo()
	if canvas == nil {
		return
	}
	c.body = container.New(ui.Columna{Hueco: tema.Espacio(4)})
	c.footer = container.New(ui.Columna{Hueco: tema.Espacio(2)})
	c.dialog = ui.NuevoDialogo(c.t("matrix.title"), c.body).Con(ui.DialogoMD, c.footer)
	c.dialog.RotuloCerrar = c.t("action.close")
	c.dialog.AlCerrar = func() {
		if c.authWait != nil {
			select {
			case c.authWait <- false:
			default:
			}
			c.authWait = nil
		}
		c.dialog.Cerrar()
		c.dialog = nil
	}
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	c.dialog.Mostrar(canvas)
}
func (c *Panel) t(key string) string { return c.Traducir(key) }
func (c *Panel) anchoTexto() float32 {
	width := float32(480)
	if canvas := c.lienzo(); canvas != nil {
		width = max(180, min(width, canvas.Size().Width-48))
	}
	return width
}

func (c *Panel) parrafo(text string) fyne.CanvasObject {
	return ui.NuevoParrafo(text, tema.TextoSM, ui.Normal, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())).A(c.anchoTexto())
}

func (c *Panel) boton(key string, fn func()) *ui.Boton {
	b := ui.NuevoBoton(c.t(key), fn)
	b.Inerte = c.busy
	return b
}

func (c *Panel) pintar() {
	if c.dialog == nil {
		return
	}
	c.dialog.NoDescartable = false
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.subtitle"))}
	c.footer.Objects = nil
	switch {
	case c.comparison != "":
		c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.compare")), c.parrafo(c.comparison)}
		c.footer.Add(c.boton("matrix.mismatch", func() {
			c.comparison = ""
			c.verificationWaiting = false
			c.operar(func(ctx context.Context, client *mx.Client) error { return client.CancelVerification(ctx, c.txn, true) }, nil)
		}))
		c.footer.Add(c.boton("matrix.match", func() {
			c.comparison = ""
			c.verificationWaiting = true
			c.operar(func(ctx context.Context, client *mx.Client) error { return client.ConfirmSAS(ctx, c.txn) }, nil)
		}))
	case c.verificationWaiting:
		c.body.Add(c.parrafo(c.t("matrix.waiting")))
		c.footer.Add(c.boton("action.cancel", func() {
			c.verificationWaiting = false
			c.operar(func(ctx context.Context, client *mx.Client) error {
				return client.CancelVerification(ctx, c.txn, false)
			}, nil)
		}))
	case c.busy && c.client != nil && !c.status.Verified:
		c.body.Add(c.parrafo(c.t("matrix.unlocking")))
	case c.busy:
		c.body.Add(c.parrafo(c.t("matrix.waiting")))
	case c.client != nil && c.diag.Paso == PasoIniciarSesion:
		// Nada que teclear aquí: la clave se mete en Sytue ID, en el navegador.
		c.body.Add(c.parrafo(c.t("matrix.signInBody")))
		c.footer.Add(c.boton("matrix.signIn", c.iniciarSesion))
	case c.status.Verified && !c.status.RecoverySetupPending:
		c.body.Add(c.parrafo(c.t("matrix.trusted")))
		c.footer.Add(c.boton("matrix.devices", c.dispositivos))
		c.footer.Add(c.boton("matrix.verify", c.verificar))
		if c.Personas != nil {
			c.footer.Add(c.boton("matrix.verifyPerson", c.verificarPersona))
		}
		c.footer.Add(c.boton("matrix.notices", c.licencias))
		c.botonSoltarFuente()
	case c.client == nil:
		c.footer.Add(c.boton("matrix.connect", c.conectar))
	case c.diag.Paso == PasoDesbloquear || c.diag.Paso == PasoCrear:
		// Solo queda aquí si abrir o crear falló (red, servicio): se reintenta igual.
		c.body.Add(c.parrafo(c.t("matrix.failed")))
		c.footer.Add(c.boton("action.retry", c.Recuperar))
		c.botonSoltarFuente()
	default:
		// La cuenta tiene una clave que nadie aquí abre: ni envoltorio, ni la copia de
		// este dispositivo, ni una cuenta a medias que rehacer (Diagnosticar ya lo
		// intentó). En Rooms la salida principal es empezar de cero, tras su
		// confirmación; verificar desde otro dispositivo queda de segunda, como en la web.
		c.body.Add(c.parrafo(c.t("matrix.unknownKey")))
		if c.status.HasIdentity && !c.nativeRecovery && c.Base.Bootstrap.StorageProduct == "" {
			c.footer.Add(c.boton("matrix.reset", c.Restablecer))
		}
		c.botonSoltarFuente()
		c.botonVerificar()
	}
	c.actualizar()
}

// botonSoltarFuente ofrece «Vaciar y volver a los datos del servidor» en una app de
// registros: se abra o no Matrix, como el aviso de la web (SourceReleaseNotice), que
// se ve siempre que la fuente esté congelada. Quien tiene sus datos en Matrix puede
// así dejarlo y pasar a la bóveda nueva sin abrirlo antes (ADR 0086, ADR 0087).
func (c *Panel) botonSoltarFuente() {
	if c.SoltarFuente != nil {
		c.footer.Add(c.boton("vault.release", func() { c.ConfirmarSoltarFuente(false) }))
	}
}

func (c *Panel) botonVerificar() {
	verify := c.boton("matrix.verify", c.verificar)
	verify.Variante = ui.Secundario
	c.footer.Add(verify)
}

// custodiar corre una operación de la custodia y vuelve a decidir el paso.
func (c *Panel) custodiar(fn func(context.Context, *mx.Client, Custodia) (Diagnostico, bool, error)) {
	if c.busy {
		return
	}
	client, err := c.Cliente(c.ctx)
	if err != nil {
		c.fallo()
		return
	}
	custodia := c.custodia()
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	ctx, generation := c.ctx, c.generation
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		diag, done, err := fn(job, client, custodia)
		if err == nil && !done {
			diag.Status, err = c.recoveryStatus(job, client)
			if err == nil {
				diag, err = Diagnosticar(job, client, custodia)
			}
		}
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				return
			}
			c.busy = false
			switch {
			case errors.Is(err, ErrSinDesbloqueo):
				c.diag.Paso = PasoIniciarSesion
			case err != nil:
				c.fallo()
			default:
				c.diag.olvidar()
				c.diag, c.status = diag, diag.Status
			}
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.lista()
			c.irSoloAEntrar() //nolint:contextcheck // Como iniciarSesion: vive con el ciclo del dispositivo (c.ctx).
		})
	})
}

// iniciarSesion vuelve a entrar en Sytue ID para traer la clave de desbloqueo a este
// dispositivo (la persona mete allí su clave) y, al volver, desbloquea solo.
func (c *Panel) iniciarSesion() {
	if c.busy || c.IniciarSesion == nil {
		return
	}
	client, err := c.Cliente(c.ctx)
	if err != nil {
		c.fallo()
		return
	}
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	ctx, generation, legado := c.ctx, c.generation, c.diag.Legado
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		err := c.IniciarSesion(job, legado)
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				return
			}
			if err != nil {
				c.busy = false
				c.fallo()
				c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
				return
			}
			c.revisar(ctx, generation, client, false)
		})
	})
}

func (c *Panel) fallo() {
	if c.Avisar != nil {
		c.Avisar(c.t("matrix.failed"))
	}
}

func (c *Panel) conectar() {
	if c.busy || c.Login == nil {
		return
	}
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	ctx, cfg, generation := c.ctx, c.Base, c.generation
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		// Refresh the authenticated mapping before every login attempt.
		bootstrap, err := c.Bootstrap(job)
		cfg.Bootstrap = bootstrap
		var client *mx.Client
		if err == nil {
			client, err = c.Login(job, cfg)
		}
		c.remember(client, worker)
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				if client != nil {
					go c.releaseClient(client, worker)
				}
				return
			}
			c.busy = false
			if err != nil {
				c.fallo()
				c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
				return
			}
			c.Base = cfg
			c.instalar(client) //nolint:contextcheck // The UI installs future actions bound to the retained device lifetime.
		})
	})
}

func (c *Panel) operar(fn func(context.Context, *mx.Client) error, after func()) {
	if c.busy {
		return
	}
	client, err := c.Cliente(c.ctx)
	if err != nil {
		c.fallo()
		return
	}
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	ctx, generation := c.ctx, c.generation
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		err := fn(job, client)
		status, statusErr := c.recoveryStatus(job, client)
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) {
				return
			}
			c.busy = false
			c.status = status
			if err != nil || statusErr != nil {
				c.fallo()
			}
			if after != nil {
				after()
			}
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.lista()
		})
	})
}

// Restablecer es «No tengo otro dispositivo» (solo Rooms): un paso de
// confirmación con la advertencia y una casilla obligatoria antes del botón de
// peligro, igual que el modal de la web. La clave nueva se envuelve con la clave de
// desbloqueo de Sytue ID antes de publicarla y no se enseña; aquí no se teclea nada.
// Lo destructivo nunca va a un solo clic.
func (c *Panel) Restablecer() {
	var accion *ui.Boton
	understood := ui.NuevaCasilla(c.t("matrix.resetUnderstood"), func(marcada bool) {
		accion.Inerte = c.busy || !marcada
		accion.Refresh()
	})
	accion = c.boton("matrix.resetAction", func() {
		if !understood.Marcada {
			return
		}
		c.custodiar(func(ctx context.Context, client *mx.Client, custodia Custodia) (Diagnostico, bool, error) {
			m := c.material()
			defer m.Borrar()
			return Diagnostico{}, false, Restablecer(ctx, client, custodia, m, c.uia(ctx, client))
		})
	})
	accion.Variante = ui.Peligro
	accion.Inerte = true
	understood.Escalon = tema.TextoSM
	understood.AnchoEtiqueta = c.anchoTexto()
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.resetBody")), understood}
	back := c.boton("action.back", c.pintar)
	back.Variante = ui.Secundario
	c.footer.Objects = []fyne.CanvasObject{accion, back}
	c.actualizar()
}

// ConfirmarSoltarFuente es el paso de confirmación de «Vaciar y volver a los datos
// del servidor»: qué se pierde, que Sytue ID pedirá identificarse, y una casilla
// obligatoria antes del botón de peligro. `fallo` repite el paso con el aviso de
// que no salió. Lo destructivo nunca va a un solo clic.
func (c *Panel) ConfirmarSoltarFuente(fallo bool) {
	if c.dialog == nil || c.SoltarFuente == nil {
		return
	}
	var accion *ui.Boton
	understood := ui.NuevaCasilla(c.t("vault.releaseUnderstood"), func(marcada bool) {
		accion.Inerte = c.busy || !marcada
		accion.Refresh()
	})
	accion = c.boton("vault.release", func() {
		if !understood.Marcada || c.busy {
			return
		}
		c.busy = true
		accion.Inerte = true
		accion.Refresh()
		ctx, soltar := c.ctx, c.SoltarFuente
		if ctx == nil {
			ctx = context.Background()
		}
		go func() {
			err := soltar(ctx)
			fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
				c.busy = false
				if err != nil {
					c.ConfirmarSoltarFuente(true)
					return
				}
				// La app ya ha vuelto a la API; el diálogo de Matrix sobra.
				c.pintar()
			})
		}()
	})
	accion.Variante = ui.Peligro
	accion.Inerte = true
	understood.Escalon = tema.TextoSM
	understood.AnchoEtiqueta = c.anchoTexto()
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("vault.releaseBody"))}
	if !c.SoltarSinIdentificar {
		c.body.Add(c.parrafo(c.t("vault.releaseSignIn")))
	}
	c.body.Add(understood)
	if fallo {
		c.body.Add(c.parrafo(c.t("vault.releaseFailed")))
	}
	back := c.boton("action.back", c.pintar)
	back.Variante = ui.Secundario
	c.footer.Objects = []fyne.CanvasObject{accion, back}
	c.actualizar()
}

// Objetos devuelve las vistas del cuerpo y del pie del diálogo abierto.
func (c *Panel) Objetos() (cuerpo, pie []fyne.CanvasObject) {
	if c.body != nil {
		cuerpo = c.body.Objects
	}
	if c.footer != nil {
		pie = c.footer.Objects
	}
	return cuerpo, pie
}
func (c *Panel) verificar() { c.verificarCon("") }
func (c *Panel) verificarCon(user string) {
	var txn id.VerificationTransactionID
	c.operar(func(ctx context.Context, client *mx.Client) error {
		var err error
		txn, err = client.StartVerificationWith(ctx, user)
		return err
	}, func() { c.txn = txn; c.verifyUser = user; c.verificationWaiting = txn != "" })
}

func (c *Panel) verificarPersona() {
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.choosePerson"))}
	client, err := c.Cliente(c.ctx)
	if err != nil {
		return
	}
	for _, member := range c.personas() {
		if member.Subject == c.Base.Subject {
			continue
		}
		user, err := client.UserForSubject(member.Subject)
		if err != nil {
			continue
		}
		c.body.Add(c.parrafo(member.Name))
		c.body.Add(ui.NuevoBoton(c.t("matrix.verify"), func() { c.verificarCon(user) }))
	}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.back", c.pintar)}
	c.actualizar()
}

func (c *Panel) uia(ctx context.Context, client *mx.Client) mautrix.UIACallback {
	return func(challenge *mautrix.RespUserInteractive) any {
		target, err := client.UIASSOURL(challenge)
		if err != nil {
			return nil
		}
		continued := make(chan bool, 1)
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if ctx.Err() != nil {
				return
			}
			c.authWait = continued
			c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.auth"))}
			b := ui.NuevoBoton(c.t("matrix.authContinue"), func() {
				select {
				case continued <- true:
				default:
				}
			})
			c.footer.Objects = []fyne.CanvasObject{b}
			c.actualizar()
			if c.Abrir == nil || c.Abrir(target) != nil {
				select {
				case continued <- false:
				default:
				}
			}
		})
		select {
		case ok := <-continued:
			if ok {
				return map[string]string{"session": challenge.Session}
			}
		case <-ctx.Done():
		}
		return nil
	}
}

func (c *Panel) dispositivos() {
	client, err := c.Cliente(c.ctx)
	if err != nil {
		return
	}
	ctx, generation := c.ctx, c.generation
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		devices, err := client.Devices(job)
		accepted := job.Err() == nil
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || !accepted || !c.workCurrent(ctx, worker) || c.dialog == nil {
				return
			}
			if err != nil {
				c.fallo()
				return
			}
			c.body.Objects = nil
			for _, device := range devices {
				d := device
				label := d.Name
				if label == "" {
					label = d.ID
				}
				label += " · " + d.ID
				c.body.Add(c.parrafo(label))
				if !d.Current {
					c.body.Add(c.boton("matrix.revoke", func() { //nolint:contextcheck // Revoke is a future UI action using the retained panel lifetime.
						c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.revoke") + " · " + label)}
						c.footer.Objects = []fyne.CanvasObject{c.boton("action.cancel", c.pintar), c.boton("matrix.revoke", func() {
							c.operar(func(ctx context.Context, client *mx.Client) error {
								return client.RevokeDevice(ctx, d.ID, c.uia(ctx, client))
							}, nil)
						})}
						c.body.Refresh()
						c.footer.Refresh()
					}))
				}
			}
			c.footer.Objects = []fyne.CanvasObject{c.boton("action.close", func() { c.dialog.Cerrar(); c.dialog = nil })}
			c.actualizar()
		})
	})
}

func (c *Panel) VerificationRequested(ctx context.Context, txn id.VerificationTransactionID, from id.UserID, device id.DeviceID) {
	ctx, generation, _, valid := c.callback(ctx)
	if !valid {
		return
	}
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		client, err := c.Cliente(job)
		if err != nil {
			return
		}
		own, _ := client.Identity()

		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if !c.workCurrent(ctx, worker) || generation != 0 && generation != c.generation {
				return
			}
			if from.String() != own && from.String() != c.verifyUser {
				allowed := slices.ContainsFunc(c.personas(), func(member Persona) bool {
					user, err := client.UserForSubject(member.Subject)
					return err == nil && user == from.String()
				})
				if !allowed {
					c.cancelVerification(ctx, txn, client)
					return
				}
			}
			c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.txn = txn
			c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.incoming")), c.parrafo(from.String() + " · " + device.String())}
			c.footer.Objects = []fyne.CanvasObject{c.boton("action.cancel", func() { c.cancelVerification(ctx, txn, client); c.pintar() }), c.boton("matrix.verify", func() { //nolint:contextcheck // The accepted action inherits the active device lifecycle.
				c.operar(func(ctx context.Context, client *mx.Client) error { return client.AcceptVerification(ctx, txn) }, nil)
			})}
			c.actualizar()
		})
	})
}

func (c *Panel) cancelVerification(ctx context.Context, txn id.VerificationTransactionID, client *mx.Client) {
	ctx, generation, _, valid := c.callback(ctx)
	if valid {
		c.runWork(ctx, generation, false, func(job context.Context, _ *PanelWorkers) { _ = client.CancelVerification(job, txn, false) })
	}
}

func (c *Panel) VerificationReady(ctx context.Context, txn id.VerificationTransactionID, _ id.DeviceID, sas, _ bool, _ *verificationhelper.QRCode) {
	if !sas {
		return
	}
	ctx, generation, _, valid := c.callback(ctx)
	if !valid {
		return
	}
	client, err := c.Cliente(ctx)
	if err != nil {
		return
	}
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		if err := client.StartSAS(job, txn); err != nil {
			fyne.Do(func() {
				if c.workCurrent(ctx, worker) {
					c.fallo()
				}
			})
		}
	})
}

func (c *Panel) VerificationCancelled(ctx context.Context, _ id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) {
	ctx, generation, worker, valid := c.callback(ctx)
	if !valid {
		return
	}
	fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
		if c.workCurrent(ctx, worker) && (generation == 0 || generation == c.generation) {
			c.busy = false
			c.comparison = ""
			c.verificationWaiting = false
			c.fallo()
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
		}
	})
}

func (c *Panel) VerificationDone(ctx context.Context, _ id.VerificationTransactionID, _ event.VerificationMethod) {
	ctx, generation, _, valid := c.callback(ctx)
	if !valid {
		return
	}
	c.runWork(ctx, generation, false, func(job context.Context, worker *PanelWorkers) {
		client, err := c.Cliente(job)
		if err != nil {
			return
		}
		status, err := client.VerificationStatus(job)
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if !c.workCurrent(ctx, worker) || generation != 0 && generation != c.generation {
				return
			}
			c.status = status
			c.busy = false
			c.comparison = ""
			c.verificationWaiting = false
			if err != nil {
				c.fallo()
			}
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.lista()
		})
	})
}

func (c *Panel) ShowSAS(ctx context.Context, txn id.VerificationTransactionID, emojis []rune, _ []string, decimals []int) {
	ctx, generation, worker, valid := c.callback(ctx)
	if !valid {
		return
	}
	_, err := c.Cliente(ctx)
	if err != nil {
		return
	}
	// Include the SDK's decimal SAS for platforms whose system font lacks emoji.
	comparison := string(emojis) + "\n" + fmt.Sprint(decimals)
	fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
		if !c.workCurrent(ctx, worker) || generation != 0 && generation != c.generation {
			return
		}
		c.busy = false
		c.comparison = comparison
		c.txn = txn
		c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
		c.pintar()     //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	})
}

func (c *Panel) licencias() {
	notices := mx.ThirdPartyNotices + c.Licencias
	text := ui.NuevaAreaTexto(c.t("matrix.notices"))
	text.PonerTexto(notices)
	text.SoloLectura(true)
	c.body.Objects = []fyne.CanvasObject{text}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.copy", func() { c.Copiar(notices) }), c.boton("action.close", c.pintar)}
	c.actualizar()
}

func (c *Panel) actualizar() {
	c.body.Refresh()
	c.footer.Refresh()
	if c.dialog != nil {
		c.dialog.Recolocar()
	}
}

func (c *Panel) esperarCierre(ctx context.Context) error {
	c.mu.RLock()
	closing := c.closing
	c.mu.RUnlock()
	if closing == nil {
		return nil
	}
	select {
	case <-closing:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PedirVerificacion explains a blocked send while preserving the composer. The
// human explicitly chooses whom to verify; no trust is granted automatically.
func (c *Panel) PedirVerificacion() {
	c.AbrirPanel()
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.verifyPeopleFirst"))}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.close", c.pintar), c.boton("matrix.verifyPerson", c.verificarPersona)}
	c.actualizar()
}

// Persona is an application-authorized peer offered for manual SAS verification.
// Private storage panels leave Personas nil and permit only the owner's devices.
type Persona struct{ Subject, Name string }

func (c *Panel) personas() []Persona {
	if c.Personas == nil {
		return nil
	}
	return c.Personas()
}

// Recuperar vuelve a decidir el paso de un dispositivo (también uno verificado al
// que le falta la clave de almacenamiento) y abre el panel. Nunca pide la clave.
func (c *Panel) Recuperar() {
	c.AbrirPanel()
	if c.dialog == nil || c.busy || c.client == nil {
		return
	}
	c.busy = true
	c.pintar()
	c.revisar(c.ctx, c.generation, c.client, false)
}

// Verificado is the UI readiness state; private repositories still authenticate
// their storage keys and scope independently before accessing any source.
func (c *Panel) Verificado() bool {
	return c.listo() && c.ctx != nil && c.ctx.Err() == nil
}

func (c *Panel) recoveryStatus(ctx context.Context, client *mx.Client) (mx.VerificationStatus, error) {
	status, err := client.VerificationStatus(ctx)
	if err != nil || !c.nativeRecovery || status.RecoverySetupPending {
		return status, err
	}
	if err = client.SyncNativeRecovery(ctx); err != nil && !errors.Is(err, mx.ErrRecoveryRequired) {
		return status, err
	}
	return client.VerificationStatus(ctx)
}
