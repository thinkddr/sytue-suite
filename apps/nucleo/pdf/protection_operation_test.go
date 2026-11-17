// SPDX-License-Identifier: Apache-2.0
package pdf

import "testing"

func TestProtectionOperationPreservesXMPWarningAndRejectsMalformedRequests(t *testing.T) {
	result, err := ApplyProtectionOperation(pdfADePrueba(t, "2", "B", 3, false), map[string]string{"op": "metadatos", "metadatos": `{"titulo":"Private title"}`})
	if err != nil || !result.UnchangedXMP || len(result.Data) == 0 {
		t.Fatal("untouched XMP warning lost", err)
	}
	for _, fields := range []map[string]string{
		{"op": "proteger"},
		{"op": "missing"},
		{"op": "metadatos", "metadatos": `{"unknown":true}`},
		{"op": "metadatos", "metadatos": `{} {}`},
		{"op": "redactar", "areas": `[{"Pagina":0,"unknown":true}]`},
		{"op": "redactar", "areas": `[]`},
	} {
		if _, err := ApplyProtectionOperation(docDePrueba(t, "unchanged"), fields); err == nil {
			t.Fatal("malformed protection operation accepted", fields["op"])
		}
	}
	if _, err := ApplyProtectionOperation(nil, nil); err == nil {
		t.Fatal("nil document accepted")
	}
	permissions := ParseProtectionPermissions(" IMPRIMIR, anotar, unknown ")
	if !permissions.Imprimir || !permissions.Anotar || !permissions.Accesibilidad || permissions.Copiar || permissions.Modificar || permissions.ImprimirEnAlta || permissions.Ensamblar || permissions.RellenarCampos {
		t.Fatal("permission allow-list changed", permissions)
	}
}
