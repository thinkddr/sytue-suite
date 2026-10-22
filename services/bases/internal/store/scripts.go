// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/guiones"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// SCRIPTS DE USUARIO (bases-2, automatización; ADR 0069)
//
// Todo script cuelga de una base y TODA operación de aquí empieza por la base de la
// ruta: se resuelve el rol de quien llama sobre ella y el script ha de ser de ESA base
// (un id de script de otra base o de otra organización no existe). Como en el resto del
// servicio, la decisión y la escritura van en la misma transacción.
//
// Quien viene de otra organización no usa scripts (403): allí no es más que su `sub`, y
// un guion compartido con gente de fuera es otra decisión.

const colsScript = `s.id::text, s.base_id::text, s.name, s.scope, s.author_sub, s.created_at,
	coalesce((SELECT max(v.version) FROM base_script_versions v WHERE v.tenant = s.tenant AND v.script_id = s.id), 0),
	coalesce((SELECT max(v.version) FROM base_script_versions v WHERE v.tenant = s.tenant AND v.script_id = s.id AND v.status = 'approved'), 0)`

func leerScript(row pgx.Row, sc *domain.Script) error {
	return row.Scan(&sc.ID, &sc.BaseID, &sc.Name, &sc.Scope, &sc.AuthorSub, &sc.CreatedAt, &sc.LatestVersion, &sc.ApprovedVersion)
}

const colsVersion = `script_id::text, version, code, code_hash, manifest, status, created_by, created_at, coalesce(reviewed_by, ''), reviewed_at`

func leerVersion(row pgx.Row, v *domain.VersionScript) error {
	var m []byte
	if err := row.Scan(&v.ScriptID, &v.Version, &v.Code, &v.CodeHash, &m, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.ReviewedBy, &v.ReviewedAt); err != nil {
		return err
	}
	v.Manifest = m
	return nil
}

// rolParaScripts es el rol de quien llama sobre la base (nivel base). Sin acceso, 404;
// de otra organización, 403.
func rolParaScripts(ctx context.Context, tx pgx.Tx, a domain.Actor, base string) (domain.Rol, error) {
	rol, err := rolEn(ctx, tx, a, base, "")
	if err != nil {
		return "", err
	}
	if a.Externo {
		return "", domain.ErrSinPermiso
	}
	return rol, nil
}

// scriptVisible carga un script de ESA base si quien llama puede verlo; si no, no existe.
func scriptVisible(ctx context.Context, tx pgx.Tx, a domain.Actor, base, sid string) (domain.Script, domain.Rol, error) {
	var sc domain.Script
	rol, err := rolParaScripts(ctx, tx, a, base)
	if err != nil {
		return sc, "", err
	}
	if err := leerScript(tx.QueryRow(ctx, `SELECT `+colsScript+` FROM base_scripts s WHERE s.id = $1 AND s.base_id = $2 AND s.tenant = $3`, sid, base, a.Tenant), &sc); err != nil {
		return sc, "", errNoFilas(err)
	}
	if !domain.VeScript(sc, a.Sub, domain.Permite(rol, domain.AccRevisarScripts)) {
		return sc, "", domain.ErrNoExiste
	}
	return sc, rol, nil
}

// tablasDeLaBase exige que todas las tablas del manifiesto sean de ESA base.
func tablasDeLaBase(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, tablas []string) error {
	if len(tablas) == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_tables WHERE tenant = $1 AND base_id = $2 AND id = ANY($3::uuid[])`, a.Tenant, base, tablas).Scan(&n); err != nil {
		return err
	}
	if n != len(tablas) {
		return domain.ErrInvalido
	}
	return nil
}

// cerrojoDeScripts serializa, por base, lo que cuenta scripts o numera versiones.
func cerrojoDeScripts(ctx context.Context, tx pgx.Tx, base string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('base_scripts:' || $1))`, base)
	return err
}

// ListarScripts devuelve los scripts de la base que quien llama puede ver, el más
// reciente primero, y si puede revisarlos.
func (s *Store) ListarScripts(ctx context.Context, a domain.Actor, base string) (out []domain.Script, revisa bool, err error) {
	out = []domain.Script{}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		rol, err := rolParaScripts(ctx, tx, a, base)
		if err != nil {
			return err
		}
		revisa = domain.Permite(rol, domain.AccRevisarScripts)
		rows, err := tx.Query(ctx, `SELECT `+colsScript+` FROM base_scripts s WHERE s.tenant = $1 AND s.base_id = $2 ORDER BY s.created_at DESC, s.id LIMIT $3`,
			a.Tenant, base, domain.MaxScriptsPorBase)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sc domain.Script
			if err := leerScript(rows, &sc); err != nil {
				return err
			}
			if domain.VeScript(sc, a.Sub, revisa) {
				out = append(out, sc)
			}
		}
		return rows.Err()
	})
	return out, revisa, err
}

// CrearScript guarda un script con su versión 1, en borrador (cualquiera con acceso a
// la base: lo que el script haga lo hará con el permiso de quien lo ejecute).
// `manifiesto` es el canónico de `m`.
func (s *Store) CrearScript(ctx context.Context, a domain.Actor, base, nombre, ambito, codigo string, m domain.ManifiestoScript, manifiesto []byte) (domain.Script, domain.VersionScript, error) {
	sc := domain.Script{BaseID: base, Name: nombre, Scope: ambito, AuthorSub: a.Sub, LatestVersion: 1}
	var v domain.VersionScript
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if _, err := rolParaScripts(ctx, tx, a, base); err != nil {
			return err
		}
		if err := tablasDeLaBase(ctx, tx, a, base, m.Tables); err != nil {
			return err
		}
		if err := cerrojoDeScripts(ctx, tx, base); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_scripts WHERE tenant = $1 AND base_id = $2`, a.Tenant, base).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxScriptsPorBase {
			return domain.ErrLimite
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_scripts (tenant, base_id, name, scope, author_sub) VALUES ($1, $2, $3, $4, $5)
			RETURNING id::text, created_at`, a.Tenant, base, nombre, ambito, a.Sub).Scan(&sc.ID, &sc.CreatedAt); err != nil {
			return err
		}
		return leerVersion(tx.QueryRow(ctx, `
			INSERT INTO base_script_versions (tenant, script_id, version, code, code_hash, manifest, created_by)
			VALUES ($1, $2, 1, $3, $4, $5, $6) RETURNING `+colsVersion,
			a.Tenant, sc.ID, codigo, guiones.HashCodigo(codigo), manifiesto, a.Sub), &v)
	})
	return sc, v, err
}

// VerScript devuelve el script y las versiones que quien llama puede ver, con código:
// todas a su autora y a quien revisa; las aprobadas a los demás.
func (s *Store) VerScript(ctx context.Context, a domain.Actor, base, sid string) (sc domain.Script, vs []domain.VersionScript, revisa bool, err error) {
	vs = []domain.VersionScript{}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		var rol domain.Rol
		if sc, rol, err = scriptVisible(ctx, tx, a, base, sid); err != nil {
			return err
		}
		revisa = domain.Permite(rol, domain.AccRevisarScripts)
		rows, err := tx.Query(ctx, `SELECT `+colsVersion+` FROM base_script_versions WHERE tenant = $1 AND script_id = $2 ORDER BY version DESC LIMIT $3`,
			a.Tenant, sid, domain.MaxVersionesDeScript)
		if err != nil {
			return err
		}
		defer rows.Close()
		todas := sc.AuthorSub == a.Sub || (revisa && sc.Scope == domain.ScriptCompartido)
		for rows.Next() {
			var v domain.VersionScript
			if err := leerVersion(rows, &v); err != nil {
				return err
			}
			if todas || v.Status == guiones.EstadoAprobada {
				vs = append(vs, v)
			}
		}
		return rows.Err()
	})
	return sc, vs, revisa, err
}

// delAutor carga el script y exige que quien llama sea su autora.
func delAutor(ctx context.Context, tx pgx.Tx, a domain.Actor, base, sid string) (domain.Script, error) {
	sc, _, err := scriptVisible(ctx, tx, a, base, sid)
	if err == nil && sc.AuthorSub != a.Sub {
		err = domain.ErrSinPermiso
	}
	return sc, err
}

// NuevaVersionScript añade la siguiente versión, SIEMPRE en borrador: editar no hereda
// la aprobación de la anterior. Solo la autora.
func (s *Store) NuevaVersionScript(ctx context.Context, a domain.Actor, base, sid, codigo string, m domain.ManifiestoScript, manifiesto []byte) (domain.VersionScript, error) {
	var v domain.VersionScript
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		if _, err := delAutor(ctx, tx, a, base, sid); err != nil {
			return err
		}
		if err := tablasDeLaBase(ctx, tx, a, base, m.Tables); err != nil {
			return err
		}
		// Dos guardados a la vez del mismo script no pueden repetir número.
		if err := cerrojoDeScripts(ctx, tx, base); err != nil {
			return err
		}
		var ultima int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(version), 0) FROM base_script_versions WHERE tenant = $1 AND script_id = $2`, a.Tenant, sid).Scan(&ultima); err != nil {
			return err
		}
		if ultima >= domain.MaxVersionesDeScript {
			return domain.ErrLimite
		}
		return leerVersion(tx.QueryRow(ctx, `
			INSERT INTO base_script_versions (tenant, script_id, version, code, code_hash, manifest, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+colsVersion,
			a.Tenant, sid, ultima+1, codigo, guiones.HashCodigo(codigo), manifiesto, a.Sub), &v)
	})
	return v, err
}

// CambiarAmbitoScript pasa un script de privado a compartido o al revés. Solo la autora.
func (s *Store) CambiarAmbitoScript(ctx context.Context, a domain.Actor, base, sid, ambito string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		if _, err := delAutor(ctx, tx, a, base, sid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE base_scripts SET scope = $3 WHERE id = $1 AND tenant = $2`, sid, a.Tenant, ambito)
		return err
	})
}

// BorrarScript lo borra con sus versiones: su autora, o quien revisa si es compartido.
// El registro de ejecuciones se conserva.
func (s *Store) BorrarScript(ctx context.Context, a domain.Actor, base, sid string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		sc, rol, err := scriptVisible(ctx, tx, a, base, sid)
		if err != nil {
			return err
		}
		if sc.AuthorSub != a.Sub && (sc.Scope != domain.ScriptCompartido || !domain.Permite(rol, domain.AccRevisarScripts)) {
			return domain.ErrSinPermiso
		}
		_, err = tx.Exec(ctx, `DELETE FROM base_scripts WHERE id = $1 AND tenant = $2`, sid, a.Tenant)
		return err
	})
}

// ProponerVersionScript pasa una versión en borrador (o rechazada) a pendiente de
// revisión. Solo la autora, y solo de un script compartido.
func (s *Store) ProponerVersionScript(ctx context.Context, a domain.Actor, base, sid string, version int) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		sc, err := delAutor(ctx, tx, a, base, sid)
		if err != nil {
			return err
		}
		if sc.Scope != domain.ScriptCompartido {
			return domain.ErrEstadoScript
		}
		return transicion(ctx, tx, a, sid, version, guiones.DesdeProponer, guiones.EstadoPendiente, "", "")
	})
}

// RevisarVersionScript aprueba o rechaza una versión PENDIENTE de un script compartido.
// Lo hace quien puede revisar (la propietaria de la base, también la suya propia), y la
// decisión lleva el hash del código que revisó: si no es el de la versión, no se aplica.
func (s *Store) RevisarVersionScript(ctx context.Context, a domain.Actor, base, sid string, version int, hash string, aprobar bool) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		sc, rol, err := scriptVisible(ctx, tx, a, base, sid)
		if err != nil {
			return err
		}
		if !domain.Permite(rol, domain.AccRevisarScripts) {
			return domain.ErrSinPermiso
		}
		if sc.Scope != domain.ScriptCompartido {
			return domain.ErrEstadoScript
		}
		hacia := guiones.EstadoRechazada
		if aprobar {
			hacia = guiones.EstadoAprobada
		}
		return transicion(ctx, tx, a, sid, version, guiones.DesdeRevisar, hacia, a.Sub, hash)
	})
}

// transicion cambia el estado de una versión si está en uno de `desde`. Con `hash`, ha
// de ser el de su código. Revisor vacío = se borra la revisión anterior.
func transicion(ctx context.Context, tx pgx.Tx, a domain.Actor, sid string, version int, desde []string, hacia, revisor, hash string) error {
	var h, estado string
	if err := tx.QueryRow(ctx, `SELECT code_hash, status FROM base_script_versions WHERE tenant = $1 AND script_id = $2 AND version = $3 FOR UPDATE`,
		a.Tenant, sid, version).Scan(&h, &estado); err != nil {
		return errNoFilas(err)
	}
	if hash != "" && hash != h {
		return domain.ErrHashNoCoincide
	}
	if !slices.Contains(desde, estado) {
		return domain.ErrEstadoScript
	}
	_, err := tx.Exec(ctx, `
		UPDATE base_script_versions
		   SET status = $4, reviewed_by = $5::text, reviewed_at = CASE WHEN $5::text IS NULL THEN NULL ELSE now() END
		 WHERE tenant = $1 AND script_id = $2 AND version = $3`, a.Tenant, sid, version, hacia, ptr(revisor))
	return err
}

// denegada es un rechazo que se deja en el registro: el error que ve quien llama y el
// código corto que se anota.
type denegada struct {
	causa  error
	codigo string
	e      domain.EjecucionScript
}

func (d *denegada) Error() string { return d.causa.Error() }
func (d *denegada) Unwrap() error { return d.causa }

// AutorizarEjecucion decide si quien llama puede ejecutar esa versión AHORA, reserva la
// cuota y abre la línea del registro, todo en una transacción serializada por
// organización (dos ejecuciones a la vez no se cuelan las dos por el último hueco).
// Devuelve el código y el manifiesto autorizados: el cliente ejecuta exactamente eso.
//
// Comprueba, en este orden: que el script es de la base y visible; que la versión es
// ejecutable por esta persona (la suya, o una aprobada de un script compartido); que el
// manifiesto sigue siendo válido HOY (una versión guardada con una operación que ya no
// se admite no se borra, pero no corre); que sus tablas siguen siendo de la base; y que
// la persona puede hacer en cada una lo que el manifiesto declara. Esto último es un
// aviso temprano, no la frontera: cada operación del script vuelve a pasar por
// `autorizar` en su propia petición.
//
// Un rechazo a partir de la versión queda anotado (`denied`) y no gasta cuota.
func (s *Store) AutorizarEjecucion(ctx context.Context, a domain.Actor, base, sid string, version int) (domain.EjecucionAutorizada, error) {
	var out domain.EjecucionAutorizada
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		sc, _, err := scriptVisible(ctx, tx, a, base, sid)
		if err != nil {
			return err
		}
		var v domain.VersionScript
		if err := leerVersion(tx.QueryRow(ctx, `SELECT `+colsVersion+` FROM base_script_versions WHERE tenant = $1 AND script_id = $2 AND version = $3`, a.Tenant, sid, version), &v); err != nil {
			return errNoFilas(err)
		}
		e := domain.EjecucionScript{ScriptID: sid, Version: version, CodeHash: v.CodeHash, Sub: a.Sub}
		denegar := func(causa error, codigo string) error { return &denegada{causa: causa, codigo: codigo, e: e} }
		if !guiones.PuedeEjecutar(sc.AuthorSub, a.Sub, sc.Scope == domain.ScriptCompartido, v.Status) {
			return denegar(domain.ErrScriptSinAprobar, "not_approved")
		}
		m, err := domain.ParseManifiesto(v.Manifest)
		if err != nil {
			return denegar(err, "manifest_not_allowed")
		}
		if err := tablasDeLaBase(ctx, tx, a, base, m.Tables); err != nil {
			return denegar(err, "manifest_not_allowed")
		}
		owner, entradas, err := cargarACL(ctx, tx, a, base)
		if err != nil {
			return err
		}
		for _, tabla := range append([]string{""}, m.Tables...) {
			if tabla == "" && len(m.Tables) > 0 {
				continue // acotado a tablas: el permiso se mira en cada una
			}
			rol, _ := domain.ResolverRol(owner, entradas, a, tabla)
			if !domain.Permite(rol, domain.AccionParaEjecutar(m)) {
				return denegar(domain.ErrSinPermiso, "forbidden")
			}
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('base_script_runs:' || $1))`, a.Tenant); err != nil {
			return err
		}
		uso, err := usoCuota(ctx, tx, a)
		if err != nil {
			return err
		}
		if guiones.DecidirCuota(uso, m.MaxMs) != guiones.CuotaLibre {
			return domain.ErrCuotaScripts
		}
		out = domain.EjecucionAutorizada{Code: v.Code, CodeHash: v.CodeHash, Manifest: v.Manifest}
		return tx.QueryRow(ctx, `
			INSERT INTO base_script_runs (tenant, base_id, script_id, version, code_hash, sub, max_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			a.Tenant, base, sid, version, v.CodeHash, a.Sub, m.MaxMs).Scan(&out.RunID)
	})
	var d *denegada
	if errors.As(err, &d) {
		// La transacción de arriba se ha deshecho; el intento se anota en otra, corta. Si
		// esto falla, el rechazo se devuelve igual: lo que no puede pasar es que ejecute.
		// ponytail: sin tope propio; lo acota el limitador de peticiones de la plataforma.
		_ = s.en(ctx, a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO base_script_runs (tenant, base_id, script_id, version, code_hash, sub, max_ms, outcome, error_code, duration_ms)
				VALUES ($1, $2, $3, $4, $5, $6, 1, 'denied', $7, 0)`,
				a.Tenant, base, d.e.ScriptID, d.e.Version, d.e.CodeHash, a.Sub, d.codigo)
			return err
		})
		return out, d.causa
	}
	return out, err
}

func usoCuota(ctx context.Context, tx pgx.Tx, a domain.Actor) (guiones.UsoCuota, error) {
	var uso guiones.UsoCuota
	err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sub = $2), count(*),
		       coalesce(sum(coalesce(duration_ms, max_ms)) FILTER (WHERE sub = $2), 0)::bigint
		  FROM base_script_runs
		 WHERE tenant = $1 AND started_at > now() - interval '1 hour' AND outcome <> 'denied'`, a.Tenant, a.Sub).
		Scan(&uso.EjecucionesUsuario, &uso.EjecucionesOrg, &uso.MsUsuario)
	return uso, err
}

// UsoCuotaScripts devuelve lo consumido en la última hora (para enseñarlo).
func (s *Store) UsoCuotaScripts(ctx context.Context, a domain.Actor) (guiones.UsoCuota, error) {
	var uso guiones.UsoCuota
	err := s.en(ctx, a, func(tx pgx.Tx) (err error) {
		uso, err = usoCuota(ctx, tx, a)
		return err
	})
	return uso, err
}

// CerrarEjecucion anota el resultado, UNA sola vez y solo por quien la abrió, en esa base.
func (s *Store) CerrarEjecucion(ctx context.Context, a domain.Actor, base string, run int64, resultado, codigo string, ms, ops int) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE base_script_runs SET outcome = $5, error_code = $6, duration_ms = $7, ops = $8
			 WHERE id = $1 AND tenant = $2 AND base_id = $3 AND sub = $4 AND outcome = 'running'`,
			run, a.Tenant, base, a.Sub, resultado, codigo, ms, ops)
		if err == nil && tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return err
	})
}

// EjecucionesDeScripts pagina el registro de la base, de la más reciente a la más
// antigua (id < antes; 0 = desde el final). Solo quien revisa (la propietaria).
func (s *Store) EjecucionesDeScripts(ctx context.Context, a domain.Actor, base string, antes int64, limite int) ([]domain.EjecucionScript, error) {
	limite = min(max(limite, 1), domain.MaxPaginaRuns)
	out := []domain.EjecucionScript{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		rol, err := rolParaScripts(ctx, tx, a, base)
		if err != nil {
			return err
		}
		if !domain.Permite(rol, domain.AccRevisarScripts) {
			return domain.ErrSinPermiso
		}
		rows, err := tx.Query(ctx, `
			SELECT id, script_id::text, version, code_hash, sub, outcome, error_code, duration_ms, ops, started_at
			  FROM base_script_runs
			 WHERE tenant = $1 AND base_id = $2 AND ($3::bigint = 0 OR id < $3)
			 ORDER BY id DESC LIMIT $4`, a.Tenant, base, antes, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.EjecucionScript
			if err := rows.Scan(&e.ID, &e.ScriptID, &e.Version, &e.CodeHash, &e.Sub, &e.Outcome, &e.ErrorCode, &e.DurationMs, &e.Ops, &e.StartedAt); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
