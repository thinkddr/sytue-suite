// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// TestUnLibroGuardadoPorLaWebVaYVuelveEditable es la ida y vuelta con un libro TAL COMO LO
// GUARDA LA WEB (`testdata/libro_web.json`, que `libroFyne.test.ts` de la web reconstruye
// con sus propias funciones y vigila): hojas con `id`, una fila y una columna insertadas
// —`rowIds`/`colIds` con sus bases—, formatos, estilos, combinadas, anchos, inmovilizado y
// nombres. Fyne lo abre EDITABLE desde el servicio (no en solo lectura, que es lo que
// pasaba con el `id` de hoja antes de PEND-CALC-11N), sin escribir nada sale idéntico, y
// tras escribir una celda manda al servicio un parche con solo esa celda, con la hoja por
// su `id`, y el libro es lo de antes más esa celda. Lo guardado se deja en
// `testdata/libro_web_desde_fyne.json`, que la web vuelve a leer (las mismas hojas, las
// mismas anclas de filas y columnas, nada perdido). Con SYTUE_ACTUALIZAR=1 lo reescribe.
func TestUnLibroGuardadoPorLaWebVaYVuelveEditable(t *testing.T) {
	web, err := os.ReadFile(filepath.Join("testdata", "libro_web.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &servicioHojas{rol: "editor", lamport: 4, hoja: string(web)}
	a := appDePrueba(t, s)
	a.AbrirID("h1", "")
	ab := a.abierto
	if ab.ctx.Estado != editor.Listo || ab.ctx.SoloLectura || ab.lz.Rejilla.SoloLectura {
		t.Fatalf("el libro de la web se abre con estado %v y solo lectura %v/%v",
			ab.ctx.Estado, ab.ctx.SoloLectura, ab.lz.Rejilla.SoloLectura)
	}
	if ids := ab.lz.Libro.IDsHojas(); len(ids) != 2 || ids[0] != "6f1c2a9e-0b7d-4c3e-9a51-2d8e4f7b1c60" {
		t.Errorf("ids de las hojas: %v", ids)
	}
	if !igual(t, string(web), ab.lz.Serializar()) {
		t.Fatal("sin escribir nada, el libro no sale como entró")
	}

	// B3: en la fila Y la columna insertadas en la web, para que se vea que sus anclas
	// siguen siendo las mismas.
	ab.lz.Rejilla.Seleccionar(1, 2, false)
	ab.lz.Rejilla.Escribir("Marzo")
	ab.auto.Parar()
	if err := a.guardar(ab); err != nil {
		t.Fatal(err)
	}
	// Lo que sale al servicio es un PARCHE sobre el `doc:sheet` de la web, con la hoja por su
	// `id` (la fusión de la co-edición de la web, state/fusion.ts): solo la celda escrita.
	s.mu.Lock()
	ops := append([]string(nil), s.ops...)
	s.mu.Unlock()
	if len(ops) != 1 {
		t.Fatalf("se guarda en %d envíos: %v", len(ops), ops)
	}
	var envio struct {
		Ops []struct {
			Key     string `json:"key"`
			Kind    string `json:"kind"`
			Cambios []struct {
				Path  []any `json:"path"`
				Value any   `json:"value"`
			} `json:"cambios"`
		} `json:"ops"`
	}
	if err := json.Unmarshal([]byte(ops[0]), &envio); err != nil || len(envio.Ops) != 1 {
		t.Fatalf("envío: %v %s", err, ops[0])
	}
	op := envio.Ops[0]
	camino, _ := json.Marshal(op.Cambios[0].Path)
	if op.Key != "doc:sheet" || op.Kind != "patch" || len(op.Cambios) != 1 ||
		string(camino) != `["sheets",{"id":"6f1c2a9e-0b7d-4c3e-9a51-2d8e4f7b1c60"},"cells","B3"]` || op.Cambios[0].Value != "Marzo" {
		t.Fatalf("el parche no es solo B3 de «Ventas» por su id: %s", ops[0])
	}

	// Y el libro entero, que es lo que queda en el servicio: el de la web más esa celda.
	guardado := ab.lz.Serializar()
	var antes map[string]any
	if err := json.Unmarshal(web, &antes); err != nil {
		t.Fatal(err)
	}
	antes["sheets"].([]any)[0].(map[string]any)["cells"].(map[string]any)["B3"] = "Marzo"
	esperado, _ := json.Marshal(antes)
	if !igual(t, string(esperado), guardado) {
		t.Fatal("lo guardado desde Fyne no es el libro de la web más la celda escrita")
	}

	// Para la web: el mismo JSON, con sangría para que el diff se lea.
	var bonito bytes.Buffer
	if err := json.Indent(&bonito, []byte(guardado), "", "  "); err != nil {
		t.Fatal(err)
	}
	bonito.WriteByte('\n')
	ruta := filepath.Join("testdata", "libro_web_desde_fyne.json")
	if os.Getenv("SYTUE_ACTUALIZAR") == "1" {
		if err := os.WriteFile(ruta, bonito.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fijado, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("falta %s (SYTUE_ACTUALIZAR=1 lo escribe): %v", ruta, err)
	}
	if !igual(t, string(fijado), guardado) {
		t.Errorf("lo que guarda Fyne ya no es %s: con SYTUE_ACTUALIZAR=1 se reescribe, y la web lo vuelve a mirar", ruta)
	}
}
