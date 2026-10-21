// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: los diálogos de Notepads —MoverNotaModal.tsx, BacklinksPanel.tsx, el
// modal de exportar una nota y `AplicarEtiquetas` de NotesApp.tsx—. Los AJUSTES del
// cuaderno, que son el más grande, están en ajustes.go.
//
// Todos son `ui.Dialogo` (Modal.tsx) con su talla; lo que cambia es lo que va dentro.

// Medidas de las filas de los diálogos, de sus clases.
const (
	// PXFilaDialogo y PYFilaDialogo son el `px-3 py-2` de las filas de mover, exportar
	// y retroenlaces.
	PXFilaDialogo = 3 * 0.25 * tema.Rem
	PYFilaDialogo = 2 * 0.25 * tema.Rem
	// HuecoFilaDialogo es el `gap-2` entre lo que va dentro de una fila.
	HuecoFilaDialogo = 2 * 0.25 * tema.Rem
	// AnchoEmojiMover es el `w-5` del hueco del emoji en «mover a un cuaderno».
	AnchoEmojiMover = 5 * 0.25 * tema.Rem
)

// mostrar abre un diálogo en la ventana de la pantalla con el aspa ya traducida.
func (p *Pantalla) mostrar(d *ui.Dialogo) {
	if p.win == nil {
		return
	}
	d.RotuloCerrar = p.tr.T("common.close", nil)
	d.Mostrar(p.win.Canvas())
}

// listaConFiletes es `rounded-md border divide-y divide-border/60`: las filas una
// debajo de otra, con un filete entre cada dos y el borde alrededor.
func listaConFiletes(v tema.Variante, filas []fyne.CanvasObject) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	filete := velo(tema.Color("border", v), 0.6)
	objs := append([]fyne.CanvasObject{}, caja.Objetos()...)
	for i, f := range filas {
		if i > 0 {
			objs = append(objs, canvas.NewRectangle(filete))
		}
		objs = append(objs, f)
	}
	return container.New(&filetes{caja: caja, fondo: len(caja.Objetos())}, objs...)
}

// filetes coloca la caja de fondo y apila lo demás dentro del borde: filas y filetes
// alternos, los filetes de un píxel (`divide-y` es un `border-top` de 1).
type filetes struct {
	caja  *ui.Caja
	fondo int
}

func (f *filetes) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs[f.fondo:] {
		if i%2 == 1 {
			alto += tema.Borde
			continue
		}
		m := o.MinSize()
		ancho, alto = max(ancho, m.Width), alto+m.Height
	}
	return fyne.NewSize(ancho+2*tema.Borde, alto+2*tema.Borde)
}

func (f *filetes) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.caja.Redimensionar(t)
	y := tema.Borde
	ancho := max(t.Width-2*tema.Borde, 0)
	for i, o := range objs[f.fondo:] {
		alto := tema.Borde
		if i%2 == 0 {
			alto = o.MinSize().Height
		}
		o.Move(fyne.NewPos(tema.Borde, y))
		o.Resize(fyne.NewSize(ancho, alto))
		y += alto
	}
}

// ---- Mover a otro cuaderno ------------------------------------------------------

// profundidad es cuántos antepasados de un cuaderno están ENTRE los candidatos: el
// `profundidad()` de MoverNotaModal.tsx, que sangra cada fila 1 rem por nivel.
func profundidad(c Cuaderno, todos []Cuaderno) int {
	n := 0
	visto := map[string]bool{c.ID: true} // un ciclo en los datos no cuelga la ventana
	for padre := c.Padre; padre != "" && !visto[padre]; {
		visto[padre] = true
		encontrado := false
		for _, x := range todos {
			if x.ID == padre {
				n++
				padre, encontrado = x.Padre, true
				break
			}
		}
		if !encontrado {
			break
		}
	}
	return n
}

// dialogoMover es MoverNotaModal.tsx: los cuadernos que no son el abierto, en el orden
// del árbol, y los cifrados apagados —su clave vive en el cliente que los cifró—.
func (p *Pantalla) dialogoMover(n Nota) {
	v, t := p.variante(), p.tr.T
	var candidatos []Cuaderno
	for _, c := range enArbol(p.cuadernos) { // lo archivado no es destino, como en la web
		if c.ID != p.activo.ID {
			candidatos = append(candidatos, c)
		}
	}
	var d *ui.Dialogo
	var cuerpo fyne.CanvasObject
	if len(candidatos) == 0 {
		cuerpo = lineaTok(t("move.noOther", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	} else {
		filas := make([]fyne.CanvasObject, 0, len(candidatos))
		for _, c := range candidatos {
			filas = append(filas, p.filaMover(c, profundidad(c, candidatos), func() {
				d.Cerrar()
				lanzar(func() error { return p.moverACuaderno(n.ID, c, true) })
			}, v))
		}
		cuerpo = listaConFiletes(v, filas)
	}
	d = ui.NuevoDialogo(t("move.title", nil), cuerpo).Con(ui.DialogoSM, nil)
	p.mostrar(d)
}

// filaMover es una fila de «mover a»: `px-3 py-2 text-sm`, sangrada 0,75 + nivel rem.
func (p *Pantalla) filaMover(c Cuaderno, nivel int, alPulsar func(), v tema.Variante) *Opcion {
	tinta := "text"
	if c.Cifrado {
		tinta = "text-subtle"
	}
	marca := c.Emoji
	if marca == "" {
		marca = "·"
	}
	emoji := container.New(&centradoEn{ancho: AnchoEmojiMover},
		lineaTok(marca, tema.TextoSM, ui.Normal, tinta, v))
	titulo := lineaTok(c.Titulo, tema.TextoSM, ui.Normal, tinta, v)
	piezas := []fyne.CanvasObject{emoji, titulo}
	if c.Cifrado {
		piezas = append(piezas, lineaTok(p.tr.T("move.encrypted", nil), tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	o := NuevaOpcion(c.Titulo, container.New(&filaElastica{hueco: HuecoFilaDialogo, elastico: 1}, piezas...), alPulsar)
	o.Radio, o.PY, o.Fila = 0, PYFilaDialogo, true
	o.PX = PXFilaDialogo + float32(nivel)*tema.Rem
	o.Inerte = c.Cifrado
	return o
}

// centradoEn es un hueco de ancho fijo con su hijo centrado: el `w-5 text-center`.
type centradoEn struct{ ancho float32 }

func (c *centradoEn) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for _, o := range objs {
		alto = max(alto, o.MinSize().Height)
	}
	return fyne.NewSize(c.ancho, alto)
}

func (c *centradoEn) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		m := o.MinSize()
		o.Resize(m)
		o.Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	}
}

// ---- Exportar una nota ----------------------------------------------------------

// FormatoExport es uno de `FORMATOS_EXPORT` de notesApi.ts: el formato y si se abre
// para VER en vez de bajarse.
type FormatoExport struct {
	ID  string
	Ver bool
}

// FormatosExport son los cinco de la web EN SU ORDEN —el nativo, HTML, el primero—. Solo
// el PDF se abre para ver: lo pinta un proceso aparte.
var FormatosExport = []FormatoExport{
	{"html", false}, {"md", false}, {"txt", false}, {"pdf", true}, {"onex", false},
}

// dialogoExportarNota es el modal «Exportar…» de una nota: los mismos cinco formatos que
// el cuaderno, con el identificador en versalitas a la derecha.
func (p *Pantalla) dialogoExportarNota(n Nota) {
	v, t := p.variante(), p.tr.T
	cuaderno := p.activo.ID
	var d *ui.Dialogo
	filas := make([]fyne.CanvasObject, 0, len(FormatosExport))
	for _, f := range FormatosExport {
		rotulo := lineaTok(t("export."+f.ID, nil), tema.TextoSM, ui.Normal, "text", v)
		id := lineaTok(strings.ToUpper(f.ID), tema.TextoXS, ui.Normal, "text-subtle", v)
		o := NuevaOpcion(t("export."+f.ID, nil),
			container.New(&filaElastica{hueco: HuecoFilaDialogo}, rotulo, id), func() {
				d.Cerrar()
				go p.exportar(cuaderno, n.ID, f.ID, f.Ver)
			})
		o.Radio, o.PX, o.PY, o.Fila = 0, PXFilaDialogo, PYFilaDialogo, true
		filas = append(filas, o)
	}
	d = ui.NuevoDialogo(t("note.export", nil), listaConFiletes(v, filas)).Con(ui.DialogoSM, nil)
	p.mostrar(d)
}

// ---- Qué apunta aquí ------------------------------------------------------------

// limiteRetroenlaces es el `50` que el BFF de la web pide al servicio.
const limiteRetroenlaces = 50

// dialogoRetroenlaces es BacklinksPanel.tsx: las notas que enlazan a esta, que es la
// VUELTA de la referencia entre notas. Se abre cargando y se rellena al llegar.
func (p *Pantalla) dialogoRetroenlaces(n Nota) {
	t := p.tr.T
	cuerpo := container.NewStack()
	d := ui.NuevoDialogo(t("backlinks.title", nil), cuerpo).Con(ui.DialogoSM, nil)
	pintar := func(fase Fase, notas []api.Note) {
		cuerpo.Objects = []fyne.CanvasObject{p.cuerpoRetroenlaces(n, fase, notas, d.Cerrar)}
		cuerpo.Refresh()
		d.Refresh()
	}
	pintar(Cargando, nil)
	p.mostrar(d)
	if p.svc == nil {
		return
	}
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		notas, err := p.svc.LinkedNotes(ctx, "note", n.ID, limiteRetroenlaces)
		enHilo(func() { pintar(FaseDe(err, p.hayRed(), len(notas)), notas) })
	}()
}

// cuerpoRetroenlaces es lo de dentro del panel en cada fase.
func (p *Pantalla) cuerpoRetroenlaces(n Nota, fase Fase, notas []api.Note, cerrar func()) fyne.CanvasObject {
	v, t := p.variante(), p.tr.T
	titulo := n.Titulo
	if titulo == "" {
		titulo = t("note.new", nil)
	}
	// De QUÉ nota, escrito: una vez abierto, el modal tapa la tarjeta de la que salió.
	de := lineaTok(t("backlinks.of", i18n.Vars{"title": titulo}), tema.TextoXS, ui.Normal, "text-subtle", v)
	piezas := []fyne.CanvasObject{de}
	switch fase {
	case Cargando:
		// `grid place-items-center py-8` con la rueda de la suite.
		piezas = append(piezas, container.New(&centrado{margen: 8 * 0.25 * tema.Rem},
			ui.NuevaRueda(24, "text-muted", v)))
	case Vacia:
		// El vacío se EXPLICA: «ninguna» a secas se lee como un fallo.
		piezas = append(piezas, container.New(&margenY{arriba: 4 * 0.25 * tema.Rem},
			container.New(ui.Columna{Hueco: 1.5 * 0.25 * tema.Rem},
				lineaTok(t("backlinks.empty", nil), tema.TextoBase, ui.Normal, "text", v),
				lineaTok(t("backlinks.emptyHint", nil), tema.TextoXS, ui.Normal, "text-subtle", v))))
	case Fallo, SinSesion, Prohibida, SinConexion:
		piezas = append(piezas, lineaTok(t("error.loadFailed", nil), tema.TextoBase, ui.Normal, "danger-text", v))
	case Exito:
		filas := make([]fyne.CanvasObject, 0, len(notas))
		for _, x := range notas {
			filas = append(filas, p.filaRetroenlace(x, cerrar, v))
		}
		piezas = append(piezas, container.New(ui.Columna{Hueco: 1.5 * 0.25 * tema.Rem}, filas...))
	}
	// `mb-2` bajo la línea de «de qué nota».
	return container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, piezas...)
}

// filaRetroenlace es una nota que apunta aquí: `rounded-md border px-3 py-2`, el glifo
// de enlace y, debajo del título, desde dónde. Pulsarla la abre, que en la web es un
// `<a href="?notebook=…&note=…">`.
func (p *Pantalla) filaRetroenlace(x api.Note, cerrar func(), v tema.Variante) *Opcion {
	t := p.tr.T
	titulo := x.Title
	if titulo == "" {
		titulo = t("note.new", nil)
	}
	meta := t("backlinks.from", nil)
	if x.ContentCipher != "" {
		meta = t("crypto.locked", nil)
	}
	texto := container.New(ui.Columna{},
		lineaTok(titulo, tema.TextoBase, ui.Medio, "text", v),
		lineaTok(meta, tema.TextoXS, ui.Normal, "text-subtle", v))
	// `mt-0.5` del glifo: arriba, con medio paso de aire.
	glifo := container.New(&margenY{arriba: 0.5 * 0.25 * tema.Rem},
		Glifo(EnlaceD, GlifoTarjeta, "text-subtle", v))
	o := NuevaOpcion(titulo, container.New(&filaElastica{hueco: HuecoFilaDialogo, elastico: 1, arriba: true},
		glifo, texto), func() {
		cerrar()
		p.abrirResultado(api.NoteHit{ID: x.ID, NotebookID: x.NotebookID})
	})
	o.PX, o.PY, o.Borde = PXFilaDialogo, PYFilaDialogo, true
	return o
}

// margenY añade aire arriba de su único hijo (`mt-*`, `py-*` de un solo lado).
type margenY struct{ arriba float32 }

func (m *margenY) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	s := objs[0].MinSize()
	return fyne.NewSize(s.Width, s.Height+m.arriba)
}

func (m *margenY) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	objs[0].Move(fyne.NewPos(0, m.arriba))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-m.arriba, 0)))
}

// ---- Etiquetas de una nota ------------------------------------------------------

// dialogoEtiquetasDe es `AplicarEtiquetas` de NotesApp.tsx: cada etiqueta con sus
// estados en píldoras, uno por etiqueta y excluyentes. Pulsar el puesto lo quita.
//
// Aquí las píldoras SE REPINTAN al marcar, y en la web no: allí el modal se queda con la
// nota que se abrió y no ve el estado nuevo hasta cerrarlo y volver a abrirlo. Es la
// misma pantalla diciendo la verdad, no otra pantalla.
func (p *Pantalla) aplicarEtiquetas(n Nota) {
	t := p.tr.T
	cuerpo := container.New(ui.Columna{})
	cerrar := ui.NuevoBoton(t("common.close", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	d := ui.NuevoDialogo(t("tags.apply", nil), cuerpo).Con(ui.DialogoSM, cerrar)
	cerrar.AlPulsar = d.Cerrar
	var pintar func()
	pintar = func() {
		actual := n
		if x, ok := p.nota(n.ID); ok {
			actual = x
		}
		cuerpo.Objects = []fyne.CanvasObject{p.cuerpoEtiquetas(actual, func(tag, estado string) {
			go func() {
				p.marcarEstado(n.ID, tag, estado)
				enHilo(pintar)
			}()
		})}
		cuerpo.Refresh()
		d.Refresh()
	}
	pintar()
	p.mostrar(d)
}

// cuerpoEtiquetas son las etiquetas con sus píldoras (`grid gap-2.5`).
func (p *Pantalla) cuerpoEtiquetas(n Nota, alPoner func(tag, estado string)) fyne.CanvasObject {
	v, t := p.variante(), p.tr.T
	if len(p.etiquetas) == 0 {
		return lineaTok(t("tags.none", nil), tema.TextoBase, ui.Normal, "text-muted", v)
	}
	puesto := func(tag string) string {
		for _, s := range n.Estados {
			if s.TagID == tag {
				return s.StateID
			}
		}
		return ""
	}
	bloques := make([]fyne.CanvasObject, 0, len(p.etiquetas))
	for _, tag := range p.etiquetas {
		pildoras := make([]fyne.CanvasObject, 0, len(tag.States))
		for _, s := range tag.States {
			marcado := puesto(tag.ID) == s.ID
			siguiente := s.ID
			if marcado {
				siguiente = "" // pulsar el que ya está lo quita
			}
			pildoras = append(pildoras, pildoraEstado(s, marcado, func() { alPoner(tag.ID, siguiente) }, v))
		}
		// `mb-1` bajo el nombre y `flex flex-wrap gap-1` en las píldoras.
		bloques = append(bloques, container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem},
			lineaTok(tag.Name, tema.TextoBase, ui.Medio, "text", v),
			container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: len(pildoras), Partir: true, SaltoFila: tema.Espacio(1)}, pildoras...)))
	}
	return container.New(ui.Columna{Hueco: 2.5 * 0.25 * tema.Rem}, bloques...)
}

// pildoraEstado es un estado de etiqueta: `rounded-full border px-2.5 py-1 text-base`.
// Marcado lleva de fondo el color del ESTADO —un dato, no un token— y el texto en
// blanco, que es el `color: '#fff'` de la web.
func pildoraEstado(s api.TagState, marcado bool, alPulsar func(), v tema.Variante) *Opcion {
	rotulo := s.Name
	if s.Emoji != "" {
		rotulo = s.Emoji + " " + s.Name
	}
	tinta := tema.Color("text", v)
	if marcado {
		tinta = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	o := NuevaOpcion(rotulo, linea(rotulo, tema.TextoBase, ui.Normal, tinta), alPulsar)
	o.Radio, o.PX, o.PY, o.Borde = tema.Forma("radius-control"), 2.5*0.25*tema.Rem, 1*0.25*tema.Rem, true
	o.Activa = marcado
	o.FondoActivo = hexO(s.Color, tema.Color("primary", v))
	o.BordeActivo = o.FondoActivo // `border-transparent` sobre su propio fondo
	return o
}

// marcarEstado es el `marcarEstado` de la web: pone (o quita, con `estado` vacío) el
// estado de una etiqueta en una nota, y vuelve a leer las notas del cuaderno, que es de
// donde salen las insignias.
func (p *Pantalla) marcarEstado(nota, tag, estado string) {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.SetNoteState(ctx, nota, tag, estado); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return
	}
	p.releerNotas(ctx)
}

// releerNotas vuelve a pedir las notas del cuaderno abierto sin pasar por «cargando»:
// es la web haciendo `setNotes(await api.listNotes(activo.id))`.
//
//nolint:contextcheck // repintar la pantalla no es de esta petición: lo que guarda al repintar (cerrar el editor) lleva su propio plazo
func (p *Pantalla) releerNotas(ctx context.Context) {
	var c, generacion string
	var epoch uint64
	fyne.DoAndWait(func() { c, generacion, epoch = p.activo.ID, p.generacionCrypto(), p.cryptoEpoch })
	if c == "" {
		return
	}
	lista, err := p.svc.ListNotes(ctx, c)
	if err != nil {
		return
	}
	enHilo(func() {
		if p.activo.ID == c && p.cryptoVigente(epoch, generacion) {
			p.ponerNotas(p.deNotas(lista))
			p.Repintar()
		}
	})
}

// confirmar abre la confirmación de la suite con su mensaje repartido en líneas. Las que
// borran algo, que son casi todas las de Notepads: de peligro.
func (p *Pantalla) confirmar(titulo, mensaje, rotulo string, si func()) {
	p.preguntar(titulo, mensaje, rotulo, true, si)
}

// preguntar es confirmar diciendo si es de peligro: la de la nota que pide un enlace
// entrante no borra nada.
func (p *Pantalla) preguntar(titulo, mensaje, rotulo string, peligro bool, si func()) {
	v := p.variante()
	// Sin ancho fijo: se reparte al del cuerpo (350 en el escritorio, 342 en la hoja). A
	// 352 fijos, una línea larga se cortaba por la derecha.
	cuerpo := ui.NuevoParrafo(mensaje, tema.TextoBase, ui.Normal, "text", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, p.tr.T("common.cancel", nil), rotulo, peligro, cuerpo,
		func() { d.Cerrar() }, func() { d.Cerrar(); si() })
	p.mostrar(d)
}
