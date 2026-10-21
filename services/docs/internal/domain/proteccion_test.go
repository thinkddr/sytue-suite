// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type mj = map[string]any

// val es el valor del registro tal como lo guarda el editor: JSON.stringify(modelo)
// dentro de un string JSON.
func valT(t *testing.T, modelo mj) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(modelo)
	if err != nil {
		t.Fatal(err)
	}
	return jsonString(string(b))
}

func hojaT(nombre string, celdas, extra mj) mj {
	h := mj{"name": nombre, "cells": celdas}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func libroT(hojas ...mj) mj {
	hs := make([]any, len(hojas))
	for i, h := range hojas {
		hs[i] = h
	}
	return mj{"active": 0, "sheets": hs}
}

// simP lleva el estado de protección de un documento como lo haría el almacén.
type simP struct {
	t   *testing.T
	est EstadoProteccion
	lam uint64
	// última op aceptada, para mirar el valor saneado.
	ultima Op
}

func nuevaSimP(t *testing.T) *simP {
	return &simP{t: t, est: EstadoProteccion{Claves: map[string]ClaveHash{}}}
}

func (s *simP) clonar(t *testing.T) *simP {
	c := &simP{
		t: t,
		est: EstadoProteccion{
			Modelo:      s.est.Modelo,
			HayRegistro: s.est.HayRegistro,
			Lamport:     s.est.Lamport,
			Dot:         s.est.Dot,
			Claves:      make(map[string]ClaveHash, len(s.est.Claves)),
			Secuencia:   s.est.Secuencia,
		},
		lam:    s.lam,
		ultima: s.ultima,
	}
	for k, v := range s.est.Claves {
		c.est.Claves[k] = v
	}
	return c
}

func (s *simP) escribir(modelo mj, dueno bool, p Proteccion) error {
	s.t.Helper()
	s.lam++
	return s.escribirCon(modelo, dueno, p, s.lam)
}

func (s *simP) escribirCon(modelo mj, dueno bool, p Proteccion, lamport uint64) error {
	s.t.Helper()
	op := Op{Kind: OpSet, Key: claveHoja, Value: valT(s.t, modelo), Dot: Dot{Actor: "c", Counter: lamport}, Lamport: lamport}
	res, err := AplicarProteccion(s.est, []Op{op}, dueno, p)
	if err != nil {
		return err
	}
	s.est = res.Estado
	s.ultima = res.Ops[0]
	return nil
}

// protegidaT deja una hoja «H» protegida con la contraseña dada y devuelve el
// modelo tal como queda en el servidor (sin contraseña, con hasPassword).
func protegidaT(t *testing.T, s *simP, clave string, opciones mj) {
	t.Helper()
	prot := mj{}
	for k, v := range opciones {
		prot[k] = v
	}
	nuevas := map[string]string{}
	if clave != "" {
		nuevas[AmbitoHoja("H")] = clave
	}
	err := s.escribir(libroT(hojaT("H",
		mj{"A1": "10", "A2": "20", "B1": "libre"},
		mj{"protect": prot, "styles": mj{"B1": mj{"locked": false}}})), false, Proteccion{Nuevas: nuevas})
	if err != nil {
		t.Fatalf("proteger: %v", err)
	}
}

func TestProteccionContrasenaSoloComoHash(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "secreto", mj{"formatCells": true})

	if _, ok := s.est.Claves[AmbitoHoja("H")]; !ok {
		t.Fatal("no se guardó el hash")
	}
	if strings.Contains(string(s.ultima.Value), "secreto") || strings.Contains(s.est.Modelo, "secreto") {
		t.Fatal("la contraseña en claro quedó en la operación o en el modelo guardado")
	}
	h := s.est.Claves[AmbitoHoja("H")]
	if h.Algoritmo != "SHA-512" || h.Vueltas != 100000 || len(h.Sal) != 16 || len(h.Hash) != 64 {
		t.Fatalf("hash fuera del esquema OOXML: %+v", h)
	}
	if !h.Coincide("secreto") || h.Coincide("Secreto") || h.Coincide("") {
		t.Fatal("la verificación no distingue contraseñas")
	}
	// El editor sabe que HAY contraseña (para pedirla) sin conocer nada más.
	if !strings.Contains(string(s.ultima.Value), `hasPassword`) {
		t.Fatalf("falta hasPassword en lo que se sirve: %s", s.ultima.Value)
	}
	// Dos protecciones con la misma contraseña no comparten sal.
	s2 := nuevaSimP(t)
	protegidaT(t, s2, "secreto", nil)
	if bytes.Equal(s2.est.Claves[AmbitoHoja("H")].Sal, h.Sal) {
		t.Fatal("la sal se repite")
	}
}

// Esquema OOXML: H0 = SHA512(sal ‖ UTF-16LE(clave)), Hn = SHA512(Hn-1 ‖ n uint32 LE).
func TestHashClaveSigueElEsquemaOOXML(t *testing.T) {
	sal := []byte("0123456789abcdef")
	clave := "Contraseña ñ 😀"
	var u []byte
	for _, r := range clave {
		if r > 0xFFFF {
			r -= 0x10000
			hi, lo := 0xD800+(r>>10), 0xDC00+(r&0x3FF)
			u = append(u, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
			continue
		}
		u = append(u, byte(r), byte(r>>8))
	}
	h := sha512.Sum512(append(append([]byte{}, sal...), u...))
	for i := uint32(0); i < 7; i++ {
		var it [4]byte
		binary.LittleEndian.PutUint32(it[:], i)
		h = sha512.Sum512(append(h[:], it[:]...))
	}
	if got := derivarClaveOOXML(sal, clave, 7); !bytes.Equal(got, h[:]) {
		t.Fatalf("hash distinto del esquema:\n%x\n%x", got, h)
	}
}

func TestProteccionCeldasBloqueadasYDesbloqueadas(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	base := func() mj {
		return hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})
	}

	// Celda desbloqueada: pasa.
	ok := base()
	ok["cells"] = mj{"A1": "10", "A2": "20", "B1": "cambiada"}
	if err := s.escribir(libroT(ok), false, Proteccion{}); err != nil {
		t.Fatalf("editar una celda desbloqueada: %v", err)
	}
	// Celda bloqueada (existente y nueva): no.
	for nombre, celdas := range map[string]mj{
		"existente": {"A1": "99", "A2": "20", "B1": "cambiada"},
		"nueva":     {"A1": "10", "A2": "20", "B1": "cambiada", "C9": "x"},
		"borrada":   {"A2": "20", "B1": "cambiada"},
	} {
		h := base()
		h["cells"] = celdas
		if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s: se escribió una celda bloqueada: %v", nombre, err)
		}
	}
	// El propietario y quien trae la contraseña, sí.
	h := base()
	h["cells"] = mj{"A1": "99", "A2": "20", "B1": "cambiada"}
	if err := s.escribir(libroT(h), true, Proteccion{}); err != nil {
		t.Fatalf("propietario: %v", err)
	}
	h = base()
	h["cells"] = mj{"A1": "100", "A2": "20", "B1": "cambiada"}
	if err := s.escribir(libroT(h), false, Proteccion{Clave: "k"}); err != nil {
		t.Fatalf("con contraseña: %v", err)
	}
}

func TestProteccionQuitarOCambiarSoloConClave(t *testing.T) {
	cuerpo := func(extra mj) mj {
		return libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, extra))
	}
	sinProteger := cuerpo(nil)
	casos := []struct {
		nombre  string
		modelo  mj
		p       Proteccion
		dueno   bool
		quiere  error
		quedaPw bool
	}{
		{"quitar sin clave", sinProteger, Proteccion{}, false, ErrProtegido, true},
		{"quitar con clave mala", sinProteger, Proteccion{Clave: "mala"}, false, ErrClaveProteccion, true},
		{"quitar con clave buena", sinProteger, Proteccion{Clave: "k"}, false, nil, false},
		{"quitar siendo propietario", sinProteger, Proteccion{}, true, nil, false},
		{"cambiar opciones sin clave", cuerpo(mj{"protect": mj{"hasPassword": true, "formatCells": true}}), Proteccion{}, false, ErrProtegido, true},
		{"cambiar opciones con clave", cuerpo(mj{"protect": mj{"hasPassword": true, "formatCells": true}}), Proteccion{Clave: "k"}, false, nil, true},
		{"nueva contraseña sin clave", cuerpo(mj{"protect": mj{"hasPassword": true}}), Proteccion{Nuevas: map[string]string{AmbitoHoja("H"): "otra"}}, false, ErrClaveProteccion, true},
		{"nueva contraseña con clave", cuerpo(mj{"protect": mj{"hasPassword": true}}), Proteccion{Clave: "k", Nuevas: map[string]string{AmbitoHoja("H"): "otra"}}, false, nil, true},
		{"quitar solo la contraseña sin clave", cuerpo(mj{"protect": mj{"hasPassword": false}}), Proteccion{}, false, ErrProtegido, true},
		{"quitar solo la contraseña con clave", cuerpo(mj{"protect": mj{"hasPassword": false}}), Proteccion{Clave: "k"}, false, nil, false},
		{"contraseña en claro de un cliente anterior, la misma", cuerpo(mj{"protect": mj{"password": "k"}}), Proteccion{}, false, nil, true},
		{"contraseña en claro de un cliente anterior, otra", cuerpo(mj{"protect": mj{"password": "otra"}}), Proteccion{}, false, ErrClaveProteccion, true},
		{"agotados los intentos, ni la buena del modelo", cuerpo(mj{"protect": mj{"password": "k"}}), Proteccion{SinIntentos: true}, false, ErrClaveProteccion, true},
		{
			"agotados los intentos, se escribe lo que no pide contraseña",
			libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "otra cosa"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})),
			Proteccion{SinIntentos: true},
			false, nil, true,
		},
	}
	base := nuevaSimP(t)
	protegidaT(t, base, "k", nil)
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := base.clonar(t)
			antes := s.est.Modelo
			err := s.escribir(c.modelo, c.dueno, c.p)
			if !errors.Is(err, c.quiere) || (c.quiere == nil && err != nil) {
				t.Fatalf("esperaba %v, salió %v", c.quiere, err)
			}
			if c.quiere != nil && s.est.Modelo != antes {
				t.Fatal("una escritura denegada cambió el estado")
			}
			_, hay := s.est.Claves[AmbitoHoja("H")]
			if c.quiere == nil && c.quedaPw != hay {
				t.Fatalf("hash vigente=%v, esperaba %v", hay, c.quedaPw)
			}
			if strings.Contains(string(s.ultima.Value), `"password"`) || strings.Contains(string(s.ultima.Value), `\"password\"`) {
				t.Fatalf("quedó una contraseña en claro en la op: %s", s.ultima.Value)
			}
		})
	}
}

// Una protección sin contraseña solo la quita el propietario (decisión del dueño).
func TestProteccionSinClaveSoloLaQuitaElPropietario(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "", nil)
	desprotegida := libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, nil))
	if err := s.escribir(desprotegida, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("un editor quitó una protección sin contraseña: %v", err)
	}
	if err := s.escribir(desprotegida, false, Proteccion{Clave: "lo que sea"}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("una contraseña inventada abrió una protección sin contraseña: %v", err)
	}
	if err := s.escribir(desprotegida, true, Proteccion{}); err != nil {
		t.Fatalf("el propietario no pudo quitarla: %v", err)
	}
	if s.est.Modelo != "" {
		t.Fatal("quedó estado de protección sin protección")
	}
}

func TestProteccionBloqueoYOcultacionNoSeCambian(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", mj{"formatCells": true})
	prot := mj{"hasPassword": true, "formatCells": true}
	casos := map[string]mj{
		"desbloquear una celda":    {"B1": mj{"locked": false}, "A1": mj{"locked": false}},
		"bloquear la desbloqueada": {},
		"ocultar una fórmula":      {"B1": mj{"locked": false}, "A1": mj{"hidden": true}},
	}
	for nombre, estilos := range casos {
		h := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": prot, "styles": estilos})
		if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
	// Dar formato con formatCells concedido: pasa.
	h := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": prot, "styles": mj{"B1": mj{"locked": false, "color": "#ff0000"}, "A1": mj{"color": "#00ff00"}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); err != nil {
		t.Fatalf("formato permitido: %v", err)
	}
}

func TestProteccionFormatoSinPermisoSeVeta(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	prot := mj{"hasPassword": true}
	con := func(extra mj) mj {
		e := mj{"protect": prot, "styles": mj{"B1": mj{"locked": false}}}
		for k, v := range extra {
			e[k] = v
		}
		return libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, e))
	}
	for nombre, extra := range map[string]mj{
		"color":            {"styles": mj{"B1": mj{"locked": false, "color": "#ff0000"}}},
		"formato numérico": {"formats": mj{"B1": mj{"id": "percent"}}},
	} {
		if err := s.escribir(con(extra), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s sin formatCells: %v", nombre, err)
		}
	}
}

func TestProteccionPermisosGranulares(t *testing.T) {
	campos := map[string]struct {
		permiso string
		campo   string
		valor   any
	}{
		"autofiltro":          {"autoFilter", "autofilter", mj{"row": 0, "c1": 0, "c2": 3}},
		"formato condicional": {"objects", "condFormats", []any{mj{"range": "A1:A9"}}},
		"validación":          {"objects", "validations", []any{mj{"range": "B1"}}},
		"gráfico":             {"objects", "charts", []any{mj{"type": "bar"}}},
		"sparkline":           {"objects", "sparklines", []any{mj{"cell": "C1"}}},
		"segmentación":        {"objects", "slicers", []any{mj{"id": "s1"}}},
		"escenario":           {"scenarios", "scenarios", []any{mj{"name": "optimista"}}},
	}
	for nombre, c := range campos {
		t.Run(nombre, func(t *testing.T) {
			// Sin el permiso: vetado.
			s := nuevaSimP(t)
			protegidaT(t, s, "k", nil)
			h := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}, c.campo: c.valor})
			if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("sin permiso: %v", err)
			}
			// Con el permiso concedido: pasa.
			s2 := nuevaSimP(t)
			protegidaT(t, s2, "k", mj{c.permiso: true})
			h = hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true, c.permiso: true}, "styles": mj{"B1": mj{"locked": false}}, c.campo: c.valor})
			if err := s2.escribir(libroT(h), false, Proteccion{}); err != nil {
				t.Fatalf("con permiso: %v", err)
			}
		})
	}
}

func TestProteccionEstructuralMueveContenidoPeroNoLoInventa(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", mj{"insertRows": true})
	prot := mj{"hasPassword": true, "insertRows": true}
	// Insertar una fila arriba: lo bloqueado baja una fila.
	h := hojaT("H", mj{"A2": "10", "A3": "20", "B2": "libre"}, mj{"protect": prot, "styles": mj{"B2": mj{"locked": false}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); err != nil {
		t.Fatalf("insertar fila: %v", err)
	}
	// Contenido nuevo en una celda bloqueada: no.
	h = hojaT("H", mj{"A2": "10", "A3": "inventado", "B2": "libre"}, mj{"protect": prot, "styles": mj{"B2": mj{"locked": false}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("inventó contenido bloqueado: %v", err)
	}
	// Duplicar contenido bloqueado en otra celda bloqueada: no.
	h = hojaT("H", mj{"A2": "10", "A3": "20", "A4": "20", "B2": "libre"}, mj{"protect": prot, "styles": mj{"B2": mj{"locked": false}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("duplicó contenido bloqueado: %v", err)
	}
	// Desbloquear una celda bloqueada de las que había: no.
	h = hojaT("H", mj{"A2": "10", "A3": "20", "B2": "libre"}, mj{"protect": prot, "styles": mj{"B2": mj{"locked": false}, "A2": mj{"locked": false}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("desbloqueó una celda: %v", err)
	}
}

func TestProteccionDeLibroBloqueaLaEstructura(t *testing.T) {
	s := nuevaSimP(t)
	a, b := hojaT("A", mj{"A1": "1"}, nil), hojaT("B", mj{"A1": "2"}, nil)
	protegido := libroT(a, b)
	protegido["protect"] = mj{}
	if err := s.escribir(protegido, false, Proteccion{Nuevas: map[string]string{AmbitoLibro: "lib"}}); err != nil {
		t.Fatalf("proteger libro: %v", err)
	}
	protegido["protect"] = mj{"hasPassword": true}
	c := hojaT("C", nil, nil)
	casos := map[string]mj{
		"añadir hoja":    {"sheets": []any{a, b, c}},
		"borrar hoja":    {"sheets": []any{a}},
		"renombrar hoja": {"sheets": []any{hojaT("Z", mj{"A1": "1"}, nil), b}},
		"reordenar":      {"sheets": []any{b, a}},
		"quitar":         {"sheets": []any{a, b}, "protect": nil},
	}
	for nombre, c := range casos {
		l := libroT(a, b)
		l["protect"] = protegido["protect"]
		for k, v := range c {
			l[k] = v
		}
		if v, ok := c["protect"]; ok && v == nil {
			delete(l, "protect")
		}
		if err := s.escribir(l, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
	// Editar celdas y cambiar de hoja activa es lo que el libro protegido SÍ deja.
	l := libroT(hojaT("A", mj{"A1": "cambiada"}, nil), b)
	l["protect"], l["active"] = protegido["protect"], 1
	if err := s.escribir(l, false, Proteccion{}); err != nil {
		t.Fatalf("editar con el libro protegido: %v", err)
	}
	// Con la contraseña del libro, sí se cambia la estructura.
	l = libroT(a, b, c)
	l["protect"] = protegido["protect"]
	if err := s.escribir(l, false, Proteccion{Clave: "lib"}); err != nil {
		t.Fatalf("estructura con contraseña: %v", err)
	}
}

// Borrar una hoja protegida destruye contenido protegido: es una operación protegida.
func TestProteccionBorrarORenombrarLaHojaProtegida(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	otra := hojaT("Otra", mj{"A1": "x"}, nil)
	for nombre, l := range map[string]mj{
		"borrar":           libroT(otra),
		"renombrar":        libroT(hojaT("H2", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})),
		"quitar el modelo": nil,
	} {
		var err error
		if l == nil {
			s.lam++
			var res ResultadoProteccion
			res, err = AplicarProteccion(s.est, []Op{{Kind: OpSet, Key: claveHoja, Value: json.RawMessage("null"), Dot: Dot{Actor: "c", Counter: s.lam}, Lamport: s.lam}}, false, Proteccion{})
			_ = res
		} else {
			err = s.escribir(l, false, Proteccion{})
		}
		if !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
}

// Una hoja gemela con el mismo nombre, puesta delante, no puede sustituir a la protegida.
func TestProteccionNombresRepetidos(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	gemela := hojaT("H", mj{"A1": "falsa"}, nil)
	original := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})
	for nombre, l := range map[string]mj{"delante": libroT(gemela, original), "detrás": libroT(original, gemela)} {
		if err := s.escribir(l, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
}

func TestProteccionCeldasSueltasHeredadasYValoresIlegibles(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	cell := Op{Kind: OpSet, Key: "cell:H!1:1", Value: json.RawMessage(`"x"`), Dot: Dot{Actor: "c", Counter: 99}, Lamport: 99}
	if _, err := AplicarProteccion(s.est, []Op{cell}, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("celda suelta en documento protegido: %v", err)
	}
	if _, err := AplicarProteccion(s.est, []Op{cell}, true, Proteccion{}); err != nil {
		t.Fatalf("propietario: %v", err)
	}
	for nombre, v := range map[string]json.RawMessage{
		"basura":    json.RawMessage(`"no es json"`),
		"un número": json.RawMessage(`7`),
		"un objeto": json.RawMessage(`{"sheets":[]}`),
		"sin hojas": jsonString(`{"active":0}`),
	} {
		op := Op{Kind: OpSet, Key: claveHoja, Value: v, Dot: Dot{Actor: "c", Counter: 100}, Lamport: 100}
		if _, err := AplicarProteccion(s.est, []Op{op}, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("%s sustituyó al modelo protegido: %v", nombre, err)
		}
	}
}

// Una op que el registro descarta por LWW no cambia nada y no se evalúa como cambio;
// sí se le quita la contraseña en claro.
func TestProteccionOpPerdedoraNoCuentaPeroSeSanea(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil) // lamport 1
	s.lam = 5
	if err := s.escribir(libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre2"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}})), false, Proteccion{}); err != nil {
		t.Fatal(err)
	}
	antes := s.est.Modelo
	// Lamport 2 < 6: pierde. Intenta desproteger y trae una contraseña en claro.
	perdedora := libroT(hojaT("H", mj{"A1": "hackeada"}, mj{"protect": mj{"password": "en claro"}}))
	if err := s.escribirCon(perdedora, false, Proteccion{}, 2); err != nil {
		t.Fatalf("la op perdedora no debe fallar: %v", err)
	}
	if s.est.Modelo != antes {
		t.Fatal("una op perdedora cambió el estado")
	}
	if strings.Contains(string(s.ultima.Value), "en claro") {
		t.Fatalf("la contraseña de una op perdedora quedó en el registro: %s", s.ultima.Value)
	}
}

func TestProteccionDeVariasOpsEnUnLote(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	limpio := libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, nil))
	ok := Op{Kind: OpSet, Key: claveHoja, Value: valT(t, libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "x"}, mj{"protect": mj{"hasPassword": true}, "styles": mj{"B1": mj{"locked": false}}}))), Dot: Dot{Actor: "c", Counter: 10}, Lamport: 10}
	mal := Op{Kind: OpSet, Key: claveHoja, Value: valT(t, limpio), Dot: Dot{Actor: "c", Counter: 11}, Lamport: 11}
	if _, err := AplicarProteccion(s.est, []Op{ok, mal}, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("la segunda op del lote se coló: %v", err)
	}
	// Orden inverso: la desprotección va primero y también cae.
	if _, err := AplicarProteccion(s.est, []Op{mal, ok}, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("la primera op del lote se coló: %v", err)
	}
}

// Migración de los modelos con la contraseña en claro: sin perder la protección.
func TestProteccionMigraLaContrasenaEnClaro(t *testing.T) {
	legado := libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": mj{"password": "vieja", "formatCells": true}, "styles": mj{"B1": mj{"locked": false}}}))
	legado["protect"] = mj{"password": "libro"}
	st := NewState()
	if _, err := st.Apply(Op{Kind: OpSet, Key: claveHoja, Value: valT(t, legado), Dot: Dot{Actor: "x", Counter: 1}, Lamport: 7}); err != nil {
		t.Fatal(err)
	}
	est, err := EstadoInicialProteccion(KindSpreadsheet, st)
	if err != nil {
		t.Fatal(err)
	}
	if !est.HayRegistro || est.Lamport != 7 {
		t.Fatalf("metadatos LWW perdidos: %+v", est)
	}
	if !est.Claves[AmbitoHoja("H")].Coincide("vieja") || !est.Claves[AmbitoLibro].Coincide("libro") {
		t.Fatal("la contraseña heredada no se convirtió en hash")
	}
	if strings.Contains(est.Modelo, "vieja") || strings.Contains(est.Modelo, "libro\"") && strings.Contains(est.Modelo, "password") {
		t.Fatalf("el modelo vigente conserva la contraseña: %s", est.Modelo)
	}
	// Y la protección sigue en pie: sin contraseña no se desprotege.
	s := &simP{t: t, est: est, lam: 10}
	if err := s.escribir(libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, nil)), false, Proteccion{}); err == nil {
		t.Fatal("la migración perdió la protección")
	}
	l := libroT(hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, nil))
	l["protect"] = mj{"hasPassword": true}
	if err := s.escribir(l, false, Proteccion{Clave: "vieja"}); err != nil {
		t.Fatalf("la contraseña heredada ya no abre: %v", err)
	}
}

func TestRedactarNoDejaSalirContrasenas(t *testing.T) {
	legado := libroT(hojaT("H", mj{"A1": "1"}, mj{"protect": mj{"password": "vieja", "sort": true}}))
	legado["protect"] = mj{"password": "libro"}
	v := valT(t, legado)
	out := string(RedactarValor(v))
	if strings.Contains(out, "vieja") || strings.Contains(out, "libro\\\"}") || strings.Contains(out, "password") {
		t.Fatalf("salió una contraseña: %s", out)
	}
	if !strings.Contains(out, "sort") {
		t.Fatalf("la redacción perdió las opciones: %s", out)
	}
	// Sin contraseña, el valor se devuelve idéntico.
	limpio := valT(t, libroT(hojaT("H", mj{"A1": "1"}, nil)))
	if string(RedactarValor(limpio)) != string(limpio) {
		t.Fatal("tocó un valor sin contraseñas")
	}
	st := NewState()
	_, _ = st.Apply(Op{Kind: OpSet, Key: claveHoja, Value: v, Dot: Dot{Actor: "x", Counter: 1}, Lamport: 1})
	RedactarEstado(st)
	if strings.Contains(string(st.Registers[claveHoja].Value), "vieja") {
		t.Fatal("el estado servido lleva la contraseña")
	}
	ops := RedactarOps([]SequencedOp{{Op: Op{Kind: OpSet, Key: claveHoja, Value: v}}, {Op: Op{Kind: OpSet, Key: "otra", Value: v}}})
	if strings.Contains(string(ops[0].Op.Value), "vieja") {
		t.Fatal("la op servida lleva la contraseña")
	}
	if string(ops[1].Op.Value) != string(v) {
		t.Fatal("redactó una clave que no es la hoja")
	}
}

func TestLimitadorDeClaves(t *testing.T) {
	var l limitadorClaves
	for i := 0; i < intentosMax-1; i++ {
		l.fallo("d|ana")
	}
	if l.bloqueada("d|ana") {
		t.Fatal("bloqueó antes de tiempo")
	}
	l.fallo("d|ana")
	if !l.bloqueada("d|ana") || l.bloqueada("d|bea") {
		t.Fatal("el bloqueo no es por documento y persona")
	}
	// Caducan.
	l.fallos["d|ana"] = []time.Time{time.Now().Add(-intentosVenta - time.Second)}
	if l.bloqueada("d|ana") {
		t.Fatal("el bloqueo no caduca")
	}
}

// Tablas, tablas dinámicas y datos externos no se cambian con la hoja protegida, aunque no
// toquen ninguna celda: Excel deshabilita crearlas o cambiarlas.
func TestProteccionTablasPivotsYDatosExternos(t *testing.T) {
	campos := map[string]any{
		"tabla":          []any{mj{"name": "T1", "range": "A1:B2"}},
		"tabla dinámica": []any{mj{"name": "P1"}},
		"datos externos": []any{mj{"name": "X1"}},
	}
	clave := map[string]string{"tabla": "tables", "tabla dinámica": "pivots", "datos externos": "externalTables"}
	for nombre, valor := range campos {
		t.Run(nombre, func(t *testing.T) {
			s := nuevaSimP(t)
			protegidaT(t, s, "k", nil)
			prot := mj{"hasPassword": true}
			h := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": prot, "styles": mj{"B1": mj{"locked": false}}, clave[nombre]: valor})
			if err := s.escribir(libroT(h), false, Proteccion{}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("sin permiso: %v", err)
			}
			// Ni con todos los permisos de objetos: no hay uno que lo conceda.
			s2 := nuevaSimP(t)
			protegidaT(t, s2, "k", mj{"objects": true, "scenarios": true, "autoFilter": true})
			prot = mj{"hasPassword": true, "objects": true, "scenarios": true, "autoFilter": true}
			h = hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"protect": prot, "styles": mj{"B1": mj{"locked": false}}, clave[nombre]: valor})
			err := s2.escribir(libroT(h), false, Proteccion{})
			// Las tablas pasan solo con un permiso estructural (se desplazan con las filas).
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("con permisos de objetos: %v", err)
			}
		})
	}
	// El propietario sí puede: el cambio entra con la protección quitada.
	s := nuevaSimP(t)
	protegidaT(t, s, "k", nil)
	h := hojaT("H", mj{"A1": "10", "A2": "20", "B1": "libre"}, mj{"tables": []any{mj{"name": "T1"}}})
	if err := s.escribir(libroT(h), true, Proteccion{}); err != nil {
		t.Fatalf("propietario: %v", err)
	}
}

func TestProteccionTablasSeDesplazanConPermisoEstructural(t *testing.T) {
	s := nuevaSimP(t)
	protegidaT(t, s, "k", mj{"insertRows": true})
	prot := mj{"hasPassword": true, "insertRows": true}
	h := hojaT("H", mj{"A2": "10", "A3": "20", "B2": "libre"}, mj{"protect": prot, "styles": mj{"B2": mj{"locked": false}}, "tables": []any{mj{"name": "T1", "range": "A2:B3"}}})
	if err := s.escribir(libroT(h), false, Proteccion{}); err != nil {
		t.Fatalf("tabla desplazada: %v", err)
	}
}
