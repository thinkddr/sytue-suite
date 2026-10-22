// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SytueMail es el destino del buzón. Habla el JMAP público de Sytue Mail —no su
// base de datos—, así que la migración hereda sus cuotas, el indexado de hilos y
// su auditoría, y un correo importado se comporta exactamente igual que uno
// recibido.
//
// Dos llamadas por mensaje: subir el .eml como blob y `Email/import`, que es el
// método del estándar (RFC 8621 §2.7) para meter correo que NO llega por SMTP,
// preservando su fecha y su estado.
type SytueMail struct {
	base  string
	token TokenSource
	hc    *http.Client

	// La cuenta y las carpetas se resuelven UNA vez por trabajo: son las mismas
	// para los miles de mensajes que vienen detrás.
	once       sync.Once
	accountID  string
	accountErr error
	mu         sync.Mutex
	folders    map[string]string // nombre normalizado → id
}

// NewSytueMail construye el cliente de Sytue Mail. hc es opcional.
func NewSytueMail(baseURL string, token TokenSource, hc *http.Client) *SytueMail {
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	return &SytueMail{base: strings.TrimRight(baseURL, "/"), token: token, hc: hc, folders: map[string]string{}}
}

// jmapCall es una llamada JMAP con un solo método (el lote no aporta nada aquí:
// cada mensaje necesita su blob subido antes).
func (s *SytueMail) jmapCall(ctx context.Context, method string, args map[string]any, out any) error {
	body := map[string]any{
		"using":       []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": [][]any{{method, args, "c0"}},
	}
	//nolint:tagliatelle // los tags son el CONTRATO de JMAP (RFC 8620, camelCase).
	var resp struct {
		MethodResponses []json.RawMessage `json:"methodResponses"`
	}
	if err := s.call(ctx, http.MethodPost, "/jmap", body, &resp); err != nil {
		return err
	}
	if len(resp.MethodResponses) == 0 {
		return fmt.Errorf("migrate: Sytue Mail no respondió a %s", method)
	}
	// La respuesta es ["Método", {…}, "callId"]: el error viaja en la posición del
	// método ("error"), así que se mira ANTES de decodificar el resultado.
	var triple []json.RawMessage
	if err := json.Unmarshal(resp.MethodResponses[0], &triple); err != nil || len(triple) < 2 {
		return fmt.Errorf("migrate: respuesta JMAP ilegible de Sytue Mail")
	}
	var name string
	_ = json.Unmarshal(triple[0], &name)
	if name == "error" {
		return fmt.Errorf("%w: Sytue Mail rechazó %s: %s", ErrUnimportable, method, string(triple[1]))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(triple[1], out)
}

// account resuelve el id de la cuenta de correo de la persona destino.
func (s *SytueMail) account(ctx context.Context) (string, error) {
	s.once.Do(func() {
		//nolint:tagliatelle // contrato de la sesión JMAP (RFC 8620 §2).
		var session struct {
			PrimaryAccounts map[string]string `json:"primaryAccounts"`
			Accounts        map[string]any    `json:"accounts"`
		}
		if err := s.call(ctx, http.MethodGet, "/jmap/session", nil, &session); err != nil {
			s.accountErr = err
			return
		}
		if id := session.PrimaryAccounts["urn:ietf:params:jmap:mail"]; id != "" {
			s.accountID = id
			return
		}
		for id := range session.Accounts {
			s.accountID = id
			return
		}
		s.accountErr = fmt.Errorf("migrate: Sytue Mail no declara ninguna cuenta para esa persona")
	})
	return s.accountID, s.accountErr
}

// EnsureFolder devuelve el id de la carpeta destino, creándola si no existe.
//
// Las carpetas con papel conocido (bandeja, enviados, papelera…) se reutilizan
// por su ROL, no por su nombre: el buzón de destino ya tiene su «Inbox» y crear
// otra carpeta llamada «Recibidos» dejaría el correo importado fuera de la vista
// principal.
func (s *SytueMail) EnsureFolder(ctx context.Context, f MailFolder) (string, error) {
	accountID, err := s.account(ctx)
	if err != nil {
		return "", err
	}
	if err := s.loadFolders(ctx, accountID); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if f.Role != "" {
		if id, ok := s.folders["role:"+f.Role]; ok {
			return id, nil
		}
	}
	if id, ok := s.folders[strings.ToLower(f.Name)]; ok {
		return id, nil
	}

	//nolint:tagliatelle // contrato de /set de JMAP.
	var created struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
		NotCreated map[string]any `json:"notCreated"`
	}
	err = s.jmapCall(ctx, "Mailbox/set", map[string]any{
		"accountId": accountID,
		"create":    map[string]any{"new": map[string]any{"name": f.Name}},
	}, &created)
	if err != nil {
		return "", err
	}
	if c, ok := created.Created["new"]; ok && c.ID != "" {
		s.folders[strings.ToLower(f.Name)] = c.ID
		return c.ID, nil
	}
	return "", fmt.Errorf("%w: Sytue Mail no pudo crear la carpeta %q: %v", ErrUnimportable, f.Name, created.NotCreated)
}

// loadFolders trae el mapa de carpetas del destino (una vez por trabajo).
func (s *SytueMail) loadFolders(ctx context.Context, accountID string) error {
	s.mu.Lock()
	loaded := len(s.folders) > 0
	s.mu.Unlock()
	if loaded {
		return nil
	}
	var got struct {
		List []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"list"`
	}
	if err := s.jmapCall(ctx, "Mailbox/get", map[string]any{"accountId": accountID}, &got); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, mb := range got.List {
		s.folders[strings.ToLower(mb.Name)] = mb.ID
		if mb.Role != "" {
			s.folders["role:"+strings.ToLower(mb.Role)] = mb.ID
		}
	}
	return nil
}

// ImportMessage sube el .eml y lo importa en la carpeta indicada, conservando la
// fecha y el estado que traía del origen.
func (s *SytueMail) ImportMessage(ctx context.Context, folderID string, raw []byte, m MailMessage) (string, error) {
	accountID, err := s.account(ctx)
	if err != nil {
		return "", err
	}
	blobID, err := s.upload(ctx, accountID, raw)
	if err != nil {
		return "", err
	}

	keywords := map[string]bool{}
	if m.Seen {
		keywords["$seen"] = true
	}
	if m.Flagged {
		keywords["$flagged"] = true
	}
	if m.Draft {
		keywords["$draft"] = true
	}
	email := map[string]any{
		"blobId":     blobID,
		"mailboxIds": map[string]bool{folderID: true},
		"keywords":   keywords,
	}
	if !m.ReceivedAt.IsZero() {
		email["receivedAt"] = m.ReceivedAt.UTC().Format(time.RFC3339)
	}

	//nolint:tagliatelle // contrato de Email/import (RFC 8621 §2.7).
	var out struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
		NotCreated map[string]any `json:"notCreated"`
	}
	if err := s.jmapCall(ctx, "Email/import", map[string]any{
		"accountId": accountID,
		"emails":    map[string]any{"m": email},
	}, &out); err != nil {
		return "", err
	}
	if c, ok := out.Created["m"]; ok && c.ID != "" {
		return c.ID, nil
	}
	// Que el destino lo rechace es definitivo: reintentarlo daría lo mismo.
	return "", fmt.Errorf("%w: Sytue Mail no importó el mensaje: %v", ErrUnimportable, out.NotCreated)
}

// upload sube el .eml crudo y devuelve su blobId.
func (s *SytueMail) upload(ctx context.Context, accountID string, raw []byte) (string, error) {
	token, err := s.token(ctx)
	if err != nil {
		return "", err
	}
	u := s.base + "/jmap/upload/" + url.PathEscape(accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("Accept", "application/json")

	resp, err := s.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("migrate: subir el mensaje a Sytue Mail: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusBadRequest {
		// Mensaje por encima del tamaño máximo del buzón: definitivo.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("%w: Sytue Mail no acepta este mensaje: %s", ErrUnimportable, strings.TrimSpace(string(msg)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Mail"}
	}
	//nolint:tagliatelle // contrato de subida de blobs (RFC 8620 §6.1).
	var out struct {
		BlobID string `json:"blobId"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("migrate: respuesta de subida ilegible: %w", err)
	}
	if out.BlobID == "" {
		return "", fmt.Errorf("migrate: Sytue Mail no devolvió el identificador del mensaje subido")
	}
	return out.BlobID, nil
}

// call emite una petición JSON autenticada a Sytue Mail.
func (s *SytueMail) call(ctx context.Context, method, path string, body, dst any) error {
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
		return fmt.Errorf("migrate: Sytue Mail rechazó la escritura (HTTP %d): el permiso para escribir en el buzón de destino no es válido; comprueba que esa persona sigue activa y reanuda el trabajo", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Mail"}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Mail ilegible: %w", err)
	}
	return nil
}

var _ MailDestination = (*SytueMail)(nil)
