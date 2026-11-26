// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El autoguardado de Docus encadena cada guardado con la respuesta del anterior
// (`ab.base = res`): esa respuesta tiene que ser el registro tal como quedó, o el
// guardado siguiente de la misma ventana se ve como de otro y falla con conflicto. Con
// `autoritativo` la marca `:lww` consume el reloj pedido y el registro queda con uno más.
func TestGuardarEncadenandoLaRespuestaNoEsConflicto(t *testing.T) {
	b := abrir(t, t.TempDir(), "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "x", "")
	base, err := b.DocumentContent(ctx, d.ID, "doc:html")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		for _, autoritativo := range []bool{true, false} {
			valor := fmt.Sprintf("<p>%d %v</p>", i, autoritativo)
			res, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", valor, base, api.DocDot{Actor: "yo", Counter: uint64(i + 1)}, autoritativo)
			if err != nil {
				t.Fatalf("guardado %d (autoritativo %v) sobre la respuesta del anterior: %v", i, autoritativo, err)
			}
			ahora, err := b.DocumentContent(ctx, d.ID, "doc:html")
			if err != nil || ahora.Value != valor || res.Value != valor || res.Dot != ahora.Dot || res.Lamport != ahora.Lamport || res.MaxLamport != ahora.MaxLamport || res.Seq != ahora.Seq {
				t.Fatalf("la respuesta no es el registro guardado: %+v frente a %+v (%v)", res, ahora, err)
			}
			base = res
		}
	}
}
