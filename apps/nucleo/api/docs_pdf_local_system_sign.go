// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"sync"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// LA FIRMA CON EL CERTIFICADO DEL SISTEMA, EN EL DISPOSITIVO (ADR 0051 y ADR 0086,
// cuarta entrega). Con el PDF en el Drive privado, los dos pasos que en el Drive del
// servidor hacía el servicio de Docs (`/pdf/sign/prepare` y `/pdf/sign/complete`)
// se hacen aquí con las MISMAS funciones del motor (`PrepararFirmaPDF` y
// `CompletarFirmaPDF`): el documento con su hueco, lo que firma el Llavero, el
// almacén de Windows o la tarjeta, y el CMS incrustado. Ni el PDF, ni el resumen, ni
// el certificado salen del equipo.
//
// Lo que liga la vuelta a la ida vive en la memoria de la app, no en una base: la
// preparación es de ESTE proceso, vale diez minutos, se gasta al completar y el
// preparado tiene que volver byte a byte (su SHA-256). Sin autoridad de sellado al
// alcance (el servicio no ve nada de esto), la firma sale sin sello y la pantalla
// lo dice antes (`Sello` vacío), como la firma local con un `.p12`.

const (
	localPreparedLife      = 10 * time.Minute
	localPreparedMax       = 5
	localChainMax          = 6
	localChainCertMaxBytes = 16 << 10
)

// localPrepared es una preparación abierta: lo que el segundo paso comprueba.
type localPrepared struct {
	expires    time.Time
	digest     [32]byte
	start, end int
	attributes []byte
	chain      []*x509.Certificate
}

// localSignatures guarda las preparaciones abiertas de una fachada.
type localSignatures struct {
	mu    sync.Mutex
	items map[string]localPrepared
}

func newLocalSignatures() *localSignatures {
	return &localSignatures{items: map[string]localPrepared{}}
}

func (s *localSignatures) save(p localPrepared) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, item := range s.items {
		if now.After(item.expires) {
			delete(s.items, id)
		}
	}
	if len(s.items) >= localPreparedMax {
		return "", localPDFError(http.StatusTooManyRequests, "pdf_signature_too_many",
			"There are too many signatures being prepared. Finish or wait for one of them.")
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	s.items[id] = p
	return id, nil
}

// take devuelve la preparación sin gastarla: un fallo deja reintentar.
func (s *localSignatures) take(id string) (localPrepared, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.items[id]
	if !ok || time.Now().After(p.expires) {
		delete(s.items, id)
		return localPrepared{}, false
	}
	return p, true
}

func (s *localSignatures) spend(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

var (
	errLocalSignatureExpired = localPDFError(http.StatusConflict, "pdf_signature_expired",
		"This signature is no longer available: prepare it again.")
	errLocalPreparedMismatch = localPDFError(http.StatusConflict, "pdf_prepared_mismatch",
		"The document is not the one that was prepared for signing.")
	errLocalSignatureMismatch = localPDFError(http.StatusBadRequest, "pdf_signature_mismatch",
		"The signature does not match the prepared digest and the chosen certificate.")
)

// localPreparedJSON es la respuesta del primer paso, la misma que la del servicio.
//
//nolint:tagliatelle // las etiquetas son las del motor del servicio (`firmaPreparadaJSON`)
type localPreparedJSON struct {
	ID         string    `json:"id"`
	Preparado  []byte    `json:"preparado"`
	Resumen    []byte    `json:"resumen"`
	DigestInfo []byte    `json:"digest_info"`
	PorFirmar  []byte    `json:"por_firmar"`
	Hueco      [2]int    `json:"hueco"`
	Anexado    bool      `json:"anexado"`
	Algoritmo  string    `json:"algoritmo"`
	Caduca     time.Time `json:"caduca"`
	Fecha      time.Time `json:"fecha"`
	Firmante   struct {
		Nombre      string    `json:"nombre"`
		Emisor      string    `json:"emisor"`
		ValidoHasta time.Time `json:"valido_hasta"`
	} `json:"firmante"`
	Nivel   string `json:"nivel"`
	Paginas int    `json:"paginas"`
	Sello   string `json:"sello"`
}

// localChain lee el certificado elegido y sus intermedios (DER o PEM, en su orden).
func localChain(parts []PDFParte) ([]*x509.Certificate, error) {
	var chain []*x509.Certificate
	for _, part := range parts {
		if part.Campo != "certificado" || len(part.Datos) > localChainCertMaxBytes {
			return nil, localPDFError(http.StatusBadRequest, "bad_request", `Send the chosen certificate in the "certificado" field.`)
		}
		for _, der := range certificateBlocks(part.Datos) {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				failure := localpdf.DescribeOperationError(localpdf.ErrCertificadoNoUsable)
				return nil, localPDFError(failure.Status, failure.Code, failure.Message)
			}
			chain = append(chain, c)
		}
	}
	if len(chain) == 0 || len(chain) > localChainMax {
		return nil, localPDFError(http.StatusBadRequest, "bad_request", `Send the chosen certificate in the "certificado" field.`)
	}
	return chain, nil
}

// certificateBlocks son los certificados de una parte: ella misma si es DER, o sus
// bloques CERTIFICATE si es PEM.
func certificateBlocks(p []byte) [][]byte {
	if !bytes.Contains(p, []byte("-----BEGIN")) {
		return [][]byte{p}
	}
	var out [][]byte
	for rest := p; ; {
		b, next := pem.Decode(rest)
		if b == nil {
			return out
		}
		rest = next
		if b.Type == "CERTIFICATE" {
			out = append(out, b.Bytes)
		}
	}
}

// localSignPrepare es `/pdf/sign/prepare` en el dispositivo.
func (p *PDF) localSignPrepare(ctx context.Context, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
	if p.signatures == nil {
		return PDFRespuesta{}, ErrPrivatePDFOperation
	}
	if len(docs) != 1 {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Send a single PDF.")
	}
	chain, err := localChain(parts)
	if err != nil {
		return PDFRespuesta{}, err
	}
	// Las opciones que elige la persona; el nombre y la fecha los pone el motor desde
	// el certificado, y la identidad solo llega en las partes.
	var options struct {
		Motivo, Lugar, Contacto string
		Pagina                  int
		Rect                    [4]float64
		Bloquear                bool
	}
	if err := localpdf.DecodeOperationJSON(fields["firma"], "firma", &options); err != nil {
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	var prepared *localpdf.FirmaPreparada
	var pages int
	_, err = p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		pages = doc.NumPaginas()
		var err error
		prepared, err = localpdf.PrepararFirmaPDF(doc, localpdf.OpcionesFirma{
			Motivo: options.Motivo, Lugar: options.Lugar, Contacto: options.Contacto,
			Pagina: options.Pagina, Rect: options.Rect, Bloquear: options.Bloquear,
		}, chain)
		// La respuesta es JSON: el preparado no se cuenta como PDF de salida.
		return PDFRespuesta{Cabecera: http.Header{}}, err
	}, docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	expires := time.Now().Add(localPreparedLife)
	id, err := p.signatures.save(localPrepared{
		expires: expires, digest: sha256.Sum256(prepared.PDF),
		start: prepared.Inicio, end: prepared.Fin, attributes: prepared.PorFirmar, chain: chain,
	})
	if err != nil {
		return PDFRespuesta{}, err
	}
	out := localPreparedJSON{
		ID: id, Preparado: prepared.PDF, Resumen: prepared.Resumen, DigestInfo: prepared.DigestInfo,
		PorFirmar: prepared.PorFirmar, Hueco: [2]int{prepared.Inicio, prepared.Fin}, Anexado: prepared.Anexado,
		Algoritmo: "rsa", Caduca: expires, Fecha: prepared.Fecha, Paginas: pages, Nivel: "avanzada",
	}
	if _, ok := chain[0].PublicKey.(*ecdsa.PublicKey); ok {
		out.Algoritmo = "ecdsa"
	}
	if qualified, device := localpdf.NivelDelCertificado(chain[0]); qualified && device && len(chain) > 1 {
		out.Nivel = "cualificada"
	}
	out.Firmante.Nombre = prepared.Nombre
	out.Firmante.Emisor = chain[0].Issuer.CommonName
	out.Firmante.ValidoHasta = chain[0].NotAfter
	raw, err := json.Marshal(out)
	if err != nil {
		return PDFRespuesta{}, err
	}
	return localPDFResponse("application/json", raw), nil
}

// localSignComplete es `/pdf/sign/complete` en el dispositivo: el preparado vuelve
// tal cual, la firma tiene que ser de ESE resumen con ESA clave, y sale sin sello.
func (p *PDF) localSignComplete(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	if p.signatures == nil {
		return PDFRespuesta{}, ErrPrivatePDFOperation
	}
	if err := p.authorizeLocalPDF(ctx); err != nil {
		return PDFRespuesta{}, err
	}
	if len(docs) != 1 || len(docs[0].Datos) > PDFMaxBytes {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", `Send the prepared PDF in the "file" field.`)
	}
	signature, err := base64.StdEncoding.DecodeString(fields["firma"])
	if err != nil || len(signature) == 0 || len(signature) > 2048 {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", `The "firma" field must be the signature in base64.`)
	}
	id := fields["id"]
	prepared, ok := p.signatures.take(id)
	if !ok {
		return PDFRespuesta{}, errLocalSignatureExpired
	}
	if digest := sha256.Sum256(docs[0].Datos); digest != prepared.digest {
		return PDFRespuesta{}, errLocalPreparedMismatch
	}
	signature, err = localpdf.FirmaEnDER(prepared.chain[0].PublicKey, signature, fields["formato"])
	if err != nil {
		return PDFRespuesta{}, errLocalSignatureMismatch
	}
	signed, _, err := localpdf.CompletarFirmaPDF(localpdf.FirmaPreparada{
		PDF: docs[0].Datos, Inicio: prepared.start, Fin: prepared.end, PorFirmar: prepared.attributes,
	}, prepared.chain, signature, nil)
	switch {
	case errors.Is(err, localpdf.ErrFirmaNoCorresponde):
		return PDFRespuesta{}, errLocalSignatureMismatch
	case errors.Is(err, localpdf.ErrPreparadoAlterado):
		return PDFRespuesta{}, errLocalPreparedMismatch
	case err != nil:
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	p.signatures.spend(id)
	if err := p.authorizeLocalPDF(ctx); err != nil {
		clear(signed)
		return PDFRespuesta{}, err
	}
	response := localPDFResponse("application/pdf", signed)
	response.Cabecera.Set("X-Sytue-Sello", "no")
	return response, nil
}
