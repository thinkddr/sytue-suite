// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la firma con certificado. No se comprueba «que no dé error»: se
// VALIDA la firma como lo haría un lector —recalculando el resumen sobre el
// /ByteRange y verificando el PKCS#7 con la clave pública del certificado— y se
// comprueba que tocar un byte del documento la invalida.
//
// El certificado se genera en el propio test: no hay claves en el repo.
package domain

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

// identidadDePrueba genera un certificado autofirmado y su clave, en PEM.
func identidadDePrueba(t *testing.T) (certPEM, clavePEM []byte) {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	plantilla := x509.Certificate{
		SerialNumber: big.NewInt(20260813),
		Subject:      pkix.Name{CommonName: "Ana Pérez", Organization: []string{"Sytue"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &plantilla, &plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatalf("crear certificado: %v", err)
	}
	claveDER, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatalf("codificar clave: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER})
}

func TestFirmarConCertificadoProduceUnaFirmaValida(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "contrato de arrendamiento"), OpcionesFirma{
		CertificadoPEM: certPEM,
		ClavePEM:       clavePEM,
		Motivo:         "Conforme con las condiciones",
		Lugar:          "Madrid",
		Rect:           [4]float64{80, 80, 300, 150},
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	if err := validarFirma(firmado); err != nil {
		t.Fatalf("la firma no valida: %v", err)
	}

	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatalf("el PDF firmado no abre: %v", err)
	}
	firmas := d.FirmasDelDocumento()
	if len(firmas) != 1 {
		t.Fatalf("firmas leídas = %+v", firmas)
	}
	if firmas[0].Nombre != "Ana Pérez" {
		t.Errorf("nombre = %q (debería salir del CN del certificado)", firmas[0].Nombre)
	}
	if firmas[0].Motivo != "Conforme con las condiciones" {
		t.Errorf("motivo = %q", firmas[0].Motivo)
	}
	if !firmas[0].CubreTodo {
		t.Error("el /ByteRange debería cubrir el fichero entero")
	}
}

// TestManipularElDocumentoInvalidaLaFirma es EL check de esta funcionalidad: si
// cambiar el documento no invalidara la firma, la firma no serviría de nada.
func TestManipularElDocumentoInvalidaLaFirma(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "importe: 1000 euros"), OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: clavePEM,
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	if err := validarFirma(firmado); err != nil {
		t.Fatalf("la firma de partida ya no valida: %v", err)
	}

	i := bytes.Index(firmado, []byte("1000"))
	if i < 0 {
		t.Skip("el texto no está en claro en este PDF; el check necesita verlo")
	}
	manipulado := append([]byte(nil), firmado...)
	copy(manipulado[i:], "9000")
	if err := validarFirma(manipulado); err == nil {
		t.Fatal("se cambió el importe y la firma sigue validando")
	}
}

// PDF/A pide una apariencia en cada campo (ISO 19005-1 6.9): la firma invisible
// la lleva aunque no dibuje nada, por los dos caminos. Sin ella, firmar un BOE
// —PDF/A-1a— lo dejaba fuera de norma (medido con veraPDF).
func TestFirmaInvisibleLlevaApariencia(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	opts := OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM}
	reescrita, err := FirmarConCertificadoPDF(docDePrueba(t, "acta"), opts)
	if err != nil {
		t.Fatal(err)
	}
	anexada, err := FirmarIncrementalPDF(docDePrueba(t, "acta"), opts)
	if err != nil {
		t.Fatal(err)
	}
	for nombre, pdf := range map[string][]byte{"reescrita": reescrita, "anexada": anexada} {
		d, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		p, _ := d.Pagina(0)
		widgets := 0
		for _, a := range arrayResuelto(d, p.dict["Annots"]) {
			w, _ := d.resolver(a).(pdfDict)
			if ft, _ := d.resolver(w["FT"]).(pdfName); ft != "Sig" {
				continue
			}
			widgets++
			ap, _ := d.resolver(w["AP"]).(pdfDict)
			if _, ok := d.resolver(ap["N"]).(*pdfStream); !ok {
				t.Errorf("%s: el widget de la firma invisible no tiene apariencia", nombre)
			}
		}
		if widgets != 1 {
			t.Errorf("%s: widgets de firma = %d, quería 1", nombre, widgets)
		}
	}
}

func TestFirmaInvisibleNoDibujaNada(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	// Sin /Rect: la firma es invisible y el documento se ve exactamente igual.
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "acta"), OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: clavePEM,
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	if err := validarFirma(firmado); err != nil {
		t.Fatalf("una firma invisible tiene que valer igual: %v", err)
	}
	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(svg, "Firmado por") {
		t.Error("una firma invisible no debe pintar recuadro")
	}
}

func TestFirmaVisibleSeDibuja(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "acta"), OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: clavePEM,
		Rect:   [4]float64{60, 60, 320, 140},
		Trazos: [][][2]float64{{{80, 90}, {130, 120}, {180, 80}}},
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "Firmado por Ana P") {
		t.Errorf("el recuadro de firma no se ve: %s", primerasLineas(svg))
	}
}

func TestClaveQueNoEsLaDelCertificadoSeRechaza(t *testing.T) {
	certPEM, _ := identidadDePrueba(t)
	_, otraClave := identidadDePrueba(t)
	_, err := FirmarConCertificadoPDF(docDePrueba(t, "x"), OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: otraClave,
	})
	if !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("firmar con una clave ajena debería fallar antes de escribir nada, dio: %v", err)
	}
}

func TestCertificadoIlegibleSeRechaza(t *testing.T) {
	_, clavePEM := identidadDePrueba(t)
	_, err := FirmarConCertificadoPDF(docDePrueba(t, "x"), OpcionesFirma{
		CertificadoPEM: []byte("esto no es un certificado"), ClavePEM: clavePEM,
	})
	if !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("esperaba ErrCertificadoNoUsable, dio: %v", err)
	}
}

func TestByteRangeNoMueveNingunByte(t *testing.T) {
	// Sustituir el /ByteRange de relleno por el real no puede cambiar la longitud
	// del fichero: un solo byte de más invalidaría todos los desplazamientos de
	// la tabla xref y el PDF dejaría de abrir.
	datos := []byte("<< /ByteRange [0 9999999999 9999999999 9999999999] /X 1 >>")
	antes := len(datos)
	if err := escribirByteRange(datos, [4]int{0, 840, 17264, 1200}); err != nil {
		t.Fatalf("escribirByteRange: %v", err)
	}
	if len(datos) != antes {
		t.Fatalf("la longitud cambió: %d → %d", antes, len(datos))
	}
	if !bytes.Contains(datos, []byte("[0 840 17264 1200")) {
		t.Errorf("no se escribieron los valores reales: %s", datos)
	}
	if !bytes.Contains(datos, []byte("] /X 1 >>")) {
		t.Errorf("se estropeó lo que había detrás: %s", datos)
	}
}

// --- Validación, que es lo que haría un lector ------------------------------

// validarFirma valida la primera firma del fichero.
func validarFirma(pdf []byte) error { return validarFirmaN(pdf, 0) }

// validarFirmaN valida la firma número `n` con el verificador del producto
// (pdfverificar.go): estructura del /ByteRange, resumen sobre SU rango y firma
// con el certificado. La confianza no cuenta aquí —el certificado de prueba no es
// de nadie—: eso lo miran los tests de VerificarFirmas.
func validarFirmaN(pdf []byte, n int) error {
	d, err := LeerPDF(pdf)
	if err != nil {
		return err
	}
	firmas := d.VerificarFirmas(OpcionesVerificacion{})
	if n >= len(firmas) {
		return fmt.Errorf("no hay firma número %d (hay %d)", n, len(firmas))
	}
	if f := firmas[n]; !f.Integra {
		return fmt.Errorf("la firma no valida: %v", f.Motivos)
	}
	return nil
}

// TestBloquearCertificaElDocumento comprueba lo que hace verificable un
// «bloquear tras firmar»: sin /DocMDP en la firma Y /Perms en el catálogo, el
// lector no sabe que el documento está certificado y lo trata como uno firmado
// cualquiera, al que se le pueden añadir páginas por actualización incremental
// sin que salte ningún aviso.
func TestBloquearCertificaElDocumento(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "acta"), OpcionesFirma{
		CertificadoPEM: certPEM,
		ClavePEM:       clavePEM,
		Bloquear:       true,
		Rect:           [4]float64{80, 80, 300, 150},
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	// Certificar no puede costar la validez: es la misma firma con una entrada más.
	if err := validarFirma(firmado); err != nil {
		t.Fatalf("la firma certificada no valida: %v", err)
	}
	for _, marca := range []string{"/DocMDP", "/TransformParams", "/Perms"} {
		if !bytes.Contains(firmado, []byte(marca)) {
			t.Errorf("falta %s: el documento no queda certificado", marca)
		}
	}
}

func TestSinBloquearNoSeCertifica(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	firmado, err := FirmarConCertificadoPDF(docDePrueba(t, "acta"), OpcionesFirma{
		CertificadoPEM: certPEM,
		ClavePEM:       clavePEM,
		Rect:           [4]float64{80, 80, 300, 150},
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	// Certificar impide TODO cambio posterior, incluida una segunda firma: no
	// puede ser el comportamiento por defecto de un contrato que firman dos.
	if bytes.Contains(firmado, []byte("/DocMDP")) {
		t.Error("sin pedirlo, el documento no debería quedar certificado")
	}
}
