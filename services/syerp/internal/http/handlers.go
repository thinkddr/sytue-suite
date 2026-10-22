// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
	"git.sytue.eu/sytue/one/services/syerp/internal/web"
)

// Handlers agrupa los manejadores HTTP de SyERP.
type Handlers struct {
	store    *store.Store
	cat      *catalogo.Catalogo
	cfg      config.Config
	cifra    *keycrypto.Cipher
	pasarela *conectores.Pasarela
	// obligaciones es el calendario por país; nulo solo en tests que no lo usan.
	obligaciones *obligaciones.Catalogo
	// ia es el cliente de Sytue AI; inactivo si el despliegue no tiene SYTUE_AI_URL.
	ia *asistente.Cliente
	// web comprueba en internet lo que propone Sytue AI; nulo = sin comprobar.
	web *web.Comprobador
	// verifactu remite los registros a la AEAT; nulo = sin certificado de respaldo.
	verifactu *verifactu.Cliente
	// declaracionFecha sobreescribe verifactu.DeclaracionFecha en tests de producción.
	declaracionFecha string
	// hub deja en la campana de quien la tiene una tarea que no se ha puesto él.
	hub *hubnotify.Client
}

var (
	errModeloDesconocido = httpx.NewError(http.StatusNotFound, "modelo_desconocido",
		"That model does not exist in the catalogue.")
	errEstadoInvalido = httpx.NewError(http.StatusUnprocessableEntity, "estado_invalido",
		"That status is not part of the document's workflow.")
	errNoContable = httpx.NewError(http.StatusUnprocessableEntity, "no_contable",
		"This model does not post to the ledger.")
	errYaAsentado = httpx.NewError(http.StatusConflict, "ya_asentado",
		"This document has already been posted to the ledger.")
	errRefDuplicada = httpx.NewError(http.StatusConflict, "referencia_duplicada",
		"A document with that reference already exists.")
	errAccionInvalida = httpx.NewError(http.StatusBadRequest, "accion_invalida",
		"Unsupported batch action (supported: estado, borrar, restaurar).")
	errSinIDs = httpx.NewError(http.StatusBadRequest, "sin_ids",
		"At least one record id is required.")
)

// modeloDe resuelve el modelo de la ruta contra el catálogo. Devuelve nil y ya
// ha respondido si no existe.
func (h *Handlers) modeloDe(w http.ResponseWriter, r *http.Request) *catalogo.Modelo {
	m, ok := h.cat.Modelo(chi.URLParam(r, "modelo"))
	if !ok {
		httpx.Fail(w, r, errModeloDesconocido)
		return nil
	}
	return m
}

func sub(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// --- Catálogo y módulos -----------------------------------------------------

type respuestaCatalogo struct {
	// Ya serializado por el catálogo al arrancar: aquí solo se copia. Ver
	// `Catalogo.vistas`.
	Modulos json.RawMessage `json:"modulos"`
	// Activos son los módulos encendidos por la organización. Nulo cuando aún no
	// ha elegido ninguno: entonces la interfaz enseña la pantalla de bienvenida
	// en vez de veinticuatro entradas de golpe, que es el primer motivo por el
	// que un ERP se percibe como inmanejable.
	Activos []string `json:"activos"`
	Idioma  string   `json:"idioma"`
}

// Catalogo devuelve el catálogo entero ya traducido al idioma de la petición.
//
// Es la PRIMERA petición de cada carga de página: hasta que llega, la interfaz
// no sabe ni qué poner en la barra lateral. Por eso lleva dos cosas que el resto
// de rutas no necesitan:
//
//   - Los módulos vienen ya serializados del catálogo (se calculan al arrancar,
//     una vez por idioma). Aquí no se traduce ni se codifica nada: se copian
//     32 KB de bytes que no cambian.
//   - ETag. La respuesta solo cambia si cambia la imagen o si la organización
//     enciende o apaga un módulo, así que una navegación normal contesta 304 y
//     no baja un solo byte del catálogo. `private` porque `activos` es de esta
//     organización y no de otra.
func (h *Handlers) Catalogo(w http.ResponseWriter, r *http.Request) {
	idioma := httpx.LanguageFromRequest(r)
	activos, err := h.store.ModulosActivos(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// El sello mezcla la huella del catálogo con los módulos encendidos: apagar
	// uno tiene que invalidar la copia del navegador en el acto.
	suma := sha256.Sum256([]byte(h.cat.Sello(idioma) + "|" + strings.Join(activos, ",")))
	etag := `"` + hex.EncodeToString(suma[:8]) + `"`
	w.Header().Set("ETag", etag)
	// Sin `max-age`: se revalida siempre, pero la revalidación cuesta un 304 de
	// cabeceras en vez de 32 KB. Un `max-age` serviría catálogo viejo justo
	// después de encender un módulo, que es cuando más se mira.
	w.Header().Set("Cache-Control", "private, no-cache")
	if coincideETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	httpx.JSON(w, r, http.StatusOK, respuestaCatalogo{
		Modulos: h.cat.VistaJSON(idioma), Activos: activos, Idioma: idioma,
	})
}

// coincideETag compara la cabecera If-None-Match con el sello. Admite lista y
// la marca `W/` de validador débil, que es lo que añaden algunos proxies.
func coincideETag(cabecera, etag string) bool {
	for _, candidato := range strings.Split(cabecera, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidato), "W/") == etag {
			return true
		}
	}
	return false
}

type cuerpoModulo struct {
	Activo bool `json:"activo"`
}

// ActivarModulo enciende o apaga un módulo para la organización.
// Al encender un módulo, activa automáticamente sus dependencias transitivas.
func (h *Handlers) ActivarModulo(w http.ResponseWriter, r *http.Request) {
	modulo := chi.URLParam(r, "modulo")
	conocido := false
	for _, m := range h.cat.Modulos() {
		if m.Clave == modulo {
			conocido = true
			break
		}
	}
	if !conocido {
		httpx.Fail(w, r, errModeloDesconocido)
		return
	}
	var cuerpo cuerpoModulo
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var activadosAuto []string
	var desactivadosAuto []string
	if cuerpo.Activo {
		transitivas := h.cat.DependenciasTransitivas(modulo)
		for _, dep := range transitivas {
			if err := h.store.ActivarModulo(r.Context(), dep, true); err != nil {
				httpx.Fail(w, r, err)
				return
			}
			activadosAuto = append(activadosAuto, dep)
		}
		if err := h.store.ActivarModulo(r.Context(), modulo, true); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else {
		// Al desactivar un módulo, desactivar también todos los módulos que dependen de él
		// o que tienen dependencias bidireccionales / interdependientes con él.
		dependientes := h.cat.DependientesTransitivas(modulo)
		activosPrevios, err := h.store.ModulosActivos(r.Context())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		activosSet := map[string]bool{}
		for _, a := range activosPrevios {
			activosSet[a] = true
		}
		for _, dep := range dependientes {
			if activosSet[dep] {
				if err := h.store.ActivarModulo(r.Context(), dep, false); err != nil {
					httpx.Fail(w, r, err)
					return
				}
				desactivadosAuto = append(desactivadosAuto, dep)
			}
		}
		if err := h.store.ActivarModulo(r.Context(), modulo, false); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}

	activos, err := h.store.ModulosActivos(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"activos":      activos,
		"activados":    activadosAuto,
		"desactivados": desactivadosAuto,
	})
}

// Buscar atiende GET /api/v1/search?q=...&limit=...
// Permite al Hub federar la búsqueda global y consultar la actividad reciente de SyERP.
func (h *Handlers) Buscar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limite := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limite = l
	} else if l, err := strconv.Atoi(r.URL.Query().Get("limite")); err == nil && l > 0 {
		limite = l
	}

	regs, err := h.store.Buscar(r.Context(), q, limite)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La búsqueda del Hub no enseña lo que la persona no puede abrir.
	visibles := make([]store.Registro, 0, len(regs))
	for _, reg := range regs {
		if modulo := h.moduloDe(reg.Modelo); modulo != "" && h.puede(r.Context(), modulo, permisos.Lectura) {
			visibles = append(visibles, reg)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": visibles})
}

// --- CRUD genérico ----------------------------------------------------------

// filtroDe construye el filtro de listado a partir de la query y lo comprueba
// contra el modelo. Devuelve ok=false y YA HA RESPONDIDO si algún criterio no
// vale.
//
// Lo comparten la lista y la exportación a propósito: lo que se descarga tiene
// que ser exactamente lo que se está viendo en pantalla, y eso se garantiza
// leyendo los filtros en un solo sitio, no escribiéndolos parecido en dos.
func filtroDe(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) (store.Filtro, bool) {
	q := r.URL.Query()
	f := store.Filtro{
		Modelo: m.Clave,
		Estado: q.Get("estado"),
		Texto:  q.Get("q"),
		Cursor: q.Get("cursor"),
	}
	if n, err := strconv.Atoi(q.Get("limite")); err == nil {
		f.Limite = n
	}
	if f.Estado != "" && !domain.EstadoValido(m, f.Estado) {
		httpx.Fail(w, r, errEstadoInvalido)
		return f, false
	}
	// Filtro por campo declarado. Se comprueba contra el modelo ANTES de llegar
	// al store: así el parámetro que entra en la consulta solo puede ser una de
	// las claves del catálogo.
	if campo := q.Get("campo"); campo != "" {
		if !tieneCampo(m, campo) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("unknown field: "+campo))
			return f, false
		}
		f.Campo, f.Valor = campo, q.Get("valor")
	}
	// Orden. Se resuelve contra el MODELO, y lo que llega al store es la clave
	// del catálogo más el tipo que ese catálogo declara: el store no recibe nunca
	// texto libre que vaya a acabar dentro de un ORDER BY.
	if orden := q.Get("orden"); orden != "" {
		switch orden {
		case "creado", "etiqueta":
		case "ref":
			if m.Secuencia == "" {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("this model has no reference"))
				return f, false
			}
		case "estado":
			if len(m.Estados) == 0 {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("this model has no status"))
				return f, false
			}
		default:
			campo, ok := campoDe(m, orden)
			if !ok {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("unknown sort field: "+orden))
				return f, false
			}
			f.OrdenTipo = campo.Tipo
		}
		f.Orden = orden
	}
	// Solo «asc»: cualquier otra cosa es el descendente de siempre, que es lo que
	// espera quien abre una lista —lo último, arriba—.
	f.Ascendente = q.Get("dir") == "asc"
	f.Papelera = q.Get("papelera") == "1"
	return f, true
}

// Listar devuelve una página de registros del modelo.
func (h *Handlers) Listar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	f, ok := filtroDe(w, r, m)
	if !ok {
		return
	}

	pag, err := h.store.Listar(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Nombres legibles de las relaciones de esta página, en una consulta.
	if ids := idsReferenciados(m, pag.Registros); len(ids) > 0 {
		if pag.Etiquetas, err = h.store.ResolverEtiquetas(r.Context(), ids); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.JSON(w, r, http.StatusOK, pag)
}

type cuerpoRegistro struct {
	Data json.RawMessage `json:"data"`
}

// Crear inserta un registro nuevo.
func (h *Handlers) Crear(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var cuerpo cuerpoRegistro
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	data, etiqueta, err := domain.Validar(m, cuerpo.Data)
	if err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	reg, err := h.store.Crear(r.Context(), m, data, etiqueta, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, reg)
}

// Obtener devuelve un registro.
func (h *Handlers) Obtener(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

// Actualizar reemplaza los datos de un registro.
func (h *Handlers) Actualizar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var cuerpo cuerpoRegistro
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	data, etiqueta, err := domain.Validar(m, cuerpo.Data)
	if err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	reg, err := h.store.Actualizar(r.Context(), m, chi.URLParam(r, "id"), data, etiqueta)
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

// verboDelEstado es el gesto que pide llevar un documento CONTABLE a ese estado.
// Cambiar el estado es otra puerta a lo mismo que `/asentar`, `/aprobaciones` y el
// cobro: emitir una factura (con su registro de VERI*FACTU), anularla, darla por
// pagada o aprobar un gasto. Sin esto, quien solo tenía el verbo de crear se
// saltaba la separación de funciones cambiando el estado a mano (NEG-11).
func verboDelEstado(m *catalogo.Modelo, estado string) string {
	if m.Contable == nil {
		return ""
	}
	switch estado {
	case "confirmado", "anulado":
		return permisos.Asentar
	case "pagado":
		return permisos.Pagar
	case "aprobado", "rechazado":
		return permisos.Aprobar
	}
	return ""
}

type cuerpoEstado struct {
	Estado string `json:"estado"`
}

// CambiarEstado mueve el documento dentro de su flujo.
func (h *Handlers) CambiarEstado(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var cuerpo cuerpoEstado
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !domain.EstadoValido(m, cuerpo.Estado) {
		httpx.Fail(w, r, errEstadoInvalido)
		return
	}
	if v := verboDelEstado(m, cuerpo.Estado); v != "" && !h.puedeVerbo(r.Context(), m.Modulo, v) {
		httpx.Fail(w, r, errSinVerbo(v))
		return
	}
	reg, err := h.store.CambiarEstado(r.Context(), m, chi.URLParam(r, "id"), cuerpo.Estado, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

// Restaurar saca el registro de la papelera.
//
// Existe porque borrar era de ida sola desde la interfaz: el registro se marcaba
// y desaparecía de todas las listas sin forma de recuperarlo salvo entrando en la
// base. Una papelera de la que no se puede sacar nada es peor que un borrado
// honesto, porque además da una sensación de reversibilidad que no era cierta.
func (h *Handlers) Restaurar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	reg, err := h.store.Restaurar(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, reg)
}

// Borrar manda el registro a la papelera.
func (h *Handlers) Borrar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if err := h.store.Borrar(r.Context(), m.Clave, chi.URLParam(r, "id")); err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.NoContent(w)
}

type cuerpoLote struct {
	Accion string   `json:"accion"`
	IDs    []string `json:"ids"`
	Estado string   `json:"estado,omitempty"`
}

// Lote atiende operaciones masivas (cambio de estado o eliminación en bloque)
// en una sola transacción para el modelo de la ruta.
func (h *Handlers) Lote(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var cuerpo cuerpoLote
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(cuerpo.IDs) == 0 {
		httpx.Fail(w, r, errSinIDs)
		return
	}

	var afectadas int64
	var err error

	switch cuerpo.Accion {
	case "estado":
		if cuerpo.Estado == "" || !domain.EstadoValido(m, cuerpo.Estado) {
			httpx.Fail(w, r, errEstadoInvalido)
			return
		}
		if v := verboDelEstado(m, cuerpo.Estado); v != "" && !h.puedeVerbo(r.Context(), m.Modulo, v) {
			httpx.Fail(w, r, errSinVerbo(v))
			return
		}
		afectadas, err = h.store.LoteEstado(r.Context(), m, cuerpo.IDs, cuerpo.Estado, sub(r))
	case "borrar":
		afectadas, err = h.store.LoteBorrar(r.Context(), m.Clave, cuerpo.IDs)
	case "restaurar":
		afectadas, err = h.store.LoteRestaurar(r.Context(), m.Clave, cuerpo.IDs)
	default:
		httpx.Fail(w, r, errAccionInvalida)
		return
	}

	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"afectados": afectadas,
	})
}

// --- Libro diario -----------------------------------------------------------

// Asentar lleva el documento al libro por partida doble.
func (h *Handlers) Asentar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil {
		httpx.Fail(w, r, errNoContable)
		return
	}
	a, err := h.store.Asentar(r.Context(), m, chi.URLParam(r, "id"), sub(r))
	switch {
	case errors.Is(err, store.ErrYaAsentado):
		httpx.Fail(w, r, errYaAsentado)
	case err != nil:
		httpx.Fail(w, r, errorDeEscritura(err))
	default:
		httpx.JSON(w, r, http.StatusCreated, a)
	}
}

// AsientosDe devuelve los asientos que generó un documento.
func (h *Handlers) AsientosDe(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	as, err := h.store.AsientosDe(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"asientos": as})
}

// Historial — GET /api/v1/{modelo}/{id}/historial — lo que ha pasado con el
// registro: quién lo creó, qué cambió quién y cuándo, y las notas.
func (h *Handlers) Historial(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	entradas, err := h.store.Historial(r.Context(), m.Clave, chi.URLParam(r, "id"), 100)
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"historial": entradas})
}

// Anotar — POST /api/v1/{modelo}/{id}/historial — deja una nota en el registro.
func (h *Handlers) Anotar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	var cuerpo struct {
		Texto string `json:"texto"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := h.store.Anotar(r.Context(), m.Clave, chi.URLParam(r, "id"), sub(r), cuerpo.Texto)
	switch {
	case errors.Is(err, store.ErrNotaVacia):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The note is empty."))
	case err != nil:
		httpx.Fail(w, r, errorDeEscritura(err))
	default:
		httpx.JSON(w, r, http.StatusCreated, e)
	}
}

// Libro devuelve los asientos de un rango de fechas.
func (h *Handlers) Libro(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	desde, err := fechaOpcional(q.Get("desde"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("desde: YYYY-MM-DD"))
		return
	}
	hasta, err := fechaOpcional(q.Get("hasta"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: YYYY-MM-DD"))
		return
	}
	limite, _ := strconv.Atoi(q.Get("limite"))
	as, err := h.store.Libro(r.Context(), desde, hasta, limite)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"asientos": as})
}

// Balance devuelve el sumas y saldos por cuenta.
func (h *Handlers) Balance(w http.ResponseWriter, r *http.Request) {
	hasta, err := fechaOpcional(r.URL.Query().Get("hasta"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: YYYY-MM-DD"))
		return
	}
	saldos, err := h.store.Balance(r.Context(), hasta)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El total de los dos lados sale aquí porque es lo primero que mira quien
	// abre esta pantalla: si no coinciden, algo va muy mal.
	var debe, haber int64
	for _, s := range saldos {
		debe += s.Debe
		haber += s.Haber
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"cuentas": saldos, "total_debe": debe, "total_haber": haber,
	})
}

type cuerpoAnular struct {
	Motivo string `json:"motivo"`
}

// AnularAsiento marca un asiento como anulado (no lo borra).
func (h *Handlers) AnularAsiento(w http.ResponseWriter, r *http.Request) {
	var cuerpo cuerpoAnular
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if cuerpo.Motivo == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("motivo required"))
		return
	}
	if err := h.store.Anular(r.Context(), chi.URLParam(r, "id"), cuerpo.Motivo); err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	httpx.NoContent(w)
}

// Resumen devuelve los indicadores operativos globales del tenant para el dashboard de inicio.
func (h *Handlers) Resumen(w http.ResponseWriter, r *http.Request) {
	res, err := h.store.Resumen(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Las cifras de la portada son de facturación, contabilidad y contactos: se
	// enseñan a quien puede leer cada cosa.
	if !h.puede(r.Context(), "facturacion", permisos.Lectura) {
		res.Facturacion = store.ResumenFacturacion{}
	}
	if !h.puede(r.Context(), "contabilidad", permisos.Lectura) {
		res.Contabilidad = store.ResumenContabilidad{}
	}
	if !h.puede(r.Context(), "contactos", permisos.Lectura) {
		res.Contactos = 0
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// Estadisticas devuelve los conteos por estado y agregados de un modelo.
func (h *Handlers) Estadisticas(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	stats, err := h.store.Estadisticas(r.Context(), m)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, stats)
}

// Duplicar clona un registro existente como borrador con nueva referencia.
func (h *Handlers) Duplicar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	reg, err := h.store.Duplicar(r.Context(), m, chi.URLParam(r, "id"), sub(r))
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, reg)
}

// --- Ayudas -----------------------------------------------------------------

func tieneCampo(m *catalogo.Modelo, clave string) bool {
	_, ok := campoDe(m, clave)
	return ok
}

// campoDe busca un campo declarado por su clave.
func campoDe(m *catalogo.Modelo, clave string) (catalogo.Campo, bool) {
	for _, f := range m.Campos {
		if f.Clave == clave {
			return f, true
		}
	}
	return catalogo.Campo{}, false
}

// idsReferenciados recoge los identificadores que aparecen en los campos de
// relación de una página.
func idsReferenciados(m *catalogo.Modelo, regs []store.Registro) []string {
	vistos := map[string]bool{}
	var ids []string
	for _, f := range m.Campos {
		if f.Tipo != catalogo.TipoRelacion {
			continue
		}
		for _, r := range regs {
			if s, ok := r.Data[f.Clave].(string); ok && s != "" && !vistos[s] {
				vistos[s] = true
				ids = append(ids, s)
			}
		}
	}
	return ids
}

func fechaOpcional(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.DateOnly, s)
}

// errorDeValidacion conserva QUÉ CAMPO falló.
//
// `validation.Errors` ya agrupa los fallos por campo y `httpx.Error.Details` ya
// los serializa —el comentario de ese tipo lo dice literalmente—, pero el
// manejador los aplastaba con `WithMessage(err.Error())` y lo que llegaba al
// navegador era una frase única: «validación fallida: fecha: fecha AAAA-MM-DD;
// base: obligatorio». La interfaz solo podía pintarla entera arriba del
// formulario, así que quien rellenaba diez campos tenía que releerlos todos
// para encontrar los dos malos.
//
// Con los detalles puestos, cada mensaje va DEBAJO de su campo. No cambia el
// código HTTP ni el mensaje de respaldo: solo deja de tirar lo que ya venía
// calculado.
func errorDeValidacion(err error) error {
	e := httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error())
	if v, ok := validation.AsErrors(err); ok {
		return e.WithDetails(v)
	}
	return e
}

func errorDeLectura(err error) error {
	switch {
	case errors.Is(err, store.ErrNoEncontrado):
		return httpx.ErrNotFound
	case errors.Is(err, store.ErrPeriodoCerrado):
		return errPeriodoCerrado
	case errors.Is(err, store.ErrRequiereAprobacion):
		return errRequiereAprobacion
	case errors.Is(err, store.ErrCuatroOjos):
		return errCuatroOjos
	}
	return err
}

// El período cerrado sale en los DOS traductores: anular llega por el de
// lectura y asentar por el de escritura, y un 500 por «el ejercicio está
// cerrado» no le dice nada a quien lo intenta.

// errorDeEscritura traduce los fallos que son culpa de lo que se envió —una
// relación que no existe, una referencia repetida— para que no salgan como un
// 500. El resto sube tal cual.
func errorDeEscritura(err error) error {
	switch {
	case errors.Is(err, store.ErrNoEncontrado):
		return httpx.ErrNotFound
	case errors.Is(err, store.ErrRefDuplicada):
		return errRefDuplicada
	case errors.Is(err, store.ErrYaAsentado):
		return errYaAsentado
	case errors.Is(err, domain.ErrDatos), errors.Is(err, verifactu.ErrDatos):
		return httpx.ErrUnprocessable.Wrap(err).WithMessage(err.Error())
	case errors.Is(err, store.ErrFacturaEmitida):
		return errFacturaEmitida
	case errors.Is(err, store.ErrVerifactuPendiente):
		return errVerifactuPendiente
	case errors.Is(err, store.ErrNadaQueSubsanar):
		return errNadaQueSubsanar
	case errors.Is(err, store.ErrNIFConVerifactu):
		return errNIFConVerifactu
	case errors.Is(err, store.ErrCertificadoInvalido):
		return errCertificadoInvalido
	case errors.Is(err, store.ErrCertificadoNIFNoCoincide):
		return errCertificadoNIFNoCoincide
	case errors.Is(err, store.ErrPeriodoCerrado):
		return errPeriodoCerrado
	case errors.Is(err, store.ErrRequiereAprobacion):
		return errRequiereAprobacion
	case errors.Is(err, store.ErrModulosDesactivados):
		return httpx.ErrUnprocessable.WithMessage("activa los módulos necesarios antes de continuar")
	default:
		return err
	}
}
