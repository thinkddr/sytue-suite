// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"testing"
)

func TestFutureFieldsSurviveClientEditAndSnapshot(t *testing.T) {
	raw := []byte(`{"seq":9007199254740993,"lamport":9,"future":{"counter":18446744073709551615},"elements":[{"dot":{"a":"a","c":1},"after":{"a":"","c":0},"value":"a","deleted":false,"lamport":1,"future_element":[9007199254740993]}],"registers":{"doc:html":{"value":"<p>a</p>","dot":{"a":"a","c":2},"lamport":2,"future_register":{"mode":"new"}}}}`)
	state, err := StateFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	var op Op
	if err := json.Unmarshal([]byte(`{"kind":"set","dot":{"a":"b","c":3},"key":"doc:html","value":"<p>edited</p>","lamport":10,"future_operation":{"counter":18446744073709551615}}`), &op); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(op); err != nil {
		t.Fatal(err)
	}
	encoded, err := state.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["future"]) != `{"counter":18446744073709551615}` || state.Seq != 9007199254740993 {
		t.Fatal("future state or sequence changed")
	}
	reopened, err := StateFromJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(reopened.Elements[0].Extensions["future_element"]) != `[9007199254740993]` || string(reopened.Registers["doc:html"].Extensions["future_register"]) != `{"mode":"new"}` {
		t.Fatal("nested future content lost")
	}
	operation, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	var round Op
	if err := json.Unmarshal(operation, &round); err != nil {
		t.Fatal(err)
	}
	if string(round.Extensions["future_operation"]) != `{"counter":18446744073709551615}` {
		t.Fatal("operation extension lost")
	}
	op.Extensions = map[string]json.RawMessage{"kind": json.RawMessage(`"delete"`), "target": json.RawMessage(`{"a":"x","c":1}`)}
	operation, err = json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(operation, &round); err != nil {
		t.Fatal(err)
	}
	if round.Kind != OpSet || round.Target != nil {
		t.Fatal("extension overrode operation authority")
	}
}
