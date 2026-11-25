// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

// bytesDePrueba es un PDF de una página por texto, en bytes (lo que llega del host).
func bytesDePrueba(textos ...string) []byte {
	w := pdfsimple.Nuevo()
	for i, txt := range textos {
		if i > 0 {
			w.NuevaPagina()
		}
		w.Texto(txt, "F1", 12)
	}
	return w.Bytes()
}

func abrirEnSesion(t *testing.T, s *DeviceSession, datos []byte) (string, map[string]any) {
	t.Helper()
	id, resumen, err := s.Open(datos, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return id, resumen
}

// La sesión del dispositivo es el motor de siempre: lo que dibuja y lo que escribe se
// vuelve a leer con el lector, como en el resto de las pruebas del ensamblador.
func TestDeviceSessionRendersAndEditsWithTheSharedEngine(t *testing.T) {
	s := NewDeviceSession()
	id, resumen := abrirEnSesion(t, s, bytesDePrueba("uno", "dos", "tres"))
	if resumen["total"] != 3 {
		t.Fatalf("total = %v", resumen["total"])
	}
	vista, err := s.Render(context.Background(), id, map[string]string{"paginas": "1-2", "escala": "1", "ajeno": "x"})
	if err != nil || vista.ContentType != "application/json" {
		t.Fatalf("Render: %v %q", err, vista.ContentType)
	}
	var lote struct {
		Paginas map[string]string `json:"paginas"`
		Total   int               `json:"total"`
	}
	if json.Unmarshal(vista.Data, &lote) != nil || lote.Total != 3 || !strings.Contains(lote.Paginas["1"], "<svg") || lote.Paginas["3"] != "" {
		t.Fatalf("lote = %+v", lote)
	}

	// Reordenar es extraer en otro orden.
	orden, err := s.Pages([]string{id}, map[string]string{"op": "extraer", "paginas": "3,1,2"})
	if err != nil {
		t.Fatalf("reordenar: %v", err)
	}
	if got := textos(t, orden.Data); strings.Join(got, "|") != "tres|uno|dos" {
		t.Fatalf("orden = %v", got)
	}
	girada, err := s.Pages([]string{id}, map[string]string{"op": "rotar", "paginas": "2", "grados": "90"})
	if err != nil {
		t.Fatalf("rotar: %v", err)
	}
	if d, _ := LeerPDF(girada.Data); d == nil {
		t.Fatal("rotada ilegible")
	} else if p, _ := d.Pagina(1); p.Rotacion != 90 {
		t.Fatalf("rotación = %d", p.Rotacion)
	}
	if _, err := s.Pages([]string{id}, map[string]string{"op": "borrar", "paginas": "1-3"}); err == nil {
		t.Fatal("borró todas las páginas")
	}

	otro, _ := abrirEnSesion(t, s, bytesDePrueba("cuatro"))
	junto, err := s.Pages([]string{id, otro}, map[string]string{"op": "combinar"})
	if err != nil {
		t.Fatalf("combinar: %v", err)
	}
	if got := textos(t, junto.Data); strings.Join(got, "|") != "uno|dos|tres|cuatro" {
		t.Fatalf("combinado = %v", got)
	}

	nota, err := s.Annotate(id, map[string]string{"op": "poner", "anotaciones": `[{"pagina":0,"tipo":"nota","rect":[10,10,30,30],"color":[1,0.7,0.1],"opacidad":1,"grosor":1,"texto":"revisar","autor":"ana@example.test"}]`}, "ana@example.test")
	if err != nil {
		t.Fatalf("anotar: %v", err)
	}
	anotado, err := LeerPDF(nota.Data)
	if err != nil {
		t.Fatal(err)
	}
	anots := anotado.AnotacionesDelDocumento()
	if len(anots) != 1 || anots[0].Texto != "revisar" || anots[0].Autor != "ana@example.test" {
		t.Fatalf("anotaciones = %+v", anots)
	}

	comprimido, err := s.Tool(id, map[string]string{"op": "comprimir", "nivel": "sin-perdida"})
	if err != nil || comprimido.Compression == nil || comprimido.ContentType != "application/pdf" {
		t.Fatalf("comprimir: %v %+v", err, comprimido)
	}
	if _, err := s.Tool(id, map[string]string{"op": "proteger"}); err == nil {
		t.Fatal("una operación que no es de herramientas pasó")
	}
}

func TestDeviceSessionFillsForms(t *testing.T) {
	s := NewDeviceSession()
	id, resumen := abrirEnSesion(t, s, pdfConFormulario(t))
	if campos, _ := resumen["campos"].([]CampoFormulario); len(campos) != 2 {
		t.Fatalf("campos = %v", resumen["campos"])
	}
	relleno, err := s.Form(id, map[string]string{"op": "rellenar", "valores": `{"nombre":"Ana Pérez","acepto":"Marcado"}`}, "acta.pdf", nil)
	if err != nil {
		t.Fatalf("rellenar: %v", err)
	}
	d, err := LeerPDF(relleno.Data)
	if err != nil {
		t.Fatal(err)
	}
	campos := d.CamposDelFormulario()
	if campos[0].Valor != "Ana Pérez" || campos[1].Valor != "Marcado" {
		t.Fatalf("campos = %+v", campos)
	}
}

// Un identificador que no está abierto, un documento que no es PDF, una sesión llena
// o una operación de otro sitio fallan con su código y sin tocar nada.
func TestDeviceSessionRejectsWithStableCodes(t *testing.T) {
	s := NewDeviceSession()
	if _, err := s.Render(context.Background(), "d99", nil); DescribeDeviceError(err).Code != "pdf_not_open" {
		t.Fatalf("sin abrir: %v", err)
	}
	if _, _, err := s.Open([]byte("no soy un pdf"), ""); DescribeDeviceError(err).Code != "pdf_not_pdf" {
		t.Fatalf("no pdf: %v", err)
	}
	if _, _, err := s.Open(nil, ""); err == nil {
		t.Fatal("vacío aceptado")
	}
	var ultimo string
	for range MaxDeviceDocuments {
		ultimo, _ = abrirEnSesion(t, s, bytesDePrueba("x"))
	}
	if _, _, err := s.Open(bytesDePrueba("y"), ""); DescribeDeviceError(err).Code != "pdf_device_full" {
		t.Fatalf("lleno: %v", err)
	}
	if !s.Close(ultimo) || s.Close(ultimo) {
		t.Fatal("cerrar dos veces")
	}
	if _, _, err := s.Open(bytesDePrueba("y"), ""); err != nil {
		t.Fatalf("tras cerrar: %v", err)
	}
	if _, err := s.Pages([]string{ultimo}, map[string]string{"op": "rotar", "paginas": "1", "grados": "90"}); DescribeDeviceError(err).Code != "pdf_not_open" {
		t.Fatalf("cerrado: %v", err)
	}
	if _, err := s.Pages(nil, map[string]string{"op": "combinar"}); err == nil {
		t.Fatal("sin documentos")
	}
	// Una capa sin su texto no se dibuja (las capas, en device_session_boveda_test.go).
	if _, err := s.Pages([]string{"d1"}, map[string]string{"op": "marca-agua"}); DescribeDeviceError(err).Code != "bad_request" {
		t.Fatalf("marca de agua sin texto: %v", err)
	}
}

// Un PDF firmado no se reescribe sin que la persona lo confirme (`invalidar_firmas`), y
// la confirmación vale para ESA operación: el documento abierto no se queda con ella.
func TestDeviceSessionAsksBeforeInvalidatingSignatures(t *testing.T) {
	raw, err := os.ReadFile("testdata/pdf/firma-valida-rsa.pdf")
	if err != nil {
		t.Fatal(err)
	}
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, raw)
	girar := map[string]string{"op": "rotar", "paginas": "1", "grados": "90"}
	if _, err := s.Pages([]string{id}, girar); DescribeDeviceError(err).Code != "pdf_invalidates_signatures" {
		t.Fatalf("sin confirmar: %v", err)
	}
	girar["invalidar_firmas"] = "1"
	if _, err := s.Pages([]string{id}, girar); err != nil {
		t.Fatalf("confirmado: %v", err)
	}
	delete(girar, "invalidar_firmas")
	if _, err := s.Pages([]string{id}, girar); DescribeDeviceError(err).Code != "pdf_invalidates_signatures" {
		t.Fatalf("la confirmación se quedó pegada: %v", err)
	}
}
