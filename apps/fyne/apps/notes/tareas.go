// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

// Contrapartida de `PanelTareas` y `PosponerModal` (components/Tareas.tsx) y de
// `verTareas`, `cambiarTarea` y `posponer` de NotesApp.tsx: las tareas de todos los
// cuadernos en una lista; se marcan, se cancelan y se posponen sin abrir su cuaderno.

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// estadoTareas es lo de la vista «Tareas».
type estadoTareas struct {
	fase   Fase
	estado string
	lista  []api.Tarea
}

// estadosTarea son las tres pestañas, con el rótulo de cada una (el de los filtros del
// diario para las dos primeras, como la web).
var estadosTarea = [...][2]string{
	{api.TareaAbierta, "filter.unchecked"},
	{api.TareaHecha, "filter.checked"},
	{api.TareaCancelada, "tasks.cancelled"},
}

// verTareas abre la vista en un estado y la carga.
func (p *Pantalla) verTareas(estado string) {
	p.cerrarEditor()
	p.vista, p.cajon = VistaTareas, false
	p.tareas = estadoTareas{fase: Cargando, estado: estado}
	p.Repintar()
	// Por valor: la carga es de la vista, no hereda el contexto de quien repinta.
	cargar := p.cargarTareas
	go cargar(estado)
}

// cargarTareas trae las tareas de un estado. Fuera del hilo.
func (p *Pantalla) cargarTareas(estado string) {
	if p.svc == nil || !p.hayRed() {
		enHilo(func() { p.tareas.fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	ls, err := p.svc.ListTasks(ctx, estado)
	enHilo(func() {
		if p.tareas.estado != estado {
			return // se ha cambiado de pestaña mientras llegaba
		}
		p.tareas.fase = FaseDe(err, p.hayRed(), len(ls))
		p.tareas.lista = ls
		p.Repintar()
	})
}

// vistaTareas es la página: el título, el segmentado de los tres estados y las tareas
// agrupadas por el sitio donde viven (Tareas.tsx).
func (p *Pantalla) vistaTareas(v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	estados := make([]fyne.CanvasObject, 0, len(estadosTarea))
	for _, e := range estadosTarea {
		estados = append(estados, segmento(t(e[1], nil), "", p.tareas.estado == e[0], v, func() { p.verTareas(e[0]) }))
	}
	derecha := container.New(filaCentro{hueco: tema.Espacio(0.5)}, estados...)
	var cuerpo fyne.CanvasObject
	switch p.tareas.fase {
	case Cargando:
		cuerpo = centrar(ui.NuevaRueda(24, "text-muted", v), v)
	case Vacia:
		cuerpo = fraseVacia(t("tasks.empty."+p.tareas.estado, nil), v)
	case Fallo:
		cuerpo = fraseVacia(t("error.loadFailed", nil), v)
	case SinConexion:
		cuerpo = fraseVacia(t("offline.status", nil), v)
	case SinSesion:
		cuerpo = fraseVacia(t("error.sessionExpired", nil), v)
	case Prohibida:
		cuerpo = fraseVacia(t("forbidden.title", nil), v)
	case Exito:
		// Las de un mismo sitio —su día («Hoy», o la fecha) o su cuaderno— van bajo un
		// solo rótulo, en vez de repetirlo debajo de cada una.
		var grupos []fyne.CanvasObject
		filas := make([]fyne.CanvasObject, 0, len(p.tareas.lista))
		donde := ""
		cerrar := func() {
			if len(filas) > 0 {
				grupos = append(grupos, container.New(ui.Columna{Hueco: tema.Espacio(1)}, rotuloGrupo(donde, v), filasDePagina(filas...)))
			}
			// Una rebanada NUEVA: el contenedor del grupo se queda con la anterior.
			filas = make([]fyne.CanvasObject, 0, 4)
		}
		for _, x := range p.tareas.lista {
			if d := p.dondeEsta(x); d != donde {
				cerrar()
				donde = d
			}
			filas = append(filas, p.filaTarea(x, v))
		}
		cerrar()
		cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(6)}, grupos...)
	}
	return paginaDe(t("tasks.title", nil), derecha, cuerpo, v)
}

// dondeEsta es el día del diario de la tarea («Hoy», o la fecha) o, si no es de un
// diario, su cuaderno.
func (p *Pantalla) dondeEsta(x api.Tarea) string {
	if d, err := time.ParseInLocation(time.DateOnly, x.DailyDate, time.Local); err == nil {
		if x.DailyDate == time.Now().Format(time.DateOnly) {
			return p.tr.T("daily.today", nil)
		}
		return conMayuscula(i18n.FormatWeekdayDayMonth(p.loc, d))
	}
	if x.Notebook != "" {
		return x.Notebook
	}
	return p.tr.T("notebook.untitled", nil)
}

// filaTarea es una tarea de la página (`.notes-row`): la casilla en la columna del glifo,
// el título (que abre su cuaderno), cuántas veces se trasladó, «Mañana» si está pendiente
// y el «…».
func (p *Pantalla) filaTarea(x api.Tarea, v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	n := DeNota(x.Note)
	// Por valor, como en verTareas: las escrituras no heredan el contexto del repintado.
	cambiar, posponer := p.cambiarTarea, p.posponer
	casilla := ui.NuevaCasilla("", func(m bool) { go cambiar(x, Parche{Marcada: &m}) })
	casilla.Marcada = x.Checked
	titulo := n.Titulo
	if titulo == "" {
		titulo = t("note.untitled", nil)
	}
	tinta := "text"
	if n.Terminada() {
		tinta = "text-muted"
	}
	boton := NuevoBotonTitulo(titulo, tinta, t("tasks.openNotebook", nil), func() { p.abrirTarea(x) }).ConLetra(tema.TextoBase, ui.Normal)
	boton.Tachada = n.Terminada()
	piezas := []fyne.CanvasObject{celdaFila(casilla), boton}
	if n.Traslados > 0 && !n.Terminada() {
		piezas = append(piezas, lineaTok(t("task.carried", i18n.Vars{"count": n.Traslados}), tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	if p.tareas.estado == api.TareaAbierta {
		piezas = append(piezas, botonDiario(t("task.tomorrow", nil), ui.TallaXS, !p.hayRed(), func() {
			go posponer(x.Note, time.Now().AddDate(0, 0, 1).Format(time.DateOnly))
		}))
	}
	var mas *ui.BotonIcono
	mas = ui.NuevoBotonIcono(iconos.Puntos, t("common.more", nil), func() {
		el := []fyne.CanvasObject{ui.NuevoElementoMenu(t("tasks.openNotebook", nil), iconos.Ninguno, func() { p.abrirTarea(x) })}
		if p.tareas.estado == api.TareaAbierta {
			el = append(el, ui.NuevoElementoMenu(t("task.postpone", nil), iconos.Ninguno, func() { p.dialogoPosponer(x.Note) }))
		}
		cancelar := "task.cancel"
		if n.Cancelada {
			cancelar = "task.reopen"
		}
		el = append(el, ui.NuevoElementoMenu(t(cancelar, nil), iconos.Ninguno, func() {
			go cambiar(x, Parche{Cancelada: ptr(!n.Cancelada)})
		}))
		m := ui.NuevoMenu(el...)
		m.Rotulo = t("common.more", nil)
		p.abrirMenu(mas, m)
	})
	mas.Lado, mas.TamIcono = ui.LadoIconoSM, GlifoAccion
	piezas = append(piezas, mas)
	// `flex items-start gap-2 px-2 py-1.5`: el título es el que crece.
	// ponytail: en la web «Mañana» y «…» solo se ven al pasar por encima; aquí van
	// siempre, que una fila suelta no sabe cuándo tiene el puntero. Si molesta, hacerla
	// un widget con MouseIn/MouseOut como la Tarjeta.
	return container.New(ui.Margen{Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5), Izq: tema.Espacio(2), Der: tema.Espacio(2)},
		container.New(&filaElastica{hueco: tema.Espacio(2), elastico: 1}, piezas...))
}

// abrirTarea abre el cuaderno de la tarea con ella elegida.
func (p *Pantalla) abrirTarea(x api.Tarea) {
	for _, c := range p.cuadernos {
		if c.ID == x.NotebookID {
			p.abrirCuaderno(c)
			p.lienzo.Seleccionada = x.ID
			p.Repintar()
			return
		}
	}
}

// cambiarTarea marca, cancela o reactiva desde la vista: la nota ENTERA, como en su
// cuaderno. Al cambiar de estado sale de la lista. Fuera del hilo.
func (p *Pantalla) cambiarTarea(x api.Tarea, pa Parche) {
	if p.svc == nil {
		return
	}
	in := pa.sobre(x.Note).Input()
	in.Cancelled = pa.Cancelada
	ctx, cancelar := p.ctx()
	defer cancelar()
	if x.ContentCipher != "" {
		nb, err := p.svc.GetNotebook(ctx, x.NotebookID)
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
			return
		}
		in.ExpectedCrypto = nb.Crypto
	}
	actualizada, err := p.svc.UpdateNote(ctx, x.ID, x.NotebookID, in)
	enHilo(func() {
		if err != nil {
			p.fallo("note.saveFailed", err)
			return
		}
		p.quitarTarea(x.ID)
		p.Repintar()
		if actualizada.RepeatedInto != nil {
			go p.cargarTareas(p.tareas.estado)
			go p.cargarRecordatorios()
		}
	})
}

func (p *Pantalla) quitarTarea(id string) {
	ls := make([]api.Tarea, 0, len(p.tareas.lista))
	for _, x := range p.tareas.lista {
		if x.ID != id {
			ls = append(ls, x)
		}
	}
	p.tareas.lista = ls
	if len(ls) == 0 && p.tareas.fase == Exito {
		p.tareas.fase = Vacia
	}
}

// posponer lleva la tarea, con lo que cuelga de ella, al diario de `dia`. Fuera del hilo.
func (p *Pantalla) posponer(n api.Note, dia string) {
	d, err := time.ParseInLocation(time.DateOnly, dia, time.Local)
	if err != nil || p.svc == nil {
		return
	}
	titulo := i18n.FormatDateLong(p.loc, d)
	ctx, cancelar := p.ctx()
	defer cancelar()
	_, err = p.svc.PostponeTask(ctx, n.ID, dia, titulo)
	enHilo(func() {
		if err != nil {
			p.fallo("task.postponeFailed", err)
			return
		}
		p.quitarTarea(n.ID)
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.tr.T("task.postponed", i18n.Vars{"date": titulo}), ""))
		// Lo que colgaba de ella se fue con ella: el cuaderno abierto se vuelve a leer.
		if p.vista == VistaCuaderno && p.activo.ID == n.NotebookID {
			go p.cargarNotas(p.activo)
		}
		go p.cargarCuadernos()
		p.Repintar()
	})
}

// dialogoPosponer es `PosponerModal`: mañana con un clic, o el día que se teclee.
func (p *Pantalla) dialogoPosponer(n api.Note) {
	v, t := p.variante(), p.tr.T
	manana := time.Now().AddDate(0, 0, 1)
	campo := ui.CampoFecha(ui.CampoMD)
	campo.PonerTexto(ui.EscribirFecha(manana))
	var d *ui.Dialogo
	posponer := p.posponer
	elegir := func(dia time.Time) {
		d.Cerrar()
		go posponer(n, dia.Format(time.DateOnly))
	}
	aceptar := ui.NuevoBoton(t("task.postponeAction", nil), func() {
		if dia, vacia, ok := ui.LeerFecha(campo.Texto(), time.Local); ok && !vacia {
			elegir(dia)
		}
	})
	cancelar := ui.NuevoBoton(t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, aceptar)
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(3)},
		ui.NuevoBoton(t("task.tomorrow", nil), func() { elegir(manana) }).Con(ui.Secundario, ui.TallaMD),
		grupoCampo(t("task.postponeDate", nil), campo, v))
	d = ui.NuevoDialogo(t("task.postpone", nil), cuerpo).Con(ui.DialogoSM, pie)
	p.mostrar(d)
}
