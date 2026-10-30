// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package http implementa la superficie HTTP de Sytue Drive: API REST propia
// (carpetas, ficheros, compartición, búsqueda, cuotas) y el endpoint público de
// descarga por enlace. El servidor WebDAV se monta en internal/webdav.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS). Aislamiento por tenant vía
// RLS (el tenant se inyecta en el contexto desde los claims).
package drivehttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/webdav"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// Deps son las dependencias del router HTTP de Drive.
type Deps struct {
	Cfg      config.Config
	Service  *service.Service
	Verifier *authn.Verifier
	WebDAV   http.Handler
	// AppPasswords deja entrar en /dav con Basic y una contraseña de aplicación. Nil ⇒
	// solo el Bearer.
	AppPasswords *authn.AppPasswords
	// Compliance es la persistencia de legal hold + eDiscovery (la satisface
	// *store.Store). Si es nil, la API admin de compliance no se monta (p. ej. en
	// arranques que no la habilitan). Blobs (objectstore) la usa el export para
	// bajar el contenido de S3; nil => el export responde 500 controlado.
	Compliance complianceStore
	// Locks persiste los bloqueos WOPI (la satisface *store.Store). Nil ⇒ el host WOPI
	// no se monta: sin bloqueo, dos editores sobre el mismo fichero se pisarían sin que
	// nadie se entere, y eso es peor que no ofrecer el protocolo.
	Locks wopiLockStore
	// Editors persiste QUÉ editor externo ha conectado cada organización (la satisface
	// *store.Store). Nil ⇒ el plano de administración responde 404 y solo queda el
	// editor por defecto del despliegue, si lo hay.
	Editors wopiEditorStore
	// External persiste el almacén S3 de terceros (la satisface *store.Store).
	External externalStorageStore
	// ExternalUsage guarda lo que las otras apps de la suite ocupan en la cuota del
	// plan (mig 0026; la satisface *store.Store). Nil ⇒ la puerta interna por la que
	// Sytue Cast lo declara no se monta y la cuota cuenta solo lo de Drive.
	ExternalUsage externalUsageStore
	Blobs         blobGetter
	ReadyCheckers []httpx.Checker
}

// NewRouter construye el router completo de Sytue Drive.
func NewRouter(d Deps) http.Handler {
	// EL TOPE DEL ROUTER TIENE QUE DAR PARA LO QUE ESTE SERVICIO RECIBE.
	//
	// Sin `MaxBodyBytes`, el router aplica su defecto de 1 MiB a TODAS las rutas,
	// y `MaxUploadBytes` —5 GB por defecto, configurable con MAX_UPLOAD_MB— se
	// quedaba en letra muerta: el middleware cortaba mucho antes de llegar al
	// handler. Medido en el piloto: una foto de 1,4 MB, HTTP 413 «file_too_large».
	// Con eso, en Sytue Drive no entraba ningún fichero de más de un mega, y en
	// Sytue Fotos —que guarda los originales en Drive— no entraba ninguna foto de
	// un móvil.
	//
	// El margen de 1 MiB por encima es para lo que no es el fichero: cabeceras,
	// separadores multipart y el redondeo de quien sube.
	//
	// Subir este tope NO afloja las rutas de JSON: `DecodeJSON` tiene su propio
	// límite de 1 MiB y sigue mandando ahí.
	r := httpx.NewRouter(httpx.RouterOptions{
		RequestTimeout: 60 * time.Second,
		MaxBodyBytes:   max(d.Cfg.MaxUploadBytes, storage.MaxCipherBlockBytes) + (1 << 20),
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	// El acuñador se comparte entre la puerta nativa (POST …/wopi-token) y el host
	// WOPI: si no hay clave, ambos quedan apagados a la vez y no hay medias tintas.
	wopiMinter := wopi.NewMinter(wopiCipher(d.Cfg), 0)
	// Qué editor usa cada organización sale de SU configuración, no del despliegue.
	// `WOPI_DISCOVERY_URL` sobrevive como valor por defecto de una instalación con un
	// solo editor para todos: quitarlo obligaría a reconfigurar a mano a quien ya lo
	// tuviera puesto. Lo que decida una organización gana sobre él.
	editorReg := wopi.NewRegistry(editorLookup(d.Editors, d.Cfg.WopiDiscoveryURL), d.Cfg.WopiDiscoveryTTL)
	editorReg.Confiar(d.Cfg.WopiDiscoveryURL)
	h := &Handlers{
		svc:           d.Service,
		cfg:           d.Cfg,
		wopi:          wopiMinter,
		editorReg:     editorReg,
		editors:       d.Editors,
		external:      d.External,
		externalUsage: d.ExternalUsage,
	}
	if d.Service != nil {
		h.bases = d.Service
		h.cipher = d.Service
	}
	tenantAuth := func(next http.Handler) http.Handler {
		return d.Verifier.RequireAuth(d.Verifier.RequireTenant(next))
	}
	// ponytail: cubo en memoria, o sea POR RÉPLICA (el tope real se multiplica por
	// el número de instancias). Basta para lo que esto ataja; si hiciera falta que
	// el tope fuera del servicio entero, pg.RateLimiter ya lo comparte por Postgres
	// como hace Sytue ID.
	apiLimiter := httpx.NewTokenBucket(50, 100)

	// API REST autenticada bajo /api/v1.
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		// Tope por IDENTIDAD del token, DESPUÉS de RequireAuth (antes no hay sujeto
		// que leer). No por IP: detrás del ingress todas las peticiones llegan con
		// la misma, así que un cubo por IP sería un cubo para todos los clientes.
		// El tope es alto a propósito —ninguna sesión legítima se acerca—: lo que
		// corta es el bucle desbocado y el barrido que se lleva la unidad entera.
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// Carpetas.
		// «Exportar tus datos»: los archivos con sus bytes (platform/pkg/portabilidad).
		api.Get("/export", h.Export)
		api.With(httpx.MaxBodyBytes(storage.MaxCipherBlockBytes)).Put("/cipher/blobs/{product}/{id}", h.PutCipherBlob)
		api.Get("/cipher/blobs/{product}/{id}", h.GetCipherBlob)
		api.Get("/cipher/blobs/{product}/{id}/content", h.DownloadCipherBlob)
		api.Delete("/cipher/blobs/{product}/{id}", h.DeleteCipherBlob)
		api.Get("/folders/root", h.GetRoot)
		api.Post("/folders", h.CreateFolder)
		api.Get("/folders/{id}", h.GetFolder)
		api.Get("/folders/{id}/children", h.ListChildren)
		api.Get("/folders/{id}/files", h.ListFolderFiles) // paginación keyset
		api.Patch("/folders/{id}", h.UpdateFolder)        // rename/move
		api.Delete("/folders/{id}", h.DeleteFolder)

		// Ficheros.
		api.Post("/folders/{id}/files", h.UploadFile) // multipart/octet-stream
		// Exportar una vista de Sytue Bases como CSV a la carpeta, y de dónde salió un fichero.
		api.With(httpx.MaxBodyBytes(4<<10)).Post("/folders/{id}/imports/bases", h.ExportarVistaDeBases)
		api.Get("/files/{id}/provenance", h.ProcedenciaDeFichero)
		// Subida directa a S3 con URL prefirmada (descarga la subida de ficheros
		// grandes del app-server): presign (permiso+cuota → URL PUT acotada + handle)
		// y finalize (verifica en S3 + registra). Vía ADICIONAL; PUT /content sigue de
		// fallback. Rutas estáticas: no colisionan con /files/{id} (estático gana).
		api.Post("/files/presign", h.Presign)
		api.Post("/files/finalize", h.Finalize)
		api.Get("/files/{id}", h.GetFile)
		api.Get("/files/{id}/content", h.DownloadFile)
		api.Get("/files/{id}/thumbnail", h.Thumbnail) // miniatura server-side (imágenes ráster)
		api.Patch("/files/{id}", h.UpdateFile)        // rename/move
		api.Delete("/files/{id}", h.DeleteFile)
		// Copia server-side (nueva fila que referencia el MISMO blob; sin duplicar
		// bytes). POST (CSRF: mutación). Fichero y carpeta (recursiva).
		api.Post("/files/{id}/copy", h.CopyFile)
		// Abrir ESTE fichero en un editor WOPI externo: devuelve el token acotado y la
		// dirección del fichero. 404 si no hay WOPI_KEY configurada.
		api.Post("/files/{id}/wopi-token", h.MintWopiToken)
		// Qué sabe abrir el editor configurado. La UI lo pide UNA vez para decidir en
		// qué ficheros enseña «abrir en el editor», en vez de acuñar un token por icono.
		api.Get("/wopi/capabilities", h.WopiCapabilities)
		// Conectar el editor de la ORGANIZACIÓN. Antes era una variable de entorno del
		// servicio: había que editar el despliegue y todas las organizaciones de una
		// instalación compartían editor. Es un ajuste de administración.
		api.Get("/wopi/editor", h.GetWopiEditor)
		api.Group(func(adm chi.Router) {
			adm.Use(d.Verifier.RequireRole("owner", "admin"))
			adm.Post("/wopi/editor/check", h.CheckWopiEditor)
			adm.Put("/wopi/editor", h.SaveWopiEditor)
			adm.Delete("/wopi/editor", h.DeleteWopiEditor)
			// Almacén S3 de terceros de la organización (mig 0023): mismo patrón que
			// el editor. owner/admin, que en una cuenta personal es la persona.
			adm.Get("/storage/external", h.GetExternalStorage)
			adm.Get("/storage/external/instances", h.ListExternalInstances)
			// NAS del cliente por el túnel de Sytue Cloud: crear y ver si ya responde.
			adm.Post("/storage/external/nas", h.ConnectNAS)
			adm.Get("/storage/external/nas/status", h.NASStatus)
			adm.Post("/storage/external/check", h.CheckExternalStorage)
			adm.Put("/storage/external", h.SaveExternalStorage)
			adm.Delete("/storage/external", h.DeleteExternalStorage)
		})
		api.Post("/folders/{id}/copy", h.CopyFolder)
		// La búsqueda lleva lo que escribe la persona: por GET acaba en el log de
		// accesos de cada salto, en el `Referer` y en el historial. Por QUERY va en
		// el cuerpo. GET se queda como respaldo para los clientes ya desplegados.
		httpx.GetYQuery(api, "/search", h.Search)

		// Etiquetas de sensibilidad (paridad Purview / Google Drive labels). La
		// TAXONOMÍA vive en Sytue ID; aquí se lee (proxy con el mismo Bearer, para el
		// selector de la UI) y se pone/quita sobre un fichero, guardando un SNAPSHOT
		// (mig 0013). El cliente solo manda el id de etiqueta: la política la resuelve
		// el servidor contra la taxonomía, así que no se puede falsificar. El
		// enforcement (bloquear enlaces públicos) vive en el servicio, no aquí. Ver
		// handlers_labels.go.
		api.Get("/sensitivity-labels", h.ListSensitivityLabels)
		api.Put("/files/{id}/label", h.SetFileLabel)
		api.Delete("/files/{id}/label", h.ClearFileLabel)

		// Historial de versiones: listar (keyset), descargar una versión concreta y
		// restaurar (no destructivo; solo editor). Restaurar por POST (CSRF: mutación).
		api.Get("/files/{id}/versions", h.ListFileVersions)
		api.Get("/files/{id}/versions/{version}/content", h.DownloadFileVersion)
		api.Post("/files/{id}/versions/{version}/restore", h.RestoreFileVersion)

		// Recientes (todo el tenant, updated_at DESC keyset) y Favoritos
		// (por-usuario): toggle por POST (CSRF: mutación con cuerpo).
		api.Get("/recent", h.Recent)
		api.Get("/favorites", h.ListFavorites)
		api.Post("/favorites", h.SetFavorite)

		// Operaciones en lote: mover/papelera N en UNA operación (POST, CSRF) y
		// descargar una selección/carpeta como ZIP en streaming (GET, navegable).
		api.Post("/batch/move", h.BatchMove)
		api.Post("/batch/trash", h.BatchTrash)
		api.Get("/batch/zip", h.BatchZip)

		// Papelera: listar borrados, restaurar y borrado definitivo (purga).
		api.Get("/trash", h.ListTrash)
		api.Post("/files/{id}/restore", h.RestoreFile)
		api.Post("/folders/{id}/restore", h.RestoreFolder)
		api.Delete("/files/{id}/purge", h.PurgeFile)
		api.Delete("/folders/{id}/purge", h.PurgeFolder)

		// Compartición.
		api.Post("/shares", h.CreateShare)
		api.Get("/files/{id}/shares", h.ListFileShares)
		api.Get("/folders/{id}/shares", h.ListFolderShares)
		api.Delete("/shares/{id}", h.RevokeShare)

		// Compartido conmigo (destinatario de shares kind=user|group): listado plano
		// de raíces + navegación/descarga DENTRO de una carpeta compartida (verja de
		// acceso por share, deny-by-default) + subida gated a editor. Escritura por
		// POST (CSRF: mutación con cuerpo).
		api.Get("/shared-with-me", h.SharedWithMe)
		api.Get("/shared/{id}/list", h.SharedList)
		api.Get("/shared/{id}/content", h.SharedContent)
		api.Post("/shared/{id}/files", h.SharedUpload)
		// Pedir acceso de edición a lo compartido en lectura (peticiones.go).
		api.Post("/shared/{id}/edit-requests", h.PedirEdicion)
		api.Get("/shared/{id}/edit-requests", h.ListarPeticiones)
		api.Post("/shared/{id}/edit-requests/{sub}/grant", h.ConcederPeticion)
		api.Delete("/shared/{id}/edit-requests/{sub}", h.RechazarPeticion)

		// Unidades de equipo (team/shared drives): propiedad de la ORG, no de un
		// usuario. Acceso por MEMBRESÍA deny-by-default y por ROL (viewer<editor<
		// manager), impuesto en el servicio (no-miembro → 404, rol insuficiente →
		// 403). La gestión de la unidad y de miembros es de manager; el contenido
		// (navegar/crear/subir/descargar/borrar) se AÍSLA por la unidad vía RLS.
		api.Post("/team-drives", h.CreateTeamDrive)
		api.Get("/team-drives", h.ListTeamDrives)
		api.Get("/team-drives/{id}", h.GetTeamDrive)
		api.Patch("/team-drives/{id}", h.UpdateTeamDrive) // renombrar/archivar
		api.Get("/team-drives/{id}/members", h.ListTeamDriveMembers)
		api.Put("/team-drives/{id}/members", h.PutTeamDriveMember)
		api.Delete("/team-drives/{id}/members/{subject}", h.RemoveTeamDriveMember)
		api.Get("/team-drives/{id}/children", h.ListTeamDriveChildren)
		api.Post("/team-drives/{id}/folders", h.CreateTeamDriveFolder)
		api.Post("/team-drives/{id}/files", h.UploadTeamDriveFile)
		api.Get("/team-drives/{id}/files/{fileID}/content", h.DownloadTeamDriveFile)
		api.Delete("/team-drives/{id}/folders/{folderID}", h.DeleteTeamDriveFolder)
		api.Delete("/team-drives/{id}/files/{fileID}", h.DeleteTeamDriveFile)

		// Adjuntos de CANAL de Sytue Rooms: ficheros cuyo dueño es el canal, no «Mi
		// unidad» (que es un espacio compartido del tenant). La RLS de la mig 0014 los
		// saca del ámbito personal —así que ninguna ruta de arriba los expone— y estas
		// cuatro son la única puerta: exigen MEMBRESÍA del canal, verificada contra
		// services/rooms en CADA petición (no-miembro → 404, sin filtrar existencia).
		api.Post("/channels/{channelID}/files", h.UploadChannelFile)
		api.Get("/channels/{channelID}/files/{fileID}", h.GetChannelFile)
		api.Get("/channels/{channelID}/files/{fileID}/content", h.DownloadChannelFile)
		api.Get("/channels/{channelID}/files/{fileID}/thumbnail", h.ThumbnailChannelFile)
		// «Guardar en mi Drive»: copia el adjunto al espacio del usuario. Autoriza la
		// MISMA verja de canal que las tres de arriba, no el permiso de «Mi unidad».
		api.Post("/channels/{channelID}/files/{fileID}/copy", h.CopyChannelFileToMyDrive)

		// Cuotas. Lectura para cualquier miembro; cambio de límite solo admin/owner.
		api.Get("/quota", h.GetQuota)
		api.With(d.Verifier.RequireRole("admin", "owner")).Put("/quota", h.SetQuota)

		// Compliance: legal hold + eDiscovery (paridad M365 Purview / Google Vault,
		// pieza NO destructiva). Acceso PRIVILEGIADO a ficheros ajenos, gateado por
		// rol de org (owner|admin) en el borde; cada handler re-verifica el rol
		// (defensa en profundidad) y AUDITA. Aislamiento por org via RLS + predicado
		// tenant explícito. La UI de Console es una ola posterior. Ver
		// handlers_compliance.go.
		if d.Compliance != nil {
			comp := &complianceHandlers{store: d.Compliance, blobs: d.Blobs}
			api.Route("/admin", func(adm chi.Router) {
				adm.Use(d.Verifier.RequireRole(complianceRoles...))
				adm.Post("/legal-holds", comp.createHold)
				adm.Get("/legal-holds", comp.listHolds)
				adm.Post("/legal-holds/{holdID}/release", comp.releaseHold)
				// POST SOLO PORQUE GET NO LLEVA CUERPO: esta búsqueda no cambia
				// nada —valida, lee y devuelve— y un POST anuncia lo contrario, así
				// que los intermediarios no la reintentan y quien lee el log supone
				// que hubo escritura. Con QUERY la petición dice la verdad. POST se
				// queda para la consola ya desplegada.
				httpx.PostYQuery(adm, "/ediscovery/search", comp.ediscoverySearch)
				adm.Get("/ediscovery/export", comp.ediscoveryExport)
				// Traza de compliance (holds + eDiscovery) para la Auditoría unificada
				// de Console. Listado keyset + filtros + CSV (SIEM). Ver handlers_audit.go.
				adm.Get("/audit-log", comp.listAuditLog)
			})
		}
	})

	// Compatibilidad de ficheros (lado ENTRANTE): Sytue Drive respondiendo la API
	// de Google Drive v3 bajo /compat/google y Microsoft Graph (files) bajo
	// /compat/graph, para integraciones de terceros con «base URL» configurable.
	// Mismo Bearer de Sytue ID y MISMO servicio que /api/v1 ⇒ mismo aislamiento
	// (tenant/RLS, unidades de equipo, permisos). Ver handlers_compat.go.
	mountCompat(r, &compatHandlers{svc: d.Service, cfg: d.Cfg}, func(next http.Handler) http.Handler {
		return d.Verifier.RequireGoogleAPIs(googleScopesLectura, googleScopesEscritura)(d.Verifier.RequireTenant(next))
	}, tenantAuth)

	// Host WOPI (lado ENTRANTE): Office Online / Collabora / OnlyOffice abriendo un
	// fichero que vive aquí, como hacen contra Nextcloud u ownCloud. NO lleva el
	// middleware de auth: autoriza su propio token acotado (ver handlers_wopi.go), no
	// el Bearer de Sytue ID. Si no hay WOPI_KEY configurada, no se monta nada: sin
	// clave no se pueden sellar tokens y ofrecer el protocolo a medias sería peor.
	if wopiMinter.Enabled() && d.Locks != nil {
		mountWopi(r, &wopiHandlers{
			svc:       d.Service,
			locks:     d.Locks,
			minter:    wopiMinter,
			maxUpload: d.Cfg.MaxUploadBytes,
		})
	}

	// Puerta INTERNA servicio-a-servicio, fuera de /api/v1 (no hereda RequireAuth):
	// Sytue Cast declara aquí el total que ocupa en cada organización, porque la
	// cuota del plan es una y Drive es su dueño (CAST-ENCARGO §8, mig 0026). La
	// autoriza el secreto compartido, no un JWT de persona; el tenant viaja en el
	// cuerpo y se fija en la transacción para que RLS aísle la fila. Con el secreto
	// vacío la ruta existe y no deja pasar a nadie —es lo que hace SecretoInterno—,
	// así que ingress_test la ve siempre y comprueba que /internal no se publica.
	// Ver handlers_internal.go.
	if d.ExternalUsage != nil {
		r.With(httpx.SecretoInterno(d.Cfg.CastInternalSecret)).Put("/internal/quota/external/{app}", h.SetExternalUsage)
	}

	// Photos copia al Drive de la dueña de un álbum la foto que le añade quien lo edita.
	// De servicio a servicio: con X-Forwarded-For (el ingress) es 404, y sin el secreto
	// de la suite, 401.
	r.With(httpx.SoloDesdeDentro(d.Cfg.HubInternalSecret)).Post(RutaCopiaFotos, copiaFotos(d.Service))
	// Docs tira, restaura o purga el fichero de un documento cuando la persona lo
	// hace desde Docs (docs_internas.go). Misma frontera que la de Photos, con un
	// secreto PROPIO de Docs y Drive: esta puerta borra.
	r.With(httpx.SoloDesdeDentro(d.Cfg.DocsInternalSecret)).Post(RutaFicheroDocs, ficheroDocs(d.Service))

	// Enlace público (sin auth; el token es el secreto): fichero (descarga) o
	// carpeta (listado navegable con migas). POST = envío de contraseña; /* =
	// subruta dentro de un enlace a carpeta.
	r.Get("/public/{token}", h.PublicEntry)
	r.Post("/public/{token}", h.PublicUnlock)
	r.Get("/public/{token}/*", h.PublicPath)

	// WebDAV (RFC 4918) bajo /dav, autenticado con el mismo JWT o, para los clientes
	// del sistema, con Basic y una contraseña de aplicación canjeada por él.
	if d.WebDAV != nil {
		r.Mount("/dav", d.AppPasswords.Basic(tenantAuth(webdav.WithTenant(d.WebDAV))))
	}

	return r
}
