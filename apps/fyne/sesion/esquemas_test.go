// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// productoDe es qué clave del catálogo es cada binario. No coinciden los nombres: la
// clave es la de la WEB —`docs`, `sheets`, `slides`, `pdf`—, que es la que usan el
// emisor (`api.Esquema`), el `PRODUCT_SCHEMES` del frontend y los redirect_uri que Sytue
// ID tiene dados de alta (migración 0071); el directorio lleva el NOMBRE COMERCIAL, que
// es otra cosa (Sytue Docus edita `docs`).
var productoDe = map[string]string{
	"calcus": "sheets", "calendar": "calendar", "certipdf": "pdf", "contacts": "contacts",
	"diapos": "slides", "docus": "docs", "drive": "drive", "erp": "erp", "hub": "one",
	"mail": "mail", "notes": "notes", "photos": "photos", "rooms": "rooms", "cast": "cast",
	"bases": "bases",
}

// TestCadaAppDeclaraElEsquemaDeSuProducto ata el esquema por los DOS lados. Es el mismo
// dato en dos sitios que no se leen entre sí:
//
//   - `FyneApp.toml`, que es lo que la cadena de empaquetado registra en el sistema
//     (`intent-filter`, `x-scheme-handler`, `CFBundleURLTypes`) y lo que la app usa como
//     redirect_uri al entrar desde el móvil;
//   - `api.Esquema`, que es con lo que las OTRAS apps la llaman.
//
// Separados, no falla nada visible: el sistema no encuentra manejador para
// `sytue-docs:` y Drive cae al navegador, que es exactamente lo que parece «la app
// nativa no está instalada». Así estuvieron Docus, Calcus, Diapos y CertiPDF hasta el
// 2026-09-22, declarando `sytue-docus`, `sytue-calcus`, `sytue-diapos` y
// `sytue-certipdf`, que no los llama nadie.
func TestCadaAppDeclaraElEsquemaDeSuProducto(t *testing.T) {
	dirs, err := os.ReadDir("../cmd")
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, d := range dirs {
		// La demo no es un producto de la suite y tiene su propio esquema.
		if !d.IsDir() || d.Name() == "demo" {
			continue
		}
		clave, ok := productoDe[d.Name()]
		if !ok {
			t.Errorf("cmd/%s no dice de qué producto es: añádelo a productoDe", d.Name())
			continue
		}
		crudo, err := os.ReadFile(filepath.Join("..", "cmd", d.Name(), "FyneApp.toml"))
		if err != nil {
			t.Errorf("cmd/%s: %v", d.Name(), err)
			continue
		}
		quiero := `esquema = "` + api.Esquema(clave) + `"`
		if !strings.Contains(string(crudo), quiero) {
			t.Errorf("cmd/%s/FyneApp.toml no declara %s", d.Name(), quiero)
		}
		vistos++
	}
	if vistos != len(productoDe) {
		t.Errorf("se han mirado %d binarios y la tabla tiene %d", vistos, len(productoDe))
	}
}

// TestCadaAppDeclaraLasExtensionesDeSuProducto ata «Abrir con» por los TRES lados que
// tienen que decir lo mismo y no se leen entre sí:
//
//   - `FyneApp.toml`, `extensiones`: lo que el empaquetado declara al sistema;
//   - `api.Extensiones`: lo que la web abre en ese producto;
//   - que el `main` llame a `AceptarFicheros`: sin eso la app se ofrece para un `.docx`
//     y, al elegirla, lo ignora.
//
// Una app sin `extensiones` no se ofrece para nada, y está bien; lo que no puede es
// declarar a medias ni declarar sin atender.
func TestCadaAppDeclaraLasExtensionesDeSuProducto(t *testing.T) {
	declaran := 0
	for dir, clave := range productoDe {
		toml, err := os.ReadFile(filepath.Join("..", "cmd", dir, "FyneApp.toml"))
		if err != nil {
			t.Fatal(err)
		}
		principal, err := os.ReadFile(filepath.Join("..", "cmd", dir, "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		declaradas := ""
		for _, linea := range strings.Split(string(toml), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(linea), "extensiones = "); ok {
				declaradas = strings.Trim(v, `"`)
			}
		}
		atiende := strings.Contains(string(principal), ".AceptarFicheros(")
		switch {
		case declaradas == "" && atiende:
			t.Errorf("cmd/%s acepta ficheros y su FyneApp.toml no declara extensiones: el sistema nunca se la ofrecerá", dir)
		case declaradas != "" && !atiende:
			t.Errorf("cmd/%s declara %q y no llama a AceptarFicheros: se ofrecería y no abriría nada", dir, declaradas)
		case declaradas != "":
			declaran++
			if quiero := strings.Join(api.Extensiones(clave), ","); declaradas != quiero {
				t.Errorf("cmd/%s declara %q y la web abre %q en %s", dir, declaradas, quiero, clave)
			}
		}
	}
	// Docus, Calcus y Diapos. Si baja, alguien ha quitado una sin querer.
	if declaran < 3 {
		t.Errorf("solo %d apps se ofrecen en «Abrir con»; eran tres", declaran)
	}
}

// reIcono es la ruta del icono en `FyneApp.toml`.
var reIcono = regexp.MustCompile(`(?m)^\s*Icon = "([^"]+)"`)

// TestElIconoDeCadaAppEsSuMarca: el icono que enseña el SISTEMA —lanzador, Dock, barra
// de tareas, `.desktop`— es el de SU producto y de la familia vigente, la marca de
// `branding/marks` que pinta el selector de aplicaciones de la web (`ProductIcon`), y no
// el de la suite. Hasta el 2026-09-23 nueve de las trece apuntaban al del lanzador,
// que es la marca de Sytue Hub.
//
// Cada app lleva su `Icon.png` al lado del FyneApp.toml. Lo generaba el `gen-apps.sh`
// del escritorio de Wails, retirado el 2026-09-23; hoy se regenera con la orden que da
// el fallo. Se mira su COLOR: la esquina de arriba a la izquierda es la tesela, del
// `--sytue-product-<clave>`. Así se ve también un icono viejo, que es de la otra
// familia y de otro color.
func TestElIconoDeCadaAppEsSuMarca(t *testing.T) {
	for dir, clave := range productoDe {
		crudo, err := os.ReadFile(filepath.Join("..", "cmd", dir, "FyneApp.toml"))
		if err != nil {
			t.Errorf("cmd/%s: %v", dir, err)
			continue
		}
		m := reIcono.FindStringSubmatch(string(crudo))
		if m == nil {
			t.Errorf("cmd/%s/FyneApp.toml no declara icono", dir)
			continue
		}
		if m[1] != "Icon.png" {
			t.Errorf("cmd/%s usa el icono %s y el suyo es Icon.png, a su lado", dir, m[1])
			continue
		}
		f, err := os.Open(filepath.Join("..", "cmd", dir, m[1]))
		if err != nil {
			t.Errorf("cmd/%s: %v", dir, err)
			continue
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Errorf("cmd/%s: %v", dir, err)
			continue
		}
		got := color.NRGBAModel.Convert(img.At(8, 8)).(color.NRGBA)
		quiero := tokens.Color("product-"+clave, tokens.Light)
		d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
		if d(got.R, quiero.R) > 2 || d(got.G, quiero.G) > 2 || d(got.B, quiero.B) > 2 {
			t.Errorf("el icono de cmd/%s es %v y la tesela de %s es %v: no es su marca vigente.\n"+
				"  Se regenera desde la raíz con scripts/gen-iconos-fyne.sh", dir, got, clave, quiero)
			continue
		}
		// Y el CONTORNO del selector (`--sytue-product-tile-ring` del tema oscuro), que el
		// guion pinta a 3,5-4,5 unidades del glifo: en (512, 152) el píxel es la tesela
		// aclarada, no la tesela. Sin él, el icono del sistema no es el del selector.
		borde := color.NRGBAModel.Convert(img.At(512, 152)).(color.NRGBA)
		if int(borde.R)+int(borde.G)+int(borde.B) < int(got.R)+int(got.G)+int(got.B)+30 {
			t.Errorf("el icono de cmd/%s no lleva el contorno de la tesela (en 512,152 es %v y la tesela %v).\n"+
				"  Se regenera desde la raíz con scripts/gen-iconos-fyne.sh", dir, borde, got)
		}
	}
}

// TestCadaEsquemaMovilEstaDadoDeAlta: cada app que se empaqueta para el teléfono vuelve
// del login por `<esquema>://auth/callback`, y Sytue ID solo lo acepta si está entre los
// redirect_uri del client móvil (`sytue-mobile`). Sin él la app ni siquiera llega a pedir
// la contraseña: el navegador enseña un 400. Rooms y el Hub estuvieron así hasta la
// migración 0103 (medido contra id.sytue.eu el 2026-09-28).
//
// VISTO EN ROJO sin la 0103: faltan sytue-rooms y sytue-one.
func TestCadaEsquemaMovilEstaDadoDeAlta(t *testing.T) {
	migraciones, err := filepath.Glob("../../../services/id/db/migrations/*.up.sql")
	if err != nil || len(migraciones) == 0 {
		t.Fatalf("no se encuentran las migraciones de Sytue ID: %v", err)
	}
	var todo strings.Builder
	for _, m := range migraciones {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		todo.Write(b)
	}
	for dir, clave := range productoDe {
		vuelta := api.Esquema(clave) + "://auth/callback"
		if !strings.Contains(todo.String(), "'"+vuelta+"'") {
			t.Errorf("cmd/%s vuelve por %s y ninguna migración de Sytue ID lo da de alta en sytue-mobile", dir, vuelta)
		}
	}
}
