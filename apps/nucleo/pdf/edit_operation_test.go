// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"errors"
	"os"
	"testing"
)

func TestEditOperationRequiresConsentBeforeChangingSignedContent(t *testing.T) {
	raw, err := os.ReadFile("testdata/pdf/certificacion-p1.pdf")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := LeerPDF(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyEditOperation(doc, EditOperationInput{Fields: map[string]string{"op": "analizar"}}); err != nil {
		t.Fatal("read-only analysis demanded signature invalidation", err)
	}
	if _, err := ApplyEditOperation(doc, EditOperationInput{Fields: map[string]string{"op": "indice"}}); !errors.Is(err, ErrInvalidaFirmas) {
		t.Fatal("signed edit did not require consent", err)
	}
	doc.PermitirInvalidarFirmas()
	if _, err := ApplyEditOperation(doc, EditOperationInput{Fields: map[string]string{"op": "indice"}}); err != nil {
		t.Fatal("explicit signature invalidation rejected", err)
	}
}

func TestEditOperationRejectsUnknownOrMalformedChanges(t *testing.T) {
	doc := docDePrueba(t, "Private original text")
	for _, fields := range []map[string]string{{"op": "unknown"}, {"op": "texto", "cambios": "[] {}"}, {"op": "analizar", "pagina": "0"}, {"op": "capas", "visibles": "-1"}, {"op": "indice", "marcadores": "[{}] []"}, {"op": "imagen"}, {"op": "imagen", "id": "missing"}} {
		if _, err := ApplyEditOperation(doc, EditOperationInput{Fields: fields}); err == nil {
			t.Fatal("invalid content edit accepted", fields["op"])
		}
	}
}
