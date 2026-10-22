// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import "git.sytue.eu/sytue/one/apps/nucleo/tokens"

// Rem es lo que mide 1rem en la web, en px de CSS. El tema compartido fija la raíz
// en 16px: text-base mide 14px y cada paso de espaciado, 4px. Se mantiene como
// constante porque la geometría de los widgets se declara desde esta base.
// TestLaRaizEsLaDeLaWeb comprueba la regla real de theme.css.
const Rem = 16

// # Los decimales se declaran y NO se redondean. Medido el 2026-09-21
//
// Casi toda la escala de la suite cae en medios píxeles: `0.25rem` con el rem de 14 son
// 3,5, así que `p-3` es 10,5 y `h-9` es 31,5. La tentación es redondearlos aquí, y es un
// error caro: el redondeo lo hace el pintor por coordenada y **depende de la escala del
// lienzo**, así que redondear en el código congela a ×1 un error que la pantalla
// arreglaría sola.
//
// Lo medido con el pintor de software de Fyne v2.8.1, en filas de DISPOSITIVO:
//
//	medida    ×1        ×2            ×3
//	31,5      32 (+0,5) 63 (exacto)   95 (+0,17)
//	10,5      11 (+0,5) 21 (exacto)   32 (+0,17)
//	19,25     20 (+0,75) 39 (+0,25)   58 (+0,08)
//
// A ×2 —cualquier pantalla Retina o HiDPI, y todos los teléfonos— **los medios píxeles
// salen exactos**. A ×1 sobra medio píxel por borde, igual que le pasa al navegador, que
// también tiene que elegir una fila. Los cuartos (19,25, que es el interlineado de
// `text-base`) no son exactos ni a ×2; harían falta ×4.
//
// No hay antialias en los lados rectos: el rectángulo se redondea, no se interpola. Las
// esquinas redondeadas SÍ llevan antialias.
//
// La conclusión para quien escriba un widget: **escribe el número de la web con sus
// decimales**. La cuantización es del dispositivo y se mide a ×2, que es donde los
// guardianes de este módulo comparan.

// Espacio es la escala de espaciado de Tailwind: `p-4` es Espacio(4). Cada paso es
// 0.25rem, o sea 4 px con el rem de la suite. Los pasos fraccionarios se conservan.
func Espacio(n float32) float32 { return n * 0.25 * Rem }

// Los radios salen del mismo spacing.json que la web, sin un espejo a mano.
var (
	RadioSM   = tokens.Radio("sm")
	RadioMD   = tokens.Radio("md")
	RadioLG   = tokens.Radio("lg")
	RadioXL   = tokens.Radio("xl")
	Radio2XL  = tokens.Radio("2xl")
	RadioPill = tokens.Radio("pill")
)

// Layout canónico de theme.css. Van en px en la hoja, así que el rem no los toca.
const (
	AltoBarraSuperior float32 = 56
	AnchoLateral      float32 = 260
	LateralPlegado    float32 = 64
	AnchoCarril       float32 = 64
	AnchoContenido    float32 = 1280
)

// AnilloFoco es el grosor del anillo de foco (WCAG 2.4.13); su color es el token
// "focus-ring".
const AnilloFoco float32 = 2

// Borde es el grosor de `border` en la hoja. Se pinta RELLENO y por dentro de la caja,
// no como trazo: ver `ui.Caja`. Así la medida exterior es la del `box-sizing:
// border-box` de la web, que es lo que la hoja usa en todas partes.
const Borde float32 = 1

// Escalon es un peldaño de la escala tipográfica: el tamaño de letra y la ALTURA DE
// LÍNEA de la hoja, los dos en px de CSS.
//
// Linea no es lo que mide el glifo: es la caja de línea del navegador, que es la que
// ocupa sitio en el layout. Un `canvas.Text` de `text-base` devuelve 14,70 de alto —la
// caja de la FUENTE, 1,2 em—, y usarlo como altura deja el texto pegado a su vecino y las
// filas más bajas que en la web: la caja de CSS son 19,25. El glifo se centra dentro de la
// caja de línea; lo hace `ui.ColocarTexto`.
type Escalon struct{ Tam, Linea float32 }

// La escala de theme.css (`--text-*` y su `--line-height`), convertida desde rem con el
// rem de la suite y SIN redondear.
//
// Estos tamaños llegan ENTEROS al conformador desde el 2026-09-21, y antes no:
// github.com/go-text/typesetting escalaba la fuente a `ceil(tamaño)` antes de pedir los
// avances, así que `text-base` (12,25) salía con los de 13 —un +6,12 % de ancho en cada
// rótulo— y `text-sm` (11,375) con los de 12, +5,49 %.
//
// Y no lo arreglaba una pantalla HiDPI, como se creyó aquí hasta esa fecha: Fyne
// conforma en px de CSS y multiplica por la escala del lienzo DESPUÉS
// (`internal/painter/font.go`), así que el error medido a ×1 y a ×2 es el mismo.
//
// Lo quita el `replace` de `apps/fyne/go.mod` y lo vigila
// `ui.TestElConformadorNoRedondea`. El porqué, en el ADR 0048.
var (
	Texto2XS  = escalon(0.6875, 1)
	TextoXS   = escalon(0.75, 1)
	TextoSM   = escalon(0.8125, 1.125)
	TextoBase = escalon(0.875, 1.375)
	TextoMD   = escalon(1, 1.5)
	TextoLG   = escalon(1.125, 1.625)
	TextoXL   = escalon(1.375, 1.875)
	Texto2XL  = escalon(1.75, 2.25)
	Texto3XL  = escalon(2.25, 2.625)
	Texto4XL  = escalon(3, 3.25)
)

func escalon(rem, linea float32) Escalon {
	return Escalon{Tam: rem * Rem, Linea: linea * Rem}
}

// TextoRaiz es el texto sin clase: hereda los 16px de la raíz, con una caja de línea
// de 1.375rem. TextoBase sigue siendo el peldaño explícito de 0.875rem (14px).
var TextoRaiz = Escalon{Tam: Rem, Linea: 1.375 * Rem}
