// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Un álbum de u1 compartido con u2: u2 lo ve en «Compartido conmigo» y ve sus fotos,
// sin el enlace de su dueña; u3, de la misma organización, no; solo la dueña lo
// comparte, ve con quién y lo retira; y retirado, u2 deja de verlo.
func TestAlbum_compartirConUnaPersona(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("a.jpg", "d1", nil))
	al, err := st.CrearAlbum(ctx, "u1", "Verano")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CompartirAlbum(ctx, al.ID, "u1"); err != nil { // con enlace también
		t.Fatal(err)
	}

	if _, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u1", false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("compartir con una misma: %v", err)
	}
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "u2", "u3", false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("u2 comparte un álbum que no es suyo: %v", err)
	}
	if _, err := st.AlbumCompartido(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("u2 ve el álbum antes de que se lo compartan: %v", err)
	}
	nueva, titulo, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", false)
	if err != nil || !nueva || titulo != "Verano" {
		t.Fatalf("compartir: nueva=%v título=%q err=%v", nueva, titulo, err)
	}
	if nueva, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", false); err != nil || nueva {
		t.Fatalf("compartir dos veces no es nuevo: nueva=%v err=%v", nueva, err)
	}

	lista, err := st.AlbumesCompartidos(ctx, "u2")
	if err != nil || len(lista) != 1 || lista[0].ID != al.ID || lista[0].Duenyo != "u1" || lista[0].Compartido {
		t.Fatalf("«Compartido conmigo» de u2: %+v %v", lista, err)
	}
	visto, err := st.AlbumCompartido(ctx, al.ID, "u2")
	if err != nil || visto.Compartido || visto.Fotos != 1 {
		t.Fatalf("u2 abre el álbum: %+v %v", visto, err)
	}
	if fs, err := st.FotosDelAlbumPublico(ctx, visto); err != nil || len(fs) != 1 || fs[0].Activo.ID != a.ID {
		t.Fatalf("fotos del álbum compartido: %+v %v", fs, err)
	}
	if l, _ := st.AlbumesCompartidos(ctx, "u3"); len(l) != 0 {
		t.Fatal("u3 ve un álbum compartido solo con u2")
	}
	if _, err := st.AlbumCompartido(ctx, al.ID, "u3"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("u3 abre el álbum de u2: %v", err)
	}
	// Ver no es editar: quien lo recibe no es su dueña.
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u2", []uuid.UUID{a.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("u2 añade fotos a un álbum compartido para ver: %v", err)
	}
	if _, err := st.PersonasDelAlbum(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("u2 ve con quién está compartido: %v", err)
	}
	ps, err := st.PersonasDelAlbum(ctx, al.ID, "u1")
	if err != nil || len(ps) != 1 || ps[0].Sujeto != "u2" {
		t.Fatalf("personas: %+v %v", ps, err)
	}

	if err := st.DejarDeCompartirConPersona(ctx, al.ID, "u2", "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("u2 retira lo que no es suyo: %v", err)
	}
	if err := st.DejarDeCompartirConPersona(ctx, al.ID, "u1", "u2"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AlbumCompartido(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("retirado, u2 sigue abriéndolo: %v", err)
	}
	if err := st.DejarDeCompartirConPersona(ctx, al.ID, "u1", "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("retirar dos veces: %v", err)
	}
	// Borrar el álbum se lleva lo compartido.
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", false); err != nil {
		t.Fatal(err)
	}
	if err := st.BorrarAlbum(ctx, al.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.AlbumesCompartidos(ctx, "u2"); len(l) != 0 {
		t.Fatal("un álbum borrado le sigue saliendo a u2")
	}
}

// En un álbum solo entran fotos de su dueña: una ajena no se añade, y aunque estuviera
// (de antes), no se sirve ni por el enlace ni a quien se lo compartieron.
func TestAlbum_soloFotosDeSuDuenya(t *testing.T) {
	st, ctx := nuevoStore(t)
	mia, _ := st.Ingerir(ctx, foto("mia.jpg", "d1", nil))
	ajena := foto("ajena.jpg", "d2", nil)
	ajena.Duenyo = "u9"
	otra, err := st.Ingerir(ctx, ajena)
	if err != nil {
		t.Fatal(err)
	}
	al, _ := st.CrearAlbum(ctx, "u1", "Mío")
	n, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{mia.ID, otra.ID})
	if err != nil || n != 1 {
		t.Fatalf("añadir: n=%d err=%v (solo entra la mía)", n, err)
	}
	if _, err := st.FotoDelAlbumPublico(ctx, al, otra.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("la foto ajena se sirve por el álbum: %v", err)
	}
}

// Con edición, quien lo recibe PUEDE EDITARLO (y la dueña sigue siendo la dueña); solo
// para ver, no; y bajar el nivel se nota en el acto. El nivel se cambia sin «compartir
// de nuevo» (no se vuelve a avisar).
func TestAlbum_compartirParaEditar(t *testing.T) {
	st, ctx := nuevoStore(t)
	al, _ := st.CrearAlbum(ctx, "u1", "Viaje")
	if _, _, err := st.DuenyaParaEditar(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("u2 edita sin que se lo compartan: %v", err)
	}
	if duenya, editora, err := st.DuenyaParaEditar(ctx, al.ID, "u1"); err != nil || duenya != "u1" || editora {
		t.Fatalf("la dueña: %q editora=%v %v", duenya, editora, err)
	}
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.DuenyaParaEditar(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("solo para ver, u2 edita: %v", err)
	}
	if nueva, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", true); err != nil || nueva {
		t.Fatalf("dar edición: nueva=%v err=%v", nueva, err)
	}
	if duenya, editora, err := st.DuenyaParaEditar(ctx, al.ID, "u2"); err != nil || duenya != "u1" || !editora {
		t.Fatalf("con edición: %q editora=%v %v", duenya, editora, err)
	}
	if visto, _ := st.AlbumCompartido(ctx, al.ID, "u2"); !visto.Editable {
		t.Error("«Compartido conmigo» no dice que se puede editar")
	}
	if l, _ := st.AlbumesCompartidos(ctx, "u2"); len(l) != 1 || !l[0].Editable {
		t.Errorf("la lista no dice que se puede editar: %+v", l)
	}
	if ps, _ := st.PersonasDelAlbum(ctx, al.ID, "u1"); len(ps) != 1 || !ps[0].Editar {
		t.Errorf("personas: %+v", ps)
	}
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "u1", "u2", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.DuenyaParaEditar(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bajado a ver, u2 sigue editando: %v", err)
	}
}

// Una foto que ya está en Photos a nombre de otra persona no se le quita, ni se le
// reescriben los datos, ni sale de su papelera: ni reindexando su fichero ni dándolo de
// alta como copia para la dueña de un álbum.
func TestAlbum_noSeApropiaDeFotosAjenas(t *testing.T) {
	st, ctx := nuevoStore(t)
	de3 := foto("tercera.jpg", "d-3", nil)
	de3.Duenyo = "u3"
	f3, err := st.Ingerir(ctx, de3)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.ALaPapelera(ctx, f3.ID, "u3")
	otro := foto("renombrada.jpg", "d-3", nil) // mismo fichero, de u1
	if _, err := st.Ingerir(ctx, otro); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("reindexar un fichero ajeno: %v", err)
	}
	copia := foto("copia.jpg", "d-3", nil)
	if _, err := st.AltaDeCopia(ctx, copia); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("dar de alta como copia un fichero ajeno: %v", err)
	}
	if a, err := st.Activo(ctx, f3.ID, "u3"); err == nil {
		t.Errorf("la foto de u3 salió de su papelera: %+v", a)
	}
	// La propia en la papelera tampoco sale como copia.
	mia, _ := st.Ingerir(ctx, foto("mia.jpg", "d-1", nil))
	_ = st.ALaPapelera(ctx, mia.ID, "u1")
	if _, err := st.AltaDeCopia(ctx, foto("mia.jpg", "d-1", nil)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("una copia saca de la papelera: %v", err)
	}
	// Y una nueva sí.
	if a, err := st.AltaDeCopia(ctx, foto("nueva.jpg", "d-9", nil)); err != nil || a.Duenyo != "u1" {
		t.Errorf("alta de una copia nueva: %+v %v", a, err)
	}
}
