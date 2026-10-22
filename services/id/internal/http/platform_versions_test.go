// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// saludo levanta un componente de mentira que contesta lo que se le diga.
func saludo(t *testing.T, cuerpo string, codigo int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(codigo)
		_, _ = w.Write([]byte(cuerpo))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestParseComponents(t *testing.T) {
	// Lo válido entra; lo malformado se descarta sin tumbar el arranque, porque
	// esta lista es diagnóstico y no puede dejar al IdP sin levantar.
	got := ParseComponents(
		"id|service|http://a/, drive|service|http://b ,, roto|solo-dos, |service|http://c," +
			"sinurl|service|, www|surface|http://d",
	)
	quiero := []Component{
		{Name: "id", Kind: "service", URL: "http://a"},
		{Name: "drive", Kind: "service", URL: "http://b"},
		{Name: "www", Kind: "surface", URL: "http://d"},
	}
	if len(got) != len(quiero) {
		t.Fatalf("%d componentes, quiero %d: %+v", len(got), len(quiero), got)
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("[%d] = %+v, quiero %+v", i, got[i], quiero[i])
		}
	}
	if len(ParseComponents("")) != 0 {
		t.Error("una lista vacía no puede producir componentes")
	}
}

// El caso que da sentido a la pantalla: un despliegue a medias. Veinticuatro
// piezas en v2, una que se quedó en v1 y otra que no contesta.
func TestVersiones_DespliegueAMedias(t *testing.T) {
	alDia := saludo(t, `{"status":"ok","version":"v2","commit":"cafe1234"}`, http.StatusOK)
	atras := saludo(t, `{"status":"ok","version":"v1","commit":"beef5678"}`, http.StatusOK)
	rota := saludo(t, `no soy json`, http.StatusOK)

	s := &Server{cfg: Config{Components: []Component{
		{Name: "id", Kind: "service", URL: alDia.URL},
		{Name: "drive", Kind: "service", URL: alDia.URL},
		{Name: "docs", Kind: "service", URL: atras.URL},
		{Name: "www", Kind: "surface", URL: rota.URL},
		// Un puerto donde no escucha nadie: es el caso «el pod no está».
		{Name: "hub", Kind: "service", URL: "http://127.0.0.1:1"},
	}}}

	rec := httptest.NewRecorder()
	s.handlePlatformVersions(rec, httptest.NewRequest(http.MethodGet, "/versions", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("código=%d body=%s", rec.Code, rec.Body)
	}
	var out struct {
		Items    []ComponentVersion `json:"items"`
		Expected string             `json:"expected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("respuesta ilegible: %v (%s)", err, rec.Body)
	}

	// TODAS las filas, contesten o no: una fila que desaparece cuando el servicio
	// se cae es la peor forma de enterarse de que se ha caído.
	if len(out.Items) != 5 {
		t.Fatalf("%d filas, quiero 5: %+v", len(out.Items), out.Items)
	}
	// La mayoría manda: v2 la declaran dos, v1 solo uno.
	if out.Expected != "v2" {
		t.Errorf("expected=%q, quiero v2", out.Expected)
	}

	por := map[string]ComponentVersion{}
	for _, it := range out.Items {
		por[it.Name] = it
	}
	if v := por["docs"]; !v.OK || v.Release != "v1" {
		t.Errorf("docs = %+v; el que se quedó atrás debe salir con SU versión, no con la buena", v)
	}
	if v := por["hub"]; v.OK || v.Error == "" {
		t.Errorf("hub = %+v; el que no contesta debe decirlo", v)
	}
	if v := por["www"]; v.OK {
		t.Errorf("www = %+v; una respuesta ilegible no es una versión válida", v)
	}
	// Y ninguno de los mudos puede haber heredado la versión de nadie: eso
	// convertiría un despliegue roto en uno que parece bueno.
	for _, n := range []string{"hub", "www"} {
		if por[n].Release != "" || por[n].Commit != "" {
			t.Errorf("%s = %+v; sin respuesta no se inventa versión", n, por[n])
		}
	}

	// Orden estable: los servicios antes que las superficies, y alfabético dentro.
	nombres := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		nombres = append(nombres, it.Kind+"/"+it.Name)
	}
	quiero := []string{"service/docs", "service/drive", "service/hub", "service/id", "surface/www"}
	for i := range quiero {
		if nombres[i] != quiero[i] {
			t.Errorf("orden[%d]=%s, quiero %s (la tabla se compara a ojo y no puede bailar)",
				i, nombres[i], quiero[i])
			break
		}
	}
}

func TestVersionMayoritaria(t *testing.T) {
	casos := []struct {
		nombre string
		items  []ComponentVersion
		quiero string
	}{
		{"mayoría clara", []ComponentVersion{
			{OK: true, Release: "v2"}, {OK: true, Release: "v2"}, {OK: true, Release: "v1"},
		}, "v2"},
		{"empate ⇒ nadie manda", []ComponentVersion{
			{OK: true, Release: "v2"}, {OK: true, Release: "v1"},
		}, ""},
		{"los mudos no votan", []ComponentVersion{
			{OK: true, Release: "v2"}, {OK: false, Release: ""}, {OK: false, Release: ""},
		}, "v2"},
		{"sin nadie que conteste", []ComponentVersion{{OK: false}}, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := versionMayoritaria(c.items); got != c.quiero {
				t.Errorf("= %q, quiero %q", got, c.quiero)
			}
		})
	}
}

// Un componente que responde 200 pero no es de los nuestros no puede colarse
// como «al día»: sin versión no hay nada que comparar.
func TestVersiones_RespuestaSinVersion(t *testing.T) {
	vacio := saludo(t, `{"status":"ok"}`, http.StatusOK)
	s := &Server{cfg: Config{Components: []Component{{Name: "x", Kind: "service", URL: vacio.URL}}}}
	rec := httptest.NewRecorder()
	s.handlePlatformVersions(rec, httptest.NewRequest(http.MethodGet, "/versions", http.NoBody))
	var out struct {
		Items    []ComponentVersion `json:"items"`
		Expected string             `json:"expected"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Expected != "" {
		t.Errorf("expected=%q; una respuesta sin versión no puede fijar el patrón", out.Expected)
	}
	if !out.Items[0].OK || out.Items[0].Release != "" {
		t.Errorf("= %+v; contestó, pero sin versión", out.Items[0])
	}
}
