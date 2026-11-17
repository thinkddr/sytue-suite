// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// gemelasT es un libro sin protección ya guardado con esas hojas: el estado de protección
// sabe que hay registro pero no guarda el modelo, y lo que hay se lee con Vigente, que
// cuenta las veces que se le pide.
func gemelasT(t *testing.T, hojas ...string) (*simP, *Proteccion, *int) {
	t.Helper()
	s := nuevaSimP(t)
	hs := make([]mj, len(hojas))
	for i, h := range hojas {
		hs[i] = hojaT(h, mj{"A1": "1"}, nil)
	}
	guardado := valT(t, libroT(hs...))
	// El primer contenido del libro no se compara con nada: así quedan las gemelas de
	// antes de la regla.
	if err := s.escribir(libroT(hs...), false, Proteccion{}); err != nil {
		t.Fatalf("primer guardado: %v", err)
	}
	if s.est.Modelo != "" || !s.est.HayRegistro {
		t.Fatalf("un libro sin protección no guarda su modelo en el estado: %q", s.est.Modelo)
	}
	lecturas := 0
	p := &Proteccion{Vigente: func() (json.RawMessage, error) { lecturas++; return guardado, nil }}
	return s, p, &lecturas
}

func libroDe(hojas ...string) mj {
	hs := make([]mj, len(hojas))
	for i, h := range hojas {
		hs[i] = hojaT(h, mj{"A1": "2"}, nil)
	}
	return libroT(hs...)
}

// TestUnGuardadoNoCreaHojasGemelas: añadir una hoja o renombrar una con el nombre de otra
// salvo mayúsculas se rechaza (ErrHojaGemela, que es una operación inválida); un nombre
// distinto pasa. VISTO EN ROJO sin la regla: los dos guardados se aceptaban.
func TestUnGuardadoNoCreaHojasGemelas(t *testing.T) {
	for _, c := range []struct {
		que   string
		antes []string
		ahora []string
	}{
		{"añadir «VENTAS» a «Ventas»", []string{"Ventas"}, []string{"Ventas", "VENTAS"}},
		{"renombrar «Otra» a «ventas»", []string{"Ventas", "Otra"}, []string{"Ventas", "ventas"}},
		{"duplicar con el mismo nombre", []string{"Hoja 1"}, []string{"Hoja 1", "Hoja 1"}},
	} {
		s, p, lecturas := gemelasT(t, c.antes...)
		err := s.escribir(libroDe(c.ahora...), false, *p)
		if !errors.Is(err, ErrHojaGemela) || !errors.Is(err, ErrInvalidOp) {
			t.Errorf("%s: el guardado da %v y tenía que dar ErrHojaGemela", c.que, err)
		}
		if *lecturas != 1 {
			t.Errorf("%s: el registro se ha leído %d veces", c.que, *lecturas)
		}
		// Tampoco el propietario: no es una cuestión de permisos.
		if err := s.escribir(libroDe(c.ahora...), true, *p); !errors.Is(err, ErrHojaGemela) {
			t.Errorf("%s: el propietario sí puede: %v", c.que, err)
		}
	}
	s, p, _ := gemelasT(t, "Ventas")
	if err := s.escribir(libroDe("Ventas", "Ventas (2)"), false, *p); err != nil {
		t.Errorf("una hoja con otro nombre no se guarda: %v", err)
	}
}

// TestUnLibroConGemelasSeSigueGuardando: las gemelas que el libro ya tenía no lo dejan sin
// poder guardarse —editar, añadir otra hoja, reordenar, deshacer la gemela—, pero no se
// le puede hacer una tercera.
func TestUnLibroConGemelasSeSigueGuardando(t *testing.T) {
	for _, ahora := range [][]string{
		{"Ventas", "VENTAS"},
		{"Ventas", "VENTAS", "Hoja 3"},
		{"VENTAS", "Ventas"},
		{"Ventas", "Ventas 2"},
	} {
		s, p, _ := gemelasT(t, "Ventas", "VENTAS")
		if err := s.escribir(libroDe(ahora...), false, *p); err != nil {
			t.Errorf("el libro con gemelas no se guarda como %v: %v", ahora, err)
		}
	}
	s, p, _ := gemelasT(t, "Ventas", "VENTAS")
	if err := s.escribir(libroDe("Ventas", "VENTAS", "ventas"), false, *p); !errors.Is(err, ErrHojaGemela) {
		t.Errorf("una tercera gemela se guarda: %v", err)
	}
}

// TestLasGemelasNoCuestanUnaLecturaEnCadaGuardado: sin gemelas en lo que llega no se lee
// lo guardado; sin nada guardado no hay con qué comparar y el contenido se acepta; una
// restauración puede devolver las gemelas de entonces.
func TestLasGemelasNoCuestanUnaLecturaEnCadaGuardado(t *testing.T) {
	s, p, lecturas := gemelasT(t, "Ventas")
	if err := s.escribir(libroDe("Ventas", "Resumen"), false, *p); err != nil || *lecturas != 0 {
		t.Errorf("un guardado sin gemelas da %v y lee el registro %d veces", err, *lecturas)
	}

	nuevo := nuevaSimP(t)
	if err := nuevo.escribir(libroDe("Ventas", "VENTAS"), false, Proteccion{Vigente: func() (json.RawMessage, error) {
		t.Error("sin registro guardado no hay nada que leer")
		return nil, nil
	}}); err != nil {
		t.Errorf("el primer contenido de un libro se rechaza: %v", err)
	}

	s, p, _ = gemelasT(t, "Ventas")
	r := *p
	r.Restaurando = true
	if err := s.escribir(libroDe("Ventas", "VENTAS"), false, r); err != nil {
		t.Errorf("restaurar una versión con gemelas se rechaza: %v", err)
	}
}

// TestLasGemelasEnUnLoteSeComparanConLaAnterior: con dos modelos en un lote, el segundo se
// compara con el primero (el que acaba de ganar), no con lo guardado.
func TestLasGemelasEnUnLoteSeComparanConLaAnterior(t *testing.T) {
	s, p, _ := gemelasT(t, "Ventas", "VENTAS")
	op := func(m mj, lam uint64) Op {
		return Op{Kind: OpSet, Key: claveHoja, Value: valT(t, m), Dot: Dot{Actor: "c", Counter: lam}, Lamport: lam}
	}
	lote := []Op{op(libroDe("Ventas", "Ventas 2"), 10), op(libroDe("Ventas", "VENTAS"), 11)}
	if _, err := AplicarProteccion(s.est, lote, false, *p); !errors.Is(err, ErrHojaGemela) {
		t.Errorf("la gemela deshecha y rehecha en el mismo lote se cuela: %v", err)
	}
}

// TestUnLibroProtegidoComparaConSuModelo: con protección el estado ya guarda el modelo
// vigente y la regla compara con él, sin leer el registro.
func TestUnLibroProtegidoComparaConSuModelo(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	hoja := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})
	sinLeer := Proteccion{Vigente: func() (json.RawMessage, error) {
		t.Error("con el modelo en el estado no hace falta leer el registro")
		return nil, nil
	}}
	if err := s.escribir(libroT(hoja, hojaT("h", nil, nil)), true, sinLeer); !errors.Is(err, ErrHojaGemela) {
		t.Errorf("una gemela de la hoja protegida se guarda: %v", err)
	}
	if err := s.escribir(libroT(hoja, hojaT("Otra", nil, nil)), true, sinLeer); err != nil {
		t.Errorf("una hoja con otro nombre no se guarda: %v", err)
	}
}

// TestElImportadorReparaLasHojasGemelas: un .xlsx o un .ods con «Ventas» y «VENTAS» (Excel
// no los escribe, pero un fichero hecho a mano o por otro programa sí) se abre con la
// segunda renombrada —«VENTAS (2)», o el primer número libre: aquí «VENTAS (3)», que
// «Ventas (2)» ya está— y sus nombres definidos van con ella. VISTO EN ROJO sin la
// reparación: «Ventas,VENTAS,Ventas (2)».
func TestElImportadorReparaLasHojasGemelas(t *testing.T) {
	m := &sheetModel{
		Sheets: []sheetData{
			{Name: "Ventas", Cells: map[string]string{"A1": "1"}},
			{Name: "VENTAS", Cells: map[string]string{"A1": "2"}},
			{Name: "Ventas (2)", Cells: map[string]string{"A1": "3"}},
		},
		Names: []definedName{{Name: "Total", Ref: "$A$1", Sheet: "VENTAS"}},
	}
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, ext, err := Export(KindSpreadsheet, "x", stateWithContent(t, "doc:sheet", marshalSheetModel(m)), f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import("gemelas."+ext, data)
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		got := parseSheetModelJSON(t, res.Content)
		nombres := make([]string, len(got.Sheets))
		for i, h := range got.Sheets {
			nombres[i] = h.Name
		}
		if strings.Join(nombres, ",") != "Ventas,VENTAS (3),Ventas (2)" {
			t.Errorf("%s: las hojas se abren como %q", ext, nombres)
		}
		if got.Sheets[1].Cells["A1"] != "2" {
			t.Errorf("%s: la hoja renombrada no es la que era: %v", ext, got.Sheets[1].Cells)
		}
		for _, n := range got.Names {
			if n.Name == "Total" && n.Sheet != "VENTAS (3)" {
				t.Errorf("%s: el nombre de la hoja renombrada queda en %q", ext, n.Sheet)
			}
		}
	}
}
