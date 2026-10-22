// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// VERI*FACTU visto desde la interfaz: qué impide activarlo, que la factura
// emitida sale con su QR impreso y en la Facturae, que ya no se deja cambiar y que
// la declaración responsable y la exportación están a mano.
func TestVerifactuPorHTTP(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	st.Version = "v-test"
	hs := manejadoresCompletos(t, st)
	perfil := func(tenant, cp string) {
		w := llamar(t, tenant, http.MethodPut, "/api/v1/organizacion", "application/json",
			`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"`+cp+`","ciudad":"Madrid","pais":"ES","forma":"sl"}`,
			nil, hs.GuardarOrganizacion)
		if w.Code != http.StatusOK {
			t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
		}
	}
	motivo := func(tenant string) string {
		w := llamar(t, tenant, http.MethodGet, "/api/v1/verifactu", "", "", nil, hs.Verifactu)
		var e estadoVerifactu
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || w.Code != http.StatusOK || e.Entorno != "pruebas" {
			t.Fatalf("estado: %d %s", w.Code, w.Body.String())
		}
		return e.Motivo
	}

	if m := motivo("acme"); m != motivoSinPerfil {
		t.Fatalf("sin perfil: %q", m)
	}
	perfil("acme", "28013")
	if m := motivo("acme"); m != motivoSinCertificado {
		t.Fatalf("sin certificado: %q", m)
	}
	w := llamar(t, "acme", http.MethodPost, "/api/v1/verifactu/activar", "", "", nil, hs.ActivarVerifactu)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), motivoSinCertificado) {
		t.Fatalf("activar sin certificado: %d %s", w.Code, w.Body.String())
	}
	hs.verifactu = &verifactu.Cliente{}
	perfil("bilbao", "48001")
	if m := motivo("bilbao"); m != motivoForal {
		t.Fatalf("Bizkaia usa TicketBAI: %q", m)
	}
	w = llamar(t, "acme", http.MethodPost, "/api/v1/verifactu/activar", "", "", nil, hs.ActivarVerifactu)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"activo":true`) {
		t.Fatalf("activar: %d %s", w.Code, w.Body.String())
	}

	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	contacto, _ := hs.cat.Modelo("contacto")
	factura, _ := hs.cat.Modelo("factura_venta")
	cliente := crear(ctx, t, st, contacto, `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","direccion":"Calle Sol 2","cp":"28004","ciudad":"Madrid"}`)
	fv := crear(ctx, t, st, factura, `{"cliente":"`+cliente.ID+`","fecha":"2026-09-12","base":100000,"iva":21000,"total":121000,"notas":"Pan de septiembre"}`)
	params := map[string]string{"modelo": "factura_venta", "id": fv.ID}
	w = comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/factura_venta/"+fv.ID+"/estado", `{"estado":"confirmado"}`, params, http.HandlerFunc(hs.CambiarEstado))
	if w.Code != http.StatusOK {
		t.Fatalf("confirmar: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+fv.ID+"/verifactu", "", "", params, hs.RegistrosFacturaVerifactu)
	var vf struct {
		Registros []store.RegistroVerifactu `json:"registros"`
		QR        string                    `json:"qr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &vf); err != nil || len(vf.Registros) != 1 ||
		vf.QR != "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=B12345678&numserie=FV-2026-00001&fecha=12-09-2026&importe=1210.00" {
		t.Fatalf("registros: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+fv.ID+"/imprimir", "", "", params, hs.Imprimir)
	for _, parte := range []string{"QR tributario:", "VERI*FACTU", "data:image/png;base64,", "Factura FV-2026-00001", "1.210,00 €", "IVA 21 %", "Cafetería Cliente S.L."} {
		if !strings.Contains(w.Body.String(), parte) {
			t.Errorf("la factura impresa no lleva %q", parte)
		}
	}
	w = llamar(t, "acme", http.MethodGet, "/api/v1/factura_venta/"+fv.ID+"/facturae", "", "", params, hs.Facturae)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<InvoiceAdditionalInformation>VERI*FACTU. QR tributario: https://prewww2.aeat.es/") {
		t.Fatalf("facturae: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodPut, "/api/v1/factura_venta/"+fv.ID, "application/json",
		`{"data":{"cliente":"`+cliente.ID+`","fecha":"2026-09-12","base":90000,"iva":18900,"total":108900}}`, params, hs.Actualizar)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "factura_emitida") {
		t.Fatalf("cambiar una factura emitida: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/verifactu/declaracion", "", "", nil, hs.DeclaracionVerifactu)
	if !strings.Contains(w.Body.String(), verifactu.ProductorNIF) || !strings.Contains(w.Body.String(), "Pendiente de suscribir") {
		t.Fatalf("declaración: %s", w.Body.String())
	}
	w = llamar(t, "acme", http.MethodGet, "/api/v1/verifactu/exportar", "", "", nil, hs.ExportarVerifactu)
	var exportado struct {
		Altas []struct {
			Serie string `xml:"IDFactura>NumSerieFactura"`
		} `xml:"RegistroAlta"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &exportado); err != nil || len(exportado.Altas) != 1 || exportado.Altas[0].Serie != "FV-2026-00001" {
		t.Fatalf("exportar: %v %s", err, w.Body.String())
	}
}

func emitirCertHTTPPrueba(t *testing.T, orgID string) (certPEM, keyPEM string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sujeto := pkix.Name{CommonName: "Certificado HTTP VERI*FACTU"}
	if orgID != "" {
		sujeto.ExtraNames = append(sujeto.ExtraNames, pkix.AttributeTypeAndValue{
			Type:  asn1.ObjectIdentifier{2, 5, 4, 97},
			Value: orgID,
		})
	}
	ahora := time.Now().UTC()
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               sujeto,
		NotBefore:             ahora.Add(-time.Hour),
		NotAfter:              ahora.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

// TestVerifactuProduccionExigeCertificadoPropioPorHTTP (NEG-08): en producción
// el certificado de respaldo del despliegue no basta para activar VERI*FACTU;
// hace falta subir el certificado cualificado propio de la organización cuyo
// NIF coincida con el NIF del emisor.
func TestVerifactuProduccionExigeCertificadoPropioPorHTTP(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	st.Version = "v-test"
	hs := manejadoresCompletos(t, st)
	hs.cfg.VerifactuProduccion = true
	hs.declaracionFecha = "2026-10-21"
	hs.verifactu = &verifactu.Cliente{} // certificado de plataforma: no vale en producción

	w := llamar(t, "acme", http.MethodPut, "/api/v1/organizacion", "application/json",
		`{"nombre":"Acme Ibérica SL","nif":"B12345678","direccion":"Gran Vía 1","cp":"28013","ciudad":"Madrid","pais":"ES","forma":"sl"}`,
		nil, hs.GuardarOrganizacion)
	if w.Code != http.StatusOK {
		t.Fatalf("perfil: %d %s", w.Code, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodGet, "/api/v1/verifactu", "", "", nil, hs.Verifactu)
	var e estadoVerifactu
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || w.Code != http.StatusOK || e.Entorno != "produccion" || e.Motivo != motivoSinCertificado {
		t.Fatalf("sin certificado propio en producción debe pedir sin_certificado: %d %+v %s", w.Code, e, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodPost, "/api/v1/verifactu/activar", "", "", nil, hs.ActivarVerifactu)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), motivoSinCertificado) {
		t.Fatalf("activar en producción sin certificado propio: %d %s", w.Code, w.Body.String())
	}

	// Certificado con otro NIF -> 422 certificado_nif_no_coincide.
	certAjeno, keyAjeno := emitirCertHTTPPrueba(t, "VATES-B89890001")
	cuerpoAjeno, _ := json.Marshal(map[string]any{"cert_pem": certAjeno, "key_pem": keyAjeno})
	w = llamar(t, "acme", http.MethodPut, "/api/v1/verifactu/certificado", "application/json", string(cuerpoAjeno), nil, hs.GuardarCertificadoVerifactu)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "certificado_nif_no_coincide") {
		t.Fatalf("certificado con NIF ajeno: %d %s", w.Code, w.Body.String())
	}

	// Certificado propio con el mismo NIF (B12345678) -> 200 y permite activar en producción.
	certPropio, keyPropio := emitirCertHTTPPrueba(t, "VATES-B12345678")
	cuerpoPropio, _ := json.Marshal(map[string]any{"cert_pem": certPropio, "key_pem": keyPropio})
	w = llamar(t, "acme", http.MethodPut, "/api/v1/verifactu/certificado", "application/json", string(cuerpoPropio), nil, hs.GuardarCertificadoVerifactu)
	e = estadoVerifactu{}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || w.Code != http.StatusOK || !e.TieneCertificado || e.CertificadoNIF != "B12345678" || e.Motivo != "" {
		t.Fatalf("guardar certificado propio: %d %+v %s", w.Code, e, w.Body.String())
	}

	w = llamar(t, "acme", http.MethodPost, "/api/v1/verifactu/activar", "", "", nil, hs.ActivarVerifactu)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"activo":true`) {
		t.Fatalf("activar en producción con certificado propio: %d %s", w.Code, w.Body.String())
	}
}
