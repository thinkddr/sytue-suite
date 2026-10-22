// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package holded es el driver TabularSource para Holded, el ERP y programa de
// facturación más extendido entre las pymes españolas: trae contactos,
// productos y documentos (facturas emitidas y recibidas, pedidos, presupuestos)
// de su API REST como tablas, que es lo que SyERP importa.
//
// Auth: Holded no usa Authorization, lleva la API key en la cabecera `key`. Por
// eso, como Odoo, este driver SÍ recibe su secreto (Instance.Secrets["api_key"])
// y pone la cabecera él. Todo el egress sale por rt.HTTP (SSRF + rate-limit +
// auditoría); el driver nunca construye su propio cliente.
//
// Dos traducciones que solo este driver sabe hacer, porque son semántica de la
// API de Holded y no de quien la consume:
//   - las fechas llegan como segundos Unix y salen como AAAA-MM-DD;
//   - las direcciones llegan anidadas (`billAddress.city`) y salen aplanadas a un
//     nivel, que es lo que una tabla puede llevar.
package holded

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "holded"

const (
	defaultBase = "https://api.holded.com/api/invoicing/v1"
	// ConfigBaseURL permite otra raíz (pruebas). Vacía = la API pública.
	ConfigBaseURL = "base_url"
	secretAPIKey  = "api_key"

	defaultMaxRows = 10000
	maxPages       = 200
	maxRespBytes   = 16 << 20
)

// recursos son las colecciones que se pueden pedir. Lista cerrada: la ruta se
// compone con esto y no con lo que llegue en la spec.
var recursos = map[string]bool{
	"contacts": true, "products": true,
	"documents/invoice": true, "documents/purchase": true, "documents/creditnote": true,
	"documents/salesorder": true, "documents/purchaseorder": true, "documents/estimate": true,
}

// camposFecha son los que Holded da en segundos Unix.
var camposFecha = map[string]bool{"date": true, "dueDate": true, "datePaid": true, "accountingDate": true}

// Driver es el conector de Holded. Sin estado.
type Driver struct{}

// New construye el driver (se cablea en el Registry de la pasarela).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Dataset es sinónimo de Resource: es la clave con la que pregunta Sheets.
type consulta struct {
	Resource string   `json:"resource"`
	Dataset  string   `json:"dataset,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

// Test comprueba la API key pidiendo la primera página de contactos.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	if _, err := pedir(ctx, inst, rt, "contacts", 1); err != nil {
		return fmt.Errorf("holded: test: %w", err)
	}
	return nil
}

// Query trae una colección entera, página a página, hasta el límite.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	var q consulta
	if err := json.Unmarshal(spec, &q); err != nil {
		return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
	}
	if q.Resource == "" {
		q.Resource = q.Dataset
	}
	q.Resource = strings.Trim(strings.TrimSpace(q.Resource), "/")
	if !recursos[q.Resource] {
		return connector.Table{}, fmt.Errorf("%w: recurso de Holded desconocido %q", connector.ErrBadQuery, q.Resource)
	}
	limite := q.Limit
	if limite <= 0 || limite > defaultMaxRows {
		limite = defaultMaxRows
	}
	var filas []map[string]any
	primero := ""
	for pagina := 1; pagina <= maxPages && len(filas) < limite; pagina++ {
		items, err := pedir(ctx, inst, rt, q.Resource, pagina)
		if err != nil {
			return connector.Table{}, err
		}
		if len(items) == 0 {
			break
		}
		// Si una colección no pagina, Holded devuelve lo mismo en cada página:
		// se corta al ver repetido el primer registro, en vez de dar 200 vueltas.
		id := fmt.Sprint(items[0]["id"])
		if pagina == 1 {
			primero = id
		} else if id == primero {
			break
		}
		for _, it := range items {
			filas = append(filas, aplanar(it))
			if len(filas) >= limite {
				break
			}
		}
	}
	return connector.BuildTable(filas, q.Fields, limite), nil
}

func pedir(ctx context.Context, inst connector.Instance, rt connector.Runtime, recurso string, pagina int) ([]map[string]any, error) {
	clave := strings.TrimSpace(inst.Secrets[secretAPIKey])
	if clave == "" {
		return nil, errors.New("holded: falta la API key de la instancia")
	}
	base := strings.TrimRight(strings.TrimSpace(inst.Config[ConfigBaseURL]), "/")
	if base == "" {
		base = defaultBase
	}
	u := base + "/" + recurso + "?" + url.Values{"page": {strconv.Itoa(pagina)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("key", clave)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("holded: %w", err)
	}
	defer resp.Body.Close()
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("holded: leer respuesta: %w", err)
	}
	if len(cuerpo) > maxRespBytes {
		return nil, fmt.Errorf("holded: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("holded: el origen respondió HTTP %d: %s", resp.StatusCode, recortar(string(cuerpo), 200))
	}
	// UseNumber: un importe no puede pasar por float64 camino del libro.
	dec := json.NewDecoder(bytes.NewReader(cuerpo))
	dec.UseNumber()
	var items []map[string]any
	if err := dec.Decode(&items); err != nil {
		return nil, fmt.Errorf("holded: la respuesta no es una lista: %w", err)
	}
	return items, nil
}

// aplanar lleva un registro de Holded a claves planas: los objetos anidados a
// un nivel con punto, las fechas Unix a AAAA-MM-DD y el estado a texto (el
// importador compara estados como texto).
func aplanar(registro map[string]any) map[string]any {
	out := make(map[string]any, len(registro))
	for k, v := range registro {
		switch val := v.(type) {
		case map[string]any:
			for k2, v2 := range val {
				out[k+"."+k2] = v2
			}
		case json.Number:
			switch {
			case camposFecha[k]:
				if n, err := val.Int64(); err == nil && n > 0 {
					out[k] = time.Unix(n, 0).UTC().Format("2006-01-02")
				}
			case k == "status":
				out[k] = val.String()
			default:
				out[k] = val
			}
		default:
			out[k] = v
		}
	}
	return out
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
