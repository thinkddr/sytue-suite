// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package fuentes son las lecturas de los widgets nativos de las quince apps: una
// Fuente por producto, que pide lo justo, sin depender de la pantalla que se esté
// viendo, y arma el widgets.Detail con un constructor puro (ADR 0078, enmienda del
// 2026-11-06).
//
// No importa Fyne a propósito (lo fija TestLasFuentesNoImportanFyne): la misma Fuente
// tiene que poder correr en un despertar en segundo plano, sin ventana ni interfaz.
// Lo que antes vivía en cada pantalla (`eventosWidget`, `itemsDriveParaWidget`…) se
// ha mudado aquí; las pantallas ya no publican.
package fuentes

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Comunes es lo que toda Fuente puede necesitar de la app, sin Fyne. Todo es
// opcional: sin T sale la clave, sin Zona la local, sin Ahora time.Now.
type Comunes struct {
	// T traduce una clave del catálogo de la app (p. ej. «Sin título»).
	T func(clave string) string
	// Zona es la zona de visualización.
	Zona func() *time.Location
	// Ahora es la hora.
	Ahora func() time.Time
	// Idioma da formato a números y tamaños.
	Idioma func() i18n.Locale
}

func (c Comunes) t(clave string) string {
	if c.T == nil {
		return clave
	}
	return c.T(clave)
}

func (c Comunes) zona() *time.Location {
	if c.Zona == nil {
		return time.Local
	}
	if z := c.Zona(); z != nil {
		return z
	}
	return time.Local
}

func (c Comunes) ahora() time.Time {
	if c.Ahora == nil {
		return time.Now().In(c.zona())
	}
	return c.Ahora().In(c.zona())
}

func (c Comunes) idioma() i18n.Locale {
	if c.Idioma == nil {
		return i18n.Default
	}
	return c.Idioma()
}

// unix es el instante para el widget, o 0 si no hay o es anterior a 1970.
func unix(t time.Time) int64 {
	if t.IsZero() || t.Unix() <= 0 {
		return 0
	}
	return t.Unix()
}

// sinAcceso: el servicio dice que ese recurso ya no es para esta persona.
func sinAcceso(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && (e.Status == http.StatusForbidden || e.Status == http.StatusNotFound)
}

// secundaria es el error de una lectura que aporta filas o contadores al widget (la
// cuota, los directos, los álbumes) que no es la principal. Un 403/404 es que ese dato
// no es para esta persona o el servicio no lo tiene: se publica sin él (nil). Otro
// error —la red, un 5xx, un 401— se devuelve, y el Publicador conserva lo publicado:
// publicar con éxito un parcial borraría del widget, p. ej., un directo en curso.
func secundaria(err error) error {
	if err == nil || sinAcceso(err) {
		return nil
	}
	return err
}

// llenos corta los items crudos a MaxItems antes de sanearlos.
func llenos(items []widgets.Item) bool { return len(items) >= widgets.MaxItems }

func primero(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// todoFalla es el error de una Fuente cuyas lecturas fallaron todas: se conserva lo
// publicado y se reintenta con el siguiente aviso.
func todoFalla(errs ...error) error {
	for _, err := range errs {
		if err == nil {
			return nil
		}
	}
	return errors.Join(errs...)
}
