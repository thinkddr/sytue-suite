// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package merge

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// El DSN de cada servicio se deriva del de Sytue ID cambiando SOLO la base. Un
// error aquí no falla: escribe en la base equivocada, que es peor.
func TestDSNForDatabaseSoloCambiaLaBase(t *testing.T) {
	const origen = "postgres://sytue_admin:p%40ss@pgbouncer:6432/id?sslmode=disable"
	got, err := DSNForDatabase(origen, "mail")
	if err != nil {
		t.Fatalf("derivar: %v", err)
	}
	const want = "postgres://sytue_admin:p%40ss@pgbouncer:6432/mail?sslmode=disable"
	if got != want {
		t.Fatalf("DSN derivado\n  got:  %s\n  want: %s", got, want)
	}
}

// Las bases de un despliegue se llaman todas igual: el piloto les antepone
// `sytue_`, y derivar `/mail` desde `/sytue_id` conectaba a una base inexistente.
func TestDSNForDatabaseSigueElNombreDeLaBaseDeID(t *testing.T) {
	casos := map[string]string{
		"postgres://u:p@postgres:5432/sytue_id?sslmode=disable": "postgres://u:p@postgres:5432/sytue_mail?sslmode=disable",
		"postgres://u:p@pgbouncer:6432/id":                      "postgres://u:p@pgbouncer:6432/mail",
	}
	for origen, want := range casos {
		got, err := DSNForDatabase(origen, "mail")
		if err != nil || got != want {
			t.Errorf("DSNForDatabase(%q)\n  got:  %s (%v)\n  want: %s", origen, got, err, want)
		}
	}
}

func TestDSNForDatabaseRechazaEntradasPeligrosas(t *testing.T) {
	const valido = "postgres://u:p@host:5432/id"
	casos := []struct {
		nombre string
		dsn    string
		base   string
	}{
		// Un nombre con comilla o barra reescribiría la ruta del DSN.
		{"base con barra", valido, "mail/otra"},
		{"base con espacio", valido, "mail otra"},
		{"base vacía", valido, ""},
		{"base que empieza por dígito", valido, "1mail"},
		// key=value (libpq) no es una URL: cambiar la base a ciegas apuntaría mal.
		{"DSN no URL", "host=localhost dbname=id", "mail"},
		{"esquema ajeno", "mysql://u:p@host/id", "mail"},
		{"sin base", "postgres://u:p@host", "mail"},
		// Sin el sufijo `id` no hay convención que seguir: adivinar es conectar mal.
		{"base de ID con otro nombre", "postgres://u:p@host:5432/identity", "mail"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got, err := DSNForDatabase(c.dsn, c.base); err == nil {
				t.Fatalf("se aceptó una entrada peligrosa: %q", got)
			}
		})
	}
}

// columnaTenant reconoce la DECLARACIÓN de una columna `tenant` en una
// migración, no cualquier mención: los ficheros están llenos de comentarios que
// hablan de tenants y de políticas que la nombran.
var columnaTenant = regexp.MustCompile(`(?m)^\s*tenant\s+text`)

// serviciosConDatosPorTenant recorre services/*/db/migrations y devuelve las
// bases lógicas que declaran una columna `tenant` en alguna tabla.
func serviciosConDatosPorTenant(t *testing.T) []string {
	t.Helper()
	// El paquete vive en services/id/internal/merge.
	raiz := filepath.Join("..", "..", "..")
	dirs, err := os.ReadDir(raiz)
	if err != nil {
		t.Fatalf("listar services/: %v", err)
	}
	var out []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		migraciones := filepath.Join(raiz, d.Name(), "db", "migrations")
		ficheros, err := filepath.Glob(filepath.Join(migraciones, "*.up.sql"))
		if err != nil {
			t.Fatalf("glob %s: %v", migraciones, err)
		}
		for _, f := range ficheros {
			sql, err := os.ReadFile(f) //nolint:gosec // ruta derivada del propio árbol del repo.
			if err != nil {
				t.Fatalf("leer %s: %v", f, err)
			}
			if columnaTenant.Match(sql) {
				out = append(out, d.Name())
				break
			}
		}
	}
	return out
}

// TestServicesCubreTodaLaSuite comprueba la lista contra EL DISCO, no contra
// otra lista escrita a mano.
//
// Es la diferencia entre un test que acompaña al bug y uno que lo caza. Sytue
// Analytics estuvo fuera de `Services` desde que nació: al fusionar, sus sitios y
// sus históricos se quedaban con el tenant de la organización absorbida y
// desaparecían de la vista del cliente. No falló nada, no hubo error, y el test
// que había —una segunda lista con los mismos nueve nombres— pasaba en verde,
// porque quien olvida añadir el servicio a una lista olvida añadirlo a las dos.
//
// Recorrer las migraciones sí lo detecta: en cuanto una base declara una columna
// `tenant`, este test exige que alguien decida qué hace la fusión con ella.
func TestServicesCubreTodaLaSuite(t *testing.T) {
	enLista := make(map[string]bool, len(Services))
	for _, s := range Services {
		if enLista[s] {
			t.Errorf("servicio duplicado en la fusión: %s", s)
		}
		enLista[s] = true
	}

	conTenant := serviciosConDatosPorTenant(t)
	if len(conTenant) == 0 {
		t.Fatal("no se encontró ninguna migración con columna tenant: " +
			"el test no está mirando donde cree")
	}
	for _, s := range conTenant {
		// `id` NO va en la lista: su parte la mueve MergeIdentity en una transacción
		// propia, con reglas de rol que un UPDATE de tenant no puede expresar.
		if s == "id" {
			continue
		}
		if !enLista[s] {
			t.Errorf("la base %s tiene datos por tenant y NO está en Services: "+
				"tras una fusión se quedarían con el tenant de la organización absorbida", s)
		}
		delete(enLista, s)
	}
	for s := range enLista {
		t.Errorf("Services incluye %s, que no declara ninguna columna tenant", s)
	}

	if enLista["id"] {
		t.Error("la base id no se mueve por tenant: la fusiona MergeIdentity")
	}
}

// El fallo por restos NOMBRA qué quedó y dónde.
//
// Este texto acaba en el campo de fallo de `org_merges`, que es lo que ve quien
// abre la consola tras una fusión parada. «La verificación encontró restos» no
// sirve de nada: hay que poder ir a la tabla. Y el total va aparte porque es lo
// que decide si esto se reintenta o lo mira una persona.
func TestElFalloPorRestosNombraTablaYRecuento(t *testing.T) {
	err := LeftoversError([]Leftover{
		{Service: "mail", Table: "messages", Rows: 3},
		{Service: "drive", Table: "files", Rows: 1},
	})
	got := err.Error()
	for _, quiero := range []string{"4", "mail.messages (3)", "drive.files (1)"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("el mensaje no dice %q: %s", quiero, got)
		}
	}
}
