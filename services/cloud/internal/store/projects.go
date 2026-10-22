// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const projectCols = `id, tenant, slug, name, repo_url, git_provider, production_ref,
	framework, root_dir, build_command, output_dir, install_cmd, owner_sub,
	notify_webhook_url, created_at, updated_at, build_strategy, app_port, health_path,
	instance_size, instance_count, autoscale_max_count, autoscale_cpu_target,
	release_command, preview_auth_hash, paused, http_rules`

// projectDests son los destinos de `projectCols`, EN SU ORDEN. Existe para que
// haya UN solo sitio que mantener: los destinos estaban escritos a mano en cada
// consulta que junta el proyecto con otra tabla y, al añadir las seis columnas de
// dimensionado (app_port … autoscale_cpu_target), se actualizaron unas y otras
// no. La que se quedó atrás —la de los deployments pendientes— rompía con «41
// field descriptions, 34 destinations» y dejó a Sytue Cloud sin reconciliar ni un
// despliegue. Con la lista compartida, una columna nueva se añade aquí y todas
// las consultas quedan bien a la vez.
func projectDests(p *domain.Project) []any {
	return []any{
		&p.ID, &p.Tenant, &p.Slug, &p.Name, &p.RepoURL, &p.GitProvider,
		&p.ProductionRef, &p.Framework, &p.RootDir, &p.BuildCommand, &p.OutputDir,
		&p.InstallCmd, &p.OwnerSub, &p.NotifyWebhookURL, &p.CreatedAt, &p.UpdatedAt,
		&p.BuildStrategy, &p.AppPort, &p.HealthPath, &p.InstanceSize, &p.InstanceCount,
		&p.AutoscaleMaxCount, &p.AutoscaleCPUTarget, &p.ReleaseCommand,
		&p.PreviewAuthHash, &p.Paused, &p.HTTPRules,
	}
}

func scanProject(row pgx.Row, t string) (domain.Project, error) {
	var p domain.Project
	if err := row.Scan(projectDests(&p)...); err != nil {
		return domain.Project{}, err
	}
	// Derivado aquí, en el ÚNICO sitio por el que pasan todas las lecturas: así
	// ninguna capa de arriba tiene que acordarse de calcularlo, y el hash —que
	// nunca sale en JSON— no necesita viajar más allá del controlador.
	p.PreviewProtected = p.PreviewAuthHash != ""
	if t != "" { // t vacío: conserva el tenant leído de la fila (lecturas cross-tenant)
		p.Tenant = t
	}
	return p, nil
}

// scanProjectWithSecret escanea projectCols seguidos de webhook_secret_enc (para
// la resolución del webhook, que necesita el secreto del proyecto). El bytea se
// vuelca en *enc (nil si la columna es NULL). t vacío conserva el tenant de la fila.
func scanProjectWithSecret(row pgx.Row, t string, enc *[]byte) (domain.Project, error) {
	var p domain.Project
	if err := row.Scan(&p.ID, &p.Tenant, &p.Slug, &p.Name, &p.RepoURL, &p.GitProvider,
		&p.ProductionRef, &p.Framework, &p.RootDir, &p.BuildCommand, &p.OutputDir,
		&p.InstallCmd, &p.OwnerSub, &p.NotifyWebhookURL, &p.CreatedAt, &p.UpdatedAt,
		&p.BuildStrategy, &p.AppPort, &p.HealthPath, &p.InstanceSize, &p.InstanceCount,
		&p.AutoscaleMaxCount, &p.AutoscaleCPUTarget, &p.ReleaseCommand,
		&p.PreviewAuthHash, &p.Paused, &p.HTTPRules,
		enc); err != nil {
		return domain.Project{}, err
	}
	p.PreviewProtected = p.PreviewAuthHash != ""
	if t != "" {
		p.Tenant = t
	}
	return p, nil
}

// NewProjectRecord son los datos de creación de un proyecto.
type NewProjectRecord struct {
	Slug          string
	Name          string
	RepoURL       string
	GitProvider   domain.GitProvider
	ProductionRef string
	Framework     domain.Framework
	BuildStrategy domain.BuildStrategy
	RootDir       string
	BuildCommand  string
	OutputDir     string
	InstallCmd    string
	OwnerSub      string
	// AppPort / HealthPath: 0 y "" significan «usa el default» (8080 y "/").
	AppPort    int
	HealthPath string
	// InstanceSize / InstanceCount: "" y 0 = defaults ('basic' y 1).
	InstanceSize  string
	InstanceCount int
	// AutoscaleMaxCount / AutoscaleCPUTarget: 0 = defaults (techo automático y 75 %).
	AutoscaleMaxCount  int
	AutoscaleCPUTarget int
	// WebhookSecretEnc es el secreto HMAC del webhook git del proyecto, cifrado en
	// reposo (AES-256-GCM). El servicio lo genera y sella; aquí solo se persiste.
	WebhookSecretEnc []byte
}

// CreateProject inserta un proyecto, validando la cuota de proyectos por org.
func (s *Store) CreateProject(ctx context.Context, rec NewProjectRecord, maxPerOrg int) (domain.Project, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.Project
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		p, e = createProjectTx(ctx, tx, t.Slug, rec, maxPerOrg)
		return e
	})
	return p, err
}

// createProjectTx es el cuerpo transaccional de CreateProject. Vive aparte para
// que el alta desde plantilla (from_template.go) lo ejecute DENTRO de su propia
// transacción, junto al resto de pasos del alta: la atomicidad no se puede
// conseguir encadenando métodos que abren una transacción cada uno.
//
// El tenant va como PREDICADO EXPLÍCITO en cada consulta, no solo apoyado en la
// RLS: la política es la segunda cerradura, no la única.
func createProjectTx(
	ctx context.Context, tx pgx.Tx, tenant string, rec NewProjectRecord, maxPerOrg int,
) (domain.Project, error) {
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM projects WHERE tenant=$1 AND deleted_at IS NULL`, tenant).
		Scan(&count); err != nil {
		return domain.Project{}, fmtErr("contar proyectos", err)
	}
	if maxPerOrg > 0 && count >= maxPerOrg {
		return domain.Project{}, domain.ErrQuotaExceeded
	}
	if reservado, err := slugReservadoParaOtros(ctx, tx, rec.Slug, tenant); err != nil {
		return domain.Project{}, err
	} else if reservado {
		return domain.Project{}, domain.ErrConflict
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO projects (tenant, slug, name, repo_url, git_provider, production_ref,
			framework, root_dir, build_command, output_dir, install_cmd, owner_sub, webhook_secret_enc,
			build_strategy, app_port, health_path, instance_size, instance_count,
			autoscale_max_count, autoscale_cpu_target)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			COALESCE(NULLIF($15, 0), 8080), COALESCE(NULLIF($16, ''), '/'),
			COALESCE(NULLIF($17, ''), 'basic'), COALESCE(NULLIF($18, 0), 1),
			$19, COALESCE(NULLIF($20, 0), 75))
		RETURNING `+projectCols,
		tenant, rec.Slug, rec.Name, rec.RepoURL, string(rec.GitProvider), rec.ProductionRef,
		string(rec.Framework), rec.RootDir, rec.BuildCommand, rec.OutputDir, rec.InstallCmd, rec.OwnerSub,
		rec.WebhookSecretEnc, string(rec.BuildStrategy), rec.AppPort, rec.HealthPath,
		rec.InstanceSize, rec.InstanceCount, rec.AutoscaleMaxCount, rec.AutoscaleCPUTarget)
	p, err := scanProject(row, tenant)
	switch {
	case uniqueViolation(err):
		return domain.Project{}, domain.ErrConflict
	case checkViolation(err):
		return domain.Project{}, domain.ErrInvalidInput
	case err != nil:
		return domain.Project{}, err
	}
	return p, nil
}

// GetProject devuelve un proyecto vivo por ID.
func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.Project
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+projectCols+` FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id)
		var e error
		p, e = scanProject(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	return p, err
}

// GetProjectWebhookSecretEnc devuelve el secreto de webhook cifrado de un
// proyecto vivo (bytes sellados; el servicio los descifra). ErrNotFound si no
// existe el proyecto; devuelve nil si el proyecto aún no tiene secreto propio.
func (s *Store) GetProjectWebhookSecretEnc(ctx context.Context, id uuid.UUID) ([]byte, error) {
	t := tenancy.MustFrom(ctx)
	var enc []byte
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT webhook_secret_enc FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id).Scan(&enc)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return enc, err
}

// SetProjectWebhookSecretEnc sustituye el secreto de webhook de un proyecto por
// otro ya sellado. Es lo que hace posible ROTARLO: hasta ahora el secreto se
// escribía al crear el proyecto y no había forma de cambiarlo, así que uno
// filtrado obligaba a rehacer el proyecto entero.
//
// Va bajo la política por tenant (no la de mantenimiento): lo dispara una
// petición de un humano de esa organización, y la RLS es la que impide tocar el
// proyecto de otra.
func (s *Store) SetProjectWebhookSecretEnc(ctx context.Context, id uuid.UUID, enc []byte) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE projects SET webhook_secret_enc=$3, updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, id, enc)
		if err != nil {
			return fmtErr("rotar secreto de webhook", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// GetProjectBySlug devuelve un proyecto vivo por slug.
func (s *Store) GetProjectBySlug(ctx context.Context, slug string) (domain.Project, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.Project
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+projectCols+` FROM projects WHERE tenant=$1 AND slug=$2 AND deleted_at IS NULL`,
			t.Slug, slug)
		var e error
		p, e = scanProject(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	return p, err
}

// ListProjects lista los proyectos vivos del tenant.
func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Project
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+projectCols+` FROM projects WHERE tenant=$1 AND deleted_at IS NULL
			 ORDER BY created_at DESC`, t.Slug)
		if err != nil {
			return fmtErr("listar proyectos", err)
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanProject(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateProjectRecord son los campos mutables de un proyecto (punteros = opcional).
type UpdateProjectRecord struct {
	Name             *string
	RepoURL          *string
	ProductionRef    *string
	Framework        *domain.Framework
	BuildStrategy    *domain.BuildStrategy
	RootDir          *string
	BuildCommand     *string
	OutputDir        *string
	InstallCmd       *string
	NotifyWebhookURL *string
	// AppPort / HealthPath: puerto de escucha y ruta de sondeo (nil = sin cambio).
	AppPort    *int
	HealthPath *string
	// InstanceSize / InstanceCount: tamaño y número de instancias (nil = sin cambio).
	InstanceSize  *string
	InstanceCount *int
	// AutoscaleMaxCount / AutoscaleCPUTarget: techo y objetivo del HPA (nil = sin
	// cambio). 0 en el techo vuelve al automático.
	AutoscaleMaxCount  *int
	AutoscaleCPUTarget *int
	// ReleaseCommand es lo que se ejecuta antes de arrancar la versión nueva
	// (nil = sin cambio; cadena vacía = quitarlo).
	ReleaseCommand *string
	// PreviewAuthHash es la línea htpasswd que cierra las previews (nil = sin
	// cambio; cadena vacía = dejarlas abiertas otra vez).
	PreviewAuthHash *string
	// Paused apaga el proyecto sin borrarlo: cero réplicas y cero facturación de
	// cómputo (nil = sin cambio).
	Paused *bool
	// HTTPRules son las redirecciones y cabeceras del borde (nil = sin cambio).
	// Se sustituyen ENTERAS: son una lista, y un merge por clave no significaría
	// nada útil sobre un array.
	HTTPRules *domain.HTTPRules
	// DeployKey es la clave privada git en claro (solo capa service; se cifra).
	DeployKey *string
	// RepoCredsEnc es el ciphertext persistido (nil = sin cambio).
	RepoCredsEnc *[]byte
}

// UpdateProject aplica una actualización parcial de configuración del proyecto.
func (s *Store) UpdateProject(ctx context.Context, id uuid.UUID, upd UpdateProjectRecord) (domain.Project, error) {
	t := tenancy.MustFrom(ctx)
	var fw *string
	if upd.Framework != nil {
		v := string(*upd.Framework)
		fw = &v
	}
	// bs=nil deja la estrategia sin cambio; bs=&"" la vuelve a 'auto' (valor real).
	var bs *string
	if upd.BuildStrategy != nil {
		v := string(*upd.BuildStrategy)
		bs = &v
	}
	var p domain.Project
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		setCreds := upd.RepoCredsEnc != nil
		row := tx.QueryRow(ctx, `
			UPDATE projects SET
				name           = COALESCE($3, name),
				repo_url       = COALESCE($4, repo_url),
				production_ref = COALESCE($5, production_ref),
				framework      = COALESCE($6, framework),
				root_dir       = COALESCE($7, root_dir),
				build_command  = COALESCE($8, build_command),
				output_dir         = COALESCE($9, output_dir),
				install_cmd        = COALESCE($10, install_cmd),
				notify_webhook_url = COALESCE($11, notify_webhook_url),
				repo_creds_enc     = CASE WHEN $12 THEN $13 ELSE repo_creds_enc END,
				build_strategy     = COALESCE($14, build_strategy),
				app_port           = COALESCE($15, app_port),
				health_path        = COALESCE($16, health_path),
				instance_size      = COALESCE($17, instance_size),
				instance_count     = COALESCE($18, instance_count),
				autoscale_max_count  = COALESCE($19, autoscale_max_count),
				autoscale_cpu_target = COALESCE($20, autoscale_cpu_target),
				release_command      = COALESCE($21, release_command),
				preview_auth_hash    = COALESCE($22, preview_auth_hash),
				paused               = COALESCE($23, paused),
				http_rules           = COALESCE($24, http_rules),
				updated_at         = now()
			WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL
			RETURNING `+projectCols,
			t.Slug, id, upd.Name, upd.RepoURL, upd.ProductionRef, fw,
			upd.RootDir, upd.BuildCommand, upd.OutputDir, upd.InstallCmd, upd.NotifyWebhookURL,
			setCreds, byteaOrNull(upd.RepoCredsEnc), bs, upd.AppPort, upd.HealthPath,
			upd.InstanceSize, upd.InstanceCount, upd.AutoscaleMaxCount, upd.AutoscaleCPUTarget,
			upd.ReleaseCommand, upd.PreviewAuthHash, upd.Paused, upd.HTTPRules)
		var e error
		p, e = scanProject(row, t.Slug)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		if e != nil {
			return e
		}
		return touchProductionIfRuntimeChanged(ctx, tx, t.Slug, id, upd)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	return p, err
}

// touchProductionIfRuntimeChanged devuelve la producción del proyecto a la ventana
// de reconciliación cuando cambia su contrato de ejecución (escala, puerto, sonda).
//
// Sin esto el cambio no se aplicaría: el controlador solo REAPLICA las producciones
// cuyo updated_at está dentro de su ventana de recencia —reaplicar todas en cada
// ciclo sería un cuello a escala—, así que una app estable desplegada hace semanas,
// que es justo el caso típico cuando alguien decide escalar, se quedaría con los
// pods viejos hasta el siguiente despliegue. Guardaría bien en la base de datos y no
// pasaría nada en el clúster.
//
// Va en la MISMA transacción que el UPDATE del proyecto a propósito: si se hicieran
// por separado quedaría el estado peor de todos, un proyecto reconfigurado que nadie
// reaplica y que además ya no se distingue de uno al día.
func touchProductionIfRuntimeChanged(
	ctx context.Context, tx pgx.Tx, tenant string, projectID uuid.UUID, upd UpdateProjectRecord,
) error {
	if upd.AppPort == nil && upd.HealthPath == nil &&
		upd.InstanceSize == nil && upd.InstanceCount == nil &&
		upd.AutoscaleMaxCount == nil && upd.AutoscaleCPUTarget == nil &&
		upd.Paused == nil && upd.HTTPRules == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE deployments SET updated_at = now()
		WHERE tenant=$1 AND project_id=$2 AND target='production' AND status='ready'`,
		tenant, projectID)
	if err != nil {
		return fmtErr("marcar la producción para reaplicar", err)
	}
	return nil
}

func byteaOrNull(enc *[]byte) []byte {
	if enc == nil {
		return nil
	}
	return *enc
}

// GetProjectRepoCredsEnc devuelve la deploy key cifrada del proyecto (nil si no hay).
func (s *Store) GetProjectRepoCredsEnc(ctx context.Context, tenant string, projectID uuid.UUID) ([]byte, error) {
	var enc []byte
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT repo_creds_enc FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			tenant, projectID).Scan(&enc)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return enc, err
}

// DeleteProject marca el proyecto (y en cascada por FK, sus recursos) como
// borrado. Devuelve los IDs de deployments activos a retirar por el controller.
func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Marca para retirada los deployments activos del proyecto.
		if _, err := tx.Exec(ctx,
			`UPDATE deployments SET status='canceled', updated_at=now()
			 WHERE tenant=$1 AND project_id=$2 AND status IN ('queued','building','deploying','ready')`,
			t.Slug, id); err != nil {
			return fmtErr("cancelar deployments", err)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE projects SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id)
		if err != nil {
			return fmtErr("borrar proyecto", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// Revoca los tokens confinados a este proyecto (deploy-keys): el soft-delete
		// no dispara el ON DELETE CASCADE de api_tokens.project_id, así que se borran
		// explícitamente para que la credencial "muera con su proyecto" (0017).
		if _, err := tx.Exec(ctx,
			`DELETE FROM api_tokens WHERE tenant=$1 AND project_id=$2`, t.Slug, id); err != nil {
			return fmtErr("revocar tokens del proyecto", err)
		}
		// Sus dominios propios se liberan con él (CLOUD-08): sus certificados van
		// en los Ingress de los deployments que el controlador ya está retirando.
		if _, err := tx.Exec(ctx,
			`DELETE FROM domains WHERE tenant=$1 AND project_id=$2`, t.Slug, id); err != nil {
			return fmtErr("liberar dominios del proyecto", err)
		}
		return nil
	})
}

// slugReservadoCond es cuándo un proyecto BORRADO sigue reteniendo su slug: hasta
// que se hayan borrado sus imágenes (purged_at) y hayan pasado 90 días. Decisión
// del dueño (2026-10-17, CLOUD-08): antes del plazo solo la misma organización lo
// recupera. Antes el slug quedaba libre al borrar, y quien lo tomaba heredaba el
// subdominio y el repositorio de imágenes del dueño anterior.
const slugReservadoCond = `deleted_at IS NOT NULL
	AND (purged_at IS NULL OR deleted_at > now() - interval '90 days')`

// slugReservadoParaOtros mira, dentro de la transacción del alta, si el slug lo
// retiene un proyecto borrado de OTRA organización. La RLS esconde las filas de
// los demás, así que abre la puerta de mantenimiento solo para esta consulta y la
// cierra antes de insertar.
func slugReservadoParaOtros(ctx context.Context, tx pgx.Tx, slug, tenant string) (bool, error) {
	var reservado bool
	err := enMantenimiento(ctx, tx, func() error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects
			WHERE slug=$1 AND tenant<>$2 AND `+slugReservadoCond+`)`, slug, tenant).Scan(&reservado)
	})
	if err != nil {
		return false, fmtErr("comprobar la reserva del slug", err)
	}
	return reservado, nil
}

// enMantenimiento abre la puerta de mantenimiento (app.maintenance) solo para fn,
// dentro de una transacción de organización, y la vuelve a cerrar. Es para las
// comprobaciones de unicidad que cruzan organizaciones (un slug, un dominio): la
// RLS esconde las filas ajenas y la decisión necesita verlas.
func enMantenimiento(ctx context.Context, tx pgx.Tx, fn func() error) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('app.maintenance', 'on', true)`); err != nil {
		return err
	}
	err := fn()
	if _, cerr := tx.Exec(ctx, `SELECT set_config('app.maintenance', '', true)`); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// SlugsTaken devuelve, de entre los slugs dados, cuáles están ya en uso por
// CUALQUIER tenant.
//
// UNA sola consulta para todos los candidatos (`slug = ANY($1)`) y no una por
// candidato: quien pide una sugerencia prueba hasta 8 nombres y quien escribe el
// suyo pide además una lista de alternativas, así que la versión de uno en uno
// convertía cada pulsación en una decena de viajes a la base de datos con alguien
// esperando delante de la pantalla.
//
// La comprobación va bajo mantenimiento a propósito: el índice único de slug es
// GLOBAL —el slug es el subdominio público de la app—, así que una consulta acotada
// al tenant diría «libre» de un nombre que otra organización ya tiene y la creación
// fallaría igual, que es justo lo que esto viene a evitar.
//
// No filtra nada nuevo: dice qué nombres están cogidos, y eso ya se deduce del 409
// de la creación. De quién son, no se dice.
//
// Cuenta los vivos y los borrados que aún retienen el slug (slugReservadoCond):
// el índice único solo cubre los vivos, la retención la aplica createProjectTx.
func (s *Store) SlugsTaken(ctx context.Context, slugs []string) (map[string]bool, error) {
	taken := make(map[string]bool, len(slugs))
	if len(slugs) == 0 {
		return taken, nil
	}
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT slug FROM projects WHERE slug = ANY($1)
			AND (deleted_at IS NULL OR (`+slugReservadoCond+`))`, slugs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var slug string
			if err := rows.Scan(&slug); err != nil {
				return err
			}
			taken[slug] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmtErr("comprobar disponibilidad de slugs", err)
	}
	return taken, nil
}

// ProjectToPurge es un proyecto borrado cuyas imágenes siguen en el registry.
type ProjectToPurge struct {
	ID   uuid.UUID
	Slug string
	// SlugVivo: la misma organización ya ha vuelto a crear el slug. Sus imágenes
	// son ahora del proyecto nuevo y no se tocan.
	SlugVivo bool
}

// ListProjectsToPurge lista proyectos borrados sin purgar. Cross-tenant.
func (s *Store) ListProjectsToPurge(ctx context.Context, limit int) ([]ProjectToPurge, error) {
	var out []ProjectToPurge
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.slug, EXISTS (SELECT 1 FROM projects v WHERE v.slug = p.slug AND v.deleted_at IS NULL)
			FROM projects p WHERE p.deleted_at IS NOT NULL AND p.purged_at IS NULL
			ORDER BY p.deleted_at LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p ProjectToPurge
			if err := rows.Scan(&p.ID, &p.Slug, &p.SlugVivo); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// MarkProjectPurged anota que las imágenes de un proyecto borrado ya no están.
func (s *Store) MarkProjectPurged(ctx context.Context, id uuid.UUID) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE projects SET purged_at = now() WHERE id=$1 AND deleted_at IS NOT NULL`, id)
		return err
	})
}
