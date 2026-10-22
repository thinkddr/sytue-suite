// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/Composer.tsx (el autocompletado:
// `triggerAt`, `suggestionsFor`, `pick` y el `<ul role="listbox">`) y
// apps/frontend/apps/rooms/src/lib/commands.ts (el registro y el parseo de «/»).
//
// Al escribir, la caja ofrece encima una lista: personas y @channel/@here/@all tras
// «@», emoji tras «:algo» y comandos si el texto entero es «/palabra». Flechas para
// recorrerla, Intro o el tabulador para elegir, Escape para cerrarla.

// ---- Los comandos: `lib/commands.ts` ----

// Comando es un `SlashCommand`: el nombre canónico y sus alias. La descripción y el uso
// viven en el catálogo, en `command.<nombre>.description` y `.usage`.
type Comando struct {
	Nombre string
	Alias  []string
}

// Comandos es `COMMANDS`, en el orden del autocompletado y de «/help».
var Comandos = []Comando{
	{Nombre: "shrug"},
	{Nombre: "me"},
	{Nombre: "away"},
	{Nombre: "online"},
	{Nombre: "dnd"},
	{Nombre: "status"},
	{Nombre: "leave"},
	{Nombre: "topic", Alias: []string{"header"}},
	{Nombre: "purpose"},
	{Nombre: "invite"},
	{Nombre: "remove", Alias: []string{"kick"}},
	{Nombre: "msg", Alias: []string{"dm"}},
	{Nombre: "search"},
	{Nombre: "help"},
}

// reComando es `COMMAND_RE`: «/» y una LETRA al principio absoluto, para que «//x» o
// «/ x» se envíen como texto.
var reComando = regexp.MustCompile(`^/([a-zA-Z][\w-]*)\s?([\s\S]*)$`)

// TipoComando dice qué es lo escrito: nada (texto normal), un integrado, uno de
// servidor o uno que no existe.
type TipoComando uint8

// Los cuatro.
const (
	NoEsComando TipoComando = iota
	ComandoIntegrado
	ComandoRemoto
	ComandoDesconocido
)

// ParsearComando es `parseCommand`: el nombre CANÓNICO (los alias resueltos) y el
// argumento crudo. Los integrados ganan a los de servidor.
func ParsearComando(s string, remotos []api.RoomsCommand) (tipo TipoComando, nombre, arg string) {
	m := reComando.FindStringSubmatch(s)
	if m == nil {
		return NoEsComando, "", ""
	}
	n := strings.ToLower(m[1])
	for _, c := range Comandos {
		if c.Nombre == n || contiene(c.Alias, n) {
			return ComandoIntegrado, c.Nombre, m[2]
		}
	}
	for _, r := range remotos {
		if r.Name == n {
			return ComandoRemoto, r.Name, m[2]
		}
	}
	return ComandoDesconocido, m[1], ""
}

func contiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Encoger es `shrugBody`; Emote, `emoteBody`.
const encogimiento = `¯\_(ツ)_/¯`

// Encoger añade el encogimiento de hombros al texto, o lo deja solo.
func Encoger(s string) string {
	if t := strings.TrimSpace(s); t != "" {
		return t + " " + encogimiento
	}
	return encogimiento
}

// Emote es el cuerpo de «/me»: se guarda así y el historial lo pinta «* Autor acción».
func Emote(s string) string { return "/me " + strings.TrimSpace(s) }

var reEmote = regexp.MustCompile(`^/me\s+([\s\S]+)$`)

// AccionDeEmote es `parseEmote`: la acción de un «/me …», o "" si no lo es.
func AccionDeEmote(cuerpo string) string {
	if m := reEmote.FindStringSubmatch(cuerpo); m != nil {
		return m[1]
	}
	return ""
}

// ---- El disparador: `triggerAt` y `commandAutocompleteQuery` ----

// TipoDisparador es qué se está completando.
type TipoDisparador uint8

// Los tres, y ninguno.
const (
	SinDisparador TipoDisparador = iota
	DisparaMencion
	DisparaEmoji
	DisparaComando
)

var (
	reMencionAqui    = regexp.MustCompile(`(^|\s)@([^\s@]*)$`)
	reEmojiAqui      = regexp.MustCompile(`(^|\s):([a-z0-9_+-]{2,})$`)
	reComandoEnCurso = regexp.MustCompile(`(?i)^/([a-z0-9-]*)$`)
)

// Disparador es `refreshTrigger`: «/palabra» con el texto entero gana; si no, lo que
// haya pegado al cursor —«:algo» antes que «@algo»—. `cursor` es el desplazamiento en
// RUNAS, que es lo que da el editor.
func Disparador(texto string, cursor int) (TipoDisparador, string) {
	if m := reComandoEnCurso.FindStringSubmatch(texto); m != nil {
		return DisparaComando, strings.ToLower(m[1])
	}
	r := []rune(texto)
	antes := string(r[:min(max(cursor, 0), len(r))])
	if m := reEmojiAqui.FindStringSubmatch(antes); m != nil {
		return DisparaEmoji, m[2]
	}
	if m := reMencionAqui.FindStringSubmatch(antes); m != nil {
		return DisparaMencion, m[2]
	}
	return SinDisparador, ""
}

// ---- Las sugerencias: `suggestionsFor` ----

// TipoSugerencia es el `kind` de una sugerencia.
type TipoSugerencia uint8

// Las cinco.
const (
	SugierePersona TipoSugerencia = iota
	SugiereEspecial
	SugiereEmoji
	SugiereComando
	SugiereRemoto
)

// Sugerencia es una entrada de la lista.
type Sugerencia struct {
	Tipo TipoSugerencia
	// Nombre es el de la persona, el token («channel»), el shortcode o el comando.
	Nombre string
	// Glifo es el emoji; Pista, la clave de catálogo de una especial o el texto de un
	// comando de servidor; Uso, el uso de un comando de servidor.
	Glifo, Pista, Uso string
}

// especiales es `SPECIAL_MENTIONS`: el token no se traduce, que es lo que se teclea.
var especiales = []struct{ token, pista string }{
	{"channel", "mention.notifyChannel"}, {"here", "mention.notifyHere"}, {"all", "mention.notifyChannel"},
}

// SugerenciasPara son las de ese disparador, ya acotadas: 8 personas tras las
// especiales y 10 en total; 8 emoji; todos los comandos que casen.
func SugerenciasPara(tipo TipoDisparador, q string, roster []Miembro, remotos []api.RoomsCommand) []Sugerencia {
	var out []Sugerencia
	q = strings.ToLower(q)
	switch tipo {
	case DisparaComando:
		for _, c := range Comandos {
			if q == "" || strings.HasPrefix(c.Nombre, q) || hayPrefijo(c.Alias, q) {
				out = append(out, Sugerencia{Tipo: SugiereComando, Nombre: c.Nombre})
			}
		}
		for _, r := range remotos {
			if q == "" || strings.HasPrefix(r.Name, q) {
				out = append(out, Sugerencia{Tipo: SugiereRemoto, Nombre: r.Name, Pista: r.Description, Uso: r.UsageHint})
			}
		}
	case DisparaMencion:
		for _, e := range especiales {
			if strings.HasPrefix(e.token, q) {
				out = append(out, Sugerencia{Tipo: SugiereEspecial, Nombre: e.token, Pista: e.pista})
			}
		}
		n := 0
		for _, m := range roster {
			if n < 8 && buscar.Contiene(m.Nombre, q) {
				out = append(out, Sugerencia{Tipo: SugierePersona, Nombre: m.Nombre})
				n++
			}
		}
		out = out[:min(len(out), 10)]
	case DisparaEmoji:
		for _, e := range &emojiLista {
			if len(out) < 8 && strings.Contains(e.nombre, q) {
				out = append(out, Sugerencia{Tipo: SugiereEmoji, Nombre: e.nombre, Glifo: e.glifo})
			}
		}
	case SinDisparador:
	}
	return out
}

func hayPrefijo(xs []string, q string) bool {
	for _, x := range xs {
		if strings.HasPrefix(x, q) {
			return true
		}
	}
	return false
}

// Reemplazo es `pick`: lo que hay que BORRAR detrás del cursor (en runas) y lo que hay
// que escribir en su lugar. Un comando sustituye el texto entero, que es «/palabra».
func Reemplazo(s Sugerencia, consulta string) (borrar int, escribir string) {
	switch s.Tipo {
	case SugierePersona, SugiereEspecial:
		return 1 + len([]rune(consulta)), "@" + s.Nombre + " "
	case SugiereEmoji:
		return 1 + len([]rune(consulta)), s.Glifo + " "
	case SugiereComando, SugiereRemoto:
	}
	return -1, "/" + s.Nombre + " "
}

// ---- La lista ----

// Las medidas de la lista, de la hoja de Composer.tsx.
var (
	// AltoMaxSugerencias es el `max-h-56`.
	AltoMaxSugerencias = tema.Espacio(56)
	// RellenoSugerencias es el `py-1` de la lista y AireSugerencias su `mb-1`.
	RellenoSugerencias = tema.Espacio(1)
	AireSugerencias    = tema.Espacio(1)
	// CanalonSugerencia es el `px-3` de una entrada, RellenoSugerencia su `py-1.5` y
	// HuecoSugerencia su `gap-2`.
	CanalonSugerencia = tema.Espacio(3)
	RellenoSugerencia = tema.Espacio(1.5)
	HuecoSugerencia   = tema.Espacio(2)
	// LadoMarcaSugerencia es el `h-5 w-5` del cuadro del «@» o de la «/».
	LadoMarcaSugerencia = tema.Espacio(5)
)

// ListaSugerencias es el `<ul role="listbox">` sobre la caja de escribir.
type ListaSugerencias struct {
	widget.BaseWidget

	T        *i18n.Translator
	Items    []Sugerencia
	Elegida  int
	AlElegir func(Sugerencia)
	// Ancha enseña el uso de los comandos, que es `hidden sm:inline`.
	Ancha bool
	// Rotulo es el `aria-label` según lo que se complete.
	Rotulo string

	filas  []*filaSugerencia
	fondo  *ui.Caja
	sombra *ui.Sombra
	scroll *container.Scroll
}

// NuevaListaSugerencias crea la lista vacía.
func NuevaListaSugerencias(t *i18n.Translator) *ListaSugerencias {
	l := &ListaSugerencias{T: t}
	l.ExtendBaseWidget(l)
	return l
}

// AccessibilityLabel es el `aria-label` de la lista.
func (l *ListaSugerencias) AccessibilityLabel() string { return l.Rotulo }

// AccessibilityRole: una lista de opciones.
func (l *ListaSugerencias) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// Mover es la flecha: la elegida avanza o retrocede dando la vuelta.
func (l *ListaSugerencias) Mover(paso int) {
	if n := len(l.Items); n > 0 {
		l.Elegida = ((l.Elegida+paso)%n + n) % n
		l.Refresh()
	}
}

// AltoFilaSugerencia es una entrada: `py-1.5` y la caja de línea de `text-sm`.
func AltoFilaSugerencia() float32 { return 2*RellenoSugerencia + tema.TextoSM.Linea }

// MedidaListaSugerencias es el alto de la lista con `n` entradas, con su `py-1`, su
// borde y el tope de `max-h-56`.
func MedidaListaSugerencias(n int) float32 {
	return min(float32(n)*AltoFilaSugerencia()+2*RellenoSugerencias+2*tema.Borde, AltoMaxSugerencias)
}

func (l *ListaSugerencias) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.sombra = ui.NuevaSombra(tema.SombraMD, tema.RadioLG)
	l.fondo = ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG)
	l.scroll = container.NewVScroll(container.New(ui.Columna{}))
	r := &listaSugerenciasRenderer{l: l}
	r.Refresh()
	return r
}

type listaSugerenciasRenderer struct{ l *ListaSugerencias }

func (r *listaSugerenciasRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaListaSugerencias(len(r.l.Items)))
}

func (r *listaSugerenciasRenderer) Objects() []fyne.CanvasObject {
	l := r.l
	out := make([]fyne.CanvasObject, 0, 1+len(l.fondo.Objetos()))
	out = append(out, l.sombra.Objeto())
	return append(append(out, l.fondo.Objetos()...), l.scroll)
}
func (r *listaSugerenciasRenderer) Destroy() {}

func (r *listaSugerenciasRenderer) Layout(t fyne.Size) {
	l := r.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.sombra.Ajustar(fyne.Position{}, t, v)
	ui.MoverCaja(l.fondo, fyne.Position{}, t)
	l.scroll.Move(fyne.NewPos(tema.Borde, tema.Borde+RellenoSugerencias))
	l.scroll.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde-2*RellenoSugerencias, 0)))
	// El viewport debe estar medido antes de revelar la elegida: durante el primer
	// MinSize todavía puede medir cero, y desplazar ahí ocultaría la primera fila.
	if alto := l.scroll.Size().Height; alto > 0 {
		if y := float32(l.Elegida) * AltoFilaSugerencia(); y < l.scroll.Offset.Y {
			l.scroll.Offset.Y = y
		} else if fin := y + AltoFilaSugerencia() - alto; fin > l.scroll.Offset.Y {
			l.scroll.Offset.Y = fin
		}
	}
	l.scroll.Refresh()
}

func (r *listaSugerenciasRenderer) Refresh() {
	l := r.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.fondo.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	col := l.scroll.Content.(*fyne.Container) //nolint:errcheck,forcetypeassert // CreateRenderer siempre instala este contenedor como contenido del scroll.
	col.Objects = col.Objects[:0]
	l.filas = l.filas[:0]
	for i, s := range l.Items {
		f := &filaSugerencia{l: l, i: i, s: s}
		f.ExtendBaseWidget(f)
		l.filas = append(l.filas, f)
		col.Objects = append(col.Objects, f)
	}
	col.Refresh()
	r.Layout(l.Size())
}

// filaSugerencia es un `<li role="option">` con su botón.
type filaSugerencia struct {
	widget.BaseWidget
	l *ListaSugerencias
	i int
	s Sugerencia
}

var (
	_ fyne.Tappable     = (*filaSugerencia)(nil)
	_ desktop.Hoverable = (*filaSugerencia)(nil)
)

// Tapped es el `onMouseDown` de la web: elige sin quitarle el foco a la caja.
func (f *filaSugerencia) Tapped(*fyne.PointEvent) {
	if f.l.AlElegir != nil {
		f.l.AlElegir(f.s)
	}
}

// MouseIn es el `onMouseEnter`: pasa a ser la elegida.
func (f *filaSugerencia) MouseIn(*desktop.MouseEvent) {
	if f.l.Elegida != f.i {
		f.l.Elegida = f.i
		for _, x := range f.l.filas {
			x.Refresh()
		}
	}
}
func (f *filaSugerencia) MouseMoved(*desktop.MouseEvent) {}
func (f *filaSugerencia) MouseOut()                      {}

// AccessibilityLabel es lo que dice la fila.
func (f *filaSugerencia) AccessibilityLabel() string {
	switch f.s.Tipo {
	case SugiereEspecial, SugierePersona:
		return "@" + f.s.Nombre
	case SugiereEmoji:
		return ":" + f.s.Nombre + ":"
	case SugiereComando, SugiereRemoto:
	}
	return "/" + f.s.Nombre
}

// AccessibilityRole: una opción de la lista.
func (f *filaSugerencia) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (f *filaSugerencia) MinSize() fyne.Size { return fyne.NewSize(0, AltoFilaSugerencia()) }

func (f *filaSugerencia) CreateRenderer() fyne.WidgetRenderer {
	r := &filaSugerenciaRenderer{f: f, fondo: canvas.NewRectangle(color.NRGBA{})}
	return r
}

type filaSugerenciaRenderer struct {
	f     *filaSugerencia
	fondo *canvas.Rectangle
	objs  []fyne.CanvasObject
}

func (r *filaSugerenciaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaSugerenciaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaSugerenciaRenderer) Destroy()                     {}
func (r *filaSugerenciaRenderer) Layout(fyne.Size)             { r.Refresh() }

// Refresh rehace la fila: son cuatro piezas y la fila se repinta al cambiar la elegida.
func (r *filaSugerenciaRenderer) Refresh() {
	f := r.f
	t := f.Size()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	elegida := f.l.Elegida == f.i
	r.fondo.FillColor = color.NRGBA{}
	tinta := "text"
	if elegida {
		r.fondo.FillColor, tinta = tema.Color("menu-selected", v), "primary-text"
	}
	r.fondo.Resize(t)
	r.objs = []fyne.CanvasObject{r.fondo}
	x, alto := CanalonSugerencia, t.Height
	marca := func(texto, fondo, color string, peso ui.Peso) {
		c := ui.NuevaCaja(tema.Color(fondo, v), tema.Color(fondo, v), tema.RadioSM)
		ui.MoverCaja(c, fyne.NewPos(x, (alto-LadoMarcaSugerencia)/2), fyne.NewSize(LadoMarcaSugerencia, LadoMarcaSugerencia))
		tx := ui.Texto(texto, tema.TextoXS, peso, tema.Color(color, v))
		ui.ColocarTexto(tx, tema.TextoXS, x+(LadoMarcaSugerencia-ui.Medir(texto, tema.TextoXS, peso).Width)/2, (alto-tema.TextoXS.Linea)/2)
		r.objs = append(r.objs, c.Objetos()...)
		r.objs = append(r.objs, tx)
		x += LadoMarcaSugerencia + HuecoSugerencia
	}
	texto := func(s string, e tema.Escalon, peso ui.Peso, token string, cabe float32) float32 {
		tx := ui.Texto("", e, peso, tema.Color(token, v))
		ancho := ui.Truncar(tx, s, e, peso, max(cabe, 0))
		ui.ColocarTexto(tx, e, x, (alto-e.Linea)/2)
		r.objs = append(r.objs, tx)
		return ancho
	}
	derecha := t.Width - CanalonSugerencia
	s := f.s
	switch s.Tipo {
	case SugierePersona:
		av := ui.NuevoAvatar(s.Nombre).Con(ui.AvatarXS)
		lado := ui.MedidaAvatar(ui.AvatarXS).Width
		av.Move(fyne.NewPos(x, (alto-lado)/2))
		av.Resize(fyne.NewSize(lado, lado))
		r.objs = append(r.objs, av)
		x += lado + HuecoSugerencia
		texto(s.Nombre, tema.TextoSM, ui.Normal, tinta, derecha-x)
	case SugiereEspecial:
		marca("@", "warning-bg", "warning-text", ui.Negrita)
		pista := f.l.T.T(s.Pista, nil)
		anchoPista := ui.Medir(pista, tema.TextoXS, ui.Normal).Width
		x += texto("@"+s.Nombre, tema.TextoSM, ui.Medio, tinta, derecha-x-anchoPista-HuecoSugerencia) + HuecoSugerencia
		x = max(x, derecha-anchoPista) // `ml-auto`
		texto(pista, tema.TextoXS, ui.Normal, "text-subtle", derecha-x)
	case SugiereComando, SugiereRemoto:
		marca("/", "bg-subtle", "text-muted", ui.Negrita)
		x += texto("/"+s.Nombre, tema.TextoSM, ui.Medio, tinta, derecha-x) + HuecoSugerencia
		desc, uso := s.Pista, s.Uso
		if s.Tipo == SugiereComando {
			desc, uso = f.l.T.T("command."+s.Nombre+".description", nil), f.l.T.T("command."+s.Nombre+".usage", nil)
		}
		anchoUso := float32(0)
		if f.l.Ancha && uso != "" {
			anchoUso = ui.Medir(uso, tema.TextoXS, ui.Normal).Width + HuecoSugerencia
		}
		texto(desc, tema.TextoXS, ui.Normal, "text-subtle", derecha-x-anchoUso)
		if anchoUso > 0 {
			x = derecha - anchoUso + HuecoSugerencia
			texto(uso, tema.TextoXS, ui.Normal, "text-subtle", derecha-x)
		}
	case SugiereEmoji:
		tx := ui.Texto(s.Glifo, tema.TextoBase, ui.Normal, tema.Color("text", v))
		ui.ColocarTexto(tx, tema.TextoBase, x, (alto-tema.TextoBase.Linea)/2)
		r.objs = append(r.objs, tx)
		x += LadoMarcaSugerencia + HuecoSugerencia
		texto(":"+s.Nombre+":", tema.TextoSM, ui.Normal, "text-muted", derecha-x)
	}
	r.fondo.Refresh()
}
