// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// # La firma con el certificado del SISTEMA, en dos pasos (ADR 0051)
//
// [PDF.PrepararFirma] manda el documento y el certificado elegido y recibe lo que hay
// que firmar; la app se lo da a firmar al sistema (Llavero, almacén de Windows, tarjeta
// por PKCS#11, KeyChain de Android) y [PDF.CompletarFirma] devuelve el PDF firmado y,
// si la autoridad respondió, sellado. La clave privada no pasa nunca por aquí.

// PDFFirmaOpciones es lo que la persona elige al firmar. El nombre y la fecha NO van:
// los pone el servicio desde el certificado. Las etiquetas son las de
// `domain.OpcionesFirma`, que el motor lee sin etiquetas: PascalCase a propósito.
//
//nolint:tagliatelle // el motor decodifica por nombre de campo Go, no en snake_case
type PDFFirmaOpciones struct {
	Motivo   string     `json:"Motivo,omitempty"`
	Lugar    string     `json:"Lugar,omitempty"`
	Contacto string     `json:"Contacto,omitempty"`
	Pagina   int        `json:"Pagina,omitempty"`
	Rect     [4]float64 `json:"Rect"`
	Bloquear bool       `json:"Bloquear,omitempty"`
}

// PDFFirmaPreparada es la respuesta del primer paso: lo que se firma y lo que la
// persona tiene que ver ANTES de poner el PIN.
type PDFFirmaPreparada struct {
	ID string `json:"id"`
	// Preparado es el PDF con el hueco: vuelve tal cual en el segundo paso.
	Preparado []byte `json:"preparado"`
	// Resumen es lo que firman el Llavero, CNG, ECDSA por PKCS#11 y Android.
	Resumen []byte `json:"resumen"`
	// DigestInfo es lo que firman CKM_RSA_PKCS y NONEwithRSA.
	DigestInfo []byte `json:"digest_info"`
	// Algoritmo es "rsa" o "ecdsa".
	Algoritmo string    `json:"algoritmo"`
	Caduca    time.Time `json:"caduca"`
	Fecha     time.Time `json:"fecha"`
	Firmante  struct {
		Nombre      string    `json:"nombre"`
		Emisor      string    `json:"emisor"`
		ValidoHasta time.Time `json:"valido_hasta"`
	} `json:"firmante"`
	// Nivel es "cualificada" (PUEDE serlo: lo declara el certificado) o "avanzada".
	Nivel   string `json:"nivel"`
	Paginas int    `json:"paginas"`
	// Sello es el host de la autoridad que sellará, o "" si no hay.
	Sello string `json:"sello"`
}

// Formatos de firma ECDSA que devuelve el sistema: DER (Llavero, Android) o r‖s de
// tamaño fijo (CNG, PKCS#11). Con RSA da igual.
const (
	PDFFirmaDER   = "der"
	PDFFirmaP1363 = "p1363"
)

// PDFSelloCaido dice si el segundo paso falló porque la autoridad de sellado no
// respondió. La preparación sigue viva y la MISMA firma vale para reintentar o para
// seguir sin sello: no hay que volver a pedir el PIN.
func PDFSelloCaido(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code() == "pdf_timestamp_unavailable"
}

// PDFFirmaCaducada dice si la preparación ya no vale (caducó, se gastó o es de otra
// persona): hay que preparar otra vez.
func PDFFirmaCaducada(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code() == "pdf_signature_expired"
}

// PrepararFirma es el primer paso. `cadena` es el certificado elegido en DER y, detrás,
// sus intermedios en orden (cada uno emite al anterior).
func (p *PDF) PrepararFirma(ctx context.Context, doc PDFFichero, clave string, opts PDFFirmaOpciones, cadena [][]byte) (PDFFirmaPreparada, error) {
	if len(cadena) == 0 {
		return PDFFirmaPreparada{}, errors.New("api: falta el certificado con que se firma")
	}
	j, err := json.Marshal(opts)
	if err != nil {
		return PDFFirmaPreparada{}, err
	}
	partes := make([]PDFParte, len(cadena))
	for i, c := range cadena {
		partes[i] = PDFParte{Campo: "certificado", Nombre: "certificado-" + strconv.Itoa(i) + ".der", Datos: c}
	}
	r, err := p.Motor(ctx, "/pdf/sign/prepare", map[string]string{"firma": string(j), "clave": clave}, partes, doc)
	if err != nil {
		return PDFFirmaPreparada{}, err
	}
	var out PDFFirmaPreparada
	if err := json.Unmarshal(r.Datos, &out); err != nil {
		return PDFFirmaPreparada{}, fmt.Errorf("api: la firma preparada no es JSON: %w", err)
	}
	return out, nil
}

// CompletarFirma es el segundo paso: la firma que hizo el sistema, en `formato`
// ([PDFFirmaDER] o [PDFFirmaP1363]). Devuelve el PDF firmado y si lleva sello.
// `sinSello` es la decisión EXPLÍCITA de firmar sin él cuando [PDFSelloCaido].
func (p *PDF) CompletarFirma(ctx context.Context, prep PDFFirmaPreparada, firma []byte, formato string, sinSello bool) ([]byte, bool, error) {
	campos := map[string]string{
		"id": prep.ID, "firma": base64.StdEncoding.EncodeToString(firma), "formato": formato,
	}
	if sinSello {
		campos["sin_sello"] = "1"
	}
	r, err := p.Motor(ctx, "/pdf/sign/complete", campos, nil, PDFFichero{Nombre: "preparado.pdf", Datos: prep.Preparado})
	if err != nil {
		return nil, false, err
	}
	return r.Datos, r.Cabecera.Get("X-Sytue-Sello") == "si", nil
}

// # Pedir firma a otra persona (ADR 0051 §7)
//
// La web lo hace por su BFF (`pages/api/pdf-firmas.ts`), que traduce la RUTA de Drive a
// su identificador; aquí no hay BFF y lo hace [PDF.PorRuta] con el token de la persona.

// PDFPeticionFirma es una petición de firma tal como la devuelve el servicio.
type PDFPeticionFirma struct {
	ID           string `json:"id"`
	Solicitante  string `json:"requester"`
	NombrePide   string `json:"requester_name,omitempty"`
	Destinatario string `json:"recipient"`
	// CorreoDestino es el correo de quien firma, tal como lo resolvió Sytue ID al pedir.
	CorreoDestino string     `json:"recipient_email,omitempty"`
	Fichero       string     `json:"file_id"`
	Ruta          string     `json:"path"`
	Mensaje       string     `json:"message,omitempty"`
	Estado        string     `json:"status"`
	Creada        time.Time  `json:"created_at"`
	Resuelta      *time.Time `json:"resolved_at,omitempty"`
	Firmado       string     `json:"signed_file_id,omitempty"`
	RutaFirmada   string     `json:"signed_path,omitempty"`
	// Externo: quien firma es de OTRA organización (ADR 0057). En las recibidas, Ruta
	// es la copia en mi unidad —vacía hasta [PDF.AbrirFirma]— y Nombre el del fichero;
	// en las pedidas, una firmada sin RutaFirmada se trae con [PDF.TraerFirmado].
	Externo bool   `json:"external,omitempty"`
	Nombre  string `json:"name,omitempty"`
}

// NombrePDF es el nombre del fichero de la petición, también sin ruta.
func (p PDFPeticionFirma) NombrePDF() string {
	if p.Nombre != "" {
		return p.Nombre
	}
	return p.Ruta[strings.LastIndexByte(p.Ruta, '/')+1:]
}

// Estados de una petición de firma.
const (
	PDFFirmaPendiente = "pendiente"
	PDFFirmaFirmada   = "firmada"
	PDFFirmaRechazada = "rechazada"
	PDFFirmaCancelada = "cancelada"
)

// PDFPeticionesFirma son las que me han hecho y las que he hecho, las más recientes antes.
type PDFPeticionesFirma struct {
	Recibidas []PDFPeticionFirma `json:"received"`
	Pedidas   []PDFPeticionFirma `json:"sent"`
	// Bloqueados son las personas cuyas peticiones de fuera no me llegan (ADR 0059).
	Bloqueados []PDFBloqueo `json:"blocked"`
}

// PDFBloqueo es alguien a quien he bloqueado.
type PDFBloqueo struct {
	Sub    string    `json:"sub"`
	Nombre string    `json:"name,omitempty"`
	Creado time.Time `json:"created_at"`
}

// BloquearFirma bloquea a quien me pidió, desde otra organización, la petición `id`:
// sus peticiones dejan de llegarme, y no se le dice (ADR 0059).
func (p *PDF) BloquearFirma(ctx context.Context, id string) error {
	resp, err := p.motor.Do(ctx, Request{Method: http.MethodPost, Path: "/pdf/signature-requests/" + url.PathEscape(id) + "/block"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// DesbloquearFirma quita el bloqueo a `sub`.
func (p *PDF) DesbloquearFirma(ctx context.Context, sub string) error {
	resp, err := p.motor.Do(ctx, Request{Method: http.MethodDelete, Path: "/pdf/signature-blocks/" + url.PathEscape(sub)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// PendienteDe es la que me han pedido firmar sobre `ruta` y sigue pendiente, si la hay.
func (ps PDFPeticionesFirma) PendienteDe(ruta string) (PDFPeticionFirma, bool) {
	for _, p := range ps.Recibidas {
		if p.Estado == PDFFirmaPendiente && p.Ruta != "" && p.Ruta == ruta {
			return p, true
		}
	}
	return PDFPeticionFirma{}, false
}

// PeticionesFirma lista las de quien llama.
func (p *PDF) PeticionesFirma(ctx context.Context) (PDFPeticionesFirma, error) {
	return JSON[PDFPeticionesFirma](ctx, p.motor, Request{Path: "/pdf/signature-requests"})
}

// PedirFirma pide a quien usa `email` (o un @usuario) que firme el PDF de `ruta`. Si es
// de otra organización y no se confirmó, el servicio no crea nada y contesta con
// [PDFFirmaDeFuera]; `confirmar` es el `confirm` que dio: la confirmación de que el PDF
// sale por un enlace de Drive hacia ESA persona (ADR 0057).
func (p *PDF) PedirFirma(ctx context.Context, ruta, email, mensaje, confirmar string) (PDFPeticionFirma, error) {
	f, err := p.PorRuta(ctx, ruta)
	if err != nil {
		return PDFPeticionFirma{}, err
	}
	return PostJSON[PDFPeticionFirma](ctx, p.motor, "/pdf/signature-requests",
		map[string]any{"file_id": f.ID, "path": ruta, "email": email, "message": mensaje, "confirm": confirmar})
}

// PDFFirmaDeFuera dice si [PDF.PedirFirma] se paró porque la persona es de otra
// organización, cómo se llama y qué hay que mandar en `confirmar` si quien pide dice que
// sí: hay que preguntar antes de compartir.
func PDFFirmaDeFuera(err error) (nombre, confirmar string, ok bool) {
	var e *Error
	if !errors.As(err, &e) || e.Code() != "pdf_signature_recipient_external" {
		return "", "", false
	}
	var d struct {
		Name    string `json:"name"`
		Confirm string `json:"confirm"`
	}
	_ = json.Unmarshal(e.cuerpo().Details, &d)
	return d.Name, d.Confirm, true
}

// AbrirFirma trae a mi unidad el PDF de una petición de otra organización y devuelve
// la petición con su Ruta. La segunda vez, la misma copia.
func (p *PDF) AbrirFirma(ctx context.Context, id string) (PDFPeticionFirma, error) {
	return PostJSON[PDFPeticionFirma](ctx, p.motor, "/pdf/signature-requests/"+url.PathEscape(id)+"/open", struct{}{})
}

// TraerFirmado trae a mi unidad la copia firmada por alguien de otra organización.
func (p *PDF) TraerFirmado(ctx context.Context, id string) (PDFPeticionFirma, error) {
	return PostJSON[PDFPeticionFirma](ctx, p.motor, "/pdf/signature-requests/"+url.PathEscape(id)+"/fetch-signed", struct{}{})
}

// EntregarFirma da por firmada la petición `id` con el PDF firmado de `rutaFirmada`, que
// tiene que estar ya en Drive: el servicio lo baja y comprueba la firma nueva.
func (p *PDF) EntregarFirma(ctx context.Context, id, rutaFirmada string) (PDFPeticionFirma, error) {
	f, err := p.PorRuta(ctx, rutaFirmada)
	if err != nil {
		return PDFPeticionFirma{}, err
	}
	return PostJSON[PDFPeticionFirma](ctx, p.motor, "/pdf/signature-requests/"+url.PathEscape(id)+"/sign",
		map[string]string{"signed_file_id": f.ID, "signed_path": rutaFirmada})
}

// RechazarFirma: quien la recibió no la firma.
func (p *PDF) RechazarFirma(ctx context.Context, id string) (PDFPeticionFirma, error) {
	return PostJSON[PDFPeticionFirma](ctx, p.motor, "/pdf/signature-requests/"+url.PathEscape(id)+"/decline", struct{}{})
}

// CancelarFirma: quien la pidió la retira.
func (p *PDF) CancelarFirma(ctx context.Context, id string) error {
	resp, err := p.motor.Do(ctx, Request{Method: http.MethodDelete, Path: "/pdf/signature-requests/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
