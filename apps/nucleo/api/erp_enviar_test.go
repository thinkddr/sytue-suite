// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// El PDF de la factura y su envío por correo, contra un servicio de mentira con la forma
// de services/syerp: la ruta, el nombre del fichero, el cuerpo del envío y los errores.
func TestERPFacturaEnPDFYPorCorreo(t *testing.T) {
	var enviado ERPCorreoFactura
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Language") != "es" {
			t.Errorf("%s: sin el idioma de la sesión", r.URL.Path)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/factura_venta/f1/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="FV-2026-00042.pdf"`)
			_, _ = io.WriteString(w, "%PDF-1.7")
		case "GET /api/v1/factura_venta/f2/pdf":
			// Sin cabecera: el nombre de respaldo, no uno vacío.
			_, _ = io.WriteString(w, "%PDF-1.7")
		case "GET /api/v1/factura_venta/f1/enviar":
			_, _ = io.WriteString(w, `{"para":"pagos@cliente.es","enviable":true,"disponible":true,"por_cuenta":true,`+
				`"cuentas":[{"id":"c1","direccion":"ana@ejemplo.eu","nombre":"Ana"}],"asunto":"Factura FV-2026-00042","mensaje":"Hola"}`)
		case "GET /api/v1/factura_venta/f3/enviar":
			// El servicio de la suite: ni cuentas ni plantilla.
			_, _ = io.WriteString(w, `{"para":"","enviable":false,"disponible":true}`)
		case "POST /api/v1/factura_venta/f1/enviar":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("el envío va en JSON")
			}
			if err := json.NewDecoder(r.Body).Decode(&enviado); err != nil {
				t.Error(err)
			}
			_, _ = io.WriteString(w, `{"enviada":true}`)
		case "POST /api/v1/factura_venta/f2/enviar":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"code":"correo_no_enviado","message":"The mail server did not accept the message."}}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	e, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	cuerpo, nombre, err := e.PDFFactura(ctx, ERPFacturaVenta, "f1")
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(cuerpo)
	_ = cuerpo.Close()
	if string(datos) != "%PDF-1.7" || nombre != "FV-2026-00042.pdf" {
		t.Errorf("PDF: %q en %q", datos, nombre)
	}
	cuerpo, nombre, err = e.PDFFactura(ctx, ERPFacturaVenta, "f2")
	if err != nil {
		t.Fatal(err)
	}
	_ = cuerpo.Close()
	if nombre != "factura.pdf" {
		t.Errorf("respaldo del nombre: %q", nombre)
	}

	d, err := e.EnvioFactura(ctx, ERPFacturaVenta, "f1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Para != "pagos@cliente.es" || !d.Enviable || !d.Disponible || !d.PorCuenta || len(d.Cuentas) != 1 ||
		d.Cuentas[0].Direccion != "ana@ejemplo.eu" || d.Asunto == "" || d.Mensaje != "Hola" {
		t.Errorf("envío: %+v", d)
	}
	if d, err = e.EnvioFactura(ctx, ERPFacturaVenta, "f3"); err != nil || d.PorCuenta || d.Cuentas != nil || d.Enviable {
		t.Errorf("el de la suite: %+v %v", d, err)
	}

	c := ERPCorreoFactura{Para: "pagos@cliente.es", Asunto: "Factura FV-2026-00042", Mensaje: "Hola", Cuenta: "c1"}
	if err := e.EnviarCorreoFactura(ctx, ERPFacturaVenta, "f1", c); err != nil {
		t.Fatal(err)
	}
	if enviado != c {
		t.Errorf("cuerpo del envío: %+v", enviado)
	}
	var ae *Error
	if err := e.EnviarCorreoFactura(ctx, ERPFacturaVenta, "f2", c); !errors.As(err, &ae) || ae.Status != http.StatusBadGateway || ae.Code() != "correo_no_enviado" {
		t.Errorf("un rechazo del servidor de correo: %v", err)
	}
}

// Las rutas y los campos son los del servicio: si cambian allí, esto lo dice.
func TestERPEnviarFacturaUsaLasRutasDelServicio(t *testing.T) {
	servidor := webref.Read(t, "services/syerp/internal/http/server.go")
	for _, ruta := range []string{`mod.Get("/{id}/pdf", h.PDFFactura)`, `mod.Get("/{id}/enviar", h.EnvioFactura)`, `mod.Post("/{id}/enviar", h.EnviarFactura)`} {
		if !strings.Contains(servidor, ruta) {
			t.Errorf("services/syerp ya no sirve %s", ruta)
		}
	}
	envio := webref.Read(t, "services/syerp/internal/http/enviar_factura.go")
	for _, campo := range []string{`json:"para"`, `json:"asunto"`, `json:"mensaje"`, `"para":`, `"enviable":`, `"disponible":`, `"enviada": true`} {
		if !strings.Contains(envio, campo) {
			t.Errorf("el envío del servicio ya no lleva %s", campo)
		}
	}
}

// Con la fachada privada no hay ruta: falla cerrado, como el resto.
func TestERPEnviarFacturaFallaCerradoConLaFachadaPrivada(t *testing.T) {
	e := &ERP{c: privateDataClient(New("http://erp.invalid/api/v1", nil, nil), ErrPrivateERPOperation), idioma: "es"}
	if _, _, err := e.PDFFactura(context.Background(), ERPFacturaVenta, "f1"); !errors.Is(err, ErrPrivateERPOperation) {
		t.Errorf("PDF: %v", err)
	}
	if err := e.EnviarCorreoFactura(context.Background(), ERPFacturaVenta, "f1", ERPCorreoFactura{}); !errors.Is(err, ErrPrivateERPOperation) {
		t.Errorf("enviar: %v", err)
	}
}
