// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"crypto/rand"
	"image"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El redactor (`ComposeModal.tsx`)
//
// Un diálogo `lg` (720) con la cabecera de correo en filas —De, Para, CC, CCO, Asunto—
// pegadas unas a otras por su pelo, el cuerpo debajo y el pie con las herramientas a
// la izquierda y las dos salidas a la derecha. MEDIDO en el arnés sin sesión
// (2026-09-23, 1280×800): cada fila mide 32,5 (31,5 y su pelo), el rótulo `w-16` va en
// x = 305,5 en `text-sm`, el campo empieza en x = 368,5, el cuerpo tiene `min-h` de 168
// y los botones del pie son `md` (31,5) con `gap-1` entre herramientas y `gap-2` entre
// las salidas.
//
// Lo que se comporta como en la web, porque es lo que protege lo escrito, está en
// `redactar.go`: el autoguardado, cerrar guardando, el aspa que pregunta, el aviso de
// adjunto olvidado, el envío con deshacer y ⌘/Ctrl + Intro.
//
// El cuerpo es el editor con formato (`formato.go`): la barra de la web, plegable con
// «Formato del texto» (Aa) en el pie, y un HTML que sale en `multipart/alternative` con
// su texto. Sus renglones son los 19,91 de `leading-relaxed`: el editor pinta con su
// propio maquetador y no con el `widget.Entry` de Fyne (que se quedaba en 14,70).
//
// Adjuntar (del equipo y de Drive), programar, el autocompletado y la libreta están en
// `redactar.go`, `elegirdrive.go`, `sugerencias.go` y `libreta.go`.
const (
	// VentanaDeshacer es `UNDO_WINDOW_SECONDS`.
	VentanaDeshacer = 10
	// EsperaAutoguardado es el `setTimeout(…, 2000)` del autoguardado en vivo.
	EsperaAutoguardado = 2 * time.Second

	anchoRotuloFila float32 = 16 * 0.25 * tema.Rem
	margenFila      float32 = 1 * 0.25 * tema.Rem
	huecoFilaCab    float32 = 2 * 0.25 * tema.Rem
	altoFilaCab     float32 = 9 * 0.25 * tema.Rem
	maxSelectorDe   float32 = 16 * tema.Rem
	minCuerpo       float32 = 12 * tema.Rem
	topeCuerpo      float32 = 28 * tema.Rem // y no más del 45 % del alto de la ventana
	rellenoCuerpoX  float32 = 1 * 0.25 * tema.Rem
	rellenoCuerpoY  float32 = 3 * 0.25 * tema.Rem
	huecoSalidas    float32 = 2 * 0.25 * tema.Rem
	aireAdjRedactor float32 = 3 * 0.25 * tema.Rem
	huecoAdjRed     float32 = 2 * 0.25 * tema.Rem
	altoErrorFila   float32 = 1*tema.Rem + 1.5*0.25*tema.Rem
	iconoErrorFila  float32 = 14
	aireProgramar   float32 = 4 * 0.25 * tema.Rem
	huecoProgramar  float32 = 3 * 0.25 * tema.Rem
)

// ---- Un campo dentro de una fila ----

// campoFila es lo que va a la derecha del rótulo: el selector «De», las direcciones o el
// asunto. Tres tipos que se colocan distinto, y por eso una interfaz y no un `switch`.
type campoFila interface {
	objeto() fyne.CanvasObject
	// alto es lo que pide a ese ancho; la fila nunca baja de 31,5.
	alto(ancho float32) float32
	colocar(pos fyne.Position, tam fyne.Size)
}

type campoDe struct{ s *ui.Selector }

func (c campoDe) objeto() fyne.CanvasObject { return c.s }
func (c campoDe) alto(float32) float32      { return c.s.MinSize().Height }
func (c campoDe) colocar(p fyne.Position, t fyne.Size) {
	c.s.Move(p)
	c.s.Resize(fyne.NewSize(min(t.Width, maxSelectorDe), c.s.MinSize().Height))
}

type campoDir struct{ c *CampoDirecciones }

func (c campoDir) objeto() fyne.CanvasObject            { return c.c }
func (c campoDir) alto(ancho float32) float32           { return c.c.AltoCampoDirecciones(ancho) }
func (c campoDir) colocar(p fyne.Position, t fyne.Size) { c.c.Move(p); c.c.Resize(t) }

// campoTexto es el asunto: un `<Input>` con la caja apagada (`!border-0 !bg-transparent
// !px-0 !shadow-none`), o sea el editor sin cromo en su caja de 31,5.
type campoTexto struct{ e *ui.EditorPlano }

func (c campoTexto) objeto() fyne.CanvasObject { return c.e.Objeto() }
func (c campoTexto) alto(float32) float32      { return ui.AltoCampo(ui.CampoMD) }
func (c campoTexto) colocar(p fyne.Position, t fyne.Size) {
	fa := c.e.AltoFuente()
	c.e.Colocar(fyne.NewPos(p.X, p.Y+(t.Height-fa)/2), fyne.NewSize(t.Width, fa))
}

// ---- Una fila de la cabecera (`FilaDeCabecera`) ----

// filaCabecera es el rótulo a la izquierda, el campo desnudo y su pelo debajo, que se
// oscurece con el foco y se pone en rojo con un error, dicho DENTRO de la fila.
type filaCabecera struct {
	widget.BaseWidget
	rotulo string
	campo  campoFila
	extras []fyne.CanvasObject
	Error  string
	// Enfocada es `focus-within`: el pelo pasa a `border-strong`.
	Enfocada bool
}

func nuevaFilaCabecera(rotulo string, campo campoFila, extras ...fyne.CanvasObject) *filaCabecera {
	f := &filaCabecera{rotulo: rotulo, campo: campo, extras: extras}
	f.ExtendBaseWidget(f)
	return f
}

// interior es el ancho que le queda al campo: el `flex-1` entre el rótulo y los extras.
func (f *filaCabecera) interior(ancho float32) float32 {
	w := ancho - 2*margenFila - anchoRotuloFila - huecoFilaCab
	for _, e := range f.extras {
		if e.Visible() {
			w -= e.MinSize().Width + huecoFilaCab
		}
	}
	return max(w, 0)
}

func (f *filaCabecera) fila(ancho float32) float32 {
	return max(altoFilaCab, f.campo.alto(f.interior(ancho)))
}

// AltoFila es lo que mide la fila a ese ancho: su línea (31,5, o lo que pida el campo
// si parte en fichas), el pelo y, con error, su renglón.
func (f *filaCabecera) AltoFila(ancho float32) float32 {
	alto := f.fila(ancho) + tema.Borde
	if f.Error != "" {
		alto += altoErrorFila
	}
	return alto
}

func (f *filaCabecera) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaCabeceraRenderer{
		f:        f,
		rotulo:   ui.Texto(f.rotulo, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		pelo:     canvas.NewRectangle(tema.Color("border", v)),
		error:    ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("danger-text", v)),
		iconoErr: iconos.Trazo(iconos.Peligro, iconoErrorFila, "danger-text", v),
	}
	r.objetos = append([]fyne.CanvasObject{r.rotulo, f.campo.objeto()}, f.extras...)
	r.objetos = append(r.objetos, r.pelo, r.error, r.iconoErr.Objeto())
	r.Refresh()
	return r
}

type filaCabeceraRenderer struct {
	f        *filaCabecera
	rotulo   *canvas.Text
	pelo     *canvas.Rectangle
	error    *canvas.Text
	iconoErr *iconos.Dibujo
	objetos  []fyne.CanvasObject
}

func (r *filaCabeceraRenderer) MinSize() fyne.Size {
	return fyne.NewSize(anchoRotuloFila, r.f.AltoFila(r.f.Size().Width))
}
func (r *filaCabeceraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaCabeceraRenderer) Destroy()                     {}

func (r *filaCabeceraRenderer) Layout(t fyne.Size) {
	f := r.f
	fila := f.fila(t.Width)
	centro := func(h float32) float32 { return (fila - h) / 2 }
	x := margenFila
	ui.ColocarTexto(r.rotulo, tema.TextoSM, x, centro(tema.TextoSM.Linea))
	x += anchoRotuloFila + huecoFilaCab
	w := f.interior(t.Width)
	h := f.campo.alto(w)
	f.campo.colocar(fyne.NewPos(x, centro(h)), fyne.NewSize(w, h))
	fin := t.Width - margenFila
	for i := len(f.extras) - 1; i >= 0; i-- {
		e := f.extras[i]
		if !e.Visible() {
			continue
		}
		m := e.MinSize()
		fin -= m.Width
		e.Move(fyne.NewPos(fin, centro(m.Height)))
		e.Resize(m)
		fin -= huecoFilaCab
	}
	r.pelo.Move(fyne.NewPos(0, fila))
	r.pelo.Resize(fyne.NewSize(t.Width, tema.Borde))
	if f.Error != "" {
		// `flex items-center gap-1 px-1 pb-1.5 text-xs` con el icono de 14.
		y := fila + tema.Borde
		r.iconoErr.Colocar(margenFila, y+(tema.TextoXS.Linea-iconoErrorFila)/2, iconoErrorFila)
		ui.ColocarTexto(r.error, tema.TextoXS, margenFila+iconoErrorFila+tema.Espacio(1), y)
	}
}

func (r *filaCabeceraRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Color = tema.Color("text-muted", v)
	pelo := "border"
	switch {
	case f.Error != "":
		pelo = "danger"
	case f.Enfocada:
		pelo = "border-strong"
	}
	r.pelo.FillColor = tema.Color(pelo, v)
	r.error.Text, r.error.Color = f.Error, tema.Color("danger-text", v)
	verlo(r.error, f.Error != "")
	r.iconoErr.Tenir("danger-text", v)
	r.iconoErr.Ver(f.Error != "")
	for _, o := range r.objetos {
		o.Refresh()
	}
	if t := f.Size(); t.Width > 0 {
		r.Layout(t)
	}
}

// ---- «CC/CCO»: un botón de texto ----

// botonTexto es `text-text-muted hover:text-text rounded-sm px-1.5 py-1 text-xs`: un
// `<button>` sin caja, que solo cambia de tinta.
type botonTexto struct {
	ui.Pulsable
	texto string
}

func nuevoBotonTexto(texto string, alPulsar func()) *botonTexto {
	b := &botonTexto{texto: texto}
	b.Nombre, b.AlPulsar = texto, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonTexto) MinSize() fyne.Size {
	return fyne.NewSize(ui.Medir(b.texto, tema.TextoXS, ui.Normal).Width+2*tema.Espacio(1.5),
		tema.TextoXS.Linea+2*tema.Espacio(1))
}

func (b *botonTexto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonTextoRenderer{
		b:   b,
		aro: ui.NuevoAroCon("focus-ring", tema.RadioSM, ui.SinHueco, v),
		t:   ui.Texto(b.texto, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.t)
	return r
}

type botonTextoRenderer struct {
	b       *botonTexto
	aro     *ui.Aro
	t       *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *botonTextoRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonTextoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonTextoRenderer) Destroy()                     {}
func (r *botonTextoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	ui.ColocarTexto(r.t, tema.TextoXS, tema.Espacio(1.5), tema.Espacio(1))
}

func (r *botonTextoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := "text-muted"
	if r.b.Encima() {
		tinta = "text"
	}
	r.t.Color = tema.Color(tinta, v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.t.Refresh()
}

// ---- Un adjunto del borrador ----

// fichaAdjunto es `bg-bg-subtle rounded-full py-1 pl-3 pr-1 text-sm gap-2`: el nombre,
// el tamaño en `text-subtle` y el aspa de 24 con el glifo de 14.
type fichaAdjunto struct {
	widget.BaseWidget
	nombre, tamano string
	aspa           *ui.BotonIcono
}

const (
	margenIzqAdjRed float32 = 3 * 0.25 * tema.Rem
	margenDerAdjRed float32 = 1 * 0.25 * tema.Rem
	rellenoAdjRed   float32 = 1 * 0.25 * tema.Rem
	aspaAdjRed      float32 = 24
)

func nuevaFichaAdjunto(nombre, tamano, rotulo string, alQuitar func()) *fichaAdjunto {
	f := &fichaAdjunto{nombre: nombre, tamano: tamano}
	f.aspa = ui.NuevoBotonIcono(iconos.Cerrar, rotulo, alQuitar)
	f.aspa.Lado, f.aspa.TamIcono = aspaAdjRed, 14
	f.aspa.Trazo, f.aspa.TrazoEncima, f.aspa.FondoEncima = "text-subtle", "text", "border"
	f.ExtendBaseWidget(f)
	return f
}

func (f *fichaAdjunto) MinSize() fyne.Size {
	ancho := margenIzqAdjRed + ui.Medir(f.nombre, tema.TextoSM, ui.Normal).Width + huecoAdjRed + aspaAdjRed + margenDerAdjRed
	if f.tamano != "" { // los de Drive no llevan tamaño, ni su hueco
		ancho += ui.Medir(f.tamano, tema.TextoSM, ui.Normal).Width + huecoAdjRed
	}
	return fyne.NewSize(ancho, 2*rellenoAdjRed+aspaAdjRed)
}

func (f *fichaAdjunto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &fichaAdjuntoRenderer{
		f:      f,
		caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioPill),
		nombre: ui.Texto(f.nombre, tema.TextoSM, ui.Normal, tema.Color("text", v)),
		tamano: ui.Texto(f.tamano, tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.nombre, r.tamano, f.aspa)
	return r
}

type fichaAdjuntoRenderer struct {
	f              *fichaAdjunto
	caja           *ui.Caja
	nombre, tamano *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *fichaAdjuntoRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *fichaAdjuntoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *fichaAdjuntoRenderer) Destroy()                     {}
func (r *fichaAdjuntoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	y := (t.Height - tema.TextoSM.Linea) / 2
	fin := t.Width - margenDerAdjRed - aspaAdjRed
	r.f.aspa.Move(fyne.NewPos(fin, (t.Height-aspaAdjRed)/2))
	r.f.aspa.Resize(fyne.NewSize(aspaAdjRed, aspaAdjRed))
	if r.f.tamano != "" {
		fin -= huecoAdjRed + ui.Medir(r.f.tamano, tema.TextoSM, ui.Normal).Width
		ui.ColocarTexto(r.tamano, tema.TextoSM, fin, y)
	}
	ui.Truncar(r.nombre, r.f.nombre, tema.TextoSM, ui.Normal, max(fin-huecoAdjRed-margenIzqAdjRed, 0))
	ui.ColocarTexto(r.nombre, tema.TextoSM, margenIzqAdjRed, y)
}

func (r *fichaAdjuntoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v))
	r.nombre.Color, r.tamano.Color = tema.Color("text", v), tema.Color("text-subtle", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- El cuerpo del diálogo ----

// Redactor es el formulario del correo que se escribe. El envío, el guardado y el
// cierre son del Buzón (`redactar.go`): esto solo pinta y recoge.
type Redactor struct {
	widget.BaseWidget
	T      *i18n.Translator
	Locale i18n.Locale
	// B es lo que hay escrito; se actualiza con cada tecla.
	B Borrador
	// OpcionesDe son las direcciones «De»: con una sola no hay fila.
	OpcionesDe []string
	// AlCambiar avisa de cada cambio (el autoguardado cuelga de aquí).
	AlCambiar func()
	// AlEnviar es ⌘/Ctrl + Intro dentro de cualquier campo.
	AlEnviar func()
	// AlPegar es pegar en el cuerpo (⌘/Ctrl + V): quien lo pone busca el HTML del
	// portapapeles y lo pega con su formato, o `texto` si no lo hay. Sin él, el cuerpo
	// pega el texto.
	AlPegar func(texto string)
	// AlBuscar es lo que se escribe en Para, CC o CCO, para el autocompletado.
	AlBuscar func(c *CampoDirecciones, q string)
	// AlLibreta abre «Elegir de Contactos» para ese campo.
	AlLibreta func(c *CampoDirecciones)
	libretas  []*ui.BotonIcono
	// sugerencias es el panel abierto bajo campoSug, si lo hay.
	sugerencias fyne.CanvasObject
	campoSug    *CampoDirecciones
	// AlProgramar es el «Programar envío» del panel, con la hora elegida.
	AlProgramar func(time.Time, bool)
	// AlCrecer avisa de que la columna ha cambiado de alto (CC/CCO, un error, un
	// adjunto quitado): quien la aloja tiene que volver a medir su diálogo.
	AlCrecer func()
	// AlImagenDelEquipo es «Desde este dispositivo» del panel de imagen
	// (`onInsertarImagen`): quien lo pone elige el fichero y lo mete con InsertarImagen.
	// Sin él, el panel solo ofrece la dirección.
	AlImagenDelEquipo func()

	ancho            float32 // el del cuerpo del diálogo; lo fija quien lo abre
	filaDe, filaPara *filaCabecera
	filaCc, filaCco  *filaCabecera
	filaAsunto       *filaCabecera
	para, cc, cco    *CampoDirecciones
	asunto           *ui.EditorPlano
	// rico es el cuerpo, dentro de su desplazador (`max-h` de 28rem o el 45 % de la
	// ventana, `overflow-y-auto`); barra, su barra de formato (VerFormato la enseña) con su
	// pelo debajo, y la de tabla con el suyo cuando el cursor está en una celda (verTabla);
	// panel, el de enlace o el de imagen abierto, si lo hay.
	rico        *notes.EditorNota
	cuerpo      *container.Scroll
	barra       *barraFormato
	fileteBarra *canvas.Rectangle
	fileteTabla *canvas.Rectangle
	VerFormato  bool
	verTabla    bool
	panel       *panelEnlace
	// imagenes son las del cuerpo por su `src`, ya decodificadas; nil mientras llega (o
	// si no hay), para no pedirla dos veces.
	imagenes map[string]image.Image
	// iaFallo es que el asistente no ha contestado: avisoIA lo dice bajo la barra.
	iaFallo       bool
	avisoIA       *ui.Parrafo
	ventana       float32
	ccCco         *botonTexto
	verCc         bool
	textoAdjuntos *canvas.Text
	fichas        []*fichaAdjunto
	contenedor    *fyne.Container
	// El panel de «Programar envío»: se despliega con el reloj del pie.
	verProgramar    bool
	fileteProgramar *canvas.Rectangle
	cuando          *campoCuando
	grupoProgramar  *fyne.Container
	BotonProgramar  *ui.Boton
}

// NuevoRedactor monta el formulario con lo que trae el borrador.
func NuevoRedactor(t *i18n.Translator, l i18n.Locale, b Borrador, opcionesDe []string, ancho float32) *Redactor {
	return NuevoRedactorEn(t, l, b, opcionesDe, ancho, 0)
}

// NuevoRedactorEn es NuevoRedactor sabiendo el ancho de la VENTANA, que decide lo que la
// web decide con `md:` y `max-[820px]:` (la barra en una fila y el alto del cuerpo).
func NuevoRedactorEn(t *i18n.Translator, l i18n.Locale, b Borrador, opcionesDe []string, ancho, ventana float32) *Redactor {
	r := &Redactor{T: t, Locale: l, B: b, OpcionesDe: opcionesDe, ancho: ancho, ventana: ventana, verCc: b.Cc != "" || b.Cco != ""}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cambio := func() {
		if r.AlCambiar != nil {
			r.AlCambiar()
		}
	}
	quitar := func(n string) string { return t.T("compose.removeRecipient", i18n.Vars{"name": n}) }
	direcciones := func(marca, valor string, poner func(string)) *CampoDirecciones {
		c := NuevoCampoDirecciones(marca)
		c.RotuloQuitar, c.AvisoNoValida = quitar, t.T("compose.invalidAddress", nil)
		c.Poner(valor)
		c.AlCambiar = func(s string) { poner(s); cambio() }
		c.editor.Atajo = r.atajoEnviar
		c.Buscar = func(q string) {
			if r.AlBuscar != nil {
				r.AlBuscar(c, q)
			}
		}
		c.AlSugerir = func(p fyne.CanvasObject) {
			r.sugerencias, r.campoSug = p, c
			r.relayout()
		}
		return c
	}
	r.para = direcciones(t.T("compose.toPlaceholder", nil), b.Para, func(s string) {
		r.B.Para = s
		if r.filaPara.Error != "" { // escribir quita el aviso de «falta destinatario»
			r.filaPara.Error = ""
			r.relayout()
		}
	})
	r.cc = direcciones(t.T("compose.ccPlaceholder", nil), b.Cc, func(s string) { r.B.Cc = s })
	r.cco = direcciones(t.T("compose.bccPlaceholder", nil), b.Cco, func(s string) { r.B.Cco = s })

	r.asunto = ui.NuevoEditorPlano(tema.TextoBase, nil)
	r.asunto.PlaceHolder = t.T("compose.subjectPlaceholder", nil)
	r.asunto.PonerTexto(b.Asunto)
	r.asunto.AlCambiar(func(s string) { r.B.Asunto = s; cambio() })
	r.asunto.Atajo = r.atajoEnviar

	// El cuerpo: el HTML del borrador si lo trae, y si no su texto (la cita y la firma)
	// un `<div>` por renglón. Un HTML que el editor no entiende entero (un borrador de
	// la web con una tabla) se abre por su texto: editarlo lo perdería.
	html := b.HTML
	if d := notes.Leer(html); html == "" || !d.Fiel() || len(d.NoPintado()) > 0 {
		html = HTMLDeTexto(b.Cuerpo)
	}
	r.rico = notes.NuevoEditorNota(t, notes.Leer(html), t.T("compose.bodyPlaceholder", nil))
	r.rico.AlPrincipio() // se escribe ENCIMA de la cita
	r.rico.AlCambiar = func(h string) {
		r.B.HTML, r.B.Cuerpo = h, TextoDe(r.rico.Documento())
		r.barra.refrescar()
		cambio()
	}
	r.rico.AlCrecer = r.relayout
	r.rico.Atajo = r.atajoCuerpo
	r.rico.Tecla = r.teclaCuerpo
	r.rico.AlEnlazar = func(fyne.CanvasObject) { r.abrirEnlace() }
	r.rico.Imagenes = r.imagen
	r.rico.AlMoverCursor = r.alMoverCursor
	zona, ok := r.rico.Zona().(fyne.CanvasObject)
	if !ok {
		panic("mail: la zona del editor no es un CanvasObject")
	}
	r.cuerpo = container.NewVScroll(container.New(marcoCuerpo{}, zona))
	r.barra = nuevaBarraFormato(t, r.rico, ventana, r.abrirEnlace, r.abrirImagen, nil)
	r.fileteBarra = canvas.NewRectangle(tema.Color("border", v))
	r.fileteTabla = canvas.NewRectangle(tema.Color("border", v))
	r.VerFormato = true

	r.ccCco = nuevoBotonTexto(t.T("compose.ccBcc", nil), func() { r.verCc = true; r.relayout() })
	// El icono de Contactos de cada fila (`IconButton size="sm"` con `users` de 16):
	// abre la libreta para ESE campo. Sin libreta enganchada no se ve.
	libreta := func(c *CampoDirecciones) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Usuarios, t.T("compose.addressBook", nil), func() {
			if r.AlLibreta != nil {
				r.AlLibreta(c)
			}
		})
		b.Lado, b.TamIcono = ui.LadoIconoSM, 16
		b.Hide()
		r.libretas = append(r.libretas, b)
		return b
	}
	r.filaPara = nuevaFilaCabecera(t.T("compose.to", nil), campoDir{r.para}, libreta(r.para), r.ccCco)
	r.filaCc = nuevaFilaCabecera(t.T("compose.cc", nil), campoDir{r.cc}, libreta(r.cc))
	r.filaCco = nuevaFilaCabecera(t.T("compose.bcc", nil), campoDir{r.cco}, libreta(r.cco))
	r.filaAsunto = nuevaFilaCabecera(t.T("common.subject", nil), campoTexto{r.asunto})
	for _, par := range []struct {
		f *filaCabecera
		c *CampoDirecciones
	}{{r.filaPara, r.para}, {r.filaCc, r.cc}, {r.filaCco, r.cco}} {
		f := par.f
		par.c.AlEnfocar = func(si bool) { f.Enfocada = si; f.Refresh() }
	}
	if len(opcionesDe) > 1 {
		i := 0
		for j, o := range opcionesDe {
			if o == b.De {
				i = j
			}
		}
		s := ui.NuevoSelector(opcionesDe, func(j int) { r.B.De = opcionesDe[j]; cambio() })
		s.Elegida = i
		r.B.De = opcionesDe[i]
		r.filaDe = nuevaFilaCabecera(t.T("compose.from", nil), campoDe{s})
	}
	r.textoAdjuntos = ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	r.contenedor = container.New(&columnaRedactor{r: r})
	r.montar()
	r.ExtendBaseWidget(r)
	return r
}

// Foco es donde se empieza a escribir: el «Para» en un mensaje nuevo, y el cuerpo al
// responder o reenviar, que es lo que falta (`destinoDelFoco`).
func (r *Redactor) Foco() fyne.Focusable {
	if r.B.Para != "" {
		return r.rico.Zona()
	}
	return r.para.Foco()
}

// PonerError es la validación en línea de «Para» (`compose.toRequired`).
func (r *Redactor) PonerError(msg string) {
	r.filaPara.Error = msg
	r.relayout()
}

func (r *Redactor) relayout() {
	r.montar()
	r.contenedor.Refresh()
	r.Refresh()
	if r.AlCrecer != nil {
		r.AlCrecer()
	}
}

// montar arma la columna en el orden de la web: De, Para, CC, CCO, Asunto, cuerpo y
// adjuntos.
func (r *Redactor) montar() {
	var objs []fyne.CanvasObject
	if r.filaDe != nil {
		objs = append(objs, r.filaDe)
	}
	verlo(r.ccCco, !r.verCc)
	objs = append(objs, r.filaPara)
	if r.verCc {
		objs = append(objs, r.filaCc, r.filaCco)
	}
	objs = append(objs, r.filaAsunto)
	if r.VerFormato {
		objs = append(objs, r.barra.caja, r.fileteBarra)
	}
	if r.iaFallo {
		objs = append(objs, r.avisoIA)
	}
	if r.verTabla {
		objs = append(objs, r.barra.tabla.caja, r.fileteTabla)
	}
	if r.panel != nil {
		objs = append(objs, r.panel.caja)
	}
	objs = append(objs, r.cuerpo)
	r.fichas = r.fichas[:0]
	if n := len(r.B.Adjuntos) + len(r.B.Drive); n > 0 {
		r.textoAdjuntos.Text = r.T.T("compose.attachmentCount", i18n.Vars{"count": n})
		objs = append(objs, r.textoAdjuntos)
		for i, a := range r.B.Adjuntos {
			nombre := nombreAdjunto(a)
			f := nuevaFichaAdjunto(nombre, i18n.FormatBytes(r.Locale, float64(len(a.Datos))),
				r.T.T("compose.removeAttachment", i18n.Vars{"name": nombre}), func() { r.quitarAdjunto(i) })
			r.fichas = append(r.fichas, f)
			objs = append(objs, f)
		}
		// Los de Drive van detrás y sin tamaño: son un enlace.
		for i, e := range r.B.Drive {
			f := nuevaFichaAdjunto(e.Nombre, "", r.T.T("compose.removeAttachment", i18n.Vars{"name": e.Nombre}),
				func() { r.quitarDrive(i) })
			r.fichas = append(r.fichas, f)
			objs = append(objs, f)
		}
	}
	if r.verProgramar {
		r.montarProgramar()
		objs = append(objs, r.fileteProgramar, r.grupoProgramar, r.BotonProgramar)
	}
	// Las sugerencias van las ÚLTIMAS: encima del asunto y del cuerpo, como la lista
	// `absolute` de la web.
	if r.sugerencias != nil {
		objs = append(objs, r.sugerencias)
	}
	r.contenedor.Objects = objs
}

// montarProgramar arma el panel la primera vez, con la hora de dentro de una hora
// (`toLocalInput(now + 3.600.000)` de la web).
func (r *Redactor) montarProgramar() {
	if r.cuando != nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fileteProgramar = canvas.NewRectangle(tema.Color("border", v))
	r.cuando = nuevoCampoCuando(time.Now().Add(time.Hour))
	r.grupoProgramar = ui.GrupoCampo(r.T.T("compose.scheduleDateTime", nil), false, r.cuando.objeto(), "", "")
	r.BotonProgramar = ui.NuevoBoton(r.T.T("compose.schedule", nil), func() {
		if r.AlProgramar != nil {
			cuando, ok := r.cuando.leer()
			r.AlProgramar(cuando, ok)
		}
	})
}

// AlternarProgramar despliega o pliega el panel.
func (r *Redactor) AlternarProgramar() {
	r.verProgramar = !r.verProgramar
	r.relayout()
}

// ConLibreta enseña u oculta el icono de Contactos de las filas de direcciones.
func (r *Redactor) ConLibreta(si bool) {
	for _, b := range r.libretas {
		if si {
			b.Show()
		} else {
			b.Hide()
		}
	}
	r.relayout()
}

// Adjuntar añade ficheros al borrador (`setFiles(prev => [...prev, ...nuevos])`).
func (r *Redactor) Adjuntar(as ...eml.Adjunto) {
	if len(as) == 0 {
		return
	}
	r.B.Adjuntos = append(r.B.Adjuntos, as...)
	r.relayout()
	if r.AlCambiar != nil {
		r.AlCambiar()
	}
}

// AdjuntarDrive añade un enlace de Drive, sin repetir el mismo.
func (r *Redactor) AdjuntarDrive(e EnlaceDrive) {
	if slices.ContainsFunc(r.B.Drive, func(x EnlaceDrive) bool { return x.URL == e.URL }) {
		return
	}
	r.B.Drive = append(r.B.Drive, e)
	r.relayout()
	if r.AlCambiar != nil {
		r.AlCambiar()
	}
}

func (r *Redactor) quitarDrive(i int) {
	if i < 0 || i >= len(r.B.Drive) {
		return
	}
	r.B.Drive = append(r.B.Drive[:i:i], r.B.Drive[i+1:]...)
	r.relayout()
	if r.AlCambiar != nil {
		r.AlCambiar()
	}
}

func (r *Redactor) quitarAdjunto(i int) {
	if i < 0 || i >= len(r.B.Adjuntos) {
		return
	}
	r.B.Adjuntos = append(r.B.Adjuntos[:i:i], r.B.Adjuntos[i+1:]...)
	r.relayout()
	if r.AlCambiar != nil {
		r.AlCambiar()
	}
}

// AlternarFormato es «Formato del texto» (Aa): pliega o despliega la barra.
func (r *Redactor) AlternarFormato() {
	r.VerFormato = !r.VerFormato
	if !r.VerFormato {
		r.panel = nil
	}
	r.relayout()
}

// abrirEnlace es `abrirPanel('enlace')`: el panel bajo la barra, con lo seleccionado
// como rótulo y el foco en la dirección.
func (r *Redactor) abrirEnlace() {
	r.panel = nuevoPanelEnlace(r.T, r.rico.Seleccionado(), r.ventana, r.aplicarEnlace, r.cerrarPanel)
	r.relayout()
	r.enfocarPanel()
}

// abrirImagen es `abrirPanel('imagen')`.
func (r *Redactor) abrirImagen() {
	r.panel = nuevoPanelImagen(r.T, r.ventana, r.AlImagenDelEquipo, r.aplicarImagen, r.cerrarPanel,
		func() { r.relayout(); r.enfocarPanel() })
	r.relayout()
}

// enfocarPanel lleva el foco a la dirección del panel, si la pide.
func (r *Redactor) enfocarPanel() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(r.contenedor); c != nil && r.panel != nil && r.panel.url != nil {
		c.Focus(r.panel.url)
	}
}

// aplicarEnlace es `aplicarPanel`: sin `http(s)://` ni `mailto:` no se enlaza nada.
func (r *Redactor) aplicarEnlace(url, rotulo string) {
	r.cerrarPanel()
	if URLDeEnlaceValida(url) {
		r.rico.Enlazar(url, rotulo)
	}
}

// aplicarImagen es `insertarImagen`: la de una dirección `https://`, donde está el cursor.
func (r *Redactor) aplicarImagen(url string) {
	r.cerrarPanel()
	if URLDeImagenValida(url) {
		r.rico.Orden("imagen", url)
	}
}

// InsertarImagen es `insertarImagenLocal`: la imagen va al mensaje como una parte más
// —con su `cid`, que es lo que la pone DENTRO del texto (`multipart/related`) y no como
// fichero— y en el cursor queda el `<img src="cid:…">` que apunta a ella. Sale también en
// la lista de adjuntos, como en la web.
func (r *Redactor) InsertarImagen(a eml.Adjunto) {
	a.CID = nuevoCID()
	r.B.Adjuntos = append(r.B.Adjuntos, a)
	r.rico.Orden("imagen", "cid:"+a.CID)
	r.relayout()
}

// nuevoCID es el `cid` de una imagen del cuerpo: sin caracteres raros, que acaba en una
// cabecera del mensaje (`img` + `crypto.randomUUID()` sin guiones en la web).
func nuevoCID() string { return "img" + rand.Text() }

func (r *Redactor) cerrarPanel() {
	r.panel = nil
	r.relayout()
	if c := fyne.CurrentApp().Driver().CanvasForObject(r.contenedor); c != nil {
		c.Focus(r.rico.Zona())
	}
}

// bajarImagen trae una imagen `https:` del cuerpo; los guardianes la sustituyen.
var bajarImagen = externo.Imagen

// imagen es el `Documento.Imagenes` del cuerpo: la de una parte `cid:` del mensaje o la
// de una dirección `https:`, decodificada en su rutina —una foto de móvil tarda en
// descomprimirse, y la red más— y pintada al llegar. Hasta entonces, el hueco de Notepads.
//
// ponytail: se decodifica a su tamaño y se guarda así; si una foto enorme pesa al
// pintar, reducirla al ancho del cuerpo al llegar.
func (r *Redactor) imagen(src string) image.Image {
	if img, ok := r.imagenes[src]; ok {
		return img
	}
	if r.imagenes == nil {
		r.imagenes = map[string]image.Image{}
	}
	r.imagenes[src] = nil
	var datos []byte
	if cid, ok := strings.CutPrefix(src, "cid:"); ok {
		i := slices.IndexFunc(r.B.Adjuntos, func(a eml.Adjunto) bool { return a.CID == cid })
		if i < 0 {
			return nil
		}
		datos = r.B.Adjuntos[i].Datos
	} else if !URLDeImagenValida(src) {
		return nil
	}
	go func() {
		var img image.Image
		if datos != nil {
			img, _ = externo.Decodificar(datos)
		} else {
			ctx, cancelar := context.WithTimeout(context.Background(), externo.Tiempo)
			img, _ = bajarImagen(ctx, src)
			cancelar()
		}
		if img == nil {
			return
		}
		fyne.Do(func() {
			r.imagenes[src] = img
			if z, ok := r.rico.Zona().(fyne.Widget); ok {
				z.Refresh()
			}
			r.relayout()
		})
	}()
	return nil
}

// Medidas del fallo del asistente: `px-1 py-0.5`.
const (
	pxAvisoIA float32 = 1 * 0.25 * tema.Rem
	pyAvisoIA float32 = 0.5 * 0.25 * tema.Rem
)

// ConAsistente pone el asistente de IA al final de la barra: `asistir` es Sytue AI (el
// `/api/ai` de la web, en directo y con la sesión) e `idioma`, la lengua en que contesta.
// Sin él no sale.
func (r *Redactor) ConAsistente(asistir func(context.Context, api.PeticionIA) (string, error), idioma string) {
	r.rico.Asistir, r.rico.Idioma, r.rico.AlAsistir = asistir, idioma, r.alAsistir
	r.rico.TitulosEnNegrita = true // `escribirBloques`: el título, en negrita en su línea
	r.avisoIA = ui.NuevoParrafo(r.T.T("editor.aiError", nil), tema.TextoXS, ui.Normal, "danger-text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.barra = nuevaBarraFormato(r.T, r.rico, r.ventana, r.abrirEnlace, r.abrirImagen, r.menuIA)
	r.relayout()
}

// menuIA es el desplegable del asistente, colgando de su botón.
func (r *Redactor) menuIA(disparador fyne.CanvasObject) {
	elementos := make([]fyne.CanvasObject, 0, len(accionesIA))
	for _, a := range accionesIA {
		if a.accion == "" {
			elementos = append(elementos, ui.NuevoSeparadorMenu())
			continue
		}
		elementos = append(elementos, ui.NuevoElementoMenu(r.T.T(a.clave, nil), iconos.Ninguno, func() { r.PedirIA(a.accion) }))
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = r.T.T("editor.ai", nil)
	m.Mostrar(disparador)
}

// PedirIA es `aplicarIA`: la acción sobre lo seleccionado o, sin selección, el correo
// entero. Lo que vuelve lo aplica el editor de Notepads, en un paso de deshacer.
func (r *Redactor) PedirIA(accion string) {
	r.rico.PedirIA(func(texto string) api.PeticionIA { return PeticionIA(accion, texto) })
	if c := fyne.CurrentApp().Driver().CanvasForObject(r.contenedor); c != nil {
		c.Focus(r.rico.Zona())
	}
}

// alAsistir apaga el botón mientras el asistente trabaja y dice si ha fallado; una
// petición nueva quita el aviso de la anterior.
func (r *Redactor) alAsistir(trabajando, fallo bool) {
	r.barra.trabajandoIA(trabajando)
	if fallo != r.iaFallo {
		r.iaFallo = fallo
		r.relayout()
	}
}

// alMoverCursor sigue al cursor: la barra enciende lo que hay bajo él, la de tabla sale
// o se va al entrar o salir de una celda, y el desplazador del cuerpo lo tiene a la
// vista (el `overflow-y-auto` del navegador).
func (r *Redactor) alMoverCursor(arriba, alto float32) {
	r.barra.refrescar()
	if en := r.rico.Activa("tabla", ""); en != r.verTabla {
		r.verTabla = en
		r.relayout()
	}
	s := r.cuerpo
	arriba += anilloCuerpo // la zona va dentro de su marco
	h := s.Size().Height
	if h <= 0 {
		return
	}
	// Con su renglón y el relleno del cuerpo alrededor: al principio, arriba del todo.
	margen := float32(LineaCuerpo/2) + notes.PYZonaEd
	off := s.Offset.Y
	switch {
	case arriba-margen < off:
		off = arriba - margen
	case arriba+alto+margen > off+h:
		off = arriba + alto + margen - h
	}
	off = max(0, min(off, s.Content.Size().Height-h))
	if off != s.Offset.Y {
		s.ScrollToOffset(fyne.NewPos(0, off))
	}
}

// teclaCuerpo son las teclas del cuerpo que son del redactor y no del editor: Escape
// cierra el diálogo, como en cualquier otro campo, y el tabulador SALE del cuerpo (en
// la web el `contenteditable` no lo atrapa; en Notepads es sangrar).
func (r *Redactor) teclaCuerpo(k *fyne.KeyEvent) bool {
	c := fyne.CurrentApp().Driver().CanvasForObject(r.contenedor)
	switch k.Name {
	case fyne.KeyEscape:
		return c != nil && ui.Descartar(c)
	case fyne.KeyTab:
		if c != nil {
			if r.rico.Mayus() {
				c.FocusPrevious()
			} else {
				c.FocusNext()
			}
		}
		return true
	default:
		return false
	}
}

// atajoCuerpo son los atajos del cuerpo: pegar con formato (⌘/Ctrl + V) y sin él
// (⌘/Ctrl + Mayús + V, como en la web), y los de cualquier campo.
func (r *Redactor) atajoCuerpo(s fyne.Shortcut) bool {
	switch x := s.(type) {
	case *fyne.ShortcutPaste:
		if r.AlPegar == nil || x.Clipboard == nil {
			return false
		}
		r.AlPegar(x.Clipboard.Content())
		return true
	case *desktop.CustomShortcut:
		if x.KeyName == fyne.KeyV && x.Modifier&fyne.KeyModifierShift != 0 &&
			x.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0 {
			if a := fyne.CurrentApp(); a != nil {
				r.rico.PegarTexto(a.Clipboard().Content())
			}
			return true
		}
	}
	return r.atajoEnviar(s)
}

// atajoEnviar es ⌘/Ctrl + Intro en cualquier campo (la web escucha `metaKey` y
// `ctrlKey` a la vez). Fyne lo manda al campo enfocado como atajo, no como tecla.
func (r *Redactor) atajoEnviar(s fyne.Shortcut) bool {
	c, ok := s.(*desktop.CustomShortcut)
	if !ok || r.AlEnviar == nil || (c.KeyName != fyne.KeyReturn && c.KeyName != fyne.KeyEnter) ||
		c.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) == 0 {
		return false
	}
	r.AlEnviar()
	return true
}

// Objeto es lo que va de cuerpo del diálogo.
func (r *Redactor) Objeto() fyne.CanvasObject { return r.contenedor }

// CreateRenderer: el Redactor como widget solo existe para que sus campos compartan
// ciclo de vida; lo que se pinta es su columna.
func (r *Redactor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.contenedor)
}

// anilloCuerpo es lo que el anillo de foco del editor (`ring-2`) se pinta por fuera de
// la zona: su marco dentro del desplazador, que si no lo recortaría.
const anilloCuerpo float32 = 2

// marcoCuerpo pone la zona dentro del desplazador con el sitio de su anillo alrededor.
//
// ponytail: pasado el tope, el anillo es el del contenido y su lado de abajo se desplaza
// con él (en la web es el de la caja que se ve); dibujarlo aquí fuera pediría saber del
// foco de la zona, que es de Notepads.
type marcoCuerpo struct{}

func (marcoCuerpo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*anilloCuerpo, m.Height+2*anilloCuerpo)
}

func (marcoCuerpo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(anilloCuerpo, anilloCuerpo))
	objs[0].Resize(fyne.NewSize(t.Width-2*anilloCuerpo, t.Height-2*anilloCuerpo))
}

// columnaRedactor apila las filas pegadas (sin `gap`: los pelos forman una sola
// rejilla), el cuerpo con su `min-h` y los adjuntos con su `pt-3 gap-2`.
type columnaRedactor struct{ r *Redactor }

func (c *columnaRedactor) ancho(t float32) float32 {
	if t > 0 {
		return t
	}
	return c.r.ancho
}

func (c *columnaRedactor) repartir(ancho float32, poner bool) float32 {
	r := c.r
	y := float32(0)
	for _, f := range []*filaCabecera{r.filaDe, r.filaPara, r.filaCc, r.filaCco, r.filaAsunto} {
		if f == nil || (!r.verCc && (f == r.filaCc || f == r.filaCco)) {
			continue
		}
		h := f.AltoFila(ancho)
		if poner {
			f.Move(fyne.NewPos(0, y))
			f.Resize(fyne.NewSize(ancho, h))
		}
		y += h
	}
	y = c.barras(y, ancho, poner)
	y += c.cuerpo(y, ancho, poner)
	if len(r.fichas) > 0 {
		y += aireAdjRedactor
		if poner {
			ui.ColocarTexto(r.textoAdjuntos, tema.TextoXS, margenFila, y)
		}
		y += tema.TextoXS.Linea + huecoAdjRed
		x, linea := margenFila, float32(0)
		for _, f := range r.fichas {
			m := f.MinSize()
			w := min(m.Width, ancho-2*margenFila)
			if x > margenFila && x+w > ancho-margenFila {
				x, y = margenFila, y+linea+huecoAdjRed
			}
			if poner {
				f.Move(fyne.NewPos(x, y))
				f.Resize(fyne.NewSize(w, m.Height))
			}
			x += w + huecoAdjRed
			linea = m.Height
		}
		y += linea
	}
	y = c.panelProgramar(y, ancho, poner)
	if poner {
		c.colocarSugerencias()
	}
	return y
}

// panelProgramar coloca el panel de «Programar envío», si está abierto, desde `y`, y
// devuelve dónde acaba.
func (c *columnaRedactor) panelProgramar(y, ancho float32, poner bool) float32 {
	r := c.r
	if !r.verProgramar || r.grupoProgramar == nil {
		return y
	}
	// `mt-4 border-t pt-4` y la fila `flex flex-wrap items-end gap-3`: el campo se
	// queda con lo que no ocupa el botón, y el botón se apoya abajo.
	y += aireProgramar
	if poner {
		r.fileteProgramar.Move(fyne.NewPos(0, y))
		r.fileteProgramar.Resize(fyne.NewSize(ancho, tema.Borde))
	}
	y += tema.Borde + aireProgramar
	mb := r.BotonProgramar.MinSize()
	mg := r.grupoProgramar.MinSize()
	if l := fyne.CurrentApp().Driver().CanvasForObject(r.grupoProgramar); l != nil && l.Size().Width < ui.Estrecho {
		// En el teléfono, el campo a todo el ancho y el botón debajo, también a todo
		// el ancho (`max-md:basis-full`, `max-md:w-full`): al lado, fecha y hora
		// pedían 242 y tenían 193.
		if poner {
			r.grupoProgramar.Move(fyne.NewPos(0, y))
			r.grupoProgramar.Resize(fyne.NewSize(ancho, mg.Height))
			r.BotonProgramar.Move(fyne.NewPos(0, y+mg.Height+huecoProgramar))
			r.BotonProgramar.Resize(fyne.NewSize(ancho, mb.Height))
		}
		y += mg.Height + huecoProgramar + mb.Height
		return y
	}
	alto := max(mg.Height, mb.Height)
	if poner {
		r.grupoProgramar.Move(fyne.NewPos(0, y+alto-mg.Height))
		r.grupoProgramar.Resize(fyne.NewSize(max(ancho-mb.Width-huecoProgramar, 0), mg.Height))
		r.BotonProgramar.Move(fyne.NewPos(ancho-mb.Width, y+alto-mb.Height))
		r.BotonProgramar.Resize(mb)
	}
	y += alto
	return y
}

// barras coloca la barra de formato y la de tabla con sus pelos, y el panel si está
// abierto, desde `y`; devuelve dónde acaban.
func (c *columnaRedactor) barras(y, ancho float32, poner bool) float32 {
	r := c.r
	// La barra de formato y la de tabla con sus pelos, el fallo del asistente entre las
	// dos (`px-1 py-0.5 text-xs`) y el panel si está abierto.
	barra := func(ver bool, fila filaDeBarra, filete *canvas.Rectangle) {
		if !ver {
			return
		}
		h := fila.alto(ancho)
		if poner {
			fila.caja.Move(fyne.NewPos(0, y))
			fila.caja.Resize(fyne.NewSize(ancho, h))
			filete.Move(fyne.NewPos(0, y+h))
			filete.Resize(fyne.NewSize(ancho, tema.Borde))
		}
		y += h + tema.Borde
	}
	barra(r.VerFormato, r.barra.filaDeBarra, r.fileteBarra)
	if r.iaFallo {
		util := ancho - 2*pxAvisoIA
		h := ui.AltoParrafo(r.avisoIA, util)
		if poner {
			r.avisoIA.Move(fyne.NewPos(pxAvisoIA, y+pyAvisoIA))
			r.avisoIA.Resize(fyne.NewSize(util, h))
		}
		y += h + 2*pyAvisoIA
	}
	barra(r.verTabla, r.barra.tabla, r.fileteTabla)
	if r.panel != nil {
		fila := &filaPanelEnlace{}
		h := fila.colocar(r.panel.caja.Objects, ancho, false)
		if poner {
			r.panel.caja.Move(fyne.NewPos(0, y))
			r.panel.caja.Resize(fyne.NewSize(ancho, h))
		}
		y += h
	}
	return y
}

// cuerpo coloca el cuerpo en `y` y dice cuánto ocupa.
func (c *columnaRedactor) cuerpo(y, ancho float32, poner bool) float32 {
	r := c.r
	// El cuerpo: `px-1 py-3` y `min-h` de 12rem, y crece con lo escrito. El editor trae
	// su propio `px-3 py-2` (el de Notepads), así que se coloca corrido para que el
	// texto caiga donde lo pone la web.
	zona, ok := r.rico.Zona().(fyne.CanvasObject)
	if !ok {
		return y
	}
	anchoZona := ancho - 2*rellenoCuerpoX + 2*notes.PXZonaEd
	if poner || zona.Size().Width != anchoZona {
		zona.Resize(fyne.NewSize(anchoZona, zona.Size().Height))
	}
	altoTexto := zona.MinSize().Height - 2*notes.PYZonaEd
	minimo := float32(minCuerpo)
	if r.ventana > 0 && r.ventana <= 820 {
		minimo = minCuerpo / 2 // `max-[820px]:min-h-[6rem]`
	}
	// `max-h-[min(28rem,calc(var(--sytue-vvh)*0.45))]`: pasado el tope, el cuerpo se
	// desplaza por dentro (alMoverCursor sigue al cursor). Si el mínimo pasa del tope (una
	// ventana muy baja), gana el mínimo, como en CSS.
	tope := topeCuerpo
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(r.contenedor); lienzo != nil && lienzo.Size().Height > 0 {
		tope = min(tope, 0.45*lienzo.Size().Height)
	}
	caja := max(minimo, min(altoTexto+2*rellenoCuerpoY, tope))
	if poner {
		visible := caja - 2*rellenoCuerpoY + 2*notes.PYZonaEd
		r.cuerpo.Move(fyne.NewPos(rellenoCuerpoX-notes.PXZonaEd-anilloCuerpo, y+rellenoCuerpoY-notes.PYZonaEd-anilloCuerpo))
		r.cuerpo.Resize(fyne.NewSize(anchoZona+2*anilloCuerpo, visible+2*anilloCuerpo))
		r.cuerpo.Content.Resize(fyne.NewSize(anchoZona+2*anilloCuerpo, max(zona.MinSize().Height, visible)+2*anilloCuerpo))
		r.cuerpo.Refresh()
	}
	return caja
}

// colocarSugerencias pone el panel del autocompletado bajo su campo. No cuenta en el
// alto: flota sobre lo de debajo.
func (c *columnaRedactor) colocarSugerencias() {
	r := c.r
	if r.sugerencias == nil || r.campoSug == nil {
		return
	}
	for _, f := range []*filaCabecera{r.filaPara, r.filaCc, r.filaCco} {
		if f != nil && f.campo == campoFila(campoDir{r.campoSug}) {
			r.campoSug.ColocarSugerencias(f.Position().Add(r.campoSug.Position()))
		}
	}
}

func (c *columnaRedactor) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, c.repartir(c.ancho(0), false))
}

func (c *columnaRedactor) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.r.ancho = c.ancho(t.Width)
	c.repartir(c.r.ancho, true)
}

// ---- El pie ----

// pieRedactor es `flex w-full justify-between`: las herramientas a la izquierda y
// las salidas a la derecha, a TODO el ancho del cuerpo. El pie de `ui.Dialogo` se
// pega a la derecha con su mínimo, así que el mínimo de este es el ancho entero.
type pieRedactor struct {
	ancho              float32
	izquierda, derecha []fyne.CanvasObject
}

func (p *pieRedactor) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(p.ancho, ui.MedidaBoton("", ui.TallaMD, 1).Height)
}

func (p *pieRedactor) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for _, o := range p.izquierda {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + tema.Espacio(1)
	}
	x = t.Width
	for i := len(p.derecha) - 1; i >= 0; i-- {
		o := p.derecha[i]
		m := o.MinSize()
		x -= m.Width
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x -= huecoSalidas
	}
}

// adjuntosDe copia los adjuntos para que quitar uno en el redactor no toque los del
// mensaje del que salen.
func adjuntosDe(as []eml.Adjunto) []eml.Adjunto { return append([]eml.Adjunto(nil), as...) }
