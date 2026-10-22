// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// extractoN43 escribe un Norma 43 de una cuenta con los movimientos dados
// (importe en céntimos, concepto común, referencia y texto) y sus totales.
func extractoN43(movs ...[4]string) string {
	var b strings.Builder
	linea := func(s string) { fmt.Fprintf(&b, "%-80s\r\n", s) }
	linea("11" + "2100" + "0001" + "0123456789" + "260901" + "260930" + "2" + "00000000000000" + "978" + "3" + "AUTONOMA DE PRUEBA")
	var nDebe, nHaber int
	var debe, haber int64
	for _, m := range movs {
		var c int64
		_, _ = fmt.Sscan(m[0], &c)
		clave := "2"
		if c < 0 {
			clave, c = "1", -c
			nDebe++
			debe += c
		} else {
			nHaber++
			haber += c
		}
		linea("22" + "    " + "0001" + "260915" + "260915" + m[1] + "000" + clave + fmt.Sprintf("%014d", c) + "0000000000" + "000000000000" + fmt.Sprintf("%-16s", m[2]))
		linea("23" + "01" + fmt.Sprintf("%-38s", m[3]))
	}
	clave, final := "2", haber-debe
	if final < 0 {
		clave, final = "1", -final
	}
	linea("33" + "2100" + "0001" + "0123456789" + fmt.Sprintf("%05d%014d%05d%014d", nDebe, debe, nHaber, haber) + clave + fmt.Sprintf("%014d", final) + "978")
	linea("88" + "999999999999999999" + fmt.Sprintf("%06d", 2+2*len(movs)))
	return b.String()
}

func TestBancoNorma43(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)

	w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
		`{"nombre":"Laura Gómez","nif":"12345678Z","cp":"46001","pais":"ES","forma":"autonomo","condiciones":{"iva":true}}`,
		nil, hs.GuardarOrganizacion)
	if w.Code != http.StatusOK {
		t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
	}

	// Una factura emitida y confirmada, que el cliente paga citando su número.
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	contacto, _ := hs.cat.Modelo("contacto")
	fv, _ := hs.cat.Modelo("factura_venta")
	data, etiqueta, err := domain.Validar(contacto, json.RawMessage(`{"nombre":"Bolt España SL","tipo":"organizacion"}`))
	if err != nil {
		t.Fatal(err)
	}
	cliente, err := st.Crear(ctx, contacto, data, etiqueta, "ada")
	if err != nil {
		t.Fatal(err)
	}
	data, etiqueta, err = domain.Validar(fv, json.RawMessage(fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-01","base":100000,"iva":21000,"total":121000}`, cliente.ID)))
	if err != nil {
		t.Fatal(err)
	}
	factura, err := st.Crear(ctx, fv, data, etiqueta, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Asentar(ctx, fv, factura.ID, "ada"); err != nil {
		t.Fatal(err)
	}

	n43 := extractoN43(
		[4]string{"121000", "04", factura.Ref, "TRANSF DE BOLT ESPANA SL"},
		[4]string{"-30000", "03", "", "AGENCIA TRIBUTARIA MOD 303"},
		[4]string{"-900", "04", "", "BIZUM A PEPE"},
	)
	importar := func() resultadoN43 {
		t.Helper()
		w := llamar(t, "acme", http.MethodPost, "/api/v1/linea_extracto/importar", "text/plain", n43,
			map[string]string{"modelo": "linea_extracto"}, hs.Importar)
		var res resultadoN43
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &res) != nil {
			t.Fatalf("importar: %d %s", w.Code, w.Body.String())
		}
		return res
	}
	if res := importar(); res.Nuevos != 3 || res.Casados != 1 || res.Explicados != 1 || res.Pendientes != 1 {
		t.Fatalf("el cobro se casa solo, Hacienda se explica sola y el Bizum queda como tarea: %+v", res)
	}
	if res := importar(); res.Nuevos != 0 || res.Repetidos != 3 {
		t.Fatalf("subir otra vez el mismo extracto no duplica: %+v", res)
	}
	if reg, _ := st.Obtener(ctx, "factura_venta", factura.ID); reg.Estado != "pagado" {
		t.Fatalf("la factura queda cobrada: %s", reg.Estado)
	}

	var banco struct {
		Pendientes []struct {
			ID          string `json:"id"`
			Descripcion string `json:"descripcion"`
		} `json:"pendientes"`
		Total int `json:"total_pendientes"`
	}
	w = llamar(t, "acme", http.MethodGet, "/api/v1/banco", "", "", nil, hs.Banco)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &banco) != nil || banco.Total != 1 ||
		banco.Pendientes[0].Descripcion != "BIZUM A PEPE" {
		t.Fatalf("banco: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones?anio=2026", "", "", nil, hs.Obligaciones)
	var cal struct {
		Region       string `json:"region"`
		Obligaciones []struct {
			Clave      string         `json:"clave"`
			Periodo    string         `json:"periodo"`
			Pendientes map[string]int `json:"pendientes"`
		} `json:"obligaciones"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cal); err != nil || cal.Region != "VC" {
		t.Fatalf("obligaciones: %d %s", w.Code, w.Body.String())
	}
	for _, o := range cal.Obligaciones {
		if o.Clave == "es_303" && o.Periodo == "2026-T3" && o.Pendientes["banco"] != 1 {
			t.Fatalf("el 303 del tercer trimestre sabe que falta un movimiento: %+v", o)
		}
	}

	w = llamar(t, "acme", http.MethodPost, "/api/v1/banco/x/explicar", "application/json", `{"motivo":"socios"}`,
		map[string]string{"id": banco.Pendientes[0].ID}, hs.ExplicarMovimiento)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"550000"`) {
		t.Fatalf("el dinero que saca una autónoma va a su cuenta de titular (550): %d %s", w.Code, w.Body.String())
	}
	w = llamar(t, "acme", http.MethodPost, "/api/v1/banco/x/explicar", "application/json", `{"motivo":"socios"}`,
		map[string]string{"id": banco.Pendientes[0].ID}, hs.ExplicarMovimiento)
	if w.Code != http.StatusConflict {
		t.Fatalf("justificar dos veces es un conflicto: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/obligaciones/ics?anio=2026", "", "", nil, hs.ObligacionesICS)
	if ics := w.Body.String(); w.Code != http.StatusOK || !strings.Contains(ics, "RRULE:FREQ=WEEKLY;BYDAY=MO") ||
		!strings.Contains(ics, "SUMMARY:Revisión semanal en Sytue ERP") {
		t.Fatalf("ics sin la revisión semanal: %d %s", w.Code, ics)
	}
}
