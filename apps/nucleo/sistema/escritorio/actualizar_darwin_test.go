// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && !tienda

package escritorio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestElDMGReemplazaAlBundle: el .app del DMG queda en lugar del instalado, sin copias a
// medias al lado, el DMG se desmonta y se abre la app nueva. VISTO EN ROJO sin el
// segundo rename (el instalado desaparecía y quedaba solo el `.nuevo`).
func TestElDMGReemplazaAlBundle(t *testing.T) {
	raiz := t.TempDir()
	hacerApp := func(dir, nombre, exe, contenido string) {
		t.Helper()
		macos := filepath.Join(dir, nombre+".app", "Contents", "MacOS")
		if err := os.MkdirAll(macos, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(macos, exe), []byte(contenido), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hacer := func(dir, contenido string) { hacerApp(dir, "Sytue Prueba", "sytue-prueba", contenido) }
	// El DMG es el de un BUNDLE: más de un .app, y se toma el de su mismo nombre. Visto en
	// rojo quitando la búsqueda por nombre («el DMG no trae…» con dos .app dentro).
	escenario := filepath.Join(raiz, "escenario")
	hacer(escenario, "nuevo")
	hacerApp(escenario, "Sytue Otra", "sytue-otra", "otra")
	dmg := filepath.Join(raiz, "sytue-prueba-macos-universal.dmg")
	if salida, err := exec.Command("hdiutil", "create", "-quiet", "-volname", "Sytue Prueba", "-srcfolder", escenario,
		"-format", "UDZO", "-fs", "HFS+", dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil no deja crear el DMG de prueba: %v %s", err, salida)
	}
	instalada := filepath.Join(raiz, "Aplicaciones")
	hacer(instalada, "viejo")
	bundle := filepath.Join(instalada, "Sytue Prueba.app")

	var abierto string
	if err := instalarEn(context.Background(), dmg, bundle, func(b string) { abierto = b }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bundle, "Contents", "MacOS", "sytue-prueba")); string(b) != "nuevo" {
		t.Errorf("la app instalada sigue siendo %q", b)
	}
	if restos, _ := filepath.Glob(bundle + ".*"); len(restos) != 0 {
		t.Errorf("quedan copias al lado: %v", restos)
	}
	if abierto != bundle {
		t.Errorf("se ha abierto %q", abierto)
	}
}

// TestLasHermanasDelMacSonLasAppsDeLaSuiteDeSuCarpeta: junto al Hub, los .app cuyo
// ejecutable es `sytue-<app>`, con la versión de su Info.plist; ni él mismo ni una app
// cualquiera. VISTO EN ROJO sin saltarse el bundle propio.
func TestLasHermanasDelMacSonLasAppsDeLaSuiteDeSuCarpeta(t *testing.T) {
	raiz := t.TempDir()
	app := func(nombre, exe, plist string) string {
		t.Helper()
		macos := filepath.Join(raiz, nombre+".app", "Contents", "MacOS")
		if err := os.MkdirAll(macos, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(macos, exe), []byte("x"), 0o755)
		if plist != "" {
			_ = os.WriteFile(filepath.Join(raiz, nombre+".app", "Contents", "Info.plist"), []byte(plist), 0o644)
		}
		return filepath.Join(macos, exe)
	}
	hub := app("Sytue Hub", "sytue-hub", "")
	app("Sytue Mail", "sytue-mail", "<plist><dict><key>CFBundleShortVersionString</key>\n\t<string>3.5.0</string></dict></plist>")
	app("Otra", "otra", "")
	// La de la Mac App Store tampoco: la pone al día la tienda.
	app("Sytue Drive", "sytue-drive", "")
	recibo := filepath.Join(raiz, "Sytue Drive.app", "Contents", "_MASReceipt")
	if err := os.MkdirAll(recibo, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(recibo, "receipt"), []byte("x"), 0o644)
	got := hermanasDe(hub)
	if len(got) != 1 || got[0].App != "mail" || got[0].Version != "3.5.0" ||
		got[0].Destino != filepath.Join(raiz, "Sytue Mail.app") {
		t.Fatalf("hermanas = %+v", got)
	}
}
