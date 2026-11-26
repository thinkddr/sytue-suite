// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// casosAgrupar son los de `testdata/agrupar.json`, que lee también la prueba de la web
// (`lib/agruparIA.test.ts`): la instrucción, el contexto y el lector tienen que decir lo
// mismo desde las dos, o el modelo lee una cosa en la web y otra en Fyne.
type casosAgrupar struct {
	Instrucciones []struct{ Idioma, Texto string } `json:"instrucciones"`
	Contextos     []struct {
		Fotos    []Photo  `json:"fotos"`
		Contexto string   `json:"contexto"`
		IDs      []string `json:"ids"`
	} `json:"contextos"`
	Lector []struct {
		Caso    string           `json:"caso"`
		Texto   string           `json:"texto"`
		IDs     []string         `json:"ids"`
		Albumes []AlbumPropuesto `json:"albumes"`
	} `json:"lector"`
}

func leerCasosAgrupar(t *testing.T) casosAgrupar {
	t.Helper()
	b, err := os.ReadFile("testdata/agrupar.json")
	if err != nil {
		t.Fatal(err)
	}
	var c casosAgrupar
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Instrucciones) < 3 || len(c.Contextos) == 0 || len(c.Lector) < 10 {
		t.Fatalf("faltan casos: %d instrucciones, %d contextos, %d del lector", len(c.Instrucciones), len(c.Contextos), len(c.Lector))
	}
	return c
}

func TestInstruccionAgruparEsLaDeLaWeb(t *testing.T) {
	for _, c := range leerCasosAgrupar(t).Instrucciones {
		if got := instruccionAgrupar(c.Idioma); got != c.Texto {
			t.Errorf("instruccionAgrupar(%q):\n%s\nquiero:\n%s", c.Idioma, got, c.Texto)
		}
	}
}

func TestContextoAgruparEsElDeLaWeb(t *testing.T) {
	for _, c := range leerCasosAgrupar(t).Contextos {
		got, ids := ContextoAgrupar(c.Fotos)
		if got != c.Contexto || !slices.Equal(ids, c.IDs) {
			t.Errorf("ContextoAgrupar:\n%s\n%v\nquiero:\n%s\n%v", got, ids, c.Contexto, c.IDs)
		}
	}
}

func TestLeerAgrupacion(t *testing.T) {
	for _, c := range leerCasosAgrupar(t).Lector {
		got, ok := LeerAgrupacion(c.Texto, c.IDs)
		if c.Albumes == nil {
			if ok {
				t.Errorf("%s: se leyó %v y no se puede leer", c.Caso, got)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, c.Albumes) {
			t.Errorf("%s:\n%v %v\nquiero:\n%v", c.Caso, got, ok, c.Albumes)
		}
	}
}

// AgruparFotos manda UNA llamada de texto —sin imagen, sin razonar y con la lengua de
// la sesión— y devuelve la propuesta con los ids DE VERDAD, no con los números que lee el
// modelo. Las fotos sin nada escrito no viajan.
func TestAgruparFotosDevuelveLosIdsDeVerdad(t *testing.T) {
	var cuerpo map[string]any
	llamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas++
		if r.URL.Path != "/api/v1/assist" || r.Header.Get("Accept-Language") != "es" {
			t.Errorf("petición: %s %q", r.URL.Path, r.Header.Get("Accept-Language"))
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		_, _ = io.WriteString(w, `{"text":"{\"albumes\":[{\"titulo\":\"Playa\",\"ids\":[\"2\",\"1\"]},{\"titulo\":\"Inventado\",\"ids\":[\"3\",\"4\"]}]}"}`)
	}))
	defer srv.Close()
	fotos := []Photo{
		{ID: "a", Description: "Una playa"}, {ID: "sin-nada"}, {ID: "b", Tags: []string{"playa"}},
	}
	got, err := AgruparFotos(context.Background(), NewIA(srv.URL, nil, nil), fotos, "es")
	if err != nil || !reflect.DeepEqual(got, []AlbumPropuesto{{Titulo: "Playa", IDs: []string{"b", "a"}}}) {
		t.Fatalf("AgruparFotos = %v %v", got, err)
	}
	if cuerpo["action"] != "agrupar-fotos" || cuerpo["esfuerzo"] != "none" || cuerpo["imagen"] != nil ||
		cuerpo["instruction"] != instruccionAgrupar("es") || strings.Contains(cuerpo["context"].(string), "sin-nada") {
		t.Errorf("cuerpo de /assist: %v", cuerpo)
	}
	// Con menos de dos fotos que contar no se gasta una llamada.
	if got, err := AgruparFotos(context.Background(), NewIA(srv.URL, nil, nil), fotos[:2], "es"); err != nil || len(got) != 0 || got == nil || llamadas != 1 {
		t.Errorf("con una foto: %v %v y %d llamadas", got, err, llamadas)
	}
}

// Una respuesta sin la lista se dice como ilegible, no como «no hay grupos».
func TestAgruparFotosIlegible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"text":"No sé agrupar esto."}`)
	}))
	defer srv.Close()
	fotos := []Photo{{ID: "a", Description: "x"}, {ID: "b", Description: "y"}}
	if _, err := AgruparFotos(context.Background(), NewIA(srv.URL, nil, nil), fotos, "es"); !errors.Is(err, ErrIAIlegible) {
		t.Fatalf("err = %v", err)
	}
}
