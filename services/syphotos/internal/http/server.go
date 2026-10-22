// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Deps son las dependencias del router HTTP de Sytue Photos.
type Deps struct {
	Cfg     config.Config
	Store   *store.Store
	Objetos *objectstore.Client
	Ingesta *service.Ingesta
	// Drive es el cliente con que se copia al Drive de la dueña de un álbum lo que le
	// añade quien lo edita (edicion.go).
	Drive         *drive.Cliente
	Limpieza      *service.Limpieza
	Verifier      *authn.Verifier
	ReadyCheckers []httpx.Checker
}

// MaxDerivadasSimultaneas acota cuántas fotos se decodifican a la vez.
//
// Hay tope y no «las que pida el runtime» porque decodificar una foto de 24
// megapíxeles reserva del orden de 100 MB, y una cuadrícula puede pedir cien
// miniaturas de golpe. Sin tope, el primer scroll de una galería recién subida
// se lleva por delante el proceso — y un hueco gris se convierte en una caída.
//
// DOS, Y NO CUATRO. Cuatro salía de razonar sobre la memoria del proceso sin
// mirar la que le da el contenedor: con el límite de 192Mi que tenía el chart,
// el proceso moría por OOM en cuanto se abría una galería de diez fotos. El
// límite ya sube a 512Mi, y aun así el peor caso de cuatro (400 MB solo de
// decodificación, más la cascada, que mantiene todos los tamaños a la vez) no
// cabe con holgura. Dos deja margen y solo cuesta que la primera visita a una
// galería nueva tarde un poco más: las derivadas se guardan y no se repiten.
//
// Este número y `resources.limits.memory` del chart van juntos. Subir uno sin el
// otro es volver al OOM.
const MaxDerivadasSimultaneas = 2

// NewRouter construye el router completo de Sytue Photos.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 30 * time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{
		store:     d.Store,
		cfg:       d.Cfg,
		objetos:   d.Objetos,
		ingesta:   d.Ingesta,
		limpieza:  d.Limpieza,
		derivando: make(chan struct{}, MaxDerivadasSimultaneas),
		hub:       hubnotify.New(d.Cfg.HubBaseURL, d.Cfg.HubInternalSecret),
		avisados:  hubnotify.NuevaUnaVez(VentanaAviso),
		anyadidos: hubnotify.NuevaUnaVez(VentanaAvisoAnyadidos),
		drive:     d.Drive,

		completando: hubnotify.NuevaUnaVez(VentanaMetadatos),
	}

	// ponytail: cubo en memoria, o sea POR RÉPLICA. Esta API no tiene superficie
	// anónima —todo va tras RequireAuth—, así que no hace falta el limitador
	// compartido por Postgres que sí necesita el enlace público de Notes.
	//
	// El tope es más ancho que el de otros servicios a propósito: pintar una
	// línea de tiempo son decenas de peticiones seguidas de derivadas, y un cubo
	// estrecho convertiría un desplazamiento rápido en huecos grises.
	apiLimiter := httpx.NewTokenBucket(100, 200)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// QUERY ADEMÁS DE GET en los dos que lo piden, y en ninguno más.
		//
		// `/timeline` lleva el texto que escribe la persona (`q=`) y hasta ocho
		// términos alternativos: eso acaba escrito en el log de accesos de cada
		// salto y en el `Referer`. `/similar` lleva hasta doce UUID, que son unos
		// quinientos caracteres de URL y crecen con la selección.
		//
		// `/duplicates` y `/themes` NO: su único parámetro es `limit`, que cabe de
		// sobra en una URL, y moverlo al cuerpo solo añadiría otra forma de llamar
		// a lo mismo.
		httpx.GetYQuery(api, "/timeline", h.LineaDeTiempo)

		// «Exportar tus datos»: lo que Fotos sabe de cada foto (platform/pkg/portabilidad).
		api.Get("/export", h.ExportarDatos)
		api.Get("/settings", h.AjustesOrganizacion)
		api.With(d.Verifier.RequireRole("owner", "admin")).Put("/settings", h.GuardarAjustesOrganizacion)

		api.Post("/assets", h.Indexar)

		api.Get("/assets/{id}", h.Activo)
		api.Delete("/assets/{id}", h.ALaPapelera)

		// La variante ANTES de la receta no importa —son segmentos distintos—,
		// pero `variants` va con nombre propio y no como `{algo}` para que una
		// ruta nueva no lo capture por accidente.
		api.Get("/assets/{id}/variants/{variante}", h.Derivada)

		api.Put("/assets/{id}/favorite", h.MarcarFavorita)
		api.Put("/assets/{id}/description", h.GuardarDescripcion)

		api.Get("/assets/{id}/similar", h.Parecidas)
		api.Get("/assets/{id}/nearby", h.CercaDe)
		httpx.GetYQuery(api, "/similar", h.ParecidasAVarias)
		api.Get("/duplicates", h.Duplicadas)
		api.Get("/themes", h.Temas)
		api.Get("/memories", h.Recuerdos)

		api.Get("/assets/{id}/recipe", h.Receta)
		api.Put("/assets/{id}/recipe", h.GuardarReceta)
		api.Get("/assets/{id}/export", h.Exportar)
		api.Get("/trash", h.Papelera)
		api.Delete("/trash", h.Purgar)
		api.Delete("/trash/{id}", h.Purgar)
		api.Post("/assets/{id}/restore", h.Restaurar)
		api.Get("/albums", h.Albumes)
		api.Post("/albums", h.CrearAlbum)
		api.Get("/albums/{album}", h.Album)
		api.Patch("/albums/{album}", h.RenombrarAlbum)
		api.Delete("/albums/{album}", h.BorrarAlbum)
		api.Get("/albums/{album}/assets", h.FotosDelAlbum)
		api.Post("/albums/{album}/assets", h.AnyadirAlAlbum)
		api.Delete("/albums/{album}/assets/{id}", h.QuitarDelAlbum)
		api.Post("/albums/{album}/share", h.CompartirAlbum)
		api.Delete("/albums/{album}/share", h.DejarDeCompartirAlbum)
		// Compartir con UNA persona (personas.go): su dueña lo da y lo retira; quien
		// lo recibe lo ve en «Compartido conmigo», con su sesión y solo para ver.
		api.Get("/albums/{album}/people", h.PersonasDelAlbum)
		api.Put("/albums/{album}/people/{sub}", h.CompartirConPersona)
		api.Delete("/albums/{album}/people/{sub}", h.DejarDeCompartirConPersona)
		api.Get("/shared/albums", h.AlbumesCompartidos)
		api.Get("/shared/albums/{album}", h.AlbumCompartido)
		api.Get("/shared/albums/{album}/assets/{id}/variants/{variante}", h.VarianteCompartida)
	})

	// Álbum compartido: SIN sesión, solo lectura y con su propio tope por IP.
	//
	// El limitador va por Postgres y no en memoria: esta es la única superficie
	// anónima del servicio, así que su tope no puede multiplicarse por el número
	// de réplicas —que crece justo cuando hay carga—.
	//
	// Y DOS VECES, con el mismo tope. `/public` es por donde pregunta la web: su BFF,
	// dentro del clúster. `/api/v1/public` es lo mismo por el ingress —que solo publica
	// `/api/v1`— para las apps de escritorio y del teléfono, que no tienen BFF y abren
	// el enlace `/p/{token}` en su propia pantalla. Chi prueba los tramos fijos antes
	// que el comodín del grupo con sesión, así que la ruta no pasa por RequireAuth.
	publico := pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 4, Burst: 40})
	r.Group(func(pub chi.Router) {
		pub.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: publico}))
		for _, base := range []string{"", "/api/v1"} {
			pub.Get(base+"/public/albums/{token}", h.AlbumPublico)
			pub.Get(base+"/public/albums/{token}/assets/{id}/variants/{variante}", h.VariantePublica)
		}
	})

	return r
}
