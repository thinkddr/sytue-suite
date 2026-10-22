// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Dobles de las dos dependencias de fuera: el almacén (en memoria, con la misma
// regla de prefijo por organización que el real) y la cola (para probar el bucle
// sin Postgres; el test de integración usa la store real).

type objetoFalso struct {
	datos []byte
	tipo  string
}

// almacenFalso cumple Objetos en memoria.
type almacenFalso struct {
	mu   sync.Mutex
	objs map[string]objetoFalso
}

func nuevoAlmacenFalso() *almacenFalso { return &almacenFalso{objs: map[string]objetoFalso{}} }

func claveCompleta(t tenancy.Tenant, key string) string {
	return t.StoragePrefix() + strings.TrimPrefix(key, "/")
}

func (a *almacenFalso) Get(_ context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.objs[claveCompleta(t, key)]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(o.datos)), nil
}

func (a *almacenFalso) GetRange(_ context.Context, t tenancy.Tenant, key, rango string) (*objectstore.Objeto, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.objs[claveCompleta(t, key)]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	total := int64(len(o.datos))
	out := &objectstore.Objeto{Body: io.NopCloser(bytes.NewReader(o.datos)), Size: total, Total: total, ContentType: o.tipo, LastModified: time.Now()}
	if spec, ok := strings.CutPrefix(rango, "bytes="); ok {
		a, b, _ := strings.Cut(spec, "-")
		ini, _ := strconv.ParseInt(a, 10, 64)
		fin, err := strconv.ParseInt(b, 10, 64)
		if err != nil || fin >= total {
			fin = total - 1
		}
		if ini > fin {
			return nil, objectstore.ErrRangoInvalido
		}
		out.Body = io.NopCloser(bytes.NewReader(o.datos[ini : fin+1]))
		out.Size = fin - ini + 1
		out.ContentRange = fmt.Sprintf("bytes %d-%d/%d", ini, fin, total)
	}
	return out, nil
}

func (a *almacenFalso) Put(_ context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error {
	datos, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if size >= 0 && int64(len(datos)) != size {
		return fmt.Errorf("almacén falso: %q anuncia %d bytes y trae %d", key, size, len(datos))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objs[claveCompleta(t, key)] = objetoFalso{datos: datos, tipo: opts.ContentType}
	return nil
}

func (a *almacenFalso) Delete(_ context.Context, t tenancy.Tenant, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.objs, claveCompleta(t, key))
	return nil
}

func (a *almacenFalso) DeletePrefix(_ context.Context, t tenancy.Tenant, prefix string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.objs {
		if strings.HasPrefix(k, claveCompleta(t, prefix)) {
			delete(a.objs, k)
		}
	}
	return nil
}

func (a *almacenFalso) SizePrefix(_ context.Context, t tenancy.Tenant, prefix string) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var n int64
	for k, o := range a.objs {
		if strings.HasPrefix(k, claveCompleta(t, prefix)) {
			n += int64(len(o.datos))
		}
	}
	return n, nil
}

// poner deja un objeto directamente (para preparar un test).
func (a *almacenFalso) poner(t tenancy.Tenant, key, tipo string, datos []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objs[claveCompleta(t, key)] = objetoFalso{datos: datos, tipo: tipo}
}

// leer devuelve el objeto o (nil, "", false).
func (a *almacenFalso) leer(t tenancy.Tenant, key string) ([]byte, string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.objs[claveCompleta(t, key)]
	return o.datos, o.tipo, ok
}

// colaFalsa cumple Cola en memoria y apunta lo que el bucle le dice.
type colaFalsa struct {
	mu         sync.Mutex
	pendientes []domain.Job
	videos     map[uuid.UUID]domain.Video
	errReclam  error
	terminados map[uuid.UUID]store.Estado
	fallos     map[uuid.UUID]string
}

func nuevaColaFalsa() *colaFalsa {
	return &colaFalsa{videos: map[uuid.UUID]domain.Video{}, terminados: map[uuid.UUID]store.Estado{}, fallos: map[uuid.UUID]string{}}
}

func (c *colaFalsa) encolar(v domain.Video, kind domain.JobKind, payload string) domain.Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := domain.Job{ID: uuid.New(), Tenant: v.Tenant, VideoID: v.ID, Kind: kind, Status: domain.JobPending, Payload: []byte(payload)}
	c.videos[v.ID] = v
	c.pendientes = append(c.pendientes, j)
	return j
}

func (c *colaFalsa) Reclamar(_ context.Context, quien string) (domain.Job, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.errReclam != nil {
		return domain.Job{}, c.errReclam
	}
	if len(c.pendientes) == 0 {
		return domain.Job{}, domain.ErrNoExiste
	}
	j := c.pendientes[0]
	c.pendientes = c.pendientes[1:]
	j.Status, j.Attempts, j.ClaimedBy = domain.JobRunning, j.Attempts+1, quien
	return j, nil
}

func (c *colaFalsa) VideoDelTrabajo(_ context.Context, j domain.Job) (domain.Video, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.videos[j.VideoID]
	if !ok {
		return domain.Video{}, domain.ErrNoExiste
	}
	return v, nil
}

func (c *colaFalsa) Terminar(_ context.Context, jobID uuid.UUID, resultado store.Estado) (domain.Video, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.terminados[jobID] = resultado
	return domain.Video{Status: domain.StatusReady, BytesTotal: derefInt64(resultado.BytesTotal)}, nil
}

func (c *colaFalsa) Fallar(_ context.Context, jobID uuid.UUID, motivo string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fallos[jobID] = motivo
	return true, nil
}

var errBaseCaida = errors.New("postgres: connection refused")

// trabajadorDePrueba monta el bucle con los dobles, SIN pasar por `nuevo` (que
// exige ffmpeg en el PATH): lo que se prueba aquí no lo necesita.
func trabajadorDePrueba(cola Cola, objetos Objetos, tmp string) *trabajador {
	return &trabajador{
		cola: cola, objetos: objetos, logger: slog.New(slog.DiscardHandler), quien: "prueba",
		ffmpeg: "/definitivamente/no/hay/ffmpeg", ffprobe: "/definitivamente/no/hay/ffprobe",
		latido: tmp + "/" + NombreLatido,
	}
}
