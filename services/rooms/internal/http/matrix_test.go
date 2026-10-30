// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// Matrix endpoints use a dedicated fake below; legacy handler fixtures cannot
// accidentally mark an unconfigured encrypted transport as ready.
func (*fakeService) BootstrapMatrix(context.Context, string, string) (service.MatrixBootstrap, error) {
	return service.MatrixBootstrap{}, domain.ErrMatrixUnavailable
}

func (*fakeService) BootstrapMatrixStorage(context.Context, string, string) (service.MatrixBootstrap, error) {
	return service.MatrixBootstrap{}, domain.ErrMatrixUnavailable
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

func (*fakeService) StartMatrixNative(context.Context, string, string, string, string) (service.MatrixNativeStart, error) {
	return service.MatrixNativeStart{}, domain.ErrMatrixUnavailable
}

func (*fakeService) PreviewMatrixNative(context.Context, string, string) (service.MatrixNativePreview, error) {
	return service.MatrixNativePreview{}, domain.ErrMatrixUnavailable
}

func (*fakeService) AcceptMatrixNative(context.Context, string, string, string) (service.MatrixNativeAccepted, error) {
	return service.MatrixNativeAccepted{}, domain.ErrMatrixUnavailable
}

func (*fakeService) FinishMatrixNative(context.Context, string, string, string, string) (string, error) {
	return "", domain.ErrMatrixUnavailable
}
