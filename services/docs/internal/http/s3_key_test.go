// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"strings"
	"testing"
)

// TestExportKeyEsUnicaPorExport fija por qué la clave lleva un identificador propio.
//
// La terna (documento, secuencia, formato) NO determina el contenido: `RenameDocument`
// cambia el título sin tocar `seq`, y el título viaja dentro del fichero exportado. Con
// una clave derivada solo de esa terna, exportar → renombrar → exportar dejaba dos filas
// en `listExports` apuntando al MISMO objeto, así que descargar la primera entregaba el
// contenido de la segunda: un export histórico que cambia retroactivamente, y con el
// `size_bytes` de su fila ya sin corresponder.
func TestExportKeyEsUnicaPorExport(t *testing.T) {
	const doc, seq, ext = "d1", int64(5), "pdf"

	a := exportKey(doc, seq, "id-primero", ext)
	b := exportKey(doc, seq, "id-segundo", ext)
	if a == b {
		t.Fatalf("dos exports del mismo (doc, seq, formato) comparten clave: %q", a)
	}

	// La clave sigue siendo legible y ordenable: prefijo del documento, la secuencia
	// delante y la extensión al final (importa para el Content-Type del presignado).
	for _, k := range []string{a, b} {
		if !strings.HasPrefix(k, "docs/d1/exports/5-") {
			t.Fatalf("clave con forma inesperada: %q", k)
		}
		if !strings.HasSuffix(k, ".pdf") {
			t.Fatalf("la clave debe conservar la extensión: %q", k)
		}
	}
}

// TestSnapshotKeyEsDeterminista: el snapshot SÍ es determinista por (documento,
// secuencia) —es el estado en ese punto, y regenerarlo da lo mismo—, así que aquí la
// clave estable es correcta y reutilizarla es lo que se busca. Se fija para que nadie
// «arregle» esta por simetría con la de export, que tiene el motivo contrario.
func TestSnapshotKeyEsDeterminista(t *testing.T) {
	// Las dos llamadas van a variables en vez de compararse en la misma expresión:
	// escrito `snapshotKey(...) != snapshotKey(...)` el análisis estático lo ve como
	// comparar algo consigo mismo (SA4000) y, sobre todo, se lee como si no probara
	// nada. Con dos valores nombrados queda claro que son dos invocaciones distintas.
	primera := snapshotKey("d1", 5)
	segunda := snapshotKey("d1", 5)
	if primera != segunda {
		t.Fatalf("el snapshot de una misma secuencia debe caer siempre en la misma clave: %q vs %q", primera, segunda)
	}
	if otra := snapshotKey("d1", 6); primera == otra {
		t.Fatalf("secuencias distintas no pueden compartir clave: %q", otra)
	}
	if otroDoc := snapshotKey("d2", 5); primera == otroDoc {
		t.Fatalf("documentos distintos no pueden compartir clave: %q", otroDoc)
	}
}
