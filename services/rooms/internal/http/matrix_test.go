// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// Matrix endpoints use a dedicated fake below; legacy handler fixtures cannot
// accidentally mark an unconfigured encrypted transport as ready.
func (*fakeService) BootstrapMatrix(context.Context, string, string) (service.MatrixBootstrap, error) {
	return service.MatrixBootstrap{}, domain.ErrMatrixUnavailable
}

func (*fakeService) BootstrapMatrixStorageForIdentity(context.Context, string, string, storage.IdentityVersion) (service.MatrixBootstrap, error) {
	return service.MatrixBootstrap{}, domain.ErrMatrixUnavailable
}

func (*fakeService) MatrixRecoveryKey(context.Context, string) (string, error) {
	return "", domain.ErrMatrixUnavailable
}

func (*fakeService) StoreMatrixRecoveryKey(context.Context, string, string) error {
	return domain.ErrMatrixUnavailable
}

func (*fakeService) MatrixRecoveryWrappers(context.Context, string, service.MatrixRecoveryAccount) (service.MatrixRecoveryWrappers, error) {
	return service.MatrixRecoveryWrappers{}, domain.ErrMatrixUnavailable
}

func (*fakeService) PutMatrixRecoveryWrapper(context.Context, string, service.MatrixRecoveryAccount, recoverywrap.Wrapper) (string, error) {
	return "", domain.ErrMatrixUnavailable
}

func (*fakeService) MatrixAccountAccess(context.Context, string, string, int64) error {
	return domain.ErrMatrixUnavailable
}

func (*fakeService) MatrixTenantAccess(context.Context, string, string, string, int64) error {
	return domain.ErrMatrixUnavailable
}

func (*fakeService) EditMatrixMessage(context.Context, uuid.UUID, string, string, domain.MatrixEventRef) (domain.Message, error) {
	return domain.Message{}, domain.ErrMatrixUnavailable
}

func (*fakeService) StartMatrixNativeForConsumer(context.Context, string, string, string, string, string, storage.IdentityVersion) (service.MatrixNativeStart, error) {
	return service.MatrixNativeStart{}, domain.ErrMatrixUnavailable
}

func (*fakeService) PreviewMatrixNative(context.Context, string, string) (service.MatrixNativePreview, error) {
	return service.MatrixNativePreview{}, domain.ErrMatrixUnavailable
}

func (*fakeService) AcceptMatrixNative(context.Context, string, string, string) (service.MatrixNativeAccepted, error) {
	return service.MatrixNativeAccepted{}, domain.ErrMatrixUnavailable
}

func (*fakeService) FinishMatrixNativeForConsumer(context.Context, string, string, string, string, string, string, storage.IdentityVersion) (string, error) {
	return "", domain.ErrMatrixUnavailable
}

func (*fakeService) MatrixStorageMigration(context.Context, string, string) (storage.IdentityCutoverState, error) {
	return storage.IdentityCutoverState{}, domain.ErrMatrixUnavailable
}

func (*fakeService) BeginMatrixStorageMigration(context.Context, string, string, string) (storage.IdentityCutoverState, error) {
	return storage.IdentityCutoverState{}, domain.ErrMatrixUnavailable
}

func (*fakeService) CompleteMatrixStorageMigration(context.Context, string, string, string, storage.Reference) (storage.IdentityCutoverState, error) {
	return storage.IdentityCutoverState{}, domain.ErrMatrixUnavailable
}
