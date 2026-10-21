// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Store es el contrato de persistencia que el servicio de dominio necesita.
// Lo implementa internal/store.Store; declararlo aquí mantiene el dominio
// independiente de pgx (inversión de dependencias, testeable con fakes).
type Store interface {
	CreateDocument(ctx context.Context, p CreateDocumentParams) (*Document, bool, error)
	GetDocument(ctx context.Context, id string) (*Document, error)
	ListDocuments(ctx context.Context, subj Subject, f ListFilter) ([]Document, error)
	RenameDocument(ctx context.Context, id, title string) error
	SoftDeleteDocument(ctx context.Context, id string) error
	SetStarred(ctx context.Context, id, sub string, starred bool) error

	// AppendOps secuencia un lote y reautoriza al subject dentro de la transacción.
	// Con baseSeq != nil rechaza con ErrStaleSeq si alguna clave de los `set` del
	// lote se escribió después de esa seq.
	AppendOps(ctx context.Context, docID string, author Subject, ops []Op, baseSeq *int64) ([]SequencedOp, int64, error)
	OpsSince(ctx context.Context, docID string, sinceSeq int64, limit int) ([]SequencedOp, error)
	LoadState(ctx context.Context, docID string) (*LoadedState, error)
	SaveSnapshot(ctx context.Context, p SaveSnapshotParams) (string, error)
	GetSnapshot(ctx context.Context, id string) (*Snapshot, error)

	ListPermissions(ctx context.Context, docID string) ([]Permission, error)
	UpsertPermission(ctx context.Context, docID string, p Permission) error
	RevokePermission(ctx context.Context, docID string, kind PrincipalKind, principalID string) error
	// CreateShareLink: `passwordHash` vacío = el enlace no pide contraseña.
	CreateShareLink(ctx context.Context, docID, tokenHash, passwordHash, createdBy string, role Role, expiresAt *time.Time) (*ShareLink, error)
	ListShareLinks(ctx context.Context, docID string) ([]ShareLink, error)
	RevokeShareLink(ctx context.Context, docID, linkID string) error
	ResolveShareToken(ctx context.Context, tokenHash string) (*ShareResolution, error)
	// PoliticaEnlaces es el techo de caducidad de la organización del contexto.
	PoliticaEnlaces(ctx context.Context) (PoliticaEnlaces, error)

	CreateVersion(ctx context.Context, docID, snapshotID, authorSub string, seq int64, label *string, auto bool) (*Version, error)
	ListVersions(ctx context.Context, docID string, limit int) ([]Version, error)
	GetVersion(ctx context.Context, docID, versionID string) (*Version, error)
	SnapshotState(ctx context.Context, snapshotID string, hydrate func(ctx context.Context, blobKey string) (*State, error)) (*State, int64, error)

	CreateComment(ctx context.Context, p CreateCommentParams) (*Comment, error)
	ListComments(ctx context.Context, docID string, includeResolved bool) ([]Comment, error)
	ResolveComment(ctx context.Context, docID, commentID, bySub string, resolved bool) error
	DeleteComment(ctx context.Context, docID, commentID, bySub string, moderador bool) error

	RecordBlob(ctx context.Context, b Blob) (*Blob, error)
	GetBlob(ctx context.Context, docID, blobID string) (*Blob, error)
	ListBlobs(ctx context.Context, docID string) ([]Blob, error)
}

// Tipos compartidos entre dominio y store (parámetros/DTO neutrales).

// CreateDocumentParams son los datos para crear un documento.
type CreateDocumentParams struct {
	Kind     Kind
	Title    string
	OwnerSub string
	// SourceRef ancla el documento al fichero del que salió («drive:<uuid>»).
	// Vacío para un documento nacido aquí. Con él, crear es idempotente: la
	// segunda vez devuelve el mismo documento en lugar de otra copia.
	SourceRef string
	// Seed son las operaciones con las que nace el documento (el contenido de una
	// plantilla). Se escriben en la MISMA transacción que la fila: un documento
	// creado desde una plantilla nunca existe vacío. Solo se aplican si el
	// documento es nuevo; si ya existía (mismo SourceRef), no se tocan.
	Seed []Op
}

// ListFilter filtra el listado de documentos. La paginación es keyset por
// (updated_at, id) descendente: se devuelven documentos ESTRICTAMENTE
// anteriores al cursor (nunca OFFSET, que degrada con la profundidad).
type ListFilter struct {
	Kind     Kind
	OwnerSub string
	Query    string
	Limit    int
	// StarredOnly acota a los que el SOLICITANTE ha destacado. Se filtra en la
	// base y no en el cliente: con paginación keyset, filtrar la página traída
	// enseñaría «ninguno» mientras los destacados esperan tres páginas más allá.
	StarredOnly bool
	// Cursor keyset: (updated_at, id) del último documento de la página
	// anterior. Nil/vacío = primera página.
	BeforeUpdatedAt *time.Time
	BeforeID        string
	// SourceRef acota al documento nacido de un fichero concreto («drive:<uuid>»).
	// Es lo que permite que abrir el mismo fichero otra vez encuentre el documento
	// que ya había sin tener que descargarlo y convertirlo de nuevo para saberlo.
	SourceRef string
}

// SequencedOp es una operación CRDT ya secuenciada por el servidor.
type SequencedOp struct {
	DocumentID string `json:"document_id"`
	Seq        int64  `json:"seq"`
	Actor      string `json:"actor"`
	AuthorSub  string `json:"author_sub"`
	Lamport    int64  `json:"lamport"`
	Op         Op     `json:"op"`
}

// LoadedState es el estado CRDT cargado y su contexto de carga.
type LoadedState struct {
	State   *State
	Seq     int64
	BlobKey string
	BaseSeq int64
}

// SaveSnapshotParams describe un snapshot a persistir.
type SaveSnapshotParams struct {
	DocumentID string
	Seq        int64
	State      []byte
	BlobKey    string
	SizeBytes  int64
}

// CreateCommentParams describe un comentario nuevo.
type CreateCommentParams struct {
	DocumentID string
	ParentID   *string
	AuthorSub  string
	Anchor     []byte
	Body       string
}

// ShareResolution es el resultado mínimo de resolver un token de enlace.
type ShareResolution struct {
	DocumentID string
	Tenant     string
	Role       Role
}

// Service orquesta la lógica de negocio de Sytue Docs: aplica autorización
// (deny-by-default) sobre el store y coordina versionado/colaboración.
type Service struct {
	store Store
	// Hidratar descarga el estado base de un snapshot alojado en Object Storage. Lo
	// necesita la protección de hojas para leer el modelo vigente de un documento antes
	// de su primera escritura; nil si no hay almacenamiento de objetos.
	Hidratar  func(ctx context.Context, blobKey string) (*State, error)
	claves    limitadorClaves
	iniciados iniciadosDocs
}

// NewService construye el servicio de dominio.
func NewService(store Store) *Service { return &Service{store: store} }

// Store expone el store subyacente (para cableado de colab/exports).
func (s *Service) Store() Store { return s.store }

// Authorize resuelve el rol efectivo del subject sobre un documento y comprueba
// que alcanza el mínimo requerido. Centraliza la autorización deny-by-default.
func (s *Service) Authorize(ctx context.Context, docID string, subj Subject, minRole Role) (*Document, Role, error) {
	doc, err := s.store.GetDocument(ctx, docID)
	if err != nil {
		return nil, "", err
	}
	perms, err := s.store.ListPermissions(ctx, docID)
	if err != nil {
		return nil, "", err
	}
	role, ok := ResolveRole(doc, perms, subj)
	if !ok || !role.AtLeast(minRole) {
		return nil, "", ErrForbidden
	}
	doc.Starred = contains(doc.StarredBy, subj.Sub)
	return doc, role, nil
}

// Create crea un documento; el creador queda como owner.
// Create crea un documento. `sourceRef` lo ancla al fichero del que salió y hace
// la operación idempotente: devuelve `creado=false` y el documento que ya había si
// esa persona ya abrió ese mismo fichero antes.
func (s *Service) Create(ctx context.Context, subj Subject, kind Kind, title, sourceRef string) (*Document, bool, error) {
	return s.CreateSeeded(ctx, subj, kind, title, sourceRef, nil)
}

// CreateSeeded es Create con el contenido inicial ya dentro: `seed` se escribe en
// la misma transacción que crea el documento (ver CreateDocumentParams.Seed).
func (s *Service) CreateSeeded(ctx context.Context, subj Subject, kind Kind, title, sourceRef string, seed []Op) (*Document, bool, error) {
	if !kind.Valid() {
		return nil, false, ErrInvalidKind
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Sin título"
	}
	return s.store.CreateDocument(ctx, CreateDocumentParams{
		Kind: kind, Title: title, OwnerSub: subj.Sub, SourceRef: strings.TrimSpace(sourceRef), Seed: seed,
	})
}

// List devuelve los documentos accesibles por el subject.
func (s *Service) List(ctx context.Context, subj Subject, f ListFilter) ([]Document, error) {
	docs, err := s.store.ListDocuments(ctx, subj, f)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i].Starred = contains(docs[i].StarredBy, subj.Sub)
	}
	return docs, nil
}

// Rename cambia el título (requiere editor).
func (s *Service) Rename(ctx context.Context, subj Subject, docID, title string) error {
	if strings.TrimSpace(title) == "" {
		return ErrTitleRequired
	}
	if _, _, err := s.Authorize(ctx, docID, subj, RoleEditor); err != nil {
		return err
	}
	return s.store.RenameDocument(ctx, docID, strings.TrimSpace(title))
}

// Delete envía el documento a la papelera (requiere owner).
func (s *Service) Delete(ctx context.Context, subj Subject, docID string) error {
	if _, _, err := s.Authorize(ctx, docID, subj, RoleOwner); err != nil {
		return err
	}
	return s.store.SoftDeleteDocument(ctx, docID)
}

// Star marca/desmarca favorito (requiere lectura).
func (s *Service) Star(ctx context.Context, subj Subject, docID string, starred bool) error {
	if _, _, err := s.Authorize(ctx, docID, subj, RoleViewer); err != nil {
		return err
	}
	return s.store.SetStarred(ctx, docID, subj.Sub, starred)
}

// ApplyOps secuencia y persiste operaciones de edición (requiere editor).
// Devuelve las operaciones secuenciadas para difundirlas a los colaboradores.
//
// baseSeq es la seq hasta la que quien escribe conoce el documento. Con ella el
// guardado es condicional: si otra persona escribió después alguna de las claves
// del lote, ErrStaleSeq (409) y nada se persiste; el cliente fusiona y reintenta.
// nil = escritura LWW incondicional de siempre (Fyne, documentos de texto).
func (s *Service) ApplyOps(ctx context.Context, subj Subject, docID string, ops []Op, baseSeq *int64) ([]SequencedOp, int64, error) {
	doc, _, err := s.Authorize(ctx, docID, subj, RoleEditor)
	if err != nil {
		return nil, 0, err
	}
	return s.appendProtegido(ctx, doc, subj, ops, baseSeq)
}

// appendProtegido es AppendOps con la protección de hojas: la que usan TODAS las vías
// de escritura (POST /ops, WebSocket y restauración de versiones). La regla vive en el
// almacén, dentro de la transacción; aquí solo se prepara su estado la primera vez y
// se frena el tanteo de contraseñas.
func (s *Service) appendProtegido(ctx context.Context, doc *Document, subj Subject, ops []Op, baseSeq *int64) ([]SequencedOp, int64, error) {
	clave := doc.ID + "|" + subj.Sub
	bloqueada := s.claves.bloqueada(clave)
	if bloqueada {
		if ProteccionDe(ctx).Clave != "" {
			return nil, 0, ErrClaveProteccion
		}
		// La contraseña también puede venir dentro del modelo o como contraseña nueva,
		// y eso solo lo ve el almacén al abrir el lote: se le dice que no compruebe ninguna.
		p := ProteccionDe(ctx)
		p.SinIntentos = true
		ctx = context.WithValue(ctx, ctxProteccion{}, p)
	}
	for intento := 0; ; intento++ {
		if err := s.iniciarProteccion(ctx, doc, ops); err != nil {
			return nil, 0, err
		}
		seqd, seq, err := s.store.AppendOps(ctx, doc.ID, subj, ops, baseSeq)
		switch {
		case errors.Is(err, ErrProteccionPendiente) && intento == 0:
			continue
		case errors.Is(err, ErrSecuenciaPendiente) && intento < 2:
			if err := s.iniciarSecuencia(ctx, doc); err != nil {
				return nil, 0, err
			}
			continue
		case errors.Is(err, ErrClaveProteccion) && !bloqueada:
			s.claves.fallo(clave)
		}
		return seqd, seq, err
	}
}

// almacenProteccion lo implementa el almacén real; un fake que no lo tenga se salta la
// protección (no tiene transacción donde cumplirla).
type almacenProteccion interface {
	ProteccionIniciada(ctx context.Context, docID string) (bool, error)
	IniciarProteccion(ctx context.Context, docID string, est EstadoProteccion) error
}

// almacenSecuencia lo implementa el almacén real: establece la copia de la secuencia de
// un documento con secciones (proteccion_secuencia.go).
type almacenSecuencia interface {
	IniciarSecuencia(ctx context.Context, docID string, st *State) error
}

// iniciarSecuencia reconstruye, desde el estado completo, la copia de la secuencia de un
// documento con secciones cuyo propietario las añadió sin que el servicio la tuviera.
func (s *Service) iniciarSecuencia(ctx context.Context, doc *Document) error {
	ss, ok := s.store.(almacenSecuencia)
	if !ok {
		return ErrSecuenciaPendiente
	}
	st, _, err := s.load(ctx, doc.ID, s.Hidratar)
	if err != nil {
		return err
	}
	return ss.IniciarSecuencia(ctx, doc.ID, st)
}

// iniciarProteccion deduce, una sola vez por documento, el estado de protección del
// contenido que ya tiene (también migra las contraseñas en claro de antes).
func (s *Service) iniciarProteccion(ctx context.Context, doc *Document, ops []Op) error {
	ps, ok := s.store.(almacenProteccion)
	if !ok || !ProtegeEscritura(doc.Kind, ops) || s.iniciados.hay(doc.ID) {
		return nil
	}
	hecho, err := ps.ProteccionIniciada(ctx, doc.ID)
	if err != nil {
		return err
	}
	if !hecho {
		st, _, err := s.load(ctx, doc.ID, s.Hidratar)
		if err != nil {
			return err
		}
		est, err := EstadoInicialProteccion(doc.Kind, st)
		if err != nil {
			return err
		}
		if err := ps.IniciarProteccion(ctx, doc.ID, est); err != nil {
			return err
		}
	}
	s.iniciados.pon(doc.ID)
	return nil
}

// ProtegeEscritura: ¿este lote, en un documento de este tipo, pasa por la protección?
func ProtegeEscritura(kind Kind, ops []Op) bool {
	switch kind {
	case KindSpreadsheet:
		return TocaModelo(ops)
	case KindDocument:
		return TocaHTML(ops) || TocaSecuencia(ops)
	case KindPresentation:
		return TocaDiapos(ops)
	}
	return false
}

// EstadoInicialProteccion deduce el estado de protección de un documento ya existente.
func EstadoInicialProteccion(kind Kind, st *State) (EstadoProteccion, error) {
	switch kind {
	case KindDocument:
		return EstadoInicialSecciones(st)
	case KindPresentation:
		return EstadoInicialDiapos(st)
	case KindSpreadsheet:
		return EstadoInicialHoja(st)
	}
	return EstadoInicialHoja(st)
}

// iniciadosDocs recuerda, por proceso, qué documentos ya tienen su estado de protección
// guardado: la fila no se borra nunca (cae con el documento), así que preguntarlo una
// vez basta y los guardados siguientes se ahorran un viaje.
type iniciadosDocs struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func (c *iniciadosDocs) hay(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.ids[id]
	return ok
}

func (c *iniciadosDocs) pon(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil || len(c.ids) > 50_000 { // ponytail: acotado a lo bruto; se vuelve a preguntar una vez por documento
		c.ids = map[string]struct{}{}
	}
	c.ids[id] = struct{}{}
}

// LoadFull carga el estado CRDT completo (requiere lectura). Para snapshots en
// S3, hydrate descarga el estado base; pásalo nil si no hay objectstore.
func (s *Service) LoadFull(ctx context.Context, subj Subject, docID string, hydrate func(ctx context.Context, blobKey string) (*State, error)) (*State, int64, error) {
	if _, _, err := s.Authorize(ctx, docID, subj, RoleViewer); err != nil {
		return nil, 0, err
	}
	st, seq, err := s.load(ctx, docID, hydrate)
	if err != nil {
		return nil, 0, err
	}
	RedactarEstado(st) // las contraseñas heredadas en claro no salen del servicio
	return st, seq, nil
}

// LoadShared carga el estado de un documento que se abre por ENLACE, sin sesión.
// La autorización es la ShareResolution, que solo sale de ResolveShare con un
// token vigente; el contexto debe llevar ya el tenant del documento (RLS).
func (s *Service) LoadShared(ctx context.Context, res *ShareResolution, hydrate func(ctx context.Context, blobKey string) (*State, error)) (*State, error) {
	st, _, err := s.load(ctx, res.DocumentID, hydrate)
	RedactarEstado(st)
	return st, err
}

// load es la carga del estado sin autorizar: la hacen LoadFull y LoadShared.
func (s *Service) load(ctx context.Context, docID string, hydrate func(ctx context.Context, blobKey string) (*State, error)) (*State, int64, error) {
	loaded, err := s.store.LoadState(ctx, docID)
	if err != nil {
		return nil, 0, err
	}
	if loaded.BlobKey != "" {
		if hydrate == nil {
			return nil, 0, fmt.Errorf("docs: snapshot en S3 sin hidratador")
		}
		base, err := hydrate(ctx, loaded.BlobKey)
		if err != nil {
			return nil, 0, err
		}
		// Aplicar ops posteriores al snapshot S3.
		more, err := s.store.OpsSince(ctx, docID, loaded.BaseSeq, 0)
		if err != nil {
			return nil, 0, err
		}
		for _, so := range more {
			if _, err := base.Apply(so.Op); err != nil {
				return nil, 0, err
			}
		}
		base.Seq = loaded.Seq
		return base, loaded.Seq, nil
	}
	return loaded.State, loaded.Seq, nil
}

// Snapshot consolida el estado actual y registra una versión (requiere editor).
// Decide inline vs S3 según el tamaño (inlineLimit bytes); para S3 usa upload.
func (s *Service) Snapshot(ctx context.Context, subj Subject, docID string, label *string, auto bool, inlineLimit int, hydrate func(ctx context.Context, blobKey string) (*State, error), upload func(ctx context.Context, key string, data []byte) error, keyFor func(docID string, seq int64) string) (*Version, error) {
	if _, _, err := s.Authorize(ctx, docID, subj, RoleEditor); err != nil {
		return nil, err
	}
	// Sin redactar: la instantánea es del servicio y debe conservar lo que el documento
	// tiene (también una contraseña heredada aún sin migrar).
	state, seq, err := s.load(ctx, docID, hydrate)
	if err != nil {
		return nil, err
	}
	raw, err := state.JSON()
	if err != nil {
		return nil, err
	}

	p := SaveSnapshotParams{DocumentID: docID, Seq: seq, SizeBytes: int64(len(raw))}
	if inlineLimit > 0 && len(raw) > inlineLimit && upload != nil && keyFor != nil {
		key := keyFor(docID, seq)
		if err := upload(ctx, key, raw); err != nil {
			return nil, err
		}
		p.BlobKey = key
	} else {
		p.State = raw
	}

	snapID, err := s.store.SaveSnapshot(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.store.CreateVersion(ctx, docID, snapID, subj.Sub, seq, label, auto)
}

// RestoreResult describe el resultado de restaurar una versión.
type RestoreResult struct {
	Version *Version      // la versión restaurada
	Ops     []SequencedOp // operaciones de restauración secuenciadas
	Seq     int64         // nueva seq del documento tras la restauración
	Changed bool          // false si el documento ya coincidía con la versión
}

// RestoreVersion restaura el documento al contenido de una versión histórica de
// forma NO DESTRUCTIVA y coherente con el modelo append-only del CRDT: en lugar
// de reescribir el log, calcula el diff entre el estado actual y el estado de la
// versión (Diff) y lo AÑADE como operaciones nuevas. El historial completo se
// conserva; una restauración es reversible restaurando la versión previa.
//
// Requiere rol editor. Devuelve las operaciones secuenciadas para difundirlas a
// los colaboradores conectados (misma vía que una edición normal).
func (s *Service) RestoreVersion(ctx context.Context, subj Subject, docID, versionID string, hydrate func(ctx context.Context, blobKey string) (*State, error)) (*RestoreResult, error) {
	doc, _, err := s.Authorize(ctx, docID, subj, RoleEditor)
	if err != nil {
		return nil, err
	}
	version, err := s.store.GetVersion(ctx, docID, versionID)
	if err != nil {
		return nil, err
	}
	// Estado destino: el consolidado en el snapshot de la versión.
	target, _, err := s.store.SnapshotState(ctx, version.SnapshotID, hydrate)
	if err != nil {
		return nil, err
	}
	// Estado actual del documento.
	current, curSeq, err := s.load(ctx, docID, hydrate)
	if err != nil {
		return nil, err
	}

	// Actor de restauración: único por operación (deriva de un token aleatorio)
	// para que sus Dots no colisionen jamás con los de una sesión de edición.
	tok, err := NewToken()
	if err != nil {
		return nil, err
	}
	actor := "restore:" + tok[:16]
	ops := current.Diff(target, actor, 0, current.Lamport)
	for i := range ops {
		// Una versión anterior a la protección en servidor puede traer una contraseña
		// en claro: restaurarla no debe contar como «traer la contraseña» ni fijarla.
		if ops[i].Kind == OpSet && ops[i].Key == claveHoja {
			ops[i].Value = RedactarValor(ops[i].Value)
		}
	}
	if len(ops) == 0 {
		return &RestoreResult{Version: version, Seq: curSeq, Changed: false}, nil
	}

	// La restauración escribe por la misma puerta que cualquier edición: restaurar una
	// versión sin protección sobre una hoja protegida exige ser propietario o traer la
	// contraseña, como quitarla a mano.
	seqd, newSeq, err := s.appendProtegido(ctx, doc, subj, ops, nil)
	if err != nil {
		return nil, err
	}
	return &RestoreResult{Version: version, Ops: seqd, Seq: newSeq, Changed: true}, nil
}

// ResolveShare resuelve un token de enlace a (documento, tenant, rol) si está
// activo. Devuelve ErrShareExpired si no procede.
func (s *Service) ResolveShare(ctx context.Context, token string) (*ShareResolution, error) {
	resolution, err := s.store.ResolveShareToken(ctx, HashToken(token))
	if err != nil {
		return nil, err
	}
	if resolution == nil || !validShareRole(resolution.Role) {
		return nil, ErrShareExpired
	}
	return resolution, nil
}

// CreateShare crea un enlace de compartición generando un token opaco seguro.
// Devuelve el ShareLink con el token EN CLARO (única vez que se expone).
//
// `password` es opcional: con ella, el enlace exige además esa contraseña. Se guarda solo
// su hash (Argon2id con sal) y no se devuelve.
func (s *Service) CreateShare(ctx context.Context, subj Subject, docID string, role Role, expiresAt *time.Time, password string) (*ShareLink, error) {
	if !validShareRole(role) {
		return nil, ErrInvalidRole // los enlaces no conceden ownership
	}
	if _, _, err := s.Authorize(ctx, docID, subj, RoleOwner); err != nil {
		return nil, err
	}
	// El techo de caducidad de la organización se aplica AQUÍ, no en la interfaz ni en
	// el BFF: una llamada directa a la API pasa por el mismo sitio.
	politica, err := s.store.PoliticaEnlaces(ctx)
	if err != nil {
		return nil, err
	}
	expiresAt, err = CaducidadDeEnlace(time.Now(), expiresAt, politica.MaxDias)
	if err != nil {
		return nil, err
	}
	token, err := NewToken()
	if err != nil {
		return nil, err
	}
	hash, err := HashClaveDeEnlace(password)
	if err != nil {
		return nil, err
	}
	link, err := s.store.CreateShareLink(ctx, docID, HashToken(token), hash, subj.Sub, role, expiresAt)
	if err != nil {
		return nil, err
	}
	link.Token = token
	return link, nil
}

// contains indica si s contiene v.
func contains(ss []string, v string) bool {
	for _, x := range ss {
		if x == v {
			return true
		}
	}
	return false
}

// NewToken genera un token opaco de 256 bits codificado en base32hex-like (hex).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("docs: generar token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken devuelve el sha-256 hex de un token (lo que se persiste).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
