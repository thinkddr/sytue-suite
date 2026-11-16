// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// OpcionesBusqueda son los campos del panel «Opciones de búsqueda». Es
// `OpcionesBusqueda` de utils.ts, campo a campo.
//
// POR QUÉ EXISTE: el servicio entiende `from:`, `to:`, `subject:`, `has:attachment`,
// `is:unread`, `after:` y `before:` (services/mail/internal/domain/searchquery.go),
// pero eso solo lo sabía quien lo hubiera leído en un `title` en inglés técnico. El
// panel pregunta en lenguaje llano y escribe la consulta; los nombres de los
// operadores no se traducen porque son los que entiende el servicio.
//
// Los vectores de testdata/consulta-busqueda.json usan los nombres de utils.ts
// (`conAdjuntos`), que `encoding/json` casa con estos sin distinguir mayúsculas.
type OpcionesBusqueda struct {
	De       string
	Para     string
	Asunto   string
	Contiene string
	// ConAdjuntos y SinLeer son las dos casillas.
	ConAdjuntos bool
	SinLeer     bool
	// Desde y Hasta son AAAA-MM-DD y LOS DOS inclusivos, que es lo que entiende
	// quien rellena «Hasta el 28»; vacío, sin acotar.
	Desde string
	Hasta string
}

// ComponerConsulta escribe la consulta que entiende el servicio. Los casos, en
// testdata/consulta-busqueda.json, los pasa también la web.
//
//   - Un valor con espacios va entre comillas (`subject:"informe anual"`), y las
//     comillas que traiga dentro se quitan: el servicio no tiene cómo escaparlas, y
//     una suelta partiría la consulta por donde no es.
//   - `before:` EXCLUYE su día, así que «Hasta el 28 de febrero» es
//     `before:2026-03-01`. Escribir el 28 tal cual dejaría fuera justo ese día.
//   - Una fecha que no sea AAAA-MM-DD válida no se escribe: el servicio la tomaría
//     por texto y buscaría la palabra.
func ComponerConsulta(o OpcionesBusqueda) string {
	var partes []string
	for _, c := range []struct{ op, valor string }{
		{"from", o.De}, {"to", o.Para}, {"subject", o.Asunto},
	} {
		if v := valorDeOperador(c.valor); v != "" {
			partes = append(partes, c.op+":"+v)
		}
	}
	if o.ConAdjuntos {
		partes = append(partes, "has:attachment")
	}
	if o.SinLeer {
		partes = append(partes, "is:unread")
	}
	if d, ok := leerDia(o.Desde); ok {
		partes = append(partes, "after:"+d.Format(time.DateOnly))
	}
	if d, ok := leerDia(o.Hasta); ok {
		partes = append(partes, "before:"+d.AddDate(0, 0, 1).Format(time.DateOnly))
	}
	if t := strings.Join(strings.Fields(o.Contiene), " "); t != "" {
		partes = append(partes, t)
	}
	return strings.Join(partes, " ")
}

// DesmontarConsulta es lo contrario: al reabrir el panel con una consulta ya escrita,
// cada operador vuelve a su campo. Lee como `ParseSearchQuery` —sin distinguir
// mayúsculas en el operador, con sus sinónimos y ganando el último repetido— y lo que
// el panel no tiene (`is:starred`, un operador que no existe, una fecha mal escrita) se
// queda en «Contiene» tal cual, para que aplicar el panel no lo borre.
func DesmontarConsulta(q string) OpcionesBusqueda {
	var o OpcionesBusqueda
	var libre []string
	for _, tok := range trocearConsulta(q) {
		op, val, ok := strings.Cut(tok.texto, ":")
		if !ok || tok.entrecomillado || val == "" {
			libre = append(libre, tok.texto)
			continue
		}
		val = strings.Trim(val, `"`)
		entendido := true
		switch strings.ToLower(op) {
		case "from":
			o.De = val
		case "to":
			o.Para = val
		case "subject":
			o.Asunto = val
		case "has":
			entendido = strings.EqualFold(val, "attachment") || strings.EqualFold(val, "attachments")
			o.ConAdjuntos = o.ConAdjuntos || entendido
		case "is":
			entendido = strings.EqualFold(val, "unread") || strings.EqualFold(val, "unseen")
			o.SinLeer = o.SinLeer || entendido
		case "after", "since":
			var d time.Time
			if d, entendido = leerDia(val); entendido {
				o.Desde = d.Format(time.DateOnly)
			}
		case "before", "until":
			var d time.Time
			if d, entendido = leerDia(val); entendido {
				o.Hasta = d.AddDate(0, 0, -1).Format(time.DateOnly)
			}
		default:
			entendido = false
		}
		if !entendido {
			libre = append(libre, tok.texto)
		}
	}
	o.Contiene = strings.Join(libre, " ")
	return o
}

// valorDeOperador limpia lo escrito en un campo para ponerlo detrás de `op:`.
func valorDeOperador(v string) string {
	v = strings.Join(strings.Fields(strings.ReplaceAll(v, `"`, "")), " ")
	if strings.Contains(v, " ") {
		return `"` + v + `"`
	}
	return v
}

// leerDia lee AAAA-MM-DD y nada más, con el día de verdad (`2026-02-30` no vale).
func leerDia(s string) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	return d, err == nil
}

// trozoConsulta es una palabra o una frase entre comillas de la consulta.
type trozoConsulta struct {
	texto string
	// entrecomillado es la frase que venía ENTERA entre comillas: texto, aunque lleve
	// dos puntos.
	entrecomillado bool
}

// trocearConsulta parte como `tokenizeQuery` del servicio: por espacios, tabuladores y
// saltos fuera de las comillas, que se conservan, y una comilla sin cerrar llega
// hasta el final.
func trocearConsulta(q string) []trozoConsulta {
	var out []trozoConsulta
	var cur strings.Builder
	enComillas, empezoConComilla := false, false
	soltar := func() {
		if cur.Len() > 0 {
			out = append(out, trozoConsulta{texto: cur.String(), entrecomillado: empezoConComilla})
			cur.Reset()
		}
		empezoConComilla = false
	}
	for _, r := range q {
		switch {
		case r == '"':
			if !enComillas && cur.Len() == 0 {
				empezoConComilla = true
			}
			enComillas = !enComillas
			cur.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n') && !enComillas:
			soltar()
		default:
			cur.WriteRune(r)
		}
	}
	soltar()
	return out
}

// El panel es PanelOpcionesBusqueda.tsx de la web.
//
// AnchoOpcionesBusqueda es el `w-72` del formulario del panel, y HuecoOpcionesBusqueda
// su `gap-3` entre grupos.
var (
	AnchoOpcionesBusqueda float32 = 72 * 0.25 * tema.Rem
	HuecoOpcionesBusqueda         = tema.Espacio(3)
)

// PonerConsulta escribe `q` en el buscador y busca, como si se hubiera tecleado: lo usan
// el panel y «Buscar correos de esta persona».
func (b *Bandeja) PonerConsulta(q string) {
	avisado := false
	if b.r != nil {
		// Escribir en el campo ya avisa a quien busca (es su `OnChanged`); si el texto
		// no cambia, no avisa, y la búsqueda se pide igual abajo.
		antes := b.AlBuscar
		b.AlBuscar = func(s string) {
			avisado = true
			if antes != nil {
				antes(s)
			}
		}
		b.r.busqueda.PonerTexto(q)
		b.AlBuscar = antes
		b.r.Refresh()
	}
	if !avisado && b.AlBuscar != nil {
		b.AlBuscar(q)
	}
}

// panelBusqueda son los controles del panel «Opciones de búsqueda» mientras está
// abierto. Es el `Formulario` de PanelOpcionesBusqueda.tsx.
type panelBusqueda struct {
	de, para, asunto, contiene, desde, hasta *ui.Entrada
	adjuntos, sinLeer                        *ui.Eleccion
	buscar, vaciar                           *ui.Boton
	emergente                                *ui.Emergente
}

// abrirOpcionesBusqueda despliega el panel desde el embudo. Cada apertura desmonta la
// consulta de ese momento: se afina lo que ya se buscaba sin volver a teclearlo.
func (b *Bandeja) abrirOpcionesBusqueda(disparador fyne.CanvasObject) *panelBusqueda {
	t := b.T
	o := DesmontarConsulta(b.Consulta)
	hueco := tema.Espacio(2)
	p := &panelBusqueda{}
	texto := func(valor string) *ui.Entrada {
		e := ui.NuevaEntrada("")
		e.Ancho = AnchoOpcionesBusqueda
		e.PonerTexto(valor)
		return e
	}
	p.de, p.para, p.asunto, p.contiene = texto(o.De), texto(o.Para), texto(o.Asunto), texto(o.Contiene)
	// El `type="date"` de la web: aquí se teclea, con el día delante, y se le pasa al
	// servicio como AAAA-MM-DD.
	fecha := func(iso string) *ui.Entrada {
		e := ui.CampoFecha(ui.CampoMD)
		e.Ancho = (AnchoOpcionesBusqueda - hueco) / 2
		if d, ok := leerDia(iso); ok {
			e.PonerTexto(ui.EscribirFecha(d))
		}
		return e
	}
	p.desde, p.hasta = fecha(o.Desde), fecha(o.Hasta)
	p.adjuntos = ui.NuevaCasilla(t.T("filter.attachments", nil), nil)
	p.sinLeer = ui.NuevaCasilla(t.T("filter.unread", nil), nil)
	p.adjuntos.Marcada, p.sinLeer.Marcada = o.ConAdjuntos, o.SinLeer

	enviar := func() {
		b.PonerConsulta(ComponerConsulta(p.leer()))
		if p.emergente != nil {
			p.emergente.Cerrar()
		}
	}
	p.buscar = ui.NuevoBoton(t.T("list.searchApply", nil), enviar).Con(ui.Primario, ui.TallaSM)
	p.vaciar = ui.NuevoBoton(t.T("list.searchReset", nil), p.limpiar).Con(ui.Fantasma, ui.TallaSM)
	// Intro en cualquier campo es el `submit` del formulario.
	for _, e := range p.campos() {
		e.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			enviar()
			return true
		}
	}

	grupo := func(clave string, control fyne.CanvasObject) fyne.CanvasObject {
		return ui.GrupoCampo(t.T(clave, nil), false, control, "", "")
	}
	formulario := container.New(ui.Columna{Hueco: HuecoOpcionesBusqueda},
		grupo("list.searchFrom", p.de),
		grupo("list.searchTo", p.para),
		grupo("list.searchSubject", p.asunto),
		grupo("list.searchWords", p.contiene),
		container.New(ui.Fila{Hueco: hueco}, grupo("list.searchSince", p.desde), grupo("list.searchUntil", p.hasta)),
		container.New(ui.Columna{Hueco: hueco}, p.adjuntos, p.sinLeer),
		container.New(ui.PieAcciones{Hueco: hueco}, p.vaciar, p.buscar),
	)
	if disparador != nil {
		p.emergente = ui.MostrarPopover(disparador, formulario)
		p.emergente.Rotulo = t.T("list.searchOptions", nil)
	}
	return p
}

func (p *panelBusqueda) campos() []*ui.Entrada {
	return []*ui.Entrada{p.de, p.para, p.asunto, p.contiene, p.desde, p.hasta}
}

// leer es lo que hay en el panel. Una fecha que no se entiende se queda fuera, como
// en ComponerConsulta.
func (p *panelBusqueda) leer() OpcionesBusqueda {
	iso := func(e *ui.Entrada) string {
		d, vacia, ok := ui.LeerFecha(e.Texto(), time.UTC)
		if vacia || !ok {
			return ""
		}
		return d.Format(time.DateOnly)
	}
	return OpcionesBusqueda{
		De: p.de.Texto(), Para: p.para.Texto(), Asunto: p.asunto.Texto(), Contiene: p.contiene.Texto(),
		ConAdjuntos: p.adjuntos.Marcada, SinLeer: p.sinLeer.Marcada,
		Desde: iso(p.desde), Hasta: iso(p.hasta),
	}
}

// limpiar es «Vaciar campos»: el panel en blanco, sin buscar todavía.
func (p *panelBusqueda) limpiar() {
	for _, e := range p.campos() {
		e.PonerTexto("")
	}
	p.adjuntos.Marcada, p.sinLeer.Marcada = false, false
	p.adjuntos.Refresh()
	p.sinLeer.Refresh()
}
