// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package coedicion es la coedición por carácter de los documentos de texto de Sytue Docs:
// la secuencia RGA del cliente (`editor/state/rga.ts`) y la política que la une con el
// guardado (`editor/state/coedicion.ts`). Es lo que deja que dos personas escriban a la
// vez sin que el último guardado borre lo del otro.
//
// EL ORDEN ES EL CONTRATO con `services/docs/internal/domain/crdt.go`, y ningún
// compilador lo comprueba: cada elemento cuelga de su predecesor (`after`, la raíz es el
// punto cero); los hermanos van de MAYOR a menor punto; se recorre en profundidad y en
// preorden. El punto se compara por contador y, si empata, por actor. Cambiar una de las
// tres reglas aquí y no allí no da error: cada lado ve un texto distinto.
package coedicion

import (
	"slices"
	"unicode/utf8"
)

// Punto es la identidad de un elemento: (actor, contador), estable para siempre.
type Punto struct {
	A string `json:"a"`
	C uint64 `json:"c"`
}

// Menor es `Dot.Less` del servidor: primero el contador y, si empata, el actor.
func (p Punto) Menor(o Punto) bool {
	if p.C != o.C {
		return p.C < o.C
	}
	return p.A < o.A
}

// Op es una operación de la secuencia tal y como viaja al servicio: `insert` con su
// carácter y su predecesor, o `delete` con su objetivo.
type Op struct {
	Kind    string `json:"kind"`
	Dot     Punto  `json:"dot"`
	After   *Punto `json:"after,omitempty"`
	Target  *Punto `json:"target,omitempty"`
	Value   string `json:"value,omitempty"`
	Lamport uint64 `json:"lamport"`
}

type elemento struct {
	dot, after Punto
	ch         string
	borrado    bool
}

// Secuencia es `SecuenciaRGA`: todos los elementos, vivos y tumbas, con el orden
// materializado en caché hasta el siguiente cambio.
//
// ponytail: el orden se recalcula entero al invalidarse, O(n) por tanda de ops. Sirve de
// sobra para un documento de oficina; para cientos de miles de caracteres en vivo, un
// árbol indexado por posición.
type Secuencia struct {
	actor    string
	elems    map[Punto]*elemento
	hijos    map[Punto][]*elemento
	cache    []*elemento
	valida   bool
	contador uint64
	// tumbas son los borrados que llegaron ANTES que la inserción de su objetivo (un
	// replay desordenado): sin esto, el carácter resucitaría al llegar.
	tumbas map[Punto]bool
}

// NuevaSecuencia es una secuencia vacía de ese actor.
func NuevaSecuencia(actor string) *Secuencia {
	return &Secuencia{actor: actor, elems: map[Punto]*elemento{}, hijos: map[Punto][]*elemento{}, tumbas: map[Punto]bool{}}
}

// Contador es el último contador local.
func (s *Secuencia) Contador() uint64 { return s.contador }

// FijarContador adelanta el contador local, nunca lo atrasa.
func (s *Secuencia) FijarContador(n uint64) { s.contador = max(s.contador, n) }

func (s *Secuencia) siguiente() Punto {
	s.contador++
	return Punto{A: s.actor, C: s.contador}
}

func (s *Secuencia) registrar(e *elemento) {
	if _, hay := s.elems[e.dot]; hay {
		return // idempotente: reenvíos y ecos no duplican
	}
	if s.tumbas[e.dot] {
		delete(s.tumbas, e.dot)
		e.borrado = true // su borrado llegó primero: nace ya muerto
	}
	s.elems[e.dot] = e
	s.hijos[e.after] = append(s.hijos[e.after], e)
	s.valida = false
}

// orden es el recorrido en profundidad y preorden con los hermanos de mayor a menor, con
// pila explícita como en el servidor: la recursión desbordaría con un documento largo.
func (s *Secuencia) orden() []*elemento {
	if s.valida {
		return s.cache
	}
	for _, l := range s.hijos {
		slices.SortFunc(l, func(x, y *elemento) int {
			if y.dot.Menor(x.dot) {
				return -1
			}
			return 1
		})
	}
	type marco struct {
		padre Punto
		i     int
	}
	out := make([]*elemento, 0, len(s.elems))
	vistos := make(map[Punto]bool, len(s.elems))
	pila := []marco{{}}
	for len(pila) > 0 {
		tope := &pila[len(pila)-1]
		hijos := s.hijos[tope.padre]
		if tope.i >= len(hijos) {
			pila = pila[:len(pila)-1]
			continue
		}
		e := hijos[tope.i]
		tope.i++
		if vistos[e.dot] {
			continue // ciclo o punto repetido: no se vuelve a entrar
		}
		vistos[e.dot] = true
		if !e.borrado {
			out = append(out, e)
		}
		pila = append(pila, marco{padre: e.dot})
	}
	s.cache, s.valida = out, true
	return out
}

// Texto es el texto visible.
func (s *Secuencia) Texto() string {
	n := 0
	for _, e := range s.orden() {
		n += len(e.ch)
	}
	b := make([]byte, 0, n)
	for _, e := range s.orden() {
		b = append(b, e.ch...)
	}
	return string(b)
}

// AplicarRemota aplica una operación del servidor (propia confirmada o ajena). Es
// idempotente y tolera el desorden: un borrado cuyo objetivo aún no ha llegado queda
// como tumba anticipada.
func (s *Secuencia) AplicarRemota(op Op) {
	switch op.Kind {
	case "insert":
		if op.Value == "" {
			return
		}
		after := Punto{}
		if op.After != nil {
			after = *op.After
		}
		s.registrar(&elemento{dot: op.Dot, after: after, ch: op.Value})
		s.FijarContador(op.Dot.C) // de quien sea: ver DesdeElementos
	case "delete":
		if op.Target == nil {
			return
		}
		if e, hay := s.elems[*op.Target]; hay {
			if !e.borrado {
				e.borrado, s.valida = true, false
			}
			return
		}
		s.tumbas[*op.Target] = true
	}
}

// InsertarLocal inserta `texto` en la posición visible `indice` (en caracteres) y da las
// ops que hay que mandar: un elemento por carácter, encadenado al anterior, para que dos
// personas que escriben en el mismo punto se intercalen de forma determinista.
func (s *Secuencia) InsertarLocal(indice int, texto string, lamport uint64) []Op {
	vivos := s.orden()
	pos := max(0, min(indice, len(vivos)))
	anterior := Punto{}
	if pos > 0 {
		anterior = vivos[pos-1].dot
	}
	ops := make([]Op, 0, utf8.RuneCountInString(texto))
	for _, r := range texto { // por carácter, no por byte: un emoji no se parte
		dot := s.siguiente()
		lamport++
		s.registrar(&elemento{dot: dot, after: anterior, ch: string(r)})
		a := anterior
		ops = append(ops, Op{Kind: "insert", Dot: dot, After: &a, Value: string(r), Lamport: lamport})
		anterior = dot
	}
	return ops
}

// BorrarLocal borra `cuantos` caracteres visibles desde `indice`.
func (s *Secuencia) BorrarLocal(indice, cuantos int, lamport uint64) []Op {
	vivos := s.orden()
	desde := max(0, min(indice, len(vivos)))
	hasta := min(desde+max(0, cuantos), len(vivos))
	ops := make([]Op, 0, hasta-desde)
	for _, e := range vivos[desde:hasta] {
		e.borrado = true
		lamport++
		t := e.dot
		ops = append(ops, Op{Kind: "delete", Dot: s.siguiente(), Target: &t, Lamport: lamport})
	}
	if len(ops) > 0 {
		s.valida = false
	}
	return ops
}

// OpsDesdeTexto son las ops mínimas para llegar a `nuevo`: prefijo y sufijo comunes, y lo
// de en medio como un reemplazo, el borrado PRIMERO (si no, se desplazaría el índice y se
// borraría lo recién escrito). Es lo que dejan escribir, borrar, sustituir y pegar; mover
// un párrafo sale como borrar allí e insertar aquí, que converge igual.
func (s *Secuencia) OpsDesdeTexto(nuevo string, lamport uint64) []Op {
	viejo := s.Texto()
	if viejo == nuevo {
		return nil
	}
	vi, nu := []rune(viejo), []rune(nuevo)
	pre := 0
	for pre < len(vi) && pre < len(nu) && vi[pre] == nu[pre] {
		pre++
	}
	suf := 0
	for suf < len(vi)-pre && suf < len(nu)-pre && vi[len(vi)-1-suf] == nu[len(nu)-1-suf] {
		suf++
	}
	ops := s.BorrarLocal(pre, len(vi)-pre-suf, lamport)
	return append(ops, s.InsertarLocal(pre, string(nu[pre:len(nu)-suf]), lamport+uint64(len(ops)))...)
}

// Elemento es un elemento de la secuencia tal y como lo da el estado del servicio.
type Elemento struct {
	Dot     Punto  `json:"dot"`
	After   Punto  `json:"after"`
	Value   string `json:"value"`
	Deleted bool   `json:"deleted"`
}

// DesdeElementos rehidrata la secuencia desde el estado materializado del servidor.
//
// El contador parte del MAYOR que haya, sea de quien sea, igual que `desdeElementos` de
// la web: los hermanos van de mayor a menor punto, y una letra con un
// contador menor que la de al lado se colocaría detrás de todo lo que cuelga de ella —al
// final del documento, si se escribe en medio de lo de otro—.
func DesdeElementos(actor string, elementos []Elemento) *Secuencia {
	s := NuevaSecuencia(actor)
	for _, e := range elementos {
		if e.Value == "" {
			continue
		}
		s.registrar(&elemento{dot: e.Dot, after: e.After, ch: e.Value, borrado: e.Deleted})
		s.FijarContador(e.Dot.C)
	}
	return s
}
