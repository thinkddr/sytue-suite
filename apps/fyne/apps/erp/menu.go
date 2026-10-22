// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El menú del ERP (`BarraLateral` de ErpApp.tsx)
//
// NO es `ui.Lateral`, y la diferencia no es de estilo: el de la suite lleva `px-4` y
// sus filas 33,25 con hueco para un icono; el del ERP lleva `px-3` y sus entradas
// **29,75**, sin icono. Con treinta y tres áreas encendidas esos 3,5 px por fila son
// media pantalla. Medido en `facturas.ancho.json` el 2026-09-22:
//
//	nav             x 0 → 260, padding 14 / 10,5      (medido a 210; ver abajo)
//	«Ir a…»         x 10,5, h 31,75                   (medido)
//	«Sytue AI»      h 28 (`h-8`), mb-4 = 14           (medido por la posición de Inicio)
//	entrada         h 29,75 (`text-base` + `py-1.5`)  (medido a 26,25; ver abajo)
//	cabecera de área h 24,5                           (medido, `repeticiones`)
//
// El ANCHO y la LETRA sí son ya los de la suite, desde el PR #836 de la web: el menú
// medía 210 (`w-60`) y sus entradas iban en `text-sm` (11,4), contra los 260 y los
// 12,25 de todas las demás apps. Ahora es `--spacing-sidebar` y `text-base`, y lo que se
// queda es su densidad.
//
// Y una cosa que solo se ve midiendo: el fondo del carril sale con el token
// `--color-header` (26, 53, 65): la hoja apunta `--sytue-sidebar-bg` al mismo sitio.
//
// Desde que el ERP lleva carril de iconos (carril.go), este menú es un CAJÓN a cualquier
// ancho, y app.css le pone por su id lo que pide un cajón: `padding: 1.25rem 1rem` en
// vez del `px-3 py-4` de su clase, y `min-height: 2.75rem` en sus botones y entradas
// (`#erp-cajon > button` y `#erp-cajon li > button`).
//

// Medidas del cajón. Todas de HOJA salvo donde dice lo contrario.
const (
	// AnchoMenu es `--spacing-sidebar`, el de todas las apps (`ui.AnchoMenu`).
	AnchoMenu float32 = tema.AnchoLateral
	// MenuPX y MenuPY son el `padding: 1.25rem 1rem` de `#erp-cajon` en app.css, que
	// gana al `px-3 py-4` de la clase.
	MenuPX float32 = 1 * tema.Rem
	MenuPY float32 = 1.25 * tema.Rem
	// AltoMinimoCajon es el `min-height: 2.75rem` de los botones y las entradas del
	// cajón: la diana táctil, que app.css no acota a ningún ancho.
	AltoMinimoCajon float32 = 2.75 * tema.Rem
	// AnchoCerrarCajon es el `w-9` del aspa (`Button` fantasma de solo icono) y
	// CerrarCajonMB su `mb-3`.
	AnchoCerrarCajon float32 = 9 * 0.25 * tema.Rem
	CerrarCajonMB    float32 = 3 * 0.25 * tema.Rem
	// EntradaPX es `px-3` y EntradaPY el `py-1.5` de una entrada.
	EntradaPX float32 = 3 * 0.25 * tema.Rem
	EntradaPY float32 = 1.5 * 0.25 * tema.Rem
	// AnchoIndicador es el tope de la barra de «estás aquí» (el `2px` de
	// `before:w-[min(2px,var(--sytue-nav-indicator))]`), y MargenIndicador su
	// `before:top-1.5 before:bottom-1.5`.
	AnchoIndicador  float32 = 2
	MargenIndicador float32 = 1.5 * 0.25 * tema.Rem
	// AreaPY es el `py-1.5` de la cabecera de un área, sobre la caja de `text-xs` (14).
	AreaPY float32 = 1.5 * 0.25 * tema.Rem
	// LadoGalonArea es el `›` que gira al abrir. En Fyne es el chevron del catálogo, que
	// es un icono y no un carácter: el `›` de la web es texto y su tamaño depende de la
	// fuente, cosa que aquí no hace falta heredar.
	LadoGalonArea float32 = 14
)

// AltoEntrada es lo que ocupa una entrada del menú: la caja de `text-base` más su
// `py-1.5`, con el mínimo del cajón, que es el que manda: 44.
var AltoEntrada = max(tema.TextoBase.Linea+2*EntradaPY, AltoMinimoCajon)

// anchoIndicador es la barra de «estás aquí» de la forma en vigor: en Marco no hay
// (`--sytue-nav-indicator` es 0 y lo dicen el velo y la negrita); en Carril, 2.
func anchoIndicador() float32 { return min(AnchoIndicador, tema.Forma("nav-indicator")) }

// AltoCabeceraArea es lo que ocupa el nombre de un área: la caja de `.sytue-overline`
// (`text-xs`, caja de 14) más su `py-1.5`. Medido: 24,5.
var AltoCabeceraArea = tema.TextoXS.Linea + 2*AreaPY

// Entrada es una fila del menú: un destino, y nada más. Todas se pintan igual —activa o
// no—, que es la primera de las tres decisiones de interfaz del ERP.
//
// Es un widget y no un contenedor porque se pulsa, se enfoca y se anuncia: la regla de
// `ui.go` es elegir por comportamiento.
type Entrada struct {
	ui.Pulsable

	// Rotulo es el nombre del destino, ya traducido.
	Rotulo string
	// Activa es `aria-current="page"`: lleva el fondo del ítem activo y la barra de la
	// izquierda.
	Activa bool
	// Contador es la insignia de la derecha (las tareas de hoy). Cero: no hay insignia.
	Contador int
}

var _ fyne.Widget = (*Entrada)(nil)

// NuevaEntrada crea una fila del menú.
func NuevaEntrada(rotulo string, activa bool, alPulsar func()) *Entrada {
	e := &Entrada{Rotulo: rotulo, Activa: activa}
	e.Nombre, e.AlPulsar = rotulo, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityLabel es el rótulo, o el nombre con la insignia si lo tiene.
func (e *Entrada) AccessibilityLabel() string {
	if e.Nombre != "" {
		return e.Nombre
	}
	return e.Rotulo
}

func (e *Entrada) peso() ui.Peso {
	if e.Activa {
		return ui.PesoNavActivo() // `[font-weight:var(--sytue-nav-active-weight)]`
	}
	return ui.Normal
}

func (e *Entrada) insignia() string {
	if e.Contador <= 0 {
		return ""
	}
	return contar(e.Contador)
}

// MedidaEntrada es el rótulo con su `px-3` por los 26,25 de alto. El ancho de verdad lo
// pone el carril (`w-full`); esto es el mínimo.
func MedidaEntrada(e *Entrada) fyne.Size {
	ancho := ui.Medir(e.Rotulo, tema.TextoBase, e.peso()).Width + 2*EntradaPX
	if t := e.insignia(); t != "" {
		ancho += tema.Espacio(2) + ui.MedidaInsignia(t).Width
	}
	return fyne.NewSize(ancho, AltoEntrada)
}

// CreateRenderer construye fondo, barra de «estás aquí», rótulo e insignia.
func (e *Entrada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &entradaRenderer{
		e: e,
		// El aro del menú es el BLANCO (`chrome-focus`) y no el azul: sobre la tinta del
		// carril el azul no llega a 3:1 contra el ítem activo (ver `ui.NuevoAroCon`).
		// `rounded-nav`: el radio del ítem es de la forma.
		aro:   ui.NuevoAroCon("sidebar-focus", tema.Forma("radius-nav"), ui.SinHueco, v).Abierto(),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-nav")),
		barra: ui.NuevaCaja(tema.Color("sidebar-indicator", v), color.NRGBA{}, AnchoIndicador/2),
		texto: ui.Texto(e.Rotulo, tema.TextoBase, e.peso(), tema.Color("sidebar-fg", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.barra.Objetos()...)
	r.objetos = append(r.objetos, r.texto)
	if t := e.insignia(); t != "" {
		r.marca = ui.NuevaInsignia(t, ui.InsigniaAviso)
		r.objetos = append(r.objetos, r.marca)
	}
	// Regla 7 de `ui.go`: Fyne no llama al Refresh del renderer antes del primer
	// pintado, así que un renderer que nazca a medias sale invisible.
	r.Refresh()
	return r
}

type entradaRenderer struct {
	e           *Entrada
	aro         *ui.Aro
	caja, barra *ui.Caja
	texto       *canvas.Text
	marca       *ui.Insignia
	objetos     []fyne.CanvasObject
}

func (r *entradaRenderer) MinSize() fyne.Size           { return MedidaEntrada(r.e) }
func (r *entradaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *entradaRenderer) Destroy()                     {}

func (r *entradaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ui.MoverCaja(r.barra, fyne.NewPos(0, MargenIndicador),
		fyne.NewSize(anchoIndicador(), max(t.Height-2*MargenIndicador, 0)))

	cabe := t.Width - 2*EntradaPX
	if r.marca != nil {
		m := ui.MedidaInsignia(r.marca.Texto)
		r.marca.Resize(m)
		r.marca.Move(fyne.NewPos(t.Width-EntradaPX-m.Width, (t.Height-m.Height)/2))
		cabe -= m.Width + tema.Espacio(2)
	}
	ui.Truncar(r.texto, r.e.Rotulo, tema.TextoBase, r.e.peso(), cabe)
	ui.ColocarTexto(r.texto, tema.TextoBase, EntradaPX, (t.Height-tema.TextoBase.Linea)/2)
}

// Refresh pinta los tres estados del ítem: activo, con el ratón encima, y en reposo.
func (r *entradaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("sidebar-fg", v)
	switch {
	case r.e.Activa:
		fondo, tinta = tema.Color("sidebar-active-bg", v), tema.Color("sidebar-active-fg", v)
	case r.e.Encima():
		fondo = tema.Color("sidebar-hover-bg", v)
	}
	radio := tema.Forma("radius-nav") // la forma se cambia en caliente
	r.aro.CambiarRadio(radio)
	r.aro.Ver(r.e.Enfocado())
	r.aro.Pintar(v)
	r.caja.CambiarRadio(radio)
	r.caja.Pintar(fondo, color.NRGBA{})
	// La barra de «estás aquí» solo existe en la entrada activa: es el `before:` del
	// ramo activo y no una decoración permanente.
	r.barra.Pintar(barraActiva(r.e.Activa, v), color.NRGBA{})
	r.texto.Color, r.texto.TextSize = tinta, tema.TextoBase.Tam
	r.texto.FontSource = r.e.peso().Cara()
	r.texto.Refresh()
	if r.marca != nil {
		r.marca.Refresh()
	}
}

// Sin ancho tampoco tiene color: en Marco mide 0, y el pintor de GL le dejaba a un
// rectángulo de ancho 0 una raya de dos píxeles (visto en ventana real, 2026-10-05).
func barraActiva(activa bool, v tema.Variante) color.NRGBA {
	if !activa || anchoIndicador() == 0 {
		return color.NRGBA{}
	}
	return tema.Color("sidebar-indicator", v)
}

// CabeceraArea es el `<summary>` de un área: su nombre en versalitas y el galón que
// gira. Pliega y despliega sus modelos.
//
// Es un widget porque `<details>` nativo es pulsable, enfocable y anunciable, que es
// justo lo que la web dice que gana usándolo.
type CabeceraArea struct {
	ui.Pulsable

	// Rotulo es el nombre del área, ya traducido por el servicio.
	Rotulo string
	// Abierta es el `open` del `<details>`.
	Abierta bool
}

var _ fyne.Widget = (*CabeceraArea)(nil)

// NuevaCabeceraArea crea el encabezado de un área.
func NuevaCabeceraArea(rotulo string, abierta bool, alPulsar func()) *CabeceraArea {
	c := &CabeceraArea{Rotulo: rotulo, Abierta: abierta}
	c.Nombre, c.AlPulsar = rotulo, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el nombre del área.
func (c *CabeceraArea) AccessibilityLabel() string { return c.Rotulo }

// MedidaCabeceraArea es el nombre en versalitas más el galón, por los 24,5 de alto.
func MedidaCabeceraArea(c *CabeceraArea) fyne.Size {
	ancho := ui.MedirVersal(c.Rotulo, tema.TextoXS, ui.SemiNegrita) + 2*EntradaPX +
		tema.Espacio(2) + LadoGalonArea
	return fyne.NewSize(ancho, AltoCabeceraArea)
}

// CreateRenderer construye fondo, versalitas y galón.
func (c *CabeceraArea) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraAreaRenderer{
		c:      c,
		aro:    ui.NuevoAroCon("sidebar-focus", tema.RadioMD, ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		rotulo: ui.NuevoVersal(c.Rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color("sidebar-fg-muted", v)),
	}
	r.galon = iconos.Trazo(galonDeArea(c.Abierta), LadoGalonArea, "sidebar-fg-muted", v).Objeto()
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	r.objetos = append(r.objetos, r.galon)
	r.Refresh()
	return r
}

// galonDeArea: el `›` de la web gira 90° al abrir (`group-open:rotate-90`), así que el
// dibujo de abierta es el chevron hacia abajo. Rotar un raster aquí sería dibujar dos
// veces lo que el catálogo ya tiene dibujado bien.
func galonDeArea(abierta bool) iconos.Icono {
	if abierta {
		return iconos.ChevronAbajo
	}
	return iconos.ChevronDerecha
}

type cabeceraAreaRenderer struct {
	c       *CabeceraArea
	aro     *ui.Aro
	caja    *ui.Caja
	rotulo  *ui.Versal
	galon   fyne.CanvasObject
	objetos []fyne.CanvasObject
}

func (r *cabeceraAreaRenderer) MinSize() fyne.Size           { return MedidaCabeceraArea(r.c) }
func (r *cabeceraAreaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraAreaRenderer) Destroy()                     {}

func (r *cabeceraAreaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.rotulo.Colocar(EntradaPX, (t.Height-tema.TextoXS.Linea)/2)
	r.galon.Resize(fyne.NewSize(LadoGalonArea, LadoGalonArea))
	r.galon.Move(fyne.NewPos(t.Width-EntradaPX-LadoGalonArea, (t.Height-LadoGalonArea)/2))
}

func (r *cabeceraAreaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := tema.Color("sidebar-fg-muted", v)
	if r.c.Encima() { // hover:text-sidebar-fg
		tinta = tema.Color("sidebar-fg", v)
	}
	r.aro.Ver(r.c.Enfocado())
	r.aro.Pintar(v)
	r.rotulo.Pintar(tinta)
	// El galón cambia de DIBUJO al abrir, así que se vuelve a pedir: pintarlo girado
	// pediría rotar un raster, y el catálogo ya trae los dos.
	r.galon = iconos.Trazo(galonDeArea(r.c.Abierta), LadoGalonArea, "sidebar-fg-muted", v).Objeto()
	r.objetos[len(r.objetos)-1] = r.galon
	// El galón es un objeto NUEVO: sin colocarlo se pinta en (0, 0), encima del nombre
	// del área, en cuanto el menú se repinta con el área cerrada.
	r.Layout(r.c.Size())
}

// Menu es el carril entero: es ESTADO, no vista. Quien lo usa rellena sus campos y pide
// el objeto; nada de aquí habla con el servicio.
type Menu struct {
	// Areas son los módulos que se enseñan: SOLO los que la organización ha encendido y
	// en los que esta persona tiene lectura. Filtrarlos es de quien monta el menú, no de
	// aquí: enseñar un área que el servicio va a responder con un 403 es prometer algo
	// que no se puede cumplir.
	Areas []Modulo
	// Fijas son las entradas de arriba (Inicio, Tareas, Banco…), ya filtradas por
	// permiso.
	Fijas []Destino
	// Tableros son los guardados de Sytue AI, con su encabezado propio.
	Tableros []Destino
	// ModeloActivo es la clave del modelo abierto: marca su entrada y abre su área.
	ModeloActivo string
	// Plegadas son las áreas que la persona ha cerrado a mano. Por defecto está abierta
	// solo la del modelo abierto, igual que el `open` del `<details>`.
	Plegadas map[string]bool

	// AlElegir abre un modelo; AlPlegar abre o cierra un área.
	AlElegir func(Modelo)
	AlPlegar func(clave string)
	// AlPaleta y AlIA son los dos botones de arriba.
	AlPaleta, AlIA func()
	// AlCerrar es el aspa de arriba y RotuloCerrar su `aria-label` («Cerrar»).
	AlCerrar     func()
	RotuloCerrar string

	// RotuloPaleta, AtajoPaleta y NombrePaleta son «Ir a…», «⌘K» y el nombre accesible.
	RotuloPaleta, AtajoPaleta, NombrePaleta string
	// RotuloIA es la marca del panel de IA. No se traduce: es un nombre de producto.
	RotuloIA string
	// TituloTableros es el encabezado de la sección de tableros.
	TituloTableros string
}

// Destino es una entrada que no sale del catálogo (Inicio, Tareas, un tablero).
type Destino struct {
	Rotulo string
	// Nombre es el accesible cuando no basta el rótulo: «Tareas — 3 tareas para hoy»,
	// que es el `aria-label` de la insignia de la web.
	Nombre string
	Activa bool
	// Contador es la insignia de la derecha. Cero: no hay.
	Contador int
	// Icono es el del carril de iconos; `iconos.Ninguno`, el destino no va en él.
	Icono    iconos.Icono
	AlPulsar func()
}

// abierta dice si un área se pinta desplegada: la del modelo abierto, salvo que la
// persona la haya cerrado a mano.
func (m *Menu) abierta(a Modulo) bool {
	if m.Plegadas[a.Clave] {
		return false
	}
	for _, x := range a.Modelos {
		if x.Clave == m.ModeloActivo {
			return true
		}
	}
	return false
}

// Objeto arma el carril con todas sus filas.
func (m *Menu) Objeto(v tema.Variante) fyne.CanvasObject {
	l := &carril{}
	hijos := make([]fyne.CanvasObject, 0, 2+len(m.Fijas)+len(m.Tableros)+4*len(m.Areas))
	hijos = append(hijos, canvas.NewRectangle(tema.Color("sidebar-bg", v)), filete(v))

	// El aspa: `<Button variant="ghost" iconOnly class="mb-3 ml-auto flex">`, a la
	// derecha. Dentro del carril el fantasma pinta con sus tokens (`primary-text` y
	// `selected` reasignados por `bg-sidebar`).
	cerrar := ui.NuevoBotonIcono(iconos.Cerrar, m.RotuloCerrar, m.AlCerrar)
	cerrar.Lado, cerrar.TamIcono, cerrar.Radio = AnchoCerrarCajon, 20, tema.Forma("radius-control")
	cerrar.Trazo, cerrar.TrazoEncima, cerrar.TokenAro = "sidebar-fg", "sidebar-fg", "sidebar-focus"
	cerrar.FondoEncima, cerrar.FondoActivo, cerrar.FondoPulsado = "sidebar-active-bg", "sidebar-active-bg", ""
	l.arriba = append(l.arriba, bloque{container.New(aLaDerecha{AnchoCerrarCajon}, cerrar), AltoMinimoCajon, CerrarCajonMB})

	paleta := NuevoBotonPaleta(m.RotuloPaleta, m.AtajoPaleta, m.NombrePaleta, m.AlPaleta)
	l.arriba = append(l.arriba, bloque{paleta, AltoBotonPaleta, tema.Espacio(4)})
	ia := NuevoBotonIA(m.RotuloIA, m.AlIA)
	l.arriba = append(l.arriba, bloque{ia, AltoBotonIA, tema.Espacio(4)})

	for _, d := range m.Fijas {
		l.arriba = append(l.arriba, bloque{entradaDe(d), AltoEntrada, 0})
	}
	if n := len(l.arriba); n > 0 {
		l.arriba[n-1].abajo = tema.Espacio(5) // el `mb-5` del `<ul>`
	}

	if len(m.Tableros) > 0 {
		l.arriba = append(l.arriba, bloque{
			cabeceraSeccion(m.TituloTableros, v), AltoCabeceraArea, 0,
		})
		for _, d := range m.Tableros {
			l.arriba = append(l.arriba, bloque{entradaDe(d), AltoEntrada, 0})
		}
		l.arriba[len(l.arriba)-1].abajo = tema.Espacio(5)
	}

	for _, a := range m.Areas {
		area := a
		abierta := m.abierta(area)
		cab := NuevaCabeceraArea(area.Etiqueta, abierta, func() {
			if m.AlPlegar != nil {
				m.AlPlegar(area.Clave)
			}
		})
		l.arriba = append(l.arriba, bloque{cab, AltoCabeceraArea, 0})
		if !abierta {
			// `mb-1` entre áreas cerradas.
			l.arriba[len(l.arriba)-1].abajo = tema.Espacio(1)
			continue
		}
		for _, x := range area.Modelos {
			modelo := x
			e := NuevaEntrada(modelo.Etiqueta, modelo.Clave == m.ModeloActivo, func() {
				if m.AlElegir != nil {
					m.AlElegir(modelo)
				}
			})
			l.arriba = append(l.arriba, bloque{e, AltoEntrada, 0})
		}
		// `ul.mb-2` dentro del área y `mb-1` del propio `<details>`.
		l.arriba[len(l.arriba)-1].abajo = tema.Espacio(2) + tema.Espacio(1)
	}

	for _, b := range l.arriba {
		hijos = append(hijos, b.obj)
	}
	return container.New(l, hijos...)
}

func entradaDe(d Destino) *Entrada {
	e := NuevaEntrada(d.Rotulo, d.Activa, d.AlPulsar)
	e.Contador = d.Contador
	if d.Nombre != "" && d.Contador > 0 {
		e.Nombre = d.Nombre
	}
	return e
}

// cabeceraSeccion es el `<p class="sytue-overline">` que encabeza los tableros: mismas
// versalitas que un área, pero sin galón porque no se pliega.
func cabeceraSeccion(titulo string, v tema.Variante) fyne.CanvasObject {
	r := ui.NuevoVersal(titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("sidebar-fg-muted", v))
	return container.New(&versalEnFila{r}, r.Objetos()...)
}

type versalEnFila struct{ v *ui.Versal }

func (f *versalEnFila) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.v.Ancho()+2*EntradaPX, AltoCabeceraArea)
}

func (f *versalEnFila) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.v.Colocar(EntradaPX, (t.Height-tema.TextoXS.Linea)/2)
}

// filete es el `border-r` del carril: un rectángulo relleno por DENTRO, no un trazo
// (regla 4 del patrón de widget), con el borde compuesto sobre el color del carril y
// no aplanado sobre `surface`, que lo pintaba como una raya clara. Es SOLO del carril:
// la web lo conserva (`border-border-subtle … border-r` en su `BarraLateral`).
func filete(v tema.Variante) *canvas.Rectangle {
	return canvas.NewRectangle(ui.BordeSobre("border-subtle", "sidebar-bg", v))
}

// fileteHoja es un divisor del CONTENIDO: `border-subtle` sobre la superficie. No es
// `filete`, que lo compone sobre el color del carril: con el carril de color, un divisor
// del contenido hecho con él salía como una raya petróleo sobre el papel.
func fileteHoja(v tema.Variante) *canvas.Rectangle {
	return canvas.NewRectangle(tema.Color("border-subtle", v))
}

// aLaDerecha es el `ml-auto` del aspa: su único hijo, con ese ancho, pegado a la derecha
// y a todo el alto de la fila.
type aLaDerecha struct{ ancho float32 }

func (l aLaDerecha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(l.ancho, 0) }

func (l aLaDerecha) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(t.Width-l.ancho, 0))
	objs[0].Resize(fyne.NewSize(l.ancho, t.Height))
}

// bloque es una fila del carril con lo que deja debajo.
type bloque struct {
	obj   fyne.CanvasObject
	alto  float32
	abajo float32
}

// carril apila las filas con el `px-3 py-4` del `<nav>`. No desplaza por su cuenta: lo
// decide quien lo monta, como en `ui.Lateral`.
type carril struct{ arriba []bloque }

var _ fyne.Layout = (*carril)(nil)

func (c *carril) MinSize([]fyne.CanvasObject) fyne.Size {
	alto := 2 * MenuPY
	for _, b := range c.arriba {
		alto += b.alto + b.abajo
	}
	return fyne.NewSize(AnchoMenu, alto)
}

func (c *carril) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(t.Width-tema.Borde, 0))
	objs[1].Resize(fyne.NewSize(tema.Borde, t.Height))

	ancho := max(t.Width-tema.Borde-2*MenuPX, 0)
	y := MenuPY
	for _, b := range c.arriba {
		b.obj.Move(fyne.NewPos(MenuPX, y))
		b.obj.Resize(fyne.NewSize(ancho, b.alto))
		y += b.alto + b.abajo
	}
}
