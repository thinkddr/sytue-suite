// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

// Firma S/MIME: CMS SignedData en modo DETACHED dentro de un multipart/signed
// (RFC 8551 §3.4.3), que es lo que entienden Outlook, Apple Mail, Thunderbird y
// los clientes de móvil.
//
// El ASN.1 del CMS lo arma platform/pkg/cms, el mismo que firma los PDF. Este
// paquete tenía su propia copia y el ADR que creó el común ya la dejó señalada
// para migrar; lo que ha forzado la migración es que aquella copia solo sabía
// firmar con ECDSA y solo metía en el sobre el certificado del firmante. Las dos
// cosas impiden justo lo que hace falta para que un correo se valide FUERA: los
// certificados de las CAs públicas suelen ser RSA y siempre traen una intermedia
// que hay que incrustar.

// SignDetached devuelve el CMS SignedData (DER) que firma `content` sin
// incluirlo: el contenido viaja en claro en la otra parte del multipart, que es
// lo que permite leer el correo aunque el cliente no sepa de S/MIME.
func SignDetached(id *Identity, content []byte) ([]byte, error) {
	if id == nil || id.Key == nil || id.Certificate == nil {
		return nil, fmt.Errorf("smime: no hay identidad con la que firmar")
	}
	// La cadena va detrás del certificado del buzón, en ese orden: es lo que
	// permite a quien recibe llegar hasta una raíz que ya tiene.
	certs := append([]*x509.Certificate{id.Certificate}, id.Chain...)
	der, err := cms.Sign(id.Key, certs, content)
	if err != nil {
		return nil, fmt.Errorf("smime: %w", err)
	}
	return der, nil
}

// --- multipart/signed --------------------------------------------------------

// Boundary del multipart. Fijo en formato pero con parte aleatoria: dos partes
// distintas del mismo mensaje no pueden compartirlo.
func newBoundary() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("smime: generar separador MIME: %w", err)
	}
	return "sytue-smime-" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Sign envuelve un mensaje RFC 5322 completo en un multipart/signed.
//
// Las cabeceras del mensaje original se conservan TAL CUAL en el exterior (es
// lo que ve el servidor de destino y lo que firma DKIM después); lo que se firma
// es el cuerpo con sus cabeceras de contenido, que es lo que exige RFC 8551.
func Sign(id *Identity, raw []byte) ([]byte, error) {
	headers, body, ok := splitMessage(raw)
	if !ok {
		return nil, fmt.Errorf("smime: el mensaje no separa cabeceras y cuerpo")
	}
	outer, inner := splitContentHeaders(headers)

	// La parte firmada es: cabeceras de contenido + cuerpo, con CRLF canónico.
	signedPart := canonicalize(append(append([]byte(inner), "\r\n"...), body...))
	sig, err := SignDetached(id, signedPart)
	if err != nil {
		return nil, err
	}
	boundary, err := newBoundary()
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(outer)
	out.WriteString("MIME-Version: 1.0\r\n")
	out.WriteString("Content-Type: multipart/signed; protocol=\"application/pkcs7-signature\"; ")
	out.WriteString("micalg=sha-256; boundary=\"" + boundary + "\"\r\n")
	out.WriteString("\r\n")
	out.WriteString("Este mensaje va firmado digitalmente (S/MIME).\r\n")
	out.WriteString("\r\n--" + boundary + "\r\n")
	out.Write(signedPart)
	out.WriteString("\r\n--" + boundary + "\r\n")
	out.WriteString("Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n")
	out.WriteString("Content-Transfer-Encoding: base64\r\n")
	out.WriteString("Content-Disposition: attachment; filename=\"smime.p7s\"\r\n")
	out.WriteString("\r\n")
	out.WriteString(wrapBase64(sig))
	out.WriteString("\r\n--" + boundary + "--\r\n")
	return out.Bytes(), nil
}

// splitMessage separa cabeceras y cuerpo por la primera línea en blanco.
func splitMessage(raw []byte) (headers, body string, ok bool) {
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := bytes.Index(raw, []byte(sep)); i >= 0 {
			return string(raw[:i+len(sep)]), string(raw[i+len(sep):]), true
		}
	}
	return "", "", false
}

// splitContentHeaders reparte las cabeceras: las de CONTENIDO describen la parte
// que se firma y bajan dentro del multipart; las demás (From, To, Subject…) se
// quedan fuera, porque son las que el correo necesita para entregarse.
func splitContentHeaders(headers string) (outer, inner string) {
	var out, in strings.Builder
	for _, line := range splitHeaderLines(headers) {
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(line, ":", 2)[0]))
		switch {
		case strings.HasPrefix(name, "content-"):
			in.WriteString(line)
		case name == "mime-version":
			// La versión MIME la reescribe el envoltorio exterior.
		default:
			out.WriteString(line)
		}
	}
	return out.String(), in.String()
}

// splitHeaderLines devuelve cada cabecera con sus líneas de continuación juntas.
func splitHeaderLines(headers string) []string {
	raw := strings.SplitAfter(strings.ReplaceAll(headers, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if line == "" || line == "\n" {
			continue
		}
		normalized := strings.TrimSuffix(line, "\n") + "\r\n"
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(out) > 0 {
			out[len(out)-1] += normalized
			continue
		}
		out = append(out, normalized)
	}
	return out
}

// canonicalize pasa el contenido a CRLF, que es como se firma y como se
// verifica: un salto de línea distinto entre firmar y enviar invalida la firma.
func canonicalize(b []byte) []byte {
	normalized := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(normalized, []byte("\n"), []byte("\r\n"))
}

// wrapBase64 corta el base64 a 64 columnas, como exige MIME.
func wrapBase64(der []byte) string {
	encoded := base64.StdEncoding.EncodeToString(der)
	var out strings.Builder
	for len(encoded) > 64 {
		out.WriteString(encoded[:64] + "\r\n")
		encoded = encoded[64:]
	}
	out.WriteString(encoded)
	return out.String()
}
