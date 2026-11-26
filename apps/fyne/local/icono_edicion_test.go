// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// Lo que mide el icono de las apps de Fyne: el `Icon.png` del que `fyne package` saca el
// .icns de macOS (hasta 1024), el .ico de Windows, el PNG del `.desktop`, el mipmap de
// Android y el catálogo de iOS (con el 1024 de la tienda). Y el círculo seguro de la
// maskable (`scripts/gen-marks.sh`): r = 40 % del lado.
const (
	ladoIcono      = 1024
	radioSeguroIco = 0.40 * ladoIcono
)

// insignia son las medidas de la marca de la edición, en píxeles del icono de 1024.
type insignia struct{ x, radio, aro, hueco int }

// reInsignia lee las medidas de la insignia de `scripts/gen-iconos-fyne.sh`, que es su
// única fuente.
var reInsignia = regexp.MustCompile(`(?m)^INSIGNIA_(X|RADIO|ARO|HUECO)=(\d+)\b`)

func insigniaDelGuion(t *testing.T) insignia {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "gen-iconos-fyne.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var m insignia
	vistos := 0
	for _, c := range reInsignia.FindAllStringSubmatch(string(crudo), -1) {
		v, _ := strconv.Atoi(c[2])
		switch c[1] {
		case "X":
			m.x = v
		case "RADIO":
			m.radio = v
		case "ARO":
			m.aro = v
		case "HUECO":
			m.hueco = v
		}
		vistos++
	}
	if vistos != 4 {
		t.Fatalf("scripts/gen-iconos-fyne.sh no declara las cuatro medidas de la insignia (INSIGNIA_X, _RADIO, _ARO, _HUECO): %d", vistos)
	}
	return m
}

// TestElIconoDeLaEdicionLlevaSuMarca (PLAN-EDICION-LOCAL §5.2, R-CONVIVE): el icono de
// cada app de Sytue Office es el de su producto con la marca de la edición
// (`scripts/gen-iconos-fyne.sh office`), así que en el Dock, la barra de tareas o el
// lanzador no se confunde con el de la conectada instalada al lado.
//
//   - Existe en el tamaño del que salen todos los demás: 1024 × 1024, opaco (iOS no
//     admite transparencia) y cuadrado; `FyneApp.toml` lo nombra y `empaquetar.sh` lo
//     empaqueta tal cual (`fyne package` en el directorio de la app).
//   - NO es el de la conectada, y lo que cambia es la insignia: FUERA del círculo seguro
//     (r = 40 %) es el de la conectada píxel a píxel, así que la máscara de los iconos
//     adaptativos de Android y el escudo de iOS y de macOS no cortan la marca; y la
//     insignia es la misma en las quince (el negro de la casa con su aro blanco y la
//     pantalla dentro), con 3:1 o más entre su aro y la tesela de alrededor.
//   - Se ve pequeño: reducido a 16, 32 y 64 px, la insignia sigue siendo un punto
//     claramente más oscuro que lo que hay ahí en el de la conectada (tres píxeles a 16).
//
// VISTO EN ROJO (2026-11-25): con los iconos de antes (copias de los de `cmd`), dijo de
// las quince que eran el de la conectada; con la insignia movida a 760 y regenerada, que
// se salía del círculo seguro (por la geometría del guion, a 490,7, y por los píxeles de
// las quince); con su radio a 40, que a 16 px no se distinguía en once de ellas.
func TestElIconoDeLaEdicionLlevaSuMarca(t *testing.T) {
	m := insigniaDelGuion(t)
	// La geometría, antes que los píxeles: su borde más lejano (el del hueco, que borra
	// el glifo) dentro del círculo seguro, con dos píxeles de suavizado.
	centro := float64(ladoIcono) / 2
	if lejos := math.Sqrt2*(float64(m.x)-centro) + float64(m.radio+m.hueco); lejos > radioSeguroIco-2 {
		t.Errorf("la insignia llega a %.1f del centro del icono; el círculo seguro acaba en %.1f", lejos, radioSeguroIco)
	}
	ids := identidades(t, "cmd-local")
	apps := make([]string, 0, len(appsLocales))
	for app := range appsLocales {
		apps = append(apps, app)
	}
	slices.Sort(apps)

	sinConectada := sinLaConectada()
	if sinConectada {
		// En sytue-office: la insignia de la edición, sí; el cruce píxel a píxel con el
		// icono de la conectada, en sytue-one (como TestLaIdentidadLocalNoChocaConLaConectada).
		t.Log("sin apps/fyne/cmd (el árbol público): la insignia de la edición, sin cruzarla con el icono de la conectada")
	}

	var modelo image.RGBA64Image // la insignia de la primera app, para comparar las demás
	var deModelo string
	for _, app := range apps {
		if ids[app].icono != "Icon.png" {
			t.Errorf("cmd-local/%s: FyneApp.toml nombra el icono %q; es Icon.png, a su lado", app, ids[app].icono)
			continue
		}
		local, err := leerIcono(filepath.Join("..", "cmd-local", app, "Icon.png"))
		if err != nil {
			t.Errorf("cmd-local/%s: %v", app, err)
			continue
		}
		if b := local.Bounds(); b != image.Rect(0, 0, ladoIcono, ladoIcono) {
			t.Errorf("cmd-local/%s: el icono mide %v; quiero %d × %d, el tamaño del que salen los demás", app, b.Size(), ladoIcono, ladoIcono)
			continue
		}
		if o, ok := local.(interface{ Opaque() bool }); !ok || !o.Opaque() {
			t.Errorf("cmd-local/%s: el icono no es opaco (iOS no admite transparencia)", app)
		}
		var conectada image.RGBA64Image
		if !sinConectada {
			if conectada, err = leerIcono(filepath.Join("..", "cmd", app, "Icon.png")); err != nil {
				t.Errorf("cmd/%s: %v", app, err)
				continue
			}
			if !llevaLaMarcaSobreLaConectada(t, app, local, conectada, m) {
				continue
			}
		}

		// La insignia es la misma en todas: el disco entero, sin el último píxel del aro.
		if modelo == nil {
			modelo, deModelo = local, app
		} else if p, ok := mismoDisco(local, modelo, m.x, m.radio-1); !ok {
			t.Errorf("cmd-local/%s: la insignia no es la de %s (en %v): la marca de la edición es una sola", app, deModelo, p)
		}
		// El negro de la casa por dentro, el aro blanco y su contraste con la tesela, que
		// asoma por el hueco de encima.
		negro := local.RGBA64At(m.x-m.radio+m.aro+10, m.x)
		aro := local.RGBA64At(m.x, m.x-m.radio+m.aro/2)
		tesela := local.RGBA64At(m.x, m.x-m.radio-m.hueco/2)
		if luminancia(negro) > 0.01 || luminancia(aro) < 0.85 {
			t.Errorf("cmd-local/%s: la insignia es %v por dentro y %v en su aro; quiero el negro de la casa (#0a0a0a) y blanco", app, negro, aro)
		}
		if c := contraste(aro, tesela); c < 3 {
			t.Errorf("cmd-local/%s: el aro de la insignia contra la tesela (%v) da %.2f:1; quiero 3:1 o más", app, tesela, c)
		}

		// Y se ve pequeño: reducido, la insignia sigue siendo un punto oscuro de tres
		// píxeles a 16 px (y en proporción, más grande).
		for _, n := range []int{16, 32, 64} {
			if conectada == nil {
				break
			}
			oscuros := oscurecidos(local, conectada, n, m)
			if quiero := 3 * (n / 16) * (n / 16); oscuros < quiero {
				t.Errorf("cmd-local/%s: a %d px la insignia oscurece %d píxeles; quiero %d o más: no se distingue del de la conectada", app, n, oscuros, quiero)
			}
		}
	}
}

// llevaLaMarcaSobreLaConectada compara el icono de la edición con el de su producto en la
// conectada: lo que cambia es bastante y está dentro del círculo seguro. false si no
// tiene sentido seguir mirando esa app.
func llevaLaMarcaSobreLaConectada(t *testing.T, app string, local, conectada image.RGBA64Image, m insignia) bool {
	t.Helper()
	centro := float64(ladoIcono) / 2
	if conectada.Bounds() != local.Bounds() {
		t.Errorf("cmd/%s: el icono de la conectada mide %v", app, conectada.Bounds().Size())
		return false
	}

	// Lo que cambia respecto a la conectada: dentro del círculo seguro, y bastante.
	distintos, fuera := 0, 0
	var primero image.Point
	for y := range ladoIcono {
		for x := range ladoIcono {
			d := difRGB(local.RGBA64At(x, y), conectada.RGBA64At(x, y))
			if d > 16 {
				distintos++
			}
			if d > 2 && math.Hypot(float64(x)+0.5-centro, float64(y)+0.5-centro) > radioSeguroIco {
				if fuera == 0 {
					primero = image.Pt(x, y)
				}
				fuera++
			}
		}
	}
	interior := float64(m.radio - m.aro)
	if float64(distintos) < math.Pi*interior*interior/2 {
		t.Errorf("cmd-local/%s: el icono es el de la conectada (%d píxeles distintos): le falta la marca de la edición.\n"+
			"  Se regenera desde la raíz con scripts/gen-iconos-fyne.sh office", app, distintos)
		return false
	}
	if fuera > 0 {
		t.Errorf("cmd-local/%s: %d píxeles distintos de la conectada fuera del círculo seguro (el primero en %v): la máscara del sistema cortaría la marca", app, fuera, primero)
	}
	return true
}

func leerIcono(ruta string) (image.RGBA64Image, error) {
	img, err := leerPNG(ruta)
	if err != nil {
		return nil, err
	}
	r, ok := img.(image.RGBA64Image)
	if !ok {
		return nil, os.ErrInvalid
	}
	return r, nil
}

// difRGB es la mayor diferencia entre dos colores en un canal, en la escala de 8 bits.
func difRGB(a, b interface{ RGBA() (r, g, b, a uint32) }) int {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) int { return max(int(x>>8)-int(y>>8), int(y>>8)-int(x>>8)) }
	return max(d(ar, br), d(ag, bg), d(ab, bb))
}

// mismoDisco dice si dos iconos son iguales dentro del círculo de radio r en (c, c).
func mismoDisco(a, b image.RGBA64Image, c, r int) (image.Point, bool) {
	for y := c - r; y <= c+r; y++ {
		for x := c - r; x <= c+r; x++ {
			if (x-c)*(x-c)+(y-c)*(y-c) <= r*r && difRGB(a.RGBA64At(x, y), b.RGBA64At(x, y)) > 2 {
				return image.Pt(x, y), false
			}
		}
	}
	return image.Point{}, true
}

// luminancia es la luminancia relativa de WCAG.
func luminancia(c interface{ RGBA() (r, g, b, a uint32) }) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contraste es la razón de contraste de WCAG entre dos colores.
func contraste(a, b interface{ RGBA() (r, g, b, a uint32) }) float64 {
	la, lb := luminancia(a), luminancia(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// oscurecidos reduce los dos iconos a n × n (la media de cada bloque, como hace un
// sistema al pintarlo pequeño) y cuenta los píxeles de alrededor de la insignia que en el
// de la edición son claramente más oscuros que en el de la conectada: 10 o más de
// claridad (la L* de CIELAB, que mide lo que el ojo ve; en luminancia, el negro sobre una
// tesela oscura como la de Cast apenas resta 0,08).
func oscurecidos(local, conectada image.RGBA64Image, n int, m insignia) int {
	bloque := ladoIcono / n
	media := func(img image.RGBA64Image, bx, by int) float64 {
		var r, g, b float64
		for y := by * bloque; y < (by+1)*bloque; y++ {
			for x := bx * bloque; x < (bx+1)*bloque; x++ {
				c := img.RGBA64At(x, y)
				r, g, b = r+float64(c.R), g+float64(c.G), b+float64(c.B)
			}
		}
		k := float64(bloque * bloque)
		return claridad(luminancia(rgb64{uint32(r / k), uint32(g / k), uint32(b / k)}))
	}
	oscuros := 0
	desde, hasta := (m.x-m.radio)/bloque, min((m.x+m.radio)/bloque, n-1)
	for by := desde; by <= hasta; by++ {
		for bx := desde; bx <= hasta; bx++ {
			if media(conectada, bx, by)-media(local, bx, by) >= 10 {
				oscuros++
			}
		}
	}
	return oscuros
}

// claridad es la L* de CIELAB (0 a 100) de una luminancia relativa.
func claridad(y float64) float64 {
	if y <= 216.0/24389 {
		return y * 24389 / 27
	}
	return 116*math.Cbrt(y) - 16
}

type rgb64 struct{ r, g, b uint32 }

func (c rgb64) RGBA() (r, g, b, a uint32) { return c.r, c.g, c.b, 0xffff }
