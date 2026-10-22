// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// baseManifest es un manifiesto válido mínimo, en JSON, sobre el que los tests
// de rechazo introducen UN defecto cada vez.
const baseManifest = `{
  "id": "demo", "name": "Demo", "description": "Una demo.",
  "version": "1.0.0", "license": "Apache-2.0", "maintainer": "Sytue",
  "framework": "go", "build": {}
}`

// conCampo devuelve baseManifest con campos extra inyectados, para no repetir el
// JSON entero en cada caso.
func conCampo(extra string) []byte {
	return []byte(strings.TrimSuffix(baseManifest, "}") + ", " + extra + "}")
}

// TestLoadTemplateManifests_catalogoEmbebido carga TODOS los manifiestos del
// embed y comprueba sus invariantes. Es la red que impide que un fichero de
// catálogo mal escrito llegue al arranque: si esto pasa, el proceso arranca.
func TestLoadTemplateManifests_catalogoEmbebido(t *testing.T) {
	list, err := LoadTemplateManifests()
	if err != nil {
		t.Fatalf("LoadTemplateManifests: %v", err)
	}
	if len(list) < 7 {
		t.Fatalf("el catálogo tiene %d plantillas; se esperaban al menos 7", len(list))
	}
	// StarterTemplates se carga en el arranque del proceso: si el embed fuera
	// inválido, el paquete habría entrado en pánico antes de llegar aquí.
	if len(StarterTemplates) != len(list) {
		t.Errorf("StarterTemplates tiene %d plantillas y el embed %d", len(StarterTemplates), len(list))
	}
	for _, m := range list {
		// La licencia está en la allowlist: es el gate mecánico contra AGPL /
		// source-available. validate() ya lo comprueba; esto lo deja explícito.
		if m.License == "" {
			t.Errorf("%s: licencia vacía", m.ID)
		}
		got, ok := StarterTemplateByID(m.ID)
		if !ok || got.ID != m.ID {
			t.Errorf("StarterTemplateByID(%q) no devolvió la plantilla", m.ID)
		}
		for _, e := range m.Env {
			if e.Origin == EnvOriginGenerate && !e.Secret {
				t.Errorf("%s: %s es origin=generate y debería estar marcada como secreta", m.ID, e.Key)
			}
		}
	}
	if _, ok := StarterTemplateByID("no-existe"); ok {
		t.Error("StarterTemplateByID debe devolver false para un id desconocido")
	}
}

// TestParseTemplateManifest_rechazaConfiguracionDePod es EL test de la ola: el
// manifiesto no puede expresar configuración de pod. No hay lista negra; la
// decodificación estricta rechaza cualquier clave que no esté en el esquema, así
// que estos seis campos —y los que Kubernetes invente mañana— mueren al cargar.
func TestParseTemplateManifest_rechazaConfiguracionDePod(t *testing.T) {
	prohibidos := []string{
		`"securityContext": {"runAsUser": 0}`,
		`"hostNetwork": true`,
		`"capabilities": ["NET_ADMIN"]`,
		`"volumes": [{"hostPath": "/"}]`,
		`"nodeSelector": {"node": "big"}`,
		`"command": ["/bin/sh", "-c", "curl evil"]`,
		`"privileged": true`,
	}
	for _, campo := range prohibidos {
		if _, err := ParseTemplateManifest(conCampo(campo)); err == nil {
			t.Errorf("se aceptó un manifiesto con %s; debe rechazarse por clave desconocida", campo)
		}
	}
}

// TestParseTemplateManifest_licencia comprueba que la licencia es obligatoria y
// que la allowlist es lo que decide (no una revisión humana).
func TestParseTemplateManifest_licencia(t *testing.T) {
	for _, lic := range []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MPL-2.0"} {
		raw := strings.Replace(baseManifest, `"license": "Apache-2.0"`, `"license": "`+lic+`"`, 1)
		if _, err := ParseTemplateManifest([]byte(raw)); err != nil {
			t.Errorf("licencia permisiva %q rechazada: %v", lic, err)
		}
	}
	for _, lic := range []string{"", "AGPL-3.0", "BUSL-1.1", "SSPL-1.0", "Elastic-2.0", "mit"} {
		raw := strings.Replace(baseManifest, `"license": "Apache-2.0"`, `"license": "`+lic+`"`, 1)
		if _, err := ParseTemplateManifest([]byte(raw)); err == nil {
			t.Errorf("licencia %q aceptada; debe estar fuera de la allowlist", lic)
		}
	}
}

// TestParseTemplateManifest_env cubre las reglas de origen de cada variable: las
// dos que importan son que un literal del manifiesto nunca es secreto y que
// from_service tiene que apuntar a un servicio que la plantilla aprovisione.
func TestParseTemplateManifest_env(t *testing.T) {
	env := func(items string) []byte {
		return []byte(strings.TrimSuffix(baseManifest, "}") + `, "services": ["postgres"], "env": [` + items + `]}`)
	}
	casos := []struct {
		nombre string
		items  string
		quiero bool // true = debe aceptarse
		porQue string
	}{
		{"value literal", `{"key":"LOG_LEVEL","origin":"value","value":"info"}`, true, ""},
		{"ask con prompt", `{"key":"SITE_URL","origin":"ask","prompt":"URL pública"}`, true, ""},
		{"generate", `{"key":"SESSION_SECRET","origin":"generate"}`, true, ""},
		{"from_service declarado", `{"key":"DATABASE_URL","origin":"from_service","service":"postgres"}`, true, ""},

		{
			"literal secreto", `{"key":"API_KEY","origin":"value","value":"s3cr3t","secret":true}`, false,
			"un literal del manifiesto está en el repositorio en claro",
		},
		{"value sin valor", `{"key":"API_KEY","origin":"value"}`, false, "falta value"},
		{"ask sin prompt", `{"key":"API_KEY","origin":"ask"}`, false, "el usuario no sabría qué escribir"},
		{"generate con valor", `{"key":"API_KEY","origin":"generate","value":"x"}`, false, "lo genera la plataforma"},
		{
			"from_service no aprovisionado", `{"key":"S3_BUCKET","origin":"from_service","service":"storage"}`, false,
			"la plantilla no aprovisiona storage",
		},
		{"origen desconocido", `{"key":"API_KEY","origin":"env"}`, false, "origen fuera del enum"},
		{"clave inválida", `{"key":"api-key","origin":"value","value":"x"}`, false, "no es MAYUS_CON_GUION_BAJO"},
		{
			"clave duplicada",
			`{"key":"A","origin":"value","value":"1"},{"key":"A","origin":"value","value":"2"}`, false,
			"dos veces la misma variable",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := ParseTemplateManifest(env(c.items))
			if c.quiero && err != nil {
				t.Fatalf("rechazado y debía aceptarse: %v", err)
			}
			if !c.quiero && err == nil {
				t.Fatalf("aceptado y debía rechazarse (%s)", c.porQue)
			}
		})
	}
}

// TestParseTemplateManifest_generateEsSiempreSecreto: el manifiesto no decide si
// un secreto generado es secreto. Aunque el fichero no lo diga (o diga que no),
// se persiste cifrado.
func TestParseTemplateManifest_generateEsSiempreSecreto(t *testing.T) {
	raw := strings.TrimSuffix(baseManifest, "}") +
		`, "env": [{"key":"SESSION_SECRET","origin":"generate","secret":false}]}`
	m, err := ParseTemplateManifest([]byte(raw))
	if err != nil {
		t.Fatalf("ParseTemplateManifest: %v", err)
	}
	if !m.Env[0].Secret {
		t.Error("una variable origin=generate debe quedar marcada como secreta")
	}
}

// TestParseTemplateManifest_servicios: una plantilla solo puede aprovisionar los
// servicios que no exigen una imagen del propio proyecto (cron/job/functions la
// exigen, y esa imagen no existe hasta que termina la primera build).
func TestParseTemplateManifest_servicios(t *testing.T) {
	for _, k := range []ServiceKind{ServicePostgres, ServiceStorage, ServiceAuth} {
		raw := strings.TrimSuffix(baseManifest, "}") + `, "services": ["` + string(k) + `"]}`
		if _, err := ParseTemplateManifest([]byte(raw)); err != nil {
			t.Errorf("servicio %q rechazado: %v", k, err)
		}
	}
	for _, k := range []ServiceKind{ServiceCron, ServiceJob, ServiceFunctions, "inventado"} {
		raw := strings.TrimSuffix(baseManifest, "}") + `, "services": ["` + string(k) + `"]}`
		if _, err := ParseTemplateManifest([]byte(raw)); err == nil {
			t.Errorf("servicio %q aceptado; debe rechazarse", k)
		}
	}
	dup := strings.TrimSuffix(baseManifest, "}") + `, "services": ["postgres", "postgres"]}`
	if _, err := ParseTemplateManifest([]byte(dup)); err == nil {
		t.Error("se aceptó el mismo servicio dos veces")
	}
}

// TestParseTemplateManifest_camposObligatorios comprueba el resto del esquema.
func TestParseTemplateManifest_camposObligatorios(t *testing.T) {
	malos := []string{
		strings.Replace(baseManifest, `"id": "demo"`, `"id": "Demo Mayúsculas"`, 1),
		strings.Replace(baseManifest, `"name": "Demo"`, `"name": ""`, 1),
		strings.Replace(baseManifest, `"description": "Una demo."`, `"description": ""`, 1),
		strings.Replace(baseManifest, `"version": "1.0.0"`, `"version": ""`, 1),
		strings.Replace(baseManifest, `"maintainer": "Sytue"`, `"maintainer": ""`, 1),
		strings.Replace(baseManifest, `"framework": "go"`, `"framework": "cobol"`, 1),
		strings.Replace(baseManifest, `"build": {}`, `"build": {"strategy": "make"}`, 1),
		string(conCampo(`"docs_url": "http://inseguro.example"`)),
		string(conCampo(`"app_port": 70000`)),
		string(conCampo(`"health_path": "relativa"`)),
		string(conCampo(`"instance_size": "gigante"`)),
	}
	for i, raw := range malos {
		if _, err := ParseTemplateManifest([]byte(raw)); err == nil {
			t.Errorf("caso %d: manifiesto inválido aceptado:\n%s", i, raw)
		}
	}
}
