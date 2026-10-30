// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// CORREO A CANAL: escribir a un canal de Sytue Rooms mandando un email.
//
// Es como entran hoy las alertas de la mitad de las herramientas del mercado: saben
// mandar un correo y poco más. Sin esto, la única vía a un canal es que alguien programe
// contra el webhook.
//
// La dirección es el TOKEN del webhook entrante que ya existía: `<token>@<dominio>`. No
// hay tabla nueva ni identidad nueva —el token ya dice canal y organización, ya está
// autorizado y ya se puede rotar—, y el modelo de confianza es exactamente el mismo que
// el de la URL del webhook: quien la conoce, publica.
//
// Los adjuntos NO se suben a Drive todavía; se nombran en el mensaje.
// ponytail: adjuntos por nombre. Subirlos exige credenciales de Drive en Mail y decidir
// en qué carpeta caen; hacerlo cuando alguien lo pida de verdad.

// channelMailTimeout acota la publicación. Corre dentro de la fase DATA del MX: si
// tardara, el remitente vería un tiempo de espera y reintentaría el lote entero.
const channelMailTimeout = 10 * time.Second

// channelMailMaxRunes acota el texto publicado. Un correo puede traer una firma
// corporativa de diez párrafos; el canal no es el sitio para ella.
const channelMailMaxRunes = 4000

// ChannelBridge publica en un canal de Sytue Rooms el correo dirigido a su dirección.
type ChannelBridge struct {
	// domain es el dominio de las direcciones de canal (p. ej. "rooms.sytue.eu"). Vacío
	// ⇒ la función que decide lo trata como apagado y ninguna dirección casa.
	domain string
	// hooksBase es la raíz del servicio Rooms (p. ej. "http://rooms:8085").
	hooksBase string
	client    *http.Client
}

// NewChannelBridge construye el puente. Con dominio o base vacíos devuelve nil: el
// correo a canal queda apagado y el resto de la entrega no cambia.
func NewChannelBridge(dominio, hooksBase string) *ChannelBridge {
	dominio = strings.ToLower(strings.Trim(strings.TrimSpace(dominio), "@."))
	hooksBase = strings.TrimSuffix(strings.TrimSpace(hooksBase), "/")
	if dominio == "" || hooksBase == "" {
		return nil
	}
	return &ChannelBridge{
		domain:    dominio,
		hooksBase: hooksBase,
		client:    &http.Client{Timeout: channelMailTimeout},
	}
}

// Token devuelve el token de canal de una dirección, y si esa dirección es de canal.
//
// Acepta dos formas, y la que se ENSEÑA lleva delante el nombre del canal:
//
//	general+<token>@rooms.ejemplo    ← la que da la interfaz
//	<token>@rooms.ejemplo            ← sigue valiendo
//
// El nombre delante no lo comprueba nadie: es decoración para quien lee la dirección en
// un formulario de alertas o se la dicta a alguien. Un token de cuarenta y tres
// caracteres a secas no se puede dictar, ni teclear, ni reconocer en una lista.
//
// Lo que autoriza es el token, y va SIEMPRE al final: quien cambie el nombre de delante
// no cambia a qué canal escribe, y quien no ponga ninguno tampoco.
//
// Solo se mira el dominio: la validez del token la decide Rooms, que es quien lo guarda.
// Adelantar aquí esa decisión duplicaría la verja en dos servicios.
func (b *ChannelBridge) Token(address string) (string, bool) {
	if b == nil {
		return "", false
	}
	at := strings.LastIndex(address, "@")
	if at <= 0 {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSpace(address[at+1:]), b.domain) {
		return "", false
	}
	local := strings.TrimSpace(address[:at])
	// El ÚLTIMO «+»: un nombre de canal puede llevarlo, el token no.
	if plus := strings.LastIndexByte(local, '+'); plus >= 0 {
		local = strings.TrimSpace(local[plus+1:])
	}
	if local == "" {
		return "", false
	}
	return local, true
}

// ErrChannelRejected indica que Rooms no aceptó la publicación (token desconocido,
// revocado o rate-limit). El MX lo traduce a un rechazo, nunca a un «entregado» mudo.
var ErrChannelRejected = errors.New("delivery: el canal no aceptó el mensaje")

// Publish manda al canal el asunto y el cuerpo del correo.
func (b *ChannelBridge) Publish(ctx context.Context, token string, raw []byte) error {
	return b.PublishReader(ctx, token, bytes.NewReader(raw), int64(len(raw)))
}

// PublishReader manda al canal el asunto y el cuerpo leyendo el correo en flujo desde rs.
func (b *ChannelBridge) PublishReader(ctx context.Context, token string, rs io.ReadSeeker, size int64) error {
	env, err := mime.ParseSobre(rs, size)
	if err != nil {
		return fmt.Errorf("delivery: parsear correo de canal: %w", err)
	}
	_, _ = rs.Seek(0, io.SeekStart)
	texto := channelText(env, mime.ReadBodyReader(rs, channelMailMaxRunes))
	body, err := json.Marshal(map[string]string{"text": texto})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		// Escapado: la parte local de una dirección admite `/`, `?`, `#` y `..` (COM-11).
		b.hooksBase+"/hooks/"+url.PathEscape(token), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("delivery: publicar en el canal: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: estado %d", ErrChannelRejected, resp.StatusCode)
	}
	return nil
}

// channelText compone el mensaje: el asunto como título y el cuerpo debajo.
func channelText(env domain.Message, body mime.Body) string {
	var sb strings.Builder
	if asunto := strings.TrimSpace(env.Subject); asunto != "" {
		// Negrita, no encabezado: en un canal, un `#` produce un titular enorme.
		sb.WriteString("**" + asunto + "**\n")
	}
	if de := remitenteLegible(env.FromAddr, env.FromName); de != "" {
		sb.WriteString("_" + de + "_\n\n")
	}
	sb.WriteString(body.Text)

	if len(body.Attachments) > 0 {
		sb.WriteString("\n")
		for _, n := range body.Attachments {
			sb.WriteString("\n📎 " + n)
		}
	}
	texto := strings.TrimSpace(sb.String())
	if texto == "" {
		// Rooms rechaza un cuerpo vacío. Mejor una línea que diga que llegó algo que un
		// rechazo que el remitente interpretará como fallo de entrega.
		return "(correo sin contenido)"
	}
	return texto
}

// remitenteLegible devuelve «Nombre <dirección>», o lo que haya de los dos.
func remitenteLegible(addr, name string) string {
	addr, name = strings.TrimSpace(addr), strings.TrimSpace(name)
	switch {
	case addr == "":
		return name
	case name == "" || strings.EqualFold(name, addr):
		return addr
	default:
		return name + " <" + addr + ">"
	}
}

// esAutomatico indica si el correo es una respuesta automática o un envío masivo.
//
// Se descartan porque son la mitad de un bucle: si un canal recibiera el «estoy de
// vacaciones» de alguien, y ese alguien tuviera reenvío al canal, el par se retroalimenta.
// RFC 3834 existe justo para poder cortarlo.
func esAutomatico(raw []byte) bool {
	return esAutomaticoReader(bytes.NewReader(raw))
}

func esAutomaticoReader(r io.Reader) bool {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return false
	}
	h := m.Header
	if v := strings.ToLower(strings.TrimSpace(h.Get("Auto-Submitted"))); v != "" && v != "no" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(h.Get("Precedence"))) {
	case "bulk", "list", "junk":
		return true
	}
	return h.Get("List-Id") != "" || h.Get("X-Auto-Response-Suppress") != ""
}
