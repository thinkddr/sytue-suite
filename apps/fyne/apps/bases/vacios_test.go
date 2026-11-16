// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// vacioEn es el estado vacío que se ve en la ventana, o nil.
func vacioEn(w fyne.Window) *ui.Vacio {
	var hallado *ui.Vacio
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if v, ok := o.(*ui.Vacio); ok {
			hallado = v
		}
	})
	return hallado
}

// accionesDe son los rótulos de los botones del estado vacío, principal primero.
func accionesDe(v *ui.Vacio) []string {
	var out []string
	for _, o := range []fyne.CanvasObject{v.Accion, v.AccionSecundaria} {
		if b, ok := o.(*ui.Boton); ok {
			out = append(out, b.Texto)
		}
	}
	return out
}

// capturarEnLosDosTemas retrata la ventana en claro y en oscuro.
func capturarEnLosDosTemas(t *testing.T, p *Pantalla, w fyne.Window, nombre string) {
	t.Helper()
	capturar(t, w, nombre)
	// Volver a poner el tema avisa a todo el lienzo, fondo incluido, como al cambiar la
	// apariencia en la app.
	cambiar := func(apariencia string) {
		tema.Elegir(apariencia)
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
		fyne.DoAndWait(p.Repintar)
	}
	t.Cleanup(func() { tema.Elegir("") })
	cambiar("oscuro")
	capturar(t, w, nombre+"-oscuro")
	cambiar("")
}

// TestQueAccionOfreceCadaVacioSegunElRol es la tabla de `lib/vacios.test.ts`: el
// propietario crea tablas y campos, quien edita añade registros, quien lee no ve
// botones que el servicio le negaría, y volver a verlo todo es de cualquiera.
func TestQueAccionOfreceCadaVacioSegunElRol(t *testing.T) {
	propietario, editor, lector := permisos{true, true}, permisos{false, true}, permisos{}
	casos := []struct {
		e      estadoVacio
		p      permisos
		busca  bool
		desc   string
		quiero []accionVacio
	}{
		{sinTablas, propietario, false, "base.noTables.owner", []accionVacio{crearTabla, importarCsv}},
		{sinTablas, editor, false, "base.noTables.reader", nil},
		{sinTablas, lector, false, "base.noTables.reader", nil},
		{sinCampos, propietario, false, "grid.noFields.owner", []accionVacio{anadirCampo}},
		{sinCampos, editor, false, "grid.noFields.reader", nil},
		{sinRegistros, editor, false, "grid.empty.editor", []accionVacio{anadirRegistro}},
		{sinRegistros, lector, false, "grid.empty.reader", nil},
		{sinResultados, lector, true, "grid.noMatches.body", []accionVacio{verTodos}},
		{sinResultados, lector, false, "grid.viewEmpty.body", []accionVacio{verTodos}},
	}
	for _, c := range casos {
		v := vacioDe(c.e, c.p, c.busca)
		if !slices.Equal(v.acciones, c.quiero) || v.descripcion != c.desc {
			t.Errorf("vacioDe(%d, %+v) = %v %q; quiero %v %q", c.e, c.p, v.acciones, v.descripcion, c.quiero, c.desc)
		}
	}
}

// TestUnaBaseSinTablasGuia: el propietario ve el estado vacío con «Añadir tabla» e
// «Importar CSV»; quien solo lee, el porqué y ningún botón.
func TestUnaBaseSinTablasGuia(t *testing.T) {
	f := nuevoFalso(0)
	f.tablas = nil
	p, w := abrir(t, f)
	v := vacioEn(w)
	if v == nil || v.Titulo != "Esta base todavía no tiene tablas" {
		t.Fatalf("sin tablas no hay estado vacío:\n%s", textos(w))
	}
	if got := accionesDe(v); !slices.Equal(got, []string{"Añadir tabla", "Importar CSV"}) {
		t.Errorf("acciones del propietario: %q", got)
	}
	capturarEnLosDosTemas(t, p, w, "vacio-sin-tablas")
	// «Añadir tabla» pide el nombre, como desde el menú.
	test.Tap(v.Accion.(*ui.Boton))
	var campo *ui.Entrada
	enCapas(w, func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Entrada); ok {
			campo = e
		}
	})
	if campo == nil {
		t.Fatal("«Añadir tabla» no pide el nombre")
	}
	campo.PonerTexto("Clientes")
	pulsar(t, w, "Crear tabla")
	if len(f.tablas) != 1 {
		t.Error("«Añadir tabla» no crea la tabla")
	}

	f = nuevoFalso(0)
	f.tablas, f.rol = nil, "editor"
	_, w = abrir(t, f)
	v = vacioEn(w)
	if v == nil || len(accionesDe(v)) != 0 || !strings.Contains(v.Descripcion, "administra") {
		t.Fatalf("un editor ve acciones de esquema o no sabe por qué:\n%s", textos(w))
	}
}

// TestUnaTablaSinCamposGuia: «Añadir campo» solo para el propietario.
func TestUnaTablaSinCamposGuia(t *testing.T) {
	for _, rol := range []string{"owner", "editor"} {
		f := nuevoFalso(0)
		f.tablas[0]["fields"] = []m{}
		f.rol = rol
		p, w := abrir(t, f)
		v := vacioEn(w)
		if v == nil || v.Titulo != "Esta tabla todavía no tiene campos" {
			t.Fatalf("%s: sin campos no hay estado vacío:\n%s", rol, textos(w))
		}
		got := accionesDe(v)
		if rol == "owner" {
			if !slices.Equal(got, []string{"Añadir campo"}) {
				t.Errorf("acciones del propietario: %q", got)
			}
			capturarEnLosDosTemas(t, p, w, "vacio-sin-campos")
		} else if len(got) != 0 {
			t.Errorf("un editor puede añadir campos: %q", got)
		}
	}
}

// TestUnaTablaSinRegistrosGuia: quien edita añade el primero desde el estado vacío (sin
// otro «Añadir registro» al pie); quien lee, no.
func TestUnaTablaSinRegistrosGuia(t *testing.T) {
	f := nuevoFalso(0)
	f.rol = "editor"
	p, w := abrir(t, f)
	v := vacioEn(w)
	if v == nil || v.Titulo != "Todavía no hay registros" || !slices.Equal(accionesDe(v), []string{"Añadir registro"}) {
		t.Fatalf("sin registros no se guía al editor:\n%s", textos(w))
	}
	if n := strings.Count(textos(w), "[Añadir registro]"); n != 1 {
		t.Errorf("«Añadir registro» sale %d veces", n)
	}
	capturarEnLosDosTemas(t, p, w, "vacio-sin-registros")
	test.Tap(v.Accion.(*ui.Boton))
	if len(f.registros["t1"]) != 1 || len(p.abierta.rejilla.Registros) != 1 {
		t.Fatal("«Añadir registro» no añade")
	}
	if vacioEn(w) != nil {
		t.Error("con un registro sigue el estado vacío")
	}

	f = nuevoFalso(0)
	f.rol = "viewer"
	_, w = abrir(t, f)
	if v := vacioEn(w); v == nil || len(accionesDe(v)) != 0 || strings.Contains(textos(w), "[Añadir registro]") {
		t.Fatalf("un lector puede añadir registros:\n%s", textos(w))
	}
}

// TestUnRecorteVacioOfreceVerTodos: una búsqueda o una vista con filtros que no
// devuelven nada no son «no hay registros»: se ofrece verlos todos, también a quien lee.
func TestUnRecorteVacioOfreceVerTodos(t *testing.T) {
	f := nuevoFalso(3)
	f.rol = "viewer"
	p, w := abrir(t, f)
	b := p.abierta
	fyne.DoAndWait(func() {
		b.busqueda.PonerTexto("nadie")
		b.busqueda.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	})
	v := vacioEn(w)
	if v == nil || v.Titulo != "Ningún registro coincide con la búsqueda" || !slices.Equal(accionesDe(v), []string{"Ver todos los registros"}) {
		t.Fatalf("una búsqueda sin resultados no ofrece verlo todo:\n%s", textos(w))
	}
	capturarEnLosDosTemas(t, p, w, "vacio-busqueda")
	test.Tap(v.Accion.(*ui.Boton))
	if b.lectura.Q != "" || b.busqueda.Texto() != "" || len(b.rejilla.Registros) != 3 {
		t.Fatalf("«Ver todos» no quita la búsqueda: %+v, %d registros", b.lectura, len(b.rejilla.Registros))
	}

	// Una vista con filtros que no deja pasar nada: «Ver todos» vuelve a «todos».
	f = nuevoFalso(0)
	f.vistas["t1"] = []m{{"id": "vf", "table_id": "t1", "name": "Mayores", "kind": "grid", "config": m{
		"filters": []m{{"field": "eda", "op": "gt", "value": 90}},
	}}}
	p, w = abrir(t, f)
	b = p.abierta
	elegir(t, w, "Vistas", "Mayores · Rejilla")
	v = vacioEn(w)
	if v == nil || v.Titulo != "No hay registros en esta vista" || !slices.Equal(accionesDe(v), []string{"Ver todos los registros"}) {
		t.Fatalf("una vista vacía no ofrece verlo todo:\n%s", textos(w))
	}
	// El recorte no es la tabla vacía: añadir sigue al pie.
	if !strings.Contains(textos(w), "[Añadir registro]") {
		t.Error("en una vista filtrada vacía desaparece «Añadir registro»")
	}
	test.Tap(v.Accion.(*ui.Boton))
	if b.VistaID != "" {
		t.Errorf("«Ver todos» no vuelve a todos los registros: vista %q", b.VistaID)
	}
}
