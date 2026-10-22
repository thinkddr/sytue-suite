// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package certificados

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func init() { fuentes = append(fuentes, func() []Almacen { return []Almacen{&AlmacenWindows{}} }) }

// AlmacenWindows es el almacén «Personal» de la persona (`MY`) con CNG: sus
// certificados con clave privada —los importados de un .p12 y los de las tarjetas con
// minidriver, que Windows enseña ahí mismo—, firmando con NCryptSignHash. El PIN y la
// confirmación los pide el propio sistema, así que PidePIN es falso.
type AlmacenWindows struct {
	mu sync.Mutex
}

var _ Almacen = (*AlmacenWindows)(nil)

// refWindows es la huella SHA-256 del certificado: se vuelve a buscar al firmar.
type refWindows [sha256.Size]byte

var (
	crypt32                           = windows.NewLazySystemDLL("crypt32.dll")
	procCertGetCertificateContextProp = crypt32.NewProc("CertGetCertificateContextProperty")
	ncrypt                            = windows.NewLazySystemDLL("ncrypt.dll")
	procNCryptSignHash                = ncrypt.NewProc("NCryptSignHash")
	procNCryptFreeObject              = ncrypt.NewProc("NCryptFreeObject")
	sha256UTF16                       = windows.StringToUTF16Ptr("SHA256")
)

const (
	certKeyProvInfoPropID     = 2
	cryptAcquireOnlyNCryptKey = 0x00040000
	certNCryptKeySpec         = 0xFFFFFFFF
	bcryptPadPKCS1            = 0x2
	ntePINIncorrecto          = 0x8010006B // SCARD_W_WRONG_CHV
	ntePINBloqueado           = 0x8010006C // SCARD_W_CHV_BLOCKED
	nteCancelado              = 0x8010006E // SCARD_W_CANCELLED_BY_USER
	errCanceladoPorLaPersona  = 0x800704C7 // ERROR_CANCELLED
	maxBytesFirmaWindows      = 1024
)

// Certificados son los del almacén Personal que tienen clave privada y sirven para
// firmar, con la cadena que monta el propio Windows.
func (w *AlmacenWindows) Certificados(ctx context.Context) ([]Certificado, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	store, err := windows.CertOpenSystemStore(0, windows.StringToUTF16Ptr("MY"))
	if err != nil {
		return nil, fmt.Errorf("certificados: el almacén de Windows no se abre: %w", err)
	}
	defer func() { _ = windows.CertCloseStore(store, 0) }()
	var out []Certificado
	var c *windows.CertContext
	for {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		c, err = windows.CertEnumCertificatesInStore(store, c)
		if err != nil || c == nil {
			break
		}
		if !tieneClave(c) {
			continue
		}
		hoja, err := x509.ParseCertificate(derDe(c))
		if err != nil || !sirveParaFirmar(hoja) {
			continue
		}
		out = append(out, Certificado{
			Cadena: cadenaDeWindows(c, hoja), Almacen: "Windows",
			ref: refWindows(sha256.Sum256(hoja.Raw)),
		})
	}
	return out, nil
}

// Firmar pide a CNG la firma del resumen: PKCS#1 v1.5 con SHA-256 (CNG pone el
// DigestInfo) o ECDSA, que CNG devuelve en r‖s.
func (w *AlmacenWindows) Firmar(ctx context.Context, c Certificado, prep api.PDFFirmaPreparada, _ string) ([]byte, string, error) {
	ref, ok := c.ref.(refWindows)
	if !ok || len(c.Cadena) == 0 {
		return nil, "", ErrNoEsta
	}
	if err := comprobarPreparada(prep); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	store, err := windows.CertOpenSystemStore(0, windows.StringToUTF16Ptr("MY"))
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = windows.CertCloseStore(store, 0) }()
	cert := buscarPorHuella(store, ref)
	if cert == nil {
		return nil, "", ErrNoEsta
	}
	defer func() { _ = windows.CertFreeCertificateContext(cert) }()

	var clave windows.Handle
	var spec uint32
	var soltar bool
	if err := windows.CryptAcquireCertificatePrivateKey(cert, cryptAcquireOnlyNCryptKey, nil, &clave, &spec, &soltar); err != nil {
		return nil, "", fmt.Errorf("certificados: la clave de Windows no está disponible: %w", err)
	}
	if soltar {
		defer procNCryptFreeObject.Call(uintptr(clave)) //nolint:errcheck // soltar no tiene vuelta atrás
	}
	if spec != certNCryptKeySpec {
		return nil, "", errors.New("certificados: la clave está en un proveedor antiguo (CAPI) que CNG no firma")
	}

	var relleno unsafe.Pointer
	var banderas uintptr
	formato := api.PDFFirmaP1363
	pkcs1 := struct{ alg *uint16 }{sha256UTF16}
	if !esECDSA(c.Cadena[0]) {
		relleno, banderas, formato = unsafe.Pointer(&pkcs1), bcryptPadPKCS1, api.PDFFirmaDER
	}
	firma := make([]byte, maxBytesFirmaWindows)
	var n uint32
	r, _, _ := procNCryptSignHash.Call(uintptr(clave), uintptr(relleno),
		uintptr(unsafe.Pointer(&prep.Resumen[0])), uintptr(len(prep.Resumen)),
		uintptr(unsafe.Pointer(&firma[0])), uintptr(len(firma)), uintptr(unsafe.Pointer(&n)), banderas)
	codigo := uint32(r & 0xFFFFFFFF) // un HRESULT/SECURITY_STATUS son 32 bits
	switch codigo {
	case 0:
		return firma[:n], formato, nil
	case ntePINIncorrecto:
		return nil, "", ErrPIN
	case ntePINBloqueado:
		return nil, "", ErrPINBloqueado
	case nteCancelado, errCanceladoPorLaPersona:
		return nil, "", context.Canceled
	}
	return nil, "", fmt.Errorf("certificados: CNG no firma (0x%X)", codigo)
}

// derDe son los bytes DER de un contexto de certificado.
func derDe(c *windows.CertContext) []byte {
	return append([]byte(nil), unsafe.Slice(c.EncodedCert, c.Length)...)
}

// tieneClave dice si el certificado lleva una clave privada asociada, sin tocarla (una
// tarjeta pediría el PIN solo por mirar).
func tieneClave(c *windows.CertContext) bool {
	var n uint32
	r, _, _ := procCertGetCertificateContextProp.Call(uintptr(unsafe.Pointer(c)), certKeyProvInfoPropID, 0, uintptr(unsafe.Pointer(&n)))
	return r != 0 && n > 0
}

// buscarPorHuella es el certificado del almacén con esa huella, duplicado (lo suelta
// quien llama), o nil.
func buscarPorHuella(store windows.Handle, ref refWindows) *windows.CertContext {
	var c *windows.CertContext
	var err error
	for {
		c, err = windows.CertEnumCertificatesInStore(store, c)
		if err != nil || c == nil {
			return nil
		}
		if sha256.Sum256(unsafe.Slice(c.EncodedCert, c.Length)) == ref {
			return windows.CertDuplicateCertificateContext(c)
		}
	}
}

// cadenaDeWindows es la cadena que monta Windows con sus almacenes (intermedios y
// raíces), en el orden que exige el servicio: cada uno emite al anterior.
func cadenaDeWindows(c *windows.CertContext, hoja *x509.Certificate) []*x509.Certificate {
	para := windows.CertChainPara{Size: uint32(unsafe.Sizeof(windows.CertChainPara{}))}
	var cc *windows.CertChainContext
	if err := windows.CertGetCertificateChain(0, c, nil, 0, &para, 0, 0, &cc); err != nil || cc == nil {
		return []*x509.Certificate{hoja}
	}
	defer windows.CertFreeCertificateChain(cc)
	var otros []*x509.Certificate
	if cc.ChainCount > 0 {
		simple := *cc.Chains
		for _, e := range unsafe.Slice(simple.Elements, simple.NumElements) {
			if x, err := x509.ParseCertificate(derDe(e.CertContext)); err == nil {
				otros = append(otros, x)
			}
		}
	}
	return cadenaDe(hoja, otros)
}
