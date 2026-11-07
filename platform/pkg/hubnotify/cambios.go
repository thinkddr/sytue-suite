// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// # La señal de cambios
//
// EventoDatos dice a las apps abiertas de una persona que algo de UN producto ha
// cambiado —un evento creado en la web, una foto que terminó de procesarse, una firma
// que otra persona completó— para que su widget nativo vuelva a leer lo suyo (ADR
// 0078, enmienda del 2026-11-06). Datos: {"producto": "<producto>"} y nada más: sin
// título, sin id, sin contenido. Quien lo recibe ya sabe pedir lo que necesita.
//
// Lo emiten los servicios con AvisarCambio: en cada escritura con éxito, a quien
// escribe (TrasEscribir) y a quienes ven lo escrito (AvisarTambien), y al terminar un
// trabajo asíncrono, al dueño de lo procesado.
const EventoDatos = "datos"

// VentanaCambios agrupa la señal por (organización, persona, producto): la primera
// sale en el acto y las que lleguen dentro de la ventana salen en UNA sola al cerrarla.
// Una ráfaga —subir cuarenta fotos, mover un hilo de veinte correos— no manda veinte.
//
// ponytail: en memoria, o sea POR RÉPLICA: con N réplicas pasan como mucho N por
// ventana. Es un ahorro, no un tope de seguridad; el que importa (un push por aparato
// cada 5 min) va en la base de datos del Hub.
const VentanaCambios = 3 * time.Second

// productosDeDatos es la lista CERRADA de productos de la señal: los que tienen widget
// nativo, con el nombre del esquema de su app (`sytue-<producto>`, `FyneApp.toml`), que
// es con el que cada app se reconoce. El Hub rechaza cualquier otro, y
// TestLosProductosDeLaSenalSonLosDeLasApps la ata a las apps.
var productosDeDatos = map[string]bool{
	"bases": true, "calendar": true, "cast": true, "contacts": true, "docs": true,
	"drive": true, "erp": true, "mail": true, "notes": true, "pdf": true,
	"photos": true, "rooms": true, "sheets": true, "slides": true,
}

// ProductoDeDatos dice si `p` está en la lista cerrada de la señal.
func ProductoDeDatos(p string) bool { return productosDeDatos[p] }

// ProductosDeDatos es la lista cerrada, ordenada.
func ProductosDeDatos() []string {
	out := make([]string, 0, len(productosDeDatos))
	for p := range productosDeDatos {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// DatosCambio es el cuerpo de EventoDatos.
type DatosCambio struct {
	Producto string `json:"producto"`
}

// AvisarCambio manda EventoDatos de `producto` a cada una de `subjects` (un sujeto o
// un PorCorreo: el Hub lo reparte igual que los avisos), agrupado por VentanaCambios.
// No espera ni falla: es best effort, como SendAsync. Un producto fuera de la lista es
// un error de programación y se registra.
func (c *Client) AvisarCambio(ctx context.Context, tenant, producto string, subjects ...string) {
	if c == nil {
		return
	}
	if !ProductoDeDatos(producto) {
		log.From(ctx).Warn("hubnotify: producto fuera de la lista de la señal de cambios", "producto", producto)
		return
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return
	}
	vistos := map[string]bool{}
	for _, s := range subjects {
		s = strings.TrimSpace(s)
		if s == "" || s == PorCorreo("") || vistos[s] {
			continue
		}
		vistos[s] = true
		c.cambios.avisar(claveCambio{tenant: tenant, subject: s, producto: producto})
	}
}

func (c *Client) enviarCambio(k claveCambio) {
	e := Evento{Tenant: k.tenant, Subject: k.subject, Event: EventoDatos, Data: DatosCambio{Producto: k.producto}}
	c.async(context.Background(), func(ctx context.Context) error { return c.SendEvent(ctx, e) },
		"event", EventoDatos, "producto", k.producto)
}

type claveCambio struct{ tenant, subject, producto string }

// agrupador es la ventana de AvisarCambio. `abiertas` guarda, por clave con la
// ventana abierta, si ha llegado otra señal desde que salió la última.
type agrupador struct {
	ventana  time.Duration
	tras     func(time.Duration, func())
	enviar   func(claveCambio)
	mu       sync.Mutex
	abiertas map[claveCambio]bool
}

func nuevoAgrupador(enviar func(claveCambio)) *agrupador {
	return &agrupador{
		ventana:  VentanaCambios,
		tras:     func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		enviar:   enviar,
		abiertas: map[claveCambio]bool{},
	}
}

func (a *agrupador) avisar(k claveCambio) {
	a.mu.Lock()
	if _, abierta := a.abiertas[k]; abierta {
		a.abiertas[k] = true
		a.mu.Unlock()
		return
	}
	a.abiertas[k] = false
	a.mu.Unlock()
	a.enviar(k)
	a.tras(a.ventana, func() { a.cerrar(k) })
}

// cerrar acaba la ventana de `k`: si llegó algo dentro, sale una señal y se abre otra.
func (a *agrupador) cerrar(k claveCambio) {
	a.mu.Lock()
	if !a.abiertas[k] {
		delete(a.abiertas, k)
		a.mu.Unlock()
		return
	}
	a.abiertas[k] = false
	a.mu.Unlock()
	a.enviar(k)
	a.tras(a.ventana, func() { a.cerrar(k) })
}

// cambioPeticion es lo que una escritura va sabiendo de su señal mientras se atiende:
// quién más ve lo escrito y, si el handler lo precisa, de qué producto es.
type cambioPeticion struct {
	mu        sync.Mutex
	productos []string
	otros     []string
}

type claveCtxCambio struct{}

// TrasEscribir es el middleware de la señal: tras cada escritura (httpx.EsEscritura)
// que el handler contestó con un 2xx, manda EventoDatos de `productos` a quien la
// hizo y a quienes añadió AvisarTambien. Va DESPUÉS de RequireAuth: sin claims no
// avisa a nadie. `saltar` (puede ser nil) deja fuera las rutas que escriben sin cambiar
// lo que enseña un widget: una búsqueda por POST, un borrador que se autoguarda, la
// subida de un adjunto que aún no está en ningún correo. Con el cliente nil (Hub sin
// configurar) no envuelve nada.
func (c *Client) TrasEscribir(saltar func(*http.Request) bool, productos ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if c == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !httpx.EsEscritura(r.Method) || (saltar != nil && saltar(r)) {
				next.ServeHTTP(w, r)
				return
			}
			cp := &cambioPeticion{productos: productos}
			ctx := context.WithValue(r.Context(), claveCtxCambio{}, cp)
			if estado := httpx.ServirConEstado(next, w, r.WithContext(ctx)); estado < 200 || estado > 299 {
				return
			}
			claims, ok := authn.FromContext(ctx)
			if !ok {
				return
			}
			tenant := claims.Tenant
			if t, err := tenancy.From(ctx); err == nil {
				tenant = t.Slug
			}
			cp.mu.Lock()
			ps, para := cp.productos, append([]string{claims.Subject}, cp.otros...)
			cp.mu.Unlock()
			for _, p := range ps {
				c.AvisarCambio(ctx, tenant, p, para...)
			}
		})
	}
}

// AvisarTambien añade, a la señal de la escritura en curso, a quienes también ven lo
// escrito: el dueño de un calendario compartido y quienes lo comparten, por su sujeto
// o por PorCorreo. Fuera de una petición de TrasEscribir no hace nada, así que los
// servicios lo llaman en su verja de escritura sin preguntar quién los ha llamado.
func AvisarTambien(ctx context.Context, subjects ...string) {
	if cp, ok := ctx.Value(claveCtxCambio{}).(*cambioPeticion); ok {
		cp.mu.Lock()
		cp.otros = append(cp.otros, subjects...)
		cp.mu.Unlock()
	}
}

// CambioEn precisa de qué productos es la escritura en curso, cuando la ruta no lo
// dice: un documento es de Docus, Calcus o Diapos según su tipo. Sin productos, la
// petición no lleva señal: un POST que resultó no escribir nada (un lote JMAP que
// solo lee). Fuera de TrasEscribir no hace nada.
func CambioEn(ctx context.Context, productos ...string) {
	if cp, ok := ctx.Value(claveCtxCambio{}).(*cambioPeticion); ok {
		cp.mu.Lock()
		cp.productos = productos
		cp.mu.Unlock()
	}
}

// EnEscritura dice si `ctx` es el de una escritura que lleva la señal (TrasEscribir):
// para no consultar quién ve un recurso cuando nadie va a avisar.
func EnEscritura(ctx context.Context) bool {
	_, ok := ctx.Value(claveCtxCambio{}).(*cambioPeticion)
	return ok
}
