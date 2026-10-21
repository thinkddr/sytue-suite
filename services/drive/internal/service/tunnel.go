// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Un NAS en las instalaciones del cliente como almacén de la suite (ADR 0038):
// Drive lo publica hacia sí mismo por el túnel de Sytue Cloud —el proxy de la
// plataforma (Pangolin) y su agente (newt)— sin que el cliente toque el proxy:
// Drive crea el sitio y el recurso por la API de integración y le da al
// cliente un solo comando para su NAS. El WebDAV del NAS queda en
// https://nas-<tenant>.<dominio>, con TLS del proxy y las credenciales del
// propio NAS como única puerta; el sitio solo conoce la dirección local que el
// cliente declaró.

// ErrTunnelOff se devuelve cuando Drive no tiene llave del proxy de la
// plataforma: la clase «NAS» no está.
var ErrTunnelOff = errors.New("service: el túnel de Sytue Cloud no está configurado")

type tunnelClient struct {
	base, key, org, domainID, domain, endpoint string
	http                                       *http.Client
}

func newTunnelClient(base, key, org, domainID, domain, endpoint string) *tunnelClient {
	// Sin dominio de clientes propio no hay túnel: el de la suite no se presta
	// (FILE-09). Apagado, no en error: el resto de Drive arranca igual.
	if strings.TrimSpace(base) == "" || key == "" || strings.TrimSpace(domain) == "" || strings.TrimSpace(domainID) == "" {
		return nil
	}
	return &tunnelClient{
		base: strings.TrimRight(base, "/"), key: key, org: org, domainID: domainID,
		domain: strings.ToLower(domain), endpoint: strings.TrimRight(endpoint, "/"),
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// respuesta es el sobre de la API de integración: {data, success, message}.
type respuesta struct {
	Data    json.RawMessage `json:"data"`
	Success bool            `json:"success"`
	Message string          `json:"message"`
}

type tunnelError struct {
	Status int
	Msg    string
}

func (e *tunnelError) Error() string { return e.Msg }

func esNoEncontrado(err error) bool {
	var te *tunnelError
	return errors.As(err, &te) && te.Status == http.StatusNotFound
}

func (c *tunnelClient) do(ctx context.Context, metodo, ruta string, in, out any) error {
	var body io.Reader = http.NoBody
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.base+ruta, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("túnel: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	var r respuesta
	_ = json.Unmarshal(raw, &r)
	if res.StatusCode < 200 || res.StatusCode > 299 {
		msg := r.Message
		if msg == "" {
			msg = res.Status
		}
		return &tunnelError{Status: res.StatusCode, Msg: fmt.Sprintf("túnel: %s %s: %s", metodo, ruta, msg)}
	}
	if out != nil && len(r.Data) > 0 {
		return json.Unmarshal(r.Data, out)
	}
	return nil
}

// crearSitio pide al proxy los valores de un sitio nuevo y lo crea. Devuelve el
// id del sitio y las credenciales del agente (se enseñan al cliente y se
// guardan selladas para poder volver a enseñarlas).
func (c *tunnelClient) crearSitio(ctx context.Context, nombre string) (siteID int, newtID, newtSecret string, err error) {
	var d struct {
		ExitNodeID int    `json:"exitNodeId"` //nolint:tagliatelle // así lo llama el proxy
		Subnet     string `json:"subnet"`
		NewtID     string `json:"newtId"`     //nolint:tagliatelle // así lo llama el proxy
		NewtSecret string `json:"newtSecret"` //nolint:tagliatelle // así lo llama el proxy
	}
	if err := c.do(ctx, http.MethodGet, "/org/"+c.org+"/pick-site-defaults", nil, &d); err != nil {
		return 0, "", "", err
	}
	var sitio struct {
		SiteID int `json:"siteId"` //nolint:tagliatelle // así lo llama el proxy
	}
	if err := c.do(ctx, http.MethodPut, "/org/"+c.org+"/site", map[string]any{
		"name": nombre, "type": "newt", "exitNodeId": d.ExitNodeID, "subnet": d.Subnet,
		"newtId": d.NewtID, "secret": d.NewtSecret,
	}, &sitio); err != nil {
		return 0, "", "", err
	}
	return sitio.SiteID, d.NewtID, d.NewtSecret, nil
}

// crearRecurso publica <subdominio>.<dominio> hacia la dirección local del NAS a
// través del sitio, sin el SSO del proxy (la puerta es la del NAS).
func (c *tunnelClient) crearRecurso(ctx context.Context, siteID int, subdominio, ip string, puerto int, esquema string) (int, error) {
	var rec struct {
		ResourceID int `json:"resourceId"` //nolint:tagliatelle // así lo llama el proxy
	}
	// El esquema del proxy es ESTRICTO (z.strictObject en 1.22): un campo de más
	// es un 400. El recurso ya no lleva sitio —se ata al sitio por el destino—,
	// y `isBaseDomain` dejó de existir.
	if err := c.do(ctx, http.MethodPut, "/org/"+c.org+"/resource", map[string]any{
		"name": subdominio, "subdomain": subdominio, "domainId": c.domainID,
		"http": true, "protocol": "tcp",
	}, &rec); err != nil {
		return 0, err
	}
	id := fmt.Sprint(rec.ResourceID)
	// Los recursos nacen con el SSO del proxy activado (política por defecto con
	// sso=true) y responderían 401 a Drive.
	if err := c.do(ctx, http.MethodPost, "/resource/"+id, map[string]any{"sso": false}, nil); err != nil {
		return rec.ResourceID, err
	}
	if err := c.do(ctx, http.MethodPut, "/resource/"+id+"/target", map[string]any{
		"ip": ip, "port": puerto, "method": esquema, "enabled": true, "siteId": siteID,
	}, nil); err != nil {
		return rec.ResourceID, err
	}
	return rec.ResourceID, nil
}

// sitioEnLinea dice si el agente del cliente está conectado al proxy.
func (c *tunnelClient) sitioEnLinea(ctx context.Context, siteID int) (bool, error) {
	var s struct {
		Online bool `json:"online"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/site/%d", siteID), nil, &s); err != nil {
		return false, err
	}
	return s.Online, nil
}

// borrar desmonta el recurso y el sitio. Un 404 es «ya no está».
func (c *tunnelClient) borrar(ctx context.Context, siteID, resourceID int) error {
	if resourceID > 0 {
		if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/resource/%d", resourceID), nil, nil); err != nil && !esNoEncontrado(err) {
			return err
		}
	}
	if siteID > 0 {
		if err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/site/%d", siteID), nil, nil); err != nil && !esNoEncontrado(err) {
			return err
		}
	}
	return nil
}

// comandoAgente es lo único que el cliente tiene que ejecutar en su NAS (Docker,
// que traen Synology, QNAP y TrueNAS). --network host para que el agente vea
// la dirección local del NAS tal cual la declaró.
func (c *tunnelClient) comandoAgente(newtID, secret string) string {
	return "docker run -d --name sytue-tunel --restart unless-stopped --network host fosrl/newt" +
		" --id " + newtID + " --secret " + secret + " --endpoint " + c.endpoint
}
