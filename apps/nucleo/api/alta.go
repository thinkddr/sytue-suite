// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// Alta es el asistente de alta inicial de Sytue ID (services/id, `/api/v1/onboarding`):
// lo que en la web es `/bienvenida` (`Onboarding.tsx`), que habla con él a través de un
// proxy del BFF que solo reenvía el token. Aquí se le habla directo con el de la sesión.
//
// Cada llamada HACE algo de verdad —crear la organización, publicar su dirección,
// comprobar un DNS, invitar, crear roles— y el estado se guarda en el servicio, así que
// cerrar la app a medias y volver retoma donde se dejó.
type Alta struct{ c *Client }

// NewAlta crea el cliente sobre la raíz de Sytue ID (ServiceURL(apex, "id")).
func NewAlta(servicio string, s Session, h *http.Client) *Alta {
	return &Alta{c: New(servicio+"/api/v1/onboarding", s, h)}
}

// Los pasos, con los identificadores que valida Sytue ID (domain/onboarding.go).
const (
	AltaPasoModo        = "modo"
	AltaPasoIdentidad   = "identidad"
	AltaPasoPersonas    = "personas"
	AltaPasoFacturacion = "facturacion"
	AltaPasoRoles       = "roles"
	AltaPasoListo       = "listo"
)

// Los dos recorridos.
const (
	AltaIndividual   = "individual"
	AltaOrganizacion = "organization"
)

// AltaSubdominio es el estado REAL del registro DNS de `acme.sytue.eu`: `live`
// publicado, `failed` no se pudo (con `Detail` ya traducido), `unconfigured` aquí no se
// publica.
type AltaSubdominio struct {
	Host   string `json:"host"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// AltaOrg es la organización del alta, con su dirección y el estado de su DNS.
type AltaOrg struct {
	ID        string          `json:"id"`
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Subdomain string          `json:"subdomain"`
	DNS       *AltaSubdominio `json:"dns"`
}

// AltaDominio es el dominio propio: el registro TXT exacto que hay que crear y el
// estado de la última comprobación (`verified`, `pending`, `mismatch`, `unreachable`, o
// vacío si en esta respuesta no se ha consultado el DNS).
type AltaDominio struct {
	Host        string   `json:"host"`
	RecordName  string   `json:"record_name"`
	RecordValue string   `json:"record_value"`
	Verified    bool     `json:"verified"`
	Status      string   `json:"status"`
	Found       []string `json:"found"`
}

// AltaFacturacion son los datos de facturación de la organización.
type AltaFacturacion struct {
	LegalName    string `json:"legal_name"`
	TaxID        string `json:"tax_id"`
	Address      string `json:"address"`
	BillingEmail string `json:"billing_email"`
}

// AltaRolRecomendado es uno del catálogo fijo, con sus permisos reales.
type AltaRolRecomendado struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Permissions []string `json:"permissions"`
}

// AltaCuenta es quién se está dando de alta.
type AltaCuenta struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AltaEstado es dónde se quedó el alta y lo que REALMENTE existe ya.
type AltaEstado struct {
	Mode         string           `json:"mode"`
	Step         string           `json:"step"`
	Completed    bool             `json:"completed"`
	SuiteDomain  string           `json:"suite_domain"`
	Account      *AltaCuenta      `json:"account"`
	Organization *AltaOrg         `json:"organization"`
	Domain       *AltaDominio     `json:"domain"`
	Billing      *AltaFacturacion `json:"billing"`
	Invitations  []struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"invitations"`
	Roles []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	} `json:"roles"`
	RecommendedRoles []AltaRolRecomendado `json:"recommended_roles"`
}

// Estado lee el alta (`GET /onboarding`).
func (a *Alta) Estado(ctx context.Context) (AltaEstado, error) {
	return JSON[AltaEstado](ctx, a.c, Request{Path: "/"})
}

// AltaGuardado es lo que se guarda al avanzar: el modo, el paso alcanzado y, si
// viene, la facturación. Lo vacío no se toca en el servicio.
type AltaGuardado struct {
	Mode      string           `json:"mode,omitempty"`
	Step      string           `json:"step,omitempty"`
	Completed bool             `json:"completed,omitempty"`
	Billing   *AltaFacturacion `json:"billing,omitempty"`
}

// Guardar guarda el paso (`PATCH /onboarding`): es lo que hace real la reanudación.
func (a *Alta) Guardar(ctx context.Context, g AltaGuardado) error {
	return a.enviar(ctx, http.MethodPatch, "/", g, nil)
}

// AltaIdentificador es la disponibilidad del identificador, en vivo: `Reason` lo
// traduce Sytue ID; `Subdomain` es la dirección que tendría.
type AltaIdentificador struct {
	Slug      string `json:"slug"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Subdomain string `json:"subdomain"`
}

// Identificador pregunta si un identificador está libre (`GET /onboarding/slug`).
func (a *Alta) Identificador(ctx context.Context, slug string) (AltaIdentificador, error) {
	return JSON[AltaIdentificador](ctx, a.c, Request{Path: "/slug", Query: url.Values{"slug": {slug}}})
}

// CrearOrganizacion la crea —o, si ya existe, la renombra— y deja a quien llama de
// OWNER (`POST /onboarding/organization`).
func (a *Alta) CrearOrganizacion(ctx context.Context, nombre, slug string) (AltaOrg, error) {
	var out AltaOrg
	err := a.enviar(ctx, http.MethodPost, "/organization", map[string]string{"name": nombre, "slug": slug}, &out)
	return out, err
}

// AltaSuscripcion es el contrato de la organización del alta, con el precio por
// asiento que se cobrará (el vigente, con el descuento educativo si lo hay).
type AltaSuscripcion struct {
	Status         string     `json:"status"`
	Seats          int        `json:"seats"`
	ActiveMembers  int        `json:"active_members"`
	UnitPriceCents int        `json:"unit_price_cents"`
	Currency       string     `json:"currency"`
	PaidUntil      *time.Time `json:"paid_until"`
	Entitled       bool       `json:"entitled"`
}

// Suscripcion la lee (`GET /onboarding/subscription`).
func (a *Alta) Suscripcion(ctx context.Context) (AltaSuscripcion, error) {
	return JSON[AltaSuscripcion](ctx, a.c, Request{Path: "/subscription"})
}

// Pagar abre el cobro de esos asientos y devuelve la dirección de la pasarela
// (`POST /onboarding/subscription/checkout`). Se abre en el navegador del sistema.
func (a *Alta) Pagar(ctx context.Context, asientos int) (string, error) {
	var out struct {
		CheckoutURL string `json:"checkout_url"`
	}
	err := a.enviar(ctx, http.MethodPost, "/subscription/checkout", map[string]int{"seats": asientos}, &out)
	return out.CheckoutURL, err
}

// Dominio guarda el dominio propio y comprueba su DNS de verdad
// (`POST /onboarding/domain`). Mismo endpoint para guardarlo y para volver a mirar.
func (a *Alta) Dominio(ctx context.Context, host string) (AltaDominio, error) {
	var out AltaDominio
	err := a.enviar(ctx, http.MethodPost, "/domain", map[string]string{"domain": host}, &out)
	return out, err
}

// AltaCloudflarePeticion configura el dominio en la cuenta de Cloudflare DEL CLIENTE,
// en dos tiempos: `Apply` falso solo mira y devuelve el plan; verdadero lo aplica y solo
// sustituye lo que se haya marcado en `Replace`. El token viaja en el cuerpo y el
// servicio no lo devuelve nunca.
type AltaCloudflarePeticion struct {
	Domain        string   `json:"domain"`
	Token         string   `json:"token"`
	Mail          bool     `json:"mail"`
	Apply         bool     `json:"apply"`
	Replace       []string `json:"replace"`
	RememberToken bool     `json:"remember_token"`
}

// AltaRegistroCloudflare es un registro del plan: `State` es lo que Sytue ha VISTO o
// hecho en la zona (`ok`, `missing`, `conflict`, `created`, `replaced`, `failed`,
// `pending`).
type AltaRegistroCloudflare struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Priority int    `json:"priority"`
	Purpose  string `json:"purpose"`
	State    string `json:"state"`
	Current  string `json:"current"`
	Detail   string `json:"detail"`
}

// AltaCloudflare es la respuesta: la zona, el plan, cuántos se han tocado y qué se ha
// hecho con el token.
type AltaCloudflare struct {
	Zone struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"zone"`
	Records     []AltaRegistroCloudflare `json:"records"`
	Applied     int                      `json:"applied"`
	TokenStored bool                     `json:"token_stored"`
	Domain      AltaDominio              `json:"domain"`
}

// Cloudflare mira o aplica el plan (`POST /onboarding/domain/cloudflare`).
func (a *Alta) Cloudflare(ctx context.Context, p AltaCloudflarePeticion) (AltaCloudflare, error) {
	if p.Replace == nil {
		p.Replace = []string{}
	}
	var out AltaCloudflare
	err := a.enviar(ctx, http.MethodPost, "/domain/cloudflare", p, &out)
	return out, err
}

// AltaDisponibilidad es lo que contesta el registrador por un dominio que aún no se
// tiene: `available`, `taken`, `unsupported`, `unknown`, `unconfigured` o `invalid`.
type AltaDisponibilidad struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
	Price  *struct {
		Currency     string `json:"currency"`
		Registration string `json:"registration"`
		Renewal      string `json:"renewal"`
	} `json:"price"`
}

// Disponibilidad pregunta al registrador (`GET /onboarding/domain-availability`). No
// reserva ni compra: solo pregunta.
func (a *Alta) Disponibilidad(ctx context.Context, dominio string) (AltaDisponibilidad, error) {
	return JSON[AltaDisponibilidad](ctx, a.c, Request{Path: "/domain-availability", Query: url.Values{"domain": {dominio}}})
}

// AltaInvitacion es el resultado REAL de una dirección: `sent`, `already_member`,
// `invalid`, `not_sent` o `failed`, con el motivo ya traducido.
type AltaInvitacion struct {
	Email  string `json:"email"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Invitar invita a varias personas de golpe (`POST /onboarding/invitations`).
func (a *Alta) Invitar(ctx context.Context, correos, rol string) (enviadas int, resultados []AltaInvitacion, err error) {
	var out struct {
		Sent    int              `json:"sent"`
		Results []AltaInvitacion `json:"results"`
	}
	err = a.enviar(ctx, http.MethodPost, "/invitations", map[string]string{"emails": correos, "role": rol}, &out)
	return out.Sent, out.Results, err
}

// CrearRoles crea los roles recomendados elegidos (`POST /onboarding/roles`).
func (a *Alta) CrearRoles(ctx context.Context, claves []string) error {
	return a.enviar(ctx, http.MethodPost, "/roles", map[string][]string{"keys": claves}, nil)
}

// enviar manda un cuerpo JSON y, si se le pide, lee la respuesta.
func (a *Alta) enviar(ctx context.Context, metodo, ruta string, cuerpo, out any) error {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	resp, err := a.c.Do(ctx, Request{
		Method: metodo, Path: ruta, Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
