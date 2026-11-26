// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
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

// # La página de ajustes (`MailSettings.tsx`, design.md §8.4)
//
// Antes eran ocho filas sueltas en una sección plegada del lateral, cada una abriendo
// su diálogo sin decir qué cambiaba: se leían como carpetas y «Certificado de firma del
// correo» no explicaba para qué servía hasta pulsarlo. Ahora el lateral tiene UNA
// entrada, «Ajustes», que pone en lugar de la lista y el lector una página por temas
// (`SettingsShell` + `SettingsSection`): cada fila con su frase llana, su estado cuando
// se sabe sin abrir nada (la firma está puesta; la autorespuesta responde ahora) y su
// botón, que abre el editor de siempre.

// Medidas de la página: `SettingsShell` con el rem de 16.
const (
	// BaseTextoFilaAjuste es el `basis-64` del texto de una fila: por debajo, el
	// botón baja a su propia línea (`flex-wrap`).
	BaseTextoFilaAjuste float32 = 64 * 0.25 * tema.Rem
)

// entradaAjuste es una fila de la página: la clave del editor (la misma que en la web,
// `AjusteMail`), su nombre, para qué sirve y el verbo del botón.
type entradaAjuste struct {
	clave, titulo, descripcion, accion string
}

// grupoAjustes es una sección de la página, en el orden de `MailSettings.tsx`.
type grupoAjustes struct {
	titulo, descripcion string
	filas               []entradaAjuste
}

var gruposAjustes = []grupoAjustes{
	{titulo: "settings.writingTitle", filas: []entradaAjuste{
		{"signature", "nav.signature", "settings.signatureDescription", "settings.edit"},
		{"vacation", "nav.vacation", "settings.vacationDescription", "settings.edit"},
	}},
	{titulo: "settings.addressesTitle", filas: []entradaAjuste{
		{"aliases", "nav.aliases", "settings.aliasesDescription", "settings.manage"},
		{"shared", "nav.sharedAndDelegation", "settings.sharedDescription", "settings.manage"},
	}},
	{titulo: "settings.sortingTitle", filas: []entradaAjuste{
		{"rules", "nav.rules", "settings.rulesDescription", "settings.manage"},
	}},
	{titulo: "settings.securityTitle", descripcion: "settings.securityDescription", filas: []entradaAjuste{
		{"smime-anchor", "nav.smimeAnchor", "settings.anchorDescription", "settings.download"},
		{"smime-identity", "nav.smimeIdentity", "settings.identityDescription", "settings.install"},
	}},
	{titulo: "settings.clientsTitle", filas: []entradaAjuste{
		{"mail-clients", "nav.mailClients", "settings.mailClientsDescription", "settings.openId"},
	}},
}

// EstadoAjuste es la insignia de una fila y la frase que la explica, si se sabe.
type EstadoAjuste struct {
	Tono    ui.Tono
	Rotulo  string
	Detalle string
}

// PaginaAjustes es el ESTADO de la página: lo que la app sabe y la acción de cada fila.
type PaginaAjustes struct {
	T *i18n.Translator
	// Ventana es el ancho de la ventana: decide el canalón (`px-4 sm:px-6`).
	Ventana float32
	// Firma dice si hay firma guardada; Vacaciones, el estado de la autorespuesta o nil
	// si no se ha podido leer (la fila se queda sin insignia: no se inventa «apagada»).
	Firma      bool
	Vacaciones *EstadoAjuste
	// AlAbrir abre el editor de una fila, por su clave.
	AlAbrir func(clave string)
	// Ocultas son las filas que no se ofrecen, por su clave (la edición local no tiene
	// alias, compartidos, S/MIME ni «Clientes de correo»). Un grupo sin filas no sale.
	// Nil: todas, como siempre.
	Ocultas map[string]bool

	// desplaza dura lo que la página: rehacerla con cada dato no la devuelve arriba.
	desplaza *container.Scroll
	// Botones por clave, para las pruebas y para el foco.
	botones map[string]*ui.Boton
}

// estado es la insignia de una fila, o nil si esa fila no tiene.
func (p *PaginaAjustes) estado(clave string) *EstadoAjuste {
	switch clave {
	case "signature":
		if p.Firma {
			return &EstadoAjuste{Tono: ui.InsigniaExito, Rotulo: p.T.T("settings.signatureOn", nil)}
		}
		return &EstadoAjuste{Tono: ui.InsigniaNeutra, Rotulo: p.T.T("settings.signatureOff", nil)}
	case "vacation":
		return p.Vacaciones
	}
	return nil
}

// Objeto monta la página sobre el fondo de la hoja: el `SettingsShell` de la suite con
// una `SettingsSection` por grupo (§2.8: no se reimplementan por app).
//
//nolint:gocognit // Una sección por grupo de ajustes, montada de una vez como el SettingsShell de la web.
func (p *PaginaAjustes) Objeto(v tema.Variante) fyne.CanvasObject {
	t := p.T
	p.botones = map[string]*ui.Boton{}

	snug := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}
	secciones := make([]fyne.CanvasObject, 0, len(gruposAjustes))
	for _, g := range gruposAjustes {
		lista := &listaAjustes{}
		var objetos []fyne.CanvasObject
		filas := g.filas
		if len(p.Ocultas) > 0 {
			filas = nil
			for _, e := range g.filas {
				if !p.Ocultas[e.clave] {
					filas = append(filas, e)
				}
			}
			if len(filas) == 0 {
				continue
			}
		}
		for i, e := range filas {
			f := &filaAjuste{clave: e.clave}
			if i > 0 {
				// `divide-y divide-border`: el filete va ENTRE filas, no encima de la primera.
				f.linea = canvas.NewRectangle(tema.Color("border", v))
				objetos = append(objetos, f.linea)
			}
			f.titulo = ui.NuevoRotulo(t.T(e.titulo, nil), tema.TextoSM, ui.SemiNegrita, "text")
			f.desc = ui.NuevoParrafo(t.T(e.descripcion, nil), snug, ui.Normal, "text-muted", v)
			objetos = append(objetos, f.titulo, f.desc)
			if st := p.estado(e.clave); st != nil {
				f.insignia = ui.NuevaInsignia(st.Rotulo, st.Tono)
				objetos = append(objetos, f.insignia)
				// El detalle solo cuando dice algo que la descripción no («responde hasta
				// el 14 de marzo»). Apagada, la insignia ya lo dice todo.
				if st.Detalle != "" && st.Tono != ui.InsigniaNeutra {
					f.detalle = ui.NuevoParrafo(st.Detalle, snug, ui.Normal, "text", v)
					objetos = append(objetos, f.detalle)
				}
			}
			clave := e.clave
			f.boton = ui.NuevoBoton(t.T(e.accion, nil), func() {
				if p.AlAbrir != nil {
					p.AlAbrir(clave)
				}
			}).Con(ui.Secundario, ui.TallaSM)
			// El rótulo solo («Gestionar») se repite en varias filas: el nombre accesible
			// dice QUÉ se gestiona, como lo diría quien lo ve al lado.
			f.boton.Rotulo = t.T(e.accion, nil) + ": " + t.T(e.titulo, nil)
			if clave == "mail-clients" {
				f.boton.IconoDetras = iconos.FlechaArribaDerecha
			}
			p.botones[clave] = f.boton
			objetos = append(objetos, f.boton)
			lista.filas = append(lista.filas, f)
		}
		descripcion := ""
		if g.descripcion != "" {
			descripcion = t.T(g.descripcion, nil)
		}
		secciones = append(secciones, ui.NuevaSeccionAjustes(t.T(g.titulo, nil), descripcion, container.New(lista, objetos...)))
	}
	marco := ui.NuevoMarcoAjustes(t.T("settings.title", nil), t.T("settings.description", nil), secciones...)
	// La hoja va junto al menú: el `sm:px-6` lo decide la ventana. Y hasta su primer
	// Layout, el desplazamiento la mide a lo que mide la hoja.
	marco.Ventana = p.Ventana
	marco.Resize(fyne.NewSize(max(ui.AnchoHoja(p.Ventana, ui.AnchoMenu(false), 0), 1), 0))

	if p.desplaza == nil {
		p.desplaza = container.NewVScroll(marco)
	} else {
		p.desplaza.Content = marco
		p.desplaza.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color(ui.FondoHoja(), v)), p.desplaza)
}

// Boton es el botón de una fila, por su clave (nil si no está montada).
func (p *PaginaAjustes) Boton(clave string) *ui.Boton { return p.botones[clave] }

type filaAjuste struct {
	clave     string
	linea     *canvas.Rectangle
	titulo    *ui.Rotulo
	insignia  *ui.Insignia
	desc      *ui.Parrafo
	detalle   *ui.Parrafo
	boton     *ui.Boton
	apilada   bool // el botón bajó a su línea: no cabía al lado del texto
	anchoText float32
}

// listaAjustes es lo de dentro de una sección: las filas con sus filetes, con el
// `-my-2` de la lista (sube 8 hacia el título y baja 8 en el borde de la tarjeta).
// Todo depende del ancho —el reparto de cada párrafo y si el botón cabe al lado del
// texto—, así que dice su alto a un ancho (`AltoCon`) y la sección lo pide así.
type listaAjustes struct{ filas []*filaAjuste }

// AltoCon es el alto de la lista a ese ancho.
func (l *listaAjustes) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 {
	alto := -2 * tema.Espacio(2)
	for _, f := range l.filas {
		alto += f.medir(ancho)
	}
	return alto
}

func (l *listaAjustes) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, l.AltoCon(objs, BaseTextoFilaAjuste))
}

func (l *listaAjustes) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	y := -tema.Espacio(2)
	for _, f := range l.filas {
		h := f.medir(t.Width)
		if f.linea != nil {
			f.linea.Move(fyne.NewPos(0, y))
			f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
		}
		f.colocar(0, y, t.Width, h)
		y += h
	}
}

// altoFila mide una fila a ese ancho interior y decide si el botón va al lado.
func (f *filaAjuste) medir(ancho float32) float32 {
	b := f.boton.MinSize()
	f.apilada = ancho-b.Width-tema.Espacio(4) < BaseTextoFilaAjuste
	f.anchoText = ancho
	if !f.apilada {
		f.anchoText = ancho - b.Width - tema.Espacio(4)
	}
	texto := max(f.titulo.MinSize().Height, f.altoInsignia()) +
		tema.Espacio(0.5) + ui.AltoParrafo(f.desc, f.anchoText)
	if f.detalle != nil {
		texto += tema.Espacio(0.5) + ui.AltoParrafo(f.detalle, f.anchoText)
	}
	if f.apilada {
		return 2*tema.Espacio(3) + texto + tema.Espacio(2) + b.Height
	}
	return 2*tema.Espacio(3) + max(texto, b.Height)
}

func (f *filaAjuste) altoInsignia() float32 {
	if f.insignia == nil {
		return 0
	}
	return f.insignia.MinSize().Height
}

// colocar pone la fila ya medida en su caja.
func (f *filaAjuste) colocar(x, y, ancho, alto float32) {
	b := f.boton.MinSize()
	texto := alto - 2*tema.Espacio(3)
	if f.apilada {
		texto -= tema.Espacio(2) + b.Height
	}
	yt := y + tema.Espacio(3)
	if !f.apilada {
		// `items-center`: el bloque de texto y el botón, centrados entre sí.
		yt += (texto - f.altoTexto()) / 2
	}
	linea := max(f.titulo.MinSize().Height, f.altoInsignia())
	tt := f.titulo.MinSize()
	f.titulo.Move(fyne.NewPos(x, yt+(linea-tt.Height)/2))
	f.titulo.Resize(fyne.NewSize(min(tt.Width, f.anchoText), tt.Height))
	if f.insignia != nil {
		is := f.insignia.MinSize()
		f.insignia.Move(fyne.NewPos(x+min(tt.Width, f.anchoText)+tema.Espacio(2), yt+(linea-is.Height)/2))
		f.insignia.Resize(is)
	}
	yd := yt + linea + tema.Espacio(0.5)
	ponerParrafo(f.desc, x, yd, f.anchoText)
	if f.detalle != nil {
		ponerParrafo(f.detalle, x, yd+ui.AltoParrafo(f.desc, f.anchoText)+tema.Espacio(0.5), f.anchoText)
	}
	if f.apilada {
		f.boton.Move(fyne.NewPos(x, y+alto-tema.Espacio(3)-b.Height))
	} else {
		f.boton.Move(fyne.NewPos(x+ancho-b.Width, y+(alto-b.Height)/2))
	}
	f.boton.Resize(b)
}

func (f *filaAjuste) altoTexto() float32 {
	alto := max(f.titulo.MinSize().Height, f.altoInsignia()) + tema.Espacio(0.5) + ui.AltoParrafo(f.desc, f.anchoText)
	if f.detalle != nil {
		alto += tema.Espacio(0.5) + ui.AltoParrafo(f.detalle, f.anchoText)
	}
	return alto
}

func ponerParrafo(p *ui.Parrafo, x, y, ancho float32) {
	p.Move(fyne.NewPos(x, y))
	p.Resize(ui.MedidaParrafo(p, ancho))
}

// ---- El buzón ----

// verAjustes pone la página de ajustes en lugar de la lista y el lector.
func (b *Buzon) verAjustes() {
	b.P.CerrarCajon()
	if b.P.Ajustes == nil {
		b.P.Ajustes = &PaginaAjustes{T: b.P.T, AlAbrir: b.abrirAjuste}
	}
	b.P.Ajustes.Ocultas = b.ajustesOcultos()
	if c := b.P.Lienzo(); c != nil {
		b.P.Ajustes.Ventana = c.Size().Width
	}
	b.P.Ajustes.Firma = strings.TrimSpace(b.firma) != ""
	b.P.Refrescar()
	b.leerEstadoAjustes()
}

// dejarAjustes cierra la página: elegir una carpeta, una etiqueta o un buzón en el
// lateral es navegación, como cualquier otra entrada del menú. Repinta quien llama.
func (b *Buzon) dejarAjustes() { b.P.Ajustes = nil }

// leerEstadoAjustes vuelve a leer lo que la página enseña sin abrir nada. Se llama al
// abrirla y tras guardar en un editor: lo recién guardado es lo primero que se mira.
func (b *Buzon) leerEstadoAjustes() {
	if b.P.Ajustes == nil {
		return
	}
	b.P.Ajustes.Firma = strings.TrimSpace(b.firma) != ""
	if b.S == nil {
		b.P.Refrescar()
		return
	}
	// La respuesta automática por cuenta, en el servidor de cada una (la edición local,
	// M-10): se pregunta a cada servidor ahora, que la persona está mirando los Ajustes.
	if rs, ok := b.S.(api.MailRespuestas); ok {
		b.leerAutorespuestas(rs)
		return
	}
	pedir(b, b.S.Vacation, func(v api.MailVacation, err error) {
		p := b.P.Ajustes
		if p == nil {
			return
		}
		p.Vacaciones = nil
		if err == nil {
			tono, rotulo, detalle := EstadoVacaciones(b.P.T, b.P.Locale, v.Enabled, diaLocal(v.StartsAt), diaLocal(v.EndsAt), time.Now())
			p.Vacaciones = &EstadoAjuste{Tono: tono, Rotulo: rotulo, Detalle: detalle}
		}
		b.P.Refrescar()
	})
}

// diaLocal es el día (a las 00:00 locales) de una fecha del servidor, o cero.
func diaLocal(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}
