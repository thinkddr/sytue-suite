// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardar(t *testing.T) {
	carpeta := filepath.Join(t.TempDir(), "Descargas") // no existe: se crea
	for _, c := range []struct{ nombre, quiero string }{
		{"informe.pdf", "informe.pdf"},
		{"informe.pdf", "informe (1).pdf"}, // no se pisa lo que ya había
		{"informe.pdf", "informe (2).pdf"},
		{"../../.ssh/authorized_keys", "authorized_keys"}, // ni se sale de la carpeta
		{`C:\Windows\system32\drivers\etc\hosts`, "hosts"},
		{"..", "descarga"},
		{"", "descarga (1)"},
	} {
		ruta, err := Guardar(carpeta, c.nombre, strings.NewReader("contenido de "+c.quiero))
		if err != nil {
			t.Fatalf("%q: %v", c.nombre, err)
		}
		if ruta != filepath.Join(carpeta, c.quiero) {
			t.Errorf("%q ha quedado en %q, quiero %q", c.nombre, ruta, c.quiero)
		}
		if datos, _ := os.ReadFile(ruta); string(datos) != "contenido de "+c.quiero {
			t.Errorf("%q contiene %q", ruta, datos)
		}
	}
	// Nada ha acabado fuera de la carpeta.
	if fuera, _ := filepath.Glob(filepath.Join(filepath.Dir(carpeta), "*")); len(fuera) != 1 {
		t.Errorf("hay ficheros fuera de la carpeta: %v", fuera)
	}
}

// TestLocalRechaza es el guardián de lo que se le pasa a un programa del sistema: Revelar,
// Abrir e Imprimir acaban las tres en `local`, y lo que no sea un fichero de verdad no
// pasa de ahí. Se ha visto en rojo quitando la comprobación de IsRegular: entonces una
// carpeta pasaba y el gestor de ficheros se abría con ella.
func TestLocalRechaza(t *testing.T) {
	dir := t.TempDir()
	bueno := filepath.Join(dir, "informe.pdf")
	if err := os.WriteFile(bueno, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ nombre, ruta string }{
		{"vacía", ""},
		{"espacios", "   "},
		{"no existe", filepath.Join(dir, "no-está.pdf")},
		{"una carpeta no es un fichero", dir},
	} {
		if _, err := local(c.ruta); err == nil {
			t.Errorf("%s: local(%q) la ha dado por buena", c.nombre, c.ruta)
		}
	}
	abs, err := local(bueno)
	if err != nil {
		t.Fatalf("un fichero de verdad: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Errorf("local ha devuelto %q, que no es absoluta: una relativa que empiece por «-» se leería como opción", abs)
	}
	// El error NO lleva la ruta: v4 §4 prohíbe rutas locales en errores.
	_, err = local(filepath.Join(dir, "no-está.pdf"))
	if err == nil || strings.Contains(err.Error(), dir) {
		t.Errorf("el error enseña la ruta local: %v", err)
	}
}

// TestLineas: la salida de los tres selectores, con el retorno de carro de Windows y la
// línea vacía del final, se queda en las rutas y nada más.
func TestLineas(t *testing.T) {
	got := lineas([]byte("/tmp/a b.pdf\r\n/tmp/c.txt\n\n"))
	if len(got) != 2 || got[0] != "/tmp/a b.pdf" || got[1] != "/tmp/c.txt" {
		t.Errorf("lineas = %q", got)
	}
	if lineas(nil) != nil {
		t.Error("sin salida tiene que salir nada")
	}
}

// TestLaCopiaParaImprimirNoSeSaleDeSuCarpeta: el nombre del PDF lo pone quien lo subió,
// y la copia que se manda a imprimir se queda en su carpeta temporal con ese nombre y el
// contenido tal cual, legible solo por quien la escribe. VISTO EN ROJO sin el
// `filepath.Base` (la copia de «../fuera.pdf» salía de la carpeta).
func TestLaCopiaParaImprimirNoSeSaleDeSuCarpeta(t *testing.T) {
	tmp := t.TempDir()
	for _, nombre := range []string{"acta.pdf", "../fuera.pdf", `..\..\fuera.pdf`} {
		ruta, err := copiaParaImprimir(tmp, nombre, []byte("%PDF-1.7 "+nombre))
		if err != nil {
			t.Fatalf("%q: %v", nombre, err)
		}
		if d := filepath.Dir(ruta); filepath.Dir(d) != tmp || !strings.HasPrefix(filepath.Base(d), "sytue-imprimir-") {
			t.Errorf("%q ha quedado en %s, fuera de su carpeta", nombre, ruta)
		}
		if b, err := os.ReadFile(ruta); err != nil || string(b) != "%PDF-1.7 "+nombre {
			t.Errorf("%q: se ha escrito %q (%v)", nombre, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "fuera.pdf")); err == nil {
		t.Error("una copia ha salido de la carpeta de impresión")
	}
}

// TestLaCopiaParaImprimirNoUsaUnaCarpetaAjena (NAT-13): una carpeta
// `sytue-imprimir` que otra cuenta dejó en el /tmp compartido no recibe la copia;
// cada impresión va a una carpeta nueva, solo para quien la crea.
func TestLaCopiaParaImprimirNoUsaUnaCarpetaAjena(t *testing.T) {
	tmp := t.TempDir()
	ajena := filepath.Join(tmp, "sytue-imprimir")
	if err := os.Mkdir(ajena, 0o777); err != nil {
		t.Fatal(err)
	}
	ruta, err := copiaParaImprimir(tmp, "nomina.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(ruta) == ajena {
		t.Fatal("la copia fue a la carpeta que otra cuenta había preparado")
	}
	if fi, err := os.Stat(filepath.Dir(ruta)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("la carpeta de la copia: %v %v", fi.Mode(), err)
	}
	otra, _ := copiaParaImprimir(tmp, "nomina.pdf", []byte("%PDF"))
	if filepath.Dir(otra) == filepath.Dir(ruta) {
		t.Fatal("dos impresiones comparten carpeta")
	}
}

// ConProgreso avisa una vez por punto porcentual —del 0 al 100, 101 veces— y la última
// con el total.
func TestConProgresoAvisaPorPuntoPorcentual(t *testing.T) {
	var avisos []float64
	r := ConProgreso(strings.NewReader(strings.Repeat("x", 1000)), 1000, func(f float64) { avisos = append(avisos, f) })
	b := make([]byte, 3) // 334 lecturas de 3 bytes: sin el freno serían 334 avisos
	for {
		if _, err := r.Read(b); err != nil {
			break
		}
	}
	if len(avisos) != 101 || avisos[len(avisos)-1] != 1 {
		t.Errorf("%d avisos, el último %v", len(avisos), avisos[len(avisos)-1])
	}
}

// TestGuardarQuitaLosGirosDeEscritura (FYC-04): un adjunto llamado con U+202E no
// llega así al disco.
func TestGuardarQuitaLosGirosDeEscritura(t *testing.T) {
	ruta, err := Guardar(t.TempDir(), "factura\u202Efdp.exe", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(ruta) != "facturafdp.exe" {
		t.Fatalf("guardado como %q", filepath.Base(ruta))
	}
}

func TestLeerLocalSeguro(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "prueba.pdf")
	contenido := []byte("%PDF-1.7 prueba segura")
	if err := os.WriteFile(f, contenido, 0o600); err != nil {
		t.Fatal(err)
	}

	// Como ruta normal
	leido, err := LeerLocalSeguro(f)
	if err != nil {
		t.Fatalf("no se pudo leer por ruta: %v", err)
	}
	if !bytes.Equal(leido, contenido) {
		t.Fatalf("contenido inesperado: %q", string(leido))
	}

	// Como file:// URI
	leidoURI, err := LeerLocalSeguro("file://" + f)
	if err != nil {
		t.Fatalf("no se pudo leer por file:// URI: %v", err)
	}
	if !bytes.Equal(leidoURI, contenido) {
		t.Fatalf("contenido URI inesperado: %q", string(leidoURI))
	}
}
