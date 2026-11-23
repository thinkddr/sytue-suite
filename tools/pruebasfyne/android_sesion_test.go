// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// android-sesion.py valida, cerrándose ante la duda, el catálogo público de
// firmantes finales de los builds (apps/fyne/android/session-signers.json).

// androidSesionResultado es lo que devuelve el fragmento: el catálogo leído o
// la excepción, y lo que haya pedido el caso.
type androidSesionResultado struct {
	Error    *pyRestoExcepcion   `json:"error"`
	Enabled  bool                `json:"enabled"`
	Apps     map[string][]string `json:"apps"`
	Java     string              `json:"java"`
	Products map[string]string   `json:"products"`
	Queries  string              `json:"queries"`
	Provider string              `json:"provider"`
	Demo     *pyRestoExcepcion   `json:"demo"`
}

const androidSesionCargar = `
m = cargar("android_sesion", SCRIPTS / "android-sesion.py")
`

// androidSesionOriginal es el catálogo revisado, como JSON genérico para mutarlo.
func androidSesionOriginal(t *testing.T) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(guiontest.Leer(t, "apps/fyne/android/session-signers.json")), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// androidSesionCopia es una copia profunda del catálogo.
func androidSesionCopia(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func androidSesionApp(v map[string]any, i int) map[string]any {
	return v["apps"].([]any)[i].(map[string]any)
}

// androidSesionParse escribe el catálogo en un temporal y lo lee con catalog().
func androidSesionParse(t *testing.T, valor map[string]any) androidSesionResultado {
	t.Helper()
	raw, err := json.Marshal(valor)
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(t.TempDir(), "catalog.json")
	guiontest.Escribir(t, ruta, string(raw))
	var r androidSesionResultado
	pyRestoCorrer(t, androidSesionCargar+`
try:
    enabled, apps = m.catalog(ENTRADA)
    devolver({"enabled": enabled, "apps": apps})
except Exception as e:
    devolver({"error": excepcion(e)})
`, &r, pyRestoConEntrada(t, ruta))
	return r
}

func TestAndroidSesionPinesFinalesRevisadosPorPaqueteYSinActivarEnEjecucion(t *testing.T) {
	var r androidSesionResultado
	pyRestoCorrer(t, androidSesionCargar+`
enabled, apps = m.catalog()
devolver({"enabled": enabled, "apps": apps, "java": m.java(enabled, apps), "products": m.products(apps)})
`, &r)
	if r.Enabled {
		t.Fatal("la verificación de Binder/Keystore en un dispositivo no está registrada: runtime_enabled tiene que ser false")
	}
	if len(r.Apps) != 15 {
		t.Fatalf("se esperaban 15 apps y hay %d", len(r.Apps))
	}
	pines := map[string]bool{}
	for _, ps := range r.Apps {
		for _, p := range ps {
			pines[p] = true
		}
	}
	if len(pines) != 15 {
		t.Fatalf("se esperaban 15 pines distintos y hay %d", len(pines))
	}
	if pines["975e779a17f48687ae96be7726a49c96c01180460e6873b3b3e8f303289c5f82"] {
		t.Fatal("el firmante de subida del AAB no prueba el firmante del APK")
	}
	for _, s := range []string{"HABILITADO = false", "Collections.unmodifiableMap"} {
		if !strings.Contains(r.Java, s) {
			t.Fatalf("el Java generado no contiene %q", s)
		}
	}
	valores := map[string]bool{}
	for _, app := range r.Products {
		valores[app] = true
	}
	if len(valores) != len(r.Apps) {
		t.Fatalf("los productos no cubren las apps: %v", r.Products)
	}
	for app := range r.Apps {
		if !valores[app] {
			t.Fatalf("ningún producto apunta a %s", app)
		}
	}
	if len(r.Products) != 15 {
		t.Fatalf("se esperaban 15 productos y hay %d", len(r.Products))
	}
	for producto, app := range map[string]string{"hub": "one", "docs": "docus", "sheets": "calcus", "slides": "diapos", "pdf": "certipdf"} {
		if r.Products[producto] != "eu.sytue."+app {
			t.Fatalf("producto %s → %q, no eu.sytue.%s", producto, r.Products[producto], app)
		}
		if linea := `products.put("` + producto + `", "eu.sytue.` + app + `");`; !strings.Contains(r.Java, linea) {
			t.Fatalf("el Java generado no contiene %s", linea)
		}
	}
}

func TestAndroidSesionPaquetesAusentesDuplicadosOAjenosNuncaAmplianLaConfianza(t *testing.T) {
	original := androidSesionOriginal(t)
	ausente := androidSesionCopia(t, original)
	apps := ausente["apps"].([]any)
	ausente["apps"] = apps[:len(apps)-1]
	duplicado := androidSesionCopia(t, original)
	duplicado["apps"] = append(duplicado["apps"].([]any), duplicado["apps"].([]any)[0])
	ajeno := androidSesionCopia(t, original)
	androidSesionApp(ajeno, 0)["package_name"] = "evil.app"
	for nombre, invalido := range map[string]map[string]any{"ausente": ausente, "duplicado": duplicado, "ajeno": ajeno} {
		t.Run(nombre, func(t *testing.T) {
			pyRestoLanza(t, androidSesionParse(t, invalido).Error, "ValueError", "")
		})
	}
}

func TestAndroidSesionHuellasYActivacionInvalidasSeRechazan(t *testing.T) {
	original := androidSesionOriginal(t)
	for _, valor := range []any{
		[]any{},
		[]any{""},
		[]any{"certificate"},
		[]any{strings.Repeat("A", 64)},
		[]any{strings.Repeat("0", 64), strings.Repeat("0", 64)},
	} {
		invalido := androidSesionCopia(t, original)
		androidSesionApp(invalido, 0)["certificate_sha256"] = valor
		pyRestoLanza(t, androidSesionParse(t, invalido).Error, "ValueError", "")
	}
	for _, valor := range []any{"true", 1, nil} {
		invalido := androidSesionCopia(t, original)
		invalido["runtime_enabled"] = valor
		pyRestoLanza(t, androidSesionParse(t, invalido).Error, "ValueError", "")
	}
}

func TestAndroidSesionMapaDeProductosRechazaMetadatosAusentesDuplicadosOCambiados(t *testing.T) {
	raiz := guiontest.Raiz(t)
	fuentes, err := filepath.Glob(filepath.Join(raiz, "apps", "fyne", "cmd", "*", "FyneApp.toml"))
	if err != nil || len(fuentes) == 0 {
		t.Fatalf("sin FyneApp.toml: %v", err)
	}
	for _, mutacion := range []string{"missing", "duplicate", "scheme", "package"} {
		t.Run(mutacion, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cmd")
			for _, f := range fuentes {
				guiontest.Escribir(t, filepath.Join(root, filepath.Base(filepath.Dir(f)), "FyneApp.toml"), guiontest.Leer(t, f))
			}
			docs := filepath.Join(root, "docus", "FyneApp.toml")
			texto := guiontest.Leer(t, docs)
			switch mutacion {
			case "missing":
				if err := os.Remove(docs); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				guiontest.Escribir(t, filepath.Join(root, "another", "FyneApp.toml"), texto)
			case "scheme":
				guiontest.Escribir(t, docs, strings.ReplaceAll(texto, `"sytue-docs"`, `"sytue-slides"`))
			default:
				guiontest.Escribir(t, docs, strings.ReplaceAll(texto, `"eu.sytue.docus"`, `"eu.sytue.unknown"`))
			}
			var r androidSesionResultado
			pyRestoCorrer(t, androidSesionCargar+`
_, apps = m.catalog()
try:
    m.products(apps, metadata=pathlib.Path(ENTRADA))
    devolver({})
except Exception as e:
    devolver({"error": excepcion(e)})
`, &r, pyRestoConEntrada(t, root))
			pyRestoLanza(t, r.Error, "ValueError", "")
		})
	}
}

func TestAndroidSesionMapaDeProductosRechazaProductoDeAlmacenamientoDuplicadoODesconocido(t *testing.T) {
	productos := strings.Fields(guiontest.Leer(t, "apps/nucleo/storage/products.txt"))
	for _, cambio := range []string{"duplicate", "unknown"} {
		t.Run(cambio, func(t *testing.T) {
			lista := append([]string(nil), productos...)
			if cambio == "duplicate" {
				lista[len(lista)-1] = lista[0]
			} else {
				lista[len(lista)-1] = "foreign"
			}
			catalogo := filepath.Join(t.TempDir(), "products.txt")
			guiontest.Escribir(t, catalogo, strings.Join(lista, "\n"))
			var r androidSesionResultado
			pyRestoCorrer(t, androidSesionCargar+`
_, apps = m.catalog()
try:
    m.products(apps, product_catalog=pathlib.Path(ENTRADA))
    devolver({})
except Exception as e:
    devolver({"error": excepcion(e)})
`, &r, pyRestoConEntrada(t, catalogo))
			pyRestoLanza(t, r.Error, "ValueError", "")
		})
	}
}

func TestAndroidSesionElManifiestoDeclaraSoloLosQuincePaquetesConocidos(t *testing.T) {
	var r androidSesionResultado
	pyRestoCorrer(t, androidSesionCargar+`
_, apps = m.catalog()
devolver({"queries": m.manifest(apps, "queries", "eu.sytue.mail"),
          "provider": m.manifest(apps, "components", "eu.sytue.mail"),
          "demo": fallo(m.manifest, apps, "components", "eu.sytue.fyne.demo")})
`, &r)
	if n := strings.Count(r.Queries, "<package "); n != 15 {
		t.Fatalf("queries declara %d paquetes, no 15\n%s", n, r.Queries)
	}
	if strings.Contains(r.Queries, "QUERY_ALL_PACKAGES") {
		t.Fatal("queries pide QUERY_ALL_PACKAGES")
	}
	for _, s := range []string{`android:authorities="eu.sytue.mail.sytue.sesion"`, `android:exported="false"`, `android:grantUriPermissions="false"`} {
		if !strings.Contains(r.Provider, s) {
			t.Fatalf("el proveedor no contiene %s\n%s", s, r.Provider)
		}
	}
	pyRestoLanza(t, r.Demo, "ValueError", "")
}
