// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El cierre del ejercicio (`CierreDelLibro` de Libro.tsx)
//
// Bajo la cabecera del libro diario, la franja del cierre: la insignia «Cerrado hasta…»
// y, para quien administra la contabilidad (nivel administrador y verbo `administrar`,
// lo que exige la ruta), «Cerrar período…» y «Reabrir…». El historial se despliega
// debajo. Sin nada que decir —abierto, sin historial y sin permiso— no ocupa sitio.
//
// Medidas de la web: la fila es `flex-wrap items-center gap-2` con botones `sm` (28) y
// `mb-5` debajo; el historial, una caja `rounded-card border` con filas `px-4 py-2
// text-sm` separadas por `divide-y`, y `space-y-3` entre la fila y la caja.

// Medidas de la franja.
var (
	// AltoFranjaCierre es la fila: la de un botón `sm`.
	AltoFranjaCierre = tema.Espacio(8)
	// AltoFilaCierre es una fila del historial: `py-2` y una línea `text-sm`, con su filete.
	AltoFilaCierre = tema.TextoSM.Linea + 2*tema.Espacio(2) + tema.Borde
)

// diaCierre es el día de un `hasta` del historial, que llega como instante.
func diaCierre(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

// ReaperturaHasta es `reaperturaHasta`: el corte que queda al REABRIR, el estado
// anterior al cierre vigente. Sin ninguno —el primer cierre—, el libro entero (`""`).
// Reabrir deshace UN paso: quien reabre diciembre para corregir una factura no quiere
// abrir de paso los ejercicios anteriores ya declarados.
func ReaperturaHasta(c api.ERPCierre) string {
	for i, m := range c.Historial {
		if i == 0 {
			continue
		}
		if d := diaCierre(m.Hasta); d == "" || d < c.CerradoHasta {
			return d
		}
	}
	return ""
}

// diaLegible escribe un `YYYY-MM-DD` en el formato de fecha de quien mira.
func diaLegible(loc i18n.Locale, iso string) string {
	if t, ok := fechaISO(iso); ok {
		return i18n.FormatDate(loc, t)
	}
	return iso
}

// textoMovimiento es lo que dice una fila del historial.
func (b *Libro) textoMovimiento(m api.ERPMovimientoCierre) string {
	d := diaCierre(m.Hasta)
	switch {
	case m.Accion == "cierre":
		return b.tr.T("close.closedUntil", i18n.Vars{"fecha": diaLegible(b.loc, d)})
	case d != "":
		return b.tr.T("close.historyReopened", i18n.Vars{"fecha": diaLegible(b.loc, d)})
	}
	return b.tr.T("close.historyReopenedAll", nil)
}

// franjaCierre pone en la pila la fila del cierre y, si se ha desplegado, el historial.
func (b *Libro) franjaCierre(p *pila, v tema.Variante) {
	c := b.Cierre
	if c == nil || (c.CerradoHasta == "" && len(c.Historial) == 0 && !b.PuedeCerrar) {
		return
	}
	fila := &franjaCierreLay{}
	var objs []fyne.CanvasObject
	if c.CerradoHasta != "" {
		fila.insignia = ui.NuevaInsignia(b.tr.T("close.closedUntil", i18n.Vars{"fecha": diaLegible(b.loc, c.CerradoHasta)}), ui.InsigniaInfo)
		objs = append(objs, fila.insignia)
	}
	boton := func(clave string, variante ui.Variante, al func()) {
		if al == nil {
			return
		}
		x := ui.NuevoBoton(b.tr.T(clave, nil), al).Con(variante, ui.TallaSM)
		fila.botones = append(fila.botones, x)
		objs = append(objs, x)
	}
	if len(c.Historial) > 0 {
		boton("close.history", ui.Fantasma, b.AlHistorial)
	}
	if b.PuedeCerrar && c.CerradoHasta != "" {
		boton("close.reopen", ui.Fantasma, b.AlReabrir)
	}
	if b.PuedeCerrar {
		boton("close.close", ui.Secundario, b.AlCerrarPeriodo)
	}
	// El alto depende de cuántas líneas haga falta partirla (`flex-wrap`): en una ventana
	// estrecha la insignia y los botones no caben en una.
	fila.ancho = anchoUtilDe(b.Ventana, AnchoContenidoLista)
	_, _, alto := fila.colocar(fila.ancho)
	p.add(container.New(fila, objs...), alto, 0)
	if b.VerHistorial && len(c.Historial) > 0 {
		o, alto := b.historialCierre(v)
		p.add(o, alto, tema.Espacio(3))
	}
}

// franjaCierreLay es `flex flex-wrap items-center gap-2` con un `mr-auto` tras la
// insignia: los botones que caben en la línea de la insignia van a la derecha; si no
// caben, bajan a las siguientes desde la izquierda, como en el navegador.
type franjaCierreLay struct {
	insignia *ui.Insignia
	botones  []*ui.Boton
	ancho    float32
}

// colocar reparte la fila en `ancho` (sin ancho, en una línea) y devuelve la posición y
// el tamaño de cada pieza —la insignia primero, si la hay— y el alto total.
func (f *franjaCierreLay) colocar(ancho float32) (pos []fyne.Position, tams []fyne.Size, alto float32) {
	hueco := tema.Espacio(2)
	if f.insignia != nil {
		tams = append(tams, ui.MedidaInsignia(f.insignia.Texto))
	}
	for _, b := range f.botones {
		tams = append(tams, b.MinSize())
	}
	pos = make([]fyne.Position, len(tams))
	if len(tams) == 0 {
		return pos, tams, AltoFranjaCierre
	}
	x, linea := float32(0), 0
	primeraLinea := []int{} // las piezas de la línea con el `mr-auto`
	for i, t := range tams {
		if x > 0 && ancho > 0 && x+t.Width > ancho {
			x, linea = 0, linea+1
		}
		y := float32(linea)*(AltoFranjaCierre+hueco) + (AltoFranjaCierre-t.Height)/2
		pos[i] = fyne.NewPos(x, y)
		if linea == 0 {
			primeraLinea = append(primeraLinea, i)
		}
		x += t.Width + hueco
	}
	// `mr-auto` tras la insignia: lo que sobra de la primera línea va antes de sus botones.
	desde := 0
	if f.insignia != nil {
		desde = 1
	}
	if ancho > 0 && len(primeraLinea) > desde {
		u := primeraLinea[len(primeraLinea)-1]
		sobra := max(ancho-(pos[u].X+tams[u].Width), 0)
		for _, i := range primeraLinea[desde:] {
			pos[i].X += sobra
		}
	}
	return pos, tams, float32(linea+1)*AltoFranjaCierre + float32(linea)*hueco
}

func (f *franjaCierreLay) MinSize([]fyne.CanvasObject) fyne.Size {
	_, _, alto := f.colocar(f.ancho)
	return fyne.NewSize(0, alto)
}

func (f *franjaCierreLay) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	pos, tams, _ := f.colocar(t.Width)
	objs := make([]fyne.CanvasObject, 0, len(tams))
	if f.insignia != nil {
		objs = append(objs, f.insignia)
	}
	for _, b := range f.botones {
		objs = append(objs, b)
	}
	for i, o := range objs {
		o.Resize(tams[i])
		o.Move(pos[i])
	}
}

// historialCierre es la caja del historial: cuándo, qué y el motivo, por fila. Por
// debajo de `sm` el motivo baja a su línea (`w-full sm:w-auto`): a su lado no cabría.
func (b *Libro) historialCierre(v tema.Variante) (fyne.CanvasObject, float32) {
	h := &historialCierreLay{
		caja:     ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.Forma("radius-card")),
		estrecha: b.Ventana > 0 && b.Ventana < ui.CorteSM,
	}
	objs := append([]fyne.CanvasObject{}, h.caja.Objetos()...)
	for i, m := range b.Cierre.Historial {
		f := filaCierre{
			cuando: ui.Texto(i18n.FormatDateTime(b.loc, m.En.Local()), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
			que:    ui.Texto(b.textoMovimiento(m), tema.TextoSM, ui.Medio, tema.Color("text", v)),
			motivo: strings.TrimSpace(m.Motivo),
		}
		objs = append(objs, f.cuando, f.que)
		if f.motivo != "" {
			f.porque = ui.Texto(f.motivo, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
			objs = append(objs, f.porque)
		}
		if i > 0 {
			f.filete = canvas.NewRectangle(tema.Color("border", v))
			objs = append(objs, f.filete)
		}
		h.filas = append(h.filas, f)
	}
	return container.New(h, objs...), h.alto()
}

type filaCierre struct {
	cuando, que, porque *canvas.Text
	motivo              string
	filete              *canvas.Rectangle
}

type historialCierreLay struct {
	caja     *ui.Caja
	filas    []filaCierre
	estrecha bool
}

// altoFila es el de una fila: con el motivo en su línea (`gap-y-1`) si es estrecha.
func (h *historialCierreLay) altoFila(f filaCierre) float32 {
	if h.estrecha && f.porque != nil {
		return AltoFilaCierre + tema.Espacio(1) + tema.TextoSM.Linea
	}
	return AltoFilaCierre
}

func (h *historialCierreLay) alto() float32 {
	a := tema.Borde
	for _, f := range h.filas {
		a += h.altoFila(f)
	}
	return a
}

func (h *historialCierreLay) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, h.alto())
}

func (h *historialCierreLay) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	h.caja.Redimensionar(t)
	px, py, hueco := tema.Espacio(4), tema.Espacio(2), tema.Espacio(3)
	y := tema.Borde / 2
	for _, f := range h.filas {
		if f.filete != nil {
			f.filete.Move(fyne.NewPos(tema.Borde, y-tema.Borde/2))
			f.filete.Resize(fyne.NewSize(t.Width-2*tema.Borde, tema.Borde))
		}
		x := px
		ui.ColocarTexto(f.cuando, tema.TextoSM, x, y+py)
		x += ui.Medir(f.cuando.Text, tema.TextoSM, ui.Normal).Width + hueco
		ui.ColocarTexto(f.que, tema.TextoSM, x, y+py)
		x += ui.Medir(f.que.Text, tema.TextoSM, ui.Medio).Width + hueco
		if f.porque != nil {
			yMotivo := y + py
			if h.estrecha {
				x, yMotivo = px, y+py+tema.TextoSM.Linea+tema.Espacio(1)
			}
			// El motivo puede ser largo (hasta mil caracteres): se recorta a lo que cabe.
			ui.Truncar(f.porque, f.motivo, tema.TextoSM, ui.Normal, max(t.Width-px-x, 0))
			ui.ColocarTexto(f.porque, tema.TextoSM, x, yMotivo)
		}
		y += h.altoFila(f)
	}
}

// --- Las acciones -----------------------------------------------------------------

// PuedeCerrarPeriodo es lo que exige la ruta del servicio: NIVEL administrador en
// contabilidad y el verbo `administrar`. Con solo el verbo, el servicio contestaría 403
// al confirmar, que es el peor momento para enterarse.
func (p *Pantalla) PuedeCerrarPeriodo() bool {
	return p.SabePermisos && p.Permisos.Alcanza("contabilidad", api.ERPAdministrador) &&
		p.PuedeAccion("contabilidad", "administrar")
}

// cargarCierre pide hasta cuándo está cerrado el libro. Si no se puede leer, la franja
// no sale: el libro sigue sirviendo para mirarlo, y cerrar sin saber hasta dónde está
// cerrado sería a ciegas.
func (p *Pantalla) cargarCierre() {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		c, err := p.svc.Cierre(ctx)
		enHilo(func() {
			if p.Vista != VistaLibro {
				return
			}
			p.libro.Cierre = nil
			if err == nil {
				p.libro.Cierre = &c
			}
			p.Repintar()
		})
	})
}

// prepararCierre engancha la franja del cierre al libro que se abre.
func (p *Pantalla) prepararCierre() {
	b := p.libro
	b.Cierre, b.VerHistorial = nil, false
	b.PuedeCerrar = p.PuedeCerrarPeriodo()
	b.AlHistorial = func() { b.VerHistorial = !b.VerHistorial; p.Repintar() }
	b.AlCerrarPeriodo, b.AlReabrir = nil, nil
	if b.PuedeCerrar {
		b.AlCerrarPeriodo, b.AlReabrir = p.pedirCerrarPeriodo, p.pedirReabrir
	}
	p.cargarCierre()
}

// faltaParaCerrar dice qué le falta a lo tecleado para poder cerrar; vacío, nada.
func (p *Pantalla) faltaParaCerrar(texto, cerradoHasta string, hoy time.Time) (hasta, falta string) {
	t, vacia, ok := ui.LeerFecha(texto, time.Local)
	if vacia || !ok {
		// A medio teclear no se regaña: el botón sigue apagado hasta que sea una fecha.
		return "", ""
	}
	hasta = t.Format(time.DateOnly)
	if hasta > hoy.Format(time.DateOnly) {
		return "", p.t("close.futureDate")
	}
	if cerradoHasta != "" && hasta <= cerradoHasta {
		return "", p.t("close.notAfterCurrent", i18n.Vars{"fecha": diaLegible(p.tr.Locale(), cerradoHasta)})
	}
	return hasta, ""
}

// pedirCerrarPeriodo es el diálogo de «Cerrar período…»: propone el fin del último mes,
// trimestre y año ya acabados, deja teclear otra fecha —nunca futura: el botón no se
// enciende— y, antes de confirmar, dice la CONSECUENCIA con la fecha elegida.
func (p *Pantalla) pedirCerrarPeriodo() {
	if p.lienzo == nil || p.libro.Cierre == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	loc := p.tr.Locale()
	cerradoHasta := p.libro.Cierre.CerradoHasta
	hoy := time.Now()
	ancho := anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)

	campo := ui.CampoFecha(ui.CampoMD)
	consecuencia := ui.NuevoParrafo("", tema.TextoSM, ui.Normal, "text", v).A(ancho)
	aviso := ui.NuevoParrafo("", tema.TextoSM, ui.Normal, "danger-text", v).A(ancho)
	fallo := ui.NuevoParrafo("", tema.TextoSM, ui.Normal, "danger-text", v).A(ancho)
	fallo.Hide()

	type opcion struct {
		hasta string
		b     *ui.Boton
	}
	var opciones []opcion
	var d *ui.Dialogo
	var hasta string
	refrescar := func() {
		var falta string
		hasta, falta = p.faltaParaCerrar(campo.Texto(), cerradoHasta, hoy)
		consecuencia.Texto = ""
		if hasta != "" {
			consecuencia.Texto = p.t("close.consequence", i18n.Vars{"fecha": diaLegible(loc, hasta)})
		}
		aviso.Texto = falta
		for _, o := range []*ui.Parrafo{consecuencia, aviso} {
			if o.Texto == "" {
				o.Hide()
			} else {
				o.Show()
			}
			o.Refresh()
		}
		// La elegida en primario y las demás fantasma: secundario es también relleno de
		// marca, y la elegida no se distinguiría.
		for _, o := range opciones {
			variante := ui.Fantasma
			if o.hasta == hasta {
				variante = ui.Primario
			}
			o.b.Con(variante, ui.TallaSM).Refresh()
		}
		if d != nil {
			d.SinConfirmar(hasta == "")
			d.Recolocar()
		}
	}
	var chips []fyne.CanvasObject
	for _, pr := range PropuestasDeCierre(hoy) {
		if cerradoHasta != "" && pr.Hasta <= cerradoHasta {
			continue
		}
		t, _ := time.ParseInLocation(time.DateOnly, pr.Hasta, time.Local)
		texto := p.t(rotuloPropuesta[pr.Clave]) + " · " + i18n.FormatDate(loc, t)
		b := ui.NuevoBoton(texto, func() { campo.PonerTexto(ui.EscribirFecha(t)); refrescar() })
		opciones = append(opciones, opcion{pr.Hasta, b})
		chips = append(chips, b)
	}
	if len(opciones) > 0 {
		t, _ := time.ParseInLocation(time.DateOnly, opciones[0].hasta, time.Local)
		campo.PonerTexto(ui.EscribirFecha(t))
	}
	campo.AlCambiar(func(string) { refrescar() })

	partes := []fyne.CanvasObject{}
	if len(chips) > 0 {
		partes = append(partes, container.New(ui.Flujo{Hueco: tema.Espacio(2), Ancho: ancho}, chips...))
	}
	partes = append(partes,
		ui.GrupoCampo(p.t("close.closeUpTo"), true, campo, "", ""),
		aviso, consecuencia, fallo)
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(4)}, partes...)

	d = ui.Confirmacion(p.t("close.closeTitle"), p.tr.T("ui.cancel", nil), p.t("close.closeConfirm"), false, cuerpo,
		func() { d.Cerrar() },
		func() {
			if hasta == "" {
				return
			}
			d.Ocupado(true)
			fallo.Hide()
			elegida := hasta
			p.fondo(func() {
				ctx, cancelar := p.ctx()
				defer cancelar()
				err := p.svc.Cerrar(ctx, elegida)
				enHilo(func() {
					d.Ocupado(false)
					if err != nil {
						fallo.Texto = MensajeDeError(err, p.tr)
						fallo.Show()
						fallo.Refresh()
						d.Recolocar()
						return
					}
					d.Cerrar()
					p.cargarCierre()
				})
			})
		})
	d.Talla = ui.DialogoMD
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Foco = campo
	refrescar()
	d.Mostrar(p.lienzo)
}

// pedirReabrir pregunta el motivo —el `usePrompt` de la web, con el botón apagado
// mientras esté vacío— y reabre hasta el corte anterior.
func (p *Pantalla) pedirReabrir() {
	if p.lienzo == nil || p.libro.Cierre == nil {
		return
	}
	hasta := ReaperturaHasta(*p.libro.Cierre)
	d := ui.Pregunta(p.t("close.reopenReason"), "", p.tr.T("ui.cancel", nil), p.t("close.reopenConfirm"),
		p.tr.T("ui.required", nil), func(motivo string) {
			motivo = strings.TrimSpace(motivo)
			if motivo == "" {
				return
			}
			p.fondo(func() {
				ctx, cancelar := p.ctx()
				defer cancelar()
				err := p.svc.Reabrir(ctx, hasta, motivo)
				enHilo(func() {
					if err != nil {
						p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
						return
					}
					p.cargarCierre()
				})
			})
		})
	d.Titulo = p.t("close.reopenAll")
	if hasta != "" {
		d.Titulo = p.t("close.reopenTo", i18n.Vars{"fecha": diaLegible(p.tr.Locale(), hasta)})
	}
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}
