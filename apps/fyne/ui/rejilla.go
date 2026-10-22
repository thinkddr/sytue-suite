// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los cortes de Tailwind, en px.
//
// # La trampa: miden la VENTANA, no la caja
//
// `sm:` y `xl:` son media queries. Comparar el corte con el ancho DISPONIBLE —que es
// lo natural cuando se porta un layout— pinta siete teselas donde la web pinta seis:
// en producción la galería de Sytue Photos mide 1224 de caja dentro de una ventana de
// 1280, y con 1224 el `xl:` no se cumpliría y la rejilla bajaría a `minmax(11rem)`.
//
// Y los cortes se miden con el rem INICIAL del navegador, 16, no con el 14 de la
// suite: en una media query `rem` es el valor inicial de `font-size`, no el del
// elemento raíz. Por eso `sm` son 640 y no 560.
const (
	CorteSM  float32 = 640
	CorteMD  float32 = 768
	CorteLG  float32 = 1024
	CorteXL  float32 = 1280
	Corte2XL float32 = 1536
)

// Rejilla es un `grid-cols-[repeat(auto-fill,minmax(Min,1fr))] gap-<n>` de la web.
//
// El reparto de columnas es el del navegador y no una división entera: con un ancho
// de 1224, hueco de 3,5 y mínimo de 13rem salen SEIS columnas de 201,078, que es lo
// que mide la galería de Photos en producción.
//
// Ni redondear cada columna a entero (daría 201 y la última se quedaría corta del
// margen) ni redondear cada BORDE (daría 205 de paso y 0,42 px de desvío por columna,
// que es lo que se hizo dos veces antes en este repositorio y aquí NO vale): el
// navegador trunca el ancho a 1/64 y acumula desde ahí. Ver AnchoColumna.
type Rejilla struct {
	// Min es el `minmax(<Min>, 1fr)` en px de CSS.
	Min float32
	// Hueco es el `gap`.
	Hueco float32
	// Fijas es un `grid-cols-N`: N columnas exactas, pase lo que pase con el ancho.
	// Con 0 manda Min y la rejilla es un `auto-fill`.
	//
	// Es el caso MAYORITARIO y no el raro: en esta web hay 88 rejillas de columnas
	// fijas y 9 de `auto-fill`. Portar solo el `auto-fill` deja fuera casi todas las
	// rejillas de tarjetas de la suite.
	Fijas int
	// Cuadrada fuerza el alto de cada tesela al ancho de columna (`aspect-square`). La
	// galería de Sytue Photos ya NO es esto: va en filas justificadas, con su propio
	// layout en su paquete (`photos.Filas`).
	Cuadrada bool
}

var _ fyne.Layout = Rejilla{}

// Columnas es el `auto-fill`: tantas columnas de al menos Min como quepan, y al
// menos una.
func (r Rejilla) Columnas(ancho float32) int {
	if r.Fijas > 0 {
		return r.Fijas
	}
	if r.Min <= 0 {
		return 1
	}
	n := int((ancho + r.Hueco) / (r.Min + r.Hueco))
	return max(n, 1)
}

// LayoutUnit es la rejilla interna del navegador: 1/64 de píxel de CSS. Todo lo que
// Blink coloca cae en un múltiplo de esto, y `getBoundingClientRect` lo enseña
// redondeado a dos decimales.
const LayoutUnit float32 = 1.0 / 64

// AnchoColumna es lo que mide una tesela, cuantizado como lo cuantiza el navegador.
//
// El reparto exacto de la galería da 201,0833; el volcado de producción dice 201,078,
// que es `floor(201,0833 × 64) / 64`. La diferencia importa porque las posiciones se
// ACUMULAN desde el ancho ya cuantizado: la cuarta tesela sale en 613,734 y no en
// 613,75.
//
// Son 1/64 de píxel de CSS, y el error es de CSS, no de dispositivo: una pantalla
// HiDPI NO lo parte por la mitad —eso es lo que este repositorio creyó durante un
// tiempo en cuatro sitios—. Lo que sí hace la densidad es decidir de qué lado del
// píxel cae: a ×2, 613,734 redondea a 1227 y 613,75 a 1228. Una fila de diferencia
// por elegir mal la cuenta.
func (r Rejilla) AnchoColumna(ancho float32) float32 {
	exacto := (ancho+r.Hueco)/float32(r.Columnas(ancho)) - r.Hueco
	return float32(math.Floor(float64(exacto)/float64(LayoutUnit))) * LayoutUnit
}

// Paso es la distancia entre los bordes izquierdos de dos columnas seguidas. Se suma
// desde el ancho YA cuantizado, que es lo que hace el navegador; por eso la última
// columna se queda un par de sesentaicuatroavos corta del margen derecho, también en
// Chrome.
func (r Rejilla) Paso(ancho float32) float32 { return r.AnchoColumna(ancho) + r.Hueco }

// X es el borde izquierdo de la columna c.
func (r Rejilla) X(ancho float32, c int) float32 { return float32(c) * r.Paso(ancho) }

// MinSizeCon es lo que ocupa la rejilla a un ancho dado. Se llama con el ancho porque
// el número de filas depende de él, y `fyne.Layout.MinSize` no lo recibe.
func (r Rejilla) MinSizeCon(objs []fyne.CanvasObject, ancho float32) fyne.Size {
	cols := r.Columnas(ancho)
	alto := float32(0)
	for i := 0; i < len(objs); i += cols {
		if i > 0 {
			alto += r.Hueco
		}
		alto += r.altoFila(objs, i, cols, r.AnchoColumna(ancho))
	}
	return fyne.NewSize(ancho, alto)
}

// MinSize es el de fyne.Layout: sin ancho no hay columnas que contar, así que
// devuelve el de UNA columna. Quien quiera el de verdad llama a MinSizeCon.
func (r Rejilla) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return r.MinSizeCon(objs, r.Min)
}

// altoFila es el alto de la fila que empieza en `desde`: el del hijo más alto, como
// hace una fila implícita de CSS grid.
func (r Rejilla) altoFila(objs []fyne.CanvasObject, desde, cols int, anchoCol float32) float32 {
	if r.Cuadrada {
		return anchoCol
	}
	alto := float32(0)
	for i := desde; i < len(objs) && i < desde+cols; i++ {
		alto = max(alto, AltoA(objs[i], max(anchoCol, 0)))
	}
	return alto
}

// Layout coloca las teselas.
func (r Rejilla) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	cols := r.Columnas(t.Width)
	anchoCol := r.AnchoColumna(t.Width)
	y := float32(0)
	for i := 0; i < len(objs); i += cols {
		alto := r.altoFila(objs, i, cols, anchoCol)
		for c := 0; c < cols && i+c < len(objs); c++ {
			o := objs[i+c]
			o.Move(fyne.NewPos(r.X(t.Width, c), y))
			o.Resize(fyne.NewSize(anchoCol, alto))
		}
		y += alto + r.Hueco
	}
}

// Cortes es un `grid-cols-N sm:grid-cols-M …`, indexado por corte de VENTANA:
// [base, sm, md, lg, xl, 2xl]. Un 0 HEREDA del corte anterior, igual que una
// variante que no se declara en la clase.
type Cortes [6]int

// RejillaTarjetas resuelve esos cortes contra el ancho de VENTANA —no contra el de la
// caja: son media queries— y devuelve la rejilla que toca.
//
// Ejemplo, la vista de tarjetas del explorador (`grid-cols-2 gap-4 sm:grid-cols-3
// lg:grid-cols-4 xl:grid-cols-5`):
//
//	RejillaTarjetas(ventana, tema.Espacio(4), ui.Cortes{2, 3, 0, 4, 5, 0})
func RejillaTarjetas(ventana, hueco float32, cols Cortes) Rejilla {
	limites := [6]float32{0, CorteSM, CorteMD, CorteLG, CorteXL, Corte2XL}
	n := 1
	for i, desde := range limites {
		if ventana < desde && i > 0 {
			break
		}
		if cols[i] > 0 {
			n = cols[i]
		}
	}
	return Rejilla{Fijas: n, Hueco: hueco}
}

// RejillaAlbumes es la de «Álbumes»: `minmax(11rem,1fr) gap-3`, con la portada
// cuadrada y un pie debajo, así que el alto lo pone cada tarjeta.
func RejillaAlbumes() Rejilla {
	return Rejilla{Min: 11 * tema.Rem, Hueco: 3 * 0.25 * tema.Rem}
}

// ---- La tesela de tipo de fichero ----

// tintaExtension es el `KIND_TINT` de FileBrowser.tsx: el token de color de cada
// familia de ficheros. Lo que no está en el mapa va en `text-muted`.
var tintaExtension = map[string]string{
	"pdf": "danger-text",
	"doc": "primary-text", "docx": "primary-text", "rtf": "primary-text", "odt": "primary-text",
	"xls": "success-text", "xlsx": "success-text", "csv": "success-text", "ods": "success-text",
	"ppt": "warning-text", "pptx": "warning-text", "odp": "warning-text", "key": "warning-text",
	"png": "violet-text", "jpg": "violet-text", "jpeg": "violet-text", "gif": "violet-text",
	"webp": "violet-text", "svg": "violet-text", "avif": "violet-text", "heic": "violet-text",
	"mp4": "violet-text", "mov": "violet-text", "webm": "violet-text",
	"mp3": "violet-text", "wav": "violet-text",
	"zip": "warning-text", "rar": "warning-text", "7z": "warning-text",
	"tar": "warning-text", "gz": "warning-text",
}

// TeselaTipo es el `FileIcon` del explorador: un cuadrado con la extensión escrita, o
// una carpeta.
//
// No lleva cáscara de widget: es decoración (`aria-hidden="true"` en la web) y va
// SIEMPRE dentro de una fila o de una tarjeta, que son las que reciben el clic y las
// que se anuncian.
type TeselaTipo struct {
	caja      *Caja
	ext       *canvas.Text
	carpeta   *iconos.Dibujo
	tam       float32
	escalon   tema.Escalon
	extension string
	esCarpeta bool
}

// NuevaTeselaTipo compone la tesela. `tam` es el `size` de la web: 36 en la lista del
// explorador.
func NuevaTeselaTipo(extension string, esCarpeta bool, tam float32) *TeselaTipo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := &TeselaTipo{tam: tam, extension: normalizarExtension(extension), esCarpeta: esCarpeta}
	// `fontSize: Math.max(9, Math.round(size * 0.26))`, y `leading-none`: la caja de
	// línea es el propio tamaño de letra.
	lado := max(9, redondear(tam*0.26))
	t.escalon = tema.Escalon{Tam: lado, Linea: lado}
	t.caja = NuevaCaja(t.relleno(v), color.NRGBA{}, tema.RadioMD)
	t.ext = Texto(t.rotulo(), t.escalon, Negrita, t.tinta(v))
	// La carpeta es el `<Glyph name="folder" size={Math.round(size * 0.5)} />` de
	// FileBrowser.tsx. Hasta el 2026-09-22 eran DOS RECTÁNGULOS «hasta que este módulo
	// tenga juego de iconos»; `iconos.DriveCarpeta` es ese juego, y es la misma ruta
	// que dibuja la web.
	t.carpeta = iconos.Trazo(iconos.DriveCarpeta, t.ladoCarpeta(), "primary-text", v)
	t.carpeta.TenirCon(t.tinta(v))
	return t
}

// normalizarExtension es el `fileExt()` de la web, reducido a lo que la tesela usa:
// minúsculas y sin el punto.
func normalizarExtension(e string) string {
	return strings.ToLower(strings.TrimPrefix(e, "."))
}

// rotulo es `ext.slice(0, 4)` en versalitas.
func (t *TeselaTipo) rotulo() string {
	r := []rune(t.extension)
	if len(r) > 4 {
		r = r[:4]
	}
	return strings.ToUpper(string(r))
}

func (t *TeselaTipo) relleno(v tema.Variante) color.NRGBA {
	if t.esCarpeta {
		return tema.Color("selected", v)
	}
	return tema.Color("bg-subtle", v)
}

func (t *TeselaTipo) tinta(v tema.Variante) color.NRGBA {
	if t.esCarpeta {
		return tema.Color("primary-text", v)
	}
	if tok, ok := tintaExtension[t.extension]; ok {
		return tema.Color(tok, v)
	}
	if t.extension == "" {
		return tema.Color("text-subtle", v)
	}
	return tema.Color("text-muted", v)
}

// ladoCarpeta es el `Math.round(size * 0.5)` del glifo de carpeta.
func (t *TeselaTipo) ladoCarpeta() float32 { return redondear(t.tam * 0.5) }

// Objetos son las primitivas, para el `Objects()` del renderer que la contiene.
func (t *TeselaTipo) Objetos() []fyne.CanvasObject {
	return append(t.caja.Objetos(), t.carpeta.Objeto(), t.ext)
}

// Poner recicla la tesela con otro fichero, que es lo que necesita el `UpdateItem` de
// un `widget.List` para no reconstruir el árbol en cada desplazamiento.
func (t *TeselaTipo) Poner(extension string, esCarpeta bool, v tema.Variante) {
	t.extension, t.esCarpeta = normalizarExtension(extension), esCarpeta
	t.caja.Pintar(t.relleno(v), color.NRGBA{})
	t.ext.Text = t.rotulo()
	t.ext.Color = t.tinta(v)
	t.carpeta.TenirCon(t.tinta(v))
}

// SobreTono repinta la tesela de CARPETA como va dentro de una tarjeta de recurso de
// color pleno (`.sytue-card-tone` de appearance.css): `bg-selected` pasa a un velo del
// 22 % de la tinta del relleno sobre lo que tenga detrás, `text-primary-text` a esa
// tinta y el relleno del glifo a su 30 %. Va después de Poner, que la deja como siempre.
func (t *TeselaTipo) SobreTono(detras, tinta color.NRGBA) {
	caja := tokens.Aplanar(tinta, 0.22, detras)
	t.caja.Pintar(caja, color.NRGBA{})
	t.ext.Color = tinta
	t.carpeta.TenirCon(tinta)
	t.carpeta.Acento(tokens.Aplanar(tinta, 0.30, caja))
}

// Colocar pone la tesela con su esquina en (x, y).
func (t *TeselaTipo) Colocar(x, y float32) {
	MoverCaja(t.caja, fyne.NewPos(x, y), fyne.NewSize(t.tam, t.tam))
	if t.esCarpeta {
		t.ext.Hide()
		g := t.ladoCarpeta()
		t.carpeta.Colocar(x+(t.tam-g)/2, y+(t.tam-g)/2, g)
		t.carpeta.Ver(true)
		return
	}
	t.carpeta.Ver(false)
	t.ext.Show()
	ancho := Medir(t.ext.Text, t.escalon, Negrita).Width
	// `tracking-wide` son 0,025em, que a 9 px es 0,225 px de CSS por letra. Se cuenta
	// en el ANCHO —para centrar donde centra el navegador— pero no se reparte entre
	// los glifos: en «XLSX» eso deja las cuatro letras 0,9 px más juntas que en la web.
	//
	// ponytail: no encoge con la densidad —el error es de CSS— y se cierra pasándole
	// un tracking a `NuevoVersal`, que hoy fija el de las versalitas. Cuatro letras a
	// 9 px dentro de una tesela de 36 no lo justifican todavía.
	ancho += 0.025 * t.escalon.Tam * float32(len([]rune(t.ext.Text)))
	ColocarTexto(t.ext, t.escalon, x+(t.tam-ancho)/2, y+(t.tam-t.escalon.Linea)/2)
}
