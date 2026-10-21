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
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// graphSimpleUploadMax es lo que Microsoft Graph acepta en una subida de una sola
// petición (PUT …/content). Lo que pasa de ahí va por una sesión de subida.
const graphSimpleUploadMax = 250 << 20

// graphTrozo es cada petición de una sesión de subida: Graph exige múltiplos de
// 320 KiB, y 10 MiB sube en el tope de una petición de migración con holgura.
const graphTrozo = 32 * 320 << 10

// errGraphConflicto es el 409 de Graph: ya existe algo con ese nombre.
var errGraphConflicto = errors.New("migrate: ya existe en Microsoft 365")

// m365Dest escribe en OneDrive y en la agenda de Outlook: el destino del camino de
// SALIDA hacia Microsoft 365. Se autentica igual que el origen (m365): con el
// consentimiento de quien conectó —token inyectado, `/me`— o con permisos de
// aplicación sobre la cuenta indicada.
type m365Dest struct {
	m *m365
}

// NewM365Destination construye el destino de salida hacia Microsoft 365.
func NewM365Destination(ctx context.Context, rt connector.Runtime, inst connector.Instance, user string) (Destination, error) {
	m, err := newM365(ctx, rt, inst, user)
	if err != nil {
		return nil, err
	}
	return &m365Dest{m: m}, nil
}

// Root devuelve la carpeta «Sytue» de OneDrive, creándola si no está. Como en
// Google, es una carpeta propia: reanudar no crea otra y nada cae en la raíz.
func (d *m365Dest) Root(ctx context.Context) (string, error) {
	drive := d.m.userRoot() + "/drive"
	id, err := d.idDe(ctx, drive+"/root:/Sytue")
	if err == nil {
		return id, nil
	}
	id, err = d.crearCarpeta(ctx, drive+"/root/children", "Sytue")
	if errors.Is(err, errGraphConflicto) {
		return d.idDe(ctx, drive+"/root:/Sytue")
	}
	return id, err
}

// CreateFolder crea una subcarpeta. Si ya existe (una pasada anterior la creó) se
// usa esa: OneDrive no admite dos carpetas con el mismo nombre en el mismo sitio.
func (d *m365Dest) CreateFolder(ctx context.Context, parentID, name string) (string, error) {
	nombre := nombreOneDrive(name)
	item := d.m.userRoot() + "/drive/items/" + url.PathEscape(parentID)
	id, err := d.crearCarpeta(ctx, item+"/children", nombre)
	if errors.Is(err, errGraphConflicto) {
		return d.idDe(ctx, item+":/"+url.PathEscape(nombre))
	}
	return id, err
}

// UploadFile sube un fichero de tamaño desconocido: Graph exige Content-Length, así
// que se lee entero hasta el tope de la subida simple.
func (d *m365Dest) UploadFile(ctx context.Context, folderID, name, contentType string, body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, graphSimpleUploadMax+1))
	if err != nil {
		return "", err
	}
	return d.UploadFileSized(ctx, folderID, name, contentType, int64(len(data)), bytes.NewReader(data))
}

// UploadFileSized sube un fichero cuyo tamaño ya se sabe, en streaming.
func (d *m365Dest) UploadFileSized(ctx context.Context, folderID, name, contentType string, size int64, body io.Reader) (string, error) {
	if size > graphSimpleUploadMax {
		return d.porPartes(ctx, folderID, name, size, body)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// replace y no rename: reanudar un trabajo cortado a mitad de fichero debe
	// dejar UNA copia completa, no «informe (1).pdf» junto a una a medias.
	u := d.m.userRoot() + "/drive/items/" + url.PathEscape(folderID) + ":/" +
		url.PathEscape(nombreOneDrive(name)) + ":/content?@microsoft.graph.conflictBehavior=replace"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	var creado struct {
		ID string `json:"id"`
	}
	if err := d.do(req, &creado); err != nil {
		return "", err
	}
	return creado.ID, nil
}

// porPartes sube un fichero grande por una sesión de subida de OneDrive.
func (d *m365Dest) porPartes(ctx context.Context, folderID, name string, size int64, body io.Reader) (string, error) {
	u := d.m.userRoot() + "/drive/items/" + url.PathEscape(folderID) + ":/" +
		url.PathEscape(nombreOneDrive(name)) + ":/createUploadSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u,
		strings.NewReader(`{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var sesion struct {
		UploadURL string `json:"uploadUrl"` //nolint:tagliatelle // contrato de Microsoft Graph.
	}
	if err := d.do(req, &sesion); err != nil {
		return "", err
	}
	return d.enSesion(ctx, sesion.UploadURL, size, body)
}

// enSesion manda el contenido a una URL de sesión de subida de Graph (la de un
// fichero de OneDrive o la de un adjunto) en trozos, y devuelve el id del final.
// La URL va autenticada por sí misma y en otro host: sin el token de la pasarela.
func (d *m365Dest) enSesion(ctx context.Context, uploadURL string, size int64, body io.Reader) (string, error) {
	if uploadURL == "" {
		return "", errors.New("migrate: Microsoft 365 no abrió la sesión de subida")
	}
	sctx := policy.SinCredencial(ctx)
	trozo := make([]byte, graphTrozo)
	var creado struct {
		ID string `json:"id"`
	}
	for desde := int64(0); desde < size; {
		n, err := io.ReadFull(body, trozo[:min(int64(len(trozo)), size-desde)])
		if err != nil {
			return "", fmt.Errorf("migrate: leer el contenido a mitad de subida: %w", err)
		}
		put, err := http.NewRequestWithContext(sctx, http.MethodPut, uploadURL, bytes.NewReader(trozo[:n]))
		if err != nil {
			return "", err
		}
		put.ContentLength = int64(n)
		put.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", desde, desde+int64(n)-1, size))
		if err := d.enviar(put, &creado, false); err != nil {
			return "", err
		}
		desde += int64(n)
	}
	return creado.ID, nil
}

// AddressBook: los contactos de Outlook que se crean por `/contacts` van a la agenda
// predeterminada, así que no hay libreta que elegir.
func (d *m365Dest) AddressBook(context.Context) (string, error) { return "m365:contacts", nil }

// CreateContact crea un contacto en la agenda de Outlook.
func (d *m365Dest) CreateContact(ctx context.Context, _ string, c Contact) (string, error) {
	//nolint:tagliatelle // contrato de Microsoft Graph (camelCase).
	type correo struct {
		Address string `json:"address"`
		Name    string `json:"name,omitempty"`
	}
	//nolint:tagliatelle // contrato de Microsoft Graph (camelCase).
	contacto := struct {
		DisplayName    string   `json:"displayName,omitempty"`
		GivenName      string   `json:"givenName,omitempty"`
		Surname        string   `json:"surname,omitempty"`
		CompanyName    string   `json:"companyName,omitempty"`
		JobTitle       string   `json:"jobTitle,omitempty"`
		PersonalNotes  string   `json:"personalNotes,omitempty"`
		EmailAddresses []correo `json:"emailAddresses,omitempty"`
		BusinessPhones []string `json:"businessPhones,omitempty"`
	}{
		DisplayName: c.FormattedName, GivenName: c.GivenName, Surname: c.FamilyName,
		CompanyName: c.Organization, JobTitle: c.Title, PersonalNotes: c.Note,
		BusinessPhones: c.Phones,
	}
	for _, e := range c.Emails {
		contacto.EmailAddresses = append(contacto.EmailAddresses, correo{Address: e, Name: c.FormattedName})
	}
	raw, err := json.Marshal(contacto)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.m.userRoot()+"/contacts", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creado struct {
		ID string `json:"id"`
	}
	if err := d.do(req, &creado); err != nil {
		return "", err
	}
	return creado.ID, nil
}

func (d *m365Dest) crearCarpeta(ctx context.Context, children, nombre string) (string, error) {
	//nolint:tagliatelle // contrato de Microsoft Graph.
	cuerpo := struct {
		Name     string   `json:"name"`
		Folder   struct{} `json:"folder"`
		Conflict string   `json:"@microsoft.graph.conflictBehavior"`
	}{Name: nombre, Conflict: "fail"}
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, children, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var creada struct {
		ID string `json:"id"`
	}
	if err := d.do(req, &creada); err != nil {
		return "", err
	}
	return creada.ID, nil
}

func (d *m365Dest) idDe(ctx context.Context, item string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item+"?$select=id", http.NoBody)
	if err != nil {
		return "", err
	}
	var it struct {
		ID string `json:"id"`
	}
	if err := d.do(req, &it); err != nil {
		return "", err
	}
	if it.ID == "" {
		return "", errors.New("migrate: Microsoft 365 no devolvió el id")
	}
	return it.ID, nil
}

// do emite la petición por el egress de la pasarela y traduce el error.
func (d *m365Dest) do(req *http.Request, dst any) error { return d.enviar(req, dst, true) }

// enviar es do con la opción de NO poner el token de aplicación (URL de sesión).
func (d *m365Dest) enviar(req *http.Request, dst any, conToken bool) error {
	if conToken && d.m.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.m.token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := d.m.rt.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: escribir en Microsoft 365: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusConflict:
		return errGraphConflicto
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf(
			"migrate: Microsoft 365 rechazó la escritura (HTTP %d): la conexión no tiene permiso para crear; "+
				"vuelve a autorizarla aceptando los permisos de escritura: %w", resp.StatusCode, ErrSinPermiso,
		)
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w: Microsoft 365 no acepta un fichero de este tamaño", ErrUnimportable)
	case resp.StatusCode == http.StatusBadRequest:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: Microsoft 365 no acepta esta entrada: %s", ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Microsoft 365"}
	}
	if dst == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst)
}

// nombreOneDrive cambia por «_» lo que OneDrive no admite en un nombre (" * : < > ?
// / \ |) y quita el punto o espacio final, que tampoco acepta. «Reunión 10:00.txt»
// llega como «Reunión 10_00.txt» en vez de no llegar.
func nombreOneDrive(name string) string {
	limpio := strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`"*:<>?/\|`, r) {
			return '_'
		}
		return r
	}, name)
	limpio = strings.TrimRight(limpio, ". ")
	if limpio == "" {
		return "_" + strconv.Itoa(len(name))
	}
	return limpio
}

var (
	_ Destination   = (*m365Dest)(nil)
	_ SizedUploader = (*m365Dest)(nil)
)
