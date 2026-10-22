// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// moderable monta el servicio con la clave de las denuncias anónimas y sin tope
// de ritmo (el tope tiene su prueba).
func moderable(t *testing.T) (*Servicio, *accesoDePrueba, *pg.Pool) {
	t.Helper()
	h := montar(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver).ConModeracion(Moderacion{Secreto: []byte("clave-de-prueba")})
	svc.ritmoDenuncias = httpx.NewTokenBucket(1000, 1000)
	return svc, acc, h.Pool
}

func administra(sub, tenant string) Quien {
	q := persona(sub, tenant)
	q.Admin = true
	return q
}

func anonimo(ip string) Quien { return Quien{Anonimo: true, IP: ip} }

func comentar(ctx context.Context, t *testing.T, svc *Servicio, q Quien, v Video, body string) Comentario {
	t.Helper()
	c, err := svc.Comentar(ctx, q, v.ID, body, nil)
	if err != nil {
		t.Fatalf("%s comenta: %v", q.Sub, err)
	}
	return c
}

func cola(ctx context.Context, t *testing.T, svc *Servicio, q Quien, resueltas bool) []Denuncia {
	t.Helper()
	ds, _, err := svc.Denuncias(ctx, q, resueltas, "", 0)
	if err != nil {
		t.Fatalf("cola de %s: %v", q.Sub, err)
	}
	return ds
}

// La denuncia llega a la cola de la organización del contenido, una sola vez por
// persona, y de ahí no sale quién denunció.
func TestDenuncia_LlegaALaColaSinDecirQuien(t *testing.T) {
	svc, acc, _ := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "public", "ready"), true)
	bea := persona("bea", "acme")
	c := comentar(ctx, t, svc, bea, v, "un comentario feo")

	// Zoe es de otra organización y ve el vídeo porque es público.
	zoe := persona("zoe-la-que-denuncia", "globex")
	d := NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "harassment", Details: "  insulta  "}
	for range 3 {
		if err := svc.Denunciar(ctx, zoe, v.ID, d); err != nil {
			t.Fatal(err)
		}
	}
	jefa := administra("jefa", "acme")
	ds := cola(ctx, t, svc, jefa, false)
	if len(ds) != 1 {
		t.Fatalf("tres denuncias iguales dejaron %d filas, no una", len(ds))
	}
	got := ds[0]
	if got.Kind != DenunciaComentario || got.TargetID != c.ID.String() || got.TargetSub != "bea" || got.Excerpt != "un comentario feo" ||
		got.Reason != "harassment" || got.Details != "insulta" || got.Anonymous || got.Status != "open" {
		t.Fatalf("fila de la cola: %+v", got)
	}
	b, _ := json.Marshal(ds)
	if strings.Contains(string(b), "zoe") || strings.Contains(string(b), "globex") {
		t.Fatalf("la cola dice quién denunció: %s", b)
	}
	// La traza tampoco: la lee quien modera, que puede ser el denunciado.
	traza, err := svc.Auditoria(ctx, jefa, 100)
	if err != nil || len(traza) != 1 || traza[0].Action != "report.created" || traza[0].Actor != "account" {
		t.Fatalf("traza: %+v, %v", traza, err)
	}
	// Quién fue queda guardado, para quien tenga que mirarlo en la base.
	var sub, tenant string
	if err := svc.store.enOrg(ctx, "acme", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT reporter_sub, reporter_tenant FROM reports WHERE id = $1`, got.ID).Scan(&sub, &tenant)
	}); err != nil || sub != zoe.Sub || tenant != "globex" {
		t.Fatalf("quién denunció: %q %q, %v", sub, tenant, err)
	}
}

// Nadie ve ni resuelve la cola de otra organización, y dentro de la suya solo
// quien la administra.
func TestDenuncia_LaColaEsDeQuienAdministraLaOrganizacion(t *testing.T) {
	svc, acc, _ := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "public", "ready"), true)
	if err := svc.Denunciar(ctx, persona("zoe", "globex"), v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "violence"}); err != nil {
		t.Fatal(err)
	}
	jefa := administra("jefa", "acme")
	id := cola(ctx, t, svc, jefa, false)[0].ID

	// Un miembro sin rol, la dueña del vídeo sin rol y alguien sin sesión: prohibido.
	for _, q := range []Quien{persona("bea", "acme"), persona("duenya", "acme"), nadie} {
		if _, _, err := svc.Denuncias(ctx, q, false, "", 0); !errors.Is(err, ErrProhibido) {
			t.Errorf("%q lee la cola: %v", q.Sub, err)
		}
		if _, err := svc.DenunciasAbiertas(ctx, q); !errors.Is(err, ErrProhibido) {
			t.Errorf("%q cuenta la cola: %v", q.Sub, err)
		}
		if err := svc.ResolverDenuncia(ctx, q, id, AccionDescartar); !errors.Is(err, ErrProhibido) {
			t.Errorf("%q resuelve: %v", q.Sub, err)
		}
		if _, err := svc.Auditoria(ctx, q, 10); !errors.Is(err, ErrProhibido) {
			t.Errorf("%q lee la traza: %v", q.Sub, err)
		}
	}
	// Quien administra OTRA organización: su cola está vacía y esa denuncia no existe.
	otra := administra("jefa-de-globex", "globex")
	if ds := cola(ctx, t, svc, otra, false); len(ds) != 0 {
		t.Fatalf("globex ve %d denuncias de acme", len(ds))
	}
	for _, accion := range []string{AccionDescartar, AccionRetirar, AccionDespublicar, AccionBloquear} {
		if err := svc.ResolverDenuncia(ctx, otra, id, accion); !errors.Is(err, ErrNoExiste) {
			t.Errorf("globex aplica %s a una denuncia de acme: %v", accion, err)
		}
	}
	if traza, err := svc.Auditoria(ctx, otra, 100); err != nil || len(traza) != 0 {
		t.Fatalf("globex lee la traza de acme: %+v, %v", traza, err)
	}
	if n, err := svc.DenunciasAbiertas(ctx, jefa); err != nil || n != 1 {
		t.Fatalf("abiertas en acme: %d, %v", n, err)
	}
}

// Quien administra no ve las denuncias contra sí mismo —contra su vídeo, su
// comentario o su persona—: ni en la cola, ni en la cuenta, ni en la traza, ni
// para resolverlas. Otra persona que administre la organización, sí.
func TestDenuncia_QuienAdministraNoVeLasSuyas(t *testing.T) {
	svc, acc, _ := moderable(t)
	ctx := ctxCorto(t)
	jefa, otra := administra("jefa", "acme"), administra("otra-jefa", "acme")
	v := acc.conoce(videoEn(t, svc.store.pool, "acme", "jefa", "public", "ready"), true)
	c := comentar(ctx, t, svc, jefa, v, "lo escribe la jefa")
	zoe := persona("zoe", "globex")
	for _, d := range []NuevaDenuncia{
		{Kind: DenunciaVideo, Reason: "violence"},
		{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "harassment"},
		{Kind: DenunciaPersona, Target: "jefa", Reason: "spam"},
	} {
		if err := svc.Denunciar(ctx, zoe, v.ID, d); err != nil {
			t.Fatal(err)
		}
	}

	if ds := cola(ctx, t, svc, jefa, false); len(ds) != 0 {
		t.Fatalf("la jefa ve %d denuncias contra ella: %+v", len(ds), ds)
	}
	if n, err := svc.DenunciasAbiertas(ctx, jefa); err != nil || n != 0 {
		t.Fatalf("la jefa cuenta %d denuncias contra ella: %v", n, err)
	}
	if traza, err := svc.Auditoria(ctx, jefa, 100); err != nil || len(traza) != 0 {
		t.Fatalf("la traza le enseña a la jefa las denuncias contra ella: %+v, %v", traza, err)
	}
	ds := cola(ctx, t, svc, otra, false)
	if len(ds) != 3 {
		t.Fatalf("la otra administradora ve %d de 3", len(ds))
	}
	for _, d := range ds {
		if err := svc.ResolverDenuncia(ctx, jefa, d.ID, AccionDescartar); !errors.Is(err, ErrNoExiste) {
			t.Fatalf("la jefa resuelve una denuncia contra ella: %v", err)
		}
	}
	if err := svc.ResolverDenuncia(ctx, otra, ds[0].ID, AccionDescartar); err != nil {
		t.Fatal(err)
	}
	// Resuelta tampoco: ni en las resueltas ni en la traza de la resolución.
	if ds := cola(ctx, t, svc, jefa, true); len(ds) != 0 {
		t.Fatalf("la jefa ve resuelta una denuncia contra ella: %+v", ds)
	}
	if traza, _ := svc.Auditoria(ctx, jefa, 100); len(traza) != 0 {
		t.Fatalf("la traza enseña la resolución a la jefa: %+v", traza)
	}
	if traza, _ := svc.Auditoria(ctx, otra, 100); len(traza) != 4 {
		t.Fatalf("la otra administradora lee %d filas de 4", len(traza))
	}
	// Una fila de antes, sin `target_sub` en el detalle, se mira contra su denuncia;
	// y una de ahora sigue escondida cuando su denuncia ya se purgó.
	if err := svc.store.enOrg(ctx, "acme", func(tx pgx.Tx) error {
		if err := auditar(ctx, tx, "acme", "account", "report.created", "video:"+v.ID.String(),
			map[string]any{"report_id": ds[1].ID}); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM reports WHERE id = $1`, ds[0].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if traza, _ := svc.Auditoria(ctx, jefa, 100); len(traza) != 0 {
		t.Fatalf("la traza enseña a la jefa una fila antigua o purgada: %+v", traza)
	}
}

// Sin cuenta se denuncia lo que se ve sin cuenta, y nada más; la fila no guarda
// la IP y la respuesta no lleva nada.
func TestDenuncia_SinCuenta(t *testing.T) {
	svc, acc, _ := moderable(t)
	ctx := ctxCorto(t)
	publico := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "public", "ready"), true)
	interno := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "org", "ready"), false)
	c := comentar(ctx, t, svc, persona("bea", "acme"), publico, "spam spam")
	m, err := svc.EnviarChat(ctx, persona("bea", "acme"), publico.ID, "hola")
	if err != nil {
		t.Fatal(err)
	}

	visitante := anonimo("203.0.113.7")
	if err := svc.Denunciar(ctx, visitante, publico.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	// La misma IP otra vez: no suma. Otra IP: sí.
	if err := svc.Denunciar(ctx, visitante, publico.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "other"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Denunciar(ctx, anonimo("203.0.113.8"), publico.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	ds := cola(ctx, t, svc, administra("jefa", "acme"), false)
	if len(ds) != 2 || !ds[0].Anonymous || !ds[1].Anonymous {
		t.Fatalf("cola tras tres denuncias de dos IP: %+v", ds)
	}
	var claves string
	if err := svc.store.enOrg(ctx, "acme", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT string_agg(reporter_key || reporter_sub, ' ') FROM reports`).Scan(&claves)
	}); err != nil || strings.Contains(claves, "203.0.113") {
		t.Fatalf("la IP quedó guardada: %q, %v", claves, err)
	}

	// Lo que no se ve sin sesión no existe, y el chat no se denuncia sin sesión.
	if err := svc.Denunciar(ctx, visitante, interno.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}); !errors.Is(err, ErrNoExiste) {
		t.Errorf("denuncia anónima de un vídeo de organización: %v", err)
	}
	if err := svc.Denunciar(ctx, visitante, publico.ID, NuevaDenuncia{Kind: DenunciaChat, Target: m.ID.String(), Reason: "spam"}); !errors.Is(err, ErrProhibido) {
		t.Errorf("denuncia anónima de un mensaje del chat: %v", err)
	}
	// Sin clave configurada no hay denuncia anónima: no se guarda una fila sin
	// con qué deduplicarla.
	sinClave := New(svc.store.pool, acc.resolver)
	if err := sinClave.Denunciar(ctx, visitante, publico.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}); !errors.Is(err, ErrProhibido) {
		t.Errorf("denuncia anónima sin clave: %v", err)
	}
}

// Lo que se denuncia tiene que estar en ESE vídeo: ni comentarios de otro, ni
// personas que no aparecen, ni motivos fuera de la lista.
func TestDenuncia_ElObjetivoEsDelVideo(t *testing.T) {
	svc, acc, _ := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "org", "ready"), false)
	otro := acc.conoce(videoEn(t, svc.store.pool, "acme", "duenya", "org", "ready"), false)
	ajeno := acc.conoce(videoEn(t, svc.store.pool, "globex", "zoe", "org", "ready"), false)
	bea, ana := persona("bea", "acme"), persona("ana", "acme")
	enOtro := comentar(ctx, t, svc, bea, otro, "en otro vídeo")

	casos := []struct {
		nombre string
		video  uuid.UUID
		d      NuevaDenuncia
		quiere error
	}{
		{"comentario de otro vídeo", v.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: enOtro.ID.String(), Reason: "spam"}, ErrNoExiste},
		{"comentario inexistente", v.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: uuid.NewString(), Reason: "spam"}, ErrNoExiste},
		{"objetivo ilegible", v.ID, NuevaDenuncia{Kind: DenunciaComentario, Target: "x", Reason: "spam"}, ErrEntradaInvalida},
		{"persona que no aparece", v.ID, NuevaDenuncia{Kind: DenunciaPersona, Target: "bea", Reason: "spam"}, ErrNoExiste},
		{"motivo inventado", v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "porque sí"}, ErrEntradaInvalida},
		{"tipo inventado", v.ID, NuevaDenuncia{Kind: "org", Reason: "spam"}, ErrEntradaInvalida},
		{"detalle enorme", v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam", Details: strings.Repeat("a", MaxDetalle+1)}, ErrEntradaInvalida},
		{"vídeo de otra organización", ajeno.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}, ErrNoExiste},
		{"a una misma", otro.ID, NuevaDenuncia{Kind: DenunciaPersona, Target: "ana", Reason: "spam"}, ErrNoExiste},
	}
	for _, c := range casos {
		if err := svc.Denunciar(ctx, ana, c.video, c.d); !errors.Is(err, c.quiere) {
			t.Errorf("%s: %v, se esperaba %v", c.nombre, err, c.quiere)
		}
	}
	// A la dueña del vídeo sí: aparece en él.
	if err := svc.Denunciar(ctx, ana, v.ID, NuevaDenuncia{Kind: DenunciaPersona, Target: "duenya", Reason: "harassment"}); err != nil {
		t.Fatal(err)
	}
	// Y nadie se denuncia a sí mismo.
	if err := svc.Denunciar(ctx, persona("duenya", "acme"), v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("la dueña denuncia su vídeo: %v", err)
	}
	if ds := cola(ctx, t, svc, administra("jefa", "acme"), false); len(ds) != 1 {
		t.Fatalf("la cola tiene %d filas, no la única válida", len(ds))
	}
}

// Las acciones: borrar el comentario, retirar el vídeo de la portada,
// despublicarlo, bloquear. Cada una cierra la denuncia y deja su fila de traza.
func TestDenuncia_Acciones(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "public", "ready"), true)
	bea, ana, jefa := persona("bea", "acme"), persona("ana", "acme"), administra("jefa", "acme")
	c := comentar(ctx, t, svc, bea, v, "fuera de tono")

	denunciar := func(q Quien, d NuevaDenuncia) {
		t.Helper()
		if err := svc.Denunciar(ctx, q, v.ID, d); err != nil {
			t.Fatal(err)
		}
	}
	porTipo := func(kind string) Denuncia {
		t.Helper()
		for _, d := range cola(ctx, t, svc, jefa, false) {
			if d.Kind == kind {
				return d
			}
		}
		t.Fatalf("no hay denuncia abierta de tipo %s", kind)
		return Denuncia{}
	}
	visibilidad := func() (vis string) {
		t.Helper()
		if err := svc.store.enOrg(ctx, "acme", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT visibility FROM videos WHERE id = $1`, v.ID).Scan(&vis)
		}); err != nil {
			t.Fatal(err)
		}
		return vis
	}

	// Dos personas denuncian el comentario; borrar cierra las dos.
	denunciar(ana, NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "harassment"})
	denunciar(persona("carla", "acme"), NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "other"})
	d := porTipo(DenunciaComentario)
	// Una acción de vídeo no vale para un comentario.
	if err := svc.ResolverDenuncia(ctx, jefa, d.ID, AccionRetirar); !errors.Is(err, ErrEntradaInvalida) {
		t.Fatalf("retirar un comentario: %v", err)
	}
	if err := svc.ResolverDenuncia(ctx, jefa, d.ID, AccionBorrar); err != nil {
		t.Fatal(err)
	}
	if lista, _, _ := svc.Comentarios(ctx, ana, v.ID, "", 0); len(lista) != 0 {
		t.Fatalf("el comentario borrado sigue: %+v", lista)
	}
	if ds := cola(ctx, t, svc, jefa, false); len(ds) != 0 {
		t.Fatalf("quedan %d abiertas tras borrar", len(ds))
	}
	if ds := cola(ctx, t, svc, jefa, true); len(ds) != 2 || ds[0].Status != "actioned" || ds[0].Resolution != AccionBorrar || len(ds[0].Actions) != 0 {
		t.Fatalf("resueltas: %+v", ds)
	}
	// Resuelta, ya no existe para resolverla otra vez.
	if err := svc.ResolverDenuncia(ctx, jefa, d.ID, AccionDescartar); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("resolver dos veces: %v", err)
	}

	// El vídeo: descartar no toca nada; retirar lo saca de la portada; despublicar
	// lo deja para la organización.
	denunciar(ana, NuevaDenuncia{Kind: DenunciaVideo, Reason: "violence"})
	if err := svc.ResolverDenuncia(ctx, jefa, porTipo(DenunciaVideo).ID, AccionDescartar); err != nil || visibilidad() != "public" {
		t.Fatalf("descartar: %v, visibilidad %s", err, visibilidad())
	}
	denunciar(ana, NuevaDenuncia{Kind: DenunciaVideo, Reason: "violence"})
	if err := svc.ResolverDenuncia(ctx, jefa, porTipo(DenunciaVideo).ID, AccionRetirar); err != nil || visibilidad() != "unlisted" {
		t.Fatalf("retirar: %v, visibilidad %s", err, visibilidad())
	}
	denunciar(ana, NuevaDenuncia{Kind: DenunciaVideo, Reason: "sexual"})
	if err := svc.ResolverDenuncia(ctx, jefa, porTipo(DenunciaVideo).ID, AccionDespublicar); err != nil || visibilidad() != "org" {
		t.Fatalf("despublicar: %v, visibilidad %s", err, visibilidad())
	}

	// Bloquear desde la denuncia: bea deja de poder comentar en la organización.
	c2 := comentar(ctx, t, svc, bea, v, "otra vez")
	denunciar(ana, NuevaDenuncia{Kind: DenunciaPersona, Target: "bea", Reason: "harassment"})
	if err := svc.ResolverDenuncia(ctx, jefa, porTipo(DenunciaPersona).ID, AccionBloquear); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Comentar(ctx, bea, v.ID, "y otra", nil); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("bea comenta estando bloqueada en la organización: %v", err)
	}
	// Bloquear no borra: lo que escribió sigue ahí.
	if lista, _, _ := svc.Comentarios(ctx, ana, v.ID, "", 0); len(lista) != 1 || lista[0].ID != c2.ID {
		t.Fatalf("bloquear borró algo: %+v", lista)
	}

	// La traza: cada denuncia y cada acción, con quién actuó.
	traza, err := svc.Auditoria(ctx, jefa, 100)
	if err != nil {
		t.Fatal(err)
	}
	cuenta := map[string]int{}
	for _, e := range traza {
		cuenta[e.Action]++
		if strings.HasPrefix(e.Action, "report.") && e.Action != "report.created" && e.Actor != "jefa" {
			t.Errorf("acción sin su actor: %+v", e)
		}
	}
	if cuenta["report.created"] != 6 || cuenta["report.actioned"] != 4 || cuenta["report.dismissed"] != 1 || cuenta["block.created"] != 1 {
		t.Fatalf("traza: %v", cuenta)
	}
}

// El tope de ritmo: la sexta denuncia seguida de la misma persona no entra.
func TestDenuncia_Ritmo(t *testing.T) {
	svc, acc, pool := moderable(t)
	svc.ritmoDenuncias = ritmoDeDenuncias()
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "org", "ready"), false)
	ana := persona("ana", "acme")
	var err error
	for range 6 {
		err = svc.Denunciar(ctx, ana, v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"})
	}
	if !errors.Is(err, ErrDemasiadoRapido) {
		t.Fatalf("la sexta denuncia seguida: %v", err)
	}
	// El tope es por persona: otra entra.
	if err := svc.Denunciar(ctx, persona("carla", "acme"), v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
}

// El bloqueo personal: quien bloquea deja de ver lo del otro; los demás no notan
// nada; y el bloqueado no escribe en los vídeos de quien le bloqueó.
func TestBloqueo_Personal(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "org", "live"), false)
	deAna := acc.conoce(videoEn(t, pool, "acme", "ana", "org", "ready"), false)
	ana, bea, carla := persona("ana", "acme"), persona("bea", "acme"), persona("carla", "acme")
	raiz := comentar(ctx, t, svc, bea, v, "de bea")
	if _, err := svc.Comentar(ctx, carla, v.ID, "respuesta a bea", &raiz.ID); err != nil {
		t.Fatal(err)
	}
	comentar(ctx, t, svc, carla, v, "de carla")
	if _, err := svc.EnviarChat(ctx, bea, v.ID, "chat de bea"); err != nil {
		t.Fatal(err)
	}

	// No se bloquea a quien no aparece en el vídeo, ni a una misma, ni sin sesión.
	if err := svc.Bloquear(ctx, ana, v.ID, "alguien-de-fuera", false); !errors.Is(err, ErrNoExiste) {
		t.Errorf("bloquear a quien no aparece: %v", err)
	}
	if err := svc.Bloquear(ctx, ana, v.ID, "ana", false); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("bloquearse: %v", err)
	}
	if err := svc.Bloquear(ctx, nadie, v.ID, "bea", false); !errors.Is(err, ErrProhibido) {
		t.Errorf("bloquear sin sesión: %v", err)
	}

	if err := svc.Bloquear(ctx, ana, v.ID, "bea", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Bloquear(ctx, ana, v.ID, "bea", false); err != nil {
		t.Fatalf("bloquear dos veces: %v", err)
	}
	// Ana no ve lo de bea ni la respuesta colgada de ello; carla lo ve todo.
	if lista, _, _ := svc.Comentarios(ctx, ana, v.ID, "", 0); len(lista) != 1 || lista[0].Body != "de carla" {
		t.Fatalf("lo que ve ana: %+v", lista)
	}
	if lista, _, _ := svc.Comentarios(ctx, carla, v.ID, "", 0); len(lista) != 3 {
		t.Fatalf("lo que ve carla: %+v", lista)
	}
	if hist, _ := svc.HistorialChat(ctx, ana, v.ID, time.Time{}); len(hist) != 0 {
		t.Fatalf("ana ve el chat de bea: %+v", hist)
	}
	if hist, _ := svc.HistorialChat(ctx, carla, v.ID, time.Time{}); len(hist) != 1 {
		t.Fatalf("carla no ve el chat de bea: %+v", hist)
	}

	// Bea sigue escribiendo en el vídeo de la dueña (la dueña no la bloqueó), pero
	// no en el de ana, ni por comentario ni por chat.
	comentar(ctx, t, svc, bea, v, "sigo aquí")
	if _, err := svc.Comentar(ctx, bea, deAna.ID, "hola ana", nil); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("bea comenta en el vídeo de quien la bloqueó: %v", err)
	}
	if _, err := svc.EnviarChat(ctx, bea, deAna.ID, "hola ana"); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("bea escribe en el chat de quien la bloqueó: %v", err)
	}

	// Las listas son de cada cual: la de ana tiene a bea, la de bea está vacía, y
	// bea no puede quitar el bloqueo que le puso ana.
	if l, err := svc.Bloqueados(ctx, ana); err != nil || len(l) != 1 || l[0].Sub != "bea" || l[0].Name != "Persona bea" {
		t.Fatalf("lista de ana: %+v, %v", l, err)
	}
	if l, err := svc.Bloqueados(ctx, bea); err != nil || len(l) != 0 {
		t.Fatalf("lista de bea: %+v, %v", l, err)
	}
	if err := svc.Desbloquear(ctx, bea, "bea"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Desbloquear(ctx, bea, "ana"); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.Bloqueados(ctx, ana); len(l) != 1 {
		t.Fatal("bea quitó un bloqueo que no era suyo")
	}

	// Avisos: de las personas a las que avisaría bea, ana no quiere los suyos.
	if b, err := svc.QuienesBloquean(ctx, "bea", []string{"ana", "carla"}); err != nil || !b["ana"] || b["carla"] {
		t.Fatalf("quiénes bloquean a bea: %v, %v", b, err)
	}

	// Desbloquear lo deshace todo.
	if err := svc.Desbloquear(ctx, ana, "bea"); err != nil {
		t.Fatal(err)
	}
	if lista, _, _ := svc.Comentarios(ctx, ana, v.ID, "", 0); len(lista) != 4 {
		t.Fatalf("tras desbloquear, ana ve %d comentarios", len(lista))
	}
	comentar(ctx, t, svc, bea, deAna, "ya puedo")
}

// El bloqueo de organización: solo quien administra ESA organización, y vale en
// todos sus vídeos.
func TestBloqueo_DeOrganizacion(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "public", "ready"), true)
	otro := acc.conoce(videoEn(t, pool, "acme", "ana", "public", "ready"), true)
	deGlobex := acc.conoce(videoEn(t, pool, "globex", "zoe", "public", "ready"), true)
	bea, jefa := persona("bea", "initech"), administra("jefa", "acme")
	comentar(ctx, t, svc, bea, v, "de fuera")
	comentar(ctx, t, svc, bea, deGlobex, "también aquí")

	// Ni la dueña sin rol, ni quien administra otra organización.
	if err := svc.Bloquear(ctx, persona("duenya", "acme"), v.ID, "bea", true); !errors.Is(err, ErrProhibido) {
		t.Errorf("la dueña sin rol bloquea para toda la organización: %v", err)
	}
	if err := svc.Bloquear(ctx, administra("jefa-de-globex", "globex"), v.ID, "bea", true); !errors.Is(err, ErrProhibido) {
		t.Errorf("globex bloquea en acme: %v", err)
	}
	if err := svc.Bloquear(ctx, jefa, v.ID, "bea", true); err != nil {
		t.Fatal(err)
	}
	for _, video := range []Video{v, otro} {
		if _, err := svc.Comentar(ctx, bea, video.ID, "no debería", nil); !errors.Is(err, ErrBloqueado) {
			t.Errorf("bea comenta en acme estando bloqueada: %v", err)
		}
	}
	// En otra organización no cuenta.
	comentar(ctx, t, svc, bea, deGlobex, "aquí sí")

	// La lista y quitarlo: solo quien administra acme.
	if _, err := svc.BloqueadosEnOrg(ctx, persona("ana", "acme")); !errors.Is(err, ErrProhibido) {
		t.Errorf("un miembro lee los bloqueos de la organización: %v", err)
	}
	otra := administra("jefa-de-globex", "globex")
	if l, err := svc.BloqueadosEnOrg(ctx, otra); err != nil || len(l) != 0 {
		t.Fatalf("globex ve los bloqueos de acme: %+v, %v", l, err)
	}
	if err := svc.DesbloquearEnOrg(ctx, otra, "bea"); err != nil {
		t.Fatal(err)
	}
	if l, err := svc.BloqueadosEnOrg(ctx, jefa); err != nil || len(l) != 1 || l[0].Sub != "bea" {
		t.Fatalf("bloqueos de acme tras intentarlo globex: %+v, %v", l, err)
	}
	if err := svc.DesbloquearEnOrg(ctx, jefa, "bea"); err != nil {
		t.Fatal(err)
	}
	comentar(ctx, t, svc, bea, v, "de vuelta")
}

// En el chat en directo, a quien bloquea no le llega lo del bloqueado; a los
// demás sí. Y vale desde el momento de bloquear, sin reconectar.
func TestBloqueo_ChatEnDirecto(t *testing.T) {
	svc, acc, pool := moderable(t)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "org", "live"), false)
	abrir, ctx := servidorDeChat(t, svc, v, time.Time{})
	ana, bea, carla := abrir("ana"), abrir("bea"), abrir("carla")
	for _, c := range []*websocket.Conn{ana, bea, carla} {
		leerTipo(ctx, t, c, TramaEspectadores)
	}
	enviar := func(c *websocket.Conn, body string) {
		t.Helper()
		if err := wsjson.Write(ctx, c, Orden{Type: OrdenEnviar, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	enviar(bea, "uno")
	if tr := leerTipo(ctx, t, ana, TramaMensaje); tr.Body != "uno" {
		t.Fatalf("antes de bloquear, ana recibe: %+v", tr)
	}
	leerTipo(ctx, t, carla, TramaMensaje)

	if err := svc.Bloquear(ctx, persona("ana", "acme"), v.ID, "bea", false); err != nil {
		t.Fatal(err)
	}
	enviar(bea, "dos")
	if tr := leerTipo(ctx, t, carla, TramaMensaje); tr.Body != "dos" {
		t.Fatalf("carla recibe: %+v", tr)
	}
	// Lo siguiente que le llega a ana es lo de carla, no «dos».
	enviar(carla, "tres")
	if tr := leerTipo(ctx, t, ana, TramaMensaje); tr.Body != "tres" {
		t.Fatalf("a ana le llegó lo de la persona bloqueada: %+v", tr)
	}
	// Quien entra con el bloqueo ya puesto tampoco lo recibe.
	ana2 := abrir("ana")
	leerTipo(ctx, t, ana2, TramaEspectadores)
	enviar(bea, "cuatro")
	enviar(carla, "cinco")
	if tr := leerTipo(ctx, t, ana2, TramaMensaje); tr.Body != "cinco" {
		t.Fatalf("a la conexión nueva de ana le llegó: %+v", tr)
	}
}

// Con un rol que se salta la RLS, la cola, la traza y los bloqueos siguen siendo
// de su organización y de su persona: lo sostiene el SQL.
func TestSinRLSLaModeracionEsDeSuOrganizacion(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	pool := h.PoolSinRLS(t)
	acc := nuevoAcceso()
	svc := New(pool, acc.resolver)
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "public", "ready"), true)
	comentar(ctx, t, svc, persona("bea", "acme"), v, "hola")
	if err := svc.Denunciar(ctx, persona("ana", "acme"), v.ID, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Bloquear(ctx, persona("ana", "acme"), v.ID, "bea", false); err != nil {
		t.Fatal(err)
	}
	jefa, otra := administra("jefa", "acme"), administra("jefa-de-globex", "globex")
	if err := svc.Bloquear(ctx, jefa, v.ID, "bea", true); err != nil {
		t.Fatal(err)
	}
	id := cola(ctx, t, svc, jefa, false)[0].ID

	if ds := cola(ctx, t, svc, otra, false); len(ds) != 0 {
		t.Errorf("globex ve %d denuncias de acme", len(ds))
	}
	if err := svc.ResolverDenuncia(ctx, otra, id, AccionDespublicar); !errors.Is(err, ErrNoExiste) {
		t.Errorf("globex resuelve una denuncia de acme: %v", err)
	}
	if traza, _ := svc.Auditoria(ctx, otra, 100); len(traza) != 0 {
		t.Errorf("globex lee %d filas de la traza de acme", len(traza))
	}
	if l, _ := svc.BloqueadosEnOrg(ctx, otra); len(l) != 0 {
		t.Errorf("globex ve los bloqueos de acme: %+v", l)
	}
	if l, _ := svc.Bloqueados(ctx, persona("carla", "acme")); len(l) != 0 {
		t.Errorf("carla ve los bloqueos de ana: %+v", l)
	}
	if n, _ := svc.DenunciasAbiertas(ctx, otra); n != 0 {
		t.Errorf("globex cuenta %d denuncias de acme", n)
	}
	// La exportación: la dueña del vídeo denunciado no se lleva la denuncia de ana.
	if ds, _ := svc.DenunciasDe(ctx, persona("duenya", "acme")); len(ds) != 0 {
		t.Errorf("la denunciada exporta la denuncia contra ella: %+v", ds)
	}
	if ds, _ := svc.DenunciasDe(ctx, persona("ana", "acme")); len(ds) != 1 {
		t.Errorf("ana exporta %d denuncias, quiero la suya", len(ds))
	}
}

// La cola del operador: todas las organizaciones, también las denuncias contra
// quien las administra; resolver en una cola la cierra en la otra, y la traza de
// la organización dice que fue desde la plataforma y quién.
func TestDenuncia_ColaDelOperador(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	jefa := administra("jefa", "acme")
	vAcme := acc.conoce(videoEn(t, pool, "acme", "jefa", "public", "ready"), true)
	vGlobex := acc.conoce(videoEn(t, pool, "globex", "duenyo", "public", "ready"), true)
	c := comentar(ctx, t, svc, persona("bea", "acme"), vAcme, "comentario feo")
	zoe := persona("zoe", "initech")
	for _, p := range []struct {
		v Video
		d NuevaDenuncia
	}{
		{vAcme, NuevaDenuncia{Kind: DenunciaVideo, Reason: "violence"}}, // contra la jefa
		{vAcme, NuevaDenuncia{Kind: DenunciaComentario, Target: c.ID.String(), Reason: "harassment"}},
		{vGlobex, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam"}},
	} {
		if err := svc.Denunciar(ctx, zoe, p.v.ID, p.d); err != nil {
			t.Fatal(err)
		}
	}
	op := Quien{Sub: "operadora", Tenant: "corporate", Operador: true}

	// Sin la marca de operador no hay cola, administre lo que administre.
	for _, q := range []Quien{jefa, administra("jefa-corporate", "corporate"), persona("operadora", "corporate"), {Operador: true}} {
		if _, _, err := svc.DenunciasOperador(ctx, q, false, "", 0); !errors.Is(err, ErrProhibido) {
			t.Errorf("%+v abre la cola del operador: %v", q, err)
		}
		if err := svc.ResolverDenunciaOperador(ctx, Quien{Sub: q.Sub, Tenant: "acme", Admin: q.Admin}, c.ID, AccionDescartar); !errors.Is(err, ErrProhibido) {
			t.Errorf("%+v resuelve como operador: %v", q, err)
		}
	}
	ds, _, err := svc.DenunciasOperador(ctx, op, false, "", 0)
	if err != nil || len(ds) != 3 {
		t.Fatalf("la operadora ve %d de 3: %v", len(ds), err)
	}
	orgs := map[string]int{}
	var deComentario, contraJefa Denuncia
	for _, d := range ds {
		orgs[d.Tenant]++
		switch {
		case d.Kind == DenunciaComentario:
			deComentario = d
		case d.TargetSub == "jefa":
			contraJefa = d
		}
	}
	if orgs["acme"] != 2 || orgs["globex"] != 1 || contraJefa.ID == uuid.Nil {
		t.Fatalf("cola del operador: %v", orgs)
	}
	b, _ := json.Marshal(ds)
	if strings.Contains(string(b), "zoe") || strings.Contains(string(b), "initech") {
		t.Fatalf("la cola del operador dice quién denunció: %s", b)
	}

	// La organización tiene que ser la de la denuncia: en otra, no existe.
	if err := svc.ResolverDenunciaOperador(ctx, Quien{Sub: "operadora", Tenant: "globex", Operador: true}, deComentario.ID, AccionBorrar); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("resolver en otra organización: %v", err)
	}
	if org, err := svc.OrgDeDenuncia(ctx, deComentario.ID); err != nil || org != "acme" {
		t.Fatalf("OrgDeDenuncia: %q %v", org, err)
	}
	en := op
	en.Tenant = "acme"
	if err := svc.ResolverDenunciaOperador(ctx, en, deComentario.ID, AccionBorrar); err != nil {
		t.Fatal(err)
	}
	// Cerrada en las dos colas, y la de la organización dice desde dónde.
	if abiertas := cola(ctx, t, svc, jefa, false); len(abiertas) != 0 {
		t.Fatalf("sigue abierta para la jefa: %+v", abiertas)
	}
	res := cola(ctx, t, svc, jefa, true)
	if len(res) != 1 || res[0].ResolvedFrom != "platform" || res[0].Resolution != AccionBorrar {
		t.Fatalf("resuelta por la operadora, vista por la jefa: %+v", res)
	}
	traza, _ := svc.Auditoria(ctx, jefa, 100)
	var vista bool
	for _, e := range traza {
		var det map[string]any
		_ = json.Unmarshal(e.Detail, &det)
		if e.Action == "report.actioned" {
			vista = e.Actor == "operadora" && det["from"] == "platform"
		}
	}
	if !vista {
		t.Fatalf("la traza de acme no dice que actuó la operadora desde la plataforma: %+v", traza)
	}

	// Al revés: la que resuelve la organización sale cerrada en la del operador.
	otra := administra("otra-jefa", "globex")
	if err := svc.ResolverDenuncia(ctx, otra, cola(ctx, t, svc, otra, false)[0].ID, AccionRetirar); err != nil {
		t.Fatal(err)
	}
	if ds, _, _ := svc.DenunciasOperador(ctx, op, false, "", 0); len(ds) != 1 || ds[0].ID != contraJefa.ID {
		t.Fatalf("abiertas para la operadora: %+v", ds)
	}
	res, _, _ = svc.DenunciasOperador(ctx, op, true, "", 0)
	if len(res) != 2 || res[0].ResolvedFrom != "org" || res[1].ResolvedFrom != "platform" {
		t.Fatalf("resueltas para la operadora: %+v", res)
	}
}

// La purga: las resueltas o descartadas hace más de un año se borran, con su copia
// de lo denunciado; las abiertas se quedan aunque sean viejas, las resueltas hace
// menos también, y la traza no se toca. De todas las organizaciones.
func TestDenuncia_Purga(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	zoe := persona("zoe", "initech")
	ids := map[string]uuid.UUID{}
	for _, org := range []string{"acme", "globex"} {
		v := acc.conoce(videoEn(t, pool, org, "duenya", "public", "ready"), true)
		c := comentar(ctx, t, svc, persona("bea", org), v, "uno")
		c2 := comentar(ctx, t, svc, persona("bea", org), v, "dos")
		for k, d := range map[string]NuevaDenuncia{
			"abierta-vieja":     {Kind: DenunciaVideo, Reason: "spam"},
			"resuelta-vieja":    {Kind: DenunciaComentario, Target: c.ID.String(), Reason: "harassment"},
			"resuelta-reciente": {Kind: DenunciaComentario, Target: c2.ID.String(), Reason: "harassment"},
		} {
			if err := svc.Denunciar(ctx, zoe, v.ID, d); err != nil {
				t.Fatal(err)
			}
			if err := svc.store.enOrg(ctx, org, func(tx pgx.Tx) error {
				var id uuid.UUID
				if err := tx.QueryRow(ctx, `SELECT id FROM reports WHERE tenant = $1 AND kind = $2 AND target_id = $3`,
					org, d.Kind, map[bool]string{true: v.ID.String(), false: d.Target}[d.Kind == DenunciaVideo]).Scan(&id); err != nil {
					return err
				}
				ids[org+"/"+k] = id
				hace := map[string]string{"abierta-vieja": "2 years", "resuelta-vieja": "13 months", "resuelta-reciente": "11 months"}[k]
				estado := "'dismissed'"
				if k == "abierta-vieja" {
					estado = "status"
				}
				_, err := tx.Exec(ctx, `UPDATE reports SET status = `+estado+`, created_at = now() - $2::interval,
					resolved_at = CASE WHEN `+estado+` = 'open' THEN NULL ELSE now() - $2::interval END WHERE id = $1`, id, hace)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	trazaAntes, _ := svc.Auditoria(ctx, administra("jefa", "acme"), 100)

	n, err := svc.PurgarDenuncias(ctx)
	if err != nil || n != 2 {
		t.Fatalf("purgadas %d (%v), quiero las 2 resueltas hace más de un año", n, err)
	}
	for k, id := range ids {
		var queda bool
		org := strings.Split(k, "/")[0]
		if err := svc.store.enOrg(ctx, org, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM reports WHERE id = $1)`, id).Scan(&queda)
		}); err != nil {
			t.Fatal(err)
		}
		if quiero := !strings.HasSuffix(k, "resuelta-vieja"); queda != quiero {
			t.Errorf("%s: queda=%v, quiero %v", k, queda, quiero)
		}
	}
	if trazaDespues, _ := svc.Auditoria(ctx, administra("jefa", "acme"), 100); len(trazaDespues) != len(trazaAntes) {
		t.Fatalf("la purga tocó la traza: %d filas antes, %d después", len(trazaAntes), len(trazaDespues))
	}
}

// «Exportar tus datos»: las denuncias que puso ella, de cualquier organización,
// sin copia de lo denunciado; nunca las que otros pusieron contra ella, ni las
// anónimas.
func TestDenuncia_ExportarLasPropias(t *testing.T) {
	svc, acc, pool := moderable(t)
	ctx := ctxCorto(t)
	zoe, bea := persona("zoe", "globex"), persona("bea", "acme")
	v := acc.conoce(videoEn(t, pool, "acme", "duenya", "public", "ready"), true)
	vg := acc.conoce(videoEn(t, pool, "initech", "otro", "public", "ready"), true)
	comentar(ctx, t, svc, zoe, v, "escribe zoe")
	for _, p := range []struct {
		q Quien
		v Video
		d NuevaDenuncia
	}{
		{zoe, v, NuevaDenuncia{Kind: DenunciaVideo, Reason: "spam", Details: "lo de zoe"}},
		{zoe, vg, NuevaDenuncia{Kind: DenunciaVideo, Reason: "violence"}},
		{bea, v, NuevaDenuncia{Kind: DenunciaPersona, Target: "zoe", Reason: "harassment"}},
		{anonimo("203.0.113.9"), v, NuevaDenuncia{Kind: DenunciaPersona, Target: "zoe", Reason: "other"}},
	} {
		if err := svc.Denunciar(ctx, p.q, p.v.ID, p.d); err != nil {
			t.Fatal(err)
		}
	}
	mias, err := svc.DenunciasDe(ctx, zoe)
	if err != nil || len(mias) != 2 {
		t.Fatalf("las de zoe: %+v, %v; quiero sus 2, de dos organizaciones", mias, err)
	}
	for _, d := range mias {
		if d.Kind != DenunciaVideo {
			t.Fatalf("a zoe le sale una denuncia que no puso: %+v", d)
		}
	}
	if de, _ := svc.DenunciasDe(ctx, bea); len(de) != 1 || de[0].TargetID != "zoe" {
		t.Fatalf("las de bea: %+v", de)
	}
	if _, err := svc.DenunciasDe(ctx, anonimo("203.0.113.9")); !errors.Is(err, ErrProhibido) {
		t.Fatalf("sin cuenta: %v", err)
	}
}
