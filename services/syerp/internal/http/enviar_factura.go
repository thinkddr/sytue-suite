// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"cmp"
	"context"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// La factura al cliente, por correo y con el PDF adjunto: lo que se hacía a mano
// imprimiendo a PDF desde el navegador, guardando el fichero y adjuntándolo desde
// el correo de cada cual. Sale del buzón del ERP (SMTP_FROM, el que firma el relé)
// con el nombre de la organización, y las respuestas van a quien la mandó.

var (
	errCorreoSinConfigurar = httpx.NewError(http.StatusServiceUnavailable, "correo_no_configurado",
		"This deployment has no outgoing mail server configured.")
	errFacturaNoEnviable = httpx.NewError(http.StatusUnprocessableEntity, "factura_no_enviable",
		"Only issued invoices can be sent: drafts and cancelled invoices cannot.")
	errDestinatario = httpx.NewError(http.StatusUnprocessableEntity, "destinatario_invalido",
		"The recipient is not a valid email address.")
	errEnvioCorreo = httpx.NewError(http.StatusBadGateway, "correo_no_enviado",
		"The mail server did not accept the message.")
)

// Topes del correo que escribe la persona: un asunto es una línea y el texto, unas
// frases. Lo demás es un error de quien llama, no una carta.
const (
	maxAsuntoEnvio  = 300
	maxMensajeEnvio = 10_000
)

// enviarAdjunto es correo.EnviarAdjunto; los tests lo sustituyen por uno que no
// abre conexiones.
type enviarAdjunto func(ctx context.Context, c correo.Config, op correo.Opciones, para, asunto, cuerpo string, a correo.Adjunto) error

func (h *Handlers) correoSaliente() correo.Config {
	return correo.Config{
		Addr: h.cfg.SMTPAddr, From: h.cfg.SMTPFrom, User: h.cfg.SMTPUser, Password: h.cfg.SMTPPassword,
		TLSServerName: h.cfg.SMTPTLSServerName,
	}
}

// facturaEnviable: el borrador aún no es una factura y la anulada ya no lo es.
func facturaEnviable(reg store.Registro) bool {
	return reg.Estado != "borrador" && reg.Estado != "anulado"
}

// PDFFactura — GET /api/v1/{modelo}/{id}/pdf — la factura en PDF, para descargar.
func (h *Handlers) PDFFactura(w http.ResponseWriter, r *http.Request) {
	f, _, ok := h.facturaParaCliente(w, r)
	if !ok {
		return
	}
	pdf, err := facturaPDF(f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	// FormatMediaType y no comillas a mano: un número con tildes o «» sale en la
	// forma de RFC 2231 y el navegador no lo destroza.
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": nombrePDF(f)}))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

// EnvioFactura — GET /api/v1/{modelo}/{id}/enviar — con qué arranca el diálogo de
// enviar: la dirección de facturación del cliente (o la general), si se puede
// enviar ya y si el despliegue tiene con qué.
func (h *Handlers) EnvioFactura(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Clave != "factura_venta" {
		httpx.Fail(w, r, errNoContable.WithMessage("Only customer invoices can be sent."))
		return
	}
	ctx := r.Context()
	reg, err := h.store.Obtener(ctx, m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"para":       h.correoDelCliente(ctx, reg),
		"enviable":   facturaEnviable(reg),
		"disponible": h.correoSaliente().Activo(),
	})
}

// correoDelCliente es la dirección de facturación del cliente, o su correo general.
func (h *Handlers) correoDelCliente(ctx context.Context, reg store.Registro) string {
	id := cadena(reg.Data["cliente"])
	if id == "" {
		return ""
	}
	c, err := h.store.Obtener(ctx, "contacto", id)
	if err != nil {
		return ""
	}
	return cmp.Or(strings.TrimSpace(cadena(c.Data["email_facturas"])), strings.TrimSpace(cadena(c.Data["correo"])))
}

// EnviarFactura — POST /api/v1/{modelo}/{id}/enviar — manda la factura en PDF a
// UNA dirección y lo apunta en el historial del documento.
func (h *Handlers) EnviarFactura(w http.ResponseWriter, r *http.Request) {
	// Recordatorio: es un recordatorio de pago de una factura ya enviada y sin cobrar. El
	// correo es el mismo —la factura en PDF a una dirección—, pero solo vale para una
	// factura abierta y en el historial queda como lo que es: «un recordatorio», no otro
	// envío, que es lo que se mira al decidir si hace falta el segundo.
	var cuerpo struct {
		Para         string `json:"para"`
		Asunto       string `json:"asunto"`
		Mensaje      string `json:"mensaje"`
		Recordatorio bool   `json:"recordatorio"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	para := strings.TrimSpace(cuerpo.Para)
	asunto := strings.TrimSpace(cuerpo.Asunto)
	switch {
	case !validation.IsEmail(para):
		httpx.Fail(w, r, errDestinatario)
		return
	case asunto == "" || utf8.RuneCountInString(asunto) > maxAsuntoEnvio:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The subject is empty or too long."))
		return
	case utf8.RuneCountInString(cuerpo.Mensaje) > maxMensajeEnvio:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The message is too long."))
		return
	}
	saliente := h.correoSaliente()
	if !saliente.Activo() {
		httpx.Fail(w, r, errCorreoSinConfigurar)
		return
	}
	f, reg, ok := h.facturaParaCliente(w, r)
	if !ok {
		return
	}
	if !facturaEnviable(reg) {
		httpx.Fail(w, r, errFacturaNoEnviable)
		return
	}
	if cuerpo.Recordatorio && !facturaAbierta(reg) {
		httpx.Fail(w, r, errFacturaNoAbierta)
		return
	}
	pdf, err := facturaPDF(f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	ctx := r.Context()
	op := correo.Opciones{NombreRemitente: strings.TrimSpace(f.Emisor.Nombre)}
	if c, ok := authn.FromContext(ctx); ok {
		if dir := cmp.Or(c.Mailbox, c.Email); validation.IsEmail(dir) {
			op.ResponderA = dir
		}
	}
	enviar := h.enviarCorreo
	if enviar == nil {
		enviar = correo.EnviarAdjunto
	}
	adjunto := correo.Adjunto{Nombre: nombrePDF(f), Tipo: "application/pdf", Datos: pdf}
	if err := enviar(ctx, saliente, op, para, asunto, cuerpo.Mensaje, adjunto); err != nil {
		log.From(ctx).Warn("factura: no salió el correo", "factura", reg.ID, "error", err)
		httpx.Fail(w, r, errEnvioCorreo)
		return
	}

	// En el historial, en el idioma de quien la manda: es quien lo va a leer.
	clave := "historial.enviada_por_correo"
	if cuerpo.Recordatorio {
		clave = "historial.recordatorio_por_correo"
	}
	nota := strings.ReplaceAll(h.cat.Rotulo(clave, httpx.LanguageFromRequest(r)), "{para}", para)
	e, err := h.store.Anotar(ctx, "factura_venta", reg.ID, sub(r), nota)
	if err != nil {
		// El correo YA salió: decir que falló invitaría a mandarlo otra vez.
		log.From(ctx).Warn("factura: enviada, pero no se apuntó en el historial", "factura", reg.ID, "error", err)
		httpx.JSON(w, r, http.StatusOK, map[string]any{"enviada": true})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"enviada": true, "historial": e})
}
