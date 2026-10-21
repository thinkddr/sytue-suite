// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func sinTraducir(c string) string { return c }

// TestLasPlantillasSonLasDeLaWeb: las tres disposiciones con las cajas de
// `layoutBoxes`, en % del lienzo.
func TestLasPlantillasSonLasDeLaWeb(t *testing.T) {
	quiere := map[string][][4]float64{
		DispTitulo:       {{10, 35, 80, 20}},
		DispTituloCuerpo: {{8, 8, 84, 16}, {8, 30, 84, 60}},
		DispDosColumnas:  {{8, 8, 84, 16}, {8, 30, 40, 60}, {52, 30, 40, 60}},
	}
	for disp, cajas := range quiere {
		d := NuevaDiapo(disp, sinTraducir)
		if len(d.Cajas) != len(cajas) {
			t.Fatalf("%s: %d cajas, la web %d", disp, len(d.Cajas), len(cajas))
		}
		for i, c := range d.Cajas {
			if got := [4]float64{c.X, c.Y, c.W, c.H}; got != cajas[i] {
				t.Errorf("%s caja %d: %v, la web %v", disp, i, got, cajas[i])
			}
		}
		if d.Cajas[0].HTML != "<h1>slides.placeholder.title</h1>" {
			t.Errorf("%s: el título es %q", disp, d.Cajas[0].HTML)
		}
	}
}

// TestLosIdentificadoresSonLosDeLaWeb: un UUID v4 como `crypto.randomUUID()`.
func TestLosIdentificadoresSonLosDeLaWeb(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if id := uuid(); !re.MatchString(id) {
		t.Errorf("%q no es un UUID v4", id)
	}
}

// TestDuplicarRemapeaLosConectores: la copia tiene ids nuevos y sus conectores apuntan
// a SUS cajas; un punto libre se queda como estaba.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin remapear, el conector de la copia
// seguía apuntando a la caja de la original y el test lo decía.
func TestDuplicarRemapeaLosConectores(t *testing.T) {
	x := 30.0
	m := &Modelo{Diapos: []Diapo{{
		ID:    "d1",
		Cajas: []Caja{{ID: "a"}, {ID: "b"}},
		Conectores: []Conector{{
			ID: "k", Trazado: "straight",
			Desde: Extremo{Caja: "a"}, Hasta: Extremo{X: &x, Y: &x},
		}},
		Notas: "hola",
	}}}
	pos, err := m.Duplicar(0)
	if err != nil || pos != 1 || len(m.Diapos) != 2 {
		t.Fatalf("duplicar: pos %d, err %v, %d diapositivas", pos, err, len(m.Diapos))
	}
	c := m.Diapos[1]
	if c.ID == "d1" || c.Cajas[0].ID == "a" || c.Notas != "hola" {
		t.Errorf("la copia tiene que llevar ids nuevos y conservar el resto: %+v", c)
	}
	if c.Conectores[0].Desde.Caja != c.Cajas[0].ID || c.Conectores[0].Hasta.X == nil {
		t.Errorf("el conector de la copia no apunta a su caja: %+v", c.Conectores[0])
	}
	if m.Diapos[0].Conectores[0].Desde.Caja != "a" {
		t.Error("duplicar ha tocado el conector de la ORIGINAL")
	}
}

// TestDuplicarNoPasaDelTope: si la copia no cabe en un guardado, no se aplica.
func TestDuplicarNoPasaDelTope(t *testing.T) {
	m := &Modelo{Diapos: []Diapo{{ID: "d", Cajas: []Caja{{ID: "a", HTML: strings.Repeat("x", MaxCaracteres/2+10)}}}}}
	if _, err := m.Duplicar(0); !errors.Is(err, ErrDemasiadoGrande) || len(m.Diapos) != 1 {
		t.Fatalf("una copia que no cabe tiene que rechazarse sin tocar nada (err %v, %d)", err, len(m.Diapos))
	}
}

// TestLasOperacionesDejanElJSONDeLaWeb: sin tema, sin fondo, sin notas y sin patrón
// oculto = sin campo; 16:9 = sin tamaño; y el texto de las cajas no se escapa.
func TestLasOperacionesDejanElJSONDeLaWeb(t *testing.T) {
	m := &Modelo{Diapos: []Diapo{{ID: "d", Cajas: []Caja{{ID: "a", HTML: "<p>a & b</p>"}}}}}
	m.PonerTema("carbon")
	m.PonerTema("inventado")
	m.PonerFondo(0, "#FED7AA")
	m.PonerFondo(0, "transparent")
	m.PonerNotas(0, "x")
	m.PonerNotas(0, "")
	m.AlternarPatron(0)
	m.AlternarPatron(0)
	m.PonerTamano(&Tamano{Preset: "4:3"})
	m.PonerTamano(&Tamano{Preset: "16:9"})
	s, err := m.Serializar()
	if err != nil {
		t.Fatal(err)
	}
	const quiere = `{"slides":[{"id":"d","boxes":[{"id":"a","x":0,"y":0,"w":0,"h":0,"html":"<p>a & b</p>"}]}]}`
	if s != quiere {
		t.Errorf("JSON distinto del de la web:\n  %s\n  %s", s, quiere)
	}
	var vuelta Modelo
	if json.Unmarshal([]byte(s), &vuelta) != nil || !vuelta.Fiel([]byte(s)) {
		t.Error("lo serializado no hace el viaje de ida y vuelta")
	}
}

// TestMoverYEliminar: reordenar lleva la diapositiva a su sitio y siempre queda una.
func TestMoverYEliminar(t *testing.T) {
	m := &Modelo{Diapos: []Diapo{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	if !m.Mover(0, 2) || m.Diapos[2].ID != "a" || m.Diapos[0].ID != "b" {
		t.Errorf("mover 0→2: %v", m.Diapos)
	}
	m.Eliminar(0)
	m.Eliminar(0)
	if m.Eliminar(0) || len(m.Diapos) != 1 {
		t.Error("tiene que quedar al menos una diapositiva")
	}
}

// TestLaDisposicionConservaTextoEImagenes: el texto entra por orden en la plantilla y
// la imagen sobrevive detrás.
func TestLaDisposicionConservaTextoEImagenes(t *testing.T) {
	m := &Modelo{Diapos: []Diapo{{ID: "d", Cajas: []Caja{
		{ID: "t", HTML: "<h1>Mío</h1>"},
		{ID: "i", Tipo: "image", Src: "data:x"},
	}}}}
	m.AplicarDisposicion(0, DispDosColumnas, sinTraducir)
	c := m.Diapos[0].Cajas
	if len(c) != 4 || c[0].HTML != "<h1>Mío</h1>" || c[3].ID != "i" {
		t.Errorf("la disposición no conserva el texto o la imagen: %+v", c)
	}
}

// TestDiapoDeBloque: los casos de `diapositivaDeBloque` de la web. La diapositiva nace con
// identificadores nuevos, conserva lo que cada caja traiga y descarta las que no tienen sitio.
func TestDiapoDeBloque(t *testing.T) {
	d, ok := DiapoDeBloque(`{"boxes":[{"id":"viejo","x":8,"y":14,"w":84,"h":22,"html":"<h1>Cierre</h1>"},` +
		`{"id":"f","x":1,"y":2,"w":3,"h":4,"kind":"shape","shape":"ellipse","fill":"#0F766E"},{"x":1,"html":"sin sitio"}],"notes":"decir gracias"}`)
	if !ok || len(d.Cajas) != 2 || d.Notas != "decir gracias" || d.ID == "" {
		t.Fatalf("bloque: %+v %v", d, ok)
	}
	if d.Cajas[0].ID == "viejo" || d.Cajas[0].ID == d.Cajas[1].ID || d.Cajas[0].HTML != "<h1>Cierre</h1>" {
		t.Errorf("las cajas llevan ids nuevos y su texto: %+v", d.Cajas)
	}
	if c := d.Cajas[1]; c.Tipo != "shape" || c.Forma != "ellipse" || c.Relleno != "#0F766E" {
		t.Errorf("la forma conserva lo suyo: %+v", c)
	}
	for _, malo := range []string{"", "[]", `{"boxes":[]}`, `{"boxes":[{"x":1}]}`, `{"notes":"solo notas"}`} {
		if _, ok := DiapoDeBloque(malo); ok {
			t.Errorf("%q no es un bloque", malo)
		}
	}
}

// TestInsertarBloqueLoPoneDetrasDeLaActiva: la diapositiva del bloque entra detrás de la
// activa y pasa a ser ella; en solo lectura no entra.
func TestInsertarBloqueLoPoneDetrasDeLaActiva(t *testing.T) {
	e, ctx, _ := montar(t, `{"slides":[{"id":"a","boxes":[]},{"id":"b","boxes":[]}]}`)
	const bloque = `{"boxes":[{"x":8,"y":14,"w":84,"h":22,"html":"<h1>Cierre</h1>"}]}`
	e.InsertarBloque(bloque)
	if len(e.m.Diapos) != 3 || e.activa != 1 || e.m.Diapos[1].Cajas[0].HTML != "<h1>Cierre</h1>" || e.m.Diapos[2].ID != "b" {
		t.Fatalf("el bloque no quedó tras la activa: activa %d, %d diapositivas", e.activa, len(e.m.Diapos))
	}
	ctx.SoloLectura = true
	e.InsertarBloque(bloque)
	if len(e.m.Diapos) != 3 {
		t.Error("en solo lectura no se inserta")
	}
}
