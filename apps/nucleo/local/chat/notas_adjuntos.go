// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Adjuntos y hilos de las notas.

var (
	_ Adjuntos     = (*Notas)(nil)
	_ HilosPropios = (*Notas)(nil)
)

func (n *Notas) rutaAdjunto(id string) (string, error) {
	if len(id) != 24 || strings.Trim(id, "0123456789abcdef") != "" {
		return "", fmt.Errorf("chat: adjunto %q desconocido", id)
	}
	return filepath.Join(n.carpeta, "adjuntos", id), nil
}

// EnviarFicheros copia los ficheros a `<carpeta>/adjuntos/` (uno por identificador; el
// nombre va en la nota) y apunta la nota con ellos.
func (n *Notas) EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error) {
	if len(fs) == 0 {
		return n.Enviar(ctx, canal, raiz, texto)
	}
	if err := os.MkdirAll(filepath.Join(n.carpeta, "adjuntos"), 0o700); err != nil {
		return api.RoomsMessage{}, err
	}
	var adj []api.RoomsAttachment
	quitar := func() {
		for _, a := range adj {
			if r, err := n.rutaAdjunto(a.FileID); err == nil {
				_ = os.Remove(r)
			}
		}
	}
	for _, f := range fs {
		id := nuevoID()
		destino, _ := n.rutaAdjunto(id)
		tam, err := copiarA(destino, f)
		if err != nil {
			quitar()
			return api.RoomsMessage{}, err
		}
		adj = append(adj, api.RoomsAttachment{FileID: id, Name: f.Nombre, ContentType: f.Tipo, SizeBytes: tam})
	}
	m, err := n.Enviar(ctx, canal, raiz, texto)
	if err != nil {
		quitar()
		return m, err
	}
	m, err = n.cambiar(canal, m.ID, func(x *api.RoomsMessage) { x.Attachments = adj })
	if err != nil {
		return m, err
	}
	n.avisar("message_edited", canal, m)
	return m, nil
}

func copiarA(destino string, f Fichero) (int64, error) {
	rc, err := f.Abrir()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	out, err := os.OpenFile(destino, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	tam, err := io.Copy(out, rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(destino)
	}
	return tam, err
}

// Bajar abre el adjunto guardado; la miniatura es el propio fichero (está en el disco).
func (n *Notas) Bajar(_ context.Context, _, fichero string, _ bool) (io.ReadCloser, error) {
	r, err := n.rutaAdjunto(fichero)
	if err != nil {
		return nil, err
	}
	return os.Open(r)
}

// MisHilos son las notas con respuestas: todas son de quien mira.
func (n *Notas) MisHilos(context.Context) ([]api.RoomsThreadSummary, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []api.RoomsThreadSummary
	for _, c := range n.canales {
		ms, err := n.cargar(c.ID)
		if err != nil {
			continue
		}
		for _, m := range ms {
			if m.ParentID != nil || m.ReplyCount == 0 {
				continue
			}
			m.AuthorName = cmp.Or(m.AuthorName, n.Autor)
			m.ThreadParticipants = []api.RoomsThreadParticipant{{Sub: Yo}}
			h := api.RoomsThreadSummary{Root: m, LastActivity: m.CreatedAt}
			if m.LastReplyAt != nil {
				h.LastActivity = *m.LastReplyAt
			}
			out = append(out, h)
		}
	}
	return out, nil
}
