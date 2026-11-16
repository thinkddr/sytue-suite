// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	contactsmodel "git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

// ServiceTransport is the sole browser network adapter for the existing Go API
// clients. A URL is internal to those clients: only a fixed operation and its
// validated path arguments reach the host. Cookies and credentials never enter
// Go/Wasm. The host independently matches its immutable scope/session witness.
type ServiceTransport struct { //nolint:revive // ServiceTransport stutters with the package name but is the established public API name
	owner   *webport.Storage
	service string
}

func isSourceService(service string) bool {
	switch service {
	case "calendar", "notes", "photos", "cast", "docs", "sheets", "slides", "hub", "bases", "erp":
		return true
	default:
		return false
	}
}

func serviceAllowed(product, service string) bool {
	if product == "mail" {
		return service == "mail" || service == "rooms" || service == "drive"
	}
	if product == "contacts" {
		return service == "contacts" || service == "rooms"
	}
	if isSourceService(product) {
		return service == product || service == "rooms"
	}
	return false
}

func NewServiceTransport(owner *webport.Storage, service string) (*ServiceTransport, error) {
	if owner == nil || !serviceAllowed(owner.FileScope().Product, service) {
		return nil, webport.ErrRejected
	}
	return &ServiceTransport{owner: owner, service: service}, nil
}

func (t *ServiceTransport) Mail() *api.Mail {
	return api.NewMail("https://mail.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Drive() *api.Drive {
	return api.NewDrive("https://drive.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Contacts() *api.Contacts {
	return api.NewContacts("https://contacts.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Calendars() *api.Calendars {
	return api.NewCalendars("https://calendar.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Notes() *api.Notes {
	return api.NewNotes("https://notes.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Photos() *api.Photos {
	return api.NewPhotos("https://photos.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Cast() *api.Cast {
	return api.NewCast("https://cast.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Docs() *api.Docs {
	return api.NewDocs("https://"+t.service+".invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Slides() *api.Slides {
	return api.NewSlides("https://"+t.service+".invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Hub() *api.Hub {
	return api.NewHub("https://hub.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) Bases() *api.Bases {
	return api.NewBases("https://bases.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func (t *ServiceTransport) ERP() *api.ERP {
	return api.NewERP("https://erp.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, "")
}

func (t *ServiceTransport) Rooms() *api.Rooms {
	return api.NewRooms("https://rooms.invalid", nil, &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

type serviceRequest struct {
	Operation   string   `json:"operation"`
	Arguments   []string `json:"arguments,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	Submission  string   `json:"submission,omitempty"`
	Source      string   `json:"source,omitempty"`
	Size        int64    `json:"size"`
}
type serviceResponse struct {
	Status          int    `json:"status"`
	Stream          string `json:"stream"`
	ContentType     string `json:"content_type"`
	ContentEncoding string `json:"content_encoding,omitempty"`
	ContentLength   int64  `json:"content_length"`
	SourceSHA256    string `json:"source_sha256,omitempty"`
}

func (t *ServiceTransport) classify(req *http.Request) (serviceRequest, error) {
	var out serviceRequest
	content, err := validateRequestHeaders(req, t.owner.FileScope(), t.service)
	if err != nil {
		return out, err
	}
	out.ContentType = content
	path := req.URL.EscapedPath()
	query := req.URL.Query()
	if req.URL.RawQuery != query.Encode() {
		return out, webport.ErrRejected
	}
	switch {
	case t.service == "drive":
		out, err = classifyDrive(req.Method, path, query, out)
	case t.service == "rooms":
		out, err = t.classifyRooms(req.Method, path, query, out)
	case t.service == "contacts":
		out, err = classifyContacts(req.Method, path, query, out)
	case isSourceService(t.service):
		out, err = t.classifySourceService(req.Method, path, query, out)
	default:
		out, err = classifyMail(req, path, content, query, out)
	}
	if err != nil {
		return out, err
	}
	if out.Operation != "mail.submission.send" && (content == "message/rfc822" || req.Header.Get(model.SubmissionHeader) != "") {
		return out, webport.ErrRejected
	}
	return out, nil
}

func validateRequestHeaders(req *http.Request, scope storage.Scope, service string) (string, error) {
	expected, _ := storage.EncodeScopeHeader(scope)
	if !serviceAllowed(scope.Product, service) || req == nil || req.URL == nil || req.URL.Scheme != "https" || req.URL.Host != service+".invalid" || req.URL.User != nil || req.URL.Fragment != "" || req.Header.Get(storage.ScopeHeader) != expected {
		return "", webport.ErrRejected
	}
	for key, values := range req.Header {
		if len(values) != 1 {
			return "", webport.ErrRejected
		}
		switch http.CanonicalHeaderKey(key) {
		case storage.ScopeHeader, "Accept", "Content-Type", model.SubmissionHeader:
		default:
			return "", webport.ErrRejected
		}
	}
	if accept := req.Header.Get("Accept"); accept != "" && accept != "application/json" {
		return "", webport.ErrRejected
	}
	content := req.Header.Get("Content-Type")
	if content != "" && content != "application/json" && content != "message/rfc822" {
		return "", webport.ErrRejected
	}
	return content, nil
}

func classifyDrive(method, path string, query url.Values, out serviceRequest) (serviceRequest, error) {
	if len(query) != 0 || path != "/api/v1/cipher/usage/mail" || method != http.MethodGet {
		return out, webport.ErrRejected
	}
	out.Operation = "drive.cipher.usage"
	return out, nil
}

func (t *ServiceTransport) classifyRooms(method, path string, query url.Values, out serviceRequest) (serviceRequest, error) {
	if len(query) != 0 {
		return out, webport.ErrRejected
	}
	root := "/api/v1/matrix/storage/" + t.owner.FileScope().Product + "/"
	switch {
	case path == root+"authority" && method == http.MethodGet:
		out.Operation = "rooms.authority"
	case path == root+"migration" && method == http.MethodGet:
		out.Operation = "rooms.cutover.state"
	case path == root+"migration" && method == http.MethodPost:
		out.Operation = "rooms.cutover.begin"
	case path == root+"migration/complete" && method == http.MethodPost:
		out.Operation = "rooms.cutover.complete"
	default:
		return out, webport.ErrRejected
	}
	return out, nil
}

func classifyContacts(method, path string, query url.Values, out serviceRequest) (serviceRequest, error) {
	root := "/api/v1/private-storage/migration"
	var err error
	switch {
	case path == root && method == http.MethodGet:
		out.Operation = "contacts.source.state"
	case path == root && method == http.MethodPost:
		out.Operation = "contacts.source.begin"
	case strings.HasPrefix(path, root+"/") && method == http.MethodGet:
		pieces := strings.Split(strings.TrimPrefix(path, root+"/"), "/")
		if len(pieces) < 2 || !storage.MigrationEpoch(pieces[0]) {
			return out, webport.ErrRejected
		}
		switch {
		case len(pieces) == 2 && pieces[1] == "rows":
			out, err = classifySourceRows("contacts", pieces[0], query, out)
			if err != nil {
				return out, err
			}
			query = nil
		case len(pieces) == 4 && pieces[1] == "blobs":
			kind, id := pieces[2], pieces[3]
			if !contactsmodel.SourceKind(kind) || !storage.MigrationEpoch(id) {
				return out, webport.ErrRejected
			}
			out.Operation = "contacts.source.open"
			out.Arguments = []string{pieces[0], kind, id}
		default:
			return out, webport.ErrRejected
		}
	default:
		return out, webport.ErrRejected
	}
	if len(query) != 0 {
		return out, webport.ErrRejected
	}
	return out, nil
}

func (t *ServiceTransport) classifySourceService(method, path string, query url.Values, out serviceRequest) (serviceRequest, error) {
	root := "/api/v1/private-storage/migration"
	var err error
	switch {
	case path == root && method == http.MethodGet:
		out.Operation = t.service + ".source.state"
	case path == root && method == http.MethodPost:
		out.Operation = t.service + ".source.begin"
	case strings.HasPrefix(path, root+"/") && method == http.MethodGet:
		pieces := strings.Split(strings.TrimPrefix(path, root+"/"), "/")
		out, query, err = classifySourceMigrationPieces(t.service, pieces, query, out)
		if err != nil {
			return out, err
		}
	default:
		return out, webport.ErrRejected
	}
	if len(query) != 0 {
		return out, webport.ErrRejected
	}
	return out, nil
}

func classifySourceMigrationPieces(service string, pieces []string, query url.Values, out serviceRequest) (serviceRequest, url.Values, error) {
	if len(pieces) < 2 || !storage.MigrationEpoch(pieces[0]) {
		return out, query, webport.ErrRejected
	}
	switch {
	case len(pieces) == 2 && pieces[1] == "rows":
		var err error
		out, err = classifySourceRows(service, pieces[0], query, out)
		return out, nil, err
	case len(pieces) == 4 && pieces[1] == "source":
		var err error
		out, err = classifySourceOpen(service, pieces[0], pieces[2], pieces[3], out)
		return out, query, err
	default:
		return out, query, webport.ErrRejected
	}
}

func classifySourceRows(service, epoch string, query url.Values, out serviceRequest) (serviceRequest, error) {
	if len(query) > 1 || len(query["after"]) > 1 || (len(query) == 1 && len(query["after"]) != 1) {
		return out, webport.ErrRejected
	}
	after := query.Get("after")
	if after != "" {
		if _, err := storage.DecodeSourceCursor(after); err != nil {
			return out, webport.ErrRejected
		}
	}
	out.Operation = service + ".source.page"
	out.Arguments = []string{epoch, after}
	return out, nil
}

func classifySourceOpen(service, epoch, rawKind, rawID string, out serviceRequest) (serviceRequest, error) {
	kind, e1 := url.PathUnescape(rawKind)
	id, e2 := url.PathUnescape(rawID)
	if e1 != nil || e2 != nil || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return out, webport.ErrRejected
	}
	out.Operation = service + ".source.open"
	out.Arguments = []string{epoch, kind, id}
	return out, nil
}

//nolint:gocyclo // one branch per allowed method/path route: the branching IS the allowlist
func classifyMail(req *http.Request, path, content string, query url.Values, out serviceRequest) (serviceRequest, error) {
	if (path == "/api/v1/aliases" || strings.HasPrefix(path, "/api/v1/aliases/") || path == "/api/v1/quota") && len(query) != 0 {
		return out, webport.ErrRejected
	}
	root := "/api/v1/private-storage/"
	var err error
	switch {
	case path == "/api/v1/aliases" && req.Method == http.MethodGet:
		out.Operation = "mail.routing.get"
	case path == "/api/v1/aliases" && req.Method == http.MethodPost:
		out.Operation = "mail.routing.create"
	case strings.HasPrefix(path, "/api/v1/aliases/") && req.Method == http.MethodDelete:
		alias := strings.TrimPrefix(path, "/api/v1/aliases/")
		if !storage.MigrationEpoch(alias) {
			return out, webport.ErrRejected
		}
		out.Operation = "mail.routing.delete"
		out.Arguments = []string{alias}
	case path == "/api/v1/quota" && req.Method == http.MethodGet:
		out.Operation = "mail.routing.plan"
	case path == root+"migration" && req.Method == http.MethodGet:
		out.Operation = "mail.source.state"
	case path == root+"migration" && req.Method == http.MethodPost:
		out.Operation = "mail.source.begin"
	case strings.HasPrefix(path, root+"migration/") && req.Method == http.MethodGet:
		pieces := strings.Split(strings.TrimPrefix(path, root+"migration/"), "/")
		out, query, err = classifySourceMigrationPieces("mail", pieces, query, out)
		if err != nil {
			return out, err
		}
	case strings.HasPrefix(path, root) && (req.Method == http.MethodPost || req.Method == http.MethodGet):
		out, err = classifyMailIngressOrSubmission(req, strings.TrimPrefix(path, root), content, out)
		if err != nil {
			return out, err
		}
	default:
		return out, webport.ErrRejected
	}
	if len(query) != 0 {
		return out, webport.ErrRejected
	}
	return out, nil
}

//nolint:gocyclo // one branch per allowed method/path route: the branching IS the allowlist
func classifyMailIngressOrSubmission(req *http.Request, subpath, content string, out serviceRequest) (serviceRequest, error) {
	switch {
	case strings.HasPrefix(subpath, "ingress/") && req.Method == http.MethodPost:
		name := strings.TrimPrefix(subpath, "ingress/")
		switch name {
		case "challenge", "apply", "pending", "revalidate", "body", "enrollment":
			out.Operation = "mail.ingress." + name
			return out, nil
		default:
			return out, webport.ErrRejected
		}
	case subpath == "submissions" && req.Method == http.MethodPost:
		out.Operation = "mail.submission.send"
		out.Submission = req.Header.Get(model.SubmissionHeader)
		if content != "message/rfc822" || out.Submission == "" || len(out.Submission) > 32<<10 || req.ContentLength <= 0 || req.ContentLength > storage.MaxFileBytes {
			return out, webport.ErrRejected
		}
		return out, nil
	case subpath == "submissions/register" && req.Method == http.MethodPost:
		out.Operation = "mail.submission.register"
		return out, nil
	case subpath == "submissions/control" && req.Method == http.MethodPost:
		out.Operation = "mail.submission.control"
		return out, nil
	case strings.HasPrefix(subpath, "submissions/") && req.Method == http.MethodGet:
		pieces := strings.Split(strings.TrimPrefix(subpath, "submissions/"), "/")
		if len(pieces) != 2 || !storage.MigrationEpoch(pieces[0]) {
			return out, webport.ErrRejected
		}
		operation, err := url.PathUnescape(pieces[1])
		if err != nil || !storage.Identifier(operation) {
			return out, webport.ErrRejected
		}
		out.Operation = "mail.submission.receipt"
		out.Arguments = []string{pieces[0], operation}
		return out, nil
	default:
		return out, webport.ErrRejected
	}
}

func (v serviceResponse) valid() bool {
	return v.Status >= 200 && v.Status <= 599 &&
		webport.ValidID(v.Stream) &&
		v.ContentLength >= -1 && v.ContentLength <= storage.MaxFileBytes &&
		len(v.ContentType) <= 256 && len(v.ContentEncoding) <= 128 &&
		(v.SourceSHA256 == "" || webport.ValidID(v.SourceSHA256))
}

func (v serviceResponse) header() http.Header {
	header := make(http.Header)
	header.Set("Content-Type", v.ContentType)
	if v.ContentEncoding != "" {
		header.Set("Content-Encoding", v.ContentEncoding)
	}
	if v.ContentLength >= 0 {
		header.Set("Content-Length", strconv.FormatInt(v.ContentLength, 10))
	}
	if v.SourceSHA256 != "" {
		header.Set("X-Sytue-Source-SHA256", v.SourceSHA256)
	}
	return header
}

func (t *ServiceTransport) RoundTrip(req *http.Request) (_ *http.Response, resultErr error) { //nolint:contextcheck // http.RoundTripper carries its ctx in req.Context(), not as a parameter
	input, err := t.classify(req)
	if err != nil {
		return nil, err
	}
	if req.Body != nil {
		defer req.Body.Close()
	}
	var body []byte
	if input.Operation == "mail.submission.send" {
		input.Size = req.ContentLength
		id, release, err := t.owner.AddServiceSource(req.Context(), req.Body, input.Size)
		if err != nil {
			return nil, err
		}
		defer release()
		input.Source = id
	} else if req.Body != nil {
		body, err = io.ReadAll(io.LimitReader(req.Body, 256<<10+1))
		if err != nil {
			return nil, err
		}
		defer clear(body)
		if len(body) > 256<<10 {
			return nil, webport.ErrRejected
		}
		input.Size = int64(len(body))
	}
	metadata, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	defer clear(metadata)
	frame := make([]byte, 4+len(metadata)+len(body))
	defer clear(frame)
	binary.BigEndian.PutUint32(frame, uint32(len(metadata))) //nolint:gosec // G115: metadata is bounded JSON
	copy(frame[4:], metadata)
	copy(frame[4+len(metadata):], body)
	raw, err := t.owner.ServiceCall(req.Context(), webport.ServiceRequest, frame)
	if err != nil {
		clear(raw)
		return nil, err
	}
	defer clear(raw)
	var value serviceResponse
	decodeErr := json.Unmarshal(raw, &value)
	adopted := false
	if webport.ValidID(value.Stream) {
		defer func() { //nolint:contextcheck // the stream close must outlive a cancelled request, hence context.WithoutCancel
			if !adopted {
				reply, closeErr := serviceJSON(context.WithoutCancel(req.Context()), t.owner, webport.ServiceClose, struct{ Stream string }{value.Stream})
				clear(reply)
				resultErr = errors.Join(resultErr, closeErr)
			}
		}()
	}
	if decodeErr != nil || !value.valid() {
		return nil, webport.ErrRejected
	}
	header := value.header()
	life, cancel := context.WithCancel(req.Context())
	reader := &serviceBody{owner: t.owner, ctx: life, cancel: cancel, stream: value.Stream}
	reader.stop = context.AfterFunc(req.Context(), func() { _ = reader.Close() }) //nolint:contextcheck // the body is closed when the request ctx ends; the callback needs no ctx
	adopted = true
	return &http.Response{StatusCode: value.Status, Status: strconv.Itoa(value.Status) + " " + http.StatusText(value.Status), Header: header, Body: reader, ContentLength: value.ContentLength, Request: req}, nil
}

type serviceBody struct {
	owner    *webport.Storage
	ctx      context.Context
	cancel   context.CancelFunc
	stop     func() bool
	stream   string
	mu       sync.Mutex
	offset   int64
	closed   bool
	closeErr error
}

func (r *serviceBody) Read(out []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(out) == 0 {
		return 0, nil
	}
	length := min(len(out), webport.ChunkBytes)
	raw, err := serviceJSON(r.ctx, r.owner, webport.ServiceRead, struct {
		Stream string `json:"stream"`
		Offset int64  `json:"offset"`
		Length int    `json:"length"`
	}{r.stream, r.offset, length})
	defer clear(raw)
	if err != nil {
		return 0, errors.Join(err, r.closeLocked())
	}
	if len(raw) > length {
		return 0, errors.Join(webport.ErrRejected, r.closeLocked())
	}
	if len(raw) == 0 {
		return 0, io.EOF
	}
	n := copy(out, raw)
	r.offset += int64(n)
	return n, nil
}

func (r *serviceBody) Close() error {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked()
}

func (r *serviceBody) closeLocked() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.cancel()
	if r.stop != nil {
		r.stop()
	}
	raw, err := serviceJSON(context.WithoutCancel(r.ctx), r.owner, webport.ServiceClose, struct{ Stream string }{r.stream})
	clear(raw)
	r.closeErr = err
	return err
}

func (r *serviceBody) WriteTo(writer io.Writer) (int64, error) {
	if writer == nil {
		return 0, webport.ErrRejected
	}
	buffer := make([]byte, webport.ChunkBytes)
	defer clear(buffer)
	return io.CopyBuffer(struct{ io.Writer }{writer}, struct{ io.Reader }{r}, buffer)
}

var (
	_ http.RoundTripper = (*ServiceTransport)(nil)
	_ io.ReadCloser     = (*serviceBody)(nil)
)

func serviceJSON(ctx context.Context, owner *webport.Storage, method webport.Method, value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return owner.ServiceCall(ctx, method, raw)
}
