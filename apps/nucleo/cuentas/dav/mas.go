// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Lo que el cliente hace además de sincronizar: crear un calendario en la cuenta
// (`MKCALENDAR`, RFC 4791 §5.3.1), las consultas con las repeticiones ya expandidas por
// el servidor (§9.6.5, las que usa el servicio de cuentas enlazadas) y leer un objeto
// pidiendo su tipo.

// ErrFormato: el servidor no admite el formato de lo que se le sube (415, o la
// precondición `supported-address-data` / `valid-address-data` de CardDAV y sus
// equivalentes de CalDAV). Una libreta que no quiere vCard 4.0 la quiere en 3.0.
var ErrFormato = errors.New("dav: el servidor no admite ese formato")

// formatoRechazado dice si una respuesta de error es por el formato de lo subido.
func formatoRechazado(estado int, cuerpo []byte) bool {
	if estado == http.StatusUnsupportedMediaType {
		return true
	}
	if estado != http.StatusForbidden && estado != http.StatusConflict && estado != http.StatusBadRequest {
		return false
	}
	c := strings.ToLower(string(cuerpo))
	for _, p := range []string{"supported-address-data", "valid-address-data", "supported-calendar-data"} {
		if strings.Contains(c, p) {
			return true
		}
	}
	return false
}

// CrearCalendario crea un calendario de eventos en la casa de calendarios de la cuenta
// con `MKCALENDAR`, con su nombre y su color («#RRGGBB», o vacío), y devuelve su URL. El
// nombre de la ruta es un UUID: el servidor no tiene que interpretar nada de lo escrito.
func (c *Cliente) CrearCalendario(ctx context.Context, casa, nombre, color string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", errors.New("dav: el calendario necesita un nombre")
	}
	destino := strings.TrimSuffix(casa, "/") + "/" + uuid.NewString() + "/"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><c:mkcalendar xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `" xmlns:ic="` + nsApple + `"><d:set><d:prop>`)
	b.WriteString(`<d:displayname>` + escapar(nombre) + `</d:displayname>`)
	if col := colorCSS(color); col != "" {
		b.WriteString(`<ic:calendar-color>` + col + `FF</ic:calendar-color>`)
	}
	b.WriteString(`<c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set>`)
	b.WriteString(`</d:prop></d:set></c:mkcalendar>`)
	resp, cuerpo, err := c.hacer(ctx, peticion{metodo: "MKCALENDAR", destino: destino, cuerpo: []byte(b.String()), tipo: "application/xml; charset=utf-8"}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errorDe(resp, cuerpo)
	}
	return destino, nil
}

// TodasLasColecciones es Colecciones sin quitar los calendarios que no son de eventos
// (solo tareas): lo que lista el servicio de cuentas enlazadas, que los enseña todos.
func (c *Cliente) TodasLasColecciones(ctx context.Context, casa string, clase Clase) ([]Coleccion, error) {
	return c.colecciones(ctx, casa, clase, false)
}

// rangoICS es un instante como lo escribe CalDAV en time-range y expand.
func rangoICS(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// ConsultarEventos es la `calendar-query` de los eventos que caen entre desde y hasta,
// con sus datos enteros; con expandir, el servidor manda cada repetición del rango como
// su propio VEVENT, con su RECURRENCE-ID (RFC 4791 §9.6.5).
func (c *Cliente) ConsultarEventos(ctx context.Context, coleccion string, desde, hasta time.Time, expandir bool) ([]Objeto, error) {
	datos := `<c:calendar-data/>`
	if expandir {
		datos = `<c:calendar-data><c:expand start="` + rangoICS(desde) + `" end="` + rangoICS(hasta) + `"/></c:calendar-data>`
	}
	cuerpo := `<c:calendar-query xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `"><d:prop><d:getetag/>` + datos + `</d:prop>` +
		`<c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="` + rangoICS(desde) + `" end="` + rangoICS(hasta) + `"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`
	ms, u, err := c.multiestado(ctx, "REPORT", coleccion, "1", cuerpo)
	if err != nil {
		return nil, err
	}
	return c.objetos(ms, u, Calendario), nil
}

// MultigetExpandido trae objetos de un calendario por su URL con sus repeticiones entre
// desde y hasta ya expandidas por el servidor.
func (c *Cliente) MultigetExpandido(ctx context.Context, coleccion string, urls []string, desde, hasta time.Time) ([]Objeto, error) {
	var b strings.Builder
	b.WriteString(`<c:calendar-multiget xmlns:d="DAV:" xmlns:c="` + nsCalDAV + `"><d:prop><d:getetag/><c:calendar-data><c:expand start="` +
		rangoICS(desde) + `" end="` + rangoICS(hasta) + `"/></c:calendar-data></d:prop>`)
	for _, u := range urls {
		p, err := url.Parse(u)
		if err != nil {
			continue
		}
		b.WriteString(`<d:href>` + escapar(p.EscapedPath()) + `</d:href>`)
	}
	b.WriteString(`</c:calendar-multiget>`)
	ms, desdeURL, err := c.multiestado(ctx, "REPORT", coleccion, "1", b.String())
	if err != nil {
		return nil, err
	}
	return c.objetos(ms, desdeURL, Calendario), nil
}

// LeerComo trae un objeto pidiendo su tipo (`Accept: text/calendar` o `text/vcard`) y
// dice el que dio el servidor en Objeto.Tipo.
func (c *Cliente) LeerComo(ctx context.Context, u string, clase Clase) (Objeto, error) {
	acepta := "text/calendar"
	if clase == Libreta {
		acepta = "text/vcard"
	}
	resp, datos, err := c.hacer(ctx, peticion{metodo: http.MethodGet, destino: u, cabeceras: map[string]string{"Accept": acepta}}) //nolint:bodyclose // hacer ya leyó y cerró el cuerpo
	if err != nil {
		return Objeto{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Objeto{}, errorDe(resp, datos)
	}
	return Objeto{URL: u, ETag: resp.Header.Get("ETag"), Datos: datos, Tipo: resp.Header.Get("Content-Type")}, nil
}

// RutaDe es la ruta de una URL de la cuenta («/dav/u/calendario/»), o un error si no es
// de la cuenta.
func (c *Cliente) RutaDe(u string) (string, error) {
	p, err := url.Parse(u)
	if err != nil || !c.permitido(p) {
		return "", fmt.Errorf("dav: %q no es de la cuenta", u)
	}
	return p.Path, nil
}
