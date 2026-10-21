// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
)

// leerSSE lee un flujo `text/event-stream` y llama a `cada` con el nombre y los datos de
// cada evento, al llegar la línea en blanco que lo cierra. Los comentarios —los
// `: latido` con que los servicios mantienen viva la conexión— se saltan. No vuelve
// hasta que el flujo se acaba.
//
// Lo usan los flujos del Hub y de Mail: dos lectores escritos a mano acabarían leyendo
// distinto el mismo protocolo.
func leerSSE(r io.Reader, cada func(evento, datos string)) error {
	lector := bufio.NewScanner(r)
	// Un evento es un aviso, no un adjunto: 1 MiB es de sobra y es el tope que impide
	// que un cuerpo mal formado —una línea que no acaba nunca— se coma la memoria de un
	// teléfono.
	lector.Buffer(make([]byte, 0, 4<<10), 1<<20)
	var evento, datos string
	for lector.Scan() {
		linea := lector.Text()
		switch {
		case linea == "": // fin del evento: se despacha lo acumulado
			if evento != "" || datos != "" {
				cada(evento, datos)
			}
			evento, datos = "", ""
		case strings.HasPrefix(linea, ":"): // comentario (latido)
		case strings.HasPrefix(linea, "event:"):
			evento = strings.TrimSpace(strings.TrimPrefix(linea, "event:"))
		case strings.HasPrefix(linea, "data:"):
			datos += strings.TrimSpace(strings.TrimPrefix(linea, "data:"))
		}
	}
	return lector.Err()
}

// Escuchar mantiene vivo un flujo que no vuelve hasta cortarse: lo reabre al cortarse,
// esperando 1 s, 2 s, 4 s… hasta `tope` mientras falle, y vuelve a empezar por 1 s en
// cuanto una conexión acaba sin error. Solo sale cuando muere `ctx`. Es lo que hace
// `EventSource` en el navegador, que reconecta solo.
func Escuchar(ctx context.Context, tope time.Duration, flujo func(context.Context) error) {
	seguidos := 0
	for ctx.Err() == nil {
		if err := flujo(ctx); err == nil {
			seguidos = 0
		}
		if ctx.Err() != nil {
			return
		}
		espera := min(tope, time.Second<<min(seguidos, 6))
		seguidos++
		t := time.NewTimer(espera)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}
