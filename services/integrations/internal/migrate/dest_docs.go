// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// SytueDocs es el destino de los documentos que NO son ficheros: un Google Doc,
// una hoja de cálculo o una presentación llegan a Sytue Docs como documento
// EDITABLE, no como un adjunto que nadie puede abrir sin descargarlo.
//
// El camino es el mismo que usa la persona cuando arrastra un .docx a Docs, y por
// eso no hay conversores duplicados en la pasarela:
//
//  1. POST /api/v1/import convierte el fichero ofimático a {kind, title, content}
//     con el importador de Docs (docx, xlsx, pptx, odf, csv, md, html, txt);
//  2. POST /api/v1/documents crea el documento del tipo devuelto;
//  3. POST /api/v1/documents/{id}/ops siembra el contenido con UNA operación
//     `set` sobre el registro donde vive todo el documento.
//
// Si mañana Docs aprende un formato nuevo, esta migración lo hereda sin tocar
// nada aquí.
type SytueDocs struct {
	base  string
	token TokenSource
	hc    *http.Client
}

// NewSytueDocs construye el cliente de Sytue Docs. hc es opcional.
func NewSytueDocs(baseURL string, token TokenSource, hc *http.Client) *SytueDocs {
	if hc == nil {
		// Sin timeout de cliente: una conversión de una hoja grande puede tardar; el
		// corte lo pone el contexto del trabajo.
		hc = &http.Client{Timeout: 0}
	}
	return &SytueDocs{base: strings.TrimRight(baseURL, "/"), token: token, hc: hc}
}

// contentKeyOf es dónde vive TODO el contenido de cada tipo de documento (el
// registro LWW del editor). Es el mismo contrato que usa la app de Docs al
// importar; si no coincidiera, el documento se crearía vacío.
func contentKeyOf(kind string) (string, error) {
	switch kind {
	case "document":
		return "doc:html", nil
	case "spreadsheet":
		return "doc:sheet", nil
	case "presentation":
		return "doc:slides", nil
	default:
		return "", fmt.Errorf("migrate: Sytue Docs devolvió un tipo de documento inesperado: %q", kind)
	}
}

// ImportDocument convierte y crea el documento. Devuelve su id y su tipo.
func (s *SytueDocs) ImportDocument(ctx context.Context, filename string, body io.Reader) (string, string, error) {
	conv, err := s.convert(ctx, filename, body)
	if err != nil {
		return "", "", err
	}
	key, err := contentKeyOf(conv.Kind)
	if err != nil {
		return "", "", err
	}

	var created struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
	}
	title := conv.Title
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(filename, extOf(filename))
	}
	if err := s.call(ctx, http.MethodPost, "/api/v1/documents",
		map[string]string{"kind": conv.Kind, "title": title}, &created); err != nil {
		return "", "", err
	}
	if created.Document.ID == "" {
		return "", "", fmt.Errorf("migrate: Sytue Docs no devolvió el id del documento creado")
	}
	if conv.Content == "" {
		// Documento vacío en origen: existe y es editable, no hay nada que sembrar.
		return created.Document.ID, conv.Kind, nil
	}

	// Una sola operación 'set': el documento nace con su contenido íntegro y con
	// un reloj lógico limpio, como cuando lo importa una persona desde la app.
	op := map[string]any{"ops": []map[string]any{{
		"kind":    "set",
		"dot":     map[string]any{"a": uuid.NewString(), "c": 1},
		"key":     key,
		"value":   conv.Content,
		"lamport": 1,
	}}}
	if err := s.call(ctx, http.MethodPost,
		"/api/v1/documents/"+url.PathEscape(created.Document.ID)+"/ops", op, nil); err != nil {
		return "", "", err
	}
	return created.Document.ID, conv.Kind, nil
}

// importResult es lo que devuelve el convertidor de Docs.
type importResult struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// convert sube el fichero al convertidor de Docs (multipart, campo "file")
// SIN cargarlo entero en memoria: el cuerpo se escribe con un pipe mientras la
// petición viaja.
func (s *SytueDocs) convert(ctx context.Context, filename string, body io.Reader) (importResult, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", safeFileName(filename))
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, body); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.CloseWithError(mw.Close())
	}()
	defer func() { _ = pr.Close() }()

	token, err := s.token(ctx)
	if err != nil {
		return importResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/api/v1/import", pr)
	if err != nil {
		return importResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := s.hc.Do(req)
	if err != nil {
		return importResult{}, fmt.Errorf("migrate: %s %s: %w", http.MethodPost, redactPath(s.base+"/api/v1/import"), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return importResult{}, fmt.Errorf("%w: Sytue Docs no acepta un fichero tan grande: %s",
			ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode == http.StatusBadRequest:
		// Formato que el convertidor no reconoce: reintentarlo daría lo mismo.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return importResult{}, fmt.Errorf("%w: Sytue Docs no sabe convertir este documento: %s",
			ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return importResult{}, &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Docs"}
	}

	var out importResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&out); err != nil {
		return importResult{}, fmt.Errorf("migrate: respuesta de Sytue Docs ilegible: %w", err)
	}
	return out, nil
}

// call emite una petición JSON autenticada a Sytue Docs.
func (s *SytueDocs) call(ctx context.Context, method, path string, body, dst any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	token, err := s.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s %s: %w", method, redactPath(s.base+path), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("migrate: Sytue Docs rechazó la escritura (HTTP %d): el permiso para escribir en la cuenta de destino no es válido; comprueba que esa persona sigue activa en la organización y reanuda el trabajo", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Docs"}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Docs ilegible: %w", err)
	}
	return nil
}

// extOf devuelve la extensión de un nombre de fichero ("" si no tiene).
func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[i:]
	}
	return ""
}

var _ DocsDestination = (*SytueDocs)(nil)
