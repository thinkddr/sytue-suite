// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Button.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Variante son las variantes de Button.tsx. El valor cero es la principal, como allí.
type Variante uint8

// Las cuatro variantes.
const (
	Primario Variante = iota
	Secundario
	Fantasma
	Peligro
)

// Talla son los tamaños de Button.tsx. El valor cero es `md`, como allí.
type Talla uint8

// Las cuatro tallas de Button.tsx —`xs` es la del cromo denso del editor— y una quinta
// que NO es de Button.tsx: TallaTexto es el `<button>` de texto que la web escribe a
// mano en los paneles de Calcus (`text-2xs inline-flex h-5 items-center gap-0.5
// rounded-sm px-1`, el «+ Añadir» de las restricciones del Solver en Datos.tsx). Con
// Fantasma y Atenuado lleva sus colores (`text-text-muted hover:bg-bg-subtle
// hover:text-text`). Como no es `Button`, su rótulo es de peso normal, su radio `sm` y
// no tiene el suelo táctil de 44 px.
const (
	TallaMD Talla = iota
	TallaXS
	TallaSM
	TallaLG
	TallaTexto
)

// medida es una fila de `sizes` en Button.tsx, en pasos de la escala de Tailwind
// (`h-9 gap-2 px-4 text-base`), ya convertidos a píxeles enteros.
type medida struct {
	alto, margen float32
	texto        tema.Escalon
	// deTexto es la talla que no es de Button.tsx: rótulo normal, `rounded-sm` y sin
	// suelo táctil (ver TallaTexto).
	deTexto bool
}

var medidas = [...]medida{
	// h-9 px-4 text-base
	TallaMD: {alto: tema.Espacio(9), margen: tema.Espacio(4), texto: tema.TextoBase},
	// h-6 px-2 text-xs, con el `min-h-[24px]` que la hoja le pone encima
	TallaXS: {alto: max(tema.Espacio(6), 24), margen: tema.Espacio(2), texto: tema.TextoXS},
	// h-8 px-3 text-sm
	TallaSM: {alto: tema.Espacio(8), margen: tema.Espacio(3), texto: tema.TextoSM},
	// h-11 px-5 text-md
	TallaLG: {alto: tema.Espacio(11), margen: tema.Espacio(5), texto: tema.TextoMD},
	// h-5 px-1 text-2xs
	TallaTexto: {alto: tema.Espacio(5), margen: tema.Espacio(1), texto: tema.Texto2XS, deTexto: true},
}

// pesoRotulo es PesoRotulo, salvo en la talla de texto, que no es `Button`.
func pesoRotulo(t Talla) Peso {
	if medidas[t].deTexto {
		return Normal
	}
	return PesoRotulo
}

// radioTalla es el `rounded-control` de Button.tsx —4 px en las dos formas—,
// o el `rounded-sm` de la talla de texto, que también mide 4 px.
func radioTalla(t Talla) float32 {
	if medidas[t].deTexto {
		return tema.RadioSM
	}
	return tema.Forma("radius-control")
}

// PesoRotulo es el `font-semibold` del rótulo de un botón. Está fuera de `medidas`
// porque es el mismo en las cuatro tallas.
const PesoRotulo = SemiNegrita

// TamGlifoBoton es el `size` por defecto de `<Icon>` en la web (Icon.tsx, `size = 20`),
// que es lo que mide el glifo de `leadingIcon` cuando quien lo pone no dice otra cosa.
// Un `<Icon name="plus" size={16} />` se pide con `Boton.TamIcono`.
const TamGlifoBoton float32 = 20

// huecoIcono es el `gap-*` de la talla: `gap-1` en `xs` y `gap-2` en las otras tres.
// Va aquí y no en `medidas` porque solo cuenta cuando hay icono, y porque en la web
// también vive en la TALLA y no en la base (el comentario de `sizes` en Button.tsx
// explica por qué: dos `gap-*` en el mismo elemento se resuelven por el orden de la
// hoja generada, no por el de la cadena de clases).
var huecoIcono = [...]float32{
	TallaMD:    tema.Espacio(2),
	TallaXS:    tema.Espacio(1),
	TallaSM:    tema.Espacio(2),
	TallaLG:    tema.Espacio(2),
	TallaTexto: tema.Espacio(0.5),
}

// ladoSoloIcono es `iconSizes` de Button.tsx: el botón se vuelve CUADRADO y pierde su
// relleno horizontal (`h-9 w-9 p-0`). El `min-w-[24px]` de `xs` es el gemelo del
// `min-h-[24px]` que ya lleva `medidas`: un botón alto y estrecho se sigue fallando
// con el dedo, porque el dedo yerra en los dos ejes.
func ladoSoloIcono(t Talla) float32 {
	if t == TallaXS {
		return max(tema.Espacio(6), 24)
	}
	return medidas[t].alto
}

// MedidaBoton es lo que va a ocupar un botón: el rótulo más su margen horizontal, por
// el alto de la talla.
//
// Es pública y es la ÚNICA cuenta: el `MinSize()` del renderer la llama, y el guardián
// compara contra ella y contra los píxeles que salen pintados.
//
// `ancho` fija el ancho (el `fullWidth` de la web); con 0, el que pida el rótulo.
func MedidaBoton(texto string, t Talla, ancho float32) fyne.Size {
	m := medidas[t]
	if ancho <= 0 {
		ancho = Medir(texto, m.texto, pesoRotulo(t)).Width + 2*m.margen
	}
	return fyne.NewSize(ancho, sueloTactilDe(t, m.alto))
}

// sueloTactilDe es SueloTactil para una talla: la de texto no lo lleva.
func sueloTactilDe(t Talla, v float32) float32 {
	if medidas[t].deTexto {
		return v
	}
	return SueloTactil(v)
}

// SueloTactil es el `pointer-coarse:min-h-[max(44px,2.75rem)]` que Button.tsx pone en las
// CUATRO tallas: con el dedo, un botón no baja de 44 px (de alto, y de ancho si es de
// solo icono), porque un blanco de 28 no se acierta con el pulgar. En un teléfono el
// botón de menú de la barra móvil de Notepads mide 44×44 en la web y aquí medía 28×28.
//
// Es pública porque la cinta de la ofimática (`editor`) lo necesita igual: el
// `data-suelo-tactil` del editor web sube a 44 todo lo que hay en ella.
func SueloTactil(v float32) float32 {
	if EsTactil() {
		return max(v, SueloTactilBarra)
	}
	return v
}

func sueloTactil(v float32) float32 { return SueloTactil(v) }

// MedidaBotonCon es MedidaBoton contando los glifos: el `leadingIcon`/`trailingIcon` y
// el `iconOnly` de Button.tsx.
//
//   - Con `soloIcono` el botón es CUADRADO y pierde el relleno horizontal
//     (`iconSizes`: `h-9 w-9 p-0`). El rótulo no se pinta; sigue siendo el nombre
//     accesible, que es lo que la web pone en `aria-label`.
//   - Con glifos y rótulo, cada uno suma su lado más el `gap` de la talla.
//
// `tamIcono` a 0 vale el `size = 20` por defecto de Icon.tsx.
func MedidaBotonCon(texto string, t Talla, ancho float32, glifos int, soloIcono bool, tamIcono float32) fyne.Size {
	if tamIcono <= 0 {
		tamIcono = TamGlifoBoton
	}
	if soloIcono {
		lado := sueloTactilDe(t, ladoSoloIcono(t))
		if ancho > 0 {
			return fyne.NewSize(ancho, lado)
		}
		return fyne.NewSize(lado, lado)
	}
	if ancho > 0 || glifos == 0 {
		return MedidaBoton(texto, t, ancho)
	}
	m := medidas[t]
	contenido := bloqueBoton(Medir(texto, m.texto, pesoRotulo(t)).Width, glifos, tamIcono, huecoIcono[t])
	return fyne.NewSize(contenido+2*m.margen, sueloTactilDe(t, m.alto))
}

// bloqueBoton es lo que ocupan juntos los glifos y el rótulo con el `gap` de la talla
// ENTRE ellos. Un `gap` separa hijos, así que sin rótulo hay uno menos: es el botón
// con el texto en `max-md:sr-only` de la cabecera del lector de Sytue Mail, que en el
// móvil mide 37 (`px-3`, glifo de 16) y no 44. `sr-only` saca el rótulo del flujo con
// `position: absolute`, y el `gap` deja de contarlo.
func bloqueBoton(texto float32, glifos int, lado, hueco float32) float32 {
	piezas := glifos
	if texto > 0 {
		piezas++
	}
	if piezas == 0 {
		return 0
	}
	return texto + float32(glifos)*lado + float32(piezas-1)*hueco
}

// Boton es el botón de la suite.
//
// Es el widget de referencia del paquete: si hay que construir otro, este es el que se
// copia (la forma, no el contenido). Ver el comentario de ui.go.
type Boton struct {
	widget.BaseWidget
	Texto    string
	Variante Variante
	Talla    Talla
	// Ancho fija el ancho del botón (`fullWidth`). Con 0, el que pida el rótulo.
	Ancho float32
	// Icono es el `leadingIcon`: el glifo que va ANTES del rótulo. `iconos.Ninguno`,
	// no hay.
	Icono iconos.Icono
	// IconoDetras es el `trailingIcon`, después del rótulo.
	IconoDetras iconos.Icono
	// Glifo sustituye a Icono cuando el dibujo NO es del catálogo compartido: los
	// ~22 glifos propios de cada app, que en la web tampoco están en `Icon.tsx`
	// (la estrella de Contacts, los ocho del triaje de Mail). Se arma con
	// `iconos.TrazoDeRuta` y gana a `Icono` si los dos están puestos.
	//
	// Es un puntero a un dibujo YA CONSTRUIDO y no una ruta porque el color del
	// glifo puede no ser el del rótulo —la estrella de un destacado va en
	// `warning-text` sobre un botón secundario—, y quien lo construye es quien lo
	// sabe. Por eso el botón no lo tiñe: lo pinta tal y como se lo dan.
	Glifo *iconos.Dibujo
	// SoloIcono es `iconOnly`: la GEOMETRÍA cuadrada (`h-9 w-9 p-0`), no «hay un
	// icono». Es lo mismo que dice su tipo en la web: el rótulo deja de pintarse y
	// pasa a ser solo el nombre accesible.
	SoloIcono bool
	// TamIcono es el `size` del glifo. Con 0, el 20 por defecto de Icon.tsx.
	TamIcono float32
	// MargenIcono es el `mr-1` que una app pone en el glifo de delante, que se SUMA al
	// `gap` de la talla. Solo cuenta con rótulo.
	MargenIcono float32
	// Hueco es el `gap-*` entre glifo y rótulo cuando NO es el de la talla. Con 0, el
	// de la talla. Lo piden los disparadores de menú que la web escribe a mano en vez
	// de usar `Button` —«Nuevo ▾» de Drive es `gap-1.5`—, que miden 1,75 menos.
	Hueco float32
	// Inerte es `disabled`: no responde y se pinta al 50 % de opacidad.
	Inerte bool
	// Cargando es `loading`: la rueda RELEVA al glifo de delante, el botón sale de la
	// interacción y se queda al 75 %. Cargando NO es lo mismo que Inerte, y por eso se
	// atenúa menos: lo dice el propio Button.tsx («cargando ≠ deshabilitado»).
	Cargando bool
	// Atenuado es el fantasma con `text-text-muted hover:text-text` encima (ACTION_BTN de
	// RevisionPanel.tsx, TEXT_BTN de CommentsPanel.tsx): en reposo, rótulo y glifo en
	// `text-muted`; con el puntero o apretado, en `text`. Solo cuenta en Fantasma. Como en
	// la web el glifo es `currentColor`, aquí también se tiñe un `Glifo` puesto a mano.
	Atenuado bool
	// Grupo es la opacidad del contenedor que lo envuelve (ver Grupo). Cero, ninguna.
	Grupo Grupo
	// Rotulo es el `aria-label`: el nombre accesible. Como en el navegador, MANDA sobre
	// el texto escrito («Copiar» se anuncia «Copiar la dirección para Slack»); vacío, se
	// anuncia el texto. Con `SoloIcono` y un `Texto` vacío es lo único que llega al lector
	// de pantalla.
	Rotulo string
	// SobreCromo es el botón principal que va SOBRE EL CARRIL (el menú lateral) y el
	// fantasma que va sobre una banda de cabecera. Lo marca el contenedor, no la app: ver
	// `colores`.
	SobreCromo bool
	// TokenAro es el color del anillo de foco, como en `BotonIcono`. Vacío es
	// `focus-ring`; sobre el carril va `sidebar-focus` —en la web, la cabecera de
	// Sidebar.tsx o `.sytue-focus-ring-chrome` en el botón—, porque el color de foco del tema
	// no se ve sobre el carril. La cabecera lo pone sola (`Lateral.ConCabecera`).
	TokenAro string
	AlPulsar func()
	// AlMover recibe −1 o +1 con las flechas y 0 o el último con Inicio y Fin. Lo pone
	// la BARRA que contiene al botón; suelto, las flechas no son suyas. Es el mismo
	// contrato que `Pestana.AlMover`, que ya lo usaba para el patrón `tablist`.
	AlMover func(delta int, extremo bool)
	// AlEscapar es Escape con el foco dentro de la barra. Sin él, Escape no es suyo.
	AlEscapar func()
	// AlEnfocar se llama al ganar el foco. Lo pone quien lo contiene cuando tiene que
	// enseñarlo —la TiraDeslizable lo desplaza a la vista, como hace el navegador al
	// enfocar un botón que se sale de un `overflow-x-auto`—.
	AlEnfocar   func()
	mostrarFoco func()

	encima, pulsado, enfocado bool
	pista                     *Pista // la ayuda que lo envuelve, para abrirla con el teclado
	// fueraDelRecorrido aparta el botón del TABULADOR sin deshabilitarlo: es el
	// `tabindex="-1"` del «roving tabindex». Ver EnElRecorrido.
	fueraDelRecorrido bool
	// sinAro es el foco SIN `:focus-visible`: se lo dio el código tras un clic (el aspa
	// de un diálogo abierto con el ratón). La primera tecla lo enciende.
	sinAro bool
}

// enfocarSinAro le da el foco sin encender el anillo, que es lo que hace el navegador
// con un `.focus()` cuando la última interacción fue con el puntero.
func (b *Boton) enfocarSinAro(c fyne.Canvas) {
	b.sinAro = true
	enfocar(c, b)
	if c.Focused() != fyne.Focusable(b) {
		b.sinAro = false
	}
}

func (b *Boton) tokenAro() string {
	if b.TokenAro == "" {
		return "focus-ring"
	}
	return b.TokenAro
}

// aroVisible es `:focus-visible`.
func (b *Boton) aroVisible() bool { return b.enfocado && !b.sinAro }

// teclear enciende el anillo que un foco por código dejó apagado: en cuanto se usa el
// teclado, el navegador pasa a pintarlo.
func (b *Boton) teclear() {
	if b.sinAro {
		b.sinAro = false
		b.pista.alFoco(true)
		b.Refresh()
	}
}

var (
	_ fyne.Widget       = (*Boton)(nil)
	_ fyne.Tappable     = (*Boton)(nil)
	_ fyne.Focusable    = (*Boton)(nil)
	_ fyne.Accessible   = (*Boton)(nil)
	_ fyne.Disableable  = (*Boton)(nil)
	_ desktop.Hoverable = (*Boton)(nil)
)

// Enable, Disable y Disabled son `fyne.Disableable`, y el botón lo implementa por DOS
// motivos, los dos del recorrido del tabulador:
//
//  1. Un botón `disabled` no es parada de tabulación en el navegador, y hasta ahora
//     aquí sí lo era: se podía tabular hasta un botón inerte y quedarse ahí.
//  2. Es el ÚNICO gancho que Fyne da para apartar algo del recorrido. El gestor de foco
//     salta los objetos con `Disabled()` cierto al tabular y al pedir el foco por
//     programa (`internal/app/focus_manager.go`, líneas 48 y 114), y eso es exactamente
//     el «roving tabindex» que necesita una barra de herramientas.
//
// Con `fyne.Tabbable` NO se puede: `capturesTab` no le pasa el modificador a
// `TypedKey`, así que dentro de la barra Shift+Tab llegaría igual que Tab y saldría
// hacia delante.
func (b *Boton) Enable() { b.Inerte = false; b.Refresh() }

// Disable apaga el botón.
func (b *Boton) Disable() { b.Inerte = true; b.Refresh() }

// Disabled es si el botón está fuera del recorrido del tabulador, por inerte o por
// apartado.
func (b *Boton) Disabled() bool { return b.inerte() || b.fueraDelRecorrido }

// EnElRecorrido pone o quita el botón del recorrido del tabulador SIN tocar su aspecto
// ni su comportamiento: apartado se sigue pulsando y se sigue enfocando con las
// flechas de su barra. Lo llama la barra; suelto, un botón siempre está dentro.
func (b *Boton) EnElRecorrido(si bool) { b.fueraDelRecorrido = !si }

// NuevoBoton crea un botón principal de talla md.
func NuevoBoton(texto string, alPulsar func()) *Boton {
	b := &Boton{Texto: texto, AlPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

// Con devuelve el mismo botón con otra variante y otra talla. Es azúcar para que una
// pantalla se lea como el JSX del que sale.
func (b *Boton) Con(v Variante, t Talla) *Boton {
	b.Variante, b.Talla = v, t
	return b
}

// colores devuelve relleno, texto y borde de la variante según el estado del puntero.
// Los tokens son los de Button.tsx, uno a uno.
func (b *Boton) colores(v tema.Variante) (relleno, texto, borde color.NRGBA) {
	elige := func(reposo, encima, activo string) color.NRGBA {
		switch {
		case b.pulsado:
			return tema.Color(activo, v)
		case b.encima:
			return tema.Color(encima, v)
		}
		return tema.Color(reposo, v)
	}
	if b.inerte() { // `disabled:` congela el estado de reposo antes de bajar la opacidad
		b.encima, b.pulsado = false, false
	}
	switch b.Variante {
	case Secundario:
		// El ROL primario y su rótulo, como el principal (Button.tsx: `bg-primary
		// text-on-primary` con `border-border-strong`). Fue la escalera 600/700/800 fija
		// con `on-primary` encima: en oscuro, casi negro sobre el 600, 4,1:1.
		relleno = elige("primary", "primary-hover", "primary-active")
		if b.SobreCromo {
			// Dentro del carril el primario es el suyo, también para el secundario: la
			// web reasigna `--color-primary` y su rótulo para todo lo que cuelga de él.
			relleno = elige("sidebar-primary", "sidebar-primary-hover", "sidebar-primary-active")
			return relleno, tema.Color("sidebar-on-primary", v), tokens.SobreSRGB(tokens.Crudo("border-strong", v), relleno)
		}
		return relleno, tema.Color("on-primary", v), tokens.SobreSRGB(tokens.Crudo("border-strong", v), relleno)
	case Fantasma:
		relleno = color.NRGBA{}
		// SOBRE LA TINTA, EL FANTASMA CAMBIA DE VELOS. Es el aspa de un diálogo:
		// `Modal.tsx` le pone `style={{color: header-fg}}` y `hover:bg-chrome-hover
		// active:bg-chrome-active` ENCIMA del `variant="ghost"`, porque el `text-text`
		// y el `hover:bg-bg-subtle` de la variante son los de una superficie clara y
		// sobre la banda de tinta dejan el glifo a 1,1:1.
		if b.SobreCromo {
			if b.encima || b.pulsado {
				relleno = elige("chrome-hover", "chrome-hover", "chrome-active")
			}
			return relleno, tema.Color("header-fg", v), color.NRGBA{}
		}
		tinta := "primary-text"
		if b.encima || b.pulsado {
			relleno = elige("selected", "selected", "selected")
		}
		if b.Atenuado {
			if b.encima || b.pulsado {
				relleno = elige("bg-subtle", "bg-subtle", "border")
			}
			tinta = "text-muted"
			if b.encima || b.pulsado {
				tinta = "text"
			}
		}
		return relleno, tema.Color(tinta, v), color.NRGBA{}
	case Peligro:
		// Blanco (`text-white` en Button.tsx) y no `on-primary`: aquel es el rótulo del
		// relleno primario y en oscuro es casi negro; sobre el rojo daría 3,7:1.
		return elige("danger", "danger-hover", "danger-hover"), tema.Color("neutral-0", v), color.NRGBA{}
	case Primario:
	}
	// SOBRE EL CARRIL, EL BOTÓN PRINCIPAL ES BLANCO con el rótulo del color del carril:
	// uno del color del carril no se distinguiría del fondo. En oscuro vuelve al primario del tema,
	// que ya contrasta con el carril oscuro. Son los `--sytue-sidebar-primary*` que la
	// web reasigna a `--color-primary` para todo lo que cuelga de `bg-sidebar`
	// (appearance.css). Lo pone el MENÚ y no cada app (`ui.Lateral` lo marca en su
	// cabecera y en los principales de su cuerpo): seis apps repitiendo la variante es
	// como se pierde en la séptima.
	if b.SobreCromo {
		return elige("sidebar-primary", "sidebar-primary-hover", "sidebar-primary-active"),
			tema.Color("sidebar-on-primary", v), color.NRGBA{}
	}
	// La acción principal sigue los tokens compartidos de la hoja canónica.
	return elige("primary", "primary-hover", "primary-active"), tema.Color("on-primary", v), color.NRGBA{}
}

// inerte junta `disabled` y `loading`: los dos sacan al botón de la interacción (el
// `const inerte = disabled || loading` de Button.tsx).
func (b *Boton) inerte() bool { return b.Inerte || b.Cargando }

// opacidad es la del grupo entero: `disabled:opacity-50` y, cargando,
// `cursor-wait opacity-75`.
func (b *Boton) opacidad() float32 {
	switch {
	case b.Inerte:
		return 0.5
	case b.Cargando:
		return 0.75
	}
	return 1
}

// ConIcono devuelve el mismo botón con su `leadingIcon`, como hace Con con la variante
// y la talla: azúcar para que la pantalla se lea como el JSX del que sale.
func (b *Boton) ConIcono(i iconos.Icono, tam float32) *Boton {
	b.Icono, b.TamIcono = i, tam
	return b
}

// tamIcono es el lado del glifo con el 20 por defecto de Icon.tsx ya resuelto.
func (b *Boton) tamIcono() float32 {
	if b.TamIcono <= 0 {
		return TamGlifoBoton
	}
	return b.TamIcono
}

// LadoRuedaBoton es el `<Spinner size>` de Button.tsx cuando carga: 20 en `lg`, 12 en
// `xs` y 16 en las demás, lleve el botón icono o no. Medido en el panel de Sytue AI del
// ERP (2026-09-24): «Crear tablero» mide 101,48 en reposo y 124,48 cargando (16 de rueda
// y el `gap-2`); con el glifo por defecto de 20 salía 128,53.
func LadoRuedaBoton(t Talla) float32 {
	switch t {
	case TallaLG:
		return 20
	case TallaXS:
		return 12
	case TallaMD, TallaSM, TallaTexto:
	}
	return 16
}

// delante son los glifos que se pintan y su lado: cargando, solo la rueda —el `!loading
// && trailingIcon` de la web quita también el de detrás— con el lado de su talla.
func (b *Boton) delante() (glifos int, lado float32) {
	if b.Cargando {
		return 1, LadoRuedaBoton(b.Talla)
	}
	return b.glifos(), b.tamIcono()
}

// glifos son los iconos que este botón pinta a los lados del rótulo.
func (b *Boton) glifos() int {
	n := 0
	if b.Cargando || b.Glifo != nil || b.Icono != iconos.Ninguno {
		n++
	}
	if b.IconoDetras != iconos.Ninguno {
		n++
	}
	return n
}

// CreateRenderer construye la geometría. Solo primitivas: dos rectángulos, un texto y,
// si los hay, los glifos —que son `canvas.Raster`, no imágenes escaladas (ver el
// comentario de `iconos/pintar.go`).
func (b *Boton) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, colTexto, borde := b.colores(v)
	m := medidas[b.Talla]
	r := &botonRenderer{
		b:     b,
		aro:   NuevoAroCon(b.tokenAro(), radioTalla(b.Talla), ConHueco, v),
		caja:  NuevaCaja(b.pintura(relleno, v), b.pintura(borde, v), radioTalla(b.Talla)),
		texto: Texto(b.Texto, m.texto, pesoRotulo(b.Talla), b.pintura(colTexto, v)),
	}
	if b.Variante == Fantasma {
		// ponytail: el fantasma no tiene caja en reposo y su aro va abierto (punto 3 de
		// `Aro`); si la variante cambia DESPUÉS de montarlo, el aro se queda como nació.
		r.aro.Abierto()
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	tinta := b.pintura(colTexto, v)
	switch {
	case b.Glifo != nil:
		r.delante, r.tenirDelante = b.Glifo, b.Atenuado
		r.objetos = append(r.objetos, r.delante.Objeto())
	case b.Icono != iconos.Ninguno:
		r.delante, r.tenirDelante = iconos.TrazoConTinta(b.Icono, b.tamIcono(), tinta), true
		r.objetos = append(r.objetos, r.delante.Objeto())
	}
	// Sobre el relleno de un botón, el acento del glifo es su propia tinta, como en la
	// web. Se dice aquí y no se deja al color porque un botón inerte lleva la tinta
	// MEZCLADA con la superficie (`atenuar`), que ya no es reconocible como `on-primary`.
	//
	// ponytail: se fija al montar, como el aro del fantasma; si la variante cambia
	// después entre rellena y fantasma, el acento se queda como nació.
	sobreRelleno := b.Variante != Fantasma
	if r.delante != nil && sobreRelleno {
		r.delante.AcentoTinta()
	}
	r.montarRueda(tinta)
	// Con `iconOnly` el rótulo no se pinta, pero el texto sigue en el árbol como
	// nombre accesible: quitarlo dejaría un botón que el lector anuncia «botón».
	if b.SoloIcono {
		r.texto.Hide()
	}
	r.objetos = append(r.objetos, r.texto)
	if b.IconoDetras != iconos.Ninguno {
		r.detras = iconos.TrazoConTinta(b.IconoDetras, b.tamIcono(), tinta)
		if sobreRelleno {
			r.detras.AcentoTinta()
		}
		r.objetos = append(r.objetos, r.detras.Objeto())
	}
	return r
}

// atenuar es `disabled:opacity-50`, y devuelve un color OPACO ya mezclado con la
// superficie. Lo segundo no es un detalle: es lo que hace que la cuenta salga.
//
// # Por qué no basta con bajar el alfa a la mitad. Medido el 2026-09-21
//
// En la web la opacidad se aplica UNA VEZ sobre el grupo: el control se pinta entero y
// el resultado se compone al 50 % sobre lo que haya detrás. Bajando el alfa color a
// color, cada capa se compone por su cuenta, y donde se apilan dos translúcidas el
// efecto se multiplica en vez de sumarse.
//
// Pasaba en el INTERIOR de todo lo que lleva `Caja`, que son dos rectángulos: el de
// fuera ocupa la caja ENTERA y el relleno va encima. Con los dos al 50 %, el interior
// salía `0,75 × relleno + 0,25 × borde` en vez de relleno a secas. Medido sobre una
// entrada inerte, con el pintor de software a ×2:
//
//	franja del borde   pintado (193,200,203)   web (193,201,204)   error 0 / −1 / −1
//	interior           pintado (224,228,229)   web (255,255,255)   error −31 / −27 / −26
//
// O sea: la franja ya estaba bien —los tokens `border*` llegan aplanados de
// `apps/nucleo/tokens`, así que ahí no había dos translúcidas— y el fallo estaba en el
// relleno, con hasta 31 niveles de más. Tras el cambio los dos salen a 0.
//
// # Contra qué se mezcla
//
// Contra `surface`, que es sobre lo que va un control, y es lo mismo que hace
// `aplanarBordes` en los tokens. Cuando lo de detrás NO es la superficie —un control
// sobre `bg-subtle`— queda un residuo de `0,5 × (surface − fondo real)`, que entre
// `surface` y `bg` de esta paleta son unos cuatro niveles. Es una fracción del defecto
// que corrige, y arreglarlo del todo pediría opacidad de GRUPO, que Fyne no tiene.
//
// Un color ya transparente se devuelve tal cual: mezclarlo daría la superficie opaca,
// y un botón fantasma inerte pasaría a tener fondo.
func atenuar(c color.NRGBA, inerte bool) color.NRGBA {
	if !inerte {
		return c
	}
	return atenuarCon(c, 0.5)
}

// atenuarCon es atenuar con la opacidad del grupo, que en un botón cargando es 0,75 y
// no 0,5. El alfa se TRUNCA, que es lo que hacía la división entera de la primera
// versión: con 0,5 un 255 da 127, no 128.
func atenuarCon(c color.NRGBA, o float32) color.NRGBA {
	if o >= 1 || c.A == 0 {
		return c
	}
	c.A = uint8(float32(c.A) * o)
	return tokens.SobreSRGB(c, tema.Color("surface", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
}

// Grupo es el `opacity-*` de un contenedor que envuelve al control —una tarjeta apagada
// al 80 % (`opacity-80`)—. Fyne no tiene opacidad de grupo, así que cada color del
// control se compone ya con ella sobre lo que hay detrás del grupo (`Fondo`, un token).
// Medido en la portada del ERP: el borde del botón de un área apagada sale (154, 165,
// 170), que es `border-strong` al 80 % sobre `bg`; sobre `surface` saldría tres niveles
// más claro. El valor cero es «sin grupo».
type Grupo struct {
	Opacidad float32
	Fondo    string
}

// Aplicar compone c con la opacidad del grupo sobre su fondo. Un color transparente se
// queda transparente: deja ver lo de detrás, que ya lleva su propia opacidad.
func (g Grupo) Aplicar(c color.NRGBA, v tema.Variante) color.NRGBA {
	if g.Opacidad <= 0 || g.Opacidad >= 1 || c.A == 0 {
		return c
	}
	c.A = uint8(float32(c.A)*g.Opacidad + 0.5)
	return tokens.SobreSRGB(c, tema.Color(g.Fondo, v))
}

// pintura es un color del botón con su opacidad propia y la de su grupo.
func (b *Boton) pintura(c color.NRGBA, v tema.Variante) color.NRGBA {
	return b.Grupo.Aplicar(atenuarCon(c, b.opacidad()), v)
}

// Tapped es `onClick`.
func (b *Boton) Tapped(*fyne.PointEvent) {
	if b.inerte() || b.AlPulsar == nil {
		return
	}
	b.AlPulsar()
}

// MouseIn, MouseMoved y MouseOut son `:hover`.
func (b *Boton) MouseIn(*desktop.MouseEvent) {
	if b.inerte() {
		return
	}
	b.encima = true
	b.pista.alPuntero(true)
	b.Refresh()
}

// MouseMoved no hace nada: el estado ya lo fijan MouseIn y MouseOut.
func (b *Boton) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve el botón a su color de reposo.
func (b *Boton) MouseOut() {
	if !b.encima {
		return
	}
	b.encima = false
	b.pista.alPuntero(false)
	b.Refresh()
}

// MouseDown y MouseUp son `:active`.
func (b *Boton) MouseDown(*desktop.MouseEvent) {
	if b.inerte() {
		return
	}
	b.pulsado = true
	b.Refresh()
}

func (b *Boton) MouseUp(*desktop.MouseEvent) {
	if !b.pulsado {
		return
	}
	b.pulsado = false
	b.Refresh()
}

type botonRenderer struct {
	b               *Boton
	aro             *Aro
	caja            *Caja
	texto           *canvas.Text
	delante, detras *iconos.Dibujo
	// tenirDelante es falso cuando el glifo lo trae la app con su propio color: un
	// `Glifo` puesto a mano no se repinta con la tinta del rótulo.
	tenirDelante bool
	// rueda se monta la primera vez que el botón carga, y ya no se tira: ver
	// `montarRueda`.
	rueda *Rueda
	// ultimo es lo último que se colocó, para recolocar cuando aparece la rueda.
	ultimo  fyne.Size
	objetos []fyne.CanvasObject
}

// MinSize es la medida declarada. No hay otra cuenta: si el botón ocupa algo distinto,
// es que Layout o el pintor se han desviado, y eso es lo que mira el guardián.
func (r *botonRenderer) MinSize() fyne.Size { return MedidaBotonDe(r.b) }

// MedidaBotonDe es la medida de UN botón ya montado: la de MedidaBotonCon, con su
// Hueco si lo trae y, si es secundario, su borde.
//
// El secundario es la única variante con `border` en Button.tsx, y con
// `box-sizing: border-box` el borde va FUERA del `px-*`: medido en el lector de Sytue
// Mail, «Responder a todos» mide 142,70 como secundario donde el mismo contenido en
// fantasma mediría 140,70, y el de solo glifo en el móvil, 39 y no 37. El alto no
// cambia (`h-8` ya lo contiene) ni el cuadrado de `iconOnly` (`w-8` también), ni un
// ancho fijado (`fullWidth`), que ya es la caja entera.
func MedidaBotonDe(b *Boton) fyne.Size {
	glifos, lado := b.delante()
	m := MedidaBotonCon(b.Texto, b.Talla, b.Ancho, glifos, b.SoloIcono, lado)
	if b.Hueco > 0 && b.Ancho <= 0 && !b.SoloIcono {
		m.Width -= float32(glifos) * (huecoIcono[b.Talla] - b.Hueco)
	}
	if b.Variante == Secundario && !b.SoloIcono && b.Ancho <= 0 {
		m.Width += 2 * tema.Borde
	}
	// El `mr-1` que algunas apps ponen en el glifo de delante se suma al `gap`: solo
	// con rótulo detrás, y no con `iconOnly` ni con un ancho fijado.
	if b.MargenIcono > 0 && b.Texto != "" && glifos > 0 && !b.SoloIcono && b.Ancho <= 0 {
		m.Width += b.MargenIcono
	}
	return m
}

func (r *botonRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *botonRenderer) Destroy() {}

// montarRueda crea la rueda la PRIMERA vez que el botón se pone a cargar, la enseña o
// la esconde según toque, y esconde el glifo al que releva.
//
// No se monta siempre: su animación es infinita y arrancarla en todos los botones de la
// suite sería un hilo por botón. Y una vez creada no se tira: un botón que carga dos
// veces no debe arrancar dos animaciones ni rasterizar de nuevo.
//
// Devuelve si la lista de objetos ha cambiado, porque entonces hay que recolocar: una
// pieza que aparece después de montar el widget no la coloca nadie —`Layout` solo corre
// al redimensionar— y se pinta en la esquina.
func (r *botonRenderer) montarRueda(tinta color.NRGBA) bool {
	if !r.b.Cargando && r.rueda == nil {
		return false
	}
	nueva := false
	if r.rueda == nil {
		r.rueda = NuevaRuedaCon(r.b.tamIcono(), tinta)
		r.rueda.Rotulo = r.b.AccessibilityLabel()
		r.objetos = append(r.objetos, r.rueda)
		nueva = true
	}
	if r.b.Cargando {
		r.rueda.Show()
	} else {
		r.rueda.Hide()
		r.rueda.Parar()
	}
	// El `leadingIcon` no se pinta mientras carga: el ternario del JSX lo releva.
	if r.delante != nil {
		r.delante.Ver(!r.b.Cargando)
	}
	// Y el de detrás tampoco (`{!loading && trailingIcon}`).
	if r.detras != nil {
		r.detras.Ver(!r.b.Cargando)
	}
	return nueva
}

// Layout coloca la caja a tamaño completo y el rótulo centrado en su caja de línea.
//
// El centrado se hace sobre la caja de LÍNEA y no sobre la de la fuente: si no, el
// rótulo sube un par de píxeles respecto a la web, y en una botonera se ve.
func (r *botonRenderer) Layout(t fyne.Size) {
	r.ultimo = t
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	glifos, lado := r.b.delante()
	if r.b.SoloIcono {
		// `p-0`: el glifo va centrado en el cuadrado y no hay rótulo que colocar.
		if r.delante != nil {
			r.delante.Colocar((t.Width-lado)/2, (t.Height-lado)/2, lado)
		}
		r.colocarRueda((t.Width-lado)/2, (t.Height-lado)/2, lado)
		return
	}
	e := medidas[r.b.Talla].texto
	hueco := huecoIcono[r.b.Talla]
	if r.b.Hueco > 0 {
		hueco = r.b.Hueco
	}
	ancho := Medir(r.b.Texto, e, pesoRotulo(r.b.Talla)).Width
	// `justify-center`: el bloque entero (glifos + hueco + rótulo) va centrado.
	bloque := bloqueBoton(ancho, glifos, lado, hueco)
	margen := float32(0)
	if r.b.Texto != "" && (r.delante != nil || r.rueda != nil) {
		margen = r.b.MargenIcono
		bloque += margen
	}
	x := (t.Width - bloque) / 2
	if r.delante != nil || r.rueda != nil {
		if r.delante != nil {
			r.delante.Colocar(x, (t.Height-lado)/2, lado)
		}
		r.colocarRueda(x, (t.Height-lado)/2, lado)
		x += lado + hueco + margen
	}
	ColocarTexto(r.texto, e, x, (t.Height-e.Linea)/2)
	if r.detras != nil {
		if ancho > 0 {
			x += ancho + hueco // sin rótulo, el hueco de delante ya es el único
		}
		r.detras.Colocar(x, (t.Height-lado)/2, lado)
	}
}

// colocarRueda la pone donde iría el glifo de delante, que es a quien releva.
func (r *botonRenderer) colocarRueda(x, y, lado float32) {
	if r.rueda == nil {
		return
	}
	r.rueda.Move(fyne.NewPos(x, y))
	r.rueda.Resize(fyne.NewSize(lado, lado))
}

// Refresh repinta con los colores del estado actual sin reconstruir la geometría.
func (r *botonRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, colTexto, borde := r.b.colores(v)
	tinta := r.b.pintura(colTexto, v)
	// El radio es de la forma, que puede cambiar con la app abierta.
	r.caja.CambiarRadio(radioTalla(r.b.Talla))
	r.aro.CambiarRadio(radioTalla(r.b.Talla))
	r.caja.Pintar(r.b.pintura(relleno, v), r.b.pintura(borde, v))
	r.aro.token = r.b.tokenAro() // el menú puede marcarlo después de montarlo
	r.aro.Pintar(v)
	r.aro.Ver(r.b.aroVisible())
	r.texto.Text = r.b.Texto
	r.texto.Color = tinta
	r.texto.TextSize = medidas[r.b.Talla].texto.Tam
	r.texto.Refresh()
	// El color del glifo va DENTRO de su SVG, así que cambiarlo es `TenirCon`: el
	// `Dibujo` solo rehace su mapa de bits si el color ha cambiado de verdad.
	if r.delante != nil && r.tenirDelante {
		r.delante.TenirCon(tinta)
	}
	if r.detras != nil {
		r.detras.TenirCon(tinta)
	}
	if r.rueda != nil {
		r.rueda.Tenir(tinta)
	}
	if r.montarRueda(tinta) && r.ultimo.Width > 0 {
		r.Layout(r.ultimo) // la rueda acaba de aparecer y nadie la ha colocado
	}
}

// ---- foco ----

// El botón es enfocable: con el tabulador se llega a él y con Intro o la barra
// espaciadora se pulsa, que es lo que hace un `<button>`.
//
// Su aro es el `.sytue-focus-ring-inset` de la hoja —un pelo de la superficie y luego
// el anillo—, y no el pegado: sobre el relleno de un botón principal, un anillo sin
// hueco se funde con el propio relleno y deja de leerse.

// FocusGained enciende el aro.
func (b *Boton) FocusGained() {
	b.enfocado = true
	b.pista.alFoco(!b.sinAro)
	b.Refresh()
	if b.AlEnfocar != nil {
		b.AlEnfocar()
	}
	if b.mostrarFoco != nil {
		b.mostrarFoco()
	}
}

func (b *Boton) revelarAlFoco(f func()) { b.mostrarFoco = f }

// boton es el Boton que lleva dentro cualquier tipo que lo incruste (el `botonTriaje`
// de Mail): así la TiraDeslizable le engancha AlEnfocar sin conocer el tipo.
func (b *Boton) boton() *Boton { return b }

// FocusLost lo apaga.
func (b *Boton) FocusLost() {
	b.enfocado, b.sinAro = false, false
	b.pista.alFoco(false)
	b.Refresh()
}

func (b *Boton) ponerPista(p *Pista) { b.pista = p }

// TypedRune pulsa con la barra espaciadora.
func (b *Boton) TypedRune(r rune) {
	b.teclear()
	if r == ' ' {
		b.Tapped(nil)
	}
}

// TypedKey pulsa con Intro.
// Las flechas, Inicio, Fin y Escape solo hacen algo si una barra los ha pedido; sueltas
// no son del botón, que es lo que hace el navegador con un `<button>` a secas.
//
//nolint:exhaustive // el linter quiere las ochenta teclas y aquí hay siete con sentido.
func (b *Boton) TypedKey(e *fyne.KeyEvent) {
	b.teclear()
	switch e.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		b.Tapped(nil)
	case fyne.KeyRight, fyne.KeyDown:
		b.mover(1, false)
	case fyne.KeyLeft, fyne.KeyUp:
		b.mover(-1, false)
	case fyne.KeyHome:
		b.mover(0, true)
	case fyne.KeyEnd:
		b.mover(1, true)
	case fyne.KeyEscape, mobile.KeyBack:
		b.pista.alEscape()
		if b.AlEscapar != nil {
			b.AlEscapar()
			return
		}
		SubirEscape(b, e)
	}
}

func (b *Boton) mover(delta int, extremo bool) {
	if b.AlMover != nil {
		b.AlMover(delta, extremo)
	}
}

// AccessibilityLabel es el `aria-label` si lo hay, y si no el texto escrito. Un botón de
// solo icono sin `Rotulo` es invisible para un lector de pantalla, que es el fallo que
// Button.tsx evita pidiéndolo en el tipo.
func (b *Boton) AccessibilityLabel() string {
	if b.Rotulo != "" {
		return b.Rotulo
	}
	return b.Texto
}

// AccessibilityRole es «button», que aquí sí existe tal cual en Fyne.
func (b *Boton) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }
