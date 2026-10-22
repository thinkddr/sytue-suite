// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
)

func TestLeerRango(t *testing.T) {
	casos := []struct {
		ref   string
		ok    bool
		deja  []string // celdas dentro
		fuera []string
	}{
		{"B2", true, []string{"B2"}, []string{"B3", "C2", "A1"}},
		{"b2:d10", true, []string{"B2", "C5", "D10"}, []string{"A2", "E5", "B11"}},
		{"D10:B2", true, []string{"C5"}, []string{"E5"}}, // al revés también
		{"B:C", true, []string{"B1", "C999"}, []string{"A1", "D1"}},
		{"3:4", true, []string{"A3", "Z4"}, []string{"A2", "A5"}},
		{"", false, nil, nil},
		{"A1:", false, nil, nil},
		{"A0", false, nil, nil},
		{"A1+1", false, nil, nil},
		{"XFE1", false, nil, nil},
		{"A:3", false, nil, nil},
	}
	for _, c := range casos {
		r, ok := leerRangoEditable(c.ref)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v, esperaba %v", c.ref, ok, c.ok)
		}
		rs := rangosEditables{r}
		for _, d := range c.deja {
			if !rs.contiene(d) {
				t.Fatalf("%q debería contener %s", c.ref, d)
			}
		}
		for _, f := range c.fuera {
			if rs.contiene(f) {
				t.Fatalf("%q no debería contener %s", c.ref, f)
			}
		}
	}
}

// protegidaConRangos deja «H» protegida con dos rangos: B2:B3 para ana y A1:A2 para el
// grupo «contabilidad». Con la contraseña «k».
func protegidaConRangos(t *testing.T) *simP {
	t.Helper()
	s := nuevaSimP(t)
	err := s.escribir(libroT(hojaT("H", mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "5"}, mj{
		"protect": mj{"ranges": []any{
			mj{"ref": "B2:B3", "users": []any{"ana"}},
			mj{"ref": "A1:A2", "groups": []any{"contabilidad"}},
		}},
	})), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("H"): "k"}})
	if err != nil {
		t.Fatalf("proteger con rangos: %v", err)
	}
	return s
}

func (s *simP) conCelda(ref, valor string, quien Subject, p Proteccion) error {
	s.t.Helper()
	celdas := mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "5"}
	celdas[ref] = valor
	p.Quien = quien
	return s.escribir(libroT(hojaT("H", celdas, mj{"protect": mj{"hasPassword": true, "ranges": []any{
		mj{"ref": "B2:B3", "users": []any{"ana"}},
		mj{"ref": "A1:A2", "groups": []any{"contabilidad"}},
	}}})), false, p)
}

func TestProteccionRangosEditablesPorPersonaYGrupo(t *testing.T) {
	ana := Subject{Sub: "ana"}
	luis := Subject{Sub: "luis"}
	conta := Subject{Sub: "eva", Groups: []string{"contabilidad"}}
	casos := []struct {
		nombre string
		ref    string
		quien  Subject
		quiere error
	}{
		{"ana en su rango", "B2", ana, nil},
		{"ana en el final de su rango", "B3", ana, nil},
		{"ana fuera de su rango", "C1", ana, ErrProtegido},
		{"ana en el rango de otro grupo", "A1", ana, ErrProtegido},
		{"luis en el rango de ana", "B2", luis, ErrProtegido},
		{"grupo en su rango", "A2", conta, nil},
		{"grupo en el rango de ana", "B2", conta, ErrProtegido},
		{"sin identidad", "B2", Subject{}, ErrProtegido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaConRangos(t)
			antes := s.est.Modelo
			err := s.conCelda(c.ref, "nuevo", c.quien, Proteccion{})
			if !errors.Is(err, c.quiere) {
				t.Fatalf("esperaba %v, salió %v", c.quiere, err)
			}
			if err != nil && s.est.Modelo != antes {
				t.Fatal("una escritura denegada cambió el estado")
			}
		})
	}
}

// El rango deja escribir celdas, no tocar la protección ni sus rangos.
func TestProteccionRangosNoDejanCambiarLosRangos(t *testing.T) {
	s := protegidaConRangos(t)
	ana := Subject{Sub: "ana"}
	// Ana se concede a sí misma todo el libro.
	mal := libroT(hojaT("H", mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "5"}, mj{"protect": mj{"hasPassword": true, "ranges": []any{
		mj{"ref": "A1:Z99", "users": []any{"ana"}},
	}}}))
	if err := s.escribir(mal, false, Proteccion{Quien: ana}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("ana amplió su propio rango: %v", err)
	}
	// Ni quitando la protección, con el rango de su lado.
	if err := s.escribir(libroT(hojaT("H", mj{"A1": "1", "A2": "2", "B2": "9", "B3": "4", "C1": "5"}, nil)), false, Proteccion{Quien: ana}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("ana quitó la protección: %v", err)
	}
	// El propietario y quien trae la contraseña sí pueden reordenar los rangos.
	nuevo := libroT(hojaT("H", mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "5"}, mj{"protect": mj{"hasPassword": true, "ranges": []any{
		mj{"ref": "C1", "users": []any{"ana"}},
	}}}))
	if err := s.escribir(nuevo, false, Proteccion{Clave: "k"}); err != nil {
		t.Fatalf("con la contraseña: %v", err)
	}
	if err := s.conCelda("C1", "ahora si", ana, Proteccion{}); err == nil {
		t.Fatal("conCelda usa los rangos viejos: debería denegar (B2:B3 ya no)")
	}
}

// Rangos ilegibles o sin nadie concedido no desbloquean nada.
func TestProteccionRangosIlegiblesNoDesbloquean(t *testing.T) {
	for nombre, rangos := range map[string]any{
		"no es lista":       "A1:Z99",
		"sin referencia":    []any{mj{"users": []any{"ana"}}},
		"referencia mala":   []any{mj{"ref": "no-es-un-rango", "users": []any{"ana"}}},
		"sin nadie":         []any{mj{"ref": "A1:Z99"}},
		"users no es lista": []any{mj{"ref": "A1:Z99", "users": "ana"}},
	} {
		t.Run(nombre, func(t *testing.T) {
			s := nuevaSimP(t)
			err := s.escribir(libroT(hojaT("H", mj{"A1": "1"}, mj{"protect": mj{"ranges": rangos}})), false, Proteccion{})
			if err != nil {
				t.Fatalf("proteger: %v", err)
			}
			err = s.escribir(libroT(hojaT("H", mj{"A1": "2"}, mj{"protect": mj{"ranges": rangos}})), false, Proteccion{Quien: Subject{Sub: "ana"}})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("un rango inservible desbloqueó una celda: %v", err)
			}
		})
	}
}
