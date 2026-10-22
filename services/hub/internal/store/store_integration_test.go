// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

// Tests de integracion de Sytue Hub contra un PostgreSQL real (efimero, via
// platform/pkg/pgtest sobre testcontainers-go). Detras del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - que las migraciones del servicio aplican (esquema + RLS ENABLE+FORCE),
//   - notificaciones: crear, listar, contar no leidas, MarkRead y MarkAllRead,
//   - presencia: Heartbeat + GetPresence + SetOffline,
//   - AISLAMIENTO por tenant (RLS): datos de un tenant no visibles para otro.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// tenantCtx devuelve un contexto con el tenant fijado (lo consume RLS).
func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// sampleNotification construye una NewNotification valida para un usuario dado.
func sampleNotification(subject, title string) domain.NewNotification {
	return domain.NewNotification{
		Subject: subject,
		Source:  domain.SourceDrive,
		Type:    "share",
		Level:   domain.LevelInfo,
		Title:   title,
		Body:    "cuerpo de " + title,
		URL:     "https://sytue.eu/drive",
	}
}

// TestMigrationsApply verifica que las migraciones embebidas aplican limpiamente
// contra un PostgreSQL real y que las tablas clave llevan RLS ENABLE+FORCE.
func TestMigrationsApply(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	ctx := context.Background()

	for _, tbl := range []string{"notifications", "presence"} {
		var relrowsecurity, relforcerowsecurity bool
		err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity
			 FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&relrowsecurity, &relforcerowsecurity)
		if err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !relrowsecurity || !relforcerowsecurity {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, relrowsecurity, relforcerowsecurity)
		}
	}
}

// TestNotificationsLifecycle ejercita el ciclo de notificaciones a traves del
// store (que fija app.tenant por RLS): crear varias, listar, contar no leidas,
// marcar una como leida y marcar todas.
func TestNotificationsLifecycle(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	const sub = "user-1"

	// Crear tres notificaciones para el mismo usuario.
	var created []domain.Notification
	for _, title := range []string{"n1", "n2", "n3"} {
		n, err := st.CreateNotification(ctx, sampleNotification(sub, title))
		if err != nil {
			t.Fatalf("CreateNotification(%q): %v", title, err)
		}
		if n.ID == uuid.Nil || n.Read {
			t.Fatalf("notificacion inesperada: %+v", n)
		}
		if n.Title != title || n.Source != domain.SourceDrive {
			t.Fatalf("campos de notificacion inesperados: %+v", n)
		}
		created = append(created, n)
	}

	// Ruido: notificacion de OTRO usuario del mismo tenant; no debe contar.
	if _, err := st.CreateNotification(ctx, sampleNotification("user-2", "ruido")); err != nil {
		t.Fatalf("CreateNotification(user-2): %v", err)
	}

	// Listar todas las del usuario (feed keyset descendente).
	list, err := st.ListNotifications(ctx, []string{sub}, false, nil, 50)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListNotifications devolvio %d, quiero 3: %+v", len(list), list)
	}

	// Conteo de no leidas del usuario (excluye a user-2).
	count, err := st.UnreadCount(ctx, []string{sub})
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 3 {
		t.Fatalf("UnreadCount=%d, quiero 3", count)
	}

	// Marcar una como leida.
	if err := st.MarkRead(ctx, created[0].ID, []string{sub}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	count, err = st.UnreadCount(ctx, []string{sub})
	if err != nil {
		t.Fatalf("UnreadCount tras MarkRead: %v", err)
	}
	if count != 2 {
		t.Fatalf("UnreadCount tras MarkRead=%d, quiero 2", count)
	}

	// Filtrar solo no leidas.
	unread, err := st.ListNotifications(ctx, []string{sub}, true, nil, 50)
	if err != nil {
		t.Fatalf("ListNotifications(onlyUnread): %v", err)
	}
	if len(unread) != 2 {
		t.Fatalf("ListNotifications(onlyUnread) devolvio %d, quiero 2", len(unread))
	}

	// MarkRead de un id inexistente -> ErrNotFound.
	if err := st.MarkRead(ctx, uuid.New(), []string{sub}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("MarkRead(inexistente): esperaba ErrNotFound, obtuve %v", err)
	}

	// Marcar todas las restantes como leidas.
	affected, err := st.MarkAllRead(ctx, []string{sub})
	if err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if affected != 2 {
		t.Fatalf("MarkAllRead afecto %d filas, quiero 2", affected)
	}
	count, err = st.UnreadCount(ctx, []string{sub})
	if err != nil {
		t.Fatalf("UnreadCount tras MarkAllRead: %v", err)
	}
	if count != 0 {
		t.Fatalf("UnreadCount tras MarkAllRead=%d, quiero 0", count)
	}
}

// TestPresenceLifecycle comprueba Heartbeat + GetPresence + SetOffline y la
// derivacion a offline por TTL vencida.
func TestPresenceLifecycle(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	const sub = "user-1"
	const ttl = time.Minute

	// Sin heartbeat previo, GetPresence omite al usuario.
	got, err := st.GetPresence(ctx, []string{sub}, ttl)
	if err != nil {
		t.Fatalf("GetPresence (vacio): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetPresence sin heartbeat devolvio %+v, quiero vacio", got)
	}

	// Heartbeat online.
	if err := st.Heartbeat(ctx, sub, domain.StateOnline, "en una llamada"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	got, err = st.GetPresence(ctx, []string{sub}, ttl)
	if err != nil {
		t.Fatalf("GetPresence tras heartbeat: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetPresence devolvio %d filas, quiero 1: %+v", len(got), got)
	}
	if got[0].Subject != sub || got[0].State != domain.StateOnline || got[0].Status != "en una llamada" {
		t.Fatalf("presencia inesperada: %+v", got[0])
	}

	// Con TTL cero, el heartbeat se considera obsoleto -> offline derivado.
	stale, err := st.GetPresence(ctx, []string{sub}, 0)
	if err != nil {
		t.Fatalf("GetPresence (ttl=0): %v", err)
	}
	if len(stale) != 1 || stale[0].State != domain.StateOffline {
		t.Fatalf("presencia con ttl=0 no derivo a offline: %+v", stale)
	}

	// SetOffline explicito.
	if err := st.SetOffline(ctx, sub); err != nil {
		t.Fatalf("SetOffline: %v", err)
	}
	got, err = st.GetPresence(ctx, []string{sub}, ttl)
	if err != nil {
		t.Fatalf("GetPresence tras SetOffline: %v", err)
	}
	if len(got) != 1 || got[0].State != domain.StateOffline {
		t.Fatalf("presencia tras SetOffline no es offline: %+v", got)
	}
}

// TestTenantIsolation es la prueba nuclear de RLS (CONVENTIONS §0.7/§10): los
// datos de un tenant NO son visibles ni contabilizables desde otro tenant,
// aunque se conozca el ID de la fila o el subject.
func TestTenantIsolation(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)

	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	// Un `sub` es de UNA persona en toda la suite (lo emite Sytue ID, único para todas
	// las organizaciones). Sus avisos la siguen de una organización a otra (ADR 0053,
	// TestAvisosDeLaPersona); lo que sigue aislado por organización es la presencia.
	const sub = "user-shared"

	// Tenant A crea una notificacion y registra presencia.
	nA, err := st.CreateNotification(ctxA, sampleNotification(sub, "solo-a"))
	if err != nil {
		t.Fatalf("A CreateNotification: %v", err)
	}
	if err := st.Heartbeat(ctxA, sub, domain.StateOnline, "a"); err != nil {
		t.Fatalf("A Heartbeat: %v", err)
	}

	// Otra persona de B no ve, no cuenta ni marca el aviso de A.
	if listB, _ := st.ListNotifications(ctxB, []string{"otra"}, false, nil, 50); len(listB) != 0 {
		t.Fatalf("aislamiento roto: otra persona de B ve avisos de A: %+v", listB)
	}
	if countB, _ := st.UnreadCount(ctxB, []string{"otra"}); countB != 0 {
		t.Fatalf("aislamiento roto: otra persona de B cuenta %d no leidas de A", countB)
	}
	if err := st.MarkRead(ctxB, nA.ID, []string{"otra"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: otra persona de B marca el aviso de A (err=%v)", err)
	}
	// Tenant B NO ve la presencia de A.
	presB, err := st.GetPresence(ctxB, []string{sub}, time.Minute)
	if err != nil {
		t.Fatalf("B GetPresence: %v", err)
	}
	if len(presB) != 0 {
		t.Fatalf("aislamiento roto: B ve la presencia de A: %+v", presB)
	}

	// A sigue viendo lo suyo intacto.
	listA, err := st.ListNotifications(ctxA, []string{sub}, false, nil, 50)
	if err != nil {
		t.Fatalf("A ListNotifications: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != nA.ID {
		t.Fatalf("A perdio su notificacion: %+v", listA)
	}
	countA, err := st.UnreadCount(ctxA, []string{sub})
	if err != nil {
		t.Fatalf("A UnreadCount: %v", err)
	}
	if countA != 1 {
		t.Fatalf("A UnreadCount=%d, quiero 1", countA)
	}

	// B crea su propia presencia con el mismo subject; no colisiona con A.
	if err := st.Heartbeat(ctxB, sub, domain.StateBusy, "b"); err != nil {
		t.Fatalf("B Heartbeat: %v", err)
	}
	presA, err := st.GetPresence(ctxA, []string{sub}, time.Minute)
	if err != nil {
		t.Fatalf("A GetPresence: %v", err)
	}
	if len(presA) != 1 || presA[0].State != domain.StateOnline || presA[0].Status != "a" {
		t.Fatalf("A ve presencia alterada por B: %+v", presA)
	}
}

// Las preferencias son EXCEPCIONES: solo se guarda lo que el usuario cambia, y volver
// al estado por defecto BORRA la fila en vez de dejar un «no silenciado» que no dice
// nada. Se prueba contra Postgres real porque lo que importa es el upsert por clave
// primaria (tenant, subject, source, kind) y que la RLS deje ver solo lo del tenant.
func TestNotificationPrefs(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	const sub = "user-prefs"

	// Sin configurar nada: ninguna excepción, así que todo llega.
	prefs, err := st.ListNotificationPrefs(ctx, sub)
	if err != nil {
		t.Fatalf("ListNotificationPrefs: %v", err)
	}
	if len(prefs) != 0 {
		t.Fatalf("sin configurar no debe haber reglas, hay %d", len(prefs))
	}

	// Silenciar un origen entero y una excepción por tipo dentro de otro.
	for _, p := range []domain.NotificationPref{
		{Source: domain.SourceMail, Kind: "", Muted: true},
		{Source: domain.SourceDrive, Kind: "comment", Muted: true},
	} {
		if err := st.SetNotificationPref(ctx, sub, p); err != nil {
			t.Fatalf("SetNotificationPref(%v): %v", p, err)
		}
	}
	prefs, err = st.ListNotificationPrefs(ctx, sub)
	if err != nil {
		t.Fatalf("ListNotificationPrefs: %v", err)
	}
	if len(prefs) != 2 {
		t.Fatalf("se guardaron %d reglas, quiero 2: %+v", len(prefs), prefs)
	}
	if !domain.Muted(prefs, domain.SourceMail, "digest") {
		t.Error("el origen silenciado debe silenciar sus tipos")
	}
	if domain.Muted(prefs, domain.SourceDrive, "share") {
		t.Error("un tipo silenciado no puede silenciar el origen entero")
	}

	// Volver a permitir el origen: la fila desaparece (el defecto ya es «llega»).
	if err := st.SetNotificationPref(ctx, sub,
		domain.NotificationPref{Source: domain.SourceMail, Muted: false}); err != nil {
		t.Fatalf("SetNotificationPref(unmute): %v", err)
	}
	prefs, _ = st.ListNotificationPrefs(ctx, sub)
	if len(prefs) != 1 {
		t.Fatalf("volver al defecto debe BORRAR la regla, quedan %+v", prefs)
	}
	if domain.Muted(prefs, domain.SourceMail, "digest") {
		t.Error("tras quitar la regla, el correo debe volver a llegar")
	}

	// Upsert, no duplicado: repetir la misma clave actualiza.
	for i := 0; i < 3; i++ {
		if err := st.SetNotificationPref(ctx, sub,
			domain.NotificationPref{Source: domain.SourceDrive, Kind: "comment", Muted: true}); err != nil {
			t.Fatalf("SetNotificationPref(repetido): %v", err)
		}
	}
	prefs, _ = st.ListNotificationPrefs(ctx, sub)
	if len(prefs) != 1 {
		t.Fatalf("la misma regla no puede duplicarse: %+v", prefs)
	}

	// Otro tenant no ve nada de este (RLS).
	otros, err := st.ListNotificationPrefs(tenantCtx("otra"), sub)
	if err != nil {
		t.Fatalf("ListNotificationPrefs(otro tenant): %v", err)
	}
	if len(otros) != 0 {
		t.Fatalf("fuga entre tenants: %+v", otros)
	}
}

// UN AVISO ES DE LA PERSONA (ADR 0053): quien lo recibe lo ve y lo marca desde la
// sesión de cualquiera de sus organizaciones, por sus buzones PERSONALES (su `sub` y
// su `mailbox`). Y NADIE MÁS, por ninguna vía: ni otra persona de la misma
// organización, ni otra de otra organización, ni un token sin buzones. El correo de
// entrar no es personal: su aviso se queda en la organización que lo mandó. Se prueba
// contra Postgres de verdad porque lo que se vigila es la RLS.
func TestAvisosDeLaPersona(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	// El contexto que arma el handler con el token: la organización y los buzones personales.
	sesion := func(org context.Context, personales ...string) context.Context {
		return domain.ConBuzonesPersonales(org, personales, time.Now())
	}

	// Bea es de globex y comenta en un documento de acme: su aviso vive en acme.
	aviso, err := st.CreateNotification(acme, sampleNotification("bea", "para bea"))
	if err != nil {
		t.Fatal(err)
	}
	// Y uno a su buzón de globex, como una invitación de Calendar desde acme.
	if _, err := st.CreateNotification(acme, sampleNotification("mail:bea@globex.test", "a su buzón")); err != nil {
		t.Fatal(err)
	}
	// Y uno a su correo de ENTRAR, que no es personal: no debe cruzar.
	if _, err := st.CreateNotification(acme, sampleNotification("mail:bea@gmail.test", "a su correo de entrar")); err != nil {
		t.Fatal(err)
	}
	bea := []string{"bea", "mail:bea@gmail.test", "mail:bea@globex.test"}
	beaEnGlobex := sesion(globex, "bea", "mail:bea@globex.test")

	t.Run("la destinataria ve los suyos desde su organización, sin el de su correo de entrar", func(t *testing.T) {
		l, _ := st.ListNotifications(beaEnGlobex, bea, false, nil, 50)
		if len(l) != 2 || l[0].Tenant != "acme" {
			t.Fatalf("bea desde globex ve %+v", l)
		}
		for _, n := range l {
			if n.Subject == "mail:bea@gmail.test" {
				t.Errorf("el aviso a su correo de entrar cruza de organización")
			}
		}
		if c, _ := st.UnreadCount(beaEnGlobex, bea); c != 2 {
			t.Fatalf("bea desde globex cuenta %d", c)
		}
	})

	ajenas := map[string]struct {
		ctx  context.Context
		dest []string
	}{
		"otra persona de la misma organización": {sesion(acme, "ana", "mail:ana@acme.test"), []string{"ana", "mail:ana@acme.test"}},
		"otra persona de otra organización":     {sesion(globex, "carla", "mail:carla@globex.test"), []string{"carla", "mail:carla@globex.test"}},
		"un token sin buzones":                  {sesion(globex), nil},
		"un token sin buzones en la anfitriona": {acme, []string{}},
	}
	for nombre, v := range ajenas {
		t.Run(nombre+" no lo lee ni lo marca", func(t *testing.T) {
			if l, _ := st.ListNotifications(v.ctx, v.dest, false, nil, 50); len(l) != 0 {
				t.Errorf("lee avisos ajenos: %+v", l)
			}
			if c, _ := st.UnreadCount(v.ctx, v.dest); c != 0 {
				t.Errorf("cuenta %d avisos ajenos", c)
			}
			if err := st.MarkRead(v.ctx, aviso.ID, v.dest); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("marca un aviso ajeno (err=%v)", err)
			}
			if n, _ := st.MarkAllRead(v.ctx, v.dest); n != 0 {
				t.Errorf("marca %d avisos ajenos", n)
			}
		})
	}

	// Lo ajeno no ha tocado nada: siguen sin leer, y los marca ella.
	if c, _ := st.UnreadCount(beaEnGlobex, bea); c != 2 {
		t.Fatalf("tras los intentos ajenos, bea tiene %d sin leer", c)
	}
	if err := st.MarkRead(beaEnGlobex, aviso.ID, bea); err != nil {
		t.Fatalf("bea no puede marcar su aviso desde globex: %v", err)
	}
	if n, _ := st.MarkAllRead(beaEnGlobex, bea); n != 1 {
		t.Fatalf("bea marca %d de una vez, quiero 1", n)
	}
	// El de su correo de entrar sigue sin leer, en acme.
	if c, _ := st.UnreadCount(sesion(acme, "bea"), []string{"mail:bea@gmail.test"}); c != 1 {
		t.Fatalf("el aviso a su correo de entrar: %d sin leer en acme, quiero 1", c)
	}
}
