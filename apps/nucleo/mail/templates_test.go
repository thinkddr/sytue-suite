// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"strings"
	"testing"
)

// Las plantillas de respuesta viven cifradas en el repositorio privado, con los
// mismos topes y el mismo nombre único que el servicio.
func TestPrivateReplyTemplatesKeepServiceRules(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	ctx := t.Context()
	if values, err := f.repo.Templates(ctx); err != nil || len(values) != 0 {
		t.Fatal("empty account has templates", values, err)
	}
	created, err := f.repo.SaveTemplate(ctx, "", "  Horario  ", "De 9 a 14.\n")
	if err != nil || created.ID == "" || created.Name != "Horario" || created.Body != "De 9 a 14.\n" {
		t.Fatal(created, err)
	}
	if _, err := f.repo.SaveTemplate(ctx, "", "HORARIO", "otro"); !errors.Is(err, ErrTemplateExists) {
		t.Fatal("duplicate name accepted", err)
	}
	for _, bad := range [][2]string{{"", "x"}, {strings.Repeat("n", TemplateNameMax+1), "x"}, {"a\tb", "x"}, {"ok", strings.Repeat("b", TemplateBodyMaxBytes+1)}} {
		if _, err := f.repo.SaveTemplate(ctx, "", bad[0], bad[1]); !errors.Is(err, ErrInvalidTemplate) {
			t.Fatal("invalid template accepted", err)
		}
	}
	edited, err := f.repo.SaveTemplate(ctx, created.ID, "horario", "De 8 a 15.")
	if err != nil || edited.ID != created.ID || edited.Name != "horario" {
		t.Fatal("renaming a template to its own name in another case failed", edited, err)
	}
	if _, err := f.repo.SaveTemplate(ctx, "00000000-0000-4000-8000-000000000000", "Otra", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown template updated", err)
	}
	values, err := f.repo.Templates(ctx)
	if err != nil || len(values) != 1 || values[0].Body != "De 8 a 15." {
		t.Fatal(values, err)
	}
	if err := f.repo.DeleteTemplate(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.DeleteTemplate(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted template deleted twice", err)
	}
	for i := range TemplatesMax {
		if _, err := f.repo.SaveTemplate(ctx, "", "t"+strings.Repeat("x", i), "x"); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := f.repo.SaveTemplate(ctx, "", "una más", "x"); !errors.Is(err, ErrTemplateLimit) {
		t.Fatal("template limit not enforced", err)
	}
}
