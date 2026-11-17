// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAnnotationOperationUsesStrictExistingJSONAndIndexRules(t *testing.T) {
	doc := docDePrueba(t, "annotation original")
	for _, fields := range []map[string]string{
		{"op": "poner", "anotaciones": "[] {}"},
		{"op": "poner", "anotaciones": `[{"pagina":0,"tipo":"nota","unknown":true}]`},
		{"op": "poner"},
		{"op": "borrar", "indices": "1-3"},
		{"op": "borrar", "indices": "-1"},
		{"op": "borrar", "indices": ""},
		{"op": "responder", "indice": "100001"},
		{"op": "marcar", "indice": "0"},
		{"op": "missing"},
	} {
		if _, err := ApplyAnnotationOperation(doc, AnnotationOperationInput{Fields: fields, Author: "owner"}); err == nil {
			t.Fatal("malformed annotation operation accepted", fields["op"])
		}
	}
	if _, err := ParseNonNegativeIndices(strings.Repeat("1,", MaxSelectedPages+1)); err == nil {
		t.Fatal("unbounded index list accepted")
	}
	indices, err := ParseNonNegativeIndices("0, 2,, 5,2")
	if err != nil || !reflect.DeepEqual(indices, []int{0, 2, 5, 2}) {
		t.Fatal("legacy selection changed", indices, err)
	}
	if _, err := ApplyAnnotationOperation(nil, AnnotationOperationInput{}); err == nil {
		t.Fatal("nil document accepted")
	}
	_, err = ParseNonNegativeIndices("invalid")
	var input *InputError
	if !errors.As(err, &input) {
		t.Fatal("invalid index lost client failure", err)
	}
}
