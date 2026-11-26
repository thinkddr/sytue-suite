// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// nubeMemoria es una cuenta de nube en memoria con rutas como id (como WebDAV), que
// gestiona carpetas y, como Google Drive, deja dos ficheros con el mismo nombre visible
// («a.txt@2» se ve como «a.txt»).
type nubeMemoria struct {
	mu       sync.Mutex
	ficheros map[string][]byte
	carpetas map[string]bool
}

func (n *nubeMemoria) Listar(_ context.Context, carpeta string) ([]nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []nube.Elemento
	for r, d := range n.ficheros {
		if path.Dir(r) == cmpRaiz(carpeta) {
			nombre, _, _ := strings.Cut(path.Base(r), "@")
			out = append(out, nube.Elemento{ID: r, Nombre: nombre, Tamano: int64(len(d))})
		}
	}
	for c := range n.carpetas {
		if path.Dir(c) == cmpRaiz(carpeta) {
			out = append(out, nube.Elemento{ID: c, Nombre: path.Base(c), Carpeta: true})
		}
	}
	nube.Ordenar(out)
	return out, nil
}

func cmpRaiz(c string) string {
	if c == "" {
		return "/"
	}
	return c
}

func (n *nubeMemoria) Descargar(_ context.Context, e nube.Elemento) ([]byte, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	d, ok := n.ficheros[e.ID]
	if !ok {
		return nil, "", errors.New("no está")
	}
	return slices.Clone(d), e.Nombre, nil
}

func (n *nubeMemoria) Subir(_ context.Context, carpeta, nombre string, datos []byte) (nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r := path.Join(cmpRaiz(carpeta), nombre)
	n.ficheros[r] = slices.Clone(datos)
	return nube.Elemento{ID: r, Nombre: nombre}, nil
}

func (n *nubeMemoria) Reemplazar(_ context.Context, e nube.Elemento, datos []byte) (nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ficheros[e.ID] = slices.Clone(datos)
	return e, nil
}

func (n *nubeMemoria) CrearCarpeta(_ context.Context, padre, nombre string) (nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := path.Join(cmpRaiz(padre), nombre)
	n.carpetas[c] = true
	return nube.Elemento{ID: c, Nombre: nombre, Carpeta: true}, nil
}

func (n *nubeMemoria) Renombrar(context.Context, nube.Elemento, string) (nube.Elemento, error) {
	return nube.Elemento{}, errors.New("no hace falta")
}

func (n *nubeMemoria) Mover(context.Context, nube.Elemento, string) (nube.Elemento, error) {
	return nube.Elemento{}, errors.New("no hace falta")
}

func (n *nubeMemoria) Borrar(_ context.Context, e nube.Elemento) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.ficheros, e.ID)
	delete(n.carpetas, e.ID)
	return nil
}

func (n *nubeMemoria) datos(r string) (string, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	d, ok := n.ficheros[r]
	return string(d), ok
}

// pantallaConNube es Drive en el equipo con el disco y una nube en memoria, como lo monta
// `cmd-local/drive`: el selector de «Mover a…» con las unidades, mover entre unidades y la
// pregunta de los homónimos.
func pantallaConNube(t *testing.T) (*Pantalla, *archivos.Remotas, *nubeMemoria, fyne.Window) {
	t.Helper()
	disco, err := archivos.Abrir(filepath.Join(t.TempDir(), "Drive"), archivos.Opciones{Instalacion: "i"})
	if err != nil {
		t.Fatal(err)
	}
	n := &nubeMemoria{
		ficheros: map[string][]byte{"/Informes/a.txt": []byte("uno"), "/Informes/a.txt@2": []byte("otro")},
		carpetas: map[string]bool{"/Informes": true},
	}
	cuentas := func() []nube.Cuenta { return []nube.Cuenta{{ID: "c1", Tipo: nube.WebDAV, Rotulo: "Nextcloud"}} }
	rem := archivos.ConRemotas(disco, cuentas, func(string) (nube.Proveedor, error) { return n, nil }, filepath.Join(t.TempDir(), "cache"))
	svc, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(rem)
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	p := NuevaPantalla(traductor(t), svc, nil, w)
	p.EnEquipo = (&equipoDoble{}).enEquipo()
	p.EnEquipo.EsRemoto = archivos.EsRemoto
	p.EnEquipo.UnidadDe = archivos.UnidadDe
	p.EnEquipo.Unidades = func() []UnidadEquipo {
		unidades := rem.Unidades()
		out := make([]UnidadEquipo, 0, len(unidades))
		for _, u := range unidades {
			out = append(out, UnidadEquipo{ID: u.ID, Nombre: u.Nombre})
		}
		return out
	}
	p.EnEquipo.MoverEntreUnidades = func(ctx context.Context, els []Elemento, destino string, avance func(int, int, string)) InformeMover {
		ids := make([]string, 0, len(els))
		for _, el := range els {
			ids = append(ids, el.ID)
		}
		tr, err := rem.Trasladar(ctx, ids, destino, func(a archivos.AvanceTraslado) { avance(a.Hechos, a.Total, a.Nombre) })
		inf := InformeMover{Total: tr.Total, Movidos: len(tr.Movidos), Cancelado: tr.Cancelado, Err: err}
		for _, x := range tr.NoMovidos {
			inf.NoMovidos = append(inf.NoMovidos, x.Nombre)
		}
		return inf
	}
	p.EnEquipo.Homonimos = func(ctx context.Context, carpeta, nombre string) ([]Elemento, string, error) {
		fs, libre, err := rem.Homonimos(ctx, carpeta, nombre)
		out := make([]Elemento, 0, len(fs))
		for _, f := range fs {
			out = append(out, DeFichero(f))
		}
		return out, libre, err
	}
	w.SetContent(p.Objeto())
	return p, rem, n, w
}

// esperarQue espera a que se cumpla algo que hacen las rutinas de la pantalla.
func esperarQue(t *testing.T, que string, cumple func() bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for !cumple() {
		if time.Now().After(limite) {
			t.Fatal("no llegó:", que)
		}
		esperar()
	}
}

// «Mover a…» de algo del disco empieza en su unidad; por encima de su raíz está la lista
// de unidades («Ubicaciones»: «Mi unidad» y cada nube), donde «Mover aquí» no se puede
// pulsar; elegida otra unidad, mover copia, comprueba y quita el origen, con su hoja de
// avance, y la lista se recarga.
func TestMoverAOtraUnidadDesdeElSelector(t *testing.T) {
	p, rem, n, _ := pantallaConNube(t)
	if err := os.WriteFile(filepath.Join(rem.Raiz(), "carta.txt"), []byte("Querida Ana"), 0o600); err != nil {
		t.Fatal(err)
	}
	raiz, _ := p.svc.Root(t.Context())
	hijos, _ := p.svc.Children(t.Context(), raiz.ID, "")
	el := DeFichero(hijos.Files[0])

	s := p.enSuUnidad(&selectorCarpeta{p: p, el: el}, el)
	p.abrirSelector(s, "Mover")
	esperarQue(t, "la raíz del disco", func() bool { return s.carpetas != nil })
	if s.unidad != "" || s.origen != "" {
		t.Fatal("empieza en su carpeta del disco", s.unidad, s.origen)
	}
	s.aUnidades()
	esperarQue(t, "la lista de unidades", func() bool { return s.carpetas != nil })
	nombres := make([]string, 0, len(s.carpetas))
	for _, c := range s.carpetas {
		nombres = append(nombres, c.Nombre)
	}
	if strings.Join(nombres, "|") != p.tr.T("nav.myDrive", nil)+"|Nextcloud" || !s.noHaceNada() {
		t.Fatal("ubicaciones", nombres, s.noHaceNada())
	}
	s.elegirUnidad(s.carpetas[1])
	esperarQue(t, "la raíz de la nube", func() bool { return s.carpetas != nil })
	if s.unidad != "nube~c1~" || s.noHaceNada() || s.nombreDestino() != "Nextcloud" {
		t.Fatal("en la nube", s.unidad, s.noHaceNada(), s.nombreDestino())
	}
	s.mover()
	esperarQue(t, "el fichero en la nube", func() bool { d, _ := n.datos("/carta.txt"); return d == "Querida Ana" })
	esperarQue(t, "fuera del disco", func() bool { _, err := os.Stat(filepath.Join(rem.Raiz(), "carta.txt")); return os.IsNotExist(err) })
}

// Sin unidades (la conectada, o el equipo sin nubes) el selector es el de siempre: sin
// «Ubicaciones» y con «Mi unidad» de primera miga.
func TestElSelectorSinUnidadesNoCambia(t *testing.T) {
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	if p.conUnidades() {
		t.Fatal("la conectada no ofrece otras unidades")
	}
	s := p.enSuUnidad(&selectorCarpeta{p: p, el: Elemento{ID: "f1", Nombre: "a.txt"}}, Elemento{ID: "f1"})
	if s.origen != "" || s.unidad != "" || s.raiz != "" || p.nombreUnidad(s.unidad) != p.tr.T("nav.myDrive", nil) {
		t.Fatal(s.origen, s.unidad, s.raiz)
	}
}

// Subir a una carpeta de la nube con el nombre de dos que ya están pregunta: «Sustituir»
// no se puede pulsar hasta elegir cuál, y lo elegido va encima de ESE por su id; «Guardar
// los dos» sube con el primer nombre libre; «No subirlo», nada.
func TestSubirConHomonimosPregunta(t *testing.T) {
	p, rem, n, w := pantallaConNube(t)
	ctx := t.Context()
	raiz, _ := rem.Children(ctx, "nube~c1~", "")
	informes := raiz.Folders[0].ID
	iguales, libre, err := p.EnEquipo.Homonimos(ctx, informes, "a.txt")
	if err != nil || len(iguales) != 2 || libre != "a (3).txt" {
		t.Fatal(iguales, libre, err)
	}
	var elegido decision
	d := p.dialogoHomonimo("a.txt", iguales, libre, func(x decision) { elegido = x })
	if d == nil || !contieneTexto(w.Canvas().Overlays().Top(), "«homonimosTitulo»") {
		t.Fatal("no pregunta")
	}
	sustituir := botonCon(w.Canvas().Overlays().Top(), "«sustituir»")
	if sustituir == nil || !sustituir.Inerte {
		t.Fatal("«Sustituir» se puede pulsar sin elegir cuál")
	}
	radios := radiosDe(w.Canvas().Overlays().Top())
	if len(radios) != 2 {
		t.Fatal("una opción por cada uno", len(radios))
	}
	radios[1].Tapped(&fyne.PointEvent{})
	if sustituir.Inerte {
		t.Fatal("elegido uno, «Sustituir» sigue apagado")
	}
	sustituir.Tapped(&fyne.PointEvent{})
	if elegido.que != sustituirlo || elegido.id != iguales[1].ID {
		t.Fatal("sustituye el elegido", elegido, iguales[1].ID)
	}

	// Lo que decide la pregunta llega a la subida: encima del elegido, por su id.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("NUEVO"), 0o600); err != nil {
		t.Fatal(err)
	}
	entrada := Entrada{Ruta: filepath.Join(dir, "a.txt"), Sustituye: iguales[1].ID}
	if _, err := SubirArbol(ctx, p.svc, informes, []Entrada{entrada}, NuevoProgreso(1)); err != nil {
		t.Fatal(err)
	}
	if d, _ := n.datos("/Informes/a.txt@2"); d != "NUEVO" {
		t.Fatal("no fue encima del elegido", d)
	}
	if d, _ := n.datos("/Informes/a.txt"); d != "uno" {
		t.Fatal("pisó el otro", d)
	}
	ambos := Entrada{Ruta: filepath.Join(dir, "a.txt"), Como: libre}
	if _, err := SubirArbol(ctx, p.svc, informes, []Entrada{ambos}, NuevoProgreso(1)); err != nil {
		t.Fatal(err)
	}
	if d, _ := n.datos("/Informes/a (3).txt"); d != "NUEVO" {
		t.Fatal("guardar los dos", d)
	}
	// Sin preguntar, con dos que se llaman igual, no se adivina.
	if _, err := SubirArbol(ctx, p.svc, informes, []Entrada{{Ruta: filepath.Join(dir, "a.txt")}}, NuevoProgreso(1)); err == nil {
		t.Fatal("subió sin saber cuál sustituir")
	}

	// Con uno solo, «Sustituir» se puede pulsar ya; cerrar es «No subirlo».
	uno := iguales[:1]
	elegido = decision{que: -1}
	d2 := p.dialogoHomonimo("a.txt", uno, libre, func(x decision) { elegido = x })
	if b := botonCon(w.Canvas().Overlays().Top(), "«sustituir»"); b == nil || b.Inerte || len(radiosDe(w.Canvas().Overlays().Top())) != 0 {
		t.Fatal("con uno, sin elegir")
	}
	d2.Descartar()
	if elegido.que != noSubirlo {
		t.Fatal("cerrar no es no subirlo", elegido)
	}
}

// botonCon es el botón de la capa con ese rótulo.
func botonCon(o fyne.CanvasObject, rotulo string) *ui.Boton {
	var hallado *ui.Boton
	recorrer(o, func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.Boton); ok && b.Texto == rotulo {
			hallado = b
		}
	})
	return hallado
}

// radiosDe son los radios de la capa.
func radiosDe(o fyne.CanvasObject) []*ui.Eleccion {
	var out []*ui.Eleccion
	recorrer(o, func(x fyne.CanvasObject) {
		if e, ok := x.(*ui.Eleccion); ok && e.Redondo {
			out = append(out, e)
		}
	})
	return out
}
