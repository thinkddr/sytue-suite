// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// # Copiar, cortar, pegar y duplicar cajas
//
// Es `slides/portapapeles.ts` de la web, con su MISMO formato: la selección efectiva
// (caja, multiselección o grupo) con sus conectores internos viaja por el portapapeles
// del sistema como texto, `sytue-diapos-cajas/1:` seguido del JSON. Así se copia en la
// web y se pega en la app (y al revés), entre diapositivas y entre dos presentaciones.
//
// Pegar nunca reutiliza un id: cajas, grupos y conectores reciben ids nuevos y las anclas
// de los conectores se reasignan a las cajas pegadas. Si el sitio de la copia ya está
// ocupado, se desplaza +2 % por cada copia que ya esté ahí, para que se vea.
//
// Las dos mitades son una sola regla escrita dos veces: los vectores de
// `packages/office/test/vectores/diapos-portapapeles.json` los exigen la prueba de la web
// y `portapapeles_test.go`.

// PrefijoRecorte identifica el formato del texto y su versión (`PREFIJO_RECORTE`).
const PrefijoRecorte = "sytue-diapos-cajas/1:"

// PasoPegado es el desplazamiento, en % del lienzo, de cada pegado sobre lo que ya hay.
const PasoPegado = 2.0

// Recorte es `RecorteDiapos`: las cajas en su orden de apilado y sus conectores.
type Recorte struct {
	Cajas      []Caja     `json:"boxes"`
	Conectores []Conector `json:"connectors"`
}

// Recortar es `recortar`: las cajas `ids` en su orden de apilado (no en el de la
// selección) y los conectores internos, los que tienen al menos un extremo anclado a una
// caja copiada y ninguno a una que se queda fuera: un conector a medias no tendría a qué
// unirse.
func Recortar(cajas []Caja, conectores []Conector, ids []string) Recorte {
	sel := conjunto(ids)
	r := Recorte{Cajas: []Caja{}, Conectores: []Conector{}}
	for _, c := range cajas {
		if sel[c.ID] {
			r.Cajas = append(r.Cajas, c)
		}
	}
	dentro := func(e Extremo) bool { return e.Caja == "" || sel[e.Caja] }
	for _, c := range conectores {
		anclado := c.Desde.Caja != "" || c.Hasta.Caja != ""
		if anclado && dentro(c.Desde) && dentro(c.Hasta) {
			r.Conectores = append(r.Conectores, c)
		}
	}
	return r
}

// Texto es `serializarRecorte`: el prefijo y el JSON, sin el escapado de HTML de
// `json.Marshal` (como `JSON.stringify`, y el portapapeles no lo necesita).
func (r Recorte) Texto() string {
	if r.Cajas == nil {
		r.Cajas = []Caja{}
	}
	if r.Conectores == nil {
		r.Conectores = []Conector{}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return "" // el modelo es JSON puro: no pasa
	}
	return PrefijoRecorte + strings.TrimRight(b.String(), "\n")
}

// LeerRecorte es `leerRecorte`: el recorte de un texto del portapapeles, o false si no
// lo es (cualquier otro texto, o uno corrupto). El portapapeles es una frontera de
// confianza: solo pasan cajas con forma de caja (id, x, y, w, h numéricos y html) y los
// conectores se sanean como al abrir un documento; el HTML se sanea igualmente al pintar.
func LeerRecorte(texto string) (Recorte, bool) {
	cuerpo, ok := strings.CutPrefix(texto, PrefijoRecorte)
	if !ok {
		return Recorte{}, false
	}
	var crudo struct {
		Cajas      []json.RawMessage `json:"boxes"`
		Conectores json.RawMessage   `json:"connectors"`
	}
	if json.Unmarshal([]byte(cuerpo), &crudo) != nil || crudo.Cajas == nil {
		return Recorte{}, false
	}
	r := Recorte{}
	ids := map[string]bool{}
	for _, b := range crudo.Cajas {
		c, ok := cajaRecortada(b)
		if !ok {
			continue
		}
		r.Cajas = append(r.Cajas, c)
		ids[c.ID] = true
	}
	if len(r.Cajas) == 0 {
		return Recorte{}, false
	}
	r.Conectores = sanearConectores(crudo.Conectores, ids)
	return r, true
}

// cajaRecortada lee una caja del recorte si tiene forma de caja.
func cajaRecortada(b json.RawMessage) (Caja, bool) {
	var campos map[string]any
	if json.Unmarshal(b, &campos) != nil || campos == nil {
		return Caja{}, false
	}
	id, _ := campos["id"].(string)
	_, esHTML := campos["html"].(string)
	if id == "" || !esHTML {
		return Caja{}, false
	}
	for _, k := range []string{"x", "y", "w", "h"} {
		if _, ok := campos[k].(float64); !ok {
			return Caja{}, false
		}
	}
	var c Caja
	if json.Unmarshal(b, &c) != nil {
		return Caja{}, false // un campo con un tipo que el modelo no admite
	}
	return c, true
}

var (
	trazadosConector = map[string]bool{"straight": true, "elbow": true, "curved": true}
	ladosConector    = map[string]bool{"top": true, "right": true, "bottom": true, "left": true}
	reHex            = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// sanearConectores es `sanitizeConnectors`: trazado con reserva a 'straight', extremos
// anclados solo a cajas que existen (si no, punto libre dentro del lienzo), puntas solo
// si son `true` y color solo en #rrggbb.
func sanearConectores(crudo json.RawMessage, ids map[string]bool) []Conector {
	var lista []any
	if json.Unmarshal(crudo, &lista) != nil {
		return []Conector{}
	}
	fuera := []Conector{}
	for i, el := range lista {
		o, ok := el.(map[string]any)
		if !ok {
			continue
		}
		c := Conector{ID: "c" + strconv.Itoa(i), Trazado: "straight"}
		if id, _ := o["id"].(string); id != "" {
			c.ID = id
		}
		if t, _ := o["kind"].(string); trazadosConector[t] {
			c.Trazado = t
		}
		c.Desde, c.Hasta = sanearExtremo(o["from"], ids), sanearExtremo(o["to"], ids)
		verdadero := true
		if v, _ := o["startArrow"].(bool); v {
			c.PuntaInicio = &verdadero
		}
		if v, _ := o["endArrow"].(bool); v {
			c.PuntaFin = &verdadero
		}
		if s, _ := o["stroke"].(string); reHex.MatchString(s) {
			c.Trazo = s
		}
		fuera = append(fuera, c)
	}
	return fuera
}

// sanearExtremo es `sanitizeEnd`.
func sanearExtremo(crudo any, ids map[string]bool) Extremo {
	o, _ := crudo.(map[string]any)
	if caja, _ := o["boxId"].(string); ids[caja] {
		e := Extremo{Caja: caja}
		if a, _ := o["anchor"].(string); ladosConector[a] {
			e.Anclaje = a
		}
		return e
	}
	pct := func(v any) *float64 {
		n, _ := v.(float64)
		n = acotar(n, 0, 100)
		return &n
	}
	return Extremo{X: pct(o["x"]), Y: pct(o["y"])}
}

// DesplazamientoPegado es `desplazamientoPegado`: si el sitio de la copia está libre en
// el destino (otra diapositiva, otra presentación o tras cortar) se pega en su sitio; si
// ya lo ocupa una caja, +2 % por cada copia que ya esté ahí, para que pegar varias veces
// escalone en vez de apilar.
func DesplazamientoPegado(destino []Caja, r Recorte) float64 {
	if len(r.Cajas) == 0 {
		return 0
	}
	ref := r.Cajas[0]
	ocupado := func(d float64) bool {
		for _, b := range destino {
			if math.Abs(b.X-(ref.X+d)) < 0.01 && math.Abs(b.Y-(ref.Y+d)) < 0.01 {
				return true
			}
		}
		return false
	}
	d := 0.0
	// Tope: más allá de 20 pasos la copia ya se habría salido del lienzo.
	for i := 0; i <= 20 && ocupado(d); i++ {
		d += PasoPegado
	}
	return d
}

// Pegado es lo que deja pegar: las cajas y conectores del destino ya fusionados y los
// ids de las cajas pegadas (para elegirlas).
type Pegado struct {
	Cajas      []Caja
	Conectores []Conector
	IDs        []string
}

// PegarRecorte es `pegarRecorte`: ids nuevos de caja, grupo y conector (pedidos a
// `nuevoID` en el mismo orden que la web, para que los vectores coincidan); anclas
// reasignadas; el desplazamiento de DesplazamientoPegado sin salirse del lienzo, y las
// animaciones de lo pegado detrás de las del destino, renumeradas. Sin `conConectores`
// (el patrón) los conectores no se pegan.
func PegarRecorte(destCajas []Caja, destCon []Conector, r Recorte, nuevoID func() string, conConectores bool) Pegado {
	d := DesplazamientoPegado(destCajas, r)
	ids := map[string]string{}
	grupos := map[string]string{}
	// Las animaciones pegadas van tras las existentes, conservando su orden relativo.
	base := 0.0
	for _, b := range destCajas {
		for _, a := range AnimsDe(b) {
			base = math.Max(base, a.Orden)
		}
	}
	nuevas := make([]Caja, 0, len(r.Cajas))
	for _, b := range r.Cajas {
		copia := b
		copia.ID = nuevoID()
		ids[b.ID] = copia.ID
		w, h := acotar(b.W, 0, 100), acotar(b.H, 0, 100)
		copia.X, copia.Y = acotar(b.X+d, 0, 100-w), acotar(b.Y+d, 0, 100-h)
		if b.Grupo != "" {
			if _, ok := grupos[b.Grupo]; !ok {
				grupos[b.Grupo] = nuevoID()
			}
			copia.Grupo = grupos[b.Grupo]
		}
		if anims := AnimsDe(b); len(anims) > 0 {
			for k := range anims {
				anims[k].Orden += base
			}
			copia = conAnims(copia, anims)
		}
		nuevas = append(nuevas, copia)
	}
	// Un grupo de una sola caja no es un grupo: se suelta (como al desagrupar).
	cuenta := map[string]int{}
	for _, b := range nuevas {
		if b.Grupo != "" {
			cuenta[b.Grupo]++
		}
	}
	p := Pegado{IDs: make([]string, len(nuevas))}
	for k := range nuevas {
		if nuevas[k].Grupo != "" && cuenta[nuevas[k].Grupo] == 1 {
			nuevas[k].Grupo = ""
		}
		p.IDs[k] = nuevas[k].ID
	}
	todas := append(append(make([]Caja, 0, len(destCajas)+len(nuevas)), destCajas...), nuevas...)
	p.Cajas = NormalizarOrden(todas)
	p.Conectores = append([]Conector{}, destCon...)
	if conConectores {
		for _, c := range r.Conectores {
			c.Desde, c.Hasta = remapear(c.Desde, ids), remapear(c.Hasta, ids)
			c.ID = nuevoID()
			if c.Desde.Caja == "" {
				c.Desde = moverLibre(c.Desde, d)
			}
			if c.Hasta.Caja == "" {
				c.Hasta = moverLibre(c.Hasta, d)
			}
			p.Conectores = append(p.Conectores, c)
		}
	}
	return p
}

// moverLibre desplaza un extremo libre con la copia, dentro del lienzo.
func moverLibre(e Extremo, d float64) Extremo {
	val := func(v *float64) *float64 {
		n := 0.0
		if v != nil {
			n = *v
		}
		n = acotar(n+d, 0, 100)
		return &n
	}
	return Extremo{Caja: e.Caja, Anclaje: e.Anclaje, X: val(e.X), Y: val(e.Y)}
}

// TextoDeRecorte es lo que se pega de un recorte DENTRO de una caja que se escribe: su
// texto, una línea por caja con algo escrito. Nunca cajas, ni su JSON.
func TextoDeRecorte(r Recorte) string {
	var lineas []string
	for _, c := range r.Cajas {
		if t := textoPlano(c.HTML); t != "" {
			lineas = append(lineas, t)
		}
	}
	return strings.Join(lineas, "\n")
}
