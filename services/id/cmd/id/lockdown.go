// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	idhttp "git.sytue.eu/sytue/one/services/id/internal/http"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// BLOQUEO DE EMERGENCIA — el interruptor completo, y su rescate.
//
//	id lockdown on --reason "…" [--actor "…"]
//	id lockdown off --reason "…"
//	id lockdown status
//
// `revoke-all` invalida en el acto TODO lo emitido, y rotar el `cookie-secret`
// (`02b-secretos.sh --rotate-cookie-secret`) hace lo mismo con las cookies de
// las diez superficies. Ninguna de las dos cosas impide EMITIR de nuevo: quien
// tenga la credencial —que es justo el supuesto de la emergencia— vuelve a
// entrar un segundo después. Esta marca cierra la emisión hasta que alguien la
// levante: se consulta en la base al abrir sesión y al entregar tokens.
//
// El RESCATE vive aquí y no en la consola a propósito, y es la razón de que el
// subcomando exista: con el bloqueo puesto nadie puede abrir sesión, así que
// una consola web para quitarlo sería una puerta que se cierra por dentro con
// la llave dentro. Basta con el DSN, que es lo que sigue habiendo cuando lo
// demás ha fallado:
//
//	kubectl -n sytue run lockdown --rm -it --restart=Never \
//	  --image=<registry>/sytue-id:<tag> --overrides='…' -- lockdown off --reason "…"
func runLockdown(args []string) int {
	accion := ""
	if len(args) > 0 {
		accion = args[0]
	}
	motivo, actor, err := lockdownFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	switch accion {
	case "on", "off", "status":
	default:
		fmt.Fprintln(os.Stderr, "uso: id lockdown <on|off|status> [--reason \"motivo\"] [--actor \"quién\"]")
		return 2
	}
	// Motivo obligatorio para MOVER el interruptor, por lo mismo que en la
	// administración de plataforma: un corte total —o su levantamiento— sin
	// motivo escrito es indistinguible de un error seis meses después.
	if accion != "status" && motivo == "" {
		fmt.Fprintf(os.Stderr, "falta --reason: %q se audita y sin motivo no se puede reconstruir\n", accion)
		return 2
	}

	cfg, err := idhttp.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuración inválida: %v\n", err)
		return 2
	}
	ctx := context.Background()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseDSN, MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conexión: %v\n", err)
		return 1
	}
	defer pool.Close()
	st := store.New(pool)

	if accion == "status" {
		estado, err := st.GetLockdown(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "leer el bloqueo: %v\n", err)
			return 1
		}
		if !estado.Active {
			fmt.Println("bloqueo de emergencia: INACTIVO (se puede abrir sesión y emitir tokens)")
			return 0
		}
		fmt.Printf("bloqueo de emergencia: ACTIVO desde %s\n", estado.ChangedAt.UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("  motivo: %s\n", estado.Reason)
		if estado.Actor != "" {
			fmt.Printf("  lo puso: %s\n", estado.Actor)
		}
		return 0
	}

	activar := accion == "on"
	if err := st.SetLockdown(ctx, nil, activar, actor, motivo); err != nil {
		fmt.Fprintf(os.Stderr, "escribir el bloqueo: %v\n", err)
		return 1
	}
	if activar {
		fmt.Println("bloqueo de emergencia ACTIVADO: no se abren sesiones nuevas ni se emiten tokens")
		fmt.Println("las sesiones y tokens YA emitidos siguen siendo válidos hasta caducar:")
		fmt.Println("  · `id revoke-all` los invalida en el acto rotando la clave de firma")
		fmt.Println("  · `02b-secretos.sh <entorno> --rotate-cookie-secret` hace lo mismo con las cookies")
		fmt.Println("para levantarlo: `id lockdown off --reason \"…\"`")
		return 0
	}
	fmt.Println("bloqueo de emergencia RETIRADO: se puede volver a entrar")
	return 0
}

// lockdownFlags lee `--reason` y `--actor`. Función pura sobre los argumentos
// para poder probar cada rechazo sin tocar el proceso.
func lockdownFlags(args []string) (motivo, actor string, err error) {
	for i := 1; i < len(args); i++ {
		bandera := args[i]
		if bandera != "--reason" && bandera != "--actor" {
			return "", "", fmt.Errorf("argumento no reconocido: %q", bandera)
		}
		// El valor se toma de lo que QUEDA tras la bandera. Rebanar y comprobar la
		// longitud dice lo mismo que `i+1 >= len(args)` y además es evidente —para
		// quien lee y para el analizador— que no se indexa fuera del slice.
		resto := args[i+1:]
		if len(resto) == 0 {
			return "", "", fmt.Errorf("%s necesita un valor", bandera)
		}
		if bandera == "--reason" {
			motivo = strings.TrimSpace(resto[0])
		} else {
			actor = strings.TrimSpace(resto[0])
		}
		i++
	}
	return motivo, actor, nil
}
