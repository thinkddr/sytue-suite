// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

const tokenDeBases = "sbc_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// La fuente «Sytue Bases» de los informes, contra un Sytue Bases de mentira que se comporta
// como el de verdad en lo que importa: exige el token, el consumidor y la organización, y
// deja de contestar cuando se revoca. Cada comprobación se ha visto fallar quitándola.
func TestFuenteBasesDeLosInformes(t *testing.T) {
	var revocada atomic.Bool
	var lecturas atomic.Int32
	bases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lecturas.Add(1)
		if revocada.Load() || r.Header.Get("Authorization") != "Bearer "+tokenDeBases ||
			r.Header.Get("X-Sytue-Consumer") != "erp" || r.Header.Get("X-Sytue-Tenant") != "acme" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"columns":[{"name":"Cliente","type":"text"},{"name":"Importe","type":"number"}],
			"rows":[["Aurora",120],["Bea",85.5]],
			"provenance":{"source":"sytue-bases","consumer":"erp","connection_id":"c1","version":3,
			 "base_name":"Tienda","table_name":"Pedidos","view_name":"Grandes","granted_by":"ana",
			 "granted_at":"2026-10-01T10:00:00Z","expires_at":"2026-12-30T10:00:00Z","read_at":"2026-10-05T08:00:00Z"}}`))
	}))
	defer bases.Close()

	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	hs := manejadoresCompletos(t, store.New(harness.Pool))
	hs.cfg.BasesBaseURL = bases.URL
	var err error
	if hs.cifra, err = keycrypto.New([]byte(strings.Repeat("k", 32))); err != nil {
		t.Fatal(err)
	}
	jefa, miembro := []string{"owner"}, []string{"member"}
	guardar := func(cuerpo string) int {
		return comoPersona(t, "jefa", jefa, http.MethodPost, "/api/v1/conexiones/bases", cuerpo,
			map[string]string{"proveedor": "bases"}, http.HandlerFunc(hs.GuardarConexion)).Code
	}
	const informe = `{"informe":{"titulo":"Pedidos","partes":[{"titulo":"Todo","vista":"tabla","consulta":{"fuente":"bases","columnas":["Cliente","Importe"]}}]}}`
	type tablaLeida struct {
		Filas       [][]any        `json:"filas"`
		Error       string         `json:"error"`
		Procedencia map[string]any `json:"procedencia"`
	}
	datos := func(sub string, roles []string) (tabla tablaLeida) {
		w := comoPersona(t, sub, roles, http.MethodPost, "/api/v1/informes/datos", informe, nil, http.HandlerFunc(hs.DatosInforme))
		var r struct {
			Datos []json.RawMessage `json:"datos"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != http.StatusOK || len(r.Datos) != 1 {
			t.Fatalf("datos de %s: %d %s", sub, w.Code, w.Body)
		}
		_ = json.Unmarshal(r.Datos[0], &tabla)
		return tabla
	}

	// Lo que no es un token de conexión no se guarda ni sale hacia Bases.
	antes := lecturas.Load()
	if c := guardar(`{"secreto":"eyJhbGciOi.un.jwt","activo":true}`); c != http.StatusUnprocessableEntity {
		t.Errorf("guardar un secreto que no es de conexión: %d", c)
	}
	if lecturas.Load() != antes {
		t.Error("un secreto que no es token de conexión salió hacia Sytue Bases")
	}
	// Ni uno que Bases rechaza (revocado, de otro producto o de otra organización).
	revocada.Store(true)
	if c := guardar(`{"secreto":"` + tokenDeBases + `","activo":true}`); c != http.StatusUnprocessableEntity {
		t.Errorf("guardar un token que Bases rechaza: %d", c)
	}
	revocada.Store(false)
	if c := guardar(`{"secreto":"` + tokenDeBases + `","activo":true,"config":{"columnas":[{"clave":"colada","tipo":"text"}]}}`); c != http.StatusNoContent {
		t.Fatalf("guardar: %d", c)
	}

	// El token no vuelve a salir de aquí, y la config la escribió el servidor.
	w := comoPersona(t, "jefa", jefa, http.MethodGet, "/api/v1/conexiones", "", nil, http.HandlerFunc(hs.ListarConexiones))
	if strings.Contains(w.Body.String(), tokenDeBases) || strings.Contains(w.Body.String(), "colada") ||
		!strings.Contains(w.Body.String(), `"vista":"Grandes"`) || !strings.Contains(w.Body.String(), "Tienda › Pedidos › Grandes") {
		t.Errorf("listado de conexiones: %s", w.Body)
	}
	var sellado string
	err = harness.Pool.InTenantTx(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"}), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT secreto_enc FROM conexiones WHERE proveedor = 'bases'`).Scan(&sellado)
	})
	if err != nil || sellado == "" || strings.Contains(sellado, tokenDeBases) {
		t.Errorf("el token no está sellado en la base: %q (%v)", sellado, err)
	}

	// Administración lee la vista, con la procedencia de ESTA lectura.
	tabla := datos("jefa", jefa)
	if tabla.Error != "" || len(tabla.Filas) != 2 || tabla.Filas[0][0] != "Aurora" || tabla.Filas[1][1] != 85.5 {
		t.Fatalf("lectura de administración: %+v", tabla)
	}
	p := tabla.Procedencia
	if p["base"] != "Tienda" || p["vista"] != "Grandes" || p["concedida_por"] != "ana" || p["version"] != float64(3) || p["concedida_el"] != "2026-10-01T10:00:00Z" {
		t.Errorf("procedencia: %+v", p)
	}

	// Un miembro no ve la fuente ni la lee, aunque mande su clave: el token es de la
	// organización, no suyo, y Bases no puede decidir por él.
	antes = lecturas.Load()
	if tabla := datos("luis", miembro); tabla.Error == "" || len(tabla.Filas) != 0 || tabla.Procedencia != nil {
		t.Errorf("un miembro lee la vista de Sytue Bases: %+v", tabla)
	}
	if lecturas.Load() != antes {
		t.Error("la petición de un miembro llegó a Sytue Bases")
	}

	// Revocar en Bases corta aquí en la siguiente lectura: no hay caché.
	revocada.Store(true)
	if tabla := datos("jefa", jefa); tabla.Error != "no_disponible" || len(tabla.Filas) != 0 {
		t.Errorf("tras revocar: %+v", tabla)
	}

	// Otra organización no hereda la conexión.
	revocada.Store(false)
	if w := llamar(t, "globex", http.MethodGet, "/api/v1/conexiones", "", "", nil, hs.ListarConexiones); strings.Contains(w.Body.String(), "bases") {
		t.Errorf("globex ve la conexión de acme: %s", w.Body)
	}
}
