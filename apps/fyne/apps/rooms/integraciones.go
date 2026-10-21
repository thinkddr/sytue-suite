// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/IntegrationsModal.tsx y
// lib/hookVariants.ts.
//
// «Apps e integraciones», en dos pestañas como la web:
//   - Apps: Slack, Teams, Google Chat y WhatsApp, cada una con su asistente (solo para
//     owner/admin de la organización, que es quien puede dar el consentimiento), y una
//     sola lista con lo conectado (puentes y números de WhatsApp).
//   - Desarrolladores: bots, comandos y webhooks entrantes y salientes, con un único
//     formulario (selector de tipo) y una sola lista.
//
// El secreto en claro se enseña UNA vez, con «Copiar». Ya no hay alta manual de puentes
// ni de números de WhatsApp: la vuelta desde Slack, Teams o Google Chat la cubre un
// webhook entrante, que acepta sus formatos tal cual (VariantesGancho).

// Integraciones es lo que enseña el diálogo (`IntegrationsData`).
type Integraciones struct {
	Entrantes []api.RoomsIncomingWebhook
	Salientes []api.RoomsOutgoingWebhook
	Bots      []api.RoomsBot
	Puentes   []api.RoomsBridge
	WhatsApp  []api.RoomsWhatsApp
	Equipos   []api.RoomsTeam
}

// VarianteGancho es una dirección del mismo webhook entrante (`HookVariant`).
type VarianteGancho struct{ Rotulo, URL string }

// VariantesGancho es `hookVariants` sin el correo (ese lo pone CorreoCanal): el MISMO
// token en las rutas que autodetectan o fuerzan el formato. Si la URL no tiene la forma
// esperada, solo ella.
func VariantesGancho(u string) []VarianteGancho {
	const marca = "/hooks/"
	i := strings.LastIndex(u, marca)
	if i < 0 || i+len(marca) == len(u) {
		return []VarianteGancho{{"Sytue", u}}
	}
	antes, token := u[:i], u[i+len(marca):]
	return []VarianteGancho{
		{"Sytue", u},
		{"Slack", antes + marca + "slack/" + token},
		{"Microsoft Teams", antes + marca + "teams/" + token},
		{"Google Chat", antes + marca + "googlechat/" + token},
	}
}

// CorreoCanal es `channelEmail`: el mismo token como dirección de correo del canal,
// `<canal>+<token>@<dominio>`, con el nombre delante para que se pueda dictar y
// reconocer (lo que autoriza sigue siendo el token). Vacío sin dominio o si la URL no
// tiene la forma esperada: una dirección que rebota es peor que ninguna.
func CorreoCanal(u, dominio, canal string) string {
	const marca = "/hooks/"
	d := strings.Trim(strings.TrimSpace(dominio), "@.")
	i := strings.LastIndex(u, marca)
	if d == "" || i < 0 || i+len(marca) == len(u) {
		return ""
	}
	token := u[i+len(marca):]
	alias := strings.Trim(noAlias.ReplaceAllString(Normalizar(canal), "-"), "-")
	alias = alias[:min(len(alias), 32)] // ASCII ya: cortar bytes es cortar letras
	if alias == "" {
		return token + "@" + d
	}
	return alias + "+" + token + "@" + d
}

// noAlias es lo que no puede ir en la parte local del alias.
var noAlias = regexp.MustCompile(`[^a-z0-9-]+`)

// PalabrasDisparadoras es `parseWords`: separadas por comas o saltos de línea.
func PalabrasDisparadoras(crudo string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(crudo, func(r rune) bool { return r == ',' || r == '\n' }) {
		if w = strings.TrimSpace(w); w != "" {
			out = append(out, w)
		}
	}
	return out
}

// SecretoIntegracion es lo recién creado que se enseña una vez.
type SecretoIntegracion struct {
	Titulo, Valor string
	// Variantes son las otras direcciones del mismo token.
	Variantes []VarianteGancho
}

// Las marcas de un puente y los dialectos de un saliente (las marcas no se traducen).
var (
	nombresPlataforma = map[string]string{"slack": "Slack", "teams": "Microsoft Teams", "googlechat": "Google Chat"}
	dialectos         = []string{"sytue", "slack", "teams"}
)

// appsConectables son las de la pestaña «Apps»: su clave de consentimiento y la marca.
var appsConectables = []struct{ clave, nombre string }{
	{"slack-rooms", "Slack"},
	{"microsoft-teams-rooms", "Microsoft Teams"},
	{"google-chat", "Google Chat"},
	{"whatsapp-business", "WhatsApp Business"},
}

// Los tipos de «Desarrolladores», en el orden del selector de la web: su rótulo, su
// ayuda y qué datos entran y salen (`DataFlow` compacto).
var tiposDesarrollo = []struct {
	clave, rotulo, ayuda string
	entra, sale          []string
}{
	{"bot", "integrations.bots", "integrations.botsHelp", []string{"messages"}, nil},
	{"command", "integrations.commands", "integrations.commandsHelp", []string{"messages"}, []string{"messages"}},
	{"incoming", "integrations.incoming", "integrations.incomingHelp", []string{"messages"}, nil},
	{"outgoing", "integrations.outgoing", "integrations.outgoingHelp", nil, []string{"messages"}},
}

// Las pestañas, en el orden de la web.
const (
	PestanaApps = iota
	PestanaDesarrolladores
)

// CuerpoIntegraciones es el contenido del diálogo.
type CuerpoIntegraciones struct {
	widget.BaseWidget
	T *i18n.Translator

	Datos    Integraciones
	Comandos []api.RoomsCommand
	// Canales son los publicables (sin directos); Canal, el elegido.
	Canales []api.RoomsChannel
	Canal   string
	// AdminOrg: owner/admin de la organización, el único que conecta apps.
	AdminOrg bool
	// Cargando, Error ("forbidden" o "load") y Ocupado son los de la web.
	Cargando bool
	Error    string
	Ocupado  bool
	Secreto  *SecretoIntegracion
	// Tipo es lo elegido en «Desarrolladores» y Dialecto, el del saliente.
	Tipo, Dialecto string

	AlCanal             func(id string)
	AlCrearEntrante     func(nombre string)
	AlRotar             func(id string)
	AlBorrarEntrante    func(id string)
	AlCrearSaliente     func(api.RoomsOutgoingWebhook)
	AlBorrarSaliente    func(id string)
	AlCrearBot          func(nombre string)
	AlBorrarBot         func(id string)
	AlBotEnCanal        func(bot string, dentro bool)
	AlBorrarPuente      func(id string)
	AlDesconectarNumero func(id string)
	AlCrearComando      func(api.RoomsCommand)
	AlBorrarComando     func(id string)
	AlCopiar            func(valor string)
	// AlConectarApp abre el asistente nativo de esa app; AlPolitica, la política de
	// privacidad del `DataFlow`.
	AlConectarApp func(clave string)
	AlPolitica    func()

	// ancho es el del cuerpo: el `DataFlow` se construye a él.
	ancho float32
	// telefono es el `max-md:` de la web: el lienzo por debajo de 768.
	telefono bool

	pestanas *ui.Pestanas
	campos   map[string]*ui.Entrada
	columna  *fyne.Container
}

var _ fyne.Widget = (*CuerpoIntegraciones)(nil)

// NuevoCuerpoIntegraciones crea el contenido, cargando.
func NuevoCuerpoIntegraciones(t *i18n.Translator) *CuerpoIntegraciones {
	c := &CuerpoIntegraciones{T: t, Cargando: true, Tipo: "bot", Dialecto: "sytue", campos: map[string]*ui.Entrada{}}
	c.ExtendBaseWidget(c)
	c.pestanas = ui.NuevasPestanas([]string{t.T("integrations.tab.apps", nil), t.T("integrations.tab.developers", nil)}, nil)
	c.pestanas.AlCambiar = func(int) { c.Refresh() }
	// Un juego de campos para los cuatro tipos, como el formulario único de la web.
	for _, clave := range []string{"name", "url", "words", "desc"} {
		c.campos[clave] = c.entrada("", "")
	}
	return c
}

// entrada es un campo del formulario que repinta al escribir.
func (c *CuerpoIntegraciones) entrada(marca, texto string) *ui.Entrada {
	e := ui.NuevaEntrada(marca)
	e.PonerTexto(texto)
	e.AlCambiar(func(string) { c.Refresh() })
	return e
}

// Campo es el campo de esa clave, para los guardianes.
func (c *CuerpoIntegraciones) Campo(clave string) *ui.Entrada { return c.campos[clave] }

// Pestanas es la tira, para los guardianes.
func (c *CuerpoIntegraciones) Pestanas() *ui.Pestanas { return c.pestanas }

// Limpiar vacía el formulario tras crear o al cambiar de tipo.
func (c *CuerpoIntegraciones) Limpiar() {
	for _, e := range c.campos {
		e.PonerTexto("")
	}
	c.Dialecto = "sytue"
}

func (c *CuerpoIntegraciones) texto(clave string) string {
	return strings.TrimSpace(c.campos[clave].Texto())
}

// nombreCanal es `channelName`.
func (c *CuerpoIntegraciones) nombreCanal(id string) string {
	for _, ch := range c.Canales {
		if ch.ID == id {
			return ch.Title()
		}
	}
	return c.T.T("channel.word", nil)
}

// CreateRenderer monta la columna.
func (c *CuerpoIntegraciones) CreateRenderer() fyne.WidgetRenderer {
	c.columna = container.New(ui.Columna{Hueco: tema.Espacio(4)})
	r := &cuerpoIntegracionesRenderer{c: c}
	r.Refresh()
	return r
}

type cuerpoIntegracionesRenderer struct{ c *CuerpoIntegraciones }

func (r *cuerpoIntegracionesRenderer) MinSize() fyne.Size { return r.c.columna.MinSize() }
func (r *cuerpoIntegracionesRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.c.columna}
}
func (r *cuerpoIntegracionesRenderer) Destroy() {}
func (r *cuerpoIntegracionesRenderer) Layout(t fyne.Size) {
	if t.Width != r.c.ancho {
		r.c.ancho = t.Width
		r.Refresh()
	}
	r.c.columna.Resize(t)
}

func (r *cuerpoIntegracionesRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := c.T
	var hijos []fyne.CanvasObject
	if c.Secreto != nil {
		hijos = append(hijos, c.bloqueSecreto(v))
	}
	if c.Error != "" {
		clave, token := "integrations.loadFailed", "danger-text"
		if c.Error == "forbidden" {
			clave, token = "integrations.forbidden", "text-muted"
		}
		hijos = append(hijos, ui.NuevoParrafo(t.T(clave, nil), tema.TextoSM, ui.Normal, token, v))
	}
	// Un solo selector de canal para las dos pestañas: puentes, entrantes y «añadir
	// bot» son de un canal; los salientes, opcional.
	hijos = append(hijos, campoCon(t.T("channel.word.capitalised", nil), c.selectorCanal(), ""))
	var panel fyne.CanvasObject
	switch {
	case c.Cargando:
		panel = ui.NuevoRotulo(t.T("state.loading", nil), tema.TextoSM, ui.Normal, "text-muted")
	case c.pestanas.Activa == PestanaApps:
		panel = c.apps(v)
	default:
		panel = c.desarrolladores(v)
	}
	paneles := make([]fyne.CanvasObject, len(c.pestanas.Rotulos))
	// `min-h-[12rem]`: no salta al cambiar de pestaña.
	paneles[c.pestanas.Activa] = container.New(altoMinimo{12 * tema.Rem}, panel)
	c.pestanas.Paneles = paneles
	c.pestanas.Refresh()
	hijos = append(hijos, c.pestanas)
	c.columna.Objects = hijos
	c.columna.Refresh()
}

// altoMinimo da a su hijo al menos ese alto.
type altoMinimo struct{ alto float32 }

func (a altoMinimo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, max(m.Height, a.alto))
}

func (a altoMinimo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(fyne.NewSize(t.Width, objs[0].MinSize().Height))
}

// ---- Piezas ----

// campoCon es un `<label class="sytue-label font-medium">` sobre su control, `gap-1`.
func campoCon(rotulo string, control fyne.CanvasObject, ayuda string) fyne.CanvasObject {
	hijos := []fyne.CanvasObject{ui.NuevoRotulo(rotulo, tema.TextoSM, ui.Medio, "text"), control}
	if ayuda != "" {
		hijos = append(hijos, ui.NuevoRotulo(ayuda, tema.TextoXS, ui.Normal, "text-muted"))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
}

// botonFantasma es un `Button size="sm" variant="ghost"`.
func botonFantasma(s string, inerte bool, f func()) *ui.Boton {
	b := ui.NuevoBoton(s, f).Con(ui.Fantasma, ui.TallaSM)
	b.Inerte = inerte
	return b
}

// fila es `Row`: `rounded-md border px-3 py-2`, lo suyo a la izquierda y las acciones a
// la derecha. En el teléfono (`max-md:flex-col`), lo suyo entero arriba y las acciones
// debajo, a la derecha: con tres acciones al lado, el nombre de un bot se quedaba en 53.
func (c *CuerpoIntegraciones) fila(v tema.Variante, izq []fyne.CanvasObject, acciones ...fyne.CanvasObject) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	texto := container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, izq...)
	botones := container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, acciones...)
	var linea fyne.CanvasObject = container.New(filaFlexible{hueco: tema.Espacio(3)}, texto, botones)
	if c.telefono {
		linea = container.New(ui.Columna{Hueco: tema.Espacio(2)}, texto,
			container.New(filaFlexible{hueco: tema.Espacio(3)}, layout.NewSpacer(), botones))
	}
	dentro := container.New(ui.Margen{Izq: tema.Espacio(3), Arr: tema.Espacio(2), Der: tema.Espacio(3), Abj: tema.Espacio(2)}, linea)
	return container.New(enCaja{caja}, append(caja.Objetos(), dentro)...)
}

// listaIntegraciones es `flex flex-col gap-1.5`, o el texto de vacío.
func listaIntegraciones(vacio string, filas []fyne.CanvasObject) fyne.CanvasObject {
	if len(filas) == 0 {
		return ui.NuevoRotulo(vacio, tema.TextoSM, ui.Normal, "text-subtle")
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...)
}

// cabezaFila es el nombre de una fila con su insignia (el tipo, el estado…).
func cabezaFila(nombre string, insignias ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, append([]fyne.CanvasObject{ui.NuevoRotulo(nombre, tema.TextoSM, ui.Medio, "text")}, insignias...)...)
}

// selectorCanal es `ChannelSelect`.
func (c *CuerpoIntegraciones) selectorCanal() *ui.Selector {
	ops := make([]string, 0, len(c.Canales))
	elegida := -1
	for i, ch := range c.Canales {
		ops = append(ops, ch.Title())
		if ch.ID == c.Canal {
			elegida = i
		}
	}
	if len(ops) == 0 {
		ops = []string{c.T.T("integrations.noChannels", nil)}
	}
	s := ui.NuevoSelector(ops, func(i int) {
		if i < len(c.Canales) && c.AlCanal != nil {
			c.AlCanal(c.Canales[i].ID)
		}
	})
	s.Elegida = max(elegida, 0)
	s.Ancho = 48 * 0.25 * tema.Rem // `w-48`
	if c.telefono {
		s.Ancho = c.ancho // `max-md:w-full`
	}
	return s
}

// selectorDe elige entre valores con sus rótulos.
func selectorDe(valores []string, rotulo func(string) string, actual string, al func(string)) *ui.Selector {
	ops := make([]string, len(valores))
	elegida := 0
	for i, x := range valores {
		ops[i] = rotulo(x)
		if x == actual {
			elegida = i
		}
	}
	s := ui.NuevoSelector(ops, func(i int) { al(valores[i]) })
	s.Elegida = elegida
	return s
}

// lineaSecreto es un `<code>` de una línea con su «Copiar».
func (c *CuerpoIntegraciones) lineaSecreto(valor, rotuloCopiar string, delante ...fyne.CanvasObject) fyne.CanvasObject {
	copiar := botonFantasma(c.T.T("action.copy", nil), false, func() {
		if c.AlCopiar != nil {
			c.AlCopiar(valor)
		}
	})
	copiar.Rotulo = rotuloCopiar // el `aria-label` manda sobre «Copiar»
	hijos := append([]fyne.CanvasObject{}, delante...)
	hijos = append(hijos, NuevoCodigoEnLinea(valor), copiar)
	return container.New(filaFlexibleEn{hueco: tema.Espacio(2), flexible: len(delante)}, hijos...)
}

// secretoFirma es la línea del secreto de firma de un saliente o un comando: se ve en
// la fila y no «una sola vez», porque hay que poder recuperarlo al configurar el otro
// lado y nunca viaja en las llamadas.
func (c *CuerpoIntegraciones) secretoFirma(valor string) fyne.CanvasObject {
	t := c.T
	return c.lineaSecreto(valor, t.T("integrations.copySigningSecret", nil),
		ui.NuevaPista(t.T("integrations.signingSecretWhat", nil), ui.NuevoRotulo(t.T("integrations.signingSecret", nil), tema.TextoXS, ui.Normal, "text-muted")))
}

// bloqueSecreto es el aviso `border-warning bg-warning-bg` del secreto recién creado.
func (c *CuerpoIntegraciones) bloqueSecreto(v tema.Variante) fyne.CanvasObject {
	t, s := c.T, c.Secreto
	copiar := ui.NuevoBoton(t.T("action.copy", nil), func() {
		if c.AlCopiar != nil {
			c.AlCopiar(s.Valor)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	ocultar := botonFantasma(t.T("action.hide", nil), false, func() {
		c.Secreto = nil
		c.Refresh()
	})
	hijos := []fyne.CanvasObject{
		ui.NuevoRotulo(s.Titulo, tema.TextoSM, ui.SemiNegrita, "warning-text"),
		ui.NuevoRotulo(t.T("integrations.secretOnce", nil), tema.TextoXS, ui.Normal, "text-muted"),
		container.New(filaFlexible{hueco: tema.Espacio(2)}, NuevoCodigoEnLinea(s.Valor), copiar, ocultar),
	}
	if len(s.Variantes) > 1 {
		hijos = append(hijos, ui.NuevoParrafo(t.T("integrations.incomingCompat", nil), tema.TextoXS, ui.Normal, "text-muted", v))
		for _, va := range s.Variantes[1:] {
			rotulo := container.New(anchoFijo(28*0.25*tema.Rem), ui.NuevoRotulo(va.Rotulo, tema.TextoXS, ui.Normal, "text-muted"))
			hijos = append(hijos, c.lineaSecreto(va.URL, t.T("integrations.copyFor", i18n.Vars{"provider": va.Rotulo}), rotulo))
		}
	}
	caja := ui.NuevaCaja(tema.Color("warning-bg", v), tema.Color("warning", v), tema.RadioMD)
	dentro := container.New(ui.Margen{Izq: tema.Espacio(3), Arr: tema.Espacio(3), Der: tema.Espacio(3), Abj: tema.Espacio(3)},
		container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...))
	return container.New(enCaja{caja}, append(caja.Objetos(), dentro)...)
}

// ---- Pestañas ----

// apps es la pestaña «Apps»: la línea de qué hace conectar, los botones de cada app
// (`grid-cols-2 gap-2 sm:grid-cols-4`) o, a quien no administra la organización, una
// línea que dice quién lo hace; y la lista de lo conectado.
func (c *CuerpoIntegraciones) apps(v tema.Variante) fyne.CanvasObject {
	t := c.T
	hijos := []fyne.CanvasObject{ui.NuevoParrafo(t.T("integrations.connectApps.body", nil), tema.TextoXS, ui.Normal, "text-muted", v)}
	if c.AdminOrg {
		columnas := 4
		if lienzo := fyne.CurrentApp().Driver().CanvasForObject(c); lienzo != nil && lienzo.Size().Width < ui.CorteSM {
			columnas = 2
		}
		botones := make([]fyne.CanvasObject, len(appsConectables))
		for i, a := range appsConectables {
			b := ui.NuevoBoton(a.nombre, func() {
				if c.AlConectarApp != nil {
					c.AlConectarApp(a.clave)
				}
			}).Con(ui.Secundario, ui.TallaMD)
			b.Rotulo = t.T("appConnection.continue", i18n.Vars{"provider": a.nombre})
			botones[i] = b
		}
		hijos = append(hijos, container.New(ui.Rejilla{Fijas: columnas, Hueco: tema.Espacio(2)}, botones...))
	} else {
		hijos = append(hijos, ui.NuevoParrafo(t.T("integrations.connectApps.adminOnly", nil), tema.TextoSM, ui.Normal, "text-muted", v))
	}
	equipos := map[string]string{}
	for _, e := range c.Datos.Equipos {
		equipos[e.ID] = e.Name
	}
	filas := make([]fyne.CanvasObject, 0, len(c.Datos.Puentes)+len(c.Datos.WhatsApp))
	desconectar := t.T("integrations.bridgeDisconnect", nil)
	for _, b := range c.Datos.Puentes {
		tono, estado := ui.InsigniaExito, t.T("integrations.bridgeOk", nil)
		if b.LastError != "" {
			tono, estado = ui.InsigniaPeligro, t.T("integrations.bridgeFailing", nil)
		}
		izq := []fyne.CanvasObject{
			cabezaFila(nombresPlataforma[b.Platform], ui.NuevaInsignia(estado, tono)),
			ui.NuevoRotulo("#"+c.nombreCanal(b.ChannelID)+" → "+b.Target, escalonMeta, ui.Normal, "text-muted"),
		}
		if b.LastError != "" {
			izq = append(izq, ui.NuevoRotulo(b.LastError, tema.TextoXS, ui.Normal, "danger-text"))
		}
		filas = append(filas, c.fila(v, izq, botonFantasma(desconectar, c.Ocupado, func() { c.AlBorrarPuente(b.ID) })))
	}
	for _, a := range c.Datos.WhatsApp {
		nombre := a.DisplayPhone
		if nombre == "" {
			nombre = a.PhoneNumberID
		}
		if a.VerifiedName != "" {
			nombre += " · " + a.VerifiedName
		}
		filas = append(filas, c.fila(v, []fyne.CanvasObject{
			ui.NuevoRotulo("WhatsApp · "+nombre, tema.TextoSM, ui.Medio, "text"),
			ui.NuevoRotulo(equipos[a.TeamID], escalonMeta, ui.Normal, "text-muted"),
		}, botonFantasma(desconectar, c.Ocupado, func() { c.AlDesconectarNumero(a.ID) })))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, append(hijos, listaIntegraciones(t.T("integrations.appsEmpty", nil), filas))...)
}

// tipo es la fila de tiposDesarrollo del tipo elegido.
func (c *CuerpoIntegraciones) tipo() int {
	for i, x := range tiposDesarrollo {
		if x.clave == c.Tipo {
			return i
		}
	}
	return 0
}

// formulario son los campos del tipo elegido y si se puede crear con lo escrito.
func (c *CuerpoIntegraciones) formulario() ([]fyne.CanvasObject, bool) {
	t := c.T
	campo := func(clave, rotulo, marca, ayuda string) fyne.CanvasObject {
		c.campos[clave].PonerMarcaDeAgua(marca)
		return campoCon(t.T(rotulo, nil), c.campos[clave], ayuda)
	}
	switch c.Tipo {
	case "command":
		// Se enseña el resultado, no la regla: «así se escribirá».
		ayuda := t.T("integrations.commandNameHint", nil)
		if n := strings.TrimPrefix(c.texto("name"), "/"); n != "" {
			ayuda = t.T("integrations.commandNameLive", i18n.Vars{"name": n})
		}
		return []fyne.CanvasObject{
			campo("name", "integrations.commandName", "desplegar", ayuda),
			campo("url", "integrations.commandUrl", "https://ejemplo.test/comandos", t.T("integrations.commandUrlHint", nil)),
			campo("desc", "integrations.commandDescription", t.T("integrations.commandDescriptionHint", nil), ""),
		}, c.texto("name") != "" && c.texto("url") != ""
	case "incoming":
		return []fyne.CanvasObject{campo("name", "createChannel.name", t.T("integrations.incomingNamePlaceholder", nil), "")}, c.Canal != ""
	case "outgoing":
		dialecto := selectorDe(dialectos, func(d string) string {
			if d == "sytue" {
				return t.T("integrations.dialect.sytue", nil)
			}
			return nombresPlataforma[d]
		}, c.Dialecto, func(d string) { c.Dialecto = d; c.Refresh() })
		return []fyne.CanvasObject{
			campo("url", "integrations.callbackUrl", "https://example.com/hook", ""),
			campo("words", "integrations.triggerWords", "deploy, status", ""),
			campoCon(t.T("integrations.dialect", nil), dialecto, ""),
		}, c.texto("url") != ""
	default:
		return []fyne.CanvasObject{campo("name", "integrations.botName", t.T("integrations.botNamePlaceholder", nil), "")}, c.texto("name") != ""
	}
}

// crear crea lo elegido con lo escrito.
func (c *CuerpoIntegraciones) crear() {
	switch c.Tipo {
	case "command":
		c.AlCrearComando(api.RoomsCommand{Name: c.texto("name"), RequestURL: c.texto("url"), Description: c.texto("desc")})
	case "incoming":
		c.AlCrearEntrante(c.texto("name"))
	case "outgoing":
		c.AlCrearSaliente(api.RoomsOutgoingWebhook{
			CallbackURL: c.texto("url"), ChannelID: c.Canal,
			TriggerWords: PalabrasDisparadoras(c.campos["words"].Texto()), Dialect: c.Dialecto,
		})
	default:
		c.AlCrearBot(c.texto("name"))
	}
}

// desarrolladores es la pestaña «Desarrolladores»: la ayuda y el `DataFlow` del tipo
// elegido, un formulario (tipo + sus campos + Crear) y UNA lista con todo.
func (c *CuerpoIntegraciones) desarrolladores(v tema.Variante) fyne.CanvasObject {
	t := c.T
	elegido := tiposDesarrollo[c.tipo()]
	claves := make([]string, len(tiposDesarrollo))
	rotulos := map[string]string{}
	for i, x := range tiposDesarrollo {
		claves[i], rotulos[x.clave] = x.clave, t.T(x.rotulo, nil)
	}
	tipo := selectorDe(claves, func(k string) string { return rotulos[k] }, c.Tipo, func(k string) {
		c.Tipo = k
		c.Limpiar()
		c.Refresh()
	})
	campos, valido := c.formulario()
	crear := ui.NuevoBoton(t.T("action.create", nil), c.crear).Con(ui.Primario, ui.TallaSM)
	crear.Inerte = c.Ocupado || !valido
	insignia := func(clave string) fyne.CanvasObject { return ui.NuevaInsignia(rotulos[clave], ui.InsigniaNeutra) }
	borrar := t.T("action.delete", nil)

	filas := make([]fyne.CanvasObject, 0, len(c.Datos.Bots)+len(c.Comandos)+len(c.Datos.Entrantes)+len(c.Datos.Salientes))
	sinCanal := c.Ocupado || c.Canal == ""
	for _, b := range c.Datos.Bots {
		filas = append(filas, c.fila(v, []fyne.CanvasObject{cabezaFila(b.Name, insignia("bot"))},
			botonFantasma(t.T("integrations.addToChannel", i18n.Vars{"channel": c.nombreCanal(c.Canal)}), sinCanal, func() { c.AlBotEnCanal(b.ID, true) }),
			botonFantasma(t.T("action.remove", nil), sinCanal, func() { c.AlBotEnCanal(b.ID, false) }),
			botonFantasma(borrar, c.Ocupado, func() { c.AlBorrarBot(b.ID) })))
	}
	for _, k := range c.Comandos {
		meta := k.RequestURL
		if k.Description != "" {
			meta = k.Description + " · " + meta
		}
		izq := []fyne.CanvasObject{cabezaFila("/"+k.Name, insignia("command")), ui.NuevoRotulo(meta, escalonMeta, ui.Normal, "text-muted")}
		if k.SigningSecret != "" {
			izq = append(izq, c.secretoFirma(k.SigningSecret))
		}
		filas = append(filas, c.fila(v, izq, botonFantasma(borrar, c.Ocupado, func() { c.AlBorrarComando(k.ID) })))
	}
	for _, w := range c.Datos.Entrantes {
		nombre := w.Name
		if nombre == "" {
			nombre = "Webhook"
		}
		filas = append(filas, c.fila(v, []fyne.CanvasObject{
			cabezaFila(nombre, insignia("incoming")),
			ui.NuevoRotulo("#"+c.nombreCanal(w.ChannelID), escalonMeta, ui.Normal, "text-muted"),
		},
			botonFantasma(t.T("action.rotate", nil), c.Ocupado, func() { c.AlRotar(w.ID) }),
			botonFantasma(borrar, c.Ocupado, func() { c.AlBorrarEntrante(w.ID) })))
	}
	for _, w := range c.Datos.Salientes {
		cabeza := []fyne.CanvasObject{insignia("outgoing")}
		// Dos salientes a la misma URL con dialecto distinto no son lo mismo: solo se
		// etiqueta lo que no es nativo.
		if w.Dialect == "slack" || w.Dialect == "teams" {
			cabeza = append(cabeza, ui.NuevaInsignia(map[string]string{"slack": "Slack", "teams": "Teams"}[w.Dialect], ui.InsigniaNeutra))
		}
		meta := t.T("integrations.anyChannel", nil)
		if w.ChannelID != "" {
			meta = "#" + c.nombreCanal(w.ChannelID)
		}
		if len(w.TriggerWords) > 0 {
			meta += " · " + strings.Join(w.TriggerWords, ", ")
		}
		izq := []fyne.CanvasObject{cabezaFila(w.CallbackURL, cabeza...), ui.NuevoRotulo(meta, escalonMeta, ui.Normal, "text-muted")}
		if w.SigningSecret != "" {
			izq = append(izq, c.secretoFirma(w.SigningSecret))
		}
		filas = append(filas, c.fila(v, izq, botonFantasma(borrar, c.Ocupado, func() { c.AlBorrarSaliente(w.ID) })))
	}
	formulario := append([]fyne.CanvasObject{campoCon(t.T("integrations.kind", nil), tipo, "")}, campos...)
	return container.New(ui.Columna{Hueco: tema.Espacio(4)},
		ui.NuevoParrafo(t.T(elegido.ayuda, nil), tema.TextoXS, ui.Normal, "text-muted", v),
		c.flujo(elegido.entra, elegido.sale),
		container.New(filaFormulario{hueco: tema.Espacio(2), fijos: 1}, append(formulario, crear)...),
		listaIntegraciones(t.T("integrations.developersEmpty", nil), filas))
}

// flujo es el `DataFlow compact` del tipo elegido, al ancho del cuerpo.
func (c *CuerpoIntegraciones) flujo(entra, sale []string) fyne.CanvasObject {
	return ui.FlujoDatos(c.T, "", entra, sale, true, c.AlPolitica, max(c.ancho, 1))
}

// ---- Maquetación ----

// filaFormulario es `flex items-end gap-2`: los `fijos` primeros a su ancho, los
// flexibles repartiéndose lo que queda (`flex-1`) —todos por igual con `iguales`— y
// los del final (el botón) a su ancho, todos pegados abajo.
//
// ponytail: sin el `flex-wrap`; el diálogo es `lg` y cabe.
type filaFormulario struct {
	hueco   float32
	fijos   int
	iguales bool
}

// flexibles dice qué hijos se estiran: entre los fijos de delante y el botón del final.
func (f filaFormulario) flexibles(objs []fyne.CanvasObject) (desde, hasta int) {
	hasta = len(objs)
	if _, ok := objs[len(objs)-1].(*ui.Boton); ok && !f.iguales {
		hasta--
	}
	if f.iguales {
		if _, ok := objs[len(objs)-1].(*ui.Boton); ok {
			hasta--
		}
	}
	return min(f.fijos, hasta), hasta
}

func (f filaFormulario) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += f.hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaFormulario) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	desde, hasta := f.flexibles(objs)
	resto := t.Width - f.hueco*float32(len(objs)-1)
	for i, o := range objs {
		if i < desde || i >= hasta {
			resto -= o.MinSize().Width
		}
	}
	cada := max(resto, 0) / float32(max(hasta-desde, 1))
	var x float32
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i >= desde && i < hasta {
			w = cada
		}
		o.Move(fyne.NewPos(x, t.Height-m.Height))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + f.hueco
	}
}

// filaFlexibleEn es filaFlexible con el hijo flexible en la posición `flexible`: los de
// antes y después a su ancho.
type filaFlexibleEn struct {
	hueco    float32
	flexible int
}

func (f filaFlexibleEn) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return filaFormulario{hueco: f.hueco}.MinSize(objs)
}

func (f filaFlexibleEn) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	resto := t.Width - f.hueco*float32(len(objs)-1)
	for i, o := range objs {
		if i != f.flexible {
			resto -= o.MinSize().Width
		}
	}
	var x float32
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i == f.flexible {
			w = max(resto, 0)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + f.hueco
	}
}

// filaCentradaRooms es `flex items-center gap-2` sin estirar a nadie.
type filaCentradaRooms struct{ hueco float32 }

func (f filaCentradaRooms) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return filaFormulario{hueco: f.hueco}.MinSize(objs)
}

func (f filaCentradaRooms) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.hueco
	}
}

// ---- El código en línea ----

// CodigoEnLinea es el `<code class="truncate rounded border px-2 py-1 text-xs">` de un
// secreto: una línea en la monoespaciada, recortada con elipsis.
type CodigoEnLinea struct {
	widget.BaseWidget
	Valor string
}

// NuevoCodigoEnLinea crea el código.
func NuevoCodigoEnLinea(valor string) *CodigoEnLinea {
	c := &CodigoEnLinea{Valor: valor}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el valor.
func (c *CodigoEnLinea) AccessibilityLabel() string { return c.Valor }

// AccessibilityRole implementa fyne.Accessible.
func (c *CodigoEnLinea) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer pinta la caja y el texto.
func (c *CodigoEnLinea) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cara, estilo := caraMono()
	tx := canvas.NewText(c.Valor, tema.Color("text", v))
	tx.TextSize, tx.TextStyle, tx.FontSource = tema.TextoXS.Tam, estilo, cara
	return &codigoEnLineaRenderer{c: c, caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM), texto: tx}
}

type codigoEnLineaRenderer struct {
	c     *CodigoEnLinea
	caja  *ui.Caja
	texto *canvas.Text
}

func (r *codigoEnLineaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(4*tema.Espacio(2), tema.TextoXS.Linea+2*tema.Espacio(1)+2*tema.Borde)
}

func (r *codigoEnLineaRenderer) Objects() []fyne.CanvasObject {
	return append(r.caja.Objetos(), r.texto)
}
func (r *codigoEnLineaRenderer) Destroy() {}
func (r *codigoEnLineaRenderer) Refresh() { r.Layout(r.c.Size()) }

func (r *codigoEnLineaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	cabe := t.Width - 2*tema.Espacio(2) - 2*tema.Borde
	s := []rune(r.c.Valor)
	texto := r.c.Valor
	for len(s) > 0 && medirMono(texto, tema.TextoXS) > cabe {
		s = s[:len(s)-1]
		texto = string(s) + "…"
	}
	r.texto.Text = texto
	r.texto.Refresh()
	ui.ColocarTexto(r.texto, tema.TextoXS, tema.Borde+tema.Espacio(2), (t.Height-tema.TextoXS.Linea)/2)
}

// ---- En la Sala ----

// Integrador es lo que el diálogo pide al servicio; lo cumple *api.Rooms.
type Integrador interface {
	IncomingWebhooks(ctx context.Context, canal string) ([]api.RoomsIncomingWebhook, error)
	CreateIncomingWebhook(ctx context.Context, canal, nombre, icono string) (api.RoomsIncomingSecret, error)
	RotateIncomingWebhook(ctx context.Context, id string) (api.RoomsIncomingSecret, error)
	DeleteIncomingWebhook(ctx context.Context, id string) error
	OutgoingWebhooks(ctx context.Context) ([]api.RoomsOutgoingWebhook, error)
	CreateOutgoingWebhook(ctx context.Context, w api.RoomsOutgoingWebhook) (api.RoomsOutgoingWebhook, error)
	DeleteOutgoingWebhook(ctx context.Context, id string) error
	Bots(ctx context.Context) ([]api.RoomsBot, error)
	CreateBot(ctx context.Context, nombre string) (api.RoomsBot, string, error)
	DeleteBot(ctx context.Context, id string) error
	SetBotInChannel(ctx context.Context, bot, canal string, dentro bool) error
	Bridges(ctx context.Context, canal string) ([]api.RoomsBridge, error)
	DeleteBridge(ctx context.Context, id string) error
	WhatsApp(ctx context.Context) ([]api.RoomsWhatsApp, error)
	DisconnectWhatsApp(ctx context.Context, id string) error
	CreateCommand(ctx context.Context, c api.RoomsCommand) (api.RoomsCommand, error)
	DeleteCommand(ctx context.Context, id string) error
}

// pedirIntegraciones es el GET de /api/integrations: lo del canal solo si hay canal.
func (sa *Sala) pedirIntegraciones(in Integrador, canal string) func(context.Context) (Integraciones, error) {
	return func(ctx context.Context) (Integraciones, error) {
		var d Integraciones
		var err error
		if canal != "" {
			if d.Entrantes, err = in.IncomingWebhooks(ctx, canal); err != nil {
				return d, err
			}
			if d.Puentes, err = in.Bridges(ctx, canal); err != nil {
				return d, err
			}
		}
		if d.Salientes, err = in.OutgoingWebhooks(ctx); err != nil {
			return d, err
		}
		if d.Bots, err = in.Bots(ctx); err != nil {
			return d, err
		}
		if d.WhatsApp, err = in.WhatsApp(ctx); err != nil {
			return d, err
		}
		d.Equipos, err = sa.S.Teams(ctx)
		return d, err
	}
}

// AbrirIntegraciones es `IntegrationsModal`.
//
//nolint:gocognit // El modal coordina carga, mutaciones y los callbacks de cada sección.
func (sa *Sala) AbrirIntegraciones() {
	in, ok := sa.S.(Integrador)
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if !ok || lienzo == nil {
		return
	}
	t := sa.P.T
	c := NuevoCuerpoIntegraciones(t)
	c.AdminOrg = sa.AdminOrg
	var d *ui.Dialogo
	for _, k := range sa.canales {
		if k.Kind != "direct" && !k.Archived {
			c.Canales = append(c.Canales, k)
			if k.ID == sa.abierto || c.Canal == "" {
				c.Canal = k.ID
			}
		}
	}
	recargar := func() {
		c.Cargando, c.Error = true, ""
		c.Refresh()
		pedir(sa, sa.pedirIntegraciones(in, c.Canal), func(d Integraciones, err error) {
			c.Cargando = false
			switch {
			case estadoDe(err) == SinPermiso:
				c.Error = "forbidden"
			case err != nil:
				c.Error = "load"
			default:
				c.Datos = d
			}
			c.Refresh()
		})
	}
	comandos := func() {
		pedir(sa, sa.S.Commands, func(ks []api.RoomsCommand, _ error) {
			c.Comandos = ks
			c.Refresh()
		})
	}
	// correr es `run`: una a la vez, «no se pudo» si falla y, si no, lo suyo y recargar.
	correr := func(f func(context.Context) error, alAcabar func()) {
		if c.Ocupado {
			return
		}
		c.Ocupado = true
		c.Refresh()
		pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, f(ctx) }, func(_ struct{}, err error) {
			c.Ocupado = false
			if err != nil {
				c.Refresh()
				sa.P.Avisar(ui.AvisoPeligro, t.T("integrations.actionFailed", nil))
				return
			}
			if alAcabar != nil {
				alAcabar()
			}
			recargar()
		})
	}
	secretoEntrante := func(titulo string, s api.RoomsIncomingSecret) {
		vs := VariantesGancho(s.URL)
		// El dominio lo publica el servicio con el secreto, y solo si el correo a canal
		// está encendido: sin él no se ofrece la dirección.
		if correo := CorreoCanal(s.URL, s.ChannelMailDomain, c.nombreCanal(c.Canal)); correo != "" {
			vs = append(vs, VarianteGancho{t.T("integrations.channelEmail", nil), correo})
		}
		c.Secreto = &SecretoIntegracion{Titulo: titulo, Valor: s.URL, Variantes: vs}
	}
	c.AlCanal = func(id string) { c.Canal = id; recargar() }
	c.AlCrearEntrante = func(nombre string) {
		var s api.RoomsIncomingSecret
		correr(func(ctx context.Context) (err error) {
			s, err = in.CreateIncomingWebhook(ctx, c.Canal, nombre, "")
			return err
		}, func() { secretoEntrante(t.T("integrations.incomingUrl", nil), s); c.Limpiar() })
	}
	c.AlRotar = func(id string) {
		var s api.RoomsIncomingSecret
		correr(func(ctx context.Context) (err error) {
			s, err = in.RotateIncomingWebhook(ctx, id)
			return err
		}, func() { secretoEntrante(t.T("integrations.incomingUrlNew", nil), s) })
	}
	c.AlBorrarEntrante = func(id string) {
		correr(func(ctx context.Context) error { return in.DeleteIncomingWebhook(ctx, id) }, nil)
	}
	c.AlCrearSaliente = func(w api.RoomsOutgoingWebhook) {
		correr(func(ctx context.Context) error {
			_, err := in.CreateOutgoingWebhook(ctx, w)
			return err
		}, c.Limpiar)
	}
	c.AlBorrarSaliente = func(id string) {
		correr(func(ctx context.Context) error { return in.DeleteOutgoingWebhook(ctx, id) }, nil)
	}
	c.AlCrearBot = func(nombre string) {
		var b api.RoomsBot
		var token string
		correr(func(ctx context.Context) (err error) {
			b, token, err = in.CreateBot(ctx, nombre)
			return err
		}, func() {
			c.Secreto = &SecretoIntegracion{Titulo: t.T("integrations.botToken", i18n.Vars{"name": b.Name}), Valor: token}
			c.Limpiar()
		})
	}
	c.AlBorrarBot = func(id string) { correr(func(ctx context.Context) error { return in.DeleteBot(ctx, id) }, nil) }
	c.AlBotEnCanal = func(bot string, dentro bool) {
		canal := c.Canal
		correr(func(ctx context.Context) error { return in.SetBotInChannel(ctx, bot, canal, dentro) }, nil)
	}
	c.AlBorrarPuente = func(id string) { correr(func(ctx context.Context) error { return in.DeleteBridge(ctx, id) }, nil) }
	c.AlDesconectarNumero = func(id string) {
		correr(func(ctx context.Context) error { return in.DisconnectWhatsApp(ctx, id) }, nil)
	}
	c.AlCrearComando = func(k api.RoomsCommand) {
		correr(func(ctx context.Context) error {
			_, err := in.CreateCommand(ctx, k)
			return err
		}, func() { c.Limpiar(); comandos() })
	}
	c.AlBorrarComando = func(id string) {
		correr(func(ctx context.Context) error { return in.DeleteCommand(ctx, id) }, comandos)
	}
	c.AlCopiar = func(v string) {
		sa.Copiar(v)
		sa.P.Avisar(ui.AvisoExito, t.T("message.copied", nil))
	}
	c.AlConectarApp = func(clave string) {
		sa.integraciones = nil
		d.Cerrar()
		sa.AbrirConectarApp(clave)
	}
	c.AlPolitica = func() { sa.abrirURL(ui.PoliticaDePrivacidad(sa.Apex, t.Locale())) }
	d = ui.NuevoDialogo(t.T("integrations.title", nil), c).Con(ui.DialogoLG, nil)
	// El cuerpo del Modal: su ancho menos los filetes y el `px-6`. Lo corrige el primer
	// Layout; darlo ya evita que el primer MinSize se mida a ancho cero.
	c.ancho = ui.AnchoCuerpoDialogo(d, lienzo.Size())
	c.telefono = lienzo.Size().Width > 0 && lienzo.Size().Width < ui.Estrecho
	d.RotuloCerrar = t.T("action.close", nil)
	// `AlCerrar` es la PETICIÓN de cierre y este diálogo no tiene pie: sin `d.Cerrar()` no
	// había forma de cerrarlo.
	d.AlCerrar = func() { sa.integraciones = nil; d.Cerrar() }
	sa.integraciones = c
	d.Mostrar(lienzo)
	recargar()
	comandos()
}
