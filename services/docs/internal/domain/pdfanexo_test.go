// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// firmadoConFormulario es el impreso de pdfConFormulario firmado ANEXANDO, para
// que el formulario siga ahí: lo que llega cuando una parte firma y la otra
// todavía tiene que rellenar.
func firmadoConFormulario(t *testing.T) []byte {
	t.Helper()
	certPEM, clavePEM := identidadDePrueba(t)
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatal(err)
	}
	firmado, err := FirmarIncrementalPDF(d, OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Ana"})
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}
	return firmado
}

// conservaLaFirma comprueba lo que de verdad importa: la revisión firmada sigue
// byte a byte al principio del fichero nuevo, y la firma sigue siendo íntegra.
func conservaLaFirma(t *testing.T, que string, firmado, salida []byte) {
	t.Helper()
	if !bytes.HasPrefix(salida, firmado) {
		t.Fatalf("%s: se reescribió el documento firmado; tenía que anexarse", que)
	}
	if err := validarFirma(salida); err != nil {
		t.Errorf("%s: la firma dejó de validar: %v", que, err)
	}
}

// Anotar un PDF firmado lo reescribía entero y la firma dejaba de validar sin
// que nada lo dijera (284 de 472 salidas con firma la perdían, medido con
// scripts/certipdf-compat). Ahora se anexa.
func TestAnotarUnPDFFirmadoConservaLaFirma(t *testing.T) {
	firmado := firmadoConFormulario(t)
	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatal(err)
	}
	salida, err := AnadirAnotacionesPDF(d, []Anotacion{{Pagina: 0, Tipo: AnotNota, Rect: [4]float64{10, 10, 30, 30}, Texto: "visto"}})
	if err != nil {
		t.Fatalf("anotar: %v", err)
	}
	conservaLaFirma(t, "anotar", firmado, salida)
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(d2.AnotacionesDelDocumento()); n != len(d.AnotacionesDelDocumento())+1 {
		t.Errorf("anotaciones = %d, quería una más que antes", n)
	}
}

func TestRellenarUnPDFFirmadoConservaLaFirma(t *testing.T) {
	firmado := firmadoConFormulario(t)
	d, err := LeerPDF(firmado)
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Luis Gil", "acepto": "sí"})
	if err != nil {
		t.Fatalf("rellenar: %v", err)
	}
	conservaLaFirma(t, "rellenar", firmado, salida)
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d2.CamposDelFormulario() {
		if c.Nombre == "nombre" && c.Valor != "Luis Gil" {
			t.Errorf("nombre = %q, quería «Luis Gil»", c.Valor)
		}
		if c.Nombre == "acepto" && c.Valor != "Marcado" {
			t.Errorf("acepto = %q, quería «Marcado»", c.Valor)
		}
	}
}

// Una certificación con /P 1 no admite ningún cambio. Ni se anexa ni se
// reescribe a escondidas: se dice, y solo con permiso explícito se reescribe.
func TestCertificacionSinCambiosPidePermiso(t *testing.T) {
	datos, err := os.ReadFile(filepath.Join("testdata", "pdf", "certificacion-p1.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatal(err)
	}
	nota := []Anotacion{{Pagina: 0, Tipo: AnotNota, Rect: [4]float64{10, 10, 30, 30}, Texto: "x"}}
	if _, err := AnadirAnotacionesPDF(d, nota); !errors.Is(err, ErrInvalidaFirmas) {
		t.Fatalf("err = %v, quería ErrInvalidaFirmas", err)
	}
	d.PermitirInvalidarFirmas()
	if _, err := AnadirAnotacionesPDF(d, nota); err != nil {
		t.Fatalf("con permiso tenía que reescribirse: %v", err)
	}
}

// Sin firmas no cambia nada: se sigue reescribiendo, que es lo que quita el
// historial (pdfwrite.go, claveQueNoViaja).
func TestSinFirmasSeSigueReescribiendo(t *testing.T) {
	original := pdfConFormulario(t)
	d, err := LeerPDF(original)
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"nombre": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(salida, original) {
		t.Error("un documento sin firmas no tiene por qué anexarse")
	}
}

// Firmar REESCRIBIENDO (cuando no se puede anexar) borra las firmas que haya:
// sin permiso explícito se niega, igual que anotar o rellenar.
func TestReescribirParaFirmarUnFirmadoPidePermiso(t *testing.T) {
	d, err := LeerPDF(firmadoConFormulario(t))
	if err != nil {
		t.Fatal(err)
	}
	certPEM, clavePEM := identidadDePrueba(t)
	opts := OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Luis"}
	if _, err := FirmarConCertificadoPDF(d, opts); !errors.Is(err, ErrInvalidaFirmas) {
		t.Fatalf("err = %v, quería ErrInvalidaFirmas", err)
	}
	d.PermitirInvalidarFirmas()
	if _, err := FirmarConCertificadoPDF(d, opts); err != nil {
		t.Fatalf("con permiso tenía que firmarse: %v", err)
	}
}
