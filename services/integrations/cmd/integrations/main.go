// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command integrations es el binario del Sytue Integration Gateway: la pasarela
// única por la que TODO el tráfico saliente a terceros pasa con políticas
// encadenadas (SSRF + rate-limit + auditoría + inyección de auth). Estado durable
// en PostgreSQL (RLS por org); secretos sellados con AES-256-GCM. Auth: JWT de
// Sytue ID.
//
// Subcomandos:
//
//	integrations              arranca el servidor HTTP (por defecto)
//	integrations migrate      aplica las migraciones de base de datos y sale
//	integrations healthcheck  sonda de liveness para el healthcheck de Docker
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"

	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/db/seeds"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/drivers"
	integrationshttp "git.sytue.eu/sytue/one/services/integrations/internal/http"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(runHealthcheck())
		case "migrate":
			if err := runMigrate(); err != nil {
				fmt.Fprintln(os.Stderr, "migrate:", err)
				os.Exit(1)
			}
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// Migraciones al arrancar (paridad dev/prod; idempotente).
	if err := pg.MigrateUp(integrationsdb.Migrations, integrationsdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// KEK para sellar secretos de instancia. Sin ella el servicio arranca, pero
	// escribir secretos se rechaza (fallo explícito, nunca en claro).
	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("keycrypto: %w", err)
	}
	if !cipher.Enabled() {
		logger.Warn("SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY no configurada: la escritura de secretos quedará deshabilitada")
	}

	st := store.New(pool, cipher)

	// Cómo se sostiene el aislamiento por organización, decidido UNA vez y con
	// pruebas: o se asume el rol de aplicación, o se comprueba que la RLS forzada
	// basta sobre un rol que no puede eludirla. Si ninguna de las dos cosas se
	// cumple, esto devuelve error y el servicio NO arranca: servir con las
	// políticas desactivadas sería peor que no servir.
	//
	// Va aquí, tras las migraciones y antes de tocar la base para nada más.
	modo, err := st.ResolveAppRole(ctx)
	if err != nil {
		return fmt.Errorf("aislamiento por organización: %w", err)
	}
	logger.Info("aislamiento por organización", "modo", modo)

	// Catálogo declarativo (OLA 3): upsert de las plantillas Tier-1 embebidas al
	// arrancar. Idempotente; una plantilla mal escrita ya la habría rechazado el
	// validador del manifiesto en seeds.Load (fallo de build/arranque, no silencioso).
	if err := seedCatalog(ctx, st); err != nil {
		return fmt.Errorf("sembrar catálogo: %w", err)
	}

	// Limitador de egress compartido entre réplicas (clave org:instance).
	egressLimiter := pg.NewRateLimiter(pool, pg.RateLimiterConfig{
		PerSecond: cfg.EgressRatePerSec,
		Burst:     cfg.EgressBurst,
	})

	// Motor de gobernanza L5 (DLP/política de egress): evalúa allow|warn|block en el
	// único RoundTripper de cada egress, con las reglas de la org cacheadas por TTL.
	// Se enchufa en TODA fabricación de Runtime (HTTP y sync) ⇒ imposible de esquivar.
	governanceEngine := policy.NewEngine(st, cfg.PolicyCacheTTL, logger)

	// Cliente de LECTURA del directorio de Sytue ID (el "estado deseado" del
	// reconcile SCIM saliente): client_credentials como `sytue-gateway`. Sytue ID es
	// interno, así que usa un http.Client normal (no el Runtime.HTTP de egress).
	dirClient := directory.New(directory.Config{
		BaseURL:      cfg.IDBaseURL,
		ClientID:     cfg.IDClientID,
		ClientSecret: cfg.IDClientSecret,
	})

	// Registro de drivers, compartido con los tests (internal/drivers). El despacho es
	// por CAPACIDAD: el manifiesto de cada plantilla nombra el Kind que sirve cada una,
	// así que una instancia puede aprovisionar con el driver SCIM y traer datos con el
	// suyo, cada uno con su credencial.
	registry := drivers.Registry(dirClient, st)

	router := integrationshttp.NewRouter(integrationshttp.Deps{
		Cfg:      cfg,
		Verifier: verifier,
		Store:    st,
		Registry: registry,
		Limiter:  egressLimiter,
		Audit:    st,
		Engine:   governanceEngine,
		// La materialización 1-click da de alta la identidad SALIENTE (SP SAML / client
		// OIDC) en Sytue ID con el MISMO cliente client_credentials de `sytue-gateway`.
		IDApps: dirClient,
		// Mismo cliente para el asistente automático de migración: emparejar cada
		// cuenta del origen con su cuenta de Sytue es leer el directorio de la org.
		Directory: dirClient,
		// Misma KEK que sella los secretos de instancia: el `state` del consentimiento
		// OAuth viaja cifrado con ella (ver internal/consent).
		Cipher: cipher,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
		},
		Fondo: ctx,
	})

	// Relay del outbox: procesa los trabajos de reconciliación SCIM saliente que
	// encola POST /instances/{id}/sync, con backoff/attempts (exactly-once por
	// FOR UPDATE SKIP LOCKED entre réplicas).
	//
	// Dos colas: la reconciliación SCIM y las migraciones. Con una sola, una
	// migración de horas dejaba sin sincronizar el directorio de TODAS las
	// organizaciones mientras duraba (NEG-20). La de migraciones reserva el evento
	// más tiempo: con los 2 min por defecto, una migración larga se volvía a
	// reclamar a mitad y corría dos veces a la vez.
	handler := outboxHandler(st, registry, dirClient, egressLimiter, governanceEngine, cfg)
	syncDispatcher := events.NewDispatcher(pool, handler, events.DispatcherConfig{
		Channels:    []events.Channel{store.SyncChannel},
		BatchSize:   20,
		MaxAttempts: 8,
		Types:       []string{store.SyncRequestedType},
	})
	migrationDispatcher := events.NewDispatcher(pool, handler, events.DispatcherConfig{
		Channels:      []events.Channel{store.SyncChannel},
		BatchSize:     1,
		MaxAttempts:   8,
		LeaseDuration: 6 * time.Hour,
		Types:         []string{store.MigrationRequestedType},
	})
	go func() {
		if err := syncDispatcher.Run(ctx); err != nil {
			logger.Error("dispatcher de sync SCIM detenido", "error", err)
		}
	}()
	go func() {
		if err := migrationDispatcher.Run(ctx); err != nil {
			logger.Error("dispatcher de migraciones detenido", "error", err)
		}
	}()
	go runSyncSweep(ctx, st, logger, cfg.SyncSweepInterval)

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
		// El alta de WhatsApp tiene una excepción de contexto de60s; las demás rutas siguen en30s.
		WriteTimeout: 65 * time.Second,
	})

	logger.Info("Sytue Integration Gateway arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL)
	return srv.Run(ctx)
}

// outboxHandler despacha los trabajos asíncronos del gateway por tipo de evento:
// la reconciliación SCIM saliente y la migración de contenido desde Microsoft 365 /
// Google Workspace. Un tipo desconocido se da por procesado.
func outboxHandler(st *store.Store, reg *connector.Registry, dir *directory.Client, limiter httpx.RateLimiter, engine *policy.Engine, cfg config.Config) events.Handler {
	provision := provisionSyncHandler(st, reg, limiter, engine, cfg)
	migration := contentMigrationHandler(st, dir, limiter, engine, cfg)
	return func(ctx context.Context, ev events.Event) error {
		switch ev.Type {
		case store.SyncRequestedType:
			return provision(ctx, ev)
		case store.MigrationRequestedType:
			return migration(ctx, ev)
		default:
			return nil
		}
	}
}

// contentMigrationHandler ejecuta (o reanuda) un trabajo de migración: reconstruye
// el contexto por-tenant desde el evento (RLS), MINTEA el permiso para escribir en
// la cuenta del usuario objetivo, fabrica el origen con las credenciales de la
// instancia y el Runtime de egress, y le pasa el trabajo al runner. Un error se
// propaga para que el Dispatcher reintente con backoff: el cursor y el mapa hacen
// que el reintento CONTINÚE, no que repita.
//
// Los fallos que un reintento NO puede arreglar (sin delegación aprovisionada, la
// persona destino ya no es impersonable) cierran el trabajo en 'error' con motivo
// legible y devuelven nil: ni martillean Sytue ID ni arrastran al resto de personas
// de la misma tanda, que van en trabajos independientes.
func contentMigrationHandler(st *store.Store, dir *directory.Client, limiter httpx.RateLimiter, engine *policy.Engine, cfg config.Config) events.Handler {
	return func(ctx context.Context, ev events.Event) error {
		var ref store.MigrationJobRef
		if err := json.Unmarshal(ev.Payload, &ref); err != nil {
			return fmt.Errorf("payload de migración inválido: %w", err)
		}
		if ev.Tenant == "" || ref.JobID == "" {
			return fmt.Errorf("evento de migración incompleto (tenant/job)")
		}
		tctx := log.Into(tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant}), log.From(ctx))

		job, err := st.GetMigrationJob(tctx, ref.JobID)
		if err != nil {
			return fmt.Errorf("cargar trabajo de migración %s: %w", ref.JobID, err)
		}
		if job.Status == domain.MigrationCanceled || job.Status == domain.MigrationDone {
			return nil // cancelado o ya completo: nada que hacer.
		}
		// La salida PROPIA no la ejecuta el worker: lee la entrega de Sytue con el token
		// de la persona, que aquí no hay (ver LaunchSelfOutboundJob).
		if job.Direction == domain.DirectionOut && job.SourceUser == "" {
			return st.FinishMigrationJob(tctx, job.ID, domain.MigrationError,
				"esta copia la lanza la propia persona: vuelve a lanzarla desde Migraciones")
		}

		// Cada tipo de contenido escribe en el servicio que le toca, y cada uno
		// verifica SU audiencia: la construcción de destinos vive aparte para que
		// este handler siga contando una sola historia.
		deps, reason := buildDestinations(tctx, st, dir, cfg, job, ev.Tenant)
		if reason != "" {
			return st.FinishMigrationJob(tctx, job.ID, domain.MigrationError, reason)
		}

		inst, err := st.LoadInstanceWithSecrets(tctx, job.InstanceID)
		if err != nil {
			return st.FinishMigrationJob(tctx, job.ID, domain.MigrationError,
				fmt.Sprintf("no se pudo cargar el conector de origen: %v", err))
		}
		rt := policy.BuildRuntime(policy.RuntimeParams{
			OrgID:        inst.OrgID,
			InstanceID:   inst.ID,
			Bearer:       inst.Secrets[store.SecretKeyBearer],
			Limiter:      limiter,
			Audit:        st,
			Policy:       engine,
			APIFamily:    inst.TemplateKey,
			Log:          log.From(ctx).With("migration", job.ID, "org", inst.OrgID),
			Timeout:      cfg.MigrationEgressTimeout,
			MaxRedirects: cfg.EgressMaxRedirects,
		})
		// SALIDA: se lee de Sytue y se escribe en el tercero, así que origen y
		// destino se intercambian. El motor y sus garantías son los mismos.
		if job.Direction == domain.DirectionOut {
			dst, err := migrate.NewOutboundDestination(tctx, rt, inst, job)
			if err != nil {
				return st.FinishMigrationJob(tctx, job.ID, domain.MigrationError, err.Error())
			}
			deps.Dst = dst
			return migrate.Run(tctx, deps, job)
		}

		src, err := migrate.NewSource(tctx, rt, inst, job)
		if err != nil {
			// Credenciales/config del origen mal puestas: reintentar no lo arregla.
			// Se cierra con motivo legible; el admin corrige y reanuda por el cursor.
			return st.FinishMigrationJob(tctx, job.ID, domain.MigrationError, err.Error())
		}
		deps.Src = src
		return migrate.Run(tctx, deps, job)
	}
}

// orgPeople traduce un correo del origen a la persona de Sytue que le corresponde,
// leyendo el directorio de la organización UNA vez por trabajo (no una consulta por
// miembro: un equipo grande dispararía cientos de llamadas a Sytue ID).
//
// El emparejamiento es por correo exacto: parecerse no basta cuando el resultado es
// meter a alguien en un canal que no era el suyo.
type orgPeople struct {
	dir    *directory.Client
	org    string
	once   sync.Once
	byMail map[string]directory.User
	err    error
}

func (p *orgPeople) Subject(ctx context.Context, email string) (string, string, bool, error) {
	p.once.Do(func() {
		users, err := p.dir.ListUsers(ctx, p.org)
		if err != nil {
			p.err = fmt.Errorf("no se ha podido leer el directorio de la organización: %w", err)
			return
		}
		p.byMail = make(map[string]directory.User, len(users))
		for _, u := range users {
			p.byMail[strings.ToLower(strings.TrimSpace(u.Email))] = u
		}
	})
	if p.err != nil {
		return "", "", false, p.err
	}
	u, ok := p.byMail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return "", "", false, nil
	}
	return u.ID, u.Name, true, nil
}

// buildDestinations arma los destinos de un trabajo según su tipo de contenido.
// Devuelve el motivo LEGIBLE si el despliegue no puede recibirlo (un servicio sin
// configurar, un permiso de delegación que no se puede canjear): eso no es un
// fallo transitorio y el trabajo se cierra con ese texto en vez de reintentar.
func buildDestinations(
	ctx context.Context, st *store.Store, dir *directory.Client,
	cfg config.Config, job domain.MigrationJob, tenant string,
) (migrate.Deps, string) {
	deps := migrate.Deps{Store: st}

	if job.Direction == domain.DirectionOut {
		return outboundSource(ctx, st, cfg, job)
	}

	switch job.Domain {
	case domain.DomainTeams:
		// Los equipos aterrizan en Sytue Rooms, no en Drive: se escribe con el token
		// de quien lanza la migración y las membresías se resuelven contra el
		// directorio (Rooms identifica a cada cual por su sujeto, no por su correo).
		if cfg.RoomsBaseURL == "" {
			return deps, "este despliegue no tiene Sytue Rooms configurado, así que no puede recibir equipos"
		}
		token, err := destinationToken(ctx, st, cfg, job)
		if err != nil {
			return deps, err.Error()
		}
		deps.Chat = migrate.NewSytueRooms(cfg.RoomsBaseURL, token, nil)
		deps.People = &orgPeople{dir: dir, org: tenant}
	case domain.DomainMail:
		// El buzón va a Sytue Mail por su JMAP público, con un token acotado a esa
		// persona: un correo importado hereda sus cuotas y su auditoría.
		if cfg.MailBaseURL == "" {
			return deps, "este despliegue no tiene Sytue Mail configurado, así que no puede recibir correo"
		}
		token, err := destinationToken(ctx, st, cfg, job)
		if err != nil {
			return deps, err.Error()
		}
		deps.Mail = migrate.NewSytueMail(cfg.MailBaseURL, token, nil)
	case domain.DomainCalendar:
		// La agenda va a Sytue Calendar, que habla iCalendar: las series se crean
		// como series, no como cientos de eventos sueltos.
		if cfg.CalendarBaseURL == "" {
			return deps, "este despliegue no tiene Sytue Calendar configurado, así que no puede recibir calendarios"
		}
		token, err := destinationToken(ctx, st, cfg, job)
		if err != nil {
			return deps, err.Error()
		}
		deps.Cal = migrate.NewSytueCalendar(cfg.CalendarBaseURL, token, nil)
	case domain.DomainCloud:
		return cloudDestination(ctx, st, cfg, job)
	case domain.DomainRepos:
		// Los repositorios no aterrizan en Drive: van a la forja, que los importa
		// enteros. Su permiso no es un token de impersonación de la suite, sino el
		// token de administrador de Sytue Git más `Sudo` con la persona destino.
		if cfg.GitBaseURL == "" || cfg.GitToken == "" {
			return deps, "este despliegue no tiene Sytue Git configurado, así que no puede recibir repositorios"
		}
		deps.Git = migrate.NewSytueGit(cfg.GitBaseURL, cfg.GitToken, nil)
	default:
		token, err := destinationToken(ctx, st, cfg, job)
		if err != nil {
			return deps, err.Error()
		}
		deps.Dst = migrate.NewSytue(cfg.DriveBaseURL, cfg.ContactsBaseURL, token, nil)
		// Los documentos nativos del origen van a Sytue Docs, que verifica su PROPIA
		// audiencia: hace falta un segundo token acotado a esa persona.
		if cfg.DocsBaseURL != "" && (job.Domain == domain.DomainDrive || job.Domain == domain.DomainTeamFiles) {
			docsToken, err := delegatedToken(ctx, st, cfg, job.TargetUser, "sytue-docs")
			if err != nil {
				return deps, err.Error()
			}
			deps.Docs = migrate.NewSytueDocs(cfg.DocsBaseURL, docsToken, nil)
		}
	}
	return deps, ""
}

// destinationToken devuelve la fuente de Bearer con la que se escribe EN LA CUENTA
// del usuario objetivo del trabajo: un token de impersonación de Sytue ID (RFC
// 8693) minteado con el client delegable de la organización.
//
// Se hace un primer canje AQUÍ, antes de arrancar, para fallar pronto y con un
// motivo que el admin pueda leer en el informe (persona dada de baja, delegación
// sin aprovisionar) en vez de a mitad de la copia. A partir de ahí, la fuente se
// renueva sola: el token vive 5 minutos y el trabajo, mucho más.
// outboundSource arma el ORIGEN de un trabajo de salida: se lee de Sytue con un
// token acotado a la persona que se lleva lo suyo. El destino (el tercero) lo
// fabrica el worker, que es quien tiene el runtime de egreso.
func outboundSource(
	ctx context.Context, st *store.Store, cfg config.Config, job domain.MigrationJob,
) (migrate.Deps, string) {
	deps := migrate.Deps{Store: st}
	if job.Domain != domain.DomainDrive && job.Domain != domain.DomainContacts {
		return deps, "de momento solo se pueden llevar fuera los ficheros y los contactos; " +
			"el correo y la agenda salen por sus formatos estándar (mbox e iCalendar) desde sus propias aplicaciones"
	}
	token, err := destinationToken(ctx, st, cfg, job)
	if err != nil {
		return deps, err.Error()
	}
	deps.Src = migrate.NewSytueSource(cfg.DriveBaseURL, cfg.ContactsBaseURL, token, nil)
	return deps, ""
}

// cloudDestination arma el destino de las aplicaciones. Se escribe con un token
// acotado a la persona destino, así que la migración hereda su rol en la
// organización de Cloud y la cuota de su plan: si no puede crear un proyecto a
// mano, tampoco lo crea un asistente por la puerta de atrás.
func cloudDestination(
	ctx context.Context, st *store.Store, cfg config.Config, job domain.MigrationJob,
) (migrate.Deps, string) {
	deps := migrate.Deps{Store: st}
	if cfg.CloudBaseURL == "" {
		return deps, "este despliegue no tiene Sytue Cloud configurado, así que no puede recibir aplicaciones"
	}
	token, err := destinationToken(ctx, st, cfg, job)
	if err != nil {
		return deps, err.Error()
	}
	deps.Cloud = migrate.NewSytueCloud(cfg.CloudBaseURL, token, nil)
	return deps, ""
}

func destinationToken(ctx context.Context, st *store.Store, cfg config.Config, job domain.MigrationJob) (migrate.TokenSource, error) {
	del, err := st.DelegatedClient(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf(
				"esta organización aún no puede migrar a la cuenta de cada persona: falta que el operador de Sytue aprovisione su permiso de delegación",
			)
		}
		return nil, fmt.Errorf("no se ha podido leer el permiso de delegación de la organización: %w", err)
	}
	// El token canjeado debe declarar como audiencia el client de la suite del
	// servicio destino: es lo que su verificador acepta (OIDC_ACCEPTED_AUDIENCES).
	// Drive es el destino por defecto: ahí aterrizan los ficheros, los gists y el
	// contenido de los canales de Teams.
	audience := "sytue-drive"
	switch job.Domain {
	case domain.DomainContacts:
		audience = "sytue-contacts"
	case domain.DomainTeams:
		audience = "sytue-rooms"
	case domain.DomainMail:
		audience = "sytue-mail"
	case domain.DomainCalendar:
		audience = "sytue-calendar"
	case domain.DomainCloud:
		audience = "sytue-cloud-api"
	case domain.DomainDrive, domain.DomainGists, domain.DomainTeamFiles, domain.DomainRepos:
		// sytue-drive (los repositorios no pasan por aquí: su destino es la forja).
	}
	return newDelegation(ctx, del, cfg, job.TargetUser, audience)
}

// delegatedToken mintea un token de impersonación para OTRO servicio de la suite
// (un trabajo puede escribir en dos: los ficheros van a Drive y los documentos
// nativos, a Docs, y cada uno verifica su propia audiencia).
func delegatedToken(ctx context.Context, st *store.Store, cfg config.Config, subject, audience string) (migrate.TokenSource, error) {
	del, err := st.DelegatedClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se ha podido leer el permiso de delegación de la organización: %w", err)
	}
	return newDelegation(ctx, del, cfg, subject, audience)
}

// newDelegation canja el primer token (para fallar pronto y con motivo legible) y
// devuelve la fuente que lo renueva sola durante el trabajo.
func newDelegation(ctx context.Context, del store.Delegate, cfg config.Config, subject, audience string) (migrate.TokenSource, error) {
	d := directory.NewDelegation(directory.DelegationConfig{
		BaseURL:      cfg.IDBaseURL,
		ClientID:     del.ClientID,
		ClientSecret: del.Secret,
		Subject:      subject,
		Audience:     audience,
	})
	if _, err := d.Token(ctx); err != nil {
		return nil, err
	}
	return d.Token, nil
}

// provisionSyncHandler es el handler del outbox que ejecuta una reconciliación SCIM
// saliente: reconstruye el contexto por-tenant desde el evento (así el store aísla
// por org bajo RLS), carga la instancia con sus secretos, fabrica el Runtime (único
// cliente de egress, con el Bearer del SaaS inyectado) y despacha la capacidad
// Provisioner del driver. Un error se propaga para que el Dispatcher reintente.
func provisionSyncHandler(st *store.Store, reg *connector.Registry, limiter httpx.RateLimiter, engine *policy.Engine, cfg config.Config) events.Handler {
	return func(ctx context.Context, ev events.Event) error {
		var job store.SyncJob
		if err := json.Unmarshal(ev.Payload, &job); err != nil {
			return fmt.Errorf("payload de sync inválido: %w", err)
		}
		if ev.Tenant == "" || job.InstanceID == "" {
			return fmt.Errorf("evento de sync incompleto (tenant/instance)")
		}
		// Contexto por-tenant (RLS) conservando el logger de la petición del dispatcher.
		tctx := log.Into(tenancy.Into(ctx, tenancy.Tenant{Slug: ev.Tenant}), log.From(ctx))

		inst, err := st.LoadInstanceWithSecrets(tctx, job.InstanceID)
		if err != nil {
			return fmt.Errorf("cargar instancia %s: %w", job.InstanceID, err)
		}
		// El driver del aprovisionamiento se resuelve por CAPACIDAD, no por la clave de
		// la plantilla: es un driver distinto del de datos y con su propia config (la
		// raíz SCIM del manifiesto, resuelta contra los valores del admin).
		tpl, err := st.GetTemplate(tctx, inst.TemplateKey)
		if err != nil {
			return fmt.Errorf("cargar plantilla %q: %w", inst.TemplateKey, err)
		}
		m, err := manifest.Parse(tpl.Manifest)
		if err != nil {
			return fmt.Errorf("plantilla %q inválida: %w", inst.TemplateKey, err)
		}
		kind := m.DriverFor(manifest.CapProvision)
		if kind == "" {
			return fmt.Errorf("la plantilla %q no ofrece aprovisionamiento", inst.TemplateKey)
		}
		drv, ok := reg.Get(kind)
		if !ok {
			return fmt.Errorf("sin driver %q para el aprovisionamiento de %q", kind, inst.TemplateKey)
		}
		prov, ok := drv.(connector.Provisioner)
		if !ok {
			return fmt.Errorf("el driver %q no soporta aprovisionamiento", kind)
		}
		capCfg, err := m.CapabilityConfig(manifest.CapProvision, inst.Config)
		if err != nil {
			return fmt.Errorf("config de aprovisionamiento de %q: %w", inst.TemplateKey, err)
		}
		if len(capCfg) > 0 && inst.Config == nil {
			inst.Config = make(map[string]string, len(capCfg))
		}
		for k, v := range capCfg {
			inst.Config[k] = v
		}
		rt := policy.BuildRuntime(policy.RuntimeParams{
			OrgID:        inst.OrgID,
			InstanceID:   inst.ID,
			Bearer:       inst.Secrets[store.SecretKeyBearer],
			Limiter:      limiter,
			Audit:        st,
			Policy:       engine,
			APIFamily:    inst.TemplateKey,
			Log:          log.From(ctx).With("instance", inst.ID, "org", inst.OrgID),
			Timeout:      cfg.EgressTimeout,
			MaxRedirects: cfg.EgressMaxRedirects,
		})
		// El driver recibe la instancia SIN secretos (seguridad por construcción).
		driverInst := inst
		driverInst.Secrets = nil
		return prov.Sync(tctx, driverInst, rt)
	}
}

// seedCatalog hace upsert de las plantillas Tier-1 embebidas (catálogo declarativo).
// Idempotente. Un fallo de carga/validación de una semilla es fatal (contenido de
// build); un fallo de upsert en BD se registra pero no impide arrancar (el catálogo
// puede quedar temporalmente obsoleto, no rompe el servicio).
func seedCatalog(ctx context.Context, st *store.Store) error {
	list, err := seeds.Load()
	if err != nil {
		return err
	}
	ok := 0
	for _, s := range list {
		if err := st.UpsertTemplate(ctx, s.Key, s.Manifest, s.Tier); err != nil {
			log.From(ctx).Warn("no se pudo sembrar la plantilla", "template", s.Key, "error", err)
			continue
		}
		ok++
	}
	log.From(ctx).Info("catálogo sembrado", "plantillas", ok, "total", len(list))
	return nil
}

// runMigrate aplica las migraciones y sale (uso operativo / Job de despliegue).
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return pg.MigrateUp(integrationsdb.Migrations, integrationsdb.MigrationsDir, cfg.DatabaseURL)
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/integrations","healthcheck"]).
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8093"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// runSyncSweep encola periódicamente una reconciliación SCIM para cada conexión activa
// que aprovisione, en todas las organizaciones.
//
// Es el RELOJ que le faltaba al desaprovisionamiento. La maquinaria de reconcile ya
// desactivaba en el SaaS a quien ya no está en Sytue, pero solo se encolaba al activar la
// conexión y cuando un administrador pulsaba «Sincronizar»: dar de baja a alguien en
// Sytue ID no llegaba al SaaS del cliente hasta que a alguien se le ocurriera hacerlo a
// mano. Un control de seguridad que no se dispara solo es un control que no existe.
//
// El outbox ya garantiza exactly-once entre réplicas (FOR UPDATE SKIP LOCKED) y aplica
// backoff, así que varias réplicas barriendo a la vez no duplican trabajo: encolar de
// más es barato y el dispatcher lo serializa.
func runSyncSweep(ctx context.Context, st *store.Store, logger *slog.Logger, every time.Duration) {
	if every <= 0 {
		logger.Info("barrido de sync SCIM desactivado por configuración")
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cands, err := st.ListSyncCandidates(ctx, 0)
			if err != nil {
				logger.Error("barrido de sync SCIM: no se pudieron enumerar las conexiones", "error", err)
				continue
			}
			var queued, failed int
			for _, c := range cands {
				// EnqueueSync valida bajo la RLS de la organización, así que el encolado
				// va con el tenant de CADA conexión. El barrido solo decide a quién le
				// toca; no se salta el aislamiento para escribir.
				if err := st.EnqueueSync(tenancy.Into(ctx, tenancy.Tenant{Slug: c.OrgID}), c.ID); err != nil {
					failed++
					continue
				}
				queued++
			}
			if failed > 0 {
				logger.Warn("barrido de sync SCIM", "encoladas", queued, "fallidas", failed)
			} else if queued > 0 {
				logger.Info("barrido de sync SCIM", "encoladas", queued)
			}
		}
	}
}
