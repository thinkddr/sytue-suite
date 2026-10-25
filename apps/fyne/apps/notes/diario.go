// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

// Contrapartida del panel del diario de la web
// (apps/frontend/apps/notes/src/components/NotebookDashboard.tsx), dentro de la
// pantalla que ya porta notes.go; no es una pantalla aparte del inventario.

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

var atajoFinal = regexp.MustCompile(`(^|\s)(dd|tt) $`)

// expandirAtajos cambia `dd ` y `tt ` recién tecleados por la fecha ISO y la hora de
// 24 h, como client/diario.ts.
func expandirAtajos(s string, ahora time.Time) string {
	m := atajoFinal.FindStringSubmatchIndex(s)
	if m == nil {
		return s
	}
	valor := ahora.Format(time.DateOnly)
	if s[m[4]:m[5]] == "tt" {
		valor = ahora.Format("15:04")
	}
	return s[:m[4]] + valor + " "
}

// diarioPropio: un diario de quien pregunta. El de otra persona compartido conmigo no se
// abre por «Hoy» (va por dueño), así que es un cuaderno más.
func diarioPropio(c Cuaderno, esMio func(string) bool) bool {
	return c.Crudo.DailyDate != "" && esMio != nil && esMio(c.Crudo.OwnerSub)
}

// expandirCaptura solo expande si lo último fue teclear un espacio: borrar «dd x» hasta
// «dd » no es haber escrito la palabra (client/diario.ts).
func expandirCaptura(previo, s string, ahora time.Time) string {
	if s != previo+" " {
		return s
	}
	return expandirAtajos(s, ahora)
}

// cuadernosDelArbol deja fuera del árbol los diarios PROPIOS (uno por día lo entierra en
// un mes): a un día se llega por «Hoy», la semana y la vista «Días», que enseña los que
// tienen algo (client/diario.ts). Se queda el que tiene cuadernos dentro, o sus hijos no
// tendrían por dónde verse, y el de otra persona compartido conmigo.
func cuadernosDelArbol(cs []Cuaderno, esMio func(string) bool) []Cuaderno {
	padres := map[string]bool{}
	for _, c := range cs {
		padres[c.Padre] = true
	}
	out := make([]Cuaderno, 0, len(cs))
	for _, c := range cs {
		if !diarioPropio(c, esMio) || padres[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// enfocarNotas es la de client/diario.ts: filtra sin dejar huérfanas a las que pasan
// (entran sus antepasados), y `ocultar` quita las tareas hechas o canceladas.
func enfocarNotas(notas []Nota, enfoque string, ocultar bool) []Nota {
	todas := enfoque == "" || enfoque == "all"
	if todas && !ocultar {
		return notas
	}
	porID := make(map[string]Nota, len(notas))
	for _, n := range notas {
		porID[n.ID] = n
	}
	ids := map[string]bool{}
	for _, n := range notas {
		if !pasaEnfoque(n, enfoque) || ocultar && n.Terminada() {
			continue
		}
		for actual := n; actual.ID != "" && !ids[actual.ID]; actual = porID[actual.Grupo] {
			ids[actual.ID] = true
		}
	}
	out := make([]Nota, 0, len(ids))
	for _, n := range notas {
		if ids[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

// pasaEnfoque: la nota entra en el enfoque por sí misma. Las canceladas no son
// pendientes ni hechas.
func pasaEnfoque(n Nota, enfoque string) bool {
	tarea := n.Tipo == "todo" && !n.Cancelada
	switch enfoque {
	case "pinned":
		return n.Fijada
	case "pending":
		return tarea && !n.Marcada
	case "done":
		return tarea && n.Marcada
	}
	return true
}

func contenidoPlantilla(plantilla string, tr *i18n.Translator) string {
	prioridades := "<h2>" + html.EscapeString(tr.T("template.priorities", nil)) + "</h2><ul><li></li></ul>"
	notas := "<h2>" + html.EscapeString(tr.T("note.newText", nil)) + "</h2><p></p>"
	siguientes := "<h2>" + html.EscapeString(tr.T("template.next", nil)) + `</h2><div class="task-row"><input type="checkbox"><span></span></div>`
	if plantilla == "meeting" {
		return notas + siguientes
	}
	return prioridades + notas + siguientes
}

func (p *Pantalla) abrirDia(date string) {
	if p.abriendoDia || p.svc == nil || !p.hayRed() && p.Almacen == nil {
		return
	}
	d, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		return
	}
	title := i18n.FormatDateLong(p.loc, d)
	if !p.hayRed() {
		// Sin red: el día que ya se conozca, o uno provisional donde capturar (cola.go).
		p.filtro, p.filtroEstado = "", ""
		p.abrirCuaderno(diaSinConexion(p.cuadernos, p.dueño(), date, title))
		return
	}
	p.abriendoDia = true
	p.Repintar()
	carry := date == time.Now().Format(time.DateOnly)
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		nb, err := p.svc.DailyNotebook(ctx, date, title, carry)
		enHilo(func() {
			p.abriendoDia = false
			if err != nil {
				p.fallo("error.loadFailed", err)
				p.Repintar()
				return
			}
			p.filtro, p.filtroEstado = "", ""
			p.abrirCuaderno(DeCuaderno(nb))
			go p.cargarCuadernos()
		})
	}()
}

func (p *Pantalla) enviarCaptura() {
	if p.campoCaptura == nil || p.guardandoCaptura || p.creando || !p.puedeCapturar() || p.bloqueado {
		return
	}
	in, ok := capturaRapida(p.campoCaptura.Texto(), p.tipoCaptura)
	if !ok {
		return
	}
	campo, id := p.campoCaptura, p.activo.ID
	p.guardandoCaptura = true
	p.Repintar()
	go func() {
		_ = p.crearEn(in, id, func() {
			p.guardarBorrador(id, "")
			if p.activo.ID == id {
				campo.PonerTexto("")
				p.enfoque = "all"
			}
		})
		enHilo(func() { p.guardandoCaptura = false; p.Repintar() })
	}()
}

// anchoContenido usa los cortes del armazón, sin inventar una segunda rejilla.

func (p *Pantalla) anchoContenido() float32 {
	ancho := ui.AnchoHoja(p.anchoVentana(), ui.AnchoMenu(false), 0)
	if p.anchoVentana() >= ui.Estrecho {
		if p.activo.Vista == VistaArbol {
			ancho -= ui.AcotarAnchoPanel(ui.AnchoPanel(p.anchoVentana()), ancho)
		}
	}
	return max(ancho, 1)
}

func (p *Pantalla) dashboard(v tema.Variante) fyne.CanvasObject {
	ancho := p.anchoContenido()
	if p.activo.Vista == Lista || p.activo.Vista == VistaArbol {
		ancho = min(ancho, AnchoLectura+2*tema.Espacio(6))
	}
	margenX := tema.Espacio(4)
	if p.anchoVentana() >= ui.CorteSM {
		margenX = tema.Espacio(6)
	}
	interior := max(ancho-2*margenX, 1)
	desactivado := !p.puedeCapturar() || p.bloqueado || p.guardandoCaptura || p.creando || (p.lienzo.Fase != Exito && p.lienzo.Fase != Vacia)
	var bloques []ui.Bloque
	p.dia.semana = nil
	// Las acciones de la página (filtro, exportar, «…»): al lado del título.
	var acciones []fyne.CanvasObject
	if enPagina(p.activo) {
		acciones = p.barra.Acciones(v)
	}
	esDia := p.activo.Crudo.DailyDate != ""
	if esDia {
		heading, dia, inerte := p.cabeceraDiaria(v, interior, acciones)
		bloques = append(bloques, ui.Bloque{Obj: heading, Margen: tema.Espacio(4)})
		p.asegurarDiario(dia)
		// En «Semana» los siete días ya están debajo, con su cabecera: la tira sobraba.
		if p.vistaDiario() != VistaSem {
			fila := ui.NuevaSemanaFecha(p.loc, dia, time.Now(), inerte,
				func(d time.Time) { llamar1(p.lateral.AlDia, d.Format(time.DateOnly)) },
				func(d time.Time) (int, string) { return p.marcaDia(d, dia) })
			p.dia.semana, p.dia.semanaDia = fila, dia
			bloques = append(bloques, ui.Bloque{Obj: fila, Margen: tema.Espacio(3)})
		}
	} else if len(acciones) > 0 {
		// Un cuaderno en lista es una página como la de un día: su título en grande y
		// sus acciones al lado.
		derecha := container.New(filaCentro{hueco: tema.Espacio(0.5)}, acciones...)
		bloques = append(bloques, ui.Bloque{Obj: p.cabeceraPagina(p.activo.Titulo, derecha, interior, v), Margen: tema.Espacio(5)})
	}
	// La pista de la sintaxis solo aparece al entrar en la captura, y su sitio está
	// reservado: 4 + 16 + 8 son el `mt-7` que separa la captura de los filtros.
	pista := ui.NuevoParrafo(p.tr.T("capture.submitHint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)
	if !p.capturaEnfocada {
		pista.Hide()
	}
	p.pistaCaptura = pista
	bloques = append(bloques,
		ui.Bloque{Obj: p.capturaDiaria(v, desactivado), Margen: tema.Espacio(1)},
		ui.Bloque{Obj: container.New(reserva{alto: tema.TextoXS.Linea}, pista), Margen: tema.Espacio(2)})
	if pend := p.pendientesAqui(); len(pend) > 0 {
		bloques = append(bloques, ui.Bloque{Obj: p.listaPendientes(v, pend), Margen: tema.Espacio(3)})
	}
	if !esDia || p.vistaDiario() == VistaDia {
		bloques = append(bloques, p.enfoquesDiarios(v, interior)...)
		if a := p.agendaDia(v, interior); esDia && a != nil {
			bloques = append(bloques, ui.Bloque{Obj: a})
		}
	}
	dashboard := ui.AlAnchoFluido(interior, ui.PilaBloques(bloques...))
	return container.New(&acotado{ancho: ancho}, container.New(ui.Margen{Izq: margenX, Der: margenX, Arr: tema.Espacio(8), Abj: tema.Espacio(1)}, dashboard))
}

// botonDiario es un botón fantasma de la página: poco cromo, como en la web.
func botonDiario(s string, talla ui.Talla, inerte bool, accion func()) *ui.Boton {
	b := ui.NuevoBoton(s, accion)
	b.Variante, b.Talla, b.Inerte = ui.Fantasma, talla, inerte
	return b
}

// conMayuscula es el `first-letter:uppercase` del título del día: «jueves» abre frase.
func conMayuscula(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// cabeceraDiaria es el día como página, en dos líneas: el título UNA vez (`text-3xl`) con
// las flechas, «Hoy», Calendar y las acciones de la página a la derecha; y debajo el año
// (y «Hoy») con el segmentado de la vista a la derecha.
func (p *Pantalla) cabeceraDiaria(v tema.Variante, interior float32, acciones []fyne.CanvasObject) (fyne.CanvasObject, time.Time, bool) {
	t := p.tr.T
	dia, _ := time.ParseInLocation(time.DateOnly, p.activo.Crudo.DailyDate, time.Local)
	sub := strconv.Itoa(dia.Year())
	if p.activo.Crudo.DailyDate == time.Now().Format(time.DateOnly) {
		sub += " · " + t("daily.today", nil)
	}
	inerte := p.abriendoDia || p.svc == nil || !p.hayRed() && p.Almacen == nil
	anterior := ui.NuevoBotonIcono(iconos.ChevronIzquierda, t("daily.previous", nil), func() { llamar1(p.lateral.AlDia, dia.AddDate(0, 0, -1).Format(time.DateOnly)) })
	hoy := botonDiario(t("daily.today", nil), ui.TallaSM, inerte, p.lateral.AlHoy)
	siguiente := ui.NuevoBotonIcono(iconos.ChevronDerecha, t("daily.next", nil), func() { llamar1(p.lateral.AlDia, dia.AddDate(0, 0, 1).Format(time.DateOnly)) })
	anterior.Inerte, siguiente.Inerte = inerte, inerte
	dias := []fyne.CanvasObject{anterior, hoy, siguiente}
	if p.AbrirCalendar != nil {
		fecha := dia.Format(time.DateOnly)
		dias = append(dias, ui.NuevoBotonIcono(iconos.Calendario, "Sytue Calendar", func() { p.AbrirCalendar("/?date=" + fecha + "&all_day=1") }))
	}
	dias = append(dias, acciones...)
	navegacion := container.New(filaCentro{hueco: tema.Espacio(0.5)}, dias...)
	titulo := p.cabeceraPagina(conMayuscula(i18n.FormatWeekdayDayMonth(p.loc, dia)), navegacion, interior, v)
	selector := p.selectorVista(v)
	// `min-w-0` del año: lo que deja el segmentado.
	resto := max(interior-selector.MinSize().Width-tema.Espacio(3), 1)
	segunda := container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: 1, Elastico: 1},
		ui.AlAnchoFluido(resto, ui.NuevoParrafo(sub, tema.TextoSM, ui.Normal, "text-subtle", v)), selector)
	return ui.PilaBloques(ui.Bloque{Obj: titulo, Margen: tema.Espacio(1)}, ui.Bloque{Obj: segunda}), dia, inerte
}

// capturaDiaria es la captura en UNA fila subrayada (`.notes-capture`): el conmutador de
// tarea, el campo sin caja, las plantillas en «…» y el «+» de añadir.
func (p *Pantalla) capturaDiaria(v tema.Variante, desactivado bool) fyne.CanvasObject {
	t := p.tr.T
	if p.campoCaptura == nil {
		p.campoCaptura = p.nuevoCampoCaptura()
	}
	p.campoCaptura.Inerte(desactivado)
	tarea := nuevoBotonBarra(TareaD, iconos.Ninguno, t("note.newTodo", nil), func() {
		if p.tipoCaptura == "todo" {
			p.tipoCaptura = "text"
		} else {
			p.tipoCaptura = "todo"
		}
		p.Repintar()
	})
	tarea.ConEstado, tarea.Activo, tarea.Inerte = true, p.tipoCaptura == "todo", desactivado
	var plantillas *ui.BotonIcono
	plantillas = ui.NuevoBotonIcono(iconos.Puntos, t("template.label", nil), func() { p.abrirMenu(plantillas, p.menuPlantillas()) })
	plantillas.Inerte = desactivado
	rotulo := t("capture.add", nil)
	if p.guardandoCaptura {
		rotulo = t("note.saving", nil)
	}
	añadir := ui.NuevoBotonIcono(iconos.Mas, rotulo, p.enviarCaptura)
	añadir.Inerte = desactivado || strings.TrimSpace(p.campoCaptura.Texto()) == ""
	p.añadirCaptura, p.capturaInerte = añadir, desactivado
	fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: 2, Elastico: 2, Partir: true}, tarea, p.campoCaptura, plantillas, añadir)
	tinta := "border-subtle"
	if p.capturaEnfocada {
		tinta = "primary"
	}
	p.lineaCaptura = canvas.NewRectangle(tema.Color(tinta, v))
	return container.New(&subrayado{py: tema.Espacio(1)}, p.lineaCaptura, fila)
}

// subrayado es la fila con su `border-b` y su `py-*`: el primer objeto es la línea.
type subrayado struct{ py float32 }

func (s *subrayado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, m.Height+2*s.py+tema.Borde)
}

func (s *subrayado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	objs[1].Move(fyne.NewPos(0, s.py))
	objs[1].Resize(fyne.NewSize(t.Width, t.Height-2*s.py-tema.Borde))
}

func (p *Pantalla) enfoquesDiarios(v tema.Variante, interior float32) []ui.Bloque {
	t := p.tr.T
	cuentas := map[string]int{"all": len(p.notas)}
	total, hechas := 0, 0
	for _, n := range p.notas {
		if n.Fijada {
			cuentas["pinned"]++
		}
		// Las canceladas no cuentan: ni pendientes ni para el progreso.
		if n.Tipo == "todo" && !n.Cancelada {
			total++
			if n.Marcada {
				hechas++
			}
		}
	}
	cuentas["pending"], cuentas["done"] = total-hechas, hechas
	esDia := p.activo.Crudo.DailyDate != ""
	var bloques []ui.Bloque
	// Un día pasado con pendientes ofrece llevarlas a hoy (el traslado de siempre).
	if hoy := time.Now().Format(time.DateOnly); esDia && p.activo.Crudo.DailyDate < hoy && total > hechas {
		quedan := ui.NuevoParrafo(t("daily.pendingLeft", i18n.Vars{"count": total - hechas}), tema.TextoSM, ui.Normal, "text-muted", v)
		llevar := botonDiario(t("daily.carryToToday", nil), ui.TallaXS, !p.puedeEscribir() || p.abriendoDia, func() { llamar1(p.lateral.AlDia, hoy) })
		llevar.Icono = iconos.ChevronDerecha
		fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1, Partir: true, SaltoFila: tema.Espacio(2), Elastico: 0}, quedan, llevar)
		// `notes-carry-bar`: una fila sin tarjeta, como el resto del diario; la fila que
		// parte necesita el ancho real para medirse.
		bloques = append(bloques, ui.Bloque{Obj: ui.AlAnchoFluido(interior, fila), Margen: tema.Espacio(3)})
	}
	// Los filtros hablan de tareas y de fijadas: en un día, en cuanto hay algo; en un
	// cuaderno corriente, solo si tiene tareas (o hay un enfoque puesto: un filtro que no
	// se ve esconde notas).
	enfoque := p.enfoque
	if enfoque == "" {
		enfoque = "all"
	}
	if esDia && len(p.notas) == 0 || !esDia && total == 0 && enfoque == "all" {
		return bloques
	}
	claves := map[string]string{"all": "filter.all", "pending": "filter.unchecked", "done": "filter.checked", "pinned": "focus.pinned"}
	items := make([]fyne.CanvasObject, 0, 6)
	for _, foco := range []string{"all", "pending", "done", "pinned"} {
		items = append(items, segmento(t(claves[foco], nil), strconv.Itoa(cuentas[foco]), enfoque == foco, v,
			func() { p.cerrarEditor(); p.enfoque = foco; p.Repintar() }))
	}
	derecha := len(items)
	if total > 0 {
		ocultar := ui.NuevoBotonIcono(iconos.Ojo, t("daily.hideFinished", nil), func() {
			p.cerrarEditor()
			p.ocultarTerminadas = !p.ocultarTerminadas
			fyne.CurrentApp().Preferences().SetBool(clavePrefOcultarTerminadas, p.ocultarTerminadas)
			p.Repintar()
		})
		if p.ocultarTerminadas {
			ocultar.Icono, ocultar.Pulsado = iconos.OjoTachado, true
		}
		items = append(items, aroProgreso(hechas, total, v), ocultar)
	}
	fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(0.5), Derecha: derecha, Partir: true, SaltoFila: tema.Espacio(0.5)}, items...)
	// `-mx-2`: el texto del primer segmento cae en el borde de la columna, bajo el título.
	return append(bloques, ui.Bloque{Obj: container.New(ui.Margen{Izq: -tema.Espacio(2), Der: -tema.Espacio(2)}, ui.AlAnchoFluido(interior+2*tema.Espacio(2), fila))})
}

// atajoDiario es lo que hace cada atajo del diario: «today» abre hoy desde cualquier
// sitio; «previous», «next» y «capture» solo con un día abierto. Devuelve si lo atendió.
func (p *Pantalla) atajoDiario(a string) bool {
	// Con un diálogo o menú encima no se cambia el día de detrás (haySuperposicionAbierta).
	if p.win != nil && p.win.Canvas().Overlays().Top() != nil {
		return false
	}
	if a == "today" {
		llamar0(p.lateral.AlHoy)
		return true
	}
	dia, err := time.ParseInLocation(time.DateOnly, p.activo.Crudo.DailyDate, time.Local)
	if p.vista != VistaCuaderno || err != nil {
		return false
	}
	switch a {
	case "previous":
		llamar1(p.lateral.AlDia, dia.AddDate(0, 0, -1).Format(time.DateOnly))
	case "next":
		llamar1(p.lateral.AlDia, dia.AddDate(0, 0, 1).Format(time.DateOnly))
	case "capture":
		// Una captura inerte, o el foco en cualquier campo (enCampoDeTexto de la web), no
		// se lleva el ⌘K: que lo atienda el buscador.
		if p.campoCaptura == nil || p.capturaInerte || p.win == nil || p.win.Canvas().Focused() != nil {
			return false
		}
		p.win.Canvas().Focus(p.campoCaptura)
	default:
		return false
	}
	return true
}

// EnfocarCaptura es el ⌘K que la barra de la suite cede (`CederAtajoSi`): con un día
// abierto lleva a la captura; si no, o si la captura ya tiene el foco, el buscador.
func (p *Pantalla) EnfocarCaptura() bool { return p.atajoDiario("capture") }

// nuevoCampoCaptura es el campo sin caja de la captura: Intro guarda, `dd`/`tt` se
// expanden al teclearlos y la línea de debajo sigue al foco.
func (p *Pantalla) nuevoCampoCaptura() *ui.Entrada {
	t := p.tr.T
	campo := ui.NuevaEntrada(t("capture.placeholder", nil)).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1), MargenY: (tema.Espacio(9) - 2*tema.Borde - tema.TextoBase.Linea) / 2,
		Texto: tema.TextoBase, Plano: true,
	})
	campo.Rotulo = t("capture.label", nil)
	campo.PonerTexto(p.borradorGuardado(p.activo.ID))
	id := p.activo.ID
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			p.enviarCaptura()
			return true
		}
		return false
	}
	previo := campo.Texto()
	campo.AlCambiar(func(s string) {
		x := expandirCaptura(previo, s, time.Now())
		previo = x
		p.guardarBorrador(id, x)
		if x != s {
			campo.PonerTextoAlFinal(x)
			return
		}
		// El «+» se enciende y se apaga con lo escrito, sin repintar la pantalla.
		if b := p.añadirCaptura; b != nil {
			b.Inerte = p.capturaInerte || strings.TrimSpace(s) == ""
			b.Refresh()
		}
	})
	campo.AlEnfocar = func(f bool) {
		p.capturaEnfocada = f
		if p.lineaCaptura != nil {
			p.lineaCaptura.FillColor = tema.Color(map[bool]string{true: "primary", false: "border-subtle"}[f], p.variante())
			p.lineaCaptura.Refresh()
		}
		// La pista de la sintaxis aparece al entrar y se va al salir, sin repintar.
		if o := p.pistaCaptura; o != nil {
			if f {
				o.Show()
			} else {
				o.Hide()
			}
		}
	}
	return campo
}

// menuPlantillas son las tres plantillas del «…» de la captura.
func (p *Pantalla) menuPlantillas() *ui.Menu {
	t := p.tr.T
	items := make([]fyne.CanvasObject, 0, 3)
	for _, plantilla := range []string{"journal", "meeting", "plan"} {
		items = append(items, ui.NuevoElementoMenu(t("template."+plantilla, nil), iconos.Ninguno, func() {
			if !p.puedeCapturar() || p.bloqueado || p.creando {
				return
			}
			in := api.NoteInput{Kind: "text", Title: t("template."+plantilla, nil), Content: contenidoPlantilla(plantilla, p.tr)}
			id := p.activo.ID
			lanzar(func() error {
				return p.crearEn(in, id, func() {
					if p.activo.ID == id {
						p.enfoque = "all"
					}
				})
			})
		}))
	}
	return ui.NuevoMenu(items...)
}
