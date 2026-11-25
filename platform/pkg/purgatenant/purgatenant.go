// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package purgatenant es el trabajador que borra los OBJETOS de un tenant
// purgado, dentro de cada servicio que guarda objetos (ADR 0054, enmienda del
// 2026-11-25).
//
// El reparto es este: Sytue ID borra las filas de cada base (pg.PurgarTenant) y,
// cuando una base ya no guarda nada del tenant, deja la orden en su cola
// (`platform_tenant_purges`). Este trabajador, que corre en el servicio y es el
// único con la clave de su almacén, la cumple: borra `tenants/<slug>/`, los
// prefijos que el servicio declare y las claves sueltas de la orden, y la marca
// hecha. Sytue ID lee la cola para saber cuándo ha terminado.
//
// Por qué no lo hace Sytue ID: tendría que tener la clave del almacén de cada
// servicio, y una identidad con permiso de borrar en todos los buckets es
// exactamente lo que no se quiere tener. La base ya la alcanza con las
// credenciales de la fusión; el almacén, no.
package purgatenant

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Almacen es lo que el trabajador necesita del almacén del servicio
// (*objectstore.Client lo cumple).
type Almacen interface {
	BorrarTenant(ctx context.Context, t tenancy.Tenant) (int, error)
	BorrarPrefijo(ctx context.Context, prefijo string) (int, error)
	BorrarClave(ctx context.Context, clave string) error
}

// SinAlmacen es el almacén de un servicio que arranca sin S3 (Docs lo admite):
// no hay objetos que borrar, así que cada orden se cumple sin hacer nada. Sin
// él, la purga de esa base esperaría para siempre a un trabajador que no existe.
type SinAlmacen struct{}

// BorrarTenant no hace nada: sin almacén no hay objetos.
func (SinAlmacen) BorrarTenant(context.Context, tenancy.Tenant) (int, error) { return 0, nil }

// BorrarPrefijo no hace nada: sin almacén no hay objetos.
func (SinAlmacen) BorrarPrefijo(context.Context, string) (int, error) { return 0, nil }

// BorrarClave no hace nada: sin almacén no hay objetos.
func (SinAlmacen) BorrarClave(context.Context, string) error { return nil }

// Trabajador cumple las órdenes de borrado de objetos de UN servicio.
type Trabajador struct {
	DB      pg.Database
	Almacen Almacen
	// Prefijos son los prefijos COMPLETOS que, además de `tenants/<slug>/`, son
	// de ese tenant (Mail: `shared/mensajes/<slug>/`). nil = ninguno.
	Prefijos func(slug string) []string
	Log      *slog.Logger
	// Lote y Reintento acotan cada pasada; cero = 10 órdenes y 15 minutos.
	Lote      int
	Reintento time.Duration
}

// Cada cuánto mira la cola Ejecutar: una purga llega como mucho una vez cada
// diez minutos (el barrido de Sytue ID) y su plazo es de un mes.
const Cada = 5 * time.Minute

// Ejecutar repite Pasada cada `cada` hasta que se cancela ctx. Sus fallos no
// paran el servicio: van al log a nivel error, que es lo que dispara la alerta.
func (w Trabajador) Ejecutar(ctx context.Context, cada time.Duration) {
	if cada <= 0 {
		cada = Cada
	}
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		if _, err := w.Pasada(ctx); err != nil && ctx.Err() == nil {
			w.log().ErrorContext(ctx, "purga de tenant: no se pudo leer la cola de objetos", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Pasada reclama las órdenes pendientes y las cumple. Devuelve cuántas cerró.
// Una orden que falla se queda abierta con su error y se reintenta pasado
// Reintento: borrar lo que ya no existe no es un error, así que repetir es
// seguro.
func (w Trabajador) Pasada(ctx context.Context) (int, error) {
	if w.DB == nil || w.Almacen == nil {
		return 0, errors.New("purgatenant: falta la base o el almacén")
	}
	lote, reintento := w.Lote, w.Reintento
	if lote <= 0 {
		lote = 10
	}
	if reintento <= 0 {
		reintento = 15 * time.Minute
	}
	ordenes, err := pg.ReclamarPurgasDeObjetos(ctx, w.DB, lote, reintento)
	if err != nil {
		return 0, err
	}
	hechas := 0
	for _, o := range ordenes {
		n, err := w.cumplir(ctx, o)
		if err != nil {
			if ctx.Err() != nil {
				return hechas, ctx.Err()
			}
			w.log().ErrorContext(ctx, "purga de tenant: no se pudieron borrar sus objetos",
				"tenant", o.Slug, "borrados", n, "error", err)
			if e := pg.AnotarFalloPurgaDeObjetos(ctx, w.DB, o.Slug, err); e != nil {
				return hechas, e
			}
			continue
		}
		if err := pg.MarcarObjetosPurgados(ctx, w.DB, o); err != nil {
			return hechas, err
		}
		hechas++
		w.log().InfoContext(ctx, "purga de tenant: objetos borrados", "tenant", o.Slug, "borrados", n)
	}
	return hechas, nil
}

func (w Trabajador) cumplir(ctx context.Context, o pg.PurgaDeObjetos) (int, error) {
	t, err := tenancy.New(o.Slug)
	if err != nil {
		return 0, err
	}
	total, err := w.Almacen.BorrarTenant(ctx, t)
	errs := []error{err}
	if w.Prefijos != nil {
		for _, p := range w.Prefijos(o.Slug) {
			n, err := w.Almacen.BorrarPrefijo(ctx, p)
			total += n
			errs = append(errs, err)
		}
	}
	for _, k := range o.Claves {
		// Una clave acabada en `/` es un prefijo que el gancho de la base anotó
		// antes de borrar las filas que lo nombraban (las webs de Sytue Web viven
		// en `shared/sitios/<id>/`, y el id solo estaba en la fila).
		if strings.HasSuffix(k, "/") {
			n, err := w.Almacen.BorrarPrefijo(ctx, k)
			total += n
			errs = append(errs, err)
			continue
		}
		if err := w.Almacen.BorrarClave(ctx, k); err != nil {
			errs = append(errs, err)
			continue
		}
		total++
	}
	return total, errors.Join(errs...)
}

func (w Trabajador) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}
