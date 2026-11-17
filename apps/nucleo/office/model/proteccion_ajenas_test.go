// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"errors"
	"fmt"
	"testing"
	"time"
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

// trabajoAjenas repite lo que hace transicion con las hojas protegidas para luis y
// devuelve si se acepta el modelo y cuánto trabajo costó buscar el desplazamiento.
func trabajoAjenas(t *testing.T, s *simP, modelo mj) (bool, int) {
	t.Helper()
	prev, ok1 := leerLibro(jsonString(s.est.Modelo))
	nuevo, ok2 := leerLibro(valT(t, modelo))
	if !ok1 || !ok2 || prev == nil || nuevo == nil {
		t.Fatal("modelo ilegible")
	}
	aj := &ajenasLibro{prev: prev, nuevo: nuevo}
	nuevoA := nuevo.ambitos()
	for ambito, pp := range prev.ambitos() {
		if contenidoProtegido(ambito, prev, nuevo, pp, nuevoA[ambito], luis, aj) != nil {
			return false, aj.trabajo
		}
	}
	return aj.resolver() == "", aj.trabajo
}

// saturacionT es la hoja protegida con la que se saturaba la búsqueda: 64 fórmulas
// bloqueadas que citan «Ventas» y «Presupuesto» y `muchas` bloqueadas `=Ventas!B200`.
// Quien escribe sin permiso manda las 64 citando una fila más abajo de «Ventas»: miles
// de candidatos, y cada uno que pasaba por las 64 reescribía la hoja entera.
func saturacionT(muchas int, protResumen mj) (antes, despues mj) {
	cel, mov := mj{}, mj{}
	for k := 1; k <= 64; k++ {
		cel[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=Ventas!B%d-Presupuesto!B%d", k+100, k)
		mov[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=Ventas!B%d-Presupuesto!B%d", k+101, k)
	}
	for i := 1; i <= muchas; i++ {
		cel[fmt.Sprintf("C%d", i)] = "=Ventas!B200"
		mov[fmt.Sprintf("C%d", i)] = "=Ventas!B200"
	}
	libro := func(c mj) mj {
		return libroT(hojaT("Ventas", mj{}, nil), hojaT("Presupuesto", mj{}, nil), hojaT("Resumen", c, mj{"protect": protResumen}))
	}
	return libro(cel), libro(mov)
}

// La búsqueda del desplazamiento corre dentro de la transacción que bloquea el documento:
// un modelo hecho a mano no puede convertirla en segundos de CPU por escritura (eran ~11 s
// con 30 000 celdas). El presupuesto es de trabajo, que no depende de la máquina: unas
// pocas pasadas por la hoja; y el tiempo, con holgura para una máquina cargada.
func TestProteccionAjenasNoSeSatura(t *testing.T) {
	const muchas = 30000
	for _, c := range []struct {
		nombre string
		prot   mj
	}{
		{"sin permisos estructurales", mj{"hasPassword": true}},
		// Con ellos no se descarta nada sin reescribir: lo acota el tope de comprobaciones.
		{"con insertar filas concedido", mj{"hasPassword": true, "insertRows": true}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			antes, despues := saturacionT(muchas, c.prot)
			s := nuevaSimP(t)
			if err := s.escribir(antes, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			acepta, trabajo := trabajoAjenas(t, s, despues)
			if acepta {
				t.Fatal("se aceptó un desplazamiento que no explica las 30 000 celdas")
			}
			if tope := 2 * (muchas + 64); trabajo > tope {
				t.Errorf("la búsqueda leyó o reescribió %d celdas; el presupuesto es %d", trabajo, tope)
			}
			t0 := time.Now()
			err := s.escribir(despues, false, Proteccion{Quien: luis})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
			if dt := time.Since(t0); !conRace && dt > 2*time.Second {
				t.Errorf("la escritura rechazada tardó %v", dt)
			}
		})
	}
}

// Una inserción de verdad en una hoja grande se sigue aceptando, y sin rehacer la hoja
// más de una vez.
func TestProteccionAjenasGrandeSeAcepta(t *testing.T) {
	const n = 3000
	ventas, ventasMov, cel, mov := mj{}, mj{}, mj{}, mj{}
	for k := 1; k <= n; k++ {
		ventas[fmt.Sprintf("B%d", k)] = fmt.Sprint(k)
		cel[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=Ventas!B%d*2", k)
		f, g := k, k
		if k >= 1000 { // una fila insertada en la 1000
			f, g = k+1, k+1
		}
		ventasMov[fmt.Sprintf("B%d", f)] = fmt.Sprint(k)
		mov[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=Ventas!B%d*2", g)
	}
	libro := func(v, c mj) mj {
		return libroT(hojaT("Ventas", v, nil), hojaT("Resumen", c, mj{"protect": mj{"hasPassword": true}}))
	}
	s := nuevaSimP(t)
	if err := s.escribir(libro(ventas, cel), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	acepta, trabajo := trabajoAjenas(t, s, libro(ventasMov, mov))
	if !acepta {
		t.Fatal("se rechazó una inserción de verdad")
	}
	if tope := 3 * n; trabajo > tope {
		t.Errorf("la búsqueda leyó o reescribió %d celdas; el presupuesto es %d", trabajo, tope)
	}
	if err := s.escribir(libro(ventasMov, mov), false, Proteccion{Quien: luis}); err != nil {
		t.Fatalf("se rechazó una inserción de verdad: %v", err)
	}
}

// Con muchas referencias distintas en las celdas que cambian salen más de diez mil
// candidatos; la búsqueda no los rechaza por ser muchos: los filtra reescribiendo cada
// celda una vez por efecto distinto (unos cientos de reescrituras).
func TestProteccionAjenasMuchasReferenciasSeAceptan(t *testing.T) {
	cel, mov := mj{}, mj{}
	for k := 1; k <= 200; k++ {
		cel[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=SUM(Ventas!B%d:C%d)", 7*k, 7*k+3)
		mov[fmt.Sprintf("A%d", k)] = fmt.Sprintf("=SUM(Ventas!B%d:C%d)", 7*k+1, 7*k+4) // una fila arriba del todo
	}
	libro := func(c mj) mj {
		return libroT(hojaT("Ventas", mj{}, nil), hojaT("Resumen", c, mj{"protect": mj{"hasPassword": true}}))
	}
	s := nuevaSimP(t)
	if err := s.escribir(libro(cel), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if err := s.escribir(libro(mov), false, Proteccion{Quien: luis}); err != nil {
		t.Fatalf("se rechazó una inserción de verdad: %v", err)
	}
}

func BenchmarkProteccionAjenasSaturada(b *testing.B) {
	t := &testing.T{}
	antes, despues := saturacionT(30000, mj{"hasPassword": true})
	s := nuevaSimP(t)
	if err := s.escribir(antes, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if err := s.clonar(t).escribir(despues, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			b.Fatal(err)
		}
	}
}

// Una fórmula sigue a todas las hojas que se llaman igual sin distinguir mayúsculas, así
// que el permiso de la protegida no se salta poniéndole delante una gemela sin proteger.
// Una gemela ya no se puede crear (hojas_gemelas.go), pero un libro de antes puede
// traerla: aquí la tiene desde que se protegió.
func TestProteccionAjenasGemelaEnMayusculas(t *testing.T) {
	soloBorrar := mj{"hasPassword": true, "deleteRows": true} // insertar filas, no
	libro := func(protVentas mj, gemela bool, a1 string) mj {
		hojas := []mj{
			hojaT("Ventas", mj{"B4": "10"}, mj{"protect": protVentas}),
			hojaT("Resumen", mj{"A1": a1}, mj{"protect": mj{"hasPassword": true}}),
		}
		if gemela {
			hojas = append([]mj{hojaT("VENTAS", mj{}, nil)}, hojas...)
		}
		return libroT(hojas...)
	}
	proteger := func(t *testing.T, protVentas mj, gemela bool) *simP {
		t.Helper()
		s := nuevaSimP(t)
		nuevas := map[string]string{AmbitoHoja("Ventas"): "v", AmbitoHoja("Resumen"): "k"}
		if err := s.escribir(libro(protVentas, gemela, "=Ventas!B4"), false, Proteccion{Nuevas: nuevas}); err != nil {
			t.Fatalf("proteger: %v", err)
		}
		return s
	}
	t.Run("en dos guardados", func(t *testing.T) {
		s := proteger(t, soloBorrar, true)
		if err := s.escribir(libro(soloBorrar, true, "=Ventas!B4"), false, Proteccion{Quien: luis}); err != nil {
			t.Fatalf("guardar el libro con su gemela sin proteger: %v", err)
		}
		if err := s.escribir(libro(soloBorrar, true, "=Ventas!B5"), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("quiero ErrProtegido y es %v", err)
		}
	})
	t.Run("en uno", func(t *testing.T) {
		s := proteger(t, soloBorrar, false)
		if err := s.escribir(libro(soloBorrar, true, "=Ventas!B5"), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
			t.Fatalf("quiero ErrProtegido y es %v", err)
		}
	})
	t.Run("con el permiso en todas, vale", func(t *testing.T) {
		insertar := mj{"hasPassword": true, "insertRows": true}
		s := proteger(t, insertar, true)
		if err := s.escribir(libro(insertar, true, "=Ventas!B4"), false, Proteccion{Quien: luis}); err != nil {
			t.Fatalf("guardar el libro con su gemela sin proteger: %v", err)
		}
		if err := s.escribir(libro(insertar, true, "=Ventas!B5"), false, Proteccion{Quien: luis}); err != nil {
			t.Fatalf("el desplazamiento se rechazó: %v", err)
		}
	})
}

// dosResumenesT: «Ventas» sin proteger y «Resumen» y «Resumen2» protegidas con A1 la
// fórmula dada (bloqueada).
func dosResumenesT(a1, b1 string) mj {
	prot := mj{"protect": mj{"hasPassword": true}}
	return libroT(
		hojaT("Ventas", mj{"B4": "10"}, nil),
		hojaT("Resumen", mj{"A1": a1}, prot),
		hojaT("Resumen2", mj{"A1": b1}, prot),
	)
}

// El desplazamiento es UNO para todas las hojas protegidas: cada una no puede alegar el
// suyo, ni en el mismo guardado ni en guardados distintos (la que no cambia tiene que
// seguir valiendo con el que se alega).
func TestProteccionAjenasUnMismoDesplazamiento(t *testing.T) {
	casos := []struct {
		nombre           string
		antes1, antes2   string
		despues1, despu2 string
		vale             bool
	}{
		{"las dos siguen a la misma fila", "=Ventas!B4", "=Ventas!B4", "=Ventas!B5", "=Ventas!B5", true},
		{"cada una a la suya", "=Ventas!B10", "=Ventas!B4", "=Ventas!B11", "=Ventas!B5", true},
		{"la de antes del corte se queda", "=Ventas!B10", "=Ventas!B2", "=Ventas!B11", "=Ventas!B2", true},
		{"una a B5 y otra a B9", "=Ventas!B4", "=Ventas!B4", "=Ventas!B5", "=Ventas!B9", false},
		{"una se mueve y la otra no", "=Ventas!B4", "=Ventas!B4", "=Ventas!B5", "=Ventas!B4", false},
		{"la otra cita una fila que también se movería", "=Ventas!B4", "=Ventas!B10", "=Ventas!B5", "=Ventas!B10", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := nuevaSimP(t)
			nuevas := map[string]string{AmbitoHoja("Resumen"): "k", AmbitoHoja("Resumen2"): "k"}
			if err := s.escribir(dosResumenesT(c.antes1, c.antes2), false, Proteccion{Nuevas: nuevas}); err != nil {
				t.Fatalf("proteger: %v", err)
			}
			err := s.escribir(dosResumenesT(c.despues1, c.despu2), false, Proteccion{Quien: luis})
			if c.vale && err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
			if !c.vale && !errors.Is(err, ErrProtegido) {
				t.Fatalf("quiero ErrProtegido y es %v", err)
			}
		})
	}
	// En dos guardados: primero una, luego la otra con otro desplazamiento.
	s := nuevaSimP(t)
	nuevas := map[string]string{AmbitoHoja("Resumen"): "k", AmbitoHoja("Resumen2"): "k"}
	if err := s.escribir(dosResumenesT("=Ventas!B4", "=Ventas!B4"), false, Proteccion{Nuevas: nuevas}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	if err := s.escribir(dosResumenesT("=Ventas!B5", "=Ventas!B4"), false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("la primera, sola: quiero ErrProtegido y es %v", err)
	}
}

// Una referencia a otro libro no se mueve nunca: no se acepta como desplazamiento.
func TestProteccionAjenasOtroLibro(t *testing.T) {
	libro := func(a1 string) mj {
		return libroT(hojaT("Ventas", mj{"B4": "10"}, nil), hojaT("Resumen", mj{"A1": a1}, mj{"protect": mj{"hasPassword": true}}))
	}
	for _, c := range []struct {
		antes, despues string
		vale           bool
	}{
		{"=[Libro.xlsx]Ventas!B4", "=[Libro.xlsx]Ventas!B5", false},
		{"='[Libro.xlsx]Ventas'!B4", "='[Libro.xlsx]Ventas'!B5", false},
		{"=[Libro.xlsx]Ventas!B4+Ventas!B4", "=[Libro.xlsx]Ventas!B5+Ventas!B5", false},
		{"=[Libro.xlsx]Ventas!B4+Ventas!B4", "=[Libro.xlsx]Ventas!B4+Ventas!B5", true},
	} {
		s := nuevaSimP(t)
		if err := s.escribir(libro(c.antes), false, Proteccion{Nuevas: map[string]string{AmbitoHoja("Resumen"): "k"}}); err != nil {
			t.Fatalf("proteger: %v", err)
		}
		err := s.escribir(libro(c.despues), false, Proteccion{Quien: luis})
		if c.vale && err != nil {
			t.Errorf("%s → %s: se rechazó: %v", c.antes, c.despues, err)
		}
		if !c.vale && !errors.Is(err, ErrProtegido) {
			t.Errorf("%s → %s: quiero ErrProtegido y es %v", c.antes, c.despues, err)
		}
	}
}
