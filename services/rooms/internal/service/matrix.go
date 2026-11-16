// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type MatrixBootstrap struct {
	SourceURL              string                  `json:"source_url,omitempty"`
	DraftRoomID            string                  `json:"draft_room_id"`
	DraftRoomIDs           []string                `json:"draft_room_ids"`
	StorageIdentityVersion storage.IdentityVersion `json:"storage_identity_version,omitempty"`
	LegacyStorage          *MatrixBootstrap        `json:"legacy_storage,omitempty"`
	StorageProduct         string                  `json:"storage_product,omitempty"`
	StorageRoomID          string                  `json:"storage_room_id,omitempty"`
	StorageRoomIDs         []string                `json:"storage_room_ids,omitempty"`
	Actor                  string                  `json:"actor"`
	Tenant                 string                  `json:"tenant"`
	HomeserverURL          string                  `json:"homeserver_url"`
	SSOProviderID          string                  `json:"sso_provider_id"`
	ControllerUserID       string                  `json:"controller_user_id"`
	MatrixUserID           string                  `json:"matrix_user_id"`
	Channels               []store.MatrixChannel   `json:"channels"`
	Encryption             string                  `json:"encryption"`
}

func (s *Service) MatrixReady(ctx context.Context) error {
	if !s.cfg.MatrixRequired && s.cfg.MatrixInternalURL == "" {
		return nil
	}
	if s.cfg.MatrixLifecycleSecret == "" || s.cfg.MatrixIdentitySecret == "" {
		return domain.ErrMatrixUnavailable
	}
	return s.matrix.Ready(ctx)
}

func (s *Service) MatrixAccountAccess(ctx context.Context, subject, status string, version int64) error {
	if s.matrix == nil || s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	if version <= 0 || (status != "active" && status != "suspended" && status != "deleted") {
		return domain.ErrInvalidInput
	}
	if _, err := s.matrix.UserID(subject); err != nil {
		return domain.ErrInvalidInput
	}
	return s.pstore.ApplyMatrixAccountAccess(ctx, subject, status, version, func(ctx context.Context) error {
		if status == "active" {
			if _, _, err := s.matrixIdentity(ctx, subject); err != nil {
				return err
			}
		}
		return s.matrix.SetAccountAccess(ctx, subject, status)
	})
}

func (s *Service) MatrixTenantAccess(ctx context.Context, subject, tenant, status string, version int64) error {
	if s.matrix == nil || s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	t, err := tenancy.New(tenant)
	if err != nil || version <= 0 || (status != "active" && status != "removed") {
		return domain.ErrInvalidInput
	}
	if _, err := s.matrix.UserID(subject); err != nil {
		return domain.ErrInvalidInput
	}
	return s.pstore.ApplyMatrixTenantAccess(tenancy.Into(ctx, t), subject, status, version, func(ctx context.Context) error {
		orgs, personal, err := s.matrixIdentity(ctx, subject)
		if err != nil {
			return err
		}
		if !slices.Contains(orgs, t.Slug) && (!personal || !t.Personal()) {
			return domain.ErrForbidden
		}
		return nil
	})
}

func (s *Service) EditMatrixMessage(ctx context.Context, id uuid.UUID, actor, body string, ref domain.MatrixEventRef) (domain.Message, error) {
	if body != "" {
		return domain.Message{}, domain.ErrEncryptedRequired
	}
	if s.matrix == nil || s.pstore == nil {
		return domain.Message{}, domain.ErrMatrixUnavailable
	}
	if _, _, err := s.matrixIdentity(ctx, actor); err != nil {
		return domain.Message{}, err
	}
	msg, err := s.pstore.EditMatrixMessage(ctx, id, actor, ref, func(ctx context.Context, state store.MatrixChannel) error {
		if !slices.Contains(state.Subjects, actor) {
			return domain.ErrForbidden
		}
		if err := s.matrix.Reconcile(ctx, state.RoomID, state.Subjects, state.ReadOnly, true); err != nil {
			return err
		}
		return s.matrix.ValidateEvent(ctx, ref, actor)
	})
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(ctx, EventMessageEdited, msg.ChannelID, msg)
	return msg, nil
}

func (s *Service) matrixIdentity(ctx context.Context, subject string) ([]string, bool, error) {
	orgs, personal, err := peticiones.OrganizacionesDe(ctx, s.cfg.IDURL, s.cfg.MatrixIdentitySecret, subject)
	if err != nil {
		return nil, false, domain.ErrMatrixUnavailable
	}
	if len(orgs) == 0 && !personal {
		return nil, false, domain.ErrForbidden
	}
	return orgs, personal, nil
}

func (s *Service) BootstrapMatrix(ctx context.Context, actor, name string) (MatrixBootstrap, error) {
	result, err := s.matrixBootstrapIdentity(ctx, actor)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	tenant := tenancy.MustFrom(ctx)
	if _, err := s.EnsureDefaultTeams(ctx, actor, name); err != nil {
		return MatrixBootstrap{}, err
	}
	channels, err := s.bootstrapMatrixChannels(ctx, actor)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	drafts, err := s.pstore.EnsureMatrixDraftRooms(ctx, actor, func(ctx context.Context, room string) (string, error) {
		return s.matrix.EnsureDraftRoom(ctx, tenant.Slug, actor, room)
	})
	if err != nil {
		return MatrixBootstrap{}, err
	}
	result.Channels, result.DraftRoomIDs = channels, drafts
	if len(drafts) > 0 {
		result.DraftRoomID = drafts[0]
	}
	return result, nil
}

// BootstrapMatrixStorage does not create teams or expose another application's
// rooms. The application receives routing references; secrets stay on devices.
func (s *Service) BootstrapMatrixStorage(ctx context.Context, actor, product string) (MatrixBootstrap, error) {
	return s.BootstrapMatrixStorageForIdentity(ctx, actor, product, storage.LegacyIdentity)
}

func (s *Service) BootstrapMatrixStorageForIdentity(ctx context.Context, actor, product string, version storage.IdentityVersion) (MatrixBootstrap, error) {
	version = version.Normalized()
	if !domain.StorageProduct(product) || !version.Valid() {
		return MatrixBootstrap{}, domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixBootstrap{}, err
	}
	scope := storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: actor, Product: product}
	result, err := s.matrixStorageIdentity(ctx, scope, version)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	rooms, err := s.pstore.EnsureMatrixStorageRoomsWithPolicy(ctx, actor, product, version, func(ctx context.Context, room string, readOnly bool) (string, error) {
		if version == storage.IsolatedIdentity {
			return s.matrix.EnsureIsolatedStorageRoom(ctx, scope, room)
		}
		return s.matrix.EnsureStorageRoomPolicy(ctx, scope.Tenant, actor, product, room, readOnly)
	})
	if err != nil {
		return MatrixBootstrap{}, err
	}
	result.StorageRoomIDs = rooms
	if len(rooms) > 0 {
		result.StorageRoomID = rooms[0]
	}
	if version == storage.IsolatedIdentity {
		legacy, err := s.pstore.LegacyMatrixStorageRooms(ctx, actor, product)
		if err != nil {
			return MatrixBootstrap{}, err
		}
		if len(legacy) > 0 {
			mxid, err := s.matrix.UserID(actor)
			if err != nil {
				return MatrixBootstrap{}, err
			}
			previous := s.matrixBootstrapMetadata(actor, scope.Tenant, mxid, s.cfg.MatrixSSOProviderID)
			previous.StorageProduct, previous.StorageIdentityVersion = product, storage.LegacyIdentity
			previous.StorageRoomIDs, previous.StorageRoomID = legacy, legacy[0]
			result.LegacyStorage = &previous
		}
	}
	return result, nil
}

func (s *Service) matrixStorageIdentity(ctx context.Context, scope storage.Scope, version storage.IdentityVersion) (MatrixBootstrap, error) {
	if version == storage.LegacyIdentity {
		mxid, err := s.matrix.EnsureUser(ctx, scope.Subject)
		if err != nil {
			return MatrixBootstrap{}, err
		}
		result := s.matrixBootstrapMetadata(scope.Subject, scope.Tenant, mxid, s.cfg.MatrixSSOProviderID)
		result.StorageProduct, result.StorageIdentityVersion = scope.Product, version
		return result, nil
	}
	mxid, err := s.matrix.EnsureStorageUser(ctx, scope)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	provider, err := s.matrix.StorageSSOProviderID(scope.Product)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	result := s.matrixBootstrapMetadata(scope.Subject, scope.Tenant, mxid, provider)
	result.StorageProduct, result.StorageIdentityVersion = scope.Product, version
	return result, nil
}

func (s *Service) matrixBootstrapIdentity(ctx context.Context, actor string) (MatrixBootstrap, error) {
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixBootstrap{}, err
	}
	tenant := tenancy.MustFrom(ctx)
	mxid, err := s.matrix.EnsureUser(ctx, actor)
	if err != nil {
		return MatrixBootstrap{}, err
	}
	return s.matrixBootstrapMetadata(actor, tenant.Slug, mxid, s.cfg.MatrixSSOProviderID), nil
}

func (s *Service) matrixBootstrapMetadata(actor, tenant, mxid, provider string) MatrixBootstrap {
	return MatrixBootstrap{SourceURL: s.cfg.MatrixSourceURL, Actor: actor, Tenant: tenant, HomeserverURL: s.cfg.MatrixHomeserverURL, SSOProviderID: provider, MatrixUserID: mxid, ControllerUserID: "@" + domain.MatrixControllerLocalpart + ":" + s.cfg.MatrixServerName, Encryption: domain.MatrixEncryption, Channels: []store.MatrixChannel{}}
}

func (s *Service) bootstrapMatrixChannels(ctx context.Context, actor string) ([]store.MatrixChannel, error) {
	tenant := tenancy.MustFrom(ctx)
	channels := []store.MatrixChannel{}
	ids, err := s.pstore.MatrixChannelIDs(ctx, actor)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		binding, err := s.ensureMatrixBinding(ctx, id, actor)
		if err != nil {
			return nil, err
		}
		channels = append(channels, binding)
	}
	external, _, err := s.pstore.ListExternalChannels(ctx, actor, tenant.Slug)
	if err != nil {
		return nil, err
	}
	for _, channel := range external {
		host, err := tenancy.New(channel.Tenant)
		if err != nil {
			return nil, domain.ErrMatrixUnavailable
		}
		binding, err := s.ensureMatrixBinding(tenancy.Into(ctx, host), channel.ID, actor)
		if err != nil {
			return nil, err
		}
		channels = append(channels, binding)
	}
	return channels, nil
}

func (s *Service) ensureMatrixBinding(ctx context.Context, id uuid.UUID, actor string) (store.MatrixChannel, error) {
	tenant := tenancy.MustFrom(ctx)
	return s.pstore.EnsureMatrixChannel(ctx, id, actor, func(ctx context.Context, state store.MatrixChannel) (string, error) {
		return s.matrix.EnsureRoom(ctx, tenant.Slug, id.String(), state.RoomID, state.Subjects, state.ReadOnly)
	})
}

func (s *Service) postMatrixMessage(ctx context.Context, in PostMessageInput) (domain.Message, error) {
	if s.matrix == nil || s.pstore == nil {
		return domain.Message{}, domain.ErrMatrixUnavailable
	}
	if in.Body != "" || len(in.Mentions) != 0 || len(in.FileIDs) != 0 || in.Card != nil || in.Type != "" && in.Type != domain.MessageTypeUser {
		return domain.Message{}, domain.ErrEncryptedRequired
	}
	if _, _, err := s.matrixIdentity(ctx, in.AuthorSub); err != nil {
		return domain.Message{}, err
	}
	ref := *in.MatrixEvent
	msg, err := s.pstore.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: in.ChannelID, ParentID: in.ParentID, AuthorSub: in.AuthorSub, AuthorName: in.AuthorName, MatrixEvent: &ref,
		// La marca es metadato del servicio, no contenido: con cifrado también vale,
		// porque el feed solo lleva la referencia al evento y el cliente lo descifra.
		AlsoInChannel: in.ParentID != nil && in.AlsoInChannel,
		ValidateMatrix: func(ctx context.Context, state store.MatrixChannel) error {
			if !slices.Contains(state.Subjects, in.AuthorSub) {
				return domain.ErrForbidden
			}
			if err := s.matrix.Reconcile(ctx, state.RoomID, state.Subjects, state.ReadOnly, true); err != nil {
				return err
			}
			return s.matrix.ValidateEvent(ctx, ref, in.AuthorSub)
		},
	})
	if err != nil {
		return domain.Message{}, err
	}
	s.publish(ctx, EventMessageCreated, msg.ChannelID, msg)
	return msg, nil
}
