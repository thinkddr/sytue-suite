// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"strings"
	"testing"
	"time"
)

// escapeLike debe neutralizar los metacaracteres LIKE para que un filtro
// de acción se trate como prefijo literal (sin comodines inyectados por el
// usuario).
func TestEscapeLikePrefix(t *testing.T) {
	cases := map[string]string{
		"session.": "session.",
		"a%b":      `a\%b`,
		"a_b":      `a\_b`,
		`a\b`:      `a\\b`,
		`100%_\ok`: `100\%\_\\ok`,
		"":         "",
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// auditWhere sin filtros ni cursor: solo el anclaje por organización en $1.
func TestAuditWhereBaseline(t *testing.T) {
	where, args := auditWhere("org-1", AuditFilter{}, nil)
	if where != " WHERE l.org_id = $1" {
		t.Fatalf("where = %q", where)
	}
	if len(args) != 1 || args[0] != "org-1" {
		t.Fatalf("args = %v", args)
	}
}

// auditWhere con todos los filtros y un cursor keyset: placeholders correlativos
// ($1..$7), acción escapada+comodín, actor casteado a uuid, rango y keyset con
// las posiciones correctas. Es la garantía de que la consulta queda
// parametrizada (sin SQL dinámico) y de que el keyset compara la tupla correcta.
func TestAuditWhereFullFilterAndCursor(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	until := since.AddDate(0, 0, 7)
	curTS := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	f := AuditFilter{Action: "auth.", ActorID: "00000000-0000-0000-0000-000000000001", Since: &since, Until: &until}
	after := &AuditCursor{OccurredAt: curTS, ID: 42}

	where, args := auditWhere("org-9", f, after)

	for _, want := range []string{
		"l.org_id = $1",
		`l.action LIKE $2 ESCAPE '\'`,
		"l.actor_id = $3::uuid",
		"l.occurred_at >= $4",
		"l.occurred_at < $5",
		"(l.occurred_at, l.id) < ($6, $7)",
	} {
		if !strings.Contains(where, want) {
			t.Errorf("where no contiene %q\nwhere=%q", want, where)
		}
	}

	if len(args) != 7 {
		t.Fatalf("len(args) = %d, quiero 7 (%v)", len(args), args)
	}
	if args[0] != "org-9" || args[1] != "auth.%" || args[2] != f.ActorID {
		t.Errorf("args[0..2] = %v", args[:3])
	}
	if got := args[3].(time.Time); !got.Equal(since) {
		t.Errorf("args[3] since = %v", got)
	}
	if got := args[4].(time.Time); !got.Equal(until) {
		t.Errorf("args[4] until = %v", got)
	}
	if got := args[5].(time.Time); !got.Equal(curTS) {
		t.Errorf("args[5] cursor ts = %v", got)
	}
	if args[6] != int64(42) {
		t.Errorf("args[6] cursor id = %v", args[6])
	}
}
