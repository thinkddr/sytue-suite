// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package syerphttp implementa la superficie HTTP de SyERP.
//
// UNA API para los treinta y seis modelos: `/api/v1/{modelo}` sirve a contactos,
// facturas, tickets y vehículos con el mismo código, porque el modelo dice qué
// campos tiene y `domain` los valida. Añadir un módulo no añade rutas.
//
// Lo que sí tiene superficie propia es el libro diario, porque tiene reglas
// propias: se asienta, no se edita, y se anula con otro asiento.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS). Aislamiento por
// organización vía RLS (el tenant sale de los claims).
package syerphttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
	"git.sytue.eu/sytue/one/services/syerp/internal/web"
)

// Deps son las dependencias del router HTTP de SyERP.
type Deps struct {
	Cfg      config.Config
	Store    *store.Store
	Catalogo *catalogo.Catalogo
	Verifier *authn.Verifier
	// Cifra sella los secretos de firma de los webhooks. Nula en un despliegue
	// sin SECRET_ENC_KEY: entonces no se puede configurar ningún webhook, que es
	// mejor que guardarlo en claro.
	Cifra *keycrypto.Cipher
	// Pasarela es el cliente del Integration Gateway (egress a terceros).
	Pasarela *conectores.Pasarela
	// Obligaciones es el calendario fiscal, contable y laboral por país.
	Obligaciones *obligaciones.Catalogo
	// IA es el cliente de Sytue AI. Nulo o sin base = sin funciones de IA.
	IA *asistente.Cliente
	// Web comprueba en internet lo que propone Sytue AI (VIES, webs, correos).
	Web *web.Comprobador
	// Verifactu remite los registros de facturación a la AEAT. Nulo = sin
	// certificado: nadie puede activar VERI*FACTU.
	Verifactu     *verifactu.Cliente
	ReadyCheckers []httpx.Checker
}

// Plazos por petición. Van en cada grupo de rutas y no en el router porque un
// plazo global no se puede alargar para una sola ruta: proponer un informe con
// el modelo de calidad tardó 23 s en una petición real (2026-09-15), y con esa
// cifra 30 s no dan margen.
const (
	plazoPeticion  = 30 * time.Second
	plazoPropuesta = 100 * time.Second
)

// NewRouter construye el router completo de SyERP.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{
		store: d.Store, cat: d.Catalogo, cfg: d.Cfg,
		cifra: d.Cifra, pasarela: d.Pasarela, obligaciones: d.Obligaciones, ia: d.IA, web: d.Web,
		verifactu: d.Verifactu,
		hub:       hubnotify.New(d.Cfg.HubBaseURL, d.Cfg.HubInternalSecret),
	}

	// ponytail: cubo en memoria, o sea POR RÉPLICA. Este servicio no tiene
	// ninguna superficie sin sesión, así que el tope solo frena bucles de
	// clientes propios; si hiciera falta que fuera del servicio entero,
	// pg.RateLimiter ya lo comparte por Postgres.
	limitador := httpx.NewTokenBucket(50, 100)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		// SyERP es de organizaciones: una cuenta personal —una persona cuya
		// organización es su correo— no lo tiene, y la verja NO puede ser el rol:
		// esa persona es `owner` de su propia organización personal.
		api.Use(d.Verifier.RequireOrgKind("org"))
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: limitador, KeyFunc: authn.RateKey}))
		api.Use(conPermisos)

		largas := api.With(middleware.Timeout(plazoPropuesta))
		largas.Post("/ia/informe", h.ProponerInforme)
		largas.Post("/ia/acciones", h.ProponerAcciones)
		largas.With(h.exigir("contabilidad", permisos.Usuario)).Post("/banco/pistas", h.PistasBanco)
		api = api.With(middleware.Timeout(plazoPeticion))

		// Qué puede hacer quien pide en cada módulo, y la administración de esos
		// permisos (Sytue Admin → ERP).
		api.Get("/mis-permisos", h.MisPermisos)
		api.With(d.Verifier.RequireRole("owner", "admin")).Get("/admin/permisos", h.Permisos)
		api.With(d.Verifier.RequireRole("owner", "admin")).Put("/admin/permisos/{sub}", h.GuardarPermisos)
		// Pedir acceso de edición a un módulo en lectura, y la respuesta de la
		// administración (peticiones.go).
		api.Post("/modulos/{modulo}/edit-requests", h.PedirEdicion)
		api.With(d.Verifier.RequireRole("owner", "admin")).Get("/modulos/{modulo}/edit-requests", h.ListarPeticiones)
		api.With(d.Verifier.RequireRole("owner", "admin")).Post("/modulos/{modulo}/edit-requests/{sub}/grant", h.ConcederPeticion)
		api.With(d.Verifier.RequireRole("owner", "admin")).Delete("/modulos/{modulo}/edit-requests/{sub}", h.RechazarPeticion)

		// Tareas con vencimiento y documentos adjuntos, sobre cualquier registro.
		api.Get("/actividades", h.Actividades)
		api.Get("/equipo", h.Equipo) // el selector de responsable de una tarea
		api.Post("/actividades", h.CrearActividad)
		api.Post("/actividades/{id}/hecha", h.CerrarActividad("hecha"))
		api.Post("/actividades/{id}/cancelar", h.CerrarActividad("cancelada"))
		api.Get("/adjuntos", h.Adjuntos)
		api.Post("/adjuntos", h.SubirAdjunto)
		api.Get("/adjuntos/{id}", h.DescargarAdjunto)
		api.Delete("/adjuntos/{id}", h.BorrarAdjunto)
		api.Get("/productos/imagenes", h.ImagenesProductos)
		api.Post("/ia/acciones/crear", h.CrearAcciones)

		// El catálogo traducido: lo que la interfaz necesita para pintarse
		// entera. Es la ÚNICA fuente de rótulos del producto.
		api.Get("/catalogo", h.Catalogo)
		// Resumen operativo y métricas del dashboard de inicio.
		api.Get("/resumen", h.Resumen)
		// Búsqueda global y actividad reciente para el Sytue Hub.
		api.Get("/search", h.Buscar)

		// Encender y apagar módulos es configuración de la organización.
		api.With(d.Verifier.RequireRole("owner", "admin")).
			Post("/modulos/{modulo}", h.ActivarModulo)

		// El perfil de la organización y su calendario de obligaciones. Escribir
		// el perfil es configuración: solo administración.
		api.Get("/organizacion", h.Organizacion)
		api.Get("/organizacion/paises", h.Paises)
		api.With(d.Verifier.RequireRole("owner", "admin")).
			Put("/organizacion", h.GuardarOrganizacion)
		api.Get("/organizacion/requisitos", h.RequisitosFactura)
		obl := api.With(h.exigir(permisos.Obligaciones, permisos.Lectura))
		obl.Get("/obligaciones", h.Obligaciones)
		obl.Get("/obligaciones/ics", h.ObligacionesICS)
		obl.Get("/obligaciones/calculo", h.ObligacionCalculo)
		obl.Get("/obligaciones/proceso", h.ProcesoObligacion)
		obl.Get("/obligaciones/proceso/pendientes", h.PendientesProcesoObligacion)
		obl.Post("/obligaciones/proceso", h.ProcesoObligacion)

		// VERI*FACTU: el estado, activarlo (configuración, solo administración), la
		// declaración responsable y la exportación de todos los registros.
		api.With(h.exigir("facturacion", permisos.Lectura)).Get("/verifactu", h.Verifactu)
		api.With(d.Verifier.RequireRole("owner", "admin")).Put("/verifactu/certificado", h.GuardarCertificadoVerifactu)
		api.With(d.Verifier.RequireRole("owner", "admin")).Post("/verifactu/activar", h.ActivarVerifactu)
		api.Get("/verifactu/declaracion", h.DeclaracionVerifactu)
		api.With(h.exigir("facturacion", permisos.Lectura)).Get("/verifactu/exportar", h.ExportarVerifactu)

		// El banco como tareas: cada movimiento sin factura ni explicación. El
		// extracto entra por `/linea_extracto/importar`, que reconoce el Norma 43.
		api.With(h.exigir("contabilidad", permisos.Lectura)).Get("/banco", h.Banco)
		// Conciliar es un VERBO propio: quien mete los movimientos no tiene por
		// qué ser quien los cuadra contra las facturas (separación de funciones).
		// Y como justificar un movimiento ESCRIBE su asiento en el libro, pide
		// también `asentar` (NEG-12).
		banco := api.With(h.exigir("contabilidad", permisos.Usuario),
			h.exigirVerboContable(permisos.Conciliar), h.exigirVerboContable(permisos.Asentar))
		banco.Post("/banco/{id}/conciliar", h.ConciliarMovimiento)
		banco.Post("/banco/{id}/explicar", h.ExplicarMovimiento)
		banco.Post("/banco/{id}/reabrir", h.ReabrirMovimiento)

		// Libro diario. Rutas LITERALES antes del comodín de modelo: si no,
		// "libro" y "balance" se tomarían por nombres de modelo y darían 404.
		api.With(h.exigir("contabilidad", permisos.Lectura)).Get("/libro", h.Libro)
		api.With(h.exigir("contabilidad", permisos.Lectura)).Get("/balance", h.Balance)
		api.With(h.exigir("contabilidad", permisos.Administrador),
			h.exigirVerboContable(permisos.Asentar)).
			Post("/libro/{id}/anular", h.AnularAsiento)

		// Cerrar el ejercicio y reabrirlo. Verlo va con leer el libro; cerrar y
		// reabrir piden `administrar`: no es contabilidad del día a día, es
		// declarar que un período ya no se toca (o volver a abrirlo, que es
		// excepcional y exige motivo).
		api.With(h.exigir("contabilidad", permisos.Lectura)).Get("/contabilidad/cierre", h.Cierre)
		cierre := api.With(h.exigir("contabilidad", permisos.Administrador),
			h.exigirVerboContable(permisos.Administrar))
		cierre.Post("/contabilidad/cierre", h.Cerrar)
		cierre.Post("/contabilidad/reapertura", h.Reabrir)

		// Conectores: configurarlos e importar es administración.
		api.With(d.Verifier.RequireRole("owner", "admin")).Group(func(adm chi.Router) {
			adm.Get("/conexiones", h.ListarConexiones)
			adm.Post("/conexiones/{proveedor}", h.GuardarConexion)
			adm.Post("/importar/salesforce", h.ImportarSalesforce)
			adm.Post("/importar/odoo", h.ImportarOdoo)
			adm.Post("/importar/business-central", h.ImportarBusinessCentral)
			adm.Post("/importar/holded", h.ImportarHolded)
		})

		// Aprobaciones: a partir del umbral del módulo hace falta una segunda
		// persona. Ver la cola la ve cualquiera del ERP —no enseña más que
		// importes de documentos que ya se pueden ver—; resolverlas pide el
		// verbo `aprobar`, y los umbrales, `administrar`.
		api.Get("/aprobaciones", h.Aprobaciones)
		aprob := api.With(h.exigirVerboContable(permisos.Aprobar))
		aprob.Post("/aprobaciones/{id}/aprobar", h.ResolverAprobacion("aprobada"))
		aprob.Post("/aprobaciones/{id}/rechazar", h.ResolverAprobacion("rechazada"))
		umbrales := api.With(h.exigirVerboContable(permisos.Administrar))
		umbrales.Get("/admin/umbrales", h.Umbrales)
		umbrales.Put("/admin/umbrales/{modulo}", h.GuardarUmbral)

		// Las importaciones como LOTES, con marcha atrás. Rutas literales, antes
		// del comodín de modelo. Revertir pide `administrar` en contabilidad
		// —el módulo que manda en el ERP— porque deshacer un lote toca miles de
		// registros de cualquier módulo de una vez.
		api.Get("/importaciones", h.Importaciones)
		api.With(h.exigirVerboContable(permisos.Administrar)).
			Post("/importaciones/{id}/revertir", h.RevertirImportacion)

		// Sytue AI: informes pedidos en una frase (tableros y Excel), los tableros
		// guardados y, dentro de cada modelo, rellenar una ficha con un texto.
		// Rutas LITERALES: van antes que el comodín de modelo por lo mismo que el libro.
		api.Post("/informes/datos", h.DatosInforme)
		api.Post("/informes/excel", h.ExcelInforme)
		api.Get("/tableros", h.Tableros)
		api.Post("/tableros", h.GuardarTablero)
		api.Delete("/tableros/{id}", h.BorrarTablero)

		api.Post("/tpv/cobrar", h.CobrarTPV)
		api.Post("/tpv/presupuesto", h.PresupuestoTPV)

		// El CRUD genérico, una vez, para todos los modelos.
		api.Route("/{modelo}", func(mod chi.Router) {
			mod.Use(h.exigirModelo)
			mod.Get("/", h.Listar)
			mod.Get("/estadisticas", h.Estadisticas)
			mod.Post("/", h.Crear)
			// La lista en CSV. Que "export" no se tome por un identificador NO
			// depende de este orden: chi recorre los hijos de un nodo por tipo y
			// los estáticos van antes que los de parámetro (ntStatic < ntParam,
			// tree.go), así que /contacto/export entra aquí aunque se registrara
			// después de /{id}. Está escrito arriba por legibilidad, y lo
			// comprueba `TestExportarNoSeConfundeConUnID` contra ESTE router.
			mod.Get("/export", h.Exportar)
			// La entrada por CSV: el mismo fichero que sale por /export vuelve a
			// entrar, y con él el de cualquier otro ERP. Escribe miles de filas
			// de golpe, así que pide `administrar` y no el nivel de escritura.
			mod.With(h.exigirVerbo(permisos.Administrar)).Post("/importar", h.Importar)
			mod.Post("/lote", h.Lote)
			mod.Get("/{id}", h.Obtener)
			mod.Put("/{id}", h.Actualizar)
			mod.Delete("/{id}", h.Borrar)
			mod.Post("/{id}/estado", h.CambiarEstado)
			mod.Post("/{id}/restaurar", h.Restaurar)
			mod.Post("/{id}/duplicar", h.Duplicar)
			mod.Post("/{id}/convertir", h.ConvertirVenta)
			// Confirmar un documento contable: lo lleva al libro. Verbo propio —
			// quien crea la factura no tiene por qué ser quien la asienta.
			mod.With(h.exigirVerbo(permisos.Asentar)).Post("/{id}/asentar", h.Asentar)
			// Pedir que otra persona apruebe este documento. Lo pide quien lo
			// va a asentar, así que basta con el verbo de crear.
			mod.Post("/{id}/aprobacion", h.PedirAprobacion)
			mod.Get("/{id}/asientos", h.AsientosDe)
			// El historial del registro, como el de Odoo: cambios y notas.
			mod.Get("/{id}/historial", h.Historial)
			mod.Post("/{id}/historial", h.Anotar)
			// La factura como UBL 2.1: lo que entiende cualquier otro ERP.
			mod.Get("/{id}/ubl", h.UBL)
			mod.Get("/{id}/facturae", h.Facturae)
			// La factura para el cliente, con su QR tributario, y sus registros de
			// VERI*FACTU con lo que contestó la AEAT.
			mod.Get("/{id}/imprimir", h.Imprimir)
			mod.Get("/{id}/verifactu", h.RegistrosFacturaVerifactu)
			mod.Post("/{id}/verifactu/subsanar", h.SubsanarVerifactu)
			mod.Post("/ia/rellenar", h.RellenarFicha)
			mod.Post("/asistencia/leer", h.LeerAsistenciaFactura)
		})
	})

	// Webhook de Stripe: SIN sesión, porque Stripe no puede presentar un token de
	// Sytue ID. Su autenticación es la firma HMAC del cuerpo.
	//
	// Su límite va por IP y COMPARTIDO entre réplicas (pg.RateLimiter): es la
	// única superficie de este servicio que puede visitar cualquiera, y un cubo
	// en memoria multiplicaría el tope real por el número de pods justo cuando
	// hay carga.
	r.Group(func(pub chi.Router) {
		pub.Use(middleware.Timeout(plazoPeticion))
		pub.Use(httpx.RateLimit(httpx.RateLimitConfig{
			Limiter: pg.NewRateLimiter(d.Store.DB(), pg.RateLimiterConfig{PerSecond: 5, Burst: 50}),
		}))
		pub.Post("/webhooks/stripe/{org}", h.WebhookStripe)
	})

	if d.Cfg.PedidosTiendaSecret != "" {
		r.With(httpx.SoloDesdeDentro(d.Cfg.PedidosTiendaSecret)).
			Post("/internal/pedidos-tienda", h.handlePedidosTienda)
		r.With(httpx.SoloDesdeDentro(d.Cfg.PedidosTiendaSecret)).
			Get("/internal/pedidos-tienda/{pedido_id}/factura", h.handlePedidosTiendaFactura)
	}

	return r
}
