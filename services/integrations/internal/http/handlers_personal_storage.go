// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

// NUBES PERSONALES (decisión del dueño, 2026-10-19). Cada persona conecta su Google
// Drive, OneDrive o Dropbox con SOLO el consentimiento OAuth, sin pasar por Admin, y
// Sytue Drive (web y Fyne) la enseña como una ubicación propia: navegar, abrir y
// subir. La credencial es de la persona (`external_chat_actor`, igual que los chats
// externos de Rooms): nadie más de la organización la usa, y no entra en el
// almacenamiento externo de la organización (ListInstances la excluye).
//
// Google Drive va con `drive.file`: solo se ven los ficheros que la persona sube o
// crea desde Sytue. Ver lo demás pide `drive.readonly`, un permiso restringido con
// auditoría de seguridad externa obligatoria.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// ponytail: la transferencia pasa por este servicio, con su auditoría de egress; el
// tope es por fichero. Para más, subidas por trozos (sesiones de subida del proveedor).
const (
	personalStorageMaxBytes = 100 << 20
	personalStorageTimeout  = 10 * time.Minute
)

var personalStorageProviders = map[string]string{
	"google-drive": tmplGoogleDrive,
	"onedrive":     tmplOneDrive,
	"dropbox":      tmplDropbox,
}

func personalStorageProvider(key string) string {
	for p, k := range personalStorageProviders {
		if k == key {
			return p
		}
	}
	return ""
}

type personalItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Folder   bool   `json:"folder"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type personalPage struct {
	Items      []personalItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// PersonalStorage — GET /api/v1/personal-storage/accounts — mis nubes y qué proveedores puedo conectar.
func (h *Handlers) PersonalStorage(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.FromContext(r.Context())
	instances, err := h.store.ListExternalChatInstances(r.Context(), actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	off, err := h.store.DisabledIntegrations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := []map[string]any{}
	for _, inst := range instances {
		if p := personalStorageProvider(inst.TemplateKey); p != "" && !off[p] {
			items = append(items, map[string]any{"instance_id": inst.ID, "provider": p, "name": inst.Name, "connected": inst.Status == domain.StatusActive && inst.HasSecrets})
		}
	}
	providers := []map[string]any{}
	for _, p := range []string{"google-drive", "onedrive", "dropbox"} {
		_, _, flowErr := h.externalChatFlow(r.Context(), personalStorageProviders[p])
		providers = append(providers, map[string]any{"provider": p, "available": flowErr == nil && h.oauthCipher.Enabled() && !off[p], "disabled_by_org": off[p]})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "providers": providers})
}

// PersonalStorageConnect — POST /api/v1/personal-storage/connect {provider, instance_id?, return_to?}.
func (h *Handlers) PersonalStorageConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id,omitempty"`
		ReturnTo string `json:"return_to,omitempty"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	key := personalStorageProviders[req.Provider]
	section := map[string]string{"": "personal-storage", "drive": "personal-storage", "native-drive": "native-personal-storage"}[req.ReturnTo]
	if key == "" || section == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := h.integracionEncendida(r.Context(), req.Provider); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, flow, err := h.externalChatFlow(r.Context(), key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cipher, err := h.cipher()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	inst, err := h.externalChatInstance(r.Context(), key, name, req.Instance, actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	start, err := consent.Begin(cipher, flow, tenancy.MustFrom(r.Context()).Slug, actor.Subject, inst.ID, h.oauthRedirectURI(), section, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	if err := h.store.MarcarConsentimiento(r.Context(), inst.ID, start.Challenge); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"instance_id": inst.ID, "authorize_url": start.AuthorizeURL, "scopes": start.Scopes})
}

// personalStorageConnection carga la nube de quien pregunta, con el token al día.
func (h *Handlers) personalStorageConnection(r *http.Request) (connector.Instance, connector.Runtime, error) {
	id := chi.URLParam(r, "instance")
	if _, err := uuid.Parse(id); err != nil {
		return connector.Instance{}, connector.Runtime{}, httpx.ErrBadRequest
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		return connector.Instance{}, connector.Runtime{}, toAPIError(err)
	}
	conn, _, err := h.oauthConnection(r.Context(), id, func(k string) bool { return personalStorageProvider(k) != "" })
	if err != nil {
		return connector.Instance{}, connector.Runtime{}, err
	}
	if err := h.integracionEncendida(r.Context(), personalStorageProvider(conn.TemplateKey)); err != nil {
		return connector.Instance{}, connector.Runtime{}, err
	}
	rt := h.egressRuntime(conn, log.From(r.Context()))
	rt.HTTP.Timeout = personalStorageTimeout
	return conn, rt, nil
}

// PersonalStorageDisconnect — DELETE /api/v1/personal-storage/{instance}.
func (h *Handlers) PersonalStorageDisconnect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "instance")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	inst, m, cfg, err := h.loadInstanceManifest(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if personalStorageProvider(inst.TemplateKey) == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	h.revokeAtProvider(r.Context(), id, inst.TemplateKey, m, cfg)
	if err := h.store.DeleteInstance(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PersonalStorageItems — GET /api/v1/personal-storage/{instance}/items?folder=&cursor=.
func (h *Handlers) PersonalStorageItems(w http.ResponseWriter, r *http.Request) {
	conn, rt, err := h.personalStorageConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	folder, cursor := q.Get("folder"), q.Get("cursor")
	if len(folder) > 2048 || len(cursor) > 4096 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	page, err := personalList(r.Context(), conn.TemplateKey, rt, folder, cursor)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

// PersonalStorageDownload — GET /api/v1/personal-storage/{instance}/content?item=.
func (h *Handlers) PersonalStorageDownload(w http.ResponseWriter, r *http.Request) {
	conn, rt, err := h.personalStorageConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	item := r.URL.Query().Get("item")
	req, err := personalDownloadRequest(r.Context(), conn.TemplateKey, item)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := rt.HTTP.Do(req)
	if err != nil {
		httpx.Fail(w, r, errProveedorNube)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		httpx.Fail(w, r, personalProviderError(res))
		return
	}
	if res.ContentLength > personalStorageMaxBytes {
		httpx.Fail(w, r, errNubeDemasiadoGrande)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if res.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(res.ContentLength, 10))
	}
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, io.LimitReader(res.Body, personalStorageMaxBytes))
}

// PersonalStorageUpload — POST /api/v1/personal-storage/{instance}/content?folder=&name= (cuerpo: el fichero).
func (h *Handlers) PersonalStorageUpload(w http.ResponseWriter, r *http.Request) {
	conn, rt, err := h.personalStorageConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	name := strings.TrimSpace(q.Get("name"))
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Choose a valid file name."))
		return
	}
	if r.ContentLength > personalStorageMaxBytes {
		httpx.Fail(w, r, errNubeDemasiadoGrande)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, personalStorageMaxBytes+1))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if len(body) > personalStorageMaxBytes {
		httpx.Fail(w, r, errNubeDemasiadoGrande)
		return
	}
	item, err := personalUpload(r.Context(), conn.TemplateKey, rt, q.Get("folder"), name, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, item)
}

var (
	errProveedorNube       = httpx.NewError(http.StatusBadGateway, "personal_storage_unreachable", "Your cloud storage could not be reached.")
	errNubeDemasiadoGrande = httpx.NewError(http.StatusRequestEntityTooLarge, "personal_storage_too_large", "Files over 100 MB cannot be transferred from here yet.")
)

func personalProviderError(res *http.Response) error {
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return httpx.NewError(http.StatusUnprocessableEntity, "personal_storage_permission", "Your cloud account no longer allows this. Connect it again.")
	case http.StatusNotFound, http.StatusConflict:
		return httpx.ErrNotFound
	case http.StatusTooManyRequests:
		return httpx.NewError(http.StatusTooManyRequests, "personal_storage_busy", "Your cloud storage is busy. Try again in a minute.")
	}
	return errProveedorNube
}

func personalJSON(rt connector.Runtime, req *http.Request, out any) error {
	res, err := rt.HTTP.Do(req)
	if err != nil {
		return errProveedorNube
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return personalProviderError(res)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
		return errProveedorNube
	}
	return nil
}

const (
	googleFilesAPI   = "https://www.googleapis.com/drive/v3/files"
	googleUploadAPI  = "https://www.googleapis.com/upload/drive/v3/files"
	googleFolderMime = "application/vnd.google-apps.folder"
	graphDriveAPI    = "https://graph.microsoft.com/v1.0/me/drive"
	dropboxAPI       = "https://api.dropboxapi.com/2/files/"
	dropboxContent   = "https://content.dropboxapi.com/2/files/"
)

// Un id de proveedor va en una URL: solo lo que esos proveedores usan de verdad.
func personalValidID(key, id string) bool {
	if id == "" || len(id) > 1024 {
		return false
	}
	if key == tmplDropbox {
		// Dropbox identifica por ruta («/carpeta/fichero») y no admite «..».
		return strings.HasPrefix(id, "/") && !strings.Contains(id, "/../") && !strings.HasSuffix(id, "/..") && !strings.ContainsRune(id, 0)
	}
	return !strings.ContainsFunc(id, func(c rune) bool {
		return (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("-_!.", c)
	})
}

func personalList(ctx context.Context, key string, rt connector.Runtime, folder, cursor string) (personalPage, error) {
	if folder != "" && !personalValidID(key, folder) {
		return personalPage{}, httpx.ErrBadRequest
	}
	switch key {
	case tmplGoogleDrive:
		return googleList(ctx, rt, folder, cursor)
	case tmplOneDrive:
		return onedriveList(ctx, rt, folder, cursor)
	case tmplDropbox:
		return dropboxList(ctx, rt, folder, cursor)
	}
	return personalPage{}, httpx.ErrNotFound
}

func googleList(ctx context.Context, rt connector.Runtime, folder, cursor string) (personalPage, error) {
	page := personalPage{Items: []personalItem{}}
	if folder == "" {
		folder = "root"
	}
	v := url.Values{"q": {fmt.Sprintf("'%s' in parents and trashed = false", folder)}, "pageSize": {"100"}, "orderBy": {"folder,name"}, "fields": {"nextPageToken,files(id,name,mimeType,size,modifiedTime)"}}
	if cursor != "" {
		v.Set("pageToken", cursor)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, googleFilesAPI+"?"+v.Encode(), http.NoBody)
	var out struct {
		Next  string `json:"nextPageToken"` //nolint:tagliatelle // contrato de Google
		Files []struct {
			ID, Name, MimeType, ModifiedTime string
			Size                             string
		} `json:"files"`
	}
	if err := personalJSON(rt, req, &out); err != nil {
		return page, err
	}
	for _, f := range out.Files {
		size, _ := strconv.ParseInt(f.Size, 10, 64)
		page.Items = append(page.Items, personalItem{ID: f.ID, Name: f.Name, Folder: f.MimeType == googleFolderMime, Size: size, Modified: f.ModifiedTime, MimeType: f.MimeType})
	}
	page.NextCursor = out.Next
	return page, nil
}

func onedriveList(ctx context.Context, rt connector.Runtime, folder, cursor string) (personalPage, error) {
	page := personalPage{Items: []personalItem{}}
	target := graphDriveAPI + "/root/children"
	if folder != "" {
		target = graphDriveAPI + "/items/" + folder + "/children"
	}
	target += "?$top=100&$select=id,name,size,folder,file,lastModifiedDateTime"
	if cursor != "" {
		// El cursor de Graph es la URL entera de la página siguiente: solo la suya.
		if !strings.HasPrefix(cursor, graphDriveAPI+"/") {
			return page, httpx.ErrBadRequest
		}
		target = cursor
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	var out struct {
		Value []struct {
			ID, Name, LastModifiedDateTime string
			Size                           int64
			Folder                         *struct{} `json:"folder"`
			File                           *struct {
				MimeType string `json:"mimeType"` //nolint:tagliatelle // contrato de Graph
			} `json:"file"`
		} `json:"value"`
		Next string `json:"@odata.nextLink"` //nolint:tagliatelle // contrato de Graph
	}
	if err := personalJSON(rt, req, &out); err != nil {
		return page, err
	}
	for _, f := range out.Value {
		it := personalItem{ID: f.ID, Name: f.Name, Folder: f.Folder != nil, Size: f.Size, Modified: f.LastModifiedDateTime}
		if f.File != nil {
			it.MimeType = f.File.MimeType
		}
		page.Items = append(page.Items, it)
	}
	page.NextCursor = out.Next
	return page, nil
}

func dropboxList(ctx context.Context, rt connector.Runtime, folder, cursor string) (personalPage, error) {
	page := personalPage{Items: []personalItem{}}
	route, args := "list_folder", map[string]any{"path": folder, "limit": 100}
	if cursor != "" {
		route, args = "list_folder/continue", map[string]any{"cursor": cursor}
	}
	body, _ := json.Marshal(args)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, dropboxAPI+route, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Entries []struct {
			Tag            string `json:".tag"` //nolint:tagliatelle // contrato de Dropbox
			Name           string `json:"name"`
			PathLower      string `json:"path_lower"` //nolint:tagliatelle // contrato de Dropbox
			Size           int64  `json:"size"`
			ServerModified string `json:"server_modified"` //nolint:tagliatelle // contrato de Dropbox
		} `json:"entries"`
		Cursor  string `json:"cursor"`
		HasMore bool   `json:"has_more"` //nolint:tagliatelle // contrato de Dropbox
	}
	if err := personalJSON(rt, req, &out); err != nil {
		return page, err
	}
	for _, e := range out.Entries {
		if e.Tag == "deleted" {
			continue
		}
		page.Items = append(page.Items, personalItem{ID: e.PathLower, Name: e.Name, Folder: e.Tag == "folder", Size: e.Size, Modified: e.ServerModified})
	}
	if out.HasMore {
		page.NextCursor = out.Cursor
	}
	return page, nil
}

func personalDownloadRequest(ctx context.Context, key, item string) (*http.Request, error) {
	if !personalValidID(key, item) {
		return nil, httpx.ErrBadRequest
	}
	switch key {
	case tmplGoogleDrive:
		return http.NewRequestWithContext(ctx, http.MethodGet, googleFilesAPI+"/"+item+"?alt=media", http.NoBody)
	case tmplOneDrive:
		return http.NewRequestWithContext(ctx, http.MethodGet, graphDriveAPI+"/items/"+item+"/content", http.NoBody)
	case tmplDropbox:
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, dropboxContent+"download", http.NoBody)
		if err == nil {
			arg, _ := json.Marshal(map[string]string{"path": item})
			req.Header.Set("Dropbox-API-Arg", dropboxASCII(string(arg)))
		}
		return req, err
	}
	return nil, httpx.ErrNotFound
}

// dropboxASCII escapa lo que no es ASCII: Dropbox-API-Arg es una cabecera HTTP.
func dropboxASCII(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c < 0x80 {
			b.WriteRune(c)
			continue
		}
		for _, u := range utf16.Encode([]rune{c}) {
			fmt.Fprintf(&b, `\u%04x`, u)
		}
	}
	return b.String()
}

func personalUpload(ctx context.Context, key string, rt connector.Runtime, folder, name string, body []byte) (personalItem, error) {
	if folder != "" && !personalValidID(key, folder) {
		return personalItem{}, httpx.ErrBadRequest
	}
	switch key {
	case tmplGoogleDrive:
		if folder == "" {
			folder = "root"
		}
		meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}})
		const limite = "sytue-nube-personal"
		var b bytes.Buffer
		b.WriteString("--" + limite + "\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n")
		b.Write(meta)
		b.WriteString("\r\n--" + limite + "\r\nContent-Type: application/octet-stream\r\n\r\n")
		b.Write(body)
		b.WriteString("\r\n--" + limite + "--\r\n")
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleUploadAPI+"?uploadType=multipart&fields=id,name,mimeType,size,modifiedTime", &b)
		req.Header.Set("Content-Type", "multipart/related; boundary="+limite)
		var out struct{ ID, Name, MimeType, ModifiedTime, Size string }
		if err := personalJSON(rt, req, &out); err != nil {
			return personalItem{}, err
		}
		size, _ := strconv.ParseInt(out.Size, 10, 64)
		return personalItem{ID: out.ID, Name: out.Name, Size: size, Modified: out.ModifiedTime, MimeType: out.MimeType}, nil
	case tmplOneDrive:
		parent := graphDriveAPI + "/root"
		if folder != "" {
			parent = graphDriveAPI + "/items/" + folder
		}
		// ponytail: subida simple de Graph, hasta 250 MB (el tope de aquí es menor).
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, parent+":/"+url.PathEscape(name)+":/content?@microsoft.graph.conflictBehavior=rename", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		var out struct {
			ID, Name, LastModifiedDateTime string
			Size                           int64
		}
		if err := personalJSON(rt, req, &out); err != nil {
			return personalItem{}, err
		}
		return personalItem{ID: out.ID, Name: out.Name, Size: out.Size, Modified: out.LastModifiedDateTime}, nil
	case tmplDropbox:
		arg, _ := json.Marshal(map[string]any{"path": strings.TrimSuffix(folder, "/") + "/" + name, "mode": "add", "autorename": true})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, dropboxContent+"upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Dropbox-API-Arg", dropboxASCII(string(arg)))
		var out struct {
			Name           string `json:"name"`
			PathLower      string `json:"path_lower"`      //nolint:tagliatelle // contrato de Dropbox
			Size           int64  `json:"size"`            //nolint:tagliatelle // contrato de Dropbox
			ServerModified string `json:"server_modified"` //nolint:tagliatelle // contrato de Dropbox
		}
		if err := personalJSON(rt, req, &out); err != nil {
			return personalItem{}, err
		}
		return personalItem{ID: out.PathLower, Name: out.Name, Size: out.Size, Modified: out.ServerModified}, nil
	}
	return personalItem{}, httpx.ErrNotFound
}
