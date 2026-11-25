// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// LO QUE FALTABA EN EL DISPOSITIVO (ADR 0086, cuarta entrega): proteger, censurar,
// sellos y marcas, crear campos e importar XFDF, el índice, comparar, el texto para
// buscar y leer en voz alta, editar texto e imágenes, verificar y los adjuntos. Todo
// con el motor de verdad y releído con el lector, como el resto de la sesión.

// cuerpoDeFlujo captura el cuerpo de cada flujo de un fichero.
var cuerpoDeFlujo = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)

// enLosBytes dice si `aguja` está en el fichero: en claro o dentro de cualquier
// flujo comprimido con Flate. Censurar de verdad es que no esté en NINGUNO de los
// dos sitios; mirar solo los bytes crudos daría por borrado lo que solo se comprimió.
func enLosBytes(t *testing.T, datos []byte, aguja string) bool {
	t.Helper()
	if bytes.Contains(datos, []byte(aguja)) {
		return true
	}
	for _, m := range cuerpoDeFlujo.FindAllSubmatch(datos, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		claro, _ := io.ReadAll(io.LimitReader(r, 64<<20))
		_ = r.Close()
		if bytes.Contains(claro, []byte(aguja)) {
			return true
		}
	}
	return false
}

func textoDe(t *testing.T, datos []byte, clave string) string {
	t.Helper()
	d, err := LeerPDFConClave(datos, clave)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	out := make([]string, 0, d.NumPaginas())
	for i := range d.NumPaginas() {
		s, err := d.TextoPlano(i)
		if err != nil {
			t.Fatalf("TextoPlano(%d): %v", i, err)
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

// Proteger escribe AES-256 (V 5, R 6, AESV3) y pide la contraseña al abrir; con
// ella, la sesión lo abre y lo trabaja, y quitarla devuelve un PDF sin /Encrypt.
func TestDeviceSessionProtectsWithAES256(t *testing.T) {
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, bytesDePrueba("contrato reservado"))
	protegido, err := s.Protect(id, map[string]string{
		"op": "proteger", "usuario": "abre-me", "dueno": "dueño-secreto", "permisos": "imprimir,copiar",
	})
	if err != nil {
		t.Fatalf("proteger: %v", err)
	}
	for _, marca := range []string{"/V 5", "/R 6", "/AESV3", "/StdCF"} {
		if !bytes.Contains(protegido.Data, []byte(marca)) {
			t.Errorf("el /Encrypt no lleva %s", marca)
		}
	}
	if enLosBytes(t, protegido.Data, "contrato reservado") {
		t.Fatal("el texto sigue en claro dentro del PDF protegido")
	}
	if _, _, err := s.Open(protegido.Data, ""); DescribeDeviceError(err).Code != "pdf_password_required" {
		t.Fatalf("sin contraseña: %v", err)
	}
	if _, _, err := s.Open(protegido.Data, "otra"); DescribeDeviceError(err).Code != "pdf_password_required" {
		t.Fatalf("contraseña mala: %v", err)
	}
	abierto, resumen, err := s.Open(protegido.Data, "abre-me")
	if err != nil {
		t.Fatalf("con la contraseña: %v", err)
	}
	if resumen["cifrado"] != true {
		t.Fatalf("el resumen no dice cifrado: %v", resumen["cifrado"])
	}
	if !strings.Contains(textoDe(t, protegido.Data, "abre-me"), "contrato reservado") {
		t.Fatal("con la contraseña no se lee el texto")
	}
	libre, err := s.Protect(abierto, map[string]string{"op": "desproteger"})
	if err != nil {
		t.Fatalf("desproteger: %v", err)
	}
	if bytes.Contains(libre.Data, []byte("/Encrypt")) || !strings.Contains(textoDe(t, libre.Data, ""), "contrato reservado") {
		t.Fatal("desproteger no dejó un PDF sin contraseña con su texto")
	}
	if _, err := s.Protect(id, map[string]string{"op": "proteger"}); err == nil {
		t.Fatal("proteger sin contraseñas")
	}
}

// Censurar BORRA: ni el extractor ni los bytes del fichero (en claro o en flujos
// comprimidos) conservan lo marcado, y lo de fuera de la zona sigue.
func TestDeviceSessionRedactionRemovesTheBytes(t *testing.T) {
	const secreto = "IBAN ES9121000418450200051332"
	datos := pdfConLineas(t, "linea publica de arriba", secreto, "linea publica de abajo").buf
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, datos)
	doc, _ := s.lookup(id)
	y := yDelTexto(t, doc, "IBAN")
	areas, _ := json.Marshal([]map[string]float64{{"pagina": 0, "x1": 0, "y1": y - 4, "x2": 600, "y2": y + 12}})
	censurado, err := s.Protect(id, map[string]string{"op": "redactar", "areas": string(areas)})
	if err != nil {
		t.Fatalf("redactar: %v", err)
	}
	texto := textoDe(t, censurado.Data, "")
	if strings.Contains(texto, "IBAN") || strings.Contains(texto, "ES9121") {
		t.Fatalf("el extractor aún lo da: %q", texto)
	}
	for _, trozo := range []string{"ES9121000418450200051332", "IBAN ES91", "0418450200051332"} {
		if enLosBytes(t, censurado.Data, trozo) {
			t.Fatalf("%q sigue en los bytes: se tapó, no se borró", trozo)
		}
	}
	if !strings.Contains(texto, "arriba") || !strings.Contains(texto, "abajo") {
		t.Fatalf("se llevó lo de fuera de la zona: %q", texto)
	}
	if !enLosBytes(t, datos, "ES9121000418450200051332") {
		t.Fatal("la prueba no mira donde debe: el original sí lo lleva")
	}
}

// Las capas de «Sellos y marcas» van por `Pages`, sobre UN documento.
func TestDeviceSessionStampsLayers(t *testing.T) {
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, bytesDePrueba("uno", "dos"))
	marca, err := s.Pages([]string{id}, map[string]string{"op": "marca-agua", "texto": "BORRADOR"})
	if err != nil {
		t.Fatalf("marca de agua: %v", err)
	}
	if got := textoDe(t, marca.Data, ""); strings.Count(got, "BORRADOR") != 2 {
		t.Fatalf("la marca no está en las dos páginas: %q", got)
	}
	numerado, err := s.Pages([]string{id}, map[string]string{"op": "bates", "texto": "EXP-", "desde": "7", "digitos": "6"})
	if err != nil {
		t.Fatalf("bates: %v", err)
	}
	if got := textoDe(t, numerado.Data, ""); !strings.Contains(got, "EXP-000007") || !strings.Contains(got, "EXP-000008") {
		t.Fatalf("bates: %q", got)
	}
	otro, _ := abrirEnSesion(t, s, bytesDePrueba("tres"))
	if _, err := s.Pages([]string{id, otro}, map[string]string{"op": "pie", "texto": "x"}); err == nil {
		t.Fatal("una capa sobre dos documentos")
	}
	if _, err := s.Pages([]string{id}, map[string]string{"op": "encabezado"}); DescribeDeviceError(err).Code != "bad_request" {
		t.Fatalf("encabezado sin texto: %v", err)
	}
	firmado, err := os.ReadFile("testdata/pdf/firma-valida-rsa.pdf")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := abrirEnSesion(t, s, firmado)
	if _, err := s.Pages([]string{f}, map[string]string{"op": "marca-agua", "texto": "COPIA"}); DescribeDeviceError(err).Code != "pdf_invalidates_signatures" {
		t.Fatalf("un sello sobre un PDF firmado no pidió confirmación: %v", err)
	}
}

// Crear campos e importar sus valores desde un XFDF, todo en el dispositivo.
func TestDeviceSessionCreatesFieldsAndImportsXFDF(t *testing.T) {
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, bytesDePrueba("solicitud"))
	creado, err := s.Form(id, map[string]string{
		"op": "crear", "campos": `[{"tipo":"texto","nombre":"dni","pagina":0,"rect":[72,600,272,624]}]`,
	}, "solicitud.pdf", nil)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	conCampo, resumen := abrirEnSesion(t, s, creado.Data)
	if campos, _ := resumen["campos"].([]CampoFormulario); len(campos) != 1 || campos[0].Nombre != "dni" {
		t.Fatalf("campos = %v", resumen["campos"])
	}
	xfdf := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="dni"><value>12345678Z</value></field></fields></xfdf>`)
	importado, err := s.Form(conCampo, map[string]string{"op": "importar-datos"}, "solicitud.pdf", xfdf)
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if importado.ImportedFields != 1 {
		t.Fatalf("importados = %d", importado.ImportedFields)
	}
	d, err := LeerPDF(importado.Data)
	if err != nil {
		t.Fatal(err)
	}
	if c := d.CamposDelFormulario(); len(c) != 1 || c[0].Valor != "12345678Z" {
		t.Fatalf("campos = %+v", c)
	}
	exportado, err := s.Form(conCampo, map[string]string{"op": "exportar-datos"}, "solicitud.pdf", nil)
	if err != nil || !bytes.Contains(exportado.Data, []byte(`name="dni"`)) {
		t.Fatalf("exportar: %v %q", err, exportado.Data)
	}
}

// El índice, editar el texto, analizar la página y buscar y reemplazar.
func TestDeviceSessionEditsOutlineAndText(t *testing.T) {
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, bytesDePrueba("Capitulo primero", "Capitulo segundo"))
	indice, err := s.Edit(id, map[string]string{
		"op": "indice", "marcadores": `[{"titulo":"Uno","pagina":0,"nivel":0},{"titulo":"Dos","pagina":1,"nivel":0}]`,
	}, nil)
	if err != nil {
		t.Fatalf("indice: %v", err)
	}
	_, resumen := abrirEnSesion(t, s, indice.Data)
	marcas, _ := resumen["indice"].([]Marcador)
	if len(marcas) != 2 || marcas[1].Titulo != "Dos" || marcas[1].Pagina != 1 {
		t.Fatalf("índice = %+v", resumen["indice"])
	}

	analisis, err := s.Edit(id, map[string]string{"op": "analizar", "pagina": "1"}, nil)
	if err != nil || analisis.ContentType != "application/json" {
		t.Fatalf("analizar: %v %q", err, analisis.ContentType)
	}
	buscado, err := s.Edit(id, map[string]string{"op": "buscar-reemplazar", "busqueda": `{"buscar":"primero","reemplazar":"inicial"}`}, nil)
	if err != nil || buscado.ContentType != "application/json" || !bytes.Contains(buscado.Data, []byte("coincidencias")) {
		t.Fatalf("buscar: %v %q", err, buscado.Data)
	}
	cambiado, err := s.Edit(id, map[string]string{"op": "buscar-reemplazar", "aplicar": "1", "busqueda": `{"buscar":"primero","reemplazar":"inicial"}`}, nil)
	if err != nil {
		t.Fatalf("reemplazar: %v", err)
	}
	if cambiado.Replacements == nil || *cambiado.Replacements != 1 {
		t.Fatalf("reemplazos = %v", cambiado.Replacements)
	}
	if got := textoDe(t, cambiado.Data, ""); !strings.Contains(got, "inicial") || strings.Contains(got, "primero") {
		t.Fatalf("texto = %q", got)
	}
}

// Comparar dos documentos abiertos, por texto y por píxeles; el texto de cada página
// para buscar y leer en voz alta.
func TestDeviceSessionComparesAndReadsText(t *testing.T) {
	s := NewDeviceSession()
	a, _ := abrirEnSesion(t, s, bytesDePrueba("precio 100 euros"))
	b, _ := abrirEnSesion(t, s, bytesDePrueba("precio 900 euros"))
	raw, err := s.Compare(context.Background(), a, b, map[string]string{})
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	var texto struct {
		Diferencias []DiferenciaPagina `json:"diferencias"`
		Iguales     bool               `json:"iguales"`
	}
	if json.Unmarshal(raw, &texto) != nil || texto.Iguales || len(texto.Diferencias) != 1 {
		t.Fatalf("comparación de texto = %s", raw)
	}
	raw, err = s.Compare(context.Background(), a, b, map[string]string{"modo": "visual", "pagina": "1", "tolerancia": "10"})
	if err != nil || !bytes.Contains(raw, []byte(`"modo":"visual"`)) || !bytes.Contains(raw, []byte(`"imagen"`)) {
		t.Fatalf("comparación visual de una página: %v %s", err, raw)
	}
	if _, err := s.Compare(context.Background(), a, b, map[string]string{"modo": "visual", "tolerancia": "900"}); DescribeDeviceError(err).Code != "bad_request" {
		t.Fatalf("tolerancia fuera de rango: %v", err)
	}
	if _, err := s.Compare(context.Background(), a, "d99", nil); DescribeDeviceError(err).Code != "pdf_not_open" {
		t.Fatalf("sin el otro: %v", err)
	}
	raw, err = s.Text(context.Background(), a)
	if err != nil || !bytes.Contains(raw, []byte("precio 100 euros")) {
		t.Fatalf("texto: %v %s", err, raw)
	}
}

// Verificar en el dispositivo, sin anclas (lo dice), y un adjunto que no está.
func TestDeviceSessionVerifiesWithoutAnchors(t *testing.T) {
	firmado, err := os.ReadFile("testdata/pdf/firma-valida-rsa.pdf")
	if err != nil {
		t.Fatal(err)
	}
	s := NewDeviceSession()
	id, _ := abrirEnSesion(t, s, firmado)
	raw, err := s.Verify(id)
	if err != nil {
		t.Fatalf("verificar: %v", err)
	}
	var informe struct {
		Firmas []FirmaVerificada `json:"firmas"`
		Anclas bool              `json:"anclas"`
	}
	if json.Unmarshal(raw, &informe) != nil || informe.Anclas || len(informe.Firmas) != 1 {
		t.Fatalf("informe = %s", raw)
	}
	if _, err := s.Attachment(id, "factura.xml"); DescribeDeviceError(err).Code != "not_found" {
		t.Fatalf("adjunto que no está: %v", err)
	}
	if _, err := s.Attachment(id, " "); DescribeDeviceError(err).Code != "bad_request" {
		t.Fatalf("adjunto sin nombre: %v", err)
	}
}
