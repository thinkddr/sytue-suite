// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

// Tarjetas como las exportan de verdad Google Contacts y Contactos de Apple (vCard 3.0,
// con los TYPE apilados, grupos `itemN.` y sus X-*). La de Apple lleva la foto binaria,
// que se rellena en el test con un JPEG de verdad.
const (
	tarjetaGoogle = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ana Ruiz\r\nN:Ruiz;Ana;;;\r\n" +
		"EMAIL;TYPE=INTERNET;TYPE=WORK:ana.ruiz@acme.es\r\nEMAIL;TYPE=INTERNET;TYPE=HOME:ana@gmail.com\r\n" +
		"TEL;TYPE=CELL:+34 600 11 22 33\r\nADR;TYPE=WORK:;;Calle Mayor 1;Madrid;;28013;España\r\n" +
		"ORG:Acme S.L.\r\nTITLE:Directora Comercial\r\nBDAY:1985-02-15\r\n" +
		"item1.URL:https\\://acme.es\r\nitem1.X-ABLabel:\r\nNOTE:Conocida en la feria\r\n" +
		"CATEGORIES:myContacts,Clientes\r\nEND:VCARD\r\n"
	tarjetaApple = "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Apple Inc.//macOS 15.0//EN\r\n" +
		"N:Lovelace;Ada;;;\r\nFN:Ada Lovelace\r\nORG:Analytical Engines;\r\n" +
		"EMAIL;type=INTERNET;type=HOME;type=pref:ada@example.com\r\n" +
		"TEL;type=CELL;type=VOICE;type=pref:+44 20 7946 0000\r\n" +
		"item1.ADR;type=HOME;type=pref:;;12 St James's Square;London;;SW1Y 4JH;United Kingdom\r\n" +
		"item1.X-ABADR:gb\r\nX-SOCIALPROFILE;type=twitter:https://twitter.com/ada\r\n" +
		"PHOTO;ENCODING=b;TYPE=JPEG:%FOTO%\r\nBDAY;X-APPLE-OMIT-YEAR=1604:1604-12-10\r\n" +
		"X-ABUID:5AD380FD-B2DE-4261-BA99-DE1D1DB52FBE:ABPerson\r\nEND:VCARD\r\n"
)

// jpegDePrueba es un JPEG de 8×8 de verdad, en base64 y plegado como lo pliega Apple.
func jpegDePrueba(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(3, 3, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	var plegado strings.Builder
	for i := 0; i < len(b64); i += 60 {
		if i > 0 {
			plegado.WriteString("\r\n  ")
		}
		plegado.WriteString(b64[i:min(i+60, len(b64))])
	}
	return plegado.String()
}

func campo(t *testing.T, tarjeta, nombre string) []*vcard.Field {
	t.Helper()
	card, err := parseSingle(tarjeta)
	if err != nil {
		t.Fatalf("la tarjeta guardada no se lee: %v\n%s", err, tarjeta)
	}
	return card[nombre]
}

func TestImportCardGoogleNoPierdeNada(t *testing.T) {
	c, out, err := ImportCard(tarjetaGoogle)
	if err != nil {
		t.Fatal(err)
	}
	if v := campo(t, out, vcard.FieldVersion); len(v) != 1 || v[0].Value != "4.0" {
		t.Errorf("VERSION = %v, want 4.0", v)
	}
	// Lo que se proyecta, con las etiquetas buenas: el TYPE=INTERNET de delante no
	// puede convertir el correo del trabajo en «otro».
	if len(c.Emails) != 2 || c.Emails[0].Type != EmailWork || c.Emails[1].Type != EmailHome {
		t.Errorf("correos = %+v", c.Emails)
	}
	if len(c.Phones) != 1 || c.Phones[0].Type != PhoneCell || c.Phones[0].Phone != "+34 600 11 22 33" {
		t.Errorf("teléfonos = %+v", c.Phones)
	}
	if len(c.Addresses) != 1 || c.Addresses[0].Type != AddressWork || c.Addresses[0].Locality != "Madrid" {
		t.Errorf("direcciones = %+v", c.Addresses)
	}
	if c.Organization != "Acme S.L." || c.Title != "Directora Comercial" || c.Note != "Conocida en la feria" {
		t.Errorf("organización/cargo/nota = %q/%q/%q", c.Organization, c.Title, c.Note)
	}
	if c.Birthday == nil || c.Birthday.Format("2006-01-02") != "1985-02-15" {
		t.Errorf("cumpleaños = %v", c.Birthday)
	}
	// Lo que NO se proyecta viaja en la tarjeta.
	for _, nombre := range []string{"URL", "X-ABLABEL", "CATEGORIES"} {
		if len(campo(t, out, nombre)) == 0 {
			t.Errorf("se ha perdido %s:\n%s", nombre, out)
		}
	}
	if cats := campo(t, out, "CATEGORIES"); len(cats) == 1 && !strings.Contains(cats[0].Value, "Clientes") {
		t.Errorf("CATEGORIES = %q", cats[0].Value)
	}
}

func TestImportCardAppleConFoto(t *testing.T) {
	tarjeta := strings.Replace(tarjetaApple, "%FOTO%", jpegDePrueba(t), 1)
	c, out, err := ImportCard(tarjeta)
	if err != nil {
		t.Fatal(err)
	}
	// La foto binaria de vCard 3.0 pasa a data URI: es la que el servicio sirve.
	if !CardHasEmbeddedPhoto(out) {
		t.Fatalf("la foto no ha llegado como data URI:\n%s", out)
	}
	uri, _ := CardPhotoDataURI(out)
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,/9j/") {
		t.Errorf("foto = %.40q…", uri)
	}
	if _, err := ValidatePhotoDataURI(uri); err != nil {
		t.Errorf("la foto guardada no es válida: %v", err)
	}
	if c.PhotoURL != "" {
		t.Errorf("la foto no es una dirección y ha acabado en photo_url: %.40q…", c.PhotoURL)
	}
	if len(c.Emails) != 1 || c.Emails[0].Type != EmailHome || !c.Emails[0].IsPrimary {
		t.Errorf("correos = %+v", c.Emails)
	}
	if len(c.Phones) != 1 || c.Phones[0].Type != PhoneCell {
		t.Errorf("teléfonos = %+v", c.Phones)
	}
	for _, nombre := range []string{"X-SOCIALPROFILE", "X-ABUID", "X-ABADR", "PRODID"} {
		if len(campo(t, out, nombre)) == 0 {
			t.Errorf("se ha perdido %s:\n%s", nombre, out)
		}
	}
	// El cumpleaños «sin año» de Apple es una fecha completa (1604) y se proyecta;
	// su X-APPLE-OMIT-YEAR se pierde con el BDAY regenerado, como al editar.
	if c.Birthday == nil {
		t.Error("cumpleaños sin proyectar")
	}
}

func TestImportCardUIDNuevoYSinPerderEnLaVuelta(t *testing.T) {
	tarjeta := strings.Replace(tarjetaApple, "%FOTO%", jpegDePrueba(t), 1)
	uno, primera, err := ImportCard(tarjeta)
	if err != nil {
		t.Fatal(err)
	}
	// Exportar lo importado y volver a importarlo (de una cuenta a otra) no pierde nada
	// y no reutiliza el UID.
	dos, segunda, err := ImportCard(primera)
	if err != nil {
		t.Fatal(err)
	}
	if uno.UID == "" || uno.UID == dos.UID {
		t.Errorf("UID = %q y %q: cada importación estrena el suyo", uno.UID, dos.UID)
	}
	if u := campo(t, segunda, vcard.FieldUID); len(u) != 1 || cleanURN(u[0].Value) != dos.UID {
		t.Errorf("el UID de la tarjeta (%v) no es el de la proyección (%s)", u, dos.UID)
	}
	f1, _ := CardPhotoDataURI(primera)
	f2, _ := CardPhotoDataURI(segunda)
	if f1 == "" || f1 != f2 {
		t.Error("la foto cambia en la vuelta")
	}
	for _, nombre := range []string{"X-SOCIALPROFILE", "X-ABUID", "X-ABADR"} {
		if a, b := campo(t, primera, nombre), campo(t, segunda, nombre); len(a) != 1 || len(b) != 1 || a[0].Value != b[0].Value {
			t.Errorf("%s cambia en la vuelta: %v → %v", nombre, a, b)
		}
	}
}

func TestImportCardFotoQueNoEsImagenSeGuardaSinMas(t *testing.T) {
	tarjeta := strings.Replace(tarjetaApple, "%FOTO%", base64.StdEncoding.EncodeToString([]byte("no soy una imagen")), 1)
	c, out, err := ImportCard(tarjeta)
	if err != nil {
		t.Fatal(err)
	}
	if CardHasEmbeddedPhoto(out) {
		t.Error("un texto se ha dado por foto")
	}
	if c.PhotoURL != "" {
		t.Errorf("el base64 ha acabado en photo_url: %q", c.PhotoURL)
	}
	if fs := campo(t, out, vcard.FieldPhoto); len(fs) != 1 || !strings.EqualFold(fs[0].Params.Get("ENCODING"), "b") {
		t.Errorf("la PHOTO que no se entiende se pierde o pierde su ENCODING: %v", fs)
	}
}

func TestImportCardRechazaLoQueNoEsUnaFicha(t *testing.T) {
	for nombre, tarjeta := range map[string]string{
		"sin nombre": "BEGIN:VCARD\r\nVERSION:3.0\r\nEMAIL:x@y.es\r\nEND:VCARD\r\n",
		"un grupo":   "BEGIN:VCARD\r\nVERSION:4.0\r\nKIND:group\r\nFN:Equipo\r\nEND:VCARD\r\n",
		"basura":     "hola",
	} {
		if _, _, err := ImportCard(tarjeta); err == nil {
			t.Errorf("%s: se ha aceptado", nombre)
		}
	}
}
