// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"
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
)

// Paridad web: `PanelBusqueda`, `PanelPapelera` y `NoteTree` de la app de notas.

// ---- El árbol de notas del panel secundario -------------------------------------

// ArbolNotas es el panel de en medio de la vista de árbol: las notas del cuaderno
// anidadas unas dentro de otras, y a la derecha la que se elige.
//
// La cabecera es la MISMA franja que la barra del cuaderno de al lado (`min-h-12`),
// porque son una sola franja partida en dos columnas y con 4 px de diferencia sus
// filetes no casaban.
type ArbolNotas struct {
	tr     *i18n.Translator
	locale i18n.Locale

	Notas        []Nota
	Seleccionada string
	// Recordatorios son los míos, por nota: el indicador de cada fila.
	Recordatorios map[string]api.NoteReminder
	Bloqueado     bool
	SoloLectura   bool
	Fase          Fase
	// Renombrando es la nota cuyo título se está cambiando, y Campo su campo: vive aquí
	// y no en la fila porque la fila se rehace en cada repintado y el campo tiene el
	// foco y lo escrito.
	Renombrando string
	Campo       *CampoEnLinea

	AlSeleccionar func(string)
	AlPlegar      func(Nota, bool)
	AlNueva       func()
	AlMenuNota    func(Nota, fyne.CanvasObject)
	AlRenombrar   func(Nota)

	// ultimo es el toque anterior, para reconocer el doble clic: la fila se rehace al
	// seleccionar, así que el segundo clic cae en OTRA fila y no puede acordarse ella.
	ultimo     string
	ultimoCu   time.Time
	desplazado *container.Scroll
}

// Sangria es el `paddingLeft: nivel × 14px` del árbol de notas. Son 14 px LITERALES en
// la hoja y no una unidad de la escala: `style={{paddingLeft: `${nivel * 14}px`}}`.
const Sangria = 14

// RellenoListaArbol es el `p-2` de la lista.
const RellenoListaArbol = 2 * 0.25 * tema.Rem

// NuevoArbolNotas crea el panel.
func NuevoArbolNotas(tr *i18n.Translator, l i18n.Locale) *ArbolNotas {
	return &ArbolNotas{tr: tr, locale: l}
}

// Objeto arma el panel con su cabecera y su lista.
func (a *ArbolNotas) Objeto(v tema.Variante) fyne.CanvasObject {
	cuenta := lineaTok(a.tr.T("notebook.notes", i18n.Vars{"count": len(a.Notas)}),
		tema.TextoSM, ui.Medio, "text-muted", v)
	derecha := []fyne.CanvasObject{}
	// «Nueva nota» solo por debajo de `md`: ahí la barra del cuaderno no se ve y este
	// es el único. En escritorio las dos franjas van juntas y serían dos botones iguales.
	if !a.SoloLectura && !a.Bloqueado {
		nueva := ui.NuevoBoton(a.tr.T("note.new", nil), func() { llamar0(a.AlNueva) })
		nueva.Variante, nueva.Talla, nueva.Icono, nueva.TamIcono = ui.Secundario, ui.TallaSM, iconos.Mas, 14
		derecha = append(derecha, container.New(soloEstrecho{}, nueva))
	}

	fondo := canvas.NewRectangle(tema.Color("surface", v))
	filete := canvas.NewRectangle(tema.Color("border", v))
	cabecera := container.New(&franja{}, fondo,
		container.New(ui.Fila{Hueco: HuecoBarra}, cuenta),
		container.New(ui.Fila{Hueco: HuecoBarra}, derecha...), filete)

	return container.New(ui.Pila{Elastico: 1}, cabecera, a.lista(v))
}

func (a *ArbolNotas) lista(v tema.Variante) fyne.CanvasObject {
	t := a.tr.T
	// `text-text-muted p-4 text-base`: un párrafo, no el estado vacío grande.
	parrafo := func(clave string) fyne.CanvasObject {
		return container.New(ui.Margen{Izq: MargenLienzo, Der: MargenLienzo, Arr: MargenLienzo, Abj: MargenLienzo},
			container.New(ui.Columna{}, ui.NuevoParrafo(t(clave, nil), tema.TextoBase, ui.Normal, "text-muted", v)))
	}
	switch a.Fase {
	case Cargando:
		return centrar(ui.NuevaRueda(24, "text-muted", v), v)
	case Fallo, SinSesion, Prohibida, SinConexion:
		return parrafo("error.loadFailed")
	case Vacia, Exito:
	}
	arbol := Arbol(a.Notas)
	if len(arbol) == 0 {
		return parrafo("empty.noNotes.title")
	}
	var filas []fyne.CanvasObject
	var bajar func(nodos []Nodo, nivel int)
	bajar = func(nodos []Nodo, nivel int) {
		for _, n := range nodos {
			filas = append(filas, a.fila(n, nivel, v))
			if !n.Nota.Plegada {
				bajar(n.Hijos, nivel+1)
			}
		}
	}
	bajar(arbol, 0)
	dentro := container.New(ui.Margen{Izq: RellenoListaArbol, Der: RellenoListaArbol, Arr: RellenoListaArbol, Abj: RellenoListaArbol},
		container.New(ui.Columna{}, filas...))
	// El MISMO desplazador en cada repintado: con uno nuevo, abrir una nota del fondo
	// de la lista la devolvía arriba del todo.
	if a.desplazado == nil {
		a.desplazado = container.NewVScroll(dentro)
	} else {
		a.desplazado.Content = dentro
		a.desplazado.Refresh()
	}
	return a.desplazado
}

// fila es una nota del árbol: título arriba y, debajo, la fecha y la primera línea.
//
// Un árbol de solo títulos no dice cuál es la de ayer ni de qué va, que es por lo que
// la web pone las dos cosas. En un cuaderno cifrado y cerrado no hay texto que enseñar.
func (a *ArbolNotas) fila(n Nodo, nivel int, v tema.Variante) fyne.CanvasObject {
	nota := n.Nota
	titulo := nota.Titulo
	switch {
	case a.Bloqueado:
		titulo = a.tr.T("crypto.locked", nil)
	case titulo == "":
		titulo = a.tr.T("note.new", nil)
	}
	meta := Fecha(a.locale, nota.Tocada)
	if cuerpo := nota.Resumen(72); cuerpo != "" && !a.Bloqueado {
		meta += " · " + cuerpo
	}
	f := NuevaFilaNotaArbol(titulo)
	f.Meta, f.Nivel, f.Activa = meta, nivel, nota.ID == a.Seleccionada
	f.Hecha, f.Fijada = nota.Marcada, nota.Fijada
	if r, ok := a.Recordatorios[nota.ID]; ok {
		f.Recordatorio = indicadorRecordatorio(a.tr, r, nil, v)
	}
	// El glifo del árbol NO es el de la tarjeta: el icono elegido, la tarea, o el
	// enlace para todo lo que no sea texto.
	switch {
	case GlifoPorNombre(nota.Icono) != "":
		f.Glifo, f.TintaGlifo = GlifoPorNombre(nota.Icono), "primary-text"
	case nota.Tipo == "todo":
		f.Glifo, f.TintaGlifo = TareaD, "text-subtle"
	case nota.Tipo != "text":
		f.Glifo, f.TintaGlifo = EnlaceD, "text-subtle"
	}
	for _, e := range nota.Estados {
		f.Puntos = append(f.Puntos, hexO(e.Color, tema.Color("text-muted", v)))
	}
	if len(n.Hijos) > 0 {
		rotulo := a.tr.T("tree.collapse", nil)
		if nota.Plegada {
			rotulo = a.tr.T("tree.expand", nil)
		}
		f.Pliegue = botonPliegue(rotulo, nota.Plegada, f.Refresh,
			func() { llamar2(a.AlPlegar, nota, !nota.Plegada) })
	}
	puede := !a.SoloLectura && !a.Bloqueado
	if puede {
		f.Mas = nuevoBotonMas(a.tr.T("common.more", nil), f.Refresh)
		f.Mas.TamIcono = GlifoFilaArbol
		mas := f.Mas
		f.Mas.AlPulsar = func() { llamar2(a.AlMenuNota, nota, fyne.CanvasObject(mas)) }
	}
	if nota.ID == a.Renombrando && a.Campo != nil {
		f.Campo = a.Campo
	}
	f.AlPulsar = func() {
		// El doble clic no pisa la selección: el primer clic ya selecciona, y el
		// segundo abre la edición sobre la nota que ya estaba abierta.
		ahora := time.Now()
		doble := a.ultimo == nota.ID && ahora.Sub(a.ultimoCu) < DobleToque
		a.ultimo, a.ultimoCu = nota.ID, ahora
		if doble && puede {
			a.ultimo = ""
			llamar1(a.AlRenombrar, nota)
			return
		}
		llamar1(a.AlSeleccionar, nota.ID)
	}
	return f
}

// soloEstrecho enseña a su hijo solo con la VENTANA por debajo de `md` (768, rem de 16):
// es el `md:hidden` de la web, que mide la ventana y no la caja.
type soloEstrecho struct{}

func (soloEstrecho) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 || !objs[0].Visible() {
		return fyne.Size{}
	}
	return objs[0].MinSize()
}

func (soloEstrecho) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
			if c.Size().Width < ui.Estrecho {
				o.Show()
			} else {
				o.Hide()
			}
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// sangrado desplaza su hijo a la derecha sin recortarlo, que es el `paddingLeft` de la
// fila.
type sangrado struct{ lado float32 }

var _ fyne.Layout = (*sangrado)(nil)

func (s *sangrado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+s.lado, m.Height)
}

func (s *sangrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	objs[0].Move(fyne.NewPos(s.lado, 0))
	objs[0].Resize(fyne.NewSize(max(t.Width-s.lado, 0), t.Height))
}

// ---- Búsqueda ---------------------------------------------------------------------

// Busqueda es el panel de buscar en TODAS las notas indexables, que es la misma
// búsqueda que federa el Hub.
//
// Donde la web pone los resultados o «No se ha encontrado nada», aquí va lo que diga la
// fase. La web convierte un fallo en una lista vacía y dice «no se ha encontrado nada»
// de algo que no ha podido buscar; aquí se dice lo que ha pasado, en el mismo sitio y con
// las claves de la app.
type Busqueda struct {
	tr *i18n.Translator

	Consulta string
	// Buscada es la consulta de los resultados que se enseñan, no lo que hay escrito: es
	// lo que se resalta en ellos.
	Buscada    string
	Resultados []api.NoteHit
	Fase       Fase
	// Archivados es «Incluir cuadernos archivados»: sin ella lo archivado no sale.
	Archivados bool
	// Hashtags son los `#hashtags` de las notas, del más usado al menos: pulsar uno es
	// buscarlo.
	Hashtags []api.Hashtag

	AlBuscar func(string)
	AlAbrir  func(api.NoteHit)

	// campo es el mismo en cada repintado: llegar los resultados no debe quitarle el
	// foco a quien sigue escribiendo.
	campo *ui.Entrada
}

// NuevaBusqueda crea el panel.
func NuevaBusqueda(tr *i18n.Translator) *Busqueda { return &Busqueda{tr: tr} }

// Campo es el buscador, para darle el foco al abrir el panel (`autofocus`).
func (b *Busqueda) Campo() *ui.Entrada {
	if b.campo == nil {
		// Sin caja, sobre una raya: la misma línea que la captura del día (`.notes-capture`).
		b.campo = ui.NuevaEntrada(b.tr.T("search.placeholder", nil)).ConPropia(ui.CampoPropio{
			MargenX: tema.Espacio(1), MargenY: (tema.Espacio(9) - 2*tema.Borde - tema.TextoBase.Linea) / 2,
			Texto: tema.TextoBase, Plano: true,
		})
		b.campo.Rotulo = b.tr.T("search.placeholder", nil)
		b.campo.PonerTexto(b.Consulta)
		b.campo.AlCambiar(func(s string) { b.Consulta = s })
		// Intro envía el formulario, que es lo que hace un `<form onSubmit>`.
		b.campo.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			llamar1(b.AlBuscar, b.Consulta)
			return true
		}
	}
	return b.campo
}

// Objeto arma la página: el título, el formulario, la casilla con la pista y los resultados.
func (b *Busqueda) Objeto(v tema.Variante) fyne.CanvasObject {
	t := b.tr.T
	boton := ui.NuevoBoton(t("search.title", nil), func() { llamar1(b.AlBuscar, b.Consulta) })
	boton.Variante, boton.Talla = ui.Fantasma, ui.TallaSM
	fila := container.New(&filaElastica{hueco: tema.Espacio(1), elastico: 1},
		celdaFila(Icono(iconos.Buscar, 16, "text-subtle", v)), b.Campo(), boton)
	piezas := []fyne.CanvasObject{
		container.New(&subrayado{py: tema.Espacio(1)}, canvas.NewRectangle(tema.Color("border-subtle", v)), fila),
		container.New(ui.FilaAuto{Hueco: tema.Espacio(4), Derecha: 1, Partir: true, SaltoFila: tema.Espacio(1)},
			b.casillaArchivados(),
			lineaTok(t("search.hint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)), // `sytue-meta`
	}
	// Sin hashtags la tira no está: vacía, dejaría su hueco entre la casilla y la lista.
	if len(b.Hashtags) > 0 {
		piezas = append(piezas, b.hashtags(v))
	}
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(3)}, append(piezas, b.lista(v))...)
	return paginaDe(t("sidebar.search", nil), nil, cuerpo, v)
}

// hashtags es la tira de `#hashtags` con su recuento (`.notes-seg` de PanelBusqueda): el
// elegido es el que se está buscando.
func (b *Busqueda) hashtags(v tema.Variante) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(b.Hashtags))
	for _, h := range b.Hashtags {
		q := "#" + h.Tag
		items = append(items, segmento(q, strconv.Itoa(h.Count), b.Buscada == q, v, func() {
			b.Campo().PonerTexto(q)
			b.Consulta = q
			llamar1(b.AlBuscar, q)
		}))
	}
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(0.5), Partir: true, SaltoFila: tema.Espacio(0.5)}, items...)
}

// casillaArchivados vuelve a buscar en cuanto cambia, como el `onChange` de la web.
func (b *Busqueda) casillaArchivados() fyne.CanvasObject {
	c := ui.NuevaCasilla(b.tr.T("archive.includeInSearch", nil), func(si bool) {
		b.Archivados = si
		llamar1(b.AlBuscar, b.Consulta)
	})
	c.Marcada = b.Archivados
	return container.New(ui.Fila{}, c)
}

// lista son los resultados, o en su sitio lo que diga la fase.
func (b *Busqueda) lista(v tema.Variante) fyne.CanvasObject {
	t := b.tr.T
	aviso := func(clave, token string) fyne.CanvasObject {
		return lineaTok(t(clave, nil), tema.TextoBase, ui.Normal, token, v)
	}
	switch b.Fase {
	case Cargando:
		return container.New(ui.Fila{}, ui.NuevaRueda(24, "text-muted", v))
	case Fallo:
		return aviso("error.loadFailed", "danger-text")
	case SinConexion:
		return aviso("offline.status", "text-muted")
	case SinSesion:
		return aviso("error.sessionExpired", "text-muted")
	case Prohibida:
		return aviso("forbidden.title", "text-muted")
	case Vacia, Exito:
	}
	if b.Fase == Vacia || len(b.Resultados) == 0 {
		return aviso("search.empty", "text-subtle")
	}
	filas := make([]fyne.CanvasObject, 0, len(b.Resultados))
	for _, h := range b.Resultados {
		filas = append(filas, filaHit(h, b.Buscada, b.AlAbrir, v))
	}
	return filasDePagina(filas...)
}

// filaHit es un resultado (`ListaDeHits` de Archivo.tsx, `.notes-row`): el glifo en su
// columna, el título y —si lo hay— el fragmento en `sytue-meta line-clamp-2`, y el
// cuaderno a la derecha. Una nota sin título es su propio texto, no el nombre de su
// cuaderno. La comparten la búsqueda y «Fijadas»; `buscada` es lo que se resalta.
func filaHit(h api.NoteHit, buscada string, alAbrir func(api.NoteHit), v tema.Variante) fyne.CanvasObject {
	titulo, fragmento := h.Title, h.Snippet
	if titulo == "" {
		titulo, _, _ = strings.Cut(strings.TrimSpace(h.Snippet), "\n")
		fragmento = ""
		if titulo == "" {
			titulo = h.Notebook
		}
	}
	lineas := []fyne.CanvasObject{ui.NuevoParrafo(titulo, tema.TextoBase, ui.Normal, "text", v).Recortado(1).Resaltando(buscada, colorResalte(v))}
	if fragmento != "" {
		lineas = append(lineas, ui.NuevoParrafo(fragmento, tema.TextoXS, ui.Normal, "text-subtle", v).Recortado(2).Resaltando(buscada, colorResalte(v)))
	}
	piezas := []fyne.CanvasObject{glifoDeHit(h.Kind, v), container.New(ui.Columna{}, lineas...)}
	if h.Notebook != "" {
		piezas = append(piezas, lineaTok(h.Notebook, tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	o := NuevaOpcion(titulo, container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1, arriba: true}, piezas...), func() { llamar1(alAbrir, h) })
	o.PX, o.PY, o.Superficie = tema.Espacio(2), tema.Espacio(1.5), "bg"
	return o
}

// filaElastica es `flex gap-N` con UN hijo en `flex-1` —el primero, salvo que se diga
// otro— y los demás a su medida. `arriba` es `items-start`; si no, `items-center`.
type filaElastica struct {
	hueco    float32
	elastico int
	arriba   bool
}

var _ fyne.Layout = (*filaElastica)(nil)

func (f *filaElastica) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += f.hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f *filaElastica) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	e := min(max(f.elastico, 0), len(objs)-1)
	var fijo float32
	for i, o := range objs {
		if i != e {
			fijo += o.MinSize().Width
		}
	}
	fijo += f.hueco * float32(len(objs)-1)
	var x float32
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i == e {
			w = max(t.Width-fijo, 0)
		}
		y := (t.Height - m.Height) / 2
		if f.arriba {
			y = 0
		}
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + f.hueco
	}
}

// ---- Papelera ----------------------------------------------------------------------

// Papelera es la vista de lo borrado: restaurar o borrar del todo.
type Papelera struct {
	tr *i18n.Translator

	Notas []Nota
	Fase  Fase

	AlVaciar    func()
	AlRestaurar func(Nota)
	AlPurgar    func(Nota)
}

// NuevaPapelera crea la vista.
func NuevaPapelera(tr *i18n.Translator) *Papelera { return &Papelera{tr: tr} }

// Objeto arma la página en la fase en la que esté.
func (p *Papelera) Objeto(v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	if o, ok := estadoDeFase(p.tr, p.Fase, "trash.title", "trash.empty", v); ok {
		return o
	}
	vaciar := ui.NuevoBoton(t("trash.emptyAll", nil), func() { llamar0(p.AlVaciar) })
	vaciar.Variante, vaciar.Talla = ui.Fantasma, ui.TallaSM

	filas := make([]fyne.CanvasObject, 0, len(p.Notas))
	for _, n := range p.Notas {
		filas = append(filas, p.fila(n, v))
	}
	return paginaDe(t("trash.title", nil), vaciar, filasDePagina(filas...), v)
}

// fila es una nota de la papelera (`.notes-row`): su glifo, su título —o su texto, si no
// lo tiene— y los dos botones, en fantasma.
//
// «Borrar del todo» va en fantasma y no en peligro, igual que en la web: el rojo se
// reserva para la CONFIRMACIÓN, que es donde de verdad se pierde algo.
func (p *Papelera) fila(n Nota, v tema.Variante) fyne.CanvasObject {
	nombre := n.Titulo
	if nombre == "" {
		nombre = resumenNota("", n.Contenido, maxResumen)
	}
	if nombre == "" {
		nombre = p.tr.T("note.untitled", nil)
	}
	restaurar := ui.NuevoBoton(p.tr.T("trash.restore", nil), func() { llamar1(p.AlRestaurar, n) })
	restaurar.Variante, restaurar.Talla = ui.Fantasma, ui.TallaXS
	purgar := ui.NuevoBoton(p.tr.T("trash.purge", nil), func() { llamar1(p.AlPurgar, n) })
	purgar.Variante, purgar.Talla = ui.Fantasma, ui.TallaXS
	return container.New(ui.Margen{Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5), Izq: tema.Espacio(2), Der: tema.Espacio(2)},
		container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1},
			glifoDeHit(n.Tipo, v), lineaTok(nombre, tema.TextoBase, ui.Normal, "text", v), restaurar, purgar))
}

// enCaja mete un hijo dentro de una `ui.Caja` con su relleno: es el `rounded-md border
// px-3 py-2` de una fila suelta.
type enCaja struct {
	caja   *ui.Caja
	margen float32
}

var _ fyne.Layout = (*enCaja)(nil)

func (e *enCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	if n := len(objs); n > 0 {
		m = objs[n-1].MinSize()
	}
	return fyne.NewSize(m.Width+2*e.margen, m.Height+2*e.margen)
}

func (e *enCaja) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	if len(objs) == 0 {
		return 2 * e.margen
	}
	return ui.AltoA(objs[len(objs)-1], max(ancho-2*e.margen, 0)) + 2*e.margen
}

func (e *enCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	e.caja.Redimensionar(t)
	if n := len(objs); n > 0 {
		o := objs[n-1]
		o.Move(fyne.NewPos(e.margen, e.margen))
		o.Resize(fyne.NewSize(max(t.Width-2*e.margen, 0), max(t.Height-2*e.margen, 0)))
	}
}
