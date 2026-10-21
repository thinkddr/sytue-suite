// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package eml lee y compone mensajes RFC 5322.
//
// Hace falta porque Sytue Mail guarda el .eml CRUDO y nada más: no expone el cuerpo ni
// los adjuntos por partes, y es el cliente quien descompone el MIME (la web lo hace en
// packages/api-clients/src/mail). Y al enviar, lo mismo al revés: el servicio recibe el
// mensaje ya compuesto, lo firma y lo encola.
//
// Lo que entra aquí lo ha escrito un desconocido: todo lleva tope (tamaño, anidamiento,
// número de partes) y nada de lo que diga se usa como ruta ni como cabecera sin limpiar.
//
// # Por qué vive en el núcleo
//
// Nació en el `internal/eml` de la suite en Gio, y desde ahí NO se podía importar:
// `internal/` lo cerraba al subárbol de Gio, retirado del repositorio el 2026-09-23. Sytue Mail en Fyne lo necesita entero —leer el cuerpo y componer lo que
// se envía—, así que se trae aquí, que es donde vive lo que no depende del toolkit. La
// copia de Gio murió con Gio; ESTA es la que se mantiene, y lo nuevo entra aquí.
package eml

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"path"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/encoding/htmlindex"

	"git.sytue.eu/sytue/one/apps/nucleo/nombres"
)

// Los topes de lo que se lee de un mensaje ajeno.
const (
	maxMensaje  = 50 << 20 // el servicio no admite más por SMTP
	maxTexto    = 2 << 20  // de cuerpo que se enseña
	maxAnidado  = 8
	maxAdjuntos = 200
	// maxPartes acota cuántas partes MIME se recorren: un correo de 50 MiB con
	// millones de partes diminutas costaba minutos de CPU al abrirlo (FYC-03).
	maxPartes = 10_000
)

// Adjunto es un fichero dentro de un mensaje.
type Adjunto struct {
	Nombre string
	Tipo   string
	Datos  []byte
	// CID es el `Content-ID` sin ángulos: la imagen que el HTML del cuerpo pinta con
	// `<img src="cid:…">`. Al componer, la que el HTML cita va DENTRO del cuerpo
	// (`multipart/related`) y no como fichero; la que no cita, como un adjunto más.
	CID string
}

// Mensaje es lo que se enseña de un correo: su texto, su HTML si lo trae, y sus adjuntos.
type Mensaje struct {
	Texto    string
	HTML     string
	Adjuntos []Adjunto
	// Las cabeceras que hacen falta para RETOMAR un borrador (el sobre del servicio no
	// basta: un borrador sin enviar es lo único que lleva «Bcc») y para seguir un hilo.
	Para, Cc, Cco []string
	Asunto        string
	EnRespuestaA  string
	Referencias   []string
}

// Leer descompone un .eml. Un mensaje mal formado devuelve lo que se haya podido leer
// junto al error: medio correo legible es mejor que ninguno.
func Leer(r io.Reader) (Mensaje, error) {
	var m Mensaje
	crudo, err := mail.ReadMessage(io.LimitReader(r, maxMensaje))
	if err != nil {
		return m, err
	}
	m.Para, m.Cc, m.Cco = direccionesDe(crudo.Header, "To"), direccionesDe(crudo.Header, "Cc"), direccionesDe(crudo.Header, "Bcc")
	m.Asunto = crudo.Header.Get("Subject")
	if dec, err := palabras.DecodeHeader(m.Asunto); err == nil {
		m.Asunto = dec
	}
	m.EnRespuestaA = strings.TrimSpace(crudo.Header.Get("In-Reply-To"))
	m.Referencias = strings.Fields(crudo.Header.Get("References"))
	l := &lector{m: &m}
	err = l.parte(textproto.MIMEHeader(crudo.Header), crudo.Body, 0)
	m.HTML, m.Texto = l.html.String(), l.texto.String()
	return m, err
}

// lector es el recorrido de las partes. El cuerpo se acumula en un Builder y con
// tope: con `m.Texto += texto` cada parte copiaba todo lo anterior, y un correo de
// millones de partes era cuadrático (FYC-03).
type lector struct {
	m           *Mensaje
	html, texto strings.Builder
	partes      int
}

// acumular añade `s` a `b` sin pasar de maxTexto.
func acumular(b *strings.Builder, s string) {
	if queda := maxTexto - b.Len(); queda > 0 {
		b.WriteString(strings.ToValidUTF8(s[:min(len(s), queda)], ""))
	}
}

// direccionesDe son las direcciones de una cabecera; una ilegible se queda en ninguna.
func direccionesDe(c mail.Header, nombre string) []string {
	lista, err := (&mail.AddressParser{WordDecoder: &palabras}).ParseList(c.Get(nombre))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(lista))
	for _, a := range lista {
		out = append(out, a.Address)
	}
	return out
}

func (l *lector) parte(cabecera textproto.MIMEHeader, cuerpo io.Reader, nivel int) error {
	if nivel > maxAnidado {
		return errors.New("eml: demasiados niveles de anidamiento")
	}
	l.partes++
	if l.partes > maxPartes {
		return errors.New("eml: demasiadas partes")
	}
	m := l.m
	tipo, params, err := mime.ParseMediaType(cabecera.Get("Content-Type"))
	if err != nil {
		tipo, params = "text/plain", nil // sin Content-Type —o ilegible— es texto (RFC 2045 §5.2)
	}
	if strings.HasPrefix(tipo, "multipart/") {
		partes := multipart.NewReader(cuerpo, params["boundary"])
		for {
			p, err := partes.NextRawPart() // «Raw»: la codificación se deshace aquí, igual para todas
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := l.parte(p.Header, p, nivel+1); err != nil {
				return err
			}
		}
	}
	datos, err := io.ReadAll(io.LimitReader(decodificar(cabecera.Get("Content-Transfer-Encoding"), cuerpo), maxMensaje))
	if err != nil && len(datos) == 0 {
		return err
	}
	disposicion, dparams, _ := mime.ParseMediaType(cabecera.Get("Content-Disposition"))
	nombre := nombreDe(dparams["filename"], params["name"])
	esTexto := tipo == "text/plain" || tipo == "text/html"
	if disposicion == "attachment" || nombre != "" || !esTexto {
		if len(m.Adjuntos) < maxAdjuntos {
			cid := strings.Trim(strings.TrimSpace(cabecera.Get("Content-ID")), "<>")
			m.Adjuntos = append(m.Adjuntos, Adjunto{Nombre: nombre, Tipo: tipo, Datos: datos, CID: cid})
		}
		return nil
	}
	texto := aUTF8(datos[:min(len(datos), maxTexto)], params["charset"])
	if tipo == "text/html" {
		acumular(&l.html, texto)
	} else {
		acumular(&l.texto, texto)
	}
	return nil
}

func decodificar(codificacion string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(codificacion)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, sinSaltos{r})
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	}
	return r
}

// sinSaltos quita los saltos de línea de un base64 partido en renglones, que el
// decodificador de la biblioteca estándar no tolera en todas sus formas.
type sinSaltos struct{ r io.Reader }

func (s sinSaltos) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	k := 0
	for _, b := range p[:n] {
		if b != '\r' && b != '\n' && b != ' ' && b != '\t' {
			p[k] = b
			k++
		}
	}
	if k == 0 && n > 0 && err == nil {
		return s.Read(p) // todo eran saltos: devolver 0 sin error se toma por atasco
	}
	return k, err
}

func aUTF8(datos []byte, charset string) string {
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		return strings.ToValidUTF8(string(datos), "�")
	}
	if cod, err := htmlindex.Get(charset); err == nil {
		if salida, err := cod.NewDecoder().Bytes(datos); err == nil {
			return string(salida)
		}
	}
	return strings.ToValidUTF8(string(datos), "�")
}

var palabras = mime.WordDecoder{CharsetReader: func(charset string, r io.Reader) (io.Reader, error) {
	cod, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return cod.NewDecoder().Reader(r), nil
}}

// nombreDe es el nombre del adjunto, decodificado y SIN directorios: «../../.bashrc» es
// «.bashrc», porque acabará siendo el nombre de un fichero en el disco de alguien.
func nombreDe(candidatos ...string) string {
	for _, c := range candidatos {
		if c == "" {
			continue
		}
		if dec, err := palabras.DecodeHeader(c); err == nil {
			c = dec
		}
		c = path.Base(strings.ReplaceAll(nombres.Limpio(c), "\\", "/"))
		if c != "." && c != "/" {
			return c
		}
	}
	return ""
}

// Salida es un mensaje por enviar, o un borrador por guardar.
type Salida struct {
	De, DeNombre  string
	Para, Cc, Cco []string
	Asunto        string
	Texto         string
	// HTML es el cuerpo con formato del editor enriquecido. Si lo hay, el cuerpo sale
	// como `multipart/alternative`: el texto para quien no pinta HTML y el HTML para
	// quien sí, que es lo que hace la web (`buildRfc5322` con `html`).
	HTML string
	// EnRespuestaA y Referencias son los Message-ID del hilo al que se contesta.
	EnRespuestaA string
	Referencias  []string
	Adjuntos     []Adjunto
	// Borrador: se guarda, no se envía. Puede no tener destinatarios todavía, y es el
	// ÚNICO caso en que se escribe «Bcc»: en un envío la copia oculta viaja en el sobre
	// (Destinatarios) y jamás en el mensaje, que el servicio firma y reparte tal cual.
	Borrador bool
}

// Destinatarios es el sobre: todos —también los ocultos—, una vez cada uno.
func (s Salida) Destinatarios() []string {
	var out []string
	for _, d := range slices.Concat(s.Para, s.Cc, s.Cco) {
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, d) }) {
			out = append(out, d)
		}
	}
	return out
}

// Direcciones lee lo que una persona escribe en «Para»: direcciones separadas por comas,
// con o sin nombre. Devuelve solo las direcciones, validadas.
func Direcciones(escrito string) ([]string, error) {
	if strings.TrimSpace(escrito) == "" {
		return nil, nil
	}
	lista, err := mail.ParseAddressList(escrito)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(lista))
	for _, a := range lista {
		out = append(out, a.Address)
	}
	return out, nil
}

// unaLinea deja un valor de cabecera en una línea: un salto dentro del asunto sería otra
// cabecera —«Bcc: …»— escrita por quien compuso el enlace o pegó el texto.
func unaLinea(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }), " ")
}

// Componer escribe el mensaje RFC 5322: texto plano en UTF-8, quoted-printable, con
// CRLF; con adjuntos, multipart/mixed con cada fichero en base64, y las imágenes que el
// HTML cita (`cid:`) en `multipart/related` con él. Las direcciones tienen que venir ya
// validadas (Direcciones).
func Componer(s Salida, ahora time.Time) ([]byte, error) {
	if s.De == "" || (!s.Borrador && len(s.Destinatarios()) == 0) {
		return nil, errors.New("eml: faltan remitente o destinatarios")
	}
	var b bytes.Buffer
	cabecera := func(nombre, valor string) {
		if valor != "" {
			b.WriteString(nombre + ": " + valor + "\r\n")
		}
	}
	lista := func(direcciones []string) string {
		partes := make([]string, 0, len(direcciones))
		for _, d := range direcciones {
			partes = append(partes, (&mail.Address{Address: unaLinea(d)}).String())
		}
		return strings.Join(partes, ", ")
	}
	aleatorio := make([]byte, 16)
	_, _ = rand.Read(aleatorio) // crypto/rand.Read no falla: aborta el proceso si no hay entropía
	dominio := s.De[strings.LastIndexByte(s.De, '@')+1:]

	cabecera("From", (&mail.Address{Name: unaLinea(s.DeNombre), Address: unaLinea(s.De)}).String())
	cabecera("To", lista(s.Para))
	cabecera("Cc", lista(s.Cc))
	if s.Borrador {
		cabecera("Bcc", lista(s.Cco))
	}
	cabecera("Subject", mime.QEncoding.Encode("utf-8", unaLinea(s.Asunto)))
	cabecera("Date", ahora.Format(time.RFC1123Z))
	cabecera("Message-ID", "<"+hex.EncodeToString(aleatorio)+"@"+unaLinea(dominio)+">")
	cabecera("In-Reply-To", unaLinea(s.EnRespuestaA))
	cabecera("References", unaLinea(strings.Join(s.Referencias, " ")))
	cabecera("MIME-Version", "1.0")
	var enLinea, sueltos []Adjunto
	for _, a := range s.Adjuntos {
		if cid := limpiarCID(a.CID); cid != "" && strings.Contains(s.HTML, "cid:"+cid) {
			enLinea = append(enLinea, a)
		} else {
			sueltos = append(sueltos, a)
		}
	}
	if len(sueltos) == 0 {
		err := conImagenes(&b, s, enLinea, cabecera)
		return b.Bytes(), err
	}
	// Con ficheros, `multipart/mixed`: el cuerpo —con sus imágenes, si las hay— es su
	// primera parte y los ficheros van al lado (`buildRfc5322` de la web).
	partes := multipart.NewWriter(&b)
	cabecera("Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": partes.Boundary()}))
	b.WriteString("\r\n")
	if err := anidar(partes, func(w *bytes.Buffer, c func(nombre, valor string)) error {
		return conImagenes(w, s, enLinea, c)
	}); err != nil {
		return nil, err
	}
	for _, a := range sueltos {
		if err := adjuntar(partes, a, ""); err != nil {
			return nil, err
		}
	}
	err := partes.Close()
	return b.Bytes(), err
}

// limpiarCID deja un `Content-ID` que se puede escribir en su cabecera: sin ángulos,
// blancos ni saltos, que partirían la cabecera.
func limpiarCID(cid string) string {
	return strings.Map(func(r rune) rune {
		if r == '<' || r == '>' || r <= ' ' {
			return -1
		}
		return r
	}, cid)
}

// anidar escribe en `mw` una parte con lo que ponga `escribir`: sus cabeceras y, tras la
// línea en blanco, su contenido.
func anidar(mw *multipart.Writer, escribir func(w *bytes.Buffer, cabecera func(nombre, valor string)) error) error {
	h := textproto.MIMEHeader{}
	var dentro bytes.Buffer
	if err := escribir(&dentro, h.Set); err != nil {
		return err
	}
	p, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = p.Write(bytes.TrimPrefix(dentro.Bytes(), []byte("\r\n")))
	return err
}

// conImagenes es el cuerpo con las imágenes que cita: `multipart/related`, que es lo que
// las ata al texto —en un `mixed` llegarían como ficheros sueltos y el hueco saldría
// roto—, o el cuerpo solo si no cita ninguna.
func conImagenes(w *bytes.Buffer, s Salida, enLinea []Adjunto, cabecera func(nombre, valor string)) error {
	if len(enLinea) == 0 {
		return cuerpo(w, s, cabecera)
	}
	rel := multipart.NewWriter(w)
	// `type` dice cuál de las partes se pinta y cuáles son sus recursos (RFC 2387).
	cabecera("Content-Type", mime.FormatMediaType("multipart/related",
		map[string]string{"type": "multipart/alternative", "boundary": rel.Boundary()}))
	w.WriteString("\r\n")
	if err := anidar(rel, func(w *bytes.Buffer, c func(nombre, valor string)) error { return cuerpo(w, s, c) }); err != nil {
		return err
	}
	for _, a := range enLinea {
		if err := adjuntar(rel, a, limpiarCID(a.CID)); err != nil {
			return err
		}
	}
	return rel.Close()
}

// adjuntar escribe un fichero en base64: como adjunto o, con su `cid`, en línea.
func adjuntar(mw *multipart.Writer, a Adjunto, cid string) error {
	tipo, _, err := mime.ParseMediaType(a.Tipo)
	if err != nil {
		tipo = "application/octet-stream"
	}
	// El nombre viene del disco de alguien —o de un correo ajeno que se reenvía—: sin
	// directorios, en una línea, y lo codifica la biblioteca (RFC 2231); nunca se pega
	// tal cual a la cabecera.
	nombre := cmp.Or(nombreDe(unaLinea(a.Nombre)), "adjunto")
	disposicion := "attachment"
	h := textproto.MIMEHeader{"Content-Type": {tipo}, "Content-Transfer-Encoding": {"base64"}}
	if cid != "" {
		// El `cid:` del cuerpo es ESTE `Content-ID`, entre ángulos (RFC 2392).
		disposicion = "inline"
		h.Set("Content-ID", "<"+cid+">")
	}
	h.Set("Content-Disposition", cmp.Or(mime.FormatMediaType(disposicion, map[string]string{"filename": nombre}), disposicion))
	p, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	codificado := base64.StdEncoding.EncodeToString(a.Datos)
	for len(codificado) > 76 {
		_, _ = io.WriteString(p, codificado[:76]+"\r\n")
		codificado = codificado[76:]
	}
	_, _ = io.WriteString(p, codificado) // a un bytes.Buffer: no falla
	return nil
}

const (
	tipoTexto = "text/plain; charset=utf-8"
	tipoHTML  = "text/html; charset=utf-8"
)

// cuerpo escribe el cuerpo —sus cabeceras con `cabecera` y, tras la línea en blanco, el
// contenido—: texto plano, o `multipart/alternative` con el texto y el HTML.
func cuerpo(w *bytes.Buffer, s Salida, cabecera func(nombre, valor string)) error {
	if s.HTML == "" {
		cabecera("Content-Type", tipoTexto)
		cabecera("Content-Transfer-Encoding", "quoted-printable")
		w.WriteString("\r\n")
		return texto(w, s.Texto)
	}
	alt := multipart.NewWriter(w)
	cabecera("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alt.Boundary()}))
	w.WriteString("\r\n")
	for _, parte := range []struct{ tipo, contenido string }{{tipoTexto, s.Texto}, {tipoHTML, s.HTML}} {
		p, err := alt.CreatePart(textproto.MIMEHeader{"Content-Type": {parte.tipo}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if err != nil {
			return err
		}
		if err := texto(p, parte.contenido); err != nil {
			return err
		}
	}
	return alt.Close()
}

// texto escribe el cuerpo de texto plano en quoted-printable, con CRLF.
func texto(w io.Writer, cuerpo string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(cuerpo, "\r\n", "\n"), "\n", "\r\n"))); err != nil {
		return err
	}
	return qp.Close()
}
