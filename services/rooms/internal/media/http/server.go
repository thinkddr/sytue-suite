// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package meethttp implementa la superficie HTTP de Sytue Rooms: API REST de
// salas/señalización (crear, listar, unirse con minteo de JWT de LiveKit,
// moderar, cerrar) y el endpoint de webhooks de LiveKit.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS) en la API; el webhook se
// autentica por la firma HMAC del propio LiveKit (api_secret), no por OIDC.
// Aislamiento por tenant vía RLS (el tenant se inyecta desde los claims).
package meethttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

// Deps son las dependencias del router HTTP de Meet.
type Deps struct {
	Cfg           config.Config
	Service       *service.Service
	Signer        *livekit.Signer
	Verifier      *authn.Verifier
	ReadyCheckers []httpx.Checker
	// Anfitrion lleva la petición a la organización ANFITRIONA cuando la sala —o el
	// canal del que nace— es de una conversación de otra organización donde quien
	// llama está como persona de fuera (ADR 0040), sin los roles de su token. Lo
	// pone el servicio unificado; nil = sin conversaciones de fuera (los tests del
	// router de vídeo).
	Anfitrion func(r *http.Request, sala, canal uuid.UUID) (*http.Request, error)
}

// NewRouter construye el router completo de Sytue Rooms.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 30 * time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)
	r.Route("/api/v1", func(api chi.Router) { MountAPI(api, d) })
	MountPublic(r, d)
	return r
}

// MountAPI cuelga la API de videollamada de un router que YA está bajo /api/v1. Existe
// para que el servicio unificado de Sytue Rooms monte mensajería y vídeo en el mismo
// router —un solo binario, un solo puerto— sin duplicar la lista de rutas.
func MountAPI(api chi.Router, d Deps) {
	h := &Handlers{svc: d.Service, cfg: d.Cfg}
	api.Group(func(grupo chi.Router) {
		grupo.Use(d.Verifier.RequireAuth)
		// El token de quien llama viaja en el contexto: las salas de un documento se lo
		// presentan a Sytue Docs, que es quien decide si puede verlo.
		grupo.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := service.WithBearer(r.Context(), authn.Token(r))
				// La clave del enlace de una reunión suelta, si quien llama la trae: es
				// lo que abre la puerta a quien no la convocó ni estuvo dentro.
				ctx = service.ConClave(ctx, r.Header.Get(service.CabeceraClave))
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
		// Todo lo que es de UNA sala pasa por su organización anfitriona si hace falta
		// (Deps.Anfitrion). Crear una sala suelta o listarlas, no: son de la suya.
		api := grupo.With(porSala(d.Anfitrion), freshDocumentIdentity(d.Service))
		origen := grupo.With(porCanalDeOrigen(d.Anfitrion))

		// Salas.
		grupo.Post("/rooms", h.CreateRoom)
		// Crear sala desde un evento de Calendar o un canal de Rooms (intent del Hub).
		origen.Post("/rooms/from-source", h.CreateRoomFromSource)
		// Consultar la sala viva de un origen (kind+ref_id) SIN crearla — indicador
		// de «llamada en curso». Ruta estática: chi la prioriza sobre /rooms/{id}.
		origen.Get("/rooms/by-source", h.RoomBySource)
		grupo.Get("/rooms", h.ListRooms)
		api.Get("/rooms/{id}", h.GetRoom)
		api.Delete("/rooms/{id}", h.CloseRoom) // cerrar (host/admin)

		// Unirse: mintea el JWT de LiveKit tras validar OIDC y autorizar.
		api.Post("/rooms/{id}/join", h.JoinRoom)
		api.Get("/rooms/{id}/encryption", h.RoomEncryption)
		api.Post("/rooms/{id}/encryption/ready", h.ReadyRoomEncryption)
		api.Post("/rooms/{id}/encryption/reconnect", h.ReconnectRoomEncryption)

		// Presencia e historial.
		api.Get("/rooms/{id}/participants", h.ListParticipants)
		api.Get("/rooms/{id}/recordings", h.ListRecordings)
		// Descargar una grabación: URL firmada contra el bucket, no el fichero por
		// el proceso. La verja es más ancha que la de moderar —quien participó
		// puede ver el acta— y más estrecha que el tenant entero.
		api.Get("/rooms/{id}/recordings/{recID}", h.GetRecording)
		// Y borrarla: la puerta es la de MODERAR, no la de ver. Quien participó
		// puede descargar el acta; destruirla, solo el anfitrión o un admin.
		api.Delete("/rooms/{id}/recordings/{recID}", h.DeleteRecording)

		// Moderación: expulsar y silenciar. SIN RequireRole a la puerta —lo
		// autoriza `puedeModerar` dentro, por anfitrión de la sala o admin de la
		// organización—. Con el rol en la ruta, el anfitrión de su propia reunión
		// no podía echar a un intruso ni cortar un micro abierto, que es
		// exactamente cuando hace falta.
		api.Delete("/rooms/{id}/participants/{identity}", h.RemoveParticipant)
		api.Post("/rooms/{id}/participants/{identity}/mute", h.MuteParticipant)
		// Silenciar a todos MENOS a quien lo pide. Ruta estática antes que la de
		// {identity}: chi las prioriza así, pero dejarlas juntas evita que alguien
		// las separe y acabe con «mute-all» tratado como una identidad.
		api.Post("/rooms/{id}/participants/mute-all", h.MuteAll)
		// Retirar la palabra: el escalón entre silenciar —que el otro deshace
		// pulsando su botón— y expulsar. Sigue viendo, oyendo y con la mano.
		api.Post("/rooms/{id}/participants/{identity}/speech", h.GrantSpeech)
		api.Delete("/rooms/{id}/participants/{identity}/speech", h.RevokeSpeech)

		// Bloquear la sala: nadie NUEVO entra. Expulsar a un intruso no sirve de
		// nada si vuelve a entrar por el mismo enlace tres segundos después.
		api.Post("/rooms/{id}/lock", h.LockRoom)
		api.Delete("/rooms/{id}/lock", h.UnlockRoom)

		// Sala de espera. `knock` NO lleva verja de moderación —llamar es lo que
		// hace quien no puede entrar—; la cola y las respuestas sí.
		api.Post("/rooms/{id}/knock", h.Knock)
		api.Get("/rooms/{id}/knocks", h.ListKnocks)
		api.Post("/rooms/{id}/knocks/{identity}", h.Admit)
		api.Delete("/rooms/{id}/knocks/{identity}", h.Deny)

		// Grabación: misma puerta que la moderación (anfitrión o admin), aplicada
		// DENTRO del handler. 202 al arrancar porque el fichero no existe aún: el
		// egress lo sube al terminar y el resultado llega por webhook.
		api.Post("/rooms/{id}/recording", h.StartRecording)
		api.Delete("/rooms/{id}/recording", h.StopRecording)
	})
}

// MountPublic cuelga lo que NO lleva sesión de la suite: el webhook de LiveKit, que se
// autentica por la firma HMAC de su cuerpo.
func MountPublic(r chi.Router, d Deps) {
	wh := &WebhookHandler{svc: d.Service, signer: d.Signer}
	r.Post("/webhooks/livekit", wh.Receive)
}
