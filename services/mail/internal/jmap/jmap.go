// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package jmap implementa la API JMAP propia de Sytue Mail (RFC 8620 JMAP core
// + RFC 8621 JMAP for Mail) que consume el frontend Astro/Preact (paquete
// @sytue/api-clients). Es la API moderna de cara al cliente web; IMAP/SMTP
// quedan para clientes de terceros.
//
// Autenticación: Bearer JWT de Sytue ID validado por el middleware authn de
// platform (JWKS). Todas las operaciones corren bajo el tenant del token (RLS).
//
// Clean-room sobre los RFC de JMAP; sin código de terceros AGPL. Por pragmatismo
// se ofrece además un conjunto de endpoints REST de conveniencia (mismo modelo)
// que el SDK del frontend usa directamente; el endpoint /jmap implementa el
// sobre de peticiones por lotes (Request/Response) del estándar.
package jmap

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/push"
	"git.sytue.eu/sytue/one/services/mail/internal/smime"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// dataStore es la superficie mínima de persistencia que consumen los handlers
// JMAP/REST. La satisface *store.Store (producción) y un fake en tests, de modo
// que los handlers se ejercitan sin PostgreSQL. No cambia la firma pública de New.
type dataStore interface {
	Dominios(ctx context.Context) ([]string, error)
	EnsureAccount(ctx context.Context, subject, address, displayName string, quotaBytes int64) (domain.Account, error)
	EnsureAccountConCuota(ctx context.Context, subject, address, displayName string, quotaBytes int64, fijar bool) (domain.Account, error)
	MoveAccountAddress(ctx context.Context, acc domain.Account, nueva string) (domain.Account, error)
	AccountBySubject(ctx context.Context, subject string) (domain.Account, error)
	ListMailboxes(ctx context.Context, accountID uuid.UUID) ([]domain.Mailbox, error)
	MailboxByRole(ctx context.Context, accountID uuid.UUID, role domain.SpecialUse) (domain.Mailbox, error)
	MailboxByID(ctx context.Context, accountID, mailboxID uuid.UUID) (domain.Mailbox, error)
	CreateMailbox(ctx context.Context, accountID uuid.UUID, parentID *uuid.UUID, name string) (domain.Mailbox, error)
	RenameMailbox(ctx context.Context, accountID, mailboxID uuid.UUID, newName string) error
	DeleteMailbox(ctx context.Context, accountID, mailboxID uuid.UUID) error
	ListMessages(ctx context.Context, mailboxID uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error)
	ListThreadMessages(ctx context.Context, accountID, threadID uuid.UUID, limit int) ([]domain.Message, error)
	GetMessage(ctx context.Context, id uuid.UUID) (domain.Message, error)
	GetMessages(ctx context.Context, ids []uuid.UUID) ([]domain.Message, error)
	InsertMessage(ctx context.Context, m *domain.Message) error
	// Contabilidad de blobs compartidos entre organizaciones (ver internal/blobs).
	ReservarBlobCompartido(ctx context.Context, hash, key string, size int64) (existe, listo bool, err error)
	MarcarBlobListo(ctx context.Context, hash string) error
	LiberarBlobCompartido(ctx context.Context, hash string) error
	SearchMessages(ctx context.Context, accountID uuid.UUID, f store.SearchFilter) ([]domain.Message, error)
	SetFlags(ctx context.Context, accountID, id uuid.UUID, flag string, add bool) error
	MoveMessage(ctx context.Context, accountID, id, destMailbox uuid.UUID) error
	DeleteMessage(ctx context.Context, accountID, id uuid.UUID) error
	// Lote (multiselección) + delta incremental para el refresco en vivo.
	SetFlagsBatch(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID, flag string, add bool) (int, error)
	MoveMessages(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID, dest uuid.UUID) (int, error)
	Changes(ctx context.Context, accountID uuid.UUID, since time.Time, sinceID uuid.UUID, limit int) (changed []domain.Message, destroyed []uuid.UUID, lastTS time.Time, lastID uuid.UUID, hasMore bool, err error)
	ListFilters(ctx context.Context, accountID uuid.UUID) ([]domain.Filter, error)
	CreateFilter(ctx context.Context, accountID uuid.UUID, f domain.Filter) (domain.Filter, error)
	UpdateFilter(ctx context.Context, accountID, id uuid.UUID, f domain.Filter) error
	DeleteFilter(ctx context.Context, accountID, id uuid.UUID) error
	ListLabels(ctx context.Context, accountID uuid.UUID) ([]domain.Label, error)
	CreateLabel(ctx context.Context, accountID uuid.UUID, name, color string) (domain.Label, error)
	UpdateLabel(ctx context.Context, accountID, id uuid.UUID, name, color string) error
	DeleteLabel(ctx context.Context, accountID, id uuid.UUID) error
	ApplyLabel(ctx context.Context, accountID, id uuid.UUID, messageIDs []uuid.UUID, add bool) error
	ListMessagesByLabel(ctx context.Context, accountID, id uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error)
	ListScheduled(ctx context.Context, accountID uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.OutboundMessage, error)
	RescheduleHeld(ctx context.Context, id, accountID uuid.UUID, releaseAt time.Time) error
	CancelHeld(ctx context.Context, id, accountID uuid.UUID) (blobKey, hold string, err error)
	ListAliases(ctx context.Context, accountID uuid.UUID) ([]domain.Alias, error)
	CreateAlias(ctx context.Context, accountID uuid.UUID, address string) (domain.Alias, error)
	DeleteAlias(ctx context.Context, accountID, id uuid.UUID) error
	AliasBelongsTo(ctx context.Context, accountID uuid.UUID, address string) (bool, error)
	GetVacation(ctx context.Context, accountID uuid.UUID) (domain.Vacation, error)
	SetVacation(ctx context.Context, accountID uuid.UUID, v domain.Vacation) error
	GetSignature(ctx context.Context, accountID uuid.UUID) (string, error)
	SetSignature(ctx context.Context, accountID uuid.UUID, signature string) error
	// Buzones compartidos + delegación (ACL deny-by-default; ver shared.go).
	ListAccessibleMailboxes(ctx context.Context, subject string) ([]domain.SharedMailbox, error)
	GranteePermission(ctx context.Context, subject string, accountID uuid.UUID) (domain.Permission, error)
	GrantedIdentityByAddress(ctx context.Context, subject, address string) (domain.GrantedIdentity, error)
	MailboxInAccount(ctx context.Context, mailboxID, accountID uuid.UUID) (bool, error)
	EnsureSharedMailbox(ctx context.Context, address, displayName string, quotaBytes int64, ownerSubject string) (domain.Account, error)
	ContarBuzonesCompartidos(ctx context.Context) (int, error)
	ListGrants(ctx context.Context, accountID uuid.UUID) ([]domain.MailboxGrant, error)
	GrantAccess(ctx context.Context, accountID uuid.UUID, granteeEmail string, perm domain.Permission) error
	RevokeAccess(ctx context.Context, accountID uuid.UUID, granteeEmail string) error
}

// blobStore es la superficie mínima del objectstore que consumen los handlers de
// blobs (subida/descarga/mensaje crudo). La satisface *objectstore.Client.
type blobStore interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
	// Delete lo necesita la importación: el blob de subida es temporal y, sin
	// borrarlo, cada mensaje migrado ocuparía el doble en la organización.
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
}

// outboundSubmitter es la superficie mínima de envío saliente (encolar un raw o
// diferirlo: programado/deshacer, devolviendo el id para cancelar/editar).
type outboundSubmitter interface {
	SubmitRaw(ctx context.Context, from string, recipients []string, raw []byte) error
	Schedule(ctx context.Context, from string, recipients []string, raw []byte, releaseAt time.Time, hold string) (uuid.UUID, error)
}

// API agrupa los handlers de la API JMAP/REST de Sytue Mail.
type API struct {
	cfg        config.Config
	store      dataStore
	compliance complianceStore // legal hold + eDiscovery (admin/owner de la org)
	blobs      blobStore
	outbound   outboundSubmitter
	// smime publica el ANCLA de la autoridad S/MIME de la organización. Interfaz
	// y no el tipo concreto para no atar este paquete al de firma: aquí solo se
	// sirve un certificado público, nada de claves.
	smime anclaSMIME
	// push reparte a los clientes SSE de ESTA réplica las llegadas que anuncia
	// PostgreSQL. Nil ⇒ no se monta la ruta: el webmail sigue refrescando al
	// volver a la pestaña, como antes de que existiera esto.
	push   *push.Hub
	logger *slog.Logger
	// peticiones y hub son «pedir acceso» de un buzón compartido (peticiones.go).
	peticiones peticionesStore
	hub        *hubnotify.Client
}

// anclaSMIME es lo único que jmap necesita del servicio de firma.
type anclaSMIME interface {
	AnchorPEM(ctx context.Context, tenant string) (string, error)
	Import(ctx context.Context, tenant string, accountID uuid.UUID, address, certPEM, keyPEM string) (*smime.Identity, error)
}

// New construye la API.
func New(cfg config.Config, st *store.Store, blobs *objectstore.Client, out *delivery.Outbound, logger *slog.Logger) *API {
	// blobs/out pueden ser nil (dev sin S3); guardamos nil explícito en la
	// interfaz para que las comprobaciones `== nil` de los handlers sigan siendo
	// correctas (un puntero nil envuelto en interfaz no es == nil).
	a := &API{cfg: cfg, store: st, compliance: st, outbound: out, logger: logger, peticiones: st}
	if blobs != nil {
		a.blobs = blobs
	}
	return a
}

// UsarPush conecta el repartidor de llegadas. Se llama en el arranque, después
// de construir el API: sin él la ruta del flujo no se monta y el webmail se
// comporta como antes, refrescando al volver a la pestaña.
func (a *API) UsarPush(h *push.Hub) { a.push = h }

// WithSMIME conecta el servicio de firma para poder publicar su ancla. Sin él
// las rutas existen igual: la de dentro contesta 503 —a quien tiene sesión se le
// puede decir que este despliegue no lo tiene configurado— y la pública 404, la
// misma respuesta que da una organización que no existe.
func (a *API) WithSMIME(s anclaSMIME) *API {
	if s != nil {
		a.smime = s
	}
	return a
}

// Mount registra las rutas bajo /api y el endpoint JMAP, protegidas por authn.
// El verifier valida los JWT de Sytue ID; RequireAuth inyecta claims + tenant.
func (a *API) Mount(r chi.Router, verifier *authn.Verifier) {
	// Descubrimiento JMAP (RFC 8620 §2): well-known del recurso de sesión.
	r.Get("/.well-known/jmap", a.handleSessionRedirect)

	// EL ANCLA S/MIME, EN PÚBLICO Y SIN SESIÓN.
	//
	// Quien recibe un correo firmado de la organización es justo quien NO tiene
	// cuenta aquí: ve «el certificado no es fiable» y no puede entrar a Mail a
	// descargar nada. Sin una copia pública, la firma solo se puede verificar
	// dentro, que es tanto como no poder verificarla.
	//
	// Es un certificado PÚBLICO —el ancla, no la clave—, igual que los raíz que
	// cualquier sistema trae de fábrica. Lo que se publica es lo que ya viaja
	// dentro de cada correo firmado.
	//
	// La organización va en la RUTA y no en el host: quien recibe el correo lee
	// `@corporate.<dominio>` en el remitente y con eso ya puede pedirlo desde
	// cualquier sitio. No añade enumeración —ese nombre está en la dirección—, y
	// aun así una organización inexistente y una sin autoridad contestan EXACTO
	// lo mismo, para no confirmar cuáles existen.
	r.Get("/.well-known/smime/{org}.pem", a.getSMIMEAnchorPublico)

	// Los dominios de correo de la suite, para Calendar: de servicio a servicio.
	r.With(httpx.SecretoInterno(a.cfg.HubInternalSecret)).Get(RutaDominios, a.listarDominios)

	apiLimiter := httpx.NewTokenBucket(50, 100)

	r.Group(func(pr chi.Router) {
		pr.Use(verifier.RequireAuth)
		// RequireTenant tras RequireAuth: rechaza en el borde cualquier token sin
		// organización activa. Todas las rutas de negocio de mail son multi-tenant
		// (operan bajo RLS con app.tenant); exigir el tenant presente evita que una
		// petición sin tenant alcance el store (defensa en profundidad frente al
		// fail-closed implícito de InTenantTx).
		pr.Use(verifier.RequireTenant)
		// Tope por IDENTIDAD del token, DESPUÉS de RequireAuth (antes no hay sujeto
		// que leer). No por IP: detrás del ingress todas las peticiones llegan con
		// la misma. Aquí importa el doble que en el resto de la suite: una cuenta
		// comprometida —o la de alguien que se fue con el token vivo— podía usar
		// JMAP como relé a la velocidad que aguantara el servicio, y lo que se
		// quema no es CPU sino la reputación de la IP del MX, que se lleva por
		// delante el correo de TODOS los clientes durante semanas.
		// ponytail: cubo en memoria, por réplica, y es un tope de PETICIONES. La
		// cuota de ENVÍO por organización (mensajes/hora, destinatarios/día) es
		// otra pieza y va aparte: esta no la sustituye.
		pr.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// Sesión JMAP: capacidades + cuentas + URLs de API/descarga/subida.
		pr.Get("/jmap/session", a.handleSession)
		// El flujo de llegadas. Solo si hay repartidor: sin él la ruta no existe,
		// que es mejor que una que se conecta y no dice nada nunca.
		if a.push != nil {
			pr.Get("/jmap/eventos", a.handleStream)
		}
		// Endpoint principal JMAP: sobre Request -> Response por lotes.
		pr.Post("/jmap", a.handleJMAP)

		// Subida/descarga de blobs (RFC 8620 §6.1/§6.2): adjuntos y cuerpos MIME
		// que el cliente sube antes de componer un Email/set, y la descarga de un
		// blob por id. Los bytes viven en Object Storage S3 bajo prefijo de tenant.
		pr.Post("/jmap/upload/{accountId}", a.handleUpload)
		pr.Get("/jmap/download/{accountId}/{blobId}/{name}", a.handleDownload)

		// REST de conveniencia (mismo modelo) que usa @sytue/api-clients.
		pr.Route("/api/v1", func(ar chi.Router) {
			// «Exportar tus datos»: el buzón con sus .eml (platform/pkg/portabilidad).
			ar.Get("/export", a.exportData)
			ar.Get("/mailboxes", a.listMailboxes)
			ar.Post("/mailboxes", a.createMailbox)

			// Cuota de almacenamiento (indicador del sidebar): {used_bytes, quota_bytes}
			// del propio usuario. used_bytes es el contador agregado que el store mantiene
			// (sin N+1, O(1)); solo lectura y owner-scoped. Ver quota.go.
			ar.Get("/quota", a.getQuota)

			ar.Get("/mailboxes/{mailboxID}/messages", a.listMessages)

			// Hilo COMPLETO atravesando carpetas: lo que la vista de conversación
			// no puede armar sola, porque la lista que pliega es de UNA carpeta y
			// tu propia respuesta vive en Enviados. Sin paginación (tope duro en
			// el store) y owner-scoped: un hilo ajeno sale vacío.
			ar.Get("/threads/{threadID}/messages", a.listThreadMessages)

			// EL ANCLA DE LA AUTORIDAD S/MIME, para poder instalarla.
			//
			// Sin esto, el correo salía firmado por una CA que nadie podía
			// conocer: los clientes enseñaban «el certificado no es fiable» y no
			// había de dónde sacar el certificado que lo arregla —el método
			// existía (`AnchorPEM`) y su comentario decía «lo que puede publicar
			// la consola», pero no lo llamaba nadie—.
			//
			// Es un certificado PÚBLICO: es el ancla, no la clave. Va autenticado
			// igual que el resto de /api/v1 para no revelar qué organizaciones
			// existen en la instalación.
			ar.Get("/smime/anchor", a.getSMIMEAnchor)

			// EL CERTIFICADO DE UNA CA PÚBLICA, PARA QUE LA FIRMA VALGA FUERA.
			//
			// El ancla de arriba solo sirve a quien la instala, y quien recibe el
			// correo no instala nada. Esta es la otra mitad: subir el certificado
			// de una CA que ya está en el almacén de confianza de todos los
			// sistemas y firmar con él.
			ar.Put("/smime/identity", a.putSMIMEIdentity)

			// Búsqueda de la suite: la pide el buscador global de Sytue Hub
			// (connectors/mail.go) con el token de la persona. Existía en el Hub y
			// NO aquí, así que Mail salía en cada búsqueda como «no ha respondido».
			// Ruta estática: chi la prioriza sobre /messages/{messageID}.
			ar.Get("/messages/search", a.searchMessages)
			ar.Get("/messages/{messageID}", a.getMessage)
			ar.Get("/messages/{messageID}/raw", a.getMessageRaw)
			ar.Post("/messages/{messageID}/flags", a.setFlags)
			ar.Post("/messages/{messageID}/move", a.moveMessage)
			ar.Delete("/messages/{messageID}", a.deleteMessage)

			// Acciones en LOTE (multiselección de la lista): mover N mensajes
			// (archivar/papelera/spam/mover-a) y marcar N leídos/no leídos, cada una
			// en UNA operación set-based (id = ANY), owner-scoped. Etiquetar en lote
			// ya lo cubre /labels/{id}/apply. Ver batch.go.
			ar.Post("/messages/batch/move", a.batchMove)
			ar.Post("/messages/batch/flags", a.batchFlags)

			// Delta incremental (estilo JMAP Email/changes) para el refresco en vivo
			// de la bandeja abierta: cambios desde un cursor de estado, sin recargar
			// toda la lista. Sondeo del cliente con backoff. Ver changes.go.
			ar.Get("/changes", a.changes)

			// Darse de baja de listas (RFC 2369 List-Unsubscribe + RFC 8058 one-click):
			// GET anuncia los métodos disponibles (sin exponer URLs); POST ejecuta la
			// baja SSRF-safe (one-click https) o envía el correo de baja (mailto). Solo
			// el dueño del mensaje. Ver unsubscribe.go.
			ar.Get("/messages/{messageID}/unsubscribe", a.getUnsubscribe)
			ar.Post("/messages/{messageID}/unsubscribe", a.postUnsubscribe)

			// Envío: encola un mensaje saliente (firma DKIM + cola Postgres). Admite
			// diferir (send_at = programado, undo_seconds = ventana de deshacer).
			ar.Post("/send", a.sendMessage)

			// Diferidos: vista/gestión de programados (keyset) + editar hora + cancelar
			// (undo y programado; reabre el contenido como borrador). Solo el dueño.
			ar.Get("/scheduled", a.listScheduled)
			ar.Post("/scheduled/{id}/reschedule", a.rescheduleScheduled)
			ar.Post("/outbound/{id}/cancel", a.cancelOutbound)

			// Borradores: crear/actualizar/descartar un mensaje en la carpeta Drafts
			// (\Draft) reutilizando la composición RFC 5322 del cliente, sin encolar
			// el envío. DELETE reutiliza el borrado de mensaje (soft-delete).
			ar.Post("/drafts", a.createDraft)
			ar.Put("/drafts/{messageID}", a.updateDraft)
			ar.Delete("/drafts/{messageID}", a.deleteMessage)

			// Reglas/filtros de servidor: auto-mover/marcar el correo entrante
			// por remitente/asunto. La entrega entrante las aplica; aquí el CRUD.
			ar.Get("/filters", a.listFilters)
			ar.Post("/filters", a.createFilter)
			ar.Put("/filters/{filterID}", a.updateFilter)
			ar.Delete("/filters/{filterID}", a.deleteFilter)

			// Etiquetas (labels) de usuario: CRUD del dueño, aplicación en lote a
			// uno/varios mensajes (un hilo) y listado filtrado por etiqueta (keyset).
			// Modeladas como keywords JMAP/IMAP en messages.flags (ver labels.go).
			ar.Get("/labels", a.listLabels)
			ar.Post("/labels", a.createLabel)
			ar.Put("/labels/{labelID}", a.updateLabel)
			ar.Delete("/labels/{labelID}", a.deleteLabel)
			ar.Post("/labels/{labelID}/apply", a.applyLabel)
			ar.Get("/labels/{labelID}/messages", a.listLabelMessages)

			// Alias de correo: CRUD del dueño. Otra dirección que entrega a la misma
			// cuenta; la entrega entrante ya la resuelve. Alimenta el «enviar-como».
			ar.Get("/aliases", a.listAliases)
			ar.Post("/aliases", a.createAlias)
			ar.Delete("/aliases/{aliasID}", a.deleteAlias)

			// Autorespuesta / vacaciones: config de fuera-de-oficina del dueño. La
			// aplica la entrega entrante (RFC 3834 + dedupe); aquí el get/set.
			ar.Get("/vacation", a.getVacation)
			ar.Put("/vacation", a.setVacation)

			// Firma de correo del dueño. Vivía en localStorage del navegador (no
			// existía en otro equipo); ahora es una columna de la cuenta.
			ar.Get("/signature", a.getSignature)
			ar.Put("/signature", a.setSignature)

			// Buzones compartidos (soporte@, ventas@) + delegación (asistente ↔
			// directivo). La lista alimenta el sidebar y el selector «De»; leer
			// carpetas/mensajes de una cuenta ajena exige ACL (requireGrant, read);
			// gestionar el ACL exige `manage`; delegar el propio va por /delegation
			// (dueño ⇒ manage implícito). El envío-como/en-nombre-de lo verifica
			// /send server-side. Ver shared.go.
			ar.Get("/shared-mailboxes", a.listSharedMailboxes)
			ar.Post("/shared-mailboxes", a.createSharedMailbox)
			ar.Get("/shared-mailboxes/{accountID}/mailboxes", a.listSharedFolders)
			ar.Get("/shared-mailboxes/{accountID}/mailboxes/{mailboxID}/messages", a.listSharedMessages)
			ar.Get("/shared-mailboxes/{accountID}/grants", a.listGrants)
			ar.Post("/shared-mailboxes/{accountID}/grants", a.postGrant)
			// Pedir acceso a un buzón en `read`, y la respuesta de quien lo gestiona.
			ar.Post("/shared-mailboxes/{accountID}/edit-requests", a.pedirEdicion)
			ar.Get("/shared-mailboxes/{accountID}/edit-requests", a.listarPeticiones)
			ar.Post("/shared-mailboxes/{accountID}/edit-requests/{sub}/grant", a.concederPeticion)
			ar.Delete("/shared-mailboxes/{accountID}/edit-requests/{sub}", a.rechazarPeticion)
			ar.Get("/delegation", a.listDelegation)
			ar.Post("/delegation", a.postDelegation)

			// Compliance: legal hold + eDiscovery (paridad M365 Purview / Google
			// Vault). Acceso PRIVILEGIADO a correo ajeno, gateado por rol de org
			// (owner|admin) en el borde; cada handler re-verifica el rol (defensa en
			// profundidad) y AUDITA. Aislamiento por org via RLS (tenant del token).
			// La UI de Console es una ola posterior. Ver ediscovery.go.
			ar.Route("/admin", func(adm chi.Router) {
				adm.Use(verifier.RequireRole(complianceRoles...))
				adm.Post("/legal-holds", a.createHold)
				adm.Get("/legal-holds", a.listHolds)
				adm.Post("/legal-holds/{holdID}/release", a.releaseHold)
				adm.Post("/ediscovery/search", a.ediscoverySearch)
				adm.Get("/ediscovery/export", a.ediscoveryExport)
				// Traza de compliance (holds + eDiscovery) para la Auditoría unificada
				// de Console. Listado keyset + filtros + CSV (SIEM). Ver audit.go.
				adm.Get("/audit-log", a.listAuditLog)
			})
		})
	})
}

// handleSessionRedirect implementa el well-known de descubrimiento.
func (a *API) handleSessionRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, a.cfg.PublicURL+"/jmap/session", http.StatusPermanentRedirect)
}
