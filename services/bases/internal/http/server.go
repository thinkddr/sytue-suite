// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package baseshttp es la superficie HTTP de Sytue Bases. `/api/v1` (con sesión) la
// llaman el BFF de la app y el Hub por dentro del clúster y, desde el 2026-10-05, las apps
// nativas desde fuera, por el Ingress; `/_bases/v1` (formularios públicos y conector, sin
// sesión) NO sale del clúster. Aquí solo se
// traduce HTTP a las operaciones del store; el permiso se decide SIEMPRE dentro del
// store, en la transacción de la operación (domain.Permite), nunca aquí.
package baseshttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/bases/internal/config"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

const (
	// maxCuerpoAPI deja pasar un registro (60 KB) con su sobre y una vista.
	maxCuerpoAPI = 1 << 20
	// maxCuerpoImport es el del CSV de una importación (domain.MaxBytesCSV) con holgura.
	maxCuerpoImport = domain.MaxBytesCSV + 64<<10
)

// Deps son las dependencias del router.
type Deps struct {
	Cfg           config.Config
	Store         *store.Store
	Verifier      *authn.Verifier
	ReadyCheckers []httpx.Checker
	// Altcha comprueba la prueba de trabajo de los formularios públicos.
	Altcha *altcha.Verifier
	// Hub avisa de comentarios y menciones. nil = sin avisos (los tests que no los miran).
	Hub *hubnotify.Client
	// Objetos es el almacén de los adjuntos; nil = sin almacén (los adjuntos dan 503).
	Objetos Almacen
}

// Handlers es el receptor de los manejadores.
type Handlers struct {
	Deps
	// limitePublico frena por IP lo que llega SIN sesión (formularios públicos).
	limitePublico httpx.RateLimiter
	// limiteConector frena por token las lecturas del conector.
	limiteConector httpx.RateLimiter
	// mencionada frena el aviso de mención que alguien puede provocar a voluntad sobre
	// otra persona: una por autor, persona y registro cada diez minutos. nil = sin freno.
	mencionada *hubnotify.UnaVez
	// tic es cada cuánto un flujo de cambios vuelve a mirar (y a autorizarse); flujos
	// cuenta los abiertos, por persona y en total (tiemporeal.go).
	tic    time.Duration
	flujos contadorDeFlujos
	// subidas es el semáforo de las subidas de adjuntos (ver adjuntos.go).
	subidas chan struct{}
}

// NewRouter construye el router completo.
func NewRouter(d Deps) http.Handler {
	// El tope del router es el del cuerpo más grande (un adjunto); cada ruta lo baja al suyo.
	r := httpx.NewRouter(httpx.RouterOptions{MaxBodyBytes: domain.MaxBytesAdjunto + 1})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)
	// ponytail: cubo en memoria por réplica; el cupo del enlace (en la base) es el techo real.
	h := &Handlers{
		Deps: d, limitePublico: httpx.NewTokenBucket(2, 10), limiteConector: httpx.NewTokenBucket(1, 20),
		mencionada: hubnotify.NuevaUnaVez(10 * time.Minute), tic: time.Second, subidas: make(chan struct{}, subidasALaVez),
	}
	h.montarFormularios(r)
	h.montarConector(r)
	// ponytail: cubo en memoria por réplica; pg.RateLimiter si hiciera falta compartirlo.
	limiter := httpx.NewTokenBucket(50, 100)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Use(d.Verifier.RequireOrgKind("org")) // una cuenta personal no entra
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: limiter, KeyFunc: authn.RateKey}))
		h.montar(api)
	})
	return r
}

func (h *Handlers) montar(r chi.Router) {
	// enAnfitrion va en línea (With), después de enrutar: necesita los parámetros de la ruta.
	j := r.With(httpx.MaxBodyBytes(maxCuerpoAPI), h.enAnfitrion)
	j.Get("/bases", h.listarBases)
	// El conector de búsqueda del Hub: GET /api/v1/search?q=&limit=.
	j.Get("/search", h.buscar)
	j.Post("/bases", h.crearBase)
	j.Get("/bases/{id}", h.obtenerBase)
	j.Patch("/bases/{id}", h.actualizarBase)
	j.Delete("/bases/{id}", h.borrarBase)
	j.Get("/bases/{id}/audit", h.auditoria)
	j.Get("/bases/{id}/permissions", h.listarPermisos)
	j.Post("/bases/{id}/permissions", h.concederPermiso)
	j.Delete("/bases/{id}/permissions/{pid}", h.revocarPermiso)
	j.Post("/bases/{id}/transfer", h.transferirBase)
	j.Get("/groups", h.misGrupos)
	j.Get("/bases/{id}/tables", h.listarTablas)
	j.Post("/bases/{id}/tables", h.crearTabla)
	r.With(httpx.MaxBodyBytes(maxCuerpoImport)).Post("/bases/{id}/import", h.importar)
	h.montarAdjuntos(r)
	h.montarScripts(r)

	j.Patch("/tables/{tid}", h.renombrarTabla)
	j.Delete("/tables/{tid}", h.borrarTabla)
	j.Post("/tables/{tid}/fields", h.crearCampo)
	j.Get("/tables/{tid}/records", h.listarRegistros)
	j.Post("/tables/{tid}/records", h.crearRegistro)
	j.Get("/tables/{tid}/views", h.listarVistas)
	j.Post("/tables/{tid}/views", h.crearVista)
	j.Get("/tables/{tid}/export", h.exportar)

	j.Patch("/fields/{fid}", h.renombrarCampo)
	j.Delete("/fields/{fid}", h.borrarCampo)
	j.Get("/records/{rid}", h.obtenerRegistro)
	j.Put("/records/{rid}/cells/{fid}", h.escribirCelda)
	j.Get("/bases/{id}/events", h.flujoDeCambios)
	j.Put("/bases/{id}/presence", h.latir)
	j.Patch("/records/{rid}", h.actualizarRegistro)
	j.Delete("/records/{rid}", h.borrarRegistro)
	j.Get("/records/{rid}/comments", h.listarComentarios)
	j.Post("/records/{rid}/comments", h.crearComentario)
	j.Delete("/comments/{coid}", h.borrarComentario)
	j.Delete("/views/{vid}", h.borrarVista)
	j.Get("/views/{vid}/form-links", h.listarEnlaces)
	j.Post("/views/{vid}/form-links", h.crearEnlace)
	j.Delete("/form-links/{eid}", h.revocarEnlace)
	j.Get("/bases/{id}/connections", h.listarConexiones)
	j.Post("/tables/{tid}/connections", h.crearConexion)
	j.Delete("/connections/{cid}", h.revocarConexion)
	j.Post("/connections/{cid}/rotate", h.rotarConexion)
}

// actor es quien llama: su sub, su organización y sus grupos, de los claims ya verificados.
//
// Si enAnfitrion resolvió que lo que pide está en OTRA organización donde es invitada,
// el actor es solo su `sub` en la anfitriona: ni sus grupos ni su organización cuentan allí.
func actor(r *http.Request) (domain.Actor, error) {
	c, ok := authn.FromContext(r.Context())
	if !ok || c.Subject == "" || c.Tenant == "" {
		return domain.Actor{}, httpx.ErrUnauthorized
	}
	if host, _ := r.Context().Value(claveAnfitriona{}).(string); host != "" {
		return domain.Actor{Sub: c.Subject, Tenant: host, Externo: true}, nil
	}
	return domain.Actor{Sub: c.Subject, Tenant: c.Tenant, Groups: c.Groups}, nil
}

type claveAnfitriona struct{}

// parametrosDeRecurso son los parámetros de ruta por los que una persona de fuera puede
// llegar a una base compartida, y la clase de recurso de cada uno. Los que no están
// (campos, enlaces de formulario, conexiones, permisos) son solo del propietario, que
// nunca es de fuera.
var parametrosDeRecurso = [...][2]string{{"id", "base"}, {"tid", "table"}, {"rid", "record"}, {"vid", "view"}, {"coid", "comment"}}

// enAnfitrion decide en qué organización se atiende la petición (ADR 0068 sobre el
// 0040): la del token, salvo que el recurso de la ruta no exista en ella y sí en una
// donde quien llama es invitada. La organización NUNCA sale de la petición —ni cabecera,
// ni consulta, ni cuerpo—: la resuelve el store. Y resolverla no concede nada: la ACL
// de la base decide después, en la transacción de la operación.
func (h *Handlers) enAnfitrion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := authn.FromContext(r.Context())
		if !ok || c.Subject == "" || c.Tenant == "" {
			next.ServeHTTP(w, r)
			return
		}
		for _, p := range parametrosDeRecurso {
			u, err := uuid.Parse(chi.URLParam(r, p[0]))
			if err != nil {
				continue
			}
			host, err := h.Store.Anfitrion(r.Context(), domain.Actor{Sub: c.Subject, Tenant: c.Tenant}, p[1], u.String())
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if host != "" {
				r = r.WithContext(context.WithValue(r.Context(), claveAnfitriona{}, host))
			}
			break
		}
		next.ServeHTTP(w, r)
	})
}

// id lee un parámetro de ruta que ha de ser un UUID: un id mal formado es un 404
// (no existe), no un 500 de la base de datos.
func id(r *http.Request, nombre string) (string, error) {
	u, err := uuid.Parse(chi.URLParam(r, nombre))
	if err != nil {
		return "", httpx.ErrNotFound
	}
	return u.String(), nil
}

// errorAPI traduce los errores de dominio.
func errorAPI(err error) error {
	switch {
	case errors.Is(err, domain.ErrNoExiste):
		return httpx.ErrNotFound.Wrap(err)
	case errors.Is(err, domain.ErrSinPermiso):
		return httpx.ErrForbidden.Wrap(err)
	case errors.Is(err, domain.ErrInvalido):
		return httpx.ErrUnprocessable.WithCode("invalid_input").WithMessage(err.Error())
	case errors.Is(err, domain.ErrReferenciado):
		return httpx.NewError(http.StatusConflict, "referenced", err.Error())
	case errors.Is(err, domain.ErrLimite):
		return httpx.NewError(http.StatusConflict, "limit_reached", err.Error())
	case errors.Is(err, errEsMiembro):
		return httpx.ErrUnprocessable.WithCode("is_member").WithMessage("esa persona es de la organización")
	case errors.Is(err, peticiones.ErrSinPersona):
		return httpx.ErrUnprocessable.WithCode("not_a_member").WithMessage("nadie de la organización tiene esa dirección")
	case errors.Is(err, errVersionDeContrato):
		return httpx.NewError(http.StatusBadRequest, "unsupported_version", err.Error())
	case errors.Is(err, errDemasiadosFlujos):
		return httpx.NewError(http.StatusTooManyRequests, "too_many_streams", err.Error())
	case errors.Is(err, domain.ErrConflicto):
		return httpx.NewError(http.StatusConflict, "rev_conflict", err.Error())
	}
	return err
}

// llamada es lo común de casi todo manejador: el actor y, de la ruta, el id.
func (h *Handlers) hecho(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// idDe valida un UUID recibido en un cuerpo o una consulta.
func idDe(s string) (string, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
