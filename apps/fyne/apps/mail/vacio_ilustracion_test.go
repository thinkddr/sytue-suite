// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"image"
	"image/draw"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// vacioVisible es el estado vacío que se ve dentro de `o`, o nil.
func vacioVisible(o fyne.CanvasObject) *ui.Vacio {
	if !o.Visible() {
		return nil
	}
	if v, ok := o.(*ui.Vacio); ok {
		return v
	}
	for _, h := range hijos(o) {
		if v := vacioVisible(h); v != nil {
			return v
		}
	}
	return nil
}

// pixeles rasteriza un spot a la medida de la web (160 × 128) y lo deja en NRGBA, que
// es lo que se compara: dos ilustraciones son la misma si pintan lo mismo.
func pixeles(t *testing.T, o fyne.CanvasObject) []byte {
	t.Helper()
	r, ok := o.(*canvas.Raster)
	if !ok {
		t.Fatalf("la ilustración es %T y tenía que ser el ráster de un spot", o)
	}
	img := r.Generator(160, 128)
	dst := image.NewNRGBA(img.Bounds())
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	return dst.Pix
}

func spot(t *testing.T, i iconos.Ilustracion, v tema.Variante) []byte {
	t.Helper()
	return pixeles(t, iconos.Pintar(i, v).Objeto())
}

// comprobarSpot dice si el estado vacío visible de `o` lleva el spot `quiere` (o
// ninguno, con `hay` a false), al ancho de la web.
func comprobarSpot(t *testing.T, caso string, o fyne.CanvasObject, quiere marcaVacio) {
	t.Helper()
	x := vacioVisible(o)
	if x == nil {
		t.Fatalf("%s: no se ve ningún estado vacío", caso)
	}
	if !quiere.hay {
		if x.Ilustracion != nil {
			t.Errorf("%s: lleva ilustración y la web no pinta ninguna", caso)
		}
		return
	}
	if x.Ilustracion == nil {
		t.Fatalf("%s: sin ilustración; la web pinta `%s`", caso, quiere.ilus)
	}
	if w := x.Ilustracion.Size().Width; w != AnchoIlustracionVacio {
		t.Errorf("%s: la ilustración mide %v de ancho y la web %v", caso, w, AnchoIlustracionVacio)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !bytes.Equal(pixeles(t, x.Ilustracion), spot(t, quiere.ilus, v)) {
		t.Errorf("%s: la ilustración no es `%s` en la variante %v", caso, quiere.ilus, v)
	}
}

// TestLosVaciosDeLaBandejaLlevanSuIlustracion: cada estado de fin de la lista lleva el
// mismo spot que `MessageList.tsx` —`errorRetry` en el error, `emptyMail` en los tres
// vacíos— y «sin permiso», que no tiene equivalente, ninguno.
func TestLosVaciosDeLaBandejaLlevanSuIlustracion(t *testing.T) {
	tr := traductor(t)
	casos := []struct {
		nombre string
		prep   func(*Bandeja)
		quiere marcaVacio
	}{
		{"carpeta vacía", func(b *Bandeja) { b.Estado = Vacio }, conIlustracion(iconos.VacioMail)},
		{"búsqueda sin resultados", func(b *Bandeja) {
			b.Estado, b.Buscando, b.Consulta = Vacio, true, "factura"
		}, conIlustracion(iconos.VacioMail)},
		{"filtro sin resultados", func(b *Bandeja) {
			b.Estado, b.Filtro, b.AlFiltrar = Vacio, FiltroNoLeidas, func(FiltroRapido) {}
		}, conIlustracion(iconos.VacioMail)},
		{"error", func(b *Bandeja) { b.Estado = Error }, conIlustracion(iconos.ErrorReintentar)},
		{"sin conexión", func(b *Bandeja) { b.Estado = SinConexion }, conIlustracion(iconos.ErrorReintentar)},
		{"sin permiso", func(b *Bandeja) { b.Estado = SinPermiso }, marcaVacio{}},
	}
	for _, c := range casos {
		b := NuevaBandeja(tr, "es")
		b.Ahora = reloj()
		b.Titulo = "Recibidos"
		c.prep(b)
		w := test.NewWindow(b)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
		w.Canvas().Capture()
		comprobarSpot(t, c.nombre, b, c.quiere)
		w.Close()
	}
}

// TestElLectorVacioLlevaSuIlustracion: «elige una conversación» es el `emptyMail` de
// `MessageReader.tsx`; el error del lector, el mismo `errorRetry` que la lista; y
// «cargando el mensaje…» no lleva dibujo.
func TestElLectorVacioLlevaSuIlustracion(t *testing.T) {
	tr := traductor(t)
	casos := []struct {
		nombre string
		prep   func(*Lector)
		quiere marcaVacio
	}{
		{"sin hilo", func(x *Lector) { x.Estado = Vacio }, conIlustracion(iconos.VacioMail)},
		{"recién abierto", func(x *Lector) { x.Estado = Cargando }, conIlustracion(iconos.VacioMail)},
		{"error", func(x *Lector) { x.Estado = Error }, conIlustracion(iconos.ErrorReintentar)},
		{"cargando un hilo", func(x *Lector) {
			x.Estado = Cargando
			x.Conversacion = ejemplo()[0]
		}, marcaVacio{}},
	}
	for _, c := range casos {
		x := NuevoLector(tr, "es")
		x.AlRedactar = func() {}
		c.prep(x)
		w := test.NewWindow(x)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(740, 744))
		w.Canvas().Capture()
		comprobarSpot(t, c.nombre, x, c.quiere)
		w.Close()
	}
}

// TestLaIlustracionDelVacioSigueAlTema: `iconos.Pintar` resuelve los tokens al
// construir, así que si el estado no se rehace al cambiar de tema el sobre se queda en
// los colores de la variante con la que nació.
func TestLaIlustracionDelVacioSigueAlTema(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	tema.Elegir("claro")
	tr := traductor(t)
	b := NuevaBandeja(tr, "es")
	b.Ahora = reloj()
	b.Titulo = "Recibidos"
	b.Estado = Vacio
	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	w.Canvas().Capture()
	claro := vacioVisible(b)
	if claro == nil || claro.Ilustracion == nil {
		t.Fatal("la bandeja vacía no lleva ilustración")
	}
	if bytes.Equal(spot(t, iconos.VacioMail, tema.Claro), spot(t, iconos.VacioMail, tema.Oscuro)) {
		t.Fatal("el spot pinta igual en claro y en oscuro: esta prueba no distingue nada")
	}

	tema.Elegir("oscuro")
	b.Refresh()
	comprobarSpot(t, "tras pasar a oscuro", b, conIlustracion(iconos.VacioMail))

	// Y sin cambio de tema, refrescar no rehace el estado: el mismo objeto sigue colgado.
	antes := vacioVisible(b)
	b.Refresh()
	if vacioVisible(b) != antes {
		t.Error("un refresco sin cambios rehace el estado vacío: parpadea en cada repintado")
	}
}

// TestSinSitioElVacioVaSinIlustracion: en un móvil apaisado el spot de 128 empujaba
// «Redactar» fuera de la pantalla. Sin sitio, el dibujo se esconde y el botón se queda
// dentro; con sitio, vuelve.
func TestSinSitioElVacioVaSinIlustracion(t *testing.T) {
	tr := traductor(t)
	x := NuevoLector(tr, "es")
	x.Estado = Vacio
	x.AlRedactar = func() {}
	w := test.NewWindow(x)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(500, 300))
	w.Canvas().Capture()
	v := vacioVisible(x)
	if v == nil || v.Ilustracion == nil || v.Accion == nil {
		t.Fatal("el lector vacío no tiene ilustración y botón")
	}
	if v.Ilustracion.Visible() {
		t.Error("a 300 de alto la ilustración sigue a la vista y no cabe con el botón")
	}
	if fondo := v.Position().Y + v.Accion.Position().Y + v.Accion.Size().Height; fondo > 300 {
		t.Errorf("el botón acaba en y = %v, fuera de los 300 del lector", fondo)
	}

	w.Resize(fyne.NewSize(740, 744))
	w.Canvas().Capture()
	if !vacioVisible(x).Ilustracion.Visible() {
		t.Error("con sitio de sobra la ilustración no vuelve")
	}
}
