// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Las tablas dinámicas de una hoja protegida siguen a sus celdas al insertar o borrar
// filas o columnas, y solo con EXACTAMENTE ese desplazamiento (proteccion_dinamicas.go).
//
// VISTO EN ROJO con `pivots` en camposSiempreVetados: todos los casos aceptados dan
// ErrProtegido.

// dinamicaT es una tabla dinámica con los campos que la web guarda.
func dinamicaT(nombre, origen, hoja, esquina, rango string) mj {
	p := mj{
		"name": nombre, "source": origen, "anchor": esquina,
		"rows": []any{mj{"field": 0}}, "values": []any{mj{"field": 2, "fn": "sum"}},
	}
	if hoja != "" {
		p["sourceSheet"] = hoja
	}
	if rango != "" {
		p["range"] = rango
	}
	return p
}

// --- Las que leen de otra hoja ---

var (
	ventasDinT = mj{"A1": "Zona", "B1": "Mes", "C1": "Importe", "A2": "Norte", "B2": "ene", "C2": "10", "A3": "Sur", "B3": "feb", "C3": "20"}
	// TD2 lee de la propia «Resumen»: un desplazamiento en «Ventas» no la mueve.
	td2T = dinamicaT("TD2", "A1:A2", "", "H1", "")
)

// resumenDinT es «Ventas» (protegida con esos permisos si no son nil) y «Resumen»
// protegida con contraseña, con sus celdas y sus dos tablas dinámicas. `extra` son más
// hojas, detrás.
func resumenDinT(ventas, resumen mj, pivots []any, protVentas mj, extra ...mj) mj {
	extraV := mj{}
	if protVentas != nil {
		p := mj{"hasPassword": true}
		for k, v := range protVentas {
			p[k] = v
		}
		extraV["protect"] = p
	}
	r := mj{"protect": mj{"hasPassword": true}}
	if pivots != nil {
		r["pivots"] = pivots
	}
	hojas := append([]mj{hojaT("Ventas", ventas, extraV), hojaT("Resumen", resumen, r)}, extra...)
	return libroT(hojas...)
}

func protegidaDinT(t *testing.T, protVentas, resumen mj, extra ...mj) *simP {
	t.Helper()
	s := nuevaSimP(t)
	nuevas := map[string]string{AmbitoHoja("Resumen"): "k"}
	if protVentas != nil {
		nuevas[AmbitoHoja("Ventas")] = "v"
	}
	m := resumenDinT(ventasDinT, resumen, []any{dinamicaT("TD1", "A1:C3", "Ventas", "E1", "E1:F3"), td2T}, protVentas, extra...)
	if err := s.escribir(m, false, Proteccion{Nuevas: nuevas}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	return s
}

var resumenDinBaseT = mj{"A1": "=Ventas!C2", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"}

func TestDinamicasSiguenALaHojaDeLaQueLeen(t *testing.T) {
	casos := []struct {
		nombre     string
		protVentas mj
		ventas     mj
		resumen    mj
		pivots     []any
	}{
		{
			"insertar una fila arriba en Ventas", nil,
			mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"},
			mj{"A1": "=Ventas!C3", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"},
			[]any{dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3"), td2T},
		},
		{
			"con el permiso de insertar de Ventas",
			mj{"insertRows": true},
			mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"},
			mj{"A1": "=Ventas!C3", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"},
			[]any{dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3"), td2T},
		},
		{
			"insertar una fila dentro del origen: crece", nil,
			mj{"A1": "Zona", "B1": "Mes", "C1": "Importe", "A2": "Norte", "B2": "ene", "C2": "10", "A4": "Sur", "B4": "feb", "C4": "20"},
			resumenDinBaseT,
			[]any{dinamicaT("TD1", "A1:C4", "Ventas", "E1", "E1:F3"), td2T},
		},
		{
			"insertar una columna antes de la B", nil,
			mj{"A1": "Zona", "C1": "Mes", "D1": "Importe", "A2": "Norte", "C2": "ene", "D2": "10", "A3": "Sur", "C3": "feb", "D3": "20"},
			mj{"A1": "=Ventas!D2", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"},
			[]any{dinamicaT("TD1", "A1:D3", "Ventas", "E1", "E1:F3"), td2T},
		},
		{
			"borrar todo su origen: desaparece", nil,
			mj{},
			mj{"A1": "=#¡REF!", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"},
			[]any{td2T},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaDinT(t, c.protVentas, resumenDinBaseT)
			if err := s.escribir(resumenDinT(c.ventas, c.resumen, c.pivots, c.protVentas), false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
		})
	}
}

// Lo que no es exactamente un desplazamiento en la hoja de la que leen sigue vetado.
func TestDinamicasNoSeCambianFingiendoUnDesplazamiento(t *testing.T) {
	arriba := mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"}
	movido := mj{"A1": "=Ventas!C3", "E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"}
	sinFormula := mj{"E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"}
	td1 := dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3")
	otraFn := dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3")
	otraFn["values"] = []any{mj{"field": 2, "fn": "count"}}
	casos := []struct {
		nombre     string
		protVentas mj
		resumen    mj
		pivots     []any
		extra      []mj
	}{
		{"leer otro rango", nil, movido, []any{dinamicaT("TD1", "A1:C9", "Ventas", "E1", "E1:F3"), td2T}, nil},
		{"mover la dinámica y no la fórmula bloqueada", nil, resumenDinBaseT, []any{td1, td2T}, nil},
		// Sin la fórmula, para que lo que decida sea el permiso y no la celda.
		{"sin el permiso de insertar en Ventas", mj{"deleteRows": true}, sinFormula, []any{td1, td2T}, nil},
		{"cambiar algo más de la dinámica", nil, movido, []any{otraFn, td2T}, nil},
		{"mover también su esquina", nil, movido, []any{dinamicaT("TD1", "A2:C4", "Ventas", "E2", "E2:F4"), td2T}, nil},
		{"mover la que lee de la propia hoja", nil, movido, []any{td1, dinamicaT("TD2", "A2:A3", "", "H1", "")}, nil},
		{"añadir una", nil, movido, []any{td1, td2T, dinamicaT("TD3", "A1:C3", "Ventas", "J1", "")}, nil},
		{"quitar una que sigue teniendo origen", nil, movido, []any{td2T}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			base := resumenDinBaseT
			if c.nombre == "sin el permiso de insertar en Ventas" {
				base = sinFormula
			}
			s := protegidaDinT(t, c.protVentas, base, c.extra...)
			err := s.escribir(resumenDinT(arriba, c.resumen, c.pivots, c.protVentas, c.extra...), false, Proteccion{Quien: luis})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
}

// Con dos hojas que solo se distinguen por las mayúsculas, el desplazamiento movería las
// referencias a las dos, así que hace falta el permiso en las dos (proteccion_ajenas.go):
// una gemela «VENTAS» protegida sin el de insertar lo niega aunque «Ventas» lo conceda;
// sin proteger, no estorba.
func TestDinamicasConUnaHojaGemela(t *testing.T) {
	td1 := dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3")
	// Sin fórmulas: lo que decide es la dinámica.
	sinFormula := mj{"E1": "Zona", "F1": "Importe", "E2": "Norte", "F2": "10"}
	arriba := mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"}
	for _, c := range []struct {
		nombre string
		gemela mj
		vale   bool
	}{
		{"protegida sin el permiso", hojaT("VENTAS", mj{"A1": "x"}, mj{"protect": mj{}}), false},
		{"sin proteger", hojaT("VENTAS", mj{"A1": "x"}, mj{}), true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaDinT(t, mj{"insertRows": true}, sinFormula, c.gemela)
			err := s.escribir(resumenDinT(arriba, sinFormula, []any{td1, td2T}, mj{"insertRows": true}, c.gemela), false, Proteccion{Quien: luis})
			if c.vale && err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
			if !c.vale && !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
}

// Una referencia a OTRO libro no se mueve con la hoja local del mismo nombre: la
// dinámica sigue a «Ventas» y `[Libro.xlsx]Ventas!C2` se queda; moverla se rechaza.
func TestDinamicasNoMuevenReferenciasAOtroLibro(t *testing.T) {
	base := mj{"A1": "=Ventas!C2", "A2": "=[Libro.xlsx]Ventas!C2", "E1": "Zona"}
	arriba := mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"}
	pivots := []any{dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3"), td2T}
	for _, c := range []struct {
		a2   string
		vale bool
	}{{"=[Libro.xlsx]Ventas!C2", true}, {"=[Libro.xlsx]Ventas!C3", false}} {
		s := protegidaDinT(t, nil, base)
		err := s.escribir(resumenDinT(arriba, mj{"A1": "=Ventas!C3", "A2": c.a2, "E1": "Zona"}, pivots, nil), false, Proteccion{Quien: luis})
		if c.vale && err != nil {
			t.Errorf("con A2 = %s se rechazó: %v", c.a2, err)
		}
		if !c.vale && !errors.Is(err, ErrProtegido) {
			t.Errorf("con A2 = %s quiero ErrProtegido y es %v", c.a2, err)
		}
	}
	// Ni una dinámica que lee de otro libro: su origen no se mueve con «Ventas».
	s := nuevaSimP(t)
	externa := func(origen string) []any { return []any{dinamicaT("TD1", origen, "[Libro.xlsx]Ventas", "E1", "")} }
	if err := s.escribir(resumenDinT(ventasDinT, mj{"E1": "Zona"}, externa("A1:C3"), nil), false,
		Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if err := s.escribir(resumenDinT(arriba, mj{"E1": "Zona"}, externa("A2:C4"), nil), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Errorf("la dinámica de otro libro: quiero ErrProtegido y es %v", err)
	}
}

// Un solo desplazamiento para todo el guardado: «Resumen» mueve su dinámica como si se
// hubiera insertado una fila arriba en «Ventas», y «Otra», protegida, sigue citando
// `Ventas!C2` sin moverse, que con ese desplazamiento pasaría a C3. Y al revés: la
// fórmula movida de «Otra» y la dinámica quieta.
func TestDinamicasYFormulasConElMismoDesplazamiento(t *testing.T) {
	arriba := mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"}
	otra := func(a1 string) mj { return hojaT("Otra", mj{"A1": a1}, mj{"protect": mj{}}) }
	sinFormula := mj{"E1": "Zona"}
	for _, c := range []struct {
		nombre string
		pivots []any
		otra   string
		vale   bool
	}{
		{"las dos movidas", []any{dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3"), td2T}, "=Ventas!C3", true},
		{"la dinámica sí y la fórmula no", []any{dinamicaT("TD1", "A2:C4", "Ventas", "E1", "E1:F3"), td2T}, "=Ventas!C2", false},
		{"la fórmula sí y la dinámica no", []any{dinamicaT("TD1", "A1:C3", "Ventas", "E1", "E1:F3"), td2T}, "=Ventas!C3", false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaDinT(t, nil, sinFormula, otra("=Ventas!C2"))
			err := s.escribir(resumenDinT(arriba, sinFormula, c.pivots, nil, otra(c.otra)), false, Proteccion{Quien: luis})
			if c.vale && err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
			if !c.vale && !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
}

// --- Las de la propia hoja protegida ---

// datosDinT es la hoja «Datos» protegida con esos permisos: los datos en A1:B4 y una
// tabla dinámica de ellos en D1:E3, con lo que pinta.
func datosDinT(celdas, perms mj, pivots []any) mj {
	prot := mj{"hasPassword": true}
	for k, v := range perms {
		prot[k] = v
	}
	extra := mj{"protect": prot}
	if pivots != nil {
		extra["pivots"] = pivots
	}
	return libroT(hojaT("Datos", celdas, extra))
}

var datosT = mj{
	"A1": "Zona", "B1": "Importe", "A2": "Norte", "B2": "10", "A3": "Sur", "B3": "20", "A4": "Este", "B4": "5",
	"D1": "Zona", "E1": "Suma", "D2": "Norte", "E2": "10", "D3": "Sur", "E3": "20",
}

func protegidaPropiaDinT(t *testing.T, perms mj) *simP {
	t.Helper()
	s := nuevaSimP(t)
	if err := s.escribir(datosDinT(datosT, perms, []any{dinamicaT("TD1", "A1:B4", "", "D1", "D1:E3")}), false,
		Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	return s
}

// datosMovidosT son las celdas de datosT tras insertar `n` filas desde la fila `desde`
// (base uno).
func datosMovidosT(desde, n int) mj {
	out := mj{}
	for ref, v := range datosT {
		c, f, _ := leerCelda(ref)
		if f >= desde {
			f += n
		}
		out[columnaT(c)+itoaT(f)] = v
	}
	return out
}

func columnaT(c int) string { return string(rune('A' + c - 1)) }

func itoaT(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoaT(n/10) + string(rune('0'+n%10))
}

func TestDinamicasSiguenASuHojaProtegida(t *testing.T) {
	casos := []struct {
		nombre string
		perms  mj
		celdas mj
		pivots []any
	}{
		{"insertar una fila en medio", mj{"insertRows": true}, datosMovidosT(3, 1), []any{dinamicaT("TD1", "A1:B5", "", "D1", "D1:E4")}},
		{"insertar dos filas arriba", mj{"insertRows": true}, datosMovidosT(1, 2), []any{dinamicaT("TD1", "A3:B6", "", "D3", "D3:E5")}},
		{
			"insertar una columna antes de la B",
			mj{"insertCols": true},
			mj{
				"A1": "Zona", "C1": "Importe", "A2": "Norte", "C2": "10", "A3": "Sur", "C3": "20", "A4": "Este", "C4": "5",
				"E1": "Zona", "F1": "Suma", "E2": "Norte", "F2": "10", "E3": "Sur", "F3": "20",
			},
			[]any{dinamicaT("TD1", "A1:C4", "", "E1", "E1:F3")},
		},
		{"borrar todas sus filas: desaparece", mj{"deleteRows": true}, mj{}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaPropiaDinT(t, c.perms)
			if err := s.escribir(datosDinT(c.celdas, c.perms, c.pivots), false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
		})
	}
}

func TestDinamicasDeSuHojaNoSeCambianFingiendoUnDesplazamiento(t *testing.T) {
	movida := []any{dinamicaT("TD1", "A1:B5", "", "D1", "D1:E4")}
	renombrada := dinamicaT("TD1", "A1:B5", "", "D1", "D1:E4")
	renombrada["name"] = "Otra"
	casos := []struct {
		nombre string
		perms  mj
		celdas mj
		pivots []any
	}{
		{"sin el permiso de insertar", mj{"deleteRows": true}, datosMovidosT(3, 1), movida},
		{"mover la dinámica sin mover las celdas", mj{"insertRows": true}, datosT, movida},
		{"leer otro rango", mj{"insertRows": true}, datosMovidosT(3, 1), []any{dinamicaT("TD1", "A1:B9", "", "D1", "D1:E4")}},
		{"dejar atrás lo que ocupa", mj{"insertRows": true}, datosMovidosT(3, 1), []any{dinamicaT("TD1", "A1:B5", "", "D1", "D1:E3")}},
		{"cambiar algo más", mj{"insertRows": true}, datosMovidosT(3, 1), []any{renombrada}},
		{"quitarla sin borrar su origen", mj{"insertRows": true}, datosMovidosT(3, 1), nil},
		{"sin permisos, aunque nada más cambie", mj{}, datosT, []any{dinamicaT("TD1", "A1:B4", "", "D2", "D2:E4")}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaPropiaDinT(t, c.perms)
			err := s.escribir(datosDinT(c.celdas, c.perms, c.pivots), false, Proteccion{Quien: luis})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
	// El propietario sí puede cambiarla.
	s := protegidaPropiaDinT(t, mj{})
	if err := s.escribir(datosDinT(datosT, mj{}, []any{dinamicaT("TD1", "A1:B9", "", "D1", "")}), true, Proteccion{}); err != nil {
		t.Fatalf("el propietario: %v", err)
	}
}

// Un modelo hecho a mano con el máximo de tablas dinámicas, todas cambiadas a su aire,
// se rechaza sin convertir la búsqueda en trabajo sin fin.
func TestDinamicasHechasAManoSeRechazanPronto(t *testing.T) {
	previas, nuevas := make([]any, 0, maxDinamicas), make([]any, 0, maxDinamicas)
	celdas := mj{}
	for i := range maxDinamicas {
		f := itoaT(i*3 + 1)
		celdas["A"+f], celdas["B"+itoaT(i*3+2)] = "x"+f, "y"
		previas = append(previas, dinamicaT("T"+f, "A"+f+":B"+itoaT(i*3+9), "", "D"+f, "D"+f+":E"+itoaT(i*3+4)))
		nuevas = append(nuevas, dinamicaT("T"+f, "A"+itoaT(i*5+2)+":B"+itoaT(i*7+11), "", "D"+itoaT(i*2+3), "D1:E"+itoaT(i+9)))
	}
	s := nuevaSimP(t)
	perms := mj{"insertRows": true, "deleteRows": true, "insertCols": true, "deleteCols": true}
	if err := s.escribir(datosDinT(celdas, perms, previas), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	ini := time.Now()
	if err := s.escribir(datosDinT(celdas, perms, nuevas), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("quiero ErrProtegido y es %v", err)
	}
	// Sin -race, como la escritura rechazada de proteccion_ajenas_test.go: el detector
	// multiplica el tiempo por diez (0,35 s sin él; 4 s con él en un portátil y 11,9 en
	// el runner ARM de ci-desktop, que la daba por roja), y el tope medía al detector.
	// El pre-push prueba apps/nucleo sin -race, así que el tope sigue vigilado ahí.
	if d := time.Since(ini); !conRace && d > 2*time.Second {
		t.Errorf("rechazarlo ha costado %v", d)
	}
}

// --- Las que el servidor no sabe leer ---

// Unas dinámicas que el servidor no sabe leer no se pueden mover para comparar: pasan si
// llegan iguales y no hay desplazamiento que las pudiera mover; con uno, se rechaza. Y
// nadie sin autorización las escribe, nuevas o cambiadas (proteccion_dinamicas.go).
//
// VISTO EN ROJO sin la regla: «escribir {}», «un número que no se decodifica» y todos
// los desplazamientos con unas ilegibles se aceptaban.

// muchasDinamicasT son maxDinamicas+1 tablas que leen A1:B4 de `hoja` ("" la propia) y
// pintan en la columna Z: la web no tiene tope, el servidor no las lee.
func muchasDinamicasT(hoja string) []any {
	out := make([]any, 0, maxDinamicas+1)
	for i := range maxDinamicas + 1 {
		f := itoaT(i*4 + 1)
		out = append(out, dinamicaT("T"+f, "A1:B4", hoja, "Z"+f, ""))
	}
	return out
}

// ilegiblesT son las formas de `pivots` que el servidor no sabe leer, con lo que lleva
// cada una que pudiera moverse.
func ilegiblesT(hoja string) map[string]any {
	return map[string]any{
		"más de las que lee": muchasDinamicasT(hoja),
		"un objeto":          mj{"TD1": dinamicaT("TD1", "A1:B4", hoja, "D1", "D1:E3")},
		"un número que no se decodifica": []any{json.RawMessage(
			`{"name":"TD1","source":"A1:B4","anchor":"D1","range":"D1:E3","rows":[{"field":1e400}]}`)},
	}
}

func TestDinamicasIlegiblesNoSeEscriben(t *testing.T) {
	td1 := dinamicaT("TD1", "A1:B4", "", "D1", "D1:E3")
	roto := json.RawMessage(`{"name":"TD1","source":"A1:B4","anchor":"D1","range":"D1:E3","rows":[{"field":1e400}],"values":[{"field":2,"fn":"sum"}]}`)
	casos := []struct {
		nombre         string
		antes, despues any
	}{
		{"escribir {} donde no había nada", nil, mj{}},
		{"escribir un número que no se decodifica", nil, json.RawMessage(`1e400`)},
		{"escribir un objeto", nil, mj{"TD1": td1}},
		{"escribir más de las que lee", nil, muchasDinamicasT("")},
		{"cambiar un campo por un número que no se decodifica", []any{td1}, []any{roto}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := nuevaSimP(t)
			var antes []any
			if c.antes != nil {
				antes = c.antes.([]any)
			}
			if err := s.escribir(datosDinT(datosT, mj{}, antes), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			m := datosDinT(datosT, mj{}, nil)
			m["sheets"].([]any)[0].(mj)["pivots"] = c.despues
			if err := s.escribir(m, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
			// El propietario sí.
			if err := s.escribir(m, true, Proteccion{}); err != nil {
				t.Fatalf("el propietario: %v", err)
			}
		})
	}
}

// Sin desplazamiento, unas ilegibles que llegan iguales no estorban: el documento se
// sigue guardando. Y un `{}` que la web descarta al abrir deja de estar sin problema.
func TestDinamicasIlegiblesQueNoCambianNoEstorban(t *testing.T) {
	perms := mj{"insertRows": true}
	abierta := mj{"A9": mj{"locked": false}}
	formas := ilegiblesT("")
	formas["un objeto vacío"] = mj{}
	for nombre, pivots := range formas {
		t.Run(nombre, func(t *testing.T) {
			s := nuevaSimP(t)
			m := datosDinT(datosT, perms, nil)
			h := m["sheets"].([]any)[0].(mj)
			h["pivots"], h["styles"] = pivots, abierta
			if err := s.escribir(m, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			// Escribir en la celda abierta.
			celdas := mj{"A9": "libre"}
			for k, v := range datosT {
				celdas[k] = v
			}
			h["cells"] = celdas
			if err := s.escribir(m, false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("editar una celda abierta: %v", err)
			}
			if nombre == "un objeto vacío" {
				delete(h, "pivots") // como la guarda la web, que lo descarta al abrir
				if err := s.escribir(m, false, Proteccion{Quien: luis}); err != nil {
					t.Fatalf("guardarla sin el objeto vacío: %v", err)
				}
			}
		})
	}
}

// Con un desplazamiento en su propia hoja —que mueve sus rangos editables, o sus celdas
// bloqueadas con el permiso de insertar— no se sabe si las ilegibles tenían que moverse.
func TestDinamicasIlegiblesConDesplazamientoEnSuHoja(t *testing.T) {
	for nombre, pivots := range ilegiblesT("") {
		t.Run(nombre, func(t *testing.T) {
			for _, conRangos := range []bool{true, false} {
				perms := mj{"insertRows": true}
				if conRangos {
					perms["ranges"] = rangosT("A9:B9", "")
				}
				s := nuevaSimP(t)
				m := datosDinT(datosT, perms, nil)
				m["sheets"].([]any)[0].(mj)["pivots"] = pivots
				if err := s.escribir(m, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
					t.Fatalf("proteger: %v", err)
				}
				if conRangos {
					perms["ranges"] = rangosT("A10:B10", "")
				}
				m = datosDinT(datosMovidosT(3, 1), perms, nil)
				m["sheets"].([]any)[0].(mj)["pivots"] = pivots
				if err := s.escribir(m, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
					t.Fatalf("con rangos %v: quiero ErrProtegido y es %v", conRangos, err)
				}
			}
		})
	}
}

// Las legibles que no cambian, con los rangos movidos por una inserción, también tienen
// que quedar donde ella las deja: antes solo se miraban si cambiaban.
func TestDinamicasQuietasConLosRangosMovidos(t *testing.T) {
	perms := func(rango string) mj { return mj{"insertRows": true, "ranges": rangosT(rango, "")} }
	s := nuevaSimP(t)
	if err := s.escribir(datosDinT(datosT, perms("A9:B9"), []any{dinamicaT("TD1", "A1:B4", "", "D1", "D1:E3")}), false,
		Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	quieta := datosDinT(datosMovidosT(3, 1), perms("A10:B10"), []any{dinamicaT("TD1", "A1:B4", "", "D1", "D1:E3")})
	if err := s.clonar(t).escribir(quieta, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("la dinámica que no sigue a sus datos: quiero ErrProtegido y es %v", err)
	}
	movida := datosDinT(datosMovidosT(3, 1), perms("A10:B10"), []any{dinamicaT("TD1", "A1:B5", "", "D1", "D1:E4")})
	if err := s.clonar(t).escribir(movida, false, Proteccion{Quien: luis}); err != nil {
		t.Fatalf("la dinámica movida: %v", err)
	}
	// Una que la inserción no toca puede llegar igual.
	s = nuevaSimP(t)
	if err := s.escribir(datosDinT(datosT, perms("A9:B9"), []any{dinamicaT("TD1", "A1:B2", "", "D1", "D1:E2")}), false,
		Proteccion{Nuevas: map[string]string{AmbitoHoja("Datos"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if err := s.escribir(datosDinT(datosMovidosT(3, 1), perms("A10:B10"), []any{dinamicaT("TD1", "A1:B2", "", "D1", "D1:E2")}), false, Proteccion{Quien: luis}); err != nil {
		t.Fatalf("la que la inserción no toca: %v", err)
	}
}

// Con un desplazamiento en otra hoja tampoco: ni las de la hoja que cita la que se
// mueve, ni las de otra hoja protegida que no cambia.
func TestDinamicasIlegiblesConDesplazamientoEnOtraHoja(t *testing.T) {
	arriba := mj{"A2": "Zona", "B2": "Mes", "C2": "Importe", "A3": "Norte", "B3": "ene", "C3": "10", "A4": "Sur", "B4": "feb", "C4": "20"}
	for nombre, pivots := range ilegiblesT("Ventas") {
		t.Run(nombre, func(t *testing.T) {
			// Las de «Resumen», que cita Ventas!C2.
			s := nuevaSimP(t)
			if err := s.escribir(resumenDinT(ventasDinT, mj{"A1": "=Ventas!C2"}, nil, nil), false,
				Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			m := resumenDinT(ventasDinT, mj{"A1": "=Ventas!C2"}, nil, nil)
			m["sheets"].([]any)[1].(mj)["pivots"] = pivots
			if err := s.escribir(m, true, Proteccion{}); err != nil {
				t.Fatalf("el propietario pone las dinámicas: %v", err)
			}
			m = resumenDinT(arriba, mj{"A1": "=Ventas!C3"}, nil, nil)
			m["sheets"].([]any)[1].(mj)["pivots"] = pivots
			if err := s.escribir(m, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("en la hoja que cita: quiero ErrProtegido y es %v", err)
			}
			// Las de «Otra», protegida, que no cita nada.
			otra := func() mj { return hojaT("Otra", mj{"A1": "x"}, mj{"protect": mj{}, "pivots": pivots}) }
			s = nuevaSimP(t)
			if err := s.escribir(resumenDinT(ventasDinT, mj{"A1": "=Ventas!C2"}, nil, nil, otra()), false,
				Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			if err := s.escribir(resumenDinT(arriba, mj{"A1": "=Ventas!C3"}, nil, nil, otra()), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
				t.Fatalf("en otra hoja protegida: quiero ErrProtegido y es %v", err)
			}
		})
	}
}
