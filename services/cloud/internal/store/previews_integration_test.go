// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Una vista previa no hereda nada de producción (CLOUD-01).
//
// Lo que decide qué recibe un despliegue son dos consultas del store —las
// variables por ámbito y las credenciales de los servicios por destino—, y por
// ellas pasan todos los que inyectan algo: el despliegue, las funciones, los
// workers y DB Studio. Estos tests las ejercitan contra PostgreSQL de verdad.
package store_test

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestLaVistaPreviaNoHeredaLosSecretosDeProduccion(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	const tenant = "acme"
	ctx := tenantCtx(tenant)
	p := seedProject(t, ctx, st, "web")

	poner := func(key string, scope domain.EnvScope, secreto bool, valor string) {
		t.Helper()
		rec := store.EnvVarRecord{ProjectID: p.ID, Key: key, Scope: scope, IsSecret: secreto}
		if secreto {
			rec.ValueEnc = []byte(valor) // el store no descifra: le vale cualquier bytea
		} else {
			rec.ValuePlain = valor
		}
		if _, err := st.UpsertEnvVar(ctx, rec); err != nil {
			t.Fatalf("UpsertEnvVar(%s, %s): %v", key, scope, err)
		}
	}
	poner("STRIPE_KEY", domain.ScopeAll, true, "sk_live")         // secreto de producción «para todo»
	poner("SMTP_PASSWORD", domain.ScopeProduction, true, "prod")  // secreto solo de producción
	poner("PUBLIC_URL", domain.ScopeAll, false, "https://x")      // configuración, no secreto
	poner("MODO", domain.ScopeAll, false, "compartido")           // configuración con valor propio abajo
	poner("MODO", domain.ScopePreview, false, "previa")           // ...que tiene que ganar
	poner("PAGOS_KEY", domain.ScopeAll, true, "sk_live_pagos")    // secreto compartido...
	poner("PAGOS_KEY", domain.ScopePreview, true, "sk_test_pago") // ...con uno de pruebas para vistas previas

	// --- Vista previa -------------------------------------------------------
	previa, err := st.ResolveEnvForDeploy(ctx, tenant, p.ID, domain.TargetPreview)
	if err != nil {
		t.Fatalf("ResolveEnvForDeploy(preview): %v", err)
	}
	ultimo := map[string]store.ResolvedEnvVar{}
	for _, v := range previa {
		if v.IsSecret && v.Scope != domain.ScopePreview {
			t.Errorf("la vista previa recibió el secreto %s de ámbito %q", v.Key, v.Scope)
		}
		if v.Scope == domain.ScopeProduction {
			t.Errorf("la vista previa recibió %s de ámbito production", v.Key)
		}
		ultimo[v.Key] = v // quien recorre el resultado se queda con el último
	}
	if _, hay := ultimo["STRIPE_KEY"]; hay {
		t.Error("la vista previa recibió STRIPE_KEY, que es un secreto de producción")
	}
	if _, hay := ultimo["SMTP_PASSWORD"]; hay {
		t.Error("la vista previa recibió SMTP_PASSWORD, que es solo de producción")
	}
	if got := ultimo["PUBLIC_URL"]; got.Value != "https://x" {
		t.Errorf("la configuración que no es secreta sí se comparte: PUBLIC_URL = %+v", got)
	}
	if got := ultimo["MODO"]; got.Value != "previa" {
		t.Errorf("el valor propio de la vista previa tiene que llegar el último: MODO = %+v", got)
	}
	if got := ultimo["PAGOS_KEY"]; string(got.ValueEnc) != "sk_test_pago" {
		t.Errorf("PAGOS_KEY de la vista previa = %q; se esperaba el de pruebas", got.ValueEnc)
	}

	// --- Producción: exactamente lo de siempre -----------------------------
	prod, err := st.ResolveEnvForDeploy(ctx, tenant, p.ID, domain.TargetProduction)
	if err != nil {
		t.Fatalf("ResolveEnvForDeploy(production): %v", err)
	}
	var clavesProd []string
	for _, v := range prod {
		if v.Scope == domain.ScopePreview {
			t.Errorf("producción recibió %s de ámbito preview", v.Key)
		}
		clavesProd = append(clavesProd, v.Key)
	}
	slices.Sort(clavesProd)
	if quiero := []string{"MODO", "PAGOS_KEY", "PUBLIC_URL", "SMTP_PASSWORD", "STRIPE_KEY"}; !slices.Equal(clavesProd, quiero) {
		t.Errorf("producción recibe %v; se esperaba %v", clavesProd, quiero)
	}

	// --- Lo que falta, con nombres ------------------------------------------
	falta, err := st.PreviewWithheld(ctx, tenant, p.ID)
	if err != nil {
		t.Fatalf("PreviewWithheld: %v", err)
	}
	if quiero := []string{"SMTP_PASSWORD", "STRIPE_KEY"}; !slices.Equal(falta.SecretKeys, quiero) {
		t.Errorf("secretos sin valor de vista previa = %v; se esperaba %v", falta.SecretKeys, quiero)
	}
}

func TestLasCredencialesDeUnServicioVanSoloASuDestino(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	const tenant = "acme"
	ctx := tenantCtx(tenant)
	p := seedProject(t, ctx, st, "web")

	crear := func(nombre string, target domain.DeployTarget, creds string) {
		t.Helper()
		m, err := st.ProvisionService(ctx, store.NewServiceRecord{
			ProjectID: p.ID, Kind: domain.ServicePostgres, Name: nombre, Target: target,
		}, 0)
		if err != nil {
			t.Fatalf("ProvisionService(%s): %v", nombre, err)
		}
		if err := st.MarkServiceReady(ctx, tenant, m.ID, map[string]string{"host": nombre}, []byte(creds)); err != nil {
			t.Fatalf("MarkServiceReady(%s): %v", nombre, err)
		}
	}
	credsDe := func(target domain.DeployTarget) string {
		t.Helper()
		c, err := st.ResolveServiceCreds(ctx, tenant, p.ID, target)
		if err != nil {
			t.Fatalf("ResolveServiceCreds(%s): %v", target, err)
		}
		return string(c[string(domain.ServicePostgres)])
	}

	// Un servicio creado sin decir destino es de producción: es lo que eran todos.
	crear("db", "", "dsn-de-produccion")

	if got := credsDe(domain.TargetProduction); got != "dsn-de-produccion" {
		t.Errorf("producción recibe %q; se esperaba su base de datos", got)
	}
	if got := credsDe(domain.TargetPreview); got != "" {
		t.Errorf("la vista previa recibió las credenciales de producción: %q", got)
	}
	// Un destino desconocido no abre la puerta de las vistas previas ni deja de
	// ser producción.
	if got := credsDe("cualquiera"); got != "dsn-de-produccion" {
		t.Errorf("un destino desconocido tiene que tratarse como producción; recibió %q", got)
	}
	falta, err := st.PreviewWithheld(ctx, tenant, p.ID)
	if err != nil {
		t.Fatalf("PreviewWithheld: %v", err)
	}
	if !slices.Equal(falta.ServiceKinds, []string{"postgres"}) {
		t.Errorf("servicios sin equivalente de vista previa = %v; se esperaba [postgres]", falta.ServiceKinds)
	}

	// Con una base propia para vistas previas, cada destino recibe la suya.
	crear("db-previa", domain.TargetPreview, "dsn-de-vista-previa")

	if got := credsDe(domain.TargetPreview); got != "dsn-de-vista-previa" {
		t.Errorf("la vista previa recibe %q; se esperaba su propia base", got)
	}
	if got := credsDe(domain.TargetProduction); got != "dsn-de-produccion" {
		t.Errorf("producción recibe %q tras crear la base de vistas previas", got)
	}
	if falta, err = st.PreviewWithheld(ctx, tenant, p.ID); err != nil || len(falta.ServiceKinds) != 0 {
		t.Errorf("con base propia no falta ningún servicio: %v (err=%v)", falta.ServiceKinds, err)
	}

	// Y la ficha del servicio dice de qué destino es.
	svcs, err := st.ListServices(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	destinos := map[string]domain.DeployTarget{}
	for _, s := range svcs {
		destinos[s.Name] = s.Target
	}
	if destinos["db"] != domain.TargetProduction || destinos["db-previa"] != domain.TargetPreview {
		t.Errorf("destinos leídos = %v", destinos)
	}
}
