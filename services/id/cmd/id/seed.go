// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	idhttp "git.sytue.eu/sytue/one/services/id/internal/http"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// Constantes del seed de desarrollo. Fijas y sin secretos reales: SOLO sirven
// para poder hacer LOGIN en local. NUNCA deben aparecer en staging/prod.
const (
	seedOrgSlug   = "acme"
	seedOrgName   = "Acme, S.L."
	seedUserEmail = "admin@sytue.localhost"
	seedUserName  = "Admin Sytue"

	// Segundo usuario de dev SIN rol de consola (member) y SIN MFA. Existe para
	// ejercer el contrato de denegación de Sytue Admin (member → 403) en e2e:
	// entra en la suite pero al ir a admin.sytue.localhost recibe 403 y no se le
	// filtra administración. Comparte la contraseña de dev del admin (mismo hash).
	// Idempotente por email, igual que el admin.
	seedMemberEmail = "member@sytue.localhost"
	seedMemberName  = "Member Sytue"
	seedMemberRole  = domain.RoleMember

	// seedUserRole es admin para DESBLOQUEAR Sytue Admin en dev. admin/owner
	// exigen step-up MFA (Role.RequiresMFA, authorize.go); por eso el seed también
	// habilita TOTP (MFAEnabled=true + seedTOTPSecretB32) para que ese segundo
	// factor se pueda CUMPLIR en local. NO relaja la seguridad: el step-up sigue
	// siendo obligatorio; solo se provisiona el factor del usuario de dev.
	seedUserRole       = domain.RoleAdmin
	seedPasswordEnvVar = "SYTUE_ID_DEV_SEED_PASSWORD" //nolint:gosec // G101: nombre de variable de entorno, no un secreto.
	seedDefaultPass    = "sytue-dev-admin"            //nolint:gosec // G101: contraseña de dev por defecto, no un secreto real.

	// seedTOTPSecretB32 es el secreto TOTP (base32 RFC 4648, sin padding) del
	// usuario admin de dev. SOLO DEV: fijo y conocido para que el login con
	// step-up MFA se pueda completar en local y los tests/e2e calculen el código
	// del segundo factor. NUNCA en staging/prod (el seed aborta fuera de dev).
	seedTOTPSecretB32 = "JBSWY3DPEHPK3PXP" //nolint:gosec // G101: secreto TOTP de dev conocido, no un secreto real.

	// Secreto compartido de los clients confidenciales en DEV. El mismo valor que
	// usan los BFF de las apps (SYTUE_OIDC_CLIENT_SECRET) para autenticarse en el
	// token endpoint. Solo-dev: sin él, los clients confidenciales quedan
	// "deshabilitados" (secret_hash NULL) y el login falla.
	seedClientSecretEnvVar  = "SYTUE_ID_DEV_CLIENT_SECRET" //nolint:gosec // G101: nombre de variable de entorno, no un secreto.
	seedDefaultClientSecret = "sytue-dev-client-secret"    //nolint:gosec // G101: secreto de dev por defecto, no un secreto real.
)

// runSeed es el punto de entrada del subcomando `seed`/`seed-dev`. Crea (de
// forma idempotente) una organización y un usuario administrador de desarrollo
// con la contraseña hasheada por el mecanismo REAL del servicio (Argon2id),
// para permitir el LOGIN en local. Devuelve el código de salida del proceso.
//
// SEGURIDAD: se ABORTA si el entorno no es dev. El seed introduce credenciales
// conocidas y NUNCA debe ejecutarse en staging/producción.
func runSeed() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := idhttp.Load()
	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: cfg.ServiceName,
		Version: cfg.Version,
	})
	ctx = log.Into(ctx, logger)
	if err != nil {
		logger.Error("configuración inválida", "error", err)
		return 1
	}

	// Guardarraíl: SOLO dev. En staging/prod se rechaza con mensaje claro.
	if !seedAllowed(cfg.Env) {
		logger.Error("el subcomando seed está deshabilitado fuera de dev",
			"env", string(cfg.Env),
			"motivo", "introduce credenciales de desarrollo conocidas")
		fmt.Fprintf(os.Stderr,
			"seed: ABORTADO — entorno %q no es dev; el seed solo se permite en desarrollo\n",
			cfg.Env)
		return 1
	}

	password := resolveSeedPassword(os.Getenv(seedPasswordEnvVar))

	if err := seed(ctx, cfg.DatabaseDSN, password, logger); err != nil {
		logger.Error("seed de desarrollo falló", "error", err)
		return 1
	}
	return 0
}

// seedAllowed indica si el seed de desarrollo puede ejecutarse en este entorno.
// Es puro (sin efectos) para poder testearlo: solo `dev` está permitido; staging
// y prod quedan explícitamente fuera.
func seedAllowed(env config.Environment) bool {
	return env == config.EnvDev
}

// resolveSeedPassword devuelve la contraseña efectiva del usuario de dev: la del
// entorno si se aporta, o el default conocido. Función pura (testeable).
func resolveSeedPassword(fromEnv string) string {
	if fromEnv != "" {
		return fromEnv
	}
	return seedDefaultPass
}

// resolveSeedClientSecret devuelve el secreto compartido de los clients
// confidenciales de dev (del entorno, o el default conocido). Puro (testeable).
func resolveSeedClientSecret(fromEnv string) string {
	if fromEnv != "" {
		return fromEnv
	}
	return seedDefaultClientSecret
}

// seedClients fija el secret_hash de los clients confidenciales de dev que aún
// no lo tienen (secret_hash NULL de 0002_seed_clients), usando el hash del
// secreto compartido de dev. Idempotente: no re-hashea (ni rota) los que ya
// tengan un secreto configurado. Solo-dev: permite que los BFF de las apps se
// autentiquen en el token endpoint (client_secret_basic) y complete el login.
func seedClients(ctx context.Context, st *store.Store, q pg.Querier, secretHash string) error {
	clients, err := st.ListClients(ctx)
	if err != nil {
		return fmt.Errorf("seed: listar clients: %w", err)
	}
	for _, c := range clients {
		if c.IsPublic() || c.SecretHash != "" {
			continue
		}
		if err := st.SetClientSecretHash(ctx, q, c.ID, secretHash); err != nil {
			return fmt.Errorf("seed: fijar secreto de client %s: %w", c.ID, err)
		}
	}
	return nil
}

// seed abre el pool, aplica migraciones (idempotentes) y crea/actualiza la
// organización y el usuario admin de desarrollo dentro de una transacción.
func seed(ctx context.Context, dsn, password string, logger *slog.Logger) error {
	// Migraciones idempotentes: garantizan que las tablas existen aunque el
	// seed se lance antes del primer arranque del servidor.
	if err := pg.MigrateUp(iddb.Migrations, iddb.MigrationsDir, dsn); err != nil {
		return fmt.Errorf("seed: migraciones: %w", err)
	}

	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 4})
	if err != nil {
		return fmt.Errorf("seed: conexión: %w", err)
	}
	defer pool.Close()

	// store.New (sin cipher): el seed no toca material de firma (signing_keys). El
	// secreto TOTP del admin de dev SÍ pasa por el cipher, pero en dev no hay KEK
	// (misma config que el server), así que ambos operan en modo identidad: se
	// guarda en claro y GetTOTPSecret lo lee tal cual (ruta "en claro heredado" de
	// keycrypto). ponytail: sin cipher — el secreto es una constante de dev
	// conocida; nada que proteger en local (y el seed aborta fuera de dev).
	st := store.New(pool)
	if err := configureCommandCallIdentity(ctx, st); err != nil {
		return fmt.Errorf("seed identity call guard: %w", err)
	}

	hash, err := domain.HashPassword(password)
	if err != nil {
		return fmt.Errorf("seed: hash de contraseña: %w", err)
	}

	// Las tablas de Sytue ID (organizations/users/memberships) NO usan RLS: el
	// propio IdP es la fuente de verdad multi-tenant y las políticas RLS viven
	// aguas abajo en los servicios consumidores (ver 0001_core_identity.up.sql).
	// Por eso NO se fija app.tenant_id aquí. Todo va en una única transacción
	// para que el seed sea atómico.
	clientSecretHash, err := domain.HashPassword(resolveSeedClientSecret(os.Getenv(seedClientSecretEnvVar)))
	if err != nil {
		return fmt.Errorf("seed: hash de client secret: %w", err)
	}

	err = st.InTx(ctx, func(q pg.Querier) error {
		org, err := ensureOrg(ctx, st, q, seedOrgSlug, seedOrgName)
		if err != nil {
			return err
		}
		user, err := ensureUser(ctx, st, q, hash, seedUserEmail, seedUserName, true)
		if err != nil {
			return err
		}
		// TOTP de dev (idempotente, confirmado): fija el secreto conocido para que
		// el admin pueda cumplir el step-up MFA en local. Sin él, MFAEnabled=true
		// dejaría el login sin segundo factor válido y el flujo nunca completaría.
		totpSecret, err := totp.DecodeSecret(seedTOTPSecretB32)
		if err != nil {
			return fmt.Errorf("seed: decodificar secreto TOTP: %w", err)
		}
		if err := st.UpsertTOTPSecret(ctx, q, user.ID, totpSecret, true); err != nil {
			return fmt.Errorf("seed: fijar secreto TOTP: %w", err)
		}
		if err := seedClients(ctx, st, q, clientSecretHash); err != nil {
			return err
		}
		// Rol admin (idempotente: upsert por (org,user)) para DESBLOQUEAR Console.
		// admin exige step-up MFA (Role.RequiresMFA); el TOTP sembrado arriba +
		// MFAEnabled=true lo hacen cumplible en dev sin relajar la política.
		if err := st.UpsertMembership(ctx, q, domain.Membership{
			OrgID:  org.ID,
			UserID: user.ID,
			Role:   seedUserRole,
		}); err != nil {
			return err
		}
		// Segundo usuario de dev SIN rol de consola (member, sin MFA/TOTP): entra en
		// la suite pero recibe 403 en admin.sytue.localhost (guard admin/owner).
		member, err := ensureUser(ctx, st, q, hash, seedMemberEmail, seedMemberName, false)
		if err != nil {
			return err
		}
		return st.UpsertMembership(ctx, q, domain.Membership{
			OrgID:  org.ID,
			UserID: member.ID,
			Role:   seedMemberRole,
		})
	})
	if err != nil {
		return err
	}

	// Credenciales de dev bien visibles para el desarrollador.
	logger.Info("seed de desarrollo completado",
		"org_slug", seedOrgSlug, "org_name", seedOrgName,
		"email", seedUserEmail, "role", string(seedUserRole), "mfa", true,
		"member_email", seedMemberEmail, "member_role", string(seedMemberRole), "member_mfa", false)
	fmt.Printf("\n"+
		"────────────────────────────────────────────────────────────\n"+
		" Sytue ID — usuario admin de DESARROLLO listo para LOGIN\n"+
		"   Organización : %s (%s)\n"+
		"   Email        : %s\n"+
		"   Contraseña   : %s\n"+
		"   Rol          : %s\n"+
		"   MFA / TOTP   : habilitado — secreto dev (base32): %s\n"+
		"   ── member    : %s (rol %s, sin MFA, misma contraseña)\n"+
		" (Solo dev. Cambia la contraseña con %s.)\n"+
		"────────────────────────────────────────────────────────────\n\n",
		seedOrgName, seedOrgSlug, seedUserEmail, password,
		seedUserRole, seedTOTPSecretB32, seedMemberEmail, seedMemberRole, seedPasswordEnvVar)
	return nil
}

// ensureOrg crea la organización indicada si no existe, o devuelve la actual.
// La comparten el seed de dev y el bootstrap del propietario de producción.
func ensureOrg(ctx context.Context, st *store.Store, q pg.Querier, slug, name string) (domain.Organization, error) {
	org, err := st.GetOrgBySlug(ctx, slug)
	switch {
	case err == nil:
		return org, nil
	case !errors.Is(err, domain.ErrNotFound):
		return domain.Organization{}, fmt.Errorf("seed: buscar org: %w", err)
	}
	org, err = st.CreateOrg(ctx, q, domain.Organization{
		Slug:   slug,
		Name:   name,
		Status: domain.StatusActive,
	})
	if err != nil {
		return domain.Organization{}, fmt.Errorf("seed: crear org: %w", err)
	}
	return org, nil
}

// ensureUser crea o actualiza un usuario de dev de forma idempotente (por email):
// email verificado, contraseña Argon2id fijada por el mecanismo real del servicio
// y MFA según mfa. El admin va con mfa=true (su rol exige step-up; el secreto TOTP
// se siembra aparte en seed()); el member va con mfa=false (rol sin MFA).
func ensureUser(ctx context.Context, st *store.Store, q pg.Querier, hash, email, name string, mfa bool) (domain.User, error) {
	user, err := st.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		// Usuario existente: reafirma perfil/estado y refresca la contraseña.
		user.Name = name
		user.EmailVerified = true
		user.MFAEnabled = mfa
		user.Status = domain.StatusActive
		if user, err = st.UpdateUser(ctx, q, user); err != nil {
			return domain.User{}, fmt.Errorf("seed: actualizar usuario: %w", err)
		}
		if err := st.SetPassword(ctx, q, user.ID, hash); err != nil {
			return domain.User{}, fmt.Errorf("seed: fijar contraseña: %w", err)
		}
		return user, nil
	case !errors.Is(err, domain.ErrNotFound):
		return domain.User{}, fmt.Errorf("seed: buscar usuario: %w", err)
	}

	// Alta nueva. Si mfa=true (admin), el secreto TOTP de dev (sembrado en seed())
	// hace que el segundo factor que exige su rol sea cumplible en local.
	user, err = st.CreateUser(ctx, q, domain.User{
		Email:         email,
		EmailVerified: true,
		Name:          name,
		PasswordHash:  hash,
		MFAEnabled:    mfa,
		Status:        domain.StatusActive,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("seed: crear usuario: %w", err)
	}
	return user, nil
}
