// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// EjemploEnlace es una fila de testdata/enlaces.json: un enlace de aviso y el producto
// cuya app nativa lo tiene que abrir. Lo lee también el guardián de apps/nucleo/api.
type EjemploEnlace struct {
	Tipo     string `json:"tipo"`
	Producto string `json:"producto"`
	URL      string `json:"url"`
}

const (
	pub = "https://mail.sytue.eu/cualquier/ruta" // la ruta del PUBLIC_URL no cuenta
	uid = "0b6a4c8e-1f2d-4e3a-9b5c-7d8e9f0a1b2c"
	// La organización que aloja el recurso en los ejemplos, la misma que lleva la
	// entrada de aviso (`Notification.Tenant`): tras el prefijo de la app (ADR 0056).
	org = "acme"
)

func ejemplos() []EjemploEnlace {
	return []EjemploEnlace{
		{"mail.message.received", "mail", EnlaceHilo(pub, org, uid)},
		{"mail.access.requested", "mail", EnlaceBuzon(pub, org, uid)},
		{"calendar.reminder.due", "calendar", EnlaceEvento(pub, org, uid)},
		{"calendar.invitation", "calendar", EnlaceInvitaciones(pub, org)},
		{"calendar.access.requested", "calendar", EnlaceCalendario(pub, org, uid)},
		{"rooms.mention", "rooms", EnlaceMensaje(pub, org, uid, uid)},
		{"rooms.report", "rooms", EnlaceDenuncias(pub, org)},
		{"rooms.invitation", "rooms", EnlaceInvitacionesRooms(pub, org)},
		{"rooms.invitation.direct", "rooms", EnlaceInvitacionesRooms(pub, org)},
		{"drive.share.created", "drive", EnlaceCompartido(pub, org, uid)},
		{"drive.access.requested", "drive", EnlacePeticionDrive(pub, org, uid)},
		{"docs.shared", "docs", EnlaceDocumento(pub, org, uid, "document")},
		{"docs.shared", "sheets", EnlaceDocumento(pub, org, uid, "spreadsheet")},
		{"docs.shared", "slides", EnlaceDocumento(pub, org, uid, "presentation")},
		{"docs.comment", "docs", EnlaceDocumento(pub, org, uid, "document")},
		{"docs.signature.requested", "pdf", EnlacePDF(pub, org, "/PDF/Contrato de servicios.pdf")},
		{"erp.task.assigned", "erp", EnlaceTareas(pub, org)},
		{"erp.access.requested", "erp", EnlacePeticionesERP(pub, org, "clientes")},
		{"notes.access.requested", "notes", EnlaceCuaderno(pub, org, uid)},
		{"notes.shared", "notes", EnlaceCuaderno(pub, org, uid)},
		{"notes.shared", "notes", EnlaceNotaCompartida(pub, org, uid)},
		{"notes.reminder.due", "notes", EnlaceNota(pub, org, uid, uid)},
		{"notes.reminder.due", "notes", EnlaceNotaCompartida(pub, org, uid)},
		{"notes.mention", "notes", EnlaceNota(pub, org, uid, uid)},
		{"notes.mention", "notes", EnlaceNotaCompartida(pub, org, uid)},
		{"photos.shared", "photos", EnlaceAlbumCompartido(pub, org, uid)},
		{"photos.album.added", "photos", EnlaceAlbum(pub, org, uid)},
		{"photos.album.renamed", "photos", EnlaceAlbum(pub, org, uid)},
	}
}

// Cada enlace de aviso abre algo en la web: la página existe y lee los parámetros que
// lleva. Y el fichero de ejemplos que usa el guardián de las apps nativas es el que
// sale del código, para que no se queden atrás el uno del otro.
//
// Es el guardián de las URL rotas del 2026-09-24: `/#msg-…` (la web no lee el
// fragmento), `/shared` (no es una ruta) y un host `.sytue.localhost`.
func TestLosEnlacesDeLosAvisosAbrenAlgoEnLaWeb(t *testing.T) {
	quiero, _ := json.MarshalIndent(ejemplos(), "", "  ")
	quiero = append(quiero, '\n')
	fichero := filepath.Join("testdata", "enlaces.json")
	if tengo, err := os.ReadFile(fichero); err != nil || !bytes.Equal(tengo, quiero) {
		t.Errorf("%s no es lo que sale del código; debería ser:\n%s", fichero, quiero)
	}
	for _, e := range ejemplos() {
		u, err := url.Parse(e.URL)
		app, resto, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
		// El host del servicio; la ofimática, el de su producto en el mismo dominio (ADR 0076).
		host := "mail.sytue.eu"
		if ofimatica[app] {
			host = app + ".sytue.eu"
		}
		if err != nil || u.Host != host || u.Fragment != "" {
			t.Errorf("%s: %q no es un enlace del host %s sin fragmento", e.Tipo, e.URL, host)
			continue
		}
		// La organización va tras el prefijo y el middleware la REESCRIBE por dentro
		// (ADR 0056): la página que sirve es la que queda sin ella — vacía en las
		// raíces, como `/calendar/<org>/`—. Que vaya justo detrás del prefijo es lo
		// que este guardián ata.
		sinOrg, conOrg := strings.CutPrefix(resto, org+"/")
		if !conOrg {
			t.Errorf("%s: %q no lleva la organización %q tras el prefijo", e.Tipo, u.Path, org)
			continue
		}
		resto = sinOrg
		raiz := filepath.Join("..", "..", "..", "apps", "frontend", "apps", app, "src")
		if !paginaExiste(filepath.Join(raiz, "pages"), resto) {
			t.Errorf("%s: la web de %q no tiene página para %q", e.Tipo, app, resto)
		}
		for k := range u.Query() {
			// Las páginas de la ofimática montan las vistas de su paquete, que es quien lee.
			if ofimatica[app] && leeParametro(t, filepath.Join("..", "..", "..", "apps", "frontend", "packages", "office", "src"), k) {
				continue
			}
			if !leeParametro(t, raiz, k) {
				t.Errorf("%s: la web de %q no lee ?%s=", e.Tipo, app, k)
			}
		}
	}
}

// El enlace de un comentario de Bases: su página existe en la web y lee `?table=` y
// `?record=`. Fuera de testdata/enlaces.json hasta que Bases tenga app nativa.
// ofimatica son las apps de Docus, Calcus, Diapos y CertiPDF: host propio y vistas en
// `@sytue/office`.
var ofimatica = map[string]bool{"docus": true, "calcus": true, "diapos": true, "certipdf": true}

func TestEnlaceRegistroDeBase(t *testing.T) {
	got := EnlaceRegistroDeBase(pub, org, uid, uid, uid)
	if quiero := "https://mail.sytue.eu/bases/acme/base/" + uid + "?record=" + uid + "&table=" + uid; got != quiero {
		t.Fatalf("EnlaceRegistroDeBase = %q, quiero %q", got, quiero)
	}
	if got, quiero := EnlaceBase(pub, org, uid), "https://mail.sytue.eu/bases/acme/base/"+uid; got != quiero {
		t.Fatalf("EnlaceBase = %q, quiero %q", got, quiero)
	}
	raiz := filepath.Join("..", "..", "..", "apps", "frontend", "apps", "bases", "src")
	if !paginaExiste(filepath.Join(raiz, "pages"), "base/"+uid) {
		t.Error("la web de Bases no tiene página para base/<id>")
	}
	for _, k := range []string{"table", "record"} {
		if !leeParametro(t, raiz, k) {
			t.Errorf("la web de Bases no lee ?%s=", k)
		}
	}
}

// Sin organización el enlace sale en la forma transicional, sin segmento: el middleware
// de la web le añade la de la sesión.
func TestSinOrganizacionElEnlaceVaSinSegmento(t *testing.T) {
	quiero := "https://mail.sytue.eu/mail/?thread=" + uid
	if got := EnlaceHilo(pub, "", uid); got != quiero {
		t.Errorf("EnlaceHilo sin org = %q, quiero %q", got, quiero)
	}
}

// paginaExiste recorre el árbol de páginas de Astro como lo hace su enrutador: un
// segmento es un fichero o carpeta con su nombre, o un `[parametro]`.
func paginaExiste(dir, ruta string) bool {
	seg, resto, hay := strings.Cut(strings.Trim(ruta, "/"), "/")
	if seg == "" {
		return existe(filepath.Join(dir, "index.astro")) || existe(filepath.Join(dir, "index.ts"))
	}
	entradas, _ := os.ReadDir(dir)
	for _, en := range entradas {
		nombre := strings.TrimSuffix(strings.TrimSuffix(en.Name(), ".astro"), ".ts")
		if nombre != seg && (!strings.HasPrefix(nombre, "[") || !strings.HasSuffix(nombre, "]")) {
			continue
		}
		if en.IsDir() && paginaExiste(filepath.Join(dir, en.Name()), resto) {
			return true
		}
		if !en.IsDir() && !hay {
			return true
		}
	}
	return false
}

func existe(p string) bool { _, err := os.Stat(p); return err == nil }

// leeParametro dice si algún fuente de la app lee `?k=`.
func leeParametro(t *testing.T, raiz, k string) bool {
	t.Helper()
	patron := regexp.MustCompile(`\.get\(\s*['"]` + regexp.QuoteMeta(k) + `['"]\s*\)`)
	visto := false
	_ = filepath.WalkDir(raiz, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // un fichero ilegible no lee el parámetro; se sigue
		}
		if visto || d.IsDir() || (!strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".tsx") && !strings.HasSuffix(p, ".astro")) {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && patron.Match(b) {
			visto = true
		}
		return nil
	})
	return visto
}

// Un PUBLIC_URL inservible no da un enlace roto: no da ninguno.
func TestSinPublicURLNoHayEnlace(t *testing.T) {
	for _, pub := range []string{"", "mail.sytue.eu", "javascript:alert(1)", "://x"} {
		if got := EnlaceHilo(pub, org, uid); got != "" {
			t.Errorf("EnlaceHilo(%q) = %q", pub, got)
		}
	}
}

// Los enlaces de Pages llevan la organización tras el prefijo y la consulta de la
// ruta, y una ruta que intente salir de la app no sale del host.
func TestEnlacePages(t *testing.T) {
	for ruta, quiero := range map[string]string{
		"site/" + uid:                  "https://mail.sytue.eu/web/acme/site/" + uid,
		"/site/" + uid + "?tab=orders": "https://mail.sytue.eu/web/acme/site/" + uid + "?tab=orders",
		"":                             "https://mail.sytue.eu/web/acme/",
		"site/x?%zz":                   "",
	} {
		if got := EnlacePages(pub, org, ruta); got != quiero {
			t.Errorf("EnlacePages(%q) = %q, quiero %q", ruta, got, quiero)
		}
	}
	u, err := url.Parse(EnlacePages(pub, org, "//evil.example/x"))
	if err != nil || u.Host != "mail.sytue.eu" {
		t.Errorf("una ruta con // cambia el host: %v %v", u, err)
	}
}
