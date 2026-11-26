// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const MaxRowBytes = 32 << 20

var (
	ErrSnapshot = errors.New("photos: invalid or incomplete retained catalog")
	ErrShared   = errors.New("photos: shared albums require encrypted recipient access")
)

// Row retains every original column, including vectors and unknown fields.
// UI projections are never used as migration input.
type Row struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	JSON []byte `json:"json"`
}

// AssetRow describes the stored metadata, before EXIF orientation is applied.
// JSON is retained separately so reading a view cannot lose a column.
type AssetRow struct {
	ID             string     `json:"id"`
	Tenant         string     `json:"tenant"`
	Owner          string     `json:"owner_sub"`
	DriveID        string     `json:"drive_file_id"`
	Name           string     `json:"name"`
	MediaType      string     `json:"media_type"`
	Bytes          int64      `json:"bytes"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	Orientation    int        `json:"orientation"`
	TakenAt        *time.Time `json:"taken_at"`
	UploadedAt     time.Time  `json:"uploaded_at"`
	Camera         string     `json:"camera"`
	Lens           string     `json:"lens"`
	Favorite       bool       `json:"favorite"`
	Description    string     `json:"description"`
	Tags           []string   `json:"tags"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	Embedding      []float32  `json:"embedding"`
	EmbeddingModel string     `json:"embedding_model"`
	DeletedAt      *time.Time `json:"deleted_at"`
	// PurgedAt es cuándo se borró para siempre de la papelera (Purgar): la fila sigue
	// en el historial del motor, que no tiene recolector, pero la galería ya no la ve.
	PurgedAt     *time.Time      `json:"purged_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	PrivateMedia json.RawMessage `json:"private_media,omitempty"`
	// PrivateExternal es un original que vive FUERA del almacén (un fichero de una
	// carpeta del equipo enlazado sin copiarlo, FO-09), en vez de PrivateMedia. Solo lo
	// escribe Sytue Office.
	PrivateExternal json.RawMessage `json:"private_external,omitempty"`
	// DurationMs es la duración de un vídeo, en milisegundos (FO-09).
	DurationMs int64 `json:"duration_ms,omitempty"`
	// OriginalSHA256 es la huella del original de una foto purgada que soltó su
	// original (Repository.LiberarAlPurgar): así no se vuelve a importar lo que la
	// persona borró para siempre.
	OriginalSHA256 string `json:"original_sha256,omitempty"`
}

func parsedUUID(value string) uuid.UUID { id, _ := uuid.Parse(value); return id }

func (a AssetRow) Activo() Activo {
	return Activo{
		ID: parsedUUID(a.ID), Tenant: a.Tenant, Duenyo: a.Owner,
		DriveID: a.DriveID, Nombre: a.Name, Tipo: a.MediaType, Bytes: a.Bytes,
		Ancho: a.Width, Alto: a.Height, Orientacion: OrientacionDe(a.Orientation),
		TomadaEn: a.TakenAt, SubidaEn: a.UploadedAt, Camara: a.Camera, Objetivo: a.Lens,
		Favorita: a.Favorite, Descripcion: a.Description, Etiquetas: slices.Clone(a.Tags),
		Lat: a.Latitude, Lon: a.Longitude,
	}
}

type AlbumRow struct {
	ID              string          `json:"id"`
	Tenant          string          `json:"tenant"`
	Owner           string          `json:"owner_sub"`
	Title           string          `json:"title"`
	CoverAssetID    *string         `json:"cover_asset_id"`
	PublicToken     string          `json:"public_token"`
	PublicTokenHash string          `json:"public_token_hash"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	DeletedAt       *time.Time      `json:"deleted_at,omitempty"`
	PrivateMembers  json.RawMessage `json:"private_members,omitempty"`
	// Origin es de dónde vino el álbum, si vino de fuera («sytue:<id>», el de un álbum
	// traído de una copia de «Tus datos» a Sytue Office): así traerlo otra vez lo
	// reconoce aunque se haya renombrado. Vacío en los demás (y siempre en la conectada,
	// que nunca lo escribe).
	Origin string `json:"origin,omitempty"`
}

type EditRow struct {
	ID          string `json:"id"`
	Tenant      string `json:"tenant"`
	Owner       string `json:"owner_sub"`
	AssetID     string `json:"asset_id"`
	Recipe      Receta `json:"recipe"`
	Fingerprint string `json:"fingerprint"`
}

type MembershipRow struct {
	Tenant   string    `json:"tenant"`
	AlbumID  string    `json:"album_id"`
	AssetID  string    `json:"asset_id"`
	Position int       `json:"position"`
	AddedAt  time.Time `json:"added_at"`
}

type AlbumPersonRow struct {
	Tenant    string `json:"tenant"`
	AlbumID   string `json:"album_id"`
	Subject   string `json:"sujeto"`
	GrantedBy string `json:"concedido_por"`
	CanEdit   bool   `json:"puede_editar"`
}

// Original is a relationship to Drive, never an assertion that its bytes have
// already been imported. Media verification must finish before private readiness.
type Original struct {
	AssetID     string
	DriveID     string
	Size        int64
	ContentType string
}

type Snapshot struct {
	scope   storage.Scope
	rows    map[string]Row
	assets  map[string]AssetRow
	albums  map[string]AlbumRow
	edits   map[string]EditRow
	members map[string]MembershipRow
	people  map[string]AlbumPersonRow
}

func SourceKind(kind string) bool {
	switch kind {
	case "asset", "album", "edit", "album_asset", "album_person", "media":
		return true
	default:
		return false
	}
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func compositeID(album, other string) (string, error) {
	if !canonicalUUID(album) || other == "" || !utf8.ValidString(other) {
		return "", ErrSnapshot
	}
	sum := sha256.Sum256([]byte(album + ":" + other))
	return hex.EncodeToString(sum[:]), nil
}

func MembershipID(album, asset string) (string, error) {
	if !canonicalUUID(asset) {
		return "", ErrSnapshot
	}
	return compositeID(album, asset)
}

func AlbumPersonID(album, subject string) (string, error) { return compositeID(album, subject) }

func ValidateRow(scope storage.Scope, row Row) error {
	var header struct {
		ID     string `json:"id"`
		Tenant string `json:"tenant"`
	}
	if !scope.Valid() || scope.Product != "photos" || !SourceKind(row.Kind) || len(row.JSON) < 2 || len(row.JSON) > MaxRowBytes || json.Unmarshal(row.JSON, &header) != nil || header.Tenant != scope.Tenant {
		return ErrSnapshot
	}
	if row.Kind != "album_asset" && row.Kind != "album_person" && row.Kind != "media" && (!canonicalUUID(row.ID) || header.ID != row.ID) {
		return ErrSnapshot
	}

	switch row.Kind {
	case "asset":
		return validateAssetRow(row.JSON)
	case "media":
		return validateMediaRow(row)
	case "album":
		return validateAlbumRow(row.JSON)
	case "edit":
		return validateEditRow(row.JSON)
	case "album_asset":
		return validateMembershipRow(row)
	case "album_person":
		return validateAlbumPersonRow(row)
	default:
		return ErrSnapshot
	}
}

func validateMediaRow(row Row) error {
	var m MediaRow
	if json.Unmarshal(row.JSON, &m) != nil || m.ID != row.ID || m.Owner == "" || m.Validate() != nil {
		return ErrSnapshot
	}
	return nil
}

func validateAlbumRow(raw []byte) error {
	var a AlbumRow
	if json.Unmarshal(raw, &a) != nil || a.Owner == "" || a.Title == "" || (a.CoverAssetID != nil && !canonicalUUID(*a.CoverAssetID)) {
		return ErrSnapshot
	}
	return nil
}

func validateEditRow(raw []byte) error {
	var e EditRow
	if json.Unmarshal(raw, &e) != nil || e.Owner == "" || !canonicalUUID(e.AssetID) || e.Recipe.Valida() != nil {
		return ErrSnapshot
	}
	fingerprint, err := e.Recipe.Huella()
	if err != nil || e.Fingerprint != fingerprint {
		return ErrSnapshot
	}
	return nil
}

func validateMembershipRow(row Row) error {
	var m MembershipRow
	if json.Unmarshal(row.JSON, &m) != nil {
		return ErrSnapshot
	}
	id, err := MembershipID(m.AlbumID, m.AssetID)
	if err != nil || id != row.ID {
		return ErrSnapshot
	}
	return nil
}

func validateAlbumPersonRow(row Row) error {
	var p AlbumPersonRow
	if json.Unmarshal(row.JSON, &p) != nil || p.GrantedBy == "" {
		return ErrSnapshot
	}
	id, err := AlbumPersonID(p.AlbumID, p.Subject)
	if err != nil || id != row.ID {
		return ErrSnapshot
	}
	return nil
}

func validateAssetRow(raw []byte) error {
	var a AssetRow
	if json.Unmarshal(raw, &a) != nil || a.Owner == "" || a.DriveID == "" || a.MediaType == "" || a.Bytes < 0 || a.Width <= 0 || a.Height <= 0 || a.UploadedAt.IsZero() || (a.Latitude == nil) != (a.Longitude == nil) || (a.Embedding == nil) != (a.EmbeddingModel == "") {
		return ErrSnapshot
	}
	if a.Latitude != nil && (math.IsNaN(*a.Latitude) || math.IsInf(*a.Latitude, 0) || math.IsNaN(*a.Longitude) || math.IsInf(*a.Longitude, 0) || *a.Latitude < -90 || *a.Latitude > 90 || *a.Longitude < -180 || *a.Longitude > 180) {
		return ErrSnapshot
	}
	return nil
}

func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	if !scope.Valid() || scope.Product != "photos" {
		return nil, ErrSnapshot
	}
	s := &Snapshot{scope: scope, rows: map[string]Row{}, assets: map[string]AssetRow{}, albums: map[string]AlbumRow{}, edits: map[string]EditRow{}, members: map[string]MembershipRow{}, people: map[string]AlbumPersonRow{}}
	for _, row := range rows {
		if err := ValidateRow(scope, row); err != nil {
			return nil, err
		}
		key := row.Kind + "\x00" + row.ID
		if _, exists := s.rows[key]; exists {
			return nil, ErrSnapshot
		}
		row.JSON = slices.Clone(row.JSON)
		s.rows[key] = row
		s.decode(row)
	}
	if err := s.validateGraph(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Snapshot) decode(row Row) {
	switch row.Kind {
	case "asset":
		var value AssetRow
		_ = json.Unmarshal(row.JSON, &value)
		s.assets[row.ID] = value
	case "album":
		var value AlbumRow
		_ = json.Unmarshal(row.JSON, &value)
		s.albums[row.ID] = value
	case "edit":
		var value EditRow
		_ = json.Unmarshal(row.JSON, &value)
		s.edits[row.ID] = value
	case "album_asset":
		var value MembershipRow
		_ = json.Unmarshal(row.JSON, &value)
		s.members[row.ID] = value
	case "album_person":
		var value AlbumPersonRow
		_ = json.Unmarshal(row.JSON, &value)
		s.people[row.ID] = value
	}
}

func (s *Snapshot) validateGraph() error {
	drive := map[string]bool{}
	edits := map[string]bool{}
	for _, a := range s.assets {
		if drive[a.DriveID] {
			return ErrSnapshot
		}
		drive[a.DriveID] = true
	}
	for _, a := range s.albums {
		if a.CoverAssetID != nil {
			if _, exists := s.assets[*a.CoverAssetID]; !exists {
				return ErrSnapshot
			}
		}
	}
	for _, e := range s.edits {
		a, exists := s.assets[e.AssetID]
		key := e.AssetID + "\x00" + e.Owner
		if !exists || a.ID == "" || edits[key] {
			return ErrSnapshot
		}
		edits[key] = true
	}
	for _, m := range s.members {
		if _, exists := s.assets[m.AssetID]; !exists {
			return ErrSnapshot
		}
		if _, exists := s.albums[m.AlbumID]; !exists {
			return ErrSnapshot
		}
	}
	for _, p := range s.people {
		if _, exists := s.albums[p.AlbumID]; !exists {
			return ErrSnapshot
		}
	}
	for _, m := range s.Media() {
		asset, exists := s.assets[m.AssetID]
		if !exists || m.Owner != asset.Owner {
			return ErrSnapshot
		}
	}
	return nil
}

// PersonalCompatible is a preflight guard. Capture must call this before taking
// a write fence; receiving or granting a share cannot be silently discarded.
func (s *Snapshot) PersonalCompatible() error {
	if len(s.people) != 0 {
		return ErrShared
	}
	for _, a := range s.assets {
		if a.Owner != s.scope.Subject {
			return ErrShared
		}
	}
	for _, a := range s.albums {
		if a.Owner != s.scope.Subject || a.PublicToken != "" || a.PublicTokenHash != "" {
			return ErrShared
		}
	}
	for _, e := range s.edits {
		if e.Owner != s.scope.Subject {
			return ErrShared
		}
	}
	return nil
}

func (s *Snapshot) Rows() []Row {
	result := make([]Row, 0, len(s.rows))
	for _, row := range s.rows {
		row.JSON = slices.Clone(row.JSON)
		result = append(result, row)
	}
	slices.SortFunc(result, func(a, b Row) int {
		if compared := strings.Compare(a.Kind, b.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(a.ID, b.ID)
	})
	return result
}

func (s *Snapshot) Originals() []Original {
	result := make([]Original, 0, len(s.assets))
	for _, a := range s.assets {
		result = append(result, Original{AssetID: a.ID, DriveID: a.DriveID, Size: a.Bytes, ContentType: a.MediaType})
	}
	slices.SortFunc(result, func(a, b Original) int { return strings.Compare(a.AssetID, b.AssetID) })
	return result
}
