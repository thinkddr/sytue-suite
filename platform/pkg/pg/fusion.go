// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
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
// A Scope-bound table instead declares the complete restrictive guard,
// CHECK and read-only source/target preflight policy. It remains in the
// inventory, but is never added to Movable. Partial declarations fail closed.
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
	       ),
        c.relrowsecurity AND c.relforcerowsecurity AND EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conrelid=c.oid AND k.conname='scope_bound_merge_contract' AND pg_get_constraintdef(k.oid)='CHECK (sytue_scope_merge_allowed())') AND EXISTS (
         SELECT 1 FROM pg_policies p WHERE p.schemaname=n.nspname AND p.tablename=c.relname
         AND p.permissive='RESTRICTIVE' AND p.cmd='ALL'
         AND p.qual='sytue_scope_merge_allowed()' AND p.with_check='sytue_scope_merge_allowed()'
        ) AND EXISTS (
         SELECT 1 FROM pg_policies p WHERE p.schemaname=n.nspname AND p.tablename=c.relname
         AND p.permissive='PERMISSIVE' AND p.cmd='SELECT' AND p.qual='sytue_scope_merge_preflight(tenant)'
        ),
        EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc f ON f.oid=t.tgfoid
         WHERE t.tgrelid=c.oid AND NOT t.tgisinternal
         AND (f.proname='sytue_private_storage_state_scope_guard' OR (f.proname='sytue_scope_admission_data_guard' AND t.tgargs=convert_to('state','UTF8')||decode('00','hex')))),
        EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=c.oid AND k.conname='scope_bound_merge_contract')
        OR EXISTS(SELECT 1 FROM pg_policies p WHERE p.schemaname=n.nspname AND p.tablename=c.relname
        AND (p.qual LIKE '%sytue_scope_merge_%' OR p.with_check LIKE '%sytue_scope_merge_%'))
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant' AND a.attnum > 0
	WHERE c.relkind IN ('r', 'p')
	  AND NOT c.relispartition
	  AND n.nspname = current_schema()
	ORDER BY c.relname`

// ConsultaTablasConTenant devuelve las tablas con columna `tenant` del esquema
// actual y los contratos ausentes. Un bloqueo explícito Scope-bound también
// satisface el inventario; nunca concede permiso para mover esa tabla. Los
// escritores deben usar ConsultaInventarioDeFusion.Movable. La usan el preflight de la fusión de Sytue ID
// (services/id/internal/merge) y el guardián de cada servicio,
// pgtest.ExigirPoliticaDeFusion: la misma consulta en los dos, para que el test no
// dé por buena una política que el preflight rechazaría.
func ConsultaTablasConTenant(ctx context.Context, q Querier) (tablas, sinPolitica []string, err error) {
	inventory, err := ConsultaInventarioDeFusion(ctx, q)
	return inventory.Tables, inventory.Missing, err
}
