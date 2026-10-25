// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package tema es «Tinta y papel» para Fyne: los mismos tokens que pinta la web,
// leídos de la MISMA hoja que ella (`apps/nucleo/tokens`, que interpreta
// `branding/tokens/colors.css`).
//
// # Lo que este paquete NO es
//
// No es el tema con el que se configuran los widgets de serie de Fyne. `Sytue`
// implementa `fyne.Theme` porque hay dos cosas que Fyne resuelve por su cuenta y que
// hay que gobernar —la LETRA con la que `canvas.Text` rasteriza, y el fondo del
// lienzo—, y por nada más. Los widgets de la suite **no** preguntan colores ni medidas
// por `fyne.Theme`: preguntan `tema.Color("border-strong", v)` y las constantes de
// medidas.go.
//
// El motivo es que el mapa de Fyne tiene unos cuarenta nombres y la hoja tiene ciento
// cincuenta y pico. Pasar los nuestros por los suyos es perder ciento diez tokens por
// el camino y, peor, dejar que el ajuste de tamaño de letra del sistema mueva la
// GEOMETRÍA: `theme.Size()` multiplica por la escala del usuario, y con eso la paridad
// a píxel se acaba antes de empezar.
package tema

import (
	"image/color"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Variante es el tema: claro, oscuro o translúcido. Es el del núcleo, para no tener dos.
type Variante = tokens.Variant

// Los tres temas. Claro y Oscuro conservan sus valores cero y uno.
const (
	Claro       = tokens.Light
	Oscuro      = tokens.Dark
	Translucido = tokens.Translucent
)

// Color devuelve el token `--sytue-<nombre>` en el tema pedido: "bg", "primary-500",
// "border-strong", "product-mail", "orgmark-7"…
//
// Un nombre que no existe es un error de quien escribe el widget, no un estado de la
// aplicación, y entra en pánico como regexp.MustCompile.
func Color(nombre string, v Variante) color.NRGBA { return tokens.Color(nombre, v) }

// Las seis caras de Figtree, como recursos de Fyne. Se construyen una vez: cada
// `fyne.NewStaticResource` que se repita es una cara más que el cacheador de fuentes
// trata como distinta.
var (
	Regular   = fyne.NewStaticResource("Figtree-Regular.ttf", fuentes.Leer("Figtree-Regular.ttf"))
	Media     = fyne.NewStaticResource("Figtree-Medium.ttf", fuentes.Leer("Figtree-Medium.ttf"))
	SemiNegra = fyne.NewStaticResource("Figtree-SemiBold.ttf", fuentes.Leer("Figtree-SemiBold.ttf"))
	Negra     = fyne.NewStaticResource("Figtree-Bold.ttf", fuentes.Leer("Figtree-Bold.ttf"))
	// ExtraNegra es el 800 de los títulos de la hoja en la forma Marco.
	ExtraNegra = fyne.NewStaticResource(fuentes.ExtraNegra, fuentes.Leer(fuentes.ExtraNegra))
	Cursiva    = fyne.NewStaticResource("Figtree-Italic.ttf", fuentes.Leer("Figtree-Italic.ttf"))
	// NegraCursiva es la negrita cursiva DE VERDAD: sin ella, `<b><i>` salía en cursiva
	// sin negrita, porque Fyne no engorda una cara como hace el navegador.
	NegraCursiva = fyne.NewStaticResource("Figtree-BoldItalic.ttf", fuentes.Leer("Figtree-BoldItalic.ttf"))
	// Mono es `font-mono`: JetBrains Mono, la primera de la pila de la web.
	Mono = fyne.NewStaticResource(fuentes.Mono, fuentes.Leer(fuentes.Mono))
	// Matematica es la reserva de símbolos, con una versión común a todos los SO.
	Matematica = fyne.NewStaticResource(fuentes.Matematica, fuentes.Leer(fuentes.Matematica))
)

// Sytue es el `fyne.Theme` de la suite. El cero vale: la variante la decide Fyne con
// `fyne.ThemeVariant`, que es lo que sigue a `prefers-color-scheme`.
type Sytue struct{}

var _ fyne.Theme = Sytue{}

// De devuelve la variante de la suite que corresponde a la de Fyne, SALVO que la
// persona haya elegido una en el menú de cuenta: esa manda sobre la del sistema, igual
// que en la web el selector de tema manda sobre `prefers-color-scheme`.
//
// La elección se comprueba aquí, en el único sitio por el que ya pasaban los 243 puntos
// del árbol que preguntan la variante, en vez de cambiar los 243. Y hay que comprobarla
// aquí y no en `Sytue.Color`, porque Fyne NO deja forzarla: `settings.ThemeVariant()`
// devuelve `s.variant`, que sale del sistema operativo, y `SetTheme` no la toca
// (`fyne.io/fyne/v2@v2.8.1/app/settings.go:75-86`). Los widgets de la suite tampoco
// piden color por `fyne.Theme`, así que un `theme.DarkTheme()` no los pintaría.
func De(v fyne.ThemeVariant) Variante {
	switch elegida.Load() {
	case int32(Claro) + 1:
		return Claro
	case int32(Oscuro) + 1:
		return Oscuro
	case int32(Translucido) + 1:
		return Translucido
	}
	if v == temafyne.VariantDark {
		return Oscuro
	}
	return Claro
}

// elegida es la variante del menú de cuenta: 0 sin elegir, y si no, la variante más uno.
// Atómica porque la escribe el vigilante de los ajustes compartidos (otra app de la
// suite puede cambiar el tema) y la leen los renderers.
var elegida atomic.Int32

// Elegir fija la variante de la suite. "claro", "oscuro" y "traslucido" la fijan;
// cualquier otra cosa —"" incluida— devuelve el mando al sistema. Devuelve si ha
// cambiado algo, que es lo que le dice a quien llama si hace falta repintar.
func Elegir(nombre string) bool {
	nueva := int32(0)
	switch nombre {
	case "claro":
		nueva = int32(Claro) + 1
	case "oscuro":
		nueva = int32(Oscuro) + 1
	case "traslucido":
		nueva = int32(Translucido) + 1
	}
	return elegida.Swap(nueva) != nueva
}

// ElegirColor fija el COLOR de la suite, que es un eje aparte del tema (ADR 0071): uno
// de los que declara la hoja ("cobalto") o cualquier otra cosa —"" y "petroleo"
// incluidos— para el de la casa. Devuelve si ha cambiado algo, como Elegir. Los widgets
// no se enteran: siguen pidiendo `tema.Color("primary", v)` y sale el del color en vigor.
func ElegirColor(nombre string) bool { return tokens.ElegirColor(nombre) }

// ElegirForma fija la FORMA de la suite, el tercer eje: "carril", o cualquier otra cosa
// —"" y "marco" incluidos— para Marco, que es la de la casa y la que se ve por defecto.
// Devuelve si ha cambiado algo, como Elegir.
func ElegirForma(nombre string) bool {
	if nombre == "" {
		nombre = formaSinElegir
	}
	return tokens.ElegirForma(nombre)
}

// formaSinElegir es la forma de quien no ha elegido ninguna: vacía, la de la casa. Solo
// la cambian los guardianes (ver FormaDeLosGuardianes).
var formaSinElegir string

// EsMarco dice si la forma en vigor es Marco: la barra y el menú en un marco de color
// continuo y el contenido en una hoja redondeada que flota. Si no, es Carril.
func EsMarco() bool { return !tokens.EsCarril() }

// Forma devuelve en píxeles el token de forma `nombre` —"radius-control", "sheet-gap",
// "nav-pad"…— de la forma en vigor. Son las medidas que la web declara en el bloque
// «Tokens de forma» de theme.css, leídas de esa hoja (`apps/nucleo/tokens`): aquí no hay
// ni un número. Un `rem` de la hoja es el Rem de la suite; un peso o una escala, que no
// llevan unidad, salen tal cual.
func Forma(nombre string) float32 {
	m := tokens.MedidaDeForma(nombre)
	if m.Unidad == "rem" {
		return float32(m.Valor) * Rem
	}
	return float32(m.Valor)
}

// ColorElegido es el color de la suite en vigor ("cobalto", "app"), o "" si es el de la
// casa.
func ColorElegido() string { return tokens.ColorElegido() }

// FormaDeLosGuardianes pone la forma con la que corren los tests del driver: CARRIL,
// salvo que `SYTUE_FORMA` diga otra. Los cientos de medidas y de píxeles de esos tests
// se tomaron con Carril —la única forma hasta el 2026-10-05— y siguen vigilándola; lo
// propio de Marco lleva sus tests, que la eligen ellos con `ElegirForma("marco")`. Lo
// llama el `TestMain` de cada paquete. Con `SYTUE_FORMA=marco` las capturas de los tests
// salen en Marco, para mirarlas.
//
// Deja esa forma también como la de quien NO ha elegido: un test que abre una sesión
// aplica los ajustes de su carpeta vacía, y sin esto devolvería la forma de la casa a
// todos los que corrieran después.
func FormaDeLosGuardianes() {
	formaSinElegir = os.Getenv("SYTUE_FORMA")
	if formaSinElegir == "" {
		formaSinElegir = tokens.FormaCarril
	}
	ElegirForma(formaSinElegir)
}

// ElegirApp dice de qué producto es esta app, con su clave de catálogo ("mail"): es lo
// que pinta el color «el de cada app». Lo dice la barra de la suite al montarse.
func ElegirApp(clave string) bool { return tokens.ElegirApp(clave) }

// AppElegida devuelve el producto en vigor para poder restaurar su apariencia.
func AppElegida() string { return tokens.AppElegida() }

// Color mapea el puñado de nombres que Fyne resuelve por su cuenta —el fondo del
// lienzo, el color de un `canvas.Text` sin color, la selección— a los tokens de la
// hoja. NO es la puerta por la que los widgets de la suite piden color: ver el
// comentario del paquete.
func (Sytue) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	t := De(v)
	switch n {
	case temafyne.ColorNameBackground:
		return Color("bg", t)
	case temafyne.ColorNameForeground:
		return Color("text", t)
	case temafyne.ColorNameDisabled, temafyne.ColorNamePlaceHolder:
		return Color("text-subtle", t)
	case temafyne.ColorNamePrimary, temafyne.ColorNameHyperlink:
		return Color("primary", t)
	case temafyne.ColorNameFocus:
		return Color("focus-ring", t)
	case temafyne.ColorNameSelection:
		return Color("selected", t)
	case temafyne.ColorNameHover:
		// El fondo que `widget.List` (Mail, Contacts, Drive, `ui.Tabla`) y `widget.Slider`
		// ponen al pasar el ratón. La web pinta ahí `hover:bg-bg-subtle` —medido en oscuro:
		// la fila de Mail y la de Drive pasan a #14232a—, y sin esto salía el de serie de
		// Fyne: blanco al 6 % en oscuro (#ffffff0f) y negro al 6 % en claro.
		return Color("bg-subtle", t)
	case temafyne.ColorNameError:
		return Color("danger", t)
	case temafyne.ColorNameSuccess:
		return Color("success", t)
	case temafyne.ColorNameWarning:
		return Color("warning", t)
	case temafyne.ColorNameShadow:
		// NINGUNA sombra de Fyne en la suite: la del borde de cada `container.Scroll`, que
		// la web no tiene (un `overflow-auto` a secas), y la de `canvas.Rectangle.Shadow`,
		// que cambia con el DPI. Las de la hoja las pinta `ui.Sombra`, y las dos piezas
		// que usaban el `PopUp` de Fyne, cuya única sombra era esta, van con `ui.Flotante`.
		// Hasta el 2026-09-23 se apagaba pieza a pieza con SinSombras (petición de Diapos).
		return color.Transparent
	case temafyne.ColorNameScrollBar:
		// `scrollbar-color: var(--sytue-border-strong) transparent` (styles.css): la barra del
		// color del borde fuerte y el carril transparente. Sin esto salía la de serie de Fyne,
		// negra al 60 % —#666 sobre blanco— y, al pasar el ratón, un carril gris #dbdbdb de 16.
		return Color("border-strong", t)
	case temafyne.ColorNameScrollBarBackground:
		return color.Transparent
	case temafyne.ColorNameSeparator:
		return Color("border-subtle", t)
	case temafyne.ColorNameInputBorder:
		return Color("border-strong", t)
	case temafyne.ColorNameMenuBackground:
		return Color("menu-surface", t)
	case temafyne.ColorNameInputBackground, temafyne.ColorNameOverlayBackground,
		temafyne.ColorNameButton:
		return Color("surface", t)
	}
	return temafyne.DefaultTheme().Color(n, v)
}

// # Una cara que `widget.Entry` tiene que MEDIR igual que la pinta
//
// El editor de Fyne coloca el cursor midiendo con el tema de la APLICACIÓN
// (`RenderedTextSize(…, estilo, nil)`) aunque pinte con el de su envoltorio: una cara
// puesta solo en el envoltorio sale pintada con su letra y medida con Figtree, y el
// cursor cae entre dos glifos. La celda de una hoja se edita con SU letra —Arial,
// Georgia, la negrita cursiva—, así que la cara tiene que llegar a este tema.
//
// La única llave que Fyne pasa de un lado al otro es el `fyne.TextStyle`, y en él un
// `TabWidth` ≤ 0 es «el de siempre» (cuatro espacios): uno NEGATIVO no cambia nada de lo
// que se pinta y sirve de marca. EstiloDeCara reserva un número por cara, para siempre
// —el cacheador de Fyne recuerda la cara de cada estilo—, y Font la devuelve.
var caras struct {
	sync.Mutex
	lista []fyne.Resource
}

// EstiloDeCara es el estilo con el que Font devuelve la cara `r`.
func EstiloDeCara(r fyne.Resource) fyne.TextStyle {
	caras.Lock()
	defer caras.Unlock()
	i := slices.Index(caras.lista, r)
	if i < 0 {
		caras.lista = append(caras.lista, r)
		i = len(caras.lista) - 1
	}
	return fyne.TextStyle{TabWidth: -(i + 1)}
}

// caraDe es la cara reservada con ese estilo, o nil.
func caraDe(s fyne.TextStyle) fyne.Resource {
	if s.TabWidth >= 0 {
		return nil
	}
	caras.Lock()
	defer caras.Unlock()
	if i := -s.TabWidth - 1; i < len(caras.lista) {
		return caras.lista[i]
	}
	return nil
}

// Font devuelve Figtree. Es la razón principal de que este tipo exista: `canvas.Text`
// resuelve su cara por el tema de la aplicación, así que sin esto la suite se pinta con
// la letra de serie de Fyne.
//
// ponytail: no hay respaldo con griego ni cirílico, igual que en Gio. Figtree no los
// trae; se cierra añadiendo los estáticos de Inter a `apps/nucleo/fuentes` y una pila
// de respaldo. Hasta entonces Fyne cae a su propia fuente para esos alfabetos, que se
// leen pero no con la letra de la web.
func (Sytue) Font(s fyne.TextStyle) fyne.Resource {
	if r := caraDe(s); r != nil {
		return r
	}
	switch {
	case s.Italic:
		return Cursiva
	case s.Bold:
		return Negra
	case s.Monospace:
		return Mono
	case s.Symbol:
		return temafyne.DefaultTheme().Font(s)
	}
	return Regular
}

// Icon delega en el tema de serie: los iconos de la suite son los SVG de `branding/` y
// los pinta cada widget, no el tema.
func (Sytue) Icon(n fyne.ThemeIconName) fyne.Resource { return temafyne.DefaultTheme().Icon(n) }

// Size contesta con las medidas de la suite para lo que Fyne pregunta por su cuenta.
//
// OJO: Fyne multiplica lo que aquí se devuelva por el ajuste de tamaño de letra del
// usuario, así que estos números **no sirven para la geometría de los widgets de la
// suite**, que los toma de medidas.go sin intermediarios.
func (Sytue) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case temafyne.SizeNameText:
		return TextoRaiz.Tam
	case temafyne.SizeNameCaptionText:
		return TextoXS.Tam
	case temafyne.SizeNameHeadingText:
		return TextoXL.Tam
	case temafyne.SizeNameSubHeadingText:
		return TextoLG.Tam
	case temafyne.SizeNamePadding:
		return Espacio(2)
	case temafyne.SizeNameInnerPadding:
		return Espacio(1)
	case temafyne.SizeNameInputBorder, temafyne.SizeNameSeparatorThickness:
		return Borde
	case temafyne.SizeNameLineSpacing:
		return TextoRaiz.Linea - TextoRaiz.Tam
	case temafyne.SizeNameSelectionRadius, temafyne.SizeNameInputRadius,
		temafyne.SizeNameButtonRadius:
		return RadioMD
	case temafyne.SizeNameScrollBar, temafyne.SizeNameScrollBarSmall:
		// `scrollbar-width: thin`: medido en producción a ×2, el pulgar mide 6 de ancho, con
		// los extremos redondos, y no crece al pasar el ratón.
		return AnchoBarraDesplazamiento
	case temafyne.SizeNameScrollBarRadius:
		return AnchoBarraDesplazamiento / 2
	case temafyne.SizeNameCardRadius:
		return RadioLG
	case temafyne.SizeNameDialogRadius, temafyne.SizeNameMenuRadius, temafyne.SizeNamePopupRadius:
		return RadioXL
	}
	return temafyne.DefaultTheme().Size(n)
}

// AnchoBarraDesplazamiento es el del pulgar de `scrollbar-width: thin` (medido: 6).
const AnchoBarraDesplazamiento float32 = 6

// SinSombras es el tema de la suite con la sombra de Fyne transparente. Desde que Sytue
// la apaga para toda la suite es el mismo tema; se conserva para quien ya lo pone
// alrededor de un desplazamiento, y porque sigue diciendo lo que ese sitio necesita.
type SinSombras struct{ Sytue }

// Color devuelve transparente para la sombra y lo del tema para todo lo demás.
func (s SinSombras) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == temafyne.ColorNameShadow {
		return color.Transparent
	}
	return s.Sytue.Color(n, v)
}

// SinBarras es SinSombras y además sin la barra de desplazamiento: el
// `[scrollbar-width:none]` de las tiras que se deslizan de lado (las pestañas de Sytue
// Photos en el teléfono, la fila de mandos de su visor, la de CertiPDF). Se pone igual,
// con `container.NewThemeOverride` sobre el desplazamiento.
type SinBarras struct{ Sytue }

// Color devuelve transparente para la barra, su carril y la sombra.
func (s SinBarras) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case temafyne.ColorNameScrollBar, temafyne.ColorNameScrollBarBackground, temafyne.ColorNameShadow:
		return color.Transparent
	}
	return s.Sytue.Color(n, v)
}

// carasSistema guarda cada cara del sistema ya hecha recurso: un recurso nuevo por
// pintado sería una fuente nueva para el cacheador de Fyne.
var carasSistema = struct {
	sync.Mutex
	m map[string]fyne.Resource
}{m: map[string]fyne.Resource{}}

// CaraDelSistema es la cara de una familia que la suite no trae —Arial, Georgia,
// Times…— tal como la tenga instalada el equipo (`fuentes.DelSistema`), que es lo que
// hace el navegador con un `font-family`; nil si no la tiene, y entonces quien la pide
// cae en su reserva.
func CaraDelSistema(familia string, negrita, cursiva bool) fyne.Resource {
	clave := familia + "|" + strconv.FormatBool(negrita) + "|" + strconv.FormatBool(cursiva)
	carasSistema.Lock()
	defer carasSistema.Unlock()
	if r, ok := carasSistema.m[clave]; ok {
		return r
	}
	var r fyne.Resource
	if datos, ok := fuentes.DelSistema(familia, negrita, cursiva); ok {
		r = fyne.NewStaticResource(clave+".ttf", datos)
	}
	carasSistema.m[clave] = r
	return r
}
