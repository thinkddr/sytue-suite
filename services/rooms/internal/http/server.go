// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package chathttp implementa la superficie HTTP de Sytue Rooms: API REST
// (teams, canales, miembros, mensajes, hilos, reacciones, lectura, búsqueda) y
// el endpoint WebSocket de tiempo real (/ws). El fan-out a las salas WS lo
// alimenta el relay del outbox (LISTEN/NOTIFY), no este paquete.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS). Aislamiento por tenant vía
// RLS (el tenant se inyecta en el contexto desde los claims).
package chathttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/ws"
)

// Deps son las dependencias del router HTTP de Chat.
type Deps struct {
	Cfg      config.Config
	Service  *service.Service
	Verifier *authn.Verifier
	Hub      *ws.Hub
	// HookLimiter rate-limita el endpoint público POST /hooks/{token} por token+IP
	// (Ola 15). Nil lo deshabilita (dev/tests sin Postgres para el limitador).
	HookLimiter   httpx.RateLimiter
	ReadyCheckers []httpx.Checker
	// MountExtra y MountPublicExtra cuelgan la VIDEOLLAMADA en este mismo router:
	// Sytue Rooms es un solo servicio con mensajería y vídeo, y ambos comparten
	// proceso, puerto y base de datos. Se inyectan en vez de importar el paquete de
	// vídeo desde aquí para no crear una dependencia circular entre las dos mitades.
	// Nil en los tests del router de mensajería, que no necesitan el vídeo.
	MountExtra       func(api chi.Router)
	MountPublicExtra func(r chi.Router)
	// Operador pregunta a Sytue ID por la cola de denuncias del operador. nil =
	// esa cola contesta 403.
	Operador operador.Autorizar
}

// NewRouter construye el router completo de Sytue Rooms: mensajería y vídeo.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{
		RequestTimeout: 30 * time.Second,
		RequestTimeoutFor: func(r *http.Request) time.Duration {
			if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/whatsapp/consent" || r.URL.Path == "/internal/rooms/document-access" || r.URL.Path == "/internal/rooms/subject-access") {
				return time.Minute
			}
			return 0
		},
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{svc: d.Service, cfg: d.Cfg}
	gifs := newGifHandlers()
	ih := &intHandlers{
		svc: d.Service, publicURL: strings.TrimRight(d.Cfg.PublicURL, "/"), internalSecret: d.Cfg.InternalSecret,
		channelMailDomain: d.Cfg.ChannelMailDomain,
	}
	wsh := &wsHandler{svc: d.Service, hub: d.Hub, verifier: d.Verifier, cfg: d.Cfg}

	// API REST autenticada bajo /api/v1.
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Use(withBearer)
		api.Get("/matrix/bootstrap", h.MatrixBootstrap)
		api.Get("/matrix/storage/bootstrap", h.MatrixStorageBootstrap)
		api.Post("/matrix/native/start", h.MatrixNativeStart)
		api.Get("/matrix/native/preview", h.MatrixNativePreview)
		api.Post("/matrix/native/accept", h.MatrixNativeAccept)
		api.Post("/matrix/native/finish", h.MatrixNativeFinish)
		// Catálogo CC0 público consultado desde el servidor; medios y sesión viajan
		// siempre por Rooms. El limitador compartido evita descargar sin límite.
		api.Group(func(lib chi.Router) {
			lib.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(8, 64), KeyFunc: authn.RateKey}))
			lib.Post("/gifs/search", gifs.search)
			lib.Get("/gifs/{gifID}", gifs.image)
		})

		// ADMINISTRACIÓN de la organización. Va primero y en su propio grupo para
		// que el gate de rol quede a la vista: todo lo de abajo se autoriza por
		// PERTENENCIA (se ve lo público y aquello de lo que uno es miembro) y esto
		// por ROL, que es lo que permite a un admin ver un canal privado en el que
		// no está. Sin esto, Rooms era el único producto de la suite sin
		// administración: un canal archivado por error no lo podía deshacer nadie
		// de fuera.
		if d.Service != nil {
			adminH := NewAdminHandlers(d.Service.Store())
			api.Group(func(adm chi.Router) {
				adm.Use(d.Verifier.RequireRole("owner", "admin"))
				adm.Get("/admin/channels", adminH.ListChannels)
				adm.Patch("/admin/channels/{channelID}", adminH.SetChannelArchived)
				adm.Get("/admin/stats", adminH.Stats)
			})
		}

		// Teams.
		// «Exportar tus datos»: canales y mensajes propios (platform/pkg/portabilidad).
		api.Get("/export", h.Export)
		api.Get("/teams", h.ListTeams)
		api.Post("/teams", h.CreateTeam)
		api.Get("/teams/{teamID}", h.GetTeam)
		api.Get("/teams/{teamID}/members", h.ListTeamMembers)
		api.Post("/teams/{teamID}/members", h.AddTeamMember)
		api.Delete("/teams/{teamID}/members/{sub}", h.RemoveTeamMember)
		api.Get("/teams/{teamID}/unread", h.Unread)
		api.Get("/teams/{teamID}/search", h.SearchMessages)
		// La búsqueda lleva lo que escribe la persona: por GET acaba en el log de
		// accesos de cada salto, en el `Referer` y en el historial. Por QUERY va en
		// el cuerpo. GET se queda como respaldo para los clientes ya desplegados.
		httpx.GetYQuery(api, "/search", h.SearchAll)

		// Personas de OTRA organización (ADR 0040): encontrarlas por @usuario o
		// correo, y las conversaciones de fuera de quien pregunta.
		api.Post("/people:resolve", h.ResolvePerson)
		api.Get("/me/external-channels", h.ListExternalChannels)
		// Las invitaciones de otras organizaciones (mig 0041): la gente de fuera acepta
		// antes de entrar en un canal, un DM o un grupo.
		api.Get("/me/invitations", h.ListInvitations)
		api.Post("/me/invitations/{channelID}/accept", h.AcceptInvitation)
		api.Delete("/me/invitations/{channelID}", h.RejectInvitation)

		// Las rutas de un canal o de un mensaje concretos pasan por la organización
		// ANFITRIONA cuando son de una conversación de otra organización donde quien
		// llama está como persona de fuera (EnAnfitrion). Las de integraciones y las
		// de categorías, no: son de la organización de cada uno.
		porCanal := api.With(enAnfitrionPor(h.svc, "channelID", func(id uuid.UUID) service.RefAnfitrion {
			return service.RefAnfitrion{Canal: id}
		}))
		porMensaje := api.With(enAnfitrionPor(h.svc, "messageID", func(id uuid.UUID) service.RefAnfitrion {
			return service.RefAnfitrion{Mensaje: id}
		}))

		// Canales.
		api.Get("/teams/{teamID}/channels", h.ListChannels)
		api.Get("/teams/{teamID}/channels/browse", h.ListJoinableChannels)
		api.Post("/teams/{teamID}/channels", h.CreateChannel)
		api.Post("/teams/{teamID}/dms", h.OpenDirect)
		porCanal.Get("/channels/{channelID}", h.GetChannel)
		porCanal.Patch("/channels/{channelID}", h.UpdateChannel)
		porCanal.Post("/channels/{channelID}/join", h.JoinChannel)
		porCanal.Post("/channels/{channelID}/leave", h.LeaveChannel)
		// Borrar un canal (admin; nunca un directo) y «borrar el chat» de un directo
		// solo para quien lo pide. DELETE server-to-server; el BFF lo hace POST.
		porCanal.Delete("/channels/{channelID}", h.DeleteChannel)
		porCanal.Post("/channels/{channelID}/hide", h.HideDirect)
		// Miembros del canal: listar, «Añadir personas», expulsar y rol de admin de
		// canal (la única vía para escribir channel_members.role). DELETE/PATCH
		// server-to-server (sin CSRF de navegador); el BFF los hace POST.
		porCanal.Get("/channels/{channelID}/members", h.ListChannelMembers)
		porCanal.Post("/channels/{channelID}/members", h.AddChannelMember)
		porCanal.Delete("/channels/{channelID}/members/{sub}", h.RemoveChannelMember)
		porCanal.Patch("/channels/{channelID}/members/{sub}", h.SetChannelMemberRole)
		porCanal.Post("/channels/{channelID}/read", h.MarkRead)
		porCanal.Post("/channels/{channelID}/prefs", h.SetChannelPrefs)
		api.Post("/channels/{channelID}/category", h.SetChannelCategory)
		porCanal.Get("/channels/{channelID}/pins", h.ListPins)

		// Categorías del sidebar por usuario (Ola 11). Borrado vía DELETE
		// (server-to-server, sin CSRF de navegador); el BFF lo hace POST {delete}.
		api.Get("/me/categories", h.ListCategories)
		api.Post("/me/categories", h.CreateCategory)
		api.Patch("/me/categories/{categoryID}", h.UpdateCategory)
		api.Delete("/me/categories/{categoryID}", h.DeleteCategory)

		// Preferencias de notificación por usuario (Ola 13): palabras clave + «No
		// molestar». PUT server-to-server (sin CSRF de navegador); el BFF lo hace POST.
		api.Get("/me/notify-prefs", h.GetNotifyPrefs)
		api.Put("/me/notify-prefs", h.PutNotifyPrefs)

		// Web Push (Ola 18): clave pública VAPID + alta/baja de suscripción del navegador.
		// Mutaciones por POST (sin CSRF de navegador vía el BFF).
		api.Get("/me/push/key", h.PushKey)
		api.Post("/me/push/subscribe", h.SubscribePush)
		api.Post("/me/push/unsubscribe", h.UnsubscribePush)
		// iOS nativo: registrar/olvidar el aparato al que mandar el aviso vacío.
		api.Post("/me/push/apns", h.RegisterAPNSDevice)
		api.Post("/me/push/apns/forget", h.ForgetAPNSDevice)

		// Mensajes / hilos.
		porCanal.Get("/channels/{channelID}/messages", h.ListMessages)
		// Permalink / scroll-to-message (Ola 16): ventana centrada en un mensaje.
		porCanal.Get("/channels/{channelID}/messages/around", h.MessagesAround)
		porCanal.Post("/channels/{channelID}/messages", h.PostMessage)
		// «Visto» en DMs (Ola 16): last_read_at de los otros miembros del canal.
		porCanal.Get("/channels/{channelID}/receipts", h.Receipts)
		// Unfurl de enlaces (Ola 16): vista previa OpenGraph cacheada + SSRF-safe.
		api.Get("/unfurl", h.Unfurl)
		// El proxy de la imagen va bajo la MISMA verja autenticada: la miniatura
		// solo la ve quien puede ver el canal donde se pegó el enlace.
		api.Get("/unfurl/imagen/{hash}", h.UnfurlImagen)
		porMensaje.Get("/messages/{messageID}/thread", h.GetThread)
		porMensaje.Post("/messages/{messageID}/replies", h.PostReply)
		porMensaje.Patch("/messages/{messageID}", h.EditMessage)
		porMensaje.Delete("/messages/{messageID}", h.DeleteMessage)
		// Acuse de recibo (Ola 12).
		porMensaje.Post("/messages/{messageID}/ack", h.AckMessage)
		// Quién ha visto este mensaje. Sale de `read_state` («leído hasta aquí»),
		// no de una fila por mensaje y persona: en un canal de doscientos, eso
		// serían doscientas escrituras por mensaje.
		porMensaje.Get("/messages/{messageID}/readers", h.MessageReaders)

		// Borradores en servidor (paridad §4): lo escrito y no enviado, por ámbito de
		// redacción. PUT server-to-server (sin CSRF de navegador); el BFF lo hace POST.
		// Vaciar el cuerpo BORRA el borrador: no hace falta un DELETE aparte.
		api.Get("/me/drafts", h.ListDrafts)
		api.Put("/me/drafts", h.PutDraft)

		// Mensajes programados (Ola 12). Crear = POST .../messages con send_at.
		// Borrado vía DELETE (server-to-server, sin CSRF de navegador); el BFF lo
		// hace POST {cancel}.
		api.Get("/me/scheduled", h.ListScheduled)
		api.Patch("/scheduled/{scheduledID}", h.UpdateScheduled)
		api.Delete("/scheduled/{scheduledID}", h.CancelScheduled)

		// Reacciones.
		porMensaje.Put("/messages/{messageID}/reactions/{emoji}", h.AddReaction)
		porMensaje.Delete("/messages/{messageID}/reactions/{emoji}", h.RemoveReaction)

		// Objetos ricos: votar una opción de la encuesta o marcar la tarea como
		// hecha. La tarjeta se crea con el mensaje (POST .../messages con `card`);
		// aquí solo se mueve su ESTADO, que es lo único mutable.
		porMensaje.Put("/messages/{messageID}/card/{value}", h.SetCardVote)
		porMensaje.Delete("/messages/{messageID}/card/{value}", h.ClearCardVote)

		// Emoji personalizados por tenant (Ola 14). La imagen se sube a Drive (proxy
		// de adjuntos) y aquí se liga el shortcode al file_id. Borrado vía DELETE
		// (server-to-server, sin CSRF de navegador); el BFF lo hace POST {delete}.
		api.Get("/emoji", h.ListCustomEmoji)
		api.Post("/emoji", h.CreateCustomEmoji)
		api.Delete("/emoji/{name}", h.DeleteCustomEmoji)

		// Integraciones (Ola 15): gestión de webhooks entrantes/salientes y bots
		// (admin del canal/workspace). El token/URL en claro solo se devuelve al
		// crear/rotar. Mutaciones por POST/DELETE (sin CSRF de navegador vía el BFF).
		api.Get("/channels/{channelID}/webhooks/incoming", ih.ListIncoming)
		api.Post("/channels/{channelID}/webhooks/incoming", ih.CreateIncoming)
		api.Delete("/webhooks/incoming/{id}", ih.DeleteIncoming)
		api.Post("/webhooks/incoming/{id}/rotate", ih.RotateIncoming)
		// Puentes: el canal se repite en Slack, Teams o Google Chat.
		api.Get("/channels/{channelID}/bridges", ih.ListBridges)
		api.Post("/channels/{channelID}/bridges", ih.CreateBridge)
		api.Delete("/bridges/{id}", ih.DeleteBridge)
		api.Get("/integrations/destinations", ih.IntegrationDestinations)
		api.Post("/integrations/meetings", ih.CreateIntegrationMeeting)
		// WhatsApp Business: números conectados a un equipo (admin del workspace).
		api.Get("/whatsapp", ih.ListWhatsApp)
		api.Post("/whatsapp/consent", ih.ConnectWhatsAppConsent)
		api.Delete("/whatsapp/{id}", ih.DisconnectWhatsApp)

		// Slash commands de terceros. Listar lo puede cualquiera —el autocompletado
		// del compositor los necesita— pero registrar y borrar es de admin, y los
		// secretos solo viajan para él.
		api.Get("/commands", ih.ListCommands)
		api.Post("/commands", ih.CreateCommand)
		api.Delete("/commands/{id}", ih.DeleteCommand)
		api.Post("/commands/execute", ih.ExecuteCommand)

		api.Get("/webhooks/outgoing", ih.ListOutgoing)
		api.Post("/webhooks/outgoing", ih.CreateOutgoing)
		api.Delete("/webhooks/outgoing/{id}", ih.DeleteOutgoing)

		api.Get("/bots", ih.ListBots)
		api.Post("/bots", ih.CreateBot)
		api.Delete("/bots/{id}", ih.DeleteBot)
		api.Post("/bots/{botID}/channels/{channelID}", ih.AddBotChannel)
		api.Delete("/bots/{botID}/channels/{channelID}", ih.RemoveBotChannel)

		// Acciones sobre mensajes: fijar, guardar, marcar como no leído.
		porMensaje.Post("/messages/{messageID}/pin", h.PinMessage)
		porMensaje.Delete("/messages/{messageID}/pin", h.UnpinMessage)
		porMensaje.Post("/messages/{messageID}/save", h.SaveMessage)
		porMensaje.Delete("/messages/{messageID}/save", h.UnsaveMessage)
		porMensaje.Post("/messages/{messageID}/unread", h.MarkUnread)
		api.Get("/me/saved", h.ListSaved)

		// Denunciar y bloquear (ADR 0073). Denunciar va con su propio límite por
		// persona además del tope diario de la base. Las dos rutas de la COLA van
		// por `api` y no por `porCanal`: el rol de quien modera es el de la
		// organización de su token (ver `moderador`).
		if d.Service != nil {
			mh := NewModeracionHandlers(d.Service)
			denunciar := httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.1, 5), KeyFunc: authn.RateKey})
			porMensaje.With(denunciar).Post("/messages/{messageID}/report", mh.ReportMessage)
			porCanal.With(denunciar).Post("/channels/{channelID}/members/{sub}/report", mh.ReportMember)
			porCanal.Put("/channels/{channelID}/members/{sub}/block", mh.Block)
			api.Get("/me/blocks", mh.ListBlocks)
			api.Delete("/me/blocks/{sub}", mh.Unblock)
			api.Get("/moderation/reports", mh.ListReports)
			api.Post("/moderation/reports/{reportID}/resolve", mh.ResolveReport)
			// La cola del OPERADOR de la plataforma: todas las organizaciones. Quién
			// pasa lo decide Sytue ID en cada petición (operadorDe).
			mh.ConOperador(d.Service, d.Operador)
			api.Get("/platform/moderation/reports", mh.ListReportsOperador)
			api.Post("/platform/moderation/reports/{reportID}/resolve", mh.ResolveReportOperador)
		}

		// Seguimiento de hilos + vista global «Hilos» (Ola 9). Borrado vía DELETE
		// (server-to-server, sin CSRF de navegador); el BFF lo hace POST {delete}.
		porMensaje.Post("/messages/{messageID}/follow", h.FollowThread)
		porMensaje.Delete("/messages/{messageID}/follow", h.UnfollowThread)
		api.Get("/me/threads", h.ListMyThreads)

		// Videollamada de Rooms (/rooms, /rooms/{id}/join…). Va DENTRO de este grupo
		// para heredar la misma verificación de sesión que el resto de la API.
		if d.MountExtra != nil {
			d.MountExtra(api)
		}
	})

	if d.Cfg.DocumentCallAccessSecret != "" {
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.DocumentCallAccessSecret)).Post("/internal/rooms/subject-access", documentIdentityHandler(d.Service))
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.DocumentCallAccessSecret)).Get("/internal/rooms/subject-access/{subject}/{tenant}", documentIdentityVersionHandler(d.Service))
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.DocumentCallAccessSecret)).Post("/internal/rooms/document-access", documentCallAccessHandler(d.Service))
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.DocumentCallAccessSecret)).Get("/internal/rooms/document-access/{document}", documentCallVersionHandler(d.Service))
	}
	if d.Cfg.MatrixLifecycleSecret != "" {
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.MatrixLifecycleSecret)).Post("/internal/matrix/account-access", h.MatrixAccountAccess)
		r.With(httpx.SinPasarPorElIngress, httpx.SecretoInterno(d.Cfg.MatrixLifecycleSecret)).Post("/internal/matrix/tenant-access", h.MatrixTenantAccess)
	}
	// Endpoint PÚBLICO de webhooks entrantes (Ola 15): SIN JWT — el token de la ruta
	// ES la autorización. Rate-limitado por token+IP (cross-réplica, sobre Postgres)
	// para acotar el sondeo de tokens y el abuso; fuera del grupo autenticado /api/v1.
	// La URL universal autodetecta el dialecto (nativo/Slack/Teams/Google Chat); los
	// alias /hooks/slack|teams|googlechat/{token} lo FUERZAN, para emisores que mandan solo {"text"}
	// y esperan el ack literal de su producto.
	post := r.Post
	if d.HookLimiter != nil {
		post = r.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: d.HookLimiter, KeyFunc: hookRateKey})).Post
	}
	post("/hooks/{token}", ih.PostIncoming)
	post("/hooks/slack/{token}", ih.PostIncomingSlack)
	post("/hooks/teams/{token}", ih.PostIncomingTeams)
	post("/hooks/googlechat/{token}", ih.PostIncomingGoogleChat)
	// Avisos de WhatsApp Business (Meta). Bajo /webhooks porque el ingress ya lo lleva
	// a este servicio (lo usa el SFU); mismo limitador que los entrantes.
	get := r.Get
	if d.HookLimiter != nil {
		get = r.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: d.HookLimiter, KeyFunc: hookRateKey})).Get
	}
	get("/webhooks/whatsapp/{token}", ih.VerifyWhatsApp)
	post("/webhooks/whatsapp/{token}", ih.ReceiveWhatsApp)

	// Endpoint INTERNO service-to-service. NO público: autentica el secreto compartido
	// (Authorization: Bearer), fuera de /api/v1, y solo se monta si ROOMS_INTERNAL_SECRET
	// está configurado.
	//
	// La tarjeta de «llamada finalizada» YA NO entra por aquí: el vídeo es parte de este
	// mismo servicio y la publica en proceso (media/service.InProcessNotifier).
	if d.Cfg.InternalSecret != "" {
		// Verja de los ADJUNTOS de canal: Sytue Drive pregunta aquí si el usuario puede
		// leer el canal antes de servir el fichero (el roster vive en Rooms, los bytes en
		// Drive). Sin secreto configurado no se monta y Drive queda fail-closed: los
		// adjuntos con ámbito de canal no se sirven a nadie.
		r.With(ih.internalAuth).Get("/internal/channel-access", ih.ChannelAccess)
	}

	// Publicación de BOTS (Ola 15): token Bearer bot_… (NO JWT). Alcance MÍNIMO —
	// solo publicar mensajes; reusa el handler de PostMessage con la identidad
	// sintética bot:<id> que fija botAuth. Bajo /api/bot para no colisionar con el
	// mount de /api/v1 (grupo JWT) ni heredar su RequireAuth.
	r.Route("/api/bot", func(bot chi.Router) {
		bot.Use(ih.botAuth)
		bot.Post("/channels/{channelID}/messages", h.PostMessage)
	})

	// WebSocket de tiempo real. La autenticación se valida en el handler porque
	// el navegador no puede fijar cabeceras en el handshake: admite el token por
	// query (?access_token=) además de la cabecera Authorization.
	r.Get("/ws", wsh.serve)

	// La otra mitad de Sytue Rooms: la videollamada, en el mismo router.
	if d.MountPublicExtra != nil {
		d.MountPublicExtra(r)
	}

	return r
}
