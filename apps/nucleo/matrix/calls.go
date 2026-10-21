// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Call authorization always identifies the current verified Matrix device. The
// metadata service returns a receive-only SFU token and an epoch, never a key.
func (r *RoomsService) callDevice(ctx context.Context) (string, error) {
	client, err := r.Client(ctx)
	if err != nil {
		return "", err
	}
	if !client.ready.Load() {
		return "", ErrNotReady
	}
	status, err := client.VerificationStatus(ctx)
	if err != nil {
		return "", err
	}
	if !status.Verified {
		return "", ErrUnverified
	}
	return client.DeviceID(), nil
}

func (r *RoomsService) JoinMeeting(ctx context.Context, meeting, name, link string) (api.RoomsCall, error) {
	device, err := r.callDevice(ctx)
	if err != nil {
		return api.RoomsCall{}, err
	}
	return r.JoinMeetingDevice(ctx, meeting, name, link, device)
}

func (r *RoomsService) CallChannel(ctx context.Context, channel, title, name string) (api.RoomsCall, error) {
	device, err := r.callDevice(ctx)
	if err != nil {
		return api.RoomsCall{}, err
	}
	return r.CallChannelDevice(ctx, channel, title, name, device)
}
