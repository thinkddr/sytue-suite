// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

// Device verification and recovery mirror the MatrixRoomsGate inside Rooms.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// Cifrado owns the native device lifecycle. All UI state stays on Fyne's thread;
// network/crypto operations run outside it. Access tokens and recovery secrets
// never enter the Rooms metadata service or persistent UI preferences.
type Cifrado struct {
	Sala                *Sala
	API                 *api.Rooms
	Base                mx.Config
	Login               func(context.Context, mx.Config) (*mx.Client, error)
	Abrir               func(string) error
	AlLista             func(context.Context)
	AlEvento            func(mx.DecryptedEvent, error)
	mu                  sync.RWMutex
	client              *mx.Client
	closing             <-chan struct{}
	ctx                 context.Context
	cancel              context.CancelFunc
	generation          uint64
	body, footer        *fyne.Container
	dialog              *ui.Dialogo
	busy                bool
	status              mx.VerificationStatus
	txn                 id.VerificationTransactionID
	verifyUser          string
	key                 *ui.Entrada
	recovery            string
	comparison          string
	verificationWaiting bool
	authWait            chan bool
}

func (c *Cifrado) Cliente(ctx context.Context) (*mx.Client, error) {
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

func (c *Cifrado) Iniciar(ctx context.Context, subject, tenant string) {
	c.Cerrar() //nolint:contextcheck // Old-device shutdown is detached before the new lifecycle begins.
	ctx, c.cancel = context.WithCancel(ctx)
	c.ctx = ctx
	c.Sala.ctx = ctx
	c.Base.Subject, c.Base.Tenant = subject, tenant
	c.Base.VerificationCallbacks = c
	c.generation++
	generation := c.generation
	c.busy = true
	c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	cfg := c.Base
	go func() {
		if err := c.esperarCierre(ctx); err != nil {
			return
		}
		bootstrap, err := c.API.MatrixBootstrap(ctx)
		cfg.Bootstrap = bootstrap
		var client *mx.Client
		if err == nil {
			client, err = mx.Open(ctx, cfg)
		}
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation {
				if client != nil {
					go func() { _ = client.Close() }()
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
			c.instalar(client)
		})
	}()
}

func (c *Cifrado) instalar(client *mx.Client) {
	c.mu.Lock()
	c.client = client
	c.mu.Unlock()
	ctx, generation := c.ctx, c.generation
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	go func() {
		_ = client.Sync(ctx, func(value mx.DecryptedEvent, err error) {
			fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
				if generation == c.generation && ctx.Err() == nil && c.AlEvento != nil {
					c.AlEvento(value, err)
				}
			})
		})
	}()
	go func() {
		err := client.WaitReady(ctx)
		var status mx.VerificationStatus
		if err == nil {
			status, err = client.VerificationStatus(ctx)
		}
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation {
				return
			}
			c.busy = false
			c.status = status
			if err != nil {
				c.fallo()
			}
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.lista()
		})
	}()
}

func (c *Cifrado) lista() {
	if c.status.Verified && c.recovery == "" && c.AlLista != nil {
		c.AlLista(c.ctx)
	}
}

// Cerrar invalidates pending work before clearing private data or switching account.
func (c *Cifrado) Salir()  { c.cerrar(true) }
func (c *Cifrado) Cerrar() { c.cerrar(false) }
func (c *Cifrado) cerrar(revoke bool) {
	c.generation++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.dialog != nil {
		c.dialog.Cerrar()
		c.dialog = nil
	}
	c.mu.Lock()
	client := c.client
	c.client = nil
	previous := c.closing
	if client != nil {
		done := make(chan struct{})
		c.closing = done
		go func() {
			defer close(done)
			if previous != nil {
				<-previous
			}
			if revoke {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_ = client.Logout(ctx)
			} else {
				_ = client.Close()
			}
		}()
	}
	c.mu.Unlock()
	if c.key != nil {
		c.key.PonerTexto("")
	}
	if c.body != nil {
		c.body.Objects = nil
	}
	if c.footer != nil {
		c.footer.Objects = nil
	}
	c.recovery = ""
	c.comparison = ""
	c.verificationWaiting = false
	c.status = mx.VerificationStatus{}
	c.txn = ""
	c.busy = false
}

func (c *Cifrado) lienzo() fyne.Canvas {
	if canvas := fyne.CurrentApp().Driver().CanvasForObject(c.Sala.P.Objeto()); canvas != nil {
		return canvas
	}
	for _, win := range fyne.CurrentApp().Driver().AllWindows() {
		if win != nil && win.Canvas() != nil {
			return win.Canvas()
		}
	}
	return nil
}

func (c *Cifrado) AbrirPanel() {
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
		// A recovery key remains visible until its owner explicitly saves it.
		if c.recovery != "" {
			return
		}
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
func (c *Cifrado) t(key string) string { return c.Sala.P.T.T(key, nil) }
func (c *Cifrado) parrafo(text string) fyne.CanvasObject {
	width := float32(480)
	if canvas := c.lienzo(); canvas != nil {
		width = max(180, min(width, canvas.Size().Width-48))
	}
	return ui.NuevoParrafo(text, tema.TextoSM, ui.Normal, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())).A(width)
}

func (c *Cifrado) boton(key string, fn func()) *ui.Boton {
	b := ui.NuevoBoton(c.t(key), fn)
	b.Inerte = c.busy
	return b
}

func (c *Cifrado) pintar() {
	if c.dialog == nil {
		return
	}
	c.dialog.NoDescartable = c.recovery != ""
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.subtitle"))}
	c.footer.Objects = nil
	switch {
	case c.recovery != "":
		c.body.Objects = append(c.body.Objects, c.parrafo(c.t("matrix.saveBody")))
		key := ui.NuevaEntrada(c.t("matrix.recoveryKey"))
		key.PonerTexto(c.recovery)
		key.SoloLectura(true)
		c.body.Objects = append(c.body.Objects, key)
		c.footer.Add(c.boton("action.copy", func() { c.Sala.Copiar(c.recovery) }))
		c.footer.Add(c.boton("matrix.saved", func() { c.recovery = ""; key.PonerTexto(""); c.pintar(); c.lista() }))
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
	case c.busy:
		c.body.Add(c.parrafo(c.t("matrix.waiting")))
	case c.status.Verified:
		c.body.Add(c.parrafo(c.t("matrix.trusted")))
		c.footer.Add(c.boton("matrix.devices", c.dispositivos))
		c.footer.Add(c.boton("matrix.verify", c.verificar))
		c.footer.Add(c.boton("matrix.verifyPerson", c.verificarPersona))
		c.footer.Add(c.boton("matrix.notices", c.licencias))
	case c.client == nil:
		c.footer.Add(c.boton("matrix.connect", c.conectar))
	default:
		if !c.status.HasIdentity {
			c.footer.Add(c.boton("matrix.setup", c.crear))
		}
		c.footer.Add(c.boton("matrix.recover", c.recuperar))
		c.footer.Add(c.boton("matrix.verify", c.verificar))
	}
	c.actualizar()
}
func (c *Cifrado) fallo() { c.Sala.P.Avisar(ui.AvisoPeligro, c.t("matrix.failed")) }
func (c *Cifrado) conectar() {
	if c.busy || c.Login == nil {
		return
	}
	c.busy = true
	c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	ctx, cfg, generation := c.ctx, c.Base, c.generation
	go func() {
		// Refresh the authenticated mapping before every login attempt.
		bootstrap, err := c.API.MatrixBootstrap(ctx)
		cfg.Bootstrap = bootstrap
		var client *mx.Client
		if err == nil {
			client, err = c.Login(ctx, cfg)
		}
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation {
				if client != nil {
					go func() { _ = client.Close() }()
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
			c.instalar(client)
		})
	}()
}

func (c *Cifrado) operar(fn func(context.Context, *mx.Client) error, after func()) {
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
	go func() {
		err := fn(ctx, client)
		status, statusErr := client.VerificationStatus(ctx)
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation {
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
	}()
}

func (c *Cifrado) crear() {
	var recovery string
	c.operar(func(ctx context.Context, client *mx.Client) error {
		var err error
		recovery, err = client.BootstrapRecovery(ctx, c.uia(ctx, client))
		return err
	}, func() { c.recovery = recovery })
}

func (c *Cifrado) recuperar() {
	c.key = ui.NuevaEntrada(c.t("matrix.recoveryKey"))
	c.key.Contrasena(true)
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.recoverBody")), c.key}
	c.footer.Objects = []fyne.CanvasObject{c.boton("matrix.recover", func() {
		key := c.key.Texto()
		c.key.PonerTexto("")
		c.operar(func(ctx context.Context, client *mx.Client) error { return client.Recover(ctx, strings.TrimSpace(key)) }, nil)
	})}
	c.actualizar()
}
func (c *Cifrado) verificar() { c.verificarCon("") }
func (c *Cifrado) verificarCon(user string) {
	var txn id.VerificationTransactionID
	c.operar(func(ctx context.Context, client *mx.Client) error {
		var err error
		txn, err = client.StartVerificationWith(ctx, user)
		return err
	}, func() { c.txn = txn; c.verifyUser = user; c.verificationWaiting = txn != "" })
}

func (c *Cifrado) verificarPersona() {
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.choosePerson"))}
	client, err := c.Cliente(c.ctx)
	if err != nil {
		return
	}
	for _, member := range c.Sala.roster {
		if member.ID == c.Sala.Yo {
			continue
		}
		user, err := client.UserForSubject(member.ID)
		if err != nil {
			continue
		}
		c.body.Add(c.parrafo(member.Nombre))
		c.body.Add(ui.NuevoBoton(c.t("matrix.verify"), func() { c.verificarCon(user) }))
	}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.back", c.pintar)}
	c.actualizar()
}

func (c *Cifrado) uia(ctx context.Context, client *mx.Client) mautrix.UIACallback {
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

func (c *Cifrado) dispositivos() {
	client, err := c.Cliente(c.ctx)
	if err != nil {
		return
	}
	ctx, generation := c.ctx, c.generation
	go func() {
		devices, err := client.Devices(ctx)
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if generation != c.generation || c.dialog == nil {
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
					c.body.Add(c.boton("matrix.revoke", func() {
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
	}()
}

func (c *Cifrado) VerificationRequested(ctx context.Context, txn id.VerificationTransactionID, from id.UserID, device id.DeviceID) {
	go func() {
		client, err := c.Cliente(ctx)
		if err != nil {
			return
		}
		own, _ := client.Identity()

		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if ctx.Err() != nil {
				return
			}
			if from.String() != own && from.String() != c.verifyUser {
				allowed := slices.ContainsFunc(c.Sala.roster, func(member Miembro) bool {
					user, err := client.UserForSubject(member.ID)
					return err == nil && user == from.String()
				})
				if !allowed {
					go func() { _ = client.CancelVerification(ctx, txn, false) }()
					return
				}
			}
			c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
			c.txn = txn
			c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.incoming")), c.parrafo(from.String() + " · " + device.String())}
			c.footer.Objects = []fyne.CanvasObject{c.boton("action.cancel", func() { go func() { _ = client.CancelVerification(ctx, txn, false) }(); c.pintar() }), c.boton("matrix.verify", func() { //nolint:contextcheck // The accepted action inherits the active device lifecycle.
				c.operar(func(ctx context.Context, client *mx.Client) error { return client.AcceptVerification(ctx, txn) }, nil)
			})}
			c.actualizar()
		})
	}()
}

func (c *Cifrado) VerificationReady(ctx context.Context, txn id.VerificationTransactionID, _ id.DeviceID, sas, _ bool, _ *verificationhelper.QRCode) {
	if !sas {
		return
	}
	client, err := c.Cliente(ctx)
	if err != nil {
		return
	}
	go func() {
		if err := client.StartSAS(ctx, txn); err != nil {
			fyne.Do(c.fallo)
		}
	}()
}

func (c *Cifrado) VerificationCancelled(ctx context.Context, _ id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) {
	fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
		if ctx.Err() == nil {
			c.busy = false
			c.comparison = ""
			c.verificationWaiting = false
			c.fallo()
			c.pintar() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
		}
	})
}

func (c *Cifrado) VerificationDone(ctx context.Context, _ id.VerificationTransactionID, _ event.VerificationMethod) {
	go func() {
		client, err := c.Cliente(ctx)
		if err != nil {
			return
		}
		status, err := client.VerificationStatus(ctx)
		fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
			if ctx.Err() != nil {
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
	}()
}

func (c *Cifrado) ShowSAS(ctx context.Context, txn id.VerificationTransactionID, emojis []rune, _ []string, decimals []int) {
	_, err := c.Cliente(ctx)
	if err != nil {
		return
	}
	// Include the SDK's decimal SAS for platforms whose system font lacks emoji.
	comparison := string(emojis) + "\n" + fmt.Sprint(decimals)
	fyne.Do(func() { //nolint:contextcheck // Fyne actions resume with the device lifecycle context stored in c.ctx.
		if ctx.Err() != nil {
			return
		}
		c.busy = false
		c.comparison = comparison
		c.txn = txn
		c.AbrirPanel() //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
		c.pintar()     //nolint:contextcheck // UI rendering installs future actions scoped to the device lifecycle.
	})
}

func (c *Cifrado) licencias() {
	notices := mx.ThirdPartyNotices + "\n" + llamada.ThirdPartyNotices
	text := ui.NuevaAreaTexto(c.t("matrix.notices"))
	text.PonerTexto(notices)
	text.SoloLectura(true)
	c.body.Objects = []fyne.CanvasObject{text}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.copy", func() { c.Sala.Copiar(notices) }), c.boton("action.close", c.pintar)}
	c.actualizar()
}

func (c *Cifrado) actualizar() {
	c.body.Refresh()
	c.footer.Refresh()
	if c.dialog != nil {
		c.dialog.Recolocar()
	}
}

func (c *Cifrado) esperarCierre(ctx context.Context) error {
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
func (c *Cifrado) PedirVerificacion() {
	c.AbrirPanel()
	c.body.Objects = []fyne.CanvasObject{c.parrafo(c.t("matrix.verifyPeopleFirst"))}
	c.footer.Objects = []fyne.CanvasObject{c.boton("action.close", c.pintar), c.boton("matrix.verifyPerson", c.verificarPersona)}
	c.actualizar()
}
