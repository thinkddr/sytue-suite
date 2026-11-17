// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// lienzoConHojas es lienzoTraducido con otras hojas.
func lienzoConHojas(t *testing.T, hojas ...string) *Lienzo {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	c := NuevoLienzo(NuevoLibroCon(hojas, 0))
	c.T = tr.T
	c.Resize(fyne.NewSize(560, 300))
	return c
}

// TestLasHojasNoSeHacenGemelas: dos hojas que solo difieren en mayúsculas («Ventas» y
// «VENTAS») no se pueden tener —Excel no deja, y el servidor rechaza el guardado—. Añadir
// y duplicar eligen el siguiente nombre libre y renombrar a una gemela no hace nada; las
// mayúsculas de la propia hoja sí se cambian. VISTO EN ROJO sin la regla: «Hoja 3» junto
// a «hoja 3», «Datos (2)» junto a «DATOS (2)» y «Resumen» renombrada «VENTAS».
func TestLasHojasNoSeHacenGemelas(t *testing.T) {
	c := lienzoConHojas(t, "Hoja 1", "hoja 3")
	c.AnadirHoja()
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Hoja 1", "hoja 3", "Hoja 4"}) {
		t.Errorf("añadir junto a «hoja 3» deja %v", got)
	}

	c = lienzoConHojas(t, "Datos", "DATOS (2)")
	c.DuplicarHoja()
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Datos", "Datos (3)", "DATOS (2)"}) {
		t.Errorf("duplicar «Datos» junto a «DATOS (2)» deja %v", got)
	}

	c = lienzoConHojas(t, "Ventas", "Resumen")
	c.RenombrarHoja(1, "VENTAS")
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Ventas", "Resumen"}) {
		t.Errorf("renombrar «Resumen» a «VENTAS» deja %v", got)
	}
	if got, quiero := c.ValidarNombreHoja(1, "ventas"), c.t("dialog.sheet.nameTaken", i18n.Vars{"name": "Ventas"}); got != quiero || !strings.Contains(got, "«Ventas»") {
		t.Errorf("el aviso de renombrar a «ventas» es %q y tenía que ser %q", got, quiero)
	}
	if got := c.ValidarNombreHoja(0, "VENTAS"); got != "" {
		t.Errorf("cambiar las mayúsculas de la propia hoja avisa %q", got)
	}
	c.RenombrarHoja(0, "VENTAS")
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"VENTAS", "Resumen"}) {
		t.Errorf("cambiar las mayúsculas de la propia hoja deja %v", got)
	}
}

// TestUnLibroConGemelasSeSigueEditando: un libro que ya las trae (un importado, uno de
// antes) se edita, y renombrar una gemela a un nombre libre la deshace; su propio nombre
// no avisa aunque la otra se llame igual salvo mayúsculas.
func TestUnLibroConGemelasSeSigueEditando(t *testing.T) {
	c := lienzoConHojas(t, "Ventas", "VENTAS")
	if got := c.ValidarNombreHoja(1, "VENTAS"); got != "" {
		t.Errorf("el nombre que ya tiene la hoja avisa %q", got)
	}
	c.RenombrarHoja(1, "Ventas 2")
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Ventas", "Ventas 2"}) {
		t.Errorf("renombrar la gemela a un nombre libre deja %v", got)
	}
}

// TestRenombrarAUnaGemelaSeAvisaEnElCampo: el diálogo de renombrar dice bajo el campo
// que ya hay una hoja con ese nombre y no deja guardar; con otro nombre, renombra.
func TestRenombrarAUnaGemelaSeAvisaEnElCampo(t *testing.T) {
	c := lienzoConHojas(t, "Ventas", "Resumen")
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(900, 600))
	c.PedirRenombrarHoja(1)
	campo, ok := w.Canvas().Focused().(*ui.Entrada)
	if !ok {
		t.Fatalf("el foco ha caído en %T y tiene que caer en el campo", w.Canvas().Focused())
	}
	aviso := c.t("dialog.sheet.nameTaken", i18n.Vars{"name": "Ventas"})
	campo.PonerTexto("VENTAS")
	if !strings.Contains(textoDeLaVentana(w.Canvas().Overlays().Top()), aviso) || !campo.Invalida {
		t.Fatalf("el diálogo no avisa «%s» bajo el campo", aviso)
	}
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Ventas", "Resumen"}) || w.Canvas().Overlays().Top() == nil {
		t.Errorf("Intro con el aviso ha renombrado (%v) o cerrado el diálogo", got)
	}
	campo.PonerTexto("Ventas 2026")
	if strings.Contains(textoDeLaVentana(w.Canvas().Overlays().Top()), aviso) {
		t.Error("con otro nombre el aviso no se va")
	}
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := c.Libro.Hojas(); !slices.Equal(got, []string{"Ventas", "Ventas 2026"}) {
		t.Errorf("con otro nombre la hoja no se renombra: %v", got)
	}
}
