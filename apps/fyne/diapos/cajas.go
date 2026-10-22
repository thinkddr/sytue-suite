// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
)

// Las operaciones de SlidesCanvas.tsx sobre las CAJAS de una diapositiva: borrar,
// mover, redimensionar, organizar y los parches de la mini-barra. Como las de
// `operaciones.go`, dejan el JSON como lo deja la web: lo que la web borra con un
// `delete` aquí queda vacío y lo omite el `omitempty`.

// IndicePatron es el «índice» de las cajas del patrón en las operaciones de cajas: con
// «Editar patrón», lo que se mueve, se escribe, se inserta o se borra es el patrón y
// no la diapositiva (`applyBoxes` de la web). No es −1, que en otras partes es
// «ninguna»; y no es índice de ninguna diapositiva.
const IndicePatron = -100

// IndiceDiseno devuelve el «índice» de edición para el diseño `j` de `Patron.Disenos`:
// edita tanto sus formas fijas (`Decor`, debajo) como sus marcadores (`Cajas`, encima).
func IndiceDiseno(j int) int { return -200 - j }

func (m *Modelo) disenoEn(i int) *Diseno {
	if i <= -200 && m.Patron != nil {
		if j := -200 - i; j >= 0 && j < len(m.Patron.Disenos) {
			return &m.Patron.Disenos[j]
		}
	}
	return nil
}

func (m *Modelo) indiceDiseno(id string) int {
	if id == "" || m.Patron == nil {
		return -1
	}
	for j := range m.Patron.Disenos {
		if m.Patron.Disenos[j].ID == id {
			return j
		}
	}
	return -1
}

// repartirCajasDiseno devuelve cada caja editada de un diseño a `Decor` (formas,
// imágenes y los textos fijos que ya venían de `Decor`) o a `Cajas` (marcadores de
// texto), igual que `applyLayoutBoxes` en la web.
func repartirCajasDiseno(d *Diseno, cs []Caja) {
	enDecor := make(map[string]bool, len(d.Decor))
	for _, c := range d.Decor {
		enDecor[c.ID] = true
	}
	enCajas := make(map[string]bool, len(d.Cajas))
	for _, c := range d.Cajas {
		enCajas[c.ID] = true
	}
	var decor []Caja
	cajas := make([]Caja, 0, len(d.Cajas))
	for _, c := range cs {
		esTexto := c.Tipo == "" || c.Tipo == "text"
		switch {
		case !esTexto:
			decor = append(decor, c)
		case enDecor[c.ID] && !enCajas[c.ID]:
			decor = append(decor, c)
		default:
			cajas = append(cajas, c)
		}
	}
	d.Decor = decor
	d.Cajas = cajas
}

// listaCajas es la lista de cajas que se edita: la de la diapositiva `i` o, con
// IndicePatron, la del patrón, que nace si hace falta y `crear` lo pide. Nil si no hay.
func (m *Modelo) listaCajas(i int, crear bool) *[]Caja {
	if i == IndicePatron {
		if m.Patron == nil {
			if !crear {
				return nil
			}
			m.Patron = &Patron{Cajas: []Caja{}}
		}
		return &m.Patron.Cajas
	}
	if !m.valida(i) {
		return nil
	}
	return &m.Diapos[i].Cajas
}

func (m *Modelo) cambiarCajas(i int, crear bool, fn func([]Caja) []Caja) {
	if d := m.disenoEn(i); d != nil {
		repartirCajasDiseno(d, fn(m.cajas(i)))
		return
	}
	if l := m.listaCajas(i, crear); l != nil {
		*l = fn(*l)
	}
}

// cajas son las de la diapositiva `i` (o del patrón o diseño), o nil si no existe.
func (m *Modelo) cajas(i int) []Caja {
	if d := m.disenoEn(i); d != nil {
		out := make([]Caja, 0, len(d.Decor)+len(d.Cajas))
		out = append(out, d.Decor...)
		return append(out, d.Cajas...)
	}
	if l := m.listaCajas(i, false); l != nil {
		return *l
	}
	return nil
}

// caja es la caja `id` de la diapositiva `i` (o del patrón o diseño), para cambiarla en su sitio.
func (m *Modelo) caja(i int, id string) *Caja {
	if d := m.disenoEn(i); d != nil {
		for j := range d.Decor {
			if d.Decor[j].ID == id {
				return &d.Decor[j]
			}
		}
		for j := range d.Cajas {
			if d.Cajas[j].ID == id {
				return &d.Cajas[j]
			}
		}
		return nil
	}
	l := m.listaCajas(i, false)
	if l == nil {
		return nil
	}
	for j := range *l {
		if (*l)[j].ID == id {
			return &(*l)[j]
		}
	}
	return nil
}

// QuitarCajas es `removeBoxes`: borra las cajas, renumera las animaciones que quedan y
// poda los conectores anclados a alguna de ellas.
func (m *Modelo) QuitarCajas(i int, ids []string) {
	if len(ids) == 0 || (m.disenoEn(i) == nil && m.listaCajas(i, false) == nil) {
		return
	}
	fuera := conjunto(ids)
	m.cambiarCajas(i, false, func(cs []Caja) []Caja {
		return NormalizarOrden(slices.DeleteFunc(slices.Clone(cs), func(c Caja) bool { return fuera[c.ID] }))
	})
	if i < 0 {
		return // el patrón no tiene conectores propios de diapositiva
	}
	d := &m.Diapos[i]
	d.Conectores = slices.DeleteFunc(d.Conectores, func(c Conector) bool {
		return fuera[c.Desde.Caja] || fuera[c.Hasta.Caja]
	})
	if len(d.Conectores) == 0 {
		d.Conectores = nil
	}
}

// cabeDesplazar dice si mover el conjunto cambiaría algo con los topes del lienzo; lo
// usa el teclado para no marcar sucio un empujón contra el borde.
func (m *Modelo) cabeDesplazar(i int, ids []string, dx, dy float64) bool {
	prueba := &Modelo{Diapos: []Diapo{{Cajas: slices.Clone(m.cajas(i))}}}
	return prueba.Desplazar(0, ids, dx, dy)
}

// Desplazar es `nudgeSet`: mueve el conjunto como bloque, topando su envolvente al
// lienzo para que no se salga ni se descuadre. Devuelve si ha cambiado algo.
func (m *Modelo) Desplazar(i int, ids []string, dx, dy float64) bool {
	sel := conjunto(ids)
	var rs []rect
	for _, c := range m.cajas(i) {
		if sel[c.ID] {
			rs = append(rs, rectDe(c))
		}
	}
	if len(rs) == 0 {
		return false
	}
	e := envolvente(rs)
	dx, dy = min(max(dx, -e.x), 100-(e.x+e.w)), min(max(dy, -e.y), 100-(e.y+e.h))
	if dx == 0 && dy == 0 {
		return false
	}
	m.cambiarCajas(i, false, func(cs []Caja) []Caja {
		for j := range cs {
			if c := &cs[j]; sel[c.ID] {
				c.X, c.Y = c.X+dx, c.Y+dy
			}
		}
		return cs
	})
	return true
}

// Redimensionar pone ancho y alto a la caja con los topes de la web: un mínimo de 4 %
// y sin salirse del lienzo.
func (m *Modelo) Redimensionar(i int, id string, w, h float64) {
	if c := m.caja(i, id); c != nil {
		c.W, c.H = min(max(w, 4), 100-c.X), min(max(h, 4), 100-c.Y)
	}
}

// Organizar aplica una operación de «Organizar» a la selección efectiva: front,
// forward, backward, back; align-left… align-bottom; dist-h, dist-v; group, ungroup.
// Con dos cajas o más, alinea respecto al conjunto; con una, respecto al lienzo.
func (m *Modelo) Organizar(i int, ids []string, op, grupoNuevo string) {
	if len(ids) == 0 || (m.disenoEn(i) == nil && m.listaCajas(i, false) == nil) {
		return
	}
	bordes := map[string]string{
		"align-left": "left", "align-hcenter": "hcenter", "align-right": "right",
		"align-top": "top", "align-vmiddle": "vmiddle", "align-bottom": "bottom",
	}
	m.cambiarCajas(i, false, func(cs []Caja) []Caja {
		switch {
		case op == "group":
			cs = Agrupar(cs, ids, grupoNuevo)
		case op == "ungroup":
			cs = Desagrupar(cs, ids)
		case op == "front" || op == "forward" || op == "backward" || op == "back":
			cs = ReordenarZ(cs, ids, op)
		case bordes[op] != "":
			cs = Alinear(cs, ids, bordes[op], len(ids) < 2)
		case op == "dist-h":
			cs = Distribuir(cs, ids, "h")
		case op == "dist-v":
			cs = Distribuir(cs, ids, "v")
		}
		return cs
	})
}

// PonerAccion es `setBoxAction`: "" quita el enlace; "first", "last", "next" y "prev"
// van como texto y un número de diapositiva (desde 1) como número.
func (m *Modelo) PonerAccion(i int, id, valor string) {
	c := m.caja(i, id)
	if c == nil {
		return
	}
	switch valor {
	case "":
		c.Accion = nil
	case "first", "last", "next", "prev":
		c.Accion, _ = json.Marshal(valor)
	default:
		if n, err := strconv.Atoi(valor); err == nil && n >= 1 {
			c.Accion = json.RawMessage(strconv.Itoa(n))
		}
	}
}

// ValorAccion es el valor del selector «Al hacer clic» para la acción guardada.
func ValorAccion(a json.RawMessage) string {
	var s string
	if json.Unmarshal(a, &s) == nil {
		return s
	}
	var n float64
	if json.Unmarshal(a, &n) == nil {
		return strconv.Itoa(int(n))
	}
	return ""
}

// PonerRelleno y PonerTrazo son el parche de las muestras de una forma.
func (m *Modelo) PonerRelleno(i int, id, color string) {
	if c := m.caja(i, id); c != nil {
		c.Relleno = color
	}
}

// PonerTrazo es el color del borde de una forma.
func (m *Modelo) PonerTrazo(i int, id, color string) {
	if c := m.caja(i, id); c != nil {
		c.Trazo = color
	}
}

// PonerTipoGrafico es `patchChart` con el tipo: bar, line o pie.
func (m *Modelo) PonerTipoGrafico(i int, id, tipo string) {
	if c := m.caja(i, id); c != nil && c.Grafico != nil && slices.Contains([]string{"bar", "line", "pie"}, tipo) {
		c.Grafico.Tipo = tipo
	}
}

// PonerDisposicionSmart es `patchSmart` con la disposición.
func (m *Modelo) PonerDisposicionSmart(i int, id, disp string) {
	if c := m.caja(i, id); c != nil && c.Diagrama != nil && disposicionesSmart[disp] {
		c.Diagrama.Disposicion = disp
	}
}

// ---- conectores ----

// conector es el conector `id` de la diapositiva `i`, para cambiarlo en su sitio.
func (m *Modelo) conector(i int, id string) *Conector {
	if !m.valida(i) {
		return nil
	}
	for j := range m.Diapos[i].Conectores {
		if m.Diapos[i].Conectores[j].ID == id {
			return &m.Diapos[i].Conectores[j]
		}
	}
	return nil
}

// PonerTrazadoConector cambia el tipo (recto, codo, curvo) del conector.
func (m *Modelo) PonerTrazadoConector(i int, id, tipo string) {
	c := m.conector(i, id)
	if c == nil {
		return
	}
	for _, o := range conectoresUI {
		if o.ID == tipo {
			c.Trazado = tipo
		}
	}
}

// AlternarPunta enciende o apaga la punta de flecha del inicio o del final. Apagada no
// se escribe: `patchConnector` poda lo que no es `true`.
func (m *Modelo) AlternarPunta(i int, id string, inicio bool) {
	c := m.conector(i, id)
	if c == nil {
		return
	}
	p := &c.PuntaFin
	if inicio {
		p = &c.PuntaInicio
	}
	if *p != nil && **p {
		*p = nil
		return
	}
	si := true
	*p = &si
}

// PonerColorConector es el color del trazo.
func (m *Modelo) PonerColorConector(i int, id, color string) {
	if c := m.conector(i, id); c != nil {
		c.Trazo = color
	}
}

// QuitarConector es `deleteConnector`.
func (m *Modelo) QuitarConector(i int, id string) {
	if !m.valida(i) {
		return
	}
	d := &m.Diapos[i]
	d.Conectores = slices.DeleteFunc(slices.Clone(d.Conectores), func(c Conector) bool { return c.ID == id })
	if len(d.Conectores) == 0 {
		d.Conectores = nil
	}
}

// MoverExtremo es `onConnHandleMove`: el extremo pasa a punto libre en (x, y) % del
// lienzo, dentro de él y con una décima.
func (m *Modelo) MoverExtremo(i int, id string, final bool, x, y float64) {
	c := m.conector(i, id)
	if c == nil {
		return
	}
	x, y = math.Round(min(max(x, 0), 100)*10)/10, math.Round(min(max(y, 0), 100)*10)/10
	e := Extremo{X: &x, Y: &y}
	if final {
		c.Hasta = e
	} else {
		c.Desde = e
	}
}
