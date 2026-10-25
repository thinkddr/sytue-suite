// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recorrido

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// `SYTUE_CAPTURAS_TEMA=oscuro` saca el recorrido en oscuro. Se elige al cargar el paquete
// —antes de que el TestMain de la app construya nada—, que es cuando lo elige la app de
// verdad (`sesion.Suite` lo lee de los ajustes al arrancar): así se retrata lo que ve
// quien abre la app en oscuro, no un repintado a mitad.
func init() {
	if os.Getenv("SYTUE_CAPTURAS_TEMA") == "oscuro" {
		tema.Elegir("oscuro")
	}
}

// Las capturas de teléfono de Google Play: 360 × 640 a ×3 son los 1080 × 1920 que pide
// la ficha, y por debajo de 768 de ancho la app se pinta como en el teléfono.
const anchoPlay, altoPlay, escalaPlay = 360, 640, 3

// numeroPlay numera las capturas de todo el paquete: Docus y Calcus recorren dos
// ventanas en el mismo test y no deben pisarse.
var numeroPlay int

// Retratar es Comprobar con `SYTUE_CAPTURAS_PLAY=<dir>`: en vez de comprobar, pone la
// ventana a teléfono y deja en dir una captura de cada pantalla distinta a la que se llega
// pulsando —y de cada capa que se abre, antes de cerrarla—, con lo pulsado en el
// nombre. Las de la ficha se eligen de ahí (apps/fyne/play/README.md).
func Retratar(t testing.TB, w fyne.Window, dir string, tope int) Informe {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // la ruta la pone quien lanza el test
		t.Fatal(err)
	}
	AMedida(w)
	c := w.Canvas()
	vistas := map[[32]byte]bool{}
	guardar := func(que string) {
		b, err := imagenPNG(c.Capture())
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if vistas[h] {
			return
		}
		vistas[h] = true
		numeroPlay++
		ruta := filepath.Join(dir, fmt.Sprintf("%03d-%s.png", numeroPlay, limpiar(que)))
		// La ruta es la de SYTUE_CAPTURAS_PLAY, que pone quien lanza el test.
		if err := os.WriteFile(ruta, b, 0o600); err != nil { //nolint:gosec // G703: ver arriba
			t.Fatal(err)
		}
		// Y al lado, lo que se ha pintado y de qué color: para medir el color por pieza.
		if os.Getenv("SYTUE_CAPTURAS_VOLCADO") != "" {
			if err := Volcar(c, strings.TrimSuffix(ruta, ".png")+".json"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Lo que carga en segundo plano (miniaturas, la primera página) llega por la red
	// del servicio falso: se le da un momento antes de la primera captura.
	esperar(time.Second)
	guardar("inicio")
	if os.Getenv("SYTUE_CAPTURAS_VOLCADO") != "" {
		numeroPlay++
		if err := VolcarEncima(c, filepath.Join(dir, fmt.Sprintf("%03d-encima.hover", numeroPlay))); err != nil {
			t.Fatal(err)
		}
	}
	// Y la misma pantalla desplazada, que en un teléfono es media app.
	if sc := primerDesplazable(c.Content()); sc != nil {
		sc.ScrollToOffset(fyne.NewPos(0, sc.Content.MinSize().Height/2))
		esperar(0)
		guardar("inicio-desplazada")
		sc.ScrollToTop()
	}
	var inf Informe
	vistos := map[string]bool{}
	for inf.Pulsados < tope {
		o, tp, clave := siguiente(c.Content(), vistos)
		if o == nil {
			break
		}
		vistos[clave] = true
		inf.Pulsados++
		antes := len(c.Overlays().List())
		que := describir(o)
		if pulsar(tp) != "" {
			quitarEncima(c, antes)
			continue
		}
		esperar(200 * time.Millisecond)
		guardar(que)
		if len(c.Overlays().List()) > antes {
			escape(c)
			quitarEncima(c, antes)
		}
	}
	return inf
}

func imagenPNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// AMedida pone la ventana al tamaño y la escala de las capturas: el teléfono de Google
// Play, o el «ancho» de `SYTUE_CAPTURAS_ANCHO`. Retratar empieza por aquí; una app cuyas
// imágenes llegan en segundo plano la llama antes, para tenerlas ya a esa medida.
func AMedida(w fyne.Window) {
	ancho, alto, escala := float32(anchoPlay), float32(altoPlay), float32(escalaPlay)
	if os.Getenv("SYTUE_CAPTURAS_ANCHO") != "" {
		// El «ancho» de `test/e2e/medidas-web.mjs`: 1280 × 800 a ×1, para poner cada
		// captura al lado de la de la web.
		ancho, alto, escala = 1280, 800, 1
	}
	// `SYTUE_CAPTURAS_MEDIDA=ANCHOxALTO@ESCALA` manda sobre las dos: son las medidas de la
	// App Store (apps/fyne/appstore/README.md), p. ej. `393x852@3` para el iPhone.
	if m := os.Getenv("SYTUE_CAPTURAS_MEDIDA"); m != "" {
		if n, err := fmt.Sscanf(m, "%fx%f@%f", &ancho, &alto, &escala); n != 3 || err != nil {
			panic("SYTUE_CAPTURAS_MEDIDA tiene que ser ANCHOxALTO@ESCALA y es «" + m + "»")
		}
	}
	if c, ok := w.Canvas().(interface{ SetScale(float32) }); ok {
		c.SetScale(escala)
	}
	w.Resize(fyne.NewSize(ancho, alto))
	fyne.DoAndWait(func() {})
}

func esperar(d time.Duration) {
	time.Sleep(d)
	fyne.DoAndWait(func() {})
}

// primerDesplazable es el primer Scroll vertical cuyo contenido no cabe.
func primerDesplazable(o fyne.CanvasObject) *container.Scroll {
	if o == nil || !o.Visible() {
		return nil
	}
	if sc, es := o.(*container.Scroll); es && sc.Direction != container.ScrollHorizontalOnly &&
		sc.Content != nil && sc.Content.MinSize().Height > sc.Size().Height*1.3 {
		return sc
	}
	for _, h := range hijos(o) {
		if sc := primerDesplazable(h); sc != nil {
			return sc
		}
	}
	return nil
}

// limpiar deja el texto pulsado como nombre de fichero.
func limpiar(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == ' ' || r == '*' || r == '«' || r == '»' || r == '.' {
			return '_'
		}
		return r
	}, s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
