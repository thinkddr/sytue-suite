// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// simS lleva el estado de las secciones de un documento como lo haría el almacén.
type simS struct {
	t   *testing.T
	est EstadoProteccion
	lam uint64
}

func (s *simS) escribe(contenido string, dueno bool, quien Subject) error {
	s.t.Helper()
	s.lam++
	v, _ := json.Marshal(contenido)
	res, err := AplicarSecciones(s.est, []Op{{Kind: OpSet, Key: claveHTML, Value: v, Dot: Dot{Actor: "c", Counter: s.lam}, Lamport: s.lam}}, dueno, quien)
	if err != nil {
		return err
	}
	s.est = res.Estado
	return nil
}

const (
	secFirmas = `<div data-sytue-seccion="firmas" data-sytue-puede="ana" data-sytue-grupos="legal">%s</div>`
	secPrecio = `<div data-sytue-seccion="precio" data-sytue-puede="luis">%s</div>`
)

func doc(firmas, precio, libre string) string {
	return "<p>" + libre + "</p>" +
		strings.Replace(secFirmas, "%s", firmas, 1) +
		strings.Replace(secPrecio, "%s", precio, 1)
}

func nuevaSimS(t *testing.T) *simS {
	s := &simS{t: t, est: EstadoProteccion{Claves: map[string]ClaveHash{}}}
	if err := s.escribe(doc("firmas v1", "100", "intro"), true, Subject{Sub: "owner"}); err != nil {
		t.Fatalf("el propietario crea las secciones: %v", err)
	}
	return s
}

func TestSeccionesProtegidasPorPersonaYGrupo(t *testing.T) {
	ana, luis := Subject{Sub: "ana"}, Subject{Sub: "luis"}
	eva := Subject{Sub: "eva", Groups: []string{"legal"}}
	otra := Subject{Sub: "otra"}
	casos := []struct {
		nombre    string
		contenido string
		quien     Subject
		dueno     bool
		quiere    error
	}{
		{"cualquiera edita fuera de las secciones", doc("firmas v1", "100", "intro nueva"), otra, false, nil},
		{"ana edita su sección", doc("firmas v2", "100", "intro"), ana, false, nil},
		{"el grupo edita la suya", doc("firmas v2", "100", "intro"), eva, false, nil},
		{"luis edita la suya", doc("firmas v1", "200", "intro"), luis, false, nil},
		{"ana edita la de luis", doc("firmas v1", "200", "intro"), ana, false, ErrProtegido},
		{"un editor cualquiera edita una sección", doc("firmas v2", "100", "intro"), otra, false, ErrProtegido},
		{"sin identidad", doc("firmas v2", "100", "intro"), Subject{}, false, ErrProtegido},
		{"el propietario edita todo", doc("firmas v2", "200", "intro"), Subject{Sub: "owner"}, true, nil},
		{"ana edita su sección Y la de luis", doc("firmas v2", "200", "intro"), ana, false, ErrProtegido},
		{"quitar una sección", "<p>intro</p>" + strings.Replace(secFirmas, "%s", "firmas v1", 1), otra, false, ErrProtegido},
		{"quitar el marcado y dejar el texto", "<p>intro</p><div>firmas v1</div>" + strings.Replace(secPrecio, "%s", "100", 1), otra, false, ErrProtegido},
		{"crear una sección nueva", doc("firmas v1", "100", "intro") + `<div data-sytue-seccion="nueva" data-sytue-puede="otra">x</div>`, otra, false, ErrProtegido},
		{"ana se da permiso en la de luis", strings.Replace(doc("firmas v1", "100", "intro"), `puede="luis"`, `puede="luis ana"`, 1), ana, false, ErrProtegido},
		{"ana se quita de su propia sección", strings.Replace(doc("firmas v1", "100", "intro"), `puede="ana"`, `puede=""`, 1), ana, false, ErrProtegido},
		{"ocultar una sección con estilo", strings.Replace(doc("firmas v1", "100", "intro"), `<div data-sytue-seccion="precio"`, `<div style="display:none" data-sytue-seccion="precio"`, 1), otra, false, ErrProtegido},
		{"duplicar el identificador", doc("firmas v1", "100", "intro") + strings.Replace(secPrecio, "%s", "copia", 1), otra, false, ErrProtegido},
		{"identificador ilegible", strings.Replace(doc("firmas v1", "100", "intro"), `seccion="precio"`, `seccion="pre cio"`, 1), otra, false, ErrProtegido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := nuevaSimS(t)
			antes := s.est.Modelo
			err := s.escribe(c.contenido, c.dueno, c.quien)
			if !errors.Is(err, c.quiere) || (c.quiere == nil && err != nil) {
				t.Fatalf("esperaba %v, salió %v", c.quiere, err)
			}
			if c.quiere != nil && s.est.Modelo != antes {
				t.Fatal("una escritura denegada cambió el estado")
			}
		})
	}
}

// Las formas equivalentes de escribir lo mismo no cuentan como cambio.
func TestSeccionesCompararCanonicamente(t *testing.T) {
	s := nuevaSimS(t)
	otra := Subject{Sub: "otra"}
	equivalente := `<P>intro</P>` +
		`<div data-sytue-grupos="legal" data-sytue-puede="ana" data-sytue-seccion="firmas">firmas v1</div>` + // atributos en otro orden
		strings.Replace(secPrecio, "%s", "100", 1)
	if err := s.escribe(equivalente, false, otra); err != nil {
		t.Fatalf("el mismo contenido escrito distinto es un cambio: %v", err)
	}
}

func TestSeccionesSinSeccionesNoCuestanNada(t *testing.T) {
	s := &simS{t: t, est: EstadoProteccion{Claves: map[string]ClaveHash{}}}
	if err := s.escribe("<p>hola</p>", false, Subject{Sub: "x"}); err != nil {
		t.Fatal(err)
	}
	if s.est.Modelo != "" {
		t.Fatal("un documento sin secciones guardó estado")
	}
	// Un editor no puede introducir el marcado en un documento sin secciones.
	if err := s.escribe(doc("a", "b", "c"), false, Subject{Sub: "x"}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("un editor creó secciones: %v", err)
	}
}

func TestSeccionesOpPerdedoraNoCuenta(t *testing.T) {
	s := nuevaSimS(t)
	v, _ := json.Marshal("<p>sin secciones</p>")
	// Lamport 0: pierde contra el registro, que el CRDT descarta: no puede quitar nada.
	res, err := AplicarSecciones(s.est, []Op{{Kind: OpSet, Key: claveHTML, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 0}}, false, Subject{Sub: "x"})
	if err != nil {
		t.Fatalf("una op que pierde no debería rechazarse: %v", err)
	}
	if res.Estado.Modelo != s.est.Modelo {
		t.Fatal("una op que pierde cambió el estado")
	}
}

func TestEstadoInicialDeSecciones(t *testing.T) {
	st := NewState()
	v, _ := json.Marshal(doc("a", "b", "c"))
	if _, err := st.Apply(Op{Kind: OpSet, Key: claveHTML, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 7}); err != nil {
		t.Fatal(err)
	}
	est, err := EstadoInicialProteccion(KindDocument, st)
	if err != nil {
		t.Fatal(err)
	}
	if !est.HayRegistro || est.Lamport != 7 || len(leerHuellas(est.Modelo)) != 2 {
		t.Fatalf("estado inicial: %+v", est)
	}
	// Y el estado de un documento vacío es el de «sin registro».
	est, _ = EstadoInicialProteccion(KindDocument, NewState())
	if est.HayRegistro || est.Modelo != "" {
		t.Fatalf("estado inicial vacío: %+v", est)
	}
}
