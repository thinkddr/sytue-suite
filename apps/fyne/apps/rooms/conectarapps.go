// SPDX-License-Identifier: Apache-2.0
package rooms

import (
	"context"
	"errors"
	"net/url"
	"regexp"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

type integradorOAuth interface {
	IntegrationDestinations(context.Context, string) ([]api.RoomsIntegrationDestination, error)
	ConnectOAuthBridge(context.Context, string, string, string, string) (api.RoomsBridge, error)
	ConnectWhatsAppConsent(context.Context, api.RoomsWhatsAppConsent) (api.RoomsWhatsApp, error)
}

var proveedoresRooms = []struct{ clave, nombre, plataforma string }{
	{"slack-rooms", "Slack", "slack"},
	{"microsoft-teams-rooms", "Microsoft Teams", "teams"},
	{"google-chat", "Google Chat", "googlechat"},
	{"whatsapp-business", "WhatsApp Business", ""},
}

type conexionRooms struct {
	sa                                                              *Sala
	api                                                             integradorOAuth
	ctx                                                             context.Context
	cancel                                                          context.CancelFunc
	d                                                               *ui.Dialogo
	lienzo                                                          fyne.Canvas
	cuerpo, pie                                                     *fyne.Container
	pasos                                                           *ui.Pasos
	proveedor, elegida, instancia, canal, destino, equipo, telefono string
	cuentas                                                         []api.OAuthInstance
	proveedores                                                     []api.OAuthTemplate
	equipos                                                         []api.RoomsTeam
	destinos                                                        []api.RoomsIntegrationDestination
	numeros                                                         []api.WhatsAppNumber
	paso                                                            int
	ocupado, cargando, cerrada, confirmar, metaAbierto              bool
	errorClave                                                      string
	pendiente, esperada                                             string
	pin                                                             *ui.Entrada
	siguiente                                                       *ui.Boton
}

// AbrirConectarApp keeps configuration in Fyne; only provider consent uses the system browser.
func (sa *Sala) AbrirConectarApp(proveedor string) {
	in, ok := sa.S.(integradorOAuth)
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if !ok || lienzo == nil || sa.CuentasApps == nil || !sa.AdminOrg {
		sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("appConnection.unavailable", nil))
		return
	}
	conocido := proveedor == ""
	for _, p := range proveedoresRooms {
		if p.clave == proveedor {
			conocido = true
		}
	}
	if !conocido {
		proveedor = ""
	}
	if sa.cerrarConexionApps != nil {
		sa.cerrarConexionApps()
	}
	ctx := sa.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &conexionRooms{sa: sa, api: in, ctx: ctx, cancel: cancel, lienzo: lienzo, proveedor: proveedor, elegida: proveedor, canal: sa.abierto, cargando: true}
	c.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(4)})
	c.pie = container.New(ui.Fila{Hueco: tema.Espacio(2)})
	c.pin = ui.NuevaEntrada("")
	c.pin.Contrasena(true)
	c.pin.AlCambiar(func(string) { c.actualizarBoton() })
	c.d = ui.NuevoDialogo(sa.P.T.T("integrations.connectApps.title", nil), c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.d.RotuloCerrar = sa.P.T.T("action.close", nil)
	c.d.AlCerrar = c.cerrar
	sa.cerrarConexionApps = c.cerrar
	if proveedor != "" && proveedor != "whatsapp-business" {
		c.paso = 1
	}
	sa.comprobarWhatsApp = func(instance string) {
		if !c.cerrada && c.instancia == instance && c.esWhatsApp() {
			c.comprobarNumeros()
		}
	}
	c.pintar()
	c.d.Mostrar(lienzo)
	c.cargar()
}

func (c *conexionRooms) cerrar() {
	if c.cerrada {
		return
	}
	c.cerrada = true
	c.cancel()
	c.pin.PonerTexto("")
	c.sa.comprobarWhatsApp = nil
	c.sa.cerrarConexionApps = nil
	c.d.Cerrar()
}

func (c *conexionRooms) esWhatsApp() bool { return c.proveedor == "whatsapp-business" }
func (c *conexionRooms) nombre() string {
	for _, p := range proveedoresRooms {
		if p.clave == c.proveedor {
			return p.nombre
		}
	}
	return ""
}

func (c *conexionRooms) cuenta() *api.OAuthInstance {
	for idx := range c.cuentas {
		if c.cuentas[idx].ID == c.instancia && c.cuentas[idx].TemplateKey == c.proveedor && c.cuentas[idx].Active() {
			return &c.cuentas[idx]
		}
	}
	return nil
}

func (c *conexionRooms) ir(paso int) { c.paso, c.errorClave, c.confirmar = paso, "", false; c.pintar() }

func (c *conexionRooms) cargar() {
	c.cargando, c.errorClave = true, ""
	c.pintar()
	type datos struct {
		cuentas     []api.OAuthInstance
		proveedores []api.OAuthTemplate
		equipos     []api.RoomsTeam
	}
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (datos, error) {
		cuentas, err := c.sa.CuentasApps.OAuthInstances(ctx)
		if err != nil {
			return datos{}, err
		}
		proveedores, err := c.sa.CuentasApps.OAuthTemplates(ctx)
		if err != nil {
			return datos{}, err
		}
		equipos, err := c.sa.S.Teams(ctx)
		return datos{cuentas: cuentas, proveedores: proveedores, equipos: equipos}, err
	}, func(d datos, err error) {
		if c.cerrada {
			return
		}
		c.cargando = false
		if err != nil {
			c.errorClave = "appConnection.failed"
			c.pintar()
			return
		}
		c.cuentas, c.equipos = d.cuentas, d.equipos
		c.proveedores = d.proveedores
		if c.equipo == "" {
			for _, equipo := range c.equipos {
				if equipo.ID == c.sa.equipo {
					c.equipo = equipo.ID
				}
			}
		}
		if c.proveedor != "" && !c.esWhatsApp() {
			c.elegirProveedor()
		} else {
			c.pintar()
		}
	})
}

func (c *conexionRooms) elegirProveedor() {
	if c.proveedor == "" {
		c.proveedor = c.elegida
	}
	// La cuenta y los destinos pertenecen al proveedor anterior. El inventario
	// actual decide qué instancia se puede reutilizar para este consentimiento.
	c.instancia, c.destino, c.destinos = "", "", nil
	c.pendiente, c.esperada = "", ""
	if c.esWhatsApp() {
		c.ir(0)
		return
	}
	if !c.disponible(c.proveedor) {
		c.ir(1)
		return
	}
	c.ir(1)
}

func (c *conexionRooms) autorizar() {
	if c.ocupado || !c.puedeAutorizar() {
		return
	}
	c.ocupado, c.errorClave = true, ""
	c.pintar()
	proveedor, pendiente, esperada := c.proveedor, c.pendiente, c.esperada
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (cuentaRoomsAutorizada, error) {
		return c.pedirAutorizacion(ctx, proveedor, pendiente, esperada)
	}, func(a cuentaRoomsAutorizada, err error) {
		if c.cerrada {
			return
		}
		c.ocupado = false
		if err != nil {
			if a.id != "" {
				if a.consentida {
					c.esperada, c.pendiente = a.id, ""
				} else {
					c.pendiente = a.id
				}
			}
			c.errorClave = "appConnection.failed"
			if errors.Is(err, api.ErrOAuthDenied) {
				c.errorClave = "appConnection.denied"
			}
			if errors.Is(err, api.ErrOAuthUnavailable) {
				c.errorClave = "external.unavailable"
			}
			c.pintar()
			return
		}
		if c.proveedor != proveedor {
			return
		}
		c.cuentas = a.cuentas
		c.pendiente, c.esperada = "", ""
		c.elegirCuenta(a.id)
		c.cargarDestinos()
	})
}

type cuentaRoomsAutorizada struct {
	id         string
	cuentas    []api.OAuthInstance
	consentida bool
}

func (c *conexionRooms) pedirAutorizacion(ctx context.Context, proveedor, pendiente, esperada string) (cuentaRoomsAutorizada, error) {
	id, err := c.iniciarAutorizacion(ctx, proveedor, pendiente, esperada)
	if err != nil {
		return cuentaRoomsAutorizada{id: id}, err
	}
	a := cuentaRoomsAutorizada{id: id, consentida: true}
	a.cuentas, err = c.sa.CuentasApps.OAuthInstances(ctx)
	if err != nil {
		return a, err
	}
	for _, cuenta := range a.cuentas {
		if cuenta.ID == id && cuenta.TemplateKey == proveedor && cuenta.Active() && cuenta.OAuthAvailable {
			return a, nil
		}
	}
	return a, api.ErrOAuthState
}

func (c *conexionRooms) iniciarAutorizacion(ctx context.Context, proveedor, pendiente, esperada string) (string, error) {
	switch {
	case esperada != "":
		return esperada, nil
	case pendiente != "":
		return c.sa.AutorizarApp(ctx, proveedor, pendiente)
	case c.sa.AutorizarAppNueva != nil:
		return c.sa.AutorizarAppNueva(ctx, proveedor)
	default:
		return c.sa.AutorizarApp(ctx, proveedor, "")
	}
}

func (c *conexionRooms) autorizarNueva() {
	c.pendiente, c.esperada = "", ""
	c.autorizar()
}

func (c *conexionRooms) puedeAutorizar() bool {
	if c.esperada != "" {
		return c.sa != nil && c.sa.CuentasApps != nil
	}
	if c.pendiente != "" {
		return c.sa != nil && c.sa.AutorizarApp != nil && c.disponible(c.proveedor)
	}
	return c.puedeAgregar()
}

func (c *conexionRooms) puedeAgregar() bool {
	registrado := false
	for _, p := range c.proveedores {
		registrado = registrado || p.Key == c.proveedor && p.Available
	}
	if !registrado || c.sa == nil {
		return false
	}
	if c.sa.AutorizarAppNueva != nil {
		return true
	}
	for _, cuenta := range c.cuentas {
		if cuenta.TemplateKey == c.proveedor && cuenta.Active() {
			return false
		}
	}
	return c.sa.AutorizarApp != nil
}

func (c *conexionRooms) elegirCuenta(id string) {
	for _, cuenta := range c.cuentas {
		if cuenta.ID == id && cuenta.TemplateKey == c.proveedor && cuenta.Active() && cuenta.OAuthAvailable {
			if c.instancia != id {
				c.instancia, c.destino, c.destinos = id, "", nil
			}
			c.pendiente, c.esperada = "", ""
			return
		}
	}
}

func (c *conexionRooms) disponible(key string) bool {
	for _, p := range c.proveedores {
		if p.Key == key && p.Available {
			return true
		}
	}
	for _, cuenta := range c.cuentas {
		if cuenta.TemplateKey == key && cuenta.OAuthAvailable {
			return true
		}
	}
	return false
}

func (c *conexionRooms) cargarDestinos() {
	c.paso, c.ocupado, c.errorClave = 2, true, ""
	c.destinos, c.destino = nil, ""
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) ([]api.RoomsIntegrationDestination, error) {
		return c.api.IntegrationDestinations(ctx, c.instancia)
	}, func(ds []api.RoomsIntegrationDestination, err error) {
		if c.cerrada {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "appConnection.failed"
		} else {
			c.destinos = ds
			if len(ds) == 1 {
				c.destino = ds[0].ID
			}
		}
		c.pintar()
	})
}

func (c *conexionRooms) autorizarMeta() {
	if c.ocupado || c.equipo == "" {
		return
	}
	c.ocupado, c.errorClave = true, ""
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (api.WhatsAppSignup, error) {
		status, err := c.sa.CuentasApps.WhatsAppSignupMetadata(ctx)
		if err != nil {
			return api.WhatsAppSignup{}, err
		}
		if !status.Available {
			return api.WhatsAppSignup{}, errWhatsAppNoDisponible
		}
		return c.sa.CuentasApps.BeginWhatsAppSignup(ctx, c.instancia)
	}, func(signup api.WhatsAppSignup, err error) {
		if c.cerrada {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "appConnection.failed"
			if errors.Is(err, errWhatsAppNoDisponible) {
				c.errorClave = "whatsappConnection.unavailable"
			}
			c.pintar()
			return
		}
		c.instancia = signup.InstanceID
		u, err := url.Parse(api.AppURL(c.sa.Apex, "rooms", c.sa.org(), ""))
		if err == nil {
			q := u.Query()
			q.Set("native-whatsapp", "1")
			q.Set("instance", signup.InstanceID)
			q.Set("state", signup.State)
			u.RawQuery = q.Encode()
			if c.sa.AbrirEnlace == nil {
				err = errWhatsAppNoDisponible
			} else {
				err = c.sa.AbrirEnlace(u)
			}
		}
		if err != nil {
			c.errorClave = "appConnection.failed"
			c.pintar()
			return
		}
		c.metaAbierto = true
		c.ir(1)
	})
}

var (
	errWhatsAppNoDisponible = errors.New("whatsapp: not available")
	pinWhatsApp             = regexp.MustCompile(`^\d{6}$`)
)

func (c *conexionRooms) comprobarNumeros() {
	if c.ocupado || c.instancia == "" {
		return
	}
	c.ocupado, c.errorClave = true, ""
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) ([]api.WhatsAppNumber, error) {
		return c.sa.CuentasApps.WhatsAppNumbers(ctx, c.instancia)
	}, func(ns []api.WhatsAppNumber, err error) {
		if c.cerrada {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "appConnection.failed"
			c.pintar()
			return
		}
		c.numeros, c.telefono = ns, ""
		c.pin.PonerTexto("")
		if len(ns) == 1 {
			c.telefono = ns[0].ID
		}
		c.ir(2)
	})
}

func (c *conexionRooms) numero() *api.WhatsAppNumber {
	for idx := range c.numeros {
		if c.numeros[idx].ID == c.telefono {
			return &c.numeros[idx]
		}
	}
	return nil
}

func (c *conexionRooms) valido() bool {
	if c.ocupado || c.cargando {
		return false
	}
	if c.paso == 0 {
		if c.esWhatsApp() {
			for _, equipo := range c.equipos {
				if equipo.ID == c.equipo {
					return true
				}
			}
			return false
		}
		return c.elegida != ""
	}
	if c.paso == 1 {
		return c.esWhatsApp() || c.disponible(c.proveedor)
	}
	if c.esWhatsApp() {
		n := c.numero()
		return n != nil && (!n.RegistrationRequired || pinWhatsApp.MatchString(c.pin.Texto()))
	}
	if !c.disponible(c.proveedor) {
		return false
	}
	canalValido, destinoValido := false, false
	for _, canal := range c.sa.canales {
		if canal.ID == c.canal && canal.Kind != "direct" && !canal.Archived {
			canalValido = true
		}
	}
	for _, destino := range c.destinos {
		if destino.ID == c.destino {
			destinoValido = true
		}
	}
	return canalValido && destinoValido
}

func (c *conexionRooms) actualizarBoton() {
	if c.siguiente != nil {
		c.siguiente.Inerte = !c.valido()
		c.siguiente.Refresh()
	}
}

func (c *conexionRooms) activar() {
	if !c.valido() || c.instancia == "" {
		return
	}
	c.ocupado, c.errorClave = true, ""
	// UI controls are read on the UI thread, before the background request starts.
	whatsapp := c.esWhatsApp()
	equipo, instancia, telefono, canal, destino := c.equipo, c.instancia, c.telefono, c.canal, c.destino
	pin := ""
	if n := c.numero(); n != nil && n.RegistrationRequired {
		pin = c.pin.Texto()
	}
	plataforma := ""
	for _, p := range proveedoresRooms {
		if p.clave == c.proveedor {
			plataforma = p.plataforma
		}
	}
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (struct{}, error) {
		if whatsapp {
			a, err := c.api.ConnectWhatsAppConsent(ctx, api.RoomsWhatsAppConsent{TeamID: equipo, InstanceID: instancia, PhoneNumberID: telefono, PIN: pin})
			if err == nil && a.ID == "" {
				err = errWhatsAppNoDisponible
			}
			return struct{}{}, err
		}
		b, err := c.api.ConnectOAuthBridge(ctx, canal, plataforma, instancia, destino)
		if err == nil && b.ID == "" {
			err = errWhatsAppNoDisponible
		}
		return struct{}{}, err
	}, func(_ struct{}, err error) {
		if c.cerrada {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "appConnection.failed"
			c.pintar()
			return
		}
		c.pin.PonerTexto("")
		c.ir(4)
		c.sa.cargarCanales()
	})
}

func (c *conexionRooms) avanzar() {
	if !c.valido() {
		return
	}
	switch c.paso {
	case 0:
		if c.esWhatsApp() {
			c.autorizarMeta()
		} else {
			c.proveedor = c.elegida
			c.elegirProveedor()
		}
	case 1:
		if c.esWhatsApp() {
			c.comprobarNumeros()
		} else {
			if c.esperada != "" {
				c.autorizar()
			} else if cuenta := c.cuenta(); cuenta != nil && cuenta.OAuthAvailable {
				c.cargarDestinos()
			} else {
				c.autorizar()
			}
		}
	case 2:
		c.ir(3)
	case 3:
		c.activar()
	}
}

func (c *conexionRooms) desconectar() {
	if c.ocupado || c.cuenta() == nil {
		return
	}
	id := c.cuenta().ID
	c.ocupado = true
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, c.sa.CuentasApps.OAuthDisconnect(ctx, id)
	}, func(_ struct{}, err error) {
		if c.cerrada {
			return
		}
		c.ocupado, c.confirmar = false, false
		if err != nil {
			c.errorClave = "appConnection.failed"
			c.pintar()
			return
		}
		for idx := range c.cuentas {
			if c.cuentas[idx].ID == id {
				c.cuentas[idx].Status, c.cuentas[idx].HasSecrets = "disabled", false
			}
		}
		c.instancia, c.destino = "", ""
		c.ir(1)
	})
}

func (c *conexionRooms) selector(rotulo, actual string, ids, nombres []string, elegir func(string)) fyne.CanvasObject {
	opciones := append([]string{rotulo}, nombres...)
	s := ui.NuevoSelector(opciones, func(i int) {
		id := ""
		if i > 0 && i <= len(ids) {
			id = ids[i-1]
		}
		elegir(id)
		c.pintar()
	})
	s.Rotulo = rotulo
	for i, id := range ids {
		if id == actual {
			s.Elegida = i + 1
		}
	}
	s.Ancho = 300
	return campoCon(rotulo, s, "")
}

// pintar uses the shared native dialog footer and progress, including narrow windows.
//
//nolint:gocognit,gocyclo // Five explicit wizard views keep domain transitions visible.
func (c *conexionRooms) pintar() {
	if c.cerrada {
		return
	}
	t := c.sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	parrafo := func(s string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text-muted", v).A(max(180, min(520, c.lienzo.Size().Width-48)))
	}
	titulos := []string{t.T("appConnection.choose", nil), t.T("appConnection.authorize", nil), t.T("appConnection.channels", nil), t.T("appConnection.review", nil), t.T("appConnection.done", nil)}
	if c.esWhatsApp() {
		titulos = []string{t.T("whatsappConnection.team", nil), t.T("whatsappConnection.authorize", nil), t.T("whatsappConnection.number", nil), t.T("appConnection.review", nil), t.T("whatsappConnection.done", nil)}
	}
	c.pasos = ui.NuevosPasos(titulos...)
	c.pasos.Actual, c.pasos.Ventana = c.paso, c.lienzo.Size().Width
	hijos := []fyne.CanvasObject{c.pasos, ui.NuevoRotulo(titulos[c.paso], tema.TextoLG, ui.SemiNegrita, "text")}
	if c.errorClave != "" {
		hijos = append(hijos, ui.NuevoRotulo(t.T(c.errorClave, i18n.Vars{"provider": c.nombre()}), tema.TextoSM, ui.Normal, "danger-text"))
	}
	switch {
	case c.cargando:
		hijos = append(hijos, parrafo(t.T("state.loading", nil)))
	case c.proveedor == "" || (!c.esWhatsApp() && c.paso == 0):
		for _, p := range proveedoresRooms {
			tarjeta := ui.NuevaTarjetaEleccion(p.nombre, "", func() { c.elegida = p.clave; c.pintar() })
			tarjeta.Elegida, tarjeta.Inerte, tarjeta.Ancho = c.elegida == p.clave, c.ocupado, max(180, min(520, c.lienzo.Size().Width-48))
			hijos = append(hijos, tarjeta)
		}
	case c.paso == 4:
		switch {
		case c.esWhatsApp():
			n := c.numero()
			numero := ""
			if n != nil {
				numero = n.DisplayPhone
			}
			hijos = append(hijos, parrafo(t.T("whatsappConnection.doneBody", i18n.Vars{"number": numero, "team": c.equipoNombre()})))
		default:
			hijos = append(hijos, parrafo(t.T("appConnection.doneBody", i18n.Vars{"channel": c.canalNombre(), "provider": c.nombre()})))
		}
	case c.esWhatsApp() && c.paso == 0:
		ids, nombres := []string{}, []string{}
		for _, equipo := range c.equipos {
			ids, nombres = append(ids, equipo.ID), append(nombres, equipo.Name)
		}
		hijos = append(hijos, parrafo(t.T("whatsappConnection.teamBody", nil)), c.selector(t.T("integrations.whatsappTeam", nil), c.equipo, ids, nombres, func(id string) { c.equipo = id }))
		if len(ids) == 0 {
			hijos = append(hijos, parrafo(t.T("whatsappConnection.noTeams", nil)))
		}
	case c.paso == 1:
		clave := "appConnection.authorizeBody"
		if c.esWhatsApp() {
			clave = "whatsappConnection.authorizeBody"
		}
		hijos = append(hijos, parrafo(t.T(clave, i18n.Vars{"provider": c.nombre()})))
		if !c.esWhatsApp() {
			existen := false
			for _, cuenta := range c.cuentas {
				if cuenta.TemplateKey != c.proveedor || !cuenta.Active() || !cuenta.OAuthAvailable {
					continue
				}
				existen = true
				tarjeta := ui.NuevaTarjetaEleccion(cuenta.Name, "", func() { c.elegirCuenta(cuenta.ID); c.pintar() })
				tarjeta.Elegida, tarjeta.Inerte, tarjeta.Ancho = c.instancia == cuenta.ID, c.ocupado, max(180, min(520, c.lienzo.Size().Width-48))
				hijos = append(hijos, tarjeta)
			}
			if existen {
				agregar := ui.NuevoBoton(t.T("external.addAccount", nil), c.autorizarNueva).Con(ui.Secundario, ui.TallaMD)
				agregar.Inerte = c.ocupado || !c.puedeAgregar()
				hijos = append(hijos, agregar)
			}
			if c.pendiente != "" || c.esperada != "" {
				reintentar := ui.NuevoBoton(t.T("action.retry", nil), c.autorizar).Con(ui.Secundario, ui.TallaMD)
				reintentar.Inerte = c.ocupado || !c.puedeAutorizar()
				hijos = append(hijos, reintentar)
			}
		}
		if !c.esWhatsApp() && !c.disponible(c.proveedor) {
			hijos = append(hijos, parrafo(t.T("external.unavailable", i18n.Vars{"provider": c.nombre()})))
		}
		if c.esWhatsApp() && c.metaAbierto {
			hijos = append(hijos, ui.NuevoBoton(t.T("appConnection.continue", i18n.Vars{"provider": "Meta"}), c.autorizarMeta).Con(ui.Secundario, ui.TallaMD))
		}
	case c.paso == 2:
		if c.esWhatsApp() {
			ids, nombres := []string{}, []string{}
			for _, n := range c.numeros {
				ids, nombres = append(ids, n.ID), append(nombres, n.DisplayPhone+" · "+n.VerifiedName)
			}
			hijos = append(hijos, c.selector(t.T("whatsappConnection.number", nil), c.telefono, ids, nombres, func(id string) { c.telefono = id; c.pin.PonerTexto("") }))
			if len(ids) == 0 {
				hijos = append(hijos, parrafo(t.T("whatsappConnection.noNumbers", nil)), ui.NuevoBoton(t.T("appConnection.continue", i18n.Vars{"provider": "Meta"}), c.autorizarMeta).Con(ui.Secundario, ui.TallaMD))
			}
			if n := c.numero(); n != nil && n.RegistrationRequired {
				hijos = append(hijos, campoCon(t.T("whatsappConnection.pin", nil), c.pin, t.T("whatsappConnection.pinHint", nil)))
			}
		} else {
			ids, nombres := []string{}, []string{}
			for _, ch := range c.sa.canales {
				if ch.Kind != "direct" && !ch.Archived {
					ids, nombres = append(ids, ch.ID), append(nombres, "#"+ch.Title())
				}
			}
			hijos = append(hijos, c.selector(t.T("appConnection.roomsChannel", nil), c.canal, ids, nombres, func(id string) { c.canal = id }))
			ids, nombres = []string{}, []string{}
			for _, d := range c.destinos {
				ids, nombres = append(ids, d.ID), append(nombres, d.Name)
			}
			hijos = append(hijos, c.selector(t.T("appConnection.destination", i18n.Vars{"provider": c.nombre()}), c.destino, ids, nombres, func(id string) { c.destino = id }))
			if c.errorClave != "" {
				hijos = append(hijos, ui.NuevoBoton(t.T("action.retry", nil), c.cargarDestinos).Con(ui.Secundario, ui.TallaMD))
			}
		}
	case c.paso == 3:
		switch {
		case c.esWhatsApp():
			numero := ""
			if n := c.numero(); n != nil {
				numero = n.DisplayPhone
			}
			hijos = append(hijos, parrafo(t.T("whatsappConnection.reviewBody", i18n.Vars{"number": numero, "team": c.equipoNombre()})), parrafo(t.T("integrations.whatsappWindow", nil)))
		default:
			destino := ""
			for _, d := range c.destinos {
				if d.ID == c.destino {
					destino = d.Name
				}
			}
			hijos = append(hijos, parrafo(t.T("appConnection.shareSummary", i18n.Vars{"channel": c.canalNombre(), "destination": destino})), parrafo(t.T("appConnection.testHint", nil)))
		}
	}
	if !c.esWhatsApp() && c.cuenta() != nil && c.paso < 4 {
		hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)}, ui.NuevaInsignia(t.T("meet.connected", nil), ui.InsigniaExito), ui.NuevoBoton(t.T("integrations.bridgeDisconnect", nil), func() { c.confirmar = true; c.pintar() }).Con(ui.Fantasma, ui.TallaSM)))
	}
	if c.confirmar {
		hijos = append(hijos, parrafo(t.T("appConnection.disconnectBody", i18n.Vars{"provider": c.nombre()})), container.New(ui.Fila{Hueco: tema.Espacio(2)}, ui.NuevoBoton(t.T("action.cancel", nil), func() { c.confirmar = false; c.pintar() }).Con(ui.Secundario, ui.TallaSM), ui.NuevoBoton(t.T("integrations.bridgeDisconnect", nil), c.desconectar).Con(ui.Peligro, ui.TallaSM)))
	}
	c.cuerpo.Objects = hijos
	c.cuerpo.Refresh()
	acciones := []fyne.CanvasObject{ui.NuevoBoton(t.T("action.cancel", nil), c.cerrar).Con(ui.Fantasma, ui.TallaMD)}
	if c.paso == 4 {
		acciones = []fyne.CanvasObject{ui.NuevoBoton(t.T("action.done", nil), c.cerrar).Con(ui.Primario, ui.TallaMD)}
	} else {
		if c.paso > 0 {
			atras := ui.NuevoBoton(t.T("action.back", nil), func() { c.ir(c.paso - 1) }).Con(ui.Secundario, ui.TallaMD)
			atras.Inerte = c.ocupado
			acciones = append(acciones, atras)
		}
		rotulo := t.T("ui.wizard.next", nil)
		if c.paso == 1 {
			if c.esWhatsApp() {
				rotulo = t.T("action.retry", nil)
			} else if c.cuenta() == nil {
				rotulo = t.T("appConnection.continue", i18n.Vars{"provider": c.nombre()})
			}
		}
		if c.paso == 3 {
			rotulo = t.T("appConnection.connect", nil)
		}
		if c.ocupado {
			rotulo = t.T("state.loading", nil)
		}
		c.siguiente = ui.NuevoBoton(rotulo, c.avanzar).Con(ui.Primario, ui.TallaMD)
		c.siguiente.Inerte = !c.valido()
		if c.paso == 1 && !c.esWhatsApp() && c.cuenta() == nil {
			c.siguiente.Inerte = c.siguiente.Inerte || !c.puedeAutorizar()
		}
		acciones = append(acciones, c.siguiente)
	}
	c.pie.Objects = acciones
	c.pie.Refresh()
	c.d.Refresh()
}

func (c *conexionRooms) equipoNombre() string {
	for _, e := range c.equipos {
		if e.ID == c.equipo {
			return e.Name
		}
	}
	return ""
}

func (c *conexionRooms) canalNombre() string {
	for _, ch := range c.sa.canales {
		if ch.ID == c.canal {
			return ch.Title()
		}
	}
	return ""
}

// ReanudarWhatsApp accepts no success flag or provider code. The active wizard rechecks its own instance.
func (sa *Sala) ReanudarWhatsApp(instance string) {
	if sa.comprobarWhatsApp != nil {
		sa.comprobarWhatsApp(instance)
	}
}
