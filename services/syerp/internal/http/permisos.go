// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
)

// El ERP es de toda la organización; lo que cada persona puede hacer en cada
// módulo lo decide la administración (Sytue Admin → ERP) con los niveles de
// `permisos`. Aquí se aplica: en cada ruta de modelo, en las rutas propias
// (libro, banco, obligaciones) y en lo que Sytue AI puede leer o crear.

var errSinPermiso = httpx.NewError(http.StatusForbidden, "sin_permiso",
	"You do not have access to this module. Ask an administrator of your organisation.")

type clavePermisos struct{}

// permisosPeticion guarda lo puesto a quien pide —niveles y, si se le separaron
// las funciones, verbos—, leído UNA vez por petición y en una sola transacción.
type permisosPeticion struct {
	once    sync.Once
	puestos map[string]string
	verbos  map[string][]string
	err     error
}

// conPermisos prepara la lectura perezosa de los permisos de la petición.
func conPermisos(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clavePermisos{}, &permisosPeticion{})))
	})
}

// loPuesto lee, una sola vez por petición, los niveles y las listas de verbos de
// quien pide. `ok` es falso si la base no contestó: entonces se falla cerrado.
func (h *Handlers) loPuesto(ctx context.Context) (map[string]string, map[string][]string, bool) {
	pp, _ := ctx.Value(clavePermisos{}).(*permisosPeticion)
	if pp == nil {
		pp = &permisosPeticion{}
	}
	pp.once.Do(func() { pp.puestos, pp.verbos, pp.err = h.store.PermisosDe(ctx, subDe(ctx)) })
	if pp.err != nil {
		log.From(ctx).Warn("permisos: no se pudieron leer", "error", pp.err)
		return nil, nil, false
	}
	return pp.puestos, pp.verbos, true
}

func subDe(ctx context.Context) string {
	claims, _ := authn.FromContext(ctx)
	return claims.Subject
}

// nivelEn es el nivel de quien pide en un módulo.
func (h *Handlers) nivelEn(ctx context.Context, modulo string) string {
	claims, ok := authn.FromContext(ctx)
	if !ok {
		return permisos.Ninguno
	}
	if permisos.EsAdministradorDeLaOrganizacion(claims.Roles) {
		return permisos.Administrador
	}
	puestos, _, ok := h.loPuesto(ctx)
	if !ok {
		// Sin poder leer los permisos se aplica el defecto, que para lo sensible es
		// «sin acceso»: fallar cerrado donde importa.
		return permisos.PorDefecto(modulo)
	}
	return permisos.Nivel(claims.Roles, puestos, modulo)
}

// verbosEn son los gestos que quien pide puede hacer en un módulo.
func (h *Handlers) verbosEn(ctx context.Context, modulo string) []string {
	claims, ok := authn.FromContext(ctx)
	if !ok {
		return nil
	}
	puestos, verbos, ok := h.loPuesto(ctx)
	if !ok {
		// Fallar cerrado: sin saber si a esta persona le separaron las funciones,
		// no se le concede ningún gesto sensible. Lo que sigue funcionando es la
		// lectura, que va por nivel y ya tiene su propio defecto.
		return nil
	}
	return permisos.VerbosDe(claims.Roles, puestos, verbos, modulo)
}

// puede dice si quien pide llega a `nivel` en `modulo`.
func (h *Handlers) puede(ctx context.Context, modulo, nivel string) bool {
	return permisos.Alcanza(h.nivelEn(ctx, modulo), nivel)
}

// puedeVerbo dice si quien pide puede ese GESTO en ese módulo. Es la
// separación de funciones: no la implica ningún nivel.
func (h *Handlers) puedeVerbo(ctx context.Context, modulo, verbo string) bool {
	return slices.Contains(h.verbosEn(ctx, modulo), verbo)
}

// exigirVerbo protege una ruta que hace UN gesto concreto sobre el módulo del
// modelo de la URL (asentar, conciliar, importar…). Va DESPUÉS de exigirModelo,
// que ya resolvió que el modelo existe y que el nivel alcanza para escribir.
func (h *Handlers) exigirVerbo(verbo string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			modulo := h.moduloDe(chi.URLParam(r, "modelo"))
			if modulo != "" && !h.puedeVerbo(r.Context(), modulo, verbo) {
				httpx.Fail(w, r, errSinVerbo(verbo))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// exigirVerboContable es lo mismo para las rutas que no cuelgan de un modelo y
// viven todas en contabilidad: el libro, el banco, el cierre del ejercicio, los
// umbrales de aprobación y la vuelta atrás de una importación. El módulo va
// fijo porque es el que manda en el ERP; el día que otra familia de rutas
// propias necesite lo mismo, se le pasa por parámetro.
func (h *Handlers) exigirVerboContable(verbo string) func(http.Handler) http.Handler {
	const modulo = "contabilidad"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.puedeVerbo(r.Context(), modulo, verbo) {
				httpx.Fail(w, r, errSinVerbo(verbo))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// errSinVerbo dice QUÉ gesto falta, no solo que falta permiso: «no puedes
// asentar» lleva a pedir ese permiso; «sin acceso» lleva a discutir el módulo.
func errSinVerbo(verbo string) *httpx.Error {
	return httpx.NewError(http.StatusForbidden, "sin_verbo_"+verbo,
		"Your role in this module does not allow this action ("+verbo+"). Duties are segregated: ask an administrator of your organisation.")
}

// exigir es el middleware de las rutas propias de un módulo.
func (h *Handlers) exigir(modulo, nivel string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.puede(r.Context(), modulo, nivel) {
				httpx.Fail(w, r, errSinPermiso)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// exigirModelo protege el CRUD genérico: leer pide `lectura` en el módulo del
// modelo y escribir, `usuario`. Un modelo que no existe sigue su camino hasta el
// 404 de siempre.
func (h *Handlers) exigirModelo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, ok := h.cat.Modelo(chi.URLParam(r, "modelo"))
		nivel := permisos.Usuario
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			nivel = permisos.Lectura
		}
		if ok && !h.puede(r.Context(), m.Modulo, nivel) {
			httpx.Fail(w, r, errSinPermiso)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// moduloDe es el módulo de un modelo del catálogo, o de lo que no es un modelo
// (una obligación). "" si no se sabe.
func (h *Handlers) moduloDe(modelo string) string {
	if modelo == "obligacion" {
		return permisos.Obligaciones
	}
	if m, ok := h.cat.Modelo(modelo); ok {
		return m.Modulo
	}
	return ""
}

// modulosConRotulo son todos los módulos a los que se da permiso: los del
// catálogo y Obligaciones.
func (h *Handlers) modulosConRotulo(idioma string) []map[string]string {
	out := make([]map[string]string, 0, 1+len(h.cat.Modulos()))
	out = append(out, map[string]string{
		"clave": permisos.Obligaciones, "rotulo": h.cat.Rotulo("modulo."+permisos.Obligaciones, idioma),
		"por_defecto": permisos.PorDefecto(permisos.Obligaciones),
	})
	for _, m := range h.cat.Modulos() {
		out = append(out, map[string]string{
			"clave": m.Clave, "rotulo": h.cat.Rotulo("modulo."+m.Clave, idioma), "por_defecto": permisos.PorDefecto(m.Clave),
		})
	}
	return out
}

// MisPermisos — GET /api/v1/mis-permisos — el nivel de quien pide en cada módulo,
// para que la interfaz no ofrezca lo que va a responder 403. De paso apunta que
// esta persona usa el ERP: es a quien se podrá avisar por correo.
func (h *Handlers) MisPermisos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, _ := authn.FromContext(ctx)
	email := claims.Mailbox
	if email == "" {
		email = claims.Email
	}
	if err := h.store.VerUsuario(ctx, claims.Subject, email, claims.Name, claims.Roles); err != nil {
		log.From(ctx).Warn("permisos: no se pudo apuntar al usuario", "error", err)
	}
	niveles := map[string]string{permisos.Obligaciones: h.nivelEn(ctx, permisos.Obligaciones)}
	verbos := map[string][]string{permisos.Obligaciones: h.verbosEn(ctx, permisos.Obligaciones)}
	for _, m := range h.cat.Modulos() {
		niveles[m.Clave] = h.nivelEn(ctx, m.Clave)
		verbos[m.Clave] = h.verbosEn(ctx, m.Clave)
	}
	// Los módulos en los que ya ha pedido editar: su franja dice «Petición enviada».
	pedidos, err := h.store.ModulosPedidos(ctx, claims.Subject)
	if err != nil || pedidos == nil {
		pedidos = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"administrador": permisos.EsAdministradorDeLaOrganizacion(claims.Roles),
		"modulos":       niveles,
		"pedidos":       pedidos,
		// Los gestos, resueltos: la interfaz esconde «Asentar» en vez de
		// ofrecerlo y contestar 403 al pulsarlo.
		"verbos": verbos,
	})
}

// Permisos — GET /api/v1/admin/permisos — los módulos con su nivel por defecto y
// las personas que usan el ERP o tienen un nivel puesto (solo administración).
func (h *Handlers) Permisos(w http.ResponseWriter, r *http.Request) {
	personas, err := h.store.Personas(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"niveles": permisos.Niveles,
		// El catálogo de gestos y los que concede cada nivel por sí solo: sin lo
		// segundo, la pantalla no puede enseñar de qué se parte al separar.
		"verbos":             permisos.Verbos,
		"verbos_por_defecto": verbosPorNivel(),
		"modulos":            h.modulosConRotulo(httpx.LanguageFromRequest(r)),
		"personas":           personas,
	})
}

func verbosPorNivel() map[string][]string {
	out := make(map[string][]string, len(permisos.Niveles))
	for _, n := range permisos.Niveles {
		out[n] = permisos.VerbosPorDefecto(n)
	}
	return out
}

// GuardarPermisos — PUT /api/v1/admin/permisos/{sub} — los niveles de una persona.
// Lo que coincide con el defecto del módulo no se guarda: así un cambio de defecto
// alcanza a quien no se tocó.
func (h *Handlers) GuardarPermisos(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email   string            `json:"email"`
		Nombre  string            `json:"nombre"`
		Modulos map[string]string `json:"modulos"`
		// Verbos separa las funciones en los módulos que traiga: la lista es la
		// COMPLETA para esa persona ahí y sustituye a la que implica su nivel.
		// Un módulo ausente vuelve a la deducida.
		Verbos map[string][]string `json:"verbos"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	persona := strings.TrimSpace(chi.URLParam(r, "sub"))
	v := validation.New().Required("sub", persona).MaxLen("sub", persona, 200)
	conocidos := []string{permisos.Obligaciones}
	for _, m := range h.cat.Modulos() {
		conocidos = append(conocidos, m.Clave)
	}
	distintos := map[string]string{}
	for modulo, nivel := range c.Modulos {
		switch {
		case !slices.Contains(conocidos, modulo):
			v.Add("modulos", "módulo desconocido "+modulo)
		case !permisos.Valido(nivel):
			v.Add("modulos", "nivel desconocido "+nivel)
		case nivel != permisos.PorDefecto(modulo):
			distintos[modulo] = nivel
		}
	}
	listas := map[string][]string{}
	for modulo, lista := range c.Verbos {
		if !slices.Contains(conocidos, modulo) {
			v.Add("verbos", "módulo desconocido "+modulo)
			continue
		}
		limpia := make([]string, 0, len(lista))
		for _, verbo := range lista {
			switch {
			case !permisos.VerboValido(verbo):
				v.Add("verbos", "verbo desconocido "+verbo)
			case !slices.Contains(limpia, verbo):
				limpia = append(limpia, verbo)
			}
		}
		listas[modulo] = limpia
	}
	// Nadie se quita a sí mismo la llave de la puerta: sin `administrar` en
	// ningún módulo, quien edita esto puede dejarse fuera de la pantalla que
	// acaba de usar. El caso se da con un clic y no tiene marcha atrás desde la
	// interfaz.
	if persona == sub(r) {
		for modulo, lista := range listas {
			if !slices.Contains(lista, permisos.Administrar) &&
				h.puedeVerbo(r.Context(), modulo, permisos.Administrar) {
				v.Add("verbos", "no puedes retirarte a ti mismo «administrar» en "+modulo)
			}
		}
	}
	if err := v.Err(); err != nil {
		httpx.Fail(w, r, errorDeValidacion(err))
		return
	}
	email := validation.TruncateRunes(strings.TrimSpace(c.Email), 200)
	nombre := validation.TruncateRunes(strings.TrimSpace(c.Nombre), 200)
	if err := h.store.GuardarPermisos(r.Context(), persona, email, nombre, distintos, listas, sub(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
