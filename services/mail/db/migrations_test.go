// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package db

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// sentencias quita los comentarios de línea y cuenta las sentencias SQL no vacías.
func sentencias(sql string) []string {
	var limpio strings.Builder
	for _, l := range strings.Split(sql, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		limpio.WriteString(l)
		limpio.WriteByte('\n')
	}
	var out []string
	for _, s := range strings.Split(limpio.String(), ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

var indiceSobreMessages = regexp.MustCompile(`(?is)\b(CREATE|DROP)\s+(UNIQUE\s+)?INDEX\b.*?(\bON\s+messages\b|messages_\w+_idx)`)

// Desde la 0037 (la de lo pospuesto), ningún índice de `messages` se crea ni se
// retira con bloqueo: es la tabla más grande de Mail y un CREATE/DROP INDEX normal
// para sus escrituras (el correo que entra) mientras dura. Va CONCURRENTLY y solo en
// su fichero, porque golang-migrate manda el fichero entero en una llamada y varias
// sentencias juntas irían en una transacción implícita, que CONCURRENTLY no admite.
func TestLosIndicesDeMessagesNoBloqueanEscrituras(t *testing.T) {
	ficheros, err := fs.Glob(migrationFiles, MigrationsDir+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	revisados := 0
	for _, f := range ficheros {
		nombre := strings.TrimPrefix(f, MigrationsDir+"/")
		if nombre < "0037" {
			continue
		}
		b, err := fs.ReadFile(migrationFiles, f)
		if err != nil {
			t.Fatal(err)
		}
		todas := sentencias(string(b))
		for _, s := range todas {
			if !indiceSobreMessages.MatchString(s) {
				continue
			}
			revisados++
			if !regexp.MustCompile(`(?i)\bINDEX\s+CONCURRENTLY\b`).MatchString(s) {
				t.Errorf("%s: índice de messages sin CONCURRENTLY:\n%s", nombre, s)
			}
			if len(todas) != 1 {
				t.Errorf("%s: el índice comparte fichero con %d sentencias más (iría en transacción)", nombre, len(todas)-1)
			}
		}
	}
	// Las de la 0038 y la 0039, con sus bajadas.
	if revisados < 4 {
		t.Fatalf("solo %d sentencias de índice revisadas: ¿se movieron las migraciones?", revisados)
	}
}

// El listado de carpeta necesita el índice por la fecha de orden: que siga existiendo
// en alguna migración tras partir la 0037.
func TestElListadoTieneSuIndicePorFechaDeOrden(t *testing.T) {
	b, err := fs.ReadFile(migrationFiles, MigrationsDir+"/0038_listado_por_fecha_de_orden.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "messages_mailbox_orden_idx") || !strings.Contains(string(b), "COALESCE(unsnoozed_at, received_at)") {
		t.Fatalf("la 0038 no crea messages_mailbox_orden_idx por la fecha de orden:\n%s", b)
	}
}
