// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
)

var regexFicheroValido = regexp.MustCompile(`^[0-9a-f]{32}(-\d+)?\.(jpg|png|woff2|pdf)$`)

func esAdmin(r *http.Request) bool {
	if claims, ok := authn.FromContext(r.Context()); ok {
		return claims.HasAnyRole("owner", "admin")
	}
	return false
}

type crearSnapshotPeticion struct {
	Purpose string `json:"purpose"` // "publish" | "preview" | "export"
}

func (h *Handlers) crearSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var in crearSnapshotPeticion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	in.Purpose = strings.ToLower(strings.TrimSpace(in.Purpose))
	if in.Purpose != "publish" && in.Purpose != "preview" && in.Purpose != "export" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_purpose").WithMessage("purpose debe ser publish, preview o export"))
		return
	}

	sitio, err := h.Store.SitioCompleto(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	if in.Purpose == "publish" {
		if err := validarRequisitosPublicacionLive(r, sitio); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}

	versionID, err := h.Store.CrearVersionConTipo(r.Context(), id, "", in.Purpose, sitio.Draft, sitio.DraftRev, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	renderData := h.construirDatosRender(r.Context(), sitio)

	snapshot := domain.Snapshot{
		VersionID: versionID,
		IRVersion: ir.VersionIR,
		IR:        sitio.Draft,
		Rev:       sitio.DraftRev,
		Data:      renderData,
	}

	httpx.JSON(w, r, http.StatusCreated, snapshot)
}

func validarRequisitosPublicacionLive(r *http.Request, sitio domain.SitioCompleto) error {
	if !esAdmin(r) {
		return httpx.ErrForbidden.WithMessage("Se requiere rol admin para publicar")
	}
	if !domain.EsLegalCompleto(sitio.Legal) {
		return httpx.ErrUnprocessable.WithCode("legal_data_missing").WithMessage("Faltan datos legales requeridos para publicar")
	}
	if domain.TieneContenidoDemo(sitio.Draft, sitio.Template) {
		return httpx.ErrUnprocessable.WithCode("demo_content").WithMessage("El sitio contiene textos de relleno de plantilla")
	}

	sitioIR, err := ir.Decodificar(sitio.Draft)
	if err != nil {
		return httpx.ErrUnprocessable.WithCode("ir_invalid").WithMessage("JSON de IR corrupto")
	}
	probs := ir.Validar(sitioIR, nil, ir.ModoPublicar)
	if hayErrores(probs) {
		return httpx.ErrUnprocessable.WithCode("ir_invalid").WithParams(map[string]any{"problems": probs})
	}
	return nil
}

func (h *Handlers) obtenerDatosRender(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	sitio, err := h.Store.SitioCompleto(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	renderData := h.construirDatosRender(r.Context(), sitio)
	httpx.JSON(w, r, http.StatusOK, renderData)
}

func (h *Handlers) construirDatosRender(ctx context.Context, sitio domain.SitioCompleto) domain.DatosRender {
	sitesHost := fmt.Sprintf("%s.%s", sitio.Slug, h.Cfg.SitesDomain)
	canonicalHost := sitesHost
	if sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
		canonicalHost = *sitio.CanonicalHost
	}

	hosts := []string{sitesHost}
	if canonicalHost != sitesHost {
		hosts = append(hosts, canonicalHost)
	}

	images := make(map[string]domain.ImagenRender)
	files := make(map[string]domain.FicheroRender)

	if recursos, err := h.Store.ListarRecursos(ctx, sitio.ID); err == nil {
		poblarRecursosSitio(recursos, images, files)
	}
	if h.Catalogo != nil {
		poblarRecursosCatalogo(h.Catalogo, images)
	}

	fonts := make(map[string]domain.FuenteRender)
	if h.Catalogo != nil {
		poblarFuentesCatalogo(h.Catalogo, fonts)
	}

	forms := make(map[string]domain.FormularioRender)
	var sParsed ir.Sitio
	if err := json.Unmarshal(sitio.Draft, &sParsed); err == nil {
		for _, p := range sParsed.Pages {
			for _, sec := range p.Sections {
				recorrerNodosForm(sec.Children, forms)
			}
		}
	}

	return domain.DatosRender{
		Site: domain.SitioRender{
			ID:            sitio.ID.String(),
			Slug:          sitio.Slug,
			SitesHost:     sitesHost,
			CanonicalHost: canonicalHost,
			Hosts:         hosts,
			Analytics:     sitio.AnalyticsEnabled && h.Cfg.AnalyticsURL != "",
		},
		Images: images,
		Files:  files,
		Fonts:  fonts,
		Legal:  sitio.Legal,
		Store:  nil,
		Forms:  forms,
		Origins: domain.OrigenesRender{
			Cast:  h.Cfg.CastOrigin,
			Pages: h.Cfg.PublicURL,
		},
		Now: time.Now().UTC().Format(time.RFC3339),
	}
}

func poblarRecursosSitio(recursos []domain.Recurso, images map[string]domain.ImagenRender, files map[string]domain.FicheroRender) {
	for _, rec := range recursos {
		key := "a:" + rec.ID.String()
		if rec.Ext == "pdf" {
			files[key] = domain.FicheroRender{
				File:  rec.File,
				Name:  rec.Name,
				Bytes: rec.Bytes,
			}
		} else {
			images[key] = domain.ImagenRender{
				File:     rec.File + "." + rec.Ext,
				Ext:      rec.Ext,
				Width:    rec.Width,
				Height:   rec.Height,
				Variants: rec.Variants,
			}
		}
	}
}

func poblarRecursosCatalogo(cat *catalogo.Catalogo, images map[string]domain.ImagenRender) {
	for _, foto := range cat.Fotos {
		images["l:"+foto.ID] = domain.ImagenRender{
			File:     foto.Fichero,
			Ext:      "jpg",
			Width:    foto.Ancho,
			Height:   foto.Alto,
			Variants: []int{},
		}
	}
}

func poblarFuentesCatalogo(cat *catalogo.Catalogo, fonts map[string]domain.FuenteRender) {
	for _, fam := range cat.Familias {
		fonts[fam.ID] = domain.FuenteRender{
			Name:     fam.ID,
			Fallback: "sans-serif",
			Scripts:  fam.Scripts,
			Faces:    fam.Faces,
		}
	}
}

func recorrerNodosForm(nodos []ir.Nodo, forms map[string]domain.FormularioRender) {
	for _, n := range nodos {
		if n.Type == "form" && n.Form != "" {
			forms[n.Form] = domain.FormularioRender{Enabled: true}
		}
		if len(n.Children) > 0 {
			recorrerNodosForm(n.Children, forms)
		}
	}
}

func (h *Handlers) crearVistaPrevia(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	vivas, err := h.Store.ContarVistasVivas(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if vivas >= 5 {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "too_many_previews", "Tope de 5 vistas previas activas por sitio alcanzado."))
		return
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		httpx.Fail(w, r, fmt.Errorf("generar token: %w", err))
		return
	}
	token := hex.EncodeToString(tokenBytes)

	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"token":      token,
		"base_path":  "/_vista/" + token,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

type ManifiestoPublicacion struct {
	VersionID       uuid.UUID           `json:"version_id"`
	Kind            string              `json:"kind"`
	PreviewToken    string              `json:"preview_token,omitempty"`
	RendererVersion string              `json:"renderer_version"`
	IslasHash       string              `json:"islas_hash,omitempty"`
	Files           []FicheroManifiesto `json:"files"`
	Resources       []string            `json:"resources,omitempty"`
}

type FicheroManifiesto struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func (h *Handlers) subirPublicacion(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if err := validarParametrosPublicacion(kind, token); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	sitio, err := h.Store.SitioCompleto(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	if kind == "live" {
		if err := validarRequisitosPublicacionLive(r, sitio); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}

	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_multipart").Wrap(err))
		return
	}

	manifest, ficheros, bytesTotales, err := procesarMultipartPublicacion(mr)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if _, err := h.Store.Version(r.Context(), id, manifest.VersionID); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	if err := h.validarPublicacionSalida(r.Context(), sitio, kind, token, ficheros); err != nil {
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusServiceUnavailable {
			httpx.Fail(w, r, err)
		} else {
			httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("publication_invalid").Wrap(err))
		}
		return
	}

	if kind == "live" {
		number, err := h.publicarLive(r.Context(), id, manifest, ficheros, bytesTotales, sub(r))
		if err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"number": number})
		return
	}

	previewURL, err := h.publicarPreview(r.Context(), id, sitio, manifest, ficheros, bytesTotales, token, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"url": previewURL})
}

func validarParametrosPublicacion(kind, token string) error {
	if kind != "live" && kind != "preview" {
		return httpx.ErrBadRequest.WithCode("invalid_kind").WithMessage("kind debe ser live o preview")
	}
	if kind == "preview" && token == "" {
		return httpx.ErrBadRequest.WithCode("token_required").WithMessage("se requiere token para preview")
	}
	return nil
}

func procesarMultipartPublicacion(mr *multipart.Reader) (*ManifiestoPublicacion, []publicar.FicheroSalida, int64, error) {
	partMan, err := mr.NextPart()
	if err != nil {
		return nil, nil, 0, httpx.ErrBadRequest.WithCode("missing_manifest").Wrap(err)
	}
	if partMan.FormName() != "manifest" {
		_ = partMan.Close()
		return nil, nil, 0, httpx.ErrBadRequest.WithCode("first_part_must_be_manifest").WithMessage("La primera parte debe ser manifest")
	}

	var manifest ManifiestoPublicacion
	dec := json.NewDecoder(partMan)
	if err := dec.Decode(&manifest); err != nil {
		_ = partMan.Close()
		return nil, nil, 0, httpx.ErrBadRequest.WithCode("invalid_manifest_json").Wrap(err)
	}
	_ = partMan.Close()

	if len(manifest.Files) > 5000 {
		return nil, nil, 0, httpx.NewError(http.StatusRequestEntityTooLarge, "publication_too_large", "Superado el tope de 5000 ficheros")
	}

	var bytesEsperados int64
	mapaEsperados := make(map[string]FicheroManifiesto, len(manifest.Files))
	for _, mf := range manifest.Files {
		bytesEsperados += mf.Bytes
		mapaEsperados[mf.Path] = mf
	}
	if bytesEsperados > 64<<20 {
		return nil, nil, 0, httpx.NewError(http.StatusRequestEntityTooLarge, "publication_too_large", "Superado el tope total de 64 MiB")
	}

	ficheros, err := leerFicherosMultipart(mr, mapaEsperados)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(ficheros) != len(manifest.Files) {
		return nil, nil, 0, httpx.ErrUnprocessable.WithCode("publication_invalid").WithMessage("Faltan ficheros declarados en manifiesto")
	}

	return &manifest, ficheros, bytesEsperados, nil
}

func leerFicherosMultipart(mr *multipart.Reader, mapaEsperados map[string]FicheroManifiesto) ([]publicar.FicheroSalida, error) {
	ficheros := make([]publicar.FicheroSalida, 0, len(mapaEsperados))
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, httpx.ErrBadRequest.WithCode("read_file_part_failed").Wrap(err)
		}

		// Part.FileName elimina directorios: perdería en/index.html y assets/….
		// El manifiesto y validarPublicacionSalida validan la ruta íntegra.
		_, parametros, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			_ = part.Close()
			return nil, httpx.ErrBadRequest.WithCode("invalid_file_part").Wrap(err)
		}
		nombreRuta := parametros["filename"]
		if nombreRuta == "" {
			nombreRuta = part.FormName()
		}

		mf, ok := mapaEsperados[nombreRuta]
		if !ok {
			_ = part.Close()
			return nil, httpx.ErrUnprocessable.WithCode("publication_invalid").WithMessage(fmt.Sprintf("Fichero no declarado en manifiesto: %q", nombreRuta))
		}

		data, err := io.ReadAll(io.LimitReader(part, mf.Bytes+1024))
		_ = part.Close()
		if err != nil {
			return nil, httpx.ErrBadRequest.WithCode("read_part_failed").Wrap(err)
		}

		if int64(len(data)) != mf.Bytes {
			return nil, httpx.ErrUnprocessable.WithCode("publication_invalid").WithMessage(fmt.Sprintf("Tamaño no coincide para %s", nombreRuta))
		}

		hSha := sha256.Sum256(data)
		if hex.EncodeToString(hSha[:]) != mf.SHA256 {
			return nil, httpx.ErrUnprocessable.WithCode("publication_invalid").WithMessage(fmt.Sprintf("SHA256 no coincide para %s", nombreRuta))
		}

		ficheros = append(ficheros, publicar.FicheroSalida{
			Ruta:      nombreRuta,
			Contenido: data,
		})
	}
	return ficheros, nil
}

func (h *Handlers) validarPublicacionSalida(ctx context.Context, sitio domain.SitioCompleto, kind, token string, ficheros []publicar.FicheroSalida) error {
	hostsPermitidos := []string{fmt.Sprintf("%s.%s", sitio.Slug, h.Cfg.SitesDomain)}
	if sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
		hostsPermitidos = append(hostsPermitidos, *sitio.CanonicalHost)
	}

	optsVal := publicar.OpcionesValidacion{
		Kind:            kind,
		CastOrigin:      h.Cfg.CastOrigin,
		HostsPermitidos: hostsPermitidos,
	}
	if kind == "preview" {
		optsVal.PreviewBase = "/_vista/" + token
	}

	if err := validarNombresCSS(ficheros); err != nil {
		return err
	}

	optsVal.RecursosValidos = make(map[string]bool)
	candidatos := make(map[string]string)
	if h.Catalogo != nil {
		for nombre, rec := range h.Catalogo.Recursos() {
			candidatos[nombre] = rec.Clave
		}
	}
	if h.Store != nil && h.Objetos != nil {
		recursos, err := h.Store.ListarRecursos(ctx, sitio.ID)
		if err != nil {
			return httpx.ErrUnavailable.WithCode("resource_store_unavailable").Wrap(err)
		}
		for _, rec := range recursos {
			for _, nombre := range rec.Ficheros() {
				candidatos[nombre] = fmt.Sprintf("shared/sitios/%s/recursos/%s", sitio.ID, nombre)
			}
		}
	}
	if h.Objetos != nil {
		for nombre, clave := range candidatos {
			if !recursoReferido(ficheros, nombre) {
				continue
			}
			existe, err := h.Objetos.Existe(ctx, tenancy.Tenant{Slug: "shared"}, clave)
			if err != nil {
				return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
			}
			if existe {
				optsVal.RecursosValidos["/_astro/"+nombre] = true
			}
		}
	}
	return h.Publicar.Validar(ficheros, optsVal)
}

func validarNombresCSS(ficheros []publicar.FicheroSalida) error {
	for _, fichero := range ficheros {
		if strings.HasPrefix(fichero.Ruta, "_astro/") && strings.HasSuffix(fichero.Ruta, ".css") {
			hash := sha256.Sum256(fichero.Contenido)
			if fichero.Ruta != fmt.Sprintf("_astro/%x.css", hash[:8]) {
				return fmt.Errorf("CSS no coincide con su nombre por contenido: %s", fichero.Ruta)
			}
		}
	}

	return nil
}

func recursoReferido(ficheros []publicar.FicheroSalida, nombre string) bool {
	for _, fichero := range ficheros {
		if bytes.Contains(fichero.Contenido, []byte("/_astro/"+nombre)) {
			return true
		}
	}
	return false
}

func (h *Handlers) publicarLive(ctx context.Context, id uuid.UUID, manifest *ManifiestoPublicacion, ficheros []publicar.FicheroSalida, bytesTotales int64, sub string) (int, error) {
	prevPub, errPrev := h.Store.PublicacionVigente(ctx, id)
	var redirecciones []domain.Redirect
	if errPrev == nil {
		redirecciones = h.calcularRedirecciones(ctx, id, prevPub.VersionID, manifest.VersionID)
	}

	dataHashRaw := []byte("{}")
	number, err := h.Store.CrearPublicacionLiveConArchivos(
		ctx, id, manifest.VersionID,
		len(ficheros), bytesTotales, manifest.RendererVersion,
		dataHashRaw, sub, redirecciones,
		func(number int) error {
			return h.subirArchivosLiveS3(ctx, id, number, ficheros, manifest, redirecciones)
		},
	)
	if err != nil {
		return 0, err
	}

	if h.Mapa != nil {
		if err := h.Mapa.Regenerar(ctx); err != nil {
			return 0, httpx.ErrUnavailable.WithCode("host_map_unavailable").Wrap(err)
		}
	}
	// La publicación no se revierte si Analytics está momentáneamente caído: el
	// siguiente publish vuelve a asegurar el host. Sin embargo, solo habilitamos
	// el script cuando el alta ya devolvió un identificador válido.
	if h.Analitica != nil && h.Analitica.Disponible() {
		sitio, getErr := h.Store.SitioCompleto(ctx, id)
		if getErr == nil && sitio.AnalyticsEnabled {
			host := fmt.Sprintf("%s.%s", sitio.Slug, h.Cfg.SitesDomain)
			if sitio.CanonicalHost != nil && *sitio.CanonicalHost != "" {
				host = *sitio.CanonicalHost
			}
			if analyticsID, ensureErr := h.Analitica.Asegurar(ctx, sitio.Tenant, host); ensureErr == nil {
				_ = h.Store.FijarAnaliticaSitio(ctx, sitio.Tenant, id, analyticsID)
			}
		}
	}
	return number, nil
}

func (h *Handlers) subirArchivosLiveS3(ctx context.Context, id uuid.UUID, number int, ficheros []publicar.FicheroSalida, manifest *ManifiestoPublicacion, redirecciones []domain.Redirect) error {
	if h.Objetos == nil {
		return httpx.ErrUnavailable.WithCode("object_store_unavailable")
	}
	t, _ := tenancy.From(ctx)
	for _, f := range ficheros {
		ctype := tipoPorRuta(h.Publicar.Permitido, f.Ruta)
		if strings.HasPrefix(f.Ruta, "_astro/") {
			claveCSS := fmt.Sprintf("shared/sitios/%s/recursos/%s", id.String(), filepath.Base(f.Ruta))
			if err := h.Objetos.Put(ctx, t, claveCSS, bytes.NewReader(f.Contenido), int64(len(f.Contenido)), objectstore.PutOptions{ContentType: ctype}); err != nil {
				return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
			}
		} else {
			clave := fmt.Sprintf("shared/sitios/%s/v/%d/%s", id.String(), number, f.Ruta)
			if err := h.Objetos.Put(ctx, t, clave, bytes.NewReader(f.Contenido), int64(len(f.Contenido)), objectstore.PutOptions{ContentType: ctype}); err != nil {
				return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
			}
		}
	}

	mBytes, _ := json.Marshal(manifest)
	claveMan := fmt.Sprintf("shared/sitios/%s/v/%d/_manifiesto.json", id.String(), number)
	if err := h.Objetos.Put(ctx, t, claveMan, bytes.NewReader(mBytes), int64(len(mBytes)), objectstore.PutOptions{ContentType: "application/json"}); err != nil {
		return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
	}

	rBytes, _ := json.Marshal(map[string]any{"redirects": redirecciones})
	claveRed := fmt.Sprintf("shared/sitios/%s/v/%d/_redirecciones.json", id.String(), number)
	if err := h.Objetos.Put(ctx, t, claveRed, bytes.NewReader(rBytes), int64(len(rBytes)), objectstore.PutOptions{ContentType: "application/json"}); err != nil {
		return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
	}
	return nil
}

func (h *Handlers) publicarPreview(ctx context.Context, id uuid.UUID, sitio domain.SitioCompleto, manifest *ManifiestoPublicacion, ficheros []publicar.FicheroSalida, bytesTotales int64, token, sub string) (string, error) {
	hToken := sha256.Sum256([]byte(token))
	previewKey := hex.EncodeToString(hToken[:16])
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	if err := h.subirArchivosPreviewS3(ctx, id, previewKey, ficheros, manifest); err != nil {
		return "", err
	}

	err := h.Store.CrearPublicacionPreview(
		ctx, id, manifest.VersionID,
		previewKey, expiresAt, len(ficheros), bytesTotales,
		manifest.RendererVersion, json.RawMessage("{}"), sub,
	)
	if err != nil {
		return "", err
	}

	if h.Mapa != nil {
		if err := h.Mapa.Regenerar(ctx); err != nil {
			return "", httpx.ErrUnavailable.WithCode("host_map_unavailable").Wrap(err)
		}
	}

	return fmt.Sprintf("https://%s.%s/_vista/%s/", sitio.Slug, h.Cfg.SitesDomain, token), nil
}

func (h *Handlers) subirArchivosPreviewS3(ctx context.Context, id uuid.UUID, previewKey string, ficheros []publicar.FicheroSalida, manifest *ManifiestoPublicacion) error {
	if h.Objetos == nil {
		return httpx.ErrUnavailable.WithCode("object_store_unavailable")
	}
	t, _ := tenancy.From(ctx)
	for _, f := range ficheros {
		ctype := tipoPorRuta(h.Publicar.Permitido, f.Ruta)
		if strings.HasPrefix(f.Ruta, "_astro/") {
			claveCSS := fmt.Sprintf("shared/sitios/%s/recursos/%s", id.String(), filepath.Base(f.Ruta))
			if err := h.Objetos.Put(ctx, t, claveCSS, bytes.NewReader(f.Contenido), int64(len(f.Contenido)), objectstore.PutOptions{ContentType: ctype}); err != nil {
				return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
			}
		} else {
			clave := fmt.Sprintf("shared/sitios/%s/vistas/%s/%s", id.String(), previewKey, f.Ruta)
			if err := h.Objetos.Put(ctx, t, clave, bytes.NewReader(f.Contenido), int64(len(f.Contenido)), objectstore.PutOptions{ContentType: ctype}); err != nil {
				return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
			}
		}
	}

	mBytes, _ := json.Marshal(manifest)
	claveMan := fmt.Sprintf("shared/sitios/%s/vistas/%s/_manifiesto.json", id.String(), previewKey)
	if err := h.Objetos.Put(ctx, t, claveMan, bytes.NewReader(mBytes), int64(len(mBytes)), objectstore.PutOptions{ContentType: "application/json"}); err != nil {
		return httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err)
	}
	return nil
}

func (h *Handlers) calcularRedirecciones(ctx context.Context, siteID, vAnteriorID, vNuevaID uuid.UUID) []domain.Redirect {
	vAnt, err := h.Store.Version(ctx, siteID, vAnteriorID)
	if err != nil {
		return nil
	}
	vNue, err := h.Store.Version(ctx, siteID, vNuevaID)
	if err != nil {
		return nil
	}

	var irAnt, irNue ir.Sitio
	if err := json.Unmarshal(vAnt.IR, &irAnt); err != nil {
		return nil
	}
	if err := json.Unmarshal(vNue.IR, &irNue); err != nil {
		return nil
	}

	slugsAnt := make(map[string]map[string]string)
	for _, p := range irAnt.Pages {
		slugsAnt[p.ID] = p.Slug
	}

	var redirects []domain.Redirect
	for _, p := range irNue.Pages {
		antSlugMap, ok := slugsAnt[p.ID]
		if !ok {
			continue
		}
		for lang, newSlug := range p.Slug {
			oldSlug, tenia := antSlugMap[lang]
			if tenia && oldSlug != "" && newSlug != "" && oldSlug != newSlug {
				redirects = append(redirects, domain.Redirect{
					FromPath: fmt.Sprintf("/%s/%s", lang, oldSlug),
					ToPath:   fmt.Sprintf("/%s/%s/", lang, newSlug),
				})
			}
		}
	}
	return redirects
}

func tipoPorRuta(p *publicar.Permitido, ruta string) string {
	ext := strings.TrimPrefix(filepath.Ext(ruta), ".")
	if t, ok := p.Tipos[ext]; ok {
		return t
	}
	return "application/octet-stream"
}

func (h *Handlers) listarPublicaciones(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	pubs, err := h.Store.ListarPublicaciones(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, pubs)
}

func (h *Handlers) rollbackPublicacion(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Se requiere rol admin para hacer rollback"))
		return
	}

	numStr := chi.URLParam(r, "number")
	number, err := strconv.Atoi(numStr)
	if err != nil || number <= 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_publication_number"))
		return
	}

	if err := h.Store.Rollback(r.Context(), id, number); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	if h.Mapa != nil {
		_ = h.Mapa.Regenerar(r.Context())
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"number": number})
}

func (h *Handlers) despublicarSitio(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Se requiere rol admin para retirar publicación"))
		return
	}

	if err := h.Store.Despublicar(r.Context(), id); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	if h.Mapa != nil {
		if err := h.Mapa.Regenerar(r.Context()); err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("host_map_unavailable").Wrap(err))
			return
		}
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"published_number": nil})
}

func (h *Handlers) exportarAstro(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	sitio, err := h.Store.SitioCompleto(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_multipart").Wrap(err))
		return
	}

	var archivos []publicar.ArchivoZip
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("read_part_failed").Wrap(err))
			return
		}

		nombre := part.FileName()
		if nombre == "" {
			nombre = part.FormName()
		}

		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("read_file_failed").Wrap(err))
			return
		}

		archivos = append(archivos, publicar.ArchivoZip{
			Ruta:       nombre,
			Contenido:  data,
			Modificado: time.Now().UTC(),
		})
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sitio.Slug+"-astro.zip"))

	if err := publicar.EscribirZip(w, archivos, nil); err != nil {
		return
	}
}

func (h *Handlers) sitioFichero(w http.ResponseWriter, r *http.Request) {
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	nombre := chi.URLParam(r, "nombre")
	if !regexFicheroValido.MatchString(nombre) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_file_name").WithMessage("El nombre de archivo no cumple con el patrón permitido"))
		return
	}

	if _, err := h.Store.SitioCompleto(r.Context(), siteID); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	ext := strings.ToLower(filepath.Ext(nombre))

	if h.Objetos != nil {
		t, _ := tenancy.From(r.Context())
		claveSitio := fmt.Sprintf("shared/sitios/%s/recursos/%s", siteID.String(), nombre)
		rc, err := h.Objetos.Get(r.Context(), t, claveSitio)
		if err == nil {
			defer rc.Close()
			determinarCabecerasFichero(w, ext)
			_, _ = io.Copy(w, rc)
			return
		}
		if !errors.Is(err, objectstore.ErrNotFound) {
			w.Header().Set("Cache-Control", "no-store")
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err))
			return
		}

		claveCat, conocido := h.Catalogo.ClaveRecurso(nombre)
		if !conocido {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("file_not_found"))
			return
		}
		rcCat, err := h.Objetos.Get(r.Context(), t, claveCat)
		if err == nil {
			defer rcCat.Close()
			determinarCabecerasFichero(w, ext)
			_, _ = io.Copy(w, rcCat)
			return
		}
		if !errors.Is(err, objectstore.ErrNotFound) {
			w.Header().Set("Cache-Control", "no-store")
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err))
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.Fail(w, r, httpx.ErrNotFound.WithCode("file_not_found"))
}

func determinarCabecerasFichero(w http.ResponseWriter, ext string) {
	var ctype string
	switch ext {
	case ".woff2":
		ctype = "font/woff2"
	case ".jpg", ".jpeg":
		ctype = "image/jpeg"
	case ".png":
		ctype = "image/png"
	case ".pdf":
		ctype = "application/pdf"
	default:
		ctype = "application/octet-stream"
	}

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if ext == ".pdf" {
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
}

func (h *Handlers) exportarHub(w http.ResponseWriter, r *http.Request) {
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}

	sujeto := sub(r)
	sitios, versiones, err := h.Store.ExportarDatosHub(r.Context(), sujeto)
	if err != nil {
		_ = p.Fallo(err)
		return
	}

	if err := portabilidad.Tabla(p, "sites", "site", portabilidad.Todas(sitios)); err != nil {
		_ = p.Fallo(err)
		return
	}
	if err := portabilidad.Tabla(p, "site_versions", "site_version", portabilidad.Todas(versiones)); err != nil {
		_ = p.Fallo(err)
		return
	}

	_ = p.Cerrar()
}
