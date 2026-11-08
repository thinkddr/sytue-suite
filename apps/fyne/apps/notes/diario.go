// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

// Contrapartida del panel del diario de la web
// (apps/frontend/apps/notes/src/components/NotebookDashboard.tsx), dentro de la
// pantalla que ya porta notes.go; no es una pantalla aparte del inventario.

import (
	"html"
	"image/color"
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

// enUnaLinea es la captura como la enseña su campo, que es de UNA línea: cada salto, un
// espacio, como pega el navegador un texto de varias líneas en un `<input>`. Cada salto
// es una runa y cada espacio otra, así que las posiciones valen en los dos.
func enUnaLinea(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// ponerCaptura cambia lo que lleva la captura del cuaderno abierto: lo guarda como su
// borrador y lo enseña en el campo (`changeDraft` de NotebookDashboard.tsx).
func (p *Pantalla) ponerCaptura(texto string) {
	p.capturaTexto = texto
	p.guardarBorrador(p.activo.ID, texto)
	if c := p.campoCaptura; c != nil && c.Texto() != enUnaLinea(texto) {
		c.PonerTexto(enUnaLinea(texto))
	}
}

// enviarCaptura es Intro (o el «+») en la captura: lo que haya, una nota por línea.
func (p *Pantalla) enviarCaptura() { p.enviar(p.capturaTexto, MaxLineasPegadas) }

// enviar guarda lo que hay en la captura: una nota por línea, de una en una y en orden
// (`enviar` de NotebookDashboard.tsx). La captura se vacía según salen, y para en el
// primer fallo o a las cincuenta: lo que no ha salido se queda escrito. Si mientras tanto
// se escribe otra cosa en ella, se para también y lo escrito no se toca. `cuantas`: las
// primeras líneas y no más (lo pegado sobre el cuaderno, que va delante de lo que ya
// hubiera en la captura).
func (p *Pantalla) enviar(texto string, cuantas int) {
	if p.campoCaptura == nil || p.guardandoCaptura || p.creando || !p.puedeCapturar() || p.bloqueado {
		return
	}
	lineas := lineasDeCaptura(texto)
	if len(lineas) == 0 {
		return
	}
	campo, id, tipo := p.campoCaptura, p.activo.ID, p.tipoCaptura
	p.guardandoCaptura = true
	p.Repintar()
	go func() {
		hechas := 0
		for i, linea := range lineas[:min(len(lineas), cuantas, MaxLineasPegadas)] {
			in, ok := capturaRapida(linea, tipo)
			// `despues` es que ha salido —o que se ha quedado en la cola, sin red—, y corre
			// en el hilo de la interfaz, como lo de abajo: `fyne.Do` va en orden.
			salio, seguir := false, false
			if ok {
				_ = p.crearEn(in, id, func() { salio = true })
			}
			fyne.DoAndWait(func() {
				if salio {
					hechas++
					seguir = p.quedanEnCaptura(campo, id, texto, lineas, i)
				}
			})
			if !seguir {
				break
			}
		}
		enHilo(func() {
			p.guardandoCaptura = false
			if hechas > 0 && p.activo.ID == id {
				p.enfoque = "all"
			}
			p.Repintar()
		})
	}()
}

// quedanEnCaptura deja en la captura las líneas que siguen a la `i`, que acaba de salir,
// y dice si se sigue. Si mientras tanto se ha escrito otra cosa, lo escrito no se toca y
// se para. Si se ha cambiado de cuaderno, su borrador pierde lo que ya ha salido y el
// resto espera a que se vuelva (crearEn ya no escribiría allí). En el hilo de la
// interfaz.
func (p *Pantalla) quedanEnCaptura(campo *ui.Entrada, id, texto string, lineas []string, i int) bool {
	// Lo que había antes de esta línea: lo enviado al principio, o las que quedaban.
	enCampo := texto
	if i > 0 {
		enCampo = strings.Join(lineas[i:], "\n")
	}
	abierto := p.campoCaptura == campo && p.activo.ID == id
	actual := p.capturaTexto
	if !abierto {
		actual = p.borradorGuardado(id)
	}
	if actual != enCampo {
		return false
	}
	resto := strings.Join(lineas[i+1:], "\n")
	if !abierto {
		p.guardarBorrador(id, resto)
		return false
	}
	p.ponerCaptura(resto)
	return true
}

// pegarEnCaptura es pegar varias líneas en la captura: una nota por línea. Una sola se
// pega como siempre, en el campo (devuelve false). Lo pegado va donde iría —en lugar de
// lo seleccionado— con sus saltos, que el campo enseña como espacios, y sale enseguida.
func (p *Pantalla) pegarEnCaptura(campo *ui.Entrada, pegado string) bool {
	if p.capturaDeshabilitada() || len(lineasDeCaptura(pegado)) < 2 {
		return false
	}
	antes, enCampo := []rune(p.capturaTexto), []rune(campo.Texto())
	if len(antes) != len(enCampo) {
		antes = enCampo // no debería pasar: el campo enseña la captura runa a runa
	}
	pegado = strings.ReplaceAll(strings.ReplaceAll(pegado, "\r\n", "\n"), "\r", "\n")
	inicio, fin := campo.Pegar(pegado)
	// Lo que había seleccionado es lo que el campo tiene de menos, contando lo pegado.
	quitado := len(enCampo) - (utf8.RuneCountInString(campo.Texto()) - (fin - inicio))
	inicio = min(max(inicio, 0), len(antes))
	nuevo := string(antes[:inicio]) + pegado + string(antes[min(inicio+max(quitado, 0), len(antes)):])
	p.ponerCaptura(nuevo)
	p.enviar(nuevo, MaxLineasPegadas)
	return true
}

// pegarLineas es lo que se pega con varias líneas sobre una lista con nada enfocado
// (`pegarFuera` de NotebookDashboard.tsx): va DELANTE de lo que estuviera a medio
// escribir en la captura, y solo salen sus líneas, porque lo que la persona tenía en la
// captura no lo ha mandado nadie. Una sola línea no es esto (devuelve false).
func (p *Pantalla) pegarLineas(texto string) bool {
	lineas := lineasDeCaptura(texto)
	if p.campoCaptura == nil || p.capturaDeshabilitada() || len(lineas) < 2 {
		return false
	}
	nuevo := strings.Join(lineas, "\n")
	if strings.TrimSpace(p.capturaTexto) != "" {
		nuevo += "\n" + p.capturaTexto
	}
	p.ponerCaptura(nuevo)
	p.enviar(nuevo, len(lineas))
	return true
}

// capturaDeshabilitada es el `disabled` de la captura: sin escritura, cerrado o con las
// notas sin cargar. Guardar NO la deshabilita aquí —la web deja pegar mientras tanto y
// lo pegado espera en el campo—; el campo sí se apaga mientras guarda (capturaFija).
func (p *Pantalla) capturaDeshabilitada() bool {
	return !p.puedeCapturar() || p.bloqueado || (p.lienzo.Fase != Exito && p.lienzo.Fase != Vacia)
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
	var bloques []ui.Bloque
	p.dia.semana = nil
	p.dia.marcasDestino = nil
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
			for i, o := range fila.Objects {
				marca := canvas.NewRectangle(color.Transparent)
				marca.StrokeWidth, marca.StrokeColor, marca.CornerRadius = tema.AnilloFoco, tema.Color("primary", v), tema.RadioMD
				marca.Hide()
				fila.Objects[i] = container.NewStack(o, marca)
				p.dia.marcasDestino = append(p.dia.marcasDestino, marca)
			}
			p.dia.semana, p.dia.semanaDia = fila, dia
			bloques = append(bloques, ui.Bloque{Obj: fila, Margen: tema.Espacio(3)})
		}
	} else if len(acciones) > 0 {
		// Un cuaderno en lista es una página como la de un día: su título en grande y
		// sus acciones al lado.
		derecha := container.New(filaCentro{hueco: tema.Espacio(0.5)}, enVozBajaTodos(acciones)...)
		bloques = append(bloques, ui.Bloque{Obj: p.cabeceraPagina(p.activo.Titulo, derecha, interior, v), Margen: tema.Espacio(5)})
	}
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

// La misma captura permanece al pie mientras se recorren las notas. El campo no se
// recrea: conserva borrador, selección y foco al guardar o cambiar de disposición.
func (p *Pantalla) capturaFija(v tema.Variante) fyne.CanvasObject {
	inhabilitada := p.capturaDeshabilitada() || p.guardandoCaptura || p.creando
	pista := ui.NuevoParrafo(p.tr.T("capture.submitHint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)
	if !p.capturaEnfocada {
		pista.Hide()
	}
	p.pistaCaptura = pista
	ancho := min(p.anchoContenido(), AnchoLectura+2*tema.Espacio(6))
	margen := p.lienzo.margenDiario()
	cuerpo := ui.PilaBloques(
		ui.Bloque{Obj: p.capturaDiaria(v, inhabilitada), Margen: tema.Espacio(1)},
		ui.Bloque{Obj: container.New(reserva{alto: tema.TextoXS.Linea}, pista)})
	interior := ui.AlAnchoFluido(max(ancho-2*margen, 1), cuerpo)
	centrado := container.New(&acotado{ancho: ancho},
		container.New(ui.Margen{Izq: margen, Der: margen, Arr: tema.Espacio(2), Abj: tema.Espacio(2)}, interior))
	linea := canvas.NewRectangle(tema.Color("border-subtle", v))
	linea.SetMinSize(fyne.NewSize(0, tema.Borde))
	return ui.PilaBloques(ui.Bloque{Obj: linea}, ui.Bloque{Obj: centrado})
}

// botonDiario es un botón fantasma de la página, en gris (`Atenuado`): poco cromo, como
// en la web.
func botonDiario(s string, talla ui.Talla, inerte bool, accion func()) *ui.Boton {
	b := ui.NuevoBoton(s, accion)
	b.Variante, b.Talla, b.Inerte, b.Atenuado = ui.Fantasma, talla, inerte, true
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
	// El año en gris y, si lo es, «Hoy» en el color de marca: lo único de color en la
	// cabecera (`.notes-day-sub` y `.notes-day-today`).
	piezasSub := []fyne.CanvasObject{lineaTok(strconv.Itoa(dia.Year()), tema.TextoSM, ui.Normal, "text-subtle", v)}
	if p.activo.Crudo.DailyDate == time.Now().Format(time.DateOnly) {
		piezasSub = append(piezasSub, lineaTok("·", tema.TextoSM, ui.Normal, "text-subtle", v),
			lineaTok(t("daily.today", nil), tema.TextoSM, ui.Medio, "primary-text", v))
	}
	inerte := p.abriendoDia || p.svc == nil || !p.hayRed() && p.Almacen == nil
	anterior := ui.NuevoBotonIcono(iconos.ChevronIzquierda, t("daily.previous", nil), func() { llamar1(p.lateral.AlDia, dia.AddDate(0, 0, -1).Format(time.DateOnly)) })
	hoy := botonDiario(t("daily.today", nil), ui.TallaSM, inerte, p.lateral.AlHoy)
	siguiente := ui.NuevoBotonIcono(iconos.ChevronDerecha, t("daily.next", nil), func() { llamar1(p.lateral.AlDia, dia.AddDate(0, 0, 1).Format(time.DateOnly)) })
	anterior.Inerte, siguiente.Inerte = inerte, inerte
	dias := []fyne.CanvasObject{enVozBaja(anterior), hoy, enVozBaja(siguiente)}
	if p.AbrirCalendar != nil {
		fecha := dia.Format(time.DateOnly)
		dias = append(dias, enVozBaja(ui.NuevoBotonIcono(iconos.Calendario, "Sytue Calendar", func() { p.AbrirCalendar("/?date=" + fecha + "&all_day=1") })))
	}
	dias = append(dias, enVozBajaTodos(acciones)...)
	navegacion := container.New(filaCentro{hueco: tema.Espacio(0.5)}, dias...)
	titulo := p.cabeceraPagina(conMayuscula(i18n.FormatWeekdayDayMonth(p.loc, dia)), navegacion, interior, v)
	selector := p.selectorVista(v)
	// `min-w-0` del año: lo que deja el segmentado.
	resto := max(interior-selector.MinSize().Width-tema.Espacio(3), 1)
	segunda := container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: 1, Elastico: 1},
		ui.AlAnchoFluido(resto, container.New(filaCentro{hueco: tema.Espacio(1.5)}, piezasSub...)), selector)
	return ui.PilaBloques(ui.Bloque{Obj: titulo, Margen: tema.Espacio(1.5)}, ui.Bloque{Obj: segunda}), dia, inerte
}

// capturaDiaria es la captura en UNA fila subrayada (`.notes-capture`): el conmutador de
// tarea, el campo sin caja, las plantillas en «…» y el «+» de añadir.
func (p *Pantalla) capturaDiaria(v tema.Variante, desactivado bool) fyne.CanvasObject {
	t := p.tr.T
	if p.campoCaptura == nil {
		p.campoCaptura = p.nuevoCampoCaptura()
	}
	p.campoCaptura.Inerte(desactivado)
	// El conmutador de tarea ES la marca de la línea que va a nacer —la raya de una cosa o
	// la casilla vacía de una tarea—, en la columna de las marcas de las filas
	// (`.notes-capture-kind`).
	var marca fyne.CanvasObject
	p.lineaCaptura = nil
	if p.tipoCaptura == "todo" {
		marca = casillaVacia(v)
	} else {
		r := raya(v)
		if p.capturaEnfocada {
			r.FillColor = tema.Color("text-muted", v)
		}
		marca, p.lineaCaptura = r, r
	}
	tarea := NuevaOpcion(t("note.newTodo", nil), container.NewCenter(marca), func() {
		if p.tipoCaptura == "todo" {
			p.tipoCaptura = "text"
		} else {
			p.tipoCaptura = "todo"
		}
		p.Repintar()
	})
	tarea.PX, tarea.PY = (ui.LadoIconoMD-LadoCasilla)/2, (ui.LadoIconoMD-LadoCasilla)/2
	filaDePagina(tarea)
	tarea.Activa, tarea.FondoActivo = p.tipoCaptura == "todo", color.NRGBA{}
	tarea.Inerte = desactivado
	var plantillas *ui.BotonIcono
	plantillas = enVozBaja(ui.NuevoBotonIcono(iconos.Puntos, t("template.label", nil), func() { p.abrirMenu(plantillas, p.menuPlantillas()) }))
	plantillas.Inerte = desactivado
	rotulo := t("capture.add", nil)
	if p.guardandoCaptura {
		rotulo = t("note.saving", nil)
	}
	añadir := enVozBaja(ui.NuevoBotonIcono(iconos.Mas, rotulo, p.enviarCaptura))
	añadir.Inerte = desactivado || strings.TrimSpace(p.campoCaptura.Texto()) == ""
	p.añadirCaptura, p.capturaInerte = añadir, desactivado
	fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: 2, Elastico: 2, Partir: true}, tarea, p.campoCaptura, plantillas, añadir)
	// Sin caja ni subrayado: la última línea de la libreta (`min-h-11 px-1`).
	return container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)}, container.New(&alturaMinima{alto: tema.Espacio(11)}, fila))
}

// alturaMinima centra a su hijo en una caja de al menos `alto` (`min-h-*` con
// `items-center`).
type alturaMinima struct{ alto float32 }

func (a *alturaMinima) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, max(m.Height, a.alto))
}

func (a *alturaMinima) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-m.Height)/2))
	objs[0].Resize(fyne.NewSize(t.Width, m.Height))
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
		enVozBaja(ocultar)
		// En el teléfono el aro se cae: «Hechas 1» ya lo dice y los filtros caben en una
		// línea.
		if p.anchoVentana() >= ui.CorteSM {
			items = append(items, aroProgreso(hechas, total, v))
		}
		items = append(items, ocultar)
	}
	fila := container.New(ui.FilaAuto{Hueco: HuecoSegmento, Derecha: derecha, Partir: true, SaltoFila: tema.Espacio(1)}, items...)
	// Sin `-mx-2`: las palabras ya no llevan relleno y la primera cae bajo el título.
	return append(bloques, ui.Bloque{Obj: ui.AlAnchoFluido(interior, fila)})
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
// expanden al teclearlos, pegar varias líneas hace una nota por línea y la línea de
// debajo sigue al foco.
func (p *Pantalla) nuevoCampoCaptura() *ui.Entrada {
	t := p.tr.T
	// El texto de la captura, a 16 px como el de las líneas (`.notes-capture-input`).
	campo := ui.NuevaEntrada(t("capture.placeholder", nil)).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1), MargenY: (tema.Espacio(9) - 2*tema.Borde - tema.TextoMD.Linea) / 2,
		Texto: tema.TextoMD, Plano: true,
	})
	campo.Rotulo = t("capture.label", nil)
	id := p.activo.ID
	p.capturaTexto = p.borradorGuardado(id)
	campo.PonerTexto(enUnaLinea(p.capturaTexto))
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			p.enviarCaptura()
			return true
		}
		return false
	}
	// Pegando, el campo cambia a medias (con espacios donde iban los saltos): eso no es
	// lo que lleva la captura, y `pegarEnCaptura` lo pone enseguida.
	pegando := false
	campo.AlPegar = func(s string) bool {
		pegando = true
		defer func() { pegando = false }()
		return p.pegarEnCaptura(campo, s)
	}
	campo.AlCambiar(func(s string) {
		// Lo que el campo enseña de la captura no es un cambio: es ponerCaptura.
		if !pegando && s != enUnaLinea(p.capturaTexto) {
			x := expandirCaptura(p.capturaTexto, s, time.Now())
			p.capturaTexto = x
			p.guardarBorrador(id, x)
			if x != s {
				campo.PonerTextoAlFinal(x)
				return
			}
		}
		// El «+» se enciende y se apaga con lo escrito, sin repintar la pantalla.
		if b := p.añadirCaptura; b != nil {
			b.Inerte = p.capturaInerte || strings.TrimSpace(s) == ""
			b.Refresh()
		}
	})
	campo.AlEnfocar = func(f bool) {
		p.capturaEnfocada = f
		// Con el campo enfocado, la raya de la marca sube a `text-muted`.
		if p.lineaCaptura != nil {
			p.lineaCaptura.FillColor = tema.Color(map[bool]string{true: "text-muted", false: "border-strong"}[f], p.variante())
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
