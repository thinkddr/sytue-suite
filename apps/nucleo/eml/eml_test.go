// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eml

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

const correo = "From: =?utf-8?q?Gestor=C3=ADa_P=C3=A9rez?= <hola@gestoria.example>\r\n" +
	"To: ada@example.org\r\n" +
	"Subject: Modelo 303\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"fuera\"\r\n\r\n" +
	"--fuera\r\n" +
	"Content-Type: multipart/alternative; boundary=\"dentro\"\r\n\r\n" +
	"--dentro\r\n" +
	"Content-Type: text/plain; charset=iso-8859-1\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
	"Adjunto la declaraci=F3n del tercer trimestre.\r\n" +
	"--dentro\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"PHA+QWRqdW50byBsYSA8Yj5kZWNsYXJhY2nDs248L2I+\r\nLjwvcD4=\r\n" +
	"--dentro--\r\n" +
	"--fuera\r\n" +
	"Content-Type: application/pdf; name=\"=?utf-8?q?declaraci=C3=B3n.pdf?=\"\r\n" +
	"Content-Disposition: attachment; filename=\"../../../.ssh/authorized_keys\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"JVBERi0xLjcK\r\n" +
	"--fuera--\r\n"

func TestLeer(t *testing.T) {
	m, err := Leer(strings.NewReader(correo))
	if err != nil {
		t.Fatal(err)
	}
	// Sin el CRLF final: el que precede a la frontera es de la frontera (RFC 2046 §5.1.1).
	if m.Texto != "Adjunto la declaración del tercer trimestre." {
		t.Errorf("texto (latin-1, quoted-printable): %q", m.Texto)
	}
	if m.HTML != "<p>Adjunto la <b>declaración</b>.</p>" {
		t.Errorf("html (base64 partido en renglones): %q", m.HTML)
	}
	if len(m.Adjuntos) != 1 || string(m.Adjuntos[0].Datos) != "%PDF-1.7\n" || m.Adjuntos[0].Tipo != "application/pdf" {
		t.Fatalf("adjuntos: %+v", m.Adjuntos)
	}
	// El nombre acabará siendo un fichero en el disco de alguien: sin directorios.
	if n := m.Adjuntos[0].Nombre; n != "authorized_keys" {
		t.Errorf("el adjunto se llama %q", n)
	}
}

func TestLeerLoQueNoEsDeLibro(t *testing.T) {
	// Sin Content-Type es texto; y un nombre solo en `name=`, con RFC 2047, se decodifica.
	m, err := Leer(strings.NewReader("Subject: x\r\n\r\nhola\r\n"))
	if err != nil || m.Texto != "hola\r\n" {
		t.Fatalf("sin Content-Type: %q, %v", m.Texto, err)
	}
	if n := nombreDe("", "=?utf-8?q?n=C3=B3mina.pdf?="); n != "nómina.pdf" {
		t.Errorf("nombre RFC 2047: %q", n)
	}
	if n := nombreDe(`C:\Users\ada\..\factura.pdf`); n != "factura.pdf" {
		t.Errorf("ruta de Windows: %q", n)
	}
	// Una muñeca rusa de multiparts no se sigue hasta el fondo.
	var b strings.Builder
	b.WriteString("Content-Type: multipart/mixed; boundary=b0\r\n\r\n")
	for i := range 12 {
		b.WriteString("--b" + string(rune('0'+i)) + "\r\nContent-Type: multipart/mixed; boundary=b" + string(rune('0'+i+1)) + "\r\n\r\n")
	}
	if _, err := Leer(strings.NewReader(b.String())); err == nil {
		t.Error("doce niveles de anidamiento han pasado")
	}
}

func TestComponer(t *testing.T) {
	para, err := Direcciones(`Grace Hopper <grace@example.org>, linus@example.org`)
	if err != nil || len(para) != 2 || para[0] != "grace@example.org" {
		t.Fatalf("Direcciones: %v, %v", para, err)
	}
	if _, err := Direcciones("esto no es un correo"); err == nil {
		t.Error("Direcciones acepta lo que no es una dirección")
	}
	crudo, err := Componer(Salida{
		De: "ada@example.org", DeNombre: "Ada Lovelace", Para: para,
		// Un salto de línea en el asunto sería OTRA cabecera.
		Asunto:       "Reunión del jueves\r\nBcc: espia@example.net",
		Texto:        "Hola:\n\n¿Os viene bien a las 10? El acta está en Notepads — «Obra de Málaga».\n",
		EnRespuestaA: "<abc@example.org>", Referencias: []string{"<abc@example.org>"},
	}, time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	texto := string(crudo)
	if strings.Contains(texto, "\nBcc:") {
		t.Fatalf("inyección de cabecera:\n%s", texto)
	}
	if strings.Contains(strings.ReplaceAll(texto, "\r\n", ""), "\n") {
		t.Error("hay saltos de línea que no son CRLF")
	}
	leido, err := mail.ReadMessage(strings.NewReader(texto))
	if err != nil {
		t.Fatal(err)
	}
	asunto, _ := palabras.DecodeHeader(leido.Header.Get("Subject"))
	if asunto != "Reunión del jueves Bcc: espia@example.net" {
		t.Errorf("asunto: %q", asunto)
	}
	if id := leido.Header.Get("Message-ID"); !strings.HasSuffix(id, "@example.org>") || len(id) < 40 {
		t.Errorf("Message-ID: %q", id)
	}
	if leido.Header.Get("In-Reply-To") != "<abc@example.org>" {
		t.Errorf("In-Reply-To: %q", leido.Header.Get("In-Reply-To"))
	}
	// Ida y vuelta: lo compuesto se lee igual.
	m, err := Leer(strings.NewReader(texto))
	if err != nil || !strings.Contains(m.Texto, "«Obra de Málaga»") || !strings.HasPrefix(m.Texto, "Hola:\r\n\r\n¿Os viene") {
		t.Fatalf("ida y vuelta: %q, %v", m.Texto, err)
	}
	if _, err := Componer(Salida{De: "ada@example.org"}, time.Now()); err == nil {
		t.Error("compone un mensaje sin destinatarios")
	}
}

// La copia oculta viaja en el SOBRE y nunca en el mensaje que se envía; un borrador sí la
// guarda, o al retomarlo se habría perdido. Y un adjunto va y vuelve byte a byte, con un
// nombre que no puede ser ni una ruta ni otra cabecera.
func TestComponerConCopiaOcultaYAdjuntos(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("contenido binario \x00\x01\xff ", 40))
	s := Salida{
		De: "ada@example.org", Para: []string{"grace@example.org"}, Cc: []string{"linus@example.org"},
		Cco: []string{"oculta@example.org", "GRACE@example.org"}, Asunto: "Declaración", Texto: "Va adjunta — «303».\n",
		Adjuntos: []Adjunto{
			{Nombre: "../../etc/declaración 3T.pdf", Tipo: "application/pdf", Datos: pdf},
			{Nombre: "a.txt\r\nBcc: espia@example.net", Tipo: "no es un tipo", Datos: []byte("hola")},
		},
	}
	if d := s.Destinatarios(); len(d) != 3 || d[2] != "oculta@example.org" {
		t.Fatalf("sobre: %v", d)
	}
	enviado, err := Componer(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enviado), "oculta@example.org") || strings.Contains(strings.ToLower(string(enviado)), "\nbcc:") {
		t.Fatalf("la copia oculta sale en el mensaje enviado:\n%s", enviado)
	}
	if strings.Contains(strings.ReplaceAll(string(enviado), "\r\n", ""), "\n") {
		t.Error("hay saltos de línea que no son CRLF")
	}
	m, err := Leer(strings.NewReader(string(enviado)))
	if err != nil || m.Texto != "Va adjunta — «303».\r\n" || len(m.Adjuntos) != 2 {
		t.Fatalf("ida y vuelta: %q, %d adjuntos, %v", m.Texto, len(m.Adjuntos), err)
	}
	if a := m.Adjuntos[0]; a.Nombre != "declaración 3T.pdf" || a.Tipo != "application/pdf" || !bytes.Equal(a.Datos, pdf) {
		t.Errorf("adjunto: %q %q, %d bytes de %d", a.Nombre, a.Tipo, len(a.Datos), len(pdf))
	}
	if a := m.Adjuntos[1]; a.Tipo != "application/octet-stream" || strings.ContainsAny(a.Nombre, "\r\n") || string(a.Datos) != "hola" {
		t.Errorf("adjunto con nombre hostil: %+v", a)
	}

	s.Borrador, s.Para, s.Cc = true, nil, nil
	s.EnRespuestaA, s.Referencias = "<b@example.org>", []string{"<a@example.org>", "<b@example.org>"}
	guardado, err := Componer(s, time.Now())
	if err != nil {
		t.Fatalf("un borrador solo con copia oculta: %v", err)
	}
	b, err := Leer(strings.NewReader(string(guardado)))
	if err != nil || len(b.Cco) != 2 || b.Cco[0] != "oculta@example.org" || b.Asunto != "Declaración" || b.EnRespuestaA != "<b@example.org>" || len(b.Referencias) != 2 {
		t.Fatalf("el borrador no se retoma entero: %+v, %v", b, err)
	}
	if _, err := Componer(Salida{De: "ada@example.org", Borrador: true}, time.Now()); err != nil {
		t.Errorf("un borrador vacío no se puede guardar: %v", err)
	}
}

// Con formato, el cuerpo sale como `multipart/alternative` —el texto para quien no pinta
// HTML y el HTML para quien sí—, solo o dentro del `multipart/mixed` de los adjuntos, y
// se lee de vuelta con sus dos partes.
func TestComponerConHTML(t *testing.T) {
	s := Salida{
		De: "ada@example.org", Para: []string{"grace@example.org"}, Asunto: "Con formato",
		Texto: "Hola, Grace", HTML: "<p>Hola, <strong>Grace</strong> — «ñ»</p>",
	}
	for _, conAdjunto := range []bool{false, true} {
		if conAdjunto {
			s.Adjuntos = []Adjunto{{Nombre: "a.txt", Tipo: "text/plain", Datos: []byte("adjunto")}}
		}
		crudo, err := Componer(s, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		leido, err := mail.ReadMessage(bytes.NewReader(crudo))
		if err != nil {
			t.Fatal(err)
		}
		tipo, _, _ := mime.ParseMediaType(leido.Header.Get("Content-Type"))
		if quiero := map[bool]string{false: "multipart/alternative", true: "multipart/mixed"}[conAdjunto]; tipo != quiero {
			t.Errorf("con adjunto %v: %s, quiero %s", conAdjunto, tipo, quiero)
		}
		m, err := Leer(bytes.NewReader(crudo))
		if err != nil || m.Texto != "Hola, Grace" || m.HTML != s.HTML || len(m.Adjuntos) != len(s.Adjuntos) {
			t.Errorf("con adjunto %v, ida y vuelta: %q %q %d %v", conAdjunto, m.Texto, m.HTML, len(m.Adjuntos), err)
		}
	}
}

// arbol describe la forma MIME de un mensaje: `mixed[related[alternative[plain,html],png<a>],png]`,
// con el `Content-ID` de cada parte en línea.
func arbol(t *testing.T, h textproto.MIMEHeader, cuerpo io.Reader) string {
	t.Helper()
	tipo, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type %q: %v", h.Get("Content-Type"), err)
	}
	_, sub, _ := strings.Cut(tipo, "/")
	if !strings.HasPrefix(tipo, "multipart/") {
		if cid := h.Get("Content-ID"); cid != "" {
			disp, _, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
			return sub + cid + disp
		}
		return sub
	}
	if tipo == "multipart/related" {
		sub += "(" + params["type"] + ")"
	}
	var hijos []string
	r := multipart.NewReader(cuerpo, params["boundary"])
	for {
		p, err := r.NextRawPart()
		if err != nil {
			break
		}
		hijos = append(hijos, arbol(t, p.Header, p))
	}
	return sub + "[" + strings.Join(hijos, ",") + "]"
}

// TestLasImagenesDelCuerpoVanEnRelated: la imagen que el HTML cita con `cid:` va DENTRO
// del cuerpo (`multipart/related`, `inline`, con su `Content-ID`), la que no cita va como
// un adjunto más, y con ficheros el `related` es la primera parte del `mixed`, como en
// `buildRfc5322` de la web. Leer devuelve el `cid`, que es lo que deja reabrir un
// borrador con sus imágenes; y un `cid` con saltos y ángulos no parte la cabecera: se
// escribe limpio, que es como lo cita el HTML.
func TestLasImagenesDelCuerpoVanEnRelated(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	s := Salida{
		De: "ada@example.org", Para: []string{"grace@example.org"}, Texto: "Mira",
		HTML: `<p>Mira <img src="cid:img1" alt=""><img src="cid:raraBcc:eve@example.org"></p>`,
		Adjuntos: []Adjunto{
			{Nombre: "foto.png", Tipo: "image/png", Datos: png, CID: "img1"},
			{Nombre: "otra.png", Tipo: "image/png", Datos: png, CID: "rara>\r\nBcc: eve@example.org"},
			{Nombre: "huerfana.png", Tipo: "image/png", Datos: png, CID: "huerfana"},
		},
	}
	forma := func(s Salida) string {
		t.Helper()
		crudo, err := Componer(s, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(crudo), "\r\nBcc:") {
			t.Fatalf("un cid ha escrito una cabecera:\n%s", crudo)
		}
		m, err := mail.ReadMessage(bytes.NewReader(crudo))
		if err != nil {
			t.Fatal(err)
		}
		return arbol(t, textproto.MIMEHeader(m.Header), m.Body)
	}
	if got, quiero := forma(s), "mixed[related(multipart/alternative)[alternative[plain,html],png<img1>inline,png<raraBcc:eve@example.org>inline],png]"; got != quiero {
		t.Errorf("con una imagen sin citar:\n%s\nquiero\n%s", got, quiero)
	}
	s.HTML = strings.ReplaceAll(s.HTML, `<img src="cid:raraBcc:eve@example.org">`, "")
	s.Adjuntos = s.Adjuntos[:1]
	if got, quiero := forma(s), "related(multipart/alternative)[alternative[plain,html],png<img1>inline]"; got != quiero {
		t.Errorf("solo con imágenes del cuerpo:\n%s\nquiero\n%s", got, quiero)
	}
	crudo, _ := Componer(s, time.Now())
	if m, err := Leer(bytes.NewReader(crudo)); err != nil || len(m.Adjuntos) != 1 || m.Adjuntos[0].CID != "img1" || m.HTML != s.HTML {
		t.Errorf("ida y vuelta: %+v %v", m.Adjuntos, err)
	}
}

// TestUnCorreoDeMuchasPartesNoEsCuadratico (FYC-03): cien mil partes de texto
// diminutas se leen enseguida, y el cuerpo no pasa del tope.
func TestUnCorreoDeMuchasPartesNoEsCuadratico(t *testing.T) {
	var b strings.Builder
	b.WriteString("Content-Type: multipart/mixed; boundary=x\r\n\r\n")
	for range 100_000 {
		b.WriteString("--x\r\nContent-Type: text/plain\r\n\r\nhola mundo\r\n")
	}
	b.WriteString("--x--\r\n")
	inicio := time.Now()
	m, _ := Leer(strings.NewReader(b.String()))
	if d := time.Since(inicio); d > 5*time.Second {
		t.Fatalf("leer el correo ha tardado %v", d)
	}
	if len(m.Texto) > maxTexto || m.Texto == "" {
		t.Fatalf("cuerpo de %d bytes", len(m.Texto))
	}
}

// TestElNombreDeUnAdjuntoNoGiraLaExtension (FYC-04).
func TestElNombreDeUnAdjuntoNoGiraLaExtension(t *testing.T) {
	if n := nombreDe("factura\u202Efdp.exe"); n != "facturafdp.exe" {
		t.Fatalf("nombre: %q", n)
	}
}
