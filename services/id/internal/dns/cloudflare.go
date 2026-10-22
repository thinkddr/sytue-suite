// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package dns es el cliente REST ACOTADO de Cloudflare que usa Sytue ID en el
// alta inicial (mig 0027). Hace exactamente dos cosas, ambas reales:
//
//  1. PUBLICAR el subdominio de una organización (<slug>.sytue.eu) en NUESTRA
//     zona, con la API de registros DNS:
//     GET  /zones/{zone_id}/dns_records?name.exact=<fqdn>
//     POST /zones/{zone_id}/dns_records
//     Permiso de token necesario: «Zone · DNS · Edit» sobre la zona de la suite.
//
//  2. CONSULTAR disponibilidad y PRECIO de un dominio que el cliente se plantea
//     registrar, con la API de Registrar (beta), solo LECTURA:
//     POST /accounts/{account_id}/registrar/domain-check  {"domains":["acme.com"]}
//
// Lo que este cliente NO hace, a propósito: REGISTRAR (comprar) dominios. La API
// de Cloudflare sí lo permite hoy —POST /accounts/{account_id}/registrar/registrations,
// beta de 2026-04-15—, pero registra el dominio EN LA CUENTA DE PLATAFORMA de
// Sytue y lo cobra a SU medio de pago por defecto (requisito documentado del
// endpoint). Sin una vía para cobrárselo al cliente, «comprar» aquí sería gastar
// nuestro dinero sin factura y dejar el dominio a nombre de quien no lo pidió.
// Mientras eso no exista, no se implementa ni se insinúa: el asistente enseña el
// precio REAL y remite al cliente a registrarlo donde prefiera.
//
//  3. CONFIGURAR EL DOMINIO DEL CLIENTE en SU propia cuenta de Cloudflare, con
//     un token que aporta ÉL (ver ForCustomer): localizar su zona (GET /zones),
//     leer lo que ya tiene puesto (ListRecords) y crear o —solo con su permiso
//     explícito— sustituir registros (CreateRecord / UpdateRecord).
//
// Credenciales: el token de PLATAFORMA llega por configuración (jamás
// por-organización, jamás en el navegador) y sirve para (1) y (2). El de (3) es un
// secreto DE UN TERCERO y viaja en un cliente aparte: no se mezcla con el nuestro.
// Ningún token se registra en ningún log ni viaja dentro de ningún error: los
// errores llevan el código y el mensaje que devuelve Cloudflare, nada más.
//
// Egress por platform/pkg/ssrfhttp (barrera SSRF de la suite) aunque el destino
// sea siempre api.cloudflare.com: defensa en profundidad, sin excepciones.
package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

const (
	// apiBase es la raíz de la API v4 de Cloudflare.
	apiBase = "https://api.cloudflare.com/client/v4"
	// maxBody acota el cuerpo que decodificamos de una respuesta.
	maxBody = 1 << 20
	// requestTimeout acota cada llamada: el asistente de alta espera por ella, así
	// que no puede quedarse colgado si Cloudflare no responde.
	requestTimeout = 10 * time.Second
)

// Client habla con la API de Cloudflare con el token de plataforma. Un *Client
// nil significa «Sytue no tiene Cloudflare configurado aquí»: todos sus métodos
// devuelven ErrNotConfigured en vez de reventar, para que el llamante degrade con
// un mensaje honesto en vez de romper el alta.
type Client struct {
	token   string
	base    string
	account string
	zone    string
	http    *http.Client
}

// ErrNotConfigured: no hay token (o falta la coordenada que esa operación
// necesita: zona para el DNS, cuenta para el Registrar).
var ErrNotConfigured = errors.New("dns: Cloudflare no está configurado")

// New construye el cliente. Sin token devuelve nil (ver Client). `base` vacío ⇒
// la API real; los tests le pasan la URL de un httptest.
func New(token, account, zone, base string) *Client {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if base == "" {
		base = apiBase
	}
	return &Client{
		token:   strings.TrimSpace(token),
		base:    strings.TrimRight(base, "/"),
		account: strings.TrimSpace(account),
		zone:    strings.TrimSpace(zone),
		http:    ssrfhttp.NewClient(requestTimeout, 3),
	}
}

// ForCustomer devuelve un cliente que habla con la cuenta de Cloudflare DEL
// CLIENTE —su token, su zona— contra el MISMO destino que este y con la misma
// barrera SSRF. El token de plataforma NO se copia: son credenciales ajenas y
// mezclarlas sería darle a un cliente permisos sobre nuestra zona.
//
// Funciona sobre un receptor nil a propósito: Sytue puede no tener Cloudflare
// configurado y el cliente traer igualmente el suyo.
func (c *Client) ForCustomer(token, zone string) *Client {
	base := ""
	if c != nil {
		base = c.base
	}
	return New(token, "", zone, base)
}

// HasZone indica si se puede tocar el DNS de la suite (token + zona).
func (c *Client) HasZone() bool { return c != nil && c.zone != "" }

// HasAccount indica si se puede consultar el Registrar (token + cuenta).
func (c *Client) HasAccount() bool { return c != nil && c.account != "" }

// --- Errores de la API ------------------------------------------------------

// APIError es un error DEVUELTO por Cloudflare, con su código y su mensaje
// reales. Es lo que se registra para el operador; al usuario se le traduce.
type APIError struct {
	Status  int
	Code    int
	Message string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("cloudflare: HTTP %d, código %d: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("cloudflare: HTTP %d: %s", e.Status, e.Message)
}

// Denied indica que el TOKEN no vale o no tiene el permiso necesario. Es el
// fallo de configuración típico (token sin «DNS · Edit», o sin Registrar) y se
// reporta distinto de un fallo pasajero: no se arregla reintentando.
func (e *APIError) Denied() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// Missing indica que el recurso no existe (zona o cuenta equivocada en config).
func (e *APIError) Missing() bool { return e.Status == http.StatusNotFound }

// Exists indica que el registro YA estaba en la zona. Para nosotros es éxito:
// publicar el subdominio es idempotente.
//
// 81053/81057 son los códigos de «registro duplicado» de la API de DNS; el
// contraste con el texto cubre las variantes que Cloudflare ha ido usando.
// El 409 lo aporta Scaleway, que no numera sus errores: un conflicto sobre un
// registro es «ya estaba». Cloudflare no usa 409 para DNS, así que no cambia
// nada de su lado.
func (e *APIError) Exists() bool {
	return e.Code == 81053 || e.Code == 81057 || e.Status == http.StatusConflict ||
		strings.Contains(strings.ToLower(e.Message), "already exists")
}

// asAPIError extrae el *APIError de un error (o nil si es de transporte).
func asAPIError(err error) *APIError {
	var e *APIError
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// Denied/Missing/Exists sobre un error cualquiera (nil-safe): azúcar para los
// llamantes, que casi siempre parten de un error genérico.
func Denied(err error) bool { e := asAPIError(err); return e != nil && e.Denied() }

// Missing indica «la zona o la cuenta de la configuración no existe».
func Missing(err error) bool { e := asAPIError(err); return e != nil && e.Missing() }

// Exists indica «el registro ya estaba» (éxito idempotente).
func Exists(err error) bool { e := asAPIError(err); return e != nil && e.Exists() }

// Detail devuelve el mensaje REAL de Cloudflare para registrarlo/mostrarlo al
// operador. Nunca incluye el token.
func Detail(err error) string {
	if e := asAPIError(err); e != nil {
		return e.Message
	}
	// Sin *APIError el fallo es de transporte, y desde que hay dos proveedores
	// —Cloudflare y Scaleway— nombrar a uno de los dos aquí sería mandar a mirar
	// el sitio equivocado la mitad de las veces.
	return "no hemos podido hablar con el proveedor de DNS"
}

// --- Transporte -------------------------------------------------------------

// do ejecuta una llamada y decodifica el sobre estándar de Cloudflare
// ({success, errors, messages, result}). `in` nil ⇒ sin cuerpo; `out` nil ⇒ se
// descarta el resultado.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	if c == nil {
		return ErrNotConfigured
	}
	var body io.Reader = http.NoBody
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	// El token va SOLO en esta cabecera. Ni en la URL (acabaría en logs de acceso)
	// ni en el cuerpo.
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if derr := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&env); derr != nil {
		return &APIError{Status: resp.StatusCode, Message: "respuesta ilegible de Cloudflare"}
	}
	if !env.Success || resp.StatusCode >= 300 {
		e := &APIError{Status: resp.StatusCode, Message: "sin detalle"}
		if len(env.Errors) > 0 {
			e.Code, e.Message = env.Errors[0].Code, env.Errors[0].Message
		}
		return e
	}
	if out == nil || len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// --- Zonas ------------------------------------------------------------------

// Zone es una zona DNS tal y como la nombra Cloudflare.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// FindZone busca la zona por nombre exacto ENTRE LAS QUE ALCANZA EL TOKEN.
//
// Los dos fracasos posibles son distintos y el llamante tiene que poder
// separarlos, porque lo que el usuario debe hacer no es lo mismo:
//
//	error con Denied(err) → el token no vale o le falta el permiso
//	ok=false, err=nil     → el token vale, pero esa zona no está a su alcance
//	                        (otra cuenta, o token acotado a otro dominio)
func (c *Client) FindZone(ctx context.Context, name string) (Zone, bool, error) {
	var zs []Zone
	q := url.Values{"name": {name}, "per_page": {"1"}}
	if err := c.do(ctx, http.MethodGet, "/zones?"+q.Encode(), nil, &zs); err != nil {
		return Zone{}, false, err
	}
	if len(zs) == 0 {
		return Zone{}, false, nil
	}
	return zs[0], true, nil
}

// --- Registros DNS ----------------------------------------------------------

// Record es un registro DNS, con lo mínimo que usamos de él.
type Record struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl,omitempty"`
	// Priority solo la lleva MX. Va con omitempty porque Cloudflare rechaza el
	// campo en los tipos que no lo admiten (TXT, CNAME).
	Priority int `json:"priority,omitempty"`
	// Proxied solo aplica a A/AAAA/CNAME; omitempty la deja fuera del cuerpo en
	// TXT y MX, donde Cloudflare no la acepta. Omitirla equivale a false, que es
	// justo lo que quiere decir un registro sin proxy.
	Proxied bool   `json:"proxied,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// ListRecords devuelve los registros de la zona con ese nombre EXACTO, acotados
// al tipo si se indica (vacío = todos). Es la lectura que respalda la
// previsualización: sin ella no se puede distinguir «ya está puesto y bien» de
// «ahí hay otra cosa que no es nuestra».
func (c *Client) ListRecords(ctx context.Context, name, typ string) ([]Record, error) {
	if !c.HasZone() {
		return nil, ErrNotConfigured
	}
	q := url.Values{"name.exact": {name}, "per_page": {"100"}}
	if typ != "" {
		q.Set("type", typ)
	}
	var recs []Record
	path := "/zones/" + url.PathEscape(c.zone) + "/dns_records?" + q.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// tiposDeDireccion son los registros que PUBLICAN una dirección web. Un TXT o un
// MX en el mismo nombre no lo hacen: sirven para el correo y para verificaciones.
var tiposDeDireccion = [...]string{"CNAME", "A", "AAAA"}

// FindRecord dice si la DIRECCIÓN de ese nombre está publicada: busca en la zona
// un CNAME, A o AAAA con ese nombre EXACTO. `ok` false = no está (no es un error:
// es la respuesta a la pregunta).
//
// EL TIPO IMPORTA, y no filtrarlo costó una organización sin dirección durante
// semanas. Esto preguntaba por CUALQUIER tipo, así que en un subdominio con
// correo —el SPF y el MX de una organización con buzones en `@<slug>.sytue.eu`—
// encontraba el TXT y respondía «ya está publicada». `ensureTenantDNS` daba el
// alta por buena y NUNCA creaba el registro de la dirección: el asistente decía
// `live` y el navegador no encontraba el servidor. Un verde falso, que es la
// peor clase de verde.
//
// El hermano de Scaleway sí filtraba (por CNAME) desde el principio y lo explica
// en su propio comentario; esta implementación se quedó sin ese cuidado.
func (c *Client) FindRecord(ctx context.Context, name string) (rec Record, ok bool, err error) {
	for _, typ := range tiposDeDireccion {
		recs, e := c.ListRecords(ctx, name, typ)
		if e != nil {
			return Record{}, false, e
		}
		if len(recs) > 0 {
			return recs[0], true, nil
		}
	}
	return Record{}, false, nil
}

// tiposDeCorreo son los registros que un nombre con buzones propios lleva en su
// dirección EXACTA: el MX que dice quién recibe y el TXT del SPF. No publican una
// dirección web, pero mandan sobre cómo puede publicarse.
var tiposDeCorreo = [...]string{"MX", "TXT"}

// HasMail dice si el nombre ya tiene correo publicado, y existe por una regla del
// DNS que no se puede negociar: un CNAME NO admite ningún otro registro en el
// mismo nombre (RFC 1034 §3.6.2 y RFC 2181 §10.1). Una organización con buzones
// en `@<slug>.sytue.eu` tiene ahí su MX y su SPF, así que el CNAME de la dirección
// lo rechaza el proveedor —y el alta se queda sin publicar—. Cuando esto dice que
// sí, la dirección va con A/AAAA, que sí conviven con el correo.
//
// Pasó en producción con `corporate`: MX y SPF puestos, ninguna dirección, y el
// asistente enseñando un verde que el navegador no podía resolver.
func (c *Client) HasMail(ctx context.Context, name string) (bool, error) {
	for _, typ := range tiposDeCorreo {
		recs, err := c.ListRecords(ctx, name, typ)
		if err != nil {
			return false, err
		}
		if len(recs) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// CreateRecord crea el registro en la zona configurada. Si ya existía, Cloudflare
// responde error de duplicado: el llamante lo trata como éxito con Exists(err).
func (c *Client) CreateRecord(ctx context.Context, rec Record) (Record, error) {
	if !c.HasZone() {
		return Record{}, ErrNotConfigured
	}
	var out Record
	path := "/zones/" + url.PathEscape(c.zone) + "/dns_records"
	if err := c.do(ctx, http.MethodPost, path, rec, &out); err != nil {
		return Record{}, err
	}
	return out, nil
}

// UpdateRecord SUSTITUYE por completo el registro `id` (PUT). Pisar un registro
// que puso el cliente es destructivo: solo se llama tras su confirmación
// explícita, nunca por iniciativa nuestra.
func (c *Client) UpdateRecord(ctx context.Context, id string, rec Record) (Record, error) {
	if !c.HasZone() {
		return Record{}, ErrNotConfigured
	}
	var out Record
	path := "/zones/" + url.PathEscape(c.zone) + "/dns_records/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodPut, path, rec, &out); err != nil {
		return Record{}, err
	}
	return out, nil
}

// --- Registrar: disponibilidad y precio (SOLO LECTURA) ----------------------

// Pricing es el precio REAL que Cloudflare devuelve, en las unidades que él usa
// (cadenas decimales y su moneda: "8.57", "USD"). No se recalcula ni se redondea:
// lo que se le enseña al cliente es lo que dijo el registrador.
type Pricing struct {
	Currency         string `json:"currency"`
	RegistrationCost string `json:"registration_cost"`
	RenewalCost      string `json:"renewal_cost"`
}

// Availability es la respuesta de Cloudflare sobre UN dominio consultado.
// Reason viene informado cuando Registrable es false: "domain_unavailable",
// "extension_not_supported_via_api", "extension_not_supported",
// "extension_disallows_registration".
type Availability struct {
	Name        string   `json:"name"`
	Registrable bool     `json:"registrable"`
	Tier        string   `json:"tier"`
	Reason      string   `json:"reason"`
	Pricing     *Pricing `json:"pricing"`
}

// CheckDomain consulta disponibilidad y precio reales de un dominio.
// Es una consulta: no reserva, no registra y no mueve dinero.
func (c *Client) CheckDomain(ctx context.Context, name string) (Availability, error) {
	if !c.HasAccount() {
		return Availability{}, ErrNotConfigured
	}
	var out struct {
		Domains []Availability `json:"domains"`
	}
	path := "/accounts/" + url.PathEscape(c.account) + "/registrar/domain-check"
	if err := c.do(ctx, http.MethodPost, path, map[string][]string{"domains": {name}}, &out); err != nil {
		return Availability{}, err
	}
	if len(out.Domains) == 0 {
		return Availability{}, errors.New("dns: Cloudflare no devolvió ningún dominio")
	}
	return out.Domains[0], nil
}

// AplanaCNAMEProxied es true: con el proxy encendido Cloudflare no publica un
// CNAME, publica las direcciones de su edge (CNAME flattening). Por eso un CNAME
// proxied convive con el MX del mismo nombre, que es lo que permite dar a una
// organización con buzones un subdominio servido por el túnel y sin ninguna IP
// que mantener al día.
//
// Comprobado contra la API el 2026-09-07 sobre un nombre de usar y tirar: MX
// creado, y después CNAME proxied EN EL MISMO NOMBRE aceptado, los dos con
// success=true y errors vacío.
func (c *Client) AplanaCNAMEProxied() bool { return true }
