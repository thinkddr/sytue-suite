// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// simQ lleva un documento de texto con secciones como lo haría el almacén: el estado de
// protección, con la copia de la secuencia, y los contadores de los clientes.
type simQ struct {
	t   *testing.T
	est EstadoProteccion
	ctr uint64
}

// nuevaSimQ siembra el documento como lo hace un cliente al abrir un documento heredado:
// un `set` de doc:html y la secuencia con ese mismo texto, un elemento por carácter.
func nuevaSimQ(t *testing.T, html string) *simQ {
	t.Helper()
	q := &simQ{t: t}
	st := NewState()
	v, _ := json.Marshal(html)
	if _, err := st.Apply(Op{Kind: OpSet, Key: claveHTML, Value: v, Dot: Dot{Actor: "sembrado", Counter: 1}, Lamport: 1}); err != nil {
		t.Fatal(err)
	}
	ops, _ := q.texto("sembrado", Dot{}, html)
	for _, op := range ops {
		if _, err := st.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	est, err := EstadoInicialProteccion(KindDocument, st)
	if err != nil {
		t.Fatal(err)
	}
	q.est = est
	return q
}

// texto son los insert de `s`, un carácter cada uno, a continuación de `tras`.
func (q *simQ) texto(actor string, tras Dot, s string) ([]Op, Dot) {
	ops := make([]Op, 0, len(s))
	for _, r := range s {
		q.ctr++
		v, _ := json.Marshal(string(r))
		after := tras
		ops = append(ops, Op{Kind: OpInsert, Dot: Dot{Actor: actor, Counter: q.ctr}, After: &after, Value: v, Lamport: q.ctr})
		tras = Dot{Actor: actor, Counter: q.ctr}
	}
	return ops, tras
}

// punto es el Dot del carácter con el que EMPIEZA la primera aparición de `aguja`.
func (q *simQ) punto(aguja string, desplazamiento int) Dot {
	q.t.Helper()
	txt := q.est.Secuencia.PlainText()
	i := strings.Index(txt, aguja)
	if i < 0 {
		q.t.Fatalf("%q no está en %q", aguja, txt)
	}
	return q.est.Secuencia.rgaOrder()[i+desplazamiento].Dot
}

// borrar son los delete de los `n` caracteres que empiezan en `aguja`.
func (q *simQ) borrar(aguja string, n int) []Op {
	q.t.Helper()
	ops := make([]Op, 0, n)
	for i := range n {
		d := q.punto(aguja, i)
		q.ctr++
		ops = append(ops, Op{Kind: OpDelete, Target: &d, Dot: Dot{Actor: "x", Counter: q.ctr}, Lamport: q.ctr})
	}
	return ops
}

func (q *simQ) html(contenido string) Op {
	q.ctr++
	v, _ := json.Marshal(contenido)
	return Op{Kind: OpSet, Key: claveHTML, Value: v, Dot: Dot{Actor: "x", Counter: q.ctr}, Lamport: 1000 + q.ctr}
}

// aplica ejecuta el lote; si lo deniega, comprueba que no cambió NADA del estado.
func (q *simQ) aplica(quien Subject, dueno bool, ops ...Op) error {
	q.t.Helper()
	antes, antesTxt := q.est, ""
	if q.est.Secuencia != nil {
		antesTxt = q.est.Secuencia.PlainText()
	}
	res, err := AplicarSecciones(q.est, ops, dueno, quien)
	if err != nil {
		if !reflect.DeepEqual(q.est, antes) || (q.est.Secuencia != nil && q.est.Secuencia.PlainText() != antesTxt) {
			q.t.Fatal("un lote denegado cambió el estado")
		}
		return err
	}
	q.est = res.Estado
	return nil
}

func (q *simQ) texto2() string { return q.est.Secuencia.PlainText() }

func TestSecuenciaPorCaracterContraLasSecciones(t *testing.T) {
	ana, otra, luis := Subject{Sub: "ana"}, Subject{Sub: "otra"}, Subject{Sub: "luis"}
	base := doc("firmas v1", "100", "intro")

	casos := []struct {
		nombre string
		quien  Subject
		dueno  bool
		lote   func(q *simQ) []Op
		quiere error
	}{
		{"teclear fuera de las secciones", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("intro", 4), " nueva")
			return ops
		}, nil},
		{"teclear dentro de una sección ajena", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("firmas v1", 8), " TRUCADO")
			return ops
		}, ErrProtegido},
		{"borrar texto de una sección ajena", otra, false, func(q *simQ) []Op { return q.borrar("v1", 2) }, ErrProtegido},
		{"borrar entera una sección ajena", otra, false, func(q *simQ) []Op {
			return q.borrar(`<div data-sytue-seccion="precio" data-sytue-puede="luis">100</div>`, len(`<div data-sytue-seccion="precio" data-sytue-puede="luis">100</div>`))
		}, ErrProtegido},
		{"borrar solo la etiqueta de cierre de una sección", otra, false, func(q *simQ) []Op { return q.borrar(`</div><div data-sytue-seccion="precio"`, 6) }, ErrProtegido},
		{"partir una sección con un cierre y una apertura", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("firmas v1", 6), "</div><div>")
			return ops
		}, ErrProtegido},
		{"crear una sección nueva", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("intro", 4), `<div data-sytue-seccion="mia" data-sytue-puede="otra">x</div>`)
			return ops
		}, ErrProtegido},
		{"darse permiso en una sección", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto(`puede="luis`, 11), " otra")
			return ops
		}, ErrProtegido},
		{"un elemento con varios caracteres que cierra la sección", otra, false, func(q *simQ) []Op {
			q.ctr++
			v, _ := json.Marshal(`</div><div data-sytue-seccion="firmas" data-sytue-puede="otra">robada</div>`)
			d := q.punto("v1", 1)
			return []Op{{Kind: OpInsert, Dot: Dot{Actor: "otra", Counter: q.ctr}, After: &d, Value: v, Lamport: q.ctr}}
		}, ErrProtegido},
		{"la persona con permiso edita su sección", ana, false, func(q *simQ) []Op {
			ops, _ := q.texto("ana", q.punto("firmas v1", 8), " y v2")
			return ops
		}, nil},
		{"la persona con permiso no edita la de otra", ana, false, func(q *simQ) []Op {
			ops, _ := q.texto("ana", q.punto(">100", 3), "0")
			return ops
		}, ErrProtegido},
		{"edita su sección y la ajena en el mismo lote", luis, false, func(q *simQ) []Op {
			a, _ := q.texto("luis", q.punto(">100", 3), "0")
			b, _ := q.texto("luis", q.punto("firmas v1", 8), "!")
			return append(a, b...)
		}, ErrProtegido},
		{"el propietario edita cualquier sección", otra, true, func(q *simQ) []Op {
			ops, _ := q.texto("dueno", q.punto("firmas v1", 8), " del dueño")
			return ops
		}, nil},
		// El caso de la puerta trasera: el `set` de doc:html es idéntico al vigente (pasa la
		// regla del registro) y lo sucio va en las ops por carácter del mismo lote.
		{"set limpio de doc:html con ops sucias", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("firmas v1", 8), " TRUCADO")
			return append([]Op{q.html(base)}, ops...)
		}, ErrProtegido},
		{"set limpio con las ops sucias antes", otra, false, func(q *simQ) []Op {
			ops, _ := q.texto("otra", q.punto("firmas v1", 8), " TRUCADO")
			return append(ops, q.html(base))
		}, ErrProtegido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			q := nuevaSimQ(t, base)
			ops := c.lote(q)
			err := q.aplica(c.quien, c.dueno, ops...)
			if !errors.Is(err, c.quiere) || (c.quiere == nil && err != nil) {
				t.Fatalf("esperaba %v, salió %v", c.quiere, err)
			}
			if c.quiere == nil && TocaSecuencia(ops) {
				if q.est.Secuencia == nil || q.texto2() == base {
					t.Fatal("el lote aceptado no actualizó la copia de la secuencia")
				}
			}
		})
	}
}

// Lo aceptado se acumula en la copia: la siguiente comprobación parte de lo vigente.
func TestSecuenciaLaCopiaSigueAlDocumento(t *testing.T) {
	q := nuevaSimQ(t, doc("firmas v1", "100", "intro"))
	ana, otra := Subject{Sub: "ana"}, Subject{Sub: "otra"}
	ops, _ := q.texto("ana", q.punto("firmas v1", 8), " y v2")
	if err := q.aplica(ana, false, ops...); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.texto2(), "firmas v1 y v2</div>") {
		t.Fatalf("la copia no recoge lo de ana: %q", q.texto2())
	}
	// Quien no figura sigue sin poder tocarla, también sobre el texto ya editado...
	ops, _ = q.texto("otra", q.punto("y v2", 3), "!")
	if err := q.aplica(otra, false, ops...); !errors.Is(err, ErrProtegido) {
		t.Fatalf("editó la sección de ana: %v", err)
	}
	// ...y fuera de las secciones sigue escribiendo.
	ops, _ = q.texto("otra", q.punto("intro", 4), "!")
	if err := q.aplica(otra, false, ops...); err != nil {
		t.Fatal(err)
	}
}

func TestSecuenciaSinCopiaSePideAlServicio(t *testing.T) {
	q := nuevaSimQ(t, doc("firmas v1", "100", "intro"))
	q.est.Secuencia = nil // el propietario añadió las secciones y la copia aún no existe
	otra := Subject{Sub: "otra"}
	ops := []Op{{Kind: OpInsert, Dot: Dot{Actor: "otra", Counter: 1}, After: &Dot{}, Value: json.RawMessage(`"x"`), Lamport: 1}}
	if err := q.aplica(otra, false, ops...); !errors.Is(err, ErrSecuenciaPendiente) {
		t.Fatalf("sin copia, un no propietario escribe por carácter: %v", err)
	}
	// El propietario no se valida y la copia sigue por establecer.
	if err := q.aplica(otra, true, ops...); err != nil || q.est.Secuencia != nil {
		t.Fatalf("el propietario: err=%v copia=%v", err, q.est.Secuencia)
	}
	// Sin ops por carácter no hace falta copia.
	if err := q.aplica(otra, false, q.html(doc("firmas v1", "100", "otra intro"))); err != nil {
		t.Fatal(err)
	}
}

// Un documento heredado (secuencia vacía, texto en el registro): sembrar la secuencia con
// el mismo texto es lo normal; sembrarla con una sección cambiada no.
func TestSecuenciaSembrarUnDocumentoHeredado(t *testing.T) {
	base := doc("firmas v1", "100", "intro")
	otra := Subject{Sub: "otra"}
	for _, c := range []struct {
		nombre, texto string
		quiere        error
	}{
		{"el mismo texto", base, nil},
		{"con la sección cambiada", doc("firmas TRUCADA", "100", "intro"), ErrProtegido},
		{"sin la sección", "<p>intro</p>", ErrProtegido},
		{"fuera de las secciones cambiado", doc("firmas v1", "100", "intro y más"), nil},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			q := nuevaSimQ(t, base)
			q.est.Secuencia = &State{} // el documento nunca tuvo secuencia
			ops, _ := q.texto("otra", Dot{}, c.texto)
			if err := q.aplica(otra, false, ops...); !errors.Is(err, c.quiere) || (c.quiere == nil && err != nil) {
				t.Fatalf("esperaba %v, salió %v", c.quiere, err)
			}
		})
	}
}

// Sin secciones no se guarda ni se valida nada: el coste solo lo pagan los documentos que
// las tienen.
func TestSecuenciaSinSeccionesNoCuestaNada(t *testing.T) {
	q := nuevaSimQ(t, "<p>hola</p>")
	if q.est.Secuencia != nil || q.est.Modelo != "" {
		t.Fatal("un documento sin secciones guardó una copia")
	}
	ops, _ := q.texto("x", Dot{}, "<p>más</p>")
	res, err := AplicarSecciones(q.est, ops, false, Subject{Sub: "x"})
	if err != nil || res.Estado.Secuencia != nil || res.SecuenciaCambia {
		t.Fatalf("err=%v copia=%v cambia=%v", err, res.Estado.Secuencia, res.SecuenciaCambia)
	}
}

// Quitar las secciones (solo el propietario) descarta la copia.
func TestSecuenciaSeDescartaAlQuitarLasSecciones(t *testing.T) {
	q := nuevaSimQ(t, doc("firmas v1", "100", "intro"))
	op := q.html("<p>sin secciones</p>")
	res, err := AplicarSecciones(q.est, []Op{op}, true, Subject{Sub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Estado.Secuencia != nil || !res.SecuenciaCambia {
		t.Fatalf("la copia debía descartarse: %+v", res.SecuenciaCambia)
	}
}

// AplicarSecuencia es Apply con un índice: mismo resultado en cualquier mezcla de lotes,
// con duplicados, borrados de elementos aún no vistos y ciclos.
func TestAplicarSecuenciaEquivaleAApply(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	actores := []string{"a", "b", "c"}
	for ronda := range 200 {
		var ops []Op
		var dots []Dot
		for i := range 60 {
			if len(dots) > 0 && rng.Intn(3) == 0 {
				d := dots[rng.Intn(len(dots))]
				if rng.Intn(4) == 0 {
					d = Dot{Actor: "fantasma", Counter: uint64(rng.Intn(5) + 1)} // aún no visto
				}
				ops = append(ops, Op{Kind: OpDelete, Target: &d, Lamport: uint64(i)})
				continue
			}
			d := Dot{Actor: actores[rng.Intn(3)], Counter: uint64(rng.Intn(40) + 1)} // con repetidos
			after := Dot{}
			if len(dots) > 0 && rng.Intn(5) != 0 {
				after = dots[rng.Intn(len(dots))]
			}
			ops = append(ops, Op{Kind: OpInsert, Dot: d, After: &after, Value: json.RawMessage(`"x"`), Lamport: uint64(i)})
			dots = append(dots, d)
		}
		ref := NewState()
		for _, op := range ops {
			if _, err := ref.Apply(op); err != nil {
				t.Fatal(err)
			}
		}
		got := NewState()
		// En dos lotes: la copia viaja serializada entre ambos, como en el almacén.
		AplicarSecuencia(got, ops[:30])
		b, err := SecuenciaJSON(got)
		if err != nil {
			t.Fatal(err)
		}
		got, err = SecuenciaDeJSON(b)
		if err != nil {
			t.Fatal(err)
		}
		AplicarSecuencia(got, ops[30:])
		// Por JSON: un tombstone sin valor vuelve del almacén como `null`, no como nil.
		a, _ := json.Marshal(ref.Elements)
		b2, _ := json.Marshal(got.Elements)
		if !bytes.Equal(a, b2) {
			t.Fatalf("ronda %d: los elementos difieren", ronda)
		}
	}
}
