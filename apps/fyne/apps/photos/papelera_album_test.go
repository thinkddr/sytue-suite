// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"slices"
	"testing"
)

// TestRestaurarTodoEsUnaPeticion: «Restaurar todo» vacía la papelera con UNA petición
// al servicio (no una por foto) y lo dice con el recuento.
func TestRestaurarTodoEsUnaPeticion(t *testing.T) {
	s := &servicioFalso{papelera: fotosDePrueba(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Ir(RutaPapelera)
	d := a.datosPapelera()
	if d.AlRestaurarTodo == nil {
		t.Fatal("la papelera no ofrece «Restaurar todo»")
	}
	antes := len(s.vistas())
	d.AlRestaurarTodo()
	pedidas := s.vistas()[antes:]
	if !slices.Equal(pedidas, []string{"POST /api/v1/trash/restore?"}) {
		t.Errorf("peticiones = %v, quiero una sola a /trash/restore", pedidas)
	}
	if len(a.pap.fotos) != 0 || a.pap.aviso != a.v.T.T("trash.restoredAll", map[string]any{"count": 3}) {
		t.Errorf("tras restaurar todo: %d fotos y aviso %q", len(a.pap.fotos), a.pap.aviso)
	}
}

// TestElVisorAbreElOriginalEnDrive: con el fichero de Drive y una app que abrir, el menú
// de descargar lleva «Ver el original en Sytue Drive» y abre ESE fichero en Drive.
func TestElVisorAbreElOriginalEnDrive(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(2), porPagina: 100}
	a, _ := appDePrueba(t, s)
	var abierto []string
	a.AbrirApp = func(producto, ruta string) { abierto = append(abierto, producto, ruta) }
	a.Arrancar()
	f := a.g.fotos[0]
	f.DriveFileID = "fich ero/1"
	a.abrirVisor(desdeGaleria, 0)
	d := a.datosVisor(a.vis, f)
	if d.AlAbrirEnDrive == nil {
		t.Fatal("el visor no ofrece abrir el original en Drive")
	}
	d.AlAbrirEnDrive()
	if !slices.Equal(abierto, []string{"drive", "/files/fich%20ero%2F1"}) {
		t.Errorf("abrió %v", abierto)
	}
	// Sin fichero de Drive, no se ofrece.
	f.DriveFileID = ""
	if a.datosVisor(a.vis, f).AlAbrirEnDrive != nil {
		t.Error("ofrece abrir en Drive una foto sin fichero")
	}
}

// TestLosAtajosNuevosDelVisorYLaGaleria: F (favorita) y E (editor) en el visor, «/» en
// la galería; la hoja no promete la F donde no hay papelera (una foto que no es del
// servicio tampoco tiene corazón).
func TestLosAtajosNuevosDelVisorYLaGaleria(t *testing.T) {
	if accionDe(atajosVisor, "f") != atajoFavorita || accionDe(atajosVisor, "e") != atajoEditar {
		t.Error("F y E no son la favorita y el editor en el visor")
	}
	if accionDe(atajosGaleria, "/") != atajoBuscar {
		t.Error("«/» no lleva al buscador en la galería")
	}
	v := vistaDePrueba(t, NuevaFototeca())
	descripciones := func(papelera bool) []string {
		g := gruposDeAtajos(v, true, true, papelera)
		atajos := g[len(g)-1].Atajos
		out := make([]string, 0, len(atajos))
		for _, x := range atajos {
			out = append(out, x.Descripcion)
		}
		return out
	}
	if !slices.Contains(descripciones(true), v.Traduce("shortcuts.favorite")) {
		t.Error("la hoja del visor no cuenta la F")
	}
	if slices.Contains(descripciones(false), v.Traduce("shortcuts.favorite")) {
		t.Error("la hoja promete la F en un visor sin foto del servicio")
	}
}
