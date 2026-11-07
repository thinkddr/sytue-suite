// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package despertar despierta las apps nativas de iOS cuando cambia algo de su
// producto, para que vuelvan a leer lo suyo y repinten su widget con la app cerrada
// (ADR 0078 y 0039, enmiendas del 2026-11-06).
//
// La entrada es la señal de cambios (hubnotify.EventoDatos, `{"producto"}` de una
// lista cerrada) que los servicios ya mandan al Hub en cada escritura: el Hub la
// reparte por el SSE a las apps abiertas y, además, se la pasa aquí. Aquí se busca
// qué aparatos de la persona tienen la app de ese producto (el registro único de
// store.Aparato) y se les manda el aviso VACÍO de platform/pkg/apns.
//
// El tope —un despertar cada Ventana por aparato— vive en la base de datos y no en
// memoria, porque el Hub corre con varias réplicas: cada envío se RESERVA con un
// UPDATE … RETURNING (store.ReservarDespertar), y lo que llega dentro de la ventana
// queda pendiente para el barrido de cada minuto, que reserva igual.
package despertar

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/apns"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

const (
	// Ventana es lo mínimo entre dos despertares al mismo aparato. iOS castiga a quien
	// manda muchos pushes de fondo (los retrasa o los descarta), y un widget al día con
	// cinco minutos de margen es lo que el dueño pidió.
	Ventana = 5 * time.Minute
	// CadaBarrido es cada cuánto se miran los pendientes.
	CadaBarrido = time.Minute
	// LoteBarrido acota un barrido: lo que no quepa sale en el siguiente.
	LoteBarrido = 500
	// plazoEnvio es lo que espera un cambio por sus despertares.
	plazoEnvio = 20 * time.Second
	// maxPorCambio acota los aparatos que despierta un cambio, como el Web Push de Rooms.
	maxPorCambio = 50
)

// BundleHub es la app One (el Hub nativo): su widget enseña el correo sin leer, los
// eventos de hoy y la actividad reciente de la suite, así que se despierta también con
// los cambios de esos productos (deOne).
const BundleHub = "eu.sytue.one"

// deOne son los productos que salen en el widget de One (apps/nucleo/widgets/fuentes/hub.go):
// Mail y Calendar por sus contadores y la agenda, y los que llenan la actividad reciente
// del Hub (`/api/v1/recent`: los conectores de Bases, ERP, Notes y Drive, que es donde
// viven los ficheros de Docus, Calcus, Diapos, certiDocs y Photos). Contacts, Rooms y
// Cast no dan actividad reciente (su conector devuelve nada o no lo hay): un cambio
// suyo no mueve el widget de One y no lo despierta. Es la lista con que One republica
// con la app abierta (apps/nucleo/widgets/fuentes.ProductosDeHub, otro módulo):
// TestOneSeDespiertaConLoQueEnsenaSuWidget las ata.
var deOne = map[string]bool{
	"mail": true, "calendar": true,
	"bases": true, "drive": true, "erp": true, "notes": true,
	"docs": true, "sheets": true, "slides": true, "pdf": true, "photos": true,
}

// bundles es, por producto de la señal de cambios, la app nativa que lo enseña (el `ID`
// de su FyneApp.toml). TestLosBundlesSonLosDeLasApps la ata a las apps.
var bundles = map[string]string{
	"bases":    "eu.sytue.bases",
	"calendar": "eu.sytue.calendar",
	"cast":     "eu.sytue.cast",
	"contacts": "eu.sytue.contacts",
	"docs":     "eu.sytue.docus",
	"drive":    "eu.sytue.drive",
	"erp":      "eu.sytue.erp",
	"mail":     "eu.sytue.mail",
	"notes":    "eu.sytue.notes",
	"pdf":      "eu.sytue.certipdf",
	"photos":   "eu.sytue.photos",
	"rooms":    "eu.sytue.rooms",
	"sheets":   "eu.sytue.calcus",
	"slides":   "eu.sytue.diapos",
}

// BundlesDe son las apps que se despiertan con un cambio de `producto`; nada si el
// producto no es de la lista cerrada.
func BundlesDe(producto string) []string {
	b, ok := bundles[producto]
	if !ok || !hubnotify.ProductoDeDatos(producto) {
		return nil
	}
	if deOne[producto] {
		return []string{b, BundleHub}
	}
	return []string{b}
}

// BundleConocido dice si `b` es una de las apps de la suite que se pueden registrar.
func BundleConocido(b string) bool {
	if b == BundleHub {
		return true
	}
	for _, v := range bundles {
		if v == b {
			return true
		}
	}
	return false
}

// Almacen es lo que se usa del store (store.Store).
type Almacen interface {
	ReservarDespertar(ctx context.Context, subject string, bundles []string, ventana time.Duration) ([]store.Aparato, error)
	BarrerPendientes(ctx context.Context, ventana time.Duration, limite int) ([]store.Aparato, error)
	BorrarAparato(ctx context.Context, a store.Aparato) error
}

// Enviador manda el aviso vacío (apns.Enviador).
type Enviador interface {
	Despertar(ctx context.Context, topic, token string) error
}

// Despertador junta el registro y el enviador.
type Despertador struct {
	almacen  Almacen
	enviador Enviador
	// lanzar corre el envío de un cambio sin hacer esperar a quien lo avisa; las
	// pruebas lo hacen síncrono.
	lanzar func(func())
}

// Nuevo devuelve el despertador, o nil sin enviador (el Hub sin credenciales de
// Apple): entonces nadie se despierta y todo lo demás sigue igual.
func Nuevo(a Almacen, e Enviador) *Despertador {
	if a == nil || e == nil {
		return nil
	}
	return &Despertador{almacen: a, enviador: e, lanzar: func(f func()) { go f() }}
}

// Cambio es la señal de cambios de `producto` para `subject`: despierta lo que se
// pueda ya y deja pendiente lo demás. No espera ni falla, como la señal. A quien solo
// se conoce por su correo (hubnotify.PorCorreo) no se le despierta: el registro va
// por sujeto, y su widget se pone al día con el refresco periódico de iOS.
func (d *Despertador) Cambio(ctx context.Context, tenant, subject, producto string) {
	if d == nil {
		return
	}
	bs := BundlesDe(producto)
	subject = strings.TrimSpace(subject)
	if len(bs) == 0 || subject == "" || strings.HasPrefix(subject, hubnotify.PorCorreo("")) {
		return
	}
	t, err := tenancy.New(strings.TrimSpace(tenant))
	if err != nil {
		return
	}
	ctx = tenancy.Into(context.WithoutCancel(ctx), t)
	d.lanzar(func() {
		ctx, cancel := context.WithTimeout(ctx, plazoEnvio)
		defer cancel()
		aparatos, err := d.almacen.ReservarDespertar(ctx, subject, bs, Ventana)
		if err != nil {
			log.From(ctx).Warn("despertar: no se pudieron reservar los aparatos", "error", err)
			return
		}
		if len(aparatos) > maxPorCambio {
			aparatos = aparatos[:maxPorCambio]
		}
		d.enviar(ctx, aparatos)
	})
}

// Barrer despierta lo que quedó pendiente y ya ha salido de su ventana.
func (d *Despertador) Barrer(ctx context.Context) {
	if d == nil {
		return
	}
	aparatos, err := d.almacen.BarrerPendientes(ctx, Ventana, LoteBarrido)
	if err != nil {
		log.From(ctx).Warn("despertar: el barrido de pendientes falló", "error", err)
		return
	}
	d.enviar(ctx, aparatos)
}

// Correr barre cada CadaBarrido hasta que se cancele ctx.
func (d *Despertador) Correr(ctx context.Context) {
	if d == nil {
		return
	}
	t := time.NewTicker(CadaBarrido)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.Barrer(ctx)
		}
	}
}

// enviar manda el despertar a cada aparato. Uno que Apple ya no acepta se borra; los
// demás fallos se registran SIN el token, que es un secreto del aparato.
func (d *Despertador) enviar(ctx context.Context, aparatos []store.Aparato) {
	for _, a := range aparatos {
		err := d.enviador.Despertar(ctx, a.Bundle, a.Token)
		switch {
		case err == nil:
		case errors.Is(err, apns.ErrTokenCaduco):
			if err := d.almacen.BorrarAparato(ctx, a); err != nil {
				log.From(ctx).Warn("despertar: no se pudo borrar un token caducado", "bundle", a.Bundle, "error", err)
			}
		default:
			log.From(ctx).Warn("despertar: Apple no aceptó el despertar", "bundle", a.Bundle, "error", err)
		}
	}
}
