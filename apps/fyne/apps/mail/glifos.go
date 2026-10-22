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
// No es capricho de orden: `trash`, `clock`, `folder`, `star` y `tag` existen en los
// tres `glyphs.tsx` y en `Icon.tsx` con dibujos DISTINTOS, así que el prefijo `Mail`
// estaba ahí solo para que no chocaran. Dentro de este paquete el prefijo sobra.
//
// De los 44 de `glyphs.tsx` se portan los que las pantallas usan. Su guardián se ha
// mudado con ellos: `TestLosGlifosSonLosDeLaWeb`, en este mismo paquete.
const (
	// EstrellaD es `star`, el destacado de una fila. Se pinta RELLENA.
	EstrellaD = "m12 2.8 2.9 5.9 6.5 1-4.7 4.6 1.1 6.4L12 17.7l-5.8 3 1.1-6.4-4.7-4.6 6.5-1L12 2.8Z"
	// ArchivarD es `archive`.
	ArchivarD = "M3 4h18v5H3zM5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"
	// PapeleraD es `trash`, que NO es el del catálogo compartido.
	PapeleraD = "M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"
	// SinLeerD es `unread`, el sobre cerrado.
	SinLeerD = "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6"
	// LeidoD es `read`, el sobre abierto.
	LeidoD = "M3 10 12 3l9 7v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Zm18 0-9 6-9-6"
	// CarpetaD es `folder`.
	CarpetaD = "M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2Z"
	// SpamD es `spam`.
	SpamD = "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M5.64 5.64l12.72 12.72"
	// ClipD es `paperclip`, el adjunto de una fila.
	ClipD = "m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"
	// ResponderD es `reply`.
	ResponderD = "M9 17 4 12l5-5M4 12h9a5 5 0 0 1 5 5v2"
	// ResponderTodosD es `replyAll`.
	ResponderTodosD = "M7 17 2 12l5-5M12 17 7 12l5-5M7 12h8a5 5 0 0 1 5 5v2"
	// ReenviarD es `forward`.
	ReenviarD = "m15 17 5-5-5-5M20 12h-9a5 5 0 0 0-5 5v2"
	// RelojD es `clock`: envíos programados y respuesta automática.
	RelojD = "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 8v4l3 2"
	// EtiquetaD es `tag`, que tampoco es el del catálogo compartido.
	EtiquetaD = "M20.59 13.41 13.42 20.58a2 2 0 0 1-2.83 0L3 13V4a1 1 0 0 1 1-1h9l7.59 7.59a2 2 0 0 1 0 2.82ZM7.5 7.5h.01"
	// BajaD es `unsubscribe`.
	BajaD = "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6M4 4l16 16"

	// La barra de formato del redactor (`RichTextEditor.tsx`).
	NegritaD       = "M7 5h6a3.5 3.5 0 0 1 0 7H7zM7 12h7a3.5 3.5 0 0 1 0 7H7z"                       // `bold`
	CursivaD       = "M19 5h-7M12 19H5M15 5 9 19"                                                    // `italic`
	SubrayadoD     = "M7 4v7a5 5 0 0 0 10 0V4M5 21h14"                                               // `underline`
	TachadoD       = "M5 12h14M7 7a4 3 0 0 1 4-3h2a4 3 0 0 1 4 3M7 16a4 3 0 0 0 4 3h2a4 3 0 0 0 4-3" // `strikethrough`
	AlinearIzqD    = "M4 6h16M4 10h10M4 14h16M4 18h10"                                               // `alignLeft`
	CentrarD       = "M4 6h16M7 10h10M4 14h16M7 18h10"                                               // `alignCenter`
	AlinearDerD    = "M4 6h16M10 10h10M4 14h16M10 18h10"                                             // `alignRight`
	JustificarD    = "M4 6h16M4 10h16M4 14h16M4 18h16"                                               // `alignJustify`
	VinetasD       = "M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"                             // `bulletList`
	NumerosD       = "M10 6h10M10 12h10M10 18h10M4 5h1v4M4 9h2M4 14h2v1l-2 2v1h2"                    // `numberList`
	SangrarD       = "M11 6h9M11 12h9M11 18h9M4 9l3 3-3 3"                                           // `indent`
	DesangrarD     = "M11 6h9M11 12h9M11 18h9M7 9l-3 3 3 3"                                          // `outdent`
	CitaD          = "M7 7H4v5h5V8a4 4 0 0 0-2-1M18 7h-3v5h5V8a4 4 0 0 0-2-1"                        // `quote`
	ReglaD         = "M3 12h18"                                                                      // `rule`
	EnlaceD        = "M9 15l6-6M10 6l1-1a4 4 0 0 1 6 6l-1 1M14 18l-1 1a4 4 0 0 1-6-6l1-1"            // `link`
	QuitarFormatoD = "M6 5h12M9 5 7 19M12 19h7M15 11l6 6M21 11l-6 6"                                 // `clearFormat`
	DeshacerD      = "M9 14 4 9l5-5M4 9h9a7 7 0 0 1 0 14h-3"                                         // `undo`
	RehacerD       = "m15 14 5-5-5-5M20 9h-9a7 7 0 0 0 0 14h3"                                       // `redo`
	TablaD         = "M3 5h18v14H3zM3 10h18M3 15h18M9 5v14M15 5v14"                                  // `table`
	ImagenD        = "M3 5h18v14H3zM3 16l5-5 4 4 3-3 6 6"                                            // `image`
	// La barra de tabla, en el orden de `notes.OrdenesTabla`.
	FilaEncimaD  = "M3 11h18v10H3zM3 16h18M9 11v10M15 11v10M12 2v6M9 5l3-3 3 3"        // `rowAbove`
	FilaDebajoD  = "M3 3h18v10H3zM3 8h18M9 3v10M15 3v10M12 22v-6M9 19l3 3 3-3"         // `rowBelow`
	ColAntesD    = "M11 3h10v18H11zM16 3v18M11 8h10M11 14h10M2 12h6M5 9l-3 3 3 3"      // `colBefore`
	ColDespuesD  = "M3 3h10v18H3zM8 3v18M3 8h10M3 14h10M22 12h-6M19 9l3 3-3 3"         // `colAfter`
	BorrarFilaD  = "M3 8h18v8H3zM9 8v8M15 8v8M4 3l4 4M8 3 4 7M16 17l4 4M20 17l-4 4"    // `rowDelete`
	BorrarColD   = "M8 3v18M8 8h8M8 14h8M16 3v18M2 5l4 4M6 5 2 9M18 15l4 4M22 15l-4 4" // `colDelete`
	BorrarTablaD = "M3 5h18v14H3zM3 10h18M9 5v14M6 8l12 8M18 8 6 16"                   // `tableDelete`
)

// glifosPropios es la tabla que el guardián compara contra `glyphs.tsx`, por la CLAVE
// que tienen allí. Es la única lista: las pantallas piden la constante.
var glifosPropios = map[string]string{
	"star": EstrellaD, "archive": ArchivarD, "trash": PapeleraD,
	"unread": SinLeerD, "read": LeidoD, "folder": CarpetaD, "spam": SpamD,
	"paperclip": ClipD, "reply": ResponderD, "replyAll": ResponderTodosD,
	"forward": ReenviarD, "clock": RelojD, "tag": EtiquetaD, "unsubscribe": BajaD,
	"download": DescargaD,
	"bold":     NegritaD, "italic": CursivaD, "underline": SubrayadoD, "strikethrough": TachadoD, "alignLeft": AlinearIzqD, "alignCenter": CentrarD, "alignRight": AlinearDerD, "alignJustify": JustificarD, "bulletList": VinetasD, "numberList": NumerosD, "indent": SangrarD, "outdent": DesangrarD, "quote": CitaD, "rule": ReglaD, "link": EnlaceD, "clearFormat": QuitarFormatoD, "undo": DeshacerD, "redo": RehacerD,
	"table": TablaD, "image": ImagenD, "rowAbove": FilaEncimaD, "rowBelow": FilaDebajoD, "colBefore": ColAntesD,
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
