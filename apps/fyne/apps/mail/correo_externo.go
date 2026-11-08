// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// CorreoExterno is a person's remote mailbox, distinct from the one-off importer.
// Implemented by the shared API client on desktop, iOS and Android.
type CorreoExterno interface {
	MailAccounts(context.Context) (api.ExternalMailAccounts, error)
	MailSummary(context.Context) ([]api.ExternalMailAccountSummary, error)
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

// correoExterno es el diálogo de las cuentas externas: la lista y el asistente de
// alta. Ya no lleva buzón dentro: «Abrir buzón» y la cuenta recién conectada se abren
// en el panel principal (vista_externa.go), con la Bandeja y el Lector de siempre.
type correoExterno struct {
	b                                  *Buzon
	ctx                                context.Context
	cancelar                           context.CancelFunc
	d                                  *ui.Dialogo
	cuerpo, pie                        *fyne.Container
	cuentas                            api.ExternalMailAccounts
	proveedor, instancia, smtpSecurity string
	errorClave, aviso                  string
	paso                               int
	asistente, ocupado, cerrado        bool
	campos                             map[string]*ui.Entrada
	servicios                          []string
	avanzado                           bool
	// icloud es el preajuste de iCloud sobre «imap»: servidores fijos y solo la
	// dirección y la contraseña de app a la vista.
	icloud bool
	// foco es la cuenta cuyo botón recibe el foco al pintar la lista: la que ya
	// estaba tras un «ya conectada», para que el teclado no quede en el botón
	// «Conectar» que desaparece (en la web, focusAfterList). Se olvida al usar la lista.
	foco string
	// autorizando corta el inicio de sesión en el proveedor que está en curso: con
	// diez minutos de plazo, «Cancelar» y el aspa no pueden quedarse inertes.
	autorizando context.CancelFunc
}

var (
	// plazoCorreo es lo que espera cada llamada al buzón remoto.
	plazoCorreo = 45 * time.Second
	// plazoAutorizar es el del inicio de sesión en el proveedor: la persona está en
	// el navegador con su contraseña o su segundo factor, y los 45 s de una llamada
	// cortaban el OAuth a medias y descartaban la vuelta (B1). Es el mismo de
	// vinculados y de sesion.autorizarCuenta, que cuelga de este contexto.
	plazoAutorizar = 10 * time.Minute
)

// eleccionesCorreo son las tarjetas del asistente: los proveedores, con iCloud
// como preajuste de IMAP (lo mismo que CHOICES en la web).
var eleccionesCorreo = []string{"gmail", "microsoft", "yahoo", "icloud", "imap", "pop3"}

func nombreEleccion(e string) string {
	if e == "icloud" {
		return api.ICloudMail.Name
	}
	return api.MailProviderNames[e]
}

// ponerICloud rellena los servidores fijos de iCloud, con la dirección como
// usuario de los dos.
func (c *correoExterno) ponerICloud() {
	c.campos["host"].PonerTexto(api.ICloudMail.Incoming.Host)
	c.campos["port"].PonerTexto(strconv.Itoa(api.ICloudMail.Incoming.Port))
	c.campos["smtpHost"].PonerTexto(api.ICloudMail.Outgoing.Host)
	c.campos["smtpPort"].PonerTexto(strconv.Itoa(api.ICloudMail.Outgoing.Port))
	c.campos["username"].PonerTexto("")
	c.campos["smtpUser"].PonerTexto("")
	c.smtpSecurity = api.ICloudMail.Outgoing.Security
	c.avanzado = false
}

// cargarExternas pide el resumen de las cuentas externas (`GET /external-mail/summary`):
// cada cuenta con su estado y sus carpetas, que es lo que pinta el lateral al estilo de
// Apple Mail. Si no carga, la sección lo dice y deja reintentar: antes el fallo se
// tragaba y la sección se quedaba vacía sin decir por qué (B4).
func (b *Buzon) cargarExternas() {
	if b.CorreoExterno == nil {
		return
	}
	if b.P.Menu.EstadoExternas == Error {
		b.P.Menu.EstadoExternas = Cargando
		b.P.Refrescar()
	}
	b.cargandoExternas = true
	pedir(b, b.CorreoExterno.MailSummary, func(v []api.ExternalMailAccountSummary, err error) {
		b.cargandoExternas = false
		pendiente := b.externasPendientes
		b.externasPendientes = nil
		if err != nil {
			b.P.Menu.EstadoExternas = Error
			b.P.Refrescar()
			return
		}
		if pendiente != nil {
			// Una cuenta que el diálogo quitó mientras este resumen iba de camino no vuelve.
			v = slices.DeleteFunc(slices.Clone(v), func(c api.ExternalMailAccountSummary) bool { return !listada(pendiente.Items, c.InstanceID) })
		}
		b.resumenExternas = v
		b.P.Menu.EstadoExternas = Listo
		b.pintarExternas()
		if pendiente != nil && resumenCaducado(v, pendiente.Items) {
			b.cargarExternas()
		}
	})
}

// soloBuzones deja las cuentas que Mail abre: el endpoint es compartido y lista
// también las conectadas solo para Calendario, Contactos o Notas (B12).
func soloBuzones(v api.ExternalMailAccounts) api.ExternalMailAccounts {
	v.Items = slices.DeleteFunc(slices.Clone(v.Items), func(a api.ExternalMailAccount) bool { return !a.ServesMail() })
	return v
}

func listada(items []api.ExternalMailAccount, id string) bool {
	return slices.ContainsFunc(items, func(a api.ExternalMailAccount) bool { return a.InstanceID == id })
}

// resumenCaducado dice si la lista del diálogo trae una cuenta con buzón que el
// resumen no tiene, o que se reconectó o se reconfiguró (otra conexión u otra
// dirección): entonces hay que volver a pedirlo (`staleIn` en la web).
func resumenCaducado(resumen []api.ExternalMailAccountSummary, items []api.ExternalMailAccount) bool {
	for _, a := range items {
		if !a.ServesMail() {
			continue
		}
		if !slices.ContainsFunc(resumen, func(c api.ExternalMailAccountSummary) bool {
			return c.InstanceID == a.InstanceID && c.Connected == a.Connected && c.Address == a.Address
		}) {
			return true
		}
	}
	return false
}

// fijarExternas recibe la lista que acaba de cargar el diálogo: al volver del
// proveedor y al conectar o desconectar, el lateral cambia sin esperar a que se
// cierre. Lo que se fue sale ya; lo nuevo o cambiado vuelve a pedir el resumen, salvo
// que el que está en camino ya lo traiga (la vuelta del proveedor).
func (b *Buzon) fijarExternas(v api.ExternalMailAccounts) {
	b.cuentasDialogo = slices.Clone(v.Items)
	b.resumenExternas = slices.DeleteFunc(slices.Clone(b.resumenExternas), func(c api.ExternalMailAccountSummary) bool { return !listada(v.Items, c.InstanceID) })
	// La cuenta a la vista se desconectó: se vuelve a la Entrada de Sytue.
	if e := b.externa; e != nil && !e.Mezclada() && !listada(v.Items, e.Cuenta) {
		if c, hay := b.carpetaDeRol("inbox"); hay {
			b.ElegirCarpeta(c)
		}
	}
	b.pintarExternas()
	switch {
	case b.cargandoExternas:
		b.externasPendientes = &v
	case resumenCaducado(b.resumenExternas, v.Items):
		b.cargarExternas()
	}
}

// pintarExternas pone en el lateral el árbol de buzones y lo repinta.
func (b *Buzon) pintarExternas() {
	b.ponerArbol()
	b.P.Refrescar()
}

// ponerArbol arma el árbol de buzones del lateral (`arbolDeBuzones`) con las carpetas
// de Sytue y el resumen de las cuentas externas, también las que piden volver a iniciar
// sesión. No repinta: lo llaman también las carpetas de Sytue, que repintan ellas.
func (b *Buzon) ponerArbol() {
	arbol := arbolDeBuzones(b.P.Locale, b.carpetas, b.resumenExternas)
	b.P.Menu.Unificados, b.P.Menu.Cuentas, b.P.Menu.TituloSytue = arbol.Unificados, arbol.Cuentas, b.principal
}

// plegarRama pliega o despliega una rama del árbol del lateral.
func (b *Buzon) plegarRama(clave string) {
	if b.P.Menu.Ramas == nil {
		b.P.Menu.Ramas = map[string]bool{}
	}
	b.P.Menu.Ramas[clave] = !b.P.Menu.Ramas[clave]
	b.P.Refrescar()
}

func (b *Buzon) abrirCorreoExterno() { b.abrirCorreoExternoEn("") }

// abrirCorreoExternoEn abre el diálogo y, si id no está vacío, el inicio de sesión de
// esa cuenta («Requiere volver a iniciar sesión» del lateral, o la franja de una
// carpeta que el proveedor ya no deja leer). Pide las cuentas una sola vez (B9).
func (b *Buzon) abrirCorreoExternoEn(id string) {
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
	for _, key := range []string{"name", "address", "host", "port", "username", "password", "smtpHost", "smtpPort", "smtpUser", "smtpPassword"} {
		c.campos[key] = ui.NuevaEntrada("")
	}
	c.campos["password"].Contrasena(true)
	c.campos["smtpPassword"].Contrasena(true)
	c.limpiarConfiguracion()
	c.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	c.pie = container.New(ui.Fila{Hueco: tema.Espacio(2)})
	c.d = ui.NuevoDialogo(c.t("externalMail.title"), c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.d.RotuloCerrar = c.t("common.close")
	c.d.AlCerrar = c.pedirCerrar
	b.cerrarCorreoExterno = c.cerrar
	b.correoAbierto = c
	c.d.Mostrar(b.P.Lienzo())
	c.cargarCuentas("", id)
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

func (c *correoExterno) limpiarConfiguracion() {
	for _, key := range []string{"name", "address", "host", "username", "password", "smtpHost", "smtpUser", "smtpPassword"} {
		c.campos[key].PonerTexto("")
	}
	port := "993"
	if c.proveedor == "pop3" {
		port = "995"
	}
	c.campos["port"].PonerTexto(port)
	c.campos["smtpPort"].PonerTexto("465")
	c.smtpSecurity = "tls"
	c.avanzado = false
}

// pedirCerrar es el aspa, Escape o «Cerrar»; mientras se espera al navegador también
// vale, y corta esa espera. No vuelve a pedir el resumen: el diálogo ya no lee correo,
// y lo que conecta o desconecta ya se lo cuenta al lateral (fijarExternas).
func (c *correoExterno) pedirCerrar() {
	if c.ocupado && c.autorizando == nil {
		return
	}
	c.cerrar()
}

func (c *correoExterno) cerrar() {
	if c.cerrado {
		return
	}
	c.cerrado = true
	c.cancelar()
	c.autorizando = nil
	c.limpiarClaves()
	c.b.cerrarCorreoExterno = nil
	if c.b.correoAbierto == c {
		c.b.correoAbierto = nil
	}
	c.cuerpo.RemoveAll()
	c.d.Cerrar()
}

func pedirCorreo[T any](c *correoExterno, llamada func(context.Context) (T, error), luego func(T, error)) {
	pedirCorreoConPlazo(c, plazoCorreo, llamada, luego)
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
			c.errorClave = errorCorreo(err, c.enICloud())
		}
		luego(v, err)
		c.pintar()
	})
}

// errorCorreo es el aviso de una llamada al buzón remoto, por el `code` de la API.
// iCloud no acepta la contraseña del Apple ID: su LOGIN rechazado pide la
// contraseña específica de app, como errorKey(err, icloud) en la web.
func errorCorreo(err error, icloud bool) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Code() {
		case "mail_tls":
			return "externalMail.tlsError"
		case "mail_auth":
			if icloud {
				return "externalMail.icloudAuthError"
			}
			return "externalMail.authError"
		case "external_reconnect_required":
			return "externalMail.authError"
		case "mail_blocked":
			return "externalMail.blockedError"
		case "mail_send_unknown":
			return "externalMail.sendUnknown"
		case "mail_too_large":
			return "externalMail.tooLarge"
		case "mail_account_exists":
			return "externalMail.exists"
		}
	}
	return "externalMail.error"
}

// errorAutorizacion distingue lo que la persona puede arreglar al volver del
// proveedor: cancelar, desmarcar el permiso de correo o dejar caducar el inicio
// de sesión. Antes todo salía como el genérico «no se pudo acceder al buzón».
func errorAutorizacion(err error) string {
	switch {
	case errors.Is(err, api.ErrOAuthDenied), errors.Is(err, context.Canceled):
		return "externalMail.cancelled"
	case errors.Is(err, api.ErrOAuthScope):
		return "externalMail.scopeMissing"
	case errors.Is(err, api.ErrOAuthState), errors.Is(err, context.DeadlineExceeded):
		return "externalMail.timeout"
	}
	return errorCorreo(err, false)
}

// enICloud dice si lo que está a la vista es el formulario de iCloud del asistente.
func (c *correoExterno) enICloud() bool { return c.asistente && c.icloud }

// yaConectada lleva a la lista, con su aviso, cuando la cuenta ya estaba: no es un
// fallo que reintentar sino una cuenta que ya se puede abrir.
func (c *correoExterno) yaConectada(instancia string) {
	c.instancia, c.errorClave, c.aviso, c.asistente = "", "", "externalMail.exists", false
	c.foco = instancia
	c.limpiarClaves()
	c.cargarCuentas("", "")
}

func (c *correoExterno) disponible() bool {
	for _, p := range c.cuentas.Providers {
		if p.Provider == c.proveedor {
			return p.Available
		}
	}
	return false
}

// cargarCuentas pide la lista. Con `abrir`, la cuenta que acaba de conectarse se
// abre en el panel principal si ya está conectada, y se avisa (fase B, punto 7: el
// diálogo se ha ido y la persona tiene que saber qué pasó); con `reconectar`, el
// asistente arranca en el inicio de sesión de esa cuenta.
func (c *correoExterno) cargarCuentas(abrir, reconectar string) {
	pedirCorreo(c, c.b.CorreoExterno.MailAccounts, func(v api.ExternalMailAccounts, err error) {
		if err != nil {
			return
		}
		c.cuentas = soloBuzones(v)
		c.b.fijarExternas(c.cuentas)
		for _, a := range c.cuentas.Items {
			switch a.InstanceID {
			case abrir:
				if a.Connected {
					p := c.b.P
					c.abrirCuenta(a)
					cuenta := a.Address
					if cuenta == "" {
						cuenta = a.Name
					}
					p.Avisar(ui.AvisoExito, p.T.T("externalMail.connected", i18n.Vars{"account": cuenta}), "")
					return
				}
			case reconectar:
				c.reconectar(a)
				return
			}
		}
	})
}

// abrirCuenta es «Abrir buzón»: su Entrada en el panel principal, y el diálogo se
// cierra. Una cuenta conectada solo para Calendario o Contactos no tiene buzón.
func (c *correoExterno) abrirCuenta(a api.ExternalMailAccount) {
	if !a.ServesMail() {
		return
	}
	// El aviso («ya conectada») era de la lista: abrir el buzón lo responde.
	c.aviso = ""
	b := c.b
	c.cerrar()
	// Su rama se despliega, aunque se hubiera plegado: la Entrada que se va a elegir
	// tiene que verse elegida (`reveal` de ExternalAccountsSection).
	delete(b.P.Menu.Ramas, claveCuenta(a.InstanceID))
	b.ElegirExterna(a.InstanceID, "")
}

func (c *correoExterno) reconectar(a api.ExternalMailAccount) {
	c.proveedor, c.instancia, c.asistente, c.paso = a.Provider, a.InstanceID, true, 1
	c.icloud = a.IsICloud()
	c.limpiarConfiguracion()
	c.servicios = append([]string(nil), a.Services...)
	if !slices.Contains(c.servicios, "mail") {
		c.servicios = append([]string{"mail"}, c.servicios...)
	}
	c.errorClave, c.aviso = "", ""
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
		if c.icloud {
			// En iCloud el usuario solo se guarda si no es la dirección: si no, sigue a
			// la dirección, también si se edita ahora.
			if strings.EqualFold(cfg.Incoming.Username, a.Address) {
				c.campos["username"].PonerTexto("")
			}
			c.campos["smtpUser"].PonerTexto("")
			c.avanzado = c.campos["username"].Texto() != ""
		}
	}
	c.pintar()
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
	espera, cortar := context.WithCancel(c.ctx)
	c.autorizando = cortar
	pedirCorreoConPlazo(c, plazoAutorizar, func(ctx context.Context) (string, error) {
		ctx, cancelar := context.WithCancel(ctx)
		defer cancelar()
		stop := context.AfterFunc(espera, cancelar)
		defer stop()
		return c.b.AutorizarCorreo(ctx, provider, instance, services...)
	}, func(id string, err error) {
		cortar()
		c.autorizando = nil
		var exists *api.OAuthExistsError
		switch {
		case errors.As(err, &exists):
			// El borrador se descartó en el servidor: no se reintenta con él.
			c.yaConectada(exists.Instance)
			return
		case err != nil:
			c.errorClave = errorAutorizacion(err)
		}
		if id != "" {
			c.instancia = id
		}
		if err == nil {
			c.cargarCuentas(id, "")
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
	if smtpUser == "" || c.icloud {
		// iCloud entra en los dos servidores con la misma cuenta.
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
		if c.errorClave == "externalMail.exists" {
			var e *api.Error
			instancia := ""
			if errors.As(err, &e) {
				instancia = e.Param("instance_id")
			}
			c.yaConectada(instancia)
			return
		}
		if err == nil {
			c.limpiarClaves()
			c.cargarCuentas(id, "")
		}
	})
}

func (c *correoExterno) desconectar(a api.ExternalMailAccount) {
	d := ui.Confirmacion(c.t("externalMail.disconnect"), c.t("common.cancel"), c.t("externalMail.disconnect"), true, c.parrafo(c.t("externalMail.disconnectBody")), nil, func() {
		pedirCorreo(c, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, c.b.CorreoExterno.MailDisconnect(ctx, a.InstanceID)
		}, func(_ struct{}, err error) {
			if err == nil {
				c.cargarCuentas("", "")
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
	var objetivo fyne.Focusable
	if c.asistente {
		c.foco = ""
		c.pintarAsistente()
	} else {
		objetivo = c.pintarCuentas()
	}
	c.cuerpo.Refresh()
	c.pie.Refresh()
	c.d.Refresh()
	c.d.Recolocar()
	// Cada repintado crea botones nuevos: mientras haya foco pendiente se le vuelve a dar.
	if objetivo != nil && !c.ocupado {
		if lienzo := c.b.P.Lienzo(); lienzo != nil {
			lienzo.Focus(objetivo)
		}
	}
}

// pintarCuentas pinta la lista de cuentas y devuelve el botón que debe llevarse el
// foco pendiente (c.foco), o nil.
func (c *correoExterno) pintarCuentas() fyne.Focusable {
	var objetivo fyne.Focusable
	// usar es la acción de un botón de la lista: tocarla responde al foco pendiente.
	usar := func(f func()) func() {
		return func() { c.foco = ""; f() }
	}
	c.cuerpo.Add(c.parrafo(c.t("externalMail.body")))
	if len(c.cuentas.Items) == 0 {
		c.cuerpo.Add(c.parrafo(c.t("externalMail.empty")))
	}
	for _, a := range c.cuentas.Items {
		c.cuerpo.Add(c.parrafo(a.Name))
		c.cuerpo.Add(c.parrafo(strings.Join(slices.DeleteFunc([]string{a.Address, a.ProviderName()}, func(s string) bool { return s == "" }), " · ")))
		if a.Connected && len(a.Services) > 0 {
			names := make([]string, 0, len(a.Services))
			for _, service := range a.Services {
				names = append(names, c.t("app."+service+".description"))
			}
			c.cuerpo.Add(c.parrafo(c.t("linkedServices.connected") + ": " + strings.Join(names, " · ")))
		}
		conectar := c.boton("externalMail.connect", usar(func() { c.reconectar(a) }))
		if a.Connected {
			abrir := c.boton("externalMail.open", usar(func() { c.abrirCuenta(a) }))
			c.cuerpo.Add(abrir)
			if a.InstanceID == c.foco {
				objetivo = abrir
			}
		} else if a.InstanceID == c.foco {
			objetivo = conectar
		}
		c.cuerpo.Add(conectar)
		c.cuerpo.Add(c.boton("externalMail.disconnect", usar(func() { c.desconectar(a) })))
	}
	c.pie.Add(c.boton("externalMail.add", usar(func() {
		c.limpiarConfiguracion()
		c.proveedor = "gmail"
		c.icloud = false
		c.instancia = ""
		c.servicios = []string{"mail"}
		c.avanzado = false
		c.asistente = true
		c.paso = 0
		c.errorClave, c.aviso = "", ""
		c.pintar()
	})))
	c.pie.Add(c.boton("nav.refresh", usar(func() { c.cargarCuentas("", "") })).Con(ui.Fantasma, ui.TallaMD))
	return objetivo
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
	names := make([]string, len(eleccionesCorreo))
	for i, e := range eleccionesCorreo {
		names[i] = nombreEleccion(e)
	}
	selector := ui.NuevoSelector(names, func(index int) {
		if index < 0 || index >= len(eleccionesCorreo) {
			return
		}
		c.elegirProveedor(eleccionesCorreo[index])
		c.pintar()
	})
	actual := c.proveedor
	if c.icloud {
		actual = "icloud"
	}
	selector.Elegida = slices.Index(eleccionesCorreo, actual)
	selector.Rotulo = c.t("externalMail.provider")
	selector.Inerte(c.ocupado)
	c.cuerpo.Add(selector)
	c.cuerpo.Add(c.parrafo(c.t("externalMail.body")))
}

// elegirProveedor cambia de tarjeta en el asistente. Salir de iCloud no deja sus
// servidores en el formulario genérico.
func (c *correoExterno) elegirProveedor(e string) {
	antes := c.icloud
	c.icloud = e == "icloud"
	c.proveedor = e
	if c.icloud {
		c.proveedor = "imap"
	}
	c.avanzado = false
	c.instancia = ""
	c.servicios = []string{"mail"}
	c.limpiarClaves()
	switch {
	case c.icloud:
		c.ponerICloud()
		return
	case antes:
		for _, key := range []string{"host", "username", "smtpHost", "smtpUser"} {
			c.campos[key].PonerTexto("")
		}
		c.campos["smtpPort"].PonerTexto("465")
		c.smtpSecurity = "tls"
	}
	port := "993"
	if c.proveedor == "pop3" {
		port = "995"
	}
	c.campos["port"].PonerTexto(port)
}

func (c *correoExterno) pintarConfiguracion(oauth bool) {
	if !c.disponible() {
		c.cuerpo.Add(ui.NuevaAyudaEnLinea(c.t("externalMail.unavailable")).ConTono(ui.AyudaAviso))
	}
	switch {
	case oauth:
		c.cuerpo.Add(c.parrafo(c.t("externalMail.oauthBody")))
		c.pintarServicios()
	case c.icloud:
		c.cuerpo.Add(c.parrafo(c.t("externalMail.icloudBody")))
		c.cuerpo.Add(c.campo("name", "common.name", false))
		c.cuerpo.Add(c.campo("address", "common.address", true))
		c.cuerpo.Add(c.campo("password", "externalMail.password", true))
		// Donde se crea la contraseña de app: el mismo enlace que la web.
		c.cuerpo.Add(ui.NuevoEnlace(c.t("externalMail.icloudLink"), tema.TextoSM, ui.Medio, "primary-text", func() {
			if c.b.Abridor != nil {
				_ = api.AbrirFuera(c.b.Abridor, api.ICloudMail.AccountPage)
			}
		}))
		// Un dominio propio de iCloud+ o un alias entra con la dirección principal de
		// iCloud: vacío, el usuario es la dirección de arriba.
		c.plegable(container.New(ui.Columna{Hueco: tema.Espacio(3)}, c.campo("username", "externalMail.username", false)))
	default:
		c.cuerpo.Add(c.parrafo(c.t("externalMail.tlsBody")))
		for _, f := range []struct {
			k, label string
			required bool
		}{{"name", "common.name", false}, {"address", "common.address", true}, {"host", "externalMail.incomingHost", true}, {"password", "externalMail.password", true}, {"smtpHost", "externalMail.outgoingHost", true}} {
			c.cuerpo.Add(c.campo(f.k, f.label, f.required))
		}
		detalles := container.New(ui.Columna{Hueco: tema.Espacio(3)})
		for i, f := range []struct{ k, label string }{{"port", "externalMail.port"}, {"username", "externalMail.username"}, {"smtpPort", "externalMail.port"}, {"smtpUser", "externalMail.username"}, {"smtpPassword", "externalMail.smtpPassword"}} {
			switch i {
			case 0:
				detalles.Add(c.parrafo(c.t("externalMail.incoming")))
			case 2:
				detalles.Add(c.parrafo(c.t("externalMail.outgoing")))
			}
			detalles.Add(c.campo(f.k, f.label, f.k == "port" || f.k == "smtpPort"))
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
		detalles.Add(selector)
		c.plegable(detalles)
	}
}

// plegable añade «Avanzado» y, debajo, lo que despliega.
func (c *correoExterno) plegable(detalles *fyne.Container) {
	var opciones *ui.Boton
	opciones = c.boton("externalMail.advanced", func() {
		c.avanzado = !c.avanzado
		opciones.Icono = iconos.ChevronDerecha
		if c.avanzado {
			detalles.Show()
			opciones.Icono = iconos.ChevronAbajo
		} else {
			detalles.Hide()
		}
		opciones.Refresh()
		c.cuerpo.Refresh()
		c.d.Refresh()
		c.d.Recolocar()
	}).Con(ui.Fantasma, ui.TallaMD)
	opciones.Icono = iconos.ChevronDerecha
	if c.avanzado {
		opciones.Icono = iconos.ChevronAbajo
	} else {
		detalles.Hide()
	}
	c.cuerpo.Add(opciones)
	c.cuerpo.Add(detalles)
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
	cancelar := c.boton("common.cancel", func() {
		if c.autorizando != nil {
			// Deja de esperar al navegador y se queda en el paso: «Autorizar» lo reintenta
			// con el mismo borrador.
			c.autorizando()
			return
		}
		c.limpiarClaves()
		c.asistente = false
		c.errorClave = ""
		c.pintar()
	}).Con(ui.Fantasma, ui.TallaMD)
	cancelar.Inerte = c.ocupado && c.autorizando == nil
	c.pie.Add(cancelar)
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
			c.avanzado = true
		} else {
			c.errorClave = ""
			c.paso = 2
		}
		c.pintar()
	default:
		c.conectar()
	}
}
