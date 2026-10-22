// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// storeSinRLS conecta como el DUEÑO de la base: superusuario, con BYPASSRLS. Para ese
// rol PostgreSQL no evalúa ninguna política, y es como ha llegado a conectar el
// despliegue. Lo que separa a una organización de otra es solo lo que diga el SQL.
func storeSinRLS(t *testing.T) *store.Store {
	t.Helper()
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	return store.New(h.PoolSinRLS(t))
}

// COM-15. El reparto de los avisos —los de la organización de la sesión, más los que
// son de la PERSONA en cualquiera— lo hacían dos políticas de fila. Sin ellas, la
// consulta buscaba el `sub` en todas las organizaciones para cualquiera que lo trajera,
// también para un token delegado.
func TestSinRLSLosAvisosSonDeLaOrganizacionODeLaPersona(t *testing.T) {
	st := storeSinRLS(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	sesion := func(org context.Context, personales ...string) context.Context {
		return domain.ConBuzonesPersonales(org, personales, time.Now())
	}

	// Bea trabaja en globex. Tres avisos suyos viven en ACME: a su identidad, a su
	// buzón de globex (una invitación) y a su correo de entrar.
	aviso, err := st.CreateNotification(acme, sampleNotification("bea", "Nuevo correo de Dirección"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{"mail:bea@globex.test", "mail:bea@gmail.test"} {
		if _, err := st.CreateNotification(acme, sampleNotification(dest, "a "+dest)); err != nil {
			t.Fatal(err)
		}
	}
	// Lo que el handler saca del token: los tres buzones.
	bea := []string{"bea", "mail:bea@gmail.test", "mail:bea@globex.test"}

	t.Run("un token delegado no lee lo de la persona en otra organización", func(t *testing.T) {
		// Delegado: el handler NO le da buzones personales. La sesión es de globex.
		delegado := sesion(globex)
		if l, _ := st.ListNotifications(delegado, bea, false, nil, 50); len(l) != 0 {
			t.Errorf("lee %d avisos de acme: %q…", len(l), l[0].Title)
		}
		if c, _ := st.UnreadCount(delegado, bea); c != 0 {
			t.Errorf("cuenta %d avisos de acme", c)
		}
		if err := st.MarkRead(delegado, aviso.ID, bea); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("marca como leído un aviso de acme (err=%v)", err)
		}
		if n, _ := st.MarkAllRead(delegado, bea); n != 0 {
			t.Errorf("marca %d avisos de acme", n)
		}
	})

	t.Run("su correo de entrar no cruza de organización", func(t *testing.T) {
		l, err := st.ListNotifications(sesion(globex, "bea", "mail:bea@globex.test"), bea, false, nil, 50)
		if err != nil || len(l) != 2 {
			t.Fatalf("bea desde globex ve %d avisos (%v), quiero los 2 que son de la persona", len(l), err)
		}
		for _, n := range l {
			if n.Subject == "mail:bea@gmail.test" {
				t.Errorf("el aviso a su correo de entrar ha cruzado de organización")
			}
		}
	})

	t.Run("otra persona no lee nada, esté donde esté", func(t *testing.T) {
		for _, c := range []context.Context{sesion(acme, "ana"), sesion(globex, "carla")} {
			if l, _ := st.ListNotifications(c, []string{"ana", "carla"}, false, nil, 50); len(l) != 0 {
				t.Errorf("lee avisos ajenos: %+v", l)
			}
		}
	})

	t.Run("la persona sí lee y marca lo suyo", func(t *testing.T) {
		suya := sesion(globex, "bea", "mail:bea@globex.test")
		if c, _ := st.UnreadCount(suya, bea); c != 2 {
			t.Fatalf("bea cuenta %d sin leer, quiero 2", c)
		}
		if err := st.MarkRead(suya, aviso.ID, bea); err != nil {
			t.Fatalf("bea no puede marcar su aviso: %v", err)
		}
		if n, _ := st.MarkAllRead(suya, bea); n != 1 {
			t.Fatalf("bea marca %d de una vez, quiero 1", n)
		}
		// El de su correo de entrar sigue sin leer, y se lee desde acme.
		if c, _ := st.UnreadCount(sesion(acme, "bea"), bea); c != 1 {
			t.Fatalf("en acme le queda %d sin leer, quiero 1", c)
		}
	})
}

// Las reglas de silencio son de la organización en la que se ponen: la pantalla de
// preferencias de una no enseña ni borra las de otra.
func TestSinRLSLasPreferenciasSonDeSuOrganizacion(t *testing.T) {
	st := storeSinRLS(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")

	if err := st.SetNotificationPref(acme, "ana", domain.NotificationPref{Source: domain.SourceDrive, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if prefs, err := st.ListNotificationPrefs(globex, "ana"); err != nil || len(prefs) != 0 {
		t.Fatalf("desde globex se ven las reglas de acme: %+v (%v)", prefs, err)
	}
	// «Quitar el silencio» desde globex es un DELETE: no puede llevarse la regla de acme.
	if err := st.SetNotificationPref(globex, "ana", domain.NotificationPref{Source: domain.SourceDrive, Muted: false}); err != nil {
		t.Fatal(err)
	}
	prefs, err := st.ListNotificationPrefs(acme, "ana")
	if err != nil || len(prefs) != 1 || !prefs[0].Muted {
		t.Fatalf("la regla de acme ya no está: %+v (%v)", prefs, err)
	}
}
