// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// tablaDePrueba es un driver de datos que no sale a la red: contesta una tabla y, al
// buscar, un resultado con el id de la conexión. Basta para ver QUIÉN llega a él.
type tablaDePrueba struct{}

func (tablaDePrueba) Kind() string { return "prueba-tabla" }
func (tablaDePrueba) Test(context.Context, connector.Instance, connector.Runtime) error {
	return nil
}

func (tablaDePrueba) Query(context.Context, connector.Instance, connector.Runtime, json.RawMessage) (connector.Table, error) {
	return connector.Table{Columns: []connector.Column{{Name: "importe", Type: "number"}}, Rows: [][]any{{42}}}, nil
}

func (tablaDePrueba) Search(_ context.Context, inst connector.Instance, _ connector.Runtime, _ string, _ int) ([]connector.ExternalHit, error) {
	return []connector.ExternalHit{{Title: inst.ID, URL: "https://datos.example/" + inst.ID, Source: inst.TemplateKey}}, nil
}

// almacen son dos conexiones de datos de «acme»: la que crea Ana con la pasarela nueva
// y una de antes de la lista, sin creador (como quedan las que ya había).
type almacen struct {
	st             *store.Store
	srv            *httptest.Server
	deAna, deAntes string
}

// quien es la sesión con la que se pide: lo que dejarían RequireAuth y RequireTenant.
type quien struct {
	sub    string
	roles  []string
	grupos []string
}

var (
	ana   = quien{sub: "ana", roles: []string{"member"}} // la creó; ya no administra
	bea   = quien{sub: "bea", roles: []string{"member"}}
	carla = quien{sub: "carla", roles: []string{"admin"}}
	dani  = quien{sub: "dani", roles: []string{"member"}, grupos: []string{"finanzas"}}
)

func montarAlmacen(t *testing.T, rutas func(h *Handlers, r chi.Router)) *almacen {
	t.Helper()
	pg := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()
	if _, err := pg.Pool.Exec(ctx,
		`INSERT INTO app_templates (key, manifest, tier, enabled) VALUES ('prueba-datos', $1::jsonb, 'standard', true)`,
		`{"name": "Datos", "description": "d", "color": "#123456", "icon": "<svg/>",
		  "identity": {"kind": "none"}, "launch": {"kind": "none"}, "data": "prueba-tabla"}`); err != nil {
		t.Fatalf("plantilla: %v", err)
	}
	k := make([]byte, keycrypto.KeyLen)
	_, _ = rand.Read(k)
	cifrador, err := keycrypto.New(k)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(pg.Pool, cifrador)
	acme := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	crear := func(c context.Context, nombre string) string {
		inst, err := st.CreateInstance(c, "prueba-datos", nombre, json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("instancia %s: %v", nombre, err)
		}
		if err := st.SetStatus(acme, inst.ID, domain.StatusActive); err != nil {
			t.Fatal(err)
		}
		return inst.ID
	}
	a := &almacen{st: st}
	// Ana administraba cuando la creó: crear exige owner/admin en el router de verdad.
	a.deAna = crear(authn.Into(acme, authn.Claims{Subject: "ana", Tenant: "acme", Roles: []string{"admin"}}), "Almacén de Ana")
	a.deAntes = crear(acme, "Almacén de antes")

	h := &Handlers{
		store:         st,
		registry:      connector.NewRegistry(tablaDePrueba{}),
		egressTimeout: 5 * time.Second,
		fondo:         ctx,
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var c authn.Claims
			_ = json.Unmarshal([]byte(r.Header.Get("X-Prueba-Quien")), &c)
			r = r.WithContext(tenancy.Into(authn.Into(r.Context(), c), tenancy.Tenant{Slug: "acme"}))
			next.ServeHTTP(w, r)
		})
	})
	r.Post("/instances/{id}/data/query", h.QueryData)
	r.Get("/search", h.Search)
	if rutas != nil {
		rutas(h, r)
	}
	a.srv = httptest.NewServer(r)
	t.Cleanup(a.srv.Close)
	return a
}

// pedir hace la petición como `q` y devuelve el estado y el cuerpo.
func (a *almacen) pedir(t *testing.T, q quien, metodo, ruta string, cuerpo any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(cuerpo)
	req, err := http.NewRequest(metodo, a.srv.URL+ruta, strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	sesion, _ := json.Marshal(authn.Claims{Subject: q.sub, Tenant: "acme", Roles: q.roles, Groups: q.grupos})
	req.Header.Set("X-Prueba-Quien", string(sesion))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, crudo
}

// consulta dice qué le pasa a `q` al consultar la conexión `id`: «ok» si llega la
// tabla, el código de error si no.
func (a *almacen) consulta(t *testing.T, q quien, id string) string {
	t.Helper()
	estado, cuerpo := a.pedir(t, q, http.MethodPost, "/instances/"+id+"/data/query", map[string]any{})
	if estado == http.StatusOK {
		return "ok"
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(cuerpo, &e)
	return e.Error.Code
}

// busca devuelve las conexiones (por id) que la búsqueda federada consulta para `q`.
func (a *almacen) busca(t *testing.T, q quien) []string {
	t.Helper()
	estado, cuerpo := a.pedir(t, q, http.MethodGet, "/search?q=importe", nil)
	if estado != http.StatusOK {
		t.Fatalf("buscar como %s = %d %s", q.sub, estado, cuerpo)
	}
	var out struct {
		Hits []connector.ExternalHit `json:"hits"`
	}
	if err := json.Unmarshal(cuerpo, &out); err != nil {
		t.Fatal(err)
	}
	titulos := make([]string, 0, len(out.Hits))
	for _, h := range out.Hits {
		titulos = append(titulos, h.Title)
	}
	return titulos
}

// NEG-06. Una conexión sale con la credencial de la ORGANIZACIÓN, así que no la consulta
// cualquier miembro: solo los administradores y quien la creó (y quien esté en su
// lista, ver lectores_integration_test.go). Las conexiones de antes de la lista no
// tienen creador y se quedan para los administradores. La búsqueda federada del Hub,
// igual: lo que no se puede consultar no participa.
func TestUnMiembroNoConsultaConLaCredencialDeLaOrganizacion(t *testing.T) {
	a := montarAlmacen(t, nil)
	casos := []struct {
		quien    quien
		conexion string
		quiero   string
	}{
		{bea, a.deAna, "connection_not_allowed"},
		{bea, a.deAntes, "connection_not_allowed"},
		{ana, a.deAna, "ok"}, // la creó, aunque ya no administre
		{ana, a.deAntes, "connection_not_allowed"},
		{carla, a.deAna, "ok"},
		{carla, a.deAntes, "ok"},
	}
	for _, c := range casos {
		if got := a.consulta(t, c.quien, c.conexion); got != c.quiero {
			t.Errorf("%s consulta %s: %q, quiero %q", c.quien.sub, c.conexion, got, c.quiero)
		}
	}
	if got := a.busca(t, bea); len(got) != 0 {
		t.Errorf("la búsqueda de Bea consulta %v; no puede consultar ninguna", got)
	}
	if got := a.busca(t, ana); len(got) != 1 || got[0] != a.deAna {
		t.Errorf("la búsqueda de Ana consulta %v; solo la suya", got)
	}
	if got := a.busca(t, carla); len(got) != 2 {
		t.Errorf("la búsqueda de Carla consulta %v; administra: las dos", got)
	}
}
