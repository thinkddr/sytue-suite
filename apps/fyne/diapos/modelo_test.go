// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func muestra(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/muestra.json")
	if err != nil {
		t.Fatalf("no se puede leer la muestra: %v", err)
	}
	return b
}

// TestLaIdaYVueltaNoPierdeNada es EL guardián del documento de la gente, y la puerta
// que hay que cruzar antes de que este paquete guarde nada nunca.
//
// La muestra ejerce TODOS los campos declarados de todos los tipos, y a propósito los
// que más fácil se pierden: un `controls: false` y un `autoplay: false` (la web solo
// persiste el valor contrario, así que un `bool` con `omitempty` se los comería), un
// `action` de cadena y otro de número —la unión de tipos—, el `anim` viejo de una sola
// entrada, un `size: custom` con decimales y un patrón con campos {n}/{total}.
//
// VISTO EN ROJO (2026-09-21, en el piloto): cambiando `Controles *bool` por
// `Controles bool` con `omitempty` —que es la forma «obvia» de escribirlo— el test
// falla con la clave `controls` ausente en la vuelta. Deshecho después.
func TestLaIdaYVueltaNoPierdeNada(t *testing.T) {
	crudo := muestra(t)
	m, err := Leer(crudo)
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if !m.Fiel(crudo) {
		vuelta, _ := json.Marshal(m)
		t.Errorf("el modelo NO reproduce su origen; con esto Guardar borraría trabajo ajeno.\n"+
			"origen:  %s\nvuelta:  %s", compactar(t, crudo), vuelta)
	}
}

// TestUnCampoQueNoConozcoTumbaLaFidelidad es el mismo guardián por el otro lado: si
// mañana la web añade un campo, `Fiel` tiene que decir que NO, y la presentación se
// abre en solo lectura en vez de perderlo al guardar.
//
// Sin esto, `Fiel` podría devolver `true` siempre y nadie se enteraría.
func TestUnCampoQueNoConozcoTumbaLaFidelidad(t *testing.T) {
	crudo := []byte(`{"slides":[{"id":"s1","boxes":[],"transicionDeMañana":"cubo"}]}`)
	m, err := Leer(crudo)
	if err != nil {
		t.Fatalf("Leer: %v", err)
	}
	if m.Fiel(crudo) {
		t.Error("un campo desconocido se ha perdido en silencio y Fiel ha dicho que sí")
	}
}

// TestLaProporcionSaleDelModelo son los números de `slideAspect`/`stageHeight` de la
// web: 16:9 → 540 de alto, 4:3 → 720, y una personalizada acotada al rango sano.
func TestLaProporcionSaleDelModelo(t *testing.T) {
	de := func(j string) *Modelo {
		t.Helper()
		m, err := Leer([]byte(j))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	casos := []struct {
		nombre string
		modelo *Modelo
		alto   float32
	}{
		{"ausente = 16:9", de(`{"slides":[]}`), 540},
		{"4:3", de(`{"slides":[],"size":{"preset":"4:3"}}`), 720},
		{"custom 2:1", de(`{"slides":[],"size":{"preset":"custom","w":2,"h":1}}`), 480},
		{"custom sin medidas = 16:9", de(`{"slides":[],"size":{"preset":"custom"}}`), 540},
		// MAX_ASPECT = 5: una proporción absurda se acota, no revienta el lienzo.
		{"custom 100:1 se acota a 5", de(`{"slides":[],"size":{"preset":"custom","w":100,"h":1}}`), 192},
	}
	for _, c := range casos {
		if got := c.modelo.AltoEscenario(); got != c.alto {
			t.Errorf("%s: alto del escenario %v, quiero %v", c.nombre, got, c.alto)
		}
	}
}

// TestElPatronVaDetrasYSustituyeSusCampos es `resolveSlideBoxes` + `masterBoxesFor`,
// con los mismos casos que el self-test de la web (slides-master.test.ts).
func TestElPatronVaDetrasYSustituyeSusCampos(t *testing.T) {
	m, err := Leer(muestra(t))
	if err != nil {
		t.Fatal(err)
	}
	cajas := m.CajasResueltas(0)
	if len(cajas) != 3 {
		t.Fatalf("la diapositiva 1 resuelve %d cajas y son 1 de patrón + 2 propias", len(cajas))
	}
	if cajas[0].ID != "m1" {
		t.Errorf("la primera caja es %q y el patrón va DETRÁS de las propias", cajas[0].ID)
	}
	if quiero := "<p>1 / 3</p>"; cajas[0].HTML != quiero {
		t.Errorf("el patrón pinta %q y {n}/{total} en la 1 de 3 son %q", cajas[0].HTML, quiero)
	}
	// La 2 lleva hideMaster: ahí el patrón no sale.
	if cajas := m.CajasResueltas(1); len(cajas) != 4 {
		t.Errorf("la diapositiva 2 tiene hideMaster y resuelve %d cajas; son sus 4 propias", len(cajas))
	}
	// Y sustituir no puede tocar el patrón guardado: la 3 tiene que decir 3.
	if cajas := m.CajasResueltas(2); !strings.Contains(cajas[0].HTML, "3 / 3") {
		t.Errorf("el patrón de la diapositiva 3 dice %q: la sustitución ha mutado el original", cajas[0].HTML)
	}
	if m.Patron.Cajas[0].HTML != "<p>{n} / {total}</p>" {
		t.Errorf("el patrón guardado quedó en %q: se ha sustituido sobre el modelo", m.Patron.Cajas[0].HTML)
	}
}

func compactar(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	c, _ := json.Marshal(v)
	return string(c)
}

// TestLasFormasDelDisenoVanConElPatron: las formas de un diseño (de un .pptx importado) se
// pintan encima del patrón en las diapositivas que lo llevan, sus líneas con ellas, el diseño
// que oculta el patrón deja solo las suyas, y el documento sigue siendo FIEL (editable):
// antes los campos `decor`, `connectors` y `hideMaster` del diseño lo dejaban en solo lectura.
func TestLasFormasDelDisenoVanConElPatron(t *testing.T) {
	crudo := []byte(`{"slides":[{"id":"s1","boxes":[],"layout":"portada"},{"id":"s2","boxes":[],"layout":"limpio"},{"id":"s3","boxes":[]}],` +
		`"master":{"boxes":[{"id":"m1","x":0,"y":90,"w":20,"h":10,"html":"<p>{n}</p>"}],"layouts":[` +
		`{"id":"portada","name":"Portada","boxes":[],"decor":[{"id":"logo","x":90,"y":0,"w":10,"h":10,"html":"<p>{total}</p>"}],` +
		`"connectors":[{"id":"l1","kind":"straight","from":{"x":5,"y":20},"to":{"x":95,"y":20}}]},` +
		`{"id":"limpio","name":"Limpio","boxes":[],"decor":[{"id":"marca","x":0,"y":0,"w":5,"h":5,"html":""}],"hideMaster":true}]}}`)
	m, err := Leer(crudo)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Fiel(crudo) {
		t.Fatal("un diseño con formas deja el documento en solo lectura")
	}
	ids := func(cs []Caja) []string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	if got := ids(m.CajasResueltas(0)); strings.Join(got, ",") != "m1,logo" {
		t.Fatalf("la 1 resuelve %v: el patrón y encima las formas del diseño", got)
	}
	if got := m.CajasResueltas(0)[1].HTML; got != "<p>3</p>" {
		t.Errorf("los campos de las formas del diseño: %q", got)
	}
	if got := ids(m.CajasResueltas(1)); strings.Join(got, ",") != "marca" {
		t.Errorf("la 2 resuelve %v: su diseño oculta el patrón", got)
	}
	if got := ids(m.CajasResueltas(2)); strings.Join(got, ",") != "m1" {
		t.Errorf("la 3, sin diseño, resuelve %v", got)
	}
	if len(m.ConectoresDiseno(0)) != 1 || len(m.ConectoresDiseno(1)) != 0 {
		t.Errorf("las líneas del diseño: %v %v", m.ConectoresDiseno(0), m.ConectoresDiseno(1))
	}
}
