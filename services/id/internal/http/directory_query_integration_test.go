//go:build integration

package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// doCuerpo manda una petición con cuerpo y Content-Type propios. `do` fija JSON y
// serializa, que es justo lo que un QUERY no lleva.
func (l *liveServer) doCuerpo(t *testing.T, metodo, ruta, token, tipo, cuerpo string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(metodo, ruta, bytes.NewReader([]byte(cuerpo)))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if tipo != "" {
		r.Header.Set("Content-Type", tipo)
	}
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	out := map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

// TestDirectorioPorQuery — el directorio contesta lo MISMO por GET y por QUERY.
//
// El valor de esto no es que QUERY funcione: es que los dos métodos den el mismo
// resultado. Son dos caminos hacia el mismo handler y, si se separan, el que
// menos se use se queda atrás sin que nadie lo note —y el que menos se use va a
// ser el nuevo—.
//
// Lo que se busca aquí es el nombre de una persona: con GET viaja en la URL y
// acaba en el log de accesos de cada salto y en el `Referer`; con QUERY va en el
// cuerpo y no queda.
func TestDirectorioPorQuery(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "dirq", Name: "Dir Q"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	u := caOrgUser(t, l, org, "ada@id.test")
	tok := l.token(t, u.ID, org.Slug, nil)

	rrGet, outGet := l.do(t, http.MethodGet, "/api/v1/directory?q=ada", tok, nil)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET: status=%d cuerpo=%s", rrGet.Code, rrGet.Body)
	}
	// El servidor ANUNCIA que entiende QUERY, que es como un cliente lo descubre
	// sin tener que probarlo y comerse un 405.
	if ac := rrGet.Header().Get("Accept-Query"); !strings.Contains(ac, "x-www-form-urlencoded") {
		t.Fatalf("falta Accept-Query en la respuesta: %q", ac)
	}

	rrQ, outQ := l.doCuerpo(t, httpx.MethodQuery, "/api/v1/directory", tok,
		"application/x-www-form-urlencoded", "q=ada")
	if rrQ.Code != http.StatusOK {
		t.Fatalf("QUERY: status=%d cuerpo=%s", rrQ.Code, rrQ.Body)
	}

	gi, _ := outGet["items"].([]any)
	qi, _ := outQ["items"].([]any)
	if len(gi) == 0 {
		t.Fatal("la búsqueda no encontró a nadie; así el test no comprueba nada")
	}
	if len(gi) != len(qi) {
		t.Fatalf("GET devuelve %d y QUERY %d: los dos caminos han divergido", len(gi), len(qi))
	}

	// La query string se IGNORA en un QUERY: el RFC dice que la consulta es el
	// cuerpo, y aceptar las dos obligaría a decidir cuál gana al contradecirse.
	rrMix, outMix := l.doCuerpo(t, httpx.MethodQuery, "/api/v1/directory?q=nadie-se-llama-asi", tok,
		"application/x-www-form-urlencoded", "q=ada")
	if rrMix.Code != http.StatusOK {
		t.Fatalf("QUERY con query string: status=%d", rrMix.Code)
	}
	if mi, _ := outMix["items"].([]any); len(mi) != len(qi) {
		t.Fatalf("la URL no debe pesar en un QUERY: %d frente a %d", len(mi), len(qi))
	}

	// Un cuerpo con otro tipo se rechaza en vez de interpretarse a ciegas.
	if rr, _ := l.doCuerpo(t, httpx.MethodQuery, "/api/v1/directory", tok,
		"application/json", `{"q":"ada"}`); rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("cuerpo JSON: status=%d, quiero 415", rr.Code)
	}

	// Y el token sigue mandando: sin él, QUERY tampoco entra.
	if rr, _ := l.doCuerpo(t, httpx.MethodQuery, "/api/v1/directory", "",
		"application/x-www-form-urlencoded", "q=ada"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("QUERY sin token: status=%d, quiero 401", rr.Code)
	}
}
