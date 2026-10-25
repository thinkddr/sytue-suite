// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// castellano es el traductor de las medidas de producción, que se tomaron en español:
// el reparto en líneas depende del texto, y con él todo lo que va debajo.
func castellano(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Console(), catalogos.Portal())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// Vectores actualizados con Chromium remoto, rem16 y Figtree, sobre TusDatos.tsx.
// TestTusDatosMideLoQueMideLaWeb ata «Tus datos» a producción (`medidas/portal/datos.*`)
// a 1280 y a 390 de ventana, con los 56 de la topbar ya descontados; y a 360, el ancho
// de móvil de la suite, al marcado de `TusDatos.tsx` con la hoja del portal medido con
// Chromium en el frontal (el mismo arnés da a 390 los números de producción, al
// centésimo).
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): con `MargenLeyenda` a 0 —el
// `mb-1` de la leyenda— todo lo de debajo sube 7 y el test lo dice pieza a pieza.
func TestTusDatosMideLoQueMideLaWeb(t *testing.T) {
	type caja struct{ x, y, w, h float32 }
	casos := []struct {
		ventana                       float32
		titulo, copia, opcion, accion caja
		mover, migrar                 caja
		alto                          float32
	}{
		{
			ventana: 1280,
			titulo:  caja{152, 24, 976, 36},
			copia:   caja{152, 112.75, 976, 540},
			opcion:  caja{169, 301.125, 672, 67.875},
			accion:  caja{169, 536.75, 135.734375, 44},
			mover:   caja{152, 676.75, 976, 148.25},
			migrar:  caja{169, 790, 93.515625, 18},
		},
		{
			ventana: 390,
			titulo:  caja{16, 24, 358, 36},
			copia:   caja{16, 135.5, 358, 635.625},
			opcion:  caja{33, 382.125, 324, 85.75},
			accion:  caja{33, 635.625, 135.734375, 44},
			mover:   caja{16, 795.125, 358, 211.625},
			migrar:  caja{33, 971.75, 93.515625, 18},
		},
		{
			ventana: 360,
			titulo:  caja{16, 24, 328, 36},
			opcion:  caja{33, 403.25, 294, 85.75},
			accion:  caja{33, 656.75, 135.734375, 44},
		},
	}
	// Los anchos de rótulo salen a ≤0,17 del navegador (ADR 0048 §texto); la geometría
	// apilada, a la cuantización de 1/64 que el navegador acumula.
	cerca := func(a, b, tol float32) bool { return a-b <= tol && b-a <= tol }
	for _, c := range casos {
		d := NuevosDatos(castellano(t))
		r := test.WidgetRenderer(d).(*datosRenderer)
		ven := test.NewWindow(d)
		ven.SetPadded(false)
		d.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		ven.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		guardar(t, ven.Canvas().Capture(), "datos-"+map[float32]string{1280: "ancho", 390: "estrecho", 360: "movil"}[c.ventana])

		mira := func(nombre string, o fyne.CanvasObject, quiero caja, tol float32) {
			if quiero.w == 0 {
				return // no medido a ese ancho
			}
			p, s := o.Position(), o.Size()
			if !cerca(p.X, quiero.x, tol) || !cerca(p.Y, quiero.y, tol) || !cerca(s.Width, quiero.w, 0.5) || !cerca(s.Height, quiero.h, tol) {
				t.Errorf("a %v, %s está en (%v, %v) y mide %v × %v; en producción (%v, %v) y %v × %v",
					c.ventana, nombre, p.X, p.Y, s.Width, s.Height, quiero.x, quiero.y, quiero.w, quiero.h)
			}
		}
		const tol = 0.1
		mira("el título", r.titulo, c.titulo, tol)
		mira("«Descargar una copia»", r.copia.caja.Objetos()[0], c.copia, tol)
		mira("la primera opción", r.opciones[0], c.opcion, tol)
		mira("«Descargar copia»", r.accion, c.accion, tol)
		mira("«Llevarlo a otro servicio»", r.mover.caja.Objetos()[0], c.mover, tol)
		mira("«Ir a Migraciones»", r.migrar, c.migrar, tol)
		ven.Close()
	}
}

// exportadorDoble devuelve siempre la misma copia, o un fallo.
type exportadorDoble struct {
	mu      sync.Mutex
	formato api.HubFormato
	fallo   error
}

func (e *exportadorDoble) Export(_ context.Context, f api.HubFormato) (io.ReadCloser, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.formato = f
	if e.fallo != nil {
		return nil, "", e.fallo
	}
	return io.NopCloser(strings.NewReader("PK")), "sytue-one-acme-2026-09-22.zip", nil
}

// esperar a que la descarga salga de «descargando»: va en otra gorutina.
func esperarDescarga(t *testing.T, v *Vista) (EstadoDescarga, string) {
	t.Helper()
	for range 200 {
		fyne.DoAndWait(func() {}) // lo que la descarga manda pintar corre en la rutina del test
		v.mu.Lock()
		e, ruta := v.datos.Descarga, v.datos.Guardada
		v.mu.Unlock()
		if e != Descargando {
			return e, ruta
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la descarga no acaba nunca")
	return 0, ""
}

// TestLaCopiaAcabaEnLaCarpetaYSinPisarNada: el formato elegido va al Hub, el ZIP queda
// en la carpeta de descargas con el nombre que propone el servicio y una segunda copia
// NO pisa la primera. Un fallo lo dice con el texto de error de la web.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): pasando siempre `api.HubCSV` a
// `GuardarCopia`, el test dice que al Hub le llegó csv y se pidió json.
func TestLaCopiaAcabaEnLaCarpetaYSinPisarNada(t *testing.T) {
	dir := t.TempDir()
	e := &exportadorDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)).ConExportador(e)
	v.carpeta = func() (string, error) { return dir, nil }

	for i, quiero := range []string{"sytue-one-acme-2026-09-22.zip", "sytue-one-acme-2026-09-22 (1).zip"} {
		v.descargarDatos(api.HubJSON)
		estado, ruta := esperarDescarga(t, v)
		if estado != Descargada || ruta != filepath.Join(dir, quiero) {
			t.Fatalf("copia %d: estado %v en %q; tenía que quedar en %q", i+1, estado, ruta, quiero)
		}
		if _, err := os.Stat(ruta); err != nil {
			t.Fatalf("copia %d: el fichero no está: %v", i+1, err)
		}
	}
	if e.formato != api.HubJSON {
		t.Errorf("al Hub le llegó %q y se pidió json", e.formato)
	}

	e.fallo = errors.New("el Hub no contesta")
	v.descargarDatos(api.HubCSV)
	if estado, _ := esperarDescarga(t, v); estado != DescargaFallida {
		t.Errorf("con el Hub caído la descarga queda en %v", estado)
	}
}

// organizadorDoble tiene dos organizaciones y apunta el cambio.
type organizadorDoble struct {
	mu     sync.Mutex
	cambio string
	fallo  error
}

func (o *organizadorDoble) Organizaciones(context.Context) ([]api.Organizacion, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	activa := o.cambio == ""
	return []api.Organizacion{{ID: "o1", Slug: "acme", Nombre: "Acme", Activa: activa}, {ID: "o2", Slug: "globex", Nombre: "Globex", Activa: !activa}}, nil
}

func (o *organizadorDoble) CambiarOrganizacion(_ context.Context, id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.fallo != nil {
		return o.fallo
	}
	o.cambio = id
	return nil
}

// TestLaCopiaDeOtraOrganizacionEntraEnElla: con dos organizaciones sale el `fieldset`
// para elegir —antes que el de formato—, elegir la que no es la activa cambia la acción
// por el botón de entrar, y pulsarlo cambia de organización y pide la copia allí, con el
// formato elegido. Si Sytue ID dice que no, «No hemos podido completar la operación».
//
// Visto en ROJO a mano (en el frontal): con `otraOrg` devolviendo siempre "", la copia
// sale de la activa sin cambiar y el test lo dice.
func TestLaCopiaDeOtraOrganizacionEntraEnElla(t *testing.T) {
	dir := t.TempDir()
	e, o := &exportadorDoble{}, &organizadorDoble{}
	v := NuevaVista(castellano(t), nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)).ConExportador(e).ConOrganizaciones(o)
	v.carpeta = func() (string, error) { return dir, nil }
	v.Ir(ADatos)
	for range 200 {
		fyne.DoAndWait(func() {}) // lo que el fondo manda pintar corre en la rutina del test
		v.mu.Lock()
		n := len(v.datos.Orgs)
		v.mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := test.WidgetRenderer(v.datos).(*datosRenderer)
	v.datos.Resize(fyne.NewSize(1280, r.alto(1280)))
	if len(r.opcionesOrg) != 2 || r.botonDescarga() != r.accion {
		t.Fatalf("con dos organizaciones: %d opciones y acción %T", len(r.opcionesOrg), r.botonDescarga())
	}
	if r.opcionesOrg[0].Position().Y >= r.opciones[0].Position().Y {
		t.Error("el fieldset de organización va ANTES que el de formato")
	}
	r.opciones[1].Tapped(nil) // JSON
	r.opcionesOrg[1].Tapped(nil)
	if r.botonDescarga() != r.entrar {
		t.Fatal("con otra organización elegida, la acción es entrar en ella")
	}
	r.entrar.Tapped(nil)
	// Aquí la descarga empieza DESPUÉS del cambio: se espera a que acabe, no a que deje
	// de estar en curso.
	var estado EstadoDescarga
	for range 300 {
		fyne.DoAndWait(func() {})
		v.mu.Lock()
		estado = v.datos.Descarga
		v.mu.Unlock()
		if estado == Descargada || estado == DescargaFallida {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	o.mu.Lock()
	cambio := o.cambio
	o.mu.Unlock()
	e.mu.Lock()
	formato := e.formato
	e.mu.Unlock()
	if cambio != "o2" || estado != Descargada || formato != api.HubJSON {
		t.Fatalf("cambio %q, estado %v, formato %q: quiero o2, descargada y json", cambio, estado, formato)
	}

	o.mu.Lock()
	o.cambio, o.fallo = "", errors.New("403")
	o.mu.Unlock()
	v.entrarYDescargar("o2", api.HubCSV)
	for range 200 {
		fyne.DoAndWait(func() {}) // lo que el fondo manda pintar corre en la rutina del test
		v.mu.Lock()
		d := v.datos.Descarga
		v.mu.Unlock()
		if d == DescargaFallida {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("un cambio rechazado no dice que ha fallado")
}
