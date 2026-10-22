// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package migrate es el on-ramp de adopción: trae el contenido de una
// organización desde Microsoft 365 o Google Workspace hacia Sytue.
//
// Es el MISMO problema que el reconcile SCIM saliente (internal/scimout) —estado
// de origen vs estado ya-copiado— y sigue su patrón: trabajo por lotes sobre el
// outbox, mapa de ids cargado EN BLOQUE (sin N+1) y checkpoint por lote. De ahí
// salen las dos garantías:
//
//	IDEMPOTENCIA — un id de origen ya presente en migration_map NO se vuelve a
//	escribir nunca. Una segunda pasada del mismo trabajo hace CERO escrituras.
//	REANUDACIÓN — cada lote persiste un cursor (cola BFS de carpetas en drive,
//	pageToken en contacts). Un trabajo interrumpido retoma por ahí, sin repetir.
//
// HONESTIDAD: lo que no se puede copiar NO se intenta en silencio ni se cuenta
// como éxito — se registra en migration_map con su motivo (skip_reason) y aparece
// en el informe del trabajo. Caso principal: los ficheros NATIVOS de Google
// (Docs/Sheets/Slides) necesitan exportación y esta ola no la implementa.
//
// El egress al SaaS de origen sale SIEMPRE por Runtime.HTTP (SSRF + rate-limit +
// auditoría + política); la escritura en Sytue va por la API pública de
// services/drive y services/contacts (nunca contra sus bases de datos).
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
)

// Límites de defensa del recorrido. Son topes, no configuración: acotan un
// origen hostil o un ciclo de carpetas, no la migración legítima.
const (
	// maxItems es el tope de entidades visitadas por trabajo (drive y contacts).
	maxItems = 200000
	// maxQueue es el tope de carpetas pendientes en la cola BFS.
	maxQueue = 50000
	// maxFileBytes es el tope por fichero. Por encima se OMITE con motivo (nunca
	// se sube truncado).
	maxFileBytes = 2 << 30 // 2 GiB
	// batchItems es cada cuántas entidades se hace checkpoint dentro de una
	// carpeta muy grande o de una página de contactos.
	batchItems = 200
)

// ErrCanceled indica que el trabajo se canceló desde la API mientras corría.
var ErrCanceled = errors.New("migrate: trabajo cancelado")

// ErrUnimportable marca un fallo DEFINITIVO del destino: reintentarlo no lo
// arregla (un repositorio sin commits, un nombre ya ocupado en la forja). El
// runner lo registra como OMISIÓN con su motivo —igual que un documento nativo de
// Google— en vez de contarlo como error y reintentarlo para siempre.
var ErrUnimportable = errors.New("migrate: el destino no puede importar esta entidad")

// ErrSinPermiso es un destino que rechaza la escritura (401/403): no se arregla
// reintentando cada entidad, así que la salida para y lo dice.
var ErrSinPermiso = errors.New("migrate: el destino no deja escribir")

// Item es una entidad del árbol del origen (carpeta o fichero) ya normalizada.
// SkipReason no vacío ⇒ el ORIGEN ya sabe que no se puede copiar (p. ej. un Google
// Doc nativo): el runner la registra como omitida sin intentar la descarga.
type Item struct {
	ID          string
	Name        string
	IsFolder    bool
	Size        int64
	MimeType    string
	Modified    time.Time
	DownloadURL string
	SkipReason  string
	// Native marca el documento que NO tiene bytes propios (un Google Doc): hay
	// que EXPORTARLO a un formato ofimático y crearlo en Sytue Docs. Open() ya
	// devuelve los bytes exportados; esto solo dice adónde tiene que ir.
	Native bool
}

// Contact es un contacto del origen normalizado a lo que acepta Sytue Contacts.
type Contact struct {
	ID            string   `json:"-"`
	FormattedName string   `json:"formatted_name"`
	GivenName     string   `json:"given_name,omitempty"`
	FamilyName    string   `json:"family_name,omitempty"`
	Organization  string   `json:"organization,omitempty"`
	Title         string   `json:"title,omitempty"`
	Note          string   `json:"note,omitempty"`
	Emails        []string `json:"-"`
	Phones        []string `json:"-"`
}

// Repo es un repositorio del origen normalizado. AuthToken es la credencial con la
// que la FORJA DE DESTINO clona el origen: la migración de un repositorio no se
// hace byte a byte por aquí, la ejecuta Sytue Git contra GitHub, y sin esa
// credencial un repositorio privado no se puede leer. Nunca se registra ni se
// audita: viaja del vault al cuerpo de UNA petición a la forja.
type Repo struct {
	ID          string
	Owner       string // organización o usuario dueño en el origen
	Name        string
	FullName    string // "owner/name", el identificador legible del informe
	CloneURL    string
	Description string
	Private     bool
	Empty       bool
	Archived    bool
	// OwnerIsOrg distingue el repositorio de una organización del personal: los de
	// una organización aterrizan en una organización equivalente de Sytue Git, no
	// colgando de la cuenta de una persona.
	OwnerIsOrg bool
	AuthToken  string
}

// Source es el SaaS de origen. Tres implementaciones: Microsoft 365 (Graph),
// Google Workspace (Drive v3 + People) y GitHub. Todo su egress sale por
// Runtime.HTTP.
type Source interface {
	// Label es el nombre legible del origen, para los mensajes del informe.
	Label() string
	// ListChildren lista el contenido de una carpeta; folderID vacío = la raíz.
	ListChildren(ctx context.Context, folderID string) ([]Item, error)
	// Open abre el contenido de un fichero para copiarlo (streaming).
	Open(ctx context.Context, it Item) (io.ReadCloser, error)
	// ListContacts devuelve una página de contactos y el token de la siguiente
	// ("" = no hay más).
	ListContacts(ctx context.Context, pageToken string) ([]Contact, string, error)
}

// RepoSource es la capacidad OPCIONAL de un origen que tiene repositorios (hoy
// GitHub). Se detecta por aserción de tipo, como las capacidades de los drivers:
// un origen sin forja no la implementa y el dominio 'repos' se le rechaza.
type RepoSource interface {
	// ListRepos devuelve una página de repositorios y el cursor de la siguiente
	// ("" = no hay más).
	ListRepos(ctx context.Context, cursor string) ([]Repo, string, error)
}

// TeamSource es la capacidad OPCIONAL de un origen con equipos de mensajería (hoy
// Microsoft Teams). Como el resto de capacidades, se detecta por aserción de tipo.
type TeamSource interface {
	ListTeams(ctx context.Context) ([]Team, error)
	ListChannels(ctx context.Context, teamID string) ([]Channel, error)
	ListChannelMembers(ctx context.Context, teamID, channelID string) ([]Member, error)
}

// ChatDestination es Sytue Rooms: recrea equipos, canales y membresías. Escribe
// por la API pública de Rooms con el token de quien lanza la migración, así que
// hereda sus permisos y su auditoría.
type ChatDestination interface {
	EnsureTeam(ctx context.Context, name, description string) (string, error)
	EnsureChannel(ctx context.Context, teamID, name, topic string) (string, error)
	AddMember(ctx context.Context, teamID, channelID, subject, name string) error
}

// Calendar es un calendario del origen.
type Calendar struct {
	ID          string
	Name        string
	Description string
	TimeZone    string
	Color       string
	Primary     bool
	Skip        string // motivo si ese calendario NO se migra
}

// Attendee es una persona (o sala) invitada a un evento, en vocabulario
// iCalendar: es el que habla tanto el origen como Sytue Calendar.
type Attendee struct {
	Email    string
	Name     string
	Role     string
	PartStat string
	CUType   string
}

// Event es un evento del origen. Las recurrencias viajan como las define
// iCalendar (RRULE/RDATE/EXDATE): expandirlas convertiría «todos los martes» en
// cientos de eventos sueltos que ya no se pueden editar como serie.
type Event struct {
	ID             string
	UID            string
	Summary        string
	Description    string
	Location       string
	Organizer      string
	Start          time.Time
	End            time.Time
	TZID           string
	AllDay         bool
	Status         string
	Classification string
	Transparency   string
	ConferenceURL  string
	RRule          string
	RDate          []time.Time
	ExDate         []time.Time
	Attendees      []Attendee
	Skip           string // motivo si ese evento NO se puede traer
}

// CalendarSource es la capacidad OPCIONAL de un origen con agenda.
type CalendarSource interface {
	ListCalendars(ctx context.Context) ([]Calendar, error)
	// ListEvents devuelve una página de eventos y el cursor de la siguiente.
	ListEvents(ctx context.Context, calendarID, cursor string) ([]Event, string, error)
}

// CalendarDestination es Sytue Calendar.
type CalendarDestination interface {
	EnsureCalendar(ctx context.Context, c Calendar) (string, error)
	CreateEvent(ctx context.Context, calendarID string, e Event) (string, error)
}

// MailFolder es una carpeta (o etiqueta) del buzón de origen.
type MailFolder struct {
	ID   string
	Name string
	// Role es el papel especial de la carpeta cuando lo tiene: inbox, sent,
	// drafts, junk, trash o archive. Sirve para que el correo enviado no acabe en
	// la bandeja de entrada de destino.
	Role string
	Skip string // motivo si esa carpeta NO se migra
}

// MailMessage es un mensaje del buzón de origen, sin su contenido: los bytes se
// piden aparte para no tener miles de correos en memoria a la vez.
type MailMessage struct {
	ID string
	// FolderID es dónde vivía. Un mensaje va a UNA carpeta en Sytue Mail (como en
	// IMAP), así que si el origen usa etiquetas se elige la principal.
	FolderID   string
	Subject    string
	ReceivedAt time.Time
	Seen       bool
	Flagged    bool
	Draft      bool
	SizeBytes  int64
	Skip       string // motivo si ese mensaje NO se puede traer
}

// MailSource es la capacidad OPCIONAL de un origen con buzón (hoy Gmail).
type MailSource interface {
	ListFolders(ctx context.Context) ([]MailFolder, error)
	// ListMessages devuelve una página de mensajes y el cursor de la siguiente.
	ListMessages(ctx context.Context, cursor string) ([]MailMessage, string, error)
	// OpenMessage devuelve el mensaje RFC 5322 completo, tal cual está en origen.
	OpenMessage(ctx context.Context, id string) ([]byte, error)
}

// MailDestination es Sytue Mail. Escribe por su API pública (JMAP), así que la
// migración hereda sus cuotas, su indexado de hilos y su auditoría.
type MailDestination interface {
	// EnsureFolder devuelve el id de la carpeta destino, creándola si hace falta.
	EnsureFolder(ctx context.Context, f MailFolder) (string, error)
	// ImportMessage guarda el mensaje conservando fecha, estado y carpeta.
	ImportMessage(ctx context.Context, folderID string, raw []byte, m MailMessage) (string, error)
}

// CloudVar es una variable de entorno de una aplicación del origen. Secret marca
// las que venían de un gestor de secretos: en Sytue Cloud se guardan como
// secreto, no como variable a la vista.
type CloudVar struct {
	Name   string
	Value  string
	Secret bool
	// Skip explica por qué esa variable llega VACÍA (el valor vivía en un secreto
	// que la cuenta no puede leer). La variable se crea igual: una app a la que le
	// falta el NOMBRE de una variable falla de una forma mucho más difícil de
	// diagnosticar que una a la que le falta el valor.
	Skip string
}

// CloudApp es una aplicación del origen: un servicio de Cloud Run o un sitio de
// Firebase Hosting. Es lo que en Sytue Cloud es un PROYECTO.
type CloudApp struct {
	ID   string
	Name string
	// Kind distingue de dónde viene ("run" | "hosting"), porque el aterrizaje no
	// es el mismo: un servicio de Cloud Run trae una imagen que ya corre y un
	// sitio de Hosting trae ficheros estáticos.
	Kind         string
	Region       string
	Image        string
	PublicURL    string
	Description  string
	Vars         []CloudVar
	Domains      []string
	MinInstances int
	MaxInstances int
	// CPU y MemoryMB son los límites del origen. Se traducen al tamaño de
	// instancia de Sytue Cloud más cercano POR ENCIMA: quedarse corto convierte
	// una migración en una caída de rendimiento que nadie relaciona con esto.
	CPU      float64
	MemoryMB int
	Skip     string
}

// CloudBucket es un bucket de objetos del origen (Cloud Storage o Firebase
// Storage, que es el mismo servicio por debajo).
type CloudBucket struct {
	Name     string
	Location string
	Skip     string
}

// CloudObject es un objeto dentro de un bucket.
type CloudObject struct {
	Bucket      string
	Key         string
	ContentType string
	Size        int64
	Skip        string
}

// CloudJob es una tarea programada del origen (Cloud Scheduler).
type CloudJob struct {
	Name     string
	Schedule string
	TimeZone string
	// TargetURL y Method describen el trabajo cuando llama a una URL, que es como
	// se usa Cloud Scheduler el 90 % de las veces.
	TargetURL string
	Method    string
	Body      []byte
	Skip      string
}

// CloudDatabase es una base de datos del origen (Cloud SQL, Firestore,
// BigQuery). No se migra: existe para poder ENUMERARLA en el informe con lo que
// hay que hacer con ella, en vez de dejarla fuera y que nadie la eche en falta
// hasta que la aplicación no arranca.
type CloudDatabase struct {
	Name   string
	Engine string
	Skip   string
}

// CloudSource es la capacidad OPCIONAL de un origen con infraestructura de
// aplicaciones (hoy Google Cloud + Firebase).
type CloudSource interface {
	ListApps(ctx context.Context) ([]CloudApp, error)
	ListBuckets(ctx context.Context) ([]CloudBucket, error)
	// ListObjects devuelve una página de objetos y el cursor de la siguiente.
	ListObjects(ctx context.Context, bucket, cursor string) ([]CloudObject, string, error)
	OpenObject(ctx context.Context, o CloudObject) (io.ReadCloser, error)
	ListJobs(ctx context.Context) ([]CloudJob, error)
	ListDatabases(ctx context.Context) ([]CloudDatabase, error)
}

// CloudDestination es Sytue Cloud. Escribe por su API pública con el token del
// trabajo, así que hereda las cuotas del plan de la organización y su auditoría.
type CloudDestination interface {
	// EnsureProject crea (o reutiliza) el proyecto de esa aplicación y devuelve su id.
	EnsureProject(ctx context.Context, a CloudApp) (string, error)
	// SetEnv fija las variables y los secretos del proyecto de una vez.
	SetEnv(ctx context.Context, projectID string, vars []CloudVar) error
	// AddDomain añade un dominio propio al proyecto.
	AddDomain(ctx context.Context, projectID, host string) error
	// EnsureBucket aprovisiona el Storage gestionado del proyecto y devuelve su id.
	EnsureBucket(ctx context.Context, projectID string, b CloudBucket) (string, error)
	// UploadObject sube un objeto al Storage del proyecto.
	UploadObject(ctx context.Context, projectID string, o CloudObject, body io.Reader) error
	// EnsureCronJob recrea una tarea programada del origen.
	EnsureCronJob(ctx context.Context, projectID string, j CloudJob) (string, error)
}

// People traduce un correo del origen a la persona de Sytue que le corresponde.
// Existe porque una membresía de canal NO se puede copiar «por correo»: Rooms
// identifica a cada cual por su sujeto de Sytue ID, y meter ahí un correo crearía
// un miembro fantasma que nunca coincidiría con quien inicia sesión.
type People interface {
	// Subject devuelve el sujeto y el nombre de la persona de Sytue con ese correo.
	// ok=false ⇒ no hay equivalente: el runner lo registra como omisión con motivo,
	// nunca añade a alguien «parecido».
	Subject(ctx context.Context, email string) (subject, name string, ok bool, err error)
}

// RepoDestination es Sytue Git: recibe el repositorio y lo importa ENTERO desde el
// origen (historia, ramas, etiquetas, issues, PRs, wiki, releases). La copia la
// hace la forja, no este proceso: replicar aquí lo que un `git clone --mirror` ya
// resuelve sería reescribir mal una herramienta que existe.
type RepoDestination interface {
	// MigrateRepo importa el repositorio para el usuario destino y devuelve la ruta
	// creada en Sytue Git ("owner/nombre").
	MigrateRepo(ctx context.Context, targetUser string, r Repo) (string, error)
}

// DocsDestination es Sytue Docs: recibe un documento OFIMÁTICO y lo crea como
// documento editable (no como fichero adjunto). La conversión la hace el propio
// Docs, así que la migración hereda cada formato que aprenda.
type DocsDestination interface {
	// ImportDocument crea el documento y devuelve su id y su tipo (document |
	// spreadsheet | presentation).
	ImportDocument(ctx context.Context, filename string, body io.Reader) (id, kind string, err error)
}

// SizedUploader lo implementa el destino que necesita el tamaño antes de subir
// (Microsoft Graph exige Content-Length). El runner lo usa cuando el origen lo sabe.
type SizedUploader interface {
	UploadFileSized(ctx context.Context, folderID, name, contentType string, size int64, body io.Reader) (string, error)
}

// Destination es Sytue: se habla por su API pública con el token del trabajo.
type Destination interface {
	Root(ctx context.Context) (string, error)
	CreateFolder(ctx context.Context, parentID, name string) (string, error)
	UploadFile(ctx context.Context, folderID, name, contentType string, body io.Reader) (string, error)
	AddressBook(ctx context.Context) (string, error)
	CreateContact(ctx context.Context, bookID string, c Contact) (string, error)
}

// Store es la persistencia del trabajo. La satisface *store.Store; en los tests,
// un doble en memoria (sin base de datos).
type Store interface {
	LoadMigrationMap(ctx context.Context, jobID string) ([]domain.MigrationItem, error)
	RecordMigrationItems(ctx context.Context, jobID string, items []domain.MigrationItem) error
	SaveMigrationProgress(ctx context.Context, jobID, cursor string, c domain.MigrationCounts) (domain.MigrationStatus, error)
	StartMigrationJob(ctx context.Context, jobID string) error
	FinishMigrationJob(ctx context.Context, jobID string, st domain.MigrationStatus, lastErr string) error
}

// Deps son las dependencias de una ejecución. Git solo hace falta para el dominio
// 'repos' (el resto aterriza en Drive/Contacts por Dst).
type Deps struct {
	Src  Source
	Dst  Destination
	Git  RepoDestination
	Chat ChatDestination
	// Cal recibe los calendarios y sus eventos.
	Cal CalendarDestination
	// Mail recibe el buzón (mensajes con su carpeta, su fecha y su estado).
	Mail MailDestination
	// Docs recibe los documentos NATIVOS del origen (un Google Doc no es un
	// fichero: hay que exportarlo y convertirlo). Nil ⇒ se omiten con su motivo,
	// que es lo que pasaba antes de que Sytue Docs supiera importarlos.
	Docs DocsDestination
	// Cloud recibe las aplicaciones, los buckets y las tareas programadas.
	Cloud  CloudDestination
	People People
	Store  Store
}

// NewSource fabrica el origen de un trabajo a partir de la instancia de conector
// de la org (sus credenciales) y del usuario de origen. Cada origen mintea su
// token con el driver que ya lo sabe hacer; el scope pedido es el MÍNIMO del
// dominio a migrar y siempre de solo lectura.
func NewSource(ctx context.Context, rt connector.Runtime, inst connector.Instance, job domain.MigrationJob) (Source, error) {
	if !job.Source.Supports(job.Domain) {
		return nil, fmt.Errorf("migrate: %s no puede migrar %q", job.Source, job.Domain)
	}
	// En un trabajo de SALIDA el origen es Sytue y el tercero es el destino. Lo
	// arma el worker (necesita los tokens de la suite), así que aquí no hay origen
	// de terceros que fabricar.
	if job.Direction == domain.DirectionOut {
		return nil, fmt.Errorf(
			"migrate: un trabajo de salida lee de Sytue; su origen lo arma el worker, no NewSource",
		)
	}
	switch job.Source {
	case domain.SourceM365:
		m, err := newM365(ctx, rt, inst, job.SourceUser)
		if err != nil {
			return nil, err
		}
		if job.Domain == domain.DomainTeams || job.Domain == domain.DomainTeamFiles {
			return &m365Teams{m365: m}, nil
		}
		return m, nil
	case domain.SourceGoogle:
		// El scope pedido es el MÍNIMO del contenido a migrar, y siempre de lectura.
		scope := scopeDriveRead
		switch job.Domain {
		case domain.DomainContacts:
			scope = scopeContactsRead
		case domain.DomainMail:
			scope = scopeGmailRead
		case domain.DomainCalendar:
			scope = scopeCalendarRead
		case domain.DomainCloud:
			scope = scopeCloudRead
		case domain.DomainDrive, domain.DomainRepos, domain.DomainGists,
			domain.DomainTeams, domain.DomainTeamFiles:
			// drive.readonly (los dominios que Google no sirve ya los rechazó Supports).
		}
		g, err := newGoogle(ctx, rt, inst, job.SourceUser, scope)
		if err != nil {
			return nil, err
		}
		switch job.Domain {
		case domain.DomainMail:
			return &gmail{google: g}, nil
		case domain.DomainCalendar:
			return &gcal{google: g}, nil
		case domain.DomainCloud:
			return &gcp{google: g}, nil
		default:
			return g, nil
		}
	case domain.SourceGitHub:
		return newGitHub(rt, inst)
	default:
		return nil, fmt.Errorf("migrate: origen %q no soportado", job.Source)
	}
}

// NewOutboundDestination fabrica el destino de un trabajo de SALIDA: adónde se
// lleva el contenido que sale de Sytue. Google Workspace (Drive y People) y
// Microsoft 365 (OneDrive y Outlook). Los permisos de escritura NO vienen con la
// conexión de siempre, que es de lectura: se conceden aparte, en el
// consentimiento de salida (oauth/start con purpose "out").
func NewOutboundDestination(ctx context.Context, rt connector.Runtime, inst connector.Instance, job domain.MigrationJob) (Destination, error) {
	if job.Source == domain.SourceM365 {
		if strings.TrimSpace(job.SourceUser) == "" && !inst.HasBearer {
			return nil, fmt.Errorf(
				"%w: esta conexión con Microsoft 365 todavía no está autorizada para ESCRIBIR; autorízala aceptando los permisos de escritura", connector.ErrBadQuery,
			)
		}
		return NewM365Destination(ctx, rt, inst, job.SourceUser)
	}
	if job.Source != domain.SourceGoogle {
		return nil, fmt.Errorf(
			"migrate: llevarse el contenido a %s no está disponible; la salida asistida es a Google Workspace o Microsoft 365", job.Source,
		)
	}
	scope := scopeDriveWrite
	if job.Domain == domain.DomainContacts {
		scope = scopeContactsWrite
	}
	token := ""
	if strings.TrimSpace(job.SourceUser) != "" {
		// Con delegación de dominio se escribe en la cuenta indicada; sin ella, el
		// token del consentimiento lo inyecta la pasarela y escribe en la de quien
		// autorizó (que es justo lo que se le enseñó al aceptar).
		t, err := gws.UserToken(ctx, rt, inst, job.SourceUser, scope)
		if err != nil {
			return nil, err
		}
		token = t
	} else if !inst.HasBearer {
		return nil, fmt.Errorf(
			"%w: esta conexión con Google todavía no está autorizada para ESCRIBIR; vuelve a conectarla aceptando los permisos de escritura", connector.ErrBadQuery,
		)
	}
	return NewGoogleDestination(rt, inst, token)
}

// Run ejecuta (o reanuda) un trabajo de migración de principio a fin: lo marca
// como corriendo, recorre el origen por lotes y lo cierra con su estado final.
// Un error se devuelve para que el outbox reintente con backoff; como el mapa y
// el cursor están persistidos, el reintento continúa en vez de repetir.
func Run(ctx context.Context, d Deps, job domain.MigrationJob) error {
	if err := d.Store.StartMigrationJob(ctx, job.ID); err != nil {
		return err
	}
	// Migrated/Skipped/Bytes son hechos acumulados (respaldados por migration_map).
	// Errors NO: cuenta los fallos aún sin resolver, así que cada pasada parte de
	// cero — si no, un trabajo nunca podría llegar a 'done' tras un fallo puntual.
	counts := job.Counts
	counts.Errors = 0
	r := &run{d: d, job: job, counts: counts, done: map[string]struct{}{}}

	entries, err := d.Store.LoadMigrationMap(ctx, job.ID) // EN BLOQUE: sin N+1.
	if err != nil {
		return d.finish(ctx, job.ID, domain.MigrationError, err)
	}
	for _, e := range entries {
		r.done[e.SourceID] = struct{}{}
	}

	switch job.Domain {
	case domain.DomainDrive, domain.DomainGists, domain.DomainTeamFiles:
		// Un gist es una carpeta con ficheros y un canal de Teams es una carpeta con
		// su historial y sus adjuntos: mismo recorrido que un árbol de Drive.
		err = r.drive(ctx)
	case domain.DomainContacts:
		err = r.contacts(ctx)
	case domain.DomainRepos:
		err = r.repos(ctx)
	case domain.DomainMail:
		err = r.mail(ctx)
	case domain.DomainCalendar:
		err = r.calendar(ctx)
	case domain.DomainTeams:
		err = r.teams(ctx)
	case domain.DomainCloud:
		err = r.cloud(ctx)
	default:
		err = fmt.Errorf("migrate: dominio %q no soportado en esta ola (correo y calendario van aparte)", job.Domain)
	}

	return r.cerrar(ctx, err)
}

// finish cierra el trabajo conservando el error original (el cierre no debe
// enmascarar la causa).
func (d Deps) finish(ctx context.Context, jobID string, st domain.MigrationStatus, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	if err := d.Store.FinishMigrationJob(ctx, jobID, st, msg); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// --- Ejecución ---------------------------------------------------------------

// run es el estado vivo de una ejecución: los contadores, el mapa de lo ya hecho
// y el lote pendiente de persistir.
type run struct {
	d      Deps
	job    domain.MigrationJob
	counts domain.MigrationCounts
	done   map[string]struct{}
	batch  []domain.MigrationItem
	seen   int
}

// record apunta una entidad en el lote (migrada u omitida) y actualiza contadores.
func (r *run) record(it domain.MigrationItem) {
	r.batch = append(r.batch, it)
	r.done[it.SourceID] = struct{}{}
	if it.SkipReason != "" {
		r.counts.Skipped++
		return
	}
	r.counts.Migrated++
}

// skip registra una omisión CON SU MOTIVO. Nada se cuenta como copiado sin haberlo
// copiado: esta es la mitad honesta del informe.
func (r *run) skip(id string, kind domain.MigrationKind, path, reason string) {
	r.record(domain.MigrationItem{SourceID: id, Kind: kind, Path: path, SkipReason: reason})
}

// checkpoint persiste el lote y el cursor, y devuelve error si el trabajo se
// canceló entretanto (el worker debe parar sin tocar nada más).
func (r *run) checkpoint(ctx context.Context, cursor string) error {
	if err := r.d.Store.RecordMigrationItems(ctx, r.job.ID, r.batch); err != nil {
		return err
	}
	r.batch = r.batch[:0]
	st, err := r.d.Store.SaveMigrationProgress(ctx, r.job.ID, cursor, r.counts)
	if err != nil {
		return err
	}
	if st == domain.MigrationCanceled {
		return ErrCanceled
	}
	return nil
}

// cerrar fija el estado final de una ejecución a partir de cómo terminó el recorrido.
func (r *run) cerrar(ctx context.Context, err error) error {
	d, job := r.d, r.job
	switch {
	case errors.Is(err, ErrCanceled):
		return nil // el estado 'canceled' ya lo fijó la API; no hay nada que reintentar.
	case err != nil:
		return d.finish(ctx, job.ID, domain.MigrationError, err)
	case r.counts.Errors > 0:
		// Recorrido completo pero con entidades que NO se copiaron por fallos
		// transitorios. Cerrarlo como 'done' sería mentir: se cierra en 'error' con
		// el último motivo, y reanudar reintenta SOLO esas (no están en el mapa).
		return d.finish(ctx, job.ID, domain.MigrationError, fmt.Errorf(
			"migrate: %d entidades no se pudieron copiar; reanuda el trabajo para reintentarlas. Último fallo: %s",
			r.counts.Errors, r.job.LastError,
		))
	default:
		return d.finish(ctx, job.ID, domain.MigrationDone, nil)
	}
}

// --- Drive -------------------------------------------------------------------

// pending es una carpeta del origen pendiente de recorrer, con la carpeta de
// Sytue ya creada donde van sus hijos.
type pending struct {
	Src  string `json:"s"`
	Dst  string `json:"d"`
	Path string `json:"p"`
}

// driveCursor es el cursor de reanudación del recorrido de ficheros: la cola BFS
// de carpetas pendientes. Se serializa a JSON en migration_jobs.cursor.
type driveCursor struct {
	Queue []pending `json:"queue"`
}

// drive recorre la jerarquía del origen en anchura y la recrea en Sytue Drive:
// una carpeta de Sytue por carpeta de origen (misma ruta, mismo nombre) y una
// subida por fichero. Cada carpeta terminada es un checkpoint.
func (r *run) drive(ctx context.Context) error {
	queue, err := r.seedQueue(ctx)
	if err != nil {
		return err
	}

	for len(queue) > 0 {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		cur := queue[0]
		queue = queue[1:]

		children, err := r.d.Src.ListChildren(ctx, cur.Src)
		if err != nil {
			// La carpeta vuelve a la cola: el reintento del outbox la retoma.
			r.counts.Errors++
			_ = r.checkpoint(ctx, encodeCursor(append([]pending{cur}, queue...)))
			return fmt.Errorf("migrate: listar %q en %s: %w", cur.Path, r.d.Src.Label(), err)
		}

		for _, it := range children {
			r.seen++
			if _, ok := r.done[it.ID]; ok {
				continue // idempotencia: ya procesado en una pasada anterior.
			}
			sub, err := r.copyItem(ctx, cur, it)
			if err != nil {
				// Fallo transitorio: NO se apunta en el mapa, así el reintento lo
				// vuelve a intentar (a diferencia de una omisión, que es definitiva).
				r.counts.Errors++
				r.job.LastError = err.Error()
				continue
			}
			if len(queue) < maxQueue {
				queue = append(queue, sub...)
			}
			if len(r.batch) >= batchItems {
				if err := r.checkpoint(ctx, encodeCursor(append([]pending{cur}, queue...))); err != nil {
					return err
				}
			}
		}
		if err := r.checkpoint(ctx, encodeCursor(queue)); err != nil {
			return err
		}
	}
	return r.checkpoint(ctx, "")
}

// seedQueue devuelve la cola de carpetas pendientes: la del cursor si el trabajo
// se reanuda, o la raíz de Sytue Drive si empieza de cero.
func (r *run) seedQueue(ctx context.Context) ([]pending, error) {
	if raw := strings.TrimSpace(r.job.Cursor); raw != "" {
		var c driveCursor
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("migrate: cursor de reanudación ilegible: %w", err)
		}
		return c.Queue, nil
	}
	root, err := r.d.Dst.Root(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: raíz de Sytue Drive: %w", err)
	}
	return []pending{{Src: "", Dst: root, Path: "/"}}, nil
}

// copyItem copia UNA entidad del origen. Devuelve las carpetas que quedan por
// recorrer a raíz de ella: una si el item era una carpeta, ninguna si era un
// fichero (o si se omitió).
func (r *run) copyItem(ctx context.Context, parent pending, it Item) ([]pending, error) {
	path := parent.Path + it.Name
	if it.IsFolder {
		id, err := r.d.Dst.CreateFolder(ctx, parent.Dst, it.Name)
		if err != nil {
			return nil, fmt.Errorf("migrate: crear carpeta %q: %w", path, err)
		}
		r.record(domain.MigrationItem{SourceID: it.ID, Kind: domain.KindFolder, SytueID: id, Path: path, Hash: hashItem(it)})
		return []pending{{Src: it.ID, Dst: id, Path: path + "/"}}, nil
	}

	// Omisiones DEFINITIVAS: se registran con su motivo y no se intentan.
	if it.SkipReason != "" {
		r.skip(it.ID, domain.KindFile, path, it.SkipReason)
		return nil, nil
	}
	if it.Size > maxFileBytes {
		r.skip(it.ID, domain.KindFile, path,
			"el fichero mide "+strconv.FormatInt(it.Size, 10)+" bytes y supera el tope de la migración ("+strconv.Itoa(maxFileBytes)+")")
		return nil, nil
	}

	body, err := r.d.Src.Open(ctx, it)
	if errors.Is(err, ErrUnimportable) {
		// El ORIGEN dice que esto no se puede traer (un documento por encima del
		// límite de exportación): definitivo, se registra con su motivo.
		r.skip(it.ID, domain.KindFile, path, err.Error())
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: descargar %q: %w", path, err)
	}
	defer body.Close()

	// Un documento NATIVO del origen no es un fichero: va a Sytue Docs como
	// documento editable. Subirlo a Drive como .docx sería dejarle a la persona un
	// adjunto que no puede abrir sin descargarlo.
	if it.Native {
		return nil, r.copyNative(ctx, it, path, body)
	}

	var id string
	if su, ok := r.d.Dst.(SizedUploader); ok && it.Size > 0 {
		id, err = su.UploadFileSized(ctx, parent.Dst, it.Name, it.MimeType, it.Size, body)
	} else {
		id, err = r.d.Dst.UploadFile(ctx, parent.Dst, it.Name, it.MimeType, body)
	}
	if errors.Is(err, ErrUnimportable) {
		// El DESTINO dice que esto no entra (tamaño, nombre): definitivo, con su motivo.
		r.skip(it.ID, domain.KindFile, path, err.Error())
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: subir %q: %w", path, err)
	}
	r.record(domain.MigrationItem{SourceID: it.ID, Kind: domain.KindFile, SytueID: id, Path: path, Hash: hashItem(it)})
	r.counts.Bytes += it.Size
	return nil, nil
}

// copyNative crea en Sytue Docs un documento nativo del origen ya exportado.
//
// Sin Sytue Docs configurado se omite con el motivo de siempre: es exactamente lo
// que hacía esta migración antes de que el destino supiera convertirlos, y decirlo
// es mejor que dejar el documento a medias en Drive.
func (r *run) copyNative(ctx context.Context, it Item, path string, body io.Reader) error {
	if r.d.Docs == nil {
		r.skip(it.ID, domain.KindDoc, path,
			"documento nativo del origen: este despliegue no tiene Sytue Docs configurado para convertirlo")
		return nil
	}
	id, kind, err := r.d.Docs.ImportDocument(ctx, it.Name, body)
	if err != nil {
		// Formato que Docs no sabe convertir (un formulario, un dibujo): definitivo.
		if errors.Is(err, ErrUnimportable) {
			r.skip(it.ID, domain.KindDoc, path, err.Error())
			return nil
		}
		return fmt.Errorf("migrate: crear en Sytue Docs %q: %w", path, err)
	}
	r.record(domain.MigrationItem{
		SourceID: it.ID, Kind: docKindOf(kind), SytueID: id, Path: path, Hash: hashItem(it),
	})
	return nil
}

// docKindOf traduce el tipo que devuelve Sytue Docs al del informe.
func docKindOf(kind string) domain.MigrationKind {
	switch kind {
	case "spreadsheet":
		return domain.KindSpreadsheet
	case "presentation":
		return domain.KindPresentation
	default:
		return domain.KindDoc
	}
}

// encodeCursor serializa la cola BFS. Cola vacía ⇒ cursor vacío (trabajo agotado).
func encodeCursor(queue []pending) string {
	if len(queue) == 0 {
		return ""
	}
	b, err := json.Marshal(driveCursor{Queue: queue})
	if err != nil {
		return "" // struct de tipos serializables: inalcanzable en la práctica.
	}
	return string(b)
}

// --- Contacts ----------------------------------------------------------------

// contacts importa la agenda del origen a la libreta por defecto de Sytue
// Contacts, página a página. El cursor es el pageToken del origen.
func (r *run) contacts(ctx context.Context) error {
	book, err := r.d.Dst.AddressBook(ctx)
	if err != nil {
		return fmt.Errorf("migrate: libreta de destino: %w", err)
	}
	token := strings.TrimSpace(r.job.Cursor)
	for {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		page, next, err := r.d.Src.ListContacts(ctx, token)
		if err != nil {
			_ = r.checkpoint(ctx, token)
			return fmt.Errorf("migrate: listar contactos en %s: %w", r.d.Src.Label(), err)
		}
		for _, c := range page {
			r.seen++
			if _, ok := r.done[c.ID]; ok {
				continue // idempotencia.
			}
			if strings.TrimSpace(c.FormattedName) == "" {
				r.skip(c.ID, domain.KindContact, "", "el contacto de origen no tiene nombre ni organización: Sytue Contacts exige un nombre visible")
				continue
			}
			id, err := r.d.Dst.CreateContact(ctx, book, c)
			if err != nil {
				r.counts.Errors++
				r.job.LastError = err.Error()
				continue
			}
			r.record(domain.MigrationItem{SourceID: c.ID, Kind: domain.KindContact, SytueID: id, Path: c.FormattedName, Hash: hashContact(c)})
		}
		// Checkpoint por página: el cursor apunta ya a la SIGUIENTE, así que una
		// interrupción aquí no repite la página recién importada.
		if err := r.checkpoint(ctx, next); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		token = next
	}
}

// --- Calendario --------------------------------------------------------------

// calendar recrea los calendarios del origen y trae sus eventos.
//
// El cursor lleva el calendario en curso Y su página, porque un trabajo recorre
// varios calendarios: sin eso, reanudar volvería a empezar por el primero.
func (r *run) calendar(ctx context.Context) error {
	src, ok := r.d.Src.(CalendarSource)
	if !ok {
		return fmt.Errorf("migrate: %s no tiene calendarios que migrar", r.d.Src.Label())
	}
	if r.d.Cal == nil {
		return fmt.Errorf("migrate: este despliegue no tiene Sytue Calendar configurado, así que no puede recibir calendarios")
	}

	list, err := src.ListCalendars(ctx)
	if err != nil {
		return fmt.Errorf("migrate: listar calendarios en %s: %w", r.d.Src.Label(), err)
	}
	startCal, startPage := splitCalCursor(r.job.Cursor)
	skipping := startCal != ""

	for _, c := range list {
		if skipping {
			if c.ID != startCal {
				continue // ya recorrido en una pasada anterior.
			}
			skipping = false
		}
		if c.Skip != "" {
			r.skip("cal:"+c.ID, domain.KindCalendar, c.Name, c.Skip)
			continue
		}
		destID, err := r.d.Cal.EnsureCalendar(ctx, c)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		if _, done := r.done["cal:"+c.ID]; !done {
			r.record(domain.MigrationItem{
				SourceID: "cal:" + c.ID, Kind: domain.KindCalendar, SytueID: destID, Path: c.Name,
			})
		}
		if err := r.events(ctx, src, c, destID, startPage); err != nil {
			return err
		}
		startPage = "" // el siguiente calendario empieza por su primera página.
	}
	return r.checkpoint(ctx, "")
}

// events trae los eventos de UN calendario, página a página.
func (r *run) events(ctx context.Context, src CalendarSource, c Calendar, destID, page string) error {
	for {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		list, next, err := src.ListEvents(ctx, c.ID, page)
		if err != nil {
			_ = r.checkpoint(ctx, calCursor(c.ID, page))
			return fmt.Errorf("migrate: listar eventos de %q: %w", c.Name, err)
		}
		for _, e := range list {
			r.seen++
			if _, done := r.done["event:"+e.ID]; done {
				continue
			}
			path := c.Name + " · " + e.Summary
			if e.Skip != "" {
				r.skip("event:"+e.ID, domain.KindEvent, path, e.Skip)
				continue
			}
			id, err := r.d.Cal.CreateEvent(ctx, destID, e)
			switch {
			case errors.Is(err, ErrUnimportable):
				r.skip("event:"+e.ID, domain.KindEvent, path, err.Error())
				continue
			case err != nil:
				r.counts.Errors++
				r.job.LastError = err.Error()
				continue
			}
			r.record(domain.MigrationItem{
				SourceID: "event:" + e.ID, Kind: domain.KindEvent, SytueID: id, Path: path,
				Hash: sum(e.ID + "|" + e.Start.UTC().Format(time.RFC3339)),
			})
		}
		if err := r.checkpoint(ctx, calCursor(c.ID, next)); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		page = next
	}
}

// calCursor y splitCalCursor serializan «por dónde iba»: calendario y página.
func calCursor(calendarID, page string) string {
	if page == "" {
		return ""
	}
	return calendarID + "|" + page
}

func splitCalCursor(cursor string) (calendarID, page string) {
	calendarID, page, _ = strings.Cut(strings.TrimSpace(cursor), "|")
	return calendarID, page
}

// --- Correo ------------------------------------------------------------------

// mail trae el buzón del origen: primero el mapa de carpetas, después los
// mensajes página a página, cada uno a la carpeta donde estaba.
//
// El cursor es el de paginación del origen y el checkpoint va por página: un
// buzón de decenas de miles de mensajes se reanuda donde lo dejó, y lo ya
// importado no se repite (el mapa de idempotencia lleva el id de origen).
func (r *run) mail(ctx context.Context) error {
	src, ok := r.d.Src.(MailSource)
	if !ok {
		return fmt.Errorf("migrate: %s no tiene buzón que migrar", r.d.Src.Label())
	}
	if r.d.Mail == nil {
		return fmt.Errorf("migrate: este despliegue no tiene Sytue Mail configurado, así que no puede recibir correo")
	}

	folders, err := r.mailFolders(ctx, src)
	if err != nil {
		return err
	}

	cursor := strings.TrimSpace(r.job.Cursor)
	for {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		page, next, err := src.ListMessages(ctx, cursor)
		if err != nil {
			_ = r.checkpoint(ctx, cursor)
			return fmt.Errorf("migrate: listar mensajes en %s: %w", r.d.Src.Label(), err)
		}
		for _, m := range page {
			r.seen++
			if _, done := r.done[m.ID]; done {
				continue // idempotencia: ya importado en una pasada anterior.
			}
			r.copyMessage(ctx, src, folders, m)
			if len(r.batch) >= batchItems {
				if err := r.checkpoint(ctx, cursor); err != nil {
					return err
				}
			}
		}
		// El cursor apunta ya a la SIGUIENTE página: una interrupción aquí no
		// repite la que se acaba de importar.
		if err := r.checkpoint(ctx, next); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

// mailFolders crea en destino las carpetas del origen y devuelve su equivalencia.
// Se hace ANTES de traer un solo mensaje: sin carpeta a la que ir, un correo
// acabaría en la bandeja de entrada aunque en origen estuviera archivado.
func (r *run) mailFolders(ctx context.Context, src MailSource) (map[string]string, error) {
	list, err := src.ListFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: listar carpetas en %s: %w", r.d.Src.Label(), err)
	}
	out := make(map[string]string, len(list))
	for _, f := range list {
		if f.Skip != "" {
			r.skip("folder:"+f.ID, domain.KindMailbox, f.Name, f.Skip)
			continue
		}
		id, err := r.d.Mail.EnsureFolder(ctx, f)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		out[f.ID] = id
		if _, done := r.done["folder:"+f.ID]; !done {
			r.record(domain.MigrationItem{
				SourceID: "folder:" + f.ID, Kind: domain.KindMailbox, SytueID: id, Path: f.Name,
			})
		}
	}
	return out, r.checkpoint(ctx, r.job.Cursor)
}

// copyMessage trae UN mensaje a su carpeta.
func (r *run) copyMessage(ctx context.Context, src MailSource, folders map[string]string, m MailMessage) {
	if m.Skip != "" {
		r.skip(m.ID, domain.KindMessage, m.Subject, m.Skip)
		return
	}
	dest, ok := folders[m.FolderID]
	if !ok {
		r.skip(m.ID, domain.KindMessage, m.Subject,
			"el mensaje estaba en una carpeta que no se pudo crear en Sytue Mail")
		return
	}
	raw, err := src.OpenMessage(ctx, m.ID)
	if errors.Is(err, ErrUnimportable) {
		r.skip(m.ID, domain.KindMessage, m.Subject, err.Error())
		return
	}
	if err != nil {
		r.counts.Errors++
		r.job.LastError = err.Error()
		return
	}
	id, err := r.d.Mail.ImportMessage(ctx, dest, raw, m)
	switch {
	case errors.Is(err, ErrUnimportable):
		r.skip(m.ID, domain.KindMessage, m.Subject, err.Error())
		return
	case err != nil:
		r.counts.Errors++
		r.job.LastError = err.Error()
		return
	}
	r.record(domain.MigrationItem{
		SourceID: m.ID, Kind: domain.KindMessage, SytueID: id, Path: m.Subject,
		Hash: sum(m.ID + "|" + m.ReceivedAt.UTC().Format(time.RFC3339)),
	})
	r.counts.Bytes += int64(len(raw))
}

// --- Equipos y canales -------------------------------------------------------

// teams recrea en Sytue Rooms los equipos, canales y membresías del origen.
//
// No hay cursor: el volumen es pequeño (equipos y canales, no mensajes) y el mapa
// de idempotencia basta para reanudar —volver a listar los equipos es barato y una
// entidad ya creada no se vuelve a tocar—. Lo caro de Teams es el contenido, y ese
// viaja por el dominio 'team_files' con su propio cursor.
func (r *run) teams(ctx context.Context) error {
	src, ok := r.d.Src.(TeamSource)
	if !ok {
		return fmt.Errorf("migrate: %s no tiene equipos que migrar", r.d.Src.Label())
	}
	if r.d.Chat == nil {
		return fmt.Errorf("migrate: este despliegue no tiene Sytue Rooms configurado, así que no puede recibir equipos")
	}

	list, err := src.ListTeams(ctx)
	if err != nil {
		return fmt.Errorf("migrate: listar equipos en %s: %w", r.d.Src.Label(), err)
	}
	for _, tm := range list {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		r.seen++
		teamID, err := r.d.Chat.EnsureTeam(ctx, tm.Name, tm.Description)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		if _, done := r.done["team:"+tm.ID]; !done {
			r.record(domain.MigrationItem{
				SourceID: "team:" + tm.ID, Kind: domain.KindTeam, SytueID: teamID, Path: tm.Name,
			})
		}
		if err := r.channels(ctx, src, tm, teamID); err != nil {
			return err
		}
		if err := r.checkpoint(ctx, ""); err != nil {
			return err
		}
	}
	return r.checkpoint(ctx, "")
}

// channels recrea los canales de UN equipo y sus miembros.
func (r *run) channels(ctx context.Context, src TeamSource, tm Team, destTeamID string) error {
	chans, err := src.ListChannels(ctx, tm.ID)
	if err != nil {
		// Un equipo ilegible NO tumba la tanda: se cuenta como fallo (el trabajo
		// cerrará en 'error' y reanudarlo lo reintenta) y se sigue con los demás.
		r.counts.Errors++
		r.job.LastError = err.Error()
		return nil //nolint:nilerr // degradación deliberada, no un error tragado.
	}
	for _, c := range chans {
		r.seen++
		path := tm.Name + "/" + c.Name
		if _, done := r.done["chan:"+c.ID]; done {
			continue
		}
		if c.Skip != "" {
			r.skip("chan:"+c.ID, domain.KindChannel, path, c.Skip)
			continue
		}
		chID, err := r.d.Chat.EnsureChannel(ctx, destTeamID, c.Name, c.Topic)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		r.record(domain.MigrationItem{
			SourceID: "chan:" + c.ID, Kind: domain.KindChannel, SytueID: chID, Path: path,
		})
		r.members(ctx, src, tm, c, destTeamID, chID, path)
		if len(r.batch) >= batchItems {
			if err := r.checkpoint(ctx, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// members añade al canal de Sytue a las personas que estaban en el de origen.
//
// Quien no tenga cuenta en Sytue NO se añade: Rooms identifica a cada cual por su
// sujeto de Sytue ID, y meter ahí un correo suelto crearía un miembro fantasma que
// jamás coincidiría con quien inicia sesión. Se registra con su motivo.
func (r *run) members(ctx context.Context, src TeamSource, tm Team, c Channel, destTeamID, destChannelID, path string) {
	if r.d.People == nil {
		return
	}
	people, err := src.ListChannelMembers(ctx, tm.ID, c.ID)
	if err != nil {
		r.counts.Errors++
		r.job.LastError = err.Error()
		return
	}
	for _, m := range people {
		id := "member:" + c.ID + "|" + strings.ToLower(m.Email)
		if _, done := r.done[id]; done {
			continue
		}
		r.seen++
		subject, name, ok, err := r.d.People.Subject(ctx, m.Email)
		if err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		if !ok {
			r.skip(id, domain.KindMember, path+" · "+m.Email,
				"esa persona no tiene cuenta en esta organización de Sytue: se deja fuera del canal en vez de inventar un miembro que nadie podría usar")
			continue
		}
		if name == "" {
			name = m.Name
		}
		if err := r.d.Chat.AddMember(ctx, destTeamID, destChannelID, subject, name); err != nil {
			r.counts.Errors++
			r.job.LastError = err.Error()
			continue
		}
		r.record(domain.MigrationItem{
			SourceID: id, Kind: domain.KindMember, SytueID: subject, Path: path + " · " + m.Email,
		})
	}
}

// --- Repositorios ------------------------------------------------------------

// repos importa a Sytue Git los repositorios que el origen deja ver, página a
// página. El cursor es el de paginación del origen.
//
// Aquí NO se copia contenido: por cada repositorio se le pide a Sytue Git que lo
// importe, y la forja hace el trabajo (historia completa, ramas, etiquetas,
// issues, PRs, wiki, releases). Una llamada por repositorio, con el mismo mapa de
// idempotencia que el resto: un repositorio ya importado no se vuelve a pedir.
func (r *run) repos(ctx context.Context) error {
	src, ok := r.d.Src.(RepoSource)
	if !ok {
		return fmt.Errorf("migrate: %s no tiene repositorios que migrar", r.d.Src.Label())
	}
	if r.d.Git == nil {
		return fmt.Errorf("migrate: este despliegue no tiene Sytue Git configurado, así que no puede recibir repositorios")
	}
	cursor := strings.TrimSpace(r.job.Cursor)
	for {
		if r.seen >= maxItems {
			return fmt.Errorf("migrate: el origen supera el tope de %d entidades por trabajo", maxItems)
		}
		page, next, err := src.ListRepos(ctx, cursor)
		if err != nil {
			_ = r.checkpoint(ctx, cursor)
			return fmt.Errorf("migrate: listar repositorios en %s: %w", r.d.Src.Label(), err)
		}
		for _, repo := range page {
			r.seen++
			if _, ok := r.done[repo.ID]; ok {
				continue // idempotencia: ya importado en una pasada anterior.
			}
			// El origen puede saber ya que un repositorio no se puede traer (vacío,
			// archivado sin contenido): se registra con su motivo, no se intenta.
			if repo.Empty {
				r.skip(repo.ID, domain.KindRepo, repo.FullName, "el repositorio de origen está vacío (ningún commit)")
				continue
			}
			path, err := r.d.Git.MigrateRepo(ctx, r.job.TargetUser, repo)
			switch {
			case errors.Is(err, ErrUnimportable):
				// Definitivo: reintentar no lo arregla. Omisión con motivo.
				r.skip(repo.ID, domain.KindRepo, repo.FullName, err.Error())
				continue
			case err != nil:
				r.counts.Errors++
				r.job.LastError = err.Error()
				continue
			}
			r.record(domain.MigrationItem{
				SourceID: repo.ID, Kind: domain.KindRepo, SytueID: path,
				Path: repo.FullName, Hash: sum(repo.FullName + "|" + repo.CloneURL),
			})
		}
		if err := r.checkpoint(ctx, next); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

// --- Hashes ------------------------------------------------------------------

// hashItem/hashContact fijan el estado de origen visto al copiar la entidad.
//
// ponytail: hoy el hash SOLO se guarda (para una futura pasada delta). La
// migración es un one-shot: un id ya mapeado nunca se reescribe aunque el origen
// haya cambiado — no duplicar pesa más que refrescar. Si hiciera falta re-sync,
// comparar aquí y actualizar en destino.
func hashItem(it Item) string {
	return sum(it.Name + "|" + strconv.FormatInt(it.Size, 10) + "|" + it.Modified.UTC().Format(time.RFC3339) + "|" + it.MimeType)
}

func hashContact(c Contact) string {
	b, _ := json.Marshal(c) // struct: orden de campos determinista.
	return sum(string(b) + "|" + strings.Join(c.Emails, ",") + "|" + strings.Join(c.Phones, ","))
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
