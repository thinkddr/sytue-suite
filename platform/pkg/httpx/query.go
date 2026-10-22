// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

// MethodQuery es el método QUERY (RFC 10008, junio de 2026): una petición SEGURA
// e IDEMPOTENTE que lleva la consulta en el CUERPO en vez de en la URL.
//
// Go no trae constante para él —`net/http` solo declara los métodos del RFC
// 9110—, así que se declara aquí una vez y no se teclea suelta por ahí.
const MethodQuery = "QUERY"

// CUÁNDO VALE LA PENA, Y CUÁNDO NO.
//
// QUERY no es «GET pero mejor»: es GET para las consultas que no caben o no
// deberían ir en una URL. Se añade donde gana:
//
//   - LA URL SE QUEDA CORTA. `?ids=` con una docena de UUID son ~500 caracteres
//     y crecen con la selección. No hay límite en el estándar, pero sí en los
//     proxies y servidores reales, y el fallo cuando se cruza no es un error
//     claro: es un 414 o una URL truncada.
//   - LA CONSULTA ES DE LA PERSONA. Lo que alguien escribe para buscar en su
//     correo o en sus fotos no debería quedar escrito en el log de accesos de
//     cada salto, ni en el `Referer`, ni en el historial del navegador. En el
//     cuerpo no queda.
//   - EL FILTRO ES ESTRUCTURADO. Listas y objetos codificados a mano en una
//     query string son un formato inventado que hay que parsear a mano.
//
// Y NO se añade donde GET sigue siendo mejor:
//
//   - CUANDO HAY CACHÉ DE POR MEDIO. QUERY es cacheable sobre el papel, pero
//     ninguna caché real lo implementa todavía: pasar a QUERY una respuesta que
//     hoy se sirve de caché la convierte en una consulta a la base de datos.
//   - CON PARÁMETROS TRIVIALES. `?limit=50` cabe de sobra en una URL, y mover
//     eso al cuerpo solo añade una forma más de llamar a lo mismo.
//
// GET SE QUEDA SIEMPRE, como respaldo: hay clientes desplegados que no se
// actualizan solos —las apps de escritorio y móvil— y un método nuevo que no
// entienden no puede dejarles sin API.

// EnableQueryMethod registra QUERY en chi para que `r.Method`/`r.Handle` puedan
// enrutarlo. chi solo conoce los métodos del RFC 9110; sin esto, un QUERY cae en
// MethodNotAllowed (405) por muy bien montado que esté el handler. Es idempotente.
func EnableQueryMethod() { chi.RegisterMethod(MethodQuery) }

// Y SE REGISTRA SOLO, al importar el paquete.
//
// Los verbos WebDAV no lo hacen —`EnableWebDAVMethods` hay que llamarla— porque
// solo los usan dos servicios y son nueve métodos. QUERY lo va a usar cualquiera
// que tenga una búsqueda, y olvidarse de la llamada no da un error al compilar:
// da un 405 en producción sobre una ruta que en el código está perfectamente
// montada. Esa clase de fallo se quita de en medio registrándolo aquí.
func init() { EnableQueryMethod() }

// maxQueryBody acota el cuerpo de un QUERY. Una consulta son unos cientos de
// bytes; 64 KiB deja sitio de sobra para una selección larga y sigue siendo
// ridículo comparado con lo que cuesta servirla.
const maxQueryBody = 64 << 10

// tipoQuery es el formato del cuerpo que se acepta.
//
// FORM-URLENCODED Y NO JSON, y no es por gusto: así el cuerpo se parsea a
// `url.Values`, que es EXACTAMENTE lo que devuelve `r.URL.Query()`. Un handler
// que hoy lee `r.URL.Query().Get("q")` pasa a leer `p.Get("q")` y funciona con
// los dos métodos sin una sola rama más. Con JSON habría que escribir un struct
// y un decodificador por endpoint, que es donde se cuelan las diferencias de
// comportamiento entre la ruta vieja y la nueva.
const tipoQuery = "application/x-www-form-urlencoded"

// Consulta devuelve los parámetros de la petición: del CUERPO si es QUERY, de la
// URL si es cualquier otro método.
//
// Con QUERY se ignora la query string de la URL a propósito. El RFC dice que la
// consulta es el cuerpo; aceptar las dos a la vez obligaría a decidir cuál gana
// cuando se contradicen, y esa decisión no la conoce quien escribe el cliente.
func Consulta(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	if r.Method != MethodQuery {
		return r.URL.Query(), nil
	}

	if ct := r.Header.Get("Content-Type"); ct != "" {
		medio, _, err := mime.ParseMediaType(ct)
		if err != nil || medio != tipoQuery {
			return nil, ErrUnsupportedMedia.WithCode("query_content_type").
				Wrap(fmt.Errorf("QUERY quiere %s y ha llegado %q", tipoQuery, ct))
		}
	}

	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxQueryBody))
	if err != nil {
		return nil, ErrBadRequest.WithCode("query_body").Wrap(err)
	}
	valores, err := url.ParseQuery(string(cuerpo))
	if err != nil {
		return nil, ErrBadRequest.WithCode("query_invalid").Wrap(err)
	}
	return valores, nil
}

// AnunciarQuery pone `Accept-Query` en la respuesta: es como el RFC dice que un
// servidor declara que ese recurso entiende QUERY y con qué formato.
//
// Sirve para que un cliente lo DESCUBRA en vez de tener que saberlo: pide como
// pedía, ve la cabecera y usa QUERY a partir de ahí. Sin esto, la única forma de
// averiguarlo es lanzar un QUERY y ver si contesta 405, que cuesta un viaje.
func AnunciarQuery(w http.ResponseWriter, tipos ...string) {
	if len(tipos) == 0 {
		tipos = []string{tipoQuery}
	}
	w.Header().Set("Accept-Query", strings.Join(tipos, ", "))
}

// GetYQuery monta el mismo handler en GET y en QUERY, para los recursos cuya
// consulta va hoy en la query string.
//
// Los dos, y no solo QUERY: hay clientes desplegados que no se actualizan solos
// —las apps de escritorio y móvil— y un método nuevo que no entienden no puede
// dejarles sin API. El handler no distingue cuál le ha tocado —para eso está
// `Consulta`—, así que no hay dos caminos que puedan divergir.
func GetYQuery(r chi.Router, patron string, h http.HandlerFunc) {
	r.Get(patron, h)
	r.Method(MethodQuery, patron, h)
}

// PostYQuery monta el mismo handler en POST y en QUERY, para los recursos que
// usaban POST SOLO porque GET no lleva cuerpo.
//
// Es el caso que el RFC pone de ejemplo: una búsqueda con filtros estructurados
// no cabe en una URL, así que se hacía con POST, y entonces deja de parecer lo
// que es. Un POST anuncia que algo cambia —los intermediarios no lo reintentan,
// las cachés lo descartan, quien lee el log supone que hubo escritura— y aquí no
// cambia nada. Con QUERY la petición dice la verdad: segura e idempotente.
//
// SOLO donde el POST no tiene efectos. Si crea, borra o modifica algo, no es
// candidato por muy «de consulta» que suene su nombre.
//
// El handler no cambia: lee su cuerpo JSON igual, porque `DecodeJSON` no mira el
// método.
func PostYQuery(r chi.Router, patron string, h http.HandlerFunc) {
	r.Post(patron, h)
	r.Method(MethodQuery, patron, h)
}
