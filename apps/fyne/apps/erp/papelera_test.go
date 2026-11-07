// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// papeleraConServicio monta la pantalla con un servicio que apunta los cuerpos del lote
// y contesta `estadoLote` al de `restaurar`.
func papeleraConServicio(t *testing.T, estadoLote int) (*Pantalla, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var lotes []string
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/lote"):
			cuerpo, _ := io.ReadAll(r.Body)
			mu.Lock()
			lotes = append(lotes, string(cuerpo))
			mu.Unlock()
			if strings.Contains(string(cuerpo), "restaurar") && estadoLote != http.StatusOK {
				w.WriteHeader(estadoLote)
				_, _ = io.WriteString(w, `{"error":{"code":"sin_permiso","message":"no"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"afectados":2}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	p.Abrir(facturas())
	return p, &lotes
}

// ultimoAviso es el aviso más reciente de la pila.
func ultimoAviso(t *testing.T, p *Pantalla) *ui.Aviso {
	t.Helper()
	as := p.avisos.Avisos()
	if len(as) == 0 {
		t.Fatal("no hay avisos")
	}
	return as[len(as)-1]
}

// TestElLoteALaPapeleraSeDeshaceDesdeElAviso: mandar la selección a la papelera deja
// «Movido a la papelera» con «Deshacer» durante 10 s, que restaura el lote ENTERO de
// una vez y lo dice. Es `avisarPapelera` de lib/papelera.ts.
//
// VISTO EN ROJO: con el aviso de siempre (sin RotuloAccion) no hay «Deshacer»; con
// "borrar" en vez de "restaurar", el segundo lote vuelve a tirarlos.
func TestElLoteALaPapeleraSeDeshaceDesdeElAviso(t *testing.T) {
	p, lotes := papeleraConServicio(t, http.StatusOK)
	p.aplicarLote("borrar", "", []string{"a", "b"})
	av := ultimoAviso(t, p)
	if av.Titulo != "Movido a la papelera" || av.RotuloAccion != "Deshacer" || av.Duracion != ui.DuracionDeshacer {
		t.Fatalf("aviso: %q, acción %q, %v", av.Titulo, av.RotuloAccion, av.Duracion)
	}
	av.AlPulsarAccion()
	quiere := []string{`{"accion":"borrar","ids":["a","b"]}`, `{"accion":"restaurar","ids":["a","b"]}`}
	if strings.Join(*lotes, "\n") != strings.Join(quiere, "\n") {
		t.Errorf("lotes:\n%s\nquiere:\n%s", strings.Join(*lotes, "\n"), strings.Join(quiere, "\n"))
	}
	if got := ultimoAviso(t, p).Titulo; got != "Devuelto desde la papelera" {
		t.Errorf("tras deshacer: %q", got)
	}
}

// TestLaFichaALaPapeleraSeDeshaceYSiFallaLoDice: lo mismo desde la ficha, y si el
// servicio no lo devuelve el aviso es de peligro con el motivo.
func TestLaFichaALaPapeleraSeDeshaceYSiFallaLoDice(t *testing.T) {
	p, lotes := papeleraConServicio(t, http.StatusForbidden)
	p.AbrirFicha(Registro{ID: "f1", Etiqueta: "FV-1"})
	p.borrar(p.ficha)
	av := ultimoAviso(t, p)
	if av.RotuloAccion != "Deshacer" {
		t.Fatalf("el borrado de la ficha no ofrece «Deshacer»: %q", av.Titulo)
	}
	av.AlPulsarAccion()
	if len(*lotes) != 1 || (*lotes)[0] != `{"accion":"restaurar","ids":["f1"]}` {
		t.Errorf("lotes: %v", *lotes)
	}
	if got := ultimoAviso(t, p); got.Titulo != "No se pudo deshacer" || got.Tono != ui.AvisoPeligro {
		t.Errorf("si no vuelve: %q (%v)", got.Titulo, got.Tono)
	}
}

// TestLaPapeleraVaciaTieneSuPropioEstado: vacía, la papelera decía «Aquí todavía no hay
// nada · Crea el primero con Nuevo» con el botón de crear. Ahora dice que está vacía,
// para qué sirve y ofrece salir. Es `vacioDeLista` de vacio.ts.
//
// VISTO EN ROJO: con el `vacio()` de antes (solo estado y búsqueda), salen «Crea el
// primero con Nuevo» y el botón «Nuevo».
func TestLaPapeleraVaciaTieneSuPropioEstado(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Objeto()
	p.Abrir(facturas())
	l := p.lista
	l.Fase, l.Pagina, l.Papelera = Exito, Pagina{}, true
	x := l.vacio(tema.Claro)
	if x.Titulo != "La papelera está vacía" || x.Descripcion != "Lo que elimines aparecerá aquí y podrás restaurarlo." {
		t.Errorf("papelera vacía: %q · %q", x.Titulo, x.Descripcion)
	}
	if b, ok := x.Accion.(*ui.Boton); !ok || !contiene(textosDe(b), p.t("action.resetView")) {
		t.Error("la acción de la papelera vacía es salir de ella («Restablecer vista»), no «Nuevo»")
	}
	if !contiene(textosDe(l.Objeto(tema.Claro)), "La papelera está vacía") {
		t.Error("la lista no pinta el estado de la papelera")
	}
	if d := vacioDeLista(false, "", ""); d.titulo != "list.empty" || d.limpiar || d.papelera {
		t.Errorf("fuera de la papelera, lo de siempre: %+v", d)
	}
}

// TestDeshacerElBorradoNoCierraLaFichaQueHayaAbierta: entre borrar y pulsar «Deshacer»
// se puede empezar un alta. Deshacer devuelve el registro y refresca la lista, pero no
// cierra ese formulario ni tira lo escrito.
//
// VISTO EN ROJO: con `p.escrito(m)` como alDeshacer (lo de antes), `p.ficha` queda a nil.
func TestDeshacerElBorradoNoCierraLaFichaQueHayaAbierta(t *testing.T) {
	p, _ := papeleraConServicio(t, http.StatusOK)
	p.AbrirFicha(Registro{ID: "f1", Etiqueta: "FV-1"})
	p.borrar(p.ficha)
	av := ultimoAviso(t, p)
	p.NuevaFicha(nil)
	nueva := p.ficha
	if nueva == nil {
		t.Fatal("no se abrió el alta")
	}
	av.AlPulsarAccion()
	if p.ficha != nueva {
		t.Error("«Deshacer» cerró el alta que se estaba escribiendo")
	}
}

// TestEnLaPapeleraElLoteDevuelveYNoTira: dentro de la papelera la barra de lote no
// ofrece «Mover a papelera» ni «Cambiar estado», sino «Restaurar», que manda
// `restaurar`; y su aviso dice «Restaurado» SIN «Deshacer». Antes se tiraba lo ya
// tirado (no hacía nada) y el «Deshacer» del aviso sacaba de la papelera lo que ya
// estaba en ella. Es `aplicarLote` de lib/papelera.ts.
//
// VISTO EN ROJO: sin el `if l.Papelera` de barraLote, la barra lleva «Mover a
// papelera» y pulsarla pide `borrar`; con `avisarPapelera` también para `restaurar`,
// el aviso sale con «Deshacer».
func TestEnLaPapeleraElLoteDevuelveYNoTira(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Pagina, l.Ventana, l.Fase = facturas(), paginaEjemplo(), 1280, Exito
	l.Papelera, l.Marcadas = true, map[string]bool{"1": true, "2": true}
	var pedido []string
	l.AlLote = func(accion, estado string) { pedido = append(pedido, accion+"|"+estado) }
	barra, _ := l.barraLote(tema.Claro, anchoContenido)
	textos := textosDe(barra)
	if contiene(textos, "Mover a papelera") || contiene(textos, "Cambiar estado") {
		t.Fatalf("en la papelera la barra ofrece tirar o cambiar estado: %q", textos)
	}
	var restaurar *ui.Boton
	recorrer(barra, func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && contiene(textosDe(b), "Restaurar") {
			restaurar = b
		}
	})
	if restaurar == nil {
		t.Fatalf("la barra no ofrece «Restaurar»: %q", textos)
	}
	restaurar.Tapped(nil)
	if strings.Join(pedido, ",") != "restaurar|" {
		t.Fatalf("pide %q", pedido)
	}

	p, lotes := papeleraConServicio(t, http.StatusOK)
	p.aplicarLote("restaurar", "", []string{"a", "b"})
	if got := strings.Join(*lotes, "\n"); got != `{"accion":"restaurar","ids":["a","b"]}` {
		t.Errorf("lotes: %s", got)
	}
	if av := ultimoAviso(t, p); av.Titulo != "Restaurado" || av.RotuloAccion != "" {
		t.Errorf("aviso: %q con acción %q", av.Titulo, av.RotuloAccion)
	}
}
