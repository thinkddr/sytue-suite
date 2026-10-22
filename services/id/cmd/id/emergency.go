// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"os"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	idhttp "git.sytue.eu/sytue/one/services/id/internal/http"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// REVOCACIÓN GLOBAL INMEDIATA — interruptor de emergencia, fuera de banda.
//
//	id revoke-all
//
// Qué resuelve. Un access token no se puede «borrar»: es autocontenido y cada
// servicio lo valida contra el JWKS sin preguntar a nadie. Revocar la sesión que
// lo emitió no lo invalida, así que ante un fallo de rol —o una sesión robada—
// quedaba una ventana igual al TTL del token. Aquí no se acorta esa ventana: se
// elimina, invalidando el MATERIAL con el que están firmados.
//
// Cómo. Se rota la clave de firma y se retira la anterior EN EL ACTO
// (retireAfter=0, sin ventana de gracia). A partir de ese instante ninguna firma
// emitida antes verifica contra el JWKS publicado: los tokens no caducan, dejan
// de ser válidos. Los servicios que cachean el JWKS lo refrescan solos en cuanto
// ven un `kid` desconocido, que es exactamente lo que acaba de ocurrir.
//
// Coste: cero en el camino caliente. La alternativa —introspección por
// petición— convertiría al IdP en dependencia dura de cada llamada de cada
// servicio, y este producto se dimensiona para cientos de miles de usuarios.
//
// Alcance: GLOBAL y deliberado. Echa a todo el mundo, incluidos los admins de
// plataforma, que vuelven a entrar con su segundo factor. Ante un fallo de rol no
// se sabe a quién afecta, así que un interruptor selectivo sería el que no
// sirve. Para expulsar a UNA persona está la revocación por sesión.
//
// Vive en el binario y no solo en la consola a propósito: el escenario en el que
// se usa es precisamente aquel en el que la consola es lo que ha fallado.
func runRevokeAll(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "uso: id revoke-all (sin argumentos)")
		return 2
	}
	// Se carga la MISMA configuración que el servidor: así la KEK se decodifica y
	// valida igual, y no hay una segunda forma de leerla que pueda divergir.
	cfg, err := idhttp.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuración inválida: %v\n", err)
		return 2
	}
	// La KEK es obligatoria: sin ella el material de firma se guardaría en claro
	// al rotar, degradando en silencio el cifrado en reposo (F9) justo durante
	// una emergencia.
	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clave de cifrado: %v\n", err)
		return 2
	}
	if !cipher.Enabled() {
		fmt.Fprintln(os.Stderr, "SYTUE_ID_KEY_ENCRYPTION_KEY es obligatoria: rotar sin ella dejaría la clave nueva sin cifrar en reposo")
		return 2
	}

	ctx := context.Background()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseDSN, MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conexión: %v\n", err)
		return 1
	}
	defer pool.Close()

	st := store.NewWithCipher(pool, cipher)
	km := oidc.NewKeyManager()
	if err := km.Bootstrap(ctx, st); err != nil {
		fmt.Fprintf(os.Stderr, "cargar claves: %v\n", err)
		return 1
	}
	if err := km.Rotate(ctx, st); err != nil {
		fmt.Fprintf(os.Stderr, "rotar la clave de firma: %v\n", err)
		return 1
	}
	// retireAfter=0: la anterior se retira ya. Es lo que hace la revocación
	// INMEDIATA en vez de diferida a la ventana de gracia habitual.
	retiradas, err := km.RetireExpired(ctx, st, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retirar la clave anterior: %v\n", err)
		return 1
	}
	fmt.Printf("clave de firma rotada; %d clave(s) anterior(es) retiradas\n", retiradas)
	fmt.Println("todos los access/ID tokens emitidos hasta ahora han dejado de verificar")
	fmt.Println()
	fmt.Println("ESTO NO IMPIDE VOLVER A EMITIR: quien tenga la credencial entra otra vez.")
	fmt.Println("Para cerrar de verdad, las otras dos mitades del interruptor:")
	fmt.Println("  · `id lockdown on --reason \"…\"` cierra la emisión (marca en base,")
	fmt.Println("    consultada al abrir sesión y al entregar tokens; se levanta con")
	fmt.Println("    `id lockdown off`, que no necesita consola)")
	fmt.Println("  · `02b-secretos.sh <entorno> --rotate-cookie-secret` rota el sello de")
	fmt.Println("    las cookies de las apps y reinicia los frontends: ilegibles en el acto")
	return 0
}
