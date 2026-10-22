// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package directo es el directo de Sytue Cast (CAST-ENCARGO §6): la clave de
// emisión de OBS de cada persona, programar y terminar un directo, el token de
// emisor para quien emite desde el navegador o la app, el token de espectador,
// y el ciclo de vida que dictan los avisos de LiveKit (ingress que empieza,
// pista publicada, gente que entra y sale, room que acaba, grabación lista) con
// un sondeo de respaldo para cuando un aviso se pierde.
//
// El motor es LiveKit (platform/pkg/livekit): OBS entra por RTMP al ingress,
// que publica en la room `cast-<tenant>-<video_id>`; los espectadores ven por
// WebRTC con un token de solo suscripción; el egress graba las pistas del emisor
// en HLS dentro del bucket de Cast y la grabación queda como vídeo bajo demanda.
//
// El tenant de cada transacción es el del vídeo. Los avisos de LiveKit no traen
// organización: se saca del nombre de la room (`cast-<tenant>-<uuid>`) o, para un
// ingress, de a quién pertenece su clave (`stream_keys`, cruzando organizaciones
// solo para eso).
package directo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// ErrWebhookInvalido: el aviso no viene firmado por LiveKit con nuestro secreto.
var ErrWebhookInvalido = errors.New("directo: webhook de LiveKit rechazado")

// Config es lo que el directo necesita saber del despliegue.
type Config struct {
	// LiveKitURL es la URL pública de señalización (wss://rooms-sfu.sytue.eu) que
	// reciben los clientes. Obligatoria: sin ella no hay directo.
	LiveKitURL string
	// IngestRTMPURL es la dirección que se pega en OBS como «servidor»
	// (rtmp://cast-ingest.<dominio>/live). Vacía = la que diga LiveKit del ingress.
	IngestRTMPURL string
	// PublicURL es la de la web (https://cast.sytue.eu): los enlaces de los avisos.
	PublicURL string
	// Grabacion es el destino S3 del egress (el bucket de Cast). Sin bucket o sin
	// credenciales no se graba: el directo se emite igual.
	Grabacion livekit.DestinoS3
	// SegundosSegmento es la duración de cada trozo HLS grabado (4 s por defecto).
	SegundosSegmento int
	// TTLEspectador es la validez del token de quien mira (2 h por defecto).
	TTLEspectador time.Duration
	// PollInterval es cada cuánto se contrasta el estado con LiveKit; 0 apaga el
	// sondeo y el ciclo depende solo de los webhooks.
	PollInterval time.Duration
}

// Deps son las dependencias del directo.
type Deps struct {
	Pool *pg.Pool
	// Store y Servicio son los del núcleo: vídeos, personas, cola, ciclo de vida.
	Store    *store.Store
	Servicio *service.Servicio
	// LiveKit habla con la API del SFU; Firmante mintea tokens y verifica webhooks.
	LiveKit  *livekit.Client
	Firmante *livekit.Signer
	// Hub avisa de «está en directo». nil = sin avisos.
	Hub *hubnotify.Client
	// Bloquean dice cuáles de esas personas tienen bloqueada a quien emite, para
	// no avisarlas. nil = se avisa a todas.
	Bloquean func(ctx context.Context, sub string, candidatos []string) (map[string]bool, error)
	Cfg      Config
	Logger   *slog.Logger
}

// Servicio son los casos de uso del directo.
type Servicio struct {
	propio   *datos
	st       *store.Store
	svc      *service.Servicio
	lk       *livekit.Client
	firma    *livekit.Signer
	hub      *hubnotify.Client
	bloquean func(ctx context.Context, sub string, candidatos []string) (map[string]bool, error)
	cfg      Config
	logger   *slog.Logger
}

// New construye el directo.
func New(d Deps) *Servicio {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Cfg.SegundosSegmento <= 0 {
		d.Cfg.SegundosSegmento = 4
	}
	if d.Cfg.TTLEspectador <= 0 {
		d.Cfg.TTLEspectador = 2 * time.Hour
	}
	return &Servicio{
		propio: &datos{pool: d.Pool}, st: d.Store, svc: d.Servicio,
		lk: d.LiveKit, firma: d.Firmante, hub: d.Hub, bloquean: d.Bloquean, cfg: d.Cfg, logger: d.Logger,
	}
}

// Nucleo es el servicio de vídeos de debajo, para quien pinta las respuestas.
func (s *Servicio) Nucleo() *service.Servicio { return s.svc }

// puedeGrabar dice si hay destino donde dejar la grabación. Las tres piezas: con
// bucket y sin credenciales el egress arrancaría y fallaría al subir.
func (s *Servicio) puedeGrabar() bool {
	g := s.cfg.Grabacion
	return g.Bucket != "" && g.AccessKey != "" && g.Secret != ""
}

// ── Nombres ─────────────────────────────────────────────────────────────────

const (
	prefijoRoom   = "cast-"
	prefijoEmisor = "emisor-"
	prefijoAnon   = "anon-"
)

// nombreRoom es la room de LiveKit de un directo: `cast-<tenant>-<uuid>`. El uuid
// va al final y mide siempre 36, así que se puede separar del tenant aunque el
// slug lleve guiones.
func nombreRoom(tenant string, id uuid.UUID) string { return prefijoRoom + tenant + "-" + id.String() }

// roomDe separa `cast-<tenant>-<uuid>` en su organización; falso si la room no es
// de Cast (el SFU es el mismo que el de Rooms, y sus avisos también llegan aquí).
func roomDe(room string) (tenant string, ok bool) {
	resto, ok := strings.CutPrefix(room, prefijoRoom)
	if !ok || len(resto) < 38 {
		return "", false
	}
	corte := len(resto) - 37
	if resto[corte] != '-' {
		return "", false
	}
	if _, err := uuid.Parse(resto[corte+1:]); err != nil {
		return "", false
	}
	tenant = resto[:corte]
	if !tenancy.ValidSlug(tenant) {
		return "", false
	}
	return tenant, true
}

// identidadEmisor es cómo aparece quien emite en la room, venga por OBS o por WebRTC.
func identidadEmisor(sub string) string { return prefijoEmisor + sub }

// identidadAnonima es la de un espectador sin sesión: `anon-<16 hex>`.
func identidadAnonima() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read no falla en la práctica; si lo hiciera, mejor una identidad
		// única por tiempo que un pánico en la ruta pública.
		return prefijoAnon + fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return prefijoAnon + hex.EncodeToString(b[:])
}

// prefijoGrabacion es dónde deja el egress la grabación dentro del bucket, con el
// prefijo del tenant que pone el almacén: `tenants/<t>/<video_id>/live`.
//
// ponytail: el egress escribe siempre en RECORDING_BUCKET; si la organización
// vive en el almacén remoto (ADR 0038) la lectura iría a otro bucket. El relevo es
// elegir el destino por tenant como hace objectstore.
func prefijoGrabacion(v domain.Video) string {
	return tenancy.Tenant{Slug: v.Tenant}.StoragePrefix() + service.Prefijo(v) + "live"
}

// claveGrabacion es la playlist grabada, relativa a la organización: lo que va en
// `videos.hls_key` y sirve `/media/<id>/<token>/hls/master.m3u8`.
func claveGrabacion(v domain.Video) string {
	return service.Prefijo(v) + "live/" + livekit.PlaylistGrabada
}

// ── Clave de emisión ────────────────────────────────────────────────────────

// ClaveDeEmision es lo que se pega en OBS.
type ClaveDeEmision struct {
	RTMPURL   string
	StreamKey string
	IngressID string
}

func (s *Servicio) claveDeEmision(c Clave, urlLiveKit string) ClaveDeEmision {
	u := s.cfg.IngestRTMPURL
	if u == "" {
		u = urlLiveKit
	}
	return ClaveDeEmision{RTMPURL: u, StreamKey: c.Key, IngressID: c.IngressID}
}

// StreamKey devuelve la clave de OBS de quien llama, creándola al primer GET: un
// ingress de LiveKit apuntado a una room «de espera» que se cambia al programar.
func (s *Servicio) StreamKey(ctx context.Context, quien service.Quien) (ClaveDeEmision, error) {
	c, err := s.clave(ctx, quien)
	if err != nil {
		return ClaveDeEmision{}, err
	}
	return s.claveDeEmision(c, ""), nil
}

func conSesion(q service.Quien) bool { return !q.Anonimo && q.Sub != "" && q.Tenant != "" }

// clave lee la de quien llama o la crea.
func (s *Servicio) clave(ctx context.Context, quien service.Quien) (Clave, error) {
	if !conSesion(quien) {
		return Clave{}, domain.ErrProhibido
	}
	c, err := s.propio.clave(ctx, quien.Tenant, quien.Sub)
	if err == nil || !errors.Is(err, domain.ErrNoExiste) {
		return c, err
	}
	info, err := s.crearIngress(ctx, quien)
	if err != nil {
		return Clave{}, err
	}
	nueva := Clave{Tenant: quien.Tenant, OwnerSub: quien.Sub, OwnerName: quien.Nombre, Key: info.StreamKey, IngressID: info.IngressID}
	guardada, esNueva, err := s.propio.guardarClave(ctx, nueva)
	if err != nil {
		return Clave{}, err
	}
	if !esNueva {
		// Otra petición llegó antes: el ingress que acabamos de crear sobra.
		if err := s.lk.DeleteIngress(ctx, info.IngressID); err != nil {
			s.logger.Warn("cast: no se pudo borrar el ingress sobrante", "ingress", info.IngressID, "error", err)
		}
	}
	return guardada, nil
}

// crearIngress crea la puerta RTMP de una persona, apuntada a una room de espera
// con la misma forma que las de verdad (`cast-<tenant>-<uuid>`): si OBS emite ahí
// sin nada programado, el aviso se resuelve igual y se crea el directo.
func (s *Servicio) crearIngress(ctx context.Context, quien service.Quien) (livekit.IngressInfo, error) {
	info, err := s.lk.CreateIngress(ctx, livekit.IngressRequest{
		Name:                quien.Tenant + "/" + quien.Sub,
		RoomName:            nombreRoom(quien.Tenant, uuid.New()),
		ParticipantIdentity: identidadEmisor(quien.Sub),
		ParticipantName:     quien.Nombre,
	})
	if err != nil {
		return livekit.IngressInfo{}, fmt.Errorf("cast: crear ingress: %w", err)
	}
	return info, nil
}

// RotarClave regenera la clave de OBS: crea otro ingress, lo deja como el de la
// persona y borra el anterior, cuya clave deja de valer al momento.
func (s *Servicio) RotarClave(ctx context.Context, quien service.Quien) (ClaveDeEmision, error) {
	if !conSesion(quien) {
		return ClaveDeEmision{}, domain.ErrProhibido
	}
	if _, err := s.propio.clave(ctx, quien.Tenant, quien.Sub); err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			// Sin clave que rotar, la primera es la nueva.
			return s.StreamKey(ctx, quien)
		}
		return ClaveDeEmision{}, err
	}
	info, err := s.crearIngress(ctx, quien)
	if err != nil {
		return ClaveDeEmision{}, err
	}
	nueva := Clave{Tenant: quien.Tenant, OwnerSub: quien.Sub, OwnerName: quien.Nombre, Key: info.StreamKey, IngressID: info.IngressID}
	anterior, err := s.propio.rotarClave(ctx, nueva)
	if err != nil {
		_ = s.lk.DeleteIngress(ctx, info.IngressID)
		return ClaveDeEmision{}, err
	}
	if err := s.lk.DeleteIngress(ctx, anterior.IngressID); err != nil {
		// La clave vieja sigue valiendo en LiveKit hasta que alguien la borre: se
		// dice, porque es lo contrario de lo que la persona acaba de pedir.
		s.logger.Warn("cast: la clave anterior no se pudo borrar en LiveKit", "ingress", anterior.IngressID, "error", err)
	}
	return s.claveDeEmision(nueva, info.URL), nil
}

// ── Programar, emitir, terminar ─────────────────────────────────────────────

// Programa es lo que se pide para programar un directo.
type Programa struct {
	Title       string
	Description string
	Visibility  domain.Visibility
	// Source: rtmp (OBS) o webrtc (navegador, app). Vacía = rtmp.
	Source domain.LiveSource
}

// Conexion es lo que hace falta para entrar en la room de LiveKit, como emisor o
// como espectador (§5.1 `live` y §5.2 `publish`).
type Conexion struct {
	ServerURL string
	Token     string
	Room      string
	Identity  string
}

// Programar da de alta un directo (`scheduled`) en su room. Con OBS, apunta el
// ingress de quien llama a esa room; con WebRTC, devuelve además el token de emisor.
func (s *Servicio) Programar(ctx context.Context, quien service.Quien, p Programa) (domain.Video, *Conexion, error) {
	if !conSesion(quien) {
		return domain.Video{}, nil, domain.ErrProhibido
	}
	if p.Source == "" {
		p.Source = domain.LiveSourceRTMP
	}
	if !p.Source.Valida() {
		return domain.Video{}, nil, fmt.Errorf("%w: origen %q desconocido", domain.ErrEntradaInvalida, p.Source)
	}
	v, err := s.svc.Crear(ctx, quien, service.Alta{
		Kind: domain.KindLive, Title: p.Title, Description: p.Description, Visibility: p.Visibility, LiveSource: p.Source,
	})
	if err != nil {
		return domain.Video{}, nil, err
	}
	// El room lleva el id, que lo da la base: se fija justo después de nacer.
	room := nombreRoom(v.Tenant, v.ID)
	if err := s.propio.fijarRoom(ctx, v.Tenant, v.ID, room); err != nil {
		return domain.Video{}, nil, err
	}
	v.LiveRoom = room

	if p.Source == domain.LiveSourceRTMP {
		c, err := s.clave(ctx, quien)
		if err != nil {
			return domain.Video{}, nil, err
		}
		if _, err := s.lk.UpdateIngress(ctx, c.IngressID, room, quien.Nombre); err != nil {
			// Sin esto OBS emitiría en la room anterior y saldría OTRO directo. Se
			// dice antes que dejar uno programado que nunca va a arrancar.
			return domain.Video{}, nil, fmt.Errorf("cast: apuntar el ingress al directo: %w", err)
		}
		return v, nil, nil
	}
	emision, err := s.emision(v)
	if err != nil {
		return domain.Video{}, nil, err
	}
	return v, emision, nil
}

// emision es el token de quien emite por WebRTC: identidad `emisor-<sub>`, puede
// publicar y nada más.
func (s *Servicio) emision(v domain.Video) (*Conexion, error) {
	if v.LiveRoom == "" {
		return nil, fmt.Errorf("%w: el directo no tiene room", domain.ErrConflicto)
	}
	si, no := true, false
	identidad := identidadEmisor(v.OwnerSub)
	tok, err := s.firma.AccessToken(livekit.AccessTokenOptions{
		Identity: identidad, Name: v.OwnerName, Room: v.LiveRoom,
		Grant: livekit.VideoGrant{CanPublish: &si, CanSubscribe: &no, CanPublishData: &no},
		TTL:   6 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("cast: token de emisor: %w", err)
	}
	return &Conexion{ServerURL: s.cfg.LiveKitURL, Token: tok, Room: v.LiveRoom, Identity: identidad}, nil
}

// Publicacion es el token de emisor de un directo por WebRTC. Solo el dueño: un
// administrador puede terminarlo, pero no emitir en su nombre.
func (s *Servicio) Publicacion(ctx context.Context, quien service.Quien, id uuid.UUID) (*Conexion, error) {
	if !conSesion(quien) {
		return nil, domain.ErrProhibido
	}
	v, _, err := s.svc.Resolver(ctx, quien, id)
	if err != nil {
		return nil, err
	}
	if v.Tenant != quien.Tenant || v.OwnerSub != quien.Sub {
		return nil, domain.ErrProhibido
	}
	if v.Kind != domain.KindLive || v.LiveSource != domain.LiveSourceWebRTC {
		return nil, fmt.Errorf("%w: solo un directo desde el navegador o la app tiene token de emisor", domain.ErrConflicto)
	}
	if v.Status != domain.StatusScheduled && v.Status != domain.StatusLive {
		return nil, fmt.Errorf("%w: el directo está %s", domain.ErrConflicto, v.Status)
	}
	return s.emision(v)
}

// Terminar acaba el directo por orden de quien lo edita (dueño o administrador):
// para la grabación, cierra la room, deja el vídeo `ended` y, si hay grabación,
// la encola como vídeo bajo demanda.
func (s *Servicio) Terminar(ctx context.Context, quien service.Quien, id uuid.UUID) (domain.Video, error) {
	if !conSesion(quien) {
		return domain.Video{}, domain.ErrProhibido
	}
	v, p, err := s.svc.Resolver(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	if !p.Editar {
		return domain.Video{}, domain.ErrProhibido
	}
	if v.Kind != domain.KindLive || (v.Status != domain.StatusScheduled && v.Status != domain.StatusLive) {
		return domain.Video{}, fmt.Errorf("%w: el directo está %s", domain.ErrConflicto, v.Status)
	}
	if err := s.terminar(ctx, v); err != nil {
		return domain.Video{}, err
	}
	return s.st.EnOrganizacion(ctx, v.Tenant, v.ID)
}

// Play es el token de espectador de un directo en curso: identidad = el sub, o
// `anon-<hex>` sin sesión; solo suscribir, nunca publicar ni mandar datos.
func (s *Servicio) Play(_ context.Context, v domain.Video, quien service.Quien) (*Conexion, error) {
	if v.Status != domain.StatusLive || v.LiveRoom == "" {
		return nil, fmt.Errorf("%w: el directo no está en emisión", domain.ErrConflicto)
	}
	identidad, nombre := identidadAnonima(), ""
	if conSesion(quien) {
		identidad, nombre = quien.Sub, quien.Nombre
	}
	si, no := true, false
	tok, err := s.firma.AccessToken(livekit.AccessTokenOptions{
		Identity: identidad, Name: nombre, Room: v.LiveRoom,
		Grant: livekit.VideoGrant{CanSubscribe: &si, CanPublish: &no, CanPublishData: &no},
		TTL:   s.cfg.TTLEspectador,
	})
	if err != nil {
		return nil, fmt.Errorf("cast: token de espectador: %w", err)
	}
	return &Conexion{ServerURL: s.cfg.LiveKitURL, Token: tok, Room: v.LiveRoom, Identity: identidad}, nil
}

// ── Ciclo de vida desde LiveKit ─────────────────────────────────────────────

// arrancar pasa el directo a `live` cuando entra la señal, arranca la grabación
// y avisa. Idempotente: el webhook y el sondeo pueden pedirlo a la vez, y solo
// el primero consigue la transición.
func (s *Servicio) arrancar(ctx context.Context, v domain.Video) error {
	if v.Status != domain.StatusScheduled {
		return nil
	}
	v2, err := s.svc.CambiarEstado(ctx, v.Tenant, v.ID, service.Estado{A: domain.StatusLive})
	if err != nil {
		if errors.Is(err, domain.ErrConflicto) || errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	s.logger.Info("cast: directo en emisión", "tenant", v2.Tenant, "video", v2.ID, "room", v2.LiveRoom, "source", string(v2.LiveSource))
	if s.puedeGrabar() {
		eg, err := s.lk.StartParticipantSegments(ctx, v2.LiveRoom, identidadEmisor(v2.OwnerSub), prefijoGrabacion(v2), s.cfg.SegundosSegmento, s.cfg.Grabacion)
		if err != nil {
			// El directo sigue sin grabación: se dice, no se corta la emisión.
			s.logger.Warn("cast: no se pudo arrancar la grabación del directo", "video", v2.ID, "error", err)
		} else if err := s.propio.fijarEgress(ctx, v2.Tenant, v2.ID, eg.EgressID); err != nil {
			return err
		}
	}
	s.avisar(ctx, v2)
	return nil
}

// avisar manda «<nombre> está en directo: <título>» a las personas con las que
// se comparte el directo, con el enlace a la web dentro de su organización.
//
// DECISIÓN PENDIENTE: el Hub entrega por persona (subject) y Cast no tiene la
// lista de miembros de una organización, así que para un directo `org` o
// `public` no hay a quién avisar más que a la lista de personas. Avisar a toda la
// organización exige una ruta nueva en el Hub (o un people:list en Sytue ID).
func (s *Servicio) avisar(ctx context.Context, v domain.Video) {
	if s.hub == nil {
		return
	}
	personas, err := s.st.Personas(ctx, v.Tenant, v.ID)
	if err != nil {
		s.logger.Warn("cast: no se pudo leer con quién se comparte el directo", "video", v.ID, "error", err)
		return
	}
	// Quien tiene bloqueado a quien emite no recibe sus avisos.
	var bloquean map[string]bool
	if s.bloquean != nil {
		subs := make([]string, len(personas))
		for i, p := range personas {
			subs[i] = p.UserID
		}
		if bloquean, err = s.bloquean(ctx, v.OwnerSub, subs); err != nil {
			s.logger.Warn("cast: no se pudieron leer los bloqueos; se avisa a todos", "video", v.ID, "error", err)
		}
	}
	titulo := fmt.Sprintf("%s está en directo: %s", v.OwnerName, v.Title)
	for _, p := range personas {
		if bloquean[p.UserID] {
			continue
		}
		s.hub.SendAsync(ctx, hubnotify.Notification{
			Tenant: v.Tenant, Subject: p.UserID, Source: "cast", Type: "cast.live",
			Title: titulo, URL: enlaceDirecto(s.cfg.PublicURL, v.Tenant, v.ID),
			Params: map[string]string{"actor": v.OwnerName, "title": v.Title},
		})
	}
}

// enlaceDirecto es la página del directo en la web, con la organización tras el
// prefijo (ADR 0056): `https://cast.<dominio>/cast/<org>/watch/<id>`.
//
// ponytail: se compone aquí y no en hubnotify porque su guardián exige que la app
// nativa entienda cada enlace y el receptor de Cast en Fyne aún no existe; el
// relevo es hubnotify.EnlaceDirecto con su entrada en testdata/enlaces.json.
func enlaceDirecto(publicURL, org string, id uuid.UUID) string {
	u, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/cast/" + org + "/watch/" + id.String()}
	return out.String()
}

// terminar deja el directo `ended`, para la grabación, cierra la room y, si hay
// grabación, la encola como vídeo bajo demanda. Idempotente.
func (s *Servicio) terminar(ctx context.Context, v domain.Video) error {
	if v.Status == domain.StatusScheduled || v.Status == domain.StatusLive {
		actual, err := s.svc.CambiarEstado(ctx, v.Tenant, v.ID, service.Estado{A: domain.StatusEnded})
		if errors.Is(err, domain.ErrConflicto) {
			actual, err = s.st.EnOrganizacion(ctx, v.Tenant, v.ID)
		}
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		if err != nil {
			return err
		}
		// UPDATE serializa con fijarGrabacion y devuelve el HLS contemporáneo:
		// uno de los dos cierres siempre ve ambas condiciones para encolar.
		v = actual
		s.logger.Info("cast: directo terminado", "tenant", v.Tenant, "video", v.ID)
	}
	if v.LiveEgressID != "" {
		if _, err := s.lk.StopEgress(ctx, v.LiveEgressID); err != nil {
			// LiveKit contesta con error si el egress ya había acabado por su cuenta.
			s.logger.Debug("cast: parar la grabación", "video", v.ID, "error", err)
		}
	}
	if v.LiveRoom != "" {
		if err := s.lk.DeleteRoom(ctx, v.LiveRoom); err != nil {
			s.logger.Debug("cast: cerrar la room", "room", v.LiveRoom, "error", err)
		}
	}
	if err := s.propio.vaciarEspectadores(ctx, v.Tenant, v.ID); err != nil {
		s.logger.Warn("cast: no se pudo vaciar la lista de espectadores", "video", v.ID, "error", err)
	}
	if v.HlsKey != "" {
		return s.encolarGrabacion(ctx, v)
	}
	return nil
}

// encolarGrabacion pide al transcodificador el WebM y el póster de la grabación,
// una sola vez aunque lo pidan el fin del directo y el fin del egress.
func (s *Servicio) encolarGrabacion(ctx context.Context, v domain.Video) error {
	jobs, err := s.st.TrabajosDelVideo(ctx, v.Tenant, v.ID)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Kind == domain.JobLiveVOD && (j.Status == domain.JobPending || j.Status == domain.JobRunning) {
			return nil
		}
	}
	if _, err := s.svc.Encolar(ctx, v.Tenant, v.ID, domain.JobLiveVOD, nil); err != nil && !errors.Is(err, domain.ErrConflicto) {
		return err
	}
	return nil
}

// grabacionTerminada anota la playlist grabada y, si el directo ya acabó, la
// encola. Si el egress falló, se dice y el directo se queda sin grabación.
func (s *Servicio) grabacionTerminada(ctx context.Context, tenant string, eg livekit.WebhookEgress) error {
	id, err := s.propio.videoPorEgress(ctx, tenant, eg.EgressID)
	if err != nil {
		if errors.Is(err, domain.ErrNoExiste) {
			return nil
		}
		return err
	}
	if eg.Status != "EGRESS_COMPLETE" || len(eg.SegmentResults) == 0 {
		s.logger.Warn("cast: la grabación del directo no terminó bien", "video", id, "status", eg.Status, "error", eg.Error)
		return s.propio.fallarGrabacion(ctx, tenant, id, "grabación "+eg.Status+": "+eg.Error)
	}
	v, err := s.st.EnOrganizacion(ctx, tenant, id)
	if err != nil {
		return err
	}
	estado, err := s.propio.fijarGrabacion(ctx, tenant, id, claveGrabacion(v))
	if err != nil {
		return err
	}
	v.HlsKey = claveGrabacion(v)
	if estado == domain.StatusEnded {
		return s.encolarGrabacion(ctx, v)
	}
	return nil
}

// videoActivo lee el directo programado o en curso de una room.
func (s *Servicio) videoActivo(ctx context.Context, tenant, room string) (domain.Video, error) {
	id, err := s.propio.videoActivoPorRoom(ctx, tenant, room)
	if err != nil {
		return domain.Video{}, err
	}
	return s.st.EnOrganizacion(ctx, tenant, id)
}

// directoDeIngress es el directo en el que publica un ingress. Si OBS emite sin
// haber programado nada, se crea uno «Directo de <nombre>» para la organización
// sobre la room a la que el ingress apuntaba.
func (s *Servicio) directoDeIngress(ctx context.Context, c Clave, room string) (domain.Video, error) {
	v, err := s.videoActivo(ctx, c.Tenant, room)
	if err == nil || !errors.Is(err, domain.ErrNoExiste) {
		return v, err
	}
	if _, ok := roomDe(room); !ok {
		return domain.Video{}, fmt.Errorf("%w: la room %q del ingress no es de Cast", domain.ErrEntradaInvalida, room)
	}
	nombre := c.OwnerName
	if nombre == "" {
		nombre = c.OwnerSub
	}
	quien := service.Quien{Sub: c.OwnerSub, Tenant: c.Tenant, Nombre: c.OwnerName}
	v, err = s.svc.Crear(ctx, quien, service.Alta{
		Kind: domain.KindLive, Title: "Directo de " + nombre, Visibility: domain.VisibilityOrg,
		LiveSource: domain.LiveSourceRTMP, LiveRoom: room,
	})
	if err != nil {
		return domain.Video{}, err
	}
	s.logger.Info("cast: OBS emite sin directo programado; se crea uno", "tenant", v.Tenant, "video", v.ID, "room", room)
	return v, nil
}
