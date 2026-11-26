// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // los servidores de mentira hablan con las claves camelCase de Google People.
package agenda_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// ----- Un Google Contacts de mentira, con grupos -----

type peopleFalso struct {
	mu       sync.Mutex
	personas map[string]map[string]any // resourceName → persona
	version  map[string]int            // resourceName → cuándo cambió
	grupos   map[string]map[string]any // resourceName → grupo (con memberResourceNames)
	n        int
	peticion []string
}

func (g *peopleFalso) miembros(rn string) []string {
	ms, _ := g.grupos[rn]["memberResourceNames"].([]string)
	return ms
}

func (g *peopleFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.peticion = append(g.peticion, r.Method+" "+r.URL.Path)
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/v1/")
	g.n++
	switch {
	case ruta == "people/me/connections":
		// Incremental: con la ficha, solo lo que cambió desde ella.
		desde := -1
		if t := r.URL.Query().Get("syncToken"); t != "" {
			desde = 0
			for _, c := range strings.TrimPrefix(t, "s") {
				desde = desde*10 + int(c-'0')
			}
		}
		var cs []any
		for rn, p := range g.personas {
			if g.version[rn] > desde {
				cs = append(cs, p)
			}
		}
		escribir(map[string]any{"connections": cs, "nextSyncToken": "s" + itoa(g.n)})
	case ruta == "people:createContact":
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		rn := "people/n" + itoa(g.n)
		p["resourceName"], p["etag"] = rn, "e"+itoa(g.n)
		g.personas[rn] = p
		g.version[rn] = g.n
		escribir(p)
	case ruta == "contactGroups" && r.Method == http.MethodGet:
		gs := make([]any, 0, 1+len(g.grupos))
		gs = append(gs, map[string]any{"resourceName": "contactGroups/myContacts", "name": "myContacts", "groupType": "SYSTEM_CONTACT_GROUP"})
		for _, x := range g.grupos {
			gs = append(gs, map[string]any{"resourceName": x["resourceName"], "name": x["name"], "groupType": "USER_CONTACT_GROUP", "etag": x["etag"]})
		}
		escribir(map[string]any{"contactGroups": gs})
	case ruta == "contactGroups" && r.Method == http.MethodPost:
		var c struct {
			Grupo map[string]any `json:"contactGroup"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		for _, x := range g.grupos {
			if x["name"] == c.Grupo["name"] {
				http.Error(w, `{"error":{"status":"ALREADY_EXISTS"}}`, http.StatusConflict)
				return
			}
		}
		rn := "contactGroups/g" + itoa(g.n)
		x := map[string]any{"resourceName": rn, "name": c.Grupo["name"], "etag": "ge" + itoa(g.n), "groupType": "USER_CONTACT_GROUP"}
		g.grupos[rn] = x
		escribir(x)
	case ruta == "contactGroups:batchGet":
		var rs []any
		for _, rn := range r.URL.Query()["resourceNames"] {
			if x := g.grupos[rn]; x != nil {
				rs = append(rs, map[string]any{"contactGroup": x})
			}
		}
		escribir(map[string]any{"responses": rs})
	case strings.HasPrefix(ruta, "contactGroups/") && strings.HasSuffix(ruta, "/members:modify"):
		rn := strings.TrimSuffix(ruta, "/members:modify")
		var c struct {
			Anadir []string `json:"resourceNamesToAdd"`
			Quitar []string `json:"resourceNamesToRemove"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		ms := slices.DeleteFunc(slices.Clone(g.miembros(rn)), func(m string) bool { return slices.Contains(c.Quitar, m) })
		for _, m := range c.Anadir {
			if !slices.Contains(ms, m) {
				ms = append(ms, m)
			}
		}
		g.grupos[rn]["memberResourceNames"] = ms
		escribir(map[string]any{})
	case strings.HasPrefix(ruta, "contactGroups/"):
		x := g.grupos[ruta]
		if x == nil {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			escribir(x)
		case http.MethodPut:
			var c struct {
				Grupo map[string]any `json:"contactGroup"`
			}
			_ = json.NewDecoder(r.Body).Decode(&c)
			if c.Grupo["etag"] != x["etag"] {
				http.Error(w, "etag", http.StatusBadRequest)
				return
			}
			x["name"], x["etag"] = c.Grupo["name"], "ge"+itoa(g.n)
			escribir(x)
		case http.MethodDelete:
			if r.URL.Query().Get("deleteContacts") != "false" {
				http.Error(w, "se llevaría los contactos", http.StatusBadRequest)
				return
			}
			delete(g.grupos, ruta)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		http.NotFound(w, r)
	}
}

type libretaPrueba struct {
	t     *testing.T
	repo  *libreta.Libreta
	s     *agenda.Sincronizador
	libro string
}

func (l *libretaPrueba) sinc() agenda.Resultado {
	l.t.Helper()
	res, err := l.s.Sincronizar(l.t.Context())
	if err != nil || len(res.Errores) > 0 {
		l.t.Fatalf("%v %v", err, res.Errores)
	}
	if res.Pendientes > 0 {
		l.t.Fatalf("pendientes: %+v", res)
	}
	l.libro = l.s.Colecciones()[0].ID
	return res
}

func (l *libretaPrueba) etiquetas() map[string]api.ContactGroup {
	l.t.Helper()
	gs, err := l.repo.Groups(l.t.Context(), l.libro)
	if err != nil {
		l.t.Fatal(err)
	}
	out := map[string]api.ContactGroup{}
	for _, g := range gs {
		if _, doble := out[g.Name]; doble {
			l.t.Fatalf("etiqueta repetida: %q", g.Name)
		}
		out[g.Name] = g
	}
	return out
}

func (l *libretaPrueba) uidDe(nombre string) string {
	l.t.Helper()
	ts, err := l.repo.TarjetasCrudas(l.t.Context(), l.libro)
	if err != nil {
		l.t.Fatal(err)
	}
	for _, x := range ts {
		if strings.Contains(x.Datos, nombre) {
			return x.UID
		}
	}
	l.t.Fatalf("sin %q", nombre)
	return ""
}

func persona(rn, nombre string) map[string]any {
	return map[string]any{"resourceName": rn, "etag": "e-" + rn, "names": []any{map[string]any{"displayName": nombre, "givenName": nombre}}}
}

// Las etiquetas de una libreta de Google son sus grupos de contactos, en los dos
// sentidos: bajan con sus miembros (los del sistema, no), las de aquí suben como grupos
// (crear, renombrar, cambiar miembros, borrar sin llevarse a nadie) y lo borrado allí se
// va de aquí. Una de aquí que se llama como una de allí que aún no se había visto se
// junta con ella: no sale dos veces.
func TestEtiquetasDeGoogle(t *testing.T) {
	g := &peopleFalso{personas: map[string]map[string]any{}, version: map[string]int{}, grupos: map[string]map[string]any{}}
	g.personas["people/c1"] = persona("people/c1", "Eva")
	g.personas["people/c2"] = persona("people/c2", "Luis")
	g.grupos["contactGroups/fam"] = map[string]any{"resourceName": "contactGroups/fam", "name": "Familia", "etag": "f1", "memberResourceNames": []string{"people/c1"}}
	srv := httptest.NewTLSServer(g)
	defer srv.Close()
	con := abrirContactos(t, t.TempDir())
	l := &libretaPrueba{t: t, repo: con, s: &agenda.Sincronizador{
		Remoto: agenda.GooglePeopleDe(srv.URL+"/v1", srv.Client()),
		Local:  agenda.LocalContactos{Repo: con}, Estado: filepath.Join(t.TempDir(), "g.json"), MarcaCopia: "(copia)",
	}}

	if res := l.sinc(); res.Bajados != 3 {
		t.Fatalf("bajada: %+v", res)
	}
	eva, luis := l.uidDe("Eva"), l.uidDe("Luis")
	es := l.etiquetas()
	if len(es) != 1 || !slices.Equal(es["Familia"].Members, []string{eva}) {
		t.Fatalf("etiquetas: %+v", es)
	}
	if res := l.sinc(); res.Bajados+res.Subidos+res.Borrados != 0 {
		t.Fatalf("en vacío: %+v", res)
	}

	// Aquí: «Familia» pasa a «Familia cercana» con Luis, y se crea «Clientes» con Luis.
	if _, err := con.RenameGroup(t.Context(), es["Familia"].ID, "Familia cercana", []string{eva, luis}); err != nil {
		t.Fatal(err)
	}
	cl, err := con.CreateGroup(t.Context(), l.libro, "Clientes")
	if err != nil {
		t.Fatal(err)
	}
	if err := con.BatchGroupMembers(t.Context(), cl.ID, []string{luis}, true); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Subidos != 2 {
		t.Fatalf("subida: %+v", res)
	}
	g.mu.Lock()
	fam := g.grupos["contactGroups/fam"]
	var clientes map[string]any
	for _, x := range g.grupos {
		if x["name"] == "Clientes" {
			clientes = x
		}
	}
	g.mu.Unlock()
	if fam["name"] != "Familia cercana" || !slices.Equal(g.miembros("contactGroups/fam"), []string{"people/c1", "people/c2"}) ||
		clientes == nil || !slices.Equal(g.miembros(clientes["resourceName"].(string)), []string{"people/c2"}) {
		t.Fatalf("en Google: %+v", g.grupos)
	}
	if res := l.sinc(); res.Bajados+res.Subidos+res.Borrados != 0 {
		t.Fatalf("tras subir, en vacío: %+v", res)
	}

	// Allí se borra «Clientes»: aquí se va.
	g.mu.Lock()
	delete(g.grupos, clientes["resourceName"].(string))
	g.mu.Unlock()
	if res := l.sinc(); res.Borrados != 1 {
		t.Fatalf("borrado allí: %+v", res)
	}
	if _, sigue := l.etiquetas()["Clientes"]; sigue {
		t.Fatal("«Clientes» sigue aquí")
	}

	// Aquí se crea «Equipo» con Eva mientras allí otro crea «Equipo» con Luis: una sola
	// etiqueta, con los dos.
	eq, err := con.CreateGroup(t.Context(), l.libro, "Equipo")
	if err != nil {
		t.Fatal(err)
	}
	if err := con.BatchGroupMembers(t.Context(), eq.ID, []string{eva}, true); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.grupos["contactGroups/eq"] = map[string]any{"resourceName": "contactGroups/eq", "name": "Equipo", "etag": "q1", "memberResourceNames": []string{"people/c2"}}
	g.mu.Unlock()
	l.sinc()
	l.sinc()
	es = l.etiquetas()
	m := slices.Clone(es["Equipo"].Members)
	slices.Sort(m)
	quiero := []string{eva, luis}
	slices.Sort(quiero)
	if !slices.Equal(m, quiero) || es["Equipo"].ID != eq.ID || !slices.Equal(g.miembros("contactGroups/eq"), []string{"people/c2", "people/c1"}) {
		t.Fatalf("juntadas: %+v %v", es, g.miembros("contactGroups/eq"))
	}

	// Aquí se borra «Familia cercana»: el grupo se va de Google y sus contactos se quedan.
	if err := con.DeleteGroup(t.Context(), es["Familia cercana"].ID); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Borrados != 1 {
		t.Fatalf("borrar aquí: %+v", res)
	}
	g.mu.Lock()
	_, sigue := g.grupos["contactGroups/fam"]
	n := len(g.personas)
	g.mu.Unlock()
	if sigue || n != 2 {
		t.Fatalf("en Google tras borrar: %v %d", sigue, n)
	}
}

// ----- Un Outlook de mentira, con categorías -----

type outlookFalso struct {
	mu        sync.Mutex
	base      string
	contactos map[string]map[string]any
	version   map[string]int // id → cuándo cambió
	n         int
}

func (o *outlookFalso) categorias(id string) []string {
	cs, _ := o.contactos[id]["categories"].([]any)
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.(string))
	}
	return out
}

func (o *outlookFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	switch {
	case ruta == "/me/contactFolders" && r.Method == http.MethodGet:
		escribir(map[string]any{"value": []any{}})
	case ruta == "/me/contactFolders" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		escribir(map[string]any{"id": "carpeta1", "displayName": c["displayName"]})
	case ruta == "/me/contacts/delta":
		desde := -1
		if t := r.URL.Query().Get("token"); t != "" {
			desde = 0
			for _, c := range t {
				desde = desde*10 + int(c-'0')
			}
		}
		var vs []any
		for id, c := range o.contactos {
			if o.version[id] > desde {
				vs = append(vs, c)
			}
		}
		escribir(map[string]any{"value": vs, "@odata.deltaLink": o.base + "/v1.0/me/contacts/delta?token=" + itoa(o.n)})
	case ruta == "/me/contacts" && r.Method == http.MethodGet:
		if r.URL.Query().Get("$select") != "id,categories" {
			http.Error(w, "select", http.StatusBadRequest)
			return
		}
		var vs []any
		for _, c := range o.contactos {
			vs = append(vs, map[string]any{"id": c["id"], "categories": c["categories"]})
		}
		escribir(map[string]any{"value": vs})
	case strings.HasPrefix(ruta, "/me/contacts/") && r.Method == http.MethodPatch:
		c := o.contactos[strings.TrimPrefix(ruta, "/me/contacts/")]
		if c == nil {
			http.NotFound(w, r)
			return
		}
		var cambio map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cambio)
		for k, v := range cambio {
			c[k] = v
		}
		o.n++
		c["@odata.etag"] = "W/\"" + itoa(o.n) + "\""
		o.version[c["id"].(string)] = o.n
		escribir(c)
	default:
		http.NotFound(w, r)
	}
}

// Las etiquetas de una libreta de Outlook son las categorías de sus contactos: bajan,
// las de aquí ponen su nombre en sus contactos (y lo cambian al renombrarlas), una sin
// nadie se queda aquí aunque allí no se vea, y la que nadie lleva ya allí se va de aquí.
func TestEtiquetasDeOutlook(t *testing.T) {
	o := &outlookFalso{version: map[string]int{}, contactos: map[string]map[string]any{
		"k1": {"id": "k1", "@odata.etag": "W/\"a\"", "displayName": "Marta", "givenName": "Marta", "categories": []any{"Familia", "Azul"}},
		"k2": {"id": "k2", "@odata.etag": "W/\"b\"", "displayName": "Pablo", "givenName": "Pablo", "categories": []any{}},
	}}
	srv := httptest.NewTLSServer(o)
	defer srv.Close()
	o.base = srv.URL
	con := abrirContactos(t, t.TempDir())
	l := &libretaPrueba{t: t, repo: con, s: &agenda.Sincronizador{
		Remoto: agenda.GraphContactosDe(srv.URL+"/v1.0", srv.Client()),
		Local:  agenda.LocalContactos{Repo: con}, Estado: filepath.Join(t.TempDir(), "o.json"), MarcaCopia: "(copia)",
	}}
	if res := l.sinc(); res.Bajados != 4 {
		t.Fatalf("bajada: %+v", res)
	}
	marta, pablo := l.uidDe("Marta"), l.uidDe("Pablo")
	es := l.etiquetas()
	if len(es) != 2 || !slices.Equal(es["Familia"].Members, []string{marta}) {
		t.Fatalf("etiquetas: %+v", es)
	}

	// Pablo entra en «Familia» y se crea «Vacía» sin nadie.
	if _, err := con.RenameGroup(t.Context(), es["Familia"].ID, "Familia", []string{marta, pablo}); err != nil {
		t.Fatal(err)
	}
	if _, err := con.CreateGroup(t.Context(), l.libro, "Vacía"); err != nil {
		t.Fatal(err)
	}
	if res := l.sinc(); res.Subidos != 2 {
		t.Fatalf("subida: %+v", res)
	}
	if !slices.Equal(o.categorias("k2"), []string{"Familia"}) || !slices.Equal(o.categorias("k1"), []string{"Familia", "Azul"}) {
		t.Fatalf("en Outlook: %v %v", o.categorias("k1"), o.categorias("k2"))
	}
	l.sinc() // los contactos que cambiaron de categoría bajan igual
	if res := l.sinc(); res.Bajados+res.Subidos+res.Borrados != 0 {
		t.Fatalf("en vacío: %+v", res)
	}
	if _, sigue := l.etiquetas()["Vacía"]; !sigue {
		t.Fatal("la etiqueta sin nadie se borró por no verse en Outlook")
	}

	// Se renombra «Familia» a «Casa»: cambia en los dos contactos.
	if _, err := con.RenameGroup(t.Context(), l.etiquetas()["Familia"].ID, "Casa", []string{marta, pablo}); err != nil {
		t.Fatal(err)
	}
	l.sinc()
	if !slices.Equal(o.categorias("k1"), []string{"Azul", "Casa"}) || !slices.Equal(o.categorias("k2"), []string{"Casa"}) {
		t.Fatalf("renombrada: %v %v", o.categorias("k1"), o.categorias("k2"))
	}
	l.sinc()

	// Allí nadie lleva ya «Azul»: aquí se va.
	o.mu.Lock()
	o.contactos["k1"]["categories"] = []any{"Casa"}
	o.n++
	o.version["k1"] = o.n
	o.mu.Unlock()
	if res := l.sinc(); res.Borrados != 1 {
		t.Fatalf("borrada allí: %+v", res)
	}
	es = l.etiquetas()
	if _, sigue := es["Azul"]; sigue || len(es) != 2 {
		t.Fatalf("tras borrar allí: %+v", es)
	}

	// Una libreta nueva en la cuenta: una carpeta de contactos.
	cc, ok := agenda.GraphContactosDe(srv.URL+"/v1.0", srv.Client()).(agenda.CreaColecciones)
	if !ok {
		t.Fatal("Outlook sabe crear libretas")
	}
	if c, err := cc.CrearColeccion(t.Context(), " Proveedores ", ""); err != nil || c.ID != "carpeta1" || c.Nombre != "Proveedores" {
		t.Fatalf("carpeta: %+v %v", c, err)
	}
	if _, ok := agenda.GooglePeopleDe(srv.URL, srv.Client()).(agenda.CreaColecciones); ok {
		t.Fatal("Google no tiene más libretas que una")
	}
}
