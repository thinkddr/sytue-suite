// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// El barrido periódico recoge lo que el camino normal deja atrás:
//
//   - Los vídeos borrados. Borrar marca `deleted_at` y purga en segundo plano,
//     pero esa purga puede fallar o perderse con el proceso. Pasado el plazo de
//     gracia, el barrido vuelve a purgar su prefijo y, si el almacén contesta que
//     sí, elimina la fila (y por cascada lo que cuelga: personas, trabajos,
//     espectadores, comentarios, chat). Si no, la fila se queda y la siguiente
//     pasada lo reintenta: la fila ES el recordatorio.
//   - Las subidas que no terminaron: una copia desde Drive o una subida cortada
//     por un reinicio dejan el vídeo en `uploading` para siempre. Pasado el
//     plazo sin cambios, pasa a `failed` con el motivo y se purga lo que hubiera
//     llegado al almacén.
//
// Corre en el proceso `cast`, no en el transcodificador: es donde viven Borrar y
// la limpieza del chat, y el transcodificador es el que se dimensiona por ffmpeg
// y se reinicia por su latido. Las réplicas se turnan con un advisory lock
// (store.ConCerrojoDelBarrido): una pasada a la vez en todo el despliegue.

// Barrido son los plazos del barrido (config: CAST_SWEEP_*).
type Barrido struct {
	// Gracia es cuánto se conserva la fila de un vídeo borrado antes de purgarlo
	// del todo. 0 o negativo = no se retira ninguno (defensa: una configuración a
	// cero no vacía los borrados de ayer).
	Gracia time.Duration
	// Subida es cuánto puede estar un vídeo en `uploading` sin cambiar antes de
	// darlo por fallido. 0 o negativo = no se caduca ninguno.
	Subida time.Duration
	// Tanda es cuántos vídeos se leen de una vez. 0 = tandaDelBarrido.
	Tanda int
	// Calidades es hasta cuántos trabajos de calidades puede haber en cola a la
	// vez por el relleno de los vídeos de antes de la escalera WebM (decisión del
	// dueño, 2026-09-28: todos). En cada pasada se completa la cola hasta ese
	// número, así que todos acaban hechos sin tapar al transcodificador lo que
	// sube la gente. 0 o negativo = no se rellena.
	Calidades int
}

// tandaDelBarrido acota cada lectura: la pasada avanza por tandas y nunca tiene
// en memoria más que una.
const tandaDelBarrido = 100

// ResultadoBarrido es lo que hizo una pasada.
type ResultadoBarrido struct {
	// Ocupado: otra réplica tenía el cerrojo y esta no hizo nada.
	Ocupado bool
	// Retirados: vídeos borrados purgados y eliminados de la base.
	Retirados int
	// Caducadas: subidas pasadas a `failed`.
	Caducadas int
	// PurgasFallidas: vídeos cuyo prefijo el almacén no quiso borrar. Los
	// borrados se reintentan en la pasada siguiente.
	PurgasFallidas int
	// Organizaciones a las que se volvió a declarar la cuota.
	Organizaciones int
	// Calidades: trabajos de calidades encolados para vídeos de antes.
	Calidades int
}

// Algo dice si la pasada tocó algo (para no llenar el registro de pasadas vacías).
func (r ResultadoBarrido) Algo() bool {
	return r.Retirados+r.Caducadas+r.PurgasFallidas+r.Calidades > 0
}

// datosDelBarrido es lo que el barrido usa del store; en las pruebas
// unitarias, un doble.
type datosDelBarrido interface {
	ConCerrojoDelBarrido(ctx context.Context, fn func(ctx context.Context) error) (bool, error)
	BorradosCaducados(ctx context.Context, gracia time.Duration, desde *store.Cursor, limite int) ([]store.Borrado, error)
	Eliminar(ctx context.Context, id uuid.UUID) error
	CaducarSubidas(ctx context.Context, plazo time.Duration, limite int) ([]domain.Video, error)
	CalidadesEnCola(ctx context.Context) (int, error)
	EncolarCalidadesPendientes(ctx context.Context, limite int) (int, error)
}

// Barrer hace una pasada, si ninguna otra réplica la está haciendo.
func (s *Servicio) Barrer(ctx context.Context, b Barrido) (ResultadoBarrido, error) {
	return s.barrer(ctx, s.store, b)
}

func (s *Servicio) barrer(ctx context.Context, d datosDelBarrido, b Barrido) (ResultadoBarrido, error) {
	var r ResultadoBarrido
	if s.objetos == nil {
		// Sin almacén no se puede comprobar que no quede nada: no se elimina ninguna fila.
		return r, nil
	}
	if b.Tanda <= 0 {
		b.Tanda = tandaDelBarrido
	}
	tenants := map[string]struct{}{}
	mio, err := d.ConCerrojoDelBarrido(ctx, func(ctx context.Context) error {
		if err := s.caducarSubidas(ctx, d, b, &r, tenants); err != nil {
			return err
		}
		if err := s.retirarBorrados(ctx, d, b, &r, tenants); err != nil {
			return err
		}
		return s.rellenarCalidades(ctx, d, b, &r)
	})
	if !mio && err == nil {
		r.Ocupado = true
		return r, nil
	}
	// Lo que se tocó se declara aunque la pasada se cortara a medias.
	orden := make([]string, 0, len(tenants))
	for t := range tenants {
		orden = append(orden, t)
	}
	slices.Sort(orden)
	fin := context.WithoutCancel(ctx)
	for _, t := range orden {
		s.declararCuota(fin, t)
	}
	r.Organizaciones = len(orden)
	return r, err
}

// rellenarCalidades completa la cola de trabajos de calidades hasta b.Calidades
// con vídeos de antes de la escalera WebM. No declara cuota: la declara el
// trabajo al terminar, con lo que de verdad ocupa.
func (s *Servicio) rellenarCalidades(ctx context.Context, d datosDelBarrido, b Barrido, r *ResultadoBarrido) error {
	if b.Calidades <= 0 {
		return nil
	}
	enCola, err := d.CalidadesEnCola(ctx)
	if err != nil {
		return err
	}
	if enCola >= b.Calidades {
		return nil
	}
	n, err := d.EncolarCalidadesPendientes(ctx, b.Calidades-enCola)
	r.Calidades = n
	return err
}

// caducarSubidas pasa a `failed` las subidas atascadas, tanda a tanda, y purga lo
// que dejaron. La purga que falla no se reintenta aquí: el vídeo ya no está en
// `uploading`; lo recoge Borrar cuando su dueño lo quite (y, si también falla,
// este mismo barrido tras el plazo de gracia).
func (s *Servicio) caducarSubidas(ctx context.Context, d datosDelBarrido, b Barrido, r *ResultadoBarrido, tenants map[string]struct{}) error {
	if b.Subida <= 0 {
		return nil
	}
	for ctx.Err() == nil {
		vs, err := d.CaducarSubidas(ctx, b.Subida, b.Tanda)
		if err != nil {
			return err
		}
		for _, v := range vs {
			r.Caducadas++
			tenants[v.Tenant] = struct{}{}
			if err := s.borrarObjetos(ctx, v); err != nil {
				r.PurgasFallidas++
				s.logger.Warn("cast/barrido: no se pudo purgar la subida caducada", "video", v.ID, "tenant", v.Tenant, "error", err)
				continue
			}
			s.logger.Info("cast/barrido: subida caducada", "video", v.ID, "tenant", v.Tenant)
		}
		if len(vs) < b.Tanda {
			return nil
		}
	}
	return ctx.Err()
}

// retirarBorrados purga y elimina los vídeos borrados hace más de la gracia. El
// cursor avanza aunque una purga falle, así que una fila que no se deja purgar
// no atasca la pasada: se queda para la siguiente.
func (s *Servicio) retirarBorrados(ctx context.Context, d datosDelBarrido, b Barrido, r *ResultadoBarrido, tenants map[string]struct{}) error {
	if b.Gracia <= 0 {
		return nil
	}
	var desde *store.Cursor
	for ctx.Err() == nil {
		bs, err := d.BorradosCaducados(ctx, b.Gracia, desde, b.Tanda)
		if err != nil {
			return err
		}
		for _, x := range bs {
			desde = x.Cursor()
			if err := s.borrarObjetos(ctx, domain.Video{ID: x.ID, Tenant: x.Tenant}); err != nil {
				r.PurgasFallidas++
				s.logger.Warn("cast/barrido: no se pudo purgar el vídeo borrado; se reintenta en la próxima pasada",
					"video", x.ID, "tenant", x.Tenant, "error", err)
				continue
			}
			switch err := d.Eliminar(ctx, x.ID); {
			case errors.Is(err, domain.ErrNoExiste):
				// Con el cerrojo nadie más retira filas: si no se pudo borrar, lo más
				// probable es que falte la política de 0006. Que se vea.
				s.logger.Warn("cast/barrido: la fila del vídeo borrado no se dejó eliminar", "video", x.ID, "tenant", x.Tenant)
				continue
			case err != nil:
				return err
			}
			r.Retirados++
			tenants[x.Tenant] = struct{}{}
		}
		if len(bs) < b.Tanda {
			return nil
		}
	}
	return ctx.Err()
}

// BucleDeBarrido corre Barrer al arrancar y luego cada `cada`, hasta que el
// contexto termine. Para `cmd/cast`.
func (s *Servicio) BucleDeBarrido(ctx context.Context, cada time.Duration, b Barrido) {
	tic := time.NewTicker(cada)
	defer tic.Stop()
	for {
		r, err := s.Barrer(ctx, b)
		switch {
		case err != nil && ctx.Err() == nil:
			s.logger.Warn("cast/barrido: la pasada falló", "error", err,
				"retirados", r.Retirados, "caducadas", r.Caducadas, "purgas_fallidas", r.PurgasFallidas)
		case r.Algo():
			s.logger.Info("cast/barrido: pasada hecha",
				"retirados", r.Retirados, "caducadas", r.Caducadas, "purgas_fallidas", r.PurgasFallidas,
				"organizaciones", r.Organizaciones, "calidades_encoladas", r.Calidades)
		}
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
		}
	}
}
