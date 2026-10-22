// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"encoding/base64"
	"encoding/xml"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// TestElToastLlevaSuEnlaceYNoRompeLasComillas: el toast de Windows abre su enlace al
// pulsarlo, y ningún texto del Hub se sale de la cadena de PowerShell. La ’ tipográfica
// es la trampa: PowerShell la toma por una comilla simple, y un «O’Brien» en el título
// dejaría el resto del cuerpo como órdenes. VISTO EN ROJO quitando las tipográficas del
// reemplazo y con `activationType` en «foreground».
func TestElToastLlevaSuEnlaceYNoRompeLasComillas(t *testing.T) {
	titulo, cuerpo := "Correo de O’Brien", "«Hola» <b>y</b> 'adiós'; & ‘fin’\nsegunda línea"
	guion := guionToast("eu.sytue.mail", "sytue-mail:/_aviso?clave=ab&x=1", titulo, cuerpo)

	const abre = "$xml.LoadXml('"
	i := strings.Index(guion, abre)
	if i < 0 {
		t.Fatalf("el guion no carga el XML:\n%s", guion)
	}
	resto := guion[i+len(abre):]
	fin := strings.IndexAny(resto, "'‘’‚‛")
	if fin < 0 || !strings.HasPrefix(resto[fin:], "')\n") {
		t.Fatalf("la cadena de PowerShell se cierra antes de tiempo: %q", resto)
	}
	var toast struct {
		Activacion string   `xml:"activationType,attr"`
		Enlace     string   `xml:"launch,attr"`
		Textos     []string `xml:"visual>binding>text"`
	}
	if err := xml.Unmarshal([]byte(resto[:fin]), &toast); err != nil {
		t.Fatalf("el XML del toast no es XML: %v", err)
	}
	if toast.Activacion != "protocol" || toast.Enlace != "sytue-mail:/_aviso?clave=ab&x=1" {
		t.Errorf("activación %q, enlace %q", toast.Activacion, toast.Enlace)
	}
	if len(toast.Textos) != 2 || toast.Textos[0] != titulo || toast.Textos[1] != cuerpo {
		t.Errorf("textos = %q", toast.Textos)
	}

	// Y lo que se le pasa a `-EncodedCommand` es ese mismo guion, en UTF-16LE.
	crudo, err := base64.StdEncoding.DecodeString(codificado(guion))
	if err != nil || len(crudo)%2 != 0 {
		t.Fatalf("codificado no es base64 de UTF-16: %v", err)
	}
	u := make([]uint16, len(crudo)/2)
	for i := range u {
		u[i] = uint16(crudo[2*i]) | uint16(crudo[2*i+1])<<8
	}
	if string(utf16.Decode(u)) != guion {
		t.Error("el guion codificado no es el guion")
	}

	// NAT-12: tampoco un appID con comilla tipográfica (p. ej. el nombre de la app cuando
	// no hay AUMID) puede cerrar la cadena de CreateToastNotifier('…').
	guionApp := guionToast("Sytue d’essai", "sytue-mail:/_aviso?clave=ab", "Hola", "Mundo")
	const abreNotif = "CreateToastNotifier('"
	j := strings.Index(guionApp, abreNotif)
	if j < 0 {
		t.Fatalf("el guion no llama a CreateToastNotifier:\n%s", guionApp)
	}
	restoApp := guionApp[j+len(abreNotif):]
	finApp := strings.Index(restoApp, "').Show(")
	if finApp < 0 || restoApp[:finApp] != "Sytue d’’essai" {
		t.Fatalf("el appID deja comillas tipográficas sin escapar en CreateToastNotifier: %q", restoApp)
	}
}

// TestElEnlaceDelAvisoLlegaASuFuncionUnaVez: el clic que vuelve por enlace (Windows)
// hace lo que el aviso dice, una sola vez; un enlace de otra cosa no es de esto, y la
// clave más vieja se olvida al pasar del tope. VISTO EN ROJO sin el `delete` de `pulsar`.
func TestElEnlaceDelAvisoLlegaASuFuncionUnaVez(t *testing.T) {
	llega := make(chan struct{}, 2)
	u, err := url.Parse(EnlaceDeAviso("sytue-mail", apuntar(func() { llega <- struct{}{} }), Destino{}))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if !Pulsado(u) {
			t.Fatalf("%s no se ha reconocido como aviso pulsado", u)
		}
		select {
		case <-llega:
			if i == 1 {
				t.Fatal("el mismo aviso ha llevado a su destino dos veces")
			}
		case <-time.After(300 * time.Millisecond):
			if i == 0 {
				t.Fatal("pulsar el aviso no ha llevado a ninguna parte")
			}
		}
	}
	otro, _ := url.Parse("sytue-mail:/mail/inbox?clave=x")
	if Pulsado(otro) {
		t.Error("un enlace de la app se ha tomado por un aviso")
	}

	primera := apuntar(func() {})
	for range maxClaves {
		apuntar(func() {})
	}
	clavesMu.Lock()
	_, sigue := claves[primera]
	cuantas := len(claves)
	clavesMu.Unlock()
	if sigue || cuantas > maxClaves {
		t.Errorf("con %d avisos se recuerdan %d, y el más viejo sigue: %v", maxClaves+1, cuantas, sigue)
	}
}

// TestElAvisoDeUnaAppYaCerradaLlevaASuDestino: un aviso pulsado cuando ya no vive el
// proceso que lo puso —su clave no la conoce nadie— sigue el destino que lleva escrito,
// y lo que llega antes de que nadie se apunte espera a quien lo haga. Y como el enlace lo
// puede escribir cualquiera, no se sigue un destino que no sea una ruta de un producto.
// VISTO EN ROJO dejando que `destinoValido` aceptara todo (`//otro.host` llegaba a
// seguirse) y sin el `pendiente` (el clic de macOS con la app cerrada se perdía).
func TestElAvisoDeUnaAppYaCerradaLlevaASuDestino(t *testing.T) {
	d := DestinoDe("n-7", "mail", "https://mail.sytue.eu/mail/inbox?m=42&x=a%20b#fin")
	if d.Ruta != "/mail/inbox?m=42&x=a%20b#fin" {
		t.Fatalf("DestinoDe se queda con %q", d.Ruta)
	}
	enlace := EnlaceDeAviso("sytue-mail", "clave-de-un-proceso-que-ya-no-existe", d)
	u, err := url.Parse(enlace)
	if err != nil {
		t.Fatal(err)
	}

	AlSeguir(nil)
	if !Pulsado(u) {
		t.Fatalf("%s no se ha reconocido como aviso pulsado", enlace)
	}
	llega := make(chan Destino, 4)
	AlSeguir(func(d Destino) { llega <- d }) // se apunta DESPUÉS del clic
	t.Cleanup(func() { AlSeguir(nil) })
	select {
	case got := <-llega:
		if got != d {
			t.Fatalf("se ha seguido %+v, y el aviso llevaba %+v", got, d)
		}
	case <-time.After(time.Second):
		t.Fatal("el clic que llegó antes de apuntarse nadie se ha perdido")
	}

	for _, malo := range []Destino{
		{Aviso: "n", Origen: "mail", Ruta: "//otro.host/robar"},
		{Aviso: "n", Origen: "mail", Ruta: "https://otro.host/"},
		{Aviso: "n", Origen: "Mail.evil", Ruta: "/"},
		{Aviso: "n", Origen: "", Ruta: "/mail"},
	} {
		v := url.Values{"clave": {"x"}, "aviso": {malo.Aviso}, "origen": {malo.Origen}, "ruta": {malo.Ruta}}
		u, _ := url.Parse("sytue-mail:" + rutaAviso + "?" + v.Encode())
		if !Pulsado(u) {
			t.Fatalf("%s no se ha tomado por un aviso", u)
		}
		select {
		case got := <-llega:
			t.Errorf("se ha seguido un destino escrito a mano: %+v", got)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
