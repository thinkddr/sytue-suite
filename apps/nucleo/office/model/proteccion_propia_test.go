// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"
)

// Al insertar o borrar filas o columnas en una hoja protegida con ese permiso, sus
// propias fórmulas bloqueadas siguen a su dato, y el servidor lo acepta solo si es
// EXACTAMENTE ese desplazamiento, el mismo para todo el libro (proteccion_propia.go).
//
// VISTO EN ROJO sin proteccion_propia.go: todos los casos aceptados dan ErrProtegido
// (la fórmula reescrita no estaba en la hoja), y los rechazados siguen rechazados.

// datosPropiaT es «Datos» con sus celdas: A1:A5 los datos, A10 la suma, B1 y B2 fórmulas
// de la propia hoja (B2 con el nombre de la hoja y un `$`, que no exime), C1 desbloqueada
// y D1 una fórmula que cita otra hoja. extra son más campos de la hoja (styles, ranges…).
var datosPropiaT = mj{
	"A1": "1", "A2": "2", "A3": "3", "A4": "4", "A5": "5",
	"A10": "<b>=SUM(A1:A5)</b>", "B1": "=A3*2", "B2": "=Datos!A4+$A$5",
	"C1": "libre", "D1": "=Ventas!B1",
}

// libroPropiaT es «Ventas» sin proteger, «Datos» protegida con los permisos dados (C1
// desbloqueada, y con `rangos` si no es nil) y «Resumen» protegida sin permisos, que cita
// «Datos».
func libroPropiaT(datos, perms, estilos mj, rangos []any, resumen mj) mj {
	prot := mj{"hasPassword": true}
	maps.Copy(prot, perms)
	if rangos != nil {
		prot["ranges"] = rangos
	}
	if estilos == nil {
		estilos = mj{"C1": mj{"locked": false}}
	}
	return libroT(
		hojaT("Ventas", mj{"B1": "7"}, nil),
		hojaT("Datos", datos, mj{"protect": prot, "styles": estilos}),
		hojaT("Resumen", resumen, mj{"protect": mj{"hasPassword": true}}),
	)
}

var resumenPropiaT = mj{"A1": "=Datos!A10", "A2": "=Datos!A1"}

// protegidaPropiaT protege «Datos» (con los permisos y rangos dados) y «Resumen».
func protegidaPropiaT(t *testing.T, perms mj, rangos []any) *simP {
	t.Helper()
	s := nuevaSimP(t)
	nuevas := map[string]string{AmbitoHoja("Datos"): "d", AmbitoHoja("Resumen"): "r"}
	if err := s.escribir(libroPropiaT(datosPropiaT, perms, nil, rangos, resumenPropiaT), false, Proteccion{Nuevas: nuevas}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	return s
}

// con es una copia de m con los cambios (un valor "" quita la clave).
func con(m, cambios mj) mj {
	out := maps.Clone(m)
	for k, v := range cambios {
		if v == "" {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}

var (
	// Una fila insertada encima de la 3: lo de la 3 en adelante baja y las fórmulas lo siguen.
	datosFila3T = mj{
		"A1": "1", "A2": "2", "A4": "3", "A5": "4", "A6": "5",
		"A11": "<b>=SUM(A1:A6)</b>", "B1": "=A4*2", "B2": "=Datos!A5+$A$6",
		"C1": "libre", "D1": "=Ventas!B1",
	}
	resumenFila3T = mj{"A1": "=Datos!A11", "A2": "=Datos!A1"}
)

func TestFormulasPropiasSiguenALaInsercion(t *testing.T) {
	casos := []struct {
		nombre  string
		perms   mj
		datos   mj
		estilos mj
		resumen mj
		rangos  [2][]any // antes y después
	}{
		{"insertar una fila encima de la 3", mj{"insertRows": true}, datosFila3T, nil, resumenFila3T, [2][]any{}},
		{
			"borrar la fila 2",
			mj{"deleteRows": true},
			mj{"A1": "1", "A2": "3", "A3": "4", "A4": "5", "A9": "<b>=SUM(A1:A4)</b>", "B1": "=A2*2", "C1": "libre", "D1": "=Ventas!B1"},
			nil,
			mj{"A1": "=Datos!A9", "A2": "=Datos!A1"},
			[2][]any{},
		},
		{
			"borrar la fila 3, que una fórmula cita: #¡REF!",
			mj{"deleteRows": true},
			mj{
				"A1": "1", "A2": "2", "A3": "4", "A4": "5", "A9": "<b>=SUM(A1:A4)</b>",
				"B1": "=#¡REF!*2", "B2": "=Datos!A3+$A$4", "C1": "libre", "D1": "=Ventas!B1",
			},
			nil,
			mj{"A1": "=Datos!A9", "A2": "=Datos!A1"},
			[2][]any{},
		},
		{
			"insertar una columna antes de la A",
			mj{"insertCols": true},
			mj{
				"B1": "1", "B2": "2", "B3": "3", "B4": "4", "B5": "5",
				"B10": "<b>=SUM(B1:B5)</b>", "C1": "=B3*2", "C2": "=Datos!B4+$B$5",
				"D1": "libre", "E1": "=Ventas!B1",
			},
			mj{"D1": mj{"locked": false}},
			mj{"A1": "=Datos!B10", "A2": "=Datos!B1"},
			[2][]any{},
		},
		{
			"y escribir a la vez en la celda desbloqueada",
			mj{"insertRows": true},
			con(datosFila3T, mj{"C1": "otra cosa"}), nil, resumenFila3T,
			[2][]any{},
		},
		{
			"con un rango editable, que viaja con sus celdas",
			mj{"insertRows": true},
			datosFila3T, nil, resumenFila3T,
			[2][]any{{mj{"ref": "A4:A5", "users": []any{"ana"}}}, {mj{"ref": "A5:A6", "users": []any{"ana"}}}},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaPropiaT(t, c.perms, c.rangos[0])
			m := libroPropiaT(c.datos, c.perms, c.estilos, c.rangos[1], c.resumen)
			if err := s.escribir(m, false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
		})
	}
}

// Lo que no es exactamente el desplazamiento de la propia hoja sigue vetado.
func TestFormulasPropiasNoSeReescribenAMano(t *testing.T) {
	insertar := mj{"insertRows": true}
	casos := []struct {
		nombre  string
		perms   mj
		datos   mj
		estilos mj
		resumen mj
		rangos  []any
	}{
		{"sin el permiso de insertar", mj{"deleteRows": true}, datosFila3T, nil, resumenFila3T, nil},
		{"la suma sin reescribir", insertar, con(datosFila3T, mj{"A11": "<b>=SUM(A1:A5)</b>"}), nil, resumenFila3T, nil},
		{"una fórmula que no se movía, sin reescribir", insertar, con(datosFila3T, mj{"B1": "=A3*2"}), nil, resumenFila3T, nil},
		{
			"reescribir sin mover las celdas", insertar,
			con(datosPropiaT, mj{"A10": "<b>=SUM(A1:A6)</b>", "B1": "=A4*2", "B2": "=Datos!A5+$A$6"}),
			nil,
			mj{"A1": "=Datos!A10", "A2": "=Datos!A1"},
			nil,
		},
		{"reescribir con otro desplazamiento", insertar, con(datosFila3T, mj{"A11": "<b>=SUM(A1:A7)</b>"}), nil, resumenFila3T, nil},
		{"cambiar un literal bloqueado", insertar, con(datosFila3T, mj{"A1": "9"}), nil, resumenFila3T, nil},
		{"cambiar una fórmula que no cita la hoja", insertar, con(datosFila3T, mj{"D1": "=Ventas!B2"}), nil, resumenFila3T, nil},
		{"cambiar el formato de una fórmula", insertar, con(datosFila3T, mj{"A11": "=SUM(A1:A6)"}), nil, resumenFila3T, nil},
		{"escribir en la fila nueva", insertar, con(datosFila3T, mj{"A3": "nuevo"}), nil, resumenFila3T, nil},
		{"el desbloqueo se mueve sin su fila", insertar, datosFila3T, mj{"C2": mj{"locked": false}}, resumenFila3T, nil},
		{
			"dos inserciones en un guardado", insertar,
			mj{
				"A1": "1", "A2": "2", "A4": "3", "A5": "4", "A6": "5",
				"A12": "<b>=SUM(A1:A6)</b>", "B1": "=A4*2", "B2": "=Datos!A5+$A$6",
				"C1": "libre", "D1": "=Ventas!B1",
			},
			nil,
			mj{"A1": "=Datos!A12", "A2": "=Datos!A1"},
			nil,
		},
		{"la otra hoja protegida sin reescribir", insertar, datosFila3T, nil, resumenPropiaT, nil},
		{"la otra hoja con otro desplazamiento", insertar, datosFila3T, nil, mj{"A1": "=Datos!A12", "A2": "=Datos!A1"}, nil},
		{"la otra hoja moviendo lo que está antes del corte", insertar, datosFila3T, nil, mj{"A1": "=Datos!A11", "A2": "=Datos!A2"}, nil},
		{
			"un rango editable que no se mueve igual", insertar, datosFila3T, nil, resumenFila3T,
			[]any{mj{"ref": "A5:A7", "users": []any{"ana"}}},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var antesRangos []any
			if c.rangos != nil {
				antesRangos = []any{mj{"ref": "A4:A5", "users": []any{"ana"}}}
			}
			s := protegidaPropiaT(t, c.perms, antesRangos)
			antes := s.est.Modelo
			err := s.escribir(libroPropiaT(c.datos, c.perms, c.estilos, c.rangos, c.resumen), false, Proteccion{Quien: luis})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
			if s.est.Modelo != antes {
				t.Fatal("una escritura denegada cambió el estado")
			}
		})
	}
}

// Las demás hojas protegidas valen con el MISMO desplazamiento aunque no lo necesiten:
// también la que tiene concedido insertar filas (puede mover sus celdas, pero no dejar
// de seguir a las de «Datos»), y el permiso hace falta en todas las que se llaman como
// la que se mueve, como con las fórmulas que la citan (proteccion_ajenas.go).
func TestFormulasPropiasYLasDemasHojas(t *testing.T) {
	insertar := mj{"hasPassword": true, "insertRows": true}
	libro := func(datos, resumen, protResumen, gemela mj) mj {
		m := libroPropiaT(datos, mj{"insertRows": true}, nil, nil, resumen)
		hojas := m["sheets"].([]any)
		hojas[2].(mj)["protect"] = protResumen
		if gemela != nil {
			m["sheets"] = append([]any{hojaT("DATOS", mj{}, mj{"protect": gemela})}, hojas...)
		}
		return m
	}
	proteger := func(t *testing.T, protResumen, gemela mj) *simP {
		t.Helper()
		s := nuevaSimP(t)
		nuevas := map[string]string{AmbitoHoja("Datos"): "d", AmbitoHoja("Resumen"): "r"}
		if gemela != nil {
			nuevas[AmbitoHoja("DATOS")] = "g"
		}
		if err := s.escribir(libro(datosPropiaT, resumenPropiaT, protResumen, gemela), false, Proteccion{Nuevas: nuevas}); err != nil {
			t.Fatalf("proteger: %v", err)
		}
		return s
	}
	t.Run("la que puede insertar, reescrita", func(t *testing.T) {
		s := proteger(t, insertar, nil)
		if err := s.escribir(libro(datosFila3T, resumenFila3T, insertar, nil), false, Proteccion{Quien: luis}); err != nil {
			t.Fatalf("el desplazamiento se rechazó: %v", err)
		}
	})
	t.Run("la que puede insertar, sin reescribir", func(t *testing.T) {
		s := proteger(t, insertar, nil)
		if err := s.escribir(libro(datosFila3T, resumenPropiaT, insertar, nil), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("quiero ErrProtegido y es %v", err)
		}
	})
	t.Run("con una gemela que no puede insertar", func(t *testing.T) {
		gemela := mj{"hasPassword": true, "deleteRows": true}
		s := proteger(t, mj{"hasPassword": true}, gemela)
		if err := s.escribir(libro(datosFila3T, resumenFila3T, mj{"hasPassword": true}, gemela), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("quiero ErrProtegido y es %v", err)
		}
	})
	t.Run("con una gemela que también puede", func(t *testing.T) {
		s := proteger(t, mj{"hasPassword": true}, insertar)
		if err := s.escribir(libro(datosFila3T, resumenFila3T, mj{"hasPassword": true}, insertar), false, Proteccion{Quien: luis}); err != nil {
			t.Fatalf("el desplazamiento se rechazó: %v", err)
		}
	})
}

// Las tablas dinámicas de la hoja se mueven con el mismo desplazamiento que sus fórmulas.
func TestFormulasPropiasConSusDinamicas(t *testing.T) {
	libro := func(datos mj, pivots []any) mj {
		m := libroPropiaT(datos, mj{"insertRows": true}, nil, nil, resumenFila3T)
		m["sheets"].([]any)[1].(mj)["pivots"] = pivots
		return m
	}
	s := nuevaSimP(t)
	antes := libroPropiaT(datosPropiaT, mj{"insertRows": true}, nil, nil, resumenPropiaT)
	antes["sheets"].([]any)[1].(mj)["pivots"] = []any{dinamicaT("TD1", "A1:A5", "", "F1", "F1:G3")}
	nuevas := map[string]string{AmbitoHoja("Datos"): "d", AmbitoHoja("Resumen"): "r"}
	if err := s.escribir(antes, false, Proteccion{Nuevas: nuevas}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if err := s.clonar(t).escribir(libro(datosFila3T, []any{dinamicaT("TD1", "A1:A6", "", "F1", "F1:G4")}), false, Proteccion{Quien: luis}); err != nil {
		t.Fatalf("el desplazamiento con su dinámica se rechazó: %v", err)
	}
	for _, mal := range []mj{
		dinamicaT("TD1", "A1:A5", "", "F1", "F1:G3"), // no la mueve
		dinamicaT("TD1", "A1:A7", "", "F1", "F1:G4"), // la mueve con otro
	} {
		if err := s.clonar(t).escribir(libro(datosFila3T, []any{mal}), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			t.Errorf("dinámica %v: quiero ErrProtegido y es %v", mal["source"], err)
		}
	}
}

// grandePropiaT es una hoja «Datos» con n filas (A el dato, B una fórmula que lo dobla y
// C una que suma desde arriba) protegida con insertRows, sin y con una fila insertada
// encima de la `en`. `estropear` cambia la fórmula de la última fila en el modelo nuevo.
func grandePropiaT(n, en int, estropear bool) (antes, despues mj) {
	cel, mov := mj{}, mj{}
	for k := 1; k <= n; k++ {
		cel[fmt.Sprintf("A%d", k)] = fmt.Sprint(k)
		cel[fmt.Sprintf("B%d", k)] = fmt.Sprintf("=A%d*2", k)
		cel[fmt.Sprintf("C%d", k)] = fmt.Sprintf("=SUM(A$1:A%d)", k)
		f, g := k, k
		if k >= en {
			f, g = k+1, k+1
		}
		mov[fmt.Sprintf("A%d", f)] = fmt.Sprint(k)
		mov[fmt.Sprintf("B%d", f)] = fmt.Sprintf("=A%d*2", g)
		mov[fmt.Sprintf("C%d", f)] = fmt.Sprintf("=SUM(A$1:A%d)", g)
	}
	if estropear {
		mov[fmt.Sprintf("B%d", n+1)] = fmt.Sprintf("=A%d*2", n)
	}
	libro := func(c mj) mj {
		return libroT(hojaT("Datos", c, mj{"protect": mj{"hasPassword": true, "insertRows": true}}))
	}
	return libro(cel), libro(mov)
}

// Una inserción de verdad en una hoja grande se acepta reescribiendo cada fórmula unas
// pocas veces, y una que no cuadra se rechaza sin convertir la búsqueda en segundos de
// CPU: corre dentro de la transacción que bloquea el documento.
func TestProteccionPropiaGrandeAcotada(t *testing.T) {
	const n = 3000
	for _, c := range []struct {
		nombre    string
		estropear bool
	}{{"de verdad", false}, {"con la última fórmula sin reescribir", true}} {
		t.Run(c.nombre, func(t *testing.T) {
			antes, despues := grandePropiaT(n, 1000, c.estropear)
			s := nuevaSimP(t)
			if err := s.escribir(antes, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			acepta, trabajo := trabajoAjenas(t, s, despues)
			if acepta == c.estropear {
				t.Fatalf("aceptado %v; quiero %v", acepta, !c.estropear)
			}
			// Dos fórmulas por fila: una pasada completa las mira todas (medido: una sola).
			if tope := 2 * (2*n + maxCeldasAjenas); trabajo > tope {
				t.Errorf("la búsqueda miró %d fórmulas; el presupuesto es %d", trabajo, tope)
			}
			t0 := time.Now()
			err := s.escribir(despues, false, Proteccion{Quien: luis})
			if c.estropear != errors.Is(err, ErrProtegido) {
				t.Fatalf("escribir: %v", err)
			}
			if dt := time.Since(t0); !conRace && dt > 2*time.Second {
				t.Errorf("la escritura tardó %v", dt)
			}
		})
	}
}

// Modelos hechos a mano contra una hoja grande con insertar y borrar concedidos: se
// rechazan sin convertir la búsqueda en segundos de CPU (las líneas se comparan una vez
// por eje y, por candidato, de la última hacia atrás hasta la primera que no cuadra).
func TestProteccionPropiaNoSeSatura(t *testing.T) {
	const muchas = 30000
	casos := []struct {
		nombre string
		hacer  func(cel, mov mj)
	}{
		{
			// 64 fórmulas reescritas sin mover nada y muchas que no cambian.
			"reescribir en el sitio", func(cel, mov mj) {
				for k := 1; k <= 64; k++ {
					cel[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=A%d*2", k+100)
					mov[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=A%d*2", k+101)
				}
				for i := 1; i <= muchas; i++ {
					cel[fmt.Sprintf("C%d", i)] = "=A200"
					mov[fmt.Sprintf("C%d", i)] = "=A200"
				}
			},
		},
		{
			// Muchas referencias distintas en las que cambian (miles de candidatos) y una
			// columna de fórmulas copiadas hacia abajo, que cualquier desplazamiento explica.
			"muchas referencias y una columna copiada", func(cel, mov mj) {
				for k := 1; k <= 200; k++ {
					cel[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=SUM(B%d:C%d)", 7*k, 7*k+3)
					mov[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=SUM(B%d:C%d)", 7*k+1, 7*k+4)
				}
				for i := 1; i <= muchas; i++ {
					cel[fmt.Sprintf("D%d", i)] = fmt.Sprintf("=E%d", i)
					mov[fmt.Sprintf("D%d", i)] = fmt.Sprintf("=E%d", i)
				}
			},
		},
		{
			// Todo movido una fila y cada fórmula reescrita con otra.
			"todo movido con otras referencias", func(cel, mov mj) {
				for i := 1; i <= muchas; i++ {
					cel[fmt.Sprintf("A%d", i)] = fmt.Sprintf("=B%d+%d", i, i)
					mov[fmt.Sprintf("A%d", i+1)] = fmt.Sprintf("=B%d+%d", i+2, i)
				}
			},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cel, mov := mj{}, mj{}
			c.hacer(cel, mov)
			prot := mj{"hasPassword": true}
			maps.Copy(prot, todas)
			libro := func(cs mj) mj { return libroT(hojaT("Datos", cs, mj{"protect": prot})) }
			s := nuevaSimP(t)
			if err := s.escribir(libro(cel), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			acepta, trabajo := trabajoAjenas(t, s, libro(mov))
			if acepta {
				t.Fatal("se aceptó un desplazamiento que no lo es")
			}
			if tope := maxComprobacionesPropias * (2*muchas + maxCeldasAjenas); trabajo > tope {
				t.Errorf("la búsqueda miró %d fórmulas; el presupuesto es %d", trabajo, tope)
			}
			t0 := time.Now()
			if err := s.escribir(libro(mov), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
			if dt := time.Since(t0); !conRace && dt > 2*time.Second {
				t.Errorf("la escritura rechazada tardó %v", dt)
			}
		})
	}
}
