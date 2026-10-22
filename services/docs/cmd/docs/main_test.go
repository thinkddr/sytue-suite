// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

func TestDecodeDocUpdated(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		docID   string
		since   int64
		limit   int
		wantErr bool
	}{
		{"lote con base_seq", `{"document_id":"d1","seq":12,"base_seq":9,"author_sub":"u"}`, "d1", 9, 3, false},
		{"primer lote del documento", `{"document_id":"d1","seq":3,"base_seq":0}`, "d1", 0, 3, false},
		{"legado sin base_seq (despliegue rodante)", `{"document_id":"d1","seq":12}`, "d1", 11, 1, false},
		{"base_seq corrupta (>= seq) degrada a última op", `{"document_id":"d1","seq":5,"base_seq":7}`, "d1", 4, 1, false},
		{"sin document_id", `{"seq":3,"base_seq":0}`, "", 0, 0, true},
		{"seq cero", `{"document_id":"d1","seq":0}`, "", 0, 0, true},
		{"JSON inválido", `{`, "", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docID, since, limit, err := decodeDocUpdated([]byte(tc.payload))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("esperaba error, obtuve docID=%q since=%d limit=%d", docID, since, limit)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if docID != tc.docID || since != tc.since || limit != tc.limit {
				t.Fatalf("got (%q, %d, %d), want (%q, %d, %d)", docID, since, limit, tc.docID, tc.since, tc.limit)
			}
		})
	}
}
