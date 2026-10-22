// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/pages/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia de Sytue Web que lee o modifica una tabla
// con `tenant` lo nombra en su predicado. La política RLS es la segunda barrera, no la
// única: un rol que la elude operaba la tienda de cualquier sitio con solo su UUID
// (WEB-14). Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{".", "../http", "../tienda", "../ia", "../dominios", "../mapa", "../sitios", "../publicar"},
		Excepciones: map[string]string{
			// El mapa host → sitio es de TODA la plataforma: lo regenera la API en transacción de
			// mantenimiento y solo publica hosts, slugs y números de versión.
			"dominios.go:leerSitiosParaMapa":       "el mapa de hosts recorre los sitios de todas las organizaciones",
			"dominios.go:leerPreviewsParaMapa":     "el mapa de hosts recorre las vistas previas vivas de todas las organizaciones",
			"dominios.go:leerDominiosParaMapa":     "el mapa de hosts recorre los dominios verificados de todas las organizaciones",
			"dominios.go:leerRetiredSlugsParaMapa": "el mapa de hosts recorre los slugs retirados de todas las organizaciones",
			// Un host es único en la plataforma, no en una organización.
			"dominios.go:DominioEnUso":        "un host ya verificado por OTRA organización no se puede dar de alta: hay que mirar todas",
			"dominios.go:EsDominioVerificado": "el ask de Caddy pregunta por un host sin saber de quién es",
			// Barridos en segundo plano, en transacción de mantenimiento.
			"dominios.go:ObtenerDominiosPendientesBarrido":   "el barrido reintenta los dominios pendientes de todas las organizaciones",
			"dominios.go:ObtenerDominiosActivosRevalidacion": "la revalidación diaria recorre los dominios activos de todas las organizaciones",
			"ia.go:BarrerTrabajosVencidos":                   "el barrido cierra los trabajos de IA vencidos de todas las organizaciones",
			"tienda.go:ObtenerPedidosVencidos":               "el barrido libera las reservas vencidas de todas las organizaciones",
			// El slug de un sitio es su dirección: único en la plataforma.
			"sitios.go:slugOcupado": "la disponibilidad de un slug se mira contra los sitios de todas las organizaciones, también los borrados",
			// Entradas sin sesión ni Host de una web, y la CLI de operación.
			"dominios.go:ResolverHostPublico": "el Host de una web, su dominio o su <slug>.sites, llega sin organización: es la consulta que la averigua",
			"store.go:TenantDeSitio":          "un webhook llega con el id del sitio y sin organización: es la consulta que la averigua",
			"store.go:cambiarEstado":          "la CLI de operación suspende un sitio por su slug, que es único en la plataforma",
		},
	})
}
