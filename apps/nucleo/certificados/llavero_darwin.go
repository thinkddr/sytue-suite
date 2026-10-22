// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && cgo

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código.
//
//nolint:gocritic // cgo: los avisos no son de este fichero
package certificados

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include "llavero_sytue.h"
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func init() { fuentes = append(fuentes, func() []Almacen { return []Almacen{&Llavero{}} }) }

// Llavero es el almacén del sistema en macOS e iOS: las IDENTIDADES (certificado con su
// clave privada) del Llavero de la persona. La clave no sale de él: se le pide la firma
// con SecKeyCreateSignature, y el propio sistema pregunta si esta app puede usarla (y
// la contraseña o Touch ID, si la identidad lo exige). Por eso PidePIN es falso.
type Llavero struct {
	// ruta es un llavero concreto en vez de la lista de búsqueda de la persona: lo usan
	// las pruebas, con uno temporal, para no tocar el de nadie. Solo en macOS.
	ruta string
	mu   sync.Mutex
}

var _ Almacen = (*Llavero)(nil)

// refLlavero es la huella SHA-256 del certificado: la identidad se vuelve a buscar al
// firmar, porque una referencia del sistema no sobrevive a que el Llavero se bloquee.
type refLlavero [sha256.Size]byte

// Certificados son las identidades de firma del Llavero, con su cadena si los emisores
// están en él.
func (l *Llavero) Certificados(ctx context.Context) ([]Certificado, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids, err := l.identidades()
	if err != nil {
		return nil, err
	}
	otros := l.certificados()
	var out []Certificado
	for _, c := range ids {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !sirveParaFirmar(c) {
			continue
		}
		out = append(out, Certificado{
			Cadena: cadenaDe(c, otros), Almacen: "Llavero",
			ref: refLlavero(sha256.Sum256(c.Raw)),
		})
	}
	return out, nil
}

// Firmar le pide al Llavero la firma del resumen: PKCS#1 v1.5 con SHA-256 (el sistema
// pone el DigestInfo) o ECDSA X9.62, que sale en DER.
func (l *Llavero) Firmar(ctx context.Context, c Certificado, prep api.PDFFirmaPreparada, _ string) ([]byte, string, error) {
	ref, ok := c.ref.(refLlavero)
	if !ok || len(c.Cadena) == 0 {
		return nil, "", ErrNoEsta
	}
	if err := comprobarPreparada(prep); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ruta := l.cruta()
	defer C.sytue_soltar_cadena(ruta)
	huella := C.CBytes(ref[:])
	defer C.free(huella)
	resumen := C.CBytes(prep.Resumen)
	defer C.free(resumen)
	var firma C.CFDataRef
	var estado C.OSStatus
	ecdsa := C.int(0)
	if esECDSA(c.Cadena[0]) {
		ecdsa = 1
	}
	rv := C.sytue_llavero_firmar(ruta, huella, resumen, C.long(len(prep.Resumen)), ecdsa, &firma, &estado)
	switch {
	case rv == 1:
		return nil, "", ErrNoEsta
	case rv != 0 && estado == -128: // errSecUserCanceled: dijo que no en el diálogo
		return nil, "", context.Canceled
	case rv != 0:
		return nil, "", fmt.Errorf("certificados: el Llavero no firma (OSStatus %d)", int(estado))
	}
	defer C.CFRelease(C.CFTypeRef(firma))
	return C.GoBytes(C.sytue_datos(firma), C.int(C.CFDataGetLength(firma))), api.PDFFirmaDER, nil
}

func (l *Llavero) cruta() *C.char {
	if l.ruta == "" {
		return nil
	}
	return C.CString(l.ruta)
}

// identidades son los certificados de las identidades del Llavero.
func (l *Llavero) identidades() ([]*x509.Certificate, error) {
	ruta := l.cruta()
	defer C.sytue_soltar_cadena(ruta)
	lista := C.sytue_llavero_lista(ruta, 1)
	if lista == 0 {
		return nil, nil
	}
	defer C.CFRelease(C.CFTypeRef(lista))
	return deLista(lista), nil
}

// certificados son todos los del Llavero, para montar las cadenas.
func (l *Llavero) certificados() []*x509.Certificate {
	ruta := l.cruta()
	defer C.sytue_soltar_cadena(ruta)
	lista := C.sytue_llavero_lista(ruta, 0)
	if lista == 0 {
		return nil
	}
	defer C.CFRelease(C.CFTypeRef(lista))
	return deLista(lista)
}

// deLista lee los DER de un CFArray de CFData.
func deLista(lista C.CFArrayRef) []*x509.Certificate {
	var out []*x509.Certificate
	for i := range C.CFArrayGetCount(lista) {
		d := C.sytue_elemento(lista, i)
		der := C.GoBytes(C.sytue_datos(d), C.int(C.CFDataGetLength(d)))
		if c, err := x509.ParseCertificate(der); err == nil {
			out = append(out, c)
		}
	}
	return out
}
