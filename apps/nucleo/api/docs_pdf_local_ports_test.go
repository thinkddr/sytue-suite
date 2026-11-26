// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

// portLocalFixture es un puerto que cumple las interfaces opcionales de la edición local.
type portLocalFixture struct {
	privatePDFDataFixture
	rutas []string
}

func (f *portLocalFixture) FileByPath(_ context.Context, ruta string) (File, error) {
	f.rutas = append(f.rutas, ruta)
	if ruta == "/home/ana/contrato.pdf" {
		return File{ID: "suelto", Name: "contrato.pdf", Path: ruta}, nil
	}
	return File{}, noEsta404(ruta)
}

func (*portLocalFixture) ListPDFs(context.Context) ([]File, error) {
	return []File{{ID: "suelto", Name: "contrato.pdf", Path: "/home/ana/contrato.pdf"}}, nil
}

func (*portLocalFixture) PDFAuthorName() string { return "Ana Pérez" }

// sinRed falla la prueba si algo intenta salir al servicio.
func sinRed(t *testing.T) *http.Client {
	return &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("salió una petición al servicio: %s", r.URL)
		return nil, errors.New("sin red")
	})}
}

func TestLocalPortsResolvePathsAndListWithoutTheService(t *testing.T) {
	data := &portLocalFixture{}
	pdf, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, sinRed(t)).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := pdf.PorRuta(t.Context(), "/home/ana/contrato.pdf")
	if err != nil || f.ID != "suelto" {
		t.Fatal("la ruta absoluta no se resolvió en el puerto", f, err)
	}
	var e *Error
	if _, err := pdf.PorRuta(t.Context(), "/PDF/no.pdf"); !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatal("un fichero que no está debe ser 404", err)
	}
	lista, err := pdf.Lista(t.Context())
	if err != nil || len(lista) != 1 || lista[0].Path != "/home/ana/contrato.pdf" {
		t.Fatal("la portada no listó los recientes del puerto", lista, err)
	}
	if got := pdf.annotationAuthor(); got != "Ana Pérez" {
		t.Fatal("el autor de las anotaciones no es el del perfil", got)
	}
	// Sin las interfaces, el autor sigue siendo el sujeto, como antes.
	plain, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, sinRed(t)).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.annotationAuthor(); got != "owner" {
		t.Fatal("cambió el autor del puerto sin nombre", got)
	}
}

func dosPaginas(textos ...string) []byte {
	w := pdfsimple.Nuevo()
	for i, s := range textos {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto(s, "F1", 14)
	}
	return w.Bytes()
}

func TestLocalMotorComparesVerifiesAndExtractsOnDevice(t *testing.T) {
	pdf, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, sinRed(t)).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	a := PDFFichero{Nombre: "a.pdf", Datos: dosPaginas("Primera hoja", "Segunda hoja")}
	b := PDFFichero{Nombre: "b.pdf", Datos: dosPaginas("Primera hoja", "Otra cosa distinta")}

	r, err := pdf.Motor(t.Context(), "/pdf/compare", nil, nil, a, b)
	if err != nil {
		t.Fatal(err)
	}
	var texto struct {
		Diferencias []struct {
			Pagina int `json:"pagina"`
		} `json:"diferencias"`
		Iguales bool `json:"iguales"`
	}
	if err := json.Unmarshal(r.Datos, &texto); err != nil || texto.Iguales || len(texto.Diferencias) != 1 || texto.Diferencias[0].Pagina != 2 {
		t.Fatal("comparar texto", string(r.Datos), err)
	}
	r, err = pdf.Motor(t.Context(), "/pdf/compare", nil, nil, a, a)
	if err != nil || !strings.Contains(string(r.Datos), `"iguales":true`) {
		t.Fatal("un documento consigo mismo debe ser igual", string(r.Datos), err)
	}
	r, err = pdf.Motor(t.Context(), "/pdf/compare", map[string]string{"modo": "visual", "tolerancia": "16"}, nil, a, b)
	if err != nil || !strings.Contains(string(r.Datos), `"modo":"visual"`) || !strings.Contains(string(r.Datos), `"iguales":false`) {
		t.Fatal("comparar píxeles", string(r.Datos), err)
	}
	if _, err = pdf.Motor(t.Context(), "/pdf/compare", map[string]string{"modo": "visual", "tolerancia": "900"}, nil, a, b); err == nil {
		t.Fatal("una tolerancia fuera de rango se rechaza")
	}
	if _, err = pdf.Motor(t.Context(), "/pdf/compare", nil, nil, a); err == nil {
		t.Fatal("comparar pide dos documentos")
	}

	r, err = pdf.Motor(t.Context(), "/pdf/verify", nil, nil, a)
	if err != nil || !strings.Contains(string(r.Datos), `"anclas":false`) {
		t.Fatal("verificar", string(r.Datos), err)
	}

	_, err = pdf.Motor(t.Context(), "/pdf/attachment", map[string]string{"nombre": "factura.xml"}, nil, a)
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatal("un adjunto que no está es 404", err)
	}
	if _, err = pdf.Motor(t.Context(), "/pdf/attachment", nil, nil, a); !errors.As(err, &e) || e.Status != http.StatusBadRequest {
		t.Fatal("sin nombre de adjunto es 400", err)
	}
}
