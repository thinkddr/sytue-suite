// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "time"

// MigrationSource es el SaaS del que se importa contenido.
type MigrationSource string

const (
	SourceM365   MigrationSource = "m365"
	SourceGoogle MigrationSource = "google"
	SourceGitHub MigrationSource = "github"
)

// Valid indica si el origen es uno de los soportados.
func (s MigrationSource) Valid() bool { return len(s.Domains()) > 0 }

// MigrationDirection dice hacia dónde va el contenido.
//
// Poder IRSE es parte del producto. Una plataforma de la que no se puede salir
// no es soberana: es otra jaula, solo que con la bandera cambiada. El mismo
// motor que trae contenido lo lleva, con las mismas garantías —idempotencia,
// reanudación y honestidad sobre lo que no viaja—.
type MigrationDirection string

const (
	// DirectionIn trae contenido del tercero a Sytue. Es el valor por defecto:
	// los trabajos que ya existían no declaraban dirección.
	DirectionIn MigrationDirection = "in"
	// DirectionOut lleva contenido de Sytue al tercero.
	DirectionOut MigrationDirection = "out"
)

// Valid indica si la dirección es una de las dos que hay.
func (d MigrationDirection) Valid() bool {
	return d == DirectionIn || d == DirectionOut
}

// OrIn normaliza la dirección vacía a la de entrada.
func (d MigrationDirection) OrIn() MigrationDirection {
	if d == "" {
		return DirectionIn
	}
	return d
}

// MigrationDomain es el tipo de contenido que migra un trabajo. Correo y
// calendario NO están: son otra ola (hilos, adjuntos, recurrencias, zonas
// horarias) y aceptarlos aquí sería prometer algo que no se copia.
type MigrationDomain string

const (
	DomainDrive    MigrationDomain = "drive"
	DomainContacts MigrationDomain = "contacts"
	// DomainRepos son los repositorios de una forja (GitHub) hacia Sytue Git:
	// historia git completa, ramas, etiquetas, issues, pull requests, wiki y
	// releases. No aterriza en Drive: un repositorio archivado en un fichero no es
	// un repositorio.
	DomainRepos MigrationDomain = "repos"
	// DomainGists son los fragmentos de código de GitHub, que SÍ son ficheros: cada
	// gist aterriza en Sytue Drive como una carpeta con sus ficheros dentro.
	DomainGists MigrationDomain = "gists"
	// DomainTeams son los equipos, canales y miembros de Microsoft Teams, que se
	// recrean en Sytue Rooms para poder seguir trabajando el primer día.
	DomainTeams MigrationDomain = "teams"
	// DomainTeamFiles es el contenido de esos canales —los ficheros de SharePoint y
	// el historial de conversación— hacia Sytue Drive, una carpeta por equipo y
	// canal. El historial va como transcripción con AUTOR Y FECHA REALES dentro: la
	// API de Rooms publica los mensajes como quien los escribe ahora, así que
	// volcarlos allí falsificaría la autoría o la fecha.
	DomainTeamFiles MigrationDomain = "team_files"
	// DomainMail es el buzón: mensajes con su carpeta, su fecha y su estado de
	// leído tal y como estaban en el origen.
	DomainMail MigrationDomain = "mail"
	// DomainCalendar son los calendarios y sus eventos, con recurrencias,
	// asistentes y zonas horarias.
	DomainCalendar MigrationDomain = "calendar"
	// DomainCloud son las APLICACIONES de Google Cloud y Firebase hacia Sytue
	// Cloud: cada servicio de Cloud Run o sitio de Firebase Hosting llega como un
	// proyecto con sus variables, sus secretos, su escala y sus dominios; los
	// buckets llegan con sus objetos dentro.
	//
	// Lo que NO viaja se dice por su nombre, no se calla: una base de datos
	// (Cloud SQL, Firestore, BigQuery) no se copia por una API de control —hay
	// que exportarla y restaurarla, y eso lo decide quien es dueño de esos
	// datos—, así que se enumera con su motivo en el informe.
	DomainCloud MigrationDomain = "cloud"
)

// Domains son los tipos de contenido que ESE origen sabe migrar. Es la lista que
// el asistente automático recorre y la que valida la API: pedir contactos a GitHub
// (que no tiene agenda) o repositorios a Google no es un trabajo vacío, es un
// trabajo imposible, y se rechaza al crearlo en vez de morir en el outbox.
func (s MigrationSource) Domains() []MigrationDomain {
	switch s {
	case SourceM365:
		return []MigrationDomain{DomainDrive, DomainContacts, DomainTeams, DomainTeamFiles}
	case SourceGoogle:
		return []MigrationDomain{DomainDrive, DomainContacts, DomainMail, DomainCalendar, DomainCloud}
	case SourceGitHub:
		return []MigrationDomain{DomainRepos, DomainGists}
	default:
		return nil
	}
}

// Supports dice si el origen sabe migrar ese tipo de contenido.
func (s MigrationSource) Supports(d MigrationDomain) bool {
	for _, v := range s.Domains() {
		if v == d {
			return true
		}
	}
	return false
}

// Valid indica si el dominio es uno de los soportados EN ESTA OLA (por algún
// origen; que lo soporte EL origen del trabajo lo dice Supports).
func (d MigrationDomain) Valid() bool {
	switch d {
	case DomainDrive, DomainContacts, DomainRepos, DomainGists, DomainTeams,
		DomainTeamFiles, DomainMail, DomainCalendar, DomainCloud:
		return true
	default:
		return false
	}
}

// PerPerson dice si ese contenido es de CADA PERSONA (su Drive, su agenda) o de la
// organización (los equipos de Teams, que son compartidos).
//
// Importa para el asistente automático: migrar el Drive de 300 personas son 300
// trabajos, pero migrar los equipos 300 veces sería copiar los mismos canales 300
// veces. Lo compartido se migra UNA vez, a nombre de quien lanza la migración.
func (d MigrationDomain) PerPerson() bool {
	// Las aplicaciones de Google Cloud son de la ORGANIZACIÓN, como los equipos:
	// migrarlas una vez por persona crearía el mismo proyecto trescientas veces.
	return d != DomainTeams && d != DomainTeamFiles && d != DomainCloud
}

// MigrationStatus es el estado del ciclo de vida de un trabajo de migración.
type MigrationStatus string

const (
	MigrationQueued   MigrationStatus = "queued"   // encolado en el outbox
	MigrationRunning  MigrationStatus = "running"  // recorriendo el origen
	MigrationDone     MigrationStatus = "done"     // recorrido completo
	MigrationError    MigrationStatus = "error"    // abortado; el cursor permite reanudar
	MigrationCanceled MigrationStatus = "canceled" // cancelado por el usuario
)

// MigrationKind distingue el tipo de entidad mapeada en migration_map.
type MigrationKind string

const (
	KindFolder  MigrationKind = "folder"
	KindFile    MigrationKind = "file"
	KindContact MigrationKind = "contact"
	KindRepo    MigrationKind = "repo"
	// KindDoc/KindSpreadsheet/KindPresentation son documentos EDITABLES creados en
	// Sytue Docs (no ficheros en Drive): un Google Doc migrado se abre y se edita.
	KindDoc          MigrationKind = "document"
	KindSpreadsheet  MigrationKind = "spreadsheet"
	KindPresentation MigrationKind = "presentation"
	KindMessage      MigrationKind = "message"
	KindCalendar     MigrationKind = "calendar"
	KindEvent        MigrationKind = "event"
	KindMailbox      MigrationKind = "mailbox"
	KindTeam         MigrationKind = "team"
	KindChannel      MigrationKind = "channel"
	KindMember       MigrationKind = "member"
	// Entidades de Sytue Cloud. KindDatabase existe para poder DECIR que una base
	// de datos del origen no se copia: sin entrada en el mapa, una Cloud SQL
	// quedaría fuera del informe y parecería que no había nada.
	KindCloudApp    MigrationKind = "cloud_app"
	KindBucket      MigrationKind = "bucket"
	KindObject      MigrationKind = "object"
	KindCloudSecret MigrationKind = "cloud_secret"
	KindCloudJob    MigrationKind = "cloud_job"
	KindDatabase    MigrationKind = "database"
)

// MigrationCounts resume un trabajo con HONESTIDAD: Migrated son entidades
// efectivamente creadas en Sytue; Skipped las que NO se copiaron (cada una con su
// motivo en migration_map); Errors los fallos transitorios (se reintentan al
// reanudar). Migrated nunca incluye omitidas ni fallidas.
type MigrationCounts struct {
	Migrated int   `json:"migrated"`
	Skipped  int   `json:"skipped"`
	Errors   int   `json:"errors"`
	Bytes    int64 `json:"bytes_copied"`
}

// MigrationJob es un trabajo de importación de contenido de una organización: de
// UNA cuenta del origen a la cuenta de UNA persona en Sytue. El cursor NUNCA se
// serializa (es un puntero interno).
//
// TargetUser es el usuario de Sytue (correo o uuid) en cuya cuenta aterriza todo:
// la pasarela canjea un token acotado a él (RFC 8693) y escribe con ese token, no
// con el de quien lanzó el trabajo.
type MigrationJob struct {
	ID         string          `json:"id"`
	OrgID      string          `json:"org_id"`
	InstanceID string          `json:"instance_id"`
	TargetUser string          `json:"target_user"`
	SourceUser string          `json:"source_user"`
	Source     MigrationSource `json:"source"`
	Domain     MigrationDomain `json:"domain"`
	// Direction: 'in' trae de Source a Sytue; 'out' lleva de Sytue a Source.
	Direction  MigrationDirection `json:"direction"`
	Status     MigrationStatus    `json:"status"`
	Cursor     string             `json:"-"`
	Counts     MigrationCounts    `json:"counts"`
	LastError  string             `json:"last_error,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	StartedAt  *time.Time         `json:"started_at,omitempty"`
	FinishedAt *time.Time         `json:"finished_at,omitempty"`
}

// MigrationItem es un renglón de migration_map. SytueID poblado ⇒ se copió;
// SkipReason poblado ⇒ se omitió Y ese es el porqué. Hash es el estado de origen
// visto al copiarlo.
type MigrationItem struct {
	SourceID   string        `json:"source_id"`
	Kind       MigrationKind `json:"kind,omitempty"`
	SytueID    string        `json:"sytue_id,omitempty"`
	Hash       string        `json:"-"`
	Path       string        `json:"path,omitempty"`
	SkipReason string        `json:"skip_reason,omitempty"`
}
