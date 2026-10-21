// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/config"
)

// chartsDe son los charts que despliegan el binario de un servicio: Cloud son tres
// binarios con una configuración, y Docs lee sus variables con el prefijo SYTUE_DOCS_.
var chartsDe = map[string][]string{
	"cloud": {"cloud-api", "cloud-controller", "cloud-builder"},
}

// sinChart son los servicios que leen el secreto pero aún NO se despliegan: sin
// chart ni entrada en build-images.sh/deploy.sh, no hay pod que pueda arrancar sin
// él. En cuanto un servicio de aquí tenga chart, el test falla hasta que se le
// quite de la lista. Sytue Web entra con su esqueleto (2026-09-28) y sale cuando
// llegue deploy/helm/charts/pages (la PR1 de despliegue, PAGES-ENCARGO §16.1).
var sinChart = map[string]bool{}

// Todo servicio que lee el secreto de la puerta interna del Hub lo recibe en su chart
// del Secret `sytue-hub-internal`. Fuera de dev, sin él, no arranca: un chart que se
// olvide sale aquí en rojo y no como un CrashLoopBackOff en producción.
func TestCadaServicioQueAvisaRecibeElSecretoDelHub(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")
	configs, _ := filepath.Glob(filepath.Join(raiz, "services", "*", "internal", "config", "config.go"))
	usa := regexp.MustCompile(`\.HubInternalSecret\(`)
	var vistos int
	sinChartVistos := map[string]bool{}
	for _, c := range configs {
		b, err := os.ReadFile(c)
		if err != nil || !usa.Match(b) {
			continue
		}
		svc := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(c))))
		charts, ok := chartsDe[svc]
		if !ok {
			charts = []string{svc}
		}
		variable := "HUB_INTERNAL_SECRET"
		if strings.Contains(string(b), `LoadBase("SYTUE_DOCS"`) {
			variable = "SYTUE_DOCS_HUB_INTERNAL_SECRET"
		}
		patron := regexp.MustCompile(`- name: ` + variable + `\s+valueFrom:\s+secretKeyRef:\s+name: sytue-hub-internal\s+key: secret`)
		for _, ch := range charts {
			d, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "charts", ch, "templates", "deployment.yaml"))
			if sinChart[svc] {
				sinChartVistos[svc] = true
				if err == nil {
					t.Errorf("%s ya tiene chart: quítalo de sinChart para que se compruebe", svc)
				}
				continue
			}
			vistos++
			if err != nil {
				t.Errorf("%s: no hay chart %q: %v", svc, ch, err)
				continue
			}
			if !patron.Match(d) {
				t.Errorf("el chart %s no le da %s del Secret sytue-hub-internal a %s: fuera de dev no arrancará", ch, variable, svc)
			}
		}
	}
	for svc := range sinChart {
		if !sinChartVistos[svc] {
			t.Errorf("%s está en sinChart pero ya no lee el secreto del Hub: quítalo de la lista", svc)
		}
	}
	if vistos < 8 {
		t.Fatalf("solo %d charts comprobados: ¿ha cambiado dónde se lee el secreto? el guardián estaría apagado", vistos)
	}
	// Desde #1114 la plantilla genera todos los secretos internos que lista
	// `secretosInternos` en values.yaml; el del Hub tiene que ser uno de ellos.
	plantilla, err := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "sytue-one", "templates", "secretos-internos.yaml"))
	valores, err2 := os.ReadFile(filepath.Join(raiz, "deploy", "helm", "sytue-one", "values.yaml"))
	if err != nil || err2 != nil || !strings.Contains(string(plantilla), "name: {{ $nombre }}") ||
		!regexp.MustCompile(`(?m)^secretosInternos:\n(?:[ \t]+\S.*\n)*?[ \t]+sytue-hub-internal: secret\b`).Match(valores) {
		t.Fatal("el chart paraguas no genera el Secret sytue-hub-internal")
	}
}

// Fuera de dev, ni falta ni el de desarrollo: el servicio no arranca.
func TestElSecretoDelHubFueraDeDev(t *testing.T) {
	l := config.NewLoader("")
	if v := l.HubInternalSecret(config.EnvDev); v != config.DevHubInternalSecret || l.Err() != nil {
		t.Fatalf("en dev: %q %v", v, l.Err())
	}
	l = config.NewLoader("")
	l.HubInternalSecret(config.EnvProd)
	if l.Err() == nil {
		t.Fatal("en prod, sin HUB_INTERNAL_SECRET arranca")
	}
	t.Setenv("HUB_INTERNAL_SECRET", config.DevHubInternalSecret)
	l = config.NewLoader("")
	l.HubInternalSecret(config.EnvStaging)
	if l.Err() == nil {
		t.Fatal("fuera de dev acepta el secreto de desarrollo")
	}
	t.Setenv("HUB_INTERNAL_SECRET", "de-verdad")
	l = config.NewLoader("")
	if v := l.HubInternalSecret(config.EnvProd); v != "de-verdad" || l.Err() != nil {
		t.Fatalf("en prod: %q %v", v, l.Err())
	}
}
