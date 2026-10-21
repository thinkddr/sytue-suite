// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// datosFalsos es el store del barrido en memoria: los borrados ya caducados y las
// subidas ya atascadas (los plazos los aplica la base; aquí se prueba la pasada).
type datosFalsos struct {
	mu        sync.Mutex
	ocupado   bool
	borrados  []store.Borrado // en el orden de la base: (deleted_at, id)
	subidas   []domain.Video
	lecturas  int
	eliminado []uuid.UUID
	// enCola: trabajos de calidades activos; pendientes: vídeos de antes de la
	// escalera WebM que aún no tienen el suyo.
	enCola, pendientes int
}

func (d *datosFalsos) ConCerrojoDelBarrido(ctx context.Context, fn func(context.Context) error) (bool, error) {
	if d.ocupado {
		return false, nil
	}
	return true, fn(ctx)
}

func despues(b store.Borrado, c *store.Cursor) bool {
	if c == nil {
		return true
	}
	return b.DeletedAt.After(c.Fecha) || (b.DeletedAt.Equal(c.Fecha) && b.ID.String() > c.ID.String())
}

func (d *datosFalsos) BorradosCaducados(_ context.Context, _ time.Duration, desde *store.Cursor, limite int) ([]store.Borrado, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lecturas++
	if d.lecturas > 50 {
		return nil, errors.New("la pasada no avanza")
	}
	var out []store.Borrado
	for _, b := range d.borrados {
		if despues(b, desde) && len(out) < limite {
			out = append(out, b)
		}
	}
	return out, nil
}

func (d *datosFalsos) Eliminar(_ context.Context, id uuid.UUID) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, b := range d.borrados {
		if b.ID == id {
			d.borrados = append(d.borrados[:i], d.borrados[i+1:]...)
			d.eliminado = append(d.eliminado, id)
			return nil
		}
	}
	return domain.ErrNoExiste
}

func (d *datosFalsos) CaducarSubidas(_ context.Context, _ time.Duration, limite int) ([]domain.Video, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := min(limite, len(d.subidas))
	out := d.subidas[:n]
	d.subidas = d.subidas[n:]
	for i := range out {
		out[i].Status = domain.StatusFailed
	}
	return out, nil
}

func (d *datosFalsos) CalidadesEnCola(context.Context) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.enCola, nil
}

func (d *datosFalsos) EncolarCalidadesPendientes(_ context.Context, limite int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := min(limite, d.pendientes)
	d.pendientes -= n
	d.enCola += n
	return n, nil
}

// objetosQueFallan es el almacén en memoria con prefijos que no se dejan borrar.
type objetosQueFallan struct {
	objetosEnMemoria
	fallan map[uuid.UUID]bool
}

func (o *objetosQueFallan) DeletePrefix(ctx context.Context, tenant, prefix string) error {
	for id := range o.fallan {
		if prefix == id.String()+"/" {
			return errors.New("s3: 503")
		}
	}
	return o.objetosEnMemoria.DeletePrefix(ctx, tenant, prefix)
}

func (o *objetosQueFallan) tiene(tenant string, id uuid.UUID) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.datos[tenant+"/"+id.String()+"/"+media.RutaOriginal]
	return ok
}

func conObjetos(t *testing.T, o *objetosQueFallan, tenant string, ids ...uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		if err := o.Put(context.Background(), tenant, id.String()+"/"+media.RutaOriginal, strings.NewReader("v"), 1, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func borradosDePrueba(n int) []store.Borrado {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	out := make([]store.Borrado, n)
	for i := range out {
		out[i] = store.Borrado{ID: uuid.New(), Tenant: "acme", DeletedAt: base.Add(time.Duration(i) * time.Minute)}
	}
	return out
}

var plazos = Barrido{Gracia: 7 * 24 * time.Hour, Subida: 6 * time.Hour, Tanda: 2}

// Cinco borrados en tandas de dos, uno de ellos con el almacén caído: la pasada
// retira los otros cuatro, deja ese con sus objetos y su fila, y TERMINA (el
// cursor avanza por encima del que falla). En la pasada siguiente, con el
// almacén de vuelta, se retira.
func TestBarrerRetiraPorTandasYReintentaLoQueFalla(t *testing.T) {
	bs := borradosDePrueba(5)
	d := &datosFalsos{borrados: append([]store.Borrado(nil), bs...)}
	o := &objetosQueFallan{fallan: map[uuid.UUID]bool{bs[1].ID: true}}
	for _, b := range bs {
		conObjetos(t, o, "acme", b.ID)
	}
	svc := New(Deps{Objetos: o, Firmante: media.New("clave")})

	r, err := svc.barrer(context.Background(), d, plazos)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retirados != 4 || r.PurgasFallidas != 1 || r.Ocupado {
		t.Fatalf("resultado = %+v; quiero 4 retirados y 1 purga fallida", r)
	}
	if len(d.borrados) != 1 || d.borrados[0].ID != bs[1].ID || !o.tiene("acme", bs[1].ID) {
		t.Fatalf("quedan %v; quiero solo el que falló, con sus objetos", d.borrados)
	}
	for _, b := range []store.Borrado{bs[0], bs[2], bs[3], bs[4]} {
		if o.tiene("acme", b.ID) {
			t.Fatalf("los objetos de %s siguen en el almacén", b.ID)
		}
	}

	delete(o.fallan, bs[1].ID)
	r, err = svc.barrer(context.Background(), d, plazos)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retirados != 1 || len(d.borrados) != 0 || o.tiene("acme", bs[1].ID) {
		t.Fatalf("segunda pasada = %+v, quedan %d; quiero el reintento hecho", r, len(d.borrados))
	}
}

// Si otra réplica tiene el cerrojo, esta no lee ni borra nada.
func TestBarrerOcupadoNoHaceNada(t *testing.T) {
	bs := borradosDePrueba(1)
	d := &datosFalsos{ocupado: true, borrados: bs, subidas: []domain.Video{{ID: uuid.New(), Tenant: "acme"}}}
	o := &objetosQueFallan{}
	conObjetos(t, o, "acme", bs[0].ID)
	svc := New(Deps{Objetos: o, Firmante: media.New("clave")})

	r, err := svc.barrer(context.Background(), d, plazos)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ocupado || r.Algo() || d.lecturas != 0 || len(d.subidas) != 1 || !o.tiene("acme", bs[0].ID) {
		t.Fatalf("resultado = %+v, lecturas %d; quiero que no toque nada", r, d.lecturas)
	}
}

// Las subidas atascadas pasan a failed por tandas y se purga lo que dejaron.
func TestBarrerCaducaSubidasYPurga(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	d := &datosFalsos{}
	o := &objetosQueFallan{}
	for _, id := range ids {
		d.subidas = append(d.subidas, domain.Video{ID: id, Tenant: "globex", Status: domain.StatusUploading})
		conObjetos(t, o, "globex", id)
	}
	svc := New(Deps{Objetos: o, Firmante: media.New("clave")})

	r, err := svc.barrer(context.Background(), d, plazos)
	if err != nil {
		t.Fatal(err)
	}
	if r.Caducadas != 3 || r.Organizaciones != 1 || len(d.subidas) != 0 {
		t.Fatalf("resultado = %+v; quiero 3 subidas caducadas en 1 organización", r)
	}
	for _, id := range ids {
		if o.tiene("globex", id) {
			t.Fatalf("los objetos de la subida %s siguen en el almacén", id)
		}
	}
}

// Plazos a cero apagan cada mitad: nada se lee ni se borra.
func TestBarrerConPlazosACeroNoToca(t *testing.T) {
	d := &datosFalsos{borrados: borradosDePrueba(2), subidas: []domain.Video{{ID: uuid.New(), Tenant: "acme"}}}
	svc := New(Deps{Objetos: &objetosQueFallan{}, Firmante: media.New("clave")})
	r, err := svc.barrer(context.Background(), d, Barrido{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Algo() || d.lecturas != 0 || len(d.borrados) != 2 || len(d.subidas) != 1 {
		t.Fatalf("resultado = %+v; con los plazos a cero no se toca nada", r)
	}
}

// Sin almacén no se puede comprobar que no quede nada: no se elimina ninguna fila.
func TestBarrerSinAlmacenNoEliminaFilas(t *testing.T) {
	d := &datosFalsos{borrados: borradosDePrueba(2)}
	svc := New(Deps{Firmante: media.New("clave")})
	if _, err := svc.barrer(context.Background(), d, plazos); err != nil {
		t.Fatal(err)
	}
	if len(d.eliminado) != 0 {
		t.Fatalf("sin almacén se eliminaron %d filas", len(d.eliminado))
	}
}

// El relleno de calidades completa la cola hasta el tope y no la pasa: los
// vídeos de antes acaban todos hechos, pero sin tapar lo que sube la gente.
func TestBarridoRellenaCalidadesHastaElTope(t *testing.T) {
	svc := New(Deps{Objetos: &objetosQueFallan{}, Firmante: media.New("clave")})
	d := &datosFalsos{enCola: 3, pendientes: 20}
	b := Barrido{Calidades: 8}

	r, err := svc.barrer(context.Background(), d, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Calidades != 5 || d.enCola != 8 {
		t.Fatalf("con 3 en cola y tope 8 se esperaban 5 nuevos y 8 en cola: %d nuevos, %d en cola", r.Calidades, d.enCola)
	}
	if !r.Algo() {
		t.Fatal("encolar calidades es hacer algo: debería quedar en el registro")
	}
	// Con la cola llena, la pasada siguiente no encola nada.
	if r, _ := svc.barrer(context.Background(), d, b); r.Calidades != 0 {
		t.Fatalf("con la cola llena encoló %d", r.Calidades)
	}
	// Terminan los trabajos: la siguiente vuelve a rellenar con lo que queda.
	d.enCola = 0
	if r, _ := svc.barrer(context.Background(), d, b); r.Calidades != 8 || d.pendientes != 7 {
		t.Fatalf("tras vaciarse la cola: %d nuevos, %d pendientes", r.Calidades, d.pendientes)
	}
	// Apagado (0) no toca nada.
	d.enCola = 0
	if r, _ := svc.barrer(context.Background(), d, Barrido{}); r.Calidades != 0 {
		t.Fatalf("con el relleno apagado encoló %d", r.Calidades)
	}
}
