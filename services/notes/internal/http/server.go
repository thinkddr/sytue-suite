// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package noteshttp implementa la superficie HTTP de Sytue Notepads: API REST propia
// (cuadernos, notas, etiquetas con estado, búsqueda y referencias cruzadas) más la
// lectura sin sesión de un cuaderno publicado.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS). Aislamiento por organización
// vía RLS (el tenant se inyecta en el contexto desde los claims); la separación
// entre personas la aplican las cláusulas de visibilidad del store.
package noteshttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/notes/internal/config"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Deps son las dependencias del router HTTP de Notes.
type Deps struct {
	Cfg           config.Config
	Store         *store.Store
	Verifier      *authn.Verifier
	ReadyCheckers []httpx.Checker
}

// NewRouter construye el router completo de Sytue Notepads.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 30 * time.Second, MaxBodyBytes: max(domain.MaxNoteRequestBody, maxNotebookBackupBody)})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{
		store: d.Store, cfg: d.Cfg, hub: hubnotify.New(d.Cfg.HubBaseURL, d.Cfg.HubInternalSecret),
		avisados: hubnotify.NuevaUnaVez(VentanaAviso),
	}

	// ponytail: cubo en memoria, o sea POR RÉPLICA (el tope real se multiplica por
	// el número de instancias). Si hiciera falta que fuera del servicio entero,
	// pg.RateLimiter ya lo comparte por Postgres como hace Sytue ID.
	apiLimiter := httpx.NewTokenBucket(50, 100)
	// El enlace público no lleva token, así que su cubo va por IP y es MUCHO más
	// estrecho: es la única superficie de este servicio que puede visitar cualquiera,
	// y adivinar tokens a base de peticiones es el ataque que tiene enfrente.
	//
	// Y POR ESO ESTE SÍ VA COMPARTIDO, al contrario que el de arriba. Un cubo en
	// memoria vive en cada réplica, así que con `replicaCount: 2` —y autoescalado
	// por encima— el tope real era el doble o más del configurado: quien reparte
	// sus intentos entre las instancias multiplica su presupuesto por el número de
	// pods, y ese número crece justo cuando hay carga. Es el único límite de este
	// servicio que no puede permitirse eso.
	//
	// `pg.RateLimiter` lo lleva a Postgres de forma atómica, igual que Sytue ID
	// hace con /login. Cuesta una consulta por visita a un cuaderno publicado, que
	// es una página que además lee de la base de datos de todas formas.
	publicLimiter := pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 2, Burst: 20})

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// Cuadernos (el árbol de la barra lateral).
		// «Exportar tus datos»: la entrega de los cuadernos (platform/pkg/portabilidad).
		api.Get("/export", h.Export)
		api.Get("/notebooks", h.ListNotebooks)
		api.Post("/notebooks", h.CreateNotebook)
		api.Post("/daily", h.DailyNotebook)
		api.Get("/daily", h.DailyRange)
		api.Get("/daily/this-day", h.ThisDay)
		api.Get("/notebooks/{id}", h.GetNotebook)
		api.Put("/notebooks/{id}", h.UpdateNotebook)
		api.Delete("/notebooks/{id}", h.DeleteNotebook)
		// Archivo (archivo.go): POST archiva la rama, DELETE la devuelve.
		api.Post("/notebooks/{id}/archive", h.ArchiveNotebook)
		api.Delete("/notebooks/{id}/archive", h.UnarchiveNotebook)
		// Pedir acceso de edición a un cuaderno ajeno que se lee (peticiones.go).
		api.Post("/notebooks/{id}/edit-requests", h.PedirEdicion)
		api.Get("/notebooks/{id}/edit-requests", h.ListarPeticiones)
		api.Post("/notebooks/{id}/edit-requests/{sub}/grant", h.ConcederPeticion)
		api.Delete("/notebooks/{id}/edit-requests/{sub}", h.RechazarPeticion)
		// Compartir con una persona (personas.go): el cuaderno, su dueño; la nota, quien
		// la escribió. Y lo que otras personas han compartido conmigo, nota a nota (los
		// cuadernos compartidos ya salen en el árbol de /notebooks).
		api.Get("/notebooks/{id}/people", h.PersonasDeCuaderno)
		api.Put("/notebooks/{id}/people/{sub}", h.CompartirCuaderno)
		api.Delete("/notebooks/{id}/people/{sub}", h.DejarDeCompartirCuaderno)
		api.Get("/notes/{id}/people", h.PersonasDeNota)
		api.Put("/notes/{id}/people/{sub}", h.CompartirNota)
		api.Delete("/notes/{id}/people/{sub}", h.DejarDeCompartirNota)
		api.Get("/shared/notes", h.NotasCompartidas)

		// Notas dentro de un cuaderno.
		api.Get("/notebooks/{id}/notes", h.ListNotes)
		api.Get("/notebooks/{id}/notes/page", h.ListNotesPage)
		// Llevarse el cuaderno entero (RGPD art. 20). Va bajo la sesión normal y
		// NO tras `RequireRole`: son los datos de quien los pide, y exigir un rol
		// para ejercer un derecho propio sería justo lo contrario de lo que dice
		// ese artículo. La autorización es la de siempre —solo se exporta lo que
		// ya se puede leer— y queda anotado en la traza.
		api.Get("/notebooks/{id}/export", h.ExportNotebook)
		// Y la vuelta: subir un `.onex` crea un cuaderno nuevo. No sobrescribe
		// ninguno —restaurar encima de lo que hay es cómo se pierde lo que había—
		// así que quien importa acaba con dos y decide.
		api.Post("/notebooks/import", h.ImportNotebook)
		// Antes de eso, MIRARLO: un volcado se recibe por correo o se saca de una
		// copia de hace meses, y quien lo abre no sabe qué cuaderno es. Esta ruta
		// lo lee y lo devuelve sin escribir nada.
		api.Post("/notebooks/preview", h.PreviewNotebook)
		api.Post("/notebooks/{id}/notes", h.CreateNote)
		api.Post("/notebooks/{id}/encrypted-notes", h.CreateEncryptedNote)

		// Rutas literales ANTES de /notes/{id}: si no, "search" y "trash" se
		// tomarían por un identificador y darían 400 en vez de responder.
		// LA BÚSQUEDA ACEPTA TAMBIÉN QUERY, como `/search` de Drive y `/admin/users`
		// de Sytue ID: el cliente la pide con `api.Consulta`, que manda el término en
		// el CUERPO para que no se quede en el log de acceso de cada proxy del camino.
		//
		// Registrada solo con Get no se rompía nada visible: el router contesta 405 y
		// el cliente reintenta por GET. Lo que se perdía es lo único que QUERY venía
		// a dar —el término acababa en la URL igualmente—, y encima el cliente apaga
		// QUERY para el resto de consultas DE ESTE SERVICIO en cuanto ve el 405
		// (`sinQuery` es una marca del cliente, y hay un cliente por servicio).
		// O sea: una ruta sin registrar deja sordo al servicio entero.
		//
		// `/notes/linked` y `/notes/trash` NO se registran aquí: el cliente las pide
		// por GET a propósito (lo que viaja son identificadores, no texto buscado), y
		// declarar un método que nadie usa es cosmético.
		httpx.GetYQuery(api, "/notes/search", h.Search)
		api.Get("/notes/linked", h.LinkedNotes)
		api.Get("/notes/trash", h.ListTrash)
		// Las tareas de todos los cuadernos (vista «Tareas»), y posponer una a otro día.
		api.Get("/tasks", h.ListTasks)
		api.Post("/notes/{id}/postpone", h.PostponeTask)
		// Recordatorios (recordatorios.go): los míos, y el mío en una nota.
		api.Get("/reminders", h.ListReminders)
		api.Put("/notes/{id}/reminder", h.SetReminder)
		api.Delete("/notes/{id}/reminder", h.DeleteReminder)
		api.Get("/notes/pinned", h.PinnedNotes)
		// Los #hashtags escritos en las notas, con su recuento (no las etiquetas con
		// estado de más abajo). Las notas de uno se piden a la búsqueda: `q=#plan`.
		api.Get("/hashtags", h.Hashtags)

		api.Put("/notes/{id}", h.UpdateNote)
		api.Put("/notes/{id}/encrypted", h.UpdateEncryptedNote)
		api.Post("/notes/{id}/encrypted-content", h.MigrateEncryptedContent)
		api.Post("/notes/{id}/move", h.MoveNote)
		api.Post("/notes/{id}/state", h.SetNoteState)
		api.Delete("/notes/{id}", h.DeleteNote)
		api.Post("/notes/{id}/restore", h.RestoreNote)
		api.Delete("/notes/{id}/purge", h.PurgeNote)

		// Etiquetas con estado.
		api.Get("/tags", h.ListTags)
		api.Post("/tags", h.SaveTag)
		api.Put("/tags/{id}", h.SaveTag)
		api.Delete("/tags/{id}", h.DeleteTag)

		// Traza de auditoría: quién publicó un cuaderno hacia fuera y quién
		// destruyó contenido. Es ACCESO PRIVILEGIADO —dice qué hicieron los
		// demás— así que va tras `RequireRole`, igual que el `audit-log` de
		// Drive. Sin ese middleware, cualquier miembro vería la actividad de
		// toda la organización.
		api.With(d.Verifier.RequireRole("owner", "admin")).
			Get("/admin/audit-log", h.ListAudit)

		// Retención legal: decide qué NO destruye el recolector de papelera.
		// Mismo rol y mismas rutas que Drive (`/legal-holds`), para que la
		// consola de cumplimiento hable igual con los dos.
		//
		// Liberar es POST y no DELETE porque no borra la fila: la deja con su
		// fecha y su autor, que es lo que hace falta para responder «¿qué
		// estuvo preservado y entre qué fechas?».
		api.With(d.Verifier.RequireRole("owner", "admin")).Group(func(adm chi.Router) {
			adm.Post("/admin/legal-holds", h.CreateHold)
			adm.Get("/admin/legal-holds", h.ListHolds)
			adm.Post("/admin/legal-holds/{holdID}/release", h.ReleaseHold)
		})
	})

	// Cuaderno publicado: SIN sesión, solo lectura, con su propio tope por IP.
	r.Group(func(pub chi.Router) {
		pub.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: publicLimiter}))
		pub.Get("/public/notebooks/{token}", h.PublicNotebook)
	})

	return r
}
