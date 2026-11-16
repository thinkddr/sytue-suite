// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLosBloquesEsqueletoOcupanLoMismoQueElEsqueletoSuelto: cambiar `NuevoEsqueleto` por
// un Bloque del grupo no mueve ni un píxel: pide lo mismo y se queda con lo que le dé su
// contenedor, con el mismo `rounded-md`.
//
// VISTO EN ROJO con el Bloque a su medida fija en vez de estirado por el `Margen`: la
// celda de la rejilla de Calendar pasaba de 128 de ancho a 24.
func TestLosBloquesEsqueletoOcupanLoMismoQueElEsqueletoSuelto(t *testing.T) {
	for _, c := range []struct{ ancho, alto float32 }{{2 * tema.Rem, 0.75 * tema.Rem}, {0, 0}, {0, 36}} {
		suelto := NuevoEsqueleto(c.ancho, c.alto)
		b := NuevosBloquesEsqueleto()
		bloque := b.Bloque(c.ancho, c.alto)
		if bloque.MinSize() != suelto.MinSize() {
			t.Errorf("%v×%v: el bloque pide %v y el esqueleto suelto %v", c.ancho, c.alto, bloque.MinSize(), suelto.MinSize())
		}
		margen := Margen{Izq: 6, Arr: 6, Der: 6, Abj: 6}
		b.Con(container.New(margen, bloque))
		antes := container.New(margen, suelto)
		for _, o := range []fyne.CanvasObject{b, antes} {
			o.Resize(fyne.NewSize(140, 36))
		}
		test.WidgetRenderer(b).Layout(b.Size())
		r := test.WidgetRenderer(suelto).Objects()[0]
		if bloque.Position() != suelto.Position() || bloque.Size() != r.Size() || bloque.CornerRadius != tema.RadioMD {
			t.Errorf("%v×%v: el bloque cae en %v y mide %v (radio %g); el suelto, en %v y %v",
				c.ancho, c.alto, bloque.Position(), bloque.Size(), bloque.CornerRadius, suelto.Position(), r.Size())
		}
		if b.MinSize() != antes.MinSize() {
			t.Errorf("%v×%v: el grupo pide %v y lo de antes %v", c.ancho, c.alto, b.MinSize(), antes.MinSize())
		}
		b.Parar()
	}
}

// TestLosBloquesEsqueletoLatenAUnaYSeAnuncianUnaVez: una sola animación para todos los
// bloques del grupo, del mismo tono en cada instante, y un solo «Cargando» (§3.18).
func TestLosBloquesEsqueletoLatenAUnaYSeAnuncianUnaVez(t *testing.T) {
	b := NuevosBloquesEsqueleto()
	celdas := make([]fyne.CanvasObject, 0, 14)
	for range 14 {
		celdas = append(celdas, b.Bloque(1.5*tema.Rem, 1.5*tema.Rem))
	}
	b.Con(container.New(Rejilla{Fijas: 7}, celdas...))
	r := test.WidgetRenderer(b).(*bloquesEsqueletoRenderer)
	defer b.Parar()
	if b.anim == nil {
		t.Fatal("el grupo no ha arrancado su latido")
	}
	b.mu.Lock()
	b.fase = 0.5
	b.mu.Unlock()
	r.tenir()
	primero := b.Bloques()[0].FillColor
	for i, q := range b.Bloques() {
		if q.FillColor != primero {
			t.Fatalf("el bloque %d pinta %v y el primero %v: no laten a una", i, q.FillColor, primero)
		}
	}
	if primero == tintaEsqueleto("", 0) {
		t.Error("a mitad de latido los bloques siguen opacos: el latido no llega a los bloques")
	}
	if got, quiere := b.AccessibilityLabel(), textoComun("ui.loading"); got != quiere || got == "" {
		t.Errorf("el grupo se anuncia %q y tiene que decir %q", got, quiere)
	}
	if got := b.ConRotulo("Cargando el calendario").AccessibilityLabel(); got != "Cargando el calendario" {
		t.Errorf("con rótulo propio se anuncia %q", got)
	}
	if c := b.SobreCromo().Tinte; c != "sidebar-hover-bg" {
		t.Errorf("sobre el cromo el tinte es %q", c)
	}

	SinMovimiento = true
	defer func() { SinMovimiento = false }()
	quieto := NuevosBloquesEsqueleto()
	q := quieto.Bloque(0, 0)
	test.WidgetRenderer(quieto.Con(q))
	if quieto.anim != nil || q.FillColor != tintaEsqueleto("", 0) {
		t.Error("con SinMovimiento el grupo late o sus bloques no son opacos")
	}
}

// TestLaRayaDeUnaRecetaLlevaSuRadio: una raya a toda la fila hace de fila entera, y su
// radio es el de lo que va a llegar (`radius-nav` en los cuadernos del menú de Notes);
// sin él, el `rounded-md` de siempre.
func TestLaRayaDeUnaRecetaLlevaSuRadio(t *testing.T) {
	nav := tema.Forma("radius-nav") + 1 // distinto de RadioMD aunque la forma coincida
	f := NuevasFilasEsqueleto(2, SinDelantero, 1).ConReceta(RecetaEsqueleto{
		Alto: 32, Rayas: []RayaEsqueleto{{Alto: 32, Radio: nav}, {Alto: 8}},
	})
	f.Resize(fyne.NewSize(200, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	r.Layout(f.Size())
	if got := r.filas[0].rayas[0].CornerRadius; got != nav {
		t.Errorf("la raya con radio propio lleva %g y es %g", got, nav)
	}
	if got := r.filas[0].rayas[1].CornerRadius; got != tema.RadioMD {
		t.Errorf("la raya sin radio lleva %g y es el rounded-md (%g)", got, tema.RadioMD)
	}
}

// esqueletosPendientes son las cargas de Notes y Calendar que aún montan un
// `ui.Esqueleto` por bloque en un bucle o en una lista, con el motivo. Cada una late a su
// aire; al pasarla a FilasEsqueleto o BloquesEsqueleto, se borra de aquí.
var esqueletosPendientes = map[string]string{
	"apps/notes/esqueletos.go:esqueleto": "la hoja y las tarjetas del lienzo: líneas a fracción del ancho, que un Bloque aún no hace",
	"calendario/huecos.go:panelHuecos":   "los dos de «buscar hueco», que se paran a mano al terminar la carga (`buscaHueco.cargado`)",
}

// TestNingunaAppMontaSusBloquesDeCarga es la pareja de TestNingunaAppMontaSusFilasDeCarga
// para lo que no son filas: un `ui.NuevoEsqueleto*` dentro de un bucle, o dos o más en
// una misma lista, son un grupo de carga montado a mano —cada bloque con su latido y
// ninguno anunciando la carga—; va con ui.FilasEsqueleto o ui.BloquesEsqueleto.
//
// De momento mira Notes y Calendar, las dos que se han pasado. Por el resto de la suite
// la misma regla aún encuentra grupos a mano (Cast, Drive, ERP, Hub, Photos): cuando se
// pasen, se añaden sus carpetas.
//
// VISTO EN ROJO con los cuadernos del menú de Notes y el menú, las salas y la rejilla de
// Calendar como estaban.
func TestNingunaAppMontaSusBloquesDeCarga(t *testing.T) {
	esEsqueleto := func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		x, ok := s.X.(*ast.Ident)
		return ok && x.Name == "ui" && strings.HasPrefix(s.Sel.Name, "NuevoEsqueleto")
	}
	contar := func(n ast.Node) int {
		k := 0
		ast.Inspect(n, func(m ast.Node) bool {
			if esEsqueleto(m) {
				k++
			}
			return true
		})
		return k
	}
	vistos := map[string]bool{}
	for _, dir := range []string{"../apps/notes", "../calendario"} {
		err := filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(token.NewFileSet(), ruta, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				grupo := false
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.ForStmt:
						grupo = grupo || contar(x.Body) > 0
					case *ast.RangeStmt:
						grupo = grupo || contar(x.Body) > 0
					case *ast.CompositeLit:
						directos := 0
						for _, e := range x.Elts {
							if contar(e) > 0 { // también `ui.NuevoEsqueleto(…).SobreCromo()`
								directos++
							}
						}
						grupo = grupo || directos >= 2
					}
					return true
				})
				if !grupo {
					continue
				}
				clave := strings.TrimPrefix(filepath.ToSlash(ruta), "../") + ":" + fn.Name.Name
				vistos[clave] = true
				if _, pendiente := esqueletosPendientes[clave]; !pendiente {
					t.Errorf("%s monta sus bloques de carga con un ui.Esqueleto por bloque: van con ui.FilasEsqueleto o ui.BloquesEsqueleto", clave)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for clave := range esqueletosPendientes {
		if !vistos[clave] {
			t.Errorf("%s ya no monta bloques sueltos: bórralo de esqueletosPendientes", clave)
		}
	}
}
