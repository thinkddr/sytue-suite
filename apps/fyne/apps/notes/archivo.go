// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las vistas «Fijadas» y «Archivo»: Archivo.tsx y client/archivo.ts en la web, con la
// misma regla de qué va al árbol y qué al Archivo.

// enArbol son los cuadernos que van al árbol: los no archivados.
func enArbol(cs []Cuaderno) []Cuaderno {
	out := make([]Cuaderno, 0, len(cs))
	for _, c := range cs {
		if !c.Archivado {
			out = append(out, c)
		}
	}
	return out
}

// raicesArchivadas son la CABEZA de cada rama archivada: un hijo archivado con su padre
// vuelve cuando vuelve el padre, y ofrecerlo suelto lo colgaría de uno que no se ve.
func raicesArchivadas(cs []Cuaderno) []Cuaderno {
	archivado := map[string]bool{}
	for _, c := range cs {
		archivado[c.ID] = c.Archivado
	}
	var out []Cuaderno
	for _, c := range cs {
		if c.Archivado && !archivado[c.Padre] {
			out = append(out, c)
		}
	}
	return out
}

// Fijadas son las notas fijadas de todos los cuadernos visibles.
type Fijadas struct {
	tr *i18n.Translator

	Notas []api.NoteHit
	Fase  Fase

	AlAbrir func(api.NoteHit)
}

// Objeto arma la vista: el estado de la fase, o el título y las filas de la búsqueda.
func (f *Fijadas) Objeto(v tema.Variante) fyne.CanvasObject {
	if o, ok := estadoDeFase(f.tr, f.Fase, "focus.pinned", "pinned.empty", v); ok {
		return o
	}
	filas := make([]fyne.CanvasObject, 0, len(f.Notas))
	for _, h := range f.Notas {
		filas = append(filas, filaHit(h, "", f.AlAbrir, v))
	}
	return paginaDe(f.tr.T("focus.pinned", nil), nil, filasDePagina(filas...), v)
}

// Archivo son las cabezas de las ramas archivadas, para abrirlas o devolverlas al árbol.
type Archivo struct {
	tr *i18n.Translator

	Cuadernos []Cuaderno

	// EsMio decide quién ve «Desarchivar»: solo el dueño; a los demás el servicio
	// les responde 404.
	EsMio func(dueno string) bool

	AlAbrir       func(Cuaderno)
	AlDesarchivar func(Cuaderno)
}

// Objeto arma la página: una fila por cuaderno (`.notes-row`), con su glifo, el nombre que
// lo abre, cuántas notas tiene y «Desarchivar» en fantasma.
func (a *Archivo) Objeto(v tema.Variante) fyne.CanvasObject {
	if len(a.Cuadernos) == 0 {
		o, _ := estadoDeFase(a.tr, Vacia, "archive.title", "archive.empty", v)
		return o
	}
	filas := make([]fyne.CanvasObject, 0, len(a.Cuadernos))
	for _, c := range a.Cuadernos {
		nombre := NuevaOpcion(c.Titulo, lineaTok(c.Titulo, tema.TextoBase, ui.Normal, "text", v),
			func() { llamar1(a.AlAbrir, c) })
		nombre.Superficie, nombre.Radio = "bg", tema.RadioSM
		fila := []fyne.CanvasObject{
			celdaFila(Glifo(CuadernoD, 14, "text-subtle", v)),
			nombre,
			lineaTok(strconv.Itoa(c.Notas), tema.TextoXS, ui.Normal, "text-subtle", v),
		}
		if a.EsMio != nil && a.EsMio(c.Crudo.OwnerSub) {
			volver := ui.NuevoBoton(a.tr.T("archive.unarchive", nil), func() { llamar1(a.AlDesarchivar, c) })
			volver.Variante, volver.Talla = ui.Fantasma, ui.TallaXS
			fila = append(fila, volver)
		}
		filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5), Izq: tema.Espacio(2), Der: tema.Espacio(2)},
			container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1}, fila...)))
	}
	return paginaDe(a.tr.T("archive.title", nil), nil, filasDePagina(filas...), v)
}

// estadoDeFase es lo que se pinta en lugar de la lista mientras no hay nada que listar:
// la rueda, la página vacía —su título y una frase en voz baja, sin ilustración— o el
// motivo del fallo. Lo comparten la papelera y las dos vistas de aquí. `false` es que hay
// filas que pintar.
func estadoDeFase(tr *i18n.Translator, fase Fase, titulo, vacio string, v tema.Variante) (fyne.CanvasObject, bool) {
	t := tr.T
	frase := func(clave string) (fyne.CanvasObject, bool) {
		return paginaDe(t(titulo, nil), nil, fraseVacia(t(clave, nil), v), v), true
	}
	switch fase {
	case Cargando:
		return centrar(ui.NuevaRueda(24, "text-muted", v), v), true
	case Vacia:
		return frase(vacio)
	case Fallo:
		return frase("error.loadFailed")
	case SinConexion:
		return frase("offline.status")
	case SinSesion:
		return frase("error.sessionExpired")
	case Prohibida:
		return frase("forbidden.title")
	case Exito:
	}
	return nil, false
}

// cargarFijadas pide las notas fijadas de todos los cuadernos.
func (p *Pantalla) cargarFijadas() {
	enHilo(func() { p.fijadas.Fase = Cargando; p.Repintar() })
	if !p.hayRed() || p.svc == nil {
		enHilo(func() { p.fijadas.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	res, err := p.svc.PinnedNotes(ctx)
	enHilo(func() {
		p.fijadas.Notas = res
		p.fijadas.Fase = FaseDe(err, p.hayRed(), len(res))
		p.Repintar()
	})
}

// archivar archiva la rama de `c` o la devuelve, y relee el árbol: cambia la rama
// entera, no solo su cabeza. Si el archivado era el abierto, se cierra.
func (p *Pantalla) archivar(c Cuaderno, si bool) {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.ArchiveNotebook(ctx, c.ID, si); err != nil {
		clave := "archive.failed"
		if !si {
			clave = "archive.unarchiveFailed"
		}
		p.avisar(ui.AvisoPeligro, p.tr.T(clave, nil), Motivo(err))
		return
	}
	clave := "archive.restored"
	if si {
		clave = "archive.done"
	}
	p.avisar(ui.AvisoExito, p.tr.T(clave, nil), "")
	enHilo(func() {
		if si && p.activo.ID == c.ID {
			p.activo, p.notas, p.lienzo.Seleccionada = Cuaderno{}, nil, ""
		}
	})
	p.cargarCuadernos()
}
