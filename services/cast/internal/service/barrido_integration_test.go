// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El barrido periódico contra un PostgreSQL real, con el rol de la aplicación (la
// RLS se evalúa): la política de borrado en mantenimiento, las cascadas, los
// plazos que aplica la base y el cerrojo entre réplicas.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// cuotaQueApunta es Drive de mentira: anota a quién se declaró y cuánto.
type cuotaQueApunta struct {
	mu        sync.Mutex
	declarado map[string]int64
}

func (c *cuotaQueApunta) Consultar(context.Context, string) (cuota.Resumen, error) {
	return cuota.Resumen{}, nil
}
func (c *cuotaQueApunta) Disponible(context.Context, string) (int64, error) { return 1 << 40, nil }
func (c *cuotaQueApunta) Declarar(_ context.Context, tenant string, bytes int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.declarado == nil {
		c.declarado = map[string]int64{}
	}
	c.declarado[tenant] = bytes
	return nil
}

func (c *cuotaQueApunta) a(tenant string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.declarado[tenant]
	return n, ok
}

// banco es lo que comparten las pruebas del barrido: la base, el store, el
// almacén (que puede fallar o bloquearse) y una conexión de dueño para mover el
// reloj de las filas.
type banco struct {
	st  *store.Store
	obj *objetosQueFallan
	cu  *cuotaQueApunta
	adm *pgx.Conn
	h   *pgtest.Harness
}

func nuevoBanco(t *testing.T) *banco {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	adm, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adm.Close(context.Background()) })
	return &banco{st: store.New(h.Pool), obj: &objetosQueFallan{fallan: map[uuid.UUID]bool{}}, cu: &cuotaQueApunta{}, adm: adm, h: h}
}

func (b *banco) servicio(o Objetos) *Servicio {
	if o == nil {
		o = b.obj
	}
	return New(Deps{Store: b.st, Objetos: o, Cuota: b.cu, Firmante: media.New("clave")})
}

// subido crea un vídeo con su original en el almacén, un trabajo en la cola, una
// persona, un comentario y un mensaje de chat: todo lo que cuelga de la fila.
func (b *banco) subido(ctx context.Context, t *testing.T, tenant string) domain.Video {
	t.Helper()
	v, err := b.st.Crear(ctx, store.Nuevo{Tenant: tenant, OwnerSub: "ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: "v"})
	if err != nil {
		t.Fatal(err)
	}
	clave := Prefijo(v) + media.RutaOriginal
	if err := b.obj.Put(ctx, tenant, clave, strings.NewReader("abc"), 3, "video/mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.st.RecibirOriginal(ctx, tenant, v.ID, clave, 3); err != nil {
		t.Fatal(err)
	}
	if err := b.st.AnadirPersona(ctx, tenant, v.ID, domain.Persona{UserID: "luis", Name: "Luis"}, "ana"); err != nil {
		t.Fatal(err)
	}
	b.exec(ctx, t, `INSERT INTO comments (tenant, video_id, author_sub, body) VALUES ($1, $2, 'ana', 'hola')`, tenant, v.ID)
	b.exec(ctx, t, `INSERT INTO chat_messages (tenant, video_id, author_sub, body) VALUES ($1, $2, 'ana', 'hola')`, tenant, v.ID)
	return v
}

func (b *banco) exec(ctx context.Context, t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := b.adm.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

// cuantas cuenta, como dueño (sin RLS), las filas del vídeo en cada tabla.
func (b *banco) cuantas(ctx context.Context, t *testing.T, id uuid.UUID) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tabla := range []string{"videos", "transcode_jobs", "video_people", "comments", "chat_messages"} {
		col := "video_id"
		if tabla == "videos" {
			col = "id"
		}
		var n int
		if err := b.adm.QueryRow(ctx, `SELECT count(*) FROM `+tabla+` WHERE `+col+` = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[tabla] = n
	}
	return out
}

func (b *banco) borradoHace(ctx context.Context, t *testing.T, id uuid.UUID, hace time.Duration) {
	t.Helper()
	b.exec(ctx, t, `UPDATE videos SET deleted_at = now() - $2::interval WHERE id = $1`, id, hace)
}

var plazosReales = Barrido{Gracia: 7 * 24 * time.Hour, Subida: 6 * time.Hour}

// Un borrado cuya purga se perdió: dentro de la gracia no se toca; pasada la
// gracia, fuera sus objetos, su fila y todo lo que colgaba, y se vuelve a
// declarar la cuota de su organización.
func TestBarridoRetiraElBorradoTrasLaGracia(t *testing.T) {
	ctx := ctxDePrueba(t)
	b := nuevoBanco(t)
	svc := b.servicio(nil)
	v := b.subido(ctx, t, "acme")
	if _, err := b.st.Borrar(ctx, "acme", v.ID); err != nil { // sin purgar: la purga «se perdió»
		t.Fatal(err)
	}

	b.borradoHace(ctx, t, v.ID, 6*24*time.Hour)
	r, err := svc.Barrer(ctx, plazosReales)
	if err != nil {
		t.Fatal(err)
	}
	if r.Algo() || !b.obj.tiene("acme", v.ID) || b.cuantas(ctx, t, v.ID)["videos"] != 1 {
		t.Fatalf("dentro de la gracia: %+v; no tenía que tocar nada", r)
	}

	b.borradoHace(ctx, t, v.ID, 8*24*time.Hour)
	r, err = svc.Barrer(ctx, plazosReales)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retirados != 1 || r.Organizaciones != 1 {
		t.Fatalf("resultado = %+v; quiero 1 retirado en 1 organización", r)
	}
	if b.obj.tiene("acme", v.ID) {
		t.Fatal("los objetos del vídeo siguen en el almacén")
	}
	for tabla, n := range b.cuantas(ctx, t, v.ID) {
		if n != 0 {
			t.Fatalf("quedan %d filas en %s", n, tabla)
		}
	}
	if n, ok := b.cu.a("acme"); !ok || n != 0 {
		t.Fatalf("cuota declarada a acme = %d (%v); quiero 0 declarado", n, ok)
	}
}

// Una purga que falla deja la fila (y los objetos) para la pasada siguiente, que
// la retira cuando el almacén vuelve.
func TestBarridoReintentaLaPurgaQueFalla(t *testing.T) {
	ctx := ctxDePrueba(t)
	b := nuevoBanco(t)
	svc := b.servicio(nil)
	v := b.subido(ctx, t, "acme")
	if _, err := b.st.Borrar(ctx, "acme", v.ID); err != nil {
		t.Fatal(err)
	}
	b.borradoHace(ctx, t, v.ID, 8*24*time.Hour)

	b.obj.fallan[v.ID] = true
	r, err := svc.Barrer(ctx, plazosReales)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retirados != 0 || r.PurgasFallidas != 1 || b.cuantas(ctx, t, v.ID)["videos"] != 1 || !b.obj.tiene("acme", v.ID) {
		t.Fatalf("con el almacén caído: %+v; la fila y los objetos se quedan", r)
	}

	delete(b.obj.fallan, v.ID)
	r, err = svc.Barrer(ctx, plazosReales)
	if err != nil {
		t.Fatal(err)
	}
	if r.Retirados != 1 || b.cuantas(ctx, t, v.ID)["videos"] != 0 || b.obj.tiene("acme", v.ID) {
		t.Fatalf("en la pasada siguiente: %+v; quiero el vídeo retirado", r)
	}
}

// Una subida atascada más del plazo pasa a failed con el motivo y se purga lo que
// llegó; una reciente, y un vídeo vivo de otra organización, no se tocan.
func TestBarridoCaducaLasSubidasAtascadas(t *testing.T) {
	ctx := ctxDePrueba(t)
	b := nuevoBanco(t)
	svc := b.servicio(nil)
	nueva := func(tenant string) domain.Video {
		v, err := b.st.Crear(ctx, store.Nuevo{Tenant: tenant, OwnerSub: "ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: "s"})
		if err != nil {
			t.Fatal(err)
		}
		// Lo que una copia cortada dejó a medias en el almacén.
		if err := b.obj.Put(ctx, tenant, Prefijo(v)+media.RutaOriginal, strings.NewReader("ab"), 2, ""); err != nil {
			t.Fatal(err)
		}
		return v
	}
	vieja, reciente := nueva("acme"), nueva("globex")
	b.exec(ctx, t, `UPDATE videos SET updated_at = now() - interval '7 hours' WHERE id = $1`, vieja.ID)
	b.exec(ctx, t, `UPDATE videos SET updated_at = now() - interval '5 hours' WHERE id = $1`, reciente.ID)
	listo := b.subido(ctx, t, "globex") // en processing: no es una subida
	b.exec(ctx, t, `UPDATE videos SET updated_at = now() - interval '9 hours' WHERE id = $1`, listo.ID)

	r, err := svc.Barrer(ctx, plazosReales)
	if err != nil {
		t.Fatal(err)
	}
	if r.Caducadas != 1 || r.Retirados != 0 {
		t.Fatalf("resultado = %+v; quiero 1 subida caducada", r)
	}
	v, err := b.st.EnOrganizacion(ctx, "acme", vieja.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusFailed || v.Failure != store.MotivoSubidaCaducada || b.obj.tiene("acme", vieja.ID) {
		t.Fatalf("la vieja = %s %q, objetos %v; quiero failed, con motivo y purgada", v.Status, v.Failure, b.obj.tiene("acme", vieja.ID))
	}
	v, err = b.st.EnOrganizacion(ctx, "globex", reciente.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusUploading || !b.obj.tiene("globex", reciente.ID) {
		t.Fatalf("la reciente = %s; no se toca", v.Status)
	}
	if v, _ := b.st.EnOrganizacion(ctx, "globex", listo.ID); v.Status != domain.StatusProcessing {
		t.Fatalf("el vídeo en processing = %s; no se toca", v.Status)
	}
	if _, ok := b.cu.a("acme"); !ok {
		t.Fatal("no se volvió a declarar la cuota de acme")
	}
}

// objetosQueSeParan bloquea la primera purga hasta que la prueba la suelta; las
// demás pasan (si el cerrojo no funcionara, la segunda réplica llegaría aquí).
type objetosQueSeParan struct {
	*objetosQueFallan
	dentro  chan struct{}
	soltar  chan struct{}
	llamado int
	mu      sync.Mutex
}

func (o *objetosQueSeParan) DeletePrefix(ctx context.Context, tenant, prefix string) error {
	o.mu.Lock()
	o.llamado++
	primera := o.llamado == 1
	o.mu.Unlock()
	if primera {
		close(o.dentro)
		<-o.soltar
	}
	return o.objetosQueFallan.DeletePrefix(ctx, tenant, prefix)
}

// Dos réplicas barren a la vez: la segunda encuentra el cerrojo cogido y no
// hace nada; la primera termina su pasada.
func TestBarridoDosALaVezSoloTrabajaUno(t *testing.T) {
	ctx := ctxDePrueba(t)
	b := nuevoBanco(t)
	v := b.subido(ctx, t, "acme")
	if _, err := b.st.Borrar(ctx, "acme", v.ID); err != nil {
		t.Fatal(err)
	}
	b.borradoHace(ctx, t, v.ID, 8*24*time.Hour)

	o := &objetosQueSeParan{objetosQueFallan: b.obj, dentro: make(chan struct{}), soltar: make(chan struct{})}
	uno, otro := b.servicio(o), b.servicio(o)

	type fin struct {
		r   ResultadoBarrido
		err error
	}
	primero := make(chan fin, 1)
	go func() {
		r, err := uno.Barrer(ctx, plazosReales)
		primero <- fin{r, err}
	}()
	select {
	case <-o.dentro:
	case <-ctx.Done():
		t.Fatal("la primera pasada no llegó a purgar")
	}

	r, err := otro.Barrer(ctx, plazosReales)
	close(o.soltar)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ocupado || r.Algo() {
		t.Fatalf("la segunda = %+v; quiero que encuentre el cerrojo cogido", r)
	}
	f := <-primero
	if f.err != nil || f.r.Retirados != 1 {
		t.Fatalf("la primera = %+v, %v; quiero 1 retirado", f.r, f.err)
	}
	if o.llamado != 1 {
		t.Fatalf("el almacén recibió %d purgas; quiero 1", o.llamado)
	}

	// Y el cerrojo se suelta: una pasada después ya no está ocupado.
	r, err = otro.Barrer(ctx, plazosReales)
	if err != nil || r.Ocupado {
		t.Fatalf("tras la primera = %+v, %v; el cerrojo tenía que estar libre", r, err)
	}
}

// Eliminar solo toca vídeos borrados.
func TestEliminarNoTocaUnVideoVivo(t *testing.T) {
	ctx := ctxDePrueba(t)
	b := nuevoBanco(t)
	v := b.subido(ctx, t, "acme")
	if err := b.st.Eliminar(ctx, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("Eliminar un vídeo vivo = %v; quiero ErrNoExiste", err)
	}
	if b.cuantas(ctx, t, v.ID)["videos"] != 1 {
		t.Fatal("el vídeo vivo desapareció")
	}
}
