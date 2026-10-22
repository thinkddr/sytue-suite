// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo && (linux || darwin || windows) && !android && !ios

// gocritic lee este fichero ya pasado por cgo y da posiciones y avisos de otro código
// («dupSubExpr» en `rv != ckrOK`, «dupImport» de `C`): se silencia en el fichero, no en
// el paquete, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package certificados

/*
#cgo linux LDFLAGS: -ldl
#include "pkcs11_sytue.h"
*/
import "C"

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Códigos de retorno de Cryptoki que se distinguen.
const (
	ckrOK                    = 0x0
	ckrYaIniciado            = 0x191
	ckrYaDentro              = 0x100
	ckrPINIncorrecto         = 0xA0
	ckrPINBloqueado          = 0xA4
	ckrObjetoNoValido        = 0x82
	ckrTokenNoPresente       = 0xE0
	ckoCertificado           = 0x1
	ckoClavePrivada          = 0x3
	ckaValue                 = 0x11
	ckaID                    = 0x102
	ckaLabel                 = 0x3
	ckmRSAPKCS               = 0x1
	ckmECDSA                 = 0x1041
	ckfLoginRequerido        = 0x4
	ckfTecladoPropio         = 0x100 // CKF_PROTECTED_AUTHENTICATION_PATH
	maxObjetos               = 64
	maxBytesFirmaPKCS11 uint = 1024
)

// PKCS11 es el almacén de un módulo PKCS#11: el de OpenSC para el DNIe y casi todas las
// tarjetas, o el del fabricante.
type PKCS11 struct {
	ruta   string
	nombre string
	f      *C.sytue_p11
	// mu serializa las llamadas: CKF_OS_LOCKING_OK deja al módulo protegerse, pero una
	// tarjeta atiende una operación a la vez y un segundo login en paralelo la bloquea.
	mu sync.Mutex
}

var _ Almacen = (*PKCS11)(nil)

// ref es dónde está la clave de un certificado: su ranura y su CKA_ID.
type refPKCS11 struct {
	ranura C.CK_ULONG
	id     []byte
}

// AbrirPKCS11 carga el módulo de `ruta`. `nombre` es cómo se enseña («DNIe»).
func AbrirPKCS11(ruta, nombre string) (*PKCS11, error) {
	cr := C.CString(ruta)
	defer C.sytue_p11_soltar_cadena(cr)
	f := C.sytue_p11_abrir(cr)
	if f == nil {
		return nil, fmt.Errorf("certificados: no se puede cargar el módulo PKCS#11 %s", ruta)
	}
	if rv := C.sytue_p11_iniciar(f); rv != ckrOK && rv != ckrYaIniciado {
		return nil, fmt.Errorf("certificados: %s no arranca: %w", ruta, errCK(rv))
	}
	return &PKCS11{ruta: ruta, nombre: nombre, f: f}, nil
}

// Cerrar suelta el módulo. El proceso puede seguir cargándolo: no se descarga la
// biblioteca, que algunos módulos no toleran.
func (p *PKCS11) Cerrar() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		return nil
	}
	rv := C.sytue_p11_acabar(p.f)
	p.f = nil
	return errCK(rv)
}

// Certificados son los de firma de todas las tarjetas presentes que tienen su clave
// privada al lado (mismo CKA_ID): un certificado de una CA guardado en la tarjeta no
// firma nada.
func (p *PKCS11) Certificados(ctx context.Context) ([]Certificado, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		return nil, errors.New("certificados: el módulo está cerrado")
	}
	var n C.CK_ULONG
	if rv := C.sytue_p11_ranuras(p.f, nil, &n); rv != ckrOK {
		return nil, errCK(rv)
	}
	if n == 0 {
		return nil, nil
	}
	ranuras := make([]C.CK_ULONG, n)
	if rv := C.sytue_p11_ranuras(p.f, &ranuras[0], &n); rv != ckrOK {
		return nil, errCK(rv)
	}
	var out []Certificado
	for _, r := range ranuras[:n] {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		cs, err := p.deLaRanura(r)
		if err != nil {
			continue // una tarjeta que no se deja leer no esconde las demás
		}
		out = append(out, cs...)
	}
	return out, nil
}

func (p *PKCS11) deLaRanura(r C.CK_ULONG) ([]Certificado, error) {
	var ficha C.CK_TOKEN_INFO
	if rv := C.sytue_p11_ficha(p.f, r, &ficha); rv != ckrOK {
		return nil, errCK(rv)
	}
	banderas := C.sytue_p11_banderas(&ficha)
	etiqueta := strings.TrimSpace(C.GoStringN(C.sytue_p11_etiqueta(&ficha), 32))
	var h C.CK_ULONG
	if rv := C.sytue_p11_sesion(p.f, r, &h); rv != ckrOK {
		return nil, errCK(rv)
	}
	defer C.sytue_p11_cerrar(p.f, h)

	objs, err := p.buscar(h, ckoCertificado, nil)
	if err != nil {
		return nil, err
	}
	type leido struct {
		c  *x509.Certificate
		id []byte
	}
	var todos []leido
	for _, o := range objs {
		der, err := p.atributo(h, o, ckaValue)
		if err != nil {
			continue
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		id, _ := p.atributo(h, o, ckaID)
		todos = append(todos, leido{c, id})
	}
	otros := make([]*x509.Certificate, len(todos))
	for i, x := range todos {
		otros[i] = x.c
	}
	var out []Certificado
	for _, x := range todos {
		if !sirveParaFirmar(x.c) || len(x.id) == 0 {
			continue
		}
		// Las claves privadas de una tarjeta con PIN no se ven sin iniciar sesión, así
		// que su CKA_ID no se puede comprobar aquí: se cree al certificado, y si no
		// tuviera clave lo diría al firmar ([ErrNoEsta]).
		if banderas&ckfLoginRequerido == 0 {
			if claves, err := p.buscar(h, ckoClavePrivada, x.id); err != nil || len(claves) == 0 {
				continue
			}
		}
		almacen := "PKCS#11"
		if etiqueta != "" {
			almacen = etiqueta
		}
		if p.nombre != "" {
			almacen = p.nombre + " · " + almacen
		}
		out = append(out, Certificado{
			Cadena: cadenaDe(x.c, otros), Almacen: almacen,
			PidePIN: banderas&ckfLoginRequerido != 0 && banderas&ckfTecladoPropio == 0,
			ref:     refPKCS11{ranura: r, id: bytes.Clone(x.id)},
		})
	}
	return out, nil
}

// Firmar abre sesión en la tarjeta, entra con el PIN (o con su teclado, si lo tiene),
// busca la clave del certificado y le pide la firma: CKM_RSA_PKCS sobre el DigestInfo,
// o CKM_ECDSA sobre el resumen, que sale en r‖s.
func (p *PKCS11) Firmar(ctx context.Context, c Certificado, prep api.PDFFirmaPreparada, pin string) ([]byte, string, error) {
	ref, ok := c.ref.(refPKCS11)
	if !ok || len(c.Cadena) == 0 {
		return nil, "", ErrNoEsta
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	mec, datos, formato := C.CK_ULONG(ckmRSAPKCS), prep.DigestInfo, api.PDFFirmaDER
	if esECDSA(c.Cadena[0]) {
		mec, datos, formato = ckmECDSA, prep.Resumen, api.PDFFirmaP1363
	}
	if err := comprobarPreparada(prep); err != nil {
		return nil, "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		return nil, "", errors.New("certificados: el módulo está cerrado")
	}
	var h C.CK_ULONG
	if rv := C.sytue_p11_sesion(p.f, ref.ranura, &h); rv != ckrOK {
		if rv == ckrTokenNoPresente {
			return nil, "", ErrNoEsta
		}
		return nil, "", errCK(rv)
	}
	defer C.sytue_p11_cerrar(p.f, h)
	if err := p.entrar(h, c.PidePIN, pin); err != nil {
		return nil, "", err
	}
	defer C.sytue_p11_salir(p.f, h)

	claves, err := p.buscar(h, ckoClavePrivada, ref.id)
	if err != nil {
		return nil, "", err
	}
	if len(claves) == 0 {
		return nil, "", ErrNoEsta
	}
	// Las dos copias van en memoria de C: Go no deja pasar punteros a su montón a una
	// biblioteca que podría guardarlos.
	cd := C.CBytes(datos)
	defer C.free(cd)
	sig := C.malloc(C.size_t(maxBytesFirmaPKCS11))
	defer C.free(sig)
	ns := C.CK_ULONG(maxBytesFirmaPKCS11)
	if rv := C.sytue_p11_firmar(p.f, h, claves[0], mec, cd, C.CK_ULONG(len(datos)), sig, &ns); rv != ckrOK {
		return nil, "", errCK(rv)
	}
	return C.GoBytes(sig, C.int(ns)), formato, nil
}

// entrar inicia sesión de usuario. Con teclado propio en el lector, el PIN no pasa
// por aquí: se llama sin él y la tarjeta lo pide en su pantalla.
func (p *PKCS11) entrar(h C.CK_ULONG, pidePIN bool, pin string) error {
	var rv C.CK_RV
	if pidePIN {
		// Directo a memoria de C, sin una copia intermedia en el montón de Go que nadie
		// borraría; y de ahí se borra en cuanto la tarjeta lo ha leído.
		cp := C.CString(pin)
		rv = C.sytue_p11_entrar_c(p.f, h, cp, C.CK_ULONG(len(pin)))
		C.sytue_p11_borrar_cadena(cp, C.size_t(len(pin)))
	} else {
		rv = C.sytue_p11_entrar(p.f, h, nil, 0)
	}
	switch rv {
	case ckrOK, ckrYaDentro:
		return nil
	case ckrPINIncorrecto:
		return ErrPIN
	case ckrPINBloqueado:
		return ErrPINBloqueado
	}
	return errCK(rv)
}

func (p *PKCS11) buscar(h, clase C.CK_ULONG, id []byte) ([]C.CK_ULONG, error) {
	cid := C.sytue_p11_nulo()
	if id != nil {
		cid = C.CBytes(id)
		defer C.free(cid)
	}
	objs := make([]C.CK_ULONG, maxObjetos)
	var n C.CK_ULONG
	if rv := C.sytue_p11_buscar(p.f, h, clase, cid, C.CK_ULONG(len(id)), &objs[0], maxObjetos, &n); rv != ckrOK {
		return nil, errCK(rv)
	}
	return objs[:n], nil
}

func (p *PKCS11) atributo(h, o, tipo C.CK_ULONG) ([]byte, error) {
	v := C.sytue_p11_nulo()
	var n C.CK_ULONG
	if rv := C.sytue_p11_atributo(p.f, h, o, tipo, &v, &n); rv != ckrOK {
		return nil, errCK(rv)
	}
	defer C.sytue_p11_soltar(v)
	return C.GoBytes(v, C.int(n)), nil
}

// errCK es un CK_RV como error, o nil.
func errCK(rv C.CK_RV) error {
	if rv == ckrOK {
		return nil
	}
	if rv == ckrObjetoNoValido {
		return ErrNoEsta
	}
	return fmt.Errorf("certificados: PKCS#11 devolvió 0x%X", uint64(rv))
}

// ModulosPKCS11 son los módulos que hay instalados en esta máquina, con cómo se
// enseñan: los de `SYTUE_PKCS11` (rutas separadas por la lista del sistema) y los
// habituales de OpenSC —el del DNIe y casi todas las tarjetas europeas— y del DNIe.
func ModulosPKCS11() map[string]string {
	out := map[string]string{}
	for _, r := range filepath.SplitList(os.Getenv("SYTUE_PKCS11")) {
		if r != "" {
			out[r] = filepath.Base(r)
		}
	}
	var conocidos map[string]string
	switch runtime.GOOS {
	case "darwin":
		conocidos = map[string]string{
			"/Library/OpenSC/lib/opensc-pkcs11.so": "OpenSC",
			"/usr/local/lib/opensc-pkcs11.so":      "OpenSC",
			"/opt/homebrew/lib/opensc-pkcs11.so":   "OpenSC",
			"/usr/local/lib/libpkcs11-dnie.so":     "DNIe",
		}
	case "windows":
		conocidos = map[string]string{
			`C:\Program Files\OpenSC Project\OpenSC\pkcs11\opensc-pkcs11.dll`: "OpenSC",
			`C:\Windows\System32\opensc-pkcs11.dll`:                           "OpenSC",
		}
	default:
		conocidos = map[string]string{
			"/usr/lib/x86_64-linux-gnu/opensc-pkcs11.so":  "OpenSC",
			"/usr/lib/aarch64-linux-gnu/opensc-pkcs11.so": "OpenSC",
			"/usr/lib64/opensc-pkcs11.so":                 "OpenSC",
			"/usr/lib/opensc-pkcs11.so":                   "OpenSC",
			"/usr/lib/libpkcs11-dnie.so":                  "DNIe",
		}
	}
	for r, n := range conocidos {
		if _, err := os.Stat(r); err == nil {
			out[r] = n
		}
	}
	return out
}
