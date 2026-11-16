// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los glifos PROPIOS de Sytue Notepads.
//
// No están en `apps/fyne/iconos` porque en la web tampoco están en `packages/ui`: viven
// en `apps/frontend/apps/notes/src/components/glyphs.tsx`, que es de una app. El
// catálogo compartido es el espejo EXACTO de `Icon.tsx` y su guardián compara en los
// dos sentidos, así que meter aquí constantes que allí no existen lo pondría en rojo.
//
// Se portan los que las pantallas USAN, no el fichero entero —`glyphs.tsx` declara
// cuarenta y tantos y la mayoría son de la barra del editor de texto, que esta ola no
// trae—. `TestLosGlifosSonLosDeLaWeb` los compara ruta a ruta contra ese `.tsx`, y
// también avisa si alguno deja de existir allí.
//
// El trazo de `glyphs.tsx` es **1,7** y el del catálogo compartido es 2. `TrazoDeRuta`
// pinta con el del catálogo, así que estos salen un pelo más gruesos que en la web:
// está anotado en el informe con su número, y arreglarlo es un parámetro más en
// `iconos.TrazoDeRuta` que toca a las tres apps que tienen glifos propios.
const (
	// CuadernoD es el `notebook`: el distintivo de un cuaderno privado y el tipo de
	// nota «referencia a otra nota».
	CuadernoD = "M6 4h11a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2ZM8.5 4v16"
	// NotaD es el `note`: la hoja con la esquina doblada. Es el icono de una nota en el
	// árbol y la ilustración del estado «elige una nota».
	NotaD = "M7 4h8.5a1.5 1.5 0 0 1 1.5 1.5v6.5L12.5 17H7a1.5 1.5 0 0 1-1.5-1.5v-10A1.5 1.5 0 0 1 7 4ZM17 12h-3a1.5 1.5 0 0 0-1.5 1.5V17"
	// TareaD es el `todo`: dos casillas con sus renglones.
	TareaD = "M4.5 6.5h3v3h-3zM4.5 14.5h3v3h-3zM11 8h9M11 16h9"
	// OrdenD es el `sort`: dos flechas en sentidos opuestos. En el móvil este botón se
	// queda sin rótulo y el dibujo tiene que decirlo solo.
	OrdenD = "M7.5 4.5v15M7.5 4.5 4 8M7.5 4.5 11 8M16.5 19.5v-15M16.5 19.5 13 16M16.5 19.5 20 16"
	// FiltroD es el `filter`: el embudo de «filtrar por etiqueta».
	FiltroD = "M3.5 5.5h17l-6.5 7.5v6l-4 2v-8Z"
	// FijarD es el `pin`: la chincheta de una nota fijada.
	FijarD = "M9.5 3.5h5l-.8 5.5 3.3 3.5H7l3.3-3.5ZM12 12.5V20.5"
	// ArchivoD es el `archive`: la caja de la vista «Archivo».
	ArchivoD = "M3.5 5h17v4h-17zM5 9v10h14V9M10 13h4"
	// AbiertoD es el `unlock`: el candado abierto de un cuaderno cifrado ya
	// desbloqueado. El CERRADO es `iconos.Candado`, que sí está en el catálogo común,
	// igual que en la web.
	AbiertoD = "M8 10V7a4 4 0 0 1 7.5-2M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8Z"
	// FicheroD es el `file`: el tipo de nota que apunta a un documento o a Drive.
	FicheroD = "M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5"
	// PaletaD es el `palette`: la nota de color.
	PaletaD = "M12 3.5a8.5 8.5 0 0 0 0 17c1.4 0 2-.9 2-1.8 0-.5-.2-.9-.5-1.2-.3-.3-.5-.7-.5-1.2 0-.9.7-1.6 1.6-1.6h1.4A4.5 4.5 0 0 0 20.5 10c0-3.6-3.8-6.5-8.5-6.5ZM7.5 12h.01M9.5 8.5h.01M14 7.5h.01"
	// AsaD es el `drag`: los seis puntos del asa de arrastre de una tarjeta.
	AsaD = "M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01"
	// ArbolD, LibreD, ColumnasD y ListaD son las cuatro disposiciones de un cuaderno,
	// tal como las enseñan sus ajustes.
	ArbolD    = "M4 4.5h6v4H4zM12 11h8v4h-8zM12 17.5h8v4h-8zM7 8.5v11h5M7 13h5"
	LibreD    = "M4 4.5h6v5H4zM13 6.5h7v4h-7zM6 13h7v6.5H6zM16 14.5h4v5h-4z"
	ColumnasD = "M4.5 4.5h5v15h-5zM14.5 4.5h5v15h-5z"
	ListaD    = "M4.5 5.5h15v4h-15zM4.5 14.5h15v4h-15z"
	// ChispasD es el `sparkles`: la chispa de cuatro puntas del asistente.
	ChispasD = "M12 3.5 13.7 8.3 18.5 10 13.7 11.7 12 16.5 10.3 11.7 5.5 10 10.3 8.3ZM18 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8Z"
	// EnvioD es el `send`: el avión de papel.
	EnvioD = "M20.5 3.5 3.5 10l7 3 3 7Z"
)

// Los que Notepads pide por un nombre que también es del catálogo COMPARTIDO. Antes se
// dibujaban aquí con OTRO trazado —la papelera, el enlace, la imagen, deshacer…—, así
// que la misma acción se veía distinta en Notepads que en Mail o en Drive. Ahora la web
// los pinta con `Icon` y aquí salen de `iconos`: una sola ruta por concepto. Se quedan
// las variables para que las pantallas no cambien.
var (
	PapeleraD = iconos.Papelera.Ruta()  // `trash`
	SubirD    = iconos.Subir.Ruta()     // `upload`: importar una copia
	BajarD    = iconos.Descargar.Ruta() // `download`: exportar
	DeshacerD = iconos.Deshacer.Ruta()  // `undo`, el del cuaderno (crear, mover, eliminar)
	RehacerD  = iconos.Rehacer.Ruta()   // `redo`
	EnlaceD   = iconos.Enlace.Ruta()    // `link`: cuaderno compartido y nota «enlace»
	ImagenD   = iconos.Imagen.Ruta()    // `image`
	CitaD     = iconos.Cita.Ruta()      // `quote`
	TablaD    = iconos.Tabla.Ruta()     // `table`
)

// FuenteGlifos es el `.tsx` del que salen, relativo a la raíz del repositorio. Lo usa
// el guardián.
const FuenteGlifos = "apps/frontend/apps/notes/src/components/glyphs.tsx"

// Glifos son los que esta app usa, por su clave en `glyphs.tsx`. Es lo que compara el
// guardián, y es la lista que hay que tocar al portar uno más.
var Glifos = map[string]string{
	"notebook": CuadernoD,
	"note":     NotaD,
	"todo":     TareaD,
	"sort":     OrdenD,
	"filter":   FiltroD,
	"pin":      FijarD,
	"archive":  ArchivoD,
	"unlock":   AbiertoD,
	"file":     FicheroD,
	"palette":  PaletaD,
	"drag":     AsaD,
	// Los del selector de iconos de una nota (IconPickerModal.tsx) y, `sparkles`,
	// también el del asistente de la barra del editor.
	"sparkles": ChispasD,
	"send":     EnvioD,
	"heading":  TituloD,

	"layoutTree":    ArbolD,
	"layoutFree":    LibreD,
	"layoutColumns": ColumnasD,
	"layoutList":    ListaD,
}

// Glifo pinta uno de los de arriba al lado y con el token de color pedidos.
//
// Usa `iconos.TrazoDeRuta`, que es la maquinaria compartida: el rasterizado a píxeles
// de DISPOSITIVO, las correcciones de `oksvg` y el teñido sin rehacer la imagen. Así
// cada app guarda sus rutas donde la web también las tiene y el catálogo compartido
// sigue siendo el espejo exacto de `Icon.tsx`.
// Devuelve ya el `fyne.CanvasObject`: un `*iconos.Dibujo` no lo es —lleva su raster
// dentro— y quien lo mete en un layout quiere el objeto, no el envoltorio. Para los
// pocos sitios que necesitan el `*Dibujo` (el `Glifo` de `ui.Boton`) está `Dibujo`.
func Glifo(d string, lado float32, token string, v tema.Variante) fyne.CanvasObject {
	return Dibujo(d, lado, token, v).Objeto()
}

// Dibujo es Glifo sin envolver, para quien necesita teñirlo o medirlo después.
func Dibujo(d string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	return iconos.TrazoDeRuta(d, lado, tema.Color(token, v))
}

// Icono es `iconos.Trazo` devolviendo el objeto de lienzo, por lo mismo que Glifo: así
// las dos familias se piden igual y no hay que acordarse de cuál lleva `.Objeto()`.
func Icono(i iconos.Icono, lado float32, token string, v tema.Variante) fyne.CanvasObject {
	return iconos.Trazo(i, lado, token, v).Objeto()
}

// GlifoDeTipo es el icono de una nota según su tipo: el mapa `GLIFO` de NoteCard.tsx
// más los dos que el árbol añade (`todo` y `note`). Una nota de texto no lleva.
func GlifoDeTipo(tipo string) string {
	switch tipo {
	case "todo":
		return TareaD
	case "link", "room", "launcher":
		return EnlaceD
	case "image":
		return ImagenD
	case "file", "drive", "doc":
		return FicheroD
	case "event":
		return NotaD
	case "crossref":
		return CuadernoD
	case "color":
		return PaletaD
	}
	return ""
}

// GlifoPorNombre resuelve el icono que una nota guarda en `data.icon`, que es una clave
// de `glyphs.tsx` elegida en el selector de iconos. Los nombres que también son del
// catálogo compartido (`trash`, `link`…) salen de ahí, como en la web, donde `Glyph`
// se los pasa a `Icon`. Vacío si no se ha portado: un icono que no se conoce no se
// pinta, y no es una nota rota.
func GlifoPorNombre(nombre string) string {
	if d, ok := Glifos[nombre]; ok {
		return d
	}
	if i, ok := iconos.Comun(nombre); ok {
		return i.Ruta()
	}
	return ""
}
