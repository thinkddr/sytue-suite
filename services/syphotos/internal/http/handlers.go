// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package http sirve la API REST de Sytue Photos.
package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Handlers sirve la API REST de Sytue Photos sobre el store.
type Handlers struct {
	store   *store.Store
	cfg     config.Config
	objetos *objectstore.Client
	ingesta *service.Ingesta
	// limpieza se lleva las derivadas de lo que se purga. La comparte con el
	// worker de retención: escrito dos veces, uno de los dos se quedaría atrás.
	limpieza *service.Limpieza
	// derivando acota cuántas decodificaciones simultáneas admite el proceso.
	// Ver Handlers.generar: el límite que importa es la memoria de la réplica,
	// no la de una petición.
	derivando chan struct{}
	// hub avisa a quien recibe un álbum compartido (personas.go). nil: sin aviso.
	hub *hubnotify.Client
	// avisados frena el aviso (y la preparación) de un álbum compartido: una vez por
	// álbum y persona cada VentanaAviso (personas.go).
	avisados *hubnotify.UnaVez
	// anyadidos frena el aviso a la dueña de fotos añadidas por quien edita (edicion.go).
	anyadidos *hubnotify.UnaVez
	// drive copia al Drive de la dueña lo que añade quien edita (edicion.go). nil: no se
	// puede añadir como editora.
	drive *drive.Cliente
	// completando frena la pasada que rellena tipo y tamaño: una por persona cada
	// VentanaMetadatos (metadatos.go).
	completando *hubnotify.UnaVez
}

// sub devuelve el sujeto del token. RequireAuth garantiza que hay claims, así
// que una cadena vacía aquí solo puede venir de un token sin `sub`, que no
// autoriza nada: todas las consultas lo comparan contra owner_sub.
func sub(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// traducir mapea los errores de dominio al sobre canónico de la API.
//
// «No existe» y «no autorizado» se contestan IGUAL (404): distinguirlos deja
// enumerar qué fotos hay en la organización preguntando por identificadores.
func traducir(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return httpx.ErrNotFound.Wrap(err)
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.Wrap(err)
	default:
		return err
	}
}

func idDeRuta(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithCode("invalid_id").Wrap(err)
	}
	return id, nil
}

// ── La línea de tiempo ───────────────────────────────────────────────────────

// activoJSON es la foto tal como la ve el navegador.
//
// LAS DIMENSIONES VAN YA ORIENTADAS. En la base se guardan crudas —lo que dice
// el fichero— porque es el dato; pero quien pinta necesita el tamaño que se VE,
// y hacer que cada cliente aplique la tabla EXIF por su cuenta es garantizar
// que la web y el escritorio la apliquen distinto. Se resuelve una vez, aquí.
type activoJSON struct {
	ID     uuid.UUID `json:"id"`
	Nombre string    `json:"name"`
	Tipo   string    `json:"contentType"`
	Bytes  int64     `json:"sizeBytes"`
	// Ancho y Alto ORIENTADOS: es lo que hay que reservar en la cuadrícula.
	Ancho int `json:"width"`
	Alto  int `json:"height"`
	// Cuando ordena la línea de tiempo, y `deCaptura` dice de dónde sale. La
	// interfaz tiene que poder avisar de que una foto está colocada por su fecha
	// de subida: sin eso, una descarga de 2019 aparece entre las de hoy sin
	// explicación.
	Cuando    time.Time `json:"takenAt"`
	DeCaptura bool      `json:"takenAtFromExif"`
	Favorita  bool      `json:"favorite"`
	// Lugar solo aparece si la foto lo trae: un objeto vacío obligaría a cada
	// cliente a distinguir «sin GPS» de «en el golfo de Guinea».
	Lugar *lugarJSON `json:"location,omitempty"`
	// Descripcion es además el TEXTO ALTERNATIVO de la foto. Vacía mientras
	// nadie haya pedido describirla.
	Descripcion string   `json:"description,omitempty"`
	Etiquetas   []string `json:"tags,omitempty"`
	Camara      string   `json:"camera,omitempty"`
	Objetivo    string   `json:"lens,omitempty"`
	DriveID     string   `json:"driveFileId"`
}

// lugarJSON es dónde se hizo la foto, en grados decimales.
type lugarJSON struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func aJSON(a domain.Activo) activoJSON {
	ancho, alto := a.Vistas()
	cuando, deCaptura := a.FechaDeGaleria()
	return activoJSON{
		ID: a.ID, Nombre: a.Nombre, Tipo: a.Tipo, Bytes: a.Bytes,
		Ancho: ancho, Alto: alto,
		Cuando: cuando, DeCaptura: deCaptura, Favorita: a.Favorita,
		Camara: a.Camara, Objetivo: a.Objetivo, DriveID: a.DriveID,
		Lugar: lugarDe(a), Descripcion: a.Descripcion, Etiquetas: a.Etiquetas,
	}
}

func lugarDe(a domain.Activo) *lugarJSON {
	if a.Lat == nil || a.Lon == nil {
		return nil
	}
	return &lugarJSON{Lat: *a.Lat, Lon: *a.Lon}
}

// GET /api/v1/timeline?limit=&cursor=
//
// EL CURSOR ES OPACO para el cliente: un `<fecha>|<id>` en texto. Podría ir en
// dos parámetros, y no van, porque entonces alguien los compondría a mano y la
// paginación pasaría a depender de que acierte con la expresión de orden — que
// es exactamente la que tiene que coincidir con el índice.
func (h *Handlers) LineaDeTiempo(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`: esta ruta está montada con
	// `GetYQuery`, y por QUERY los parámetros viajan en el CUERPO —la query
	// string llega vacía—. Leyéndolos de la URL, la línea de tiempo de Fotos se quedaba sin
	// argumentos y respondía como si no le hubieran pedido nada. Pasó con el
	// buscador global del Hub, que devolvía 400 en cada pulsación.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := store.FiltroLinea{Duenyo: sub(r)}
	if q := strings.TrimSpace(params.Get("q")); q != "" {
		if len(q) > 200 {
			httpx.Fail(w, r, traducir(fmt.Errorf("%w: búsqueda de más de 200 caracteres", domain.ErrInvalidInput)))
			return
		}
		f.Texto = q
		// `tambien=` son formas alternativas de decir lo mismo, propuestas por
		// Sytue AI. Solo valen ACOMPAÑANDO a una búsqueda: sin `q`, ampliar no
		// tiene de qué partir, y aceptarlas sueltas dejaría listar la galería
		// entera con un parámetro que parece de filtro.
		for _, otro := range strings.Split(params.Get("tambien"), ",") {
			otro = strings.TrimSpace(otro)
			if otro == "" || len(otro) > 60 {
				continue
			}
			f.Tambien = append(f.Tambien, otro)
			// Tope: cada término es un patrón más en la consulta, y una URL a
			// mano con doscientos la convertiría en un recorrido caro.
			if len(f.Tambien) == 8 {
				break
			}
		}
	}
	// `favoritas=1` es la MISMA línea con un filtro: mismo orden y mismo cursor,
	// para que pasar de todas a favoritas no cambie de paginación a mitad.
	f.SoloFavoritas = params.Get("favoritas") == "1"
	if n, err := strconv.Atoi(params.Get("limit")); err == nil {
		f.Limite = n
	}
	if c := params.Get("cursor"); c != "" {
		cur, err := descifrarCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_cursor").Wrap(err))
			return
		}
		f.Desde = cur
	}

	activos, siguiente, err := h.store.LineaDeTiempo(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// Abrir la galería es el momento en que hay token de la dueña para leer de Drive.
	if f.Desde == nil {
		h.completarMetadatos(r)
	}
	salida := make([]activoJSON, 0, len(activos))
	for _, a := range activos {
		salida = append(salida, aJSON(a))
	}
	cuerpo := map[string]any{"items": salida}
	// El cursor solo aparece si hay más. Devolver siempre uno obliga al cliente
	// a pedir una página vacía para descubrir que se acabó.
	if siguiente != nil {
		cuerpo["nextCursor"] = cifrarCursor(*siguiente)
	}
	httpx.JSON(w, r, http.StatusOK, cuerpo)
}

// GET /api/v1/assets/{id}.
func (h *Handlers) Activo(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.store.Activo(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, aJSON(a))
}

// DELETE /api/v1/assets/{id}
//
// A LA PAPELERA DE SYPHOTOS, no a la de Drive. Quitar una foto de la galería no
// puede borrar el fichero de quien lo tuviera en una carpeta compartida; borrar
// el original es otra acción y se pide en Drive.
func (h *Handlers) ALaPapelera(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.ALaPapelera(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── La receta de edición ─────────────────────────────────────────────────────

// GET /api/v1/assets/{id}/recipe.
func (h *Handlers) Receta(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Se comprueba que la foto EXISTE Y ES VISIBLE antes de contestar. Sin esto,
	// pedir la receta de cualquier identificador devolvería la receta vacía con
	// un 200 alegre, que es una forma de decir «esa foto no existe» y otra de
	// decir «existe y no está editada» — indistinguibles.
	if _, err := h.store.Activo(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	rec, err := h.store.Receta(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	huella, err := rec.Huella()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"recipe": rec, "fingerprint": huella})
}

// PUT /api/v1/assets/{id}/recipe
//
// La receta se guarda ENTERA en cada guardado, no por parches. Es una lista
// ordenada y corta que el editor tiene completa en memoria; aplicar parches
// obligaría a resolver conflictos de orden entre dos pestañas, para ahorrar unos
// cientos de bytes.
func (h *Handlers) GuardarReceta(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cuerpo struct {
		Receta domain.Receta `json:"recipe"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := h.store.Activo(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	huella, err := h.store.GuardarReceta(r.Context(), id, sub(r), cuerpo.Receta)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// La huella es lo que el cliente necesita para componer la URL de la
	// derivada: devolverla evita que la recalcule y que las dos
	// implementaciones —Go y el navegador— puedan discrepar.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"fingerprint": huella})
}

// PUT /api/v1/assets/{id}/favorite  {"favorite": true|false}
//
// El cuerpo lleva el valor y no se alterna en el servidor: dos pulsaciones
// seguidas —o la misma repetida por un reintento— tienen que dejar la foto
// donde dice la interfaz, no en el estado contrario.
func (h *Handlers) MarcarFavorita(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cuerpo struct {
		Favorita bool `json:"favorite"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.MarcarFavorita(r.Context(), id, sub(r), cuerpo.Favorita); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/v1/assets/{id}/description  {"description": "...", "tags": [...]}
//
// Lo escribe la app con lo que devolvió Sytue AI, y NO este servicio llamando
// al modelo: quien tiene la sesión de quien lo pidió es el BFF, y la factura de
// una llamada de visión tiene que salir de su presupuesto, no de un servicio
// que no sabe de quién es la petición.
func (h *Handlers) GuardarDescripcion(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cuerpo struct {
		Descripcion string   `json:"description"`
		Etiquetas   []string `json:"tags"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.GuardarDescripcion(r.Context(), id, sub(r), cuerpo.Descripcion, cuerpo.Etiquetas); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/assets/{id}/similar?limit=
//
// Las fotos de esa persona que más se parecen a la dada, de más a menos.
//
// LO QUE ESTO NO ES: buscar por texto. El parecido se mide entre vectores que
// ya están guardados, así que no hace falta el modelo en la mano; escribir
// «playa» sí lo necesitaría, para vectorizar lo escrito, y ese codificador no
// está disponible al vuelo.
//
// Una foto que el lote todavía no ha mirado devuelve lista VACÍA, no un error:
// no es un fallo, es que aún no le toca.
func (h *Handlers) Parecidas(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limite := 24
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limite = n
	}
	fotos, err := h.store.Parecidas(r.Context(), id, sub(r), limite)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]activoJSON, 0, len(fotos))
	for _, a := range fotos {
		items = append(items, aJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/similar?ids=<id,id,…>&limit=
//
// «Más como estas»: lo que se parece al CONJUNTO de las dadas.
//
// LOS IDS VIENEN DEL CLIENTE, y no se recalcula aquí la búsqueda que ya hizo.
// Quien pregunta acaba de ver esas fotos en pantalla —son sus resultados o su
// selección—, así que repetir la consulta en el servidor sería hacer dos veces
// el mismo trabajo para llegar a la lista que el cliente ya tiene en la mano. Y
// de paso la función sirve para las dos cosas: ampliar una búsqueda y el botón
// de la barra de selección.
//
// Que sean ids ajenos no abre nada: el store solo mira las de quien pregunta,
// así que una lista de ids robados da exactamente el mismo resultado que una
// lista vacía.
func (h *Handlers) ParecidasAVarias(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`: esta ruta está montada con
	// `GetYQuery`, y por QUERY los parámetros viajan en el CUERPO —la query
	// string llega vacía—. Leyéndolos de la URL, «fotos parecidas» se quedaba sin
	// argumentos y respondía como si no le hubieran pedido nada. Pasó con el
	// buscador global del Hub, que devolvía 400 en cada pulsación.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var ids []uuid.UUID
	for _, s := range strings.Split(params.Get("ids"), ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_id").Wrap(err))
			return
		}
		ids = append(ids, id)
		if len(ids) == store.TopeDeSemillas {
			break
		}
	}
	limite := 24
	if n, err := strconv.Atoi(params.Get("limit")); err == nil {
		limite = n
	}
	fotos, err := h.store.ParecidasAVarias(r.Context(), sub(r), ids, limite)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]activoJSON, 0, len(fotos))
	for _, a := range fotos {
		items = append(items, aJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/themes?limit=
//
// Los montones que la galería propone sin que nadie los pida: fotos que van
// juntas por lo que se ve en ellas.
//
// NO CREA NADA. Devuelve montones y quien decide si eso es un álbum es la
// persona. Un servicio que crea álbumes solo llena la lista de álbumes de cosas
// que nadie pidió, y limpiarlos cuesta más que hacerlos.
func (h *Handlers) Temas(w http.ResponseWriter, r *http.Request) {
	limite := store.TopeDeTemas
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limite = n
	}
	grupos, err := h.store.Temas(r.Context(), sub(r), limite)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	salida := make([]map[string]any, 0, len(grupos))
	for _, g := range grupos {
		items := make([]activoJSON, 0, len(g.Fotos))
		for _, a := range g.Fotos {
			items = append(items, aJSON(a))
		}
		salida = append(salida, map[string]any{"items": items})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"themes": salida})
}

// GET /api/v1/assets/{id}/nearby?limit=
//
// Las fotos hechas cerca de esta. «Cerca» son dos kilómetros: el mismo sitio,
// no la misma ciudad.
//
// Una foto sin coordenadas devuelve lista VACÍA, no un error: la mayoría de las
// fotos de una cámara sin GPS no las tienen, y eso no es una avería.
func (h *Handlers) CercaDe(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limite := 48
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limite = n
	}
	fotos, err := h.store.CercaDe(r.Context(), id, sub(r), limite)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]activoJSON, 0, len(fotos))
	for _, a := range fotos {
		items = append(items, aJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/duplicates?limit=
//
// Las fotos que sobran: ráfagas y reintentos del mismo momento, agrupados.
//
// NO BORRA NADA. Devuelve grupos y quien decide es la persona: proponer y
// ejecutar de una vez es cómo se pierde una foto que no sobraba, y eso no se
// deshace desde la papelera si además se vació.
func (h *Handlers) Duplicadas(w http.ResponseWriter, r *http.Request) {
	limite := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limite = n
	}
	grupos, err := h.store.Duplicadas(r.Context(), sub(r), limite)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	salida := make([]map[string]any, 0, len(grupos))
	for _, g := range grupos {
		items := make([]activoJSON, 0, len(g.Fotos))
		for _, a := range g.Fotos {
			items = append(items, aJSON(a))
		}
		salida = append(salida, map[string]any{"items": items})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"groups": salida})
}

// GET /api/v1/memories
//
// Las fotos que esa persona hizo un día como hoy en años anteriores.
//
// EL DÍA LO PONE EL SERVIDOR, no el cliente. Aceptar la fecha por parámetro
// convertiría esto en «enséñame las fotos del 14 de julio de cualquier año»,
// que es otra funcionalidad —una búsqueda por fecha— y además una forma de
// pasear la galería con la URL. El recuerdo es de hoy.
func (h *Handlers) Recuerdos(w http.ResponseWriter, r *http.Request) {
	recuerdos, err := h.store.Recuerdos(r.Context(), sub(r), time.Now())
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	salida := make([]map[string]any, 0, len(recuerdos))
	for _, rec := range recuerdos {
		items := make([]activoJSON, 0, len(rec.Fotos))
		for _, a := range rec.Fotos {
			items = append(items, aJSON(a))
		}
		salida = append(salida, map[string]any{"year": rec.Anyo, "items": items})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"memories": salida})
}
