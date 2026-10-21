// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// LookupIPs resuelve varias direcciones a su ubicación aproximada de una sola
// consulta.
//
// En plural a propósito: la lista de accesos puede traer decenas de direcciones
// y una consulta por fila convertiría una pantalla en una ráfaga de idas y
// venidas a la base de datos. El coste de resolver diez o cien juntas es el
// mismo viaje.
//
// Ante CUALQUIER problema devuelve un mapa vacío en vez de error: la ubicación
// es un adorno informativo sobre datos que ya se muestran (IP y dispositivo), y
// que falte no justifica romper la página de la cuenta. El caso más común es
// justamente el benigno —la tabla existe pero está vacía porque nadie ha cargado
// la base de rangos—, y ahí no hay nada que reportar.
func (s *Store) LookupIPs(ctx context.Context, ips []string) map[string]domain.Ubicacion {
	out := make(map[string]domain.Ubicacion)
	limpias := make([]string, 0, len(ips))
	vistas := make(map[string]bool, len(ips))
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		// Se filtra aquí y no en SQL porque un valor que no sea una inet válida
		// aborta la consulta ENTERA con un error de tipo, llevándose por delante
		// la ubicación de todas las demás direcciones.
		if ip == "" || vistas[ip] || domain.NormalizeIP(ip) != ip {
			continue
		}
		vistas[ip] = true
		limpias = append(limpias, ip)
	}
	if len(limpias) == 0 {
		return out
	}

	// Por cada dirección se busca EL ÚLTIMO rango que empieza antes o justo en
	// ella, y después se comprueba que también termina después: los rangos no se
	// solapan, así que ese es el único candidato posible. El LATERAL con LIMIT 1
	// permite al planificador resolver cada dirección con un salto al índice en
	// lugar de cruzar la tabla entera.
	const sql = `SELECT host(d.ip), g.country, g.city
		FROM unnest($1::inet[]) AS d(ip)
		CROSS JOIN LATERAL (
			SELECT country, city, ip_end
			FROM geoip_networks
			WHERE ip_start <= d.ip
			ORDER BY ip_start DESC
			LIMIT 1
		) g
		WHERE d.ip <= g.ip_end`
	rows, err := s.DB().Query(ctx, sql, limpias)
	if err != nil {
		// Incluye el caso de que la migración no esté aplicada todavía.
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ip, pais, ciudad string
		if err := rows.Scan(&ip, &pais, &ciudad); err != nil {
			return out
		}
		out[ip] = domain.Ubicacion{Pais: pais, Ciudad: ciudad}
	}
	if rows.Err() != nil {
		return map[string]domain.Ubicacion{}
	}
	return out
}
