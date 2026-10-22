// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// La firma con el certificado del SISTEMA, en dos pasos (ADR 0051): el servicio
// prepara y completa, y entre medias la app nativa le pide al sistema que firme el
// resumen con una clave que no sale nunca del equipo ni de la tarjeta.
//
// Lo que liga la vuelta a la ida vive en la base (migración 0007), no en el
// cliente: la preparación es de UNA persona de UNA organización, vale diez minutos
// y se gasta al completar. La app devuelve el PDF preparado —un documento de 25 MiB
// no se aparca en la base—, y tiene que ser byte a byte el que se le entregó.

const (
	// vidaFirmaPreparada es lo que tiene la persona para elegir, leer lo que firma y
	// poner el PIN. Diez minutos sobran para eso y dejan poco a un resumen robado.
	vidaFirmaPreparada = 10 * time.Minute
	// maxFirmasAbiertas por persona a la vez.
	maxFirmasAbiertas = 5
	// reservaCompletar cubre pedir el sello (20 s de plazo) y montar el CMS.
	reservaCompletar = time.Minute
	// maxCertificadosCadena y maxBytesCertificado acotan la cadena que manda la
	// app: el firmante y sus intermedios, no un almacén entero.
	maxCertificadosCadena = 6
	maxBytesCertificado   = 16 << 10
)

// almacenFirmas es lo que estos dos endpoints necesitan de la base. Lo cumple
// *store.Store; los tests ponen uno en memoria.
type almacenFirmas interface {
	GuardarFirmaPreparada(ctx context.Context, f store.FirmaPreparada, maximo int) (string, error)
	TomarFirmaPreparada(ctx context.Context, id, sujeto string, plazo time.Duration) (store.FirmaPreparada, error)
	SoltarFirmaPreparada(ctx context.Context, id string, reserva time.Time) error
	GastarFirmaPreparada(ctx context.Context, id string) error
}

// firmaPreparadaJSON es la respuesta del primer paso: lo que la app firma y todo lo
// que la persona tiene que ver ANTES de firmar.
type firmaPreparadaJSON struct {
	ID string `json:"id"`
	// Preparado es el PDF con el hueco (base64): se devuelve tal cual al completar.
	Preparado []byte `json:"preparado"`
	// Resumen es lo que firman el Llavero, CNG y ECDSA por PKCS#11 o Android.
	Resumen []byte `json:"resumen"`
	// DigestInfo es lo que firman CKM_RSA_PKCS y NONEwithRSA, ya envuelto.
	DigestInfo []byte `json:"digest_info"`
	// PorFirmar, Hueco y Anexado son para que la app COMPRUEBE qué firma en vez de
	// fiarse de `resumen`. PorFirmar es el DER del SET OF de atributos firmados, los
	// bytes exactos cuyo SHA-256 es `resumen`: lleva contentType = id-data, messageDigest
	// = SHA-256 de `preparado` sin el hueco y signing-certificate-v2 con el SHA-256 del
	// certificado elegido. Hueco es [inicio, fin): dónde está el hueco de /Contents en
	// `preparado`, con sus `<` `>`. Anexado dice si `preparado` empieza por los bytes del
	// documento tal como llegaron (true) o si hubo que reescribirlo (false).
	// Lo fija TestPrepararDaConQueComprobarLoQueSeFirma.
	PorFirmar []byte `json:"por_firmar"`
	Hueco     [2]int `json:"hueco"`
	Anexado   bool   `json:"anexado"`
	// Algoritmo es "rsa" o "ecdsa": qué función de firma del sistema toca.
	Algoritmo string    `json:"algoritmo"`
	Caduca    time.Time `json:"caduca"`
	Fecha     time.Time `json:"fecha"`
	Firmante  struct {
		Nombre      string    `json:"nombre"`
		Emisor      string    `json:"emisor"`
		ValidoHasta time.Time `json:"valido_hasta"`
	} `json:"firmante"`
	// Nivel es "cualificada" cuando el certificado se declara cualificado y en un
	// dispositivo cualificado (la firma PUEDE serlo), y "avanzada" en todo lo demás.
	Nivel   string `json:"nivel"`
	Paginas int    `json:"paginas"`
	// Sello es la autoridad que sellará (su host), o "" si no hay ninguna.
	Sello string `json:"sello"`
}

// pdfFirmaPreparar atiende POST /api/v1/pdf/sign/prepare.
//
//	file         el PDF
//	clave        su contraseña, si está cifrado
//	firma        JSON con motivo, lugar, página, recuadro y bloquear
//	certificado  el certificado elegido (DER o PEM), y detrás sus intermedios;
//	             se puede repetir el campo
func (h *Handler) pdfFirmaPreparar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)
	quien, _ := subjectFrom(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var opts domain.OpcionesFirma
	if err := leerJSONDelFormulario(r, "firma", &opts); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cadena, err := cadenaDelFormulario(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := domain.PrepararFirmaPDF(doc, opts, cadena)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}

	huella := sha256.Sum256(p.PDF)
	var certs []byte
	for _, c := range cadena {
		certs = append(certs, c.Raw...)
	}
	caduca := time.Now().Add(vidaFirmaPreparada)
	id, err := h.firmas.GuardarFirmaPreparada(r.Context(), store.FirmaPreparada{
		Sujeto: quien.Sub, Caduca: caduca, Huella: huella[:],
		HuecoInicio: p.Inicio, HuecoFin: p.Fin, Atributos: p.PorFirmar, Certificados: certs,
	}, maxFirmasAbiertas)
	if errors.Is(err, store.ErrDemasiadasFirmas) {
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "pdf_signature_too_many",
			"There are too many signatures being prepared. Finish or wait for one of them."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out := firmaPreparadaJSON{
		ID: id, Preparado: p.PDF, Resumen: p.Resumen, DigestInfo: p.DigestInfo,
		PorFirmar: p.PorFirmar, Hueco: [2]int{p.Inicio, p.Fin}, Anexado: p.Anexado,
		Algoritmo: "rsa", Caduca: caduca, Fecha: p.Fecha, Paginas: doc.NumPaginas(),
		Nivel: "avanzada", Sello: hostDe(h.sello.URL),
	}
	if _, ok := cadena[0].PublicKey.(*ecdsa.PublicKey); ok {
		out.Algoritmo = "ecdsa"
	}
	// Lo que el certificado DECLARA solo se enseña si viene con quien lo emite (la
	// cadena ya se comprobó en orden al preparar): uno autofirmado puede declarar
	// cualquier cosa.
	if cual, disp := domain.NivelDelCertificado(cadena[0]); cual && disp && len(cadena) > 1 {
		out.Nivel = "cualificada"
	}
	out.Firmante.Nombre = p.Nombre
	out.Firmante.Emisor = cadena[0].Issuer.CommonName
	out.Firmante.ValidoHasta = cadena[0].NotAfter
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, out)
}

// pdfFirmaCompletar atiende POST /api/v1/pdf/sign/complete.
//
//	id         el de la preparación
//	file       el PDF preparado, tal cual se entregó
//	firma      la firma que hizo el sistema, en base64
//	formato    "p1363" si es ECDSA en r‖s (CNG, PKCS#11); si no, "der"
//	sin_sello  "1" para firmar sin sello cuando la autoridad no responde
func (h *Handler) pdfFirmaCompletar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)
	quien, _ := subjectFrom(r)

	id := r.FormValue("id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, errFirmaNoDisponible)
		return
	}
	firma, err := base64.StdEncoding.DecodeString(r.FormValue("firma"))
	if err != nil || len(firma) == 0 || len(firma) > 2048 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`The "firma" field must be the signature in base64.`))
		return
	}
	preparado, err := ficheroEntero(r, "file")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	f, err := h.firmas.TomarFirmaPreparada(r.Context(), id, quien.Sub, reservaCompletar)
	if err != nil {
		if !errors.Is(err, store.ErrFirmaNoDisponible) {
			httpx.Fail(w, r, err)
			return
		}
		httpx.Fail(w, r, errFirmaNoDisponible)
		return
	}
	// Mientras no haya firma, la preparación se suelta al salir: un fallo del sello
	// o una firma de otra clave dejan reintentar sin volver a empezar. El contexto
	// es otro para que soltar no dependa de que el cliente siga esperando.
	gastada, sinCancelar := false, context.WithoutCancel(r.Context())
	defer func() {
		if !gastada {
			_ = h.firmas.SoltarFirmaPreparada(sinCancelar, id, f.Reserva)
		}
	}()

	huella := sha256.Sum256(preparado)
	if !bytes.Equal(huella[:], f.Huella) {
		httpx.Fail(w, r, errPreparadoAlterado)
		return
	}
	cadena, err := x509.ParseCertificates(f.Certificados)
	if err != nil || len(cadena) == 0 {
		httpx.Fail(w, r, errPreparadoAlterado)
		return
	}
	firma, err = domain.FirmaEnDER(cadena[0].PublicKey, firma, r.FormValue("formato"))
	if err != nil {
		httpx.Fail(w, r, errFirmaNoCorresponde)
		return
	}
	var sello func([]byte) ([]byte, error)
	if r.FormValue("sin_sello") != "1" {
		sello = h.selloDeTiempo(r.Context())
	}

	firmado, sellado, err := domain.CompletarFirmaPDF(domain.FirmaPreparada{
		PDF: preparado, Inicio: f.HuecoInicio, Fin: f.HuecoFin, PorFirmar: f.Atributos,
	}, cadena, firma, sello)
	switch {
	case errors.Is(err, domain.ErrSelloNoDisponible):
		log.From(r.Context()).Warn("firma del sistema: la autoridad de sellado no dio el sello", "error", err)
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "pdf_timestamp_unavailable",
			"The time-stamping authority did not answer. Try again, or sign without a time stamp."))
		return
	case errors.Is(err, domain.ErrFirmaNoCorresponde):
		httpx.Fail(w, r, errFirmaNoCorresponde)
		return
	case errors.Is(err, domain.ErrPreparadoAlterado):
		httpx.Fail(w, r, errPreparadoAlterado)
		return
	case err != nil:
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}

	gastada = true
	if err := h.firmas.GastarFirmaPreparada(sinCancelar, id); err != nil {
		// La firma ya está hecha y es buena: se entrega. La fila caduca sola.
		log.From(r.Context()).Warn("firma del sistema: no se pudo gastar la preparación", "error", err)
	}
	// Ni el nombre ni el DNI de quien firma van al registro: la huella del
	// certificado basta para encontrarlo si hace falta.
	huellaCert := sha256.Sum256(cadena[0].Raw)
	log.From(r.Context()).Info("firma del sistema completada",
		"certificado", hex.EncodeToString(huellaCert[:8]), "sellada", sellado)
	w.Header().Set("X-Sytue-Sello", map[bool]string{true: "si", false: "no"}[sellado])
	responderPDF(w, nil, firmado, "firmado.pdf") // anexada: no toca la marca PDF/A
}

var (
	errFirmaNoDisponible = httpx.NewError(http.StatusConflict, "pdf_signature_expired",
		"This signature is no longer available: prepare it again.")
	errPreparadoAlterado = httpx.NewError(http.StatusConflict, "pdf_prepared_mismatch",
		"The document is not the one that was prepared for signing.")
	errFirmaNoCorresponde = httpx.NewError(http.StatusBadRequest, "pdf_signature_mismatch",
		"The signature does not match the prepared digest and the chosen certificate.")
)

// cadenaDelFormulario lee el certificado elegido y sus intermedios, cada uno en DER
// o en PEM, en el orden en que llegan: el primero es el que firma.
func cadenaDelFormulario(r *http.Request) ([]*x509.Certificate, error) {
	var partes [][]byte
	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["certificado"] {
			fd, err := fh.Open()
			if err != nil {
				return nil, httpx.ErrBadRequest.WithMessage(`The "certificado" file could not be read.`)
			}
			datos, err := io.ReadAll(io.LimitReader(fd, maxBytesCertificado+1))
			_ = fd.Close()
			if err != nil || len(datos) > maxBytesCertificado {
				return nil, httpx.ErrBadRequest.WithMessage(`A "certificado" file is too large.`)
			}
			partes = append(partes, datos)
		}
	}
	var cadena []*x509.Certificate
	for _, p := range partes {
		for _, der := range dersDe(p) {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, errorDeDominio(domain.ErrCertificadoNoUsable)
			}
			cadena = append(cadena, c)
		}
	}
	if len(cadena) == 0 || len(cadena) > maxCertificadosCadena {
		return nil, httpx.ErrBadRequest.WithMessage(`Send the chosen certificate in the "certificado" field.`)
	}
	return cadena, nil
}

// dersDe son los certificados de una parte: ella misma si es DER, o sus bloques
// CERTIFICATE si es PEM.
func dersDe(p []byte) [][]byte {
	if !bytes.Contains(p, []byte("-----BEGIN")) {
		return [][]byte{p}
	}
	var ders [][]byte
	for resto := p; ; {
		b, siguiente := pem.Decode(resto)
		if b == nil {
			return ders
		}
		resto = siguiente
		if b.Type == "CERTIFICATE" {
			ders = append(ders, b.Bytes)
		}
	}
}

// ficheroEntero lee un fichero del formulario hasta el tope del motor.
func ficheroEntero(r *http.Request, campo string) ([]byte, error) {
	if r.MultipartForm == nil || len(r.MultipartForm.File[campo]) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage(`Send the prepared PDF in the "` + campo + `" field.`)
	}
	f, err := r.MultipartForm.File[campo][0].Open()
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage(`The "` + campo + `" file could not be read.`)
	}
	defer func() { _ = f.Close() }()
	datos, err := io.ReadAll(io.LimitReader(f, domain.MaxPDFBytes+1))
	if err != nil || len(datos) > domain.MaxPDFBytes {
		return nil, errPayloadTooLarge
	}
	return datos, nil
}

// hostDe es el nombre de la autoridad que ve la persona: su host.
func hostDe(direccion string) string {
	u, err := url.Parse(direccion)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
