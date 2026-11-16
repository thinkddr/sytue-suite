// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import (
	"encoding/json"
	"os"
	"testing"
)

// vectoresCursor son los de testdata/cursores.json, que recorre también
// `editor/state/cursores-compartidos.test.ts`: con el mismo HTML y los mismos offsets, la
// web y Fyne tienen que anclar el cursor a los MISMOS caracteres y volver a los mismos
// offsets. Si no, el cursor de una persona sale en otro sitio en la pantalla de la otra.
type vectoresCursor struct {
	Mapeo []struct {
		Nombre string  `json:"nombre"`
		HTML   string  `json:"html"`
		A      int     `json:"anchor"`
		F      int     `json:"focus"`
		Puntos *Puntos `json:"puntos"`
	} `json:"mapeo"`
	Edicion []struct {
		Nombre   string `json:"nombre"`
		HTML     string `json:"html"`
		Nuevo    string `json:"nuevo"`
		A        int    `json:"anchor"`
		F        int    `json:"focus"`
		Esperado struct {
			A int `json:"anchor"`
			F int `json:"focus"`
		} `json:"esperado"`
	} `json:"edicion"`
}

func leerVectoresCursor(t *testing.T) vectoresCursor {
	t.Helper()
	crudo, err := os.ReadFile("testdata/cursores.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresCursor
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Mapeo) < 8 || len(v.Edicion) < 4 {
		t.Fatalf("pocos vectores: %d y %d", len(v.Mapeo), len(v.Edicion))
	}
	return v
}

// escrito es la coedición de quien ha escrito `html` de una vez, como la arma la web en
// los vectores: actor «ada», un documento nuevo, a solas.
func escrito(html string) *CoEdicion {
	c := Nueva("ada", nil, "")
	c.OpsLocales(html, 0, true)
	return c
}

// TestPuntosDeOffsetsComoLaWeb: los mismos puntos que la web, y de vuelta los mismos
// offsets; lo que la web no ancla (a mitad de un emoji, más allá del final) tampoco.
func TestPuntosDeOffsetsComoLaWeb(t *testing.T) {
	for _, c := range leerVectoresCursor(t).Mapeo {
		co := escrito(c.HTML)
		largo := LongitudVisible(c.HTML)
		p, ok := co.PuntosDeOffsets(largo, Offsets{Anchor: c.A, Focus: c.F})
		if c.Puntos == nil {
			if ok {
				t.Errorf("%s: anclado a %+v y la web no ancla", c.Nombre, p)
			}
			continue
		}
		if !ok || p != *c.Puntos {
			t.Errorf("%s: puntos %+v (%v), quiero %+v", c.Nombre, p, ok, *c.Puntos)
			continue
		}
		if off, ok := co.OffsetsDePuntos(largo, p); !ok || off != (Offsets{Anchor: c.A, Focus: c.F}) {
			t.Errorf("%s: de vuelta %+v (%v), quiero %d,%d", c.Nombre, off, ok, c.A, c.F)
		}
		// Un folio que no dice lo mismo que la secuencia (se acaba de teclear) no se ancla.
		if _, ok := co.PuntosDeOffsets(largo+1, Offsets{}); ok {
			t.Errorf("%s: anclado con el folio desacompasado", c.Nombre)
		}
		if _, ok := co.OffsetsDePuntos(largo+1, p); ok {
			t.Errorf("%s: colocado con el folio desacompasado", c.Nombre)
		}
	}
}

// TestElCursorAncladoSigueASusCaracteres: tras editar, los puntos caen donde la web los
// coloca, también si el carácter anclado se ha borrado.
func TestElCursorAncladoSigueASusCaracteres(t *testing.T) {
	for _, c := range leerVectoresCursor(t).Edicion {
		co := escrito(c.HTML)
		p, ok := co.PuntosDeOffsets(LongitudVisible(c.HTML), Offsets{Anchor: c.A, Focus: c.F})
		if !ok {
			t.Fatalf("%s: no se ancla", c.Nombre)
		}
		co.OpsLocales(c.Nuevo, 100, true)
		off, ok := co.OffsetsDePuntos(LongitudVisible(c.Nuevo), p)
		if !ok || off.Anchor != c.Esperado.A || off.Focus != c.Esperado.F {
			t.Errorf("%s: %+v (%v), quiero %+v", c.Nombre, off, ok, c.Esperado)
		}
	}
}

// TestPuntoEnEIndiceDe: los casos de `rga.test.ts` («puntoEn / indiceDe»): el punto de un
// carácter ajeno que aún no ha llegado no se coloca, y el cero es el principio.
func TestPuntoEnEIndiceDe(t *testing.T) {
	a := NuevaSecuencia("a")
	b := NuevaSecuencia("b")
	aplicar(b, a.InsertarLocal(0, "hola", 1))
	ancla, ok := b.PuntoEn(2)
	if i, ok2 := b.IndiceDe(ancla); !ok || !ok2 || i != 2 {
		t.Fatalf("ancla %+v: %d", ancla, i)
	}
	// A escribe DELANTE y DETRÁS: el cursor de B se mueve con su carácter.
	aplicar(b, a.InsertarLocal(0, "XY", 10))
	aplicar(b, a.InsertarLocal(6, "!", 20))
	if i, _ := b.IndiceDe(ancla); b.Texto() != "XYhola!" || i != 4 {
		t.Errorf("tras «XYho» en %q: %d", b.Texto(), i)
	}
	// Borrar el carácter anclado deja el cursor donde estaba.
	aplicar(b, a.BorrarLocal(3, 1, 30))
	if i, _ := b.IndiceDe(ancla); b.Texto() != "XYhla!" || i != 3 {
		t.Errorf("tras borrar la «o» en %q: %d", b.Texto(), i)
	}
	if p, ok := b.PuntoEn(0); !ok || p != (Punto{}) {
		t.Errorf("principio: %+v", p)
	}
	if i, ok := b.IndiceDe(Punto{}); !ok || i != 0 {
		t.Errorf("punto cero: %d", i)
	}
	if _, ok := b.IndiceDe(Punto{A: "zz", C: 9}); ok {
		t.Error("un punto desconocido se coloca")
	}
	if _, ok := b.PuntoEn(99); ok {
		t.Error("fuera del texto se ancla")
	}
	if _, ok := b.PuntoEn(-1); ok {
		t.Error("antes del principio se ancla")
	}
}
