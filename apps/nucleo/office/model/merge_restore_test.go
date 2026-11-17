// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

func TestDiffRestoresTombstonesAndAnchoredProperties(t *testing.T) {
	target := buildText("author:session", "abc")
	b := target.Elements[1].Dot
	target.Registers[FormatMarkKey(b, "bold")] = Register{Value: json.RawMessage("true"), Dot: Dot{Actor: "a", Counter: 30}, Lamport: 30}
	target.Registers[SlidePropKey(b, "title")] = Register{Value: json.RawMessage(`"slide title"`), Dot: Dot{Actor: "a", Counter: 31}, Lamport: 31}
	const marked = `"<p><mark data-comment-id=\"comment-1\">abc</mark></p>"`
	target.Registers[ContentKey(KindDocument)] = Register{Value: json.RawMessage(marked), Dot: Dot{Actor: "a", Counter: 32}, Lamport: 32}
	raw, err := target.JSON()
	if err != nil {
		t.Fatal(err)
	}
	current, err := StateFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = current.Apply(Op{Kind: OpDelete, Target: &b, Lamport: 40}); err != nil {
		t.Fatal(err)
	}
	before := append([]Element(nil), current.Elements...)
	ops := current.Diff(target, "restore:unique", 0, current.Lamport)
	applyAll(t, current, ops)
	if got := current.PlainText(); got != "abc" {
		t.Fatalf("restored text=%q, want abc", got)
	}
	live := current.RGAOrder()
	if live[0].Dot != before[0].Dot || live[2].Dot != before[2].Dot {
		t.Fatal("unchanged prefix/suffix identities were replaced")
	}
	if live[1].Dot == b {
		t.Fatal("a permanent tombstone was reused")
	}
	for _, key := range []string{FormatMarkKey(live[1].Dot, "bold"), SlidePropKey(live[1].Dot, "title")} {
		if IsNullJSON(current.Registers[key].Value) {
			t.Fatalf("lost anchored property %s", key)
		}
	}
	if got := string(current.Registers[ContentKey(KindDocument)].Value); got != marked {
		t.Fatalf("comment HTML anchor changed: %s", got)
	}
	if more := current.Diff(target, "restore:retry", 0, current.Lamport); len(more) != 0 {
		t.Fatalf("materially identical restore created %d more ops", len(more))
	}
	for _, e := range current.Elements {
		if e.Dot == b && !e.Deleted {
			t.Fatal("old tombstone was resurrected")
		}
	}
}

func TestDiffRestoreMatchesNativeReplicaAndReverseDelivery(t *testing.T) {
	cases := []struct{ current, target string }{{"abcXYZ", "a😀bc"}, {"XYZ", "abc"}, {"abc", ""}, {"", "abc"}, {"abc", "abc"}, {"abccde", "abcde"}, {"abcde", "abccde"}}
	for _, tc := range cases {
		t.Run(tc.current+"_to_"+tc.target, func(t *testing.T) {
			current := buildText("old", tc.current)
			target := buildText("target", tc.target)
			if len(current.Elements) > 1 {
				d := current.Elements[1].Dot
				current.Apply(Op{Kind: OpDelete, Target: &d, Lamport: 100})
			}
			encoded, _ := current.JSON()
			reverse, _ := StateFromJSON(encoded)
			elements := make([]coedicion.Elemento, 0, len(current.Elements))
			for _, e := range current.Elements {
				var value string
				json.Unmarshal(e.Value, &value)
				elements = append(elements, coedicion.Elemento{Dot: coedicion.Punto{A: e.Dot.Actor, C: e.Dot.Counter}, After: coedicion.Punto{A: e.After.Actor, C: e.After.Counter}, Value: value, Deleted: e.Deleted})
			}
			peer := coedicion.DesdeElementos("peer", elements)
			ops := current.Diff(target, "restore", 0, current.Lamport)
			applyAll(t, current, ops)
			for i := len(ops) - 1; i >= 0; i-- {
				op := ops[i]
				applyAll(t, reverse, []Op{op})
				if op.Kind == OpInsert || op.Kind == OpDelete {
					wire, _ := json.Marshal(op)
					var native coedicion.Op
					if err := json.Unmarshal(wire, &native); err != nil {
						t.Fatal(err)
					}
					peer.AplicarRemota(native)
				}
			}
			for label, got := range map[string]string{"core": current.PlainText(), "reverse": reverse.PlainText(), "native": peer.Texto()} {
				if got != tc.target {
					t.Fatalf("%s=%q want %q", label, got, tc.target)
				}
			}
			if more := current.Diff(target, "retry", 0, current.Lamport); len(more) > 0 {
				t.Fatalf("repeat restore created %d ops", len(more))
			}
		})
	}
}
