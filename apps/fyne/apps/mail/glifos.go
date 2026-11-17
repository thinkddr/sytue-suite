// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los glifos PROPIOS de Sytue Mail, verbatim de
// `apps/frontend/apps/mail/src/components/glyphs.tsx`.
//
// Vivían en `apps/fyne/iconos` detrás de un marcador `propios`, que era la vía de
// antes de que existiera `iconos.TrazoDeRuta`. Ahora la forma única de un glifo de app
// es esta —una constante en el paquete de la app, como hace `contactos/glifos.go`— y
// el catálogo compartido vuelve a ser el espejo EXACTO de `Icon.tsx`, sin rangos ni
// excepciones dentro.
//
// De los de `glyphs.tsx` se portan los que las pantallas usan. Su guardián se ha
// mudado con ellos: `TestLosGlifosSonLosDeLaWeb`, en este mismo paquete.
const (
	// EstrellaD es `star`, el destacado de una fila. Se pinta RELLENA.
	EstrellaD = "M12 3.6L14.4 9.3L20.6 9.8L15.9 13.9L17.3 19.9L12 16.7L6.7 19.9L8.1 13.9L3.4 9.8L9.6 9.3Z"
	// ArchivarD es `archive`.
	ArchivarD = "M3 4h18v5H3zM5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"
	// SinLeerD es `unread`, el sobre cerrado.
	SinLeerD = "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6"
	// LeidoD es `read`, el sobre abierto.
	LeidoD = "M3 10 12 3l9 7v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Zm18 0-9 6-9-6"
	// CarpetaD es `folder`.
	CarpetaD = "M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17"
	// BandejaSalidaD es `outbox`: la bandeja con la flecha que sale, la «Bandeja de
	// salida» de Outlook. Con la carpeta neutra se leía como una carpeta propia más.
	BandejaSalidaD = "M3 13h5l1.5 3h5l1.5-3h5M3 13v6a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-6M12 3v9M8 7l4-4 4 4"
	// SpamD es `spam`.
	SpamD = "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M5.64 5.64l12.72 12.72"
	// ClipD es `paperclip`, el adjunto de una fila.
	ClipD = "M6.75 10v5a5.25 5.25 0 0 0 10.5 0V7a3.5 3.5 0 0 0-7 0v8.5a1.75 1.75 0 0 0 3.5 0V9"
	// ResponderD es `reply`.
	ResponderD = "M9.5 6 4.5 11l5 5M4.5 11H14a5.5 5.5 0 0 1 5.5 5.5V18"
	// ResponderTodosD es `replyAll`.
	ResponderTodosD = "M8 6 3 11l5 5M12.5 6l-5 5 5 5M7.5 11H15a5 5 0 0 1 5 5v2"
	// ReenviarD es `forward`.
	ReenviarD = "M14.5 6l5 5-5 5M19.5 11H10a5.5 5.5 0 0 0-5.5 5.5V18"
	// BajaD es `unsubscribe`.
	BajaD = "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6M4 4l16 16"
	// FiltroD es `filter`, el embudo de «Opciones de búsqueda».
	FiltroD = "M4 5h16l-6 6.5V19l-4-2v-5.5Z"
	// SilenciarD es `mute`, la campana tachada de «Silenciar conversación».
	SilenciarD = "M18 8.5a6 6 0 1 0-12 0c0 5.5-2.2 7-2.2 7h16.4s-2.2-1.5-2.2-7ZM10.3 20a2 2 0 0 0 3.4 0M4 3l16 18"

	// La barra de formato del redactor (`RichTextEditor.tsx`): lo que no está en el común.
	AlinearIzqD    = "M4 6h16M4 10h10M4 14h16M4 18h10"                            // `alignLeft`
	CentrarD       = "M4 6h16M7 10h10M4 14h16M7 18h10"                            // `alignCenter`
	AlinearDerD    = "M4 6h16M10 10h10M4 14h16M10 18h10"                          // `alignRight`
	JustificarD    = "M4 6h16M4 10h16M4 14h16M4 18h16"                            // `alignJustify`
	VinetasD       = "M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"          // `bulletList`
	NumerosD       = "M10 6h10M10 12h10M10 18h10M4 5h1v4M4 9h2M4 14h2v1l-2 2v1h2" // `numberList`
	SangrarD       = "M11 6h9M11 12h9M11 18h9M4 9l3 3-3 3"                        // `indent`
	DesangrarD     = "M11 6h9M11 12h9M11 18h9M7 9l-3 3 3 3"                       // `outdent`
	ReglaD         = "M3 12h18"                                                   // `rule`
	QuitarFormatoD = "M6 5h12M9 5 7 19M12 19h7M15 11l6 6M21 11l-6 6"              // `clearFormat`
	// La barra de tabla, en el orden de `notes.OrdenesTabla`.
	FilaEncimaD  = "M3 11h18v10H3zM3 16h18M9 11v10M15 11v10M12 2v6M9 5l3-3 3 3"        // `rowAbove`
	FilaDebajoD  = "M3 3h18v10H3zM3 8h18M9 3v10M15 3v10M12 22v-6M9 19l3 3 3-3"         // `rowBelow`
	ColAntesD    = "M11 3h10v18H11zM16 3v18M11 8h10M11 14h10M2 12h6M5 9l-3 3 3 3"      // `colBefore`
	ColDespuesD  = "M3 3h10v18H3zM8 3v18M3 8h10M3 14h10M22 12h-6M19 9l3 3-3 3"         // `colAfter`
	BorrarFilaD  = "M3 8h18v8H3zM9 8v8M15 8v8M4 3l4 4M8 3 4 7M16 17l4 4M20 17l-4 4"    // `rowDelete`
	BorrarColD   = "M8 3v18M8 8h8M8 14h8M16 3v18M2 5l4 4M6 5 2 9M18 15l4 4M22 15l-4 4" // `colDelete`
	BorrarTablaD = "M3 5h18v14H3zM3 10h18M9 5v14M6 8l12 8M18 8 6 16"                   // `tableDelete`
)

// Los que Mail pide por un nombre que también es del catálogo COMPARTIDO. Antes se
// dibujaban aquí con OTRO trazado —la papelera, el reloj, la etiqueta, la descarga y
// media barra de formato—, así que la misma acción se veía distinta en Mail que en el
// resto de la suite. Ahora la web los pinta con `Icon` y aquí salen de `iconos`: una
// sola ruta por concepto. Se quedan las variables para que las pantallas no cambien.
var (
	PapeleraD  = iconos.Papelera.Ruta()  // `trash`
	RelojD     = iconos.Reloj.Ruta()     // `clock`: envíos programados y respuesta automática
	EtiquetaD  = iconos.Etiqueta.Ruta()  // `tag`
	DescargaD  = iconos.Descargar.Ruta() // `download`, la flecha de un adjunto
	NegritaD   = iconos.Negrita.Ruta()   // `bold`
	CursivaD   = iconos.Cursiva.Ruta()   // `italic`
	SubrayadoD = iconos.Subrayado.Ruta() // `underline`
	TachadoD   = iconos.Tachado.Ruta()   // `strikethrough`
	CitaD      = iconos.Cita.Ruta()      // `quote`
	EnlaceD    = iconos.Enlace.Ruta()    // `link`
	DeshacerD  = iconos.Deshacer.Ruta()  // `undo`
	RehacerD   = iconos.Rehacer.Ruta()   // `redo`
	TablaD     = iconos.Tabla.Ruta()     // `table`
	ImagenD    = iconos.Imagen.Ruta()    // `image`
)

// glifosPropios es la tabla que el guardián compara contra `glyphs.tsx`, por la CLAVE
// que tienen allí. Es la única lista: las pantallas piden la constante.
var glifosPropios = map[string]string{
	"star": EstrellaD, "archive": ArchivarD,
	"unread": SinLeerD, "read": LeidoD, "folder": CarpetaD, "spam": SpamD, "outbox": BandejaSalidaD,
	"paperclip": ClipD, "reply": ResponderD, "replyAll": ResponderTodosD,
	"forward": ReenviarD, "unsubscribe": BajaD, "filter": FiltroD, "mute": SilenciarD,
	"alignLeft": AlinearIzqD, "alignCenter": CentrarD, "alignRight": AlinearDerD, "alignJustify": JustificarD, "bulletList": VinetasD, "numberList": NumerosD, "indent": SangrarD, "outdent": DesangrarD, "rule": ReglaD, "clearFormat": QuitarFormatoD,
	"rowAbove": FilaEncimaD, "rowBelow": FilaDebajoD, "colBefore": ColAntesD,
	"colAfter": ColDespuesD, "rowDelete": BorrarFilaD, "colDelete": BorrarColD, "tableDelete": BorrarTablaD,
}

// Glifo pinta uno de los de arriba al lado y con el token pedidos.
func Glifo(d string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	return iconos.TrazoDeRuta(d, lado, tema.Color(token, v)).DeToken(token)
}

// GlifoRelleno es la estrella de un destacado: la misma ruta, RELLENA además de
// trazada (`fill={filled ? 'currentColor' : 'none'}`).
func GlifoRelleno(d string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	return iconos.TrazoDeRutaRelleno(d, lado, tema.Color(token, v))
}
