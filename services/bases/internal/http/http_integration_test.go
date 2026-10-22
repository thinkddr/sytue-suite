// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/bases/db"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// servidor monta las rutas reales sin el verificador OIDC: la identidad sale de las
// cabeceras X-Test-Sub y X-Test-Tenant, que solo existen en este test.
func servidor(t *testing.T) http.Handler { return servidorCon(t, nil) }

// servidorCon es servidor dejando retocar las dependencias (el Hub y Sytue ID de pega).
func servidorCon(t *testing.T, retocar func(*Deps)) http.Handler {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	deps := Deps{Store: st, Altcha: altcha.New([]byte("clave-de-prueba-de-32-bytes-0000"), altcha.ConRepeticiones(st.RegistrarAltcha))}
	if retocar != nil {
		retocar(&deps)
	}
	hs := &Handlers{
		Deps:       deps,
		mencionada: hubnotify.NuevaUnaVez(time.Minute),
		tic:        20 * time.Millisecond,
		// Holgado: aquí se prueba el flujo, no el límite por IP.
		limitePublico:  httpx.NewTokenBucket(1000, 1000),
		limiteConector: httpx.NewTokenBucket(1000, 1000),
		subidas:        make(chan struct{}, subidasALaVez),
	}
	r := httpx.NewRouter(httpx.RouterOptions{MaxBodyBytes: maxCuerpoImport})
	hs.montarFormularios(r)
	hs.montarConector(r)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c := authn.Claims{Subject: r.Header.Get("X-Test-Sub"), Tenant: r.Header.Get("X-Test-Tenant"), Name: r.Header.Get("X-Test-Sub")}
				if g := r.Header.Get("X-Test-Groups"); g != "" {
					c.Groups = strings.Split(g, ",")
				}
				// X-Test-Exp: en cuánto caduca el token (el flujo de cambios no le sobrevive).
				if d, err := time.ParseDuration(r.Header.Get("X-Test-Exp")); err == nil {
					c.Expira = time.Now().Add(d)
				}
				next.ServeHTTP(w, r.WithContext(authn.Into(r.Context(), c)))
			})
		})
		hs.montar(api)
	})
	return r
}

func llamar(t *testing.T, srv http.Handler, sub, tenant, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	return llamarCon(t, srv, sub, tenant, "", metodo, ruta, cuerpo)
}

// llamarCon es llamar con los grupos del token (separados por comas).
func llamarCon(t *testing.T, srv http.Handler, sub, tenant, grupos, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, "/api/v1"+ruta, strings.NewReader(cuerpo))
	req.Header.Set("X-Test-Groups", grupos)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Sub", sub)
	req.Header.Set("X-Test-Tenant", tenant)
	req.Header.Set("Authorization", "Bearer sesion-de-"+sub)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func idDeRespuesta(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var o struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil || o.ID == "" {
		t.Fatalf("respuesta sin id (%d): %s", w.Code, w.Body)
	}
	return o.ID
}

func TestFlujoYPermisosPorHTTP(t *testing.T) {
	srv := servidor(t)
	w := llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`)
	if w.Code != 201 {
		t.Fatalf("crear base: %d %s", w.Code, w.Body)
	}
	base := idDeRespuesta(t, w)
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Contactos"}`))
	campo := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Nombre","kind":"text"}`))
	num := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Saldo","kind":"number"}`))
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{"`+campo+`":"=HYPERLINK(\"http://x\")","`+num+`":-5}}`)

	// La búsqueda del Hub solo devuelve lo que quien busca puede ver.
	if b := llamar(t, srv, "ana", "acme", "GET", "/search?q=clie", ""); b.Code != 200 || !strings.Contains(b.Body.String(), base) {
		t.Errorf("búsqueda de la propietaria: %d %s", b.Code, b.Body)
	}
	for _, quien := range [][2]string{{"bea", "acme"}, {"gus", "globex"}} {
		if b := llamar(t, srv, quien[0], quien[1], "GET", "/search?q=clie", ""); b.Code != 200 || strings.Contains(b.Body.String(), base) {
			t.Errorf("búsqueda de %s: %d %s", quien[0], b.Code, b.Body)
		}
	}

	// El CSV exportado neutraliza la fórmula y deja el número negativo como número.
	w = llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/export?format=csv", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("export: %d %s", w.Code, w.Header())
	}
	cuerpo := w.Body.String()
	if !strings.Contains(cuerpo, `"'=HYPERLINK`) || !strings.Contains(cuerpo, ",-5") {
		t.Errorf("CSV mal neutralizado: %q", cuerpo)
	}
	if llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/export?format=xlsx", "").Code != 422 {
		t.Error("formato desconocido")
	}

	// Aislamiento y permisos por HTTP: 404 sin acceso, 403 con acceso insuficiente.
	if c := llamar(t, srv, "gus", "globex", "GET", "/tables/"+tabla+"/records", "").Code; c != 404 {
		t.Errorf("otra organización: %d", c)
	}
	if c := llamar(t, srv, "bea", "acme", "GET", "/tables/"+tabla+"/records", "").Code; c != 404 {
		t.Errorf("sin permiso: %d", c)
	}
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"viewer"}`)
	if c := llamar(t, srv, "bea", "acme", "GET", "/tables/"+tabla+"/records", "").Code; c != 200 {
		t.Errorf("lector lee: %d", c)
	}
	if c := llamar(t, srv, "bea", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{}}`).Code; c != 403 {
		t.Errorf("lector escribe: %d", c)
	}
	// El BFF no puede colar un base_id: la base sale siempre de la tabla real.
	if c := llamar(t, srv, "bea", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"editor"}`).Code; c != 403 {
		t.Errorf("lector se concede editor: %d", c)
	}

	// Entradas hostiles: ids que no son UUID, cuerpos desconocidos, sin identidad.
	if c := llamar(t, srv, "ana", "acme", "GET", "/tables/no-es-uuid/records", "").Code; c != 404 {
		t.Errorf("id basura: %d", c)
	}
	// Un cursor que no es nuestro es un 422, nunca un 500 de la base de datos.
	if c := llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/records?cursor=basura", "").Code; c != 422 {
		t.Errorf("cursor basura: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/records?view=basura", "").Code; c != 404 {
		t.Errorf("vista basura: %d", c)
	}
	if c := llamar(t, srv, "", "", "GET", "/bases", "").Code; c != 401 {
		t.Errorf("sin identidad: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"x","extra":1}`).Code; c != 400 && c != 422 {
		t.Errorf("campo desconocido en el cuerpo: %d", c)
	}
	// Filtros con veneno: el servidor lo rechaza antes de ejecutarlo.
	veneno := `{"name":"V","kind":"grid","config":{"filters":[{"field":"` + campo + `","op":"eq; DROP TABLE bases","value":"x"}]}}`
	if c := llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/views", veneno).Code; c != 422 {
		t.Errorf("filtro con veneno: %d", c)
	}
}

func TestImportarCSVPorHTTP(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"B"}`))
	w := llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/import?name=Importada", "a,b\n1,x\n2,y\n")
	if w.Code != 201 {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/import?name=Mala", "a,b\n1,\x00\n").Code; c != 422 {
		t.Errorf("CSV con NUL: %d", c)
	}
	grande := "a\n" + strings.Repeat("x\n", 3<<20)
	if c := llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/import?name=Grande", grande).Code; c < 400 {
		t.Errorf("CSV de más de 5 MiB aceptado: %d", c)
	}
}

// publico llama a las rutas sin sesión (las que usa el BFF por quien abre un enlace).
func publico(t *testing.T, srv http.Handler, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestFormularioPublicoPorHTTP(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"B"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Contactos"}`))
	campo := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Nombre","kind":"text"}`))
	oculto := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Interno","kind":"text"}`))
	vista := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/views", `{"name":"Contacto","kind":"form","config":{"fields":["`+campo+`"]}}`))

	w := llamar(t, srv, "ana", "acme", "POST", "/views/"+vista+"/form-links", `{"max_responses":5}`)
	if w.Code != 201 {
		t.Fatalf("crear enlace: %d %s", w.Code, w.Body)
	}
	var enlace struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &enlace)
	// Al listar, el token no vuelve a salir.
	if l := llamar(t, srv, "ana", "acme", "GET", "/views/"+vista+"/form-links", ""); l.Code != 200 || strings.Contains(l.Body.String(), enlace.Token) || strings.Contains(l.Body.String(), "token") {
		t.Errorf("el listado enseña el token: %s", l.Body)
	}
	pub := "/_bases/v1/forms/" + enlace.Token

	// La definición pública solo trae el campo de la vista.
	d := publico(t, srv, "GET", pub+"/", "")
	if d.Code != 200 || !strings.Contains(d.Body.String(), campo) || strings.Contains(d.Body.String(), oculto) {
		t.Fatalf("definición: %d %s", d.Code, d.Body)
	}

	reto := func() string {
		t.Helper()
		r := publico(t, srv, "GET", pub+"/altcha", "")
		var ch altcha.Challenge
		if err := json.Unmarshal(r.Body.Bytes(), &ch); err != nil || r.Code != 200 {
			t.Fatalf("reto: %d %s", r.Code, r.Body)
		}
		sol, err := altcha.SolveChallenge(ch)
		if err != nil {
			t.Fatal(err)
		}
		return sol
	}
	cuerpo := func(sol, valores string) string { return `{"values":` + valores + `,"altcha":"` + sol + `"}` }
	valores := `{"` + campo + `":"Panadería"}`

	// Sin prueba de trabajo, con una inventada o con la de OTRO enlace: no entra.
	if c := publico(t, srv, "POST", pub+"/", cuerpo("", valores)).Code; c != 400 {
		t.Errorf("sin ALTCHA: %d", c)
	}
	if c := publico(t, srv, "POST", pub+"/", cuerpo("bm8=", valores)).Code; c != 400 {
		t.Errorf("ALTCHA inventado: %d", c)
	}
	w2 := llamar(t, srv, "ana", "acme", "POST", "/views/"+vista+"/form-links", `{"max_responses":5}`)
	var otro struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &otro)
	rOtro := publico(t, srv, "GET", "/_bases/v1/forms/"+otro.Token+"/altcha", "")
	var chOtro altcha.Challenge
	_ = json.Unmarshal(rOtro.Body.Bytes(), &chOtro)
	solOtro, _ := altcha.SolveChallenge(chOtro)
	if c := publico(t, srv, "POST", pub+"/", cuerpo(solOtro, valores)).Code; c != 400 {
		t.Errorf("reto de otro enlace: %d", c)
	}

	// Con la prueba, entra una vez; repetirla no vale.
	sol := reto()
	if c := publico(t, srv, "POST", pub+"/", cuerpo(sol, valores)); c.Code != 204 {
		t.Fatalf("envío válido: %d %s", c.Code, c.Body)
	}
	if c := publico(t, srv, "POST", pub+"/", cuerpo(sol, valores)).Code; c != 400 {
		t.Errorf("reto repetido: %d", c)
	}
	// Un campo que el formulario no incluye se rechaza aunque exista en la tabla.
	if c := publico(t, srv, "POST", pub+"/", cuerpo(reto(), `{"`+oculto+`":"x"}`)).Code; c != 422 {
		t.Errorf("campo fuera del formulario: %d", c)
	}
	// Cuerpo enorme: se corta antes de procesarlo.
	if c := publico(t, srv, "POST", pub+"/", cuerpo(reto(), `{"`+campo+`":"`+strings.Repeat("x", 200<<10)+`"}`)).Code; c < 400 {
		t.Errorf("cuerpo de 200 KiB aceptado: %d", c)
	}
	// Las rutas públicas no dan acceso a nada con sesión, ni al revés.
	if c := publico(t, srv, "GET", "/api/v1/bases", "").Code; c != 401 {
		t.Errorf("la API sin identidad: %d", c)
	}

	// Revocar: 404 en todo, al instante.
	if c := llamar(t, srv, "bea", "acme", "DELETE", "/form-links/"+enlace.ID, "").Code; c != 404 {
		t.Errorf("otra persona revoca: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "DELETE", "/form-links/"+enlace.ID, "").Code; c != 204 {
		t.Fatalf("revocar: %d", c)
	}
	for _, r := range []*httptest.ResponseRecorder{publico(t, srv, "GET", pub+"/", ""), publico(t, srv, "GET", pub+"/altcha", ""), publico(t, srv, "POST", pub+"/", cuerpo(sol, valores))} {
		if r.Code != 404 && r.Code != 400 {
			t.Errorf("tras revocar: %d", r.Code)
		}
	}
	if c := publico(t, srv, "GET", pub+"/", "").Code; c != 404 {
		t.Errorf("definición tras revocar: %d", c)
	}
	if c := publico(t, srv, "GET", "/_bases/v1/forms/no-es-un-token/", "").Code; c != 404 {
		t.Errorf("token mal formado: %d", c)
	}
}

// conector llama a la ruta de lectura con un token de conexión.
func conector(t *testing.T, srv http.Handler, metodo, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, "/_bases/v1/connector/read", http.NoBody)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestConectorPorHTTP(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Tienda"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Pedidos"}`))
	campo := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/fields", `{"name":"Importe","kind":"number"}`))
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{"`+campo+`":5}}`)
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{"`+campo+`":50}}`)
	vista := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/views",
		`{"name":"Grandes","kind":"grid","config":{"filters":[{"field":"`+campo+`","op":"gt","value":10}]}}`))

	// Solo el propietario concede.
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"editor"}`)
	if c := llamar(t, srv, "bea", "acme", "POST", "/tables/"+tabla+"/connections", `{"name":"x"}`).Code; c != 403 {
		t.Errorf("un editor concede un conector: %d", c)
	}
	if c := llamar(t, srv, "gus", "globex", "POST", "/tables/"+tabla+"/connections", `{"name":"x"}`).Code; c != 404 {
		t.Errorf("otra organización concede: %d", c)
	}
	w := llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/connections", `{"name":"Calcus de ventas","view_id":"`+vista+`","days":30}`)
	if w.Code != 201 {
		t.Fatalf("conceder: %d %s", w.Code, w.Body)
	}
	var con struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &con)
	if l := llamar(t, srv, "ana", "acme", "GET", "/bases/"+base+"/connections", ""); l.Code != 200 || strings.Contains(l.Body.String(), con.Token) || strings.Contains(l.Body.String(), `"token"`) {
		t.Errorf("el listado enseña el token: %s", l.Body)
	}

	// Lee SOLO lo que la vista abarca, con procedencia.
	r := conector(t, srv, "GET", con.Token)
	if r.Code != 200 {
		t.Fatalf("leer: %d %s", r.Code, r.Body)
	}
	var lectura struct {
		Columns []struct{ Name, Type string } `json:"columns"`
		Rows    [][]any                       `json:"rows"`
		Prov    struct {
			Source    string `json:"source"`
			BaseName  string `json:"base_name"`
			TableName string `json:"table_name"`
			ViewName  string `json:"view_name"`
			GrantedBy string `json:"granted_by"`
			Version   int    `json:"version"`
		} `json:"provenance"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &lectura)
	if len(lectura.Rows) != 1 || lectura.Rows[0][0] != float64(50) || lectura.Columns[0].Type != "number" {
		t.Errorf("la vista filtra (>10): %+v", lectura)
	}
	if p := lectura.Prov; p.Source != "sytue-bases" || p.BaseName != "Tienda" || p.TableName != "Pedidos" || p.ViewName != "Grandes" || p.GrantedBy != "ana" || p.Version != 1 {
		t.Errorf("procedencia: %+v", p)
	}

	// Solo lectura: no hay método que escriba, y el token no vale en la API con sesión.
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if c := conector(t, srv, m, con.Token).Code; c != 405 && c != 404 {
			t.Errorf("%s con token de conector: %d", m, c)
		}
	}
	// Tokens que no valen: ninguno, basura, de formulario, otro prefijo.
	for _, malo := range []string{"", "x", "sbc_" + strings.Repeat("a", 43), strings.TrimPrefix(con.Token, "sbc_"), con.Token + "x"} {
		if c := conector(t, srv, "GET", malo).Code; c != 401 {
			t.Errorf("token «%s»: %d", malo, c)
		}
	}

	// Rotar: el token viejo muere y la versión sube.
	rot := llamar(t, srv, "ana", "acme", "POST", "/connections/"+con.ID+"/rotate", "")
	var nuevo struct {
		Version int    `json:"version"`
		Token   string `json:"token"`
	}
	_ = json.Unmarshal(rot.Body.Bytes(), &nuevo)
	if rot.Code != 200 || nuevo.Version != 2 || nuevo.Token == con.Token {
		t.Fatalf("rotar: %d %s", rot.Code, rot.Body)
	}
	if c := conector(t, srv, "GET", con.Token).Code; c != 401 {
		t.Errorf("token rotado sigue leyendo: %d", c)
	}
	if c := conector(t, srv, "GET", nuevo.Token).Code; c != 200 {
		t.Errorf("token nuevo: %d", c)
	}

	// Revocar: la siguiente lectura ya es un 401. Y solo revoca el propietario.
	if c := llamar(t, srv, "bea", "acme", "DELETE", "/connections/"+con.ID, "").Code; c != 403 {
		t.Errorf("un editor revoca: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "DELETE", "/connections/"+con.ID, "").Code; c != 204 {
		t.Fatalf("revocar: %d", c)
	}
	if c := conector(t, srv, "GET", nuevo.Token).Code; c != 401 {
		t.Errorf("tras revocar sigue leyendo: %d", c)
	}

	// Cada lectura quedó auditada (2 con éxito), con la conexión como actor.
	a := llamar(t, srv, "ana", "acme", "GET", "/bases/"+base+"/audit?limit=200", "")
	if n := strings.Count(a.Body.String(), `"action":"connector.read"`); n != 2 {
		t.Errorf("lecturas auditadas: %d", n)
	}
	for _, accion := range []string{"connector.grant", "connector.rotate", "connector.revoke", `"actor":"conn:` + con.ID} {
		if !strings.Contains(a.Body.String(), accion) {
			t.Errorf("falta en la auditoría: %s", accion)
		}
	}
}

// La exportación saca de un enlace el TEXTO del registro enlazado además de su id, en
// CSV y en JSON, y ese texto pasa por la misma neutralización de fórmulas que cualquier
// otro: lo escribió alguien en otra tabla.
func TestExportarEnlacesConValores(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Ventas"}`))
	clientes := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Clientes"}`))
	pedidos := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Pedidos"}`))
	nombre := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+clientes+"/fields", `{"name":"Nombre","kind":"text"}`))
	enlace := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+pedidos+"/fields", `{"name":"Cliente","kind":"link","link_table_id":"`+clientes+`"}`))
	malo := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+clientes+"/records", `{"data":{"`+nombre+`":"=cmd|' /C calc'!A0"}}`))
	bueno := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+clientes+"/records", `{"data":{"`+nombre+`":"Acme, S. L."}}`))
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+pedidos+"/records", `{"data":{},"links":{"`+enlace+`":["`+malo+`"]}}`)
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+pedidos+"/records", `{"data":{},"links":{"`+enlace+`":["`+bueno+`"]}}`)
	llamar(t, srv, "ana", "acme", "POST", "/tables/"+pedidos+"/records", `{"data":{}}`)

	w := llamar(t, srv, "ana", "acme", "GET", "/tables/"+pedidos+"/export?format=csv", "")
	filas, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	if w.Code != 200 || err != nil || len(filas) != 4 {
		t.Fatalf("CSV: %d %v %q", w.Code, err, w.Body)
	}
	if !slices.Equal(filas[0], []string{"Cliente", "Cliente (id)"}) {
		t.Errorf("cabecera: %q", filas[0])
	}
	if !slices.Equal(filas[1], []string{`'=cmd|' /C calc'!A0`, malo}) {
		t.Errorf("el texto del enlace no se neutraliza: %q", filas[1])
	}
	if !slices.Equal(filas[2], []string{"Acme, S. L.", bueno}) || !slices.Equal(filas[3], []string{"", ""}) {
		t.Errorf("filas: %q", filas[2:])
	}

	w = llamar(t, srv, "ana", "acme", "GET", "/tables/"+pedidos+"/export?format=json", "")
	var ex struct {
		Table   string `json:"table"`
		Fields  []struct{ ID string }
		Records []struct {
			Links      map[string][]string `json:"links"`
			LinkValues map[string][]string `json:"link_values"`
		} `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ex); err != nil || ex.Table != "Pedidos" || len(ex.Fields) != 1 || len(ex.Records) != 3 {
		t.Fatalf("JSON: %v %s", err, w.Body)
	}
	if !slices.Equal(ex.Records[1].Links[enlace], []string{bueno}) || !slices.Equal(ex.Records[1].LinkValues[enlace], []string{"Acme, S. L."}) {
		t.Errorf("JSON sin id o sin valor: %+v", ex.Records[1])
	}

	// Un lector exporta; quien no tiene acceso recibe un 404 y no medio fichero.
	if c := llamar(t, srv, "gus", "globex", "GET", "/tables/"+pedidos+"/export?format=csv", ""); c.Code != 404 || strings.Contains(c.Body.String(), "Acme") {
		t.Errorf("exportación desde otra organización: %d %q", c.Code, c.Body)
	}
}

// Una tabla de más de un tramo (500 registros) se exporta entera, sin repetir, y el
// JSON que sale a trozos es un JSON válido.
func TestExportarPorTramos(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Grande"}`))
	var b strings.Builder
	b.WriteString("n\n")
	for i := 0; i < 1203; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	w := llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/import?name=Grande", b.String())
	if w.Code != 201 {
		t.Fatalf("importar: %d %s", w.Code, w.Body)
	}
	tabla := idDeRespuesta(t, w)
	w = llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/export?format=json", "")
	var ex struct {
		Records []struct{ ID string } `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ex); err != nil {
		t.Fatalf("JSON por tramos no válido: %v", err)
	}
	vistos := map[string]bool{}
	for _, r := range ex.Records {
		vistos[r.ID] = true
	}
	if len(ex.Records) != 1203 || len(vistos) != 1203 {
		t.Errorf("JSON: %d registros, %d distintos", len(ex.Records), len(vistos))
	}
	w = llamar(t, srv, "ana", "acme", "GET", "/tables/"+tabla+"/export?format=csv", "")
	if n := strings.Count(w.Body.String(), "\n"); n != 1204 {
		t.Errorf("CSV: %d líneas", n)
	}
}

// Las cabeceras del consumidor llegan al store: el token de SyERP lee como SyERP de su
// organización y como nadie más (mismo 401 que un token revocado, sin distinguir el motivo).
func TestConectorPorHTTPExigeConsumidorYOrganizacion(t *testing.T) {
	srv := servidor(t)
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Tienda"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Pedidos"}`))
	vista := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/views", `{"name":"Todo","kind":"grid","config":{}}`))
	if c := llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/connections", `{"name":"x","consumer":"erp"}`).Code; c != 422 {
		t.Errorf("SyERP sin vista: %d", c)
	}
	w := llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/connections", `{"name":"Informe","view_id":"`+vista+`","consumer":"erp"}`)
	if w.Code != 201 {
		t.Fatalf("conceder: %d %s", w.Code, w.Body)
	}
	var con struct {
		Token    string `json:"token"`
		Consumer string `json:"consumer"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &con)
	if con.Consumer != "erp" {
		t.Errorf("consumidor de la conexión: %q", con.Consumer)
	}
	leer := func(consumidor, tenant string) int {
		req := httptest.NewRequest(http.MethodGet, "/_bases/v1/connector/read", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+con.Token)
		if consumidor != "" {
			req.Header.Set("X-Sytue-Consumer", consumidor)
		}
		if tenant != "" {
			req.Header.Set("X-Sytue-Tenant", tenant)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range []struct {
		consumidor, tenant string
		quiero             int
	}{{"erp", "acme", 200}, {"", "", 401}, {"web", "acme", 401}, {"erp", "", 401}, {"erp", "globex", 401}} {
		if got := leer(c.consumidor, c.tenant); got != c.quiero {
			t.Errorf("leer como %q de %q: %d, esperado %d", c.consumidor, c.tenant, got, c.quiero)
		}
	}
}
