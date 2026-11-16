// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"encoding/json"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Los ajustes de Calendar (`CalendarSettings.tsx`)
//
// Cómo nace un evento nuevo (duración y recordatorio) y qué enseña el calendario
// (jornada, fines de semana, número de semana). Son de quien mira: la web los deja en
// `localStorage` con su `id` en la clave y aquí van al almacén POR PERSONA de la sesión,
// con el MISMO JSON (apps/calendar/src/lib/preferencias.ts), para que lo guardado se
// lea igual en las dos plataformas.

// ClaveAjustes es la clave del almacén por persona.
const ClaveAjustes = "sytue.calendar.ajustes"

// Ajustes son los de la persona. El valor cero no es válido: `Normalizar` lo vuelve
// los de siempre.
type Ajustes struct {
	// Duracion son los minutos de un evento nuevo creado con un clic.
	Duracion int `json:"duracion"`
	// Recordatorio son los minutos antes del recordatorio por defecto; nil, ninguno.
	Recordatorio *int `json:"recordatorio"` //nolint:tagliatelle // el JSON es el de la web
	// InicioLaboral y FinLaboral son la jornada en horas [inicio, fin).
	InicioLaboral int `json:"inicioLaboral"` //nolint:tagliatelle // el JSON es el de la web
	FinLaboral    int `json:"finLaboral"`    //nolint:tagliatelle // el JSON es el de la web
	// FinesDeSemana: con false, la semana enseña de lunes a viernes.
	FinesDeSemana bool `json:"finesDeSemana"` //nolint:tagliatelle // el JSON es el de la web
	// NumeroSemana pone el número de semana ISO en la cabecera de la semana.
	NumeroSemana bool `json:"numeroSemana"` //nolint:tagliatelle // el JSON es el de la web
}

// Duraciones y Recordatorios son lo que se ofrece, como `DURACIONES` y `RECORDATORIOS`.
var (
	Duraciones    = []int{15, 30, 45, 60, 90, 120}
	Recordatorios = []int{0, 5, 10, 15, 30, 60, 1440}
)

// AjustesPorDefecto son los de siempre: una hora, sin recordatorio, de 9 a 18, la
// semana entera y sin número.
func AjustesPorDefecto() Ajustes {
	return Ajustes{Duracion: 60, InicioLaboral: 9, FinLaboral: 18, FinesDeSemana: true}
}

// ajustesLeidos es lo leído antes de normalizar: los campos que falten se distinguen
// de los puestos a cero o a false.
type ajustesLeidos struct {
	Duracion      *int  `json:"duracion"`
	Recordatorio  *int  `json:"recordatorio"`  //nolint:tagliatelle // el JSON es el de la web
	InicioLaboral *int  `json:"inicioLaboral"` //nolint:tagliatelle // el JSON es el de la web
	FinLaboral    *int  `json:"finLaboral"`    //nolint:tagliatelle // el JSON es el de la web
	FinesDeSemana *bool `json:"finesDeSemana"` //nolint:tagliatelle // el JSON es el de la web
	NumeroSemana  *bool `json:"numeroSemana"`  //nolint:tagliatelle // el JSON es el de la web
}

// LeerAjustes es `normalizarAjustes(JSON.parse(...))`: un campo roto vuelve al valor de
// siempre sin arrastrar a los demás, y una jornada al revés se descarta entera.
func LeerAjustes(texto string) Ajustes {
	a := AjustesPorDefecto()
	var l ajustesLeidos
	if texto == "" || json.Unmarshal([]byte(texto), &l) != nil {
		return a
	}
	if l.Duracion != nil && slices.Contains(Duraciones, *l.Duracion) {
		a.Duracion = *l.Duracion
	}
	if l.Recordatorio != nil && slices.Contains(Recordatorios, *l.Recordatorio) {
		r := *l.Recordatorio
		a.Recordatorio = &r
	}
	ini, fin := a.InicioLaboral, a.FinLaboral
	if l.InicioLaboral != nil && *l.InicioLaboral >= 0 && *l.InicioLaboral <= 23 {
		ini = *l.InicioLaboral
	}
	if l.FinLaboral != nil && *l.FinLaboral >= 1 && *l.FinLaboral <= 24 {
		fin = *l.FinLaboral
	}
	if fin > ini {
		a.InicioLaboral, a.FinLaboral = ini, fin
	}
	if l.FinesDeSemana != nil {
		a.FinesDeSemana = *l.FinesDeSemana
	}
	if l.NumeroSemana != nil {
		a.NumeroSemana = *l.NumeroSemana
	}
	return a
}

// Texto es el JSON que se guarda, el mismo que escribe la web.
func (a Ajustes) Texto() string {
	datos, _ := json.Marshal(a)
	return string(datos)
}

// ajustes son los de la pantalla; sin poner (valor cero), los de siempre.
func (p *Pantalla) ajustes() Ajustes {
	if p.Ajustes.Duracion == 0 {
		return AjustesPorDefecto()
	}
	return p.Ajustes
}

// restaurarAjustes lee los de quien acaba de entrar.
func (a *App) restaurarAjustes() {
	a.P.Ajustes = AjustesPorDefecto()
	if a.Almacen != nil {
		a.P.Ajustes = LeerAjustes(a.Almacen.Leer(ClaveAjustes))
	}
}

// cambiarAjustes guarda los nuevos y repinta: la página sigue abierta con el valor
// puesto y el calendario lo aplica al volver a él.
func (a *App) cambiarAjustes(nuevos Ajustes) {
	a.P.Ajustes = nuevos
	if a.Almacen != nil {
		a.Almacen.Poner(ClaveAjustes, nuevos.Texto())
	}
	a.pintar()
}

// etiquetaDuracion es la de la web: «30 min», «2 h».
func (p *Pantalla) etiquetaDuracion(minutos int) string {
	if minutos%60 == 0 {
		return p.t("settings.durationHours", i18n.Vars{"count": minutos / 60})
	}
	return p.t("settings.durationMinutes", i18n.Vars{"count": minutos})
}

// AnchoPaginaAjustes es el `max-w-5xl` de `SettingsShell`.
const AnchoPaginaAjustes float32 = 64 * tema.Rem

// controlesAjustes son los controles de la página, para las pruebas.
type controlesAjustes struct {
	duracion, recordatorio, inicio, fin *ui.Selector
	finde, numero                       *ui.Conmutador
	volver                              *ui.Boton
}

// paginaAjustes es `CalendarSettings`: título, frase y tres secciones en tarjetas, cada
// valor guardado al elegirlo.
func (p *Pantalla) paginaAjustes() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	aj := p.ajustes()
	poner := func(f func(*Ajustes)) {
		n := aj
		f(&n)
		if p.AlCambiarAjustes != nil {
			p.AlCambiarAjustes(n)
		}
	}
	c := &controlesAjustes{}
	p.controlesAjustes = c

	// Nuevos eventos: duración y recordatorio.
	dur := make([]string, len(Duraciones))
	for i, m := range Duraciones {
		dur[i] = p.etiquetaDuracion(m)
	}
	c.duracion = ui.NuevoSelector(dur, func(i int) { poner(func(n *Ajustes) { n.Duracion = Duraciones[i] }) })
	c.duracion.Elegida = slices.Index(Duraciones, aj.Duracion)
	c.duracion.Rotulo = p.t("settings.duration", nil)

	rec := make([]string, 0, 1+len(Recordatorios))
	rec = append(rec, p.t("settings.reminderNone", nil))
	for _, m := range Recordatorios {
		rec = append(rec, p.EtiquetaRecordatorio(m))
	}
	c.recordatorio = ui.NuevoSelector(rec, func(i int) {
		poner(func(n *Ajustes) {
			n.Recordatorio = nil
			if i > 0 {
				m := Recordatorios[i-1]
				n.Recordatorio = &m
			}
		})
	})
	c.recordatorio.Elegida = 0
	if aj.Recordatorio != nil {
		c.recordatorio.Elegida = slices.Index(Recordatorios, *aj.Recordatorio) + 1
	}
	c.recordatorio.Rotulo = p.t("settings.reminder", nil)

	// Jornada: solo las horas que dejan una jornada al derecho.
	hora := func(h int) string { return fmt.Sprintf("%02d:00", h) }
	var inicios, fines []string
	for h := 0; h < aj.FinLaboral; h++ {
		inicios = append(inicios, hora(h))
	}
	for h := aj.InicioLaboral + 1; h <= 24; h++ {
		fines = append(fines, hora(h))
	}
	c.inicio = ui.NuevoSelector(inicios, func(i int) { poner(func(n *Ajustes) { n.InicioLaboral = i }) })
	c.inicio.Elegida = aj.InicioLaboral
	c.inicio.Rotulo = p.t("settings.workStart", nil)
	c.fin = ui.NuevoSelector(fines, func(i int) {
		poner(func(n *Ajustes) { n.FinLaboral = aj.InicioLaboral + 1 + i })
	})
	c.fin.Elegida = aj.FinLaboral - aj.InicioLaboral - 1
	c.fin.Rotulo = p.t("settings.workEnd", nil)

	// Vista.
	c.finde = ui.NuevoConmutador(p.t("settings.weekends", nil), func(si bool) {
		poner(func(n *Ajustes) { n.FinesDeSemana = si })
	})
	c.finde.Encendido = aj.FinesDeSemana
	c.numero = ui.NuevoConmutador(p.t("settings.weekNumbers", nil), func(si bool) {
		poner(func(n *Ajustes) { n.NumeroSemana = si })
	})
	c.numero.Encendido = aj.NumeroSemana

	c.volver = ui.NuevoBoton(p.t("settings.back", nil), func() {
		if p.AlCerrarAjustes != nil {
			p.AlCerrarAjustes()
		}
	}).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.ChevronIzquierda, iconos.TallaSM)

	campo := func(clave string, s *ui.Selector) fyne.CanvasObject {
		return ui.EnColumna(tema.Espacio(1.5),
			ui.NuevoRotulo(p.t(clave, nil), tema.TextoSM, ui.SemiNegrita, "text"), s)
	}
	snug := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	seccion := func(titulo, desc string, cuerpo fyne.CanvasObject) fyne.CanvasObject {
		return ui.NuevaTarjeta("", ui.EnColumna(tema.Espacio(4),
			ui.EnColumna(tema.Espacio(1),
				ui.NuevoParrafo(p.t(titulo, nil), ui.EscalonTitulo(4), ui.PesoTitulo(4), "text", v),
				ui.NuevoParrafo(p.t(desc, nil), snug, ui.Normal, "text-muted", v)),
			cuerpo))
	}
	relajado := tema.Escalon{Tam: tema.TextoBase.Tam, Linea: 1.625 * tema.TextoBase.Tam}
	cabecera := container.NewBorder(nil, nil, nil, container.NewVBox(c.volver),
		ui.EnColumna(tema.Espacio(1.5),
			ui.NuevoParrafo(p.t("settings.title", nil), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text", v),
			ui.NuevoParrafo(p.t("settings.description", nil), relajado, ui.Normal, "text-muted", v)))
	pagina := ui.EnColumna(tema.Espacio(6),
		cabecera,
		seccion("settings.eventsTitle", "settings.eventsDescription",
			container.NewGridWithColumns(2, campo("settings.duration", c.duracion), campo("settings.reminder", c.recordatorio))),
		seccion("settings.workTitle", "settings.workDescription",
			container.NewGridWithColumns(2, campo("settings.workStart", c.inicio), campo("settings.workEnd", c.fin))),
		seccion("settings.viewTitle", "settings.viewDescription",
			ui.EnColumna(tema.Espacio(3), c.finde, c.numero)),
	)
	marco := container.New(&anchoAcotado{max: AnchoPaginaAjustes, canalon: tema.Espacio(6)}, pagina)
	return container.NewStack(canvas.NewRectangle(tema.Color(ui.FondoHoja(), v)), container.NewVScroll(marco))
}

// anchoAcotado centra su hijo con un ancho máximo y un canalón a los lados y arriba
// (`mx-auto max-w-5xl px-6 py-6`).
type anchoAcotado struct {
	max, canalon float32
}

func (l *anchoAcotado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*l.canalon, m.Height+2*l.canalon)
}

func (l *anchoAcotado) Layout(objs []fyne.CanvasObject, tam fyne.Size) {
	ancho := min(tam.Width-2*l.canalon, l.max)
	x := (tam.Width - ancho) / 2
	objs[0].Move(fyne.NewPos(x, l.canalon))
	objs[0].Resize(fyne.NewSize(ancho, max(objs[0].MinSize().Height, tam.Height-2*l.canalon)))
}
