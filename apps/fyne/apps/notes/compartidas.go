// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/compartir"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: `PanelCompartidas` (Compartidas.tsx) y «Compartir con personas» de
// NotesApp.tsx.
//
// «Compartido conmigo» son las notas SUELTAS que otras personas me han compartido; los
// cuadernos compartidos salen en el árbol como cualquier otro. Abierta, la nota va en su
// tarjeta de siempre: con «puede ver», en solo lectura; con «puede editar», se cambia su
// título y su contenido y nada más —sin el «…», porque fijarla, moverla o borrarla es de
// su dueña, y el servicio tampoco lo deja—.

// estadoCompartidas es lo de la vista.
type estadoCompartidas struct {
	fase    Fase
	notas   []api.NotaCompartida
	nombres map[string]string
	// abierta es la nota que se enseña; vacío, la lista.
	abierta string
	lienzo  *Lienzo
}

// cargarCompartidas trae las notas y quién compartió cada una. Fuera del hilo.
func (p *Pantalla) cargarCompartidas() {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	ns, err := p.svc.Compartidas(ctx)
	nombres := map[string]string{}
	if err == nil && p.Directorio != nil {
		for _, n := range ns {
			if _, ya := nombres[n.Por]; ya {
				continue
			}
			if per, e := p.Directorio.PorID(ctx, n.Por); e == nil {
				nombres[n.Por] = per.Nombre
				if per.Nombre == "" {
					nombres[n.Por] = per.Email
				}
			}
		}
	}
	enHilo(func() {
		p.comp.fase = FaseDe(err, p.hayRed(), len(ns))
		if err == nil {
			p.comp.notas, p.comp.nombres = ns, nombres
		}
		p.Repintar()
	})
}

// abrirCompartida abre una nota de la lista (o vuelve a ella con "").
func (p *Pantalla) abrirCompartida(id string) {
	p.cerrarEditor()
	p.comp.abierta = id
	p.comp.lienzo.Renombrando, p.comp.lienzo.CampoTitulo = "", nil
	p.Repintar()
}

func (p *Pantalla) compartida(id string) (api.NotaCompartida, bool) {
	for _, n := range p.comp.notas {
		if n.ID == id {
			return n, true
		}
	}
	return api.NotaCompartida{}, false
}

// vistaCompartidas es la página: la lista, o la nota abierta.
func (p *Pantalla) vistaCompartidas(v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	if o, ok := estadoDeFase(p.tr, p.comp.fase, "sharedWithMe.title", "sharedWithMe.empty", v); ok {
		return o
	}
	if n, ok := p.compartida(p.comp.abierta); ok {
		return p.notaCompartida(n, v)
	}
	filas := make([]fyne.CanvasObject, 0, len(p.comp.notas))
	for _, n := range p.comp.notas {
		filas = append(filas, p.filaCompartida(n, v))
	}
	return paginaDe(t("sharedWithMe.title", nil), nil, filasDePagina(filas...), v)
}

// porQuien es «Compartido por …», con «alguien de tu organización» si no se sabe.
func (p *Pantalla) porQuien(n api.NotaCompartida) string {
	nombre := p.comp.nombres[n.Por]
	if nombre == "" {
		nombre = p.tr.T("sharePeople.someone", nil)
	}
	return p.tr.T("sharedWithMe.by", i18n.Vars{"name": nombre})
}

func (p *Pantalla) nivel(n api.NotaCompartida) string {
	if n.Permiso == api.NivelEditar {
		return p.tr.T("sharePeople.write", nil)
	}
	return p.tr.T("sharePeople.read", nil)
}

// filaCompartida es una nota de la lista (`.notes-row`): su glifo, su título —o su texto,
// si no lo tiene— y debajo quién la compartió y con qué nivel.
func (p *Pantalla) filaCompartida(n api.NotaCompartida, v tema.Variante) fyne.CanvasObject {
	titulo := n.Title
	if titulo == "" {
		titulo = resumenNota("", n.Content, maxResumen)
	}
	if titulo == "" {
		titulo = p.tr.T("note.untitled", nil)
	}
	texto := container.New(ui.Columna{},
		lineaTok(titulo, tema.TextoBase, ui.Normal, "text", v),
		lineaTok(p.porQuien(n)+" · "+p.nivel(n), tema.TextoXS, ui.Normal, "text-subtle", v))
	dentro := container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1, arriba: true}, glifoDeHit(n.Kind, v), texto)
	o := NuevaOpcion(titulo, dentro, func() { p.abrirCompartida(n.ID) })
	o.PX, o.PY, o.Superficie = tema.Espacio(2), tema.Espacio(1.5), "bg"
	return o
}

// notaCompartida es la nota abierta: la vuelta, quién, «Solo lectura» si toca, y su
// tarjeta.
func (p *Pantalla) notaCompartida(n api.NotaCompartida, v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	volver := ui.NuevoBoton(t("sharedWithMe.title", nil), func() { p.abrirCompartida("") })
	volver.Variante, volver.Talla, volver.Icono = ui.Fantasma, ui.TallaSM, iconos.ChevronIzquierda
	derecha := []fyne.CanvasObject{volver, lineaTok(p.porQuien(n), tema.TextoSM, ui.Normal, "text-muted", v)}
	if n.Permiso != api.NivelEditar {
		derecha = append(derecha, ui.NuevaInsignia(t("sharedWithMe.readOnly", nil), ui.InsigniaNeutra))
	}
	// `ml-auto` en quién: el primero crece y empuja a los demás a la derecha.
	cabecera := container.New(&filaElastica{hueco: HuecoBarra}, derecha...)
	l := p.comp.lienzo
	l.Cuaderno = Cuaderno{ID: n.NotebookID, Vista: Lista}
	l.Notas, l.Fase, l.Seleccionada = []Nota{p.deNota(n.Note)}, Exito, n.ID
	l.Llenar, l.SinMenu = true, true
	l.SoloLectura = n.Permiso != api.NivelEditar || !p.hayRed()
	return conMargen(container.New(&acotado{ancho: AnchoLectura},
		container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, cabecera, l.Objeto(v))))
}

// engancharCompartidas cuelga del lienzo de la nota compartida lo que hace con ella.
func (p *Pantalla) engancharCompartidas() {
	l := NuevoLienzo(p.tr)
	l.AlEnlace = p.seguirEnlace
	l.AlEditar = p.abrirEditorCompartida
	l.AlRenombrar = p.renombrarCompartida
	l.AlMarcar = func(n Nota, m bool) {
		lanzar(func() error { return p.guardarCompartida(n.ID, Parche{Marcada: &m}) })
	}
	p.comp = estadoCompartidas{lienzo: l, fase: Cargando}
}

// abrirEditorCompartida es `abrirEditor` para una nota que me han compartido con edición.
func (p *Pantalla) abrirEditorCompartida(n Nota) {
	if s := p.edicion; s != nil && s.nota == n.ID {
		p.enfocar(s.editor.Zona())
		return
	}
	p.cerrarEditor()
	if nc, ok := p.compartida(n.ID); !ok || nc.Permiso != api.NivelEditar || !n.Editable(!p.hayRed(), false) {
		return
	}
	ed := NuevoEditorNota(p.tr, n.Doc, p.tr.T("note.placeholder", nil))
	s := &sesionEdicion{
		nota: n.ID, editor: ed, compartida: true,
		fusion: n.ID + ":" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}
	s.html = ed.HTML()
	s.guardado = s.html
	ed.AlCambiar = func(html string) { p.alEscribir(s, html) }
	ed.AlTerminar = p.cerrarEditor
	ed.AlCrecer = p.recolocarLienzo
	p.edicion = s
	p.comp.lienzo.Editando, p.comp.lienzo.Editor = n.ID, ed
	p.Repintar()
	p.enfocar(ed.Zona())
}

// renombrarCompartida es `renombrar` para una nota compartida conmigo con edición.
func (p *Pantalla) renombrarCompartida(n Nota) {
	l := p.comp.lienzo
	c := NuevoCampoEnLinea(n.Titulo, p.tr.T("note.titlePlaceholder", nil), PYCampoTarjeta, "surface", TopeTitulo)
	cerrar := func() {
		l.Renombrando, l.CampoTitulo = "", nil
		p.Repintar()
	}
	c.AlConfirmar = func(s string) {
		nuevo := strings.TrimSpace(s)
		cerrar()
		if nuevo != n.Titulo {
			lanzar(func() error { return p.guardarCompartida(n.ID, Parche{Titulo: &nuevo}) })
		}
	}
	c.AlCancelar = cerrar
	l.Renombrando, l.CampoTitulo = n.ID, c
	p.Repintar()
	p.enfocar(c.Editor())
}

// guardarCompartida guarda un cambio de una nota compartida conmigo: el PUT de siempre,
// que el servicio recorta a título y contenido. Fuera del hilo.
func (p *Pantalla) guardarCompartida(id string, pa Parche) error {
	var nc api.NotaCompartida
	var ok bool
	fyne.DoAndWait(func() { nc, ok = p.compartida(id) })
	if !ok || p.svc == nil {
		return nil
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	hecha, err := p.svc.UpdateNote(ctx, id, nc.NotebookID, pa.sobre(nc.Note).Input())
	enHilo(func() {
		if err != nil {
			p.fallo("note.saveFailed", err)
			return
		}
		for i := range p.comp.notas {
			if p.comp.notas[i].ID == id {
				p.comp.notas[i].Note = hecha
			}
		}
		p.Repintar()
	})
	// Una tarea que repite: la siguiente también se comparte conmigo (editarCon).
	if err == nil && hecha.RepeatedInto != nil {
		p.cargarCompartidas()
		p.cargarRecordatorios()
	}
	return err
}

// ---- Compartir lo mío ----

// esMio: lo escribió o es el dueño quien pregunta.
func (p *Pantalla) esMio(dueno string) bool {
	return p.Yo != nil && dueno != "" && dueno == p.Yo()
}

// puedeCompartirNota: la escribí, su cuaderno no está cifrado y hay con qué. Es la misma
// regla que la web (`compartirNota` de NotesApp.tsx).
func (p *Pantalla) puedeCompartirNota(n Nota) bool {
	return p.Directorio != nil && p.svc != nil && p.hayRed() && !p.activo.Cifrado && p.esMio(n.Crudo.OwnerSub)
}

// compartirConPersonas abre el diálogo común sobre lo que se comparte.
func (p *Pantalla) compartirConPersonas(nombre string, personas api.Personas) {
	if p.win == nil {
		return
	}
	compartir.Abrir(p.win.Canvas(), compartir.Opciones{
		T: p.tr, V: p.variante(), Nombre: nombre,
		Niveles:  []string{api.NivelVer, api.NivelEditar},
		Personas: personas, Directorio: p.Directorio,
		Avisar: func(exito bool, texto string) {
			tono := ui.AvisoExito
			if !exito {
				tono = ui.AvisoPeligro
			}
			p.avisos.Notificar(ui.NuevoAviso(tono, texto, ""))
		},
	})
}

func (p *Pantalla) compartirNota(n Nota) {
	nombre := n.Titulo
	if nombre == "" {
		nombre = p.tr.T("note.untitled", nil)
	}
	p.compartirConPersonas(nombre, p.svc.PersonasDeNota(n.ID))
}

func (p *Pantalla) compartirCuaderno() {
	p.compartirConPersonas(p.activo.Titulo, p.svc.PersonasDeCuaderno(p.activo.ID))
}
