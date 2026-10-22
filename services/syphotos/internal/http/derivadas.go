// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
)

// GET /api/v1/assets/{id}/variants/{variante}?f=<huella>
//
// Sirve una derivada, y la GENERA si no está.
//
// GENERAR BAJO DEMANDA y no solo al ingerir, porque las derivadas son caché: se
// pueden haber caducado, borrado por espacio o no haberse llegado a crear —la
// ingesta indexa primero y deriva después, así que hay una ventana en la que la
// foto ya se ve en la línea de tiempo y su miniatura todavía no existe—. Un
// hueco gris permanente por eso sería un fallo que se arregla solo si alguien se
// acuerda de relanzar un trabajo; esto lo arregla al mirarlo.
//
// DOS URL CON DOS CACHÉS, y la diferencia importa.
//
// CON `?f=<huella>` la respuesta es INMUTABLE de verdad: la huella nombra la
// receta, así que esa URL siempre devuelve los mismos bytes. El navegador la
// pide una vez y no vuelve a preguntar.
//
// SIN `?f=` la URL significa «la última edición», y eso CAMBIA: girar una foto
// desde el visor o guardarla en el editor deja otros bytes detrás de la misma
// dirección. Marcarla `immutable` —como se hacía— dejaba la miniatura vieja
// clavada en la cuadrícula durante un año, mientras el visor ya enseñaba la
// nueva. Ahora lleva la huella vigente como `ETag` y se revalida: cuando no ha
// cambiado son 304 de unos bytes, y cuando cambia llega sola.
//
// ponytail: se sirve por el proceso (io.Copy) y no con una URL prefirmada. El
// techo es el ancho de banda de este servicio; con `immutable` cada miniatura
// viaja una sola vez por navegador, así que hace falta medirlo antes de pagar
// el salto extra que cuesta una redirección. `objectstore.PresignGet` ya existe
// para cuando la medida lo pida.
func (h *Handlers) Derivada(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	variante := domain.Variante(chi.URLParam(r, "variante"))
	if _, ok := variante.LadoMayor(); !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("unknown_variant").
			WithMessage("Unknown variant."))
		return
	}
	// Se comprueba el activo ANTES de tocar el almacén: es lo que decide si esta
	// persona puede ver esta foto, y la clave del objeto no lleva esa
	// información.
	a, err := h.store.Activo(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}

	// La receta VIGENTE es la única que se sabe derivar: una huella que no sea la
	// suya no es una licencia para generar nada (sería derivar «algo» y
	// guardarlo bajo un nombre que no le corresponde), así que 404.
	rec, err := h.store.Receta(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	vigente, err := rec.Huella()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pedida := r.URL.Query().Get("f")
	huella := pedida
	if huella == "" {
		// Sin huella explícita, la receta VIGENTE. Así una URL sin parámetros
		// sigue siendo válida y enseña la última edición.
		huella = vigente

		// La revalidación se contesta ANTES de tocar el almacén: es el caso
		// normal al volver a una galería ya vista, y no hay que leer un objeto
		// para decir que no ha cambiado.
		w.Header().Set("ETag", etiquetaDe(vigente))
		w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
		if coincideEtiqueta(r.Header.Get("If-None-Match"), etiquetaDe(vigente)) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	formato := "jpg"
	contentType := "image/jpeg"
	if service.EsPNG(a) {
		formato = "png"
		contentType = "image/png"
	}

	d := domain.Derivada{ActivoID: a.ID, Huella: huella, Variante: variante, Formato: formato}
	clave, err := d.Clave()
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	rc, err := h.objetos.Get(r.Context(), t, clave)
	if errors.Is(err, objectstore.ErrNotFound) {
		aDerivar, sabemos := recetaDeLaHuella(rec, vigente, huella)
		if !sabemos {
			httpx.Fail(w, r, domain.ErrNotFound)
			return
		}
		if err := h.generar(r, a, aDerivar); err != nil {
			httpx.Fail(w, r, traducir(err))
			return
		}
		rc, err = h.objetos.Get(r.Context(), t, clave)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	if pedida != "" {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	}
	// `private` y no `public` en los dos casos: son fotos de una persona, y una
	// caché compartida —un proxy de empresa— no puede guardarlas para
	// servírselas a otra. La URL es adivinable si se conoce el id y la huella,
	// así que lo que protege es la sesión, no el secreto de la ruta.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, rc); err != nil {
		// La cabecera ya salió: no se puede convertir esto en un error HTTP.
		// Queda en el log, que es donde se ve si pasa a menudo.
		log.From(r.Context()).Warn("derivada cortada a mitad de envío",
			"asset", a.ID, "variante", variante, "error", err)
	}
}

// huellaVacia nombra al ORIGINAL sin editar. La calcula `service`, que es
// quien también la necesita para limpiar: dos definiciones del mismo valor se
// separan en cuanto cambie la forma canónica de la receta.
var huellaVacia = service.HuellaDelOriginal()

// recetaDeLaHuella dice qué receta produce la derivada que se pide, y si es
// una que este servicio se sabe fabricar.
//
// SON DOS, Y SOLO DOS. La VIGENTE es lo que la persona ve en la galería. La
// VACÍA es el original sin editar, y hace falta porque es lo que abre el
// EDITOR: la derivada lleva la receta horneada, así que abrir la vigente y
// aplicarle la receta encima enseñaba cada ajuste dos veces —una foto girada
// aparecía del revés y una subida de exposición, quemada—.
//
// Cualquier otra huella no se fabrica: sería derivar «algo» y archivarlo bajo
// un nombre que no le corresponde. Las de ediciones anteriores se siguen
// SIRVIENDO si siguen en el almacén; lo que no se hace es crearlas.
func recetaDeLaHuella(vigenteRec domain.Receta, vigente, pedida string) (domain.Receta, bool) {
	switch pedida {
	case vigente:
		return vigenteRec, true
	case huellaVacia:
		return domain.RecetaVacia(), true
	}
	return domain.Receta{}, false
}

// etiquetaDe envuelve una huella como ETag fuerte.
func etiquetaDe(huella string) string { return `"` + huella + `"` }

// coincideEtiqueta dice si un `If-None-Match` cubre esta etiqueta.
//
// La cabecera admite una LISTA y el comodín `*`, y las etiquetas pueden venir
// marcadas como débiles (`W/"..."`). Comparar la cabecera entera con la
// etiqueta —que es lo que sale de escribirlo a ojo— falla en cuanto el
// navegador manda dos, y entonces la revalidación no ahorra nada porque nunca
// acierta.
func coincideEtiqueta(cabecera, etiqueta string) bool {
	for candidata := range strings.SplitSeq(cabecera, ",") {
		candidata = strings.TrimSpace(candidata)
		if candidata == "*" {
			return true
		}
		if strings.TrimPrefix(candidata, "W/") == etiqueta {
			return true
		}
	}
	return false
}

// generar produce las derivadas de una receta. Serializa por FOTO: cien
// miniaturas de una cuadrícula pueden pedirse a la vez, y decodificar cien
// originales de 24 megapíxeles en paralelo agota la memoria del proceso — que
// es como un hueco gris se convierte en una caída.
func (h *Handlers) generar(r *http.Request, a domain.Activo, rec domain.Receta) error {
	if h.ingesta == nil {
		return domain.ErrNotFound
	}
	token := bearerDe(r)
	if token == "" {
		return domain.ErrForbidden
	}
	// El semáforo acota cuántas decodificaciones hay a la vez EN TODO el
	// proceso, no por petición: el límite que importa es la memoria de la
	// réplica.
	select {
	case h.derivando <- struct{}{}:
		defer func() { <-h.derivando }()
	case <-r.Context().Done():
		return r.Context().Err()
	}
	return h.ingesta.Derivar(r.Context(), token, a, rec)
}

// bearerDe saca el token CRUDO de la cabecera.
//
// Hace falta porque los originales los custodia Drive y este servicio los pide
// EN NOMBRE de la persona: quien decide si puede leer ese fichero es Drive, con
// su propia comprobación de permisos y de compartición. Un token de servicio
// aquí convertiría a Sytue Photos en una puerta trasera a cualquier fichero de la
// unidad, que es exactamente lo que no puede ser.
func bearerDe(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// POST /api/v1/assets  {"driveFileId": "..."}
//
// Da de alta una foto de Drive en la galería. Solo INDEXA —lee 128 KiB de
// cabecera— y contesta: las derivadas se generan al pedirlas por primera vez.
//
// Esperar aquí a generarlas convertiría subir un carrete de doscientas fotos en
// varios minutos de barra de progreso, cuando lo que la persona quiere ver es su
// galería. Con la fila escrita, la cuadrícula ya sabe cuántos huecos hacen falta
// y de qué tamaño es cada uno.
//
// ES IDEMPOTENTE: reindexar una carpeta no duplica nada y sí mejora lo que la
// primera pasada no supo leer.
func (h *Handlers) Indexar(w http.ResponseWriter, r *http.Request) {
	if h.ingesta == nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	var cuerpo struct {
		DriveFileID string `json:"driveFileId"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(cuerpo.DriveFileID) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_drive_file_id").
			WithMessage("driveFileId is required."))
		return
	}
	token := bearerDe(r)
	if token == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	a, err := h.ingesta.Indexar(r.Context(), token, cuerpo.DriveFileID, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// 200 y no 201: la operación es idempotente y la mitad de las veces devuelve
	// una foto que ya estaba. Un 201 diría «creada» cuando no se creó nada.
	httpx.JSON(w, r, http.StatusOK, aJSON(a))
}

// GET /api/v1/assets/{id}/export
//
// La foto a resolución completa con la receta vigente, como descarga. Se
// genera al vuelo y no se guarda —exportar es raro y pesa lo que el
// original—, y pasa por el mismo semáforo que las derivadas para que diez
// descargas a la vez no se coman la memoria del servicio.
func (h *Handlers) Exportar(w http.ResponseWriter, r *http.Request) {
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
	rec, err := h.store.Receta(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if h.ingesta == nil {
		httpx.Fail(w, r, traducir(domain.ErrNotFound))
		return
	}
	token := bearerDe(r)
	if token == "" {
		httpx.Fail(w, r, traducir(domain.ErrForbidden))
		return
	}
	select {
	case h.derivando <- struct{}{}:
		defer func() { <-h.derivando }()
	case <-r.Context().Done():
		return
	}
	// El JPEG se codifica primero en memoria: si el original falla a medias no
	// se ha mandado ya un 200 con medio fichero.
	var buf bytes.Buffer
	formato, ext, tipo := formatoDeExportacion(r.URL.Query().Get("formato"))
	if err := h.ingesta.Exportar(r.Context(), &buf, token, a, rec, formato); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	nombre := strings.TrimSuffix(a.Nombre, filepath.Ext(a.Nombre)) + "-sytue-fotos" + ext
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": nombre}))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(buf.Bytes())
}

// formatoDeExportacion traduce `?formato=` a lo que necesita la respuesta: el
// nombre que entiende Ingesta.Exportar, la extensión del fichero y el tipo MIME.
// Todo lo que no sea "png" es JPEG: es lo que se pide sin decir nada y lo que
// pesa menos para compartir.
func formatoDeExportacion(pedido string) (formato, ext, tipo string) {
	if strings.EqualFold(strings.TrimSpace(pedido), "png") {
		return "png", ".png", "image/png"
	}
	return "jpeg", ".jpg", "image/jpeg"
}
