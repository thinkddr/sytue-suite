// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// compararConOro compara `obtenido` con testdata/<fichero>. Con ACTUALIZAR_ORO=1
// lo escribe en vez de compararlo.
func compararConOro(t *testing.T, fichero, obtenido string) {
	t.Helper()
	ruta := filepath.Join("testdata", fichero)
	if os.Getenv("ACTUALIZAR_ORO") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ruta, []byte(obtenido), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	oro, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("falta %s (se genera con ACTUALIZAR_ORO=1): %v", ruta, err)
	}
	if string(oro) != obtenido {
		t.Fatalf("%s ha cambiado.\n--- oro\n%s\n--- ahora\n%s", ruta, oro, obtenido)
	}
}

// facturaImpresa prepara una organización con VERI*FACTU, emite la factura que
// devuelve `crearFactura` y devuelve su impreso.
func facturaImpresa(t *testing.T, crearFactura func(ctx context.Context, hs *Handlers, st *store.Store, cliente string) string) string {
	t.Helper()
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	st.Version = "v-test"
	hs := manejadoresCompletos(t, st)
	hs.verifactu = &verifactu.Cliente{}
	w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
		`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"28013","ciudad":"Madrid","pais":"ES","forma":"sl"}`,
		nil, hs.GuardarOrganizacion)
	if w.Code != http.StatusOK {
		t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
	}
	if w := llamar(t, "acme", http.MethodPost, "/api/v1/verifactu/activar", "", "", nil, hs.ActivarVerifactu); w.Code != http.StatusOK {
		t.Fatalf("activar: %d %s", w.Code, w.Body.String())
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	contacto, _ := hs.cat.Modelo("contacto")
	cliente := crear(ctx, t, st, contacto,
		`{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","direccion":"Calle Sol 2","cp":"28004","ciudad":"Madrid"}`)
	id := crearFactura(ctx, hs, st, cliente.ID)
	params := map[string]string{"modelo": "factura_venta", "id": id}
	if w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/factura_venta/"+id+"/estado",
		`{"estado":"confirmado"}`, params, http.HandlerFunc(hs.CambiarEstado)); w.Code != http.StatusOK {
		t.Fatalf("emitir: %d %s", w.Code, w.Body.String())
	}
	w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+id+"/imprimir", "", "", params, hs.Imprimir)
	if w.Code != http.StatusOK {
		t.Fatalf("imprimir: %d %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// Una factura SIN líneas se imprime byte a byte igual que antes de que existieran
// las líneas. El oro se generó con el código anterior a las líneas.
func TestImprimirSinLineasComoAntes(t *testing.T) {
	impreso := facturaImpresa(t, func(ctx context.Context, hs *Handlers, st *store.Store, cliente string) string {
		factura, _ := hs.cat.Modelo("factura_venta")
		return crear(ctx, t, st, factura,
			`{"cliente":"`+cliente+`","fecha":"2026-09-12","base":100000,"iva":21000,"total":121000,"notas":"Pan de septiembre"}`).ID
	})
	compararConOro(t, "factura-sin-lineas.html", impreso)
}

// facturaConLineas crea la factura y sus líneas. `lineas` son objetos JSON sin el
// campo `factura`, que se añade aquí.
func facturaConLineas(ctx context.Context, t *testing.T, hs *Handlers, st *store.Store, cabecera string, lineas ...string) string {
	t.Helper()
	factura, _ := hs.cat.Modelo("factura_venta")
	linea, _ := hs.cat.Modelo("linea_factura")
	id := crear(ctx, t, st, factura, cabecera).ID
	for _, l := range lineas {
		crear(ctx, t, st, linea, `{"factura":"`+id+`",`+strings.TrimPrefix(l, "{"))
	}
	return id
}

// Pan al 4 % y vino al 21 %: la simplificada, como un tique (precios con IVA, sin
// cliente); la completa, con el precio unitario sin IVA y la base de cada línea.
// Las dos, con la base y la cuota de cada tipo.
func TestImprimirConLineasAl4YAl21(t *testing.T) {
	pan := `{"descripcion":"Hogaza de masa madre","cantidad":"3","precio_con_iva":250,"tipo_iva":"4"}`
	vino := `{"descripcion":"Vino tinto","cantidad":"1.5","precio_con_iva":1210,"tipo_iva":"21"}`
	for _, c := range []struct{ oro, tipo string }{
		{"factura-simplificada-4-21.html", `,"tipo_factura":"F2","notas":"Pedido P-000123"`},
		{"factura-completa-4-21.html", ""},
	} {
		t.Run(c.oro, func(t *testing.T) {
			impreso := facturaImpresa(t, func(ctx context.Context, hs *Handlers, st *store.Store, cliente string) string {
				// 7,50 € al 4 % (7,21 + 0,29) y 18,15 € al 21 % (15,00 + 3,15).
				return facturaConLineas(ctx, t, hs, st,
					`{"cliente":"`+cliente+`","fecha":"2026-09-20","base":2221,"iva":344,"total":2565`+c.tipo+`}`, pan, vino)
			})
			compararConOro(t, c.oro, impreso)
		})
	}
}

// Facturae conserva varios tipos reales; UBL conserva su limitación a uno.
// Ningún formato exporta una simplificada sin comprador identificado.
func TestEfacturaSinSimplificadasYConTiposReales(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	if w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
		`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"28013","ciudad":"Madrid","pais":"ES","forma":"sl"}`,
		nil, hs.GuardarOrganizacion); w.Code != http.StatusOK {
		t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	contacto, _ := hs.cat.Modelo("contacto")
	cliente := crear(ctx, t, st, contacto,
		`{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","direccion":"Calle Sol 2","cp":"28004","ciudad":"Madrid","pais":"ES"}`).ID
	pan := `{"descripcion":"Pan","cantidad":"1","precio_con_iva":1040,"tipo_iva":"4"}`
	vino := `{"descripcion":"Vino","cantidad":"1","precio_con_iva":1210,"tipo_iva":"21"}`
	casos := map[string]struct {
		id       string
		facturae bool
		ubl      bool
	}{
		"simplificada": {facturaConLineas(ctx, t, hs, st, `{"cliente":"`+cliente+`","fecha":"2026-09-20","tipo_factura":"F2","base":1000,"iva":210,"total":1210}`, vino), false, false},
		"varios tipos": {facturaConLineas(ctx, t, hs, st, `{"cliente":"`+cliente+`","fecha":"2026-09-20","base":2000,"iva":250,"total":2250}`, pan, vino), true, false},
		"un solo tipo": {facturaConLineas(ctx, t, hs, st, `{"cliente":"`+cliente+`","fecha":"2026-09-20","base":1000,"iva":210,"total":1210}`, vino), true, true},
	}
	for caso, c := range casos {
		params := map[string]string{"modelo": "factura_venta", "id": c.id}
		for _, formato := range []struct {
			handler func(http.ResponseWriter, *http.Request)
			admite  bool
		}{{hs.Facturae, c.facturae}, {hs.UBL, c.ubl}} {
			w := llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+c.id+"/efactura", "", "", params, formato.handler)
			if (w.Code == http.StatusOK) != formato.admite {
				t.Errorf("%s: %d %s", caso, w.Code, w.Body.String())
			}
		}
	}
}
