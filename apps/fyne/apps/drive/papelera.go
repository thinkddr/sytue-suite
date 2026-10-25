// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # La papelera
//
// `TrashView` de FileBrowser.tsx. No es la lista del explorador aunque se parezca: NO
// tiene cabecera de columnas, NO tiene casillas y cada fila lleva sus dos botones
// —«Restaurar» y «Eliminar definitivamente»— pegados a la derecha. La fila es
// `border-b px-3 py-2` con la misma tesela de 36, así que mide lo mismo que la del
// explorador cuando los botones (28) no la estiran; con ellos manda la tesela igual,
// porque 36 > 28.
//
// Y no es una rejilla. La papelera que sí lo es —en filas justificadas, como su
// galería— es la de Sytue Photos (`photos.Filas`); confundirlas cuesta una pantalla
// entera.

// VistaPapelera es la pantalla de la papelera.
type VistaPapelera struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Fase      Fase
	Elementos []Elemento
	// Ocupado apaga los botones mientras hay una operación en curso, como el `busy`
	// de la web: dos «restaurar» seguidos sobre lo mismo dan un 409 confuso.
	Ocupado bool
	// Desconectado: el monitor de red dice que no hay. Se ve lo cargado y no se muta.
	Desconectado bool
	Ventana      float32
	// Ahora es desde cuándo se cuenta la cuenta atrás. Se inyecta para que el
	// guardián pueda fijar el día y comparar un número, en vez de depender del reloj.
	Ahora time.Time

	AlRestaurar  func(Elemento)
	AlEliminar   func(Elemento)
	AlReintentar func()
	desplazable  *container.Scroll
}

// NuevaVistaPapelera crea la pantalla en su estado inicial: cargando.
func NuevaVistaPapelera(tr *i18n.Translator) *VistaPapelera {
	return &VistaPapelera{tr: tr, loc: tr.Locale()}
}

func (p *VistaPapelera) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return p.tr.T(clave, v)
}

// SinRed dice si la pantalla está sin conexión, que apaga restaurar y eliminar: las dos
// son mutaciones y el contrato offline las bloquea.
func (p *VistaPapelera) SinRed() bool { return p.Fase == SinConexion || p.Desconectado }

// DiasHasta son los días enteros que faltan para una fecha, desde `ahora`. Cero o menos
// es «hoy o ya pasó».
func DiasHasta(ahora, cuando time.Time) int {
	if cuando.IsZero() {
		return 0
	}
	return int(cuando.Sub(ahora).Hours() / 24)
}

// Urgente es el criterio de la web: tres días o menos. A partir de ahí, no restaurarlo
// ES perderlo, y por eso el texto va en rojo.
func Urgente(ahora, purga time.Time) bool {
	return !purga.IsZero() && DiasHasta(ahora, purga) <= 3
}

// Objeto arma la pantalla: cabecera de sección y cuerpo.
func (p *VistaPapelera) Objeto(v tema.Variante) fyne.CanvasObject {
	cab := ui.NuevaCabeceraApp(p.t("trash.title"))
	cab.Subtitulo = p.t("trash.subtitle")
	cab.Icono = iconos.Trazo(iconos.Papelera, ui.GlifoMenu, "text", v).Objeto()
	return container.New(Pila{Elastico: 1}, cab, p.cuerpo(v))
}

func (p *VistaPapelera) cuerpo(v tema.Variante) fyne.CanvasObject {
	switch p.Fase {
	case Cargando:
		return (&Explorador{tr: p.tr, loc: p.loc, Ventana: p.Ventana}).esqueletoCuerpo(v)
	case Vacia:
		return centrar(ui.NuevoVacio(p.t("trash.emptyTitle"), p.t("trash.emptyBody")), v)
	case Fallo, SinSesion, Prohibida, SinConexion:
		return centrar(p.problema(v), v)
	case Exito:
	}
	return p.listado(v)
}

func (p *VistaPapelera) problema(v tema.Variante) *ui.Vacio {
	var x *ui.Vacio
	switch p.Fase {
	case SinSesion:
		x = ui.NuevoVacio(p.t("files.error.sessionTitle"), p.t("files.error.sessionBody"))
		x.Accion = ui.NuevoBoton(p.t("files.error.sessionCta"), p.AlReintentar).Con(ui.Primario, ui.TallaSM)
	case Prohibida:
		x = ui.NuevoVacio(p.tr.T("forbidden.title", nil), p.tr.T("forbidden.description", nil))
	case SinConexion:
		x = ui.NuevoVacio(p.tr.T("offline.status", nil), p.tr.T("offline.description", nil))
		x.Ilustracion = iconos.Trazo(iconos.Nube, 40, "text-subtle", v).Objeto()
		return x
	case Cargando, Exito, Vacia, Fallo:
		x = ui.NuevoVacio(p.t("trash.errorTitle"), p.t("files.error.network"))
		x.Accion = ui.NuevoBoton(p.t("files.retry"), p.AlReintentar).Con(ui.Secundario, ui.TallaSM)
	}
	x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
	return x
}

// listado es la ayuda en línea y las filas, dentro del `px-3 pb-6`.
func (p *VistaPapelera) listado(v tema.Variante) fyne.CanvasObject {
	texto := p.t("trash.help")
	// Dos frases INDEPENDIENTES del catálogo, y la segunda solo si hay cuenta atrás:
	// cada una se traduce entera, sin componerlas a trozos.
	for _, e := range p.Elementos {
		if !e.Purga.IsZero() {
			texto += " " + p.t("trash.helpCountdown")
			break
		}
	}
	// El ancho de verdad lo pone el layout; para repartir líneas se usa el de la
	// ventana menos los canalones, que es lo que la pantalla va a tener.
	ancho := max(ui.AnchoHoja(p.Ventana, ui.AnchoMenu(false), 0)-4*CanalonLista, 1)
	ayuda := ui.NuevaAyudaEnLinea(texto)

	hijos := make([]fyne.CanvasObject, 0, 2+len(p.Elementos))
	hijos = append(hijos, canvas.NewRectangle(tema.Color("bg", v)), ayuda)
	l := &listaPapelera{ayuda: ayuda, ayudaAlto: ui.AltoAyudaEnLinea(ayuda, ancho)}
	for _, e := range p.Elementos {
		f := p.fila(v, e)
		l.filas = append(l.filas, f)
		hijos = append(hijos, f)
	}
	// `min-h-0 flex-1 overflow-y-auto`: la lista se desplaza dentro de su hueco. Sin
	// esto, con más filas de las que caben las últimas quedaban fuera de la ventana y
	// no había forma de restaurarlas.
	p.desplazable = container.NewVScroll(container.New(l, hijos...))
	return p.desplazable
}

// MedidaFilaPapelera es el alto de una fila: `py-2` a cada lado más la tesela de 36 y
// el `border-b` por dentro. Los mismos 51 que la fila del explorador, y por la misma
// razón: manda la tesela, no el texto ni los botones (36 > 28).
func MedidaFilaPapelera() float32 { return 2*ui.AltoFila + ui.TeselaDrive + tema.Borde }

// fila es una fila de la papelera: tesela, nombre, meta con la cuenta atrás y los dos
// botones a la derecha.
//
// Es un CONTENEDOR y no un widget: la fila no se pulsa, no se enfoca y no se selecciona
// —lo interactivo son sus dos botones, que ya son widgets—. La regla del catálogo es
// elegir por comportamiento, y esta no tiene ninguno propio.
func (p *VistaPapelera) fila(v tema.Variante, e Elemento) fyne.CanvasObject {
	el := e
	f := &filaPapelera{
		tesela: ui.NuevaTeselaTipo(Extension(e.Nombre), e.Carpeta, ui.TeselaDrive),
		nombre: ui.Texto(e.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		linea:  canvas.NewRectangle(conAlfaBorde(v)),
	}
	tintaMeta := "text-subtle"
	if Urgente(p.Ahora, e.Purga) {
		// Tres días o menos: a partir de ahí, no restaurarlo ES perderlo.
		tintaMeta = "danger-text"
	}
	f.meta = ui.Texto(p.meta(e), tema.TextoXS, ui.Normal, tema.Color(tintaMeta, v))
	f.restaurar = ui.NuevoBoton(p.t("trash.restore"), func() {
		if p.AlRestaurar != nil {
			p.AlRestaurar(el)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	f.eliminar = ui.NuevoBoton(p.t("trash.purge"), func() {
		if p.AlEliminar != nil {
			p.AlEliminar(el)
		}
	}).Con(ui.Fantasma, ui.TallaSM)
	inerte := p.Ocupado || p.SinRed()
	f.restaurar.Inerte, f.eliminar.Inerte = inerte, inerte

	hijos := append(f.tesela.Objetos(), f.nombre, f.meta, f.restaurar, f.eliminar, f.linea)
	return container.New(f, hijos...)
}

// filaPapelera coloca la fila.
type filaPapelera struct {
	tesela              *ui.TeselaTipo
	nombre, meta        *canvas.Text
	restaurar, eliminar *ui.Boton
	linea               *canvas.Rectangle
}

var _ fyne.Layout = (*filaPapelera)(nil)

func (f *filaPapelera) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*ui.MargenFila+ui.TeselaDrive, MedidaFilaPapelera())
}

func (f *filaPapelera) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	contenido := t.Height - tema.Borde
	f.linea.Move(fyne.NewPos(0, contenido))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	centro := func(alto float32) float32 { return (contenido - alto) / 2 }
	x := ui.MargenFila
	f.tesela.Colocar(x, centro(ui.TeselaDrive))
	x += ui.TeselaDrive + ui.HuecoFila

	derecha := t.Width - ui.MargenFila
	for _, b := range []*ui.Boton{f.eliminar, f.restaurar} {
		m := b.MinSize()
		b.Move(fyne.NewPos(derecha-m.Width, centro(m.Height)))
		b.Resize(m)
		derecha -= m.Width + ui.HuecoFila
	}

	cabe := max(derecha-x, 0)
	// Nombre y meta apilados y centrados juntos, como el `flex-col` de la web.
	alto := tema.TextoBase.Linea + SeparaMeta + tema.TextoXS.Linea
	y := centro(alto)
	ui.Truncar(f.nombre, f.nombre.Text, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	ui.Truncar(f.meta, f.meta.Text, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.meta, tema.TextoXS, x, y+tema.TextoBase.Linea+SeparaMeta)
}

// conAlfaBorde es el `border-border/60` de las filas: el token de borde al 60 %.
func conAlfaBorde(v tema.Variante) color.NRGBA { return tokens.ConAlfa(tema.Color("border", v), 0.6) }

// parrafo reparte un texto en líneas de `text-base` y las devuelve como un objeto que
// mide lo que ocupan. Es lo que va dentro de un diálogo de confirmación.
func parrafo(v tema.Variante, texto string, ancho float32) fyne.CanvasObject {
	if ancho <= 0 {
		// El cuerpo de un diálogo `sm` mide 320 menos su `px-6`.
		ancho = 320 - 2*tema.Espacio(6)
	}
	lineas := ui.Repartir(texto, tema.TextoBase, ui.Normal, ancho)
	p := &bloqueTexto{}
	hijos := make([]fyne.CanvasObject, 0, len(lineas))
	for _, l := range lineas {
		t := ui.Texto(l, tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
		p.lineas = append(p.lineas, t)
		hijos = append(hijos, t)
	}
	return container.New(p, hijos...)
}

// bloqueTexto apila líneas de `text-base` con `leading-relaxed` (1,625).
type bloqueTexto struct{ lineas []*canvas.Text }

var _ fyne.Layout = (*bloqueTexto)(nil)

func (b *bloqueTexto) alto() float32 { return float32(len(b.lineas)) * 1.625 * tema.TextoBase.Tam }

func (b *bloqueTexto) MinSize([]fyne.CanvasObject) fyne.Size {
	var ancho float32
	for _, l := range b.lineas {
		ancho = max(ancho, ui.Medir(l.Text, tema.TextoBase, ui.Normal).Width)
	}
	return fyne.NewSize(ancho, b.alto())
}

func (b *bloqueTexto) Layout([]fyne.CanvasObject, fyne.Size) {
	linea := 1.625 * tema.TextoBase.Tam
	for i, l := range b.lineas {
		ui.ColocarTexto(l, tema.Escalon{Tam: tema.TextoBase.Tam, Linea: linea}, 0, float32(i)*linea)
	}
}

// meta es «Carpeta» o el tamaño, y detrás la cuenta atrás si la hay.
func (p *VistaPapelera) meta(e Elemento) string {
	meta := Tamano(p.loc, e.Tamano)
	if e.Carpeta {
		meta = p.t("files.folder")
	}
	if frase := p.frasePurga(e); frase != "" {
		meta += " · " + frase
	}
	return meta
}

// frasePurga es el `purgePhrase` de la web: la cuenta atrás en llano, no una fecha que
// haya que restar mentalmente.
//
// Sin fecha de purga la auto-purga está desactivada y se DICE («no se borra solo»); sin
// nada de nada no se afirma nada, que es lo contrario de escribir «caduca pronto» sobre
// un dato que no se tiene. El plural lo decide el catálogo, no un `== 1`.
func (p *VistaPapelera) frasePurga(e Elemento) string {
	if e.Purga.IsZero() {
		return ""
	}
	switch d := DiasHasta(p.Ahora, e.Purga); {
	case d <= 0:
		return p.t("purge.today")
	case d == 1:
		return p.t("purge.tomorrow")
	default:
		return p.t("purge.days", i18n.Vars{"count": d, "date": Fecha(p.loc, e.Purga)})
	}
}

// ConfirmacionPurga es el diálogo de «eliminar definitivamente»: `sm`, con el verbo en
// el botón rojo y el cuerpo que dice qué se pierde. La acción es irreversible y por eso
// pregunta; el texto es distinto para una carpeta y para un fichero.
func (p *VistaPapelera) ConfirmacionPurga(v tema.Variante, e Elemento, alCancelar, alPurgar func()) *ui.Dialogo {
	clave := "trash.purgeBodyFile"
	if e.Carpeta {
		clave = "trash.purgeBodyFolder"
	}
	cuerpo := parrafo(v, p.t(clave, i18n.Vars{"name": e.Nombre}), 0)
	d := ui.Confirmacion(p.t("trash.purgeTitle"), p.tr.T("ui.cancel", nil), p.t("trash.purge"),
		true, cuerpo, alCancelar, alPurgar)
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	return d
}

// listaPapelera coloca la ayuda y las filas con el `px-3` del contenedor.
type listaPapelera struct {
	ayuda     fyne.CanvasObject
	ayudaAlto float32
	filas     []fyne.CanvasObject
}

var _ fyne.Layout = (*listaPapelera)(nil)

// MargenAyuda es el `px-3 pt-3 pb-1` del `<li>` que envuelve la ayuda.
const (
	MargenAyuda float32 = 3 * 0.25 * tema.Rem
	ArribaAyuda float32 = 3 * 0.25 * tema.Rem
	DebajoAyuda float32 = 1 * 0.25 * tema.Rem
)

func (l *listaPapelera) MinSize([]fyne.CanvasObject) fyne.Size {
	alto := ArribaAyuda + l.ayudaAlto + DebajoAyuda
	for range l.filas {
		alto += MedidaFilaPapelera()
	}
	return fyne.NewSize(0, alto+PieLista)
}

func (l *listaPapelera) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	izq := CanalonLista + MargenAyuda
	ancho := max(t.Width-2*izq, 0)
	l.ayuda.Move(fyne.NewPos(izq, ArribaAyuda))
	l.ayuda.Resize(fyne.NewSize(ancho, l.ayudaAlto))
	y := ArribaAyuda + l.ayudaAlto + DebajoAyuda
	for _, f := range l.filas {
		f.Move(fyne.NewPos(CanalonLista, y))
		f.Resize(fyne.NewSize(max(t.Width-2*CanalonLista, 0), MedidaFilaPapelera()))
		y += MedidaFilaPapelera()
	}
}
