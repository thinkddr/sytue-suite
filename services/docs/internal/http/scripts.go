// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// # Scripts de usuario (automatizacion-1, pieza A)
//
// El servidor NO ejecuta código: decide quién puede ejecutar qué versión sobre qué
// documento, aplica cuotas y deja el registro. Las reglas que no se negocian:
//   - una versión es inmutable; editar crea otra, en borrador;
//   - un script de la organización solo lo ejecutan otras personas si la versión está
//     APROBADA por un administrador, y la aprobación lleva el hash del código revisado;
//   - ejecutar exige en el documento el rol que pide el manifiesto (editor si escribe) y
//     el script solo corre sobre su tipo de documento y, si lo tiene, su documento;
//   - los privados de otra persona no existen para nadie (404), tampoco para un administrador;
//   - la escritura sigue pasando por /ops, que reautoriza ACL y protección en cada
//     operación: esto no es una vía para saltárselas.

// scriptsStore es lo que esto necesita del store.
type scriptsStore interface {
	CrearScript(ctx context.Context, sc domain.Script, codigo string, manifiesto []byte) (domain.Script, domain.VersionScript, error)
	ListarScripts(ctx context.Context, sub string, admin bool, limite int) ([]domain.Script, error)
	ObtenerScript(ctx context.Context, id string) (domain.Script, error)
	VersionesScript(ctx context.Context, id string, limite int) ([]domain.VersionScript, error)
	ObtenerVersionScript(ctx context.Context, id string, version int) (domain.VersionScript, error)
	NuevaVersionScript(ctx context.Context, id, codigo string, manifiesto []byte, sub string) (domain.VersionScript, error)
	CambiarAmbitoScript(ctx context.Context, id, ambito string) error
	BorrarScript(ctx context.Context, id string) error
	ProponerVersionScript(ctx context.Context, id string, version int) error
	RevisarVersionScript(ctx context.Context, id string, version int, hash string, aprobar bool, sub string) error
	ReservarDenegada(ctx context.Context, e domain.EjecucionScript, codigo string) (int64, error)
	ReservarEjecucion(ctx context.Context, e domain.EjecucionScript, maxMs int) (int64, error)
	CerrarEjecucion(ctx context.Context, id int64, sub, resultado, codigoError string, ms, ops int) error
	EjecucionesScript(ctx context.Context, docID string, antesDe int64, limite int) ([]domain.EjecucionScript, error)
	UsoCuotaScripts(ctx context.Context, sub string) (domain.UsoCuota, error)
}

const maxBytesScript = domain.MaxCodigoScript + 32*1024

var reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// failScript traduce los errores propios de scripts y delega el resto en fail.
func failScript(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrScriptInvalido):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(strings.TrimPrefix(err.Error(), "docs: ")))
	case errors.Is(err, domain.ErrCuotaScripts):
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "quota_exceeded", strings.TrimPrefix(err.Error(), "docs: ")))
	case errors.Is(err, domain.ErrScriptSinAprobar):
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "script_not_approved", "This script version has not been approved by an administrator."))
	case errors.Is(err, domain.ErrConflictoEstadoScript), errors.Is(err, domain.ErrHashNoCoincide):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(strings.TrimPrefix(err.Error(), "docs: ")))
	default:
		fail(w, r, err)
	}
}

// sesionScripts exige sesión de la organización: un invitado externo (ADR 0052) no usa scripts.
func sesionScripts(w http.ResponseWriter, r *http.Request) (domain.Subject, bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return subj, false
	}
	if esExterno(r.Context()) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return subj, false
	}
	return subj, true
}

// scriptVisible carga el script y lo da por inexistente si quien llama no puede verlo:
// el privado ajeno, siempre; el de la organización sin versión aprobada, salvo a un
// administrador (que lo revisa) y a su autor.
func (h *Handler) scriptVisible(w http.ResponseWriter, r *http.Request, sub string) (sc domain.Script, ok bool) {
	id := chi.URLParam(r, "sid")
	if !reUUID.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return sc, false
	}
	sc, err := h.scripts.ObtenerScript(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return sc, false
	}
	if sc.AuthorSub == sub {
		return sc, true
	}
	if sc.Scope != domain.AmbitoOrg || (sc.ApprovedVersion == 0 && !esAdminOrg(r)) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return sc, false
	}
	return sc, true
}

func numeroVersion(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, "v"))
	if err != nil || n < 1 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return 0, false
	}
	return n, true
}

// cuerpoVersion lee {code, manifest}, valida ambos y devuelve el manifiesto canónico.
type cuerpoVersion struct {
	Code     string          `json:"code"`
	Manifest json.RawMessage `json:"manifest"`
}

func (c cuerpoVersion) validar() (canon []byte, err error) {
	if err := domain.ValidarCodigoScript(c.Code); err != nil {
		return nil, err
	}
	m, err := domain.ParseManifiesto(c.Manifest)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func decodificar(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytesScript)
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		httpx.Fail(w, r, err)
		return false
	}
	return true
}

// listarScripts — GET /scripts.
func (h *Handler) listarScripts(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	lista, err := h.scripts.ListarScripts(r.Context(), subj.Sub, esAdminOrg(r), store.MaxPaginaScripts)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"scripts": lista, "can_review": esAdminOrg(r)})
}

// crearScript — POST /scripts {name, kind, scope, document_id?, code, manifest}.
func (h *Handler) crearScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	var req struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		Scope      string `json:"scope"`
		DocumentID string `json:"document_id"`
		cuerpoVersion
	}
	if !decodificar(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > domain.MaxNombreScript {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The script needs a name of up to 120 characters."))
		return
	}
	kind := domain.Kind(req.Kind)
	if kind != domain.KindDocument && kind != domain.KindSpreadsheet && kind != domain.KindPresentation {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown document kind."))
		return
	}
	if req.Scope == "" {
		req.Scope = domain.AmbitoPrivado
	}
	if req.Scope != domain.AmbitoPrivado && req.Scope != domain.AmbitoOrg {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("scope must be private or org."))
		return
	}
	if req.DocumentID != "" {
		if !reUUID.MatchString(req.DocumentID) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("document_id is not valid."))
			return
		}
		// Atar el script a un documento exige poder editarlo.
		if _, _, err := h.svc.Authorize(r.Context(), req.DocumentID, subj, domain.RoleEditor); err != nil {
			fail(w, r, err)
			return
		}
	}
	canon, err := req.validar()
	if err != nil {
		failScript(w, r, err)
		return
	}
	sc, v, err := h.scripts.CrearScript(r.Context(), domain.Script{
		Name: name, Kind: kind, Scope: req.Scope, DocumentID: req.DocumentID, AuthorSub: subj.Sub,
	}, req.Code, canon)
	if err != nil {
		failScript(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"script": sc, "version": v})
}

// verScript — GET /scripts/{sid}: el script y las versiones que quien llama puede ver
// (con código): todas al autor y a un administrador; las aprobadas a los demás.
func (h *Handler) verScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	vs, err := h.scripts.VersionesScript(r.Context(), sc.ID, store.MaxPaginaScripts)
	if err != nil {
		fail(w, r, err)
		return
	}
	todas := sc.AuthorSub == subj.Sub || esAdminOrg(r)
	visibles := vs[:0:0]
	for _, v := range vs {
		if todas || v.Status == domain.EstadoAprobada {
			visibles = append(visibles, v)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"script": sc, "versions": visibles, "can_review": esAdminOrg(r)})
}

// nuevaVersionScript — POST /scripts/{sid}/versions {code, manifest}: solo el autor; sale en borrador.
func (h *Handler) nuevaVersionScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	if sc.AuthorSub != subj.Sub {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var req cuerpoVersion
	if !decodificar(w, r, &req) {
		return
	}
	canon, err := req.validar()
	if err != nil {
		failScript(w, r, err)
		return
	}
	v, err := h.scripts.NuevaVersionScript(r.Context(), sc.ID, req.Code, canon, subj.Sub)
	if err != nil {
		failScript(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"version": v})
}

// ambitoScript — PUT /scripts/{sid}/scope {scope}: solo el autor.
func (h *Handler) ambitoScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	var req struct {
		Scope string `json:"scope"`
	}
	if !decodificar(w, r, &req) {
		return
	}
	if sc.AuthorSub != subj.Sub {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if req.Scope != domain.AmbitoPrivado && req.Scope != domain.AmbitoOrg {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("scope must be private or org."))
		return
	}
	if err := h.scripts.CambiarAmbitoScript(r.Context(), sc.ID, req.Scope); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// borrarScript — DELETE /scripts/{sid}: su autor, o un administrador si es de la organización.
func (h *Handler) borrarScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	if sc.AuthorSub != subj.Sub && (sc.Scope != domain.AmbitoOrg || !esAdminOrg(r)) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if err := h.scripts.BorrarScript(r.Context(), sc.ID); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// proponerVersionScript — POST /scripts/{sid}/versions/{v}/propose: el autor pide la
// aprobación de una versión de un script de la organización.
func (h *Handler) proponerVersionScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	n, ok := numeroVersion(w, r)
	if !ok {
		return
	}
	if sc.AuthorSub != subj.Sub {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if sc.Scope != domain.AmbitoOrg {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("Only organization scripts are submitted for review."))
		return
	}
	if err := h.scripts.ProponerVersionScript(r.Context(), sc.ID, n); err != nil {
		failScript(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// revisarVersionScript — POST /scripts/{sid}/versions/{v}/review {decision, code_hash}:
// solo un administrador, sobre una versión pendiente; aprobar exige el hash del código revisado.
func (h *Handler) revisarVersionScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	n, ok := numeroVersion(w, r)
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
		CodeHash string `json:"code_hash"`
	}
	if !decodificar(w, r, &req) {
		return
	}
	if sc.Scope != domain.AmbitoOrg || (req.Decision != "approve" && req.Decision != "reject") {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Review an organization script with decision approve or reject."))
		return
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(req.CodeHash) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("code_hash must be the SHA-256 of the reviewed code."))
		return
	}
	if err := h.scripts.RevisarVersionScript(r.Context(), sc.ID, n, req.CodeHash, req.Decision == "approve", subj.Sub); err != nil {
		failScript(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ejecutarScript — POST /scripts/{sid}/versions/{v}/runs {document_id}: autoriza una
// ejecución, descuenta la cuota y devuelve el código y el manifiesto QUE SE AUTORIZARON
// (el cliente ejecuta exactamente eso) junto con el id de la ejecución, que cerrará con
// PUT /script-runs/{id}.
func (h *Handler) ejecutarScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	sc, ok := h.scriptVisible(w, r, subj.Sub)
	if !ok {
		return
	}
	n, ok := numeroVersion(w, r)
	if !ok {
		return
	}
	var req struct {
		DocumentID string `json:"document_id"`
	}
	if !decodificar(w, r, &req) {
		return
	}
	if !reUUID.MatchString(req.DocumentID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("document_id is not valid."))
		return
	}
	v, err := h.scripts.ObtenerVersionScript(r.Context(), sc.ID, n)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Los rechazos de aquí en adelante quedan en el registro: un intento denegado es un dato.
	denegar := func(causa error, codigo string) {
		h.denegarEjecucion(w, r, domain.EjecucionScript{
			ScriptID: sc.ID, Version: n, CodeHash: v.CodeHash, DocumentID: req.DocumentID, Sub: subj.Sub,
		}, codigo, causa)
	}
	if err := domain.PuedeEjecutarVersion(sc, v, subj.Sub); err != nil {
		denegar(err, "not_approved")
		return
	}
	// Se vuelve a validar AHORA: una versión guardada cuando el registro admitía un comando
	// que ya no admite deja de poder ejecutarse (no se borra: se corrige con otra versión).
	m, err := domain.ParseManifiesto(v.Manifest)
	if err != nil {
		denegar(err, "manifest_not_allowed")
		return
	}
	doc, _, err := h.svc.Authorize(r.Context(), req.DocumentID, subj, domain.RolParaEjecutar(m))
	if err != nil {
		denegar(err, "forbidden")
		return
	}
	if doc.Kind != sc.Kind || (sc.DocumentID != "" && sc.DocumentID != doc.ID) {
		denegar(domain.ErrForbidden, "wrong_document")
		return
	}
	id, err := h.scripts.ReservarEjecucion(r.Context(), domain.EjecucionScript{
		ScriptID: sc.ID, Version: n, CodeHash: v.CodeHash, DocumentID: doc.ID, Sub: subj.Sub,
	}, m.MaxMs)
	if err != nil {
		failScript(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"run_id": id, "code": v.Code, "code_hash": v.CodeHash, "manifest": v.Manifest,
	})
}

func (h *Handler) denegarEjecucion(w http.ResponseWriter, r *http.Request, e domain.EjecucionScript, codigo string, causa error) {
	_, _ = h.scripts.ReservarDenegada(r.Context(), e, codigo)
	failScript(w, r, causa)
}

// cerrarEjecucionScript — PUT /script-runs/{id} {outcome, error_code, duration_ms, ops}.
// Lo informa el cliente (el servidor no ve el código correr); solo quien abrió la
// ejecución, una vez, y con valores acotados.
func (h *Handler) cerrarEjecucionScript(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "runId"), 10, 64)
	if err != nil || id < 1 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req struct {
		Outcome    string `json:"outcome"`
		ErrorCode  string `json:"error_code"`
		DurationMs int    `json:"duration_ms"`
		Ops        int    `json:"ops"`
	}
	if !decodificar(w, r, &req) {
		return
	}
	switch req.Outcome {
	case domain.ResOK, domain.ResError, domain.ResDenegado, domain.ResTiempo, domain.ResLimite:
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("outcome is not valid."))
		return
	}
	if req.DurationMs < 0 || req.DurationMs > domain.TopeMsScript*2 || req.Ops < 0 || req.Ops > domain.TopeOpsScript*2 ||
		len(req.ErrorCode) > 64 || !reCodigoError.MatchString(req.ErrorCode) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The run report is out of range."))
		return
	}
	if err := h.scripts.CerrarEjecucion(r.Context(), id, subj.Sub, req.Outcome, req.ErrorCode, req.DurationMs, req.Ops); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// reCodigoError: un código corto, nunca texto libre (que podría arrastrar contenido).
var reCodigoError = regexp.MustCompile(`^[a-z0-9_.-]*$`)

// pagina lee before_id y limit.
func paginaScripts(w http.ResponseWriter, r *http.Request) (antesDe int64, limite int, ok bool) {
	q := r.URL.Query()
	limite = 50
	if s := q.Get("before_id"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("before_id must be a non-negative integer."))
			return 0, 0, false
		}
		antesDe = n
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > store.MaxPaginaScripts {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("limit must be between 1 and "+strconv.Itoa(store.MaxPaginaScripts)+"."))
			return 0, 0, false
		}
		limite = n
	}
	return antesDe, limite, true
}

func respuestaAuditoria(w http.ResponseWriter, r *http.Request, es []domain.EjecucionScript, limite int) {
	sig := int64(0)
	if len(es) == limite {
		sig = es[len(es)-1].ID
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"runs": es, "next_before_id": sig})
}

// auditoriaScriptsOrg — GET /script-runs?before_id=&limit=: el registro de la organización, solo administradores.
func (h *Handler) auditoriaScriptsOrg(w http.ResponseWriter, r *http.Request) {
	if _, ok := sesionScripts(w, r); !ok {
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	antesDe, limite, ok := paginaScripts(w, r)
	if !ok {
		return
	}
	es, err := h.scripts.EjecucionesScript(r.Context(), "", antesDe, limite)
	if err != nil {
		fail(w, r, err)
		return
	}
	respuestaAuditoria(w, r, es, limite)
}

// auditoriaScriptsDoc — GET /documents/{id}/script-runs: lo ejecutado sobre ese documento, para su propietario.
func (h *Handler) auditoriaScriptsDoc(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	antesDe, limite, ok := paginaScripts(w, r)
	if !ok {
		return
	}
	es, err := h.scripts.EjecucionesScript(r.Context(), docID(r), antesDe, limite)
	if err != nil {
		fail(w, r, err)
		return
	}
	respuestaAuditoria(w, r, es, limite)
}

// cuotaScripts — GET /script-quota: lo consumido en la última hora y los topes.
func (h *Handler) cuotaScripts(w http.ResponseWriter, r *http.Request) {
	subj, ok := sesionScripts(w, r)
	if !ok {
		return
	}
	uso, err := h.scripts.UsoCuotaScripts(r.Context(), subj.Sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"used":   map[string]any{"user_runs": uso.EjecucionesUsuario, "org_runs": uso.EjecucionesOrg, "user_ms": uso.MsUsuario},
		"limits": map[string]any{"user_runs": domain.CuotaEjecucionesUsuarioHora, "org_runs": domain.CuotaEjecucionesOrgHora, "user_ms": domain.CuotaMsUsuarioHora},
	})
}
