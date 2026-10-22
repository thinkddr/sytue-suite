// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"fmt"
)

// consultaTablasConTenant lista las tablas con columna `tenant` y si tienen política de
// fusión. Se consulta el catálogo en vez de mantener una lista: una tabla nueva
// aparece sola y, si le falta la política, el preflight lo dice.
//
// SE RECONOCE POR LO QUE HACE, NO POR CÓMO SE LLAMA. Antes se exigía el nombre
// exacto `<tabla>_platform_merge`, y eso convertía una convención de nombres en
// un requisito de funcionamiento: `wopi_locks` llevaba desde su primer día una
// política de fusión correcta llamada `wopi_locks_merge`, y el preflight la
// declaraba ausente y abortaba la fusión entera. El nombre no protege nada; lo
// que protege es que exista una política PERMISSIVE que habilite la escritura
// cuando `app.merge` está fijado. Eso es lo que se comprueba.
//
// De paso cierra el agujero contrario, que era el peligroso: una política con el
// nombre canónico pero con la condición equivocada pasaba el preflight, y el
// fallo aparecía como «movidas 0 filas» sin que nada gritara.
//
// COALESCE(with_check, qual): una política sin WITH CHECK explícito aplica su
// USING también a la escritura, y el catálogo la publica como NULL.
//
// Es un chequeo de FORMA, no una prueba semántica: comprueba que la política
// mira `app.merge` y concede sobre todos los comandos, no evalúa la expresión.
// Quien prueba de verdad que una fila se mueve es el test de integración.
//
// TABLAS PARTICIONADAS: se toma la MADRE ('p') y se descartan las particiones
// (relispartition). Antes solo se miraba 'r', y con eso `analytics_events` —la
// única tabla particionada de la suite, una partición por día— entraba justo al
// revés: la madre se ignoraba y se listaban sus particiones diarias una a una.
// Habría movido las mismas filas hoy, pero el preflight exige política POR TABLA
// listada y la partición que `analytics_ensure_partitions` crea cada mañana nace
// sin ella: la fusión habría empezado a abortar sola al día siguiente de
// desplegarse. Actualizar por la madre alcanza a todas sus particiones y solo
// necesita una política.
const consultaTablasConTenant = `
	SELECT c.relname,
	       EXISTS (
	           SELECT 1 FROM pg_policies p
	           WHERE p.schemaname = n.nspname
	             AND p.tablename = c.relname
	             AND p.permissive = 'PERMISSIVE'
	             AND p.cmd = 'ALL'
	             AND p.qual LIKE '%app.merge%'
	             AND COALESCE(p.with_check, p.qual) LIKE '%app.merge%'
	       )
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant' AND a.attnum > 0
	WHERE c.relkind IN ('r', 'p')
	  AND NOT c.relispartition
	  AND n.nspname = current_schema()
	ORDER BY c.relname`

// ConsultaTablasConTenant devuelve las tablas con columna `tenant` del esquema
// actual y, de ellas, las que una fusión de organizaciones NO podría mover por
// faltarles la política. La usan el preflight de la fusión de Sytue ID
// (services/id/internal/merge) y el guardián de cada servicio,
// pgtest.ExigirPoliticaDeFusion: la misma consulta en los dos, para que el test no
// dé por buena una política que el preflight rechazaría.
func ConsultaTablasConTenant(ctx context.Context, q Querier) (tablas, sinPolitica []string, err error) {
	rows, err := q.Query(ctx, consultaTablasConTenant)
	if err != nil {
		return nil, nil, fmt.Errorf("pg: listar tablas con tenant: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var nombre string
		var conPolitica bool
		if err := rows.Scan(&nombre, &conPolitica); err != nil {
			return nil, nil, fmt.Errorf("pg: leer tabla: %w", err)
		}
		tablas = append(tablas, nombre)
		if !conPolitica {
			sinPolitica = append(sinPolitica, nombre)
		}
	}
	return tablas, sinPolitica, rows.Err()
}
