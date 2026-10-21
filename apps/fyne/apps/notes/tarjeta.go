// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"
	"image/color"
	"image/draw"
	"regexp"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Paridad web: apps/frontend/apps/notes/src/components/NoteCard.tsx.

// UNA NOTA DEL LIENZO. Un solo tipo para los doce tipos de nota y no doce tipos: lo
// que cambia entre una nota de texto y un evento vinculado es el CUERPO; el marco, el
// menú, las etiquetas y el foco son idénticos. Es la misma decisión que la web.
//
// Medidas, de la hoja de NoteCard.tsx con el rem de 14:
//
//	marco     `rounded-lg border p-3 gap-1.5 shadow-sm`   → radio 8, relleno 10,5
//	cabecera  `flex items-start gap-1.5`, glifo de 15
//	título    text-base peso 500 (`font-medium`)
//	cuerpo    text-base, interlineado `leading-relaxed`
//	pie       estados en `<Badge>`, `flex-wrap gap-1`
//
// # ES UN WIDGET Y NO UN CONTENEDOR
//
// Porque es SELECCIONABLE y enfocable: sin la cáscara no llega un clic —`walkObjectTree`
// solo desciende en contenedores y widgets— y el lector de pantalla anunciaría
// «Container» en lugar del título de la nota. Es la regla 1 del patrón de `ui.go`.
type Tarjeta struct {
	widget.BaseWidget

	tr   *i18n.Translator
	nota Nota

	// Seleccionada pinta el anillo fino de `ring-primary/50` y sube la sombra.
	Seleccionada bool
	// Diario presenta la nota como FILA (`.notes-stream-note`): en un día y en cualquier
	// cuaderno en lista. Sin caja ni raya; la elegida es un fondo.
	Diario bool
	// Nivel es la profundidad de la nota dentro de su sublista (0 en la raíz): la fila
	// se sangra `1.75rem` por nivel, hasta cuatro.
	Nivel int
	// Bloqueada es el cuaderno cifrado y cerrado: en vez del cuerpo va el aviso de
	// desbloquear, que es lo que hace la web.
	Bloqueada bool
	// SoloLectura apaga las acciones. Se enciende sin conexión Y cuando el documento
	// no vuelve entero (`Nota.Editable`), que es la regla que manda sobre todo.
	SoloLectura bool
	// Llenar es la vista de árbol: la tarjeta ocupa el panel entero.
	Llenar bool
	// SinAcciones quita el «…»: es el `!soloLectura` de la web, que allí es solo «sin
	// conexión». No es SoloLectura, que además se enciende cuando el documento no
	// vuelve entero: una nota así se puede fijar, mover o etiquetar igual.
	SinAcciones bool
	// SinMenu quita SOLO el «…»: renombrar y escribir siguen (ver Lienzo.SinMenu).
	SinMenu bool
	// Recordatorio es el mío en esta nota, y AlRecordatorio lo abre al pulsar su
	// indicador (recordatorios.go).
	Recordatorio   *api.NoteReminder
	AlRecordatorio func()
	// Renombrar es el campo del título mientras se renombra; nil, no se renombra. Lo
	// guarda el lienzo, que es quien sobrevive a los repintados: el campo tiene el foco
	// y lo escrito.
	Renombrar *CampoEnLinea
	// Editor es el editor abierto en esta nota; nil, se lee.
	Editor *EditorNota
	// Ancha es la nota que se escribe, ensanchada: `min-h-[70dvh]` y el cuerpo estirado.
	// AlEnsanchar es su botón, que solo sale mientras se escribe.
	Ancha       bool
	AlEnsanchar func()

	// AlRenombrar empieza a renombrar: pulsar el título. nil, el título es texto.
	AlRenombrar func()
	// AlEditar abre el editor: pulsar el cuerpo, como una ficha de papel. AlEnlace sigue
	// un enlace pulsado en el cuerpo.
	AlEditar func()
	AlEnlace func(href string)
	// AlPlegar pliega una lista del cuerpo al pulsar su viñeta, y Previa da la ficha de un
	// enlace a otra nota (plegar.go y previa.go).
	AlPlegar      func(i int)
	Previa        func(href string, listo func()) (VistaPrevia, bool)
	portal        *portal
	AlSeleccionar func()
	AlMarcar      func(bool)
	AlMenu        func(disparador fyne.CanvasObject)
	// AlSoltar es el final de un arrastre en el lienzo libre, con la esquina de la
	// tarjeta en coordenadas de su contenedor. Con él puesto, la tarjeta lleva el asa.
	AlSoltar func(fyne.Position)
	// Imagen es la imagen de una dirección tal como vaya (la de una nota de imagen), y
	// Documento el HTML de un documento de Docs (la vista previa de una nota `doc`).
	Imagen    func(url string) imagenRemota
	Documento func(id string) docRemoto
	// AlDescargar y AlQuitarAdjunto son los dos botones de cada adjunto.
	AlDescargar     func(Adjunto)
	AlQuitarAdjunto func(Adjunto)
	// AlTecla recibe las letras sueltas con la tarjeta enfocada: son los atajos de
	// estado y la `n`, que en la web escucha `document` y aquí no llegarían al lienzo.
	AlTecla func(rune)
	// AlTeclaEspecial recibe las demás teclas (Escape).
	AlTeclaEspecial func(*fyne.KeyEvent)

	encima, enfocado bool
	mas              *botonMas
	// arrastre es dónde iría la tarjeta sin el recorte a cero: el `clientX - dx` de la
	// web, que sigue al puntero aunque salga por la izquierda.
	arrastre    fyne.Position
	arrastrando bool
	// alEmpezar es el principio de un arrastre: el lienzo la sube por encima de todas.
	alEmpezar func()
	// prosa es el cuerpo que se ve, si es prosa: donde están los títulos para un `h=`.
	prosa *Prosa
}

var (
	_ fyne.Widget       = (*Tarjeta)(nil)
	_ fyne.Tappable     = (*Tarjeta)(nil)
	_ fyne.Focusable    = (*Tarjeta)(nil)
	_ desktop.Hoverable = (*Tarjeta)(nil)
)

// Medidas de la tarjeta, de las clases de `<article>`.
const (
	// RellenoTarjeta es `p-3`, HuecoTarjeta es `gap-1.5` y RadioTarjeta es
	// `rounded-lg`.
	RellenoTarjeta = 3 * 0.25 * tema.Rem
	HuecoTarjeta   = 1.5 * 0.25 * tema.Rem
	// GlifoTarjeta es el `size={15}` del icono de tipo en la cabecera.
	GlifoTarjeta = 15
	// AnchoTarjetaLibre es el `note.width` por defecto del lienzo libre cuando el
	// servicio no da ninguno.
	AnchoTarjetaLibre = 260
	// PuntoEstado es el `h-2 w-2` del punto de color de un estado de etiqueta.
	PuntoEstado = 2 * 0.25 * tema.Rem
)

// Las filas (`.notes-stream-note` de app.css): `px-2 py-1.5` sin borde; la cabecera en
// `gap-2`, el glifo en su columna de `w-5` y el cuerpo con el `pl-7` que lo alinea con el
// título. La hoja de un árbol (`.notes-sheet`) es `px-6 py-6 sm:px-10`, también sin borde.
const (
	RellenoFilaX      = 2 * 0.25 * tema.Rem
	RellenoFilaY      = 1.5 * 0.25 * tema.Rem
	HuecoFila         = 2 * 0.25 * tema.Rem
	ColumnaGlifoFila  = 5 * 0.25 * tema.Rem
	SangriaCuerpoFila = 7 * 0.25 * tema.Rem
	VinetaFila        = 1.5 * 0.25 * tema.Rem
	SangriaSublista   = 1.75 * tema.Rem
	RellenoHojaX      = 10 * 0.25 * tema.Rem
	RellenoHojaXMovil = 6 * 0.25 * tema.Rem
	RellenoHojaY      = 6 * 0.25 * tema.Rem
)

// hoja es la nota de un árbol, que llena su panel: una página, no una tarjeta.
func (t *Tarjeta) hoja() bool { return t.Llenar && !t.Diario }

// margenY es el relleno vertical: el `py-1.5` de la fila, el `py-6` de la hoja o el `p-3`
// de la tarjeta, este dentro de su borde.
func (t *Tarjeta) margenY() float32 {
	switch {
	case t.Diario:
		return RellenoFilaY
	case t.hoja():
		return RellenoHojaY
	}
	return RellenoTarjeta + tema.Borde
}

// sangriaPie es el `pl-7` del pie de una fila: las etiquetas van bajo el texto, no bajo
// el glifo.
func (t *Tarjeta) sangriaPie() float32 {
	if t.Diario {
		return SangriaCuerpoFila
	}
	return 0
}

// sangriaNivel es lo que la fila se mete por su profundidad en la sublista.
func (t *Tarjeta) sangriaNivel() float32 {
	if !t.Diario {
		return 0
	}
	return float32(min(t.Nivel, 4)) * SangriaSublista
}

// geometria es el relleno horizontal, el hueco de la cabecera y la sangría del cuerpo:
// los de la tarjeta, los de la fila o los de la hoja.
func (t *Tarjeta) geometria() (mx, hueco, sangria float32) {
	switch {
	case t.hoja():
		if ventana([]fyne.CanvasObject{t}).Width < ui.CorteSM {
			return RellenoHojaXMovil, HuecoTarjeta, 0
		}
		return RellenoHojaX, HuecoTarjeta, 0
	case !t.Diario:
		return RellenoTarjeta + tema.Borde, HuecoTarjeta, 0
	}
	if !t.enLinea() {
		sangria = SangriaCuerpoFila
	}
	return RellenoFilaX, HuecoFila, sangria
}

// soloRaya es el contenido de una nota que es solo un separador (`---`).
var soloRaya = regexp.MustCompile(`(?i)^\s*<hr\s*/?>\s*$`)

// esSeparador dice si la fila es solo una raya: se pinta como tal, sin viñeta.
func (t *Tarjeta) esSeparador() bool {
	return t.enLinea() && t.Editor == nil && soloRaya.MatchString(t.nota.Contenido)
}

// enLinea es el texto sin título de un diario: la fila entera es su cuerpo, en la línea
// del glifo, sin el «Sin título» que se repetiría en cada captura.
func (t *Tarjeta) enLinea() bool {
	return t.Diario && t.nota.Titulo == "" && t.nota.Tipo == "text" && !t.Bloqueada && t.Renombrar == nil
}

// NuevaTarjeta crea la tarjeta de una nota.
func NuevaTarjeta(tr *i18n.Translator, n Nota) *Tarjeta {
	t := &Tarjeta{tr: tr, nota: n}
	t.ExtendBaseWidget(t)
	return t
}

// Nota es la que pinta, para que quien la tenga no la busque en otro sitio.
func (t *Tarjeta) Nota() Nota { return t.nota }

// AccessibilityLabel es el `aria-label` del `<article>`: el título, o «nota nueva».
func (t *Tarjeta) AccessibilityLabel() string { return t.nota.Rotulo(t.tr) }

// AccessibilityRole: la nota es lo que se pulsa para abrirla, así que se anuncia como
// botón. Fyne no tiene rol de artículo ni de región con nombre.
func (t *Tarjeta) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// MedidaTarjetaNota es lo que va a ocupar. Es LA cuenta: el `MinSize` del renderer la
// llama y el guardián compara contra ella y contra los píxeles.
func MedidaTarjetaNota(t *Tarjeta, ancho float32) fyne.Size {
	mx, _, sangria := t.geometria()
	interior := max(ancho-2*mx, 0)
	alto := t.altoCabecera(interior)
	if c := altoCuerpo(t.cuerpo(tema.Claro), interior-sangria); c > 0 || t.estirada() {
		alto += HuecoTarjeta + c
	}
	if pie := t.pie(tema.Claro); pie != nil {
		alto += HuecoTarjeta + altoDe(pie, interior-t.sangriaPie())
	}
	alto += 2 * t.margenY()
	if t.Editor != nil && t.Ancha {
		alto = max(alto, AltoAncha*ventana([]fyne.CanvasObject{t}).Height)
	}
	return fyne.NewSize(ancho, alto)
}

// AltoAncha es el `min-h-[70dvh]` de la nota ensanchada.
const AltoAncha = 0.7

// estirada es `llenar || (editando && ancha)`: el cuerpo crece hasta abajo.
func (t *Tarjeta) estirada() bool { return t.Llenar || (t.Editor != nil && t.Ancha) }

// altoCuerpo es lo que ocupa el cuerpo a ese ancho: la prosa se maqueta, un párrafo se
// reparte, y lo demás mide lo suyo.
func altoCuerpo(o fyne.CanvasObject, ancho float32) float32 {
	switch x := o.(type) {
	case *Prosa:
		return x.AltoPara(ancho)
	case *EditorNota:
		return x.AltoPara(ancho)
	case *ui.Parrafo:
		if ancho <= 0 {
			return x.MinSize().Height
		}
		return ui.AltoParrafo(x, ancho)
	}
	return altoDe(o, ancho)
}

// altoCabecera es la fila de icono, título y «…»: `items-start`, así que manda el más
// alto. Con el «…» son sus 24; sin él —sin conexión—, el título, que ahí PARTE en líneas
// (`break-words`); y mientras se renombra, el campo (24,75).
func (t *Tarjeta) altoCabecera(interior float32) float32 {
	alto := tema.TextoBase.Linea
	izq, titulo, der, _ := t.cabecera(tema.Claro)
	_, hueco, _ := t.geometria()
	switch p := titulo.(type) {
	case *ui.Parrafo:
		alto = max(alto, ui.AltoParrafo(p, huecoTitulo(izq, der, interior, hueco)))
	case *Prosa:
		alto = max(alto, p.AltoPara(huecoTitulo(izq, der, interior, hueco)))
	case *EditorNota:
		alto = max(alto, p.AltoPara(huecoTitulo(izq, der, interior, hueco)))
	}
	if !t.SinAcciones && !t.SinMenu {
		alto = max(alto, ui.LadoIconoSM)
	}
	if t.Renombrar != nil {
		alto = max(alto, AltoCampoEnLinea(PYCampoTarjeta))
	}
	if t.Editor != nil {
		alto = max(alto, LadoBotonEd)
	}
	return alto
}

// CreateRenderer construye la geometría. Solo primitivas y widgets de `ui`.
func (t *Tarjeta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// `rounded-md` la fila, `rounded-lg` la tarjeta.
	radio := float32(tema.RadioLG)
	if t.Diario {
		radio = tema.RadioMD
	}
	r := &tarjetaRenderer{
		t:      t,
		sombra: ui.NuevaSombra(tema.SombraSM, radio),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radio),
		aro:    ui.NuevoAro(radio, ui.SinHueco, v),
	}
	// Sombra, aro y caja, en ese orden: el aro es relleno y la caja opaca le tapa el
	// centro. Pintado encima de la caja, la nota enfocada salía del color del anillo.
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Antes(r.caja.Objetos()...)...)
	r.rehacerHijos(v)
	// Regla 7: el renderer termina refrescando, o nace a medias y se pinta invisible.
	r.Refresh()
	return r
}

type tarjetaRenderer struct {
	t        *Tarjeta
	asa      *asaArrastre
	sombra   *ui.Sombra
	caja     *ui.Caja
	aro      *ui.Aro
	izq, der []pieza
	titulo   fyne.CanvasObject
	cuerpo   fyne.CanvasObject
	pie      fyne.CanvasObject
	objetos  []fyne.CanvasObject
}

func (r *tarjetaRenderer) rehacerHijos(v tema.Variante) {
	t := r.t
	r.izq, r.titulo, r.der, t.mas = t.cabecera(v)
	r.cuerpo, r.pie = t.cuerpo(v), t.pie(v)
	t.prosa, _ = r.cuerpo.(*Prosa)
	r.objetos = r.objetos[:cantidadFija(r)]
	for _, p := range r.izq {
		r.objetos = append(r.objetos, p.o)
	}
	r.objetos = append(r.objetos, r.titulo)
	for _, p := range r.der {
		r.objetos = append(r.objetos, p.o)
	}
	r.objetos = append(r.objetos, r.cuerpo)
	if r.pie != nil {
		r.objetos = append(r.objetos, r.pie)
	}
	r.asa = nil
	if t.AlSoltar != nil {
		r.asa = nuevaAsa(t, v)
		r.objetos = append(r.objetos, r.asa)
	}
}

// cantidadFija son los objetos de fondo (sombra, caja, aro) que van SIEMPRE los
// primeros y nunca se rehacen.
func cantidadFija(r *tarjetaRenderer) int {
	return 1 + len(r.caja.Objetos()) + len(r.aro.Objetos())
}

// huecoTitulo es lo que queda para el título entre lo de los dos lados.
func huecoTitulo(izq, der []pieza, interior, hueco float32) float32 {
	for _, p := range slices.Concat(izq, der) {
		interior -= p.o.MinSize().Width + hueco
	}
	return max(interior, 0)
}

// Lo que la cabecera tiene a cada lado del título, y su desplazamiento vertical.
//
// `items-start`: todo va arriba, y el icono de tipo y la casilla llevan `mt-0.5`. La
// casilla de la suite mide la caja de línea (19,25) con el cuadradito de 14 centrado, y
// la de la tarjeta es un `<input class="mt-0.5 h-4 w-4">` suelto: se sube lo que
// sobra para que el cuadradito caiga en los mismos 1,75.
const (
	bajaGlifoTarjeta  = 0.5 * 0.25 * tema.Rem
	bajaCasillaTarjet = bajaGlifoTarjeta - (19.25-14)/2
	// GlifoFijada es el `size={14}` de la chincheta de la cabecera.
	GlifoFijada = 14
)

// pieza es un objeto de la cabecera con su desplazamiento vertical.
type pieza struct {
	o    fyne.CanvasObject
	baja float32
}

// cabecera es el icono de tipo (o la casilla de una tarea) a la izquierda del título, y
// la chincheta y el «…» a su derecha. El título es `flex-1`: se queda con lo que sobre.
//
// El «…» sale también aparte: es el que la tarjeta enciende al pasar, y quien solo mide
// (`MedidaTarjetaNota`) no debe quitarle el suyo.
func (t *Tarjeta) cabecera(v tema.Variante) (izq []pieza, titulo fyne.CanvasObject, der []pieza, mas *botonMas) {
	switch {
	case t.nota.Icono != "" && GlifoPorNombre(t.nota.Icono) != "":
		izq = append(izq, pieza{Glifo(GlifoPorNombre(t.nota.Icono), GlifoTarjeta, "primary-text", v), bajaGlifoTarjeta})
	case t.nota.Tipo == "todo":
		c := ui.NuevaCasilla("", func(marcada bool) { llamar1(t.AlMarcar, marcada) })
		c.Marcada = t.nota.Marcada
		if t.SoloLectura || t.Bloqueada {
			c.Disable()
		}
		izq = append(izq, pieza{c, bajaCasillaTarjet})
	default:
		if d := GlifoDeTipo(t.nota.Tipo); d != "" {
			izq = append(izq, pieza{Glifo(d, GlifoTarjeta, "text-subtle", v), bajaGlifoTarjeta})
		}
	}
	if t.Diario {
		// La columna fija de Twos (`.notes-stream-glyph`, `h-5 w-5` centrada y `mt-0.5`):
		// cada fila empieza por su glifo en el mismo sitio, y un texto lleva la viñeta.
		dentro := punto(VinetaFila, tema.Color("text-subtle", v))
		switch {
		case len(izq) > 0:
			dentro = izq[0].o
		case t.esSeparador():
			// Un separador es solo su raya: la columna del glifo se queda vacía.
			dentro = canvas.NewRectangle(color.NRGBA{})
		}
		izq = []pieza{{container.New(&celdaGlifo{lado: ColumnaGlifoFila}, dentro), bajaGlifoTarjeta}}
	}
	titulo = t.titulo(v)
	switch {
	case t.esSeparador():
		// La raya a media altura de la línea, de lado a lado de lo que deja la cabecera.
		// ponytail: el mínimo «enorme» hace que colocarTitulo le dé todo el hueco; si la
		// cabecera gana más casos, darle su propio `case` allí.
		titulo = container.New(&rayaFila{}, canvas.NewRectangle(tema.Color("border", v)))
	case t.enLinea():
		titulo = t.cuerpoContenido(v)
	}
	if t.nota.Traslados > 0 && !t.nota.Terminada() {
		// `notes-carry`: cuántas veces ha pasado de un día al siguiente.
		der = append(der, pieza{lineaTok(t.tr.T("task.carried", i18n.Vars{"count": t.nota.Traslados}), tema.TextoXS, ui.Normal, "text-subtle", v), bajaGlifoTarjeta})
	}
	if t.nota.Fijada {
		der = append(der, pieza{Glifo(FijarD, GlifoFijada, "primary-text", v), 0})
	}
	if t.Recordatorio != nil {
		der = append(der, pieza{indicadorRecordatorio(t.tr, *t.Recordatorio, t.AlRecordatorio, v), 0})
	}
	if t.Editor != nil {
		// Ensanchar: `h-7 w-7 rounded-md`, el glifo de 14. No toma el foco, como el
		// `preventDefault` del `mousedown` en la web: si se lo llevara, la tarjeta
		// creería que te has ido de la nota.
		d, rotulo := SangrarD, t.tr.T("note.widen", nil)
		if t.Ancha {
			d, rotulo = DesangrarD, t.tr.T("note.narrow", nil)
		}
		b := nuevoBotonBarra(d, iconos.Ninguno, rotulo, func() { llamar0(t.AlEnsanchar) })
		b.Glifo, b.Radio = GlifoFijada, tema.Forma("radius-control")
		der = append(der, pieza{b, 0})
	}
	if !t.SinAcciones && !t.SinMenu {
		mas = nuevoBotonMas(t.tr.T("common.more", nil), t.Refresh)
		mas.AlPulsar = func() { llamar1(t.AlMenu, fyne.CanvasObject(mas)) }
		der = append(der, pieza{mas, 0})
	}
	return izq, titulo, der, mas
}

// titulo es lo que va en el `<h3>`, en uno de sus cuatro estados: el candado de un
// cuaderno cerrado, el campo de renombrar, el botón que lo abre, o el texto a secas en
// solo lectura. Sin título, el rótulo del enlace o «Sin título».
func (t *Tarjeta) titulo(v tema.Variante) fyne.CanvasObject {
	if t.Bloqueada {
		// `text-text-subtle inline-flex items-center gap-1`: el candado de 14 y
		// «Bloqueado».
		return container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem},
			Icono(iconos.Candado, GlifoFijada, "text-subtle", v),
			lineaTok(t.tr.T("crypto.locked", nil), tema.TextoBase, ui.Medio, "text-subtle", v))
	}
	if t.Renombrar != nil {
		return t.Renombrar
	}
	tinta := "text"
	if t.nota.Terminada() {
		// `text-text-muted line-through`: el `<h3>` y, desde que se puede elegir entre
		// ocultar o tachar las completadas, también el botón del título.
		tinta = "text-muted"
	}
	texto := t.nota.Titulo
	if texto == "" && len(t.nota.Enlaces) > 0 {
		texto = t.nota.Enlaces[0].Label
	}
	escalon, peso := t.letraTitulo()
	if t.SinAcciones || t.AlRenombrar == nil {
		// Texto suelto en el `<h3>`: parte en líneas (`break-words`), medido en la web, y
		// el de una tarea hecha va tachado en cada una.
		p := ui.NuevoParrafo(texto, escalon, peso, tinta, v)
		if t.nota.Terminada() {
			p.Tachada()
		}
		return p
	}
	if texto == "" {
		texto = t.tr.T("note.untitled", nil)
	}
	b := NuevoBotonTitulo(texto, tinta, t.tr.T("note.rename", nil), t.AlRenombrar).ConLetra(escalon, peso)
	b.Tachada = t.nota.Terminada()
	return b
}

// letraTitulo es la letra del título según la forma: `text-base font-medium` en una
// tarjeta, `font-normal` en una fila —ahí el título es el texto de la nota, no un
// encabezado— y `text-2xl font-semibold` en la hoja de un árbol (`.notes-sheet-title`).
func (t *Tarjeta) letraTitulo() (tema.Escalon, ui.Peso) {
	switch {
	case t.Diario:
		return tema.TextoBase, ui.Normal
	case t.hoja():
		return tema.Texto2XL, ui.SemiNegrita
	}
	return tema.TextoBase, ui.Medio
}

// rayaFila pone la raya de un separador a media altura de una línea de texto.
type rayaFila struct{}

func (rayaFila) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1<<14, tema.TextoBase.Linea)
}

func (rayaFila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, (t.Height-tema.Borde)/2))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
}

// botonMas es el «…» de la tarjeta: un `ui.BotonIcono` que además avisa a su tarjeta
// cuando el puntero o el foco entran y salen de él.
//
// Hace falta por cómo reparte Fyne el `:hover`: al pasar de la tarjeta a un hijo que
// también atiende el puntero, la tarjeta recibe `MouseOut`. El «…» solo se ve con la
// tarjeta encima (`group-hover:opacity-100`), así que sin este aviso se escondía justo
// al llegar a él, y al esconderse la tarjeta volvía a recibir el puntero: un parpadeo
// por cada movimiento del ratón. Es el mismo botón del catálogo con dos métodos más.
type botonMas struct {
	ui.BotonIcono
	alCambiar       func()
	dentro, enfoque bool
}

func nuevoBotonMas(rotulo string, alCambiar func()) *botonMas {
	b := &botonMas{alCambiar: alCambiar}
	// Los de `ui.NuevoBotonIcono`, que no se puede usar aquí porque devuelve el botón
	// ya hecho y un widget no se copia.
	b.Icono, b.Rotulo, b.Radio = iconos.Puntos, rotulo, tema.RadioSM
	b.Lado, b.TamIcono = ui.LadoIconoSM, GlifoAccion
	b.Trazo, b.TrazoEncima, b.FondoEncima, b.FondoPulsado = "text-muted", "text", "bg-subtle", "selected"
	b.ExtendBaseWidget(b)
	return b
}

// MouseIn y MouseOut avisan a la tarjeta además de pintar el botón.
func (b *botonMas) MouseIn(e *desktop.MouseEvent) {
	b.BotonIcono.MouseIn(e)
	b.dentro = true
	llamar0(b.alCambiar)
}

func (b *botonMas) MouseOut() {
	b.BotonIcono.MouseOut()
	b.dentro = false
	llamar0(b.alCambiar)
}

// FocusGained y FocusLost son el `group-focus-within` de la web.
func (b *botonMas) FocusGained() {
	b.BotonIcono.FocusGained()
	b.enfoque = true
	llamar0(b.alCambiar)
}

func (b *botonMas) FocusLost() {
	b.BotonIcono.FocusLost()
	b.enfoque = false
	llamar0(b.alCambiar)
}

// verMas dice si el «…» se ve: con la tarjeta encima o enfocada, con el puntero o el
// foco en el propio botón, y SIEMPRE en táctil (`pointer-coarse:opacity-100`), donde no
// hay `hover` que valga.
func (t *Tarjeta) verMas() bool {
	if t.mas == nil {
		return false
	}
	return t.encima || t.enfocado || t.mas.dentro || t.mas.enfoque ||
		fyne.CurrentApp().Driver().Device().IsMobile()
}

// cuerpo es lo que va debajo del título, según el tipo de nota y el estado del cifrado:
// el `Cuerpo` de NoteCard.tsx.
func (t *Tarjeta) cuerpo(v tema.Variante) fyne.CanvasObject {
	if t.enLinea() {
		return container.NewWithoutLayout()
	}
	if t.Diario && t.nota.Tipo == "todo" && t.nota.Doc.Vacio() && !t.Seleccionada && t.Editor == nil {
		return container.NewWithoutLayout()
	}
	return t.cuerpoContenido(v)
}

func (t *Tarjeta) cuerpoContenido(v tema.Variante) fyne.CanvasObject {
	if t.Bloqueada {
		// Un cuaderno cerrado no enseña texto: enseña por qué no lo enseña.
		return ui.NuevoParrafo(t.tr.T("crypto.unlockHint", nil), tema.TextoBase, ui.Normal, "text-subtle", v)
	}
	n := t.nota
	switch n.Tipo {
	case "color":
		// `h-12 rounded-md border` con el color de la nota, que es un dato.
		c := hexO(datoCadena(n.Datos, "color"), hexO("#86198f", tema.Color("primary", v)))
		caja := ui.NuevaCaja(c, tema.Color("border", v), tema.RadioMD)
		return container.New(&altoFijo{alto: 12 * 0.25 * tema.Rem, caja: caja}, caja.Objetos()...)
	case "image":
		return t.cuerpoImagen(v)
	case "link", "drive", "file", "event", "room", "crossref", "launcher", "doc":
		// `grid gap-1`: el texto que traiga, en apagado, y el enlace para abrirlo. El
		// `doc` es `grid gap-1.5` con la vista previa encima (doc.go).
		var piezas []fyne.CanvasObject
		if n.Contenido != "" && n.Tipo != "doc" {
			piezas = append(piezas, ui.NuevoParrafo(n.Contenido, tema.TextoBase, ui.Normal, "text-muted", v))
		}
		url, rotulo := n.URL, ""
		if len(n.Enlaces) > 0 {
			if n.Enlaces[0].URL != "" {
				url = n.Enlaces[0].URL
			}
			rotulo = n.Enlaces[0].Label
		}
		hueco := float32(1 * 0.25 * tema.Rem)
		if n.Tipo == "doc" {
			rotulo, hueco = t.tr.T("docs.openViewer", nil), 1.5*0.25*tema.Rem
			id := ""
			if len(n.Enlaces) > 0 {
				id = n.Enlaces[0].TargetID
			}
			piezas = append(piezas, t.vistaDoc(id, v))
		}
		if e := t.enlaceExterno(url, rotulo, v); e != nil {
			piezas = append(piezas, e)
		}
		return container.New(ui.Columna{Hueco: hueco}, piezas...)
	}
	if t.Editor != nil {
		t.Editor.Estirado = t.estirada()
		return t.Editor
	}
	// Texto y tarea. Sin contenido y editable, se dice qué hacer (`note.placeholder`),
	// y pulsarlo abre el editor como pulsar el texto.
	p := NuevaProsa(n.Doc)
	if n.Doc.Vacio() && !t.SoloLectura {
		p.Marca = t.tr.T("note.placeholder", nil)
	}
	p.Llenar = t.Llenar
	if !t.SoloLectura {
		p.AlPulsar = t.AlEditar
		p.AlPlegar = t.AlPlegar
	}
	p.AlEnlace = t.AlEnlace
	p.Previa, p.tr, p.portal = t.Previa, t.tr, t.portal
	return p
}

// enlaceExterno es `EnlaceExterno`: solo http(s), con la flecha de 14 y el rótulo (o la
// dirección) en `primary-text`. nil si no hay dirección que abrir.
func (t *Tarjeta) enlaceExterno(url, rotulo string, v tema.Variante) fyne.CanvasObject {
	if !esWeb(url) {
		return nil
	}
	if rotulo == "" {
		rotulo = url
	}
	return container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem},
		Icono(iconos.FlechaArribaDerecha, GlifoFijada, "primary-text", v),
		lineaTok(rotulo, tema.TextoBase, ui.Normal, "primary-text", v))
}

// esWeb dice si una dirección es http(s): una fila antigua o un `data.url` de otra ruta
// no debe convertirse aquí en algo pulsable que no sea una página.
func esWeb(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// altoFijo da a su contenido un alto fijo y el ancho que le den: la muestra de color
// (`h-12`) y la marca de agua (`min-h-[24px]`).
type altoFijo struct {
	alto float32
	caja *ui.Caja
}

func (a *altoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, a.alto) }

func (a *altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if a.caja != nil {
		a.caja.Redimensionar(t)
		return
	}
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(t.Width, min(o.MinSize().Height, t.Height)))
	}
}

// pie es lo que va debajo del cuerpo: los adjuntos y los estados de etiqueta puestos
// en la nota (`flex flex-wrap gap-1` de insignias). nil si no hay nada.
func (t *Tarjeta) pie(v tema.Variante) fyne.CanvasObject {
	var partes []fyne.CanvasObject
	if len(t.nota.Adjuntos) > 0 {
		partes = append(partes, t.bloqueAdjuntos(v))
	}
	if len(t.nota.Estados) > 0 {
		out := make([]fyne.CanvasObject, 0, len(t.nota.Estados))
		for _, e := range t.nota.Estados {
			out = append(out, insigniaEstado(e, v))
		}
		partes = append(partes, container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: len(out), Partir: true, SaltoFila: tema.Espacio(1)}, out...))
	}
	switch len(partes) {
	case 0:
		return nil
	case 1:
		return partes[0]
	}
	return container.New(ui.Columna{Hueco: HuecoTarjeta}, partes...)
}

// escalonInsignia es `text-xs leading-none`: la caja de línea es el tamaño de la letra.
var escalonInsignia = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam}

// insigniaEstado es un estado en el pie: `<Badge tone="neutral" class="gap-1">` con el
// punto de SU color (`h-2 w-2`), el emoji si lo trae y el nombre. El color del estado es
// un DATO y va en el punto; el texto se queda en el tono neutro, que es el que lee.
func insigniaEstado(e api.NoteState, v tema.Variante) fyne.CanvasObject {
	rotulo := e.Name
	if e.Emoji != "" {
		rotulo = e.Emoji + " " + e.Name
	}
	fondo := tema.Color("bg-subtle", v)
	caja := ui.NuevaCaja(fondo, color.NRGBA{}, tema.RadioPill)
	dentro := container.New(ui.Margen{Izq: ui.MargenInsignia, Der: ui.MargenInsignia, Arr: ui.AltoInsignia, Abj: ui.AltoInsignia},
		container.New(ui.Fila{Hueco: 1 * 0.25 * tema.Rem},
			punto(PuntoEstado, hexO(e.Color, tema.Color("text-muted", v))),
			linea(rotulo, escalonInsignia, ui.SemiNegrita, tema.Color("text-muted", v))))
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(), dentro)...)
}

// punto es un círculo lleno de un color de datos (el `rounded-full` de un estado o de
// un cuaderno).
func punto(lado float32, c color.NRGBA) fyne.CanvasObject {
	r := canvas.NewRectangle(c)
	r.CornerRadius = lado / 2
	r.SetMinSize(fyne.NewSize(lado, lado))
	return r
}

func (r *tarjetaRenderer) MinSize() fyne.Size {
	return MedidaTarjetaNota(r.t, r.t.Size().Width)
}

func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaRenderer) Destroy()                     {}

// Layout coloca cabecera y cuerpo dentro del relleno.
func (r *tarjetaRenderer) Layout(t fyne.Size) {
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	// Ninguna forma lleva sombra ya (`shadow-none`): lo que separa es el aire. Y la
	// sombra es una imagen opaca por dentro: con la fila sin fondo se veía como una caja gris.
	r.sombra.Objeto().Hide()
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)

	// El relleno va DENTRO del borde (`border p-3` con `box-sizing: border-box`).
	margen, hueco, sangria := r.t.geometria()
	my := r.t.margenY()
	interior := max(t.Width-2*margen, 0)
	// Cabecera: lo de la izquierda, el título elástico y lo de la derecha, todo arriba.
	y := my
	altoC := r.t.altoCabecera(interior)
	x, der := margen, t.Width-margen
	for _, p := range r.izq {
		m := p.o.MinSize()
		p.o.Move(fyne.NewPos(x, y+p.baja))
		p.o.Resize(m)
		x += m.Width + hueco
	}
	for i := len(r.der) - 1; i >= 0; i-- {
		p := r.der[i]
		m := p.o.MinSize()
		der -= m.Width
		p.o.Move(fyne.NewPos(der, y+p.baja))
		p.o.Resize(m)
		der -= hueco
	}
	r.colocarTitulo(x, y, max(der-x, 0))
	y += altoC
	alto := altoCuerpo(r.cuerpo, interior-sangria)
	if alto > 0 || r.t.estirada() {
		y += HuecoTarjeta
	}
	var altoPie float32
	sangriaPie := r.t.sangriaPie()
	if r.pie != nil {
		altoPie = altoDe(r.pie, interior-sangriaPie)
	}
	if r.t.estirada() {
		// La tarjeta estirada: el cuerpo crece (`flex-1`) y el pie queda abajo.
		resto := t.Height - my - y
		if r.pie != nil {
			resto -= HuecoTarjeta + altoPie
		}
		alto = max(alto, resto)
	}
	if r.asa != nil {
		r.asa.Move(fyne.NewPos(PosAsa, PosAsa))
		r.asa.Resize(fyne.NewSize(GlifoAsa, GlifoAsa))
	}
	r.cuerpo.Move(fyne.NewPos(margen+sangria, y))
	r.cuerpo.Resize(fyne.NewSize(interior-sangria, alto))
	y += alto
	if r.pie != nil {
		r.pie.Move(fyne.NewPos(margen+sangriaPie, y+HuecoTarjeta))
		r.pie.Resize(fyne.NewSize(interior-sangriaPie, altoPie))
	}
}

// colocarTitulo pone el título en su hueco de la cabecera.
//
// ponytail: la web PARTE un título largo en líneas (`break-words`) y la cabecera crece;
// aquí se recorta con elipsis en una. Partirlo es medir la cabecera con el ancho, que
// `MedidaTarjetaNota` ya recibe.
func (r *tarjetaRenderer) colocarTitulo(x, y, cabe float32) {
	switch o := r.titulo.(type) {
	case *canvas.Text:
		e, peso := r.t.letraTitulo()
		ui.Truncar(o, o.Text, e, peso, cabe)
		ui.ColocarTexto(o, e, x, y)
	case *CampoEnLinea:
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(cabe, AltoCampoEnLinea(PYCampoTarjeta)))
	case *ui.Parrafo:
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(cabe, ui.AltoParrafo(o, cabe)))
	case *Prosa:
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(cabe, o.AltoPara(cabe)))
	case *EditorNota:
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(cabe, o.AltoPara(cabe)))
	default:
		m := o.MinSize()
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(min(m.Width, cabe), m.Height))
	}
}

// Refresh repinta los colores del estado sin rehacer la geometría.
func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := r.t
	r.sombra.Objeto().Hide()
	// Tres formas (NoteCard.tsx): fila, hoja y tarjeta. Solo la tarjeta lleva borde.
	var fondo, borde color.NRGBA
	switch {
	case t.Diario:
		// La elegida es un fondo, no un anillo; al pasar por encima, `bg-subtle`.
		switch {
		case t.Seleccionada && t.Editor == nil:
			fondo = tema.Color("selected", v)
		case t.encima || t.enfocado:
			fondo = tema.Color("bg-subtle", v)
		}
	case t.hoja():
		// La hoja es lo único que hay en su panel: ni caja ni anillo.
	default:
		fondo, borde = tema.Color("surface", v), tema.Color("border-subtle", v)
		switch {
		case t.Seleccionada && t.Editor == nil:
			// `ring-primary/50`: UN anillo fino y no dos píxeles de marca.
			borde = velo(tema.Color("primary", v), 0.5)
		case t.encima:
			borde = tema.Color("border", v)
		}
	}
	r.caja.Pintar(fondo, borde)
	r.aro.Pintar(v)
	r.aro.Ver(t.enfocado)
	if t.mas != nil {
		if t.verMas() {
			t.mas.Show()
		} else {
			t.mas.Hide()
		}
	}
	if r.asa != nil {
		// `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100`, y en táctil
		// siempre (al 60 %, que pinta `nuevaAsa`).
		if t.encima || t.enfocado || t.arrastrando || t.Editor != nil || esMovil() {
			r.asa.Show()
		} else {
			r.asa.Hide()
		}
	}
}

// Medidas del asa: `absolute top-1 left-1` y el glifo de 14.
const (
	PosAsa   = 1 * 0.25 * tema.Rem
	GlifoAsa = 14
)

func esMovil() bool { return fyne.CurrentApp().Driver().Device().IsMobile() }

// asaArrastre es el asa de la tarjeta en el lienzo libre: el `drag` en `text-subtle`.
// Arrastrarla mueve la tarjeta, que sigue al puntero y va por encima de las demás
// (`z-10`: `alFrente` del lienzo), y soltarla la coloca con una sola escritura. La
// alternativa de teclado es Alt+flechas (`moverConTeclado`).
type asaArrastre struct {
	widget.BaseWidget
	t      *Tarjeta
	dibujo fyne.CanvasObject
}

var (
	_ fyne.Draggable     = (*asaArrastre)(nil)
	_ desktop.Cursorable = (*asaArrastre)(nil)
)

// Cursor es `cursor-grab`, y mientras se arrastra el `cursor-grabbing` que la web pone
// en la tarjeta (`arr && 'cursor-grabbing'`). El puntero no se separa del asa durante el
// arrastre —la tarjeta lo sigue—, y el driver pregunta el cursor en cada movimiento.
func (a *asaArrastre) Cursor() desktop.Cursor {
	if a.t.arrastrando {
		return cursorAgarrando{}
	}
	return cursorAgarre{}
}

// cursorAgarre es la mano ABIERTA de `cursor-grab` y cursorAgarrando la CERRADA de
// `cursor-grabbing`. Fyne no trae ninguna de las dos, así que se pintan como las pinta
// el sistema: el dibujo en tinta con un halo blanco de un píxel para que se lea sobre
// cualquier fondo, con la punta en el centro. La abierta es la `hand` del catálogo
// compartido; la cerrada no es un icono de la web —allí es el cursor del sistema—, así
// que su trazo vive aquí y no en el espejo de `Icon.tsx` ni en el de `glyphs.tsx`.
type (
	cursorAgarre    struct{}
	cursorAgarrando struct{}
)

// LadoCursor es el lado de la imagen del cursor, en píxeles.
const LadoCursor = 24

// ManoCerradaD es el puño de `cursor-grabbing`: los cuatro nudillos doblados sobre la
// palma, en la rejilla de 24 y con el mismo trazo que la `hand` abierta.
const ManoCerradaD = "M9 11.5V9.5a1.5 1.5 0 0 1 3 0v1.5M12 10V8.5a1.5 1.5 0 0 1 3 0V11M15 10.5a1.5 1.5 0 0 1 3 0V15a6 6 0 0 1-6 6h-1a5 5 0 0 1-3.5-1.5l-2.2-2.7a1.5 1.5 0 0 1 .3-2.1l.4-.3V11a1.5 1.5 0 0 1 3 0v1.5"

func (cursorAgarre) Image() (image.Image, int, int) {
	return manoAgarre(), LadoCursor / 2, LadoCursor / 2
}

func (cursorAgarrando) Image() (image.Image, int, int) {
	return manoCerrada(), LadoCursor / 2, LadoCursor / 2
}

var (
	manoAgarre = sync.OnceValue(func() image.Image {
		return imagenCursor(func(c color.NRGBA) *iconos.Dibujo { return iconos.TrazoConTinta(iconos.Mano, LadoCursor-2, c) })
	})
	manoCerrada = sync.OnceValue(func() image.Image {
		return imagenCursor(func(c color.NRGBA) *iconos.Dibujo { return iconos.TrazoDeRuta(ManoCerradaD, LadoCursor-2, c) })
	})
)

// imagenCursor pinta un dibujo en negro sobre su halo blanco, en la imagen del cursor.
func imagenCursor(dibujo func(color.NRGBA) *iconos.Dibujo) image.Image {
	pintar := func(c color.NRGBA) image.Image {
		r, _ := dibujo(c).Objeto().(*canvas.Raster)
		return r.Generator(LadoCursor-2, LadoCursor-2)
	}
	tinta, halo := pintar(color.NRGBA{A: 255}), pintar(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	out := image.NewNRGBA(image.Rect(0, 0, LadoCursor, LadoCursor))
	for _, d := range []image.Point{{0, 1}, {2, 1}, {1, 0}, {1, 2}, {0, 0}, {2, 2}, {0, 2}, {2, 0}} {
		draw.Draw(out, halo.Bounds().Add(d), halo, image.Point{}, draw.Over)
	}
	draw.Draw(out, tinta.Bounds().Add(image.Pt(1, 1)), tinta, image.Point{}, draw.Over)
	return out
}

func nuevaAsa(t *Tarjeta, v tema.Variante) *asaArrastre {
	c := tema.Color("text-subtle", v)
	if esMovil() {
		c = velo(c, 0.6)
	}
	a := &asaArrastre{t: t, dibujo: iconos.TrazoDeRuta(AsaD, GlifoAsa, c).Objeto()}
	a.ExtendBaseWidget(a)
	return a
}

// AccessibilityLabel es el `aria-label` del botón del asa: el título de la nota.
func (a *asaArrastre) AccessibilityLabel() string { return a.t.nota.Rotulo(a.t.tr) }

func (a *asaArrastre) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.dibujo)
}

func (a *asaArrastre) Dragged(e *fyne.DragEvent) {
	t := a.t
	if !t.arrastrando {
		t.arrastrando, t.arrastre = true, t.Position()
		llamar0(t.alEmpezar)
	}
	t.arrastre = t.arrastre.Add(e.Dragged)
	// Recortado a cero por la izquierda y por arriba: una nota soltada fuera del
	// lienzo quedaría inalcanzable.
	t.Move(fyne.NewPos(max(t.arrastre.X, MargenLienzo), max(t.arrastre.Y, MargenLienzo)))
}

func (a *asaArrastre) DragEnd() {
	t := a.t
	if !t.arrastrando {
		return
	}
	t.arrastrando = false
	llamar1(t.AlSoltar, t.Position())
}

// velo baja el alfa de un color, que es el `/50` de Tailwind: `tokens.ConAlfa`, que
// redondea el alfa como el navegador (esta truncaba).
func velo(c color.NRGBA, f float32) color.NRGBA { return tokens.ConAlfa(c, float64(f)) }

// Tapped selecciona la nota. Editar es pulsarla, como una ficha de papel: no hay «modo
// edición» ni botón de lápiz, que es la decisión de la web.
func (t *Tarjeta) Tapped(*fyne.PointEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(t); c != nil {
		c.Focus(t)
	}
	llamar0(t.AlSeleccionar)
}

// MouseIn, MouseMoved y MouseOut son el `:hover` que sube la sombra.
func (t *Tarjeta) MouseIn(*desktop.MouseEvent)    { t.encima = true; t.Refresh() }
func (t *Tarjeta) MouseMoved(*desktop.MouseEvent) {}
func (t *Tarjeta) MouseOut()                      { t.encima = false; t.Refresh() }

// FocusGained y FocusLost encienden y apagan el aro. Seleccionar SIGUE al foco, que es
// el `onFocusCapture` de la web: los atajos de etiqueta actúan sobre la nota enfocada.
func (t *Tarjeta) FocusGained() {
	t.enfocado = true
	llamar0(t.AlSeleccionar)
	t.Refresh()
}

func (t *Tarjeta) FocusLost() { t.enfocado = false; t.Refresh() }

// TypedRune y TypedKey: espacio e Intro seleccionan, como un elemento de lista.
func (t *Tarjeta) TypedRune(r rune) {
	if r == ' ' {
		llamar0(t.AlSeleccionar)
		return
	}
	llamar1(t.AlTecla, r)
}

func (t *Tarjeta) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(t, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		// Intro sobre la nota la abre para escribir, como el `role="button"` de su
		// cuerpo en la web; si no se puede escribir, la elige.
		if t.AlEditar != nil && !t.SoloLectura && !t.Bloqueada {
			t.AlEditar()
			return
		}
		llamar0(t.AlSeleccionar)
		return
	}
	llamar1(t.AlTeclaEspecial, e)
}

// hexO es el color de un dato (`#rrggbb` de una etiqueta o de un cuaderno) o el de
// reserva si no se entiende. Los colores de datos NO son tokens y por eso no pasan por
// `tema.Color`, que entra en pánico con un nombre que no conoce.
func hexO(s string, reserva color.NRGBA) color.NRGBA {
	if c, ok := tokens.ParseColor(s); ok {
		return c
	}
	return reserva
}

// celdaGlifo pone su único hijo en el centro de un cuadrado de `lado`: la columna del
// glifo de la fila del diario (`flex h-5 w-5 items-center justify-center`).
type celdaGlifo struct{ lado float32 }

func (c *celdaGlifo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(c.lado, c.lado) }

func (c *celdaGlifo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		m := o.MinSize()
		o.Resize(m)
		o.Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	}
}
