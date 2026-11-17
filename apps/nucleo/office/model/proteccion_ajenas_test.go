// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"errors"
	"testing"
)

// resumenAjenasT es «Ventas» (con los permisos de su protección, o sin proteger si
// es nil) y «Resumen» protegida con contraseña, con las celdas dadas: A1, A2 y A4
// bloqueadas, A3 desbloqueada y B1:B2 un rango editable de ana.
func resumenAjenasT(ventas, resumen, protVentas mj) mj {
	extraV := mj{}
	if protVentas != nil {
		p := mj{"hasPassword": true}
		for k, v := range protVentas {
			p[k] = v
		}
		extraV["protect"] = p
	}
	return libroT(
		hojaT("Ventas", ventas, extraV),
		hojaT("Resumen", resumen, mj{
			"protect": mj{"hasPassword": true, "ranges": []any{mj{"ref": "B1:B2", "users": []any{"ana"}}}},
			"styles":  mj{"A3": mj{"locked": false}},
		}),
	)
}

var (
	ventasT        = mj{"B4": "10", "B5": "20", "C4": "x"}
	ventasFilaT    = mj{"B5": "10", "B6": "20", "C5": "x"} // una fila insertada encima de la 4
	resumenT       = mj{"A1": "=Ventas!B4", "A2": "<b>=SUM(ventas!B4:B5)*A9</b>", "A3": "=Ventas!B4", "A4": "=Hoja2!B4", "B1": "=Ventas!B5"}
	resumenMovidoT = mj{"A1": "=Ventas!B5", "A2": "<b>=SUM(ventas!B5:B6)*A9</b>", "A3": "=Ventas!B5", "A4": "=Hoja2!B4", "B1": "=Ventas!B6"}
)

// protegidaAjenasT protege «Resumen» (y «Ventas» con protVentas, si no es nil).
func protegidaAjenasT(t *testing.T, protVentas mj) *simP {
	t.Helper()
	s := nuevaSimP(t)
	nuevas := map[string]string{AmbitoHoja("Resumen"): "k"}
	if protVentas != nil {
		nuevas[AmbitoHoja("Ventas")] = "v"
	}
	if err := s.escribir(resumenAjenasT(ventasT, resumenT, protVentas), false, Proteccion{Nuevas: nuevas}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	return s
}

// Insertar una fila en «Ventas» reescribe las fórmulas bloqueadas de «Resumen» que la
// citan, y las del rango editable de ana aunque escriba otra persona: es el
// desplazamiento, no una edición.
func TestFormulasBloqueadasSiguenALaHojaQueSeMueve(t *testing.T) {
	casos := []struct {
		nombre     string
		protVentas mj
		ventas     mj
		resumen    mj
	}{
		{"insertar una fila en Ventas sin proteger", nil, ventasFilaT, resumenMovidoT},
		{"insertar con el permiso de Ventas", mj{"insertRows": true}, ventasFilaT, resumenMovidoT},
		{
			"borrar la fila 4: la celda suelta pasa a #¡REF! y el rango encoge", nil,
			mj{"B4": "20"},
			mj{"A1": "=#¡REF!", "A2": "<b>=SUM(ventas!B4:B4)*A9</b>", "A3": "=#¡REF!", "A4": "=Hoja2!B4", "B1": "=Ventas!B4"},
		},
		{
			"insertar dos columnas antes de la B", nil,
			mj{"D4": "10", "D5": "20", "E4": "x"},
			mj{"A1": "=Ventas!D4", "A2": "<b>=SUM(ventas!D4:D5)*A9</b>", "A3": "=Ventas!D4", "A4": "=Hoja2!B4", "B1": "=Ventas!D5"},
		},
		{
			// Las celdas de «Ventas» no tienen por qué haberse movido: insertar y volver a
			// escribirlas como estaban es algo que la persona podía hacer.
			"aunque Ventas no se vea movida", nil, ventasT, resumenMovidoT,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaAjenasT(t, c.protVentas)
			if err := s.escribir(resumenAjenasT(c.ventas, c.resumen, c.protVentas), false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
		})
	}
}

// Lo que no es exactamente un desplazamiento sigue vetado.
func TestFormulasBloqueadasNoSeReescribenAMano(t *testing.T) {
	con := func(cambios mj) mj {
		out := mj{}
		for k, v := range resumenMovidoT {
			out[k] = v
		}
		for k, v := range cambios {
			out[k] = v
		}
		return out
	}
	casos := []struct {
		nombre     string
		protVentas mj
		resumen    mj
	}{
		{"apuntar a otra celda", nil, con(mj{"A1": "=Ventas!B9"})},
		{"mover unas y otras no", nil, con(mj{"A2": "<b>=SUM(ventas!B4:B5)*A9</b>"})},
		{"cambiar algo más de la fórmula", nil, con(mj{"A2": "<b>=SUM(ventas!B5:B6)*A8</b>"})},
		{"cambiar el formato", nil, con(mj{"A2": "=SUM(ventas!B5:B6)*A9"})},
		{"mover la hoja que no se ha movido", nil, con(mj{"A4": "=Hoja2!B5"})},
		{"sin el permiso de insertar en Ventas", mj{"deleteRows": true}, resumenMovidoT},
		{"una celda bloqueada nueva", nil, con(mj{"A5": "=Ventas!B5"})},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaAjenasT(t, c.protVentas)
			err := s.escribir(resumenAjenasT(ventasFilaT, c.resumen, c.protVentas), false, Proteccion{Quien: luis})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
}

// Las propias referencias de la hoja protegida no se explican con otra hoja: `=A9` no
// sigue a «Ventas».
func TestFormulasPropiasNoSeMuevenPorOtraHoja(t *testing.T) {
	s := protegidaAjenasT(t, nil)
	r := mj{}
	for k, v := range resumenMovidoT {
		r[k] = v
	}
	r["A2"] = "<b>=SUM(ventas!B5:B6)*A10</b>"
	if err := s.escribir(resumenAjenasT(ventasFilaT, r, nil), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("quiero ErrProtegido y es %v", err)
	}
}
