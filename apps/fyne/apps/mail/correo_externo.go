// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// CorreoExterno is a person's remote mailbox, distinct from the one-off importer.
// Implemented by the shared API client on desktop, iOS and Android.
type CorreoExterno interface {
	MailAccounts(context.Context) (api.ExternalMailAccounts, error)
	MailConfigure(context.Context, api.ExternalMailConfiguration) (string, error)
	MailFolders(context.Context, string, string) ([]api.ExternalMailFolder, error)
	MailMessages(context.Context, string, string, string, string) (api.ExternalMailPage, error)
	MailMessage(context.Context, string, string, string, string) (api.ExternalMailMessage, error)
	MailSend(context.Context, string, api.ExternalMailOutgoing) (api.ExternalMailSent, error)
	MailAction(context.Context, string, string, string, string, string, string) error
	MailDisconnect(context.Context, string) error
}

func (b *Buzon) ConCorreoExterno(g CorreoExterno, autorizar func(context.Context, string, string, ...string) (string, error)) {
	b.CorreoExterno, b.AutorizarCorreo = g, autorizar
}

type correoExterno struct {
	b                                                   *Buzon
	ctx                                                 context.Context
	cancelar                                            context.CancelFunc
	d                                                   *ui.Dialogo
	cuerpo, pie                                         *fyne.Container
	cuentas                                             api.ExternalMailAccounts
	cuenta                                              *api.ExternalMailAccount
	carpetas                                            []api.ExternalMailFolder
	mensajes                                            []api.ExternalMailMessage
	mensaje                                             *api.ExternalMailMessage
	proveedor, instancia, carpeta, cursor, smtpSecurity string
	errorClave, aviso                                   string
	paso                                                int
	asistente, ocupado, cerrado, redactando, masLeidos  bool
	version                                             uint64
	parar                                               func()
	campos                                              map[string]*ui.Entrada
	texto                                               *ui.AreaTexto
	adjuntos                                            []api.ExternalMailAttachment
	servicios                                           []string
}

var proveedoresCorreo = []string{"gmail", "microsoft", "yahoo", "imap", "pop3"}

func (b *Buzon) abrirCorreoExterno() {
	if b.CorreoExterno == nil || b.P.Lienzo() == nil {
		return
	}
	if b.cerrarCorreoExterno != nil {
		b.cerrarCorreoExterno()
	}
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelar := context.WithCancel(ctx)
	c := &correoExterno{b: b, ctx: ctx, cancelar: cancelar, proveedor: "gmail", smtpSecurity: "tls", campos: map[string]*ui.Entrada{}}
	for _, key := range []string{"name", "address", "host", "port", "username", "password", "smtpHost", "smtpPort", "smtpUser", "smtpPassword", "to", "cc", "bcc", "subject"} {
		c.campos[key] = ui.NuevaEntrada("")
	}
	c.campos["password"].Contrasena(true)
	c.campos["smtpPassword"].Contrasena(true)
	c.campos["port"].PonerTexto("993")
	c.campos["smtpPort"].PonerTexto("465")
	c.texto = ui.NuevaAreaTexto("")
	c.texto.Filas = 8
	c.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	c.pie = container.New(ui.Fila{Hueco: tema.Espacio(2)})
	c.d = ui.NuevoDialogo(c.t("externalMail.title"), c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.d.RotuloCerrar = c.t("common.close")
	c.d.AlCerrar = c.pedirCerrar
	b.cerrarCorreoExterno = c.cerrar
	c.d.Mostrar(b.P.Lienzo())
	c.cargarCuentas("")
}

func (c *correoExterno) t(key string) string { return c.b.P.T.T(key, nil) }
func (c *correoExterno) parrafo(s string) fyne.CanvasObject {
	return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
}

func (c *correoExterno) boton(key string, f func()) *ui.Boton {
	b := ui.NuevoBoton(c.t(key), f).Con(ui.Secundario, ui.TallaMD)
	b.Inerte = c.ocupado
	return b
}

func (c *correoExterno) campo(key, label string, required bool) fyne.CanvasObject {
	e := c.campos[key]
	e.Rotulo = c.t(label)
	e.Inerte(c.ocupado)
	return ui.GrupoCampo(c.t(label), required, e, "", "")
}

func (c *correoExterno) limpiarClaves() {
	c.campos["password"].PonerTexto("")
	c.campos["smtpPassword"].PonerTexto("")
}

func (c *correoExterno) limpiarBorrador() {
	for _, key := range []string{"to", "cc", "bcc", "subject"} {
		c.campos[key].PonerTexto("")
	}
	c.texto.PonerTexto("")
	clear(c.adjuntos)
	c.adjuntos = nil
	c.redactando = false
}

func (c *correoExterno) hayBorrador() bool {
	return c.texto.Texto() != "" || c.campos["to"].Texto() != "" || c.campos["cc"].Texto() != "" || c.campos["bcc"].Texto() != "" || c.campos["subject"].Texto() != "" || len(c.adjuntos) > 0
}

func (c *correoExterno) salir(f func()) {
	if c.ocupado {
		return
	}
	if !c.hayBorrador() {
		f()
		return
	}
	d := ui.Confirmacion(c.t("externalMail.discardDraft"), c.t("common.cancel"), c.t("common.discard"), false, c.parrafo(c.t("externalMail.draftNote")), nil, func() { c.limpiarBorrador(); f() })
	d.Mostrar(c.b.P.Lienzo())
}
func (c *correoExterno) pedirCerrar() { c.salir(c.cerrar) }
func (c *correoExterno) cerrar() {
	if c.cerrado {
		return
	}
	c.cerrado = true
	c.cancelar()
	if c.parar != nil {
		c.parar()
	}
	c.limpiarClaves()
	c.limpiarBorrador()
	clear(c.mensajes)
	c.mensaje = nil
	c.b.cerrarCorreoExterno = nil
	c.cuerpo.RemoveAll()
	c.d.Cerrar()
}

func pedirCorreo[T any](c *correoExterno, llamada func(context.Context) (T, error), luego func(T, error)) {
	pedirCorreoConPlazo(c, 45*time.Second, llamada, luego)
}

func pedirCorreoConPlazo[T any](c *correoExterno, plazo time.Duration, llamada func(context.Context) (T, error), luego func(T, error)) {
	c.ocupado = true
	c.errorClave = ""
	c.pintar()
	pedir(c.b, func(parent context.Context) (T, error) {
		ctx, cancelar := context.WithTimeout(parent, plazo)
		defer cancelar()
		//nolint:contextcheck // c.ctx derives from b.ctx and also cancels when this dialog closes.
		stop := context.AfterFunc(c.ctx, cancelar)
		defer stop()
		return llamada(ctx)
	}, func(v T, err error) {
		if c.cerrado {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = errorCorreo(err)
		}
		luego(v, err)
		c.pintar()
	})
}

func errorCorreo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Code() {
		case "mail_tls":
			return "externalMail.tlsError"
		case "mail_auth", "external_reconnect_required":
			return "externalMail.authError"
		case "mail_blocked":
			return "externalMail.blockedError"
		case "mail_send_unknown":
			return "externalMail.sendUnknown"
		case "mail_too_large":
			return "externalMail.tooLarge"
		}
	}
	return "externalMail.error"
}

func (c *correoExterno) disponible() bool {
	for _, p := range c.cuentas.Providers {
		if p.Provider == c.proveedor {
			return p.Available
		}
	}
	return false
}

func (c *correoExterno) cargarCuentas(instancia string) {
	pedirCorreo(c, c.b.CorreoExterno.MailAccounts, func(v api.ExternalMailAccounts, err error) {
		if err != nil {
			return
		}
		c.cuentas = v
		if instancia != "" {
			for _, a := range v.Items {
				if a.InstanceID == instancia && a.Connected && (a.Services == nil || slices.Contains(a.Services, "mail")) {
					c.abrirCuenta(a)
					break
				}
			}
		}
	})
}

func (c *correoExterno) abrirCuenta(a api.ExternalMailAccount) {
	if a.Services != nil && !slices.Contains(a.Services, "mail") {
		return
	}
	c.cuenta = &a
	c.asistente = false
	c.instancia = a.InstanceID
	c.carpeta = ""
	c.mensaje = nil
	c.mensajes = nil
	c.masLeidos = false
	c.cargarCarpetas()
}

func (c *correoExterno) reconectar(a api.ExternalMailAccount) {
	c.proveedor, c.instancia, c.asistente, c.paso = a.Provider, a.InstanceID, true, 1
	c.servicios = append([]string(nil), a.Services...)
	if !slices.Contains(c.servicios, "mail") {
		c.servicios = append([]string{"mail"}, c.servicios...)
	}
	c.cuenta = nil
	c.mensaje = nil
	c.mensajes = nil
	c.limpiarClaves()
	c.errorClave = ""
	c.campos["name"].PonerTexto(a.Name)
	c.campos["address"].PonerTexto(a.Address)
	if cfg := a.Configuration; cfg != nil {
		c.campos["host"].PonerTexto(cfg.Incoming.Host)
		c.campos["port"].PonerTexto(strconv.Itoa(cfg.Incoming.Port))
		c.campos["username"].PonerTexto(cfg.Incoming.Username)
		c.campos["smtpHost"].PonerTexto(cfg.Outgoing.Host)
		c.campos["smtpPort"].PonerTexto(strconv.Itoa(cfg.Outgoing.Port))
		c.campos["smtpUser"].PonerTexto(cfg.Outgoing.Username)
		c.smtpSecurity = cfg.Outgoing.Security
	}
	c.pintar()
}

func (c *correoExterno) cargarCarpetas() {
	id := c.instancia
	pedirCorreo(c, func(ctx context.Context) ([]api.ExternalMailFolder, error) {
		return c.b.CorreoExterno.MailFolders(ctx, id, c.cuenta.Address)
	}, func(v []api.ExternalMailFolder, err error) {
		if err != nil || c.instancia != id {
			return
		}
		c.carpetas = v
		if len(v) == 0 {
			return
		}
		c.carpeta = v[0].ID
		for _, f := range v {
			if f.Role == "inbox" {
				c.carpeta = f.ID
				break
			}
		}
		c.cargarMensajes(false)
	})
}

func (c *correoExterno) cargarMensajes(mas bool) {
	if c.carpeta == "" {
		c.cargarCarpetas()
		return
	}
	c.version++
	version := c.version
	id, carpeta, cursor := c.instancia, c.carpeta, ""
	if mas {
		cursor = c.cursor
	}
	pedirCorreo(c, func(ctx context.Context) (api.ExternalMailPage, error) {
		return c.b.CorreoExterno.MailMessages(ctx, id, carpeta, cursor, c.cuenta.Address)
	}, func(v api.ExternalMailPage, err error) {
		if err != nil || c.version != version {
			return
		}
		if !mas {
			c.mensajes = v.Items
			c.masLeidos = false
		} else {
			known := map[string]bool{}
			for _, m := range c.mensajes {
				known[m.ID] = true
			}
			for _, m := range v.Items {
				if !known[m.ID] {
					c.mensajes = append(c.mensajes, m)
					known[m.ID] = true
				}
			}
			c.masLeidos = true
		}
		c.cursor = v.NextCursor
		c.programar()
	})
}

func (c *correoExterno) programar() {
	if c.parar != nil {
		c.parar()
	}
	c.parar = c.b.Esperar(30*time.Second, func() {
		if c.cerrado {
			return
		}
		if !c.ocupado && !c.redactando && c.mensaje == nil && !c.masLeidos && c.cuenta != nil {
			c.cargarMensajes(false)
		} else {
			c.programar()
		}
	})
}

func (c *correoExterno) leer(m api.ExternalMailMessage) {
	id, carpeta, version := c.instancia, c.carpeta, c.version
	pedirCorreo(c, func(ctx context.Context) (api.ExternalMailMessage, error) {
		return c.b.CorreoExterno.MailMessage(ctx, id, carpeta, m.ID, c.cuenta.Address)
	}, func(v api.ExternalMailMessage, err error) {
		if err == nil && c.version == version {
			c.mensaje = &v
		}
	})
}

func (c *correoExterno) accion(action string) {
	if c.mensaje == nil {
		return
	}
	id, folder, message := c.instancia, c.carpeta, c.mensaje.ID
	pedirCorreo(c, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, c.b.CorreoExterno.MailAction(ctx, id, folder, message, action, "", c.cuenta.Address)
	}, func(_ struct{}, err error) {
		if err == nil {
			c.mensaje = nil
			c.cargarMensajes(false)
		}
	})
}

func (c *correoExterno) autorizar() {
	if c.b.AutorizarCorreo == nil || !c.disponible() {
		return
	}
	provider, instance := c.proveedor, c.instancia
	services := append([]string(nil), c.servicios...)
	if len(services) == 0 {
		services = []string{"mail"}
	}
	pedirCorreo(c, func(ctx context.Context) (string, error) {
		return c.b.AutorizarCorreo(ctx, provider, instance, services...)
	}, func(id string, err error) {
		if id != "" {
			c.instancia = id
		}
		if err == nil {
			c.cargarCuentas(id)
		}
	})
}

func (c *correoExterno) configuracion() (api.ExternalMailConfiguration, error) {
	value := func(k string) string { return strings.TrimSpace(c.campos[k].Texto()) }
	address := value("address")
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return api.ExternalMailConfiguration{}, errors.New("address")
	}
	port, err := strconv.Atoi(value("port"))
	if err != nil || port < 1 || port > 65535 {
		return api.ExternalMailConfiguration{}, errors.New("port")
	}
	smtpPort, err := strconv.Atoi(value("smtpPort"))
	if err != nil || smtpPort < 1 || smtpPort > 65535 || value("host") == "" || value("smtpHost") == "" || c.campos["password"].Texto() == "" {
		return api.ExternalMailConfiguration{}, errors.New("server")
	}
	username := value("username")
	if username == "" {
		username = address
	}
	smtpUser := value("smtpUser")
	if smtpUser == "" {
		smtpUser = username
	}
	security := c.smtpSecurity
	return api.ExternalMailConfiguration{Provider: c.proveedor, Name: value("name"), Address: address, Incoming: api.ExternalMailServer{Host: value("host"), Port: port, Username: username, Security: "tls"}, Outgoing: api.ExternalMailServer{Host: value("smtpHost"), Port: smtpPort, Username: smtpUser, Security: security}, Password: c.campos["password"].Texto(), SMTPPassword: c.campos["smtpPassword"].Texto(), InstanceID: c.instancia}, nil
}

func (c *correoExterno) conectar() {
	config, err := c.configuracion()
	if err != nil {
		c.errorClave = "externalMail.invalidConfiguration"
		c.pintar()
		return
	}
	pedirCorreo(c, func(ctx context.Context) (string, error) { return c.b.CorreoExterno.MailConfigure(ctx, config) }, func(id string, err error) {
		if err == nil {
			c.limpiarClaves()
			c.cargarCuentas(id)
		}
	})
}

func direccionesExternas(text string) ([]api.ExternalMailAddress, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	addresses, err := mail.ParseAddressList(strings.ReplaceAll(text, ";", ","))
	if err != nil {
		return nil, err
	}
	out := make([]api.ExternalMailAddress, len(addresses))
	for i, a := range addresses {
		out[i] = api.ExternalMailAddress{Name: a.Name, Email: a.Address}
	}
	return out, nil
}

func (c *correoExterno) enviar() {
	to, err := direccionesExternas(c.campos["to"].Texto())
	if err != nil || len(to) == 0 {
		c.errorClave = "externalMail.invalidRecipients"
		c.pintar()
		return
	}
	cc, err := direccionesExternas(c.campos["cc"].Texto())
	if err != nil {
		c.errorClave = "externalMail.invalidRecipients"
		c.pintar()
		return
	}
	bcc, err := direccionesExternas(c.campos["bcc"].Texto())
	if err != nil {
		c.errorClave = "externalMail.invalidRecipients"
		c.pintar()
		return
	}
	out := api.ExternalMailOutgoing{ExpectedAddress: c.cuenta.Address, To: to, Cc: cc, Bcc: bcc, Subject: c.campos["subject"].Texto(), Text: c.texto.Texto(), Attachments: c.adjuntos}
	id := c.instancia
	pedirCorreo(c, func(ctx context.Context) (api.ExternalMailSent, error) {
		return c.b.CorreoExterno.MailSend(ctx, id, out)
	}, func(v api.ExternalMailSent, err error) {
		if err != nil {
			var e *api.Error
			if !errors.As(err, &e) || e.Status == 0 || e.Status >= 500 {
				c.errorClave = "externalMail.sendUnknown"
			}
			return
		}
		c.limpiarBorrador()
		c.aviso = "externalMail.sent"
		if v.SentCopySaved != nil && !*v.SentCopySaved {
			c.aviso = "externalMail.sentNoCopy"
		}
		c.mensaje = nil
		c.cargarMensajes(false)
	})
}

func (c *correoExterno) adjuntar() {
	if c.b.ElegirFicheros == nil {
		return
	}
	pedirCorreo(c, func(context.Context) ([]api.ExternalMailAttachment, error) {
		paths, err := c.b.ElegirFicheros()
		if err != nil {
			return nil, err
		}
		total := int64(0)
		for _, a := range c.adjuntos {
			total += a.Size
		}
		if total >= 18<<20 {
			return nil, errors.New("size")
		}
		as, err := leerAdjuntosConLimite(paths, (18<<20)-total)
		if err != nil {
			return nil, err
		}

		var out []api.ExternalMailAttachment
		for _, a := range as {
			total += int64(len(a.Datos))
			if total > 18<<20 {
				return nil, errors.New("size")
			}
			out = append(out, api.ExternalMailAttachment{Name: a.Nombre, Type: a.Tipo, Size: int64(len(a.Datos)), ContentBase64: base64.StdEncoding.EncodeToString(a.Datos)})
		}
		return out, nil
	}, func(as []api.ExternalMailAttachment, err error) {
		if err == nil {
			c.adjuntos = append(c.adjuntos, as...)
		} else {
			c.errorClave = "externalMail.tooLarge"
		}
	})
}

func (c *correoExterno) desconectar(a api.ExternalMailAccount) {
	d := ui.Confirmacion(c.t("externalMail.disconnect"), c.t("common.cancel"), c.t("externalMail.disconnect"), true, c.parrafo(c.t("externalMail.disconnectBody")), nil, func() {
		pedirCorreo(c, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, c.b.CorreoExterno.MailDisconnect(ctx, a.InstanceID)
		}, func(_ struct{}, err error) {
			if err == nil {
				c.cargarCuentas("")
			}
		})
	})
	d.Mostrar(c.b.P.Lienzo())
}

func (c *correoExterno) pintar() {
	if c.cerrado {
		return
	}
	c.d.NoDescartable = c.ocupado
	if c.cuenta != nil {
		c.d.Talla = ui.DialogoXL
		c.d.Titulo = cmp.Or(c.cuenta.Address, c.cuenta.Name)
	} else {
		c.d.Talla = ui.DialogoLG
		c.d.Titulo = c.t("externalMail.title")
	}
	c.cuerpo.RemoveAll()
	c.pie.RemoveAll()
	if c.ocupado {
		c.cuerpo.Add(c.parrafo(c.t("common.loading")))
	}
	if c.errorClave != "" {
		c.cuerpo.Add(ui.NuevaAyudaEnLinea(c.t(c.errorClave)).ConTono(ui.AyudaAviso))
	}
	if c.aviso != "" {
		c.cuerpo.Add(ui.NuevaAyudaEnLinea(c.t(c.aviso)).ConTono(ui.AyudaInfo))
	}
	switch {
	case c.cuenta != nil:
		c.pintarBuzon()
	case c.asistente:
		c.pintarAsistente()
	default:
		c.cuerpo.Add(c.parrafo(c.t("externalMail.body")))
		if len(c.cuentas.Items) == 0 {
			c.cuerpo.Add(c.parrafo(c.t("externalMail.empty")))
		}
		for _, a := range c.cuentas.Items {
			c.cuerpo.Add(c.parrafo(a.Name + " · " + a.Address))
			if a.Connected && len(a.Services) > 0 {
				names := make([]string, 0, len(a.Services))
				for _, service := range a.Services {
					names = append(names, c.t("app."+service+".description"))
				}
				c.cuerpo.Add(c.parrafo(c.t("linkedServices.connected") + ": " + strings.Join(names, " · ")))
			}
			if a.Connected && (a.Services == nil || slices.Contains(a.Services, "mail")) {
				c.cuerpo.Add(c.boton("externalMail.open", func() { c.abrirCuenta(a) }))
			}
			c.cuerpo.Add(c.boton("externalMail.connect", func() { c.reconectar(a) }))
			c.cuerpo.Add(c.boton("externalMail.disconnect", func() { c.desconectar(a) }))
		}
		c.pie.Add(c.boton("externalMail.add", func() {
			c.limpiarClaves()
			c.instancia = ""
			c.servicios = []string{"mail"}
			c.asistente = true
			c.paso = 0
			c.errorClave = ""
			c.pintar()
		}))
		c.pie.Add(c.boton("nav.refresh", func() { c.cargarCuentas("") }).Con(ui.Fantasma, ui.TallaMD))
	}
	c.cuerpo.Refresh()
	c.pie.Refresh()
	c.d.Refresh()
	c.d.Recolocar()
}

func (c *correoExterno) pintarAsistente() {
	oauth := c.proveedor != "imap" && c.proveedor != "pop3"
	labels := []string{c.t("externalMail.provider"), c.t("externalMail.account"), c.t("externalMail.review")}
	if oauth {
		labels = []string{c.t("externalMail.provider"), c.t("linkedServices.services")}
	}
	steps := ui.NuevosPasos(labels...)
	steps.Actual = c.paso
	c.cuerpo.Add(steps)
	switch c.paso {
	case 0:
		c.pintarProveedor()
	case 1:
		c.pintarConfiguracion(oauth)
	default:
		c.pintarRevision()
	}
	c.pintarPieAsistente(oauth)
}

func (c *correoExterno) pintarProveedor() {
	names := make([]string, len(proveedoresCorreo))
	for i, p := range proveedoresCorreo {
		names[i] = api.MailProviderNames[p]
	}
	selector := ui.NuevoSelector(names, func(index int) {
		if index < 0 || index >= len(proveedoresCorreo) {
			return
		}
		c.proveedor = proveedoresCorreo[index]
		c.instancia = ""
		c.servicios = []string{"mail"}
		c.limpiarClaves()
		port := "993"
		if c.proveedor == "pop3" {
			port = "995"
		}
		c.campos["port"].PonerTexto(port)
		c.pintar()
	})
	for i, p := range proveedoresCorreo {
		if p == c.proveedor {
			selector.Elegida = i
		}
	}
	selector.Rotulo = c.t("externalMail.provider")
	selector.Inerte(c.ocupado)
	c.cuerpo.Add(selector)
	c.cuerpo.Add(c.parrafo(c.t("externalMail.body")))
}

func (c *correoExterno) pintarConfiguracion(oauth bool) {
	if !c.disponible() {
		c.cuerpo.Add(ui.NuevaAyudaEnLinea(c.t("externalMail.unavailable")).ConTono(ui.AyudaAviso))
	}
	if oauth {
		c.cuerpo.Add(c.parrafo(c.t("externalMail.oauthBody")))
		c.pintarServicios()
	} else {
		c.cuerpo.Add(c.parrafo(c.t("externalMail.tlsBody")))
		for _, f := range []struct {
			k, label string
			required bool
		}{{"name", "common.name", false}, {"address", "common.address", true}, {"host", "externalMail.incomingHost", true}, {"port", "externalMail.port", true}, {"username", "externalMail.username", false}, {"password", "externalMail.password", true}, {"smtpHost", "externalMail.outgoingHost", true}, {"smtpPort", "externalMail.port", true}, {"smtpUser", "externalMail.username", false}, {"smtpPassword", "externalMail.smtpPassword", false}} {
			c.cuerpo.Add(c.campo(f.k, f.label, f.required))
		}
		selector := ui.NuevoSelector([]string{"TLS", "STARTTLS"}, func(index int) {
			port := "465"
			c.smtpSecurity = "tls"
			if index == 1 {
				port = "587"
				c.smtpSecurity = "starttls"
			}
			c.campos["smtpPort"].PonerTexto(port)
		})
		if c.smtpSecurity == "starttls" {
			selector.Elegida = 1
		}
		selector.Rotulo = c.t("externalMail.security")
		selector.Inerte(c.ocupado)
		c.cuerpo.Add(selector)
	}
}

func (c *correoExterno) pintarServicios() {
	c.cuerpo.Add(c.parrafo(c.t("linkedServices.choose")))
	if len(c.servicios) == 0 {
		c.servicios = []string{"mail"}
	}
	capabilities := []api.LinkedServiceCapability{{Service: "mail", Available: true}}
	for _, provider := range c.cuentas.Providers {
		if provider.Provider == c.proveedor && len(provider.Services) > 0 {
			capabilities = provider.Services
			break
		}
	}
	for _, capability := range capabilities {
		choice := ui.NuevaCasilla(c.t("app."+capability.Service+".description"), func(checked bool) {
			if checked {
				if !slices.Contains(c.servicios, capability.Service) {
					c.servicios = append(c.servicios, capability.Service)
				}
			} else {
				c.servicios = slices.DeleteFunc(c.servicios, func(service string) bool { return service == capability.Service })
			}
		})
		choice.Marcada = slices.Contains(c.servicios, capability.Service)
		choice.AnchoEtiqueta = ui.Medir(choice.Etiqueta, tema.TextoBase, ui.Normal).Width
		if c.ocupado || !capability.Available || capability.Service == "mail" {
			choice.Disable()
		}
		c.cuerpo.Add(choice)
		if !capability.Available {
			key := "linkedServices.notSupported"
			if capability.Reason == "workspace_admin_required" {
				key = "linkedServices.keepAdmin"
			}
			c.cuerpo.Add(c.parrafo(c.t(key)))
		}
	}
}

func (c *correoExterno) pintarRevision() {
	c.cuerpo.Add(c.parrafo(c.t("externalMail.testBody")))
	for _, key := range []string{"address", "host", "port", "smtpHost", "smtpPort"} {
		c.cuerpo.Add(c.parrafo(c.campos[key].Texto()))
	}
}

func (c *correoExterno) pintarPieAsistente(oauth bool) {
	c.pie.Add(c.boton("common.cancel", func() { c.limpiarClaves(); c.asistente = false; c.errorClave = ""; c.pintar() }).Con(ui.Fantasma, ui.TallaMD))
	if c.paso > 0 {
		c.pie.Add(c.boton("common.back", func() { c.paso--; c.pintar() }))
	}
	key := "externalMail.review"
	if c.paso == 0 {
		key = "externalMail.account"
		if oauth {
			key = "linkedServices.services"
		}
	}
	if oauth && c.paso == 1 {
		key = "externalMail.authorize"
	}
	if c.paso == 2 {
		key = "externalMail.connect"
	}
	next := c.boton(key, func() { c.avanzarAsistente(oauth) }).Con(ui.Primario, ui.TallaMD)
	next.Inerte = c.ocupado || (c.paso > 0 && !c.disponible())
	c.pie.Add(next)
}

func (c *correoExterno) avanzarAsistente(oauth bool) {
	switch {
	case c.paso == 0:
		c.paso = 1
		c.pintar()
	case oauth:
		c.autorizar()
	case c.paso == 1:
		if _, err := c.configuracion(); err != nil {
			c.errorClave = "externalMail.invalidConfiguration"
		} else {
			c.errorClave = ""
			c.paso = 2
		}
		c.pintar()
	default:
		c.conectar()
	}
}

func (c *correoExterno) pintarBuzon() {
	note := "externalMail.refreshNote"
	if c.cuenta.Provider == "pop3" {
		note = "externalMail.pop3Note"
	}
	c.cuerpo.Add(c.parrafo(c.t(note)))
	if c.errorClave == "externalMail.authError" {
		c.cuerpo.Add(c.boton("externalMail.connect", func() { c.salir(func() { c.reconectar(*c.cuenta) }) }))
	}
	c.pie.Add(c.boton("common.back", func() {
		c.salir(func() {
			c.version++
			c.cuenta = nil
			c.mensaje = nil
			c.mensajes = nil
			c.aviso = ""
			c.pintar()
		})
	}).Con(ui.Fantasma, ui.TallaMD))
	c.pie.Add(c.boton("nav.compose", func() { c.redactando = !c.redactando; c.aviso = ""; c.pintar() }))
	c.pie.Add(c.boton("nav.refresh", func() { c.cargarMensajes(false) }).Con(ui.Fantasma, ui.TallaMD))
	switch {
	case c.redactando:
		c.pintarRedactor()
	case c.mensaje != nil:
		c.pintarMensaje()
	default:
		c.pintarLista()
	}
}

func (c *correoExterno) pintarRedactor() {
	c.cuerpo.Add(c.parrafo(c.t("externalMail.draftNote")))
	for _, f := range []struct{ k, label string }{{"to", "compose.to"}, {"cc", "compose.cc"}, {"bcc", "compose.bcc"}, {"subject", "common.subject"}} {
		c.cuerpo.Add(c.campo(f.k, f.label, f.k == "to"))
	}
	c.texto.Rotulo = c.t("compose.bodyLabel")
	c.texto.Inerte(c.ocupado)
	c.cuerpo.Add(c.texto)
	c.cuerpo.Add(c.boton("externalMail.attach", c.adjuntar))
	for i, a := range c.adjuntos {
		c.cuerpo.Add(c.parrafo(a.Name))
		c.cuerpo.Add(c.boton("common.discard", func() { c.adjuntos = append(c.adjuntos[:i], c.adjuntos[i+1:]...); c.pintar() }))
	}
	c.pie.Add(c.boton("compose.send", c.enviar).Con(ui.Primario, ui.TallaMD))
}

func (c *correoExterno) pintarMensaje() {
	m := c.mensaje
	c.cuerpo.Add(c.boton("common.back", func() { c.mensaje = nil; c.pintar() }))
	c.cuerpo.Add(c.parrafo(m.Subject))
	for _, from := range m.From {
		c.cuerpo.Add(c.parrafo(from.Email))
	}
	text := m.Text
	if text == "" {
		text = htmltext.Text(m.HTML)
	}
	c.cuerpo.Add(c.parrafo(text))
	for _, a := range m.Attachments {
		button := ui.NuevoBoton(a.Name, func() {
			data, err := base64.StdEncoding.DecodeString(a.ContentBase64)
			if err != nil {
				c.errorClave = "externalMail.error"
				c.pintar()
				return
			}
			c.b.Lanzar(func() { c.b.descargar(a.Name, bytes.NewReader(data)) })
		}).Con(ui.Secundario, ui.TallaMD)
		button.Inerte = c.ocupado || a.ContentBase64 == ""
		c.cuerpo.Add(button)
	}
	c.cuerpo.Add(c.boton("compose.replyTitle", func() {
		c.salir(func() {
			if len(m.From) > 0 {
				c.campos["to"].PonerTexto(m.From[0].Email)
			}
			subject := m.Subject
			if !strings.HasPrefix(strings.ToLower(subject), "re:") {
				subject = "Re: " + subject
			}
			c.campos["subject"].PonerTexto(subject)
			c.redactando = true
			c.pintar()
		})
	}))
	if c.cuenta.Capabilities.Actions {
		read, key := "read", "externalMail.markRead"
		if m.Seen {
			read, key = "unread", "externalMail.markUnread"
		}
		c.cuerpo.Add(c.boton(key, func() { c.accion(read) }))
		flag, key := "flag", "externalMail.flag"
		if m.Flagged {
			flag, key = "unflag", "externalMail.unflag"
		}
		c.cuerpo.Add(c.boton(key, func() { c.accion(flag) }))
		c.cuerpo.Add(c.boton("externalMail.trash", func() { c.accion("trash") }))
	}
}

func (c *correoExterno) pintarLista() {
	names := make([]string, len(c.carpetas))
	for i, f := range c.carpetas {
		names[i] = f.Name
	}
	selector := ui.NuevoSelector(names, func(index int) {
		if index < 0 || index >= len(c.carpetas) {
			return
		}
		c.carpeta = c.carpetas[index].ID
		c.mensaje = nil
		c.cargarMensajes(false)
	})
	for i, f := range c.carpetas {
		if f.ID == c.carpeta {
			selector.Elegida = i
		}
	}
	selector.Rotulo = c.t("nav.folders")
	selector.Inerte(c.ocupado)
	c.cuerpo.Add(selector)
	if len(c.mensajes) == 0 {
		c.cuerpo.Add(c.parrafo(c.t("externalMail.noMessages")))
	}
	for _, m := range c.mensajes {
		subject := m.Subject
		if subject == "" {
			subject = c.t("common.noSubject")
		}
		button := ui.NuevoBoton(subject, func() { c.leer(m) }).Con(ui.Fantasma, ui.TallaMD)
		button.Inerte = c.ocupado
		c.cuerpo.Add(button)
		c.cuerpo.Add(c.parrafo(m.Snippet))
	}
	if c.cursor != "" {
		c.cuerpo.Add(c.boton("common.loadMore", func() { c.cargarMensajes(true) }))
	}
}
