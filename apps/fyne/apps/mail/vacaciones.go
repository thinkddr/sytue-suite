// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"image/color"
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

// # «Autorespuesta de vacaciones» (`VacationModal.tsx`)
//
// Qué va a pasar antes de tocar nada; el estado de AHORA en palabras —desactivada,
// respondiendo hasta tal día, programada o ya terminada—; el conmutador, el asunto, el
// mensaje y las dos fechas con su pista (y el rango al revés en rojo), y cómo lo
// recibirá quien escriba. Si la carga falla no se enseña el formulario: guardarlo en
// blanco borraría la autorespuesta de verdad.

// Vacaciones es lo que el diálogo le pide al servidor. Lo cumple *api.Mail.
type Vacaciones interface {
	Vacation(ctx context.Context) (api.MailVacation, error)
	SetVacation(ctx context.Context, v api.MailVacation) error
}

var _ Vacaciones = (*api.Mail)(nil)

// EstadoVacaciones es `statusOf`: el tono de la insignia, su rótulo y la frase que lo
// explica. Las fechas son días (a las 00:00 locales, o cero si no hay) y `hoy` entra
// de fuera para que el guardián no dependa del día en que corre.
func EstadoVacaciones(t *i18n.Translator, l i18n.Locale, activa bool, desde, hasta, hoy time.Time) (ui.Tono, string, string) {
	h := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	fecha := func(x time.Time) i18n.Vars { return i18n.Vars{"date": i18n.FormatDayMonthLong(l, x)} }
	switch {
	case !activa:
		return ui.InsigniaNeutra, t.T("vacation.statusOffLabel", nil), t.T("vacation.statusOffDetail", nil)
	case !hasta.IsZero() && hasta.Before(h):
		return ui.InsigniaAviso, t.T("vacation.statusEndedLabel", nil), t.T("vacation.statusEndedDetail", fecha(hasta))
	case !desde.IsZero() && desde.After(h):
		return ui.InsigniaAviso, t.T("vacation.statusScheduledLabel", nil), t.T("vacation.statusScheduledDetail", fecha(desde))
	case !hasta.IsZero():
		return ui.InsigniaExito, t.T("vacation.statusOnLabel", nil), t.T("vacation.statusOnDetailUntil", fecha(hasta))
	}
	return ui.InsigniaExito, t.T("vacation.statusOnLabel", nil), t.T("vacation.statusOnDetailOpen", nil)
}

// PeriodoVacaciones es `fromDateInput`: el primer día desde las 00:00 y el último
// hasta las 23:59:59, que es lo que promete la pista («hasta el final del 14»).
func PeriodoVacaciones(desde, hasta time.Time) (inicio, fin *time.Time) {
	if !desde.IsZero() {
		inicio = &desde
	}
	if !hasta.IsZero() {
		f := time.Date(hasta.Year(), hasta.Month(), hasta.Day(), 23, 59, 59, 0, hasta.Location())
		fin = &f
	}
	return inicio, fin
}

// dialogoVacaciones es el diálogo abierto. El formulario se monta UNA vez y lo que
// cambia al teclear —el estado, las pistas y la vista previa— se repinta en su
// sitio: rehacerlo entero en cada tecla le quitaría el foco al campo.
type dialogoVacaciones struct {
	b      *Buzon
	d      *ui.Dialogo
	caja   *fyne.Container
	ancho  float32
	estado Estado
	// anchoFecha es el de cada campo de fecha: la mitad con `sm:grid-cols-2`.
	anchoFecha float32

	form         *fyne.Container
	conmutador   *ui.Conmutador
	insignia     *ui.Insignia
	detalle      *ui.Parrafo
	asunto       *ui.Entrada
	cuerpo       *ui.AreaTexto
	desde, hasta *ui.Entrada
	grupoDesde   *fyne.Container
	grupoHasta   *fyne.Container
	previoAsunto *ui.Rotulo
	previoCuerpo *fyne.Container

	cancelar, guardar *ui.Boton
}

// verVacaciones abre la autorespuesta. Devuelve el diálogo para las pruebas.
func (b *Buzon) verVacaciones() *dialogoVacaciones {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &dialogoVacaciones{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width)}
	g.caja = container.New(ui.Columna{})
	g.montar(lienzo.Size().Width)
	g.cancelar = ui.NuevoBoton(t.T("common.cancel", nil), func() { g.d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	g.guardar = ui.NuevoBoton(t.T("common.save", nil), g.salvar).Con(ui.Primario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("vacation.title", nil), g.caja).
		Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, g.cancelar, g.guardar))
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.cargar()
	return g
}

// montar arma el formulario, en el orden de la web y con sus huecos: `gap-5` entre
// bloques y `sm:grid-cols-2` para las fechas desde 640 de VENTANA.
func (g *dialogoVacaciones) montar(ventana float32) {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sm := tema.TextoSM
	ajustado := tema.Escalon{Tam: sm.Tam, Linea: 1.375 * sm.Tam} // `leading-snug`

	// El estado: `rounded-md border p-3 flex items-start justify-between gap-3`, sin
	// fondo propio; a la izquierda el titular con su insignia y la frase (`mt-1`), a
	// la derecha el conmutador en `text-sm`.
	g.conmutador = ui.NuevoConmutador(t.T("vacation.enabled", nil), func(si bool) { g.pintar() })
	g.conmutador.Escalon = sm
	// La izquierda es `min-w-0`: se queda con lo que deja el conmutador y ahí se parte
	// (en el móvil la insignia baja a la segunda línea y la frase va a tres).
	izq := g.ancho - 2*tema.Borde - 3*tema.Espacio(3) - g.conmutador.MinSize().Width
	g.insignia = ui.NuevaInsignia("", ui.InsigniaNeutra)
	g.detalle = ui.NuevoParrafo("", ajustado, ui.Normal, "text-muted", v).A(izq)
	titular := container.New(flujo{hueco: tema.Espacio(2), ancho: izq},
		ui.NuevoRotulo(t.T("vacation.autoReply", nil), sm, ui.SemiNegrita, "text"), g.insignia)
	izquierda := container.New(ui.Columna{Hueco: tema.Espacio(1)}, titular, g.detalle)
	estado := enCajaDe(tema.Color("surface-raised", v), v, container.New(margen(tema.Espacio(3)),
		container.New(extremos{}, izquierda, g.conmutador)))

	g.asunto = ui.NuevaEntrada(t.T("vacation.subjectPlaceholder", nil))
	g.cuerpo = ui.NuevaAreaTexto(t.T("vacation.bodyPlaceholder", nil))
	g.cuerpo.Filas, g.cuerpo.Ancho = 5, g.ancho
	g.desde, g.hasta = ui.CampoFecha(ui.CampoMD), ui.CampoFecha(ui.CampoMD)
	for _, e := range []*ui.Entrada{g.asunto, g.desde, g.hasta} {
		e.AlCambiar(func(string) { g.pintar() })
	}
	g.cuerpo.AlCambiar(func(string) { g.pintar() })

	// Las pistas se reparten al ancho de su campo: en la web van a dos renglones.
	campo := func(rotulo string, control fyne.CanvasObject, pista string, ancho float32) *fyne.Container {
		c := ui.GrupoCampo(rotulo, false, control, "", "")
		p := ui.NuevaPistaCampo(pista)
		p.Ancho = ancho
		c.Add(p)
		return c
	}
	rejilla := ui.Rejilla{Fijas: 2, Hueco: tema.Espacio(4)}
	g.anchoFecha = g.ancho
	if ventana >= 640 {
		g.anchoFecha = rejilla.AnchoColumna(g.ancho)
	}
	g.grupoDesde = campo(t.T("vacation.from", nil), g.desde, "", g.anchoFecha)
	g.grupoHasta = campo(t.T("vacation.until", nil), g.hasta, "", g.anchoFecha)
	fechas := container.New(ui.Columna{Hueco: tema.Espacio(4)}, g.grupoDesde, g.grupoHasta)
	if ventana >= 640 {
		fechas = container.New(rejilla, g.grupoDesde, g.grupoHasta)
	}

	// Así lo recibirán: `border bg-bg-subtle rounded-md p-3`, el asunto recortado a
	// una línea y el cuerpo con sus saltos (`whitespace-pre-wrap`, `mt-1.5`).
	g.previoAsunto = ui.NuevoRotulo("", sm, ui.SemiNegrita, "text")
	g.previoCuerpo = container.New(ui.Columna{})
	previo := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		ui.NuevoRotulo(t.T("vacation.previewTitle", nil), sm, ui.SemiNegrita, "text"),
		enCajaDe(tema.Color("bg-subtle", v), v, container.New(margen(tema.Espacio(3)),
			container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, g.previoAsunto, g.previoCuerpo))))

	g.form = container.New(ui.Columna{Hueco: tema.Espacio(5)},
		ui.NuevaAyudaEnLinea(t.T("vacation.explainBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(t.T("vacation.explainTitle", nil)),
		estado,
		campo(t.T("common.subject", nil), g.asunto, t.T("vacation.subjectHint", nil), g.ancho),
		campo(t.T("common.message", nil), g.cuerpo, t.T("vacation.bodyHint", nil), g.ancho),
		fechas,
		previo)
}

func (g *dialogoVacaciones) cargar() {
	g.estado = Cargando
	g.pintar()
	pedir(g.b, g.b.S.Vacation, func(v api.MailVacation, err error) {
		if err != nil {
			g.estado = Error
			g.pintar()
			return
		}
		g.conmutador.Encendido = v.Enabled
		g.asunto.PonerTexto(v.Subject)
		g.cuerpo.PonerTexto(v.Body)
		g.desde.PonerTexto("")
		g.hasta.PonerTexto("")
		if v.StartsAt != nil {
			g.desde.PonerTexto(ui.EscribirFecha(v.StartsAt.Local()))
		}
		if v.EndsAt != nil {
			g.hasta.PonerTexto(ui.EscribirFecha(v.EndsAt.Local()))
		}
		g.estado = Listo
		g.pintar()
	})
}

// fechas lee las dos fechas. Vacías valen (sin límite); con algo que no es una fecha,
// `ok` es false.
func (g *dialogoVacaciones) fechas() (desde, hasta time.Time, ok bool) {
	d, _, okD := ui.LeerFecha(g.desde.Texto(), time.Local)
	h, _, okH := ui.LeerFecha(g.hasta.Texto(), time.Local)
	return d, h, okD && okH
}

// alReves es `badRange`: la vuelta antes que la salida.
func (g *dialogoVacaciones) alReves() bool {
	d, h, _ := g.fechas()
	return !d.IsZero() && !h.IsZero() && h.Before(d)
}

// pintar pone el cuerpo del estado en que esté y, con el formulario, lo que depende
// de lo tecleado. Después vuelve a medir el diálogo.
func (g *dialogoVacaciones) pintar() {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var cuerpo fyne.CanvasObject
	switch g.estado {
	case Cargando:
		// `flex items-center gap-2 py-6` con la rueda de 16 y «Cargando…».
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("vacation.loading", nil)
		cuerpo = container.New(ui.Margen{Arr: tema.Espacio(6), Abj: tema.Espacio(6)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	case Error:
		// `flex flex-col items-start gap-2 py-2`: el aviso con su icono y «Reintentar».
		icono := iconos.Trazo(iconos.Aviso, 16, "text", v)
		cuerpo = container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(2)},
				container.New(ui.Fila{Hueco: tema.Espacio(2)}, container.New(ui.Margen{Arr: tema.Espacio(0.5)}, icono.Objeto()),
					ui.NuevoRotulo(t.T("vacation.loadError", nil), tema.TextoSM, ui.Normal, "text")),
				container.New(ui.Fila{}, ui.NuevoBoton(t.T("common.retry", nil), g.cargar).Con(ui.Secundario, ui.TallaSM))))
	default:
		g.repintarFormulario()
		cuerpo = g.form
	}
	g.guardar.Inerte = g.estado != Listo || g.alReves()
	g.guardar.Refresh()
	g.caja.Objects = []fyne.CanvasObject{cuerpo}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// repintarFormulario pone el estado, las pistas de las fechas y la vista previa.
func (g *dialogoVacaciones) repintarFormulario() {
	t, l := g.b.P.T, g.b.P.Locale
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	desde, hasta, _ := g.fechas()
	tono, rotulo, detalle := EstadoVacaciones(t, l, g.conmutador.Encendido, desde, hasta, time.Now())
	g.insignia.Texto, g.insignia.Tono = rotulo, tono
	g.insignia.Refresh()
	g.detalle.Texto = detalle
	g.detalle.Refresh()

	pista := t.T("vacation.fromHintEmpty", nil)
	if !desde.IsZero() {
		pista = t.T("vacation.fromHintSet", i18n.Vars{"date": i18n.FormatDayMonthLong(l, desde)})
	}
	ponerPista(g.grupoDesde, pista, ui.AyudaNeutra, g.anchoFecha)
	pista, tono2 := t.T("vacation.untilHintEmpty", nil), ui.AyudaNeutra
	switch {
	case g.alReves():
		pista, tono2 = t.T("vacation.untilHintBadRange", nil), ui.AyudaPeligro
	case !hasta.IsZero():
		pista = t.T("vacation.untilHintSet", i18n.Vars{"date": i18n.FormatDayMonthLong(l, hasta)})
	}
	ponerPista(g.grupoHasta, pista, tono2, g.anchoFecha)
	if invalida := tono2 == ui.AyudaPeligro; g.hasta.Invalida != invalida {
		g.hasta.Invalida = invalida
		g.hasta.Refresh()
	}

	asunto := strings.TrimSpace(g.asunto.Texto())
	if asunto == "" {
		asunto = t.T("vacation.subjectPlaceholder", nil)
	}
	g.previoAsunto.Texto = asunto
	g.previoAsunto.Refresh()
	var lineas []fyne.CanvasObject
	if texto := strings.TrimSpace(g.cuerpo.Texto()); texto != "" {
		// `text-sm leading-relaxed whitespace-pre-wrap`: un párrafo por línea, y la
		// línea en blanco ocupa su caja de línea.
		relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
		for _, linea := range strings.Split(texto, "\n") {
			if strings.TrimSpace(linea) == "" {
				hueco := canvas.NewRectangle(color.Transparent)
				hueco.SetMinSize(fyne.NewSize(0, relajado.Linea))
				lineas = append(lineas, hueco)
				continue
			}
			lineas = append(lineas, ui.NuevoParrafo(linea, relajado, ui.Normal, "text-muted", v).A(g.ancho-2*tema.Espacio(3)-2*tema.Borde))
		}
	} else {
		// `text-sm italic text-text-subtle`.
		lineas = append(lineas, ui.NuevoParrafo(t.T("vacation.previewEmpty", nil), tema.TextoSM, ui.Cursiva, "text-subtle", v).
			A(g.ancho-2*tema.Espacio(3)-2*tema.Borde))
	}
	g.previoCuerpo.Objects = lineas
	g.previoCuerpo.Refresh()
}

// ponerPista cambia la pista del grupo (su último hijo). Se cambia la pieza y no su
// texto porque el icono del tono de peligro se decide al montarla.
func ponerPista(grupo *fyne.Container, texto string, tono ui.TonoAyuda, ancho float32) {
	i := len(grupo.Objects) - 1
	if p, es := grupo.Objects[i].(*ui.PistaCampo); es && p.Texto == texto && p.Tono == tono {
		return
	}
	p := ui.NuevaPistaCampo(texto).ConTono(tono)
	p.Ancho = ancho
	grupo.Objects[i] = p
	grupo.Refresh()
}

// salvar es `handleSave`. Nunca sin una carga buena ni con el rango al revés.
func (g *dialogoVacaciones) salvar() {
	if g.estado != Listo || g.alReves() || g.guardar.Cargando {
		return
	}
	desde, hasta, ok := g.fechas()
	if !ok {
		g.b.avisar(ui.AvisoPeligro, "vacation.saveFailed", nil)
		return
	}
	inicio, fin := PeriodoVacaciones(desde, hasta)
	vac := api.MailVacation{
		Enabled: g.conmutador.Encendido, Subject: strings.TrimSpace(g.asunto.Texto()),
		Body: g.cuerpo.Texto(), StartsAt: inicio, EndsAt: fin,
	}
	g.guardar.Cargando, g.cancelar.Inerte = true, true
	g.guardar.Refresh()
	g.cancelar.Refresh()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.SetVacation(ctx, vac)
	}, func(_ struct{}, err error) {
		g.guardar.Cargando, g.cancelar.Inerte = false, false
		g.guardar.Refresh()
		g.cancelar.Refresh()
		if err != nil {
			g.b.avisar(ui.AvisoPeligro, "vacation.saveFailed", nil)
			return
		}
		g.b.avisar(ui.AvisoExito, "vacation.saved", nil)
		g.d.Cerrar()
	})
}

// margen es un `p-N` igual por los cuatro lados.
func margen(n float32) ui.Margen { return ui.Margen{Izq: n, Arr: n, Der: n, Abj: n} }

// enCajaDe es `rounded-md border` con ese fondo alrededor del contenido.
func enCajaDe(fondo color.NRGBA, v tema.Variante, dentro fyne.CanvasObject) fyne.CanvasObject {
	caja := ui.NuevaCaja(fondo, tema.Color("border", v), tema.RadioMD)
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(), dentro)...)
}
