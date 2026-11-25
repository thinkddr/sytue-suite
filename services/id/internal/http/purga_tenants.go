// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
)

// PURGA DEL CONTENIDO DE UNA ORGANIZACIÓN QUE DESAPARECE (ADR 0054, enmienda del
// 2026-11-25).
//
// Lo que antes era «borrado manual, avisado a nivel error», ahora lo hace este
// barrido: por cada organización de la cola (mig 0129) recorre las bases de
// merge.Services y en cada una
//
//  1. borra las filas del tenant (pg.PurgarTenant: mismas credenciales y mismo
//     camino que la fusión, sin endpoints internos ni secretos nuevos), y
//  2. cuando la base ya no guarda nada, espera a que el propio servicio borre
//     sus objetos (platform/pkg/purgatenant), que es quien tiene la clave del
//     almacén.
//
// Un servicio terminado no se vuelve a tocar; los demás se reintentan con espera
// creciente. Lo retenido —una retención legal activa, un guardián de integridad
// como el libro diario del ERP— no se fuerza: se deja escrito con su motivo y va
// al log a nivel error, que es lo que dispara la alerta, porque el plazo de un
// mes del art. 12.3 RGPD sigue corriendo.

const (
	lotePurgas = 5
	// arriendoPurga es cuánto se reserva una fila mientras se trabaja en ella:
	// de sobra para dieciséis bases.
	arriendoPurga = 30 * time.Minute
	// esperaObjetos es cuándo se vuelve a mirar una purga que solo espera a que
	// un servicio borre sus objetos (su trabajador mira cada cinco minutos).
	esperaObjetos = BarridoCuentasBorradas
	// esperaMaxPurga acota la espera creciente de una purga que falla.
	esperaMaxPurga = 24 * time.Hour
)

// abridorDeBases abre la base de un servicio y devuelve con qué cerrarla. En
// producción deriva el DSN del de Sytue ID (merge.DSNForDatabase); los tests lo
// sustituyen por sus bases efímeras.
type abridorDeBases func(ctx context.Context, servicio string) (pg.Database, func(), error)

// BarrerPurgasDeTenant atiende las purgas de organizaciones cuyo turno ha
// llegado. Idempotente y seguro con varias réplicas: cada fila se reclama con
// arriendo (store.ReclamarPurgasDeTenant).
func (s *Server) BarrerPurgasDeTenant(ctx context.Context) error {
	pend, err := s.store.ReclamarPurgasDeTenant(ctx, lotePurgas, arriendoPurga)
	if err != nil || len(pend) == 0 {
		return err
	}
	abrir := s.abrirBaseDeServicio
	if s.basesDeServicio != nil {
		abrir = s.basesDeServicio
	}
	for _, p := range pend {
		prog, completa, resumen := purgarTenant(ctx, p, abrir, merge.Services, s.log)
		espera := esperaObjetos
		if resumen.fallos > 0 {
			espera = esperaPurga(p.Intentos)
		}
		if err := s.store.GuardarPurgaDeTenant(ctx, p.OrgID, prog, completa, espera, resumen.error()); err != nil {
			return err
		}
		switch {
		case completa:
			s.log.InfoContext(ctx, "purga de organización terminada", "org_id", p.OrgID, "tenant", p.Slug, "filas", resumen.filas)
			_ = s.store.Audit(ctx, nil, store.AuditEntry{
				Action: "org.content.purged", Target: p.OrgID,
				Detail: map[string]any{"reason": p.Motivo, "rows": resumen.filas, "requested_at": p.Solicitada.UTC().Format(time.RFC3339)},
			})
		case resumen.fallos > 0:
			// Error y no aviso: mientras falle, el contenido sigue ahí y el plazo
			// del RGPD corre.
			s.log.ErrorContext(ctx, "purga de organización: hay servicios que no se pudieron purgar",
				"org_id", p.OrgID, "tenant", p.Slug, "intentos", p.Intentos, "detalle", resumen.error())
		default:
			s.log.InfoContext(ctx, "purga de organización: esperando a que los servicios borren sus objetos",
				"org_id", p.OrgID, "tenant", p.Slug, "servicios", strings.Join(resumen.esperando, ","))
		}
	}
	return nil
}

// resumenPurga cuenta lo que pasó en un intento, para el log y la cola.
type resumenPurga struct {
	filas     int
	fallos    int
	detalles  []string
	esperando []string
}

func (r resumenPurga) error() string { return strings.Join(r.detalles, "; ") }

// purgarTenant hace UN intento sobre todas las bases. Devuelve el progreso
// nuevo y si ya no queda nada en ninguna.
func purgarTenant(ctx context.Context, p store.PurgaDeTenant, abrir abridorDeBases, servicios []string, log *slog.Logger) (map[string]store.ProgresoServicio, bool, resumenPurga) {
	prog := make(map[string]store.ProgresoServicio, len(servicios))
	for k, v := range p.Progreso {
		prog[k] = v
	}
	var r resumenPurga
	completa := true
	for _, svc := range servicios {
		e := prog[svc]
		if e.Hecho {
			continue
		}
		e = purgarEnServicio(ctx, p.Slug, svc, e, abrir)
		ahora := time.Now().UTC()
		e.Actualizado = &ahora
		prog[svc] = e
		if !e.Hecho {
			completa = false
		}
		switch {
		case e.Error != "":
			r.fallos++
			r.detalles = append(r.detalles, svc+": "+e.Error)
		case e.Retenido != "":
			r.fallos++
			r.detalles = append(r.detalles, svc+": retenido: "+e.Retenido)
		case e.EsperaObjetos:
			r.esperando = append(r.esperando, svc)
		}
		if log != nil && e.Error != "" {
			log.WarnContext(ctx, "purga de organización: un servicio falló", "tenant", p.Slug, "servicio", svc, "error", e.Error)
		}
	}
	for _, e := range prog {
		r.filas += e.Filas
	}
	sort.Strings(r.detalles)
	return prog, completa, r
}

// purgarEnServicio avanza la purga en UNA base todo lo que se pueda ahora.
func purgarEnServicio(ctx context.Context, slug, svc string, e store.ProgresoServicio, abrir abridorDeBases) store.ProgresoServicio {
	e.Error, e.Retenido, e.EsperaObjetos = "", "", false
	db, cerrar, err := abrir(ctx, svc)
	if err != nil {
		if baseInexistente(err) {
			// Un servicio que este despliegue no tiene (Sytue Bases apagada) no
			// guarda nada de nadie.
			e.SinBase, e.BaseVacia, e.Hecho = true, true, true
			return e
		}
		e.Error = "sin conexión: " + err.Error()
		return e
	}
	defer cerrar()
	if !e.BaseVacia {
		// El contrato de escritura de Notepads, como en la fusión: inocuo en las
		// demás bases y obligatorio para borrar en la suya.
		res, err := pg.PurgarTenant(ctx, db, slug, notesdb.MarkWriteTransaction)
		e.Filas += res.Total
		var ret *pg.PurgaRetenidaError
		switch {
		case errors.As(err, &ret):
			e.Retenido = ret.Motivo
			return e
		case err != nil:
			e.Error = err.Error()
			return e
		case !res.Completa():
			partes := make([]string, 0, len(res.Retenidas))
			for _, t := range res.Retenidas {
				partes = append(partes, fmt.Sprintf("%s (%d filas: %s)", t.Tabla, t.Filas, t.Motivo))
			}
			e.Retenido = strings.Join(partes, ", ")
			return e
		}
		e.BaseVacia = true
	}
	est, err := pg.ConsultarPurgaDeObjetos(ctx, db, slug)
	if err != nil {
		e.Error = "estado de los objetos: " + err.Error()
		return e
	}
	if !est.Hecho() {
		e.EsperaObjetos = true
		if est.UltimoErr != "" {
			e.Error = "objetos: " + est.UltimoErr
		}
		return e
	}
	e.Hecho = true
	return e
}

// esperaPurga es la espera creciente tras un intento fallido: diez minutos,
// veinte, cuarenta... hasta un día.
func esperaPurga(intentos int) time.Duration {
	espera := BarridoCuentasBorradas
	for i := 1; i < intentos && espera < esperaMaxPurga; i++ {
		espera *= 2
	}
	return min(espera, esperaMaxPurga)
}

// abrirBaseDeServicio deriva el DSN de la base del servicio del de Sytue ID, como
// la fusión y el borrado de datos personales.
func (s *Server) abrirBaseDeServicio(ctx context.Context, servicio string) (pg.Database, func(), error) {
	dsn, err := merge.DSNForDatabase(s.cfg.DatabaseDSN, servicio)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
	if err != nil {
		return nil, nil, err
	}
	return pool, pool.Close, nil
}

// baseInexistente reconoce «esa base no existe», directo (3D000) o a través de
// PgBouncer, que lo dice con su propio mensaje.
func baseInexistente(err error) bool {
	var pge *pgconn.PgError
	if errors.As(err, &pge) && pge.Code == "3D000" {
		return true
	}
	return strings.Contains(err.Error(), "no such database")
}
