// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// Los adjuntos de Sytue Rooms viven en Drive con ámbito de CANAL: fuera de «Mi unidad»
// —que es un espacio de toda la organización— y solo los sirve a quien sigue siendo
// miembro del canal (`services/drive/internal/service/channelfiles.go`). Son las rutas
// que la web usa desde `lib/drive.ts` (`driveChannelUpload`, `driveChannelDownload`).

// canal es la ruta de un fichero del canal.
func canalFichero(canal, fichero string) string {
	return "/channels/" + url.PathEscape(canal) + "/files/" + url.PathEscape(fichero)
}

// UploadToChannel sube un adjunto al ámbito de ese canal, sin pasarlo por memoria.
func (d *Drive) UploadToChannel(ctx context.Context, canal, nombre, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	resp, err := d.c.Stream(ctx, http.MethodPost, "/channels/"+url.PathEscape(canal)+"/files",
		url.Values{"name": {nombre}}, http.Header{"Content-Type": {tipo}}, cuerpo, tamano)
	if err != nil {
		return File{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var f File
	err = json.NewDecoder(resp.Body).Decode(&f)
	return f, err
}

// ChannelContent abre el contenido de un adjunto del canal; con `miniatura`, su
// miniatura (512 px como mucho). Cerrarlo es cosa de quien llama.
func (d *Drive) ChannelContent(ctx context.Context, canal, fichero string, miniatura bool) (io.ReadCloser, error) {
	ruta := canalFichero(canal, fichero) + "/content"
	if miniatura {
		ruta = canalFichero(canal, fichero) + "/thumbnail"
	}
	resp, err := d.c.Do(ctx, Request{Path: ruta})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// SaveChannelFileToMyDrive copia un adjunto del canal a «Mi unidad» de quien lo pide,
// para que le sobreviva al mensaje. El servidor reusa el contenido: no son bytes nuevos.
func (d *Drive) SaveChannelFileToMyDrive(ctx context.Context, canal, fichero string) (File, error) {
	return JSON[File](ctx, d.c, Request{Method: http.MethodPost, Path: canalFichero(canal, fichero) + "/copy"})
}
