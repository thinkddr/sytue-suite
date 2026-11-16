// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El panel de lista de Contacts. Medido en `contacts/lista.ancho.json`:
//
//	sección   279 de ancho (los 280 del panel menos su `border-r`)   medido
//	fila      49 de alto · `px-3 py-2` con avatar `sm` (32)          medido
//	cabecera  46,5: `SectionHeader` con un botón de 28 dentro        medido
//
// La fila es `FilaContacto` y no `ui.FilaLista`: ver su comentario.
const (
	// LadoEstrellaFila es el `<StarGlyph filled size={14} />` del final de la fila.
	LadoEstrellaFila float32 = 14
	// RellenoBuscador es el `p-3` del bloque del buscador.
	RellenoBuscador = 3 * 0.25 * tema.Rem
	// MargenRecuento es el `mt-3` entre el buscador y la línea de recuento. Son 3 y
	// no 2 a propósito: la zona de toque de la casilla se estira 10 px hacia arriba y
	// con 8 de hueco pisaba el borde del buscador. Es el comentario de la web.
	MargenRecuento = 3 * 0.25 * tema.Rem
	// HuecoRecuento es el `gap-2.5` entre la casilla de «seleccionar todo» y el
	// recuento, y SangriaRecuento su `px-0.5`.
	HuecoRecuento   = 2.5 * 0.25 * tema.Rem
	SangriaRecuento = 0.5 * 0.25 * tema.Rem
)

// lista es el `<section aria-label="Lista de contactos">`: cabecera, buscador con su
// recuento y la lista virtualizada.
func (p *Pantalla) lista() fyne.CanvasObject {
	cabecera := ui.NuevaCabeceraApp(p.t("list.title", nil), p.accionesDeLista()...)
	// «Contactos» ya lo dice la barra única del teléfono: ahí quedan las acciones.
	cabecera.RepiteLaBarra = true
	// La cabecera hace `flex-wrap`: en el panel de 279 las acciones bajan a una segunda
	// línea y mide 76,25, como en producción. Su alto depende del ancho del panel.
	cabecera.Ancho = p.anchoLista()
	buscador := p.bloqueBuscador()

	altos := []float32{ui.MedidaCabeceraApp(cabecera).Height, AltoBloqueBuscador(p.hayRecuento())}
	hijos := []fyne.CanvasObject{cabecera, buscador}
	// La barra de lote sale con algo marcado y red, entre el buscador y la lista.
	if lote := p.barraLote(); lote != nil {
		altos = append(altos, ui.MedidaBarraDeLoteEn(lote, p.anchoLista()).Height)
		hijos = append(hijos, lote)
	}
	franja := container.New(&franjas{Altos: altos}, append(hijos, p.cuerpoLista())...)
	// El panel va sobre `surface` con un filete a la derecha. En el volcado el
	// `<section>` es transparente y quien pinta es su padre
	// (`div.border-border.bg-surface`), que además explica por qué el `<section>`
	// mide 279 y el panel 280: el `border-r` se come uno. Sin este fondo el panel
	// enseñaba el `bg` del armazón —papel en vez de blanco—, que es lo que cazó
	// `TestLaPantallaSePintaConSusTresColumnas` mirando el píxel.
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := canvas.NewRectangle(tema.Color("surface", v))
	filete := canvas.NewRectangle(tema.Color("border", v))
	return container.New(&panelConFilete{}, fondo, franja, filete)
}

// accionesDeLista son las de la cabecera, en UN grupo `flex items-center gap-1.5`:
// las de la etiqueta activa y las de la libreta (solo con red), «Revisar duplicados»
// si los hay, y «Importar / Exportar», que sale siempre.
func (p *Pantalla) accionesDeLista() []fyne.CanvasObject {
	var hijos []fyne.CanvasObject
	if !p.SinConexion && p.EtiquetaActiva != "" && p.nombreEtiqueta() != "" {
		hijos = append(hijos, p.accionesEtiqueta())
	}
	if !p.SinConexion {
		if b := p.accionesLibreta(); b != nil {
			hijos = append(hijos, b)
		}
	}
	if !p.SinConexion && p.Duplicados > 0 {
		hijos = append(hijos, ui.NuevoBoton(p.t("list.reviewDuplicates", i18n.Vars{"count": p.Duplicados}),
			p.AlRevisarDuplicados).Con(ui.Secundario, ui.TallaSM))
	}
	hijos = append(hijos, p.botonES())
	return []fyne.CanvasObject{container.New(ui.Fila{Hueco: HuecoAccionesLista}, hijos...)}
}

// HuecoAccionesLista es el `gap-1.5` del grupo de acciones de la cabecera.
var HuecoAccionesLista = tema.Espacio(1.5)

// anchoLista es lo que mide el `<section>` de la lista: el panel sin su `border-r`, o
// la ventana en el teléfono (`max-md:w-full`, y el `border-r` sigue).
func (p *Pantalla) anchoLista() float32 {
	ventana := p.Ventana
	if ventana == 0 {
		ventana = ui.CorteXL
	}
	if ventana < ui.Estrecho {
		return ui.AnchoHoja(ventana, ui.AnchoMenu(false), 0) - tema.Borde
	}
	return ui.AcotarAnchoPanel(ui.AnchoPanel(ventana), ui.AnchoHoja(ventana, ui.AnchoMenu(false), 0)) - tema.Borde
}

// barraLote es `BatchBar`: destacar o quitar el destacado, añadir o quitar de una
// etiqueta (si la libreta tiene) y mandar a la papelera, sobre los marcados de la
// libreta ENTERA —también los que el filtro esconde, como la web—. Sin red o sin nada
// marcado no sale.
func (p *Pantalla) barraLote() *ui.BarraDeLote {
	sel := p.Lote
	if p.SinConexion || len(sel) == 0 || p.AlLote == nil {
		return nil
	}
	n := len(sel)
	boton := func(clave string, v ui.Variante) *ui.Boton {
		b := ui.NuevoBoton(p.t(clave, nil), nil).Con(v, ui.TallaSM)
		b.Inerte = p.LoteOcupado
		return b
	}
	// Lo que se hace CON la selección y no le cambia nada: no espera a `LoteOcupado`.
	var acciones []*ui.Boton
	if p.AlCorreoATodos != nil {
		correo := ui.NuevoBoton(p.t("batch.emailAll", nil), func() { p.AlCorreoATodos(sel) }).Con(ui.Secundario, ui.TallaSM)
		acciones = append(acciones, correo)
	}
	if p.AlExportarSeleccion != nil {
		exportar := ui.NuevoBoton(p.t("batch.exportVcf", nil), func() { p.AlExportarSeleccion(sel) }).Con(ui.Secundario, ui.TallaSM)
		acciones = append(acciones, exportar)
	}
	destacar, quitar := boton("batch.star", ui.Secundario), boton("batch.unstar", ui.Secundario)
	destacar.AlPulsar = func() { p.AlLote(LoteDestacar, sel, nil) }
	quitar.AlPulsar = func() { p.AlLote(LoteQuitarDestacado, sel, nil) }
	acciones = append(acciones, destacar, quitar)
	if len(p.Etiquetas) > 0 {
		for _, x := range []struct {
			clave string
			que   AccionLote
		}{{"batch.addToLabel", LoteAnadirEtiqueta}, {"batch.removeFromLabel", LoteQuitarEtiqueta}} {
			b := boton(x.clave, ui.Secundario)
			b.IconoDetras, b.TamIcono = iconos.ChevronAbajo, 14
			que, rotulo := x.que, p.t(x.clave, nil)
			b.AlPulsar = func() {
				elementos := make([]fyne.CanvasObject, 0, len(p.Etiquetas))
				for _, g := range p.Etiquetas {
					grupo := g
					elementos = append(elementos, ui.NuevoElementoMenu(g.Name, iconos.Ninguno,
						func() { p.AlLote(que, sel, &grupo) }))
				}
				menuDe(b, rotulo, elementos...)
			}
			acciones = append(acciones, b)
		}
	}
	papelera := boton("batch.toTrash", ui.Peligro)
	papelera.AlPulsar = func() { p.AlLote(LoteAPapelera, sel, nil) }
	acciones = append(acciones, papelera)
	return ui.NuevaBarraDeLote(p.t("batch.selected", i18n.Vars{"count": n}), p.t("batch.region", nil),
		p.t("ui.bulk.clear", nil), acciones, p.AlSoltarMarcados)
}

// buscadorVivo es el campo de búsqueda, que NO se rehace al repintar: la pantalla se
// reconstruye con cada tecla y un campo nuevo perdería el foco y el cursor a mitad de
// palabra. Se crea una vez y se vuelve a colgar donde toque.
func (p *Pantalla) buscadorVivo() *ui.Entrada {
	if p.buscador == nil {
		p.buscador = ui.NuevaEntrada(p.t("list.search", nil))
		p.buscador.Icono = iconos.Buscar // `leadingIcon={<Icon name="search" size={16} />}`
		p.buscador.PonerTexto(p.Busqueda)
		p.buscador.AlCambiar(func(s string) {
			if p.AlBuscar != nil && !p.callando {
				p.AlBuscar(s)
			}
		})
	}
	// «Limpiar búsqueda» lo vacía desde fuera. `SetText` dispara `OnChanged`, y sin
	// callarlo eso repintaría la pantalla a mitad de construirla.
	if p.buscador.Texto() != p.Busqueda {
		p.callando = true
		p.buscador.PonerTexto(p.Busqueda)
		p.callando = false
	}
	return p.buscador
}

// bloqueBuscador es el `border-b p-3` con el campo de búsqueda y, debajo, la casilla
// de «seleccionar todo» con el recuento.
func (p *Pantalla) bloqueBuscador() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := []fyne.CanvasObject{p.buscadorVivo()}
	if p.hayRecuento() {
		todo := ui.NuevaCasilla(p.t("list.selectAll", nil), func(bool) {
			if p.AlMarcarTodo != nil {
				p.AlMarcarTodo()
			}
		})
		todo.SinEtiqueta = true
		// Todo marcado, o a medias: el `indeterminate` que la web pone por `ref`.
		todos, alguno := p.estadoMarcado()
		todo.Marcada, todo.Parcial = todos, alguno && !todos
		fila := container.New(filaRecuento{},
			todo,
			// `.sytue-meta`: `text-xs` fijado a `text-subtle`, con `tabular-nums`.
			ui.NuevoRotulo(p.recuentoConEtiqueta(), tema.TextoXS, ui.Normal, "text-subtle"),
		)
		hijos = append(hijos, ui.Espaciador(MargenRecuento),
			conRelleno(ui.Margen{Izq: SangriaRecuento, Der: SangriaRecuento}, fila))
	}
	bloque := ui.EnColumna(0, hijos...)
	// El filete de abajo es del bloque, no del campo.
	filete := canvas.NewRectangle(tema.Color("border", v))
	return container.New(&conFilete{}, conRelleno(ui.Margen{
		Izq: RellenoBuscador, Arr: RellenoBuscador, Der: RellenoBuscador, Abj: RellenoBuscador,
	}, bloque), filete)
}

// AltoBloqueBuscador es el `border-b p-3` entero: el campo (31,5), el `p-3` arriba y
// abajo y el filete; con recuento, además el `mt-3` y su línea de 14. Son 53,5 y 78.
func AltoBloqueBuscador(conRecuento bool) float32 {
	alto := 2*RellenoBuscador + ui.AltoCampo(ui.CampoMD) + tema.Borde
	if conRecuento {
		alto += MargenRecuento + AltoRecuento
	}
	return alto
}

// AltoRecuento es lo que mide la línea de «seleccionar todo»: la casilla (14) y la caja
// de línea de `.sytue-meta` (`text-xs`, 14). NO los 19,25 de `MedidaEleccion`: esos son
// de una casilla CON etiqueta, cuyo `<label>` lleva la caja de línea de `text-base`;
// esta no tiene texto dentro y su `<label>` es una rejilla de lo que mide el `<input>`.
var AltoRecuento = max(ui.LadoEleccion, tema.TextoXS.Linea)

// filaRecuento coloca la casilla y el recuento con el `gap-2.5 items-center` de la
// web, a su alto de 14.
type filaRecuento struct{}

var _ fyne.Layout = filaRecuento{}

func (filaRecuento) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := ui.LadoEleccion + HuecoRecuento
	if len(objs) > 1 {
		ancho += objs[1].MinSize().Width
	}
	return fyne.NewSize(ancho, AltoRecuento)
}

func (filaRecuento) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(0, (t.Height-ui.LadoEleccion)/2))
	objs[0].Resize(fyne.NewSize(ui.LadoEleccion, ui.LadoEleccion))
	m := objs[1].MinSize()
	objs[1].Move(fyne.NewPos(ui.LadoEleccion+HuecoRecuento, (t.Height-m.Height)/2))
	objs[1].Resize(fyne.NewSize(max(t.Width-ui.LadoEleccion-HuecoRecuento, 0), m.Height))
}

// estadoMarcado es el `allFilteredSelected` / `someFilteredSelected` de la web: sobre
// lo FILTRADO, no sobre la libreta entera.
func (p *Pantalla) estadoMarcado() (todos, alguno bool) {
	if len(p.Contactos) == 0 {
		return false, false
	}
	todos = true
	for _, c := range p.Contactos {
		if p.Marcados[c.ID] {
			alguno = true
		} else {
			todos = false
		}
	}
	return todos, alguno
}

// hayRecuento es la condición de la web: el recuento solo sale con la lista cargada y
// con contactos.
func (p *Pantalla) hayRecuento() bool {
	return p.Estado != ui.EstadoCargando && p.Total > 0
}

// recuento es el `countSummary`: con filtro, «X de Y»; sin él, «Y contactos». Se
// compone con claves de catálogo y plural por `Intl.PluralRules`, nunca con un
// `n === 1 ? … : …`. El filtro de DESTACADOS cuenta como filtro: sin eso, la vista
// «Destacados» decía «1.234 contactos» mientras enseñaba cinco.
func (p *Pantalla) recuento() string {
	filtrado := p.Busqueda != "" || p.EtiquetaActiva != "" || p.SoloDestacados
	if filtrado {
		return p.t("list.countFiltered", i18n.Vars{"shown": len(p.Contactos), "count": p.Total})
	}
	return p.t("list.count", i18n.Vars{"count": p.Total})
}

// recuentoConEtiqueta añade el nombre de la etiqueta activa por INTERPOLACIÓN: pegar
// « · Amigos» a mano no sobrevive a un idioma que ordene las palabras al revés.
func (p *Pantalla) recuentoConEtiqueta() string {
	if nombre := p.nombreEtiqueta(); p.EtiquetaActiva != "" && nombre != "" {
		return p.t("list.countWithLabel", i18n.Vars{"summary": p.recuento(), "label": nombre})
	}
	return p.recuento()
}

// cuerpoLista es la lista, o el estado que toque.
func (p *Pantalla) cuerpoLista() fyne.CanvasObject {
	switch {
	case p.Estado == ui.EstadoVacia || p.Estado == ui.EstadoExito && len(p.Contactos) == 0:
		refresco := p.Refresco
		refresco.Arriba = func() bool { return true }
		return ui.RefrescarAlTirar(ui.PanelEstado(ui.EstadoVacia, p.textosDeVacio(), nil), &refresco)
	case p.Estado != ui.EstadoExito:
		return ui.PanelEstado(p.Estado, p.textosDeEstado(), p.esqueletoLista())
	}
	return p.listaViva()
}

// listaViva es la lista VIRTUALIZADA, como la `VirtualList` de la web: una libreta
// corporativa son miles de tarjetas y pintarlas todas medía 25.601 elementos de DOM.
// Con `widget.List` solo existen las filas visibles, que se reciclan al desplazar.
// Como el buscador, se crea una vez: así conserva el desplazamiento al repintar.
func (p *Pantalla) listaViva() fyne.CanvasObject {
	if p.virtual == nil {
		p.virtual = widget.NewList(
			func() int { return len(p.filasLista) },
			func() fyne.CanvasObject { return NuevaFilaContacto() },
			func(i widget.ListItemID, o fyne.CanvasObject) {
				f, ok := o.(*FilaContacto)
				if !ok || i < 0 || i >= len(p.filasLista) {
					return
				}
				if ini := p.filasLista[i].cabecera; ini != "" {
					p.ponerCabecera(f, ini)
					return
				}
				if c := p.filasLista[i].contacto; c < len(p.Contactos) {
					f.Cabecera = ""
					p.tecladoDeFila(f, c)
					p.rellenarFila(f, p.Contactos[c])
				}
			},
		)
		p.Refresco.Arriba = func() bool { return p.virtual.GetScrollOffset() <= 0.5 }
		p.cuerpo = ui.RefrescarAlTirar(ui.EnvolverLista(p.virtual), &p.Refresco)
	}
	p.montarFilas()
	// El alto de cada fila: 49 con correo u organización, 46 sin ninguno, y 28 la
	// cabecera de una inicial.
	for i, fila := range p.filasLista {
		alto := AltoCabeceraLista
		if fila.cabecera == "" {
			alto = MedidaFilaContacto(subtituloDe(p.Contactos[fila.contacto]) != "")
		}
		p.virtual.SetItemHeight(i, alto)
	}
	p.virtual.Refresh()
	return p.cuerpo
}

// ---- Cabeceras A–Z (CONT-05) ----
//
// La lista virtual mezcla cabeceras y contactos, pero todo lo demás —el cursor, j/k, la
// selección, `IrAFila`— sigue hablando en índices de `p.Contactos`: las cabeceras no
// existen para el teclado, que así se las salta sin saber que están. `filaDe` traduce
// de contacto a fila de la lista para desplazar.

// filaLista es una fila de la lista virtual: la cabecera de una inicial, o un contacto.
type filaLista struct {
	cabecera string // no vacía: la cabecera de esa inicial
	contacto int    // índice en p.Contactos
}

// ConCabeceras dice si la lista se agrupa por inicial: solo sin búsqueda, etiqueta ni
// destacados. Con un filtro, la lista es un resultado, no la libreta, y una «A» con un
// solo contacto debajo no sitúa a nadie.
func (p *Pantalla) ConCabeceras() bool {
	return strings.TrimSpace(p.Busqueda) == "" && p.EtiquetaActiva == "" && !p.SoloDestacados
}

// montarFilas rehace las filas de la lista virtual a partir de `p.Contactos`.
func (p *Pantalla) montarFilas() {
	p.filasLista = p.filasLista[:0]
	p.filaDe = make([]int, len(p.Contactos))
	if !p.ConCabeceras() {
		for i := range p.Contactos {
			p.filaDe[i] = len(p.filasLista)
			p.filasLista = append(p.filasLista, filaLista{contacto: i})
		}
		return
	}
	for _, g := range GruposPorInicial(nombresDe(p.Contactos), p.locale()) {
		p.filasLista = append(p.filasLista, filaLista{cabecera: g.Inicial})
		for _, i := range g.Indices {
			p.filaDe[i] = len(p.filasLista)
			p.filasLista = append(p.filasLista, filaLista{contacto: i})
		}
	}
}

// porInicial devuelve la libreta en el orden de sus cabeceras: el de la colación, con
// lo que no empieza por letra al final. Es una copia: `filtrar` devuelve la libreta
// entera sin filtro, y reordenarla en su sitio cambiaría el estado de la app.
func porInicial(lista []api.Contact, l i18n.Locale) []api.Contact {
	fuera := make([]api.Contact, 0, len(lista))
	for _, g := range GruposPorInicial(nombresDe(lista), l) {
		for _, i := range g.Indices {
			fuera = append(fuera, lista[i])
		}
	}
	return fuera
}

func nombresDe(lista []api.Contact) []string {
	nombres := make([]string, len(lista))
	for i, c := range lista {
		nombres[i] = c.DisplayName()
	}
	return nombres
}

// ponerCabecera convierte una fila reciclada en la cabecera de una inicial. Sale del
// mapa de `filas`: no enseña ningún contacto y el teclado no debe encontrarla.
func (p *Pantalla) ponerCabecera(f *FilaContacto, inicial string) {
	delete(p.filas, f)
	f.Cabecera = inicial
	f.EsCursor = false
	f.Nombre, f.Subtitulo, f.Foto = "", "", nil
	f.Abierta, f.Marcada, f.Destacado = false, false, false
	f.AlAbrir, f.AlMarcar, f.AlDestacar = nil, nil, nil
	// La rueda puede reciclar en cabecera la fila ENFOCADA: el foco se queda en ella, y
	// sin esto ↑ ↓ Inicio Fin morirían ahí. Mueven desde el cursor, que no ha cambiado.
	f.AlMover = func(k fyne.KeyName) {
		if d := destinoDeTecla(k, p.CursorFila(), len(p.Contactos)); d >= 0 {
			p.IrAFila(d)
		}
	}
	f.Refresh()
}

// rellenarFila pone un contacto en una fila reciclada.
func (p *Pantalla) rellenarFila(f *FilaContacto, c api.Contact) {
	f.Nombre = c.DisplayName()
	f.Subtitulo = subtituloDe(c)
	f.Destacado = p.Destacados[api.CleanUID(c.UID)]
	f.Foto = nil
	if p.FotoDe != nil {
		f.Foto = p.FotoDe(c)
	}
	f.Abierta = p.Seleccionado != nil && p.Seleccionado.ID == c.ID
	f.Marcada = p.Marcados[c.ID]
	f.RotuloCasilla = p.t("list.select", i18n.Vars{"name": f.Nombre})
	// Con el nombre, como la casilla: «Destacar» a secas repetido cien veces no dice de
	// quién.
	f.RotuloEstrella = p.t("list.star", i18n.Vars{"name": f.Nombre})
	if f.Destacado {
		f.RotuloEstrella = p.t("list.unstar", i18n.Vars{"name": f.Nombre})
	}
	contacto := c
	f.AlAbrir = func() {
		if p.AlElegirContacto != nil {
			p.AlElegirContacto(contacto)
		}
	}
	f.AlMarcar = func() {
		if p.AlMarcar != nil {
			p.AlMarcar(contacto.ID)
		}
	}
	// Destacar desde la fila, sin abrir la ficha: solo con red, como la de la ficha.
	f.AlDestacar = nil
	if p.AlDestacar != nil && !p.SinConexion {
		f.AlDestacar = func() { p.AlDestacar(contacto) }
	}
	f.Refresh()
}

// refrescarLista vuelve a rellenar las filas visibles sin rehacer la pantalla: es lo
// que hace falta cuando llega la foto de una fila.
func (p *Pantalla) refrescarLista() {
	if p.virtual != nil {
		p.virtual.Refresh()
	}
}

// ---- Teclado (el `tabindex` itinerante de ContactsApp.tsx) ----
//
// La lista es UNA parada del tabulador —la fila del cursor— y las flechas la recorren,
// como un `listbox`. `widget.List` recicla las filas al desplazar, así que la fila no
// sabe quién es: se lo dice `tecladoDeFila` cada vez que la rellena, y `filas` lleva qué
// índice enseña cada fila pintada, que es como se encuentra la que hay que enfocar.

// CursorFila es la fila del cursor dentro de la lista filtrada; -1 sin filas.
func (p *Pantalla) CursorFila() int {
	if len(p.Contactos) == 0 {
		return -1
	}
	return min(max(p.Cursor, 0), len(p.Contactos)-1)
}

// tecladoDeFila le dice a una fila reciclada qué índice enseña y si es la del cursor.
func (p *Pantalla) tecladoDeFila(f *FilaContacto, i int) {
	if p.filas == nil {
		p.filas = map[*FilaContacto]int{}
	}
	antes, estaba := p.filas[f]
	p.filas[f] = i
	// La rueda puede reciclar la fila ENFOCADA para otro contacto: el foco se queda en
	// el widget, así que el cursor se va con él. Si no, Intro abriría un contacto y el
	// aro estaría en otro. Rellenarla con el MISMO índice no cuenta: es un repintado.
	if f.enfocada && estaba && antes != i {
		p.Cursor = i
	}
	f.EsCursor = i == p.CursorFila()
	f.AlMover = func(k fyne.KeyName) {
		if d := destinoDeTecla(k, p.filas[f], len(p.Contactos)); d >= 0 {
			p.IrAFila(d)
		}
	}
}

// destinoDeTecla es adónde lleva ↑ ↓ Inicio Fin desde la fila `i`; -1 si no mueve.
func destinoDeTecla(k fyne.KeyName, i, total int) int {
	if total <= 0 {
		return -1
	}
	switch k {
	case fyne.KeyDown:
		return min(i+1, total-1)
	case fyne.KeyUp:
		return max(i-1, 0)
	case fyne.KeyHome:
		return 0
	case fyne.KeyEnd:
		return total - 1
	default:
		return -1
	}
}

// FilaEnfocada es el índice de la fila con el foco del teclado, o -1.
func (p *Pantalla) FilaEnfocada() int {
	for f, i := range p.filas {
		if f.enfocada && i < len(p.Contactos) {
			return i
		}
	}
	return -1
}

// IrAFila lleva el cursor a la fila `i`, la trae a la vista y le da el foco. El orden
// importa: el gestor de foco se NIEGA a enfocar lo que está fuera del recorrido, así
// que primero la fila pasa a ser la del cursor y después se enfoca.
func (p *Pantalla) IrAFila(i int) {
	if i < 0 || i >= len(p.Contactos) || p.virtual == nil {
		return
	}
	p.Cursor = i
	fila := i
	if i < len(p.filaDe) {
		fila = p.filaDe[i]
	}
	p.virtual.ScrollTo(fila)
	// Rellena las filas pintadas con el cursor nuevo: la de `i` entra en el recorrido y
	// la anterior sale.
	p.virtual.Refresh()
	c := fyne.CurrentApp().Driver().CanvasForObject(p.virtual)
	if c == nil {
		return
	}
	// Puede haber más de una fila que dijo enseñar `i`: la lista guarda en reserva las
	// que el desplazamiento dejó sin uso, fuera del lienzo. El gestor de foco no enfoca
	// lo que no encuentra en el árbol, así que vale la que acaba con el foco.
	for f, j := range p.filas {
		if j == i && f.Visible() {
			c.Focus(f)
			if c.Focused() == fyne.Focusable(f) {
				return
			}
		}
	}
}

// EnfocarBuscador es «/»: el foco al buscador de la lista.
func (p *Pantalla) EnfocarBuscador() {
	if p.buscador == nil {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(p.buscador); c != nil {
		c.Focus(p.buscador)
	}
}

// subtituloDe es el `c.emails[0] ?? c.organization` de la web.
func subtituloDe(c api.Contact) string {
	if len(c.Emails) > 0 {
		return c.Emails[0].Email
	}
	return c.Organization
}

// esqueletoLista es `ContactListSkeleton`: seis filas con el ritmo de las de verdad
// —un círculo de 2rem y dos barras de 0,75 y 0,625 rem—, alternando los anchos de las
// barras (70/65 % y 55/80 %) para que no parezca una rejilla.
func (p *Pantalla) esqueletoLista() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, 6)
	for i := range 6 {
		a, b := float32(0.70), float32(0.65)
		if i%2 == 1 {
			a, b = 0.55, 0.80
		}
		filas = append(filas, container.New(&filaEsqueleto{Anchos: [2]float32{a, b}},
			ui.NuevoEsqueletoRedondo(LadoEsqueleto),
			ui.NuevoEsqueleto(0, BarraEsqueleto1), ui.NuevoEsqueleto(0, BarraEsqueleto2)))
	}
	return ui.EnColumna(0, filas...)
}

// La geometría de `ContactListSkeleton`.
const (
	// LadoEsqueleto es el `<Skeleton circle width="2rem">`: 32 con el rem de 16, lo
	// mismo que el avatar de verdad. Es lo que pinta la web.
	LadoEsqueleto = 2 * tema.Rem
	// BarraEsqueleto1 y BarraEsqueleto2 son los `height="0.75rem"` y `"0.625rem"`.
	BarraEsqueleto1 = 0.75 * tema.Rem
	BarraEsqueleto2 = 0.625 * tema.Rem
	// HuecoBarras es el `gap-1.5` entre las dos barras y RellenoBarras su `py-0.5`.
	HuecoBarras   = 1.5 * 0.25 * tema.Rem
	RellenoBarras = 0.5 * 0.25 * tema.Rem
)

// AltoFilaEsqueleto es `py-2` más lo más alto de sus hijos: el círculo de 28 manda
// sobre las dos barras con su hueco y su relleno (26,25).
var AltoFilaEsqueleto = 2*ui.AltoFila + max(LadoEsqueleto,
	BarraEsqueleto1+HuecoBarras+BarraEsqueleto2+2*RellenoBarras)

// filaEsqueleto coloca el círculo y las dos barras de una fila del esqueleto:
// `flex items-center gap-3 px-3 py-2`, y las barras en una columna `flex-1`.
type filaEsqueleto struct{ Anchos [2]float32 }

var _ fyne.Layout = (*filaEsqueleto)(nil)

func (f *filaEsqueleto) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*ui.MargenFila+LadoEsqueleto, AltoFilaEsqueleto)
}

func (f *filaEsqueleto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 3 {
		return
	}
	x := ui.MargenFila
	objs[0].Move(fyne.NewPos(x, (t.Height-LadoEsqueleto)/2))
	objs[0].Resize(fyne.NewSize(LadoEsqueleto, LadoEsqueleto))
	x += LadoEsqueleto + ui.HuecoFila
	util := max(t.Width-ui.MargenFila-x, 0)
	y := (t.Height - (BarraEsqueleto1 + HuecoBarras + BarraEsqueleto2)) / 2
	objs[1].Move(fyne.NewPos(x, y))
	objs[1].Resize(fyne.NewSize(util*f.Anchos[0], BarraEsqueleto1))
	objs[2].Move(fyne.NewPos(x, y+BarraEsqueleto1+HuecoBarras))
	objs[2].Resize(fyne.NewSize(util*f.Anchos[1], BarraEsqueleto2))
}

// textosDeEstado es la copia de los estados de la LISTA. El error lleva el aviso de
// 40 de la web (`<Icon name="warning" size={40} />`) en lugar de ilustración.
func (p *Pantalla) textosDeEstado() ui.TextosEstado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	aviso := iconos.Trazo(iconos.Aviso, 40, "text-subtle", v).Objeto()
	switch p.Estado {
	case ui.EstadoError:
		return ui.TextosEstado{
			Titulo:       p.t("list.errorTitle", nil),
			Descripcion:  p.t("list.errorDescription", nil),
			RotuloAccion: p.t("common.retry", nil),
			Accion:       p.AlReintentar,
			Ilustracion:  aviso,
		}
	case ui.EstadoSinConexion:
		return ui.TextosEstado{
			Titulo:       p.t("list.errorTitle", nil),
			Descripcion:  p.t("list.errorDescription", nil),
			RotuloAccion: p.t("common.retry", nil),
			Ilustracion:  aviso,
		}
	case ui.EstadoSinPermiso:
		return ui.TextosEstado{
			Titulo:      p.t("nav.booksError", nil),
			Descripcion: p.t("list.errorDescription", nil),
			Ilustracion: aviso,
		}
	case ui.EstadoExito, ui.EstadoCargando, ui.EstadoVacia:
	}
	return ui.TextosEstado{}
}

// textosDeVacio es el vacío de la lista, y cada filtro explica LO SUYO y ofrece su
// salida. No es cosmética: en «Destacados» sin destacados se leía «tu libreta está
// vacía» —con la libreta llena— y el único botón creaba un contacto que tampoco iba a
// salir ahí.
func (p *Pantalla) textosDeVacio() ui.TextosEstado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ilustracion := iconos.VacioContactos
	if p.Busqueda != "" {
		ilustracion = iconos.VacioBusqueda
	}
	t := ui.TextosEstado{
		Titulo:      p.t("list.emptyTitle", nil),
		Ilustracion: iconos.Pintar(ilustracion, v).Objeto(),
	}
	switch {
	case p.Busqueda != "":
		t.Descripcion = p.t("list.emptySearch", nil)
		t.RotuloAccion, t.Accion = p.t("list.clearSearch", nil), p.AlLimpiarFiltro
	case p.EtiquetaActiva != "":
		t.Descripcion = p.t("list.emptyLabel", i18n.Vars{"label": p.nombreEtiqueta()})
		t.RotuloAccion, t.Accion = p.t("list.clearFilter", nil), p.AlLimpiarFiltro
	case p.SoloDestacados:
		t.Descripcion = p.t("list.emptyStarred", nil)
		t.RotuloAccion, t.Accion = p.t("list.clearFilter", nil), p.AlLimpiarFiltro
	default:
		t.Descripcion = p.t("list.emptyBook", nil)
		if !p.SinConexion {
			// El único vacío cuyo botón es PRIMARIO y con su «+»: crear, no limpiar.
			t.RotuloAccion, t.Accion = p.t("nav.newContact", nil), p.AlCrearContacto
			t.Primaria = true
		}
	}
	return t
}

func (p *Pantalla) nombreEtiqueta() string {
	for _, g := range p.Etiquetas {
		if g.ID == p.EtiquetaActiva {
			return g.Name
		}
	}
	return ""
}

// conFilete pone el segundo hijo como filete de un píxel abajo del todo y da al
// primero el resto: es el `border-b` de un bloque.
type conFilete struct{}

var _ fyne.Layout = conFilete{}

func (conFilete) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde)
}

func (conFilete) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-tema.Borde, 0)))
	if len(objs) > 1 {
		objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	}
}

// panelConFilete pinta el fondo del panel, su contenido y el `border-r` de la derecha:
// el contenido ocupa 279 de los 280, que es lo que mide el `<section>` en el volcado.
type panelConFilete struct{}

var _ fyne.Layout = panelConFilete{}

func (panelConFilete) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 2 {
		return fyne.Size{}
	}
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width+tema.Borde, m.Height)
}

func (panelConFilete) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 3 {
		return
	}
	objs[0].Resize(t)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-tema.Borde, 0), t.Height))
	objs[2].Move(fyne.NewPos(t.Width-tema.Borde, 0))
	objs[2].Resize(fyne.NewSize(tema.Borde, t.Height))
}
