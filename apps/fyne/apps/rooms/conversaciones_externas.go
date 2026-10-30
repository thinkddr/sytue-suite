// SPDX-License-Identifier: Apache-2.0
package rooms

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

var proveedoresConversaciones = []struct{ id, nombre string }{
	{"slack", "Slack"}, {"microsoft-teams", "Microsoft Teams"}, {"google-chat", "Google Chat"},
}

type conversacionExterna struct {
	sa                                                                         *Sala
	ctx                                                                        context.Context
	cancel                                                                     context.CancelFunc
	d                                                                          *ui.Dialogo
	lienzo                                                                     fyne.Canvas
	cuerpo, pie                                                                *fyne.Container
	cuentas                                                                    api.ExternalChatAccounts
	canales                                                                    []api.ExternalChatChannel
	mensajes                                                                   []api.ExternalChatMessage
	capacidades                                                                api.ExternalChatCapabilities
	proveedor, instancia, remoto, cursorCanales, cursor, hilo, edicion, filtro string
	esperada                                                                   string
	pendiente                                                                  string
	paso                                                                       int
	cerrado, ocupado, cargando, lector, incierto, cursorIniciado               bool
	errorClave                                                                 string
	enlaceMeet, errorMeet                                                      string
	cuentaMeet, esperadaMeet                                                   string
	pendienteMeet                                                              string
	proveedorMeet, canalMeet, envioMeet                                        string
	compartidoMeet                                                             bool
	inciertasMeet                                                              map[string]bool
	generacionMeet                                                             string
	entrada                                                                    *ui.AreaTexto
	enviarBoton                                                                *ui.Boton
	version                                                                    uint64
	siguienteLectura                                                           time.Time
	temporizador                                                               *time.Timer
}

// AbrirConversacionesExternas is available to each consenting person, including
// members. Provider text lives only in this dialog's RAM, outside native Rooms
// messages, encrypted drafts, forwarding, search and notification bridges.
func (sa *Sala) AbrirConversacionesExternas() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if sa.CuentasApps == nil || lienzo == nil {
		sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("appConnection.unavailable", nil))
		return
	}
	if sa.cerrarConexionApps != nil {
		sa.cerrarConexionApps()
	}
	ctx := sa.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &conversacionExterna{sa: sa, ctx: ctx, cancel: cancel, lienzo: lienzo, proveedor: "slack", cargando: true}
	if sa.GeneracionSesion != nil {
		c.generacionMeet = sa.GeneracionSesion()
	}
	c.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(4)})
	c.pie = container.New(ui.Fila{Hueco: tema.Espacio(2)})
	c.entrada = ui.NuevaAreaTexto("")
	c.entrada.Filas = 3
	c.entrada.AlCambiar(func(string) { c.actualizarEnvio() })
	c.d = ui.NuevoDialogo(sa.P.T.T("external.title", nil), c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.d.RotuloCerrar = sa.P.T.T("action.close", nil)
	c.d.AlCerrar = c.cerrar
	sa.cerrarConexionApps = c.cerrar
	c.pintar()
	c.d.Mostrar(lienzo)
	c.cargarCuentas("")
}

func (c *conversacionExterna) cerrar() {
	if c.cerrado {
		return
	}
	c.cerrado = true
	c.cancel()
	if c.temporizador != nil {
		c.temporizador.Stop()
	}
	c.entrada.PonerTexto("")
	clear(c.mensajes)
	clear(c.canales)
	c.mensajes, c.canales, c.cuentas.Items = nil, nil, nil
	c.cuerpo.RemoveAll()
	c.sa.cerrarConexionApps = nil
	c.d.Cerrar()
}

func (c *conversacionExterna) nombre() string {
	for _, p := range proveedoresConversaciones {
		if p.id == c.proveedor {
			return p.nombre
		}
	}
	return ""
}

func (c *conversacionExterna) cuenta() *api.ExternalChatAccount {
	for idx := range c.cuentas.Items {
		if c.cuentas.Items[idx].InstanceID == c.instancia && c.cuentas.Items[idx].Provider == c.proveedor {
			return &c.cuentas.Items[idx]
		}
	}
	return nil
}

func (c *conversacionExterna) canal() *api.ExternalChatChannel {
	for idx := range c.canales {
		if c.canales[idx].ID == c.remoto {
			return &c.canales[idx]
		}
	}
	return nil
}

func (c *conversacionExterna) estadoProveedor(proveedor string) api.ExternalChatProvider {
	for _, p := range c.cuentas.Providers {
		if p.Provider == proveedor {
			return p
		}
	}
	return api.ExternalChatProvider{}
}

func (c *conversacionExterna) disponible() bool {
	estado := c.estadoProveedor(c.proveedor)
	return estado.Available && !estado.DisabledByOrg
}

func nombreCuentaExterna(a api.ExternalChatAccount) string {
	nombre := a.Name
	if nombre == "" {
		nombre = a.Provider
	}
	if a.WorkspaceName != "" {
		return nombre + " · " + a.WorkspaceName
	}
	if a.AccountID != "" {
		return nombre + " · " + a.AccountID
	}
	return nombre
}

func (c *conversacionExterna) cargarCuentas(instancia string) {
	if instancia != "" {
		c.esperada = instancia
	}
	c.cargando = true
	c.pintar()
	pedirEn(c.ctx, c.sa, c.sa.CuentasApps.ExternalChatAccounts, func(datos api.ExternalChatAccounts, err error) {
		if c.cerrado {
			return
		}
		c.cargando = false
		if err != nil {
			c.errorClave = "appConnection.failed"
		} else {
			c.cuentas = datos
			c.reconciliarMeet()
			if c.esperada != "" {
				for _, cuenta := range datos.Items {
					if cuenta.InstanceID == c.esperada && cuenta.Provider == c.proveedor && cuenta.Connected {
						c.instancia, c.esperada, c.errorClave, c.paso = cuenta.InstanceID, "", "", 1
						c.avanzar()
						return
					}
				}
				c.errorClave = "appConnection.failed"
			}
		}
		c.pintar()
	})
}

func (c *conversacionExterna) autorizar() {
	id := c.instancia
	if c.pendiente != "" {
		id = c.pendiente
	}
	c.autorizarCuenta(id)
}

func (c *conversacionExterna) autorizarNueva() {
	c.pendiente, c.esperada = "", ""
	c.autorizarCuenta("")
}

func (c *conversacionExterna) autorizarCuenta(instancia string) {
	if c.ocupado || c.sa.AutorizarConversaciones == nil || !c.disponible() {
		return
	}
	c.ocupado, c.errorClave = true, ""
	c.pintar()
	proveedor := c.proveedor
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (string, error) {
		return c.sa.AutorizarConversaciones(ctx, proveedor, instancia)
	}, func(id string, err error) {
		if c.cerrado {
			return
		}
		c.ocupado = false
		if err != nil || id == "" {
			if id != "" {
				c.pendiente = id
			}
			c.errorClave = "appConnection.failed"
			if errors.Is(err, api.ErrOAuthDenied) {
				c.errorClave = "appConnection.denied"
			}
			c.pintar()
			return
		}
		if c.proveedor != proveedor {
			return
		}
		c.pendiente = ""
		c.cargarCuentas(id)
	})
}

func (c *conversacionExterna) cargarCanales(mas bool) {
	if c.ocupado {
		return
	}
	cursor := ""
	if mas {
		cursor = c.cursorCanales
	} else {
		c.canales, c.cursorCanales, c.remoto = nil, "", ""
	}
	c.paso, c.ocupado, c.errorClave = 2, true, ""
	c.pintar()
	instancia := c.instancia
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (api.ExternalChatChannels, error) {
		return c.sa.CuentasApps.ExternalChatChannels(ctx, instancia, cursor)
	}, func(datos api.ExternalChatChannels, err error) {
		if c.cerrado {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "appConnection.failed"
		} else {
			for _, item := range datos.Items {
				if !slices.ContainsFunc(c.canales, func(x api.ExternalChatChannel) bool { return x.ID == item.ID }) {
					c.canales = append(c.canales, item)
				}
			}
			c.cursorCanales = datos.NextCursor
			if len(c.canales) == 1 {
				c.remoto = c.canales[0].ID
			}
		}
		c.pintar()
	})
}

func (c *conversacionExterna) avanzar() {
	if c.ocupado || c.cargando {
		return
	}
	if c.estadoProveedor(c.proveedor).DisabledByOrg {
		c.pintar()
		return
	}
	c.errorClave = ""
	switch c.paso {
	case 0:
		c.paso = 1
		c.pintar()
	case 1:
		if c.esperada != "" {
			c.cargarCuentas("")
		} else if cuenta := c.cuenta(); cuenta != nil && cuenta.Connected {
			c.cargarCanales(false)
		} else {
			c.autorizar()
		}
	case 2:
		if c.canal() != nil {
			c.paso = 3
			c.pintar()
		}
	case 3:
		if canal := c.canal(); canal != nil {
			c.capacidades = canal.Capabilities
			c.lector = true
			c.d.Titulo = canal.Name
			c.d.Refresh()
			c.leer(false, true)
		}
	}
}

func (c *conversacionExterna) irAtras() {
	if c.ocupado || c.cargando || c.paso <= 0 {
		return
	}
	c.paso--
	c.errorClave = ""
	c.pintar()
}

func (c *conversacionExterna) texto(s string) fyne.CanvasObject {
	return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())).A(max(180, min(520, c.lienzo.Size().Width-48)))
}

func (c *conversacionExterna) boton(clave string, fn func()) *ui.Boton {
	b := ui.NuevoBoton(c.sa.P.T.T(clave, nil), fn).Con(ui.Secundario, ui.TallaSM)
	b.Inerte = c.ocupado || c.cargando
	return b
}

func (c *conversacionExterna) pintar() {
	if c.cerrado {
		return
	}
	if c.lector {
		c.pintarLector()
		return
	}
	t := c.sa.P.T
	titulos := []string{t.T("appConnection.choose", nil), t.T("appConnection.authorize", nil), t.T("appConnection.channels", nil), t.T("appConnection.review", nil)}
	pasos := ui.NuevosPasos(titulos...)
	pasos.Actual, pasos.Ventana = c.paso, c.lienzo.Size().Width
	hijos := []fyne.CanvasObject{pasos, ui.NuevoRotulo(titulos[c.paso], tema.TextoLG, ui.SemiNegrita, "text")}
	if c.errorClave != "" {
		hijos = append(hijos, c.texto(t.T(c.errorClave, nil)), c.boton("action.retry", func() {
			switch {
			case c.paso == 2:
				c.cargarCanales(false)
			case c.paso == 1 && c.pendiente != "":
				c.autorizar()
			default:
				c.cargarCuentas("")
			}
		}))
	}
	if c.cargando {
		hijos = append(hijos, c.texto(t.T("state.loading", nil)))
	} else {
		hijos = append(hijos, c.pasoContenido()...)
	}
	c.cuerpo.Objects = hijos
	c.cuerpo.Refresh()
	cancelar := c.boton("action.cancel", c.cerrar)
	cancelar.Inerte = false
	botones := []fyne.CanvasObject{cancelar, layout.NewSpacer()}
	if c.paso > 0 {
		botones = append(botones, c.boton("action.back", c.irAtras))
	}
	clave := "ui.wizard.next"
	if c.paso == 3 {
		clave = "external.open"
	}
	siguiente := c.boton(clave, c.avanzar)
	siguiente.Inerte = siguiente.Inerte || c.estadoProveedor(c.proveedor).DisabledByOrg
	if c.paso == 1 && c.esperada != "" {
		siguiente.Texto = t.T("action.retry", nil)
	}
	if c.requiereConsentimiento() {
		siguiente.Texto = t.T("appConnection.continue", i18n.Vars{"provider": c.nombre()})
		siguiente.Inerte = siguiente.Inerte || !c.disponible() || c.sa.AutorizarConversaciones == nil
	}
	if c.paso >= 2 {
		siguiente.Inerte = siguiente.Inerte || c.canal() == nil
	}
	botones = append(botones, siguiente)
	c.pie.Objects = botones
	c.pie.Refresh()
	c.d.Refresh()
}

func (c *conversacionExterna) requiereConsentimiento() bool {
	cuenta := c.cuenta()
	return c.paso == 1 && c.esperada == "" && (cuenta == nil || !cuenta.Connected)
}

func (c *conversacionExterna) pasoContenido() []fyne.CanvasObject {
	t := c.sa.P.T
	hijos := []fyne.CanvasObject{}
	switch c.paso {
	case 0:
		hijos = append(hijos, c.texto(t.T("external.body", nil)))
		for _, p := range proveedoresConversaciones {
			descripcion := ""
			if c.estadoProveedor(p.id).DisabledByOrg {
				descripcion = t.T("app.disabled", nil)
			}
			tarjeta := ui.NuevaTarjetaEleccion(p.nombre, descripcion, func() { c.proveedor, c.instancia, c.esperada, c.pendiente = p.id, "", "", ""; c.pintar() })
			tarjeta.Elegida, tarjeta.Inerte, tarjeta.Ancho = c.proveedor == p.id, c.ocupado, max(180, min(520, c.lienzo.Size().Width-48))
			hijos = append(hijos, tarjeta)
		}
		hijos = append(hijos, c.bloqueMeet()...)
	case 1:
		hijos = append(hijos, c.texto(t.T("appConnection.authorizeBody", i18n.Vars{"provider": c.nombre()})))
		for _, cuenta := range c.cuentas.Items {
			if cuenta.Provider != c.proveedor {
				continue
			}
			tarjeta := ui.NuevaTarjetaEleccion(nombreCuentaExterna(cuenta), "", func() { c.instancia, c.esperada, c.pendiente = cuenta.InstanceID, "", ""; c.pintar() })
			tarjeta.Elegida, tarjeta.Inerte, tarjeta.Ancho = cuenta.InstanceID == c.instancia, c.ocupado, max(180, min(520, c.lienzo.Size().Width-48))
			hijos = append(hijos, tarjeta)
		}
		agregar := c.boton("external.addAccount", c.autorizarNueva)
		agregar.Inerte = agregar.Inerte || !c.disponible() || c.sa.AutorizarConversaciones == nil
		hijos = append(hijos, agregar)
		if c.cuenta() != nil {
			hijos = append(hijos, c.boton("integrations.bridgeDisconnect", c.confirmarDesconexion))
		}
		if c.estadoProveedor(c.proveedor).DisabledByOrg {
			hijos = append(hijos, c.texto(t.T("app.disabled", nil)))
		} else if !c.disponible() {
			hijos = append(hijos, c.texto(t.T("external.unavailable", i18n.Vars{"provider": c.nombre()})))
		}
	case 2:
		entrada := ui.NuevaEntrada(t.T("browse.searchPlaceholder", nil))
		entrada.PonerTexto(c.filtro)
		seleccion := container.New(ui.Columna{}, c.selectorCanal())
		entrada.AlCambiar(func(s string) {
			c.filtro = s
			seleccion.Objects = []fyne.CanvasObject{c.selectorCanal()}
			seleccion.Refresh()
		})
		hijos = append(hijos, campoCon(t.T("browse.searchPlaceholder", nil), entrada, ""), seleccion)
		if c.cursorCanales != "" {
			hijos = append(hijos, c.boton("action.loadMore", func() { c.cargarCanales(true) }))
		}
		if len(c.canales) == 0 && !c.ocupado {
			hijos = append(hijos, c.texto(t.T("sidebar.noChannelsYet", nil)))
		}
	case 3:
		if cuenta, canal := c.cuenta(), c.canal(); cuenta != nil && canal != nil {
			hijos = append(hijos, c.texto(nombreCuentaExterna(*cuenta)), c.texto(canal.Name), c.texto(t.T("external.protection", i18n.Vars{"provider": c.nombre()})))
		}
	}
	return hijos
}

func (c *conversacionExterna) selectorCanal() fyne.CanvasObject {
	t := c.sa.P.T
	opciones := []string{""}
	nombres := map[string]string{"": t.T("appConnection.chooseChannel", nil)}
	for _, canal := range c.canales {
		if c.filtro == "" || buscar.Contiene(canal.Name, c.filtro) {
			opciones = append(opciones, canal.ID)
			nombres[canal.ID] = canal.Name
		}
	}
	selector := selectorDe(opciones, func(id string) string { return nombres[id] }, c.remoto, func(id string) { c.remoto = id; c.pintar() })
	selector.Rotulo = t.T("appConnection.destination", i18n.Vars{"provider": c.nombre()})
	return campoCon(selector.Rotulo, selector, "")
}

func (c *conversacionExterna) confirmarDesconexion() {
	cuenta := c.cuenta()
	if c.ocupado || cuenta == nil {
		return
	}
	id := cuenta.InstanceID
	t := c.sa.P.T
	var d *ui.Dialogo
	d = ui.Confirmacion(t.T("integrations.bridgeDisconnect", nil), t.T("action.cancel", nil), t.T("integrations.bridgeDisconnect", nil), true, c.texto(t.T("external.disconnectBody", i18n.Vars{"provider": c.nombre()})), func() { d.Cerrar() }, func() {
		d.Cerrar()
		c.ocupado = true
		c.pintar()
		pedirEn(c.ctx, c.sa, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, c.sa.CuentasApps.ExternalChatDisconnect(ctx, id)
		}, func(_ struct{}, err error) {
			if c.cerrado {
				return
			}
			c.ocupado = false
			if err != nil {
				c.errorClave = "appConnection.failed"
				c.pintar()
				return
			}
			c.instancia = ""
			c.cargarCuentas("")
		})
	})
	d.Mostrar(c.lienzo)
}

// A lost write response is never replayed. The person first refreshes the remote
// history; even that read keeps the draft until an explicit later action.
func (c *conversacionExterna) escribir() {
	if c.ocupado || c.cargando || c.incierto || !c.capacidades.Send {
		return
	}
	texto := strings.TrimSpace(c.entrada.Texto())
	if texto == "" || len([]rune(texto)) > 40000 {
		return
	}
	instancia, remoto, hilo, edicion := c.instancia, c.remoto, c.hilo, c.edicion
	c.ocupado, c.errorClave = true, ""
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (api.ExternalChatMessage, error) {
		if edicion != "" {
			return api.ExternalChatMessage{}, c.sa.CuentasApps.ExternalChatAction(ctx, instancia, api.ExternalChatAction{Action: "edit", RemoteID: remoto, MessageID: edicion, ThreadID: hilo, Text: texto})
		}
		return c.sa.CuentasApps.ExternalChatSend(ctx, instancia, api.ExternalChatWrite{RemoteID: remoto, Text: texto, ThreadID: hilo})
	}, func(_ api.ExternalChatMessage, err error) {
		if c.cerrado {
			return
		}
		c.ocupado = false
		if err != nil {
			c.errorClave = "external.sendUnconfirmed"
			c.incierto = true
			c.enfriar(err)
			c.pintar()
			return
		}
		c.entrada.PonerTexto("")
		c.edicion = ""
		c.leer(false, true)
	})
}

func (c *conversacionExterna) enfriar(err error) {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests {
		c.siguienteLectura = time.Now().Add(max(time.Minute, apiErr.RetryAfter()))
	}
}

func (c *conversacionExterna) programarLectura() {
	if c.temporizador != nil {
		c.temporizador.Stop()
	}
	if c.cerrado {
		return
	}
	if c.incierto {
		if demora := time.Until(c.siguienteLectura); demora > 0 {
			c.temporizador = time.AfterFunc(demora, func() {
				fyne.Do(func() {
					if !c.cerrado && c.ctx.Err() == nil {
						c.pintar()
					}
				})
			})
		}
		return
	}
	demora := 15 * time.Second
	if c.proveedor == "slack" {
		demora = time.Minute
	}
	demora = max(demora, time.Until(c.siguienteLectura))
	c.temporizador = time.AfterFunc(demora, func() {
		fyne.Do(func() {
			if !c.cerrado && c.ctx.Err() == nil {
				c.leer(false, false)
			}
		})
	})
}

func (c *conversacionExterna) leer(mas, manual bool) {
	if c.cerrado || c.ocupado || c.cargando || !c.lector {
		return
	}
	if time.Now().Before(c.siguienteLectura) {
		c.programarLectura()
		return
	}
	c.version++
	version := c.version
	instancia, remoto, hilo, cursor := c.instancia, c.remoto, c.hilo, ""
	if mas {
		cursor = c.cursor
	}
	c.cargando = true
	c.pintar()
	pedirEn(c.ctx, c.sa, func(ctx context.Context) (api.ExternalChatMessages, error) {
		return c.sa.CuentasApps.ExternalChatMessages(ctx, instancia, remoto, cursor, hilo)
	}, func(datos api.ExternalChatMessages, err error) {
		if c.cerrado || version != c.version {
			return
		}
		c.cargando = false
		if err != nil {
			c.enfriar(err)
			if !c.incierto {
				c.errorClave = "message.loadFailed"
			}
			c.pintar()
			c.programarLectura()
			return
		}
		c.capacidades = datos.Capabilities
		// Sin permiso de lectura la página siempre llega vacía: se conserva lo enviado desde aquí.
		if datos.Capabilities.Read {
			c.mensajes = mezclarMensajesExternos(c.mensajes, datos.Items, mas, c.proveedor == "slack" && hilo != "")
		}
		if mas || !c.cursorIniciado {
			c.cursor, c.cursorIniciado = datos.NextCursor, true
		}
		if manual {
			c.incierto = false
			c.errorClave = ""
		} else if !c.incierto {
			c.errorClave = ""
		}
		c.pintar()
		c.programarLectura()
	})
}

// A recent page is authoritative only for its window: missing messages there
// were deleted, while previously loaded older history must remain available.
func mezclarMensajesExternos(anteriores, nuevos []api.ExternalChatMessage, mas, ascendente bool) []api.ExternalChatMessage {
	porID := map[string]api.ExternalChatMessage{}
	var limite time.Time
	for _, m := range nuevos {
		if limite.IsZero() || (!ascendente && m.CreatedAt.Before(limite)) || (ascendente && m.CreatedAt.After(limite)) {
			limite = m.CreatedAt
		}
	}
	for _, m := range anteriores {
		if mas || (!limite.IsZero() && ((!ascendente && m.CreatedAt.Before(limite)) || (ascendente && m.CreatedAt.After(limite)))) {
			porID[m.ID] = m
		}
	}
	for _, m := range nuevos {
		porID[m.ID] = m
	}
	resultado := make([]api.ExternalChatMessage, 0, len(porID))
	for _, m := range porID {
		resultado = append(resultado, m)
	}
	slices.SortFunc(resultado, func(a, b api.ExternalChatMessage) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return resultado
}

func (c *conversacionExterna) abrirHilo(id string) {
	if c.ocupado || c.cargando || c.incierto {
		return
	}
	c.hilo, c.edicion, c.cursor = id, "", ""
	c.mensajes = nil
	c.cursorIniciado = false
	c.entrada.PonerTexto("")
	c.leer(false, true)
}

func (c *conversacionExterna) actualizarEnvio() {
	if c.enviarBoton == nil {
		return
	}
	n := len([]rune(strings.TrimSpace(c.entrada.Texto())))
	c.enviarBoton.Inerte = c.ocupado || c.cargando || c.incierto || !c.capacidades.Send || n == 0 || n > 40000
	c.enviarBoton.Refresh()
}

func (c *conversacionExterna) pintarLector() {
	t := c.sa.P.T
	hijos := []fyne.CanvasObject{c.texto(t.T("external.protection", i18n.Vars{"provider": c.nombre()}))}
	if !c.capacidades.Read {
		hijos = append(hijos, c.texto(t.T("external.sendOnly", i18n.Vars{"provider": c.nombre()})))
	}
	if c.hilo != "" {
		hijos = append(hijos, c.boton("thread.close", func() { c.abrirHilo("") }))
	}
	if c.errorClave != "" {
		reintentar := c.boton("action.retry", func() { c.leer(false, true) })
		reintentar.Inerte = reintentar.Inerte || time.Now().Before(c.siguienteLectura)
		hijos = append(hijos, c.texto(t.T(c.errorClave, nil)), reintentar)
	}
	filas := container.New(ui.Columna{Hueco: tema.Espacio(4)})
	if c.cursor != "" {
		filas.Add(c.boton("action.loadMore", func() { c.leer(true, true) }))
	}
	for _, m := range c.mensajes {
		filas.Add(c.mensaje(m))
	}
	if len(c.mensajes) == 0 {
		clave := "message.noneYet"
		if c.cargando {
			clave = "state.loading"
		}
		filas.Add(c.texto(t.T(clave, nil)))
	}
	lista := container.NewVScroll(filas)
	lista.SetMinSize(fyne.NewSize(0, min(260, max(100, c.lienzo.Size().Height*0.35))))
	hijos = append(hijos, lista)
	rotulo := t.T("composer.writeInAria", i18n.Vars{"channel": c.d.Titulo})
	if c.edicion != "" {
		rotulo = t.T("message.edit", nil)
	}
	c.entrada.Rotulo = rotulo
	c.entrada.Inerte(c.ocupado || c.cargando || c.incierto || !c.capacidades.Send)
	c.entrada.Refresh()
	hijos = append(hijos, campoCon(rotulo, c.entrada, ""))
	c.cuerpo.Objects = hijos
	c.cuerpo.Refresh()
	clave := "composer.send"
	if c.edicion != "" {
		clave = "action.save"
	}
	c.enviarBoton = c.boton(clave, c.escribir)
	c.enviarBoton.Con(ui.Primario, ui.TallaMD)
	c.actualizarEnvio()
	cerrar := c.boton("action.close", c.cerrar)
	cerrar.Inerte = false
	botones := []fyne.CanvasObject{cerrar, layout.NewSpacer()}
	if c.edicion != "" {
		botones = append(botones, c.boton("action.cancel", func() { c.edicion = ""; c.entrada.PonerTexto(""); c.pintar() }))
	}
	botones = append(botones, c.enviarBoton)
	c.pie.Objects = botones
	c.pie.Refresh()
	c.d.Refresh()
}

func (c *conversacionExterna) mensaje(m api.ExternalChatMessage) fyne.CanvasObject {
	t := c.sa.P.T
	autor := m.AuthorName
	if autor == "" {
		autor = m.AuthorID
	}
	hijos := []fyne.CanvasObject{c.texto(autor + " · " + RotuloFechaCompleta(c.sa.P.Locale, m.CreatedAt.Local())), c.texto(m.Text)}
	// Separate rows keep actions reachable at 320 px without horizontal scrolling.
	if c.capacidades.Threads && c.hilo == "" {
		b := c.boton("message.replyInThread", func() {
			id := m.ThreadID
			if id == "" {
				id = m.ID
			}
			c.abrirHilo(id)
		})
		b.Inerte = b.Inerte || c.incierto
		hijos = append(hijos, b)
	}
	if m.Owned && c.capacidades.Edit {
		b := c.boton("action.edit", func() { c.edicion = m.ID; c.entrada.PonerTexto(m.Text); c.pintar() })
		b.Inerte = b.Inerte || c.incierto
		hijos = append(hijos, b)
	}
	if m.Owned && c.capacidades.Delete {
		b := c.boton("action.delete", func() { c.confirmarBorrado(m) })
		b.Inerte = b.Inerte || c.incierto
		hijos = append(hijos, b)
	}
	if m.Edited {
		hijos = append(hijos, c.texto(t.T("message.edited", nil)))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
}

func (c *conversacionExterna) confirmarBorrado(m api.ExternalChatMessage) {
	if c.ocupado || c.cargando || c.incierto || !m.Owned || !c.capacidades.Delete {
		return
	}
	t := c.sa.P.T
	var d *ui.Dialogo
	d = ui.Confirmacion(t.T("action.delete", nil), t.T("action.cancel", nil), t.T("action.delete", nil), true, c.texto(m.Text), func() { d.Cerrar() }, func() {
		d.Cerrar()
		c.ocupado = true
		c.pintar()
		instancia, remoto, hilo := c.instancia, c.remoto, c.hilo
		pedirEn(c.ctx, c.sa, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, c.sa.CuentasApps.ExternalChatAction(ctx, instancia, api.ExternalChatAction{Action: "delete", RemoteID: remoto, MessageID: m.ID, ThreadID: hilo})
		}, func(_ struct{}, err error) {
			if c.cerrado {
				return
			}
			c.ocupado = false
			if err != nil {
				c.incierto = true
				c.errorClave = "external.sendUnconfirmed"
				c.enfriar(err)
				c.pintar()
				return
			}
			c.leer(false, true)
		})
	})
	d.Mostrar(c.lienzo)
}
