// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenSource devuelve el Bearer con el que escribir en Sytue, VÁLIDO AHORA.
//
// Es una función y no una cadena porque el token de destino es de impersonación
// (RFC 8693): dura 5 minutos y una migración dura más. Se pide en CADA petición y
// la implementación renueva cuando toca — asumir que el primero vale para todo el
// trabajo es el fallo que deja las migraciones grandes a medias con un 401.
type TokenSource func(ctx context.Context) (string, error)

// StaticToken es un TokenSource de un solo token (tests y usos sin impersonación).
func StaticToken(token string) TokenSource {
	return func(context.Context) (string, error) { return token, nil }
}

// Sytue escribe en el destino por la API PÚBLICA de services/drive y
// services/contacts (nunca contra sus bases de datos): así la migración hereda
// sus permisos, sus cuotas, su papelera y su auditoría sin duplicar reglas.
//
// Usa un *http.Client normal, NO el Runtime.HTTP de la pasarela: ese es
// exclusivamente para el egress a terceros y su barrera SSRF bloquearía —con
// razón— un host interno de la suite. Mismo criterio que internal/directory.
type Sytue struct {
	drive    string
	contacts string
	token    TokenSource
	hc       *http.Client
}

// NewSytue construye el cliente de destino. token mintea el Bearer del USUARIO
// OBJETIVO en cuya cuenta aterriza el contenido. hc es opcional.
func NewSytue(driveBase, contactsBase string, token TokenSource, hc *http.Client) *Sytue {
	if hc == nil {
		// Sin timeout de cliente: una subida grande puede durar; el corte lo pone
		// el contexto del trabajo, no un reloj fijo por petición.
		hc = &http.Client{Timeout: 0}
	}
	return &Sytue{
		drive:    strings.TrimRight(driveBase, "/"),
		contacts: strings.TrimRight(contactsBase, "/"),
		token:    token,
		hc:       hc,
	}
}

// Root devuelve el id de la carpeta raíz del usuario en Sytue Drive.
func (s *Sytue) Root(ctx context.Context) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodGet, s.drive+"/api/v1/folders/root", nil, nil, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// CreateFolder crea una subcarpeta y devuelve su id.
func (s *Sytue) CreateFolder(ctx context.Context, parentID, name string) (string, error) {
	body, err := json.Marshal(map[string]string{"parent_id": parentID, "name": name})
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	hdr := http.Header{"Content-Type": []string{"application/json"}}
	if err := s.call(ctx, http.MethodPost, s.drive+"/api/v1/folders", bytes.NewReader(body), hdr, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// UploadFile sube el contenido a una carpeta EN STREAMING (el fichero nunca se
// carga entero en memoria) y devuelve el id del fichero creado.
//
// ponytail: se preservan NOMBRE y RUTA (la jerarquía se recrea carpeta a
// carpeta). La FECHA DE MODIFICACIÓN de origen no se preserva: la API de Sytue
// Drive no acepta un created_at/updated_at de cliente, y falsear el metadato en
// su base de datos está fuera de contrato. Se informa como tal en el reporte del
// trabajo, no se finge. Añadirlo cuando services/drive exponga el campo.
func (s *Sytue) UploadFile(ctx context.Context, folderID, name, contentType string, body io.Reader) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	hdr := http.Header{
		"Content-Type": []string{contentType},
		"X-File-Name":  []string{name},
	}
	var out struct {
		ID string `json:"id"`
	}
	u := s.drive + "/api/v1/folders/" + url.PathEscape(folderID) + "/files"
	if err := s.call(ctx, http.MethodPost, u, body, hdr, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// AddressBook devuelve el id de la libreta por defecto del usuario (el listado la
// crea si aún no existe).
func (s *Sytue) AddressBook(ctx context.Context) (string, error) {
	var out struct {
		AddressBooks []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"is_default"`
		} `json:"address_books"`
	}
	if err := s.call(ctx, http.MethodGet, s.contacts+"/api/v1/address-books", nil, nil, &out); err != nil {
		return "", err
	}
	if len(out.AddressBooks) == 0 {
		return "", fmt.Errorf("migrate: el usuario no tiene ninguna libreta de direcciones en Sytue Contacts")
	}
	for _, b := range out.AddressBooks {
		if b.IsDefault {
			return b.ID, nil
		}
	}
	return out.AddressBooks[0].ID, nil
}

// CreateContact crea un contacto en la libreta indicada y devuelve su id.
func (s *Sytue) CreateContact(ctx context.Context, bookID string, c Contact) (string, error) {
	type entry struct {
		Email     string `json:"email,omitempty"`
		Phone     string `json:"phone,omitempty"`
		IsPrimary bool   `json:"is_primary,omitempty"`
	}
	req := struct {
		Contact
		Emails []entry `json:"emails,omitempty"`
		Phones []entry `json:"phones,omitempty"`
	}{Contact: c}
	for i, e := range c.Emails {
		req.Emails = append(req.Emails, entry{Email: e, IsPrimary: i == 0})
	}
	for i, p := range c.Phones {
		req.Phones = append(req.Phones, entry{Phone: p, IsPrimary: i == 0})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	hdr := http.Header{"Content-Type": []string{"application/json"}}
	u := s.contacts + "/api/v1/address-books/" + url.PathEscape(bookID) + "/contacts"
	if err := s.call(ctx, http.MethodPost, u, bytes.NewReader(body), hdr, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// call emite una petición autenticada al destino y decodifica la respuesta. Un
// status != 2xx se devuelve con el cuerpo acotado: los errores del destino
// (cuota, tamaño, permiso) tienen que llegar LEGIBLES al informe del trabajo.
func (s *Sytue) call(ctx context.Context, method, rawURL string, body io.Reader, hdr http.Header, dst any) error {
	if body == nil {
		body = http.NoBody
	}
	// El token se pide AQUÍ, no al construir el cliente: es de vida corta y su
	// fuente lo renueva sola durante el trabajo.
	token, err := s.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	for k, vs := range hdr {
		req.Header[k] = vs
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s %s: %w", method, redactPath(rawURL), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("migrate: Sytue rechazó la escritura (HTTP %d): el permiso para escribir en la cuenta de destino no es válido; comprueba que esa persona sigue activa en la organización y reanuda el trabajo", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("migrate: Sytue respondió HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue ilegible: %w", err)
	}
	return nil
}

// suiteError es un fallo de un servicio INTERNO de la suite (Sytue Git, Sytue
// Rooms) con su código, para poder distinguir «no existe» de «ya existe» de «no se
// puede» sin comparar cadenas sueltas.
type suiteError struct {
	status  int
	msg     string
	service string
}

func (e *suiteError) Error() string {
	return fmt.Sprintf("migrate: %s respondió HTTP %d: %s", e.service, e.status, e.msg)
}

// asSuiteError es errors.As sobre suiteError (azúcar para los predicados de estado).
func asSuiteError(err error, dst **suiteError) bool { return errors.As(err, dst) }

// isNotFound / isConflict clasifican el fallo de un servicio interno.
func isNotFound(err error) bool {
	var se *suiteError
	return asSuiteError(err, &se) && se.status == http.StatusNotFound
}

func isConflict(err error) bool {
	var se *suiteError
	return asSuiteError(err, &se) && se.status == http.StatusConflict
}

// defaultSuiteClient es el cliente hacia los servicios de la suite. Timeout amplio
// —importar un repositorio o crear un canal no es instantáneo— pero acotado: el
// corte de verdad lo pone el contexto del trabajo.
func defaultSuiteClient() *http.Client { return &http.Client{Timeout: 2 * time.Minute} }

// redactPath deja solo host+ruta de una URL para los mensajes de error (nunca la
// query, que podría llevar material sensible).
func redactPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "(url inválida)"
	}
	return u.Host + u.Path
}

var _ Destination = (*Sytue)(nil)
