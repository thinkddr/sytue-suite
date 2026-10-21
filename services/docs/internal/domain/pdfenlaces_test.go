// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de los enlaces. Aquí lo que más importa NO es que un enlace funcione:
// es que los que NO deben funcionar no salgan. Un PDF lo aporta quien llama y
// puede venir de cualquier sitio, así que `javascript:` dentro de un `href` que
// la interfaz pinte es ejecución de código con la sesión del usuario delante.
package domain

import (
	"strconv"
	"strings"
	"testing"
)

// docConEnlaces arma un PDF de dos páginas cuyas anotaciones se pasan tal cual.
func docConEnlaces(t *testing.T, annots ...string) *PDFDoc {
	t.Helper()
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	obj := func(cuerpo string) {
		offsets = append(offsets, b.Len())
		b.WriteString(strconv.Itoa(len(offsets)-1) + " 0 obj\n" + cuerpo + "\nendobj\n")
	}
	refs := make([]string, 0, len(annots))
	for i := range annots {
		refs = append(refs, strconv.Itoa(6+i)+" 0 R")
	}
	obj(`<< /Type /Catalog /Pages 2 0 R >>`)
	obj(`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 100 100] >>`)
	obj(`<< /Type /Page /Parent 2 0 R /Annots [` + strings.Join(refs, " ") + `] >>`)
	obj(`<< /Type /Page /Parent 2 0 R >>`)
	obj(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`) // relleno: los enlaces empiezan en el 6
	for _, a := range annots {
		obj(a)
	}

	inicio := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(len(offsets)) + "\n0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		b.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size " + strconv.Itoa(len(offsets)) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(inicio) + "\n%%EOF")

	d, err := LeerPDF([]byte(b.String()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	return d
}

// TestEnlacesRechazaLosEsquemasPeligrosos es EL check de esta pieza. La lista es
// BLANCA: enumerar lo prohibido deja fuera el esquema que alguien invente
// mañana, y basta con que uno se cuele para que un PDF ajeno ejecute código.
func TestEnlacesRechazaLosEsquemasPeligrosos(t *testing.T) {
	malos := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)", // el esquema no distingue mayúsculas
		"  javascript:alert(1)",
		"file:///etc/passwd",
		"data:text/html;base64,PHNjcmlwdD4=",
		"vbscript:msgbox(1)",
		"http://ejemplo.test/\x0d\x0aSet-Cookie:x", // control para partir la URL
	}
	for _, u := range malos {
		d := docConEnlaces(t,
			`<< /Type /Annot /Subtype /Link /Rect [10 10 90 30] /A << /S /URI /URI (`+u+`) >> >>`)
		if e := d.EnlacesDelDocumento(); len(e) != 0 {
			t.Errorf("%q se entregó como enlace: %+v", u, e)
		}
	}
}

// Y los que sí: sin ellos el visor no navega, que es el motivo de la pieza.
func TestEnlacesAceptaLoQueTieneSentidoEnUnDocumento(t *testing.T) {
	for _, u := range []string{
		"https://sytue.eu/ayuda",
		"http://ejemplo.test/a?b=1",
		"mailto:soporte@sytue.com",
	} {
		d := docConEnlaces(t,
			`<< /Type /Annot /Subtype /Link /Rect [10 10 90 30] /A << /S /URI /URI (`+u+`) >> >>`)
		e := d.EnlacesDelDocumento()
		if len(e) != 1 || e[0].URL != u {
			t.Errorf("%q no se entregó: %+v", u, e)
		}
		if e[0].Destino != -1 {
			t.Errorf("%q es externo: no debe traer destino interno (%d)", u, e[0].Destino)
		}
	}
}

// TestEnlaceInternoLlevaALaPagina: el índice interno de un informe se hace con
// estos, y son los que más se usan.
func TestEnlaceInternoLlevaALaPagina(t *testing.T) {
	d := docConEnlaces(t,
		// /Dest directo a la segunda página (objeto 4)
		`<< /Type /Annot /Subtype /Link /Rect [10 40 90 60] /Dest [4 0 R /Fit] >>`,
		// y la misma cosa dentro de una acción GoTo
		`<< /Type /Annot /Subtype /Link /Rect [10 70 90 90] /A << /S /GoTo /D [4 0 R /Fit] >> >>`)

	e := d.EnlacesDelDocumento()
	if len(e) != 2 {
		t.Fatalf("enlaces = %+v, quería 2", e)
	}
	for i, x := range e {
		if x.Destino != 1 {
			t.Errorf("enlace %d: destino = %d, quería la página 2 (índice 1)", i, x.Destino)
		}
		if x.URL != "" {
			t.Errorf("enlace %d: es interno, no debe traer URL (%q)", i, x.URL)
		}
		if x.Pagina != 0 {
			t.Errorf("enlace %d: está en la página 1, dijo %d", i, x.Pagina)
		}
	}
}

// TestEnlacesDescartaLasAccionesQueNoSonNavegacion: `Launch` ejecuta un programa
// y `GoToR` abre otro fichero. En un visor web no significan nada, y entregarlas
// sería ofrecer una zona pulsable que no hace lo que promete —o algo peor—.
func TestEnlacesDescartaLasAccionesQueNoSonNavegacion(t *testing.T) {
	d := docConEnlaces(t,
		`<< /Type /Annot /Subtype /Link /Rect [10 10 90 30] /A << /S /Launch /F (calc.exe) >> >>`,
		`<< /Type /Annot /Subtype /Link /Rect [10 40 90 60] /A << /S /GoToR /F (otro.pdf) >> >>`,
		`<< /Type /Annot /Subtype /Link /Rect [10 70 90 90] /A << /S /JavaScript /JS (app.alert\(1\)) >> >>`)

	if e := d.EnlacesDelDocumento(); len(e) != 0 {
		t.Errorf("se entregaron acciones que no son navegación: %+v", e)
	}
}

// TestEnlaceSinZonaNoSeEntrega: un rectángulo sin área no se puede pulsar, y
// puesto sobre el texto impide seleccionarlo. Es peor que no ponerlo.
func TestEnlaceSinZonaNoSeEntrega(t *testing.T) {
	d := docConEnlaces(t,
		`<< /Type /Annot /Subtype /Link /Rect [10 10 10 10] /A << /S /URI /URI (https://sytue.eu) >> >>`)
	if e := d.EnlacesDelDocumento(); len(e) != 0 {
		t.Errorf("un enlace sin área se entregó: %+v", e)
	}
}

// TestEnlaceConRectAlReves: las esquinas pueden venir en cualquier orden y hay
// generadores que las escriben al revés. Sin normalizar, la zona sale con ancho
// negativo y el navegador no pinta nada donde debería poder pulsarse.
func TestEnlaceConRectAlReves(t *testing.T) {
	d := docConEnlaces(t,
		`<< /Type /Annot /Subtype /Link /Rect [90 60 10 40] /A << /S /URI /URI (https://sytue.eu) >> >>`)
	e := d.EnlacesDelDocumento()
	if len(e) != 1 {
		t.Fatalf("enlaces = %+v, quería 1", e)
	}
	if e[0].Rect != [4]float64{10, 40, 90, 60} {
		t.Errorf("rect = %v, quería las esquinas ordenadas", e[0].Rect)
	}
}
