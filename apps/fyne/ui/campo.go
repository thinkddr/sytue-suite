// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Form.tsx

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	temafyne "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TallaCampo son las tallas de campo de la suite. El valor cero es `md`, como en
// `InputSize` de Form.tsx: `md` (31,5) es el control de formulario y `sm` (21) el del
// cromo denso —cinta del editor, barras de propiedades—, donde un `h-9` no cabe.
//
// La tercera NO sale de Form.tsx: es el campo de la CABECERA DE VISTA, que la web
// escribe a mano en cada app porque las variantes de `Input` asumen superficie clara y
// no leen sobre el cromo. Es la misma en las que la tienen (el buscador de Drive y el
// de Calendar, que su propio código señala como el mismo idioma), así que aquí es una
// talla y no una copia por app.
type TallaCampo uint8

// Las cuatro tallas.
const (
	CampoMD TallaCampo = iota
	CampoSM
	CampoCromo
	// CampoBusqueda es el campo de la página de búsqueda del Hub (`buscar.astro`),
	// escrito a mano como el de la cabecera: `h-10 pr-3 pl-10 text-base`, con la lupa
	// de 18 en `left-3`.
	CampoBusqueda
)

// medidaCampo es una fila de INPUT_SIZE en Form.tsx, en píxeles.
//
// `margen` es el relleno HORIZONTAL medido desde el borde, que es lo que hace
// `padding` con `box-sizing: border-box`: el texto empieza en `Borde + margen`.
type medidaCampo struct {
	alto, margen, margenIcono, icono, iconoX float32
	texto                                    tema.Escalon
}

var medidasCampo = [...]medidaCampo{
	// `h-9 px-3 text-base`, con icono `pl-9` y el icono en `left-3`.
	CampoMD: {
		alto: tema.Espacio(9), margen: tema.Espacio(3), margenIcono: tema.Espacio(9),
		icono: 16, iconoX: tema.Espacio(3), texto: tema.TextoBase,
	},
	// `h-6 min-h-[24px] px-1.5 text-xs`, con icono `pl-7` y el icono en `left-1.5`. El
	// alto son 24 px FIJOS: `h-6` a secas eran 21 con la raíz a 14 y la web le puso el
	// suelo de `Button xs` e `IconButton sm` (PR #841).
	CampoSM: {
		alto: LadoIconoSM, margen: tema.Espacio(1.5), margenIcono: tema.Espacio(7),
		icono: 12, iconoX: tema.Espacio(1.5), texto: tema.TextoXS,
	},
	// `h-8 pr-2.5 pl-8 text-sm`, con el icono de 16 en `left-2.5`. Medido en el
	// buscador de la cabecera de Sytue Drive: 28 de alto y 196 de ancho (`sm:w-56`).
	CampoCromo: {
		alto: tema.Espacio(8), margen: tema.Espacio(2.5), margenIcono: tema.Espacio(8),
		icono: 16, iconoX: tema.Espacio(2.5), texto: tema.TextoSM,
	},
	CampoBusqueda: {
		alto: tema.Espacio(10), margen: tema.Espacio(3), margenIcono: tema.Espacio(10),
		icono: 18, iconoX: tema.Espacio(3), texto: tema.TextoBase,
	},
}

// CampoPropio son las medidas de un campo que la web escribe A MANO con clases, fuera de
// Form.tsx —`border-border rounded-md border px-2 py-1 text-xs`—: su relleno, su letra y
// el token de su borde. Con él, la talla no cuenta.
//
// Existe porque esos campos son muchos y ninguno es una talla: CertiPDF tiene seis
// combinaciones distintas en sus paneles (23, 26,5… de alto, `px-1.5` o `px-2`, borde
// `border-border` y no el `border-strong` de Form.tsx). Una talla por combinación
// llenaría la tabla de filas que solo usa una pantalla.
type CampoPropio struct {
	// MargenX y MargenY son el `px-*` y el `py-*`.
	MargenX, MargenY float32
	Texto            tema.Escalon
	// Peso es la cara del texto; su cero conserva Normal en los campos existentes.
	Peso Peso
	// Borde es el token del borde, fijo: estos campos no cambian de borde al pasar por
	// encima. Vacío es el de Form.tsx.
	Borde string
	// Radio es el de la clase (`rounded`, `rounded-md`). Cero es el de Form.tsx.
	Radio float32
	// Tinta es el token del texto (`text-text-muted`). Vacía es el de Form.tsx.
	Tinta string
	// Relleno es el token del fondo (`bg-bg` en los formularios del ERP, `bg-surface`
	// en su buscador). Vacío es el `surface` de Form.tsx.
	Relleno string
	// MarcaDelTexto es un `<input>` SIN clase `placeholder:`: su marca de agua es la de
	// preflight de Tailwind, `color-mix(in oklab, currentcolor 50%, transparent)`, la
	// tinta del texto a media opacidad, y no el `text-text-subtle` de Form.tsx.
	MarcaDelTexto bool
	// Mono es `font-mono`: lo escrito va en JetBrains Mono. Es para lo técnico —un PEM,
	// un identificador—, donde las columnas se leen.
	Mono bool
	// Centrado es `text-center` en un campo de una línea: lo escrito va en el medio de la
	// caja. Lo que no cabe corre desde la izquierda, como un `<input>` desbordado.
	Centrado bool
	// Plano es un campo sin caja: ni fondo, ni borde, ni aro (`border-0 bg-transparent`
	// y sin `box-shadow` al enfocar). Lo enmarca quien lo contiene, como la fila de la
	// captura de Notes, que subraya la fila entera.
	Plano bool
	// FondoActivo es el hover:bg-* y focus:bg-* de un campo Plano interactivo.
	// Si se especifica, mantiene el aro de foco; vacío conserva el Plano sin aro.
	FondoActivo string
}

// Alto es el de una línea: la caja de línea, el `py-*` y el borde.
func (c CampoPropio) Alto() float32 { return c.Texto.Linea + 2*c.MargenY + 2*tema.Borde }

// AltoCampo es el alto de un campo de esa talla, que es el número de `h-9`/`h-6` de la
// hoja. Lo usan el selector y el campo de fecha, que son el mismo alto.
//
// Con el dedo, `md` mide 44 (`pointer-coarse:min-h-[max(44px,2.75rem)]` de Form.tsx), el
// mismo suelo que el botón: en el barrido de diálogos (2026-10-16) los campos se quedaban
// en 36 al lado de botones de 44. `sm` es cromo denso y se queda como está.
func AltoCampo(t TallaCampo) float32 {
	if t == CampoMD {
		return sueloTactil(medidasCampo[t].alto)
	}
	return medidasCampo[t].alto
}

// colorCampo es `controlBase` + `controlState` de Form.tsx: el relleno, el borde, la
// tinta y la marca de agua que comparten Entrada, AreaTexto y Selector.
//
// El borde en reposo es `border-strong`; al pasar por encima, `text-subtle`; con error,
// `danger`. Y NO cambia al enfocar: los campos se escribían un borde de latón propio y,
// como el aro era del mismo color, los dos se fundían en un marco de seis píxeles. El
// aro ya es la señal.
func colorCampo(v tema.Variante, invalido, encima, inerte bool) (relleno, borde, texto, pista color.NRGBA) {
	borde = tema.Color("border-strong", v)
	switch {
	case invalido:
		borde = tema.Color("danger", v)
	case encima:
		borde = tema.Color("text-subtle", v)
	}
	return atenuar(tema.Color("surface", v), inerte), atenuar(borde, inerte),
		atenuar(tema.Color("text", v), inerte), atenuar(tema.Color("text-subtle", v), inerte)
}

// colorCampoCromo es el campo que vive SOBRE LA BANDA DE TINTA: el buscador de la suite.
//
// Es lo que PINTA la web, medido en producción el 2026-09-25, y no lo que pide
// GlobalSearch.tsx. Ahí dice `bg-chrome-hover text-on-chrome
// placeholder:text-on-chrome-muted`, pero `controlBase` de Form.tsx trae `bg-surface
// text-text placeholder:text-text-subtle` y entre dos utilidades de la misma propiedad
// manda el orden de la hoja: gana la superficie. Del cromo solo quedan el borde
// (`chrome-border`) y la lupa (`text-on-chrome-muted`, que va en el icono). El borde
// al pasar por encima es `hover:border-text-subtle`, el de cualquier campo.
//
// Hasta el 2026-09-25 esto devolvía el velo translúcido del cromo, y `Caja` pinta el
// borde como un rectángulo RELLENO con el relleno encima: el interior enseñaba el
// borde entero. En reposo salía #5a6d77; al pasar por encima, el borde opaco
// `on-chrome-muted` teñía TODO el campo de #b5c4cb, que es la queja del dueño. Ahora
// los dos colores van compuestos sobre la superficie y son opacos.
func colorCampoCromo(v tema.Variante, invalido, encima, inerte bool) (relleno, borde, texto, pista color.NRGBA) {
	relleno, borde, texto, _ = colorCampo(v, invalido, encima, inerte)
	fondo := tema.Color("surface", v)
	if !invalido && !encima {
		borde = atenuar(tokens.SobreSRGB(tema.Color("chrome-border", v), fondo), inerte)
	}
	return relleno, borde, texto, atenuar(tokens.SobreSRGB(tema.Color("on-chrome-muted", v), fondo), inerte)
}

// colorCampoCarril es el campo apoyado EN EL CARRIL: el buscador de la suite cuando la
// barra entera es de color, por debajo de 768 px (`.sytue-topbar-carril input` de
// appearance.css). Un campo blanco macizo en mitad de la barra de color pesaría más que
// la marca, así que va en velo oscuro NEUTRO (`sidebar-field`), sin borde, con el texto
// y la marca de agua del carril.
//
// El relleno se devuelve CON SU ALFA y sin borde (`border-color: transparent`): es un
// velo sobre la barra, que en el traslúcido deja ver el lienzo, y `Caja` sin borde no
// pinta nada debajo del relleno.
func colorCampoCarril(v tema.Variante, invalido, _, _ bool) (relleno, borde, texto, pista color.NRGBA) {
	relleno = tema.Color("sidebar-field", v)
	if invalido {
		borde = tema.Color("danger", v)
	}
	sobre := tokens.SobreSRGB(relleno, tema.Color("sidebar-bg", v))
	return relleno, borde, tema.Color("sidebar-fg", v), tokens.SobreSRGB(tema.Color("sidebar-fg-muted", v), sobre)
}

// coloresDe elige la paleta del campo: la de superficie o la del cromo.
func coloresDe(cromo bool) func(tema.Variante, bool, bool, bool) (color.NRGBA, color.NRGBA, color.NRGBA, color.NRGBA) {
	if cromo {
		return colorCampoCromo
	}
	return colorCampo
}

// altoFuente es la caja de línea de la FUENTE para ese escalón: lo que Fyne va a
// ocupar de alto al pintar una línea.
//
// No es la caja de línea de CSS —que es la que ocupa sitio en el layout de la web— y
// por eso hay dos: la de CSS se reserva y la de la fuente se CENTRA dentro. Es la misma
// cuenta que hace ColocarTexto para un `canvas.Text`, aplicada a lo que ocupa el editor.
func altoFuente(e tema.Escalon) float32 { return altoCara(e, tema.Regular) }

// altoCara es altoFuente con otra letra: JetBrains Mono no tiene la caja de Figtree.
func altoCara(e tema.Escalon, cara fyne.Resource) float32 {
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize("Hg", e.Tam, fyne.TextStyle{}, cara)
	return tam.Height
}

// ---- el motor de edición ----

// editor es `widget.Entry` con los dos avisos que su API no expone.
//
// # Por qué se ENVUELVE el de Fyne en vez de escribir uno
//
// Medido el 2026-09-21 con el pintor de software: apagándole el cromo por tema —borde,
// fondo, rellenos y espaciado a cero—, `widget.Entry` pinta su texto EN EL MISMO PÍXEL
// que un `canvas.Text` nuestro del mismo escalón, y su `MinSize` es exactamente la caja
// de línea de la fuente. Desvío: 0,000 px en las dos cotas
// (`TestElEditorDeFynePintaDondeElNuestro`).
//
// Es decir: lo que no se podía reestilar —sus 2.177 líneas de estado privado— resulta
// que tampoco hace falta reestilarlo. La caja, el borde, el radio, el aro de foco y la
// colocación son NUESTROS; de Fyne se aprovecha solo el motor: cursor, selección,
// arrastre, portapapeles, doble clic, deshacer, contraseña y validación. Escribir eso
// son varios cientos de líneas y es donde viven los fallos que nadie prueba.
//
// # Lo que NO se arregla envolviéndolo, y hay que decirlo
//
// **No hay IME en escritorio.** La composición CJK está rota en Fyne pase lo que pase,
// así que japonés, chino y coreano no se pueden teclear en ningún campo de la interfaz
// nativa. Escribir un editor propio no lo arreglaría: el agujero está en el driver, no
// en el widget.
type editor struct {
	widget.Entry
	alEnfocar func(bool)
	// alTecla es el TypedKey de la Entrada que lo envuelve (ver editor.TypedKey).
	alTecla func(*fyne.KeyEvent)
}

func nuevoEditor(alEnfocar func(bool)) *editor {
	e := &editor{alEnfocar: alEnfocar}
	e.ExtendBaseWidget(e)
	return e
}

func (e *editor) FocusGained() {
	e.Entry.FocusGained()
	e.alEnfocar(true)
	avisarEscritura(e)
}
func (e *editor) FocusLost() { e.Entry.FocusLost(); e.alEnfocar(false) }

// TypedKey pasa la tecla por la Entrada que lo envuelve. Con el puntero DE VERDAD el
// foco cae en el editor —es el objeto más profundo bajo el clic, y `Entrada.Tapped`
// también se lo da—, no en la Entrada: sin esto, tras pulsar en un campo su `AlTeclear`
// no veía ni una tecla, e Intro no enviaba el formulario ni Escape cerraba el diálogo.
// Solo se vio en una ventana real (2026-10-05, «Crear base» de Sytue Bases): el driver
// de pruebas enfoca la Entrada.
func (e *editor) TypedKey(k *fyne.KeyEvent) {
	if e.alTecla != nil {
		e.alTecla(k)
		return
	}
	e.Entry.TypedKey(k)
}

// TypedShortcut sube al lienzo el atajo que el editor no conoce, como el `keydown` que
// en la web sube del `<input>` al documento: con el cursor en un campo, Ctrl+F sigue
// abriendo Buscar y Ctrl ± sigue ampliando. Sin esto Fyne se lo daba al campo, que no
// lo conoce, y se perdía. Los que son del campo —copiar, pegar, deshacer, y moverse o
// borrar por palabras con las flechas y el retroceso— se quedan en él.
func (e *editor) TypedShortcut(s fyne.Shortcut) {
	if subeAlLienzo(s) {
		if c := atajosDelLienzo(e); c != nil {
			c.TypedShortcut(s)
			return
		}
	}
	e.Entry.TypedShortcut(s)
}

// atajosDelLienzo es quien atiende los atajos de la ventana de `o`: el lienzo del driver
// de escritorio los atiende (`common.Canvas.TypedShortcut`); el de pruebas no, y los
// tests lo cambian.
var atajosDelLienzo = func(o fyne.CanvasObject) fyne.Shortcutable {
	c, _ := fyne.CurrentApp().Driver().CanvasForObject(o).(fyne.Shortcutable)
	return c
}

// subeAlLienzo dice si un atajo no es del editor de texto.
func subeAlLienzo(s fyne.Shortcut) bool {
	c, es := s.(*desktop.CustomShortcut)
	if !es {
		return false
	}
	switch c.KeyName {
	case fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown, fyne.KeyHome, fyne.KeyEnd,
		fyne.KeyBackspace, fyne.KeyDelete, fyne.KeyPageUp, fyne.KeyPageDown:
		return false
	default: // cualquier otra combinación es del lienzo
		return true
	}
}

// temaEditor apaga todo el cromo de `widget.Entry` y le impone el escalón del campo.
//
// El tamaño de letra de un `widget.Entry` no es un campo suyo: sale de
// `theme.Size(SizeNameText)`. Por eso cada campo lleva su propio tema en vez de un
// tema global, que solo podría tener UN tamaño para los dos de la hoja.
type temaEditor struct {
	tema.Sytue
	escalon tema.Escalon
	inerte  bool
	// tintaTexto y tintaCursor, si no son nil, sustituyen a las del tema (ver
	// EditorPlano.Tinta). tintaMarca, a la de la marca de agua.
	tintaTexto, tintaCursor, tintaMarca color.Color
	// marcaMitad es CampoPropio.MarcaDelTexto.
	marcaMitad bool
	// sinBarras apaga las barras del desplazador: el editor desplazable de una línea es
	// un `<input>`, que corre el texto sin enseñar barra.
	sinBarras bool
	// mono es CampoPropio.Mono: la caja de la fuente se mide con JetBrains Mono.
	mono bool
	peso Peso
}

// cara es la letra con la que el editor pinta, que es con la que hay que medir su caja.
func (t temaEditor) cara() fyne.Resource {
	if t.mono {
		return tema.Mono
	}
	return t.peso.Cara()
}

// Font conserva el tratamiento de mono y cursiva del tema base; un peso propio
// resuelve también el texto normal de widget.Entry con su cara exacta.
func (t temaEditor) Font(s fyne.TextStyle) fyne.Resource {
	if t.peso != Normal && !s.Monospace {
		return t.peso.Cara()
	}
	return t.Sytue.Font(s)
}

// tinta y marca son los dos colores que el editor de Fyne resuelve por tema.
func (t temaEditor) tinta(v tema.Variante) color.NRGBA {
	return tema.Color("text", v)
}

func (t temaEditor) marca(v tema.Variante) color.NRGBA {
	if t.marcaMitad {
		c := t.tinta(v)
		if t.tintaTexto != nil {
			if n, ok := color.NRGBAModel.Convert(t.tintaTexto).(color.NRGBA); ok {
				c = n
			}
		}
		c.A /= 2
		return c
	}
	return tema.Color("text-subtle", v)
}

func (t temaEditor) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	// La caja, el borde y el fondo los pinta `Caja`, que sí sabe el radio y el grosor
	// de la hoja. Los de Fyne se apagan; si no, se pintan ENCIMA de los nuestros.
	case temafyne.ColorNameInputBorder, temafyne.ColorNameInputBackground,
		temafyne.ColorNameBackground, temafyne.ColorNameHover:
		return color.NRGBA{}
	// Un campo de una línea DESPLAZA el texto que no cabe, como un `<input>`, y el
	// desplazamiento de Fyne trae barra y sombra de borde que la web no tiene.
	case temafyne.ColorNameScrollBar, temafyne.ColorNameScrollBarBackground,
		temafyne.ColorNameShadow:
		return color.NRGBA{}
	// El cursor de Fyne se pinta con el color primario. En la web el cursor es del
	// color del texto (`caret-color: auto`), y con la tinta de `--sytue-primary` un
	// cursor primario sería casi el mismo color pero no el mismo.
	case temafyne.ColorNamePrimary:
		if t.tintaCursor != nil {
			return t.tintaCursor
		}
		return atenuar(t.tinta(tema.De(v)), t.inerte)
	case temafyne.ColorNameForeground:
		if t.tintaTexto != nil {
			return t.tintaTexto
		}
		return atenuar(t.tinta(tema.De(v)), t.inerte)
	case temafyne.ColorNameDisabled, temafyne.ColorNamePlaceHolder:
		if t.tintaMarca != nil {
			return t.tintaMarca
		}
		return atenuar(t.marca(tema.De(v)), t.inerte)
	}
	return t.Sytue.Color(n, v)
}

func (t temaEditor) Size(n fyne.ThemeSizeName) float32 {
	if t.sinBarras && (n == temafyne.SizeNameScrollBar || n == temafyne.SizeNameScrollBarSmall) {
		return 0
	}
	switch n {
	case temafyne.SizeNameText:
		return t.escalon.Tam
	case temafyne.SizeNameInputBorder, temafyne.SizeNameInnerPadding,
		temafyne.SizeNamePadding, temafyne.SizeNameInputRadius:
		return 0
	// El interlineado es la diferencia entre la caja de línea de CSS y la de la fuente.
	// Con esto, en un área de texto la SEGUNDA línea cae donde la pone el navegador; sin
	// esto, cada línea sube 2,45 px sobre la anterior y a la cuarta el error es de diez.
	case temafyne.SizeNameLineSpacing:
		return t.escalon.Linea - altoCara(t.escalon, t.cara())
	}
	return t.Sytue.Size(n)
}

// ---- Entrada ----

// MedidaEntrada es lo que ocupa un campo de una línea: el ancho pedido por el alto de
// su talla.
//
// En la web el campo es `w-full` y el ancho lo pone su contenedor, así que aquí `ancho`
// manda; con 0 se declara el MÍNIMO —el cromo más un cuadratín de sitio para escribir—
// y el contenedor lo estira. Lo que no depende de nadie es el ALTO, que es el número de
// `h-9`/`h-6` y es lo que comprueba el guardián.
func MedidaEntrada(t TallaCampo, ancho float32, conIcono bool) fyne.Size {
	m := medidasCampo[t]
	if ancho <= 0 {
		izq := m.margen
		if conIcono {
			izq = m.margenIcono
		}
		ancho = 2*tema.Borde + izq + m.margen + tema.Rem
	}
	return fyne.NewSize(ancho, AltoCampo(t))
}

// Entrada es `<Input>` de Form.tsx: una caja de una línea con su borde, su marca de
// agua y su icono opcional a la izquierda.
type Entrada struct {
	widget.BaseWidget
	// Rotulo es el nombre accesible, YA traducido: el `aria-label` o el `<label>` del
	// campo en la web. Vacío: la marca de agua, que es lo que un lector lee a falta de
	// etiqueta.
	Rotulo string
	Talla  TallaCampo
	// Ancho fija el ancho (en la web es `w-full`). Con 0, el mínimo.
	Ancho float32
	// Invalida es `aria-invalid`: pinta el borde de `danger`. El aro de foco NO cambia
	// de color —`danger` se queda en 2,65:1 sobre el cromo oscuro y sería un indicador
	// de foco que incumple WCAG 1.4.11—; el error ya se señala por el borde y el texto.
	Invalida bool
	// requerida: la última Requerida la marcó; al escribir se quita.
	requerida bool
	// Icono es el glifo a la izquierda (`leadingIcon`). `iconos.Ninguno`, que es el
	// cero, no lo hay.
	Icono iconos.Icono
	// SobreCromo es el campo que vive sobre la banda de tinta —el buscador de la
	// suite—: el de superficie con el borde del cromo y la lupa apagada del cromo
	// (ver colorCampoCromo). El aro es el azul de siempre, como en la web.
	SobreCromo bool
	// EnCarril es el campo apoyado en el carril de color: el buscador de la suite cuando
	// la barra entera es carril, por debajo de 768 px (ver colorCampoCarril). Lo pone y
	// lo quita la barra según el ancho de la ventana.
	EnCarril bool
	// Radio es el redondeo de la caja. Cero es `rounded-md`, que es el de la hoja;
	// el buscador de la suite pide `rounded-pill!`.
	Radio float32
	// Propia son las medidas de un campo escrito a mano (ver CampoPropio). Nil es la
	// talla.
	Propia *CampoPropio
	// AlTeclear ve la tecla ANTES que el editor y devuelve `true` cuando se la queda.
	//
	// Existe por el combobox: el buscador de la suite deja el foco en el campo y
	// recorre los resultados con las flechas, así que Arriba, Abajo, Enter y Escape
	// son suyas y no del cursor. Sin este gancho habría que mover el foco al panel, y
	// entonces no se puede seguir escribiendo — que es la mitad de lo que hace útil a
	// un buscador.
	AlTeclear func(*fyne.KeyEvent) bool
	// Busqueda es `type="search"`: Escape vacía lo escrito, como en el navegador. Con el
	// campo vacío, Escape sigue su camino (el diálogo o el menú de encima).
	Busqueda bool
	// AlEnfocar avisa de que el campo ha ganado o perdido el foco. Lo usa quien tenga
	// que reaccionar fuera del campo: enseñar la pista del atajo, cerrar un panel.
	AlEnfocar func(bool)
	// AlPulsarIcono se llama al pulsar el icono de la izquierda: el del `CampoFecha`
	// abre el calendario (`ConCalendario`). Nil, el icono es solo dibujo.
	AlPulsarIcono func()

	// marca es la marca de agua ENTERA; la del editor es la que cabe, con «…».
	marca       string
	editor      *editor
	alCambiar   func(string)
	env         *container.ThemeOverride
	temaEd      *temaEditor
	encima      bool
	inerte      bool
	soloLectura bool
	enfocado    bool
	r           *campoRenderer
}

var (
	_ fyne.Widget       = (*Entrada)(nil)
	_ fyne.Tappable     = (*Entrada)(nil)
	_ fyne.Focusable    = (*Entrada)(nil)
	_ fyne.Shortcutable = (*Entrada)(nil)
	_ Campo             = (*Entrada)(nil)
	_ desktop.Hoverable = (*Entrada)(nil)
)

// medida es la fila de la tabla de la talla, o la propia si la hay.
func (e *Entrada) medida() medidaCampo {
	m := medidasCampo[e.Talla]
	if p := e.Propia; p != nil {
		m.alto, m.margen, m.texto = p.Alto(), p.MargenX, p.Texto
	}
	return m
}

// ---- foco ----

// El campo ES enfocable, y no lo era: lo enfocable era su editor interno, que vive
// DENTRO de un `container.ThemeOverride` y al que nadie de fuera llega. Consecuencia
// medida en Sytue Drive: el diálogo «Nueva carpeta» no podía declarar su campo como
// `Dialogo.Foco` —no satisfacía `ui.Campo`— y el foco inicial caía en el aspa.
//
// Las cuatro llamadas se delegan al editor, que es quien tiene el cursor y quien
// entiende las teclas. El widget de fuera solo hace de puerta.

// FocusGained da el foco al editor de dentro.
func (e *Entrada) FocusGained() { e.editor.FocusGained() }

// FocusLost se lo quita.
func (e *Entrada) FocusLost() { e.editor.FocusLost() }

// TypedRune pasa la letra al editor. Un campo de solo lectura se las come: tiene el
// foco y el cursor, pero no cambia.
func (e *Entrada) TypedRune(r rune) {
	if e.soloLectura {
		return
	}
	e.editor.TypedRune(r)
}

// TypedKey pasa la tecla al editor, salvo en solo lectura, donde solo dejan pasar las
// que MUEVEN —flechas, inicio, fin— para poder recorrer y seleccionar el texto.
func (e *Entrada) TypedKey(k *fyne.KeyEvent) {
	if e.AlTeclear != nil && e.AlTeclear(k) {
		return
	}
	if e.Busqueda && k.Name == fyne.KeyEscape && e.Texto() != "" {
		e.PonerTexto("")
		return
	}
	// Escape es del diálogo o del menú que haya encima, como en la web, donde el
	// `keydown` de `document` en fase de captura lo recibe con el foco en un campo.
	if SubirEscape(e, k) {
		return
	}
	if e.soloLectura && !mueveElCursor(k.Name) {
		return
	}
	e.editor.Entry.TypedKey(k) // el del motor: `editor.TypedKey` vuelve aquí
}

// TypedShortcut pasa al editor copiar, pegar, cortar, deshacer y seleccionar todo. Fyne
// entrega los atajos al objeto CON EL FOCO, y ese es este campo, no su editor: sin
// esto se iban al lienzo y ⌘C/⌘V/⌘A (Ctrl en Linux y Windows) no hacían nada en
// ningún campo de la suite. Solo se vio en una ventana real (2026-09-27, macOS y
// X11): el driver de pruebas escribe directamente en el editor. En solo lectura se
// puede seleccionar y copiar, pero no cambiar el texto.
func (e *Entrada) TypedShortcut(s fyne.Shortcut) {
	if e.soloLectura {
		switch s.(type) {
		case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		default:
			return
		}
	}
	e.editor.TypedShortcut(s)
}

// SelectedText es lo seleccionado en el editor: el driver lo mira antes de copiar.
func (e *Entrada) SelectedText() string { return e.editor.SelectedText() }

// teclasQueMueven son las que un campo de solo lectura sigue atendiendo: las que
// recorren el texto, ninguna que lo cambie.
var teclasQueMueven = []fyne.KeyName{
	fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown,
	fyne.KeyHome, fyne.KeyEnd, fyne.KeyPageUp, fyne.KeyPageDown,
}

func mueveElCursor(n fyne.KeyName) bool { return slices.Contains(teclasQueMueven, n) }

// EsCampo marca el tipo como campo de formulario, que es lo que `Dialogo` busca para
// darle el foco inicial.
func (e *Entrada) EsCampo() {}

// AccessibilityLabel es el `aria-label` del campo (peticiones de Diapos y Calendar).
func (e *Entrada) AccessibilityLabel() string {
	if e.Rotulo != "" {
		return e.Rotulo
	}
	return e.marca
}

// AccessibilityRole: Fyne 2.8 solo conoce cuatro roles y ninguno es «campo de texto»;
// el más cercano es texto.
func (e *Entrada) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// NuevaEntrada crea un campo de talla md con esa marca de agua.
func NuevaEntrada(marcaDeAgua string) *Entrada {
	e := &Entrada{}
	e.ExtendBaseWidget(e)
	e.montar(false)
	e.marca, e.editor.PlaceHolder = marcaDeAgua, marcaDeAgua
	return e
}

func (e *Entrada) montar(multilinea bool) {
	e.editor = nuevoEditor(func(f bool) {
		e.enfocado = f
		e.Refresh()
		if e.AlEnfocar != nil {
			e.AlEnfocar(f)
		}
	})
	e.editor.alTecla = e.TypedKey
	e.editor.MultiLine = multilinea
	e.editor.OnChanged = func(s string) {
		e.recentrar()
		if e.requerida && s != "" {
			e.requerida, e.Invalida = false, false
			e.Refresh()
		}
		if e.alCambiar != nil {
			e.alCambiar(s)
		}
	}
	if multilinea {
		e.editor.Wrapping = fyne.TextWrapWord
	} else {
		// `ScrollHorizontalOnly` y no `ScrollNone`: con `ScrollNone` Fyne no recorta, y
		// la marca de agua y el texto largos se salían de la caja (petición de
		// CertiPDF, en su Asistente). Un `<input>` los recorta y sigue al cursor.
		e.editor.Scroll = fyne.ScrollHorizontalOnly
		e.editor.Wrapping = fyne.TextWrapOff
	}
	e.temaEd = &temaEditor{escalon: e.medida().texto}
	e.env = envolver(e.editor, e.temaEd)
}

// envolver mete el editor en su tema. Es una línea, y está aparte porque el guardián
// que justifica envolver `widget.Entry` monta uno igual y tiene que montarlo IGUAL: si
// el envoltorio de producción cambia y el del test no, el test deja de medir esto.
func envolver(ed *editor, th *temaEditor) *container.ThemeOverride {
	return container.NewThemeOverride(ed, th)
}

// Texto es lo escrito.
func (e *Entrada) Texto() string { return e.editor.Text }

// PonerTexto escribe en el campo.
func (e *Entrada) PonerTexto(s string) { e.editor.SetText(s); e.recentrar() }

// recentrar vuelve a colocar el editor de un campo [CampoPropio.Centrado], cuyo sitio
// depende de lo escrito.
func (e *Entrada) recentrar() {
	if e.r != nil && e.Propia != nil && e.Propia.Centrado {
		e.r.Layout(e.Size())
	}
}

// PonerMarcaDeAgua cambia el `placeholder`: el del nombre del alta depende de si es
// para una persona o para una organización.
func (e *Entrada) PonerMarcaDeAgua(s string) {
	if e.marca != s {
		e.marca, e.editor.PlaceHolder = s, s
		e.editor.Refresh()
		if e.r != nil {
			e.r.Layout(e.Size())
		}
	}
}

// AlCambiar se llama con el texto nuevo en cada pulsación.
func (e *Entrada) AlCambiar(f func(string)) { e.alCambiar = f }

// PonerTextoAlFinal escribe en el campo y deja el cursor detrás de lo escrito: es lo
// que hace falta para reescribir lo que se está tecleando (los atajos `dd`/`tt` de
// Notes), con el cursor donde la persona sigue escribiendo.
func (e *Entrada) PonerTextoAlFinal(s string) {
	e.PonerTexto(s)
	e.editor.CursorRow, e.editor.CursorColumn = 0, len([]rune(s))
	e.editor.Refresh()
}

// Requerida es el `required` del navegador al enviar: vacía, se marca `aria-invalid`,
// toma el foco y devuelve false, en vez de que el botón no haga nada sin decir por qué.
// La marca se va al escribir. Como `required`, no recorta: un espacio ya cuenta.
func (e *Entrada) Requerida() bool {
	if e.Texto() != "" {
		return true
	}
	e.requerida, e.Invalida = true, true
	e.Refresh()
	if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
		enfocar(c, e.editor)
	}
	return false
}

// Contrasena pinta puntos en vez de letras (`type="password"`).
func (e *Entrada) Contrasena(si bool) { e.editor.Password = si }

// Inerte es `disabled`: no responde y se pinta al 50 % de opacidad, como todo lo que
// lleva `disabled:opacity-50` en la hoja.
func (e *Entrada) Inerte(si bool) {
	e.inerte = si
	if si {
		e.editor.Disable()
		e.encima = false
	} else {
		e.editor.Enable()
	}
	e.Refresh()
}

// SoloLectura es el `readOnly` del `<input>`: se ve y se recorre con el teclado, pero
// no se escribe.
//
// NO es `Inerte`, y la diferencia es la que hace la hoja: `disabled:opacity-50` atenúa
// el control, y `readOnly` no lleva ninguna clase, así que un campo de solo lectura se
// pinta a fuerza COMPLETA. En «Mi cuenta» son tres —el @usuario, la dirección de
// correo y el móvil— y los tres salen en el barrido con el mismo borde
// `border-strong` y la misma tinta que los editables: atenuarlos diría que están
// deshabilitados, que es otra cosa.
func (e *Entrada) SoloLectura(si bool) { e.soloLectura = si }

// Con devuelve el mismo campo con otra talla. Azúcar para que la pantalla se lea como
// el JSX del que sale.
func (e *Entrada) Con(t TallaCampo) *Entrada {
	e.Talla = t
	e.temaEd.escalon = e.medida().texto
	return e
}

// ConPropia devuelve el mismo campo con las medidas de uno escrito a mano.
func (e *Entrada) ConPropia(p CampoPropio) *Entrada {
	e.Propia = &p
	e.temaEd.escalon = p.Texto
	e.temaEd.marcaMitad = p.MarcaDelTexto
	e.temaEd.mono = p.Mono
	e.temaEd.peso = p.Peso
	e.editor.TextStyle.Monospace = p.Mono
	return e
}

// Tapped enfoca el campo al pulsar en el relleno, donde no hay editor que reciba el
// clic. Sin esto, pulsar junto al borde derecho de un campo largo no hace nada.
func (e *Entrada) Tapped(ev *fyne.PointEvent) {
	if e.inerte {
		return
	}
	// El icono vive en el relleno de la izquierda (`pl-9`), fuera del editor: el clic
	// llega aquí.
	if ev != nil && e.AlPulsarIcono != nil && e.Icono != iconos.Ninguno && ev.Position.X < e.medida().margenIcono {
		e.AlPulsarIcono()
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
		enfocar(c, e.editor)
	}
}

// MouseIn y MouseOut son `hover:border-text-subtle`. El editor de Fyne no es Hoverable,
// así que el aviso llega aquí, que es donde se pinta el borde.
func (e *Entrada) MouseIn(*desktop.MouseEvent) {
	if e.inerte {
		return
	}
	e.encima = true
	e.Refresh()
}

// MouseMoved no hace nada: el estado ya lo fijan MouseIn y MouseOut.
func (e *Entrada) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve el borde a su gris de reposo.
func (e *Entrada) MouseOut() {
	if !e.encima {
		return
	}
	e.encima = false
	e.Refresh()
}

// Cursor es la barra de texto, como en cualquier campo del sistema.
func (e *Entrada) Cursor() desktop.Cursor { return desktop.TextCursor }

// MinSize es la medida declarada, sin otra cuenta.
func (e *Entrada) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	m := MedidaEntrada(e.Talla, e.Ancho, e.Icono != iconos.Ninguno)
	if p := e.Propia; p != nil {
		m.Height = p.Alto()
		if e.Ancho <= 0 {
			m.Width = 2*tema.Borde + 2*p.MargenX + tema.Rem
		}
	}
	return m
}

// CreateRenderer compone la caja, el aro, el icono y el editor.
func (e *Entrada) CreateRenderer() fyne.WidgetRenderer {
	e.r = nuevoCampoRenderer(e, e.radio(), func() fyne.Size { return e.MinSize() })
	return e.r
}

// radio es el redondeo pedido o el de la hoja.
func (e *Entrada) radio() float32 {
	if e.Radio > 0 {
		return e.Radio
	}
	return tema.RadioMD
}

// ---- AreaTexto ----

// FilasAreaTexto es el `rows={4}` por defecto de `<Textarea>`.
const FilasAreaTexto = 4

// MedidaAreaTexto es lo que ocupa un área de texto de `filas` filas: `py-2` arriba y
// abajo, `filas` cajas de línea de `text-base` y el borde por cada lado.
//
// El alto de un `<textarea rows=N>` en el navegador es N veces la CAJA DE LÍNEA, no N
// veces la letra: con `text-base` son 4 × 19,25 = 77, más 14 de relleno y 2 de borde,
// 93 px. Es la cuenta que se equivoca quien usa el alto que devuelve el toolkit.
func MedidaAreaTexto(filas int, ancho float32) fyne.Size {
	alto := float32(filas)*tema.TextoBase.Linea + 2*tema.Espacio(2) + 2*tema.Borde
	if ancho <= 0 {
		ancho = 2*tema.Borde + 2*tema.Espacio(3) + tema.Rem
	}
	return fyne.NewSize(ancho, alto)
}

// AreaTexto es `<Textarea>` de Form.tsx: `px-3 py-2 text-base` y cuatro filas.
type AreaTexto struct {
	Entrada
	// Filas es el `rows` de la web.
	Filas int
}

var _ fyne.Widget = (*AreaTexto)(nil)

// NuevaAreaTexto crea un área de cuatro filas con esa marca de agua.
func NuevaAreaTexto(marcaDeAgua string) *AreaTexto {
	a := &AreaTexto{Filas: FilasAreaTexto}
	a.ExtendBaseWidget(a)
	a.montar(true)
	a.marca, a.editor.PlaceHolder = marcaDeAgua, marcaDeAgua
	return a
}

// MinSize es la medida declarada.
func (a *AreaTexto) MinSize() fyne.Size {
	a.ExtendBaseWidget(a)
	if p := a.Propia; p != nil {
		ancho := a.Ancho
		if ancho <= 0 {
			ancho = 2*tema.Borde + 2*p.MargenX + tema.Rem
		}
		return fyne.NewSize(ancho, float32(a.Filas)*p.Texto.Linea+2*p.MargenY+2*tema.Borde)
	}
	return MedidaAreaTexto(a.Filas, a.Ancho)
}

// CreateRenderer compone lo mismo que Entrada, con el relleno vertical del área.
func (a *AreaTexto) CreateRenderer() fyne.WidgetRenderer {
	a.r = nuevoCampoRenderer(&a.Entrada, a.radio(), func() fyne.Size { return a.MinSize() })
	a.r.margenY = tema.Espacio(2) // `py-2`
	if a.Propia != nil {
		a.r.margenY = a.Propia.MargenY
	}
	a.r.arriba = true // el texto se apoya arriba, no se centra
	return a.r
}

// ---- el renderer que comparten los dos ----

type campoRenderer struct {
	e       *Entrada
	aro     *Aro
	caja    *Caja
	glifo   *iconos.Dibujo
	objetos []fyne.CanvasObject
	medida  func() fyne.Size
	margenY float32
	arriba  bool
}

func nuevoCampoRenderer(e *Entrada, radio float32, medida func() fyne.Size) *campoRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde, _, pista := e.paleta()(v, e.Invalida, e.encima, e.inerte)
	borde, relleno = e.bordePropio(borde, v), e.rellenoPropio(relleno, v)
	r := &campoRenderer{
		e:      e,
		aro:    NuevoAroCon("focus-ring", radio, SinHueco, v),
		caja:   NuevaCaja(relleno, borde, radio),
		medida: medida,
	}
	if e.Icono != iconos.Ninguno {
		r.glifo = iconos.TrazoConTinta(e.Icono, e.medida().icono, pista)
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if r.glifo != nil {
		r.objetos = append(r.objetos, r.glifo.Objeto())
	}
	r.objetos = append(r.objetos, e.env)
	// Igual que la pestaña: `widget.Entry` solo materializa su MARCA DE AGUA en su
	// propio `Refresh`, y Fyne no lo llama antes del primer pintado. Sin esta línea,
	// «Buscar contactos» no se pinta nunca —medido: 2 324 píxeles de tinta con marca
	// de agua y sin ella, o sea ni uno del rótulo— y eso es todo campo de búsqueda de
	// la suite. Es la regla 7 del paquete.
	r.Refresh()
	return r
}

func (r *campoRenderer) MinSize() fyne.Size           { return r.medida() }
func (r *campoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *campoRenderer) Destroy()                     {}

func (r *campoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	m := r.e.medida()

	izq := tema.Borde + m.margen
	if r.glifo != nil {
		izq = tema.Borde + m.margenIcono
		// El icono va en `left-3`, que se mide desde la CAJA DE BORDE y no desde el
		// contenido: en la web es un `absolute` dentro del envoltorio, no relleno.
		r.glifo.Colocar(m.iconoX, (t.Height-m.icono)/2, m.icono)
	}
	der := tema.Borde + m.margen
	if p := r.e.Propia; p != nil && p.Centrado && !r.arriba {
		// ponytail: se mide con Figtree normal; un campo Mono o de contraseña centrado
		// quedaría corrido. Hoy solo lo usa el número de página de CertiPDF.
		s := r.e.editor.Text
		if s == "" {
			s = r.e.marca
		}
		if hueco := t.Width - izq - der - Medir(s, m.texto, Normal).Width; hueco > 0 {
			izq += hueco / 2
		}
	}

	linea := m.texto.Linea
	fuente := altoCara(m.texto, r.e.temaEd.cara())
	y := (t.Height - fuente) / 2 // una línea: centrada en la caja
	alto := fuente
	if r.arriba { // varias líneas: apoyadas arriba, con `py-2` y la caja de línea de CSS
		y = tema.Borde + r.margenY + (linea-fuente)/2
		alto = t.Height - 2*(tema.Borde+r.margenY) - (linea - fuente)
	}
	r.e.env.Move(fyne.NewPos(izq, y))
	r.e.env.Resize(fyne.NewSize(max(t.Width-izq-der, 0), max(alto, 0)))
	// La marca de agua que no cabe acaba en «…», como `text-overflow: ellipsis` en el
	// `<input>` de la web: cortada a tijera («Comida con Ana el jueves a las 14») parecía
	// el ejemplo entero.
	//
	// ponytail: se mide con la cara normal; un campo Mono con una marca larga cortaría
	// unos píxeles antes o después.
	if !r.arriba && r.e.marca != "" && t.Width > 0 {
		if cabe := recorta(r.e.marca, m.texto, Normal, max(t.Width-izq-der, 0)); cabe != r.e.editor.PlaceHolder {
			r.e.editor.PlaceHolder = cabe
			r.e.editor.Refresh()
		}
	}
}

func (r *campoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde, texto, pista := r.e.paleta()(v, r.e.Invalida, r.e.encima, r.e.inerte)
	r.caja.Pintar(r.e.rellenoPropio(relleno, v), r.e.bordePropio(borde, v))
	// En el carril el texto, el cursor, la marca de agua y el aro son los de allí; fuera,
	// los del tema (nil: los resuelve el editor).
	th, aro, tinta := r.e.temaEd, "focus-ring", ""
	th.tintaTexto, th.tintaCursor, th.tintaMarca = nil, nil, nil
	if r.e.EnCarril {
		th.tintaTexto, th.tintaCursor, th.tintaMarca = texto, texto, pista
		aro, tinta = "sidebar-focus", "sidebar-fg-muted"
	}
	r.aro.CambiarToken(aro)
	if r.glifo != nil {
		r.glifo.DeToken(tinta) // la lupa, con el relleno ámbar del carril o el del tema
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.e.enfocado)
	if p := r.e.Propia; p != nil && p.Plano {
		fondo := color.NRGBA{}
		if p.FondoActivo != "" {
			if r.e.encima || r.e.enfocado {
				fondo = tema.Color(p.FondoActivo, v)
			}
			r.aro.Ver(r.e.enfocado)
		} else {
			r.aro.Ver(false)
		}
		r.caja.Pintar(fondo, color.NRGBA{})
	}
	if r.glifo != nil {
		r.glifo.TenirCon(pista)
	}
	r.e.temaEd.escalon = r.e.medida().texto
	r.e.temaEd.inerte = r.e.inerte
	r.e.env.Refresh()
}

// paleta es la del campo según dónde viva: el carril, el cromo claro o la superficie.
func (e *Entrada) paleta() func(tema.Variante, bool, bool, bool) (color.NRGBA, color.NRGBA, color.NRGBA, color.NRGBA) {
	if e.EnCarril {
		return colorCampoCarril
	}
	return coloresDe(e.SobreCromo)
}

// bordePropio es el borde de un campo escrito a mano: su token, fijo, salvo con error.
func (e *Entrada) bordePropio(borde color.NRGBA, v tema.Variante) color.NRGBA {
	if p := e.Propia; p != nil && p.Borde != "" && !e.Invalida {
		return atenuar(tema.Color(p.Borde, v), e.inerte)
	}
	return borde
}

// rellenoPropio es el fondo de un campo escrito a mano: su token, si lo lleva.
func (e *Entrada) rellenoPropio(relleno color.NRGBA, v tema.Variante) color.NRGBA {
	if p := e.Propia; p != nil && p.Relleno != "" {
		return atenuar(tema.Color(p.Relleno, v), e.inerte)
	}
	return relleno
}
