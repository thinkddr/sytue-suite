// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/apps/certipdf"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// La firma y su verificación sin servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.3):
//
//   - P-05: la autoridad de sellado la escribe la persona. Se ofrece una lista de partida,
//     que ella puede ampliar o recortar; sin elegir ninguna, la firma sale sin sello y la
//     pantalla lo dice. Es la única conexión de la firma, y solo lleva la huella.
//   - P-06: las anclas de confianza son los certificados que la persona deja en una
//     carpeta (por defecto `<ajustes>/anclas`), y se leen cada vez que se verifica.
//
// Las dos cosas se guardan en los ajustes personales (`l.Personal()`), con guion bajo en la
// clave para que `catalogos` no las tome por claves de i18n.
const (
	claveSello   = "certipdf_sello"
	claveSellos  = "certipdf_sellos"
	claveAnclas  = "certipdf_anclas"
	plazoDeSello = 20 * time.Second
	// maxAncla acota cada fichero de la carpeta: un certificado son unos KB.
	maxAncla = 256 << 10
)

// autoridadesDePartida son las que se ofrecen la primera vez: autoridades de sellado
// CUALIFICADAS europeas con servicio abierto. Izenpe es la que mide el ADR 0051 contra la
// lista de confianza de la UE.
var autoridadesDePartida = []certipdf.AutoridadSello{
	{Nombre: "Izenpe · QTSA (ES)", URL: "http://tsa.izenpe.com"},
}

// preferencias es lo que se usa de `local.Personal`.
type preferencias interface {
	Leer(clave string) string
	Poner(clave, valor string)
}

// ajustesFirma son los de la firma local de esta instalación.
type ajustesFirma struct {
	personal   preferencias
	porDefecto string // la carpeta de anclas si la persona no eligió otra
	// red es el portero de la edición: el sello sale por él. Nil en las pruebas.
	red *red.Portero
}

func nuevosAjustesFirma(l *local.App) *ajustesFirma {
	a := &ajustesFirma{personal: l.Personal(), porDefecto: l.Espacio.Ajuste("anclas"), red: l.Red}
	if l.Red != nil {
		// La autoridad que eligió la persona es una conexión configurada por ella.
		l.Red.Fuente(a.destinoSello)
	}
	return a
}

// sello es la autoridad elegida ("" = sin sello) y la lista que se ofrece.
func (a *ajustesFirma) sello() (string, []certipdf.AutoridadSello) {
	lista := autoridadesDePartida
	if raw := a.personal.Leer(claveSellos); raw != "" {
		var guardada []certipdf.AutoridadSello
		if json.Unmarshal([]byte(raw), &guardada) == nil {
			lista = guardada
		}
	}
	return a.personal.Leer(claveSello), lista
}

func (a *ajustesFirma) ponerSello(actual string, lista []certipdf.AutoridadSello) {
	raw, _ := json.Marshal(lista)
	a.personal.Poner(claveSellos, string(raw))
	a.personal.Poner(claveSello, actual)
}

// opcionesSello es lo que el motor local usa para sellar. Sin autoridad elegida, vacía:
// la firma sale sin sello y no se conecta a nadie.
func (a *ajustesFirma) opcionesSello() localpdf.OpcionesSello {
	actual, _ := a.sello()
	if actual == "" {
		return localpdf.OpcionesSello{Timeout: plazoDeSello}
	}
	o := localpdf.OpcionesSello{URL: actual, Timeout: plazoDeSello, Anclas: anclasDeSello(a.carpetaAnclas())}
	if a.red != nil {
		o.Cliente = a.red.Cliente(red.SelloDeTiempo)
	}
	return o
}

// carpetaAnclas es la de la persona, o la de los ajustes.
func (a *ajustesFirma) carpetaAnclas() string {
	if c := a.personal.Leer(claveAnclas); c != "" {
		return c
	}
	_ = os.MkdirAll(a.porDefecto, 0o700)
	return a.porDefecto
}

// anclas lee los certificados de la carpeta: PEM (uno o varios por fichero) o DER.
// Lo que no se puede leer se salta: un fichero suelto no debe dejar sin las demás.
func (a *ajustesFirma) anclas() (*x509.CertPool, int) {
	return leerAnclas(a.carpetaAnclas())
}

func leerAnclas(carpeta string) (*x509.CertPool, int) {
	pool := x509.NewCertPool()
	cs := certificadosAncla(carpeta)
	for _, c := range cs {
		pool.AddCert(c)
	}
	return pool, len(cs)
}

// certificadosAncla son los certificados de la carpeta de confianza.
func certificadosAncla(carpeta string) []*x509.Certificate {
	var out []*x509.Certificate
	entradas, err := os.ReadDir(carpeta)
	if err != nil {
		return nil
	}
	for _, e := range entradas {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".pem" && ext != ".crt" && ext != ".cer" && ext != ".der") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > maxAncla {
			continue
		}
		datos, err := os.ReadFile(filepath.Join(carpeta, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, certificadosDe(datos)...)
	}
	return out
}

func certificadosDe(datos []byte) []*x509.Certificate {
	if !bytes.Contains(datos, []byte("-----BEGIN")) {
		c, err := x509.ParseCertificate(datos)
		if err != nil {
			return nil
		}
		return []*x509.Certificate{c}
	}
	var out []*x509.Certificate
	for resto := datos; ; {
		b, siguiente := pem.Decode(resto)
		if b == nil {
			return out
		}
		resto = siguiente
		if b.Type != "CERTIFICATE" {
			continue
		}
		if c, err := x509.ParseCertificate(b.Bytes); err == nil {
			out = append(out, c)
		}
	}
}

// pantalla da a CertiPDF los dos ajustes.
func (a *ajustesFirma) pantalla(p *certipdf.Pantalla, titulo string) {
	p.SelloDeTiempo = &certipdf.AjusteSello{Leer: a.sello, Poner: a.ponerSello}
	p.Anclas = &certipdf.AjusteAnclas{
		Carpeta: a.carpetaAnclas,
		Cuantos: func() int { _, n := a.anclas(); return n },
		Elegir: func(listo func()) {
			local.ElegirCarpeta(titulo, func(ruta string) {
				a.personal.Poner(claveAnclas, ruta)
				listo()
			})
		},
	}
}

// pool es la función de anclas del motor: nil sin certificados.
func (a *ajustesFirma) pool() *x509.CertPool {
	pool, n := a.anclas()
	if n == 0 {
		return nil
	}
	return pool
}
