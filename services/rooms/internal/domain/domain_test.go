// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"General", true},
		{"café ☕", true},
		{"con\ttab", false},
		{strings.Repeat("a", 100), true},
		{strings.Repeat("a", 101), false},
	}
	for _, c := range cases {
		if got := ValidName(c.in); got != c.want {
			t.Fatalf("ValidName(%q)=%v, quiero %v", c.in, got, c.want)
		}
	}
}

func TestValidSlug(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"general", true},
		{"team-uno", true},
		{"a1-b2", true},
		{"-lider", false},
		{"cola-", false},
		{"May_Us", false},
		{"con espacio", false},
		{"acentúo", false},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		if got := ValidSlug(c.in); got != c.want {
			t.Fatalf("ValidSlug(%q)=%v, quiero %v", c.in, got, c.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"General", "general"},
		{"Equipo Uno", "equipo-uno"},
		{"  espacios  raros  ", "espacios-raros"},
		{"café_con-leche", "caf-con-leche"},
		{"multiple   guiones", "multiple-guiones"},
		{"---bordes---", "bordes"},
		{"###", ""},
		{"Проект", ""}, // sin caracteres ASCII válidos -> vacío
	}
	for _, c := range cases {
		got := Slugify(c.in)
		if got != c.want {
			t.Fatalf("Slugify(%q)=%q, quiero %q", c.in, got, c.want)
		}
		// El resultado, si no vacío, debe ser un slug válido.
		if got != "" && !ValidSlug(got) {
			t.Fatalf("Slugify(%q)=%q no es un slug válido", c.in, got)
		}
	}
}

func TestValidEmoji(t *testing.T) {
	t.Parallel()
	if !ValidEmoji(":smile:") || !ValidEmoji("👍") {
		t.Fatal("shortcodes y emoji unicode deben ser válidos")
	}
	if ValidEmoji("") || ValidEmoji("  ") {
		t.Fatal("vacío no es válido")
	}
	if ValidEmoji(strings.Repeat("a", 65)) {
		t.Fatal("demasiado largo no es válido")
	}
	if ValidEmoji("con\nsalto") {
		t.Fatal("caracteres de control no válidos")
	}
}

func TestValidEmojiName(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"party_parrot", "shipit", "a", "x-1", "abc_def-2", strings.Repeat("a", 64)} {
		if !ValidEmojiName(ok) {
			t.Errorf("%q debería ser un nombre válido", ok)
		}
	}
	for _, bad := range []string{"", "Mayus", "con espacio", "acentúa", "emoji!", ":smile:", strings.Repeat("a", 65)} {
		if ValidEmojiName(bad) {
			t.Errorf("%q NO debería ser un nombre válido", bad)
		}
	}
}

func TestHasChannelMention(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"@channel a por ello", true},
		{"aviso @all", true},
		{"@here ¿quién está?", true},
		{"nada de esto @bob", false},
		{"escribe a foo@channel.com", false}, // correo: no es mención
		{"revisa @channels nuevos", false},   // sufijo: no es token exacto
		{"habla con @alliance", false},       // @all no debe casar dentro de otra palabra
		{"(@here)", true},                    // rodeado de puntuación
	}
	for _, c := range cases {
		if got := HasChannelMention(c.in); got != c.want {
			t.Fatalf("HasChannelMention(%q)=%v, quiero %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeBody(t *testing.T) {
	t.Parallel()
	const maxLen = 20

	got, err := NormalizeBody("  hola mundo \n\t", maxLen)
	if err != nil {
		t.Fatalf("cuerpo válido: %v", err)
	}
	if got != "  hola mundo" {
		t.Fatalf("trim de cola incorrecto: %q", got)
	}

	if _, err := NormalizeBody("   \n\t ", maxLen); !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("cuerpo en blanco debe ser ErrEmptyMessage, got %v", err)
	}
	if _, err := NormalizeBody("", maxLen); !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("cuerpo vacío debe ser ErrEmptyMessage, got %v", err)
	}
	if _, err := NormalizeBody(strings.Repeat("x", maxLen+1), maxLen); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cuerpo demasiado largo debe ser ErrInvalidInput, got %v", err)
	}
}

func TestMemberRoleCanManage(t *testing.T) {
	t.Parallel()
	if !RoleAdmin.CanManage() {
		t.Fatal("admin gestiona")
	}
	if RoleMember.CanManage() {
		t.Fatal("member no gestiona")
	}
	if !ValidMemberRole(RoleAdmin) || !ValidMemberRole(RoleMember) {
		t.Fatal("roles canónicos válidos")
	}
	if ValidMemberRole(MemberRole("owner")) {
		t.Fatal("owner no es rol de chat")
	}
}

func TestValidChannelKind(t *testing.T) {
	t.Parallel()
	for _, k := range []ChannelKind{ChannelPublic, ChannelPrivate, ChannelDirect} {
		if !ValidChannelKind(k) {
			t.Fatalf("%q debe ser válido", k)
		}
	}
	if ValidChannelKind(ChannelKind("secret")) {
		t.Fatal("secret no es un tipo de canal")
	}
}

func TestValidNotifyLevel(t *testing.T) {
	t.Parallel()
	for _, l := range []string{"", NotifyLevelAll, NotifyLevelMentions, NotifyLevelNone} {
		if !ValidNotifyLevel(l) {
			t.Fatalf("%q debe ser un nivel válido", l)
		}
	}
	if ValidNotifyLevel("loud") {
		t.Fatal("loud no es un nivel de notificación")
	}
}

func TestNormalizeNotifyPrefs(t *testing.T) {
	t.Parallel()
	got := NormalizeNotifyPrefs(NotifyPrefs{
		Keywords: []string{" Deploy ", "deploy", "", "INCIDENCIA"},
		DNDStart: -5,
		DNDEnd:   2000,
	})
	// Recorte + minúsculas + dedupe (Deploy y deploy colapsan; la vacía se descarta).
	if len(got.Keywords) != 2 || got.Keywords[0] != "deploy" || got.Keywords[1] != "incidencia" {
		t.Fatalf("keywords = %#v, quiero [deploy incidencia]", got.Keywords)
	}
	// DND acotado a [0..1439].
	if got.DNDStart != 0 || got.DNDEnd != 1439 {
		t.Fatalf("DND = %d..%d, quiero 0..1439", got.DNDStart, got.DNDEnd)
	}
}
