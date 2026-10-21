// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// Protección propia de Diapos (ROADMAP-OFFICE permisos-1), cumplida en el servidor con el
// mismo esquema que Calcus (proteccion.go): el propietario siempre; un editor solo con la
// contraseña, verificada aquí; la contraseña se guarda solo como hash y nunca vuelve al
// cliente ni entra en el modelo.
//
// QUÉ SE PROTEGE. En una presentación lo que se defiende es que el trabajo de otra persona
// no cambie sin que lo sepa, no que el texto sea secreto (quien la lee la ve entera). Hay
// tres cosas, todas dentro de `SlidesModel.protect`:
//
//	protect: { hasPassword?: bool, slides?: [id…], boxes?: [id…], master?: bool }
//
//   - `slides`: diapositivas BLOQUEADAS. Su contenido entero (cajas, conectores, notas,
//     transición, diseño, fondo) queda fijo y no se pueden borrar. Se pueden mover de sitio.
//   - `boxes`: cajas (texto, imagen, forma, gráfico, medio) BLOQUEADAS. La cajaProt queda como
//     está —posición, tamaño, contenido, formato— y en su diapositiva; el resto de la
//     diapositiva sigue editable (salvo que esté bloqueada ella también).
//   - `master`: el patrón (con sus diseños), el tema y los temas propios, y el tamaño del
//     lienzo quedan fijos: es lo que da identidad a la presentación.
//
// QUIÉN. Como en Calcus: proteger un documento sin protección es de cualquier editor (con
// la contraseña que quiera ponerle); cambiar o quitar la protección, o tocar lo bloqueado,
// solo del propietario o de quien presenta la contraseña. Una protección sin contraseña
// solo la salta el propietario.
//
// CÓMO. Se compara el último doc:slides EFECTIVO, no lo que diga el cliente: el almacén
// guarda por documento una huella de lo bloqueado (no el modelo entero) y la comprueba
// dentro de la transacción que bloquea el documento. La huella es la del JSON canónico
// (claves ordenadas), de modo que reordenar claves no cuenta como cambio.
//
// ponytail: lo bloqueado se compara entero. Un editor sin permiso no puede ni reformatear
// una diapositiva bloqueada aunque solo cambie un color; no hay «bloquear solo el texto».

const (
	claveDiapos = "doc:slides"
	// AmbitoDiapos identifica la contraseña de la protección de una presentación.
	AmbitoDiapos = "diapos"
)

// TocaDiapos indica si el lote puede cambiar el modelo de la presentación.
func TocaDiapos(ops []Op) bool {
	for i := range ops {
		if ops[i].Kind == OpSet && ops[i].Key == claveDiapos {
			return true
		}
	}
	return false
}

// presentacion es un SlidesModel abierto lo justo para razonar sobre la protección; el
// resto se conserva tal cual (json.RawMessage).
type presentacion struct {
	raw     map[string]json.RawMessage
	diapos  []diapoProt
	protect map[string]json.RawMessage // nil = sin proteger
}

type diapoProt struct {
	id    string
	raw   json.RawMessage
	cajas []cajaProt
}

type cajaProt struct {
	id  string
	raw json.RawMessage
}

// leerDiapos abre el valor del registro. (nil, true) = sin modelo; (nil, false) = ilegible.
func leerDiapos(valor json.RawMessage) (*presentacion, bool) {
	if isNullJSON(valor) {
		return nil, true
	}
	var s string
	if json.Unmarshal(valor, &s) != nil {
		return nil, false
	}
	if len(bytes.TrimSpace([]byte(s))) == 0 {
		return nil, true
	}
	p := &presentacion{}
	if json.Unmarshal([]byte(s), &p.raw) != nil || p.raw == nil {
		return nil, false
	}
	var diapos []json.RawMessage
	if r, ok := p.raw["slides"]; ok && !isNullJSON(r) && json.Unmarshal(r, &diapos) != nil {
		return nil, false
	}
	for _, raw := range diapos {
		var d struct {
			ID    string            `json:"id"`
			Boxes []json.RawMessage `json:"boxes"`
		}
		if json.Unmarshal(raw, &d) != nil {
			return nil, false
		}
		dp := diapoProt{id: d.ID, raw: raw}
		for _, b := range d.Boxes {
			var cb struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(b, &cb) != nil {
				return nil, false
			}
			dp.cajas = append(dp.cajas, cajaProt{id: cb.ID, raw: b})
		}
		p.diapos = append(p.diapos, dp)
	}
	if raw := p.raw["protect"]; protegido(raw) {
		p.protect = map[string]json.RawMessage{}
		_ = json.Unmarshal(raw, &p.protect) // una lista queda como objeto vacío
	}
	return p, true
}

// huellaJSON es el sha256 del JSON canónico (claves ordenadas, números como se escribieron).
func huellaJSON(raw json.RawMessage) string {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	b := []byte(raw)
	if d.Decode(&v) == nil {
		b, _ = json.Marshal(v)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// idsDe: la lista de identificadores de un campo de `protect` (lo que no es lista, nada).
func idsDe(pm map[string]json.RawMessage, campo string) []string {
	var ids []string
	_ = json.Unmarshal(pm[campo], &ids)
	return ids
}

// huellaMarco: lo que protege `master`: patrón y diseños, tema, temas propios y tamaño.
func (p *presentacion) huellaMarco() string {
	m := map[string]json.RawMessage{}
	for _, k := range []string{"master", "theme", "themes", "size"} {
		if v, ok := p.raw[k]; ok && !isNullJSON(v) {
			m[k] = v
		}
	}
	b, _ := json.Marshal(m)
	return huellaJSON(b)
}

// proyeccionDiapos es lo que el almacén guarda de una presentación protegida.
type proyeccionDiapos struct {
	Protect string              `json:"protect"` // el objeto `protect` sin lo que gobierna el servidor
	Slides  map[string]string   `json:"slides,omitempty"`
	Boxes   map[string]cajaFija `json:"boxes,omitempty"`
	Master  string              `json:"master,omitempty"`
}

type cajaFija struct {
	Slide string `json:"slide"`
	Hash  string `json:"hash"`
}

// huellaProtect es la del objeto `protect` sin contraseña ni hasPassword (los fija el servidor).
func huellaProtect(pm map[string]json.RawMessage) string {
	b, _ := json.Marshal(decodificar(pm, "password", "hasPassword"))
	return huellaJSON(b)
}

// proyeccion calcula lo bloqueado de la presentación; nil si no está protegida.
func (p *presentacion) proyeccion() *proyeccionDiapos {
	if p == nil || p.protect == nil {
		return nil
	}
	out := &proyeccionDiapos{Protect: huellaProtect(p.protect), Slides: map[string]string{}, Boxes: map[string]cajaFija{}}
	// Solo se fija lo que tiene un único candidato: un identificador repetido no se puede
	// proteger (contenidoDiapos lo exigiría único y dejaría fuera a todo editor).
	vecesDiapo, vecesCaja := map[string]int{}, map[string]int{}
	for _, d := range p.diapos {
		vecesDiapo[d.id]++
		for _, c := range d.cajas {
			vecesCaja[c.id]++
		}
	}
	for _, id := range idsDe(p.protect, "slides") {
		for _, d := range p.diapos {
			if d.id == id && vecesDiapo[id] == 1 {
				out.Slides[id] = huellaJSON(d.raw)
			}
		}
	}
	for _, id := range idsDe(p.protect, "boxes") {
		for _, d := range p.diapos {
			for _, c := range d.cajas {
				if c.id == id && vecesCaja[id] == 1 {
					out.Boxes[id] = cajaFija{Slide: d.id, Hash: huellaJSON(c.raw)}
				}
			}
		}
	}
	if dice(p.protect, "master", true) {
		out.Master = p.huellaMarco()
	}
	return out
}

func leerProyeccion(modelo string) *proyeccionDiapos {
	if modelo == "" {
		return nil
	}
	var out proyeccionDiapos
	if json.Unmarshal([]byte(modelo), &out) != nil {
		return nil
	}
	return &out
}

func (q *proyeccionDiapos) cadena() string {
	if q == nil {
		return ""
	}
	b, _ := json.Marshal(q)
	return string(b)
}

// contenidoDiapos: lo que NO puede cambiar quien no está autorizado.
func contenidoDiapos(prev *proyeccionDiapos, nuevo *presentacion) error {
	porDiapo := map[string][]diapoProt{}
	porCaja := map[string][]cajaFija{}
	for _, d := range nuevo.diapos {
		porDiapo[d.id] = append(porDiapo[d.id], d)
		for _, c := range d.cajas {
			porCaja[c.id] = append(porCaja[c.id], cajaFija{Slide: d.id, Hash: huellaJSON(c.raw)})
		}
	}
	// Un identificador repetido dejaría poner delante una gemela y que la bloqueada dejara de
	// ser «la primera»: se exige un solo candidato.
	for id, h := range prev.Slides {
		if c := porDiapo[id]; len(c) != 1 || huellaJSON(c[0].raw) != h {
			return ErrProtegido
		}
	}
	for id, b := range prev.Boxes {
		if c := porCaja[id]; len(c) != 1 || c[0] != b {
			return ErrProtegido
		}
	}
	if prev.Master != "" && nuevo.huellaMarco() != prev.Master {
		return ErrProtegido
	}
	return nil
}

// normalizar deja `protect` en su forma de servidor: sin contraseña en claro y con
// `hasPassword: true` solo si hay hash vigente. true si cambió algo.
func (p *presentacion) normalizar(claves map[string]ClaveHash) bool {
	if p == nil || p.protect == nil {
		return false
	}
	_, tenia := p.protect["password"]
	antes := string(bytes.TrimSpace(p.protect["hasPassword"]))
	var original json.RawMessage = bytes.TrimSpace(p.raw["protect"])
	delete(p.protect, "password")
	delete(p.protect, "hasPassword")
	_, tiene := claves[AmbitoDiapos]
	if tiene {
		p.protect["hasPassword"] = json.RawMessage("true")
	}
	if !tenia && (len(original) == 0 || original[0] != '[') && (antes == "true") == tiene {
		return false
	}
	p.raw["protect"], _ = json.Marshal(p.protect)
	return true
}

func (p *presentacion) valor() json.RawMessage {
	b, _ := json.Marshal(p.raw)
	return jsonString(string(b))
}

// aplicarDiapos procesa un set de doc:slides.
func (c *evaluador) aplicarDiapos(est *EstadoProteccion, op *Op) error {
	nuevo, ok := leerDiapos(op.Value)
	if !ok { // no es un modelo legible: equivale a no tener ninguno
		nuevo = nil
	}
	if !ganaRegistro(*est, *op) {
		// El registro lo descarta: no cambia nada, pero que no quede una contraseña en claro.
		if nuevo.normalizar(c.claves) {
			op.Value = nuevo.valor()
		}
		return nil
	}
	if err := c.transicionDiapos(leerProyeccion(est.Modelo), nuevo); err != nil {
		return err
	}
	if nuevo.normalizar(c.claves) {
		op.Value = nuevo.valor()
	}
	est.HayRegistro, est.Lamport, est.Dot = true, op.Lamport, op.Dot
	est.Modelo = nuevo.proyeccion().cadena()
	return nil
}

func (c *evaluador) transicionDiapos(prev *proyeccionDiapos, nuevo *presentacion) error {
	var np map[string]json.RawMessage
	if nuevo != nil {
		np = nuevo.protect
	}
	if prev == nil {
		// Proteger es de cualquier editor; una contraseña, la que ponga.
		delete(c.claves, AmbitoDiapos)
		if np != nil {
			if nueva := c.nueva(AmbitoDiapos, ""); nueva != "" {
				h, err := HashClave(nueva)
				if err != nil {
					return err
				}
				c.claves[AmbitoDiapos] = h
			}
		}
		return nil
	}
	aut := c.autorizado(AmbitoDiapos, "")
	if np == nil {
		if !aut {
			return c.denegar(AmbitoDiapos)
		}
		delete(c.claves, AmbitoDiapos)
		return nil
	}
	nueva := c.nueva(AmbitoDiapos, "")
	if nueva != "" && c.coincide(AmbitoDiapos, nueva) {
		nueva = "" // la misma contraseña: sin cambio
	}
	_, hay := c.claves[AmbitoDiapos]
	quita := hay && dice(np, "hasPassword", false)
	if (huellaProtect(np) != prev.Protect || nueva != "" || quita) && !aut {
		return c.denegar(AmbitoDiapos)
	}
	switch {
	case nueva != "":
		h, err := HashClave(nueva)
		if err != nil {
			return err
		}
		c.claves[AmbitoDiapos] = h
	case quita:
		delete(c.claves, AmbitoDiapos)
	}
	if !aut && contenidoDiapos(prev, nuevo) != nil {
		return c.denegar(AmbitoDiapos) // con contraseña presentada y mala, lo dice
	}
	return nil
}

// AplicarDiapos valida un lote contra el estado de protección de una presentación y
// devuelve las operaciones saneadas y el estado resultante, o ErrProtegido /
// ErrClaveProteccion sin efecto alguno. Es pura.
func AplicarDiapos(est EstadoProteccion, ops []Op, esDueno bool, p Proteccion) (ResultadoProteccion, error) {
	c, original := nuevoEvaluador(est, esDueno, p)
	res := ResultadoProteccion{Ops: slices.Clone(ops), Estado: est}
	for i := range res.Ops {
		if op := &res.Ops[i]; op.Kind == OpSet && op.Key == claveDiapos {
			if err := c.aplicarDiapos(&res.Estado, op); err != nil {
				return ResultadoProteccion{}, err
			}
		}
	}
	c.cierra(&res, original)
	return res, nil
}

// EstadoInicialDiapos deduce el estado de protección de una presentación que ya existe.
func EstadoInicialDiapos(st *State) (EstadoProteccion, error) {
	est := EstadoProteccion{Claves: map[string]ClaveHash{}}
	r, ok := st.Registers[claveDiapos]
	if !ok {
		return est, nil
	}
	est.HayRegistro, est.Lamport, est.Dot = true, r.Lamport, r.Dot
	if p, leido := leerDiapos(r.Value); leido {
		est.Modelo = p.proyeccion().cadena()
	}
	return est, nil
}
