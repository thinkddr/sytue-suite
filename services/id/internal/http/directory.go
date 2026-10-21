// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Resolver una dirección de correo a la persona de MI organización.
//
// POR QUÉ HACE FALTA. Compartir un fichero, un calendario o una carpeta con
// alguien se escribe siempre igual: escribiendo su correo. Pero por dentro los
// permisos se guardan contra el identificador de usuario (`sub`), no contra una
// dirección — una dirección cambia, se reasigna y tiene alias—. Sin una forma de
// pasar de una a otro, esa función no se puede construir: Drive lleva desde la
// migración 0001 aceptando comparticiones `kind=user` y su interfaz solo sabía
// hacer enlaces públicos porque no había de dónde sacar el `sub`.
//
// LO QUE YA HABÍA era `POST /admin/members:resolve`, reservado a owner/admin.
// Compartir un fichero no es una operación de administración, así que esto es la
// puerta equivalente para cualquiera de la casa.
//
// LAS TRES BARRERAS, que son lo importante de este fichero:
//
//  1. SOLO LA PROPIA ORGANIZACIÓN. La consulta va acotada a la org activa del
//     token. Un correo de fuera responde 404 igual que uno que no existe: decir
//     «existe pero no es de aquí» ya sería contar algo.
//  2. SOLO LO MÍNIMO. Devuelve el identificador y el correo normalizado. NO el
//     rol —eso es dato de administración— ni el estado de la cuenta.
//  3. UNO A UNO Y CON TOPE. No hay listado ni búsqueda por prefijo: hay que
//     saber la dirección. Con el límite por persona, esto deja de servir para
//     barrer el directorio de la empresa a base de probar nombres.
//
// Es POST y no GET para que el correo —que es dato personal— no acabe escrito en
// una URL, y por tanto en el registro de accesos del ingress y en el historial.

type resolveDirectoryRequest struct {
	Email string `json:"email"`
	// UserID hace el camino de vuelta: de un identificador ya guardado al correo,
	// para poder PINTAR con quién está compartido algo. Sin él, la lista de
	// destinatarios enseñaría identificadores opacos y no serviría de nada.
	UserID string `json:"user_id"`
}

// handleDirectoryResolve — POST /api/v1/directory:resolve.
func (s *Server) handleDirectoryResolve(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req resolveDirectoryRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Camino de vuelta: identificador → correo. Misma barrera de organización, y
	// por eso comparte handler: dos rutas con la misma verja acaban con la verja
	// puesta en una sola.
	if id := strings.TrimSpace(req.UserID); id != "" {
		email, name, err := s.store.ResolveOrgMemberByID(r.Context(), org.ID, id)
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"user_id": id, "email": email, "name": name})
		return
	}

	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Enter a valid email address."))
		return
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))

	userID, _, err := s.store.ResolveOrgMemberByEmail(r.Context(), org.ID, email) //nolint:dogsled // el rol NO sale de aquí
	if errors.Is(err, domain.ErrNotFound) {
		// Mismo 404 para «no existe» y para «existe en otra organización». La
		// diferencia entre las dos respuestas es exactamente el dato que no se
		// quiere regalar a quien va probando direcciones.
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("Nobody in your organisation uses that address."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// El rol que devuelve el store NO sale de aquí: quien comparte un fichero no
	// tiene por qué saber si la otra persona es administradora.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"user_id": userID, "email": email})
}

// handleDirectoryList — GET /api/v1/directory — la AGENDA de la organización:
// quién trabaja aquí, para cualquiera de la casa.
//
// ESTO CAMBIA A PROPÓSITO LA REGLA 3 DE ARRIBA, que decía «uno a uno y con
// tope, no hay listado». Aquella barrera se escribió para `directory:resolve`,
// cuya función es traducir un correo que YA se conoce a un identificador al
// compartir un fichero; allí un listado sería una vía de barrido gratis y sigue
// sin haberla.
//
// Una agenda de organización es otra cosa y es lo que se espera de una suite de
// empresa: las personas con las que se trabaja aparecen en Contactos sin que
// nadie las teclee, y cuando entra alguien nuevo aparece sola. No se descubre
// nada que no esté ya en la lista de destinatarios del correo, en el selector de
// compartir o en el propio directorio de la empresa.
//
// LO QUE SIGUE SIN SALIR: el rol, el estado de la cuenta, si tiene MFA y la
// fecha de alta. Todo eso es dato de administración y vive en /admin/users, con
// su permiso. Aquí van los datos de contacto de trabajo y nada más.
// Acepta GET y QUERY. QUERY porque `q` es LO QUE ALGUIEN TECLEA para encontrar a
// un compañero —un nombre, un correo— y en la URL eso queda escrito en el log de
// accesos de cada salto, en el `Referer` y en el historial. En el cuerpo no
// queda. GET sigue montado como respaldo: los clientes de escritorio y móvil no
// se actualizan solos y un método que no entienden no puede dejarles sin API.
func (s *Server) handleDirectoryList(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// `Consulta` lee del CUERPO si la petición es QUERY y de la URL si es GET, así
	// que el handler es el mismo para los dos y no hay dos caminos que diverjan.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.AnunciarQuery(w)
	limit := 200
	if v := params.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1..500)."))
			return
		}
		limit = n
	}
	after, err := parseUserCursor(params.Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	q := strings.TrimSpace(params.Get("q"))
	if len(q) > 128 {
		q = q[:128]
	}
	// LA AGENDA SE CACHEA, y es de los pocos sitios donde sale a cuenta: la pide
	// el autocompletado del compositor de Mail cada vez que alguien escribe un
	// destinatario, cuesta DOS consultas —la página de usuarios y los buzones de
	// esa página— y la plantilla de una organización cambia una vez al mes.
	//
	// POR ORGANIZACIÓN Y NO POR USUARIO. Lo que devuelve depende solo de `org.ID`
	// y de los parámetros: quién pregunta no cambia ni una fila. Si algún día
	// dependiera del que llama, esta clave habría que partirla o serviría a unos
	// lo de otros.
	//
	// La `q` va HASHEADA: es texto libre de hasta 128 caracteres y una clave de
	// Dragonfly no es sitio para meterlo tal cual.
	clave := claveAgenda(org.ID, q, params.Get("cursor"), limit)
	if cuerpo, ok := s.agendaCacheada(r, clave); ok {
		httpx.JSON(w, r, http.StatusOK, cuerpo)
		return
	}

	page, more, err := s.store.ListOrgUsersPage(r.Context(), org.ID, q, after, limit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	next := ""
	if more && len(page) > 0 {
		last := page[len(page)-1]
		next = formatUserCursor(last.CreatedAt, last.ID)
	}
	// LAS DIRECCIONES DE LA AGENDA SON AQUELLAS EN LAS QUE SE RECIBE AQUÍ, y eso
	// NO es el correo con el que la persona entra.
	//
	// A quien se invita desde fuera, su `Email` es la dirección por la que se le
	// mandó la invitación —su Gmail, su correo de la empresa anterior—. Ponerla
	// en la agenda de la organización es publicar el correo personal de la gente
	// a toda la empresa, y además no sirve para lo que la agenda existe: escribir
	// a esa dirección sale de Sytue y entra en el buzón de otro proveedor.
	//
	// Lo que sí vale son sus buzones: la dirección principal y sus alias, que
	// viven en el subdominio de la organización o en su dominio propio VERIFICADO
	// (ver `OrgMailDomain`). Son exactamente las direcciones en las que Sytue Mail
	// entrega.
	activos := make([]store.OrgUser, 0, len(page))
	ids := make([]string, 0, len(page))
	for _, u := range page {
		// Las cuentas suspendidas o dadas de baja NO están en la agenda: quien ya
		// no trabaja aquí no debe aparecer como si se le pudiera escribir.
		if u.Status != domain.StatusActive {
			continue
		}
		activos = append(activos, u)
		ids = append(ids, u.ID)
	}
	buzones, err := s.store.ListOrgMailboxesDe(r.Context(), org.ID, ids)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// La lista de dominios que la organización ha verificado en la consola: es lo
	// que decide si el correo de invitación cuenta como «de dentro».
	verificados, err := s.store.VerifiedOrgMailDomains(r.Context(), nil, org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}

	items := make([]map[string]any, 0, len(activos))
	for _, u := range activos {
		// La REGLA vive en el dominio y está probada ahí: qué direcciones de una
		// persona salen en la agenda de su organización.
		correos := domain.DireccionesDeAgenda(buzones[u.ID], u.Email, verificados)
		// SIN NINGUNA DIRECCIÓN DE AQUÍ, FUERA DE LA AGENDA. Una ficha sin correo
		// al que escribir no es un contacto: es una fila que ocupa sitio y que al
		// pulsarla no hace nada. Pasa mientras se aprovisiona el buzón de alguien
		// recién invitado, y se arregla sola en la siguiente pasada.
		if len(correos) == 0 {
			continue
		}
		items = append(items, map[string]any{
			"user_id": u.ID,
			// `email` sigue siendo la principal, por los clientes que ya la leen.
			// `emails` es la lista entera, con la principal delante.
			"email":      correos[0],
			"emails":     correos,
			"name":       u.Name,
			"department": u.Department,
			"title":      u.Title,
			"location":   u.Location,
		})
	}
	respuesta := map[string]any{"items": items, "nextCursor": next}
	// UN MINUTO. Lo que se retrasa como mucho es que alguien recién dado de alta
	// tarde eso en aparecer al autocompletar; lo que se gana es que la segunda
	// letra que se teclea no vuelva a PostgreSQL. Si falla el guardado no pasa
	// nada: la caché acelera, nunca decide.
	s.guardarAgenda(r, clave, respuesta)
	httpx.JSON(w, r, http.StatusOK, respuesta)
}

// agendaCacheada devuelve la página guardada, si la hay. `json.RawMessage` se
// escribe TAL CUAL, así que se sirve sin recodificar y sin inventar una función
// nueva en httpx.
//
// Aparte del handler y no dentro: con esto ahí, gocyclo lo pasaba de 20 a 22.
func (s *Server) agendaCacheada(r *http.Request, clave string) (json.RawMessage, bool) {
	if s.cache == nil {
		return nil, false
	}
	cuerpo, ok, _ := s.cache.Get(r.Context(), clave)
	if !ok {
		return nil, false
	}
	return json.RawMessage(cuerpo), true
}

// guardarAgenda deja la página en la cache. Si falla no pasa nada: la cache
// acelera, nunca decide.
func (s *Server) guardarAgenda(r *http.Request, clave string, v any) {
	if s.cache == nil {
		return
	}
	cuerpo, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = s.cache.SetTTL(r.Context(), clave, string(cuerpo), ttlAgenda)
}

// ttlAgenda es cuánto vive una página de agenda en la cache.
const ttlAgenda = time.Minute

// claveAgenda compone la clave de una página de agenda. La consulta y el cursor
// se hashean juntos: son texto libre del cliente y no pueden entrar crudos.
func claveAgenda(orgID, q, cursor string, limit int) string {
	h := sha256.Sum256([]byte(q + "\x00" + cursor + "\x00" + strconv.Itoa(limit)))
	return "id:agenda:" + orgID + ":" + hex.EncodeToString(h[:8])
}
