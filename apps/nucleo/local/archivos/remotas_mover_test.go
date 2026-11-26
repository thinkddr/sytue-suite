// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// nubeRota es la nube de rutas de las pruebas con fallos a propósito: Subir de un nombre
// falla, y la de otro guarda algo distinto del mismo tamaño (una copia que no llega
// entera).
type nubeRota struct {
	*nubeGestora
	mu                                 sync.Mutex
	fallarCon, corromper, fallarBajada string
	// alSubir se llama al empezar cada subida (para cancelar a mitad de una).
	alSubir func(nombre string)
}

func (n *nubeRota) Descargar(ctx context.Context, e nube.Elemento) ([]byte, string, error) {
	n.mu.Lock()
	fallar := n.fallarBajada != "" && strings.HasSuffix(e.ID, "/"+n.fallarBajada)
	n.mu.Unlock()
	if fallar {
		return nil, "", errors.New("nube: se cortó la bajada")
	}
	return n.nubeGestora.Descargar(ctx, e)
}

func (n *nubeRota) Subir(ctx context.Context, carpeta, nombre string, datos []byte) (nube.Elemento, error) {
	n.mu.Lock()
	fallar, corromper, al := n.fallarCon == nombre, n.corromper == nombre, n.alSubir
	n.mu.Unlock()
	if al != nil {
		al(nombre)
	}
	if fallar {
		return nube.Elemento{}, errors.New("nube: se cortó la conexión")
	}
	if corromper && len(datos) > 0 {
		datos = append(append([]byte(nil), datos[:len(datos)-1]...), '#')
	}
	return n.nubeGestora.Subir(ctx, carpeta, nombre, datos)
}

// variasUnidades es Drive con el disco, una nube de rutas (WebDAV, «w1») y una de Google
// Drive («g1»), y las marcas guardadas en los ajustes.
func variasUnidades(t *testing.T) (*Remotas, *nubeRota, *googleFalso) {
	t.Helper()
	r, g := unidadGoogle(t)
	t0 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	w := &nubeRota{nubeGestora: &nubeGestora{carpetas: map[string]bool{}, nubeFalsa: &nubeFalsa{
		ficheros: map[string][]byte{"/Informes/a.docx": []byte("DOCX v1"), "/Informes/Fotos/b.png": []byte("PNG")},
		cuando:   map[string]time.Time{"/Informes/a.docx": t0, "/Informes/Fotos/b.png": t0},
	}}}
	gp, err := r.proveedor("g1")
	if err != nil {
		t.Fatal(err)
	}
	r.cuentas = func() []nube.Cuenta {
		return []nube.Cuenta{{ID: "g1", Tipo: nube.Google, Rotulo: "Google Drive de Ana"}, {ID: "w1", Tipo: nube.WebDAV, Rotulo: "Nextcloud"}}
	}
	r.proveedor = func(id string) (nube.Proveedor, error) {
		switch id {
		case "g1":
			return gp, nil
		case "w1":
			return w, nil
		}
		return nil, nube.ErrSinCuenta
	}
	if err := r.GuardarMarcasEn(filepath.Join(t.TempDir(), "marcas.json")); err != nil {
		t.Fatal(err)
	}
	return r, w, g
}

// enDisco crea ficheros bajo la raíz: ruta relativa → contenido ("" con barra final: una
// carpeta vacía).
func enDisco(t *testing.T, r *Remotas, ficheros map[string]string) {
	t.Helper()
	for rel, c := range ficheros {
		p := filepath.Join(r.Raiz(), filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func idDisco(r *Remotas, rel string) string {
	return idDe(filepath.Join(r.Raiz(), filepath.FromSlash(rel)))
}

func (n *nubeFalsa) datos(r string) (string, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	d, ok := n.ficheros[r]
	return string(d), ok
}

// Una carpeta del disco con subcarpetas (y una vacía) pasa entera a la nube: lo copiado
// se comprueba (en una nube sin huella, volviéndolo a leer) y solo después la carpeta va
// a la papelera del disco, de una vez; el avance cuenta fichero a fichero. Su estrella y
// sus etiquetas viajan con ella.
func TestMoverUnaCarpetaDelDiscoALaNube(t *testing.T) {
	r, w, _ := variasUnidades(t)
	ctx := t.Context()
	enDisco(t, r, map[string]string{"Proyecto/a.txt": "uno", "Proyecto/sub/b.txt": "dos", "Proyecto/sub/hondo/c.txt": "tres", "Proyecto/vacia/": ""})
	if err := r.SetFavorite(ctx, api.DriveFile, idDisco(r, "Proyecto/a.txt"), true); err != nil {
		t.Fatal(err)
	}
	if err := r.PonerEtiquetas(ctx, idDisco(r, "Proyecto/sub/b.txt"), []string{"viaje"}); err != nil {
		t.Fatal(err)
	}
	var avances []AvanceTraslado
	tr, err := r.Trasladar(ctx, []string{idDisco(r, "Proyecto")}, idRemoto("w1", ""), func(a AvanceTraslado) { avances = append(avances, a) })
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(tr.Movidos)
	if tr.Total != 4 || len(tr.NoMovidos) != 0 || tr.Cancelado ||
		strings.Join(tr.Movidos, "|") != "Proyecto/a.txt|Proyecto/sub/b.txt|Proyecto/sub/hondo/c.txt|Proyecto/vacia" {
		t.Fatalf("%+v", tr)
	}
	for rel, quiero := range map[string]string{"/Proyecto/a.txt": "uno", "/Proyecto/sub/b.txt": "dos", "/Proyecto/sub/hondo/c.txt": "tres"} {
		if d, ok := w.datos(rel); !ok || d != quiero {
			t.Fatalf("%s en la nube: %q %v", rel, d, ok)
		}
	}
	if !w.carpetas["/Proyecto/vacia"] {
		t.Fatal("la carpeta vacía no llegó")
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "Proyecto")); !os.IsNotExist(err) {
		t.Fatal("el origen sigue en el disco", err)
	}
	papelera, _ := r.Trash(ctx)
	if len(papelera.Folders) != 1 || papelera.Folders[0].Name != "Proyecto" || len(papelera.Files) != 0 {
		t.Fatal("la carpeta, de una vez a la papelera del disco", papelera)
	}
	if len(avances) != 5 || avances[0].Hechos != 0 || avances[0].Total != 4 || avances[4].Hechos != 4 {
		t.Fatal("avance", avances)
	}
	// La estrella y la etiqueta, en la copia.
	favs, _ := r.Favorites(ctx)
	if len(favs) != 1 || favs[0].Name != "a.txt" || !EsRemoto(favs[0].ID) {
		t.Fatal("la estrella no viajó", favs)
	}
	if ls, _ := r.Etiquetas(ctx, idRemoto("w1", "/Proyecto/sub/b.txt")); !slices.Equal(ls, []string{"viaje"}) {
		t.Fatal("la etiqueta no viajó", ls)
	}
}

// De una nube a otra, en los dos sentidos: Google (que da MD5) a WebDAV (sin huella: se
// relee) y vuelta. Lo que se ve con «nombre (2)» llega con ese nombre, y el original va a
// la papelera de su proveedor.
func TestMoverDeUnaNubeAOtra(t *testing.T) {
	r, w, g := variasUnidades(t)
	ctx := t.Context()
	raiz, err := r.Children(ctx, idRemoto("g1", ""), "")
	if err != nil || len(raiz.Folders) != 1 {
		t.Fatal(raiz, err)
	}
	// En la raíz de w1 ya hay un «Informes»: no se pisa, y se dice una vez, por la carpeta.
	tr, err := r.Trasladar(ctx, []string{raiz.Folders[0].ID}, idRemoto("w1", ""), nil)
	if err != nil || tr.Total != 5 || len(tr.Movidos) != 0 || len(tr.NoMovidos) != 1 ||
		tr.NoMovidos[0].Nombre != "Informes" || tr.NoMovidos[0].Motivo != FalloExiste {
		t.Fatalf("%+v %v", tr, err)
	}
	if d, _ := w.datos("/Informes/a.docx"); d != "DOCX v1" {
		t.Fatal("se tocó el destino", d)
	}

	// Con el destino libre: a «Archivo», que se crea antes.
	if _, err := r.Children(ctx, idRemoto("w1", ""), ""); err != nil {
		t.Fatal(err)
	}
	archivo, err := r.CreateFolder(ctx, idRemoto("w1", ""), "Archivo")
	if err != nil {
		t.Fatal(err)
	}
	tr, err = r.Trasladar(ctx, []string{raiz.Folders[0].ID}, archivo.ID, nil)
	if err != nil || len(tr.NoMovidos) != 0 || tr.Total != 5 {
		t.Fatalf("%+v %v", tr, err)
	}
	for rel, quiero := range map[string]string{"/Archivo/Informes/Acta.docx": "ACTA VIEJA", "/Archivo/Informes/Acta (3).docx": "ACTA NUEVA", "/Archivo/Informes/Acta (2).docx": "OTRA ACTA"} {
		if d, ok := w.datos(rel); !ok || d != quiero {
			t.Fatalf("%s: %q %v", rel, d, ok)
		}
	}
	if !w.carpetas["/Archivo/Informes/Fotos"] || !w.carpetas["/Archivo/Informes/Fotos (2)"] {
		t.Fatal("las dos «Fotos»", w.carpetas)
	}
	g.mu.Lock()
	tirada := g.ficheros["inf"].tirado
	g.mu.Unlock()
	if !tirada {
		t.Fatal("«Informes» no fue a la papelera de Google")
	}

	// Y de WebDAV a Google: la copia se comprueba con el MD5 que da Google.
	inf, _ := r.Children(ctx, idRemoto("w1", ""), "")
	var informes string
	for _, c := range inf.Folders {
		if c.Name == "Informes" {
			informes = c.ID
		}
	}
	hijos, _ := r.Children(ctx, informes, "")
	var a string
	for _, f := range hijos.Files {
		if f.Name == "a.docx" {
			a = f.ID
		}
	}
	if err := r.Move(ctx, api.DriveFile, a, idRemoto("g1", "")); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	llego := false
	for _, f := range g.ficheros {
		llego = llego || (f.nombre == "a.docx" && string(f.datos) == "DOCX v1" && slices.Contains(f.padres, "root"))
	}
	g.mu.Unlock()
	if !llego {
		t.Fatal("no llegó a Google")
	}
	if _, ok := w.datos("/Informes/a.docx"); ok {
		t.Fatal("el original sigue en WebDAV")
	}
	if d, _ := w.datos("/.papelera/Informes/a.docx"); d != "DOCX v1" {
		t.Fatal("el original no fue a la papelera de WebDAV")
	}
}

// Si un fichero no se puede subir y otro llega distinto, esos dos se quedan en el origen
// tal cual (la copia mala se retira) y lo demás se mueve; la carpeta no se tira, porque
// no llegó entera, y el resultado dice qué y por qué.
func TestMoverConFalloAMitadDejaElOrigen(t *testing.T) {
	r, w, _ := variasUnidades(t)
	ctx := t.Context()
	enDisco(t, r, map[string]string{"Fotos/a.jpg": "AAAA", "Fotos/b.jpg": "BBBB", "Fotos/c.jpg": "CCCC"})
	w.fallarCon, w.corromper = "b.jpg", "c.jpg"
	tr, err := r.Trasladar(ctx, []string{idDisco(r, "Fotos")}, idRemoto("w1", ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tr.Movidos, "|") != "Fotos/a.jpg" || len(tr.NoMovidos) != 2 {
		t.Fatalf("%+v", tr)
	}
	motivos := map[string]MotivoTraslado{}
	for _, n := range tr.NoMovidos {
		motivos[n.Nombre] = n.Motivo
	}
	if motivos["Fotos/b.jpg"] != FalloCopia || motivos["Fotos/c.jpg"] != FalloComprobacion {
		t.Fatal("motivos", motivos)
	}
	for f, quiero := range map[string]string{"b.jpg": "BBBB", "c.jpg": "CCCC"} {
		if b, err := os.ReadFile(filepath.Join(r.Raiz(), "Fotos", f)); err != nil || string(b) != quiero {
			t.Fatal("el origen no quedó intacto", f, string(b), err)
		}
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "Fotos", "a.jpg")); !os.IsNotExist(err) {
		t.Fatal("lo movido sigue en el origen", err)
	}
	if d, _ := w.datos("/Fotos/a.jpg"); d != "AAAA" {
		t.Fatal("lo movido no está en la nube", d)
	}
	for _, f := range []string{"/Fotos/b.jpg", "/Fotos/c.jpg"} {
		if _, ok := w.datos(f); ok {
			t.Fatal("quedó en la nube una copia que no vale", f)
		}
	}
	papelera, _ := r.Trash(ctx)
	if len(papelera.Files) != 1 || papelera.Files[0].Name != "a.jpg" || len(papelera.Folders) != 0 {
		t.Fatal("a la papelera va solo lo que se movió", papelera)
	}

	// Al revés, de la nube al disco: si un fichero no se puede bajar, ese se queda en la
	// nube y su carpeta también; el otro llega.
	w.fallarBajada = "b.png"
	raiz, _ := r.Children(ctx, idRemoto("w1", ""), "")
	var informes string
	for _, c := range raiz.Folders {
		if c.Name == "Informes" {
			informes = c.ID
		}
	}
	tr, err = r.Trasladar(ctx, []string{informes}, "", nil)
	if err != nil || strings.Join(tr.Movidos, "|") != "Informes/a.docx" || len(tr.NoMovidos) != 1 || tr.NoMovidos[0].Nombre != "Informes/Fotos/b.png" {
		t.Fatalf("%+v %v", tr, err)
	}
	if d, _ := w.datos("/Informes/Fotos/b.png"); d != "PNG" {
		t.Fatal("el origen en la nube no quedó intacto", d)
	}
	if _, ok := w.datos("/Informes/a.docx"); ok {
		t.Fatal("lo movido sigue en la nube")
	}
	if b, _ := os.ReadFile(filepath.Join(r.Raiz(), "Informes", "a.docx")); string(b) != "DOCX v1" {
		t.Fatal("no llegó al disco", string(b))
	}
}

// Cancelar a mitad para entre fichero y fichero: lo copiado se queda movido (ya está
// comprobado) y lo demás sigue en el origen.
func TestMoverSeCancelaSinPerderNada(t *testing.T) {
	r, w, _ := variasUnidades(t)
	enDisco(t, r, map[string]string{"Lote/1.txt": "uno", "Lote/2.txt": "dos", "Lote/3.txt": "tres"})
	ctx, cancelar := context.WithCancel(t.Context())
	defer cancelar()
	tr, err := r.Trasladar(ctx, []string{idDisco(r, "Lote")}, idRemoto("w1", ""), func(a AvanceTraslado) {
		if a.Hechos == 1 {
			cancelar()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Cancelado || strings.Join(tr.Movidos, "|") != "Lote/1.txt" || len(tr.NoMovidos) != 2 || tr.NoMovidos[0].Motivo != FalloCancelado {
		t.Fatalf("%+v", tr)
	}
	for _, f := range []string{"2.txt", "3.txt"} {
		if _, err := os.Stat(filepath.Join(r.Raiz(), "Lote", f)); err != nil {
			t.Fatal("lo no movido desapareció del origen", f, err)
		}
	}
	if d, _ := w.datos("/Lote/1.txt"); d != "uno" {
		t.Fatal("lo movido antes de cancelar no está", d)
	}
	if _, ok := w.datos("/Lote/2.txt"); ok {
		t.Fatal("siguió tras cancelar")
	}
}

// Cancelar mientras sube uno no lo corta: ese termina (se comprueba y se quita del
// origen) y lo siguiente ya no empieza. Cortarlo podría dejar en la nube una copia que el
// servidor guardara después de darla aquí por fallida.
func TestCancelarDejaTerminarElQueVa(t *testing.T) {
	r, w, _ := variasUnidades(t)
	enDisco(t, r, map[string]string{"Lote/1.txt": "uno", "Lote/2.txt": "dos"})
	ctx, cancelar := context.WithCancel(t.Context())
	defer cancelar()
	w.alSubir = func(nombre string) {
		if nombre == "1.txt" {
			cancelar()
		}
	}
	tr, err := r.Trasladar(ctx, []string{idDisco(r, "Lote")}, idRemoto("w1", ""), nil)
	if err != nil || !tr.Cancelado || strings.Join(tr.Movidos, "|") != "Lote/1.txt" {
		t.Fatalf("%+v %v", tr, err)
	}
	if d, _ := w.datos("/Lote/1.txt"); d != "uno" {
		t.Fatal("el que iba no terminó", d)
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "Lote", "1.txt")); !os.IsNotExist(err) {
		t.Fatal("el que iba está en los dos sitios", err)
	}
	if _, ok := w.datos("/Lote/2.txt"); ok {
		t.Fatal("empezó el siguiente tras cancelar")
	}
	if b, _ := os.ReadFile(filepath.Join(r.Raiz(), "Lote", "2.txt")); string(b) != "dos" {
		t.Fatal("el siguiente no quedó en el origen")
	}
}

// En el destino no se pisa nada: si ya hay algo con ese nombre no se mueve, y Move lo dice
// con un 409.
func TestMoverNoPisaEnElDestino(t *testing.T) {
	r, w, _ := variasUnidades(t)
	ctx := t.Context()
	enDisco(t, r, map[string]string{"Informes/a.docx": "DEL DISCO"})
	err := r.Move(ctx, api.DriveFolder, idDisco(r, "Informes"), idRemoto("w1", ""))
	if codigo(err) != http.StatusConflict {
		t.Fatal("quiero un conflicto", err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Raiz(), "Informes", "a.docx")); string(b) != "DEL DISCO" {
		t.Fatal("el origen cambió")
	}
	if d, _ := w.datos("/Informes/a.docx"); d != "DOCX v1" {
		t.Fatal("se pisó el destino", d)
	}
}

// Un documento de Google no tiene fichero (bajarlo es exportarlo): no se mueve, y su
// carpeta se queda en Google con él; lo demás de la carpeta, sí. Y lo oculto del disco
// tampoco viaja: la carpeta se queda con ello.
func TestMoverDejaLoQueNoViaja(t *testing.T) {
	r, _, g := variasUnidades(t)
	ctx := t.Context()
	g.mu.Lock()
	g.poner("cv", "Curro", "application/vnd.google-apps.folder", "root", "", time.Now())
	g.poner("doc", "Notas", "application/vnd.google-apps.document", "cv", "", time.Now())
	g.poner("pdf", "cv.pdf", "application/pdf", "cv", "%PDF", time.Now())
	g.mu.Unlock()
	if _, err := r.Children(ctx, idRemoto("g1", ""), ""); err != nil {
		t.Fatal(err)
	}
	tr, err := r.Trasladar(ctx, []string{idRemoto("g1", "cv")}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tr.Movidos, "|") != "Curro/cv.pdf" || len(tr.NoMovidos) != 1 || tr.NoMovidos[0].Motivo != FalloDocumentoGoogle {
		t.Fatalf("%+v", tr)
	}
	g.mu.Lock()
	cvTirada, docTirado, pdfTirado := g.ficheros["cv"].tirado, g.ficheros["doc"].tirado, g.ficheros["pdf"].tirado
	g.mu.Unlock()
	if cvTirada || docTirado || !pdfTirado {
		t.Fatal("en Google se queda la carpeta con el documento", cvTirada, docTirado, pdfTirado)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Raiz(), "Curro", "cv.pdf")); string(b) != "%PDF" {
		t.Fatal("el PDF no llegó al disco")
	}

	enDisco(t, r, map[string]string{"Mac/.DS_Store": "x", "Mac/foto.jpg": "JPG"})
	tr, err = r.Trasladar(ctx, []string{idDisco(r, "Mac")}, idRemoto("g1", ""), nil)
	if err != nil || strings.Join(tr.Movidos, "|") != "Mac/foto.jpg" {
		t.Fatalf("%+v %v", tr, err)
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "Mac", ".DS_Store")); err != nil {
		t.Fatal("la carpeta con lo oculto se queda", err)
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "Mac", "foto.jpg")); !os.IsNotExist(err) {
		t.Fatal("lo movido sigue", err)
	}
}

// Mover y tirar en lote dentro de una unidad lo hace su proveedor (antes iba al disco y
// fallaba), y en lote hacia otra unidad, un traslado por elemento.
func TestLotesConUnidades(t *testing.T) {
	r, w, _ := variasUnidades(t)
	ctx := t.Context()
	raiz, _ := r.Children(ctx, idRemoto("w1", ""), "")
	inf, _ := r.Children(ctx, raiz.Folders[0].ID, "")
	lote, err := r.BatchMove(ctx, []string{inf.Files[0].ID}, nil, inf.Folders[0].ID)
	if err != nil || lote.Ficheros != 1 {
		t.Fatal(lote, err)
	}
	if d, _ := w.datos("/Informes/Fotos/a.docx"); d != "DOCX v1" {
		t.Fatal("mover en lote dentro de la unidad", d)
	}
	enDisco(t, r, map[string]string{"x.txt": "X", "y.txt": "Y"})
	lote, err = r.BatchMove(ctx, []string{idDisco(r, "x.txt"), idDisco(r, "y.txt")}, nil, idRemoto("w1", ""))
	if err != nil || lote.Ficheros != 2 {
		t.Fatal(lote, err)
	}
	if d, _ := w.datos("/y.txt"); d != "Y" {
		t.Fatal("en lote a otra unidad", d)
	}
	hijos, _ := r.Children(ctx, idRemoto("w1", ""), "")
	var x string
	for _, f := range hijos.Files {
		if f.Name == "x.txt" {
			x = f.ID
		}
	}
	if lote, err := r.BatchTrash(ctx, []string{x}, nil); err != nil || lote.Ficheros != 1 {
		t.Fatal("tirar en lote en la unidad", lote, err)
	}
	if d, _ := w.datos("/.papelera/x.txt"); d != "X" {
		t.Fatal("a la papelera del proveedor", d)
	}
}

// La estrella y las etiquetas de lo que está en la nube se guardan en este equipo por su
// identificador estable: renombrarlo o moverlo (aquí o en la web de Google) no las
// pierde; en WebDAV, sin identificador estable, van con la ruta al renombrar desde aquí. Se
// ven en «Favoritos», en las etiquetas que se ofrecen y en la búsqueda, y siguen al
// volver a abrir la app.
func TestFavoritosYEtiquetasEnLaNube(t *testing.T) {
	r, _, g := variasUnidades(t)
	ctx := t.Context()
	raiz, _ := r.Children(ctx, idRemoto("g1", ""), "")
	if _, err := r.Children(ctx, raiz.Folders[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	acta := idRemoto("g1", "a-2")
	if err := r.SetFavorite(ctx, api.DriveFile, acta, true); err != nil {
		t.Fatal(err)
	}
	if err := r.PonerEtiquetas(ctx, acta, []string{"Juntas", "2026"}); err != nil {
		t.Fatal(err)
	}
	// Se renombra y se mueve en la web de Google.
	g.mu.Lock()
	g.ficheros["a-2"].nombre = "Acta de la junta.docx"
	g.ficheros["a-2"].padres = []string{"f-1"}
	g.mu.Unlock()
	if _, err := r.Children(ctx, idRemoto("g1", "f-1"), ""); err != nil {
		t.Fatal(err)
	}
	favs, _ := r.Favorites(ctx)
	if len(favs) != 1 || favs[0].ID != acta || favs[0].Name != "Acta de la junta.docx" || favs[0].FolderID != idRemoto("g1", "f-1") {
		t.Fatal("la estrella tras renombrar en Google", favs)
	}
	// Y renombrado desde aquí.
	if err := r.Rename(ctx, api.DriveFile, acta, "Junta.docx"); err != nil {
		t.Fatal(err)
	}
	if ls, _ := r.Etiquetas(ctx, acta); !slices.Equal(ls, []string{"2026", "Juntas"}) {
		t.Fatal("las etiquetas tras renombrar", ls)
	}

	// WebDAV (la ruta es el id): renombrar el fichero y su carpeta desde aquí.
	w1, _ := r.Children(ctx, idRemoto("w1", ""), "")
	inf, _ := r.Children(ctx, w1.Folders[0].ID, "")
	if err := r.SetFavorite(ctx, api.DriveFile, inf.Files[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := r.PonerEtiquetas(ctx, inf.Files[0].ID, []string{"viaje"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(ctx, api.DriveFile, inf.Files[0].ID, "b.docx"); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(ctx, api.DriveFolder, w1.Folders[0].ID, "Viejos"); err != nil {
		t.Fatal(err)
	}
	favs, _ = r.Favorites(ctx)
	ids := make([]string, 0, len(favs))
	for _, f := range favs {
		ids = append(ids, f.ID+"="+f.Name)
	}
	slices.Sort(ids)
	quiero := []string{acta + "=Junta.docx", idRemoto("w1", "/Viejos/b.docx") + "=b.docx"}
	slices.Sort(quiero)
	if !slices.Equal(ids, quiero) {
		t.Fatal("favoritos tras renombrar en WebDAV", ids, quiero)
	}

	// Con las del disco, en lo que se ofrece y en la búsqueda.
	enDisco(t, r, map[string]string{"x.txt": "X"})
	if err := r.PonerEtiquetas(ctx, idDisco(r, "x.txt"), []string{"casa"}); err != nil {
		t.Fatal(err)
	}
	if todas := r.TodasLasEtiquetas(ctx); strings.Join(todas, "|") != "2026|casa|Juntas|viaje" {
		t.Fatal("todas", todas)
	}
	hay, _ := r.Search(ctx, "viaj")
	if len(hay.Files) != 1 || hay.Files[0].ID != idRemoto("w1", "/Viejos/b.docx") {
		t.Fatal("buscar por etiqueta en la nube", hay)
	}

	// Al volver a abrir, siguen; y abrir el favorito lo baja aunque su carpeta no se haya
	// listado en esta sesión.
	otra := ConRemotas(r.Archivos, r.cuentas, r.proveedor, filepath.Join(t.TempDir(), "cache"))
	if err := otra.GuardarMarcasEn(r.rutaMarcas); err != nil {
		t.Fatal(err)
	}
	favs, _ = otra.Favorites(ctx)
	if len(favs) != 2 {
		t.Fatal("al volver a abrir", favs)
	}
	f, err := otra.File(ctx, acta)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f.Path); string(b) != "OTRA ACTA" {
		t.Fatal("abrir un favorito", string(b))
	}

	// Tirarlo se lleva su estrella.
	if err := otra.Delete(ctx, api.DriveFile, acta); err != nil {
		t.Fatal(err)
	}
	if favs, _ = otra.Favorites(ctx); len(favs) != 1 {
		t.Fatal("tirado, sin estrella", favs)
	}

	// Sin fichero de marcas (como antes), una unidad no los admite.
	sin := ConRemotas(r.Archivos, r.cuentas, r.proveedor, filepath.Join(t.TempDir(), "cache"))
	if _, err := sin.Children(ctx, idRemoto("w1", ""), ""); err != nil {
		t.Fatal(err)
	}
	viejos, _ := sin.Children(ctx, idRemoto("w1", ""), "")
	if err := sin.SetFavorite(ctx, api.DriveFolder, viejos.Folders[0].ID, true); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("sin marcas", err)
	}
}

// Subir con un nombre que en la carpeta de Google llevan DOS documentos no adivina cuál
// sustituir: falla con un conflicto y no sube nada. Homonimos da los dos con el nombre con
// que se ven, y el primer nombre libre; sustituir el elegido (ReplaceFile con solo su id)
// va encima de ese, y guardar los dos sube con el nombre libre. Con uno solo, Upload lo
// sustituye por su id.
func TestSubirConDosQueSeLlamanIgual(t *testing.T) {
	r, g := unidadGoogle(t)
	ctx := t.Context()
	raiz, _ := r.Children(ctx, r.Unidades()[0].ID, "")
	informes := raiz.Folders[0].ID
	if _, err := r.Children(ctx, informes, ""); err != nil {
		t.Fatal(err)
	}
	antes := len(g.ficheros)
	_, err := r.Upload(ctx, informes, "Acta.docx", "", strings.NewReader("NUEVA"), 5)
	if codigo(err) != http.StatusConflict {
		t.Fatal("quiero un conflicto, no adivinar", err)
	}
	if len(g.ficheros) != antes || g.datosDe("a-viejo") != "ACTA VIEJA" || g.datosDe("a-nuevo") != "ACTA NUEVA" {
		t.Fatal("subió o sustituyó algo")
	}
	iguales, libre, err := r.Homonimos(ctx, informes, "Acta.docx")
	if err != nil || len(iguales) != 2 || libre != "Acta (4).docx" {
		t.Fatal(iguales, libre, err)
	}
	nombres := []string{iguales[0].Name, iguales[1].Name}
	slices.Sort(nombres)
	if strings.Join(nombres, "|") != "Acta (3).docx|Acta.docx" {
		t.Fatal("con el nombre con que se ven", nombres)
	}
	// El que se ve como «Acta (3).docx» es uno solo.
	if uno, _, _ := r.Homonimos(ctx, informes, "Acta (3).docx"); len(uno) != 1 || uno[0].ID != idRemoto("g1", "a-nuevo") {
		t.Fatal("por el nombre de la vista", uno)
	}
	// Sustituir el elegido.
	if _, err := r.ReplaceFile(ctx, api.File{ID: idRemoto("g1", "a-nuevo")}, "", strings.NewReader("NUEVA"), 5); err != nil {
		t.Fatal(err)
	}
	if g.datosDe("a-nuevo") != "NUEVA" || g.datosDe("a-viejo") != "ACTA VIEJA" {
		t.Fatal("sustituyó otro", g.datosDe("a-nuevo"), g.datosDe("a-viejo"))
	}
	// Guardar los dos.
	f, err := r.Upload(ctx, informes, libre, "", strings.NewReader("OTRA MÁS"), 9)
	if err != nil || f.Name != "Acta (4).docx" || len(g.ficheros) != antes+1 {
		t.Fatal("guardar los dos", f, err)
	}
	// Con uno solo, se sustituye ese, por su id.
	if _, err := r.Upload(ctx, informes, "Acta (2).docx", "", strings.NewReader("SUSTITUIDA"), 10); err != nil {
		t.Fatal(err)
	}
	if g.datosDe("a-2") != "SUSTITUIDA" || len(g.ficheros) != antes+1 {
		t.Fatal("con uno solo", g.datosDe("a-2"))
	}
	// En el disco no hay homónimos que preguntar.
	if h, _, err := r.Homonimos(ctx, idDe(r.Raiz()), "x"); err != nil || h != nil {
		t.Fatal(h, err)
	}
}
