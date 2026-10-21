// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package certificados

import (
	"context"
	"crypto/x509"
	"errors"
	"sync"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func init() { fuentes = append(fuentes, func() []Almacen { return []Almacen{&KeyChain{}} }) }

// claseCertificados es la clase propia de la APK (apps/fyne/android/java).
const claseCertificados = "eu.sytue.nativo.Certificados"

// KeyChain es el almacén de Android. Android no deja LISTAR sus certificados: se elige
// uno en el selector del sistema, así que Certificados enseña ese selector y devuelve
// el elegido (o ninguno). La clave no sale de Android y firmar la pide el sistema.
type KeyChain struct {
	mu sync.Mutex
}

var _ Almacen = (*KeyChain)(nil)

// refKeyChain es el alias que Android dio al elegir.
type refKeyChain string

// Certificados enseña el selector del sistema y devuelve el certificado elegido con su
// cadena. Vacío si la persona cierra el selector.
func (k *KeyChain) Certificados(ctx context.Context) ([]Certificado, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []Certificado
	err := sistema.Java(func(env jni.Env, actx jni.Object) error {
		cls, err := sistema.CargarClase(env, actx, claseCertificados)
		if err != nil {
			return err
		}
		elegir := jni.GetStaticMethodID(env, cls, "elegir", "(Landroid/content/Context;)Ljava/lang/String;")
		o, err := jni.CallStaticObjectMethod(env, cls, elegir, jni.Value(actx))
		if err != nil || o == 0 {
			return err
		}
		alias := jni.GoString(env, jni.String(o))
		if alias == "" {
			return nil
		}
		cadena := jni.GetStaticMethodID(env, cls, "cadena", "(Landroid/content/Context;Ljava/lang/String;)[B")
		b, err := jni.CallStaticObjectMethod(env, cls, cadena, jni.Value(actx), jni.Value(jni.JavaString(env, alias)))
		if err != nil || b == 0 {
			return err
		}
		certs, err := x509.ParseCertificates(jni.GetByteArrayElements(env, jni.ByteArray(b)))
		if err != nil || len(certs) == 0 || !sirveParaFirmar(certs[0]) {
			return err
		}
		out = append(out, Certificado{Cadena: cadenaDe(certs[0], certs[1:]), Almacen: "Android", ref: refKeyChain(alias)})
		return nil
	})
	return out, err
}

// Firmar pide a Android la firma: NONEwithRSA sobre el DigestInfo (Android pone el
// relleno PKCS#1 v1.5) o NONEwithECDSA sobre el resumen, que sale en DER.
func (k *KeyChain) Firmar(ctx context.Context, c Certificado, prep api.PDFFirmaPreparada, _ string) ([]byte, string, error) {
	alias, ok := c.ref.(refKeyChain)
	if !ok || alias == "" || len(c.Cadena) == 0 {
		return nil, "", ErrNoEsta
	}
	if err := comprobarPreparada(prep); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	datos, ecdsa := prep.DigestInfo, esECDSA(c.Cadena[0])
	if ecdsa {
		datos = prep.Resumen
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	var firma []byte
	err := sistema.Java(func(env jni.Env, actx jni.Object) error {
		cls, err := sistema.CargarClase(env, actx, claseCertificados)
		if err != nil {
			return err
		}
		m := jni.GetStaticMethodID(env, cls, "firmar", "(Landroid/content/Context;Ljava/lang/String;[BZ)[B")
		esEC := jni.Value(0)
		if ecdsa {
			esEC = 1
		}
		o, err := jni.CallStaticObjectMethod(env, cls, m, jni.Value(actx),
			jni.Value(jni.JavaString(env, string(alias))), jni.Value(jni.NewByteArray(env, datos)), esEC)
		if err != nil {
			return err
		}
		if o == 0 {
			return ErrNoEsta
		}
		firma = jni.GetByteArrayElements(env, jni.ByteArray(o))
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNoEsta) {
			return nil, "", err
		}
		return nil, "", errors.Join(errors.New("certificados: Android no firma"), err)
	}
	return firma, api.PDFFirmaDER, nil
}
