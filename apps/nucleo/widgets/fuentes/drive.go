// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Drive lee lo que hay en la RAÍZ de `datos` —el Drive privado que la app tiene abierto
// (api.Drive.WithPrivateData), aunque esté en una subcarpeta— y la cuota de la cuenta en
// `cuenta`, el cliente del servidor (los controles de la cuenta no son datos privados).
// Con almacén externo (que la cuota de Sytue no mide) o una cuota que no es para esta
// persona (403/404) no hay porcentaje; si la cuota o la raíz fallan por otra cosa, no se
// publica y se conserva lo anterior. La monta la app con Privada: la registrada no lee el
// servidor, congelado (privada.go). Peticiones: 1 de cuota y 2 lecturas locales.
func Drive(c Comunes, datos, cuenta *api.Drive) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		raiz, err := datos.Root(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		hijos, err := datos.Children(ctx, raiz.ID, "")
		if err != nil {
			return widgets.Detail{}, err
		}
		q, err := cuenta.Quota(ctx)
		if err := secundaria(err); err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		if err == nil && !q.ExternalStorage {
			if contador, valido := widgets.StoragePercent(q.UsedBytes, q.LimitBytes); valido {
				d.Counters = []widgets.Counter{contador}
			}
		}
		d.Items = itemsDeRaiz(c.idioma(), hijos)
		return d, nil
	}
}

// itemsDeRaiz: primero las carpetas y luego los ficheros, en el orden del servicio.
func itemsDeRaiz(l i18n.Locale, hijos api.Children) []widgets.Item {
	items := make([]widgets.Item, 0, widgets.MaxItems)
	for _, f := range hijos.Folders {
		if llenos(items) {
			break
		}
		if nombre := strings.TrimSpace(f.Name); nombre != "" {
			items = append(items, widgets.Item{Title: nombre, Badge: "DIR", StartAt: unix(f.UpdatedAt), Route: RutaDrive(f.Name, true)})
		}
	}
	for _, f := range hijos.Files {
		if llenos(items) {
			break
		}
		nombre := strings.TrimSpace(f.Name)
		if nombre == "" {
			continue
		}
		insignia := ""
		if f.SizeBytes >= 0 {
			insignia = i18n.FormatBytes(l, float64(f.SizeBytes))
		}
		items = append(items, widgets.Item{
			Title: nombre, Subtitle: f.ContentType, Badge: insignia, StartAt: unix(f.UpdatedAt), Route: RutaDrive(f.Name, false),
		})
	}
	return widgets.SanitizeItems(items)
}
