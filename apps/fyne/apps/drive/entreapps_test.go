// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// sinApps es un sistema sin ninguna app de la suite instalada: el esquema no lo atiende
// nadie y solo se abre la web.
type sinApps struct{ urls []string }

func (a *sinApps) OpenURL(u *url.URL) error {
	if strings.HasPrefix(u.Scheme, "sytue-") {
		return errors.New("nadie atiende " + u.Scheme)
	}
	a.urls = append(a.urls, u.String())
	return nil
}

// hacer pulsa la acción del menú que lleva ese rótulo.
func hacer(t *testing.T, p *Pantalla, el Elemento, clave string) {
	t.Helper()
	for _, a := range p.AccionesDe(el, false, false) {
		if a.texto == p.tr.T(clave, nil) {
			a.hacer()
			return
		}
	}
	t.Fatalf("el menú de %s no tiene %q", el.Nombre, clave)
}

// «Adjuntar a un correo» y «Compartir en Rooms» llaman a la OTRA app por su esquema con
// el fichero por referencia —los parámetros del Hub— y, sin la app instalada, caen a la
// misma página en la web, que existe (`/mail/compose` no: era un 404).
func TestLlevarseUnFicheroAOtraApp(t *testing.T) {
	a := &abridorDoble{}
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.App, p.Apex = a, "sytue.eu"
	acta := Elemento{ID: "f1", Nombre: "acta.odt"}
	hacer(t, p, acta, "action.attachToMail")
	hacer(t, p, acta, "action.shareToRooms")
	p.explor.Ruta = nil
	hacer(t, p, Elemento{ID: "c1", Nombre: "Contratos", Carpeta: true}, "action.shareToRooms")
	quiero := []string{
		"sytue-mail:/?action=compose&attach_drive=f1&subject=acta.odt",
		"sytue-rooms:/share?action=share&share_id=f1&share_source=drive&text=acta.odt%0Ahttps%3A%2F%2Fdrive.sytue.eu%2Fdrive%2Ffiles%2Ff1",
		// Una carpeta se enlaza por su ruta, como `driveFolderUrl`.
		"sytue-rooms:/share?action=share&share_id=c1&share_source=drive&text=Contratos%0Ahttps%3A%2F%2Fdrive.sytue.eu%2Fdrive%2F%3Fpath%3D%252FContratos",
	}
	if strings.Join(a.urls, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("se abre\n%s\ny tiene que ser\n%s", strings.Join(a.urls, "\n"), strings.Join(quiero, "\n"))
	}

	web := &sinApps{}
	p.App = web
	hacer(t, p, acta, "action.attachToMail")
	hacer(t, p, acta, "action.shareToRooms")
	if len(web.urls) != 2 || web.urls[0] != "https://mail.sytue.eu/mail/?action=compose&attach_drive=f1&subject=acta.odt" ||
		!strings.HasPrefix(web.urls[1], "https://rooms.sytue.eu/rooms/share?action=share&share_id=f1") {
		t.Errorf("sin las apps instaladas se abre %v", web.urls)
	}
}

// «Subir a Cast» abre la pantalla de Sytue Cast con el fichero, como `urlDeSubirACast`
// de la web; sin la app, su web.
func TestSubirUnVideoACast(t *testing.T) {
	a := &abridorDoble{}
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.App, p.Apex = a, "sytue.eu"
	video := Elemento{ID: "0b6f7a2e-3c1d-4e5f-8a9b-0c1d2e3f4a5b", Nombre: "demo.mp4", Tipo: "video/mp4"}
	hacer(t, p, video, "action.uploadToCast")
	if len(a.urls) != 1 || a.urls[0] != "sytue-cast:/upload?drive=0b6f7a2e-3c1d-4e5f-8a9b-0c1d2e3f4a5b" {
		t.Errorf("se abre %v", a.urls)
	}
	web := &sinApps{}
	p.App = web
	hacer(t, p, video, "action.uploadToCast")
	if len(web.urls) != 1 || web.urls[0] != "https://cast.sytue.eu/cast/upload?drive=0b6f7a2e-3c1d-4e5f-8a9b-0c1d2e3f4a5b" {
		t.Errorf("sin la app se abre %v", web.urls)
	}
	for _, el := range []Elemento{{ID: "f1", Nombre: "acta.odt", Tipo: "application/vnd.oasis.opendocument.text"}, {ID: "c1", Nombre: "Vídeos", Carpeta: true, Tipo: "video/mp4"}} {
		if SubibleACast(el) {
			t.Errorf("%s se ofrece para Cast", el.Nombre)
		}
	}
}
