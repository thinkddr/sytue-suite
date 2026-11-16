package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// botonCon busca en el árbol de la pantalla el botón con ese rótulo, también dentro de
// la acción de un estado vacío.
func botonCon(o fyne.CanvasObject, rotulo string) *ui.Boton {
	switch x := o.(type) {
	case *ui.Boton:
		if x.Texto == rotulo {
			return x
		}
	case *ui.Vacio:
		if x.Accion != nil {
			return botonCon(x.Accion, rotulo)
		}
	case *fyne.Container:
		for _, h := range x.Objects {
			if b := botonCon(h, rotulo); b != nil {
				return b
			}
		}
	case *container.Scroll:
		return botonCon(x.Content, rotulo)
	}
	return nil
}

// vacioDe es el primer estado vacío del árbol de la pantalla.
func vacioDe(o fyne.CanvasObject) *ui.Vacio {
	switch x := o.(type) {
	case *ui.Vacio:
		return x
	case *fyne.Container:
		for _, h := range x.Objects {
			if e := vacioDe(h); e != nil {
				return e
			}
		}
	case *container.Scroll:
		return vacioDe(x.Content)
	}
	return nil
}

// TestElFalloOfreceReintentarDondeSirve: el servicio caído y la red ida ofrecen
// «Reintentar»; sin permiso no (volver a pedir da el mismo 403), y una pantalla que no
// sabe recargarse no enseña un botón muerto. VISTO EN ROJO sin `AlReintentar`.
func TestElFalloOfreceReintentarDondeSirve(t *testing.T) {
	v := vistaDePrueba(t, nil)
	reintentos := 0
	v.AlReintentar = func() { reintentos++ }
	reintentar := v.Traduce("action.retry")
	for _, e := range []Estado{Error, SinConexion} {
		w := v
		w.Estado = e
		b := botonCon(CuerpoDeEstado(w, nil), reintentar)
		if b == nil {
			t.Fatalf("el estado %d no ofrece «%s»", e, reintentar)
		}
		test.Tap(b)
	}
	if reintentos != 2 {
		t.Errorf("«Reintentar» se pulsó dos veces y reintentó %d", reintentos)
	}
	w := v
	w.Estado = SinPermiso
	if botonCon(CuerpoDeEstado(w, nil), reintentar) != nil {
		t.Error("sin permiso se ofrece «Reintentar», y volver a pedir da el mismo 403")
	}
	w = vistaDePrueba(t, nil)
	w.Estado = Error
	if botonCon(CuerpoDeEstado(w, nil), reintentar) != nil {
		t.Error("sin `AlReintentar` se pinta un «Reintentar» que no hace nada")
	}
}

// TestLaGaleriaCaidaSeRecuperaConReintentar: con el servicio contestando 500, la
// galería en alemán dice `error.connection` en alemán con «Reintentar», y al pulsarlo
// vuelve a pedir la línea de tiempo y enseña las fotos.
func TestLaGaleriaCaidaSeRecuperaConReintentar(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100, lineaCaida: 1}
	a, _ := appDePrueba(t, s)
	de, err := Traductor("de")
	if err != nil {
		t.Fatal(err)
	}
	a.v.T = de
	a.Arrancar()
	if a.g.fallo == nil || estadoDeFallo(a.g.fallo) != Error {
		t.Fatalf("con un 500 la galería queda con fallo %v", a.g.fallo)
	}
	if e := vacioDe(a.scroll.Content); e == nil || e.Titulo != de.T("error.connection", nil) {
		t.Fatalf("la galería caída no dice «%s»: %+v", de.T("error.connection", nil), e)
	}
	b := botonCon(a.scroll.Content, de.T("action.retry", nil))
	if b == nil {
		t.Fatalf("la galería caída no ofrece «%s»", de.T("action.retry", nil))
	}
	test.Tap(b)
	if a.g.fallo != nil || len(a.g.fotos) != 3 {
		t.Errorf("tras reintentar: fallo %v y %d fotos (quiero 3)", a.g.fallo, len(a.g.fotos))
	}
}
