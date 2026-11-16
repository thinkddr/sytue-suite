// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"errors"
	"io"
)

var ErrResponseLength = errors.New("api: response length does not match the expected source")

// Exact framing remains checked when an intermediary uses chunked transfer.
// Consumers must read to EOF before acknowledging a copy, then close the body.
type exactResponseBody struct {
	io.ReadCloser
	remaining int64
	done      bool
}

func (r *exactResponseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		clear(probe[:])
		if n != 0 {
			r.done = true
			_ = r.Close()
			return 0, ErrResponseLength
		}
		if err != nil {
			r.done = true
		}
		return 0, err
	}
	n, err := r.ReadCloser.Read(p[:min(int64(len(p)), r.remaining)])
	r.remaining -= int64(n)
	if errors.Is(err, io.EOF) {
		r.done = true
		if r.remaining != 0 {
			return n, ErrResponseLength
		}
	}
	return n, err
}
