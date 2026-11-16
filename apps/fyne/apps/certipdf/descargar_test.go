// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// botonesDelGrupoDocumento son los rótulos del grupo «Documento» de Organizar, con la
// acción de cada uno, quitada la pista si la lleva.
func botonesDelGrupoDocumento(t *testing.T, b *Barra) ([]string, []*Accion) {
	t.Helper()
	b.Elegir(PestOrganizar)
	for _, g := range gruposDe(b) {
		if g.Rotulo != "pdf.ribbon.group.document" {
			continue
		}
		var rotulos []string
		var acciones []*Accion
		for _, c := range g.Controles {
			if a, ok := sinPista(c).(*Accion); ok {
				rotulos, acciones = append(rotulos, a.Texto), append(acciones, a)
			}
		}
		return rotulos, acciones
	}
	t.Fatal("Organizar no lleva el grupo «Documento»")
	return nil, nil
}

// TestLaCintaLlevaDescargarEImprimir: «Descargar» e «Imprimir» salen en el grupo
// «Documento» de Organizar, detrás de los cuatro diálogos, y cada uno llama a lo suyo.
// Sin ellos solo se imprimía con Ctrl+P —que nadie adivina— y no había forma de sacar
// una copia fuera de Firmar. VISTO EN ROJO sin los botones.
func TestLaCintaLlevaDescargarEImprimir(t *testing.T) {
	// Sin t.Parallel: mide texto (ver TestElGrupoDeTrazoEsElDeLaWeb).
	var descargas, impresiones int
	b := NuevaBarra(identidad)
	b.AlDescargar, b.AlImprimir = func() { descargas++ }, func() { impresiones++ }
	rotulos, acciones := botonesDelGrupoDocumento(t, b)
	quiero := []string{"pdf.menu.organize", "pdf.menu.tools", "pdf.menu.stamp", "pdf.menu.compare", "pdf.menu.versions", "pdf.menu.download", "pdf.menu.print"}
	if !slices.Equal(rotulos, quiero) {
		t.Fatalf("el grupo «Documento» lleva %v; quiero %v", rotulos, quiero)
	}
	acciones[5].Tapped(nil)
	acciones[6].Tapped(nil)
	if descargas != 1 || impresiones != 1 {
		t.Errorf("pulsar «Descargar» e «Imprimir» llamó %d y %d veces", descargas, impresiones)
	}

	// Lo que el documento prohíbe se ve apagado y con el MOTIVO de pista.
	b.PuedeImprimir = false
	b.Elegir(PestOrganizar)
	for _, g := range gruposDe(b) {
		if g.Rotulo != "pdf.ribbon.group.document" {
			continue
		}
		ultimo := g.Controles[len(g.Controles)-1]
		if a, ok := sinPista(ultimo).(*Accion); !ok || a.Texto != "pdf.menu.print" || !a.Inerte {
			t.Errorf("sin permiso, «Imprimir» no sale apagado: %#v", sinPista(ultimo))
		}
		if p, ok := ultimo.(*ui.Pista); !ok || p.Rotulo != "pdf.blocked.print" {
			t.Errorf("sin permiso, «Imprimir» no dice por qué está apagado: %#v", ultimo)
		}
	}

	// En el móvil no hay impresión: el botón no sale. «Descargar», sí.
	b.PuedeImprimir, b.SinImpresion = true, true
	if rotulos, _ = botonesDelGrupoDocumento(t, b); slices.Contains(rotulos, "pdf.menu.print") || !slices.Contains(rotulos, "pdf.menu.download") {
		t.Errorf("en el móvil el grupo «Documento» lleva %v", rotulos)
	}

	// Sin ganchos, ninguno de los dos: un botón que no hace nada es peor que no tenerlo.
	if rotulos, _ = botonesDelGrupoDocumento(t, NuevaBarra(identidad)); len(rotulos) != 5 {
		t.Errorf("sin ganchos el grupo «Documento» lleva %v", rotulos)
	}
}

// TestImprimirDesdeLaCintaRespetaElPermiso: el botón de la pantalla manda al espía el PDF
// abierto, y con un documento que pide no imprimirse no lo hace ni el botón ni Ctrl+P
// (el atajo saltaba el permiso). VISTO EN ROJO sin la comprobación en `imprimir`.
func TestImprimirDesdeLaCintaRespetaElPermiso(t *testing.T) {
	p, _, _, cerrar := abrirParaAnotar(t)
	defer cerrar()
	w := test.NewWindow(nil)
	defer w.Close()
	fyne.DoAndWait(func() { p.Atajos(w.Canvas()) })

	impreso := make(chan string, 4)
	imprimirPDF = func(nombre string, datos []byte) error {
		impreso <- nombre + " " + string(datos)
		return nil
	}
	defer func() { imprimirPDF = ficheros.ImprimirDatos }()

	if !p.barra.PuedeImprimir || p.barra.AlImprimir == nil {
		t.Fatal("un documento sin restricciones no ofrece «Imprimir»")
	}
	fyne.DoAndWait(p.barra.AlImprimir)
	select {
	case got := <-impreso:
		if got != "a.pdf %PDF-" {
			t.Errorf("«Imprimir» ha mandado %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("«Imprimir» no ha mandado nada a imprimir")
	}

	// Abierto con la contraseña de usuario y sin el permiso: nada se imprime.
	fyne.DoAndWait(func() {
		p.Contenido.Permisos = Permisos{SoloConClaveDeDueno: true}
		p.Recomponer()
	})
	if p.barra.PuedeImprimir {
		t.Error("la cinta ofrece imprimir un documento que pide no imprimirse")
	}
	fyne.DoAndWait(func() {
		p.barra.AlImprimir()
		p.atajos[fyne.KeyP]()
	})
	select {
	case got := <-impreso:
		t.Errorf("se ha impreso %q de un documento que lo prohíbe", got)
	case <-time.After(200 * time.Millisecond):
	}

	// Con la del dueño no hay restricción, como `puedeAnotar`.
	if !puedeImprimir(Permisos{SoloConClaveDeDueno: false}) {
		t.Error("con la contraseña del dueño no se puede imprimir")
	}
}

// TestDescargarEscribeElPDFTalComoEsta: «Descargar» deja en descargas los bytes ACTUALES
// del documento —con lo aplicado—, con su nombre, sin pisar otro que se llame igual, y lo
// dice en la franja verde; si no se puede, en la roja.
func TestDescargarEscribeElPDFTalComoEsta(t *testing.T) {
	dir := t.TempDir()
	carpetaDescargas = func() (string, error) { return dir, nil }
	defer func() { carpetaDescargas = ficheros.Carpeta }()

	p, _, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	// Lo aplicado cambia los bytes del documento: se descarga lo de AHORA, no lo abierto.
	p.doc.mu.Lock()
	p.doc.datos = []byte("%PDF-1.7 anotado")
	p.doc.mu.Unlock()
	if err := os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("otro"), 0o600); err != nil {
		t.Fatal(err)
	}

	fyne.DoAndWait(p.barra.AlDescargar)
	esperarRed(t, listo)
	entradas, _ := os.ReadDir(dir)
	var nuevo string
	for _, e := range entradas {
		if e.Name() != "a.pdf" {
			nuevo = e.Name()
		}
	}
	if nuevo == "" {
		t.Fatalf("no se ha escrito nada nuevo en descargas: %v", entradas)
	}
	if datos, _ := os.ReadFile(filepath.Join(dir, nuevo)); string(datos) != "%PDF-1.7 anotado" {
		t.Errorf("se ha descargado %q", datos)
	}
	if datos, _ := os.ReadFile(filepath.Join(dir, "a.pdf")); string(datos) != "otro" {
		t.Error("«Descargar» ha pisado un fichero que ya estaba")
	}
	if !strings.Contains(p.Aviso, nuevo) || p.Error != "" {
		t.Errorf("tras descargar la franja dice %q / %q", p.Aviso, p.Error)
	}

	carpetaDescargas = func() (string, error) { return "", os.ErrNotExist }
	fyne.DoAndWait(p.barra.AlDescargar)
	esperarRed(t, listo)
	if p.Error == "" || p.Aviso != "" {
		t.Errorf("un fallo al descargar no se dice: %q / %q", p.Aviso, p.Error)
	}
}
