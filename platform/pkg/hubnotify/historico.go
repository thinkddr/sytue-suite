// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Historico es un aviso RECONSTRUIDO de algo que pasó antes de que el registro de
// avisos funcionara: el mismo aviso que el servicio deja en vivo, con la fecha real en
// que ocurrió y la clave del elemento del que sale («mail:<id del mensaje>»). El Hub
// lo guarda ya leído, sin tiempo real ni push, y marcado como anterior al registro;
// con la clave, un elemento da un solo aviso a cada persona aunque se lance otra vez.
type Historico struct {
	Notification
	Clave string    `json:"key"`
	Fecha time.Time `json:"created_at"`
}

// SendHistorico entrega un aviso reconstruido por /internal/notifications/historic y
// devuelve lo que el Hub hizo con él: «creado», «repetido», «silenciado» o
// «posterior» (de cuando el registro ya funcionaba). A diferencia de Send, sin Hub
// es un error: reconstruir sin dónde guardar no puede darse por hecho.
func (c *Client) SendHistorico(ctx context.Context, h Historico) (string, error) {
	if c == nil {
		return "", errors.New("hubnotify: sin HUB_BASE_URL no hay Hub donde reconstruir los avisos")
	}
	if strings.TrimSpace(h.Tenant) == "" || strings.TrimSpace(h.Subject) == "" || strings.TrimSpace(h.Title) == "" ||
		strings.TrimSpace(h.Clave) == "" || h.Fecha.IsZero() {
		return "", fmt.Errorf("hubnotify: tenant, subject, title, clave y fecha son obligatorios")
	}
	if h.Level == "" {
		h.Level = "info"
	}
	h.Title = RecortarTitulo(h.Title)
	var out struct {
		Result string `json:"result"`
	}
	if err := c.postLee(ctx, "/internal/notifications/historic", h, &out); err != nil {
		return "", err
	}
	return out.Result, nil
}

// Reconstruir entrega los avisos uno a uno y escribe en `w` el recuento por tipo y
// resultado, sin nada de su contenido. Sigue aunque falle alguno, y al final devuelve
// error si falló alguno: se puede volver a lanzar, que lo ya hecho sale «repetido».
func Reconstruir(ctx context.Context, c *Client, w io.Writer, avisos []Historico) error {
	cuenta := map[string]int{}
	fallos := 0
	var primero error
	for _, h := range avisos {
		res, err := c.SendHistorico(ctx, h)
		if err != nil {
			fallos++
			if primero == nil {
				primero = err
			}
			res = "error"
		}
		cuenta[h.Type+" "+res]++
	}
	for _, k := range slices.Sorted(maps.Keys(cuenta)) {
		fmt.Fprintf(w, "%s %d\n", k, cuenta[k])
	}
	if fallos > 0 {
		return fmt.Errorf("hubnotify: %d de %d avisos sin reconstruir (el primero: %w)", fallos, len(avisos), primero)
	}
	return nil
}

// CierreReconstruccion es cuándo se cierran las dos puertas que se abrieron solo para
// reconstruir los avisos: la ruta histórica del Hub y el directorio sin sesión de Sytue
// ID. Antes del 2026-10-25 a propósito: la purga borra los leídos a los 30 días, y desde
// ese día podrían faltar los primeros avisos en vivo y las claves de lo ya hecho.
var CierreReconstruccion = time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)

// VentanaHistorica es hasta dónde atrás se reconstruye: lo que la campana habría
// enseñado, y no más. Cubre toda la vida de la suite en producción (desde el
// 2026-09-05) sin copiar al Hub años de mensajes de nadie.
// ponytail: cada fuente lo carga todo en memoria y se filtra aquí; con volumen, que la
// consulta reciba `desde` y lea por páginas.
const VentanaHistorica = 90 * 24 * time.Hour

// ReconstruirOrganizaciones es el comando `avisos-historicos <organización>…` de cada
// servicio: por cada organización, `fuente` saca los avisos de lo que ya existe —con
// el contexto de esa organización, así que la RLS de siempre acota lo que ve— y se
// entregan con Reconstruir. Las organizaciones se dicen a mano, a propósito: el
// servicio no puede listarlas sin saltarse su propia RLS.
func ReconstruirOrganizaciones(ctx context.Context, c *Client, w io.Writer, slugs []string, fuente func(context.Context) ([]Historico, error)) error {
	if len(slugs) == 0 {
		return errors.New("uso: avisos-historicos <organización>…")
	}
	if c == nil {
		return errors.New("hubnotify: sin HUB_BASE_URL no hay Hub donde reconstruir los avisos")
	}
	var fallo error
	desde := time.Now().Add(-VentanaHistorica)
	for _, slug := range slugs {
		t, err := tenancy.New(slug)
		if err != nil {
			fallo = errors.Join(fallo, fmt.Errorf("organización %q: %w", slug, err))
			continue
		}
		// Una fuente puede devolver parte con un error (lo que no dependía de lo que
		// falló): esa parte se entrega igual, y el error queda para el final. Por eso una
		// fuente NUNCA devuelve, junto a un error, elementos que le quedaba por filtrar:
		// lo que devuelve tiene que poder avisarse tal cual.
		todos, err := fuente(tenancy.Into(ctx, t))
		if err != nil {
			fallo = errors.Join(fallo, fmt.Errorf("organización %s: leer lo que ya existe: %w", t.Slug, err))
			if len(todos) == 0 {
				continue
			}
		}
		avisos := todos[:0]
		for _, a := range todos {
			if a.Fecha.After(desde) {
				a.Tenant = t.Slug // lo que sale de una organización se avisa en ella
				avisos = append(avisos, a)
			}
		}
		fmt.Fprintf(w, "== %s: %d avisos\n", t.Slug, len(avisos))
		if err := Reconstruir(ctx, c, w, avisos); err != nil {
			fallo = errors.Join(fallo, err)
		}
	}
	return fallo
}
