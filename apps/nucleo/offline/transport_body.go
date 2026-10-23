// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
)

// cacheBody delivers network bytes immediately. Only an intact EOF, successful
// underlying Close, live request and unchanged owner permit a cache commit. Early
// Close, cancellation and oversize streams keep their original network semantics.
type cacheBody struct {
	body             io.ReadCloser
	ctx              context.Context
	owner            string
	current          func() bool
	store            *Store
	product, id      string
	products         []string
	status           int
	header           http.Header
	expected         int64
	mu               sync.Mutex
	buffer           bytes.Buffer
	read             int64
	terminal, failed bool
	aborted          bool
	once             sync.Once
	closeErr         error
}

func (c *cacheBody) valid() bool {
	return c.ctx.Err() == nil && c.current != nil && c.current()
}

func (c *cacheBody) closeSource() error {
	c.once.Do(func() { c.closeErr = c.body.Close() })
	return c.closeErr
}

func (c *cacheBody) Read(p []byte) (int, error) {
	c.mu.Lock()
	done, aborted := c.terminal, c.aborted
	c.mu.Unlock()
	if done {
		if aborted {
			return 0, http.ErrBodyReadAfterClose
		}
		return 0, io.EOF
	}
	n, err := c.body.Read(p)
	c.mu.Lock()
	c.read += int64(n)
	if !c.terminal && !c.failed {
		if c.read > MaxCacheBytes || c.ctx.Err() != nil {
			c.failed = true
			c.buffer = bytes.Buffer{}
		} else {
			_, _ = c.buffer.Write(p[:n])
		}
	}
	if err != nil && err != io.EOF {
		c.failed = true
		c.buffer = bytes.Buffer{}
	}
	finish := err == io.EOF && !c.terminal
	if finish {
		c.terminal = true
	}
	valid := finish && !c.failed && (c.expected < 0 || c.read == c.expected)
	var payload []byte
	if finish {
		payload = c.buffer.Bytes()
		c.buffer = bytes.Buffer{}
	}
	c.mu.Unlock()
	if finish {
		closeErr := c.closeSource()
		if valid && closeErr == nil && c.valid() {
			// A cache failure cannot break the network response; the EOF remains its EOF.
			_ = c.store.writeHTTP(c.ctx, c.owner, c.product, c.id, c.products, respuestaGuardada{c.status, c.header, payload}, c.valid)
		}
		if closeErr != nil {
			return n, errors.Join(err, closeErr)
		}
	}
	return n, err
}

func (c *cacheBody) Close() error {
	c.mu.Lock()
	if !c.terminal {
		c.aborted = true
	}
	c.terminal = true
	c.buffer = bytes.Buffer{}
	c.mu.Unlock()
	return c.closeSource()
}
