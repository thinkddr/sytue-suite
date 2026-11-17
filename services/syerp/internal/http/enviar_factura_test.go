// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/qr"
)

func facturaDeEjemplo(t *testing.T) facturaImprimible {
	t.Helper()
	png, err := qr.PNG("https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=B12345678")
	if err != nil {
		t.Fatal(err)
	}
	return facturaImprimible{
		Idioma: "es", Titulo: "Factura", Numero: "F-2026/0007", Aviso: "",
		Emisor:   parteImprimible{Nombre: "Acme Ibérica SL", NIF: "B12345678", Direccion: "Gran Vía 1", Ciudad: "28013 Madrid"},
		Cliente:  parteImprimible{Nombre: "Cafetería Cliente S.L.", NIF: "B12345674", Ciudad: "28004 Madrid"},
		Datos:    [][2]string{{"Fecha", "12/09/2026"}, {"Vencimiento", ""}},
		Columnas: [5]string{"Descripción", "Cantidad", "Precio sin IVA", "Tipo de IVA", "Base"},
		Lineas:   [][5]string{{"Hogaza de masa madre", "3", "2,4038 €", "4 %", "7,21 €"}},
		Grupos:   [][3]string{{"4 %", "7,21 €", "0,29 €"}},
		Base:     "7,21 €", IVA: "0,29 €", Total: "7,50 €",
		RotuloBase: "Base", RotuloIVA: "IVA", RotTotal: "Total",
		Leyenda: "Operación exenta de IVA (artículo 20 de la Ley 37/1992)",
		QRTexto: "https://www2.agenciatributaria.gob.es/…", qrPNG: png,
	}
}

// El PDF que se adjunta dice lo mismo que el impreso: emisor, cliente, número,
// líneas, totales, la leyenda y el QR con sus dos rótulos.
func TestFacturaPDF(t *testing.T) {
	pdf, err := facturaPDF(facturaDeEjemplo(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("/Subtype /Image")) && !bytes.Contains(pdf, []byte("/Subtype/Image")) {
		t.Fatal("no es un PDF con el QR")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext no está instalado")
	}
	f := filepath.Join(t.TempDir(), "f.pdf")
	if err := os.WriteFile(f, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	salida, err := exec.Command("pdftotext", "-layout", f, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v\n%s", err, salida)
	}
	texto := string(salida)
	for _, quiero := range []string{
		"Acme Ibérica SL", "Factura F-2026/0007", "Cafetería Cliente S.L.", "Fecha: 12/09/2026",
		"Hogaza de masa madre", "2,4038 €", "7,50 €", "artículo 20", "QR tributario:", "VERI*FACTU",
	} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("falta %q en el PDF:\n%s", quiero, texto)
		}
	}
	if strings.Contains(texto, "Vencimiento") {
		t.Error("una fecha vacía no se pinta")
	}
}

func TestNombrePDF(t *testing.T) {
	if got := nombrePDF(facturaImprimible{Numero: "F-2026/0007"}); got != "F-2026-0007.pdf" {
		t.Fatalf("%q", got)
	}
	if got := nombrePDF(facturaImprimible{}); got != "factura.pdf" {
		t.Fatalf("%q", got)
	}
}

// Lo que se comprueba antes de tocar la base: una dirección mal escrita y un
// despliegue sin servidor de correo no llegan a construir la factura.
func TestEnviarFacturaValidaAntes(t *testing.T) {
	h := &Handlers{}
	pedir := func(h *Handlers, cuerpo string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/factura_venta/x/enviar", strings.NewReader(cuerpo))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.EnviarFactura(w, r.WithContext(context.Background()))
		return w
	}
	if w := pedir(h, `{"para":"no-es-un-correo","asunto":"Factura"}`); w.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(w.Body.String(), "destinatario_invalido") {
		t.Fatalf("destinatario: %d %s", w.Code, w.Body.String())
	}
	if w := pedir(h, `{"para":"ana@cliente.es","asunto":"  "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("sin asunto: %d %s", w.Code, w.Body.String())
	}
	if w := pedir(h, `{"para":"ana@cliente.es","asunto":"Factura"}`); w.Code != http.StatusServiceUnavailable ||
		!strings.Contains(w.Body.String(), "correo_no_configurado") {
		t.Fatalf("sin SMTP: %d %s", w.Code, w.Body.String())
	}
}
