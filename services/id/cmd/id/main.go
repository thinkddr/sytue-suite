// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command id arranca Sytue ID, el proveedor de identidad nuclear de Sytue One
// (OIDC/OAuth2/SCIM/MFA). Escucha en :8081 por defecto (docs/CONVENTIONS.md §6).
//
// Subcomandos:
//
//	id                                arranca el servidor HTTP (por defecto)
//	id healthcheck                    sonda de liveness para Docker
//	id seed                           usuario/organización de desarrollo (solo dev)
//	id delegated-client <org-slug>    aprovisiona el client DELEGABLE de una org
//	id revoke-all                     invalida YA todos los tokens emitidos
//	                                  (rotación de clave con retirada inmediata)
//	id rename-org <actual> <nuevo>    cambia el slug de una org y reetiqueta sus
//	                                  datos en las nueve bases de servicio
//	                                  (operación de plataforma, ver delegate.go)
//
// AVISO DE SEGURIDAD: este servicio es la pieza de autenticación crítica de la
// suite. DEBE superar una auditoría de seguridad independiente ANTES de
// producción (docs/CONVENTIONS.md §0.7, §8, §10).
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
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/auditsink"
	idhttp "git.sytue.eu/sytue/one/services/id/internal/http"
	"git.sytue.eu/sytue/one/services/id/internal/mailprov"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func main() {
	// Subcomando healthcheck (usado por el HEALTHCHECK de Docker): consulta
	// /healthz en el puerto local y devuelve 0 si el servicio está sano.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	// Subcomando seed (solo dev): crea de forma idempotente una organización y
	// un usuario admin de desarrollo para permitir el LOGIN en local. Aborta
	// fuera de dev (ver seed.go).
	if len(os.Args) > 1 && (os.Args[1] == "seed" || os.Args[1] == "seed-dev") {
		os.Exit(runSeed())
	}
	// Subcomando delegated-client (operación de PLATAFORMA): aprovisiona el client
	// delegable de una organización — el único camino por el que may_impersonate se
	// activa, deliberadamente fuera de toda API. Ver delegate.go.
	if len(os.Args) > 1 && os.Args[1] == "delegated-client" {
		os.Exit(runDelegatedClient(os.Args[2:]))
	}
	// Subcomando reconcile-client-secrets (operación de DESPLIEGUE, todos los
	// entornos): activa los clients confidenciales de las superficies propias
	// fijando su secret_hash. Sin él quedan deshabilitados por deny-by-default y
	// el token endpoint responde invalid_client. Ver clientsecrets.go.
	if len(os.Args) > 1 && os.Args[1] == "reconcile-client-secrets" {
		os.Exit(runReconcileClientSecrets(os.Args[2:]))
	}
	// Subcomando revoke-all (EMERGENCIA, fuera de banda): invalida en el acto
	// todos los tokens emitidos rotando la clave de firma y retirando la anterior
	// sin ventana de gracia. No acorta el TTL: lo elimina. Ver emergency.go.
	if len(os.Args) > 1 && os.Args[1] == "revoke-all" {
		os.Exit(runRevokeAll(os.Args[2:]))
	}
	// Subcomando lockdown (EMERGENCIA, fuera de banda): cierra la EMISIÓN —abrir
	// sesión y entregar tokens— con una marca en la base que se consulta en cada
	// emisión. Es lo que impide que quien tenga la credencial deshaga el corte
	// entrando otra vez. El rescate (`lockdown off`) vive aquí porque con el
	// bloqueo puesto no hay consola a la que entrar. Ver lockdown.go.
	if len(os.Args) > 1 && os.Args[1] == "lockdown" {
		os.Exit(runLockdown(os.Args[1:]))
	}
	// Subcomando rename-org (operación de PLATAFORMA): cambia el slug de una
	// organización reetiquetando también sus datos en las nueve bases de
	// servicio. El slug es el tenant de toda la suite, no una etiqueta: por eso
	// la Console lo declara inmutable y esto vive en el binario. Ver renameorg.go.
	if len(os.Args) > 1 && os.Args[1] == "rename-org" {
		os.Exit(runRenameOrg(os.Args[2:]))
	}
	// Subcomando bootstrap-owner (operación de DESPLIEGUE, idempotente): crea la
	// organización que opera la plataforma, su propietario y su inscripción en
	// platform_admins. Sytue ID no tiene registro público, así que sin esto una
	// producción nueva no tiene NINGÚN usuario. Ver bootstrapowner.go.
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-owner" {
		os.Exit(runBootstrapOwner())
	}
	// Subcomando verify-access (operación de DESPLIEGUE, la ejecuta 04-verificar):
	// comprueba que se puede ENTRAR de verdad —login con contraseña y segundo
	// factor, y ronda OIDC completa en cada superficie— y que la consola global
	// autoriza a un admin de plataforma y deniega al owner de una organización
	// cliente. Usa dos cuentas efímeras que borra al terminar. Ver verifyaccess.go.
	if len(os.Args) > 1 && os.Args[1] == "verify-access" {
		os.Exit(runVerifyAccess())
	}
	// os.Exit vive solo aquí: realMain devuelve el código de salida para que
	// sus defers (signal.NotifyContext) sí se ejecuten (gocritic exitAfterDefer).
	os.Exit(realMain())
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/id","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := idhttp.Load()
	// El sumidero de avisos y errores se conecta más abajo, cuando exista la
	// base. Se pasa la referencia ya aquí para que el logger sea uno solo: crear
	// otro después dejaría fuera de la pantalla justo los fallos del arranque,
	// que son los que peor se diagnostican a ciegas.
	capturados := &log.SinkRef{}
	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: cfg.ServiceName,
		Version: cfg.Version,
		Capture: capturados,
	})
	ctx = log.Into(ctx, logger)
	if err != nil {
		logger.Error("configuración inválida", "error", err)
		return 1
	}

	if err := run(ctx, cfg, logger, capturados); err != nil {
		logger.Error("fallo al ejecutar sytue-id", "error", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, cfg idhttp.Config, logger *slog.Logger, capturados *log.SinkRef) error {
	// Telemetría. El IdP no la tenía, y es el servicio por el que pasa CADA
	// inicio de sesión de la suite: sin esto no hay forma de saber cuántas
	// peticiones atiende ni con qué latencia, que es justo lo que tiene que
	// decidir cuándo escalar.
	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: cfg.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	// `context.Background()` y NO el ctx de run: cuando esto corre, aquel ya está
	// cancelado —es lo que ha disparado el apagado— y el exportador se marcharía
	// sin vaciar los datos pendientes, que son justamente los del cierre.
	defer func() { _ = tp.Shutdown(context.Background()) }() //nolint:contextcheck // ver arriba: el ctx de run ya está cancelado aquí.
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// Migraciones al arrancar (idempotentes).
	if err := pg.MigrateUp(iddb.Migrations, iddb.MigrationsDir, cfg.DatabaseDSN); err != nil {
		return err
	}
	if cfg.MatrixLifecycleSecret != "" {
		if err := events.Migrate(cfg.DatabaseDSN); err != nil {
			return err
		}
	}

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseDSN, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	// Cifrado en reposo de la clave privada de firma (F9). En prod la KEK es
	// OBLIGATORIA (Secret Manager/KMS); en dev se permite operar sin cifrado.
	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return err
	}
	if cfg.Env.IsProd() && !cipher.Enabled() {
		return errors.New("SYTUE_ID_KEY_ENCRYPTION_KEY es obligatoria en prod (cifrado en reposo de la clave de firma)")
	}
	if !cipher.Enabled() {
		logger.Warn("clave de firma SIN cifrar en reposo: fija SYTUE_ID_KEY_ENCRYPTION_KEY (obligatorio fuera de dev)")
	}
	// Sin SMTP, el Mailer de dev escribe el enlace de reseteo en el LOG y devuelve
	// éxito: el usuario ve «te hemos enviado un correo», el correo no existe y
	// nada falla. En producción eso deja fuera a cualquiera que pierda su
	// contraseña y, con el mismo Mailer, tampoco salen las invitaciones ni las
	// verificaciones de dirección. Se exige configuración explícita.
	if cfg.Env.IsProd() && cfg.SMTPAddr == "" {
		return errors.New("SYTUE_ID_SMTP_ADDR es obligatoria en prod (reseteo de contraseña, invitaciones y verificación de correo)")
	}
	if cfg.SMTPAddr == "" {
		logger.Warn("SMTP sin configurar: los enlaces transaccionales SOLO se escriben en el log (dev)")
	}

	st := store.NewWithCipher(pool, cipher)
	var documentCalls callaccess.SubjectRevoker
	if client := callaccess.New(cfg.RoomsCallAccessURL, cfg.RoomsCallAccessSecret); client != nil {
		documentCalls = client
	}
	if err := st.ConfigureCallIdentity(ctx, cfg.EncryptedDocumentCalls, documentCalls); err != nil {
		return fmt.Errorf("document call identity guard: %w", err)
	}
	roomsAccess, err := idhttp.RoomsAccessGuard(cfg.RoomsAccessURL, cfg.MatrixLifecycleSecret)
	if err != nil {
		return err
	}
	st.SetExternalAccessGuard(roomsAccess)
	if roomsAccess.Apply != nil {
		dispatcher := events.NewDispatcher(pool, func(ctx context.Context, ev events.Event) error {
			var change store.MatrixAccessChange
			if err := json.Unmarshal(ev.Payload, &change); err != nil {
				return err
			}
			return roomsAccess.Apply(ctx, change)
		}, events.DispatcherConfig{
			Channels: []events.Channel{store.MatrixAccessChannel},
			Types:    []string{store.MatrixAccessActive}, PollInterval: time.Second,
			MaxAttempts: -1,
		})
		go func() { _ = dispatcher.Run(ctx) }()
	}
	// A partir de aquí los avisos y errores se guardan y la consola de
	// plataforma puede enseñarlos. Antes de esta línea se descartan, que es lo
	// correcto: no hay dónde escribirlos todavía.
	capturados.Set(sinkDeRegistro{st})

	// Claves de firma JWKS (genera la primera si no existe).
	km := oidc.NewKeyManager()
	if err := km.Bootstrap(ctx, st); err != nil {
		return err
	}
	issuer := oidc.NewIssuer(km, cfg.Issuer, cfg.ResourceAudience, cfg.AccessTokenTTL, cfg.IDTokenTTL)

	srv := idhttp.NewServer(cfg, logger, st, km, issuer)

	// Cache efímera compartida (Dragonfly): guarda el derecho de suscripción de cada
	// organización, que se pregunta en CADA emisión y refresco de token con una
	// consulta que cruza suscripción, miembros activos y asientos concedidos.
	//
	// DEGRADACIÓN: sin DRAGONFLY_URL, o si no se puede conectar al arrancar, el IdP
	// sigue consultando PostgreSQL igual que antes. Nunca depende de la cache para
	// arrancar: es la puerta de entrada de toda la suite y no puede quedar detrás de
	// un servicio que, por contrato, puede desaparecer entero.
	if cfg.DragonflyURL != "" {
		c, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
		if err != nil {
			logger.Warn("Dragonfly no disponible al arrancar; el derecho de suscripción se consulta en PostgreSQL hasta que reconecte", "error", err)
		}
		// Sin clave no se cachea: un derecho de suscripción sin sellar lo puede
		// poner cualquiera que escriba en Dragonfly, en los dos sentidos —regalar
		// la suite o dejar a una organización sin poder emitir tokens—.
		if cfg.CacheSigningKey == "" {
			logger.Warn("CACHE_SIGNING_KEY sin definir: el derecho de suscripción NO se cachea y se consulta contra PostgreSQL")
		} else {
			c.UsarFirma(cache.ClaveDeFirma([]byte(cfg.CacheSigningKey), "id"))
		}
		srv.UsarCache(c)
		defer func() { _ = c.Close() }()
	}

	// Limpieza periódica de tokens/retos caducados (sobre PostgreSQL, sin colas externas).
	go cleanupLoop(ctx, st, logger)

	// Recálculo periódico de los grupos DINÁMICOS de todas las orgs: repara la deriva que
	// ningún disparador puntual cubre (regla creada antes que los usuarios, atributos
	// cambiados por otra vía, bajas). En estado estable no escribe nada: la sincronización
	// es diferencial.
	go dynamicGroupsLoop(ctx, st, logger)
	go auditSinkLoop(ctx, st, logger)

	// Creación en Sytue Mail de los buzones que se asignan al dar de alta, para que
	// quien acaba de entrar pueda RECIBIR correo sin haber abierto todavía el
	// webmail (ver internal/mailprov).
	go mailboxProvisionLoop(ctx, st, cfg, issuer, logger)

	// @usuario para quien aún no lo tiene: las cuentas anteriores a la mig 0065 y
	// las que dé de alta un pod del binario anterior mientras dura el despliegue.
	go usernamesLoop(ctx, st, logger)

	// «Borrar mi cuenta» (ADR 0054): lo que otros servicios guardan de quien se
	// ha ido y, vencida la ventana de 30 días, la anonimización de su ficha.
	go cuentasBorradasLoop(ctx, srv, logger)

	// Rotación programada de la clave de firma JWKS con retiro tras la ventana de
	// gracia (F10). Deshabilitada si RotationInterval <= 0.
	if cfg.RotationInterval > 0 {
		go km.RotateLoop(ctx, st, cfg.RotationInterval, cfg.KeyRetireAfter, logger)
	}

	httpSrv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         srv.Router(),
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	// Mismo Secret que monta Contacts: sin él, la foto se guarda pero no se
	// refleja en las fichas de contacto, sin error en ningún sitio.
	if cfg.Env != pcfg.EnvDev && cfg.ContactsInternalSecret == "" {
		logger.Warn("falta SYTUE_ID_CONTACTS_INTERNAL_SECRET (Secret sytue-contacts-internal): la foto de perfil no se propaga a Sytue Contacts")
	}
	logger.Info("Sytue ID iniciado",
		"addr", cfg.HTTPAddr, "issuer", cfg.Issuer, "env", string(cfg.Env))
	// EL RIESGO ES REAL Y SIGUE ABIERTO, así que la línea no se borra: hay dos
	// auditorías INTERNAS hechas (docs/security.md §6.1 y §6.2, 22 hallazgos
	// corregidos) y la EXTERNA, que §0 declara obligatoria antes de producción,
	// sin hacer.
	//
	// Pero se dice como lo que es. `Warn` significa «algo va mal AHORA y puede
	// pedir acción»; esto es un estado conocido, aceptado y documentado que será
	// cierto en cada arranque hasta que se contrate la auditoría. Gritarlo cada
	// vez es lo que enseña a no leer los Warn, y entonces el que sí importa pasa
	// desapercibido. Queda en el registro de arranque, en producción —que es
	// donde el servicio está expuesto— y nombrando el documento, para que quien
	// lo lea a las tres de la mañana sepa si le toca a él o no.
	if cfg.Env.IsProd() {
		logger.Info("riesgo aceptado: falta la auditoría de seguridad EXTERNA de Sytue ID (docs/security.md §0, §6)")
	}

	if err := httpSrv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// auditSinkInterval es cada cuánto se empuja la auditoría al SIEM de cada organización.
// Un minuto: bastante fino para que un analista vea el evento casi en vivo, y bastante
// grueso para no martillear un endpoint ajeno cuando no hay nada que enviar (el barrido
// no hace ninguna petición si la cola está vacía).
const auditSinkInterval = time.Minute

// auditSinkLoop entrega periódicamente la auditoría pendiente a los SIEM configurados.
func auditSinkLoop(ctx context.Context, st *store.Store, logger *slog.Logger) {
	d := auditsink.New(st)
	ticker := time.NewTicker(auditSinkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.DispatchAll(ctx); err != nil && ctx.Err() == nil {
				// Cada fallo por organización ya queda anotado en su sink (last_error),
				// que es donde el admin lo ve; esto solo cubre el fallo del barrido.
				logger.Warn("envío de auditoría al SIEM falló", "error", err)
			}
		}
	}
}

// dynamicGroupsSweepInterval es la cadencia del recálculo de grupos dinámicos. Ventana
// máxima de obsolescencia de una membresía calculada cuando el cambio no pasó por un
// disparador (SCIM o perfil admin), que sí la actualizan al instante.
//
// ponytail: constante, no configuración — no hay caso de uso para afinarla y un admin con
// prisa tiene el botón «Recalcular ahora» (POST /admin/groups/recompute).
const dynamicGroupsSweepInterval = 15 * time.Minute

// dynamicGroupsLoop recomputa periódicamente la membresía de los grupos dinámicos.
func dynamicGroupsLoop(ctx context.Context, st *store.Store, logger *slog.Logger) {
	ticker := time.NewTicker(dynamicGroupsSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := st.RecomputeAllDynamicGroups(ctx)
			if err != nil {
				logger.Warn("recálculo de grupos dinámicos falló", "error", err)
			}
			if res.Added > 0 || res.Removed > 0 {
				logger.Info("grupos dinámicos recalculados",
					"orgs", res.Orgs, "grupos", res.Groups, "altas", res.Added, "bajas", res.Removed)
			}
		}
	}
}

// mailboxProvisionInterval es cada cuánto se miran los buzones pendientes de crear
// en Sytue Mail. Medio minuto: nadie escribe a alguien en el mismo minuto en que lo
// dan de alta, y el barrido no hace ninguna petición si la cola está vacía (índice
// parcial, ver la migración 0048).
const mailboxProvisionInterval = 30 * time.Second

// mailboxAudience es la audiencia que exige Sytue Mail en los tokens (su
// OIDC_AUDIENCE). Va escrita aquí y no importada del paquete http porque este es
// el arranque del proceso y no debe depender de las interioridades del servidor.
const mailboxAudience = "sytue-mail"

// mailboxProvisionLoop crea en Sytue Mail las cuentas de los buzones ya asignados.
//
// El token se emite a nombre del USUARIO y no de una identidad de servicio porque
// el camino que se recorre es el de la sesión JMAP, el mismo que seguiría esa
// persona al entrar por primera vez: así Sytue Mail crea exactamente la cuenta que
// crearía sola, sin una segunda puerta de aprovisionamiento que mantener. Vive un
// minuto y solo lleva scope de correo: es una llave para una cerradura y un rato.
func mailboxProvisionLoop(ctx context.Context, st *store.Store, cfg idhttp.Config, iss *oidc.Issuer, logger *slog.Logger) {
	prov := mailprov.New(st, cfg.MailAPIURL, func(subject, tenant, address, name string) (string, error) {
		return iss.IssueAccessToken(oidc.TokenClaims{
			Subject:  subject,
			Audience: mailboxAudience,
			// `email` es el scope bajo el que viajan las direcciones (ver
			// oidc.IssueAccessToken); sin él, Sytue Mail no vería con qué dirección
			// crear la cuenta y caería en la del registro.
			Scope:         "mail email profile",
			Tenant:        tenant,
			Email:         address,
			Mailbox:       address,
			EmailVerified: true,
			Name:          name,
			TTL:           time.Minute,
		}, time.Now())
	}, logger)
	if cfg.HubInternalSecret != "" {
		prov.Hub = hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	}
	if !prov.Enabled() {
		logger.Warn("sin URL de Sytue Mail: los buzones se asignan pero no se crean allí")
		return
	}
	ticker := time.NewTicker(mailboxProvisionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := prov.Run(ctx)
			if err != nil && ctx.Err() == nil {
				logger.Warn("barrido de buzones pendientes falló", "error", err)
			}
			if n > 0 {
				logger.Info("buzones creados en Sytue Mail", "buzones", n)
			}
		}
	}
}

// cuentasBorradasLoop termina las bajas propias pendientes al arrancar y cada
// diez minutos (ver idhttp.BarrerCuentasBorradas). Sin nada pendiente es una
// consulta sobre un índice parcial vacío.
func cuentasBorradasLoop(ctx context.Context, srv *idhttp.Server, logger *slog.Logger) {
	ticker := time.NewTicker(idhttp.BarridoCuentasBorradas)
	defer ticker.Stop()
	for {
		if err := srv.BarrerCuentasBorradas(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("borrar mi cuenta: el barrido falló", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// usernamesLoop asigna los @usuario pendientes al arrancar —vaciando la cola por
// lotes— y vuelve a mirar cada diez minutos. En régimen normal no hay nada que
// hacer y el barrido es una consulta sobre un índice parcial vacío.
func usernamesLoop(ctx context.Context, st *store.Store, logger *slog.Logger) {
	const lote = 200
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		for {
			n, err := st.AsignarUsernamesPendientes(ctx, lote)
			if err != nil && ctx.Err() == nil {
				logger.Warn("asignar @usuario pendientes falló", "error", err)
			}
			if n > 0 {
				logger.Info("@usuario asignados", "personas", n)
			}
			if err != nil || n < lote {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// cleanupLoop borra periódicamente códigos/retos caducados.
func cleanupLoop(ctx context.Context, st *store.Store, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := st.CleanupExpired(ctx, time.Now()); err != nil {
				logger.Warn("limpieza de expirados falló", "error", err)
			}
		}
	}
}

// sinkDeRegistro adapta el store a la interfaz del capturador de logs.
//
// Es un envoltorio de una línea y no un método del store para que la
// persistencia no importe el paquete de logging: el store sabe guardar filas y
// el logger sabe capturarlas, y quien los presenta es este arranque.
type sinkDeRegistro struct{ st *store.Store }

func (s sinkDeRegistro) Write(ctx context.Context, batch []log.Entry) error {
	return s.st.WriteServiceLogs(ctx, batch)
}
