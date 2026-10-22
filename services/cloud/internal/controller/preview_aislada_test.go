// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// specPlatform guarda la AppSpec con la que se aplicó cada app: es donde se ve
// qué variables y qué secretos llegan de verdad al contenedor.
type specPlatform struct {
	StubPlatform
	specs []AppSpec
}

// Devuelve «todavía no está lista»: lo que se mira aquí es con qué se aplicó, y
// así el reconciliador no sigue hacia la promoción, que es otro test.
func (p *specPlatform) ApplyApp(_ context.Context, spec AppSpec) (bool, error) {
	p.specs = append(p.specs, spec)
	return false, nil
}

// Una vista previa NO recibe las credenciales de los servicios de producción del
// proyecto, y producción no recibe las de los de vista previa (CLOUD-01).
//
// El store de este test entrega un DATABASE_URL distinto por destino. Lo que se
// comprueba es cuál acaba en el Secret del contenedor: antes del arreglo el
// controlador pedía las credenciales sin decir para qué despliegue eran, así que
// toda vista previa arrancaba con la base de datos de producción.
func TestLaVistaPreviaNoRecibeLaBaseDeProduccion(t *testing.T) {
	t.Parallel()
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web", ProductionRef: "main"}
	creds := map[domain.DeployTarget]map[string][]byte{}
	sellar := func(c *Controller, dsn string) map[string][]byte {
		enc, err := c.sealer.Seal([]byte(`{"DATABASE_URL":"` + dsn + `"}`))
		if err != nil {
			t.Fatal(err)
		}
		return map[string][]byte{string(domain.ServicePostgres): enc}
	}

	for _, caso := range []struct {
		target domain.DeployTarget
		quiere string
	}{
		{domain.TargetPreview, "postgres://previa"},
		{domain.TargetProduction, "postgres://produccion"},
	} {
		fs := &fakeCtrlStore{creds: creds}
		plat := &specPlatform{}
		c := newTestController(fs, plat, nil)
		creds[domain.TargetProduction] = sellar(c, "postgres://produccion")
		creds[domain.TargetPreview] = sellar(c, "postgres://previa")

		dep := domain.Deployment{
			ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID, Target: caso.target,
			Status: domain.DeployQueued, URL: "https://web.apps.sytue.eu",
			ImageRef: "rg.fr-par.scw.cloud/sytue/web@sha256:beef", PRNumber: intptr(7),
		}
		c.reconcileDeployment(context.Background(), dep, proj)

		if len(plat.specs) != 1 {
			t.Fatalf("%s: se esperaba una app aplicada, hay %d", caso.target, len(plat.specs))
		}
		if got := plat.specs[0].Secrets["DATABASE_URL"]; got != caso.quiere {
			t.Errorf("%s arrancó con DATABASE_URL=%q; se esperaba %q", caso.target, got, caso.quiere)
		}
		for _, pedido := range fs.credsTargets {
			if pedido != caso.target {
				t.Errorf("%s pidió credenciales de %q", caso.target, pedido)
			}
		}
	}
}

// El valor propio del destino gana al compartido, y cada clave vive en un solo
// sitio: `env` va en línea en el contenedor y gana a `envFrom`, así que dejar un
// valor compartido en claro junto al secreto de la vista previa lo taparía.
func TestElValorPropioDeLaVistaPreviaGanaAlCompartido(t *testing.T) {
	t.Parallel()
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web"}
	fs := &fakeCtrlStore{}
	c := newTestController(fs, &specPlatform{}, nil)
	secreto, err := c.sealer.SealString("sk_test_previa")
	if err != nil {
		t.Fatal(err)
	}
	// En el orden en que lo entrega el store: lo compartido primero.
	fs.envVars = []store.ResolvedEnvVar{
		{Key: "API_KEY", Value: "valor-compartido-en-claro", Scope: domain.ScopeAll},
		{Key: "MODO", Value: "compartido", Scope: domain.ScopeAll},
		{Key: "API_KEY", ValueEnc: secreto, IsSecret: true, Scope: domain.ScopePreview},
		{Key: "MODO", Value: "previa", Scope: domain.ScopePreview},
	}
	dep := domain.Deployment{ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID, Target: domain.TargetPreview}

	env, secrets, err := c.resolveInjection(context.Background(), dep, proj)
	if err != nil {
		t.Fatal(err)
	}
	if secrets["API_KEY"] != "sk_test_previa" {
		t.Errorf("API_KEY secreto = %q; se esperaba el de la vista previa", secrets["API_KEY"])
	}
	if _, hay := env["API_KEY"]; hay {
		t.Errorf("API_KEY quedó también en claro (%q) y taparía al secreto", env["API_KEY"])
	}
	if env["MODO"] != "previa" {
		t.Errorf("MODO = %q; el valor propio de la vista previa tiene que ganar", env["MODO"])
	}
}

// Si a la vista previa le falta algo por no heredar lo de producción, se dice
// mientras arranca, con los nombres. En producción no hay aviso.
func TestLaVistaPreviaDiceLoQueLeFalta(t *testing.T) {
	t.Parallel()
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web"}
	falta := store.PreviewWithheld{SecretKeys: []string{"STRIPE_KEY"}, ServiceKinds: []string{"postgres"}}

	for _, target := range []domain.DeployTarget{domain.TargetPreview, domain.TargetProduction} {
		fs := &fakeCtrlStore{withheld: falta}
		c := newTestController(fs, &recPlatform{applyReady: false}, nil)
		dep := domain.Deployment{
			ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID, Target: target,
			Status: domain.DeployQueued, URL: "https://web.apps.sytue.eu",
			ImageRef: "rg.fr-par.scw.cloud/sytue/web@sha256:beef", PRNumber: intptr(7),
		}
		c.reconcileDeployment(context.Background(), dep, proj)

		var aviso string
		for _, s := range fs.statusSet {
			if s.status == domain.DeployDeploying {
				aviso = s.errMsg
			}
		}
		if target == domain.TargetProduction {
			if aviso != "" {
				t.Errorf("producción no lleva aviso de vista previa: %q", aviso)
			}
			continue
		}
		for _, nombre := range []string{"STRIPE_KEY", "postgres"} {
			if !strings.Contains(aviso, nombre) {
				t.Errorf("el aviso de la vista previa no nombra %q: %q", nombre, aviso)
			}
		}
	}
}
