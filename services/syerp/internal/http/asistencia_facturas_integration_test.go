// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestAsistenciaFacturaXMLPrevisualizaSinEscribir(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if _, err := st.GuardarOrganizacion(ctx, store.Organizacion{Pais: "ES", NIF: "B12345678", Nombre: "Acme", Forma: "sl", CP: "28013"}, "ada"); err != nil {
		t.Fatal(err)
	}
	ponerPermisos := func(facturacion, contactos string, crear bool) {
		t.Helper()
		verbos := []string{}
		if crear {
			verbos = append(verbos, permisos.Crear)
		}
		if err := st.GuardarPermisos(ctx, "luis", "luis@acme.es", "Luis", map[string]string{"facturacion": facturacion, "contactos": contactos}, map[string][]string{"facturacion": verbos}, "ada"); err != nil {
			t.Fatal(err)
		}
	}
	ponerPermisos("usuario", "lectura", true)
	handler := http.HandlerFunc(hs.LeerAsistenciaFactura)
	ruta := "/api/v1/factura_compra/asistencia/leer"
	params := map[string]string{"modelo": "factura_compra"}
	miembro := []string{"member"}
	cuentas := func() [6]int64 {
		t.Helper()
		var out [6]int64
		if err := harness.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM records), (SELECT count(*) FROM moves),
				(SELECT count(*) FROM adjuntos), (SELECT count(*) FROM importacion),
				(SELECT count(*) FROM verifactu_registros), (SELECT count(*) FROM historial)`).
				Scan(&out[0], &out[1], &out[2], &out[3], &out[4], &out[5])
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	leer := func(t *testing.T, cuerpo string) asistenciaFactura {
		t.Helper()
		antes := cuentas()
		w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, cuerpo, params, handler)
		if w.Code != http.StatusOK {
			t.Fatalf("leer XML: %d %s", w.Code, w.Body.String())
		}
		var out asistenciaFactura
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if despues := cuentas(); despues != antes {
			t.Fatalf("previsualizar escribió: %v -> %v", antes, despues)
		}
		return out
	}
	crearProveedor := func(tenant, nombre, nif string) store.Registro {
		t.Helper()
		m, ok := hs.cat.Modelo("contacto")
		if !ok {
			t.Fatal("falta catálogo de contactos")
		}
		crudo, err := json.Marshal(map[string]any{"nombre": nombre, "nif": nif, "tipo": "organizacion"})
		if err != nil {
			t.Fatal(err)
		}
		data, etiqueta, err := domain.Validar(m, crudo)
		if err != nil {
			t.Fatal(err)
		}
		reg, err := st.Crear(tenancy.Into(context.Background(), tenancy.Tenant{Slug: tenant}), m, data, etiqueta, "ada")
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}

	t.Run("cabecera y contacto propuesto sin guardar", func(t *testing.T) {
		out := leer(t, ublRecibida)
		if out.Data["fecha"] != "2026-03-02" || out.Data["base"] != float64(123456) || out.Data["iva"] != float64(25926) || out.Data["total"] != float64(149382) ||
			out.Data["vencimiento"] != "2026-04-01" || out.Data["externo_id"] != "B98765432/F-2026-0042" || out.NumeroOrigen != "F-2026-0042" {
			t.Fatalf("cabecera: %+v", out)
		}
		if _, hay := out.Data["proveedor"]; hay || len(out.Etiquetas) != 0 || out.Contacto["nombre"] != "Proveedora Levante SL" || out.Contacto["nif"] != "ESB98765432" {
			t.Fatalf("no se propone proveedor nuevo: %+v", out)
		}
	})

	proveedor := crearProveedor("acme", "Proveedor interno", "B-98765432")
	t.Run("proveedor por NIF normalizado sin buscar nombre", func(t *testing.T) {
		out := leer(t, ublRecibida)
		if out.Data["proveedor"] != proveedor.ID || out.Etiquetas[proveedor.ID] != "Proveedor interno" || out.Contacto != nil {
			t.Fatalf("relación: %+v", out)
		}
	})
	t.Run("una compra registrada se recupera sin escribir", func(t *testing.T) {
		out := leer(t, ublRecibida)
		m, _ := hs.cat.Modelo("factura_compra")
		reg, err := st.Crear(ctx, m, out.Data, "Factura de origen", "ada")
		if err != nil {
			t.Fatal(err)
		}
		out = leer(t, ublRecibida)
		if out.Existente == nil || out.Existente.ID != reg.ID || out.Contacto != nil {
			t.Fatalf("compra existente: %+v", out)
		}
	})

	crearProveedor("acme", "Proveedora Levante SL", "B55555555")
	t.Run("el mismo nombre nunca resuelve otra identidad", func(t *testing.T) {
		out := leer(t, strings.ReplaceAll(ublRecibida, "ESB98765432", "ESB44444444"))
		if _, hay := out.Data["proveedor"]; hay || out.Contacto["nif"] != "ESB44444444" {
			t.Fatalf("casó por nombre: %+v", out)
		}
	})
	crearProveedor("globex", "Proveedor secreto de Globex", "B44444444")
	t.Run("no se filtra proveedor de otra organización", func(t *testing.T) {
		out := leer(t, strings.ReplaceAll(ublRecibida, "ESB98765432", "ESB44444444"))
		if _, hay := out.Data["proveedor"]; hay || len(out.Etiquetas) != 0 || out.Contacto["nombre"] != "Proveedora Levante SL" {
			t.Fatalf("proveedor de otro tenant: %+v", out)
		}
	})

	t.Run("permisos antes de lectura o resolución", func(t *testing.T) {
		defer ponerPermisos("usuario", "lectura", true)
		for _, c := range []struct {
			nombre, facturacion, contactos string
			crear                          bool
		}{
			{"sin contactos", "usuario", "ninguno", true},
			{"solo lectura facturas", "lectura", "lectura", true},
			{"sin verbo crear", "usuario", "lectura", false},
		} {
			t.Run(c.nombre, func(t *testing.T) {
				ponerPermisos(c.facturacion, c.contactos, c.crear)
				antes := cuentas()
				w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, ublRecibida, params, handler)
				if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), proveedor.ID) || strings.Contains(w.Body.String(), "Proveedor interno") {
					t.Fatalf("leyó contacto oculto: %d %s", w.Code, w.Body.String())
				}
				if despues := cuentas(); despues != antes {
					t.Fatalf("petición sin permiso escribió: %v -> %v", antes, despues)
				}
			})
		}
	})

	t.Run("rechaza documentos que no preparan una compra segura", func(t *testing.T) {
		for _, c := range []struct{ nombre, cuerpo, modelo string }{
			{"receptor ajeno", strings.ReplaceAll(ublRecibida, "ESB12345678", "ESB00000000"), "factura_compra"},
			{"receptor ausente", strings.ReplaceAll(ublRecibida, "ESB12345678", ""), "factura_compra"},
			{"dólares", strings.ReplaceAll(ublRecibida, "EUR", "USD"), "factura_compra"},
			{"moneda no declarada", strings.ReplaceAll(ublRecibida, "<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>", ""), "factura_compra"},
			{"fecha imposible", strings.ReplaceAll(ublRecibida, "2026-03-02", "2026-02-30"), "factura_compra"},
			{"CSV", "fecha,base\n2026-03-02,10.00", "factura_compra"},
			{"venta ya emitida", ublRecibida, "factura_venta"},
			{"Facturae con dos facturas", facturaeAsistenciaLote, "factura_compra"},
		} {
			t.Run(c.nombre, func(t *testing.T) {
				antes := cuentas()
				w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, c.cuerpo, map[string]string{"modelo": c.modelo}, handler)
				if w.Code != http.StatusUnprocessableEntity {
					t.Fatalf("documento aceptado: %d %s", w.Code, w.Body.String())
				}
				if despues := cuentas(); despues != antes {
					t.Fatalf("documento rechazado escribió: %v -> %v", antes, despues)
				}
			})
		}
	})
}

const facturaeAsistenciaLote = `<Facturae>
  <Parties>
    <SellerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><TaxIdentificationNumber>B98765432</TaxIdentificationNumber></TaxIdentification>
      <LegalEntity><CorporateName>Proveedora Levante SL</CorporateName></LegalEntity></SellerParty>
    <BuyerParty><TaxIdentification><TaxIdentificationNumber>B12345678</TaxIdentificationNumber></TaxIdentification></BuyerParty>
  </Parties>
  <Invoices>
    <Invoice><InvoiceHeader><InvoiceNumber>1</InvoiceNumber></InvoiceHeader>
      <InvoiceIssueData><IssueDate>2026-03-02</IssueDate><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></InvoiceIssueData>
      <InvoiceTotals><TotalGrossAmountBeforeTaxes>100.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>21.00</TotalTaxOutputs></InvoiceTotals></Invoice>
    <Invoice><InvoiceHeader><InvoiceNumber>2</InvoiceNumber></InvoiceHeader>
      <InvoiceIssueData><IssueDate>2026-03-03</IssueDate><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></InvoiceIssueData>
      <InvoiceTotals><TotalGrossAmountBeforeTaxes>50.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>10.50</TotalTaxOutputs></InvoiceTotals></Invoice>
  </Invoices>
</Facturae>`
