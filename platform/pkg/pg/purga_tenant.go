// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// PURGA DE UN TENANT: el borrado de todo lo que una base guarda de una
// organización que desaparece (ADR 0054, enmienda del 2026-11-25).
//
// Es la otra cara de la fusión (services/id/internal/merge) y va por el mismo
// camino: Sytue ID abre cada base con sus credenciales, dentro de InMergeTx, y
// recorre el CATÁLOGO —no una lista— para que una tabla nueva entre sola. Lo
// que cambia es el verbo: donde la fusión reescribe `tenant`, aquí se borra.
//
// TRES REGLAS que gobiernan lo que se borra y lo que no:
//
//  1. Ningún guardián se salta. Un disparador que protege la integridad —el
//     libro diario inmutable del ERP, los registros VeriFactu, la fuente
//     congelada del almacenamiento privado— hace fallar el DELETE de su tabla;
//     esa tabla se queda ENTERA (el borrado va en un SAVEPOINT) y se informa
//     como retenida con su motivo. El resto de la base se borra igual. Quien
//     decide sobre lo retenido es una persona, no este bucle.
//  2. Una retención legal activa (`legal_holds`) para el tenant detiene la purga
//     de esa base entera antes de tocar nada: preservar para un litigio es
//     justamente no borrar.
//  3. Cada base puede declarar un gancho, PurgePrepareFunc, para lo que un
//     DELETE por tenant no sabe hacer (la contabilidad de objetos compartidos
//     entre organizaciones de Mail). Lo escribe quien conoce esos datos, en una
//     migración suya, igual que el gancho de la fusión.
//
// IDEMPOTENTE: el criterio es `tenant = $1`, así que repetirla tras un éxito
// borra cero filas. Y CONVERGENTE: el orden de las tablas no se conoce de
// antemano (claves foráneas sin cascada, disparadores que encolan filas al
// borrar otras), así que se repiten pasadas hasta que no queda nada o una
// pasada no avanza.

// PurgePrepareFunc es el gancho OPCIONAL de una base. Recibe el slug y devuelve
// una frase para el operador (o NULL). Si decide que la base no se puede purgar
// todavía, lanza una excepción con el SQLSTATE CodigoPurgaRetenida.
const PurgePrepareFunc = "platform_tenant_purge_prepare"

// CodigoPurgaRetenida es el SQLSTATE con el que un gancho dice «esto no se borra
// todavía» (una retención que no es un error de nadie).
const CodigoPurgaRetenida = "PTP01"

const (
	purgePrepareExistsSQL = `SELECT to_regprocedure('` + PurgePrepareFunc + `(text)') IS NOT NULL`
	purgePrepareCallSQL   = `SELECT ` + PurgePrepareFunc + `($1)`
	// pasadasMaxPurga acota las vueltas. Cada una borra al menos una fila o se
	// para; la cadena de claves foráneas más larga de la suite tiene cuatro saltos.
	pasadasMaxPurga = 8
)

//go:embed tenant_purge_v1.sql
var tenantPurgeV1 []byte

// MarcadorPurgaDeTenant es la línea que una migración escribe para recibir la
// cola del borrado de objetos (tenant_purge_v1.sql).
const MarcadorPurgaDeTenant = "-- sytue:tenant-purge-v1"

// WithTenantPurgeDDL expande MarcadorPurgaDeTenant en las migraciones de un
// servicio que guarda objetos. Solo lo necesita quien tiene almacén: una base
// sin objetos se purga igual, sin cola.
func WithTenantPurgeDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, migrationDDLFragment{marker: MarcadorPurgaDeTenant, sql: tenantPurgeV1})
}

// ErrPurgaRetenida es una purga que no se hizo por una retención: legal, o
// decidida por el gancho de la base. No es un fallo: se reintenta más tarde.
var ErrPurgaRetenida = errors.New("pg: purga de tenant retenida")

// PurgaRetenidaError lleva el motivo de la retención, en prosa para el operador.
type PurgaRetenidaError struct{ Motivo string }

func (e *PurgaRetenidaError) Error() string { return "purga retenida: " + e.Motivo }

// Is permite errors.Is(err, ErrPurgaRetenida).
func (e *PurgaRetenidaError) Is(target error) bool { return target == ErrPurgaRetenida }

// TablaRetenida es una tabla cuyas filas del tenant siguen ahí tras la purga.
type TablaRetenida struct {
	Tabla  string `json:"table"`
	Filas  int    `json:"rows"`
	Motivo string `json:"reason"`
}

// ResultadoPurga es lo que una purga hizo en UNA base.
type ResultadoPurga struct {
	Filas map[string]int `json:"rows,omitempty"`
	Total int            `json:"total"`
	// Retenidas son las tablas que conservan filas del tenant, con su motivo.
	// Vacía = la base ya no guarda nada del tenant.
	Retenidas []TablaRetenida `json:"retained,omitempty"`
	// Preparado es lo que dijo el gancho de la base, si lo hay.
	Preparado string `json:"prepared,omitempty"`
	// ObjetosEncolados dice si la base tiene cola de objetos y se dejó la orden.
	ObjetosEncolados bool `json:"objects_queued,omitempty"`
}

// Completa dice si la base ya no guarda ninguna fila del tenant.
func (r ResultadoPurga) Completa() bool { return len(r.Retenidas) == 0 }

// PurgarTenant borra de UNA base todas las filas del tenant `slug`, en una sola
// transacción. Ver el encabezado del fichero para lo que no borra.
//
// Va con `app.merge` Y `app.tenant` fijados: las políticas permisivas se suman,
// así que una tabla se alcanza tanto por su política de fusión como por la de
// aislamiento. Las tablas de ámbito ligado (scope-bound) se borran con la fusión
// apagada: su política restrictiva oculta todo cuando `app.merge` está activo,
// y un DELETE que no ve filas «terminaría» sin borrar nada.
//
// `contratos` se ejecutan los primeros dentro de la transacción: son los que una
// base exige para escribir y que este paquete no puede conocer sin importar el
// servicio (el contrato de escritura de Notepads, notesdb.MarkWriteTransaction,
// que la fusión adopta igual).
func PurgarTenant(ctx context.Context, db Database, slug string, contratos ...func(context.Context, pgx.Tx) error) (ResultadoPurga, error) {
	res := ResultadoPurga{Filas: map[string]int{}}
	if !tenancy.ValidSlug(slug) {
		return res, fmt.Errorf("pg: slug de tenant inválido para purgar: %q", slug)
	}
	err := db.InMergeTx(ctx, func(tx pgx.Tx) error {
		for _, c := range contratos {
			if err := c(ctx, tx); err != nil {
				return err
			}
		}
		if err := SetTenant(ctx, tx, slug); err != nil {
			return err
		}
		if err := comprobarRetencionLegal(ctx, tx, slug); err != nil {
			return err
		}
		nota, err := prepararPurga(ctx, tx, slug)
		if err != nil {
			return err
		}
		res.Preparado = nota
		tablas, err := tablasDelTenant(ctx, tx)
		if err != nil {
			return err
		}
		if err := purgarTablas(ctx, tx, slug, tablas, &res); err != nil {
			return err
		}
		if res.Completa() {
			encolado, err := encolarPurgaDeObjetos(ctx, tx, slug, res.Total > 0)
			if err != nil {
				return err
			}
			res.ObjetosEncolados = encolado
		}
		return nil
	})
	if err != nil {
		var pge *pgconn.PgError
		if errors.As(err, &pge) && pge.Code == CodigoPurgaRetenida {
			return res, &PurgaRetenidaError{Motivo: pge.Message}
		}
		return res, err
	}
	sort.Slice(res.Retenidas, func(i, j int) bool { return res.Retenidas[i].Tabla < res.Retenidas[j].Tabla })
	return res, nil
}

// tablaPurga es una tabla con filas de organización: su columna de tenant
// (`tenant`, o `org_id` en Sytue Integrations, que guarda el mismo slug) y si es
// de ámbito ligado.
type tablaPurga struct {
	nombre, columna string
	ligada          bool
}

// tablasDelTenant junta el inventario de la fusión (las tablas con `tenant`) con
// las que llevan el slug en `org_id`. Las dos columnas son las que reconoce
// pgtest.ExigirRLSForzada como «tabla por organización»; mirar solo una dejaría
// la otra mitad sin borrar.
func tablasDelTenant(ctx context.Context, tx pgx.Tx) ([]tablaPurga, error) {
	inventario, err := ConsultaInventarioDeFusion(ctx, tx)
	if err != nil {
		return nil, err
	}
	ligadas := map[string]bool{}
	for _, t := range inventario.ScopeBound {
		ligadas[t] = true
	}
	out := make([]tablaPurga, 0, len(inventario.Tables))
	for _, t := range inventario.Tables {
		out = append(out, tablaPurga{nombre: t, columna: "tenant", ligada: ligadas[t]})
	}
	rows, err := tx.Query(ctx, `
		SELECT c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'org_id' AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition AND n.nspname = current_schema()
		  AND a.atttypid = 'text'::regtype
		  AND NOT EXISTS (SELECT 1 FROM pg_attribute t WHERE t.attrelid = c.oid AND t.attname = 'tenant' AND NOT t.attisdropped)
		ORDER BY c.relname`)
	if err != nil {
		return nil, fmt.Errorf("pg: listar tablas por org_id: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, tablaPurga{nombre: t, columna: "org_id"})
	}
	return out, rows.Err()
}

// purgarTablas repite pasadas sobre las tablas con filas del tenant hasta que no
// queda ninguna o una pasada no borra nada.
func purgarTablas(ctx context.Context, tx pgx.Tx, slug string, tablas []tablaPurga, res *ResultadoPurga) error {
	retenidas := map[string]string{}
	pendientes := append([]tablaPurga(nil), tablas...)
	for pasada := 0; len(pendientes) > 0 && pasada < pasadasMaxPurga; pasada++ {
		avance, fallos, err := pasadaDePurga(ctx, tx, slug, pendientes, retenidas, res)
		if err != nil {
			return err
		}
		// Lo que sigue teniendo filas (una FK, o un disparador que encoló algo al
		// borrar otra tabla) va a la siguiente pasada; en la última, o si esta no
		// avanzó, queda retenido.
		ultima := avance == 0 || pasada == pasadasMaxPurga-1
		if pendientes, err = conFilasDelTenant(ctx, tx, slug, tablas, retenidas, fallos, ultima); err != nil {
			return err
		}
		if avance == 0 {
			break
		}
	}
	return anotarRetenidas(ctx, tx, slug, tablas, retenidas, res)
}

// pasadaDePurga intenta borrar cada tabla pendiente una vez. Devuelve cuántas
// filas borró y las tablas que una clave foránea frenó, con su mensaje.
func pasadaDePurga(ctx context.Context, tx pgx.Tx, slug string, pendientes []tablaPurga, retenidas map[string]string, res *ResultadoPurga) (int, map[string]string, error) {
	avance := 0
	fallos := map[string]string{}
	for _, t := range pendientes {
		n, err := borrarTablaDelTenant(ctx, tx, t, slug)
		if err != nil {
			if ctx.Err() != nil {
				return 0, nil, ctx.Err()
			}
			var pge *pgconn.PgError
			if !errors.As(err, &pge) {
				return 0, nil, fmt.Errorf("pg: purgar %s: %w", t.nombre, err)
			}
			// Una clave foránea se resuelve sola en otra pasada, cuando se haya
			// borrado la hija; cualquier otro rechazo es un guardián.
			if pge.Code == "23503" {
				fallos[t.nombre] = pge.Message
			} else {
				retenidas[t.nombre] = motivoDeRetencion(pge)
			}
			continue
		}
		if n > 0 {
			res.Filas[t.nombre] += n
			res.Total += n
			avance += n
		}
	}
	return avance, fallos, nil
}

// conFilasDelTenant devuelve las tablas no retenidas que aún tienen filas del
// tenant. Con `ultima`, además las da por retenidas con su motivo.
func conFilasDelTenant(ctx context.Context, tx pgx.Tx, slug string, tablas []tablaPurga, retenidas, fallos map[string]string, ultima bool) ([]tablaPurga, error) {
	var pendientes []tablaPurga
	for _, t := range tablas {
		if _, ret := retenidas[t.nombre]; ret {
			continue
		}
		n, err := contarDelTenant(ctx, tx, t, slug)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		pendientes = append(pendientes, t)
		if ultima {
			motivo := fallos[t.nombre]
			if motivo == "" {
				motivo = "las filas siguen visibles tras borrarlas (política o disparador)"
			}
			retenidas[t.nombre] = motivo
		}
	}
	return pendientes, nil
}

// anotarRetenidas escribe en el resultado las tablas retenidas que de verdad
// conservan filas, con cuántas.
func anotarRetenidas(ctx context.Context, tx pgx.Tx, slug string, tablas []tablaPurga, retenidas map[string]string, res *ResultadoPurga) error {
	for _, t := range tablas {
		motivo, ret := retenidas[t.nombre]
		if !ret {
			continue
		}
		n, err := contarDelTenant(ctx, tx, t, slug)
		if err != nil {
			return err
		}
		if n > 0 {
			res.Retenidas = append(res.Retenidas, TablaRetenida{Tabla: t.nombre, Filas: n, Motivo: motivo})
		}
	}
	return nil
}

// borrarTablaDelTenant borra las filas del tenant de UNA tabla dentro de un
// SAVEPOINT: si un guardián la rechaza, se deshace solo lo de esa tabla.
func borrarTablaDelTenant(ctx context.Context, tx pgx.Tx, t tablaPurga, slug string) (int, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return 0, err
	}
	n, err := conModo(ctx, sp, slug, t.ligada, func() (int, error) {
		// Los nombres vienen del catálogo y se citan; el tenant va parametrizado.
		tag, err := sp.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = $1",
			pgx.Identifier{t.nombre}.Sanitize(), pgx.Identifier{t.columna}.Sanitize()), slug)
		return int(tag.RowsAffected()), err
	})
	if err != nil {
		_ = sp.Rollback(ctx)
		return 0, err
	}
	return n, sp.Commit(ctx)
}

func contarDelTenant(ctx context.Context, tx pgx.Tx, t tablaPurga, slug string) (int, error) {
	return conModo(ctx, tx, slug, t.ligada, func() (int, error) {
		var n int
		err := tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1",
			pgx.Identifier{t.nombre}.Sanitize(), pgx.Identifier{t.columna}.Sanitize()), slug).Scan(&n)
		if err != nil {
			return 0, fmt.Errorf("pg: contar %s: %w", t.nombre, err)
		}
		return n, nil
	})
}

// conModo ejecuta fn con la fusión apagada si la tabla es de ámbito ligado, y la
// vuelve a encender al terminar (también si fn falla: el SAVEPOINT que la
// envuelve deshace igualmente el set_config).
func conModo(ctx context.Context, q Querier, slug string, ligada bool, fn func() (int, error)) (int, error) {
	if !ligada {
		return fn()
	}
	if _, err := q.Exec(ctx, `SELECT set_config('app.merge', 'off', true), set_config('app.tenant', $1, true)`, slug); err != nil {
		return 0, err
	}
	n, err := fn()
	if err != nil {
		return 0, err
	}
	if _, err := q.Exec(ctx, `SELECT set_config('app.merge', 'on', true)`); err != nil {
		return 0, err
	}
	return n, nil
}

func motivoDeRetencion(pge *pgconn.PgError) string {
	motivo := pge.Code + ": " + pge.Message
	if pge.Detail != "" {
		motivo += " (" + pge.Detail + ")"
	}
	return motivo
}

// comprobarRetencionLegal detiene la purga si la base tiene retenciones legales
// (Drive, Mail, Notepads: todas con la misma forma) activas para el tenant.
func comprobarRetencionLegal(ctx context.Context, tx pgx.Tx, slug string) error {
	var hay bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('legal_holds') IS NOT NULL`).Scan(&hay); err != nil || !hay {
		return err
	}
	var activas int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM legal_holds WHERE tenant = $1 AND released_at IS NULL`, slug).Scan(&activas); err != nil {
		return fmt.Errorf("pg: leer retenciones legales: %w", err)
	}
	if activas > 0 {
		return &PurgaRetenidaError{Motivo: fmt.Sprintf("%d retenciones legales activas", activas)}
	}
	return nil
}

// prepararPurga ejecuta el gancho de la base, si lo hay, ANTES de borrar.
func prepararPurga(ctx context.Context, tx pgx.Tx, slug string) (string, error) {
	var existe bool
	if err := tx.QueryRow(ctx, purgePrepareExistsSQL).Scan(&existe); err != nil {
		return "", fmt.Errorf("pg: buscar %s: %w", PurgePrepareFunc, err)
	}
	if !existe {
		return "", nil
	}
	var nota *string
	if err := tx.QueryRow(ctx, purgePrepareCallSQL, slug).Scan(&nota); err != nil {
		var pge *pgconn.PgError
		if errors.As(err, &pge) && pge.Code == CodigoPurgaRetenida {
			return "", err
		}
		return "", fmt.Errorf("pg: %s: %w", PurgePrepareFunc, err)
	}
	if nota == nil {
		return "", nil
	}
	return *nota, nil
}

// encolarPurgaDeObjetos deja la orden de borrar los objetos del tenant, si la
// base tiene la cola. `reabrir` vuelve a abrir una orden ya cumplida: esta purga
// ha borrado filas nuevas, y sus objetos pueden no estar barridos.
func encolarPurgaDeObjetos(ctx context.Context, tx pgx.Tx, slug string, reabrir bool) (bool, error) {
	var hay bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('platform_tenant_purges') IS NOT NULL`).Scan(&hay); err != nil || !hay {
		return false, err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO platform_tenant_purges (slug, db_purged_at) VALUES ($1, now())
		ON CONFLICT (slug) DO UPDATE SET
		    db_purged_at = now(),
		    objects_purged_at = CASE WHEN $2 OR platform_tenant_purges.db_purged_at IS NULL
		                             THEN NULL ELSE platform_tenant_purges.objects_purged_at END,
		    last_error = CASE WHEN $2 THEN '' ELSE platform_tenant_purges.last_error END`, slug, reabrir)
	if err != nil {
		return false, fmt.Errorf("pg: encolar el borrado de objetos: %w", err)
	}
	return true, nil
}

// EstadoObjetos es cómo va el borrado de objetos de un tenant en una base.
type EstadoObjetos struct {
	// SinCola: la base no guarda objetos; no hay nada que esperar.
	SinCola bool `json:"no_queue,omitempty"`
	// Pendiente: hay orden y el servicio aún no la ha cumplido.
	Pendiente  bool       `json:"pending,omitempty"`
	Purgados   *time.Time `json:"purged_at,omitempty"`
	Intentos   int        `json:"attempts,omitempty"`
	UltimoErr  string     `json:"last_error,omitempty"`
	Solicitado *time.Time `json:"requested_at,omitempty"`
}

// Hecho dice si ya no queda ningún objeto del tenant por borrar en esta base.
func (e EstadoObjetos) Hecho() bool { return e.SinCola || e.Purgados != nil }

// ConsultarPurgaDeObjetos lee, como la purga (InMergeTx), el estado de la orden
// de borrar objetos de `slug`.
func ConsultarPurgaDeObjetos(ctx context.Context, db Database, slug string) (EstadoObjetos, error) {
	var e EstadoObjetos
	err := db.InMergeTx(ctx, func(tx pgx.Tx) error {
		var hay bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass('platform_tenant_purges') IS NOT NULL`).Scan(&hay); err != nil {
			return err
		}
		if !hay {
			e.SinCola = true
			return nil
		}
		var listo *time.Time
		err := tx.QueryRow(ctx, `SELECT requested_at, db_purged_at, objects_purged_at, attempts, last_error
			FROM platform_tenant_purges WHERE slug = $1`, slug).Scan(&e.Solicitado, &listo, &e.Purgados, &e.Intentos, &e.UltimoErr)
		if errors.Is(err, pgx.ErrNoRows) {
			// Sin orden: la purga de filas no ha terminado aquí todavía.
			e.Pendiente = true
			return nil
		}
		if err != nil {
			return err
		}
		e.Pendiente = listo == nil || e.Purgados == nil
		return nil
	})
	return e, err
}

// PurgaDeObjetos es una orden de borrar objetos reclamada por el trabajador del
// servicio.
type PurgaDeObjetos struct {
	Slug string
	// Generacion es el db_purged_at reclamado: si Sytue ID reabre la orden
	// mientras se barre, marcarla hecha con la generación vieja no cuenta.
	Generacion time.Time
	// Claves sueltas (fuera de los prefijos del tenant) que también hay que borrar.
	Claves []string
}

// ReclamarPurgasDeObjetos reclama hasta `limite` órdenes pendientes cuyo último
// intento tenga más de `reintento`. Multi-réplica: SKIP LOCKED y la marca de
// intento en el mismo UPDATE.
func ReclamarPurgasDeObjetos(ctx context.Context, db Database, limite int, reintento time.Duration) ([]PurgaDeObjetos, error) {
	if limite <= 0 {
		limite = 10
	}
	var out []PurgaDeObjetos
	err := db.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE platform_tenant_purges p SET attempts = attempts + 1, last_attempt_at = now()
			WHERE p.slug IN (
			    SELECT slug FROM platform_tenant_purges
			    WHERE db_purged_at IS NOT NULL AND objects_purged_at IS NULL
			      AND (last_attempt_at IS NULL OR last_attempt_at < now() - $2::interval)
			    ORDER BY last_attempt_at NULLS FIRST
			    LIMIT $1 FOR UPDATE SKIP LOCKED)
			RETURNING p.slug, p.db_purged_at`, limite, fmt.Sprintf("%d seconds", int(reintento.Seconds())))
		if err != nil {
			return fmt.Errorf("pg: reclamar purgas de objetos: %w", err)
		}
		for rows.Next() {
			var p PurgaDeObjetos
			if err := rows.Scan(&p.Slug, &p.Generacion); err != nil {
				rows.Close()
				return err
			}
			out = append(out, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			claves, err := tx.Query(ctx, `SELECT key FROM platform_tenant_purge_keys WHERE slug = $1 ORDER BY key`, out[i].Slug)
			if err != nil {
				return err
			}
			for claves.Next() {
				var k string
				if err := claves.Scan(&k); err != nil {
					claves.Close()
					return err
				}
				out[i].Claves = append(out[i].Claves, k)
			}
			claves.Close()
			if err := claves.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// MarcarObjetosPurgados cierra una orden reclamada. No hace nada si Sytue ID la
// reabrió mientras tanto (otra generación): la siguiente pasada la barre otra vez.
func MarcarObjetosPurgados(ctx context.Context, db Database, p PurgaDeObjetos) error {
	return db.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE platform_tenant_purges SET objects_purged_at = now(), last_error = ''
			WHERE slug = $1 AND db_purged_at = $2 AND objects_purged_at IS NULL`, p.Slug, p.Generacion)
		if err != nil {
			return fmt.Errorf("pg: marcar objetos purgados: %w", err)
		}
		if tag.RowsAffected() == 1 {
			_, err = tx.Exec(ctx, `DELETE FROM platform_tenant_purge_keys WHERE slug = $1`, p.Slug)
		}
		return err
	})
}

// AnotarFalloPurgaDeObjetos deja el error del último intento, recortado: es lo
// que Sytue ID enseña al operador.
func AnotarFalloPurgaDeObjetos(ctx context.Context, db Database, slug string, causa error) error {
	msg := strings.TrimSpace(causa.Error())
	for len(msg) > 500 {
		// Recorte por runas: un error de S3 puede traer texto no ASCII.
		_, ancho := utf8.DecodeLastRuneInString(msg)
		msg = msg[:len(msg)-ancho]
	}
	return db.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE platform_tenant_purges SET last_error = $2 WHERE slug = $1`, slug, msg)
		return err
	})
}
