// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package http es la capa de transporte HTTP/WebSocket de Sytue Docs: router
// chi, validación de entrada, autorización vía Sytue ID (platform/pkg/authn) y
// el punto de entrada de la edición colaborativa (collab). Mapea los errores de
// dominio a los errores JSON canónicos (platform/pkg/httpx).
package http

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/logoorg"
	"git.sytue.eu/sytue/one/services/docs/internal/ocr"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Deps son las dependencias de la capa HTTP.
type Deps struct {
	Service  *domain.Service
	Store    *store.Store
	Hub      *collab.Hub
	Objects  *objectstore.Client // puede ser nil en configuraciones sin S3
	Verifier *authn.Verifier
	// SnapshotInlineLimit: tamaño máximo (bytes) de snapshot inline antes de S3.
	SnapshotInlineLimit int

	// Conector de datos externos (Power Query). Sealer cifra/descifra las
	// credenciales at-rest (puede ser nil: sin clave, las conexiones con
	// credenciales se rechazan). Executor ejecuta REST/PostgreSQL con SSRF+topes.
	Sealer   *connectors.Sealer
	Executor *connectors.Executor

	// OCR es el cliente de Sytue OCR. Puede ser nil: sin servicio, la interfaz
	// NO ofrece reconocer texto (ADR-0034).
	OCR *ocr.Cliente

	// Drive es el cliente de Sytue Drive: donde vive el fichero de cada documento
	// nuevo (hogar_en_drive.go). Puede ser nil —sin URL configurada— y entonces
	// los documentos se crean solo en Docs, como antes de esta ola.
	Drive *drive.Cliente

	// Sello configura la autoridad de sellado de tiempo de las firmas.
	Sello domain.OpcionesSello
	// FirmaAnclas son las raíces contra las que se verifican las firmas de un PDF
	// recibido. nil = sin decidir la confianza (pdfverificar.go).
	FirmaAnclas *x509.CertPool

	// PublicURL es la raíz pública de Sytue Docs; la necesita el catálogo WOPI.
	PublicURL string

	// HubNotifier avisa al centro de notificaciones de la suite cuando alguien
	// comparte un documento con una persona. Puede ser nil (sin HUB_BASE_URL):
	// compartir funciona igual, solo que sin aviso.
	HubNotifier *hubnotify.Client
	// IDBaseURL y Correo mandan al dueño el correo de «te piden editar» (peticiones.go).
	IDBaseURL string
	// IDOrgsSecret abre /internal/people/orgs de Sytue ID: en qué organizaciones
	// está quien firma, para la política de firmas de fuera (ADR 0059). Sin él no se
	// pide firma fuera.
	IDOrgsSecret string
	Correo       correo.Config
	// WopiHosts son los hosts WOPI cuyos ficheros se aceptan abrir.
	WopiHosts []string
	// SecretoInterno es el de la suite: abre /internal/drive/files, por donde
	// Drive trae lo que le pasa en su papelera al fichero de un documento.
	SecretoInterno string
	// Env decide qué orígenes admite el handshake del WebSocket de colaboración:
	// en producción, sin los hosts `.localhost` de desarrollo.
	Env pcfg.Environment
}

// Handler agrupa las dependencias y expone las rutas.
type Handler struct {
	svc *domain.Service
	// store es el contrato de persistencia (domain.Store). En producción lo
	// satisface *store.Store; declararlo como interfaz permite inyectar fakes en
	// los tests de la capa HTTP sin PostgreSQL.
	store    domain.Store
	hub      *collab.Hub
	presence *collab.PresenceRegistry
	objects  *objectstore.Client
	verifier *authn.Verifier
	inline   int

	// Conector de datos externos. connStore es *store.Store (contrato concreto,
	// no domain.Store, porque estas tablas no son del dominio documental).
	connStore    connectorStore
	sealer       *connectors.Sealer
	executor     *connectors.Executor
	execLimiter  httpx.RateLimiter
	publicURL    string
	hubNotifier  *hubnotify.Client
	idBaseURL    string
	idSecreto    string
	correo       correo.Config
	peticiones   peticionesStore
	vistasFiltro vistasFiltroStore
	auditoria    auditoriaStore
	canje        canjeStore
	// claveEnlaces lee y cambia la contraseña de los enlaces; limitesClave acota los
	// intentos de desbloqueo (enlaces_clave.go).
	claveEnlaces claveEnlacesStore
	limitesClave *limitesClave
	// politicaEnlaces guarda el techo de caducidad de los enlaces de la organización.
	politicaEnlaces politicaEnlacesStore
	locales         locales
	wopiHosts       []string
	env             pcfg.Environment
	// ocr es el cliente de Sytue OCR. nil = sin servicio de reconocimiento, y la
	// interfaz lo pregunta para no ofrecer un botón que va a fallar (ADR-0034).
	ocr *ocr.Cliente
	// drive deja el fichero de cada documento nuevo en la unidad de su dueño. nil =
	// sin Drive configurado: los documentos se crean solo aquí.
	drive *drive.Cliente
	// sello son las opciones de la autoridad de sellado de tiempo. Con URL vacía
	// las firmas salen sin sello y la interfaz lo dice.
	sello domain.OpcionesSello
	// firmaAnclas: ver Deps.FirmaAnclas.
	firmaAnclas *x509.CertPool
	// firmas guarda las firmas con el certificado del sistema entre sus dos pasos.
	firmas almacenFirmas
	// peticionesFirma y ficheros son «Pedir firma» (pdfpedirfirma.go). ficheros es
	// nil sin Drive: una interfaz con un *drive.Cliente nil dentro no lo sería.
	peticionesFirma peticionesFirmaStore
	ficheros        ficherosDrive
	// papelera y secretoInterno: la papelera de Docs y su puerta desde Drive
	// (papelera.go).
	papelera       papeleraStore
	secretoInterno string
	// plantillasOrg es la biblioteca de plantillas de la organización (plantillas_org.go).
	plantillasOrg plantillasOrgStore
	// marcaOrg es la marca de la organización, versionada (temas.go).
	marcaOrg marcaOrgStore
	// logos trae de Sytue ID el logotipo de la organización para incrustarlo con la marca
	// (internal/logoorg). nil = sin base de ID configurada: la marca va sin logo.
	logos *logoorg.Cliente
	// scripts es el almacén de scripts de usuario (scripts.go).
	scripts scriptsStore
}

// selloDeTiempo devuelve la función que pide el sello, o nil si no hay autoridad.
//
// Devolver NIL y no una función que falla es lo que permite al dominio distinguir
// «no hay autoridad» de «la autoridad falló», que son dos frases distintas para
// quien firma.
func (h *Handler) selloDeTiempo(ctx context.Context) func([]byte) ([]byte, error) {
	if h.sello.URL == "" {
		return nil
	}
	// El contexto se captura a propósito: la petición del sello va DENTRO de la
	// que está firmando, así que si el usuario corta, la llamada a la autoridad
	// también se cancela.
	return func(resumen []byte) ([]byte, error) {
		return domain.PedirSello(ctx, resumen, h.sello)
	}
}

// New construye el Handler.
func New(d Deps) *Handler {
	if d.SnapshotInlineLimit <= 0 {
		d.SnapshotInlineLimit = 256 * 1024 // 256 KiB inline por defecto
	}
	h := &Handler{
		svc: d.Service, store: d.Store, hub: d.Hub,
		presence: collab.NewPresenceRegistry(),
		objects:  d.Objects, verifier: d.Verifier, inline: d.SnapshotInlineLimit,
		connStore: d.Store, sealer: d.Sealer, executor: d.Executor,
		// Limitador en-proceso para el endpoint execute (llamadas a red externa):
		// 2 req/s sostenidas, ráfaga 10, por tenant.
		// ponytail: por réplica; migrar a pg.RateLimiter si hace falta presupuesto
		// compartido cross-réplica.
		execLimiter:     httpx.NewTokenBucket(2, 10),
		publicURL:       d.PublicURL,
		hubNotifier:     d.HubNotifier,
		idBaseURL:       d.IDBaseURL,
		idSecreto:       d.IDOrgsSecret,
		correo:          d.Correo,
		peticiones:      d.Store,
		vistasFiltro:    d.Store,
		auditoria:       d.Store,
		plantillasOrg:   d.Store,
		marcaOrg:        d.Store,
		logos:           logoorg.Nuevo(d.IDBaseURL),
		scripts:         d.Store,
		canje:           d.Store,
		politicaEnlaces: d.Store,
		claveEnlaces:    d.Store,
		limitesClave:    nuevosLimitesClave(),
		wopiHosts:       hostsWopi(d),
		env:             d.Env,
		ocr:             d.OCR,
		drive:           d.Drive,
		sello:           d.Sello,
		firmaAnclas:     d.FirmaAnclas,
		firmas:          d.Store,
		ficheros:        ficherosDe(d.Drive),
		peticionesFirma: d.Store,
		papelera:        papeleraDe(d.Store),
		secretoInterno:  d.SecretoInterno,
	}
	if d.Service != nil && d.Objects != nil {
		// La protección de hojas lee el modelo vigente de documentos con la base en S3.
		d.Service.Hidratar = h.hydrateFromS3
	}
	return h
}

func ficherosDe(c *drive.Cliente) ficherosDrive {
	if c == nil {
		return nil
	}
	return c
}

// Mount registra las rutas del API v1 bajo un router ya con middlewares base.
// Prefijo /api/v1 (convención de la suite: drive/chat/hub/calendar exponen igual).
// Las rutas autenticadas exigen JWT de Sytue ID (RequireAuth) y, salvo el
// acceso por enlace público, tenant en el contexto.
func (h *Handler) Mount(r chi.Router) {
	// Catálogo WOPI. PÚBLICO y sin sesión a propósito: lo lee un HOST —Sytue Drive
	// hoy, cualquier otro mañana— desde su servidor, antes de que haya usuario
	// ninguno, y exigirle un JWT lo dejaría sin poder descubrir nada. No expone
	// datos: solo dice qué extensiones abre Sytue Docs y con qué URL.
	r.Get("/wopi/discovery.xml", h.wopiDiscovery)
	// El POST de formulario que manda el HOST al abrir un fichero. Sin sesión y
	// desde otro origen: la credencial es el token que acuñó el host.
	r.Post("/wopi/view", h.wopiLanzar("view"))
	r.Post("/wopi/edit", h.wopiLanzar("edit"))

	// Abrir un fichero de un host WOPI. SIN sesión de Sytue ID: aquí la credencial
	// es el token que acuñó el host tras autorizar a esa persona sobre ESE
	// fichero, y exigir además una sesión rompería el protocolo sin añadir nada.
	// La verja es la lista de hosts admitidos, que sí decide esta instalación.
	r.Post("/api/v1/wopi/open", h.wopiOpen)
	r.Post("/api/v1/wopi/content", h.wopiContent)
	// Guardar y soltar el bloqueo. Sin sesión, igual que los otros dos: la
	// credencial es el token del host, que es quien autorizó a esa persona sobre
	// ESE fichero.
	r.Post("/api/v1/wopi/save", h.wopiSave)
	r.Post("/api/v1/wopi/unlock", h.wopiUnlock)

	// Acceso público por enlace compartido (token): NO requiere JWT.
	r.Get("/api/v1/share/{token}", h.resolveShare)
	// Su vista de solo lectura, en HTML: la página pública `/s/{token}` de la web.
	r.Get("/api/v1/share/{token}/view", h.viewShare)
	// La contraseña de un enlace protegido, a cambio de la prueba que abre lo anterior.
	r.Post("/api/v1/share/unlock", h.unlockShare)

	// La cola de Drive trae aquí lo que le pasa en su papelera al fichero de un
	// documento (papelera.go). De servicio a servicio: desde fuera es 404, y sin el
	// secreto de la suite, 401.
	r.With(httpx.SoloDesdeDentro(h.secretoInterno)).Post("/internal/drive/files", h.desdeDrive)

	r.Group(func(pr chi.Router) {
		pr.Use(h.verifier.RequireAuth)
		pr.Use(h.verifier.RequireTenant)

		// «Exportar tus datos»: los documentos propios en OOXML (platform/pkg/portabilidad).
		pr.Get("/api/v1/export", h.exportData)

		// Importar/abrir un fichero ofimático -> modelo inicial (conversión pura).
		pr.Post("/api/v1/import", h.importDocument)

		// La galería de «Crear nuevo» (plantillas.go). Crear desde una plantilla es
		// el POST de /documents con `template`.
		pr.Get("/api/v1/templates", h.listarPlantillas)
		// Bloques reutilizables para insertar desde la cinta (bloques.go).
		pr.Get("/api/v1/blocks", h.listarBloques)
		pr.Get("/api/v1/blocks/{clave}", h.obtenerBloque)
		// La biblioteca de plantillas de la organización (plantillas_org.go): la marca
		// un administrador sobre un documento y la usan todos sus miembros.
		// Los temas comunes de Docus, Calcus y Diapos y la marca de la organización
		// (temas.go); aplicar uno a un documento es POST /documents/{id}/theme.
		pr.Get("/api/v1/themes", h.listarTemas)
		pr.Put("/api/v1/brand", h.guardarMarcaOrg)
		// Scripts de usuario (scripts.go): almacén, versiones inmutables, aprobación,
		// ejecuciones autorizadas con cuota y su registro de auditoría.
		pr.Get("/api/v1/scripts", h.listarScripts)
		pr.Post("/api/v1/scripts", h.crearScript)
		pr.Get("/api/v1/script-quota", h.cuotaScripts)
		pr.Get("/api/v1/script-runs", h.auditoriaScriptsOrg)
		pr.Put("/api/v1/script-runs/{runId}", h.cerrarEjecucionScript)
		pr.Route("/api/v1/scripts/{sid}", func(sr chi.Router) {
			sr.Get("/", h.verScript)
			sr.Delete("/", h.borrarScript)
			sr.Put("/scope", h.ambitoScript)
			sr.Post("/versions", h.nuevaVersionScript)
			sr.Post("/versions/{v}/propose", h.proponerVersionScript)
			sr.Post("/versions/{v}/review", h.revisarVersionScript)
			sr.Post("/versions/{v}/runs", h.ejecutarScript)
		})
		pr.Route("/api/v1/org-templates/{id}", func(or chi.Router) {
			or.Get("/", h.verPlantillaOrg)
			or.Put("/", h.marcarPlantillaOrg)
			or.Delete("/", h.quitarPlantillaOrg)
			or.Post("/versions", h.publicarPlantillaOrg)
			or.Get("/log", h.registroPlantillaOrg)
		})

		// Operaciones de página sobre un PDF (Sytue PDF, ola P4): combinar,
		// extraer, borrar y rotar. Conversión pura como la de arriba: el fichero
		// lo aporta quien llama, no hay documento que autorizar y nada se guarda.
		pr.Post("/api/v1/pdf/pages", h.pdfPages)
		// Índice de un PDF (páginas con tamaño y extracto de texto), para poder
		// elegirlas reconociéndolas en vez de escribiendo su número de memoria.
		pr.Post("/api/v1/pdf/inspect", h.pdfInspect)
		pr.Post("/api/v1/pdf/attachment", h.pdfAdjunto)
		// El resto de Sytue PDF, todas con la misma forma de conversión pura.
		pr.Post("/api/v1/pdf/render", h.pdfRender)     // una página dibujada en SVG
		pr.Post("/api/v1/pdf/content", h.pdfContenido) // todo lo del documento, de una vez
		pr.Post("/api/v1/pdf/text", h.pdfTexto)        // texto con posición, para buscar
		pr.Post("/api/v1/pdf/annotate", h.pdfAnotar)
		pr.Post("/api/v1/pdf/form", h.pdfFormulario)
		pr.Post("/api/v1/pdf/protect", h.pdfProteger)
		pr.Post("/api/v1/pdf/sign", h.pdfFirmar)
		pr.Post("/api/v1/pdf/verify", h.pdfVerificar) // las firmas de un PDF recibido
		// La firma con el certificado del SISTEMA, en dos pasos (ADR 0051): aquí se
		// prepara y se completa, y entre medias firma el equipo de la persona.
		pr.Post("/api/v1/pdf/sign/prepare", h.pdfFirmaPreparar)
		pr.Post("/api/v1/pdf/sign/complete", h.pdfFirmaCompletar)
		// «Pedir firma» a otra persona, de la organización (ADR 0051 §7) o de otra
		// (ADR 0057): las rutas de una petición pasan a la organización donde vive
		// para su destinatario de fuera.
		pr.Post("/api/v1/pdf/signature-requests", h.pedirFirma)
		pr.Get("/api/v1/pdf/signature-requests", h.listarPeticionesFirma)
		pr.Route("/api/v1/pdf/signature-requests/{id}", func(pf chi.Router) {
			pf.Use(h.peticionEnAnfitrion)
			pf.Post("/sign", h.firmarPeticion)
			pf.Post("/decline", h.rechazarPeticionFirma)
			pf.Post("/open", h.abrirPeticionFirma)
			pf.Post("/fetch-signed", h.traerFirmado)
			pf.Post("/block", h.bloquearDesdePeticion)
			pf.Delete("/", h.cancelarPeticionFirma)
		})
		// Las personas bloqueadas de quien llama (ADR 0059): la lista va con las
		// peticiones; aquí se quita un bloqueo.
		pr.Delete("/api/v1/pdf/signature-blocks/{sub}", h.desbloquearFirma)
		// La política de la organización para las peticiones de fuera: la decide
		// quien administra (Sytue Admin).
		pr.Group(func(adm chi.Router) {
			adm.Use(h.verifier.RequireRole("owner", "admin"))
			adm.Get("/api/v1/pdf/signature-policy", h.politicaFirma)
			adm.Put("/api/v1/pdf/signature-policy", h.guardarPoliticaFirma)
		})
		pr.Post("/api/v1/pdf/compare", h.pdfComparar)
		pr.Post("/api/v1/pdf/tools", h.pdfHerramientas) // comprimir, recortar, imágenes→PDF, PDF/A
		pr.Post("/api/v1/pdf/edit", h.pdfEditar)        // ola P5: texto, imágenes y capas
		pr.Post("/api/v1/pdf/ocr", h.pdfOCR)            // capa de texto de un escaneo (ADR-0034)

		// El techo de caducidad de los enlaces de la organización: lo lee cualquiera
		// (el diálogo de compartir lo enseña) y lo cambia quien administra.
		pr.Get("/api/v1/link-policy", h.verPoliticaEnlaces)
		pr.With(h.verifier.RequireRole("owner", "admin")).Put("/api/v1/link-policy", h.guardarPoliticaEnlaces)

		// Canjear un enlace de editor o comentarista (ADR 0052), con sesión.
		pr.Post("/api/v1/share/redeem", h.redeemShare)

		// La papelera de quien pide (papelera.go): restaurar y vaciar, atada a la de
		// Drive.
		pr.Get("/api/v1/trash", h.listarPapelera)
		pr.Delete("/api/v1/trash", h.vaciarPapelera)
		pr.Post("/api/v1/trash/{id}/restore", h.restaurarDePapelera)
		pr.Delete("/api/v1/trash/{id}", h.purgarDePapelera)

		pr.Route("/api/v1/documents", func(dr chi.Router) {
			dr.Get("/", h.listDocuments)
			dr.Post("/", h.createDocument)

			dr.Route("/{id}", func(idr chi.Router) {
				idr.Use(h.enAnfitrion)
				idr.Get("/", h.getDocument)
				idr.Get("/call-access", h.documentCallAccess)
				idr.Patch("/", h.renameDocument)
				idr.Delete("/", h.deleteDocument)
				idr.Put("/star", h.starDocument)
				idr.Post("/theme", h.aplicarTema)

				// Estado y colaboración.
				idr.Get("/state", h.getState)
				idr.Post("/ops", h.postOps)
				idr.Get("/ops", h.getOps)
				idr.Get("/ws", h.collabWS)
				idr.Get("/audit", h.auditoriaOps) // solo el propietario (auditoria.go)
				idr.Put("/presence", h.putPresence)

				// Versionado.
				idr.Get("/versions", h.listVersions)
				idr.Post("/versions", h.createVersion)
				idr.Post("/versions/{versionId}/restore", h.restoreVersion)

				// Comentarios.
				idr.Get("/comments", h.listComments)
				idr.Post("/comments", h.createComment)
				idr.Post("/comments/import", h.importComments)
				idr.Put("/comments/{commentId}/resolve", h.resolveComment)
				idr.Delete("/comments/{commentId}", h.deleteComment)

				// Permisos y compartición.
				idr.Get("/permissions", h.listPermissions)
				idr.Put("/permissions", h.upsertPermission)
				idr.Delete("/permissions", h.revokePermission)
				idr.Get("/shares", h.listShares)
				idr.Post("/shares", h.createShare)
				idr.Delete("/shares/{linkId}", h.revokeShare)
				idr.Put("/shares/{linkId}/password", h.setSharePassword)

				// Pedir acceso de edición desde la franja de solo lectura, y
				// concederlo o rechazarlo (peticiones.go).
				idr.Post("/edit-requests", h.pedirEdicion)
				idr.Get("/edit-requests", h.listarPeticiones)
				idr.Post("/edit-requests/{sub}/grant", h.concederPeticion)
				idr.Delete("/edit-requests/{sub}", h.rechazarPeticion)

				// Vistas de filtro personales de Calcus (vistas_filtro.go): de quien llama.
				idr.Get("/script-runs", h.auditoriaScriptsDoc)
				idr.Get("/filter-views", h.getVistasFiltro)
				idr.Put("/filter-views", h.putVistasFiltro)

				// Exports (OOXML/ODF/PDF/HTML/MD) materializados en S3.
				idr.Get("/exports", h.listExports)
				idr.Post("/exports", h.createExport)
				idr.Get("/exports/{blobId}", h.downloadExport)
			})
		})

		// Conector de datos externos (Power Query para Sheets): CRUD de conexiones
		// y consultas + execute/refresh. Tenant-scoped (RLS). Ver connectors.go.
		pr.Route("/api/v1/connectors", func(cr chi.Router) {
			cr.Get("/", h.listConnections)
			cr.Post("/", h.createConnection)
			// execute es una ruta estática: chi la prioriza sobre {connId}.
			cr.Post("/execute", h.executeConnectorQuery)

			cr.Route("/{connId}", func(idr chi.Router) {
				idr.Get("/", h.getConnection)
				idr.Patch("/", h.updateConnection)
				idr.Delete("/", h.deleteConnection)

				idr.Get("/queries", h.listQueries)
				idr.Post("/queries", h.createQuery)
				idr.Patch("/queries/{queryId}", h.updateQuery)
				idr.Delete("/queries/{queryId}", h.deleteQuery)
			})
		})
	})
}

// subjectFrom extrae el Subject de dominio de los claims del contexto.
func subjectFrom(r *http.Request) (domain.Subject, bool) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return domain.Subject{}, false
	}
	// Los grupos del tenant activo se transportan como roles/grupos de Sytue ID;
	// usamos los roles como grupos efectivos para la resolución de ACL por grupo,
	// además del grupo canónico org/<tenant> del claim de tenant.
	groups := append([]string{}, claims.Roles...)
	if claims.Tenant != "" {
		groups = append(groups, "org/"+claims.Tenant)
	}
	if esExterno(r.Context()) {
		// En la organización anfitriona de un documento (ADR 0052) no es nada más que
		// esa persona: ni sus grupos ni su organización cuentan allí.
		return domain.Subject{Sub: claims.Subject}, true
	}
	return domain.Subject{Sub: claims.Subject, Groups: groups, Tenant: claims.Tenant}, true
}

// docID extrae el path param {id}.
func docID(r *http.Request) string { return chi.URLParam(r, "id") }

// fail mapea un error de dominio al error HTTP canónico correspondiente.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrSnapshotMissing):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, domain.ErrForbidden):
		httpx.Fail(w, r, httpx.ErrForbidden)
	case errors.Is(err, domain.ErrProtegido):
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "protected",
			"Esta hoja o este libro están protegidos: solo su propietario o quien tenga la contraseña puede cambiarlo."))
	case errors.Is(err, domain.ErrClaveProteccion):
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "protection_password",
			"La contraseña de protección no es correcta."))
	case errors.Is(err, domain.ErrShareExpired):
		httpx.Fail(w, r, httpx.NewError(http.StatusGone, "share_expired", "This share link has expired or been revoked."))
	case errors.Is(err, domain.ErrClaveDeEnlace):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "share_password_required", "This link requires a password."))
	case errors.Is(err, domain.ErrClaveDeEnlaceInvalida):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("share_password_invalid").
			WithMessage("The link password must be between 8 and 128 characters."))
	case errors.Is(err, domain.ErrCaducidadExcedida):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("share_expiry_too_far").
			WithMessage("The expiry date exceeds the maximum allowed by your organization."))
	case errors.Is(err, domain.ErrCaducidadPasada):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("share_expiry_past").
			WithMessage("The expiry date must be in the future."))
	case errors.Is(err, domain.ErrTechoInvalido):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("max_days must be between 0 and 3650."))
	case errors.Is(err, domain.ErrInvalidKind), errors.Is(err, domain.ErrInvalidRole),
		errors.Is(err, domain.ErrInvalidFormat), errors.Is(err, domain.ErrInvalidOp),
		errors.Is(err, domain.ErrTitleRequired):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(userMessage(err)))
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "already_restored",
			"You already have another document open from the same file."))
	case errors.Is(err, domain.ErrStaleSeq):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("Version conflict; reload the document."))
	case errors.Is(err, tenancy.ErrNoTenant):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The token has no tenant."))
	default:
		httpx.Fail(w, r, err)
	}
}

// userMessage devuelve un mensaje legible para errores de validación de dominio.
func userMessage(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidKind):
		return "Invalid document kind (document|spreadsheet|presentation)."
	case errors.Is(err, domain.ErrInvalidRole):
		return "Invalid role (viewer|commenter|editor)."
	case errors.Is(err, domain.ErrInvalidFormat):
		return "Invalid export format."
	case errors.Is(err, domain.ErrInvalidOp):
		return "Invalid editing operation."
	case errors.Is(err, domain.ErrTitleRequired):
		return "A title is required."
	default:
		return "Invalid request."
	}
}

// parseExpiry parsea una fecha de expiración opcional (RFC3339).
func parseExpiry(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // sin fecha: expiración opcional ausente (contrato documentado).
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// hostsWopi resuelve la lista de hosts admitidos.
//
// Sin lista explícita se admite SOLO el Drive de esta suite, derivado de la URL
// pública de Docs: si el editor vive en docs.sytue.eu, su Drive es
// drive.sytue.eu. Un valor por defecto abierto habría sido la forma cómoda de
// dejar esto mal configurado para siempre.
func hostsWopi(d Deps) []string {
	if len(d.WopiHosts) > 0 {
		return d.WopiHosts
	}
	u, err := url.Parse(d.PublicURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	// `docs.<dominio>` → `drive.<dominio>`. Si la URL pública no empieza por
	// «docs.», no hay nada que deducir: con un `sytue.eu` a secas, cortar por el
	// primer punto daba `drive.eu` —un dominio de otro— y lo daba por admitido.
	host := strings.ToLower(u.Hostname())
	if !strings.HasPrefix(host, "docs.") {
		return nil
	}
	return []string{"drive." + strings.TrimPrefix(host, "docs.")}
}
