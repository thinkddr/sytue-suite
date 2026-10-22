// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: apps/frontend/apps/notes/src/components/NoteCanvas.tsx.

// EL LIENZO: donde viven las notas de un cuaderno, en la disposición que ese cuaderno
// tenga —libre, columnas, lista o árbol—.
//
// Las cuatro comparten tipo porque comparten todo lo que cuesta: cargar, filtrar,
// descifrar, seleccionar. Lo único que cambia es DÓNDE se pinta cada tarjeta, y eso son
// las quince líneas de `reparto`. Es la misma decisión de la web, literal.
//
// Medidas, de la hoja:
//
//	lista      `grid gap-2.5 p-4`, y `mx-auto max-w-3xl` salvo en árbol
//	columnas   `grid grid-cols-3 items-start gap-3 p-4`, columnas con `gap-2.5`
//	libre      `relative min-h-full p-4`, cada nota en su (x, y) y con su ancho
const (
	// MargenLienzo es `p-4`, HuecoLista es `gap-2.5` y HuecoColumnas es `gap-3`.
	MargenLienzo  = 4 * 0.25 * tema.Rem
	HuecoLista    = 2.5 * 0.25 * tema.Rem
	HuecoColumnas = 3 * 0.25 * tema.Rem
	// AnchoLectura es `max-w-3xl` (48 rem): el ancho al que se acota la lista cuando
	// NO llena el panel.
	AnchoLectura = 48 * tema.Rem
)

// Lienzo es el panel principal del cuaderno abierto.
type Lienzo struct {
	tr *i18n.Translator

	// AnchoVentana decide el `px-4 sm:px-6` de la columna del día; 0 es escritorio.
	AnchoVentana float32

	Cuaderno Cuaderno
	Notas    []Nota
	// Recordatorios son los míos, por nota; AlRecordatorio abre el diálogo.
	Recordatorios  map[string]api.NoteReminder
	AlRecordatorio func(Nota)
	Fase           Fase
	// Seleccionada es el ID de la nota con el anillo.
	Seleccionada string
	// Bloqueado es el cuaderno cifrado y cerrado; SinRed y SoloLectura apagan lo que
	// escribe.
	Bloqueado   bool
	SoloLectura bool
	// SinMenu quita el «…» de las tarjetas sin apagar la escritura: una nota que me han
	// compartido con edición (`sinMenu` de NoteCard.tsx).
	SinMenu bool
	// Llenar es la vista de árbol: UNA nota a todo el panel.
	Llenar bool
	// Renombrando es la nota cuyo título se está cambiando, y CampoTitulo su campo, que
	// sobrevive a los repintados con el foco y lo escrito.
	Renombrando string
	CampoTitulo *CampoEnLinea
	// Editando es la nota abierta en el editor, y Editor el editor, que sobrevive a los
	// repintados con el foco, el cursor y lo escrito.
	Editando string
	Editor   *EditorNota
	// Ancha es la nota que se escribe, ensanchada (`ancha` de NoteCard.tsx); se olvida
	// al cerrar el editor.
	Ancha bool

	AlSeleccionar func(string)
	AlMarcar      func(Nota, bool)
	// AlPlegarLista pliega el elemento de lista i de una nota (plegar.go) y Previa da la
	// ficha de un enlace a otra nota (previa.go).
	AlPlegarLista   func(Nota, int)
	Previa          func(href string, listo func()) (VistaPrevia, bool)
	portal          *portal
	AlMenuNota      func(Nota, fyne.CanvasObject)
	AlCrearPrimera  func()
	AlReintentar    func()
	AlTecla         func(rune)
	AlTeclaEspecial func(*fyne.KeyEvent)
	AlRenombrar     func(Nota)
	AlEditar        func(Nota)
	AlEnlace        func(href string)
	AlEnsanchar     func()
	Imagen          func(url string) imagenRemota
	Documento       func(id string) docRemoto
	// vivas son las tarjetas del último repintado, por nota: a las que llevar la vista.
	vivas map[string]*Tarjeta
	// AlMoverLibre es soltar una nota arrastrada en el lienzo libre, en su nuevo (x, y).
	AlMoverLibre func(Nota, fyne.Position)
	// AlSoltarEnDia es soltar el asa de una nota del diario, con el puntero en
	// coordenadas absolutas: sobre un día de la barra de semana, se mueve allí.
	AlSoltarEnDia   func(Nota, fyne.Position)
	AlDescargar     func(Adjunto)
	AlQuitarAdjunto func(Nota, Adjunto)
}

// NuevoLienzo crea el lienzo vacío.
func NuevoLienzo(tr *i18n.Translator) *Lienzo { return &Lienzo{tr: tr} }

// Objeto arma el panel en la fase en la que esté.
//
// LOS SEIS ESTADOS, cada uno con su clave de catálogo y su salida:
//
//	carga            la rueda centrada (`<Spinner size={24}>` de la web)
//	vacío            el estado vacío con «nueva nota», si se puede crear
//	error            el estado vacío con «reintentar»
//	sin conexión     lo mismo, sin botón: reintentar sin red no arregla nada
//	permiso denegado el texto de `forbidden.*` del catálogo COMÚN, sin acción
//	éxito            las notas
//
// La sesión caducada es el séptimo, y la web la distingue del error de red porque lo
// que hay que hacer es otro: volver a entrar.
func (l *Lienzo) Objeto(v tema.Variante) fyne.CanvasObject {
	t := l.tr.T
	if l.Llenar && len(l.Notas) == 0 {
		// La vista de árbol sin nota elegida. Va ANTES que las fases, como en la web
		// (`enArbol && !notaSeleccionada`): el panel de la derecha no carga nada por su
		// cuenta, enseña la nota que se elija en el árbol.
		return centrar(l.elige(v), v)
	}
	switch l.Fase {
	case Cargando:
		return centrar(ui.NuevaRueda(24, "text-muted", v), v)
	case Fallo:
		return centrar(l.vacio(t("error.loadFailed", nil), "", t("common.retry", nil),
			l.AlReintentar, v), v)
	case SinConexion:
		// Sin botón: reintentar sin red no arregla nada y ofrecerlo es prometer algo
		// que no se puede cumplir.
		return centrar(l.vacio(t("offline.status", nil), t("offline.description", nil),
			"", nil, v), v)
	case SinSesion:
		return centrar(l.vacio(t("error.sessionExpired", nil), t("error.signInAgain", nil),
			"", nil, v), v)
	case Prohibida:
		return centrar(l.vacio(t("forbidden.title", nil), t("forbidden.description", nil),
			"", nil, v), v)
	case Exito:
		// Las notas: se reparten debajo del switch.
	case Vacia:
		if l.diario() {
			// Un día vacío no es un hueco que adornar: la captura está justo encima
			// (`mx-auto max-w-[51rem] px-4 py-6 sm:px-6`, una línea en `text-subtle`).
			p := ui.NuevoParrafo(t("empty.noNotes.title", nil), tema.TextoBase, ui.Normal, "text-subtle", v)
			return container.New(&acotado{ancho: AnchoLectura + 2*tema.Espacio(6)},
				container.New(ui.Margen{Izq: l.margenDiario(), Der: l.margenDiario(), Arr: tema.Espacio(4), Abj: tema.Espacio(4)}, p))
		}
		return l.vacioSinNotas(v)
	}
	return l.reparto(v)
}

// LadoIlustracion es el `size = 160` por defecto de `Illustration`: el ancho máximo al
// que la web pinta el dibujo (el SVG declara 200).
const LadoIlustracion = 160

// elige es el panel de la vista en ÁRBOL sin ninguna nota elegida: el mismo vacío que
// Mail («Selecciona una conversación») y Contacts, con la ilustración de la suite y el
// botón de crear que la frase ofrece («…o crea una nueva»). Era un panel en blanco; la
// web tenía un glifo de 40 sin botón hasta el PR #841.
func (l *Lienzo) elige(v tema.Variante) *ui.Vacio {
	t := l.tr.T
	x := ui.NuevoVacio(t("tree.pick", nil), t("tree.pickHint", nil))
	x.Ilustracion = iconos.Pintar(iconos.VacioNotas, v).A(LadoIlustracion).Objeto()
	if l.AlCrearPrimera != nil && !l.SoloLectura && !l.Bloqueado {
		b := ui.NuevoBoton(t("note.new", nil), l.AlCrearPrimera).Con(ui.Primario, ui.TallaSM)
		b.Icono = iconos.Mas
		x.Accion = b
	}
	return x
}

// vacioSinNotas es el cuaderno vacío de NoteCanvas.tsx: la ilustración de la suite, el
// botón de crear —un `<button>` suelto `bg-primary rounded-md px-3 py-1.5 text-base
// font-medium`, que NO es ninguna talla de `Button`: 29,75 de alto— y, debajo, la pista
// de soltar o pegar (`sytue-meta text-center`) con `gap-2`.
func (l *Lienzo) vacioSinNotas(v tema.Variante) fyne.CanvasObject {
	t := l.tr.T
	x := ui.NuevoVacio(t("empty.noNotes.title", nil), t("empty.noNotes.body", nil))
	x.Ilustracion = iconos.Pintar(iconos.VacioNotas, v).A(LadoIlustracion).Objeto()
	if l.SoloLectura {
		return x
	}
	if l.AlCrearPrimera != nil && !l.Bloqueado {
		rotulo := t("note.new", nil)
		b := NuevaOpcion(rotulo, lineaTok(rotulo, tema.TextoBase, ui.Medio, "on-primary", v), l.AlCrearPrimera)
		b.PX, b.PY = 3*0.25*tema.Rem, 1.5*0.25*tema.Rem
		b.Activa, b.FondoActivo, b.BordeActivo = true, tema.Color("primary", v), tema.Color("primary", v)
		x.Accion = b
	}
	pista := lineaTok(t("capture.hint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)
	return container.New(&vacioConPista{v: x}, x, pista)
}

// vacioConPista coloca un estado vacío y, `gap-2` debajo, una línea centrada; los dos
// juntos en medio del panel, que es el `grid justify-items-center gap-2` de la web.
type vacioConPista struct{ v *ui.Vacio }

func (l *vacioConPista) MinSize(objs []fyne.CanvasObject) fyne.Size {
	p := objs[1].MinSize()
	return fyne.NewSize(p.Width, l.v.MedidaVacio(p.Width)+2*0.25*tema.Rem+p.Height)
}

func (l *vacioConPista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	pista := objs[1].MinSize()
	alto := l.v.MedidaVacio(t.Width)
	y := max((t.Height-alto-2*0.25*tema.Rem-pista.Height)/2, 0)
	objs[0].Move(fyne.NewPos(0, y))
	objs[0].Resize(fyne.NewSize(t.Width, alto))
	objs[1].Move(fyne.NewPos((t.Width-pista.Width)/2, y+alto+2*0.25*tema.Rem))
	objs[1].Resize(pista)
}

// vacio arma un estado vacío con su ilustración y, si la hay, su acción.
func (l *Lienzo) vacio(titulo, cuerpo, cta string, accion func(), v tema.Variante) *ui.Vacio {
	x := ui.NuevoVacio(titulo, cuerpo)
	x.Ilustracion = Glifo(NotaD, 40, "text-subtle", v)
	if cta != "" && accion != nil {
		x.Accion = ui.NuevoBoton(cta, accion)
	}
	return x
}

// reparto coloca las tarjetas según la disposición del cuaderno.
//
// porque es el parámetro de todos los `Objeto`/`reparto` del paquete y quitarlo de uno
// obliga a mirar cuál lo lleva cada vez.
//
//nolint:unparam // `v` no se usa HOY porque las tarjetas se tiñen solas; se queda
func (l *Lienzo) reparto(v tema.Variante) fyne.CanvasObject {
	tarjetas := make([]fyne.CanvasObject, 0, len(l.Notas))
	var arrastrables []*Tarjeta
	// En lista, cada sublista va justo debajo de su nota y sangrada (`conSangria` de la web).
	notas, niveles := l.Notas, map[string]int(nil)
	if l.filas() {
		notas, niveles = conSangria(l.Notas)
	}
	for _, n := range notas {
		t := l.tarjeta(n)
		t.Nivel = niveles[n.ID]
		if l.Cuaderno.Vista == Libre && !l.SoloLectura {
			t.AlSoltar = func(p fyne.Position) {
				llamar2(l.AlMoverLibre, n, fyne.NewPos(max(p.X-MargenLienzo, 0), max(p.Y-MargenLienzo, 0)))
			}
			arrastrables = append(arrastrables, t)
		} else if t.Diario && !l.SoloLectura && !l.Cuaderno.Cifrado && l.AlSoltarEnDia != nil {
			t.AlSoltar = func(fyne.Position) {
				// El puntero va sobre el asa, que está en la esquina de la tarjeta.
				en := fyne.CurrentApp().Driver().AbsolutePositionForObject(t).AddXY(PosAsa+GlifoAsa/2, PosAsa+GlifoAsa/2)
				llamar2(l.AlSoltarEnDia, n, en)
			}
		}
		tarjetas = append(tarjetas, t)
	}
	switch l.Cuaderno.Vista {
	case Libre:
		// Cada nota donde se soltó, con su propio ancho. El lienzo CRECE con lo que
		// haya abajo del todo (`min-h-full`): con un alto fijo, una nota arrastrada al
		// fondo quedaría fuera de alcance.
		c := container.New(&libre{}, tarjetas...)
		for _, t := range arrastrables {
			t.alEmpezar = func() { alFrente(c, t) }
		}
		return c
	case Columnas:
		cols := make([]fyne.CanvasObject, NumColumnas)
		for i := range cols {
			var dentro []fyne.CanvasObject
			for j, n := range l.Notas {
				// Una columna fuera de rango cae en la última en vez de desaparecer,
				// que es lo que hace el `Math.min(Math.max(...))` de la web.
				if min(max(n.Col, 0), NumColumnas-1) == i {
					dentro = append(dentro, tarjetas[j])
				}
			}
			cols[i] = container.New(columnaTarjetas{hueco: HuecoLista}, dentro...)
		}
		// `grid-cols-1 md:grid-cols-2 lg:grid-cols-3`: en el teléfono las tres columnas
		// van una debajo de otra, y en una tableta la tercera baja a la segunda fila.
		return conMargen(container.New(&rejillaVentana{hueco: HuecoColumnas, cortes: ui.Cortes{1, 0, 2, 3, 0, 0}}, cols...))
	case Lista, VistaArbol:
	}
	if l.Llenar {
		// En árbol la nota LLENA el panel y es una HOJA, sin tarjeta ni margen alrededor
		// (`.notes-sheet`): se estira hasta abajo y, si es más larga, crece con el
		// desplazador (`min-h-full grid-rows-[minmax(min-content,1fr)]`).
		if len(tarjetas) == 1 {
			return tarjetas[0]
		}
		return container.New(columnaTarjetas{}, tarjetas...)
	}
	if !l.filas() {
		// Un árbol que no llena el panel: tarjetas en columna, como siempre.
		return conMargen(container.New(&acotado{ancho: AnchoLectura}, container.New(columnaTarjetas{hueco: HuecoLista}, tarjetas...)))
	}
	// Las filas, en la columna de lectura (`.notes-col`) y sangradas 8 px hacia fuera
	// (`.notes-stream -mx-2`): el glifo de cada una cae en el borde de la columna, bajo el
	// título. Un píxel entre fila y fila (`gap-px`) y `pb-10`.
	filas := container.New(columnaTarjetas{hueco: 1}, tarjetas...)
	margen := max(l.margenDiario()-RellenoFilaX, 0)
	return container.New(&acotado{ancho: AnchoLectura + 2*tema.Espacio(6)},
		container.New(ui.Margen{Izq: margen, Der: margen, Abj: tema.Espacio(10)}, filas))
}

// conSangria devuelve las notas en el orden en que se pintan en una lista —cada sublista
// justo debajo de su nota, respetando el orden que llega entre hermanas— y la profundidad
// de cada una. Lo que cuelga de algo que no está a la vista (filtrado, o un ciclo en los
// datos) se queda en la raíz. Es `conSangria` de client/arbol.ts.
func conSangria(notas []Nota) ([]Nota, map[string]int) {
	porID := make(map[string]Nota, len(notas))
	for _, n := range notas {
		porID[n.ID] = n
	}
	sube := func(desde Nota, id string) bool {
		vistos := map[string]bool{}
		for actual, ok := desde, true; ok && !vistos[actual.ID]; actual, ok = porID[actual.Grupo] {
			if actual.ID == id {
				return true
			}
			vistos[actual.ID] = true
			if actual.Grupo == "" {
				break
			}
		}
		return false
	}
	hijos := map[string][]Nota{}
	var raices []Nota
	for _, n := range notas {
		if padre, ok := porID[n.Grupo]; ok && n.Grupo != "" && padre.ID != n.ID && !sube(padre, n.ID) {
			hijos[padre.ID] = append(hijos[padre.ID], n)
			continue
		}
		raices = append(raices, n)
	}
	out := make([]Nota, 0, len(notas))
	niveles := make(map[string]int, len(notas))
	var bajar func(n Nota, nivel int)
	bajar = func(n Nota, nivel int) {
		out = append(out, n)
		niveles[n.ID] = nivel
		for _, h := range hijos[n.ID] {
			bajar(h, nivel+1)
		}
	}
	for _, r := range raices {
		bajar(r, 0)
	}
	return out, niveles
}

// filas dice si las notas se pintan como FILAS: en lista, sea un día o un cuaderno
// corriente; la nota de un árbol, que llena su panel, es una hoja.
func (l *Lienzo) filas() bool { return l.Cuaderno.Vista == Lista && !l.Llenar }

// diario es la lista de un día: filas al estilo de Twos en vez de tarjetas.
func (l *Lienzo) diario() bool { return l.Cuaderno.Crudo.DailyDate != "" && l.Cuaderno.Vista == Lista }

// margenDiario es el `px-4 sm:px-6` de la columna del día.
func (l *Lienzo) margenDiario() float32 {
	if l.AnchoVentana == 0 || l.AnchoVentana >= ui.CorteSM {
		return tema.Espacio(6)
	}
	return tema.Espacio(4)
}

func (l *Lienzo) tarjeta(n Nota) *Tarjeta {
	t := NuevaTarjeta(l.tr, n)
	t.Diario = l.filas()
	if l.vivas == nil {
		l.vivas = map[string]*Tarjeta{}
	}
	l.vivas[n.ID] = t
	t.Seleccionada = n.ID == l.Seleccionada
	t.Bloqueada = l.Bloqueado
	// AQUÍ es donde la regla de «los documentos no se corrompen» llega a la pantalla:
	// una nota cuyo HTML no vuelve entero se abre en SOLO LECTURA, aunque haya red y
	// el cuaderno esté abierto.
	t.SoloLectura = !n.Editable(l.SoloLectura, l.Bloqueado)
	t.Llenar = l.Llenar
	t.SinAcciones = l.SoloLectura
	t.SinMenu = l.SinMenu
	t.AlSeleccionar = func() { llamar1(l.AlSeleccionar, n.ID) }
	t.AlMarcar = func(m bool) { llamar2(l.AlMarcar, n, m) }
	t.AlMenu = func(d fyne.CanvasObject) { llamar2(l.AlMenuNota, n, d) }
	if r, ok := l.Recordatorios[n.ID]; ok {
		t.Recordatorio = &r
		if !l.SoloLectura && l.AlRecordatorio != nil {
			t.AlRecordatorio = func() { l.AlRecordatorio(n) }
		}
	}
	t.AlTecla, t.AlTeclaEspecial = l.AlTecla, l.AlTeclaEspecial
	t.AlRenombrar = func() { llamar1(l.AlRenombrar, n) }
	if l.AlEditar != nil {
		t.AlEditar = func() { l.AlEditar(n) }
	}
	t.AlEnlace = l.AlEnlace
	t.Previa, t.portal = l.Previa, l.portal
	t.AlPlegar = func(i int) { llamar2(l.AlPlegarLista, n, i) }
	t.AlDescargar, t.Imagen, t.Documento = l.AlDescargar, l.Imagen, l.Documento
	t.AlQuitarAdjunto = func(a Adjunto) { llamar2(l.AlQuitarAdjunto, n, a) }
	if n.ID == l.Renombrando {
		t.Renombrar = l.CampoTitulo
	}
	if n.ID == l.Editando {
		t.Editor, t.Ancha, t.AlEnsanchar = l.Editor, l.Ancha, l.AlEnsanchar
	}
	return t
}

// Cartel es el aviso de solo lectura de la nota abierta, o nil si no hace falta.
//
// Dice las DOS cosas por separado, que es el contrato de Diapos: qué no se dibuja
// (`NoPintado`) y si se puede guardar (`Fiel`). Un documento puede tener lo uno sin lo
// otro, y juntarlos en una sola frase perdería la mitad de la información.
func Cartel(tr *i18n.Translator, n Nota, v tema.Variante) fyne.CanvasObject {
	if n.Doc == nil || n.Cifrada {
		return nil
	}
	fiel, sinPintar := n.Doc.Fiel(), n.Doc.NoPintado()
	if fiel && len(sinPintar) == 0 {
		return nil
	}
	titulo := tr.T("public.readOnly", nil)
	var partes []string
	for _, c := range sinPintar {
		partes = append(partes, tr.T(c, nil))
	}
	descripcion := ""
	if len(partes) > 0 {
		descripcion = juntar(partes)
	}
	a := ui.NuevoAviso(ui.AvisoAdvertencia, titulo, descripcion)
	_ = v
	return a
}

// juntar une los rótulos en una línea, como el `Resumen` de Diapos.
func juntar(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// rejillaVentana es una `ui.Rejilla` con cortes que se resuelven AL COLOCAR contra la
// VENTANA, como una media query: redimensionar la recoloca sin esperar a un repintado.
type rejillaVentana struct {
	hueco  float32
	cortes ui.Cortes
}

func (r *rejillaVentana) rejilla(objs []fyne.CanvasObject) ui.Rejilla {
	return ui.RejillaTarjetas(anchoVentana(objs), r.hueco, r.cortes)
}

func (r *rejillaVentana) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return r.rejilla(objs).MinSize(objs)
}

func (r *rejillaVentana) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	r.rejilla(objs).Layout(objs, t)
}

// anchoVentana es el ancho del lienzo en el que están, o el de escritorio si todavía no
// están en ninguno.
func anchoVentana(objs []fyne.CanvasObject) float32 { return ventana(objs).Width }

// ventana es el tamaño de la ventana en la que están: el `vw`/`dvh` de la hoja. Fuera
// de una, el escritorio de referencia.
func ventana(objs []fyne.CanvasObject) fyne.Size {
	for _, o := range objs {
		if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
			return c.Size()
		}
	}
	return fyne.NewSize(1280, 800)
}

// centrar pone un objeto en el centro del panel con el `p-6` de la web.
func centrar(o fyne.CanvasObject, _ tema.Variante) fyne.CanvasObject {
	// Un estado vacío se centra SOLO, y necesita el ancho del panel para repartir su
	// frase en `max-w-sm`: encogido a su mínimo, «Escoge una del árbol de la izquierda,
	// o crea una nueva.» salía en cuatro líneas donde la web pone una.
	if _, ok := o.(*ui.Vacio); ok {
		return o
	}
	return container.New(&centrado{margen: 6 * 0.25 * tema.Rem}, o)
}

// conMargen envuelve con el `p-4` del lienzo: `ui.Margen`.
func conMargen(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: MargenLienzo, Arr: MargenLienzo, Der: MargenLienzo, Abj: MargenLienzo}, o)
}

// centrado centra su único hijo en las dos direcciones (`grid place-items-center`).
type centrado struct{ margen float32 }

var _ fyne.Layout = (*centrado)(nil)

func (c *centrado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*c.margen, m.Height+2*c.margen)
}

func (c *centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	m := objs[0].MinSize()
	objs[0].Resize(m)
	objs[0].Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
}

// acotado es `mx-auto max-w-3xl`: el hijo se queda como mucho con ese ancho y va
// centrado.
type acotado struct{ ancho float32 }

var _ fyne.Layout = (*acotado)(nil)

func (a *acotado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	return objs[0].MinSize()
}

func (a *acotado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	w := min(t.Width, a.ancho)
	objs[0].Move(fyne.NewPos((t.Width-w)/2, 0))
	objs[0].Resize(fyne.NewSize(w, max(objs[0].MinSize().Height, t.Height)))
}

// libre coloca cada nota en su (x, y) con su ancho, que es la disposición de corcho.
//
// Las coordenadas se acotan a cero por arriba y por la izquierda: una nota guardada con
// una posición negativa quedaría inalcanzable, y aquí no hay «deshacer» que la traiga.
//
// La nota de cada objeto sale de su tarjeta y no del orden: la que se arrastra pasa al
// final para pintarse encima (`z-10`), y mientras se arrastra se queda donde la lleva el
// puntero aunque algo vuelva a colocar el lienzo.
type libre struct{}

var _ fyne.Layout = (*libre)(nil)

func (l *libre) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for _, o := range objs {
		t, ok := o.(*Tarjeta)
		if !ok {
			continue
		}
		x, y := float32(max(t.nota.X, 0)), float32(max(t.nota.Y, 0))
		m := o.MinSize()
		ancho = max(ancho, x+anchoLibre(t.nota, m.Width))
		alto = max(alto, y+m.Height)
	}
	return fyne.NewSize(ancho+2*MargenLienzo, alto+2*MargenLienzo)
}

func (l *libre) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	for _, o := range objs {
		t, ok := o.(*Tarjeta)
		if !ok {
			continue
		}
		if !t.arrastrando {
			o.Move(fyne.NewPos(MargenLienzo+float32(max(t.nota.X, 0)), MargenLienzo+float32(max(t.nota.Y, 0))))
		}
		o.Resize(fyne.NewSize(anchoLibre(t.nota, o.MinSize().Width), o.MinSize().Height))
	}
}

// anchoLibre es el `note.width` guardado, o el de una tarjeta nueva si el servicio no lo da.
func anchoLibre(n Nota, minimo float32) float32 {
	if n.Ancho > 0 {
		return float32(n.Ancho)
	}
	return max(minimo, AnchoTarjetaLibre)
}

// columnaTarjetas es el `grid gap-2.5` de tarjetas: cada una con el alto que pide A SU
// ANCHO. `ui.Columna` coloca por el mínimo, y el mínimo de una tarjeta que aún no tenía
// ancho no parte ninguna línea ni sabe cuánto mide su imagen: hasta el fotograma
// siguiente (el `EnsureMinSize` del driver), su contenido se salía por debajo.
type columnaTarjetas struct{ hueco float32 }

func (c columnaTarjetas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Columna{Hueco: c.hueco}.MinSize(objs)
}

func (c columnaTarjetas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var y float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		// Una fila de una sublista entra `1.75rem` por nivel (Tarjeta.sangriaNivel).
		var x float32
		if tj, ok := o.(*Tarjeta); ok {
			x = min(tj.sangriaNivel(), t.Width/2)
		}
		alto := altoDe(o, t.Width-x)
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(t.Width-x, alto))
		y += alto + c.hueco
	}
}

// alFrente pasa un objeto al final de su contenedor, que es pintarlo encima de todos.
func alFrente(c *fyne.Container, o fyne.CanvasObject) {
	if i := slices.Index(c.Objects, o); i >= 0 && i < len(c.Objects)-1 {
		c.Objects = append(slices.Delete(c.Objects, i, i+1), o)
		c.Refresh()
	}
}
