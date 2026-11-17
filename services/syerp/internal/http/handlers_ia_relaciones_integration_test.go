// SPDX-License-Identifier: Apache-2.0
//go:build integration

package syerphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestRellenarFichaRespetaPermisosDeRelaciones(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	crear := func(tenant, modelo, nombre string) store.Registro {
		t.Helper()
		m, ok := hs.cat.Modelo(modelo)
		if !ok {
			t.Fatal("model missing", modelo)
		}
		data := map[string]any{"nombre": nombre}
		if modelo == "contacto" {
			data["tipo"] = "persona"
		}
		reg, err := st.Crear(tenancy.Into(t.Context(), tenancy.Tenant{Slug: tenant}), m, data, nombre, "admin")
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}
	contacto := crear("acme", "contacto", "Ana Privada")
	empleado := crear("acme", "empleado", "Ana Empleada")
	facturaModelo, ok := hs.cat.Modelo("factura_venta")
	if !ok {
		t.Fatal("missing invoice model")
	}
	factura, err := st.Crear(ctx, facturaModelo, map[string]any{"cliente": contacto.ID, "fecha": "2026-11-03", "base": 100}, "Factura Permitida", "admin")
	if err != nil {
		t.Fatal(err)
	}
	// A newer same-name record in another tenant must never be returned.
	ajeno := crear("otro", "contacto", "Ana Otro Tenant")
	poner := func(niveles map[string]string) {
		t.Helper()
		if err := st.GuardarPermisos(ctx, "luis", "luis@acme.test", "Luis", niveles, nil, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	responder := func(antes func() error) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/assist" {
				http.Error(w, "unexpected route", http.StatusNotFound)
				return
			}
			if antes != nil {
				if err := antes(); err != nil {
					http.Error(w, "fixture permission failure", http.StatusInternalServerError)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"text": `{"cliente":"Ana","proveedor":"Ana","empleado":"Ana","rectifica":"Factura Permitida","notas":"texto conservado"}`})
		}))
	}
	server := responder(nil)
	defer server.Close()
	hs.ia = asistente.Nuevo(server.URL, 5*time.Second)
	type respuesta struct {
		Data      map[string]any    `json:"data"`
		Etiquetas map[string]string `json:"etiquetas"`
	}
	consultar := func(modelo string, roles []string) respuesta {
		t.Helper()
		path := "/api/v1/" + modelo + "/ia/rellenar"
		w := comoPersona(t, "luis", roles, http.MethodPost, path, `{"texto":"Ficha de Ana"}`,
			map[string]string{"modelo": modelo}, hs.exigirModelo(http.HandlerFunc(hs.RellenarFicha)))
		if w.Code != http.StatusOK {
			t.Fatalf("fill failed: %d %s", w.Code, w.Body.String())
		}
		var out respuesta
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Data["notas"] != "texto conservado" {
			t.Fatal("unrelated permitted fields were lost", out.Data)
		}
		if strings.Contains(w.Body.String(), ajeno.ID) || strings.Contains(w.Body.String(), "Ana Otro Tenant") {
			t.Fatal("foreign tenant leaked")
		}
		return out
	}
	comprobarAusente := func(out respuesta, campo string, hidden store.Registro) {
		t.Helper()
		if _, ok := out.Data[campo]; ok || out.Etiquetas[hidden.ID] != "" {
			t.Fatal("hidden relation disclosed", campo, out)
		}
	}
	comprobarPresente := func(out respuesta, campo string, reg store.Registro) {
		t.Helper()
		if out.Data[campo] != reg.ID || out.Etiquetas[reg.ID] != reg.Etiqueta {
			t.Fatal("readable relation omitted", campo, out)
		}
	}

	t.Run("factura sin lectura de contactos", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Ninguno})
		venta := consultar("factura_venta", []string{"member"})
		comprobarAusente(venta, "cliente", contacto)
		comprobarPresente(venta, "rectifica", factura)
		comprobarAusente(consultar("factura_compra", []string{"member"}), "proveedor", contacto)
	})
	t.Run("factura con lectura de contactos", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Lectura})
		comprobarPresente(consultar("factura_venta", []string{"member"}), "cliente", contacto)
	})
	t.Run("owner mantiene acceso", func(t *testing.T) {
		poner(map[string]string{"contactos": permisos.Ninguno})
		comprobarPresente(consultar("factura_venta", []string{"owner"}), "cliente", contacto)
	})
	t.Run("vehiculo respeta rrhh cerrado por defecto", func(t *testing.T) {
		poner(map[string]string{})
		comprobarAusente(consultar("vehiculo", []string{"member"}), "empleado", empleado)
		poner(map[string]string{"rrhh": permisos.Lectura})
		comprobarPresente(consultar("vehiculo", []string{"member"}), "empleado", empleado)
	})
	t.Run("comodin de IA no salta permisos", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Ninguno})
		wildcard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"text": `{"cliente":"%","notas":"texto conservado"}`})
		}))
		defer wildcard.Close()
		hs.ia = asistente.Nuevo(wildcard.URL, 5*time.Second)
		comprobarAusente(consultar("factura_venta", []string{"member"}), "cliente", contacto)
		hs.ia = asistente.Nuevo(server.URL, 5*time.Second)
	})
	t.Run("revocacion durante llamada de IA", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Lectura})
		revocar := responder(func() error {
			return st.GuardarPermisos(ctx, "luis", "luis@acme.test", "Luis", map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Ninguno}, nil, "admin")
		})
		defer revocar.Close()
		hs.ia = asistente.Nuevo(revocar.URL, 5*time.Second)
		comprobarAusente(consultar("factura_venta", []string{"member"}), "cliente", contacto)
	})
	t.Run("revocacion de origen durante llamada de IA", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario, "contactos": permisos.Lectura})
		revocar := responder(func() error {
			return st.GuardarPermisos(ctx, "luis", "luis@acme.test", "Luis", map[string]string{"facturacion": permisos.Ninguno, "contactos": permisos.Lectura}, nil, "admin")
		})
		defer revocar.Close()
		hs.ia = asistente.Nuevo(revocar.URL, 5*time.Second)
		w := comoPersona(t, "luis", []string{"member"}, http.MethodPost, "/api/v1/factura_venta/ia/rellenar", `{"texto":"Ficha de Ana"}`,
			map[string]string{"modelo": "factura_venta"}, hs.exigirModelo(http.HandlerFunc(hs.RellenarFicha)))
		if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), contacto.ID) || strings.Contains(w.Body.String(), factura.ID) {
			t.Fatalf("revoked source must return 403 without relations: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("sin relaciones revalida origen", func(t *testing.T) {
		poner(map[string]string{"facturacion": permisos.Usuario})
		sinRelaciones := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"text": `{"notas":"texto conservado"}`})
		}))
		defer sinRelaciones.Close()
		hs.ia = asistente.Nuevo(sinRelaciones.URL, 5*time.Second)
		out := consultar("factura_venta", []string{"member"})
		if len(out.Etiquetas) != 0 {
			t.Fatal("unexpected relation", out)
		}
		revocar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if err := st.GuardarPermisos(ctx, "luis", "luis@acme.test", "Luis", map[string]string{"facturacion": permisos.Ninguno}, nil, "admin"); err != nil {
				http.Error(w, "fixture permission failure", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"text": `{"notas":"texto conservado"}`})
		}))
		defer revocar.Close()
		hs.ia = asistente.Nuevo(revocar.URL, 5*time.Second)
		w := comoPersona(t, "luis", []string{"member"}, http.MethodPost, "/api/v1/factura_venta/ia/rellenar", `{"texto":"Ficha de Ana"}`,
			map[string]string{"modelo": "factura_venta"}, hs.exigirModelo(http.HandlerFunc(hs.RellenarFicha)))
		if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "texto conservado") {
			t.Fatalf("revoked source without relations: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("fallo de permisos no abre el defecto", func(t *testing.T) {
		// Origin and destination both default open, but this member has an
		// explicit denial. Change only this disposable test database after the
		// route's original permission check and before relation resolution.
		poner(map[string]string{"contactos": permisos.Ninguno})
		privileged := harness.PoolSinRLS(t)
		var broken atomic.Bool
		restore := func() error {
			if !broken.Load() {
				return nil
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return privileged.InTx(cleanup, func(tx pgx.Tx) error {
				if _, err := tx.Exec(cleanup, `DROP VIEW permisos`); err != nil {
					return err
				}
				_, err := tx.Exec(cleanup, `ALTER TABLE permisos_ai_fixture RENAME TO permisos`)
				return err
			})
		}
		defer func() {
			if err := restore(); err != nil {
				t.Error("restore fixture:", err)
			}
		}()
		server := responder(func() error {
			err := privileged.InTx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `ALTER TABLE permisos RENAME TO permisos_ai_fixture`); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `CREATE VIEW permisos AS SELECT * FROM permisos_ai_fixture WHERE 1 / 0 = 0`)
				return err
			})
			if err == nil {
				broken.Store(true)
			}
			return err
		})
		defer server.Close()
		hs.ia = asistente.Nuevo(server.URL, 5*time.Second)
		w := comoPersona(t, "luis", []string{"member"}, http.MethodPost, "/api/v1/pedido_venta/ia/rellenar", `{"texto":"Ficha de Ana"}`,
			map[string]string{"modelo": "pedido_venta"}, hs.exigirModelo(http.HandlerFunc(hs.RellenarFicha)))
		if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), contacto.ID) || strings.Contains(w.Body.String(), "texto conservado") {
			t.Fatalf("permission lookup failure must not authorize output: %d %s", w.Code, w.Body.String())
		}
	})
}
