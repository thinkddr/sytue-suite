// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package web comprueba en internet lo que Sytue AI propone antes de guardarlo:
// que un NIF-IVA europeo existe (y de quién es), que la página de facturas de un
// proveedor responde y que el dominio de un correo recibe correo.
//
// POR QUÉ NO SE LE PREGUNTA AL MODELO Y YA. Un modelo se sabe de memoria la web de
// Anthropic, pero se inventa con toda tranquilidad un NIF-IVA o un
// «facturacion@empresa.es» que suena bien. Un dato fiscal falso en la ficha de un
// proveedor acaba en el 347 o en el 349; una dirección que no existe, en un
// correo que nadie recibe. Lo que se puede comprobar, se comprueba.
//
// Lo que sale y adónde (privacidad): el NIF-IVA de un proveedor a la Comisión
// Europea (VIES, servicio público de la UE), una petición HEAD a la página que
// sugiere el modelo y una consulta DNS por el dominio del correo. Ningún dato de
// la organización ni de sus movimientos.
package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Comprobador hace las comprobaciones. El cliente HTTP es el de la barrera SSRF:
// las URL las elige un modelo, y un modelo también puede escribir «http://10.0.0.1».
type Comprobador struct {
	hc    *http.Client
	vies  string
	mx    func(ctx context.Context, dominio string) ([]*net.MX, error)
	hosts func(ctx context.Context, host string) ([]string, error)
}

// Nuevo construye el comprobador de producción.
func Nuevo() *Comprobador {
	return &Comprobador{
		hc:    ssrfhttp.NewClient(8*time.Second, 3),
		vies:  "https://ec.europa.eu/taxation_customs/vies/rest-api",
		mx:    net.DefaultResolver.LookupMX,
		hosts: net.DefaultResolver.LookupHost,
	}
}

// paisesIVA son los prefijos del VIES (Grecia es EL; XI, Irlanda del Norte).
var paisesIVA = []string{
	"AT", "BE", "BG", "CY", "CZ", "DE", "DK", "EE", "EL", "ES", "FI", "FR", "HR", "HU", "IE",
	"IT", "LT", "LU", "LV", "MT", "NL", "PL", "PT", "RO", "SE", "SI", "SK", "XI",
}

var nifIVARe = regexp.MustCompile(`^([A-Z]{2})([A-Z0-9]{2,12})$`)

// PartirNIFIVA separa país y número de un NIF-IVA europeo, ya normalizado. ok es
// false si no tiene esa forma.
func PartirNIFIVA(nif string) (pais, numero string, ok bool) {
	limpio := strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "").Replace(strings.TrimSpace(nif)))
	m := nifIVARe.FindStringSubmatch(limpio)
	if m == nil {
		return "", "", false
	}
	pais = m[1]
	if pais == "GR" {
		pais = "EL"
	}
	return pais, m[2], slices.Contains(paisesIVA, pais)
}

// IVA es lo que dice el VIES de un NIF-IVA.
type IVA struct {
	// Comprobado: el VIES contestó con un sí o un no. Si el país no responde, false.
	Comprobado bool   `json:"comprobado"`
	Valido     bool   `json:"valido"`
	Nombre     string `json:"nombre,omitempty"`
	Direccion  string `json:"direccion,omitempty"`
}

// IVA consulta el VIES. Un NIF español no se consulta: el VIES solo conoce a quien
// está en el registro de operadores intracomunitarios, y un NIF perfectamente
// válido de una pyme española saldría «no válido».
func (c *Comprobador) IVA(ctx context.Context, nif string) IVA {
	pais, numero, ok := PartirNIFIVA(nif)
	if !ok || pais == "ES" {
		return IVA{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.vies+"/ms/"+pais+"/vat/"+url.PathEscape(numero), http.NoBody)
	if err != nil {
		return IVA{}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return IVA{}
	}
	defer resp.Body.Close()
	// Las claves son las del VIES, en camelCase: no son de esta API.
	var r struct {
		IsValid   bool   `json:"isValid"`   //nolint:tagliatelle // así responde el VIES
		UserError string `json:"userError"` //nolint:tagliatelle // así responde el VIES
		Name      string `json:"name"`
		Address   string `json:"address"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&r) != nil {
		return IVA{}
	}
	// Solo VALID e INVALID son respuestas; MS_UNAVAILABLE y compañía son «no sé».
	if r.UserError != "VALID" && r.UserError != "INVALID" {
		return IVA{}
	}
	out := IVA{Comprobado: true, Valido: r.IsValid}
	if out.Valido {
		out.Nombre, out.Direccion = sinGuiones(r.Name), sinGuiones(r.Address)
	}
	return out
}

// sinGuiones: el VIES devuelve «---» cuando el país no comparte el dato.
func sinGuiones(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if strings.Trim(s, "-") == "" {
		return ""
	}
	return s
}

// URL dice si una dirección https responde. Una página de facturas pide iniciar
// sesión, así que 401 y 403 valen; lo que no vale es que no exista (404, 410) o
// que no resuelva.
func (c *Comprobador) URL(ctx context.Context, direccion string) bool {
	u, err := url.Parse(strings.TrimSpace(direccion))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	for _, metodo := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequestWithContext(ctx, metodo, u.String(), http.NoBody)
		if err != nil {
			return false
		}
		req.Header.Set("User-Agent", "SytueERP/1.0 (+https://sytue.eu)")
		resp, err := c.hc.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusMethodNotAllowed, http.StatusNotImplemented:
			continue // hay servidores que no admiten HEAD: se prueba con GET
		case http.StatusNotFound, http.StatusGone:
			return false
		default:
			return true
		}
	}
	return false
}

// Correo dice si una dirección está bien escrita y su dominio recibe correo.
func (c *Comprobador) Correo(ctx context.Context, direccion string) bool {
	a, err := mail.ParseAddress(strings.TrimSpace(direccion))
	if err != nil || a.Name != "" {
		return false
	}
	_, dominio, ok := strings.Cut(a.Address, "@")
	if !ok || !strings.Contains(dominio, ".") {
		return false
	}
	if mx, err := c.mx(ctx, dominio); err == nil && len(mx) > 0 {
		return true
	}
	// Sin MX, el correo va al propio dominio (RFC 5321 §5.1).
	ips, err := c.hosts(ctx, dominio)
	return err == nil && len(ips) > 0
}
