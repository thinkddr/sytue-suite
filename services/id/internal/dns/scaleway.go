// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Publicador de la zona de la SUITE contra Scaleway Domains & DNS.
//
// El único cliente DNS que había era de Cloudflare, y la zona `sytue.eu` la
// sirve Scaleway (`dig NS sytue.eu` → `ns0.dom.scw.cloud`). Consecuencia:
// `ensureTenantDNS` salía SIEMPRE por `unconfigured` y el subdominio por
// organización —`<slug>.sytue.eu`, que el asistente de alta enseña— no lo
// publicaba nadie. No era una variable que faltara en el chart: el publicador no
// estaba escrito.
//
// Hace UNA cosa: leer y crear registros en NUESTRA zona. No registra dominios,
// no toca la zona de nadie más y no borra nada — el alta de una organización no
// puede tener permiso para retirar un registro existente.
//
//	GET   /domain/v2beta1/dns-zones/{zona}/records?name=<corto>&type=CNAME
//	PATCH /domain/v2beta1/dns-zones/{zona}/records  {"changes":[{"add":{"records":[…]}}]}
//
// Dos diferencias con Cloudflare que se traducen aquí y no en el llamante:
// Scaleway nombra los registros en formato CORTO (`acme`, no `acme.sytue.eu`) y
// llama `data` a lo que Cloudflare llama `content`. Y no tiene proxy, así que
// `Proxied` no viaja: en Scaleway un CNAME es un CNAME.
//
// Egress por platform/pkg/ssrfhttp como el resto de la suite, aunque el destino
// sea siempre api.scaleway.com.
const scwAPIBase = "https://api.scaleway.com/domain/v2beta1"

// ZonePublisher es lo que Sytue ID necesita para publicar el subdominio de una
// organización en la zona de la suite. La cumplen los DOS proveedores —Scaleway,
// que es quien sirve `sytue.eu`, y el cliente de Cloudflare que ya existía—, y
// por eso es una interfaz y no un tipo concreto: sin ella habría que elegir uno
// en tiempo de compilación y el despliegue que tenga el otro se queda sin
// publicar nada, en silencio, que es exactamente lo que pasaba.
type ZonePublisher interface {
	HasZone() bool
	FindRecord(ctx context.Context, name string) (Record, bool, error)
	// HasMail decide la FORMA de publicar: con correo en el mismo nombre el
	// CNAME es ilegal y la dirección va con A/AAAA. Ver Client.HasMail.
	HasMail(ctx context.Context, name string) (bool, error)
	// AplanaCNAMEProxied dice si el proveedor PUBLICA un CNAME proxied como
	// registros de dirección en vez de como un CNAME de verdad. Cuando lo hace,
	// la regla de RFC que prohíbe convivir con el MX no llega a aplicarse: en el
	// cable no hay ningún CNAME. Es la diferencia entre poder servir el
	// subdominio de una organización por el túnel —sin IP que mantener— y tener
	// que publicar A/AAAA con la dirección del origen.
	//
	// Solo vale con proxy ENCENDIDO. Un CNAME sin proxy es un CNAME, y ahí la
	// regla se aplica entera.
	AplanaCNAMEProxied() bool
	CreateRecord(ctx context.Context, rec Record) (Record, error)
}

// Scaleway habla con la API de Domains & DNS con la clave secreta de
// plataforma. Un *Scaleway nil significa «este despliegue no publica en
// Scaleway»: sus métodos devuelven ErrNotConfigured en vez de reventar, para que
// el alta degrade con un estado honesto en lugar de romperse.
type Scaleway struct {
	secret string
	zone   string
	base   string
	http   *http.Client
}

// NewScaleway construye el publicador. Sin clave o sin zona devuelve nil (ver
// Scaleway). `base` vacío ⇒ la API real; los tests le pasan un httptest.
func NewScaleway(secretKey, zone, base string) *Scaleway {
	secretKey, zone = strings.TrimSpace(secretKey), strings.TrimSpace(zone)
	if secretKey == "" || zone == "" {
		return nil
	}
	if base == "" {
		base = scwAPIBase
	}
	return &Scaleway{
		secret: secretKey,
		zone:   zone,
		base:   strings.TrimRight(base, "/"),
		http:   ssrfhttp.NewClient(requestTimeout, 3),
	}
}

// HasZone indica si se puede tocar el DNS de la suite.
func (c *Scaleway) HasZone() bool { return c != nil && c.zone != "" }

// corto convierte el FQDN que usa el resto del código (`acme.sytue.eu`) al
// nombre relativo que exige Scaleway (`acme`). Un nombre que no cuelgue de la
// zona se devuelve tal cual: crearlo fallará en la API, que es mejor que
// publicarlo en el sitio equivocado.
func (c *Scaleway) corto(fqdn string) string {
	fqdn = strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	if s := strings.TrimSuffix(fqdn, "."+c.zone); s != fqdn {
		return s
	}
	return fqdn
}

type scwRecord struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
	TTL  int    `json:"ttl,omitempty"`
}

// FindRecord busca un registro por su FQDN. Devuelve ok=false si no está.
//
// Se filtra TAMBIÉN en cliente, además de pasar el filtro en la URL: si la API
// ignorara el parámetro, quedarse con el primer registro de la zona publicaría
// un «ya existe» falso sobre el subdominio de otra organización.
func (c *Scaleway) FindRecord(ctx context.Context, name string) (Record, bool, error) {
	if !c.HasZone() {
		return Record{}, false, ErrNotConfigured
	}
	corto := c.corto(name)
	q := url.Values{"name": {corto}, "type": {"CNAME"}}
	var out struct {
		Records []scwRecord `json:"records"`
	}
	path := "/dns-zones/" + url.PathEscape(c.zone) + "/records?" + q.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return Record{}, false, err
	}
	for _, r := range out.Records {
		if strings.EqualFold(r.Name, corto) {
			return Record{ID: r.ID, Type: r.Type, Name: name, Content: r.Data, TTL: r.TTL}, true, nil
		}
	}
	return Record{}, false, nil
}

// HasMail dice si el nombre ya tiene MX o TXT. Mismo criterio y misma cautela
// que FindRecord: se filtra TAMBIÉN en cliente, porque si la API ignorase el
// parámetro `name` bastaría un MX de otra organización para creer que este nombre
// tiene correo y publicar la dirección de la forma equivocada.
func (c *Scaleway) HasMail(ctx context.Context, name string) (bool, error) {
	if !c.HasZone() {
		return false, ErrNotConfigured
	}
	corto := c.corto(name)
	for _, typ := range tiposDeCorreo {
		q := url.Values{"name": {corto}, "type": {typ}}
		var out struct {
			Records []scwRecord `json:"records"`
		}
		path := "/dns-zones/" + url.PathEscape(c.zone) + "/records?" + q.Encode()
		if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return false, err
		}
		for _, r := range out.Records {
			if strings.EqualFold(r.Name, corto) {
				return true, nil
			}
		}
	}
	return false, nil
}

// CreateRecord añade el registro a la zona. Idempotente por el lado del
// llamante: si ya estaba, la API responde error y `Exists(err)` lo trata como
// éxito.
func (c *Scaleway) CreateRecord(ctx context.Context, rec Record) (Record, error) {
	if !c.HasZone() {
		return Record{}, ErrNotConfigured
	}
	// TTL: el 1 de Cloudflare significa «automático» y en Scaleway sería un
	// segundo, que machaca las cachés de medio internet por un registro que no
	// cambia nunca. Una hora es el default de la propia API.
	ttl := rec.TTL
	if ttl <= 1 {
		ttl = 3600
	}
	body := map[string]any{
		"changes": []any{map[string]any{
			"add": map[string]any{"records": []scwRecord{{
				Name: c.corto(rec.Name), Type: rec.Type, Data: rec.Content, TTL: ttl,
			}}},
		}},
		// Sin devolver la zona entera: son cientos de registros que no se miran.
		"return_all_records": false,
	}
	path := "/dns-zones/" + url.PathEscape(c.zone) + "/records"
	if err := c.do(ctx, http.MethodPatch, path, body, nil); err != nil {
		return Record{}, err
	}
	rec.TTL = ttl
	return rec, nil
}

// do ejecuta una llamada contra la API de Scaleway. El sobre de error es
// `{"message":…,"type":…}`, sin el `success/errors` de Cloudflare, así que se
// traduce a *APIError para que los llamantes usen los mismos Denied/Missing/
// Exists sin saber con qué proveedor están hablando.
func (c *Scaleway) do(ctx context.Context, method, path string, in, out any) error {
	if c == nil {
		return ErrNotConfigured
	}
	var cuerpo io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		cuerpo = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, cuerpo)
	if err != nil {
		return err
	}
	// La clave NUNCA viaja en un log ni dentro de un error: los errores llevan el
	// código y el mensaje que devuelve Scaleway, nada más.
	req.Header.Set("X-Auth-Token", c.secret)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := strings.TrimSpace(e.Message)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		if e.Type != "" {
			msg += " (" + e.Type + ")"
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// AplanaCNAMEProxied es false: Scaleway no tiene proxy, así que un CNAME suyo es
// un CNAME y no puede convivir con el MX (scaleway.go:38 lo dice ya para el
// campo Proxied, que en esta API ni siquiera viaja).
func (s *Scaleway) AplanaCNAMEProxied() bool { return false }
