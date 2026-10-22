// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/runtime"
)

// wsOriginPatterns es la allowlist de hosts admitidos en el handshake del WS de
// terminal/exec. Lista cerrada de primera parte (platform/pkg/config), nunca un
// comodín de zona: bajo sytue.eu viven también los dominios donde Cloud ejecuta
// el código de sus clientes. Depende del entorno: en prod no entran los hosts
// `.localhost` de desarrollo.
func wsOriginPatterns(env pcfg.Environment) []string {
	return strings.Split(pcfg.FirstPartyWebSocketHosts(env), ",")
}

// StreamDeploymentLogs emite logs en vivo vía SSE.
func (h *Handlers) StreamDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	depID, err := uuid.Parse(chi.URLParam(r, "deploymentId"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid deployment id."))
		return
	}

	// Sin los plazos del servidor: con su WriteTimeout el log moría a los 30 s.
	flusher, ok := httpx.AbrirSSE(w)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Streaming is not supported."))
		return
	}

	ctx := r.Context()
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := h.svc.StreamDeploymentLogs(ctx, projectID, depID, pw); err != nil {
			_, _ = fmt.Fprintf(pw, "error: %v\n", err)
		}
	}()

	writeLogFrames(w, flusher, pr)
}

// maxLogFrame acota lo que se emite en un solo evento SSE. Los logs los escribe
// CÓDIGO DEL CLIENTE: sin tope, una app que escupiera megabytes sin un solo salto
// de línea haría crecer el búfer del control-plane sin límite. Al topar, la línea
// se parte en varios eventos —se ve fea pero se ve— en vez de tumbar el proceso.
const maxLogFrame = 8 * 1024

// writeLogFrames emite UNA LÍNEA POR EVENTO SSE, que es lo que promete el contrato
// («cada línea de log llega como `data: <línea>`») y lo que exige el protocolo.
//
// Antes se leía en bloques de 4 KiB y se envolvía el bloque entero en un
// `data: <bloque>\n\n`. Como el corte era por BYTES, cada bloque contenía muchos
// saltos de línea, y en SSE un salto dentro del evento TERMINA el campo: todas las
// líneas del bloque menos la primera quedaban sin el prefijo `data: `, y un cliente
// que siga la especificación —el `EventSource` de api-clients, sin ir más lejos—
// las descarta enteras. Es decir: se perdía casi todo el log y nadie lo notaba,
// porque el visor del dashboard traía su propio despiece a medida para compensarlo.
func writeLogFrames(w io.Writer, flusher http.Flusher, r io.Reader) {
	br := bufio.NewReaderSize(r, maxLogFrame)
	for {
		line, err := br.ReadSlice('\n')
		// ErrBufferFull = línea más larga que el tope: se emite el trozo y se sigue.
		if len(line) > 0 {
			if frame := sseData(line); frame != "" {
				_, _ = io.WriteString(w, frame)
				flusher.Flush()
			}
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
}

// sseData construye el evento de una línea, o "" si no queda nada que enviar (un
// evento con data vacío no lo despacha el cliente, así que no se escribe).
//
// Se quitan TODOS los retornos de carro, no solo el del final de línea: en SSE un
// `\r` suelto también termina la línea, así que uno intercalado —los deja cualquier
// barra de progreso de npm o pip— rompería el marco y volvería a partir el evento.
func sseData(line []byte) string {
	line = bytes.TrimRight(line, "\r\n")
	if bytes.IndexByte(line, '\r') >= 0 {
		line = bytes.ReplaceAll(line, []byte("\r"), nil)
	}
	if len(line) == 0 {
		return ""
	}
	return "data: " + string(line) + "\n\n"
}

// ExecDeployment abre una sesión WS de terminal sobre el pod del deployment.
func (h *Handlers) ExecDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	depID, err := uuid.Parse(chi.URLParam(r, "deploymentId"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid deployment id."))
		return
	}
	if h.execSession == nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("The terminal is not available."))
		return
	}

	dep, err := h.svc.GetDeployment(r.Context(), depID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if dep.ProjectID != projectID {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	proj, err := h.svc.GetProject(r.Context(), projectID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	// Validación de Origin por allowlist CERRADA (la misma que Rooms y docs):
	// solo el dashboard y demás orígenes de primera parte abren el terminal. Un
	// comodín de zona casaría *.apps.sytue.eu y *.preview.sytue.eu —las
	// publicaciones de clientes de este mismo servicio—, que jamás deben poder
	// abrir un exec contra un deployment.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Más el host de la propia petición —el de la organización—: ver httpx.WSOriginPatterns.
		OriginPatterns: httpx.WSOriginPatterns(r, wsOriginPatterns(h.cfg.Env)),
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Una shell en producción queda en la auditoría al abrirse y al cerrarse,
	// con su duración (CLOUD-14). El cierre usa un contexto propio: el de la
	// petición ya está cancelado cuando el cliente se va.
	abierto := time.Now()
	h.svc.AuditExec(r.Context(), projectID, depID, -1)
	defer func(ctx context.Context) {
		h.svc.AuditExec(ctx, projectID, depID, time.Since(abierto))
	}(context.WithoutCancel(r.Context()))

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	go func() {
		defer stdoutW.Close()
		defer stdinR.Close()
		_ = h.execSession.Run(ctx, dep, proj.Slug, stdinR, stdoutW, io.Discard)
	}()

	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				_ = stdinW.Close()
				return
			}
			if _, err := stdinW.Write(data); err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, err := stdoutR.Read(buf)
		if n > 0 {
			_ = conn.Write(ctx, websocket.MessageText, buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// noopExecSession satisfies interface when exec is disabled.
type noopExecSession struct{}

func (noopExecSession) Run(context.Context, domain.Deployment, string, io.Reader, io.Writer, io.Writer) error {
	return fmt.Errorf("terminal no disponible")
}

var _ runtime.ExecSession = noopExecSession{}
