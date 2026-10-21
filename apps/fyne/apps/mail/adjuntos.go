// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"path"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Los adjuntos y el pie de un mensaje (`ThreadMessage` de MessageReader.tsx)
//
// Los adjuntos salen del mismo .eml que el cuerpo: el servicio guarda el mensaje crudo
// y nada más, así que no hay otra lista que pedir.
//
// «Descargar» deja el fichero en la carpeta de descargas del sistema, sin pisar nada
// (`ficheros.Guardar`), y lo enseña en el gestor de ficheros, que es lo que hace el
// navegador con su barra de descargas.
//
// «Guardar en Drive» es un acto EXPLÍCITO y AVISADO, a propósito: en Drive «Mi unidad»
// es hoy de toda la organización, así que un adjunto que solo veía el buzón pasa a
// verlo el tenant entero (`api.Drive.GuardarAdjunto`, que lo explica medido). No pasa
// nunca solo, y al terminar se dice DÓNDE ha quedado y con qué nombre.

// Guardador es lo que Sytue Mail le pide a Drive. Lo cumple *api.Drive.
type Guardador interface {
	GuardarAdjunto(ctx context.Context, nombre, tipo string, contenido []byte) (api.Referencia, error)
}

var _ Guardador = (*api.Drive)(nil)

var (
	tipoFirma   = regexp.MustCompile(`^application/(pgp-signature|(x-)?pkcs7-signature)$`)
	nombreFirma = regexp.MustCompile(`(?i)^(signature\.asc|smime\.p7s)$`)
)

// EsFirmaDeSobre es `esFirmaDeSobre`: la firma criptográfica del mensaje (PGP o
// S/MIME) viaja como una parte más, pero no es un fichero que nadie decidiera mandar
// ni que se pueda abrir con nada. Se mira el TIPO y, como red, el nombre canónico.
func EsFirmaDeSobre(a eml.Adjunto) bool {
	tipo := strings.ToLower(strings.TrimSpace(strings.SplitN(a.Tipo, ";", 2)[0]))
	return tipoFirma.MatchString(tipo) || nombreFirma.MatchString(a.Nombre)
}

// AdjuntoVisible es un adjunto que se ENSEÑA, con su posición en el mensaje: la
// descarga va por posición, y renumerar tras quitar las firmas bajaría otro.
type AdjuntoVisible struct {
	Indice int
	eml.Adjunto
}

// Visibles son los adjuntos menos las firmas de sobre.
func Visibles(as []eml.Adjunto) []AdjuntoVisible {
	var out []AdjuntoVisible
	for i, a := range as {
		if !EsFirmaDeSobre(a) {
			out = append(out, AdjuntoVisible{Indice: i, Adjunto: a})
		}
	}
	return out
}

// nombreAdjunto es el nombre con el que se guarda: el suyo o, sin nombre, «adjunto».
func nombreAdjunto(a eml.Adjunto) string {
	if n := strings.TrimSpace(a.Nombre); n != "" {
		return n
	}
	return "adjunto"
}

// NombreOriginal es `nombreArchivo` del BFF: el asunto sin lo que un sistema de
// ficheros no admite, con el espacio aplanado, recortado a 80 y con `.eml`. Sin asunto,
// el id: que no acabe en Descargas un «.eml» sin nombre.
func NombreOriginal(asunto, id string) string {
	limpio := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`"\/:*?<>|`, r) {
			return ' '
		}
		return r
	}, asunto)
	limpio = strings.Join(strings.Fields(limpio), " ")
	if r := []rune(limpio); len(r) > 80 {
		limpio = strings.TrimSpace(string(r[:80]))
	}
	if limpio == "" {
		limpio = id
	}
	return limpio + ".eml"
}

// ---- En el Buzón ----

// Guardar deja unos bytes en la carpeta de descargas y la abre. Es una variable para
// que los guardianes simulen el disco lleno sin llenar ninguno.
var Guardar = func(nombre string, r io.Reader) error {
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return err
	}
	ruta, err := ficheros.GuardarDeInternet(carpeta, nombre, r) // marcado como de Internet (FYS-02)
	if err != nil {
		return err
	}
	_ = ficheros.Revelar(ruta) // no poder enseñarla no es no haberla guardado
	return nil
}

// descargar guarda y, si el disco falla, lo dice. En la web la descarga la lleva el
// navegador y ese estado no existe; aquí un fallo callado es un fichero que la persona
// cree que tiene.
func (b *Buzon) descargar(nombre string, r io.Reader) {
	if err := Guardar(nombre, r); err != nil {
		fyne.Do(func() {
			t := b.P.T
			b.P.Avisar(ui.AvisoPeligro, t.T("reader.downloadFailed", nil),
				t.T("reader.downloadFailedDescription", i18n.Vars{"name": nombre}))
		})
	}
}

// descargarAdjunto es el enlace de un adjunto.
func (b *Buzon) descargarAdjunto(_ Mensaje, a AdjuntoVisible) {
	b.Lanzar(func() { b.descargar(nombreAdjunto(a.Adjunto), bytes.NewReader(a.Datos)) })
}

// descargarOriginal es «Original (.eml)»: el mensaje ENTERO tal como llegó.
func (b *Buzon) descargarOriginal(m Mensaje) {
	nombre := NombreOriginal(m.Asunto, m.ID)
	b.Lanzar(func() {
		r, err := b.S.Raw(b.ctx, m.ID)
		if err != nil {
			// Sin el «revisa el espacio»: aquí el que falla es el servidor, no el disco.
			fyne.Do(func() { b.avisar(ui.AvisoPeligro, "reader.downloadFailed", nil) })
			return
		}
		defer r.Close()
		b.descargar(nombre, r)
	})
}

// guardarEnDrive es `guardarEnDrive`: una vez por pulsación —el botón se apaga
// mientras trabaja, o se guardaría dos veces— y el aviso dice el nombre GUARDADO, que
// no siempre es el del adjunto («informe (2).pdf»).
func (b *Buzon) guardarEnDrive(m Mensaje, a AdjuntoVisible) {
	l := b.P.Lector
	clave := claveAdjunto(m.ID, a.Indice)
	if b.Drive == nil || l.Guardando[clave] {
		return
	}
	if l.Guardando == nil {
		l.Guardando = map[string]bool{}
	}
	l.Guardando[clave] = true
	l.Refresh()
	nombre := nombreAdjunto(a.Adjunto)
	pedir(b, func(ctx context.Context) (api.Referencia, error) {
		return b.Drive.GuardarAdjunto(ctx, nombre, a.Tipo, a.Datos)
	}, func(ref api.Referencia, err error) {
		delete(l.Guardando, clave)
		l.Refresh()
		t := b.P.T
		if err != nil {
			b.P.Avisar(ui.AvisoPeligro, t.T("reader.saveToDriveFailed", nil),
				t.T("reader.saveToDriveFailedDescription", i18n.Vars{"name": nombre}))
			return
		}
		b.P.Avisar(ui.AvisoExito, t.T("reader.savedToDrive", nil),
			t.T("reader.savedToDriveIn", i18n.Vars{"name": path.Base(ref.Ruta)}))
	})
}

func claveAdjunto(id string, i int) string { return id + "#" + numero(i+1) }

// ---- Las piezas ----

// Medidas de la hoja del bloque de adjuntos y del pie:
//
//	bloque   `mt-6 border-t pt-4`, el rótulo en `sytue-overline mb-2`, la lista
//	         `flex-col gap-2` y cada fila `flex items-center gap-2`
//	enlace   `border rounded-md bg-bg-subtle px-3 py-2 text-sm gap-3`: nombre, tamaño
//	         en `text-subtle` y la flecha de 16
//	pie      `mt-4 flex flex-wrap gap-2`: Responder y Reenviar fantasma `sm`, y el
//	         enlace del original `px-3 py-1.5 text-sm gap-2 text-muted`
const (
	aireAdjuntos    float32 = 6 * 0.25 * tema.Rem
	rellenoAdjuntos float32 = 4 * 0.25 * tema.Rem
	bajoRotuloAdj   float32 = 2 * 0.25 * tema.Rem
	huecoAdjuntos   float32 = 2 * 0.25 * tema.Rem
	margenXEnlace   float32 = 3 * 0.25 * tema.Rem
	margenYEnlace   float32 = 2 * 0.25 * tema.Rem
	huecoEnlace     float32 = 3 * 0.25 * tema.Rem
	aireAcciones    float32 = 4 * 0.25 * tema.Rem
	margenYOriginal float32 = 1.5 * 0.25 * tema.Rem
	huecoOriginal   float32 = 2 * 0.25 * tema.Rem
	tallaFlecha     float32 = 16
)

// AltoEnlaceAdjunto es el enlace de un adjunto: `py-2` y la línea de `text-sm`, con
// su borde.
var AltoEnlaceAdjunto = 2*tema.Borde + 2*margenYEnlace + tema.TextoSM.Linea

// AltoOriginal es el enlace «Original (.eml)»: `py-1.5` y la línea de `text-sm`.
var AltoOriginal = 2*margenYOriginal + tema.TextoSM.Linea

// DescargaD es `download` de glyphs.tsx: no es la flecha del catálogo compartido.
const DescargaD = "M12 3v12M7 10l5 5 5-5M5 21h14"

// enlaceAdjunto es el `<a download>` de un adjunto: la píldora con el nombre, el
// tamaño y la flecha. Encima, `hover:border-border-strong hover:bg-bg`.
type enlaceAdjunto struct {
	ui.Pulsable
	nombre, tamano string
	ancho          float32 // el que le deja la fila; el nombre se trunca con el resto
}

func nuevoEnlaceAdjunto(nombre, tamano, rotulo string, alPulsar func()) *enlaceAdjunto {
	e := &enlaceAdjunto{nombre: nombre, tamano: tamano}
	e.Nombre, e.AlPulsar = rotulo, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// anchoNatural es el enlace sin truncar.
func (e *enlaceAdjunto) anchoNatural() float32 {
	return 2*tema.Borde + 2*margenXEnlace + ui.Medir(e.nombre, tema.TextoSM, ui.Normal).Width + huecoEnlace +
		ui.Medir(e.tamano, tema.TextoSM, ui.Normal).Width + huecoEnlace + tallaFlecha
}

func (e *enlaceAdjunto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &enlaceAdjuntoRenderer{
		e:      e,
		aro:    ui.NuevoAroCon("focus-ring", tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD),
		nombre: ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text", v)),
		tamano: ui.Texto(e.tamano, tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)),
		flecha: Glifo(DescargaD, tallaFlecha, "text-subtle", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.nombre, r.tamano, r.flecha.Objeto())
	r.Refresh()
	return r
}

type enlaceAdjuntoRenderer struct {
	e              *enlaceAdjunto
	aro            *ui.Aro
	caja           *ui.Caja
	nombre, tamano *canvas.Text
	flecha         *iconos.Dibujo
	objetos        []fyne.CanvasObject
}

func (r *enlaceAdjuntoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(min(r.e.anchoNatural(), max(r.e.ancho, 0)), AltoEnlaceAdjunto)
}
func (r *enlaceAdjuntoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *enlaceAdjuntoRenderer) Destroy()                     {}

func (r *enlaceAdjuntoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	y := (t.Height - tema.TextoSM.Linea) / 2
	fin := t.Width - tema.Borde - margenXEnlace
	r.flecha.Colocar(fin-tallaFlecha, (t.Height-tallaFlecha)/2, tallaFlecha)
	fin -= tallaFlecha + huecoEnlace
	wt := ui.Medir(r.e.tamano, tema.TextoSM, ui.Normal).Width
	ui.ColocarTexto(r.tamano, tema.TextoSM, fin-wt, y)
	fin -= wt + huecoEnlace
	x := tema.Borde + margenXEnlace
	ui.Truncar(r.nombre, r.e.nombre, tema.TextoSM, ui.Normal, max(fin-x, 0))
	ui.ColocarTexto(r.nombre, tema.TextoSM, x, y)
}

func (r *enlaceAdjuntoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := tema.Color("bg-subtle", v), tema.Color("border", v)
	if r.e.Encima() {
		fondo, borde = tema.Color("bg", v), tema.Color("border-strong", v)
	}
	r.caja.Pintar(fondo, borde)
	r.aro.Pintar(v)
	r.aro.Ver(r.e.Enfocado())
	r.nombre.Color = tema.Color("text", v)
	r.tamano.Text, r.tamano.Color = r.e.tamano, tema.Color("text-subtle", v)
	r.flecha.Tenir("text-subtle", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
	if t := r.e.Size(); t.Width > 0 {
		r.Layout(t)
	}
}

// enlaceOriginal es «Original (.eml)»: un `<a download>` con pinta de botón fantasma
// pero en `text-muted`, sin negrita y con `py-1.5` (26,25 de alto y no 28).
type enlaceOriginal struct {
	ui.Pulsable
	texto string
}

func nuevoEnlaceOriginal(texto, pista string, alPulsar func()) *enlaceOriginal {
	e := &enlaceOriginal{texto: texto}
	e.Nombre, e.AlPulsar = texto+" — "+pista, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// MedidaEnlaceOriginal es `px-3`, la flecha, el `gap-2` y el rótulo.
func MedidaEnlaceOriginal(texto string) fyne.Size {
	return fyne.NewSize(2*margenXEnlace+tallaFlecha+huecoOriginal+ui.Medir(texto, tema.TextoSM, ui.Normal).Width, AltoOriginal)
}

func (e *enlaceOriginal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &enlaceOriginalRenderer{
		e:      e,
		aro:    ui.NuevoAroCon("focus-ring", tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioMD),
		texto:  ui.Texto(e.texto, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		flecha: Glifo(DescargaD, tallaFlecha, "text-muted", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.flecha.Objeto(), r.texto)
	r.Refresh()
	return r
}

type enlaceOriginalRenderer struct {
	e       *enlaceOriginal
	aro     *ui.Aro
	caja    *ui.Caja
	texto   *canvas.Text
	flecha  *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *enlaceOriginalRenderer) MinSize() fyne.Size           { return MedidaEnlaceOriginal(r.e.texto) }
func (r *enlaceOriginalRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *enlaceOriginalRenderer) Destroy()                     {}
func (r *enlaceOriginalRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.flecha.Colocar(margenXEnlace, (t.Height-tallaFlecha)/2, tallaFlecha)
	ui.ColocarTexto(r.texto, tema.TextoSM, margenXEnlace+tallaFlecha+huecoOriginal, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *enlaceOriginalRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := "text-muted"
	if r.e.Encima() {
		tinta = "text"
		r.caja.Pintar(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v))
	} else {
		r.caja.Pintar(color.NRGBA{}, color.NRGBA{})
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.e.Enfocado())
	r.texto.Color = tema.Color(tinta, v)
	r.flecha.Tenir(tinta, v)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// lineasFlujo cuenta las líneas en que `flex-wrap` parte unos anchos con su hueco.
func lineasFlujo(anchos []float32, hueco, ancho float32) int {
	lineas, x := 1, float32(0)
	for i, w := range anchos {
		if i > 0 && x+w > ancho {
			lineas, x = lineas+1, 0
		}
		x += w + hueco
	}
	return lineas
}
