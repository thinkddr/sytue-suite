// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"errors"
	"testing"
)

// Las mismas reglas que `moverReferencia` de refshift.ts (test/sheets-refshift.test.ts).
func TestMoverRangoEditableComoRefshift(t *testing.T) {
	casos := []struct {
		ref   string
		d     desplazamiento
		queda string // vacío: desaparece
	}{
		{"B2:D10", desplazamiento{filas: true, desde: 1, n: 1}, "B3:D11"},
		{"B2:D10", desplazamiento{filas: true, desde: 2, n: 1}, "B3:D11"}, // en su primera fila: baja entero
		{"B2:D10", desplazamiento{filas: true, desde: 5, n: 3}, "B2:D13"}, // dentro: crece
		{"B2:D10", desplazamiento{filas: true, desde: 11, n: 1}, "B2:D10"},
		{"B2:D10", desplazamiento{filas: true, desde: 1, n: -1}, "B1:D9"},
		{"B2:D10", desplazamiento{filas: true, desde: 1, n: -3}, "B1:D7"}, // se le borra el principio: encoge
		{"B2:D10", desplazamiento{filas: true, desde: 8, n: -5}, "B2:D7"}, // y el final
		{"B2:D10", desplazamiento{filas: true, desde: 2, n: -9}, ""},      // entero: desaparece
		{"B5", desplazamiento{filas: true, desde: 5, n: -1}, ""},
		{"B2:D10", desplazamiento{filas: false, desde: 1, n: 1}, "C2:E10"},
		{"B2:D10", desplazamiento{filas: false, desde: 3, n: -1}, "B2:C10"},
		{"B:D", desplazamiento{filas: true, desde: 1, n: 4}, "B:D"}, // columnas enteras: las filas no las mueven
		{"3:5", desplazamiento{filas: true, desde: 1, n: 1}, "4:6"},
		{"3:5", desplazamiento{filas: false, desde: 1, n: 1}, "3:5"},
		{"B1048576", desplazamiento{filas: true, desde: 1, n: 1}, ""}, // se sale de la rejilla
	}
	for _, c := range casos {
		r, ok := leerRangoEditable(c.ref)
		if !ok {
			t.Fatalf("%s no se lee", c.ref)
		}
		got, vive := c.d.moverRango(r)
		if c.queda == "" {
			if vive {
				t.Errorf("%s con %+v debería desaparecer y queda %+v", c.ref, c.d, got)
			}
			continue
		}
		want, _ := leerRangoEditable(c.queda)
		if !vive || got != want {
			t.Errorf("%s con %+v: %+v (vive %v), quiero %s", c.ref, c.d, got, vive, c.queda)
		}
	}
}

// rangosT son dos rangos: B2:B3 de ana y A1:A2 del grupo «contabilidad».
func rangosT(ana, conta string) []any {
	out := []any{}
	if ana != "" {
		out = append(out, mj{"ref": ana, "title": "Ventas", "users": []any{"ana"}})
	}
	if conta != "" {
		out = append(out, mj{"ref": conta, "groups": []any{"contabilidad"}})
	}
	return out
}

// hojaConRangosT es la hoja «H» con sus celdas y la protección con los permisos dados.
func hojaConRangosT(celdas, perms mj, rangos []any, estilos mj) mj {
	prot := mj{"hasPassword": true, "ranges": rangos}
	for k, v := range perms {
		prot[k] = v
	}
	extra := mj{"protect": prot}
	if estilos != nil {
		extra["styles"] = estilos
	}
	return libroT(hojaT("H", celdas, extra))
}

// protegidaDesplazableT protege «H» con contraseña, los permisos dados y los dos rangos.
// C1 es una celda bloqueada con contenido, para que fingir un desplazamiento se note.
func protegidaDesplazableT(t *testing.T, perms mj) *simP {
	t.Helper()
	s := nuevaSimP(t)
	m := hojaConRangosT(mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "secreto", "C4": "pie"}, perms, rangosT("B2:B3", "A1:A2"), nil)
	if err := s.escribir(m, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("H"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	return s
}

var (
	ana   = Subject{Sub: "ana"}
	luis  = Subject{Sub: "luis"}
	todas = mj{"insertRows": true, "deleteRows": true, "insertCols": true, "deleteCols": true}
)

func TestRangosEditablesViajanConSusCeldas(t *testing.T) {
	casos := []struct {
		nombre string
		perms  mj
		celdas mj
		rangos []any
	}{
		{
			"insertar una fila arriba",
			mj{"insertRows": true},
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			rangosT("B3:B4", "A2:A3"),
		},
		{
			"insertar dentro de un rango: crece",
			mj{"insertRows": true},
			mj{"A1": "1", "A2": "2", "B2": "3", "B4": "4", "C1": "secreto", "C5": "pie"},
			rangosT("B2:B4", "A1:A2"),
		},
		{
			"insertar dos columnas a la izquierda",
			mj{"insertCols": true},
			mj{"C1": "1", "C2": "2", "D2": "3", "D3": "4", "E1": "secreto", "E4": "pie"},
			rangosT("D2:D3", "C1:C2"),
		},
		{
			"borrar la primera fila: encogen",
			mj{"deleteRows": true},
			mj{"A1": "2", "B1": "3", "B2": "4", "C3": "pie"},
			rangosT("B1:B2", "A1"),
		},
		{
			"borrar las filas de un rango: desaparece",
			mj{"deleteRows": true},
			mj{"A1": "1", "C1": "secreto", "C2": "pie"},
			rangosT("", "A1"),
		},
		{
			"borrar la columna A: el rango del grupo desaparece",
			mj{"deleteCols": true},
			mj{"A2": "3", "A3": "4", "B1": "secreto", "B4": "pie"},
			rangosT("A2:A3", ""),
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaDesplazableT(t, c.perms)
			// Cualquiera con el permiso: no hace falta tener un rango para moverlos.
			if err := s.escribir(hojaConRangosT(c.celdas, c.perms, c.rangos, nil), false, Proteccion{Quien: luis}); err != nil {
				t.Fatalf("el desplazamiento se rechazó: %v", err)
			}
		})
	}
}

func TestRangosDesplazadosSiguenConcediendoSoloLoSuyo(t *testing.T) {
	s := protegidaDesplazableT(t, mj{"insertRows": true})
	celdas := mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"}
	if err := s.escribir(hojaConRangosT(celdas, mj{"insertRows": true}, rangosT("B3:B4", "A2:A3"), nil), false, Proteccion{Quien: ana}); err != nil {
		t.Fatalf("insertar: %v", err)
	}
	escribe := func(ref string) error {
		c := mj{}
		for k, v := range celdas {
			c[k] = v
		}
		c[ref] = "nuevo"
		return s.clonar(t).escribir(hojaConRangosT(c, mj{"insertRows": true}, rangosT("B3:B4", "A2:A3"), nil), false, Proteccion{Quien: ana})
	}
	if err := escribe("B4"); err != nil {
		t.Fatalf("ana en la última fila de su rango movido: %v", err)
	}
	if err := escribe("B2"); !errors.Is(err, ErrProtegido) {
		t.Fatalf("ana fuera de su rango movido: %v", err)
	}
}

func TestRangosNoSeEstiranFingiendoUnDesplazamiento(t *testing.T) {
	original := mj{"A1": "1", "A2": "2", "B2": "3", "B3": "4", "C1": "secreto", "C4": "pie"}
	casos := []struct {
		nombre string
		perms  mj
		celdas mj
		rangos []any
		quien  Subject
	}{
		// Sin mover las celdas, ana «inserta» una fila dentro de su rango para que
		// alcance C4… o una fila arriba para llegar a B4.
		{"crecer sin mover las celdas", todas, original, rangosT("B2:B4", "A1:A2"), ana},
		{"bajar sin mover las celdas", todas, original, rangosT("B3:B4", "A2:A3"), ana},
		{
			"rangos que no son los movidos", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			rangosT("B3:B5", "A2:A3"), ana,
		},
		{
			"sin el permiso de insertar",
			mj{"deleteRows": true, "insertCols": true},
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			rangosT("B3:B4", "A2:A3"), ana,
		},
		{
			"cambiar a quién se concede", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			[]any{mj{"ref": "B3:B4", "title": "Ventas", "users": []any{"ana", "luis"}}, mj{"ref": "A2:A3", "groups": []any{"contabilidad"}}},
			luis,
		},
		{
			"quitar un rango que sigue vivo", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			rangosT("B3:B4", ""), luis,
		},
		{
			"cambiar el orden", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			[]any{mj{"ref": "A2:A3", "groups": []any{"contabilidad"}}, mj{"ref": "B3:B4", "title": "Ventas", "users": []any{"ana"}}},
			luis,
		},
		{
			"desplazar y tocar una celda bloqueada", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "otro", "C5": "pie"},
			rangosT("B3:B4", "A2:A3"), ana,
		},
		{
			"desplazar y vaciar una celda bloqueada", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C5": "pie"},
			rangosT("B3:B4", "A2:A3"), ana,
		},
		{
			"estirar un rango a toda la hoja", todas,
			mj{"A2": "1", "A3": "2", "B3": "3", "B4": "4", "C2": "secreto", "C5": "pie"},
			[]any{mj{"ref": "A1:Z99", "title": "Ventas", "users": []any{"ana"}}, mj{"ref": "A2:A3", "groups": []any{"contabilidad"}}},
			ana,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := protegidaDesplazableT(t, todas)
			if c.perms != nil {
				s = protegidaDesplazableT(t, c.perms)
			}
			antes := s.est.Modelo
			err := s.escribir(hojaConRangosT(c.celdas, c.perms, c.rangos, nil), false, Proteccion{Quien: c.quien})
			if !errors.Is(err, ErrProtegido) {
				t.Fatalf("esperaba ErrProtegido, salió %v", err)
			}
			if s.est.Modelo != antes {
				t.Fatal("una escritura denegada cambió el estado")
			}
		})
	}
}

// El bloqueo viaja con su celda: un desplazamiento no deja una celda desbloqueada atrás.
func TestRangosDesplazadosConElBloqueoDeSusCeldas(t *testing.T) {
	s := nuevaSimP(t)
	perms := mj{"insertRows": true}
	m := hojaConRangosT(mj{"A1": "1", "B2": "3"}, perms, rangosT("B2", ""), mj{"A1": mj{"locked": false}})
	if err := s.escribir(m, false, Proteccion{Nuevas: map[string]string{AmbitoHoja("H"): "k"}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	// Bien: A1 (desbloqueada) baja a A2 con su estilo.
	bien := hojaConRangosT(mj{"A2": "1", "B3": "3"}, perms, rangosT("B3", ""), mj{"A2": mj{"locked": false}})
	if err := s.clonar(t).escribir(bien, false, Proteccion{Quien: ana}); err != nil {
		t.Fatalf("insertar arriba: %v", err)
	}
	// Mal: el estilo se queda en A1 mientras todo lo demás baja.
	mal := hojaConRangosT(mj{"A2": "1", "B3": "3"}, perms, rangosT("B3", ""), mj{"A1": mj{"locked": false}})
	if err := s.clonar(t).escribir(mal, false, Proteccion{Quien: ana}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("el desbloqueo se quedó atrás: %v", err)
	}
}

// Ana puede escribir en su rango en el mismo guardado en que lo mueve, también en la
// fila recién insertada dentro de él; y el propietario sigue pudiendo cambiarlo todo.
func TestRangosDesplazadosConEdicionYPropietario(t *testing.T) {
	s := protegidaDesplazableT(t, mj{"insertRows": true})
	m := hojaConRangosT(mj{"A1": "1", "A2": "2", "B2": "tres", "B3": "en la nueva", "B4": "4", "C1": "secreto", "C5": "pie"},
		mj{"insertRows": true}, rangosT("B2:B4", "A1:A2"), nil)
	if err := s.clonar(t).escribir(m, false, Proteccion{Quien: ana}); err != nil {
		t.Fatalf("ana insertando y escribiendo en su rango: %v", err)
	}
	if err := s.clonar(t).escribir(m, false, Proteccion{Quien: luis}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("luis escribiendo en el rango de ana: %v", err)
	}
	dueno := hojaConRangosT(mj{"A1": "1"}, mj{"insertRows": true}, rangosT("Z1", ""), nil)
	if err := s.clonar(t).escribir(dueno, true, Proteccion{}); err != nil {
		t.Fatalf("propietario: %v", err)
	}
}

// Dos claves para la misma celda no se leen: cuál se compararía dependería del orden
// del mapa, y el editor podría enseñar la otra.
func TestLineasRechazaClavesDuplicadas(t *testing.T) {
	// La clave con espacio y en minúsculas es justo el caso: otra forma de escribir C4.
	if _, ok := lineas(map[string]string{"C4": "pie", " c4": ""}, nil, true); ok { //nolint:gocritic // mapKey: el espacio es a propósito
		t.Fatal("celdas duplicadas aceptadas")
	}
	if _, ok := lineas(nil, map[string]estilo{"A1": {"locked": false}, "a1": {}}, false); ok {
		t.Fatal("estilos duplicados aceptados")
	}
	if _, ok := lineas(map[string]string{"A1": "1"}, map[string]estilo{"A1": {"locked": false}}, true); !ok {
		t.Fatal("una celda con su estilo es una sola posición")
	}
}
