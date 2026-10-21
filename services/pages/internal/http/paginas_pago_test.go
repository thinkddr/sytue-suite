// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-12. Las dos páginas HTML de /_pages escapan cada valor en su contexto, y la de
// Redsys solo deja correr su script y solo deja enviar el formulario a la pasarela.
func TestLasPaginasDePagoEscapanLoQueLlevan(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/pay/x", http.NoBody)
	rec := httptest.NewRecorder()
	paginaRedsys(rec, req, "https://sis-t.redsys.es:25443/sis/realizarPago", map[string]string{
		"Ds_Signature":         `"><script>alert(1)</script>`,
		`x" onfocus="alert(2)`: "v",
	})
	cuerpo := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(cuerpo, "<script>alert") || strings.Contains(cuerpo, `" onfocus="`) {
		t.Fatalf("la página de Redsys pinta los campos sin escapar (%d):\n%s", rec.Code, cuerpo)
	}
	if n := strings.Count(cuerpo, "<script>"); n != 1 {
		t.Fatalf("la página de Redsys lleva %d scripts; quiero solo el que envía el formulario", n)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'sha256-"+hashEnvioRedsys+"'") || !strings.Contains(csp, "form-action https://sis-t.redsys.es:25443;") {
		t.Fatalf("CSP de la página de Redsys: %q", csp)
	}

	rec = httptest.NewRecorder()
	paginaRedsys(rec, req, "javascript:alert(1)", nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "javascript:") {
		t.Fatalf("una acción que no es https: %d %s", rec.Code, rec.Body.String())
	}

	var b strings.Builder
	if err := paginaFactura.Execute(&b, &domain.OrderPublicDTO{ERPInvoiceRef: `<img src=x onerror=alert(3)>`, Number: "P-000001"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "<img") {
		t.Fatalf("la factura pinta la referencia sin escapar:\n%s", b.String())
	}
}
