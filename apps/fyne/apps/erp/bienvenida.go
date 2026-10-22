// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # La portada (`Bienvenida` de ErpApp.tsx)
//
// Aquí está LA decisión de producto que más separa a SyERP de la competencia: el
// catálogo trae treinta y tres áreas y enseñarlas todas el primer día es exactamente
// por lo que un ERP se percibe como inmanejable. Se agrupan por lo que son para el
// negocio —cuatro grupos— y la barra lateral solo lleva las encendidas. Arriba va el
// trabajo de la semana: las tareas, el banco, el perfil o las próximas obligaciones, y
// el resumen.
//
// Medido en el arnés (1280×800 y 360×740, 2026-09-23):
//
//	cabecera     60,25: saludo `text-lg` con la cuenta de áreas (19,5, `rounded-full
//	             border`), frase `text-xs` (`mt-1`); a la derecha «Nueva factura»,
//	             «Libro diario» y la lupa con ⌘K (`sm` de 28). `items-baseline` baja la
//	             izquierda 5. En el móvil los botones bajan (`gap-3`)
//	avisos       65: tarjeta `p-4` con borde de aviso, título, frase y el botón; en el
//	             móvil el botón baja a todo el ancho
//	próximas     versalitas con «Ver el calendario completo» (`xs`, 24) y la lista:
//	             filas de 47,25 (`px-4 py-2.5`)
//	resumen      tres tarjetas de 94,75 (`p-4`): rótulo e insignia, la cifra `text-xl`
//	             y la pista; una columna en el móvil
//	áreas        versalitas e insignia, «Obligaciones» siempre activa (66,75), y por
//	             grupo la rejilla de tarjetas: 155,5 con una fila de modelos (`p-4`,
//	             título e insignia, «Requiere:», modelos `mt-3`, pie `mt-4 border-t
//	             pt-3` con «N modelos» y Encender/Apagar). Las de una fila miden lo que
//	             la más alta, que es lo que hace `grid`.

// Medidas de la portada.
const (
	// HuecoTarjetas es el `gap-3` de las rejillas.
	HuecoTarjetas float32 = 3 * 0.25 * tema.Rem
	// TarjetaPX es el `p-4` de una tarjeta.
	TarjetaPX float32 = 4 * 0.25 * tema.Rem
	// ChipPX y ChipPY son el `px-2.5 py-1` de un modelo dentro de la tarjeta.
	ChipPX float32 = 2.5 * 0.25 * tema.Rem
	ChipPY float32 = 1 * 0.25 * tema.Rem
	// AltoMinChip es el `min-h-7`.
	AltoMinChip float32 = 7 * 0.25 * tema.Rem
	// SeparaSecciones es el `space-y-6` de la portada, SeparaSeccion el `space-y-2`
	// de dentro de una y SeparaAreas el `space-y-4` de las áreas.
	SeparaSecciones float32 = 6 * 0.25 * tema.Rem
	SeparaSeccion   float32 = 2 * 0.25 * tema.Rem
	SeparaAreas     float32 = 4 * 0.25 * tema.Rem
	// BajaSaludo es lo que `sm:items-baseline` baja el saludo respecto a los botones:
	// la línea base del botón la pone su icono. Chromium con rem16: 4 px.
	BajaSaludo float32 = 4
	// CuentaPX y CuentaPY son el `px-2.5 py-0.5` de «11 de 33 áreas activas».
	CuentaPX float32 = 2.5 * 0.25 * tema.Rem
	CuentaPY float32 = 0.5 * 0.25 * tema.Rem
	// ProximaPY es el `py-2.5` de una próxima obligación.
	ProximaPY float32 = 2.5 * 0.25 * tema.Rem
	// RecientePX es el `p-3.5` de una tarjeta de reciente.
	RecientePX float32 = 3.5 * 0.25 * tema.Rem
	// PieTarjetaMT y PieTarjetaPT son el `mt-4` y el `pt-3` del pie de un área.
	PieTarjetaMT float32 = 4 * 0.25 * tema.Rem
	PieTarjetaPT float32 = 3 * 0.25 * tema.Rem
	// MaxProximas es el `slice(0, 4)` de las próximas obligaciones.
	MaxProximas = 4
)

// AltoChip es la caja de `text-sm` con su `py-1` y su borde, nunca por debajo del
// `min-h-7`.
var AltoChip = max(tema.TextoSM.Linea+2*ChipPY+2*tema.Borde, AltoMinChip)

// CortesTarjetas es `grid gap-3 sm:grid-cols-2 lg:grid-cols-3`: una columna, dos desde
// `sm` y tres desde `lg`. CortesResumen es el `grid-cols-1 sm:grid-cols-3` del resumen.
//
// Ojo, y está medido en el repositorio: los cortes miran la VENTANA y con el rem
// INICIAL de 16, no el de 14 de la suite ni el ancho de esta caja. Por eso se le pasa
// el ancho de la ventana y no el del contenido.
var (
	CortesTarjetas = ui.Cortes{1, 2, 2, 3, 3, 3}
	CortesResumen  = ui.Cortes{1, 3, 3, 3, 3, 3}
)

// Categorias es el agrupado de `CATEGORIAS` en ErpApp.tsx: treinta y tres tarjetas en
// orden alfabético no le dicen a nadie por dónde empezar, y cuatro grupos sí. Lo que no
// esté en ninguna lista cae en operaciones, que es el cajón grande.
var Categorias = []struct {
	Clave   string
	Modulos []string
}{
	{"sales", []string{"crm", "ventas", "marketing", "suscripciones", "tpv", "eventos", "encuestas", "ayuda", "referidos"}},
	{"finance", []string{"contabilidad", "facturacion", "gastos", "consolidacion"}},
	{"people", []string{"contactos", "rrhh", "nominas", "ausencias", "reclutamiento", "evaluaciones", "elearning"}},
	{"operations", nil},
}

// rotuloCategoria es `ROTULO_CATEGORIA`.
var rotuloCategoria = map[string]string{
	"sales":      "dashboard.categorySales",
	"operations": "dashboard.categoryOperations",
	"people":     "dashboard.categoryPeople",
	"finance":    "dashboard.categoryFinance",
}

// Bienvenida es la portada. Es estado, como la lista.
type Bienvenida struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Modulos son los treinta y tres del catálogo y Activos los encendidos.
	Modulos []Modulo
	Activos []string
	// Nombre es el de quien ha entrado, para el saludo. Vacío: se saluda sin nombre.
	Nombre string
	// Fase es la del catálogo: sin él no hay nada que agrupar.
	Fase Fase
	// Ventana es el ancho de la ventana, que decide cuántas columnas caben.
	Ventana float32
	// Recientes son los modelos abiertos hace poco, el más reciente primero.
	Recientes []Modelo
	// Resumen son los indicadores; nil, todavía no han llegado (se pinta igual).
	Resumen *api.ERPResumen
	// Tareas son las propias de hoy o vencidas; SinJustificar, los movimientos del
	// banco que esperan su factura.
	Tareas, SinJustificar int
	// ConPerfil dice si hay perfil de organización y SabePerfil si ya se sabe.
	ConPerfil, SabePerfil bool
	// VerObligaciones: sin lectura en Obligaciones no se enseñan sus fechas.
	VerObligaciones bool
	// Proximas son las cuatro obligaciones que vienen; nil, cargando.
	Proximas []api.ERPObligacion
	// Hoy es la fecha de referencia; cero, la de ahora.
	Hoy time.Time

	AlAbrirModelo func(Modelo)
	AlNuevaFicha  func(Modelo)
	AlVista       func(vista string)
	AlPaleta      func()
	AlCambiar     func(m Modulo, activo bool)
	// AlConfigurar abre el asistente y AlBanco la vista del banco. nil: la tarjeta que
	// lleva a ellos no se pinta.
	AlConfigurar func()
	AlBanco      func()
}

// NuevaBienvenida crea la portada.
func NuevaBienvenida(tr *i18n.Translator) *Bienvenida {
	return &Bienvenida{tr: tr, loc: tr.Locale()}
}

func (b *Bienvenida) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return b.tr.T(clave, v)
}

func (b *Bienvenida) sm() bool { return b.Ventana == 0 || b.Ventana >= ui.CorteSM }

func (b *Bienvenida) vista(v string) func() {
	return func() {
		if b.AlVista != nil {
			b.AlVista(v)
		}
	}
}

// Grupos reparte los módulos por categoría, en el orden de `Categorias`, y deja en
// operaciones lo que no encaje en ninguna. Un grupo vacío no sale.
func (b *Bienvenida) Grupos() []Grupo {
	asignados := map[string]bool{}
	grupos := make([]Grupo, 0, len(Categorias))
	for _, c := range Categorias {
		g := Grupo{Clave: c.Clave}
		for _, m := range b.Modulos {
			if slices.Contains(c.Modulos, m.Clave) {
				g.Modulos = append(g.Modulos, m)
				asignados[m.Clave] = true
			}
		}
		grupos = append(grupos, g)
	}
	for i := range grupos {
		if grupos[i].Clave != "operations" {
			continue
		}
		for _, m := range b.Modulos {
			if !asignados[m.Clave] {
				grupos[i].Modulos = append(grupos[i].Modulos, m)
			}
		}
	}
	return slices.DeleteFunc(grupos, func(g Grupo) bool { return len(g.Modulos) == 0 })
}

// Grupo es una categoría con sus áreas.
type Grupo struct {
	Clave   string
	Modulos []Modulo
}

// ModeloFactura es el de «Nueva factura»: `factura_venta`, o el primero que tenga
// «factura» en la clave.
func (b *Bienvenida) ModeloFactura() (Modelo, bool) {
	var otro *Modelo
	for _, m := range b.Modulos {
		for i := range m.Modelos {
			x := m.Modelos[i]
			if x.Clave == api.ERPFacturaVenta {
				return x, true
			}
			if otro == nil && strings.Contains(x.Clave, "factura") {
				otro = &m.Modelos[i]
			}
		}
	}
	if otro != nil {
		return *otro, true
	}
	return Modelo{}, false
}

// Objeto arma la portada.
func (b *Bienvenida) Objeto(v tema.Variante) fyne.CanvasObject {
	if b.Fase == Cargando {
		return centrado(ui.NuevaRueda(24, "text-muted", v), AltoEsqueleto)
	}
	ancho := anchoUtilDe(b.Ventana, AnchoContenidoLista)
	p := &pila{}
	add := func(o fyne.CanvasObject, alto float32) {
		arriba := SeparaSecciones
		if len(p.bloques) == 0 {
			arriba = 0
		}
		p.add(o, alto, arriba)
	}
	add(b.cabecera(v, ancho))
	aviso := tema.Color("warning", v)
	if b.Tareas > 0 {
		add(b.aviso(v, ancho, b.t("dashboard.tasksTitle"), b.t("dashboard.tasksDue", i18n.Vars{"count": b.Tareas}),
			b.t("dashboard.tasksAction"), aviso, b.vista(VistaTareas)))
	}
	if b.SinJustificar > 0 && b.AlBanco != nil {
		add(b.aviso(v, ancho, b.t("dashboard.weekTitle"), b.t("dashboard.bankPending", i18n.Vars{"count": b.SinJustificar}),
			b.t("dashboard.bankAction"), aviso, b.AlBanco))
	}
	switch {
	case b.SabePerfil && !b.ConPerfil && b.AlConfigurar != nil:
		// `border-primary/40`.
		borde := tema.Color("primary", v)
		borde.A = 102 // el /40: 0,4 × 255
		add(b.aviso(v, ancho, b.t("dashboard.setupTitle"), b.t("dashboard.setupBody"), b.t("dashboard.setupAction"),
			sobre(borde, tema.Color("surface-raised", v)), b.AlConfigurar))
	case b.ConPerfil && b.VerObligaciones:
		add(b.proximas(v, ancho))
	}
	add(b.resumen(v, ancho))
	if len(b.Recientes) > 0 {
		add(b.recientes(v, ancho))
	}
	add(b.areas(v, ancho))
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoContenidoLista, MargenLista)
}

// cabecera es el saludo con la cuenta de áreas y la frase, y los tres accesos.
func (b *Bienvenida) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	// La derecha: «Nueva factura», «Libro diario» y la lupa con ⌘K.
	var der []pieza
	if m, ok := b.ModeloFactura(); ok {
		nueva := ui.NuevoBoton(b.t("dashboard.quickNewInvoice"), func() {
			if b.AlNuevaFicha != nil {
				b.AlNuevaFicha(m)
			}
		}).Con(ui.Primario, ui.TallaSM)
		nueva.ConIcono(iconos.Mas, 14)
		nueva.MargenIcono = HuecoUno
		der = append(der, piezaDe(nueva, nueva.MinSize()))
	}
	libro := ui.NuevoBoton(b.t("dashboard.quickLedger"), b.vista(VistaLibro)).Con(ui.Secundario, ui.TallaSM)
	lupa := NuevoBotonLupa(b.t("palette.placeholder"), b.AlPaleta)
	der = append(der, piezaDe(libro, libro.MinSize()), piezaDe(lupa, lupa.MinSize()))
	var anchoDer float32
	for i, x := range der {
		anchoDer += x.ancho
		if i > 0 {
			anchoDer += HuecoDos
		}
	}
	// La izquierda: el saludo, la cuenta y la frase.
	saludo := b.t("nav.home")
	if b.Nombre != "" {
		saludo = b.t("dashboard.greeting", i18n.Vars{"name": b.Nombre})
	}
	anchoIzq := ancho
	if b.sm() {
		anchoIzq = max(ancho-anchoDer-CabeceraObligGap, 0)
	}
	h4, peso := ui.EscalonTitulo(4), ui.PesoTitulo(4) // `sytue-h4`: el título es de la forma
	titulo := ui.Texto(saludo, h4, peso, tema.Color("text", v))
	cuenta := b.t("dashboard.modulesCount", i18n.Vars{"active": contar(len(b.Activos)), "total": contar(len(b.Modulos))})
	tc := ui.NuevaTecla(cuenta, tema.TextoXS, ui.SemiNegrita)
	tc.Fondo, tc.Tinta, tc.Borde = "bg-subtle", "text-muted", "border"
	tc.RellenoX, tc.RellenoY, tc.Radio = CuentaPX, CuentaPY, tema.RadioPill
	mc := tc.MinSize()
	wt := ui.Truncar(titulo, saludo, h4, peso, max(anchoIzq-mc.Width-HuecoDos, 0))
	frase := ui.NuevoParrafo(b.t("dashboard.subtitle"), tema.TextoXS, ui.Normal, "text-muted", v)
	altoFrase := ui.AltoParrafo(frase, anchoIzq)
	y0 := float32(0)
	if b.sm() {
		y0 = BajaSaludo
	}
	l.texto(titulo, 0, y0)
	l.poner(tc, wt+HuecoDos, y0+(h4.Linea-mc.Height)/2, mc.Width, mc.Height)
	l.poner(frase, 0, y0+h4.Linea+HuecoUno, anchoIzq, altoFrase)
	altoIzq := h4.Linea + HuecoUno + altoFrase
	var alto float32
	xd, yd := ancho-anchoDer, float32(0)
	if b.sm() {
		alto = max(BajaSaludo+altoIzq, AltoBotonSM)
	} else {
		xd, yd = 0, altoIzq+CabeceraObligGap
		alto = yd + AltoBotonSM
	}
	for _, x := range der {
		l.pieza(x, xd, yd)
		xd += x.ancho + HuecoDos
	}
	alto += CabeceraPB
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, alto, ancho, tema.Borde)
	return l.objeto(alto + tema.Borde)
}

// AltoBotonSM es el `size="sm"` de la suite.
const AltoBotonSM float32 = 8 * 0.25 * tema.Rem

// fondoTarjeta es la caja de `Card`: `bg-surface-raised`, borde, `rounded-lg` y su
// sombra.
func fondoTarjeta(v tema.Variante, borde color.NRGBA, nivel tema.NivelSombra) fyne.CanvasObject {
	return fondoTarjetaCon(v, tema.Color("surface-raised", v), borde, nivel)
}

// fondoTarjetaCon es fondoTarjeta con otro relleno: el de un área apagada.
func fondoTarjetaCon(v tema.Variante, relleno, borde color.NRGBA, nivel tema.NivelSombra) fyne.CanvasObject {
	o, _ := fondoConCaja(v, relleno, borde, nivel)
	return o
}

// fondoConCaja es el fondo con su caja a mano, para quien tiene que cambiarle el borde
// al pasar por encima (`hover:border-border-strong`).
func fondoConCaja(v tema.Variante, relleno, borde color.NRGBA, nivel tema.NivelSombra) (fyne.CanvasObject, *ui.Caja) {
	caja := ui.NuevaCaja(relleno, borde, tema.RadioLG)
	s := ui.NuevaSombra(nivel, tema.RadioLG)
	return container.New(&conSombra{caja: caja, sombra: s, v: v}, append([]fyne.CanvasObject{s.Objeto()}, caja.Objetos()...)...), caja
}

type conSombra struct {
	caja   *ui.Caja
	sombra *ui.Sombra
	v      tema.Variante
}

func (c *conSombra) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (c *conSombra) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.sombra.Ajustar(fyne.NewPos(0, 0), t, c.v)
	c.caja.Redimensionar(t)
}

// aviso es una tarjeta de «esta semana»: título, frase y el botón que lleva a hacerlo.
// En el móvil el botón baja y ocupa todo el ancho (`flex-col`).
func (b *Bienvenida) aviso(v tema.Variante, ancho float32, titulo, cuerpo, accion string, borde color.NRGBA, al func()) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*tema.Borde - 2*TarjetaPX
	boton := ui.NuevoBoton(accion, al).Con(ui.Primario, ui.TallaSM)
	mb := boton.MinSize()
	anchoIzq := dentro
	if b.sm() {
		anchoIzq = max(dentro-mb.Width-CabeceraObligGap, 0)
	}
	t := ui.Texto(titulo, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	ui.Truncar(t, titulo, tema.TextoSM, ui.SemiNegrita, anchoIzq)
	f := ui.NuevoParrafo(cuerpo, tema.TextoSM, ui.Normal, "text-muted", v)
	altoF := ui.AltoParrafo(f, anchoIzq)
	altoIzq := tema.TextoSM.Linea + HuecoUno + altoF
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	var alto float32
	if b.sm() {
		alto = max(altoIzq, mb.Height)
		y := x0 + (alto-altoIzq)/2
		l.texto(t, x0, y)
		l.poner(f, x0, y+tema.TextoSM.Linea+HuecoUno, anchoIzq, altoF)
		l.poner(boton, x0+dentro-mb.Width, x0+(alto-mb.Height)/2, mb.Width, mb.Height)
	} else {
		l.texto(t, x0, x0)
		l.poner(f, x0, x0+tema.TextoSM.Linea+HuecoUno, anchoIzq, altoF)
		l.poner(boton, x0, x0+altoIzq+CabeceraObligGap, dentro, mb.Height)
		alto = altoIzq + CabeceraObligGap + mb.Height
	}
	alto += 2 * x0
	l.puestos = append([]puesto{{o: fondoTarjeta(v, borde, tema.SombraSM), w: ancho, h: alto}}, l.puestos...)
	return l.objeto(alto)
}

// seccion es el título en versalitas de una sección, con una pieza a la derecha si la
// lleva (`flex items-center justify-between`).
func seccionCon(v tema.Variante, titulo string, der *pieza, ancho float32) (fyne.CanvasObject, float32) {
	t := ui.NuevoVersal(titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	alto := tema.TextoXS.Linea
	if der != nil {
		alto = max(alto, der.alto)
	}
	l := &lienzo{}
	l.poner(t.Objeto(), 0, (alto-tema.TextoXS.Linea)/2, t.Ancho(), tema.TextoXS.Linea)
	if der != nil {
		l.pieza(*der, ancho-der.ancho, (alto-der.alto)/2)
	}
	return l.objeto(alto)
}

// proximas son las cuatro obligaciones que vienen, con «Ver el calendario completo».
func (b *Bienvenida) proximas(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	ver := ui.NuevoBoton(b.t("dashboard.seeAll"), b.vista(VistaObligaciones)).Con(ui.Fantasma, ui.TallaXS)
	pv := piezaDe(ver, ver.MinSize())
	o, a := seccionCon(v, b.t("dashboard.upcomingTitle"), &pv, ancho)
	p.add(o, a, 0)
	switch {
	case b.Proximas == nil:
		r := ui.NuevaRueda(16, "text-muted", v)
		l := &lienzo{}
		l.poner(r, 0, 0, 16, 16)
		o, alto := l.objeto(16)
		p.add(o, alto, SeparaSeccion)
	case len(b.Proximas) == 0:
		o := ui.NuevoParrafo(b.t("dashboard.upcomingEmpty"), tema.TextoSM, ui.Normal, "text-muted", v)
		p.add(o, ui.AltoParrafo(o, ancho), SeparaSeccion)
	default:
		filas := make([]bloque, 0, len(b.Proximas))
		for _, o := range b.Proximas {
			f, alto := b.proxima(v, o, ancho-2*tema.Borde)
			filas = append(filas, bloque{f, alto, 0})
		}
		o, alto := listaEnCaja(v, filas, radioTarjeta())
		p.add(o, alto, SeparaSeccion)
	}
	return p.objeto()
}

// proxima es una fila: nombre · periodo y lo que falta (o qué es), y a la derecha «tu
// fecha» con su insignia.
func (b *Bienvenida) proxima(v tema.Variante, o api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	hoy := b.Hoy
	if hoy.IsZero() {
		hoy = time.Now()
	}
	dias := DiasHasta(o.Recomendada, hoy)
	tono := ui.InsigniaNeutra
	switch {
	case dias < 0:
		tono = ui.InsigniaPeligro
	case dias <= 7:
		tono = ui.InsigniaAviso
	}
	var rot string
	switch {
	case dias == 0:
		rot = b.t("obligations.today")
	case dias < 0 && DiasHasta(o.Limite, hoy) == 0:
		rot = b.t("obligations.lastDay")
	case dias < 0:
		rot = b.t("obligations.lastDays", i18n.Vars{"count": DiasHasta(o.Limite, hoy)})
	default:
		rot = b.t("obligations.daysLeft", i18n.Vars{"count": dias})
	}
	fecha := b.t("obligations.yourDate") + " " + i18n.FormatDayMonth(b.loc, o.Recomendada.UTC())
	ins := piezaDe(ui.NuevaInsignia(rot, tono), ui.MedidaInsignia(rot))
	wf := anchoTabular(fecha, tema.TextoXS, ui.Normal)
	anchoDer := wf + HuecoDos + ins.ancho
	dentro := ancho - 2*FilaOblPX
	cabe := max(dentro-anchoDer-HuecoTarjetas, 0)
	nombre := o.Nombre + " · " + RotuloDePeriodo(o.Periodo, b.loc, b.tr)
	n := ui.Texto(nombre, tema.TextoSM, ui.Medio, tema.Color("text", v))
	ui.Truncar(n, nombre, tema.TextoSM, ui.Medio, cabe)
	sub, tinta := o.ConceptoRotulo, "text-subtle"
	if fs := Faltas(o, b.tr); len(fs) > 0 {
		textos := make([]string, len(fs))
		for i, f := range fs {
			textos[i] = f.Texto
		}
		sub, tinta = strings.Join(textos, " · "), "warning-text"
	}
	s := ui.Texto(sub, tema.TextoXS, ui.Normal, tema.Color(tinta, v))
	ui.Truncar(s, sub, tema.TextoXS, ui.Normal, cabe)
	altoIzq := tema.TextoSM.Linea + tema.TextoXS.Linea
	alto := altoIzq + 2*ProximaPY
	l := &lienzo{}
	l.texto(n, FilaOblPX, ProximaPY)
	l.texto(s, FilaOblPX, ProximaPY+tema.TextoSM.Linea)
	x := FilaOblPX + dentro - anchoDer
	tf, _ := textoTabular(fecha, tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	l.poner(tf, x, (alto-tema.TextoXS.Linea)/2, wf, tema.TextoXS.Linea)
	l.pieza(ins, x+wf+HuecoDos, (alto-ins.alto)/2)
	return l.objeto(alto)
}

// resumen son los tres indicadores: ventas, libro y pendiente de cobro.
func (b *Bienvenida) resumen(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	o, a := seccionCon(v, b.t("dashboard.summaryTitle"), nil, ancho)
	p.add(o, a, 0)
	r := b.Resumen
	dinero := func(c int64) string { return i18n.FormatCurrency(b.loc, c) }
	factura, hayFactura := b.ModeloFactura()
	abrirFactura := func() {
		if hayFactura && b.AlAbrirModelo != nil {
			b.AlAbrirModelo(factura)
		}
	}
	type kpi struct {
		rotulo, valor, pista string
		insignia             string
		tono                 ui.Tono
		al                   func()
	}
	var ks [3]kpi
	ks[0] = kpi{rotulo: b.t("dashboard.kpiSales"), pista: b.t("dashboard.kpiSalesHint"), al: abrirFactura, valor: dinero(0)}
	ks[1] = kpi{rotulo: b.t("nav.ledger"), valor: b.t("dashboard.doubleEntry"), pista: b.t("dashboard.kpiFinanceHint"), al: b.vista(VistaLibro)}
	ks[2] = kpi{rotulo: b.t("dashboard.kpiReceivable"), pista: b.t("dashboard.kpiPeopleHint"), al: abrirFactura, valor: dinero(0)}
	if r != nil {
		f, c := r.Facturacion, r.Contabilidad
		ks[0].valor, ks[0].insignia, ks[0].tono = dinero(f.TotalCentimos), b.t("dashboard.invoiceCount", i18n.Vars{"count": f.TotalRegistros}), ui.InsigniaExito
		ks[1].valor = b.t("dashboard.entries", i18n.Vars{"count": c.TotalAsientos})
		ks[1].pista = b.t("dashboard.debitCredit", i18n.Vars{"amount": dinero(c.TotalDebe)})
		ks[1].insignia, ks[1].tono = b.t("balance.doesNotSquare"), ui.InsigniaPeligro
		if c.Cuadra {
			ks[1].insignia, ks[1].tono = b.t("dashboard.ledgerBalanced"), ui.InsigniaExito
		}
		ks[2].valor, ks[2].insignia, ks[2].tono = dinero(f.PendienteCobroCentimos), b.t("dashboard.pendingInvoices", i18n.Vars{"count": f.FacturasPendientes}), ui.InsigniaInfo
		if f.ProximoVencimiento != "" {
			if d, err := time.Parse(time.DateOnly, f.ProximoVencimiento[:min(10, len(f.ProximoVencimiento))]); err == nil {
				ks[2].pista = b.t("dashboard.nextDue", i18n.Vars{"date": i18n.FormatDayMonth(b.loc, d)})
			}
		}
	}
	rej := ui.RejillaTarjetas(b.Ventana, HuecoTarjetas, CortesResumen)
	cols := max(rej.Columnas(ancho), 1)
	w := rej.AnchoColumna(ancho)
	celdas := make([]celdaRejilla, 0, len(ks))
	for _, k := range ks {
		o, alto := tarjetaKPI(v, k.rotulo, k.valor, k.pista, k.insignia, k.tono, w, k.al)
		celdas = append(celdas, celdaRejilla{o, alto})
	}
	o, alto := enRejilla(rej, cols, ancho, celdas)
	p.add(o, alto, SeparaSeccion)
	return p.objeto()
}

// AltoKPI es una tarjeta del resumen: p-4, fila de insignia de20px, mb-1,
// cifra text-xl y pista mt-1. Chromium remoto con rem16: 108px.
var AltoKPI = 2*tema.Borde + 2*TarjetaPX + ui.MedidaInsignia("").Height + HuecoUno + tema.TextoXL.Linea + HuecoUno + tema.TextoXS.Linea

// tarjetaKPI es un indicador del resumen: se pulsa entera y abre lo suyo.
func tarjetaKPI(v tema.Variante, rotulo, valor, pista, insignia string, tono ui.Tono, ancho float32, al func()) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*tema.Borde - 2*TarjetaPX
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	fondo, caja := fondoConCaja(v, tema.Color("surface-raised", v), tema.Color("border", v), tema.SombraSM)
	l.poner(fondo, 0, 0, ancho, AltoKPI)
	wi := float32(0)
	if insignia != "" {
		m := ui.MedidaInsignia(insignia)
		wi = m.Width
		l.poner(ui.NuevaInsignia(insignia, tono), x0+dentro-m.Width, x0, m.Width, m.Height)
	}
	// `items-baseline`: el rótulo baja hasta la línea base de la insignia. Medido: 1,5.
	r := ui.Texto(rotulo, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	ui.Truncar(r, rotulo, tema.TextoXS, ui.Medio, max(dentro-wi-HuecoDos, 0))
	l.texto(r, x0, x0+1.5)
	y := x0 + ui.MedidaInsignia("").Height + HuecoUno
	c := ui.Texto(valor, tema.TextoXL, ui.SemiNegrita, tema.Color("text", v))
	ui.Truncar(c, valor, tema.TextoXL, ui.SemiNegrita, dentro)
	l.texto(c, x0, y)
	y += tema.TextoXL.Linea + HuecoUno
	pi := ui.Texto(pista, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	ui.Truncar(pi, pista, tema.TextoXS, ui.Normal, dentro)
	l.texto(pi, x0, y)
	o, alto := l.objeto(AltoKPI)
	return NuevoPulsableCon(o, fyne.NewSize(ancho, alto), rotulo, al).ConBorde(caja, "surface-raised"), alto
}

// recientes son los últimos modelos abiertos, en tarjetas con «Abrir».
func (b *Bienvenida) recientes(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	o, a := seccionCon(v, b.t("welcome.quickAccess"), nil, ancho)
	p.add(o, a, 0)
	rej := ui.RejillaTarjetas(b.Ventana, HuecoTarjetas, CortesTarjetas)
	cols := max(rej.Columnas(ancho), 1)
	w := rej.AnchoColumna(ancho)
	celdas := make([]celdaRejilla, 0, len(b.Recientes))
	for _, m := range b.Recientes {
		modelo := m
		abrir := func() {
			if b.AlAbrirModelo != nil {
				b.AlAbrirModelo(modelo)
			}
		}
		mod := m.Modulo
		for _, x := range b.Modulos {
			if x.Clave == m.Modulo {
				mod = x.Etiqueta
			}
		}
		boton := ui.NuevoBoton(b.t("dashboard.openModel"), abrir).Con(ui.Secundario, ui.TallaXS)
		mb := boton.MinSize()
		x0 := tema.Borde + RecientePX
		dentro := w - 2*x0
		cabe := max(dentro-mb.Width-HuecoTarjetas, 0)
		alto := 2*x0 + tema.TextoSM.Linea + tema.TextoXS.Linea
		l := &lienzo{}
		fondo, caja := fondoConCaja(v, tema.Color("surface-raised", v), tema.Color("border", v), tema.SombraSM)
		l.poner(fondo, 0, 0, w, alto)
		n := ui.Texto(m.Etiqueta, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
		ui.Truncar(n, m.Etiqueta, tema.TextoSM, ui.SemiNegrita, cabe)
		s := ui.Texto(mod, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		ui.Truncar(s, mod, tema.TextoXS, ui.Normal, cabe)
		l.texto(n, x0, x0)
		l.texto(s, x0, x0+tema.TextoSM.Linea)
		l.poner(boton, x0+dentro-mb.Width, (alto-mb.Height)/2, mb.Width, mb.Height)
		o, _ := l.objeto(alto)
		celdas = append(celdas, celdaRejilla{NuevoPulsableCon(o, fyne.NewSize(w, alto), m.Etiqueta, abrir).ConBorde(caja, "surface-raised"), alto})
	}
	o, alto := enRejilla(rej, cols, ancho, celdas)
	p.add(o, alto, SeparaSeccion)
	return p.objeto()
}

// areas son «Todas las áreas de negocio»: la cuenta de encendidas, Obligaciones
// siempre activa y cada grupo con su rejilla de tarjetas.
func (b *Bienvenida) areas(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	n := 0
	for _, m := range b.Modulos {
		if slices.Contains(b.Activos, m.Clave) {
			n++
		}
	}
	rot := b.t("welcome.activeCount", i18n.Vars{"count": n})
	ins := piezaDe(ui.NuevaInsignia(rot, tonoEncendida(n > 0)), ui.MedidaInsignia(rot))
	o, a := seccionCon(v, b.t("dashboard.allModules"), &ins, ancho)
	p.add(o, a, 0)
	// Obligaciones no se enciende ni se apaga: tras el asistente está siempre, porque
	// son plazos con Hacienda y la Seguridad Social, no una preferencia.
	if b.ConPerfil && b.VerObligaciones {
		o, alto := b.siempreActiva(v, ancho)
		p.add(o, alto, SeparaAreas)
	}
	rej := ui.RejillaTarjetas(b.Ventana, HuecoTarjetas, CortesTarjetas)
	cols := max(rej.Columnas(ancho), 1)
	w := rej.AnchoColumna(ancho)
	for _, g := range b.Grupos() {
		t := ui.Texto(b.t(rotuloCategoria[g.Clave]), tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
		p.add(textoSolo(t), tema.TextoXS.Linea, SeparaAreas)
		celdas := make([]celdaRejilla, 0, len(g.Modulos))
		for _, m := range g.Modulos {
			celdas = append(celdas, b.tarjeta(v, m, w))
		}
		o, alto := enRejilla(rej, cols, ancho, celdas)
		p.add(o, alto, SeparaSeccion)
	}
	return p.objeto()
}

// siempreActiva es la tarjeta de Obligaciones, con «Abrir».
func (b *Bienvenida) siempreActiva(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*tema.Borde - 2*TarjetaPX
	x0 := tema.Borde + TarjetaPX
	abrir := ui.NuevoBoton(b.t("action.open"), b.vista(VistaObligaciones)).Con(ui.Secundario, ui.TallaSM)
	mb := abrir.MinSize()
	anchoIzq := dentro
	if b.sm() {
		anchoIzq = max(dentro-mb.Width-CabeceraObligGap, 0)
	}
	l := &lienzo{}
	titulo := textoPieza(b.t("nav.obligations"), tema.TextoSM, ui.SemiNegrita, "text", v)
	rot := b.t("dashboard.alwaysOn")
	ins := piezaDe(ui.NuevaInsignia(rot, ui.InsigniaExito), ui.MedidaInsignia(rot))
	f := ui.NuevoParrafo(b.t("dashboard.alwaysOnBody"), tema.TextoSM, ui.Normal, "text-muted", v)
	altoF := ui.AltoParrafo(f, anchoIzq)
	altoIzq := ins.alto + HuecoUno + altoF
	var alto, y float32
	if b.sm() {
		alto = max(altoIzq, mb.Height)
		y = (alto - altoIzq) / 2
		l.poner(abrir, x0+dentro-mb.Width, x0+(alto-mb.Height)/2, mb.Width, mb.Height)
	} else {
		alto = altoIzq + CabeceraObligGap + mb.Height
		l.poner(abrir, x0, x0+altoIzq+CabeceraObligGap, dentro, mb.Height)
	}
	l.pieza(titulo, x0, x0+y+(ins.alto-titulo.alto)/2)
	l.pieza(ins, x0+titulo.ancho+HuecoDos, x0+y)
	l.poner(f, x0, x0+y+ins.alto+HuecoUno, anchoIzq, altoF)
	alto += 2 * x0
	l.puestos = append([]puesto{{o: fondoTarjeta(v, tema.Color("border", v), tema.SombraXS), w: ancho, h: alto}}, l.puestos...)
	return l.objeto(alto)
}

// tarjeta es un área: su nombre, si está encendida, lo que requiere, sus modelos y el
// pie con «N modelos» y Encender/Apagar.
func (b *Bienvenida) tarjeta(v tema.Variante, m Modulo, ancho float32) celdaRejilla {
	encendida := slices.Contains(b.Activos, m.Clave)
	dentro := ancho - 2*tema.Borde - 2*TarjetaPX
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	// Título e insignia.
	rot := b.rotuloEncendida(encendida)
	ins := piezaDe(ui.NuevaInsignia(rot, tonoEncendida(encendida)), ui.MedidaInsignia(rot))
	// Apagada, la tarjeta ENTERA va al 80 % (`opacity-80`): fondo, borde y textos, y
	// también la insignia, los modelos y el botón, cada color compuesto sobre el fondo
	// de la página (`ui.Grupo`).
	var grupo ui.Grupo
	rolFondo := "surface"
	if !encendida {
		grupo = GrupoApagada
		// Card blue gana al bg-bg-subtle/50 de la apagada; encendida lleva bg-surface.
		rolFondo = "card-blue"
	}
	tinta := func(token string) color.NRGBA { return grupo.Aplicar(tema.Color(token, v), v) }
	if in, ok := ins.obj.(*ui.Insignia); ok {
		in.Grupo = grupo
	}
	t := ui.Texto(m.Etiqueta, tema.TextoSM, ui.SemiNegrita, tinta("text"))
	ui.Truncar(t, m.Etiqueta, tema.TextoSM, ui.SemiNegrita, max(dentro-ins.ancho-HuecoDos, 0))
	l.texto(t, x0, x0+(ins.alto-tema.TextoSM.Linea)/2)
	l.pieza(ins, x0+dentro-ins.ancho, x0)
	y := x0 + ins.alto
	// «Requiere: …» (el `mb-1` de la fila y el `mt-0.5` del párrafo se funden en 3,5).
	if len(m.Dependencias) > 0 {
		deps := make([]string, 0, len(m.Dependencias))
		for _, d := range m.Dependencias {
			nombre := d
			for _, x := range b.Modulos {
				if x.Clave == d {
					nombre = x.Etiqueta
				}
			}
			deps = append(deps, nombre)
		}
		r := ui.NuevoParrafoConTinta(b.t("dashboard.requires", i18n.Vars{"deps": strings.Join(deps, ", ")}), tema.TextoSM, ui.Normal, tinta("text-subtle"))
		h := ui.AltoParrafo(r, dentro)
		y += HuecoUno
		l.poner(r, x0, y, dentro, h)
		y += h
	}
	// Los modelos, que saltan de línea (`mt-3 flex flex-wrap gap-1.5`). Sin «Requiere»,
	// el `mb-1` de la fila y el `mt-3` se funden en el mayor: 10,5, medido (136,25).
	chips := make([]pieza, 0, len(m.Modelos))
	for _, x := range m.Modelos {
		modelo, modulo := x, m
		c := NuevoChip(x.Etiqueta, encendida, func() {
			if !encendida && b.AlCambiar != nil {
				b.AlCambiar(modulo, true)
			}
			if b.AlAbrirModelo != nil {
				b.AlAbrirModelo(modelo)
			}
		})
		mc := MedidaChip(c)
		mc.Width = min(mc.Width, dentro)
		chips = append(chips, piezaDe(c, mc))
	}
	y += HuecoTarjetas
	an, al := medidasDe(chips)
	pos, ocupa := Envolver(an, al, HuecoPildorasOblig, HuecoPildorasOblig, dentro)
	l.piezas(chips, pos, x0, y)
	y += ocupa.Height
	// El pie: `mt-4 border-t pt-3`, «N modelos» y el botón.
	boton := ui.NuevoBoton(b.t("welcome.enable"), func() {
		if b.AlCambiar != nil {
			b.AlCambiar(m, !encendida)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	if encendida {
		boton = ui.NuevoBoton(b.t("welcome.disable"), func() {
			if b.AlCambiar != nil {
				b.AlCambiar(m, false)
			}
		}).Con(ui.Fantasma, ui.TallaSM)
	}
	boton.Inerte = b.AlCambiar == nil
	boton.Grupo = grupo
	mb := boton.MinSize()
	pieAlto := PieTarjetaMT + tema.Borde + PieTarjetaPT + mb.Height
	alto := y + pieAlto + x0
	pie := &pieArea{
		filete: canvas.NewRectangle(grupo.Aplicar(ui.BordeSobre("border-subtle", rolFondo, v), v)),
		texto:  ui.Texto(b.t("welcome.models", i18n.Vars{"count": len(m.Modelos)}), tema.TextoSM, ui.Normal, tinta("text-subtle")),
		boton:  boton, mb: mb, alto: pieAlto,
	}
	// El borde y el filete se componen sobre el relleno resuelto por la CSS del Card.
	borde, sombra := ui.BordeSobre("border", rolFondo, v), tema.SombraXS
	if !encendida {
		borde, sombra = grupo.Aplicar(ui.BordeSobre("border-subtle", rolFondo, v), v), tema.SombraSM
	}
	o := &tarjetaAreaLay{
		fondo: fondoTarjetaCon(v, tinta(rolFondo), borde, sombra), cuerpo: l, pie: container.New(pie, pie.filete, pie.texto, boton),
		x0: x0, dentro: dentro, altoPie: pieAlto,
	}
	cuerpo, _ := l.objeto(y)
	o.cuerpoObj = cuerpo
	return celdaRejilla{container.New(o, o.fondo, cuerpo, o.pie), alto}
}

// tarjetaAreaLay estira la tarjeta al alto de su fila y deja el pie abajo del todo
// (`flex-col justify-between`).
type tarjetaAreaLay struct {
	fondo, cuerpoObj, pie fyne.CanvasObject
	cuerpo                *lienzo
	x0, dentro, altoPie   float32
}

func (t *tarjetaAreaLay) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (t *tarjetaAreaLay) Layout(_ []fyne.CanvasObject, tam fyne.Size) {
	t.fondo.Move(fyne.NewPos(0, 0))
	t.fondo.Resize(tam)
	t.cuerpoObj.Move(fyne.NewPos(0, 0))
	t.cuerpoObj.Resize(fyne.NewSize(tam.Width, t.cuerpo.alto))
	t.pie.Move(fyne.NewPos(t.x0, tam.Height-t.x0-t.altoPie))
	t.pie.Resize(fyne.NewSize(t.dentro, t.altoPie))
}

// pieArea es el filete, «N modelos» y el botón.
type pieArea struct {
	filete *canvas.Rectangle
	texto  *canvas.Text
	boton  *ui.Boton
	mb     fyne.Size
	alto   float32
}

func (p *pieArea) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto) }

func (p *pieArea) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	p.filete.Move(fyne.NewPos(0, PieTarjetaMT))
	p.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	y := PieTarjetaMT + tema.Borde + PieTarjetaPT
	p.boton.Resize(p.mb)
	p.boton.Move(fyne.NewPos(t.Width-p.mb.Width, y))
	ui.ColocarTexto(p.texto, tema.TextoSM, 0, y+(p.mb.Height-tema.TextoSM.Linea)/2)
}

func (b *Bienvenida) rotuloEncendida(si bool) string {
	if si {
		return b.t("welcome.enabled")
	}
	return b.t("welcome.disabled")
}

func tonoEncendida(si bool) ui.Tono {
	if si {
		return ui.InsigniaExito
	}
	return ui.InsigniaNeutra
}

// celdaRejilla es una tarjeta con lo que mide por su contenido.
type celdaRejilla struct {
	o    fyne.CanvasObject
	alto float32
}

// enRejilla coloca las tarjetas en su rejilla: cada fila mide lo que su tarjeta más
// alta y todas las de la fila se estiran a ese alto, que es lo que hace `grid`.
func enRejilla(r ui.Rejilla, cols int, ancho float32, celdas []celdaRejilla) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	w := r.AnchoColumna(ancho)
	var y float32
	for i := 0; i < len(celdas); i += cols {
		fila := celdas[i:min(i+cols, len(celdas))]
		var alto float32
		for _, c := range fila {
			alto = max(alto, c.alto)
		}
		if i > 0 {
			y += r.Hueco
		}
		for j, c := range fila {
			l.poner(c.o, r.X(ancho, j), y, w, alto)
		}
		y += alto
	}
	return l.objeto(y)
}

// BotonLupa es el acceso a la paleta de la cabecera: la lupa y la tecla ⌘K en un
// botón fantasma `sm`.
type BotonLupa struct {
	ui.Pulsable
}

// NuevoBotonLupa crea el botón. `nombre` es su `title`.
func NuevoBotonLupa(nombre string, alPulsar func()) *BotonLupa {
	b := &BotonLupa{}
	b.Nombre, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// LupaPX es el `px-3` del botón `sm` y LupaHueco lo que separa la lupa de la tecla
// (medido: 10,5).
const (
	LupaPX    float32 = 3 * 0.25 * tema.Rem
	LupaHueco float32 = 3 * 0.25 * tema.Rem
)

// La tecla es `<kbd class="ml-1 rounded px-1.5 py-0.5 font-mono text-xs">`: la mono es
// la del navegador del arnés, la misma que lleva la referencia de la tabla.
const (
	TeclaLupaPX float32 = 1.5 * 0.25 * tema.Rem
	TeclaLupaPY float32 = 0.5 * 0.25 * tema.Rem
)

// textoTecla es la tecla del sistema: «⌘K» en un Mac y «Ctrl K» en lo demás. La web la
// escribe siempre «⌘K», así que la prueba de medidas la fija a esa para comparar.
var textoTecla = ui.AtajoDelSistema("Ctrl K")

func medidaTeclaLupa() fyne.Size {
	return fyne.NewSize(medirTexto(textoTecla, tema.TextoXS, true, ui.Normal)+2*TeclaLupaPX, tema.TextoXS.Linea+2*TeclaLupaPY)
}

// MinSize es la lupa, el hueco y la tecla con el `px-3`. Medido: 68,66×28.
func (b *BotonLupa) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(2*LupaPX+14+LupaHueco+medidaTeclaLupa().Width, AltoBotonSM)
}

// CreateRenderer arma el anillo, el fondo al pasar, la lupa y la tecla.
func (b *BotonLupa) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &lupaRenderer{
		b: b, aro: ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		lupa:  iconos.Trazo(iconos.Buscar, 14, "text-muted", v),
		tecla: ui.NuevaCaja(tema.Color("bg-subtle", v), color.NRGBA{}, 1*0.25*tema.Rem),
		letra: canvas.NewText(textoTecla, tema.Color("text-subtle", v)),
	}
	r.letra.TextSize, r.letra.TextStyle = tema.TextoXS.Tam, fyne.TextStyle{Monospace: true}
	r.Refresh()
	return r
}

type lupaRenderer struct {
	b     *BotonLupa
	aro   *ui.Aro
	caja  *ui.Caja
	lupa  *iconos.Dibujo
	tecla *ui.Caja
	letra *canvas.Text
}

func (r *lupaRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *lupaRenderer) Objects() []fyne.CanvasObject {
	return append(append(append(r.aro.Objetos(), r.caja.Objetos()...), r.lupa.Objeto()), append(r.tecla.Objetos(), r.letra)...)
}
func (r *lupaRenderer) Destroy() {}

func (r *lupaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.lupa.Colocar(LupaPX, (t.Height-14)/2, 14)
	m := medidaTeclaLupa()
	x, y := LupaPX+14+LupaHueco, (t.Height-m.Height)/2
	r.tecla.Redimensionar(m)
	for _, o := range r.tecla.Objetos() {
		o.Move(o.Position().Add(fyne.NewPos(x, y)))
	}
	r.letra.Move(fyne.NewPos(x+TeclaLupaPX, y+TeclaLupaPY))
	r.letra.Resize(fyne.NewSize(m.Width-2*TeclaLupaPX, tema.TextoXS.Linea))
}

func (r *lupaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, "text-muted"
	if r.b.Encima() {
		fondo, tinta = tema.Color("bg-subtle", v), "text"
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.lupa.Tenir(tinta, v)
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
}

// PulsableCon hace pulsable un bloque ya armado: la tarjeta entera del resumen o de un
// reciente, con su `cursor-pointer` y su `hover:border-border-strong`, que repinta la
// caja del fondo (`ConBorde`) sin rehacer el bloque.
type PulsableCon struct {
	ui.Pulsable
	o fyne.CanvasObject
	m fyne.Size
	// caja es la del fondo del bloque y relleno el token de su fondo; nil, sin borde
	// que cambie.
	caja    *ui.Caja
	relleno string
}

// ConBorde le da la caja del fondo, que pasa a `border-strong` con el puntero encima.
func (p *PulsableCon) ConBorde(c *ui.Caja, relleno string) *PulsableCon {
	p.caja, p.relleno = c, relleno
	return p
}

// tokenBorde es el del borde en reposo o con el puntero encima.
func (p *PulsableCon) tokenBorde() string {
	if p.Encima() {
		return "border-strong"
	}
	return "border"
}

// NuevoPulsableCon envuelve el bloque.
func NuevoPulsableCon(o fyne.CanvasObject, m fyne.Size, nombre string, alPulsar func()) *PulsableCon {
	p := &PulsableCon{o: o, m: m}
	p.Nombre, p.AlPulsar = nombre, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// MinSize es lo que mide el bloque.
func (p *PulsableCon) MinSize() fyne.Size { p.ExtendBaseWidget(p); return p.m }

// CreateRenderer pone el bloque a todo el tamaño.
func (p *PulsableCon) CreateRenderer() fyne.WidgetRenderer {
	return &pulsableConRenderer{p: p}
}

type pulsableConRenderer struct{ p *PulsableCon }

func (r *pulsableConRenderer) MinSize() fyne.Size           { return r.p.m }
func (r *pulsableConRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.p.o} }
func (r *pulsableConRenderer) Destroy()                     {}
func (r *pulsableConRenderer) Refresh() {
	if c := r.p.caja; c != nil {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		c.Pintar(tema.Color(r.p.relleno, v), tema.Color(r.p.tokenBorde(), v))
	}
}
func (r *pulsableConRenderer) Layout(t fyne.Size) { r.p.o.Resize(t) }

// GrupoApagada es el `opacity-80` de la tarjeta de un área apagada, sobre `bg`.
var GrupoApagada = ui.Grupo{Opacidad: 0.8, Fondo: "bg"}

// Chip es un modelo dentro de una tarjeta de área: un botón con borde que abre su
// lista. Propio del ERP: `min-h-7` con `text-sm`, que no es ninguna talla de `ui.Boton`.
type Chip struct {
	ui.Pulsable

	Rotulo string
	// Encendida es el área a la que pertenece: apagada, el chip va apagado también.
	Encendida bool
}

var _ fyne.Widget = (*Chip)(nil)

// NuevoChip crea el chip de un modelo.
func NuevoChip(rotulo string, encendida bool, alPulsar func()) *Chip {
	c := &Chip{Rotulo: rotulo, Encendida: encendida}
	c.Nombre, c.AlPulsar = rotulo, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el nombre del modelo.
func (c *Chip) AccessibilityLabel() string { return c.Rotulo }

func (c *Chip) peso() ui.Peso {
	if c.Encendida {
		return ui.Medio
	}
	return ui.Normal
}

// MedidaChip es el rótulo con su `px-2.5`, por el alto del `min-h-7`.
func MedidaChip(c *Chip) fyne.Size {
	return fyne.NewSize(ui.Medir(c.Rotulo, tema.TextoSM, c.peso()).Width+2*ChipPX+2*tema.Borde, AltoChip)
}

// CreateRenderer construye caja y rótulo.
func (c *Chip) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &chipRenderer{
		c:      c, // `rounded-control`: el radio del botón es de la forma
		aro:    ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		rotulo: ui.Texto(c.Rotulo, tema.TextoSM, c.peso(), tema.Color("text", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.rotulo)
	r.Refresh()
	return r
}

type chipRenderer struct {
	c       *Chip
	aro     *ui.Aro
	caja    *ui.Caja
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *chipRenderer) MinSize() fyne.Size           { return MedidaChip(r.c) }
func (r *chipRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *chipRenderer) Destroy()                     {}

func (r *chipRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ui.Truncar(r.rotulo, r.c.Rotulo, tema.TextoSM, r.c.peso(), t.Width-2*ChipPX-2*tema.Borde)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, ChipPX+tema.Borde, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *chipRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := tema.Color("bg", v), tema.Color("border", v), tema.Color("text", v)
	switch {
	case !r.c.Encendida:
		// `border-border-subtle bg-bg/50 text-text-muted hover:text-text`, dentro de la
		// tarjeta apagada al 80 %: al pasar solo cambia la tinta.
		m := tema.Color("bg", v)
		m.A = 128
		fondo = sobre(m, tema.Color("card-blue", v))
		borde = sobre(tokens.Crudo("border-subtle", v), fondo)
		if !r.c.Encima() {
			tinta = tema.Color("text-muted", v)
		}
		fondo, borde, tinta = GrupoApagada.Aplicar(fondo, v), GrupoApagada.Aplicar(borde, v), GrupoApagada.Aplicar(tinta, v)
	case r.c.Encima():
		fondo, borde = tema.Color("bg-subtle", v), tema.Color("border-strong", v)
	}
	r.aro.Ver(r.c.Enfocado())
	r.aro.Pintar(v)
	r.caja.Pintar(fondo, borde)
	r.rotulo.Color, r.rotulo.FontSource = tinta, r.c.peso().Cara()
	r.rotulo.Refresh()
}
