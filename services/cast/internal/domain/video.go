// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain es el modelo de Sytue Cast: el vídeo, subido o en directo, con
// su visibilidad y su ciclo de vida, las personas con las que se comparte y los
// trabajos de transcodificación. Sin dependencias de transporte ni de base de
// datos; los CHECK de la migración 0001 dicen lo mismo que este paquete
// (CAST-ENCARGO §4 y §5.1).
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Errores del dominio. La capa HTTP los traduce al sobre canónico.
var (
	// ErrNoExiste: el vídeo no existe o quien pregunta no puede verlo. Son el
	// mismo error a propósito: un 403 diría que existe.
	ErrNoExiste = errors.New("cast: no existe")
	// ErrProhibido: lo ve pero no puede hacer eso.
	ErrProhibido = errors.New("cast: prohibido")
	// ErrEntradaInvalida: un campo fuera de sus reglas.
	ErrEntradaInvalida = errors.New("cast: entrada inválida")
	// ErrConflicto: el vídeo no está en el estado que la operación exige.
	ErrConflicto = errors.New("cast: conflicto de estado")
	// ErrDemasiadoGrande: el fichero pasa del tope por vídeo (413).
	ErrDemasiadoGrande = errors.New("cast: el fichero es demasiado grande")
	// ErrSinCuota: la organización no tiene sitio en su plan (507).
	ErrSinCuota = errors.New("cast: sin cuota")
)

// SinCuota es ErrSinCuota con los bytes que quedan, para el `details` del 507.
type SinCuota struct{ Disponibles int64 }

func (e *SinCuota) Error() string {
	return fmt.Sprintf("%v: quedan %d bytes", ErrSinCuota, e.Disponibles)
}

// Is hace que errors.Is(err, ErrSinCuota) valga.
func (e *SinCuota) Is(objetivo error) bool { return objetivo == ErrSinCuota }

// Kind es cómo llega el vídeo.
type Kind string

// Clases de vídeo.
const (
	KindUpload Kind = "upload"
	KindLive   Kind = "live"
)

// Status es el punto del ciclo de vida.
type Status string

// Estados. Ver PuedePasar.
const (
	StatusUploading  Status = "uploading"
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
	StatusScheduled  Status = "scheduled"
	StatusLive       Status = "live"
	StatusEnded      Status = "ended"
)

// Visibility es quién puede verlo.
type Visibility string

// Visibilidades.
const (
	// VisibilityPublic: portada, canal de la organización, enlace e incrustado.
	VisibilityPublic Visibility = "public"
	// VisibilityUnlisted: cualquiera con el enlace; incrustable, fuera de listados.
	VisibilityUnlisted Visibility = "unlisted"
	// VisibilityOrg: la organización del vídeo.
	VisibilityOrg Visibility = "org"
	// VisibilityPeople: las personas elegidas por @usuario, de cualquier organización.
	VisibilityPeople Visibility = "people"
)

// LiveSource es por dónde entra la señal de un directo.
type LiveSource string

// Orígenes del directo.
const (
	LiveSourceRTMP   LiveSource = "rtmp"
	LiveSourceWebRTC LiveSource = "webrtc"
)

// Topes, los mismos que los CHECK de la migración 0001 y las decisiones del dueño.
const (
	MaxTitulo      = 200
	MaxDescripcion = 5000
	// MaxPersonas acota con cuántas personas se comparte un vídeo: más que eso es
	// una organización, y para eso está `org`.
	MaxPersonas = 200
	// MaxOriginalBytes es el tope por vídeo (decisión 6: 8 GiB).
	MaxOriginalBytes int64 = 8 << 30
	// MaxPosterBytes acota el póster propio (§5.2: 5 MiB).
	MaxPosterBytes int64 = 5 << 20
)

// Valida dice si la clase es una de las dos.
func (k Kind) Valida() bool { return k == KindUpload || k == KindLive }

// Valida dice si la visibilidad es una de las cuatro.
func (v Visibility) Valida() bool {
	switch v {
	case VisibilityPublic, VisibilityUnlisted, VisibilityOrg, VisibilityPeople:
		return true
	}
	return false
}

// Abierta dice si se puede ver sin sesión (y por tanto incrustar).
func (v Visibility) Abierta() bool { return v == VisibilityPublic || v == VisibilityUnlisted }

// Valida dice si el origen del directo es uno de los dos.
func (s LiveSource) Valida() bool { return s == LiveSourceRTMP || s == LiveSourceWebRTC }

// Visible dice si el estado es uno que ven quienes no lo editan. Coincide con
// las políticas `videos_publico` y `videos_persona`.
func (s Status) Visible() bool { return s == StatusReady || s == StatusLive }

// transiciones son los pasos válidos del ciclo de vida.
var transiciones = map[Status][]Status{
	StatusUploading:  {StatusProcessing, StatusFailed},
	StatusProcessing: {StatusReady, StatusFailed},
	StatusFailed:     {StatusProcessing},
	StatusScheduled:  {StatusLive, StatusEnded},
	StatusLive:       {StatusEnded},
	// Un directo acabado se procesa: su grabación queda como vídeo bajo demanda.
	StatusEnded: {StatusProcessing},
	// Un vídeo listo no vuelve atrás: pedir otro póster es un trabajo aparte que no
	// toca el ciclo de vida (si lo tocara, el vídeo desaparecería del público
	// mientras se cuece, y un póster fallido lo dejaría en `failed`).
	StatusReady: {},
}

// PuedePasar dice si el vídeo puede ir de `de` a `a`.
func PuedePasar(de, a Status) bool {
	for _, s := range transiciones[de] {
		if s == a {
			return true
		}
	}
	return false
}

// EstadoInicial es con el que nace un vídeo de esa clase.
func EstadoInicial(k Kind) Status {
	if k == KindLive {
		return StatusScheduled
	}
	return StatusUploading
}

// Video es un vídeo de Sytue Cast (fila de `videos`).
type Video struct {
	ID          uuid.UUID
	Tenant      string
	OwnerSub    string
	OwnerName   string
	Kind        Kind
	Status      Status
	Visibility  Visibility
	Title       string
	Description string
	DurationMS  *int64
	Width       *int32
	Height      *int32
	OriginalKey string
	HlsKey      string
	WebmKey     string
	PosterKey   string
	// Qualities es la elección de calidades de quien lo subió (nil = la
	// recomendada: todas las que el original permita). HlsQualities y
	// WebmQualities, las que existen ya en cada escalera (nil = un vídeo de antes
	// de la elección; ver CalidadesHLS y CalidadesWebM).
	Qualities       []int32
	HlsQualities    []int32
	WebmQualities   []int32
	BytesOriginal   int64
	BytesTotal      int64
	CommentsEnabled bool
	ChatEnabled     bool
	DownloadEnabled bool
	LiveRoom        string
	LiveSource      LiveSource
	LiveStartedAt   *time.Time
	LiveEndedAt     *time.Time
	LiveEgressID    string
	ViewersNow      int32
	ViewersPeak     int32
	Views           int64
	Failure         string
	PublishedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EsDirecto dice si nació como directo (aunque ya sea una grabación lista).
func (v Video) EsDirecto() bool { return v.Kind == KindLive }

// Persona es alguien con quien se comparte un vídeo `people` (fila de `video_people`).
type Persona struct {
	UserID   string
	Name     string
	Username string
	AddedAt  time.Time
}

// Cambios son los campos que el dueño puede tocar; nil = no se toca.
type Cambios struct {
	Title           *string
	Description     *string
	Visibility      *Visibility
	CommentsEnabled *bool
	ChatEnabled     *bool
	DownloadEnabled *bool
}

// Vacios dice si no cambia nada.
func (c Cambios) Vacios() bool {
	return c.Title == nil && c.Description == nil && c.Visibility == nil &&
		c.CommentsEnabled == nil && c.ChatEnabled == nil && c.DownloadEnabled == nil
}

// Titulo normaliza y valida un título: recortado, de 1 a 200 runas, sin saltos de
// línea (se pinta en una sola).
func Titulo(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return "", fmt.Errorf("%w: el título está vacío", ErrEntradaInvalida)
	}
	if n > MaxTitulo {
		return "", fmt.Errorf("%w: el título tiene %d caracteres y el tope es %d", ErrEntradaInvalida, n, MaxTitulo)
	}
	return s, nil
}

// Descripcion normaliza y valida la descripción: recortada, hasta 5000 runas.
func Descripcion(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n > MaxDescripcion {
		return "", fmt.Errorf("%w: la descripción tiene %d caracteres y el tope es %d", ErrEntradaInvalida, n, MaxDescripcion)
	}
	return s, nil
}

// Normalizar valida los cambios y los deja listos para guardar.
func (c *Cambios) Normalizar() error {
	if c.Title != nil {
		t, err := Titulo(*c.Title)
		if err != nil {
			return err
		}
		c.Title = &t
	}
	if c.Description != nil {
		d, err := Descripcion(*c.Description)
		if err != nil {
			return err
		}
		c.Description = &d
	}
	if c.Visibility != nil && !c.Visibility.Valida() {
		return fmt.Errorf("%w: visibilidad %q desconocida", ErrEntradaInvalida, *c.Visibility)
	}
	return nil
}

// ── Cola de transcodificación ───────────────────────────────────────────────

// JobKind es qué trabajo hay que hacer con el vídeo.
type JobKind string

// Clases de trabajo.
const (
	// JobUpload: un original recién subido → HLS, WebM y póster.
	JobUpload JobKind = "upload"
	// JobLiveVOD: la grabación de un directo (el HLS del egress) → WebM y póster.
	JobLiveVOD JobKind = "live_vod"
	// JobPoster: un fotograma elegido (payload.at_ms) → póster.
	JobPoster JobKind = "poster"
	// JobQualities: poner las calidades generadas al día con las elegidas —
	// generar las que faltan en las dos escaleras y borrar las que sobran—. Lo
	// encola cambiar la elección, y sirve también para dar la escalera WebM
	// completa a los vídeos de antes de ella, que solo tenían 720p.
	JobQualities JobKind = "qualities"
)

// JobStatus es el punto de un trabajo.
type JobStatus string

// Estados de un trabajo.
const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

// MaxIntentos: tres reintentos; el cuarto fallo deja el vídeo en `failed`.
const MaxIntentos = 3

// Valida dice si la clase de trabajo es una de las cuatro.
func (k JobKind) Valida() bool {
	return k == JobUpload || k == JobLiveVOD || k == JobPoster || k == JobQualities
}

// MueveElVideo dice si el trabajo lleva el vídeo por `processing`: la subida y la
// grabación de un directo sí; un póster o un cambio de calidades, no (el vídeo
// sigue visible con lo que ya tenía mientras se cuece).
func (k JobKind) MueveElVideo() bool { return k == JobUpload || k == JobLiveVOD }

// Huerfano es cuánto puede estar un trabajo en `running` sin que su réplica lo
// termine antes de darlo por perdido (la réplica murió) y devolverlo a la cola.
// Por encima del tope de un trabajo (TRANSCODER_MAX_MINUTES, 120 por defecto).
const Huerfano = 3 * time.Hour

// Job es un trabajo de transcodificación (fila de `transcode_jobs`).
type Job struct {
	ID        uuid.UUID
	Tenant    string
	VideoID   uuid.UUID
	Kind      JobKind
	Status    JobStatus
	Attempts  int32
	ClaimedAt *time.Time
	ClaimedBy string
	Error     string
	// Payload es JSON tal cual; el transcodificador sabe qué espera cada clase.
	Payload   []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}
