// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFirmaEnDosPasosHablaElIdiomaDelServicio: rutas, campos y respuestas de
// `services/docs/internal/http/pdffirmasistema.go`, y el sello caído que se
// reintenta sin sello con la MISMA firma.
func TestFirmaEnDosPasosHablaElIdiomaDelServicio(t *testing.T) {
	selloCaido := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/api/v1/pdf/sign/prepare":
			certs := r.MultipartForm.File["certificado"]
			var opts PDFFirmaOpciones
			_ = json.Unmarshal([]byte(r.FormValue("firma")), &opts)
			if len(certs) != 2 || opts.Motivo != "Conforme" || len(r.MultipartForm.File["file"]) != 1 {
				t.Errorf("prepare: %d certificados, opciones %+v", len(certs), opts)
			}
			_, _ = io.WriteString(w, `{"id":"p1","preparado":"`+base64.StdEncoding.EncodeToString([]byte("%PDF-hueco"))+
				`","resumen":"`+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))+
				`","algoritmo":"ecdsa","nivel":"avanzada","firmante":{"nombre":"Irati"},"sello":"tsa.izenpe.com"}`)
		case "/api/v1/pdf/sign/complete":
			f, _ := r.MultipartForm.File["file"][0].Open()
			preparado, _ := io.ReadAll(f)
			if r.FormValue("id") != "p1" || r.FormValue("formato") != PDFFirmaP1363 || string(preparado) != "%PDF-hueco" ||
				r.FormValue("firma") != base64.StdEncoding.EncodeToString([]byte("firma")) {
				t.Errorf("complete: %v", r.Form)
			}
			if selloCaido && r.FormValue("sin_sello") != "1" {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"error":{"code":"pdf_timestamp_unavailable","message":"x"}}`)
				return
			}
			w.Header().Set("X-Sytue-Sello", map[bool]string{true: "no", false: "si"}[selloCaido])
			_, _ = io.WriteString(w, "%PDF-firmado")
		default:
			t.Errorf("ruta %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()

	prep, err := p.PrepararFirma(ctx, PDFFichero{Nombre: "c.pdf", Datos: []byte("%PDF-1.7")}, "",
		PDFFirmaOpciones{Motivo: "Conforme"}, [][]byte{[]byte("hoja"), []byte("intermedio")})
	if err != nil || prep.ID != "p1" || len(prep.Resumen) != 32 || prep.Firmante.Nombre != "Irati" || prep.Sello != "tsa.izenpe.com" {
		t.Fatalf("PrepararFirma: %+v %v", prep, err)
	}
	if _, _, err := p.CompletarFirma(ctx, prep, []byte("firma"), PDFFirmaP1363, false); !PDFSelloCaido(err) {
		t.Fatalf("sello caído: err = %v", err)
	}
	firmado, sellado, err := p.CompletarFirma(ctx, prep, []byte("firma"), PDFFirmaP1363, true)
	if err != nil || sellado || string(firmado) != "%PDF-firmado" {
		t.Fatalf("sin sello: %q %v %v", firmado, sellado, err)
	}
	selloCaido = false
	if _, sellado, err := p.CompletarFirma(ctx, prep, []byte("firma"), PDFFirmaP1363, false); err != nil || !sellado {
		t.Fatalf("con sello: %v %v", sellado, err)
	}
}

// TestPedirFirmaTraduceLaRutaAlFichero: el servicio va por identificador de Drive y
// CertiPDF por ruta; la traducción la hace el SDK con la sesión de quien pide.
func TestPedirFirmaTraduceLaRutaAlFichero(t *testing.T) {
	var cuerpos []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz"}`)
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[{"id":"cpdf","name":"PDF"}],"files":[]}`)
		case "GET /api/v1/folders/cpdf/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f9","name":"a.pdf"},{"id":"f10","name":"a (firmado).pdf"}]}`)
		case "POST /api/v1/pdf/signature-requests", "POST /api/v1/pdf/signature-requests/p1/sign":
			var c map[string]any
			_ = json.NewDecoder(r.Body).Decode(&c)
			cuerpos = append(cuerpos, c)
			if c["email"] == "@leire" && c["confirm"] != "s-leire" {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":{"code":"pdf_signature_recipient_external","message":"x","details":{"name":"Leire","confirm":"s-leire"}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"p1","status":"pendiente","path":"/PDF/a.pdf"}`)
		case "POST /api/v1/pdf/signature-requests/p2/open":
			_, _ = io.WriteString(w, `{"id":"p2","status":"pendiente","external":true,"name":"a.pdf","path":"/Sytue Docs/a.pdf"}`)
		case "POST /api/v1/pdf/signature-requests/p3/fetch-signed":
			_, _ = io.WriteString(w, `{"id":"p3","status":"firmada","external":true,"path":"/PDF/a.pdf","signed_path":"/Sytue Docs/a (firmado).pdf"}`)
		case "GET /api/v1/pdf/signature-requests":
			_, _ = io.WriteString(w, `{"received":[{"id":"p0","status":"firmada","path":"/PDF/a.pdf"},{"id":"p1","status":"pendiente","path":"/PDF/a.pdf"}],"sent":[],"blocked":[{"sub":"u-ana","name":"Ana","created_at":"2026-09-27T10:00:00Z"}]}`)
		case "DELETE /api/v1/pdf/signature-requests/p1", "POST /api/v1/pdf/signature-requests/p2/block",
			"DELETE /api/v1/pdf/signature-blocks/u-ana":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()
	if _, err := p.PedirFirma(ctx, "/PDF/a.pdf", "irati@acme.eu", "antes del viernes", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := p.EntregarFirma(ctx, "p1", "/PDF/a (firmado).pdf"); err != nil {
		t.Fatal(err)
	}
	if len(cuerpos) != 2 || cuerpos[0]["file_id"] != "f9" || cuerpos[0]["email"] != "irati@acme.eu" ||
		cuerpos[1]["signed_file_id"] != "f10" || cuerpos[1]["signed_path"] != "/PDF/a (firmado).pdf" {
		t.Errorf("cuerpos = %v", cuerpos)
	}
	ps, err := p.PeticionesFirma(ctx)
	if pend, ok := ps.PendienteDe("/PDF/a.pdf"); err != nil || !ok || pend.ID != "p1" {
		t.Errorf("pendiente = %+v %v %v", pend, ok, err)
	}
	if err := p.CancelarFirma(ctx, "p1"); err != nil {
		t.Error(err)
	}
	if len(ps.Bloqueados) != 1 || ps.Bloqueados[0].Nombre != "Ana" {
		t.Errorf("bloqueados = %+v", ps.Bloqueados)
	}
	if err := p.BloquearFirma(ctx, "p2"); err != nil {
		t.Error(err)
	}
	if err := p.DesbloquearFirma(ctx, "u-ana"); err != nil {
		t.Error(err)
	}

	// De otra organización (ADR 0057): primero se pregunta, luego se comparte.
	_, err = p.PedirFirma(ctx, "/PDF/a.pdf", "@leire", "", "")
	nombre, confirmar, ok := PDFFirmaDeFuera(err)
	if !ok || nombre != "Leire" || confirmar != "s-leire" {
		t.Errorf("de fuera = %q %q %v (%v)", nombre, confirmar, ok, err)
	}
	if _, _, ok := PDFFirmaDeFuera(errors.New("otro")); ok {
		t.Error("cualquier error parece de fuera")
	}
	if _, err := p.PedirFirma(ctx, "/PDF/a.pdf", "@leire", "", confirmar); err != nil {
		t.Errorf("compartiendo: %v", err)
	}
	if a, err := p.AbrirFirma(ctx, "p2"); err != nil || a.Ruta != "/Sytue Docs/a.pdf" || a.NombrePDF() != "a.pdf" {
		t.Errorf("abrir = %+v %v", a, err)
	}
	if f, err := p.TraerFirmado(ctx, "p3"); err != nil || f.RutaFirmada != "/Sytue Docs/a (firmado).pdf" {
		t.Errorf("traer = %+v %v", f, err)
	}
	if (PDFPeticionFirma{Externo: true, Nombre: "a.pdf"}).NombrePDF() != "a.pdf" {
		t.Error("nombre sin ruta")
	}
	if _, ok := (PDFPeticionesFirma{Recibidas: []PDFPeticionFirma{{Estado: PDFFirmaPendiente, Externo: true}}}).PendienteDe(""); ok {
		t.Error("una de fuera sin copia no es la de ningún fichero")
	}
}
