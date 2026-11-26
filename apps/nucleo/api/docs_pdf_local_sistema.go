// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// # Lo que pone Sytue Office en el motor del dispositivo
//
// El Drive privado ya hace en local la firma con el certificado del sistema, la
// verificación y el resto (docs_pdf_local_system_sign.go, docs_pdf_local_inspect.go). La
// edición local añade tres cosas que la persona configura en Ajustes (P-05, P-06 y P-07):
// la autoridad de sellado de tiempo, las anclas de confianza con que se juzga quién firma
// y el reconocedor de texto del dispositivo. Sin ellas, todo sigue como en el Drive
// privado: firmas sin sello, verificación sin anclas y sin «reconocer texto».

// PDFLocal es la configuración del motor del dispositivo que pone la app. Cada función se
// consulta en el momento de usarla, así que un cambio en los ajustes vale para la firma
// siguiente sin rehacer la fachada.
type PDFLocal struct {
	// Sello es la autoridad de sellado que eligió la persona. Nil, o con URL vacía: las
	// firmas salen sin sello.
	Sello func() localpdf.OpcionesSello
	// Anclas son los certificados en que confía la persona para juzgar la identidad de
	// quien firma y de la autoridad que sella (P-06). Nil o vacío: no se juzga, y se dice.
	Anclas func() *x509.CertPool
	// Reconocedor es el OCR del dispositivo (P-07). Nil: «reconocer texto» no se ofrece.
	Reconocedor PDFReconocedor
}

// pdfLocal guarda la configuración de una fachada de WithPrivateDrive.
type pdfLocal struct {
	mu   sync.Mutex
	conf PDFLocal
}

// ConLocal configura el motor del dispositivo. Solo tiene efecto sobre una fachada de
// WithPrivateDrive; devuelve la misma para encadenar.
func (p *PDF) ConLocal(c PDFLocal) *PDF {
	if p != nil && p.local != nil {
		p.local.mu.Lock()
		p.local.conf = c
		p.local.mu.Unlock()
	}
	return p
}

func (p *PDF) confLocal() PDFLocal {
	if p == nil || p.local == nil {
		return PDFLocal{}
	}
	p.local.mu.Lock()
	defer p.local.mu.Unlock()
	return p.local.conf
}

// selloLocal es la autoridad configurada, o una vacía.
func (p *PDF) selloLocal() localpdf.OpcionesSello {
	if f := p.confLocal().Sello; f != nil {
		o := f()
		o.URL = strings.TrimSpace(o.URL)
		return o
	}
	return localpdf.OpcionesSello{}
}

// anclasLocales son las de la persona, o nil si no hay ninguna.
func (p *PDF) anclasLocales() *x509.CertPool {
	if f := p.confLocal().Anclas; f != nil {
		if pool := f(); pool != nil && !pool.Equal(x509.NewCertPool()) {
			return pool
		}
	}
	return nil
}

var errLocalSelloCaido = localPDFError(http.StatusBadGateway, "pdf_timestamp_unavailable",
	"The time-stamping authority did not answer. Try again, or sign without a time stamp.")

// hostDeSello es el nombre de la autoridad que ve la persona: su host.
func hostDeSello(direccion string) string {
	if direccion == "" {
		return ""
	}
	u, err := url.Parse(direccion)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func localJSON(v any) (PDFRespuesta, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return PDFRespuesta{}, err
	}
	return localPDFResponse("application/json", raw), nil
}

func localOperationFailure(err error) error {
	failure := localpdf.DescribeOperationError(err)
	return localPDFError(failure.Status, failure.Code, failure.Message)
}
