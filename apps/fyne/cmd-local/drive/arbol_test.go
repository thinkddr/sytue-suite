// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// Un documento de la carpeta elegida se abre en su app de Sytue Office (un VIEW explícito
// a su paquete, con el tipo que declara); sin ella instalada, o sin app para él, con la
// del sistema, como antes.
func TestUnDocumentoDeLaCarpetaElegidaSeAbreEnSuAppDeOffice(t *testing.T) {
	descargas := nuevoArbolFalso("Download")
	s := &sistemaFalso{arboles: map[string]*arbolFalso{"content://t/descargas": descargas}, vigentes: map[string]bool{"content://t/descargas": true}}
	c, _ := carpetaDePrueba(t, s)
	disco, _, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.usar(ficheros.ArbolElegido{URI: "content://t/descargas", Nombre: "Download", Persistente: true}); err != nil {
		t.Fatal(err)
	}
	raiz, err := disco.Root(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	subir := func(nombre string) string {
		f, err := disco.Upload(t.Context(), raiz.ID, nombre, "", strings.NewReader("x"), 1)
		if err != nil {
			t.Fatal(err)
		}
		return f.Path
	}
	plan, pdf, nota := subir("plan.docx"), subir("firmado.pdf"), subir("nota.txt")
	abrir := func(ruta string) string {
		t.Helper()
		ver, ok := c.enArbol(ruta)
		if !ok {
			t.Fatal("no es del árbol:", ruta)
		}
		descargas.vistos = nil
		if err := ver(true); err != nil {
			t.Fatal(err)
		}
		if len(descargas.vistos) != 1 {
			t.Fatal(descargas.vistos)
		}
		_, que, _ := strings.Cut(descargas.vistos[0], " ")
		return que
	}

	// Sin las apps de Office: la del sistema.
	if got := abrir(plan); got != "abrir" {
		t.Fatal("sin Docus:", got)
	}
	descargas.apps = map[string]bool{"eu.sytue.office.docus": true, "eu.sytue.office.certipdf": true}
	if got := abrir(plan); got != "en eu.sytue.office.docus "+tiposOffice["docx"] {
		t.Fatal("con Docus:", got)
	}
	if got := abrir(pdf); got != "en eu.sytue.office.certipdf application/pdf" {
		t.Fatal("con CertiPDF:", got)
	}
	// Un .txt no es de ninguna (en el escritorio tampoco): la app del sistema.
	if got := abrir(nota); got != "abrir" {
		t.Fatal("un .txt:", got)
	}
	// Compartir sigue siendo la hoja del sistema.
	ver, _ := c.enArbol(plan)
	descargas.vistos = nil
	if err := ver(false); err != nil || len(descargas.vistos) != 1 || !strings.HasSuffix(descargas.vistos[0], " compartir") {
		t.Fatal("compartir:", descargas.vistos, err)
	}
}

// La tabla de tipos es la del manifiesto: cada extensión que declaran Docus, Calcus,
// Diapos y CertiPDF, con el tipo que `empaquetar.sh` pone en su filtro de «Abrir con»; y
// el paquete es su identificador.
func TestLosTiposSonLosDelManifiesto(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("sin bash")
	}
	vistas := map[string]bool{}
	for _, app := range appsQueAbren {
		toml, err := os.ReadFile(filepath.Join("..", app, "FyneApp.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(toml), `ID = "eu.sytue.office.`+app+`"`) {
			t.Errorf("%s no es eu.sytue.office.%s", app, app)
		}
		var exts []string
		for linea := range strings.SplitSeq(string(toml), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(linea), "extensiones = "); ok {
				exts = strings.Split(strings.Trim(v, `"`), ",")
			}
		}
		cmd := exec.Command(bash, filepath.Join("..", "..", "scripts", "empaquetar.sh"), "tipos", app)
		cmd.Env = append(os.Environ(), "SYTUE_ARBOL=cmd-local")
		salida, err := cmd.Output()
		if err != nil {
			t.Fatalf("empaquetar.sh tipos %s: %v", app, err)
		}
		tipos := strings.Fields(string(salida))
		if len(tipos) != len(exts) {
			t.Fatalf("%s: %d extensiones y %d tipos", app, len(exts), len(tipos))
		}
		for i, ext := range exts {
			vistas[ext] = true
			if tiposOffice[ext] != tipos[i] {
				t.Errorf("%s .%s: el manifiesto dice %q y la tabla %q", app, ext, tipos[i], tiposOffice[ext])
			}
		}
	}
	for ext := range tiposOffice {
		if !vistas[ext] {
			t.Errorf(".%s está en la tabla y ninguna app la declara", ext)
		}
	}
}

// «Copiar a…» en el teléfono: el destino lo da el selector, la copia va al árbol elegido y
// su permiso se suelta al acabar; el de la carpeta de Drive, no.
func TestCopiarAUnaCarpetaDelTelefono(t *testing.T) {
	descargas, tarjeta := nuevoArbolFalso("Download"), nuevoArbolFalso("Tarjeta")
	s := &sistemaFalso{
		arboles:  map[string]*arbolFalso{"content://t/descargas": descargas, "content://t/tarjeta": tarjeta},
		vigentes: map[string]bool{"content://t/descargas": true},
	}
	c, datos := carpetaDePrueba(t, s)
	disco, _, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"})
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := disco.Root(t.Context())
	f, err := disco.Upload(t.Context(), raiz.ID, "plan.txt", "", strings.NewReader("plan"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.Path, filepath.Join(datos, "Drive")) {
		t.Fatal("Drive empieza en la carpeta de la app:", f.Path)
	}

	// Cancelar no copia ni avisa.
	if _, elegida, err := c.copiarA(t.Context(), f.ID, disco.CopiarFueraEn); elegida || err != nil {
		t.Fatal("cancelar:", elegida, err)
	}
	// Una carpeta de la tarjeta.
	s.elegido = ficheros.ArbolElegido{URI: "content://t/tarjeta", Nombre: "Tarjeta", Persistente: true}
	s.vigentes["content://t/tarjeta"] = true
	destino, elegida, err := c.copiarA(t.Context(), f.ID, disco.CopiarFueraEn)
	if err != nil || !elegida || destino != "Tarjeta/plan.txt" {
		t.Fatal(destino, elegida, err)
	}
	if got := datosDe(t, tarjeta, "plan.txt"); got != "plan" {
		t.Fatal("en la tarjeta:", got)
	}
	if !slices.Equal(s.soltados, []string{"content://t/tarjeta"}) {
		t.Fatal("el permiso de la carpeta de destino se suelta:", s.soltados)
	}
	// Otra vez: no pisa.
	if destino, _, err := c.copiarA(t.Context(), f.ID, disco.CopiarFueraEn); err != nil || destino != "Tarjeta/plan (2).txt" {
		t.Fatal(destino, err)
	}
	// Una carpeta que solo deja leer no vale de destino.
	s.elegido, s.errElegir = ficheros.ArbolElegido{URI: "content://t/tarjeta", SoloLectura: true}, ficheros.ErrSoloLectura
	if _, elegida, err := c.copiarA(t.Context(), f.ID, disco.CopiarFueraEn); !elegida || err == nil {
		t.Fatal("solo lectura:", elegida, err)
	}
	s.errElegir = nil

	// La propia carpeta de Drive como destino: su permiso NO se suelta.
	if _, err := c.usar(ficheros.ArbolElegido{URI: "content://t/descargas", Nombre: "Download", Persistente: true}); err != nil {
		t.Fatal(err)
	}
	s.soltados = nil
	raiz, _ = disco.Root(t.Context())
	g, err := disco.Upload(t.Context(), raiz.ID, "otro.txt", "", strings.NewReader("otro"), 4)
	if err != nil {
		t.Fatal(err)
	}
	s.elegido = ficheros.ArbolElegido{URI: "content://t/descargas", Nombre: "Download", Persistente: true}
	if destino, _, err := c.copiarA(t.Context(), g.ID, disco.CopiarFueraEn); err != nil || destino != "Download/otro (2).txt" {
		t.Fatal(destino, err)
	}
	if len(s.soltados) != 0 || !s.vigentes["content://t/descargas"] {
		t.Fatal("soltó el permiso de la carpeta de Drive:", s.soltados)
	}
}

// «Subir carpeta» en el teléfono: el origen lo da el selector, cada fichero lleva su
// subcarpeta empezando por la elegida, lo oculto no sube, nada se abre hasta subirlo y el
// permiso se suelta al cerrar el último.
func TestSubirUnaCarpetaDelTelefono(t *testing.T) {
	fotos := nuevoArbolFalso("Viaje")
	poner := func(padre, nombre string, carpeta bool, datos string) string {
		d, _ := fotos.Crear(padre, nombre, carpeta)
		fotos.docs[d.ID].datos = []byte(datos)
		return d.ID
	}
	poner("r", "playa.jpg", false, "jpg")
	dia := poner("r", "Día 2", true, "")
	poner(dia, "museo.jpg", false, "museo")
	oculta := poner("r", ".thumbnails", true, "")
	poner(oculta, "x.jpg", false, "no")
	poner("r", ".nomedia", false, "")

	s := &sistemaFalso{arboles: map[string]*arbolFalso{"content://t/viaje": fotos}, vigentes: map[string]bool{}}
	c, _ := carpetaDePrueba(t, s)
	if _, _, err := c.abrirDisco(archivos.Opciones{Instalacion: "i"}); err != nil {
		t.Fatal(err)
	}
	if es, err := c.entradasDe(); es != nil || err != nil {
		t.Fatal("cancelar:", es, err)
	}
	// Una carpeta que solo deja leer vale de origen.
	s.elegido, s.errElegir = ficheros.ArbolElegido{URI: "content://t/viaje", Nombre: "Viaje", SoloLectura: true}, ficheros.ErrSoloLectura
	es, err := c.entradasDe()
	if err != nil {
		t.Fatal(err)
	}
	vistas := make([]string, 0, len(es))
	for _, e := range es {
		vistas = append(vistas, e.Dir+"/"+e.Nombre)
	}
	slices.Sort(vistas)
	if !slices.Equal(vistas, []string{"Viaje/Día 2/museo.jpg", "Viaje/playa.jpg"}) {
		t.Fatal(vistas)
	}
	for _, e := range es {
		b, err := io.ReadAll(e.Flujo)
		if err != nil || len(b) == 0 {
			t.Fatal(e.Nombre, err)
		}
		if err := e.Flujo.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Sin permiso guardado (solo lectura) no hay nada que soltar.
	if len(s.soltados) != 0 {
		t.Fatal(s.soltados)
	}

	// Con permiso guardado, se suelta cuando se cierra la última, y no antes.
	s.elegido, s.errElegir = ficheros.ArbolElegido{URI: "content://t/viaje", Nombre: "Viaje", Persistente: true}, nil
	es, err = c.entradasDe()
	if err != nil || len(es) != 2 {
		t.Fatal(es, err)
	}
	_ = es[0].Flujo.Close()
	if len(s.soltados) != 0 {
		t.Fatal("soltó antes de tiempo")
	}
	_ = es[1].Flujo.Close()
	_ = es[1].Flujo.Close()
	if !slices.Equal(s.soltados, []string{"content://t/viaje"}) {
		t.Fatal(s.soltados)
	}
}

// datosDe es lo que hay en el fichero de ese nombre de la raíz del árbol falso.
func datosDe(t *testing.T, a *arbolFalso, nombre string) string {
	t.Helper()
	hs, _ := a.Hijos("r")
	for _, h := range hs {
		if h.Nombre == nombre {
			return string(a.docs[h.ID].datos)
		}
	}
	t.Fatalf("%s no está en %s", nombre, a.nombre)
	return ""
}
