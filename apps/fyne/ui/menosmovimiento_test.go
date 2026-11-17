// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// appQueCuentaAnimaciones es la app de pruebas con un driver que apunta cada
// `Animation.Start`: el driver de pruebas salta cada animación a su final en el acto, así
// que lo que acaba en su sitio no dice si se animó o salió ya puesto; esto sí.
type appQueCuentaAnimaciones struct {
	fyne.App
	driver driverQueCuentaAnimaciones
}

func (a appQueCuentaAnimaciones) Driver() fyne.Driver { return a.driver }

type driverQueCuentaAnimaciones struct {
	fyne.Driver
	arrancadas *atomic.Int32
}

func (d driverQueCuentaAnimaciones) StartAnimation(a *fyne.Animation) {
	d.arrancadas.Add(1)
	d.Driver.StartAnimation(a)
}

// contarAnimaciones pone la app que cuenta mientras dure el test.
func contarAnimaciones(t *testing.T) *atomic.Int32 {
	t.Helper()
	antes := fyne.CurrentApp()
	n := &atomic.Int32{}
	fyne.SetCurrentApp(appQueCuentaAnimaciones{App: antes, driver: driverQueCuentaAnimaciones{Driver: antes.Driver(), arrancadas: n}})
	t.Cleanup(func() { fyne.SetCurrentApp(antes) })
	return n
}

// TestElAjusteDelSistemaApagaLasAnimaciones es el «Reducir movimiento» del sistema, sin
// que la app ponga SinMovimiento: la rueda queda quieta, el esqueleto opaco y el cajón y
// el diálogo salen ya puestos, sin arrancar animación ninguna. Con el ajuste en «no», cada
// uno se anima. VISTO EN ROJO con las comprobaciones leyendo SinMovimiento a pelo: con el
// sistema pidiéndolo, la rueda giraba y el cajón y el diálogo entraban animados.
func TestElAjusteDelSistemaApagaLasAnimaciones(t *testing.T) {
	if SinMovimiento {
		t.Fatal("el paquete corre con SinMovimiento puesto: la prueba no mediría el sistema")
	}
	casos := []struct {
		nombre string
		montar func(t *testing.T)
	}{
		{"rueda", func(t *testing.T) {
			r := NuevaRueda(24, "text", tema.Claro)
			r.Resize(MedidaRueda(24))
			_ = pinta(t, 24, 24, r)
		}},
		{"esqueleto", func(t *testing.T) {
			e := NuevoEsqueleto(40, 20)
			e.Resize(fyne.NewSize(40, 20))
			_ = pinta(t, 40, 20, e)
			if o := opacidadPulso(e.fase); o != 1 {
				t.Errorf("el esqueleto está a %g de opacidad y quieto va opaco", o)
			}
		}},
		{"cajón", func(t *testing.T) {
			a := NuevoArmazon(iconoFalso())
			a.Menu = NuevoLateral("Navegación principal", nil)
			w := test.NewTempWindow(t, a)
			w.Resize(fyne.NewSize(360, 740))
			a.Resize(fyne.NewSize(360, 740))
			a.cambiarCajon(true)
			if a.entrada != 0 {
				t.Errorf("el cajón ha quedado a %v de su sitio", a.entrada)
			}
		}},
		{"diálogo", func(t *testing.T) {
			w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
			w.Resize(fyne.NewSize(800, 600))
			d := NuevoDialogo("Reglas", cuerpoDe(60))
			d.Mostrar(w.Canvas())
			t.Cleanup(d.Cerrar)
			if d.capa.fundido != nil || !d.Visible() {
				t.Error("el diálogo no ha quedado puesto")
			}
		}},
	}
	for _, c := range casos {
		for _, menos := range []bool{true, false} {
			nombre := c.nombre + "/se anima"
			if menos {
				nombre = c.nombre + "/reducir movimiento"
			}
			t.Run(nombre, func(t *testing.T) {
				t.Cleanup(sistema.FijarMenosMovimiento(menos))
				if MenosMovimiento() != menos {
					t.Fatalf("con el sistema en %v, MenosMovimiento() = %v", menos, !menos)
				}
				n := contarAnimaciones(t)
				c.montar(t)
				switch got := n.Load(); {
				case menos && got != 0:
					t.Errorf("con «Reducir movimiento» en el sistema se han arrancado %d animaciones", got)
				case !menos && got == 0:
					t.Error("sin «Reducir movimiento» no se ha animado nada")
				}
			})
		}
	}
}

// TestNadieLeeSinMovimientoAPelo: decidir si se anima es MenosMovimiento, que atiende al
// ajuste del sistema; una comprobación que leyera SinMovimiento sola volvería a animar
// para quien lo ha pedido en el sistema. Se recorre el código del módulo (sin tests) y
// solo se admite ASIGNAR la variable, más su declaración y MenosMovimiento.
func TestNadieLeeSinMovimientoAPelo(t *testing.T) {
	uso := regexp.MustCompile(`\bSinMovimiento\b`)
	asignacion := regexp.MustCompile(`\bSinMovimiento\s*=[^=]`)
	permitidas := []string{"var SinMovimiento bool", "return SinMovimiento || sistema.MenosMovimiento()"}
	raiz := ".." // el módulo apps/fyne
	vistos := 0
	err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n != ".." && (strings.HasPrefix(n, ".") || n == "node_modules" || n == "upstream" || n == "patches") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
			return nil
		}
		vistos++
		f, err := os.Open(ruta) //nolint:gosec // G304: código del propio módulo
		if err != nil {
			return err
		}
		defer f.Close()
		lector := bufio.NewScanner(f)
		lector.Buffer(make([]byte, 0, 1<<16), 1<<22)
		for num := 1; lector.Scan(); num++ {
			linea := strings.TrimSpace(lector.Text())
			if strings.HasPrefix(linea, "//") || !uso.MatchString(linea) {
				continue
			}
			if codigo, _, _ := strings.Cut(linea, "//"); asignacion.MatchString(codigo) ||
				strings.Contains(codigo, permitidas[0]) || strings.Contains(codigo, permitidas[1]) {
				continue
			}
			t.Errorf("%s:%d lee SinMovimiento a pelo; usa MenosMovimiento(): %s", ruta, num, linea)
		}
		return lector.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistos < 100 {
		t.Fatalf("solo se han recorrido %d ficheros: la búsqueda no mira el módulo", vistos)
	}
}
