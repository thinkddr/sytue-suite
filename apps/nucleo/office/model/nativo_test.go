// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// TestNativoIdaYVueltaSinPerdida: lo que se escribe se lee igual, el contenido byte a
// byte (también lo que un HTML o un JSON «normalizado» cambiaría: espacios, orden de
// claves, escapes), con los comentarios enteros.
func TestNativoIdaYVueltaSinPerdida(t *testing.T) {
	padre := "c1"
	creado := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	casos := []Nativo{
		{
			Kind: KindDocument, Titulo: "Informe «Q3»", Contenido: "<h1>Hola</h1><p>  dos  espacios &amp; <mark data-comment-id=\"c1\">ñ</mark></p>",
			Comentarios: []Comment{
				{ID: "c1", AuthorSub: "local", AuthorName: "Ana", Anchor: json.RawMessage(`{"mark":"c1","text":"ñ"}`), Body: "¿Seguro?", CreatedAt: creado, UpdatedAt: creado},
				{ID: "c2", ParentID: &padre, AuthorSub: "local", Body: "Sí", Resolved: true, CreatedAt: creado, UpdatedAt: creado},
			},
		},
		{Kind: KindSpreadsheet, Titulo: "Gastos", Contenido: `{"sheets":[{"name":"H1","cells":{"A1":{"v":"=SUMA(B1:B2)"}}}],  "z":1, "a":2}`},
		{Kind: KindPresentation, Titulo: "", Contenido: `{"slides":[{"id":"s1","boxes":[]}],"theme":"cielo"}`},
	}
	for _, n := range casos {
		n.Creado, n.Modificado = creado, creado.Add(time.Hour)
		datos, err := EscribirNativo(n)
		if err != nil {
			t.Fatal(err)
		}
		if !EsNativo(datos) {
			t.Fatalf("%s: EsNativo no reconoce lo escrito", n.Kind)
		}
		otra, _ := EscribirNativo(n)
		if !bytes.Equal(datos, otra) {
			t.Errorf("%s: el mismo documento da bytes distintos", n.Kind)
		}
		leido, err := LeerNativo(datos)
		if err != nil {
			t.Fatal(err)
		}
		if leido.Contenido != n.Contenido {
			t.Errorf("%s: contenido %q, quería %q", n.Kind, leido.Contenido, n.Contenido)
		}
		if n.Comentarios == nil && len(leido.Comentarios) != 0 {
			t.Errorf("%s: comentarios de más: %v", n.Kind, leido.Comentarios)
		}
		// Los comentarios, por su JSON: un ancla nil vuelve como «null», que es lo mismo.
		a, _ := json.Marshal(leido.Comentarios)
		b, _ := json.Marshal(n.Comentarios)
		if len(n.Comentarios) > 0 && !bytes.Equal(a, b) {
			t.Errorf("%s: comentarios\n got %s\nwant %s", n.Kind, a, b)
		}
		leido.Comentarios, n.Comentarios = nil, nil
		if !reflect.DeepEqual(leido, n) {
			t.Errorf("%s: ida y vuelta\n got %+v\nwant %+v", n.Kind, leido, n)
		}
	}
}

// TestNativoPrimeraEntradaMimetype: la primera entrada es `mimetype`, sin comprimir.
func TestNativoPrimeraEntradaMimetype(t *testing.T) {
	datos, err := EscribirNativo(Nativo{Kind: KindSpreadsheet, Contenido: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	if f := zr.File[0]; f.Name != "mimetype" || f.Method != zip.Store {
		t.Fatalf("primera entrada %q método %d", f.Name, f.Method)
	}
	if k, ok := KindNativo("C:\\x\\Libro.CALCUS"); !ok || k != KindSpreadsheet {
		t.Errorf("KindNativo = %v %v", k, ok)
	}
	if ExtensionNativa(KindPresentation) != ".diapos" || MimeNativo(KindDocument) != "application/vnd.sytue.docus" {
		t.Error("extensión o mime")
	}
}

// TestNativoRechazaLoQueNoEntiende: un ZIP cualquiera, un formato futuro y un tipo que
// no casa con su mimetype no se leen a medias.
func TestNativoRechazaLoQueNoEntiende(t *testing.T) {
	zipCon := func(entradas map[string]string) []byte {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		for _, k := range []string{"mimetype", "meta.json", "contenido"} {
			if v, ok := entradas[k]; ok {
				f, _ := w.Create(k)
				_, _ = f.Write([]byte(v))
			}
		}
		_ = w.Close()
		return b.Bytes()
	}
	if _, err := LeerNativo([]byte("no es un zip")); !errors.Is(err, ErrNativo) {
		t.Errorf("basura: %v", err)
	}
	if _, err := LeerNativo(zipCon(map[string]string{"contenido": "x"})); !errors.Is(err, ErrNativo) {
		t.Errorf("sin meta: %v", err)
	}
	if _, err := LeerNativo(zipCon(map[string]string{"meta.json": `{"formato":2,"tipo":"document"}`, "contenido": "x"})); !errors.Is(err, ErrNativoFuturo) {
		t.Errorf("futuro: %v", err)
	}
	if _, err := LeerNativo(zipCon(map[string]string{"mimetype": "application/vnd.sytue.calcus", "meta.json": `{"formato":1,"tipo":"document"}`, "contenido": "x"})); !errors.Is(err, ErrNativo) {
		t.Errorf("mimetype cruzado: %v", err)
	}
	if EsNativo(zipCon(map[string]string{"contenido": "x"})) {
		t.Error("EsNativo con un zip cualquiera")
	}
}
