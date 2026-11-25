// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// LA TRAZA DE ACCESO A DATOS DE CLIENTE (docs/security.md §1.7).
//
// Hasta aquí la tabla `audit_log` solo la escribía el cumplimiento (legal hold y
// eDiscovery), así que quién descargó, abrió o compartió un fichero, un correo o
// un documento no dejaba traza: un acceso indebido con credenciales válidas no se
// podía reconstruir. Esto la escribe en UN punto por servicio:
//
//   - Registro.Middleware envuelve el router entero (httpx.RouterOptions.Traza)
//     y, cuando una petición que casa con una Regla termina bien, anota quién
//     (el sub del token, visto por authn.Testigo), qué (la acción), sobre qué (el
//     id del recurso, de la ruta) y desde dónde (IP y request-id).
//   - Anotar / AnotarComo, para lo que la ruta no dice: el recurso viene en el
//     cuerpo (compartir) o no hay sesión (un enlace público, cuyo tenant solo
//     se sabe al resolverlo).
//
// Tres reglas de lo que se escribe:
//
//   - NUNCA CONTENIDO: ni el cuerpo, ni el nombre de un fichero, ni el asunto
//     de un correo, ni lo que se buscó. El objetivo es un id; el detalle, la vía.
//   - NO BLOQUEA: la escritura va por una cola acotada y en lotes. Si la base no
//     responde, la petición sigue y lo que no cabe se cuenta y se avisa a nivel
//     error (la alerta), en vez de convertir la auditoría en una caída.
//   - RETENCIÓN de doce meses para `acceso.*` (la de los logs de seguridad,
//     §8.1). El cumplimiento (`legalhold.*`, `ediscovery.*`) no se toca: su plazo
//     es el del caso.

// PrefijoAcceso es el prefijo de las acciones de acceso ordinario: separa lo que
// la retención borra de la traza de cumplimiento, que no se toca.
const PrefijoAcceso = "acceso."

// RetencionAcceso es cuánto se guarda la traza de acceso (§8.1: doce meses).
const RetencionAcceso = 365 * 24 * time.Hour

const (
	tamCola       = 4096
	loteMax       = 200
	cadaVaciado   = time.Second
	maxObjetivo   = 512
	maxAccion     = 100
	loteRetencion = 5000
)

// Evento es una línea de la traza.
type Evento struct {
	Momento  time.Time
	Tenant   string
	Actor    string
	Accion   string
	Objetivo string
	Detalle  map[string]any
}

// Registro escribe la traza de acceso de UN servicio en su `audit_log`.
type Registro struct {
	db       pg.Database
	log      *slog.Logger
	cola     chan Evento
	perdidos atomic.Int64
	ahora    func() time.Time
}

// NuevoRegistro crea el registro. Hay que arrancar Ejecutar para que escriba.
func NuevoRegistro(db pg.Database, log *slog.Logger) *Registro {
	if log == nil {
		log = slog.Default()
	}
	return &Registro{db: db, log: log, cola: make(chan Evento, tamCola), ahora: time.Now}
}

// Ejecutar vacía la cola en lotes hasta que se cancela ctx; al cancelarse escribe
// lo que quede, con un plazo propio (el apagado ordenado no pierde la traza).
func (r *Registro) Ejecutar(ctx context.Context) {
	t := time.NewTicker(cadaVaciado)
	defer t.Stop()
	lote := make([]Evento, 0, loteMax)
	for {
		select {
		case e := <-r.cola:
			lote = append(lote, e)
			if len(lote) >= loteMax {
				r.vaciar(ctx, lote)
				lote = lote[:0]
			}
		case <-t.C:
			if len(lote) > 0 {
				r.vaciar(ctx, lote)
				lote = lote[:0]
			}
			if n := r.perdidos.Swap(0); n > 0 {
				r.log.ErrorContext(ctx, "traza de acceso: eventos sin escribir, la cola estaba llena", "eventos", n)
			}
		case <-ctx.Done():
			for {
				select {
				case e := <-r.cola:
					lote = append(lote, e)
					continue
				default:
				}
				break
			}
			fin, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			r.vaciar(fin, lote)
			cancel()
			return
		}
	}
}

// vaciar escribe un lote: una transacción por tenant, porque la RLS de la tabla
// exige escribir con el tenant de la fila fijado.
func (r *Registro) vaciar(ctx context.Context, lote []Evento) {
	porTenant := map[string][]Evento{}
	for _, e := range lote {
		porTenant[e.Tenant] = append(porTenant[e.Tenant], e)
	}
	for tenant, evs := range porTenant {
		if err := escribir(ctx, r.db, tenant, evs); err != nil {
			// Error y no aviso: lo que no se escribe aquí no queda en ningún sitio.
			r.log.ErrorContext(ctx, "traza de acceso: no se pudo escribir un lote", "tenant", tenant, "eventos", len(evs), "error", err)
		}
	}
}

func escribir(ctx context.Context, db pg.Database, tenant string, evs []Evento) error {
	n := len(evs)
	momentos := make([]time.Time, 0, n)
	actores := make([]string, 0, n)
	acciones := make([]string, 0, n)
	objetivos := make([]string, 0, n)
	detalles := make([]string, 0, n)
	for _, e := range evs {
		raw, err := json.Marshal(e.Detalle)
		if err != nil || e.Detalle == nil {
			raw = []byte("{}")
		}
		momentos = append(momentos, e.Momento)
		actores = append(actores, e.Actor)
		acciones = append(acciones, e.Accion)
		objetivos = append(objetivos, e.Objetivo)
		detalles = append(detalles, string(raw))
	}
	return db.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (occurred_at, tenant, actor, action, target, detail)
			SELECT m, $1, a, ac, o, d::jsonb
			FROM unnest($2::timestamptz[], $3::text[], $4::text[], $5::text[], $6::text[]) AS u(m, a, ac, o, d)`,
			tenant, momentos, actores, acciones, objetivos, detalles)
		if err != nil {
			return fmt.Errorf("auditlog: escribir traza: %w", err)
		}
		return nil
	})
}

// anotar encola un evento sin bloquear nunca a quien lo pide.
func (r *Registro) anotar(e Evento) {
	if r == nil || !tenancy.ValidSlug(e.Tenant) || e.Accion == "" {
		return
	}
	if e.Momento.IsZero() {
		e.Momento = r.ahora().UTC()
	}
	e.Accion = recortar(e.Accion, maxAccion)
	e.Objetivo = recortar(e.Objetivo, maxObjetivo)
	e.Actor = recortar(e.Actor, maxObjetivo)
	select {
	case r.cola <- e:
	default:
		r.perdidos.Add(1)
	}
}

// Regla dice qué petición, si termina bien, es un acceso que anotar.
type Regla struct {
	Metodo string // GET, POST…
	// Ruta es el patrón COMPLETO de chi tal como casa («/api/v1/files/{id}/content»),
	// sin barra final.
	Ruta string
	// Accion va sin el prefijo «acceso.» («file.download»).
	Accion string
	// Param es el parámetro de la ruta con el id del recurso; "" = ninguno.
	Param string
}

type claveRegistro struct{}

// peticion es lo que el middleware deja en el contexto para Anotar.
type peticion struct {
	reg       *Registro
	requestID string
	ip        string
}

// Middleware es el punto de estrangulamiento de la traza: envuelve el router
// ENTERO (httpx.RouterOptions.Traza), así que ve el patrón de la ruta que casó y,
// por authn.Testigo, quién se autenticó por dentro. Anota solo lo que termina
// bien (2xx y 3xx, salvo el 304: no se entregó nada) y, de una descarga por
// rangos, solo el primer trozo: un vídeo que se ve por partes es UNA descarga.
func (r *Registro) Middleware(reglas ...Regla) func(http.Handler) http.Handler {
	indice := make(map[string]Regla, len(reglas))
	for _, rg := range reglas {
		indice[rg.Metodo+" "+normalizarRuta(rg.Ruta)] = rg
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx, testigo := authn.ConTestigo(req.Context())
			p := &peticion{
				reg:       r,
				requestID: httpx.RequestIDFromContext(ctx),
				ip:        clientip.TrustedProxiesFrom(ctx).ClientIP(req.RemoteAddr, req.Header.Get("X-Forwarded-For")),
			}
			ctx = context.WithValue(ctx, claveRegistro{}, p)
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(ww, req.WithContext(ctx))

			estado := ww.Status()
			if estado == 0 {
				estado = http.StatusOK
			}
			if estado >= 400 || estado == http.StatusNotModified || !primerTrozo(req) {
				return
			}
			rctx := chi.RouteContext(ctx)
			if rctx == nil {
				return
			}
			regla, ok := indice[req.Method+" "+normalizarRuta(rctx.RoutePattern())]
			if !ok {
				return
			}
			claims, ok := testigo.Claims()
			if !ok {
				return
			}
			objetivo := ""
			if regla.Param != "" {
				objetivo = rctx.URLParam(regla.Param)
			}
			r.anotar(p.evento(claims.Tenant, claims.Subject, regla.Accion, objetivo, nil))
		})
	}
}

func (p *peticion) evento(tenant, actor, accion, objetivo string, detalle map[string]any) Evento {
	d := make(map[string]any, len(detalle)+2)
	for k, v := range detalle {
		d[k] = v
	}
	if p.requestID != "" {
		d["request_id"] = p.requestID
	}
	if p.ip != "" {
		d["ip"] = p.ip
	}
	return Evento{Tenant: tenant, Actor: actor, Accion: PrefijoAcceso + accion, Objetivo: objetivo, Detalle: d}
}

// Anotar deja un acceso con la identidad de la sesión del contexto, para lo que
// la ruta no dice (el recurso va en el cuerpo). Sin middleware o sin sesión, no
// hace nada: no es un error de quien atiende la petición.
func Anotar(ctx context.Context, accion, objetivo string, detalle map[string]any) {
	c, ok := authn.FromContext(ctx)
	if !ok {
		return
	}
	AnotarComo(ctx, c.Tenant, c.Subject, accion, objetivo, detalle)
}

// AnotarComo deja un acceso con tenant y actor explícitos: un enlace público, cuyo
// tenant solo se sabe al resolverlo y cuyo actor es el enlace («link:<id>»).
func AnotarComo(ctx context.Context, tenant, actor, accion, objetivo string, detalle map[string]any) {
	p, ok := ctx.Value(claveRegistro{}).(*peticion)
	if !ok || p == nil {
		return
	}
	p.reg.anotar(p.evento(tenant, actor, accion, objetivo, detalle))
}

// Retener borra cada `cada` la traza de acceso más vieja que RetencionAcceso. La
// traza de cumplimiento no la alcanza ni por la política de la tabla.
func (r *Registro) Retener(ctx context.Context, cada time.Duration) {
	if cada <= 0 {
		cada = 6 * time.Hour
	}
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		if n, err := PurgarAccesoVencido(ctx, r.db, r.ahora().Add(-RetencionAcceso)); err != nil && ctx.Err() == nil {
			r.log.ErrorContext(ctx, "traza de acceso: la retención falló", "error", err)
		} else if n > 0 {
			r.log.InfoContext(ctx, "traza de acceso: retención aplicada", "borrados", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PurgarAccesoVencido borra, en lotes, la traza de acceso anterior a `antes`, de
// todas las organizaciones (InMaintenanceTx: la política audit_log_retencion solo
// deja ver y borrar `acceso.*`).
func PurgarAccesoVencido(ctx context.Context, db pg.Database, antes time.Time) (int64, error) {
	var total int64
	for {
		var n int64
		err := db.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `
				DELETE FROM audit_log WHERE id IN (
				    SELECT id FROM audit_log
				    WHERE action LIKE 'acceso.%' AND occurred_at < $1
				    LIMIT $2)`, antes, loteRetencion)
			n = tag.RowsAffected()
			return err
		})
		if err != nil {
			return total, fmt.Errorf("auditlog: retención: %w", err)
		}
		total += n
		if n < loteRetencion {
			return total, nil
		}
	}
}

// primerTrozo dice si la petición es la descarga entera o su primer rango.
func primerTrozo(req *http.Request) bool {
	rango := req.Header.Get("Range")
	return rango == "" || strings.HasPrefix(strings.TrimSpace(rango), "bytes=0-")
}

func normalizarRuta(p string) string {
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

// Lectura es el GET /admin/audit-log del servicio, sobre la misma base en la que
// escribe el registro. Quién puede llamarlo (owner/admin) lo pone el montaje.
func (r *Registro) Lectura(servicio string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) { Servir(w, req, r.db, servicio) }
}

// ReglasSinRuta devuelve las reglas cuyo método y patrón no están en el router.
// Es para el test de cada servicio: una ruta renombrada dejaría su traza muda
// sin que nada fallara, que es el peor fallo que puede tener una auditoría.
func ReglasSinRuta(r chi.Routes, reglas []Regla) []Regla {
	existe := map[string]bool{}
	_ = chi.Walk(r, func(metodo, ruta string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		existe[metodo+" "+normalizarRuta(strings.ReplaceAll(ruta, "/*/", "/"))] = true
		return nil
	})
	var faltan []Regla
	for _, rg := range reglas {
		if !existe[rg.Metodo+" "+normalizarRuta(rg.Ruta)] {
			faltan = append(faltan, rg)
		}
	}
	return faltan
}

func recortar(s string, n int) string {
	for len(s) > n {
		_, ancho := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-ancho]
	}
	return s
}
