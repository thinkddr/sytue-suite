// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Key names retain the existing account algorithm. Only the configured native
// app can claim its prior material, after the maintained credential decoder
// verifies actor, tenant, homeserver, Matrix user and consumer.
func (c *Client) migrateNativeSecret(key string) ([]byte, error) {
	product := c.cfg.Bootstrap.StorageProduct
	if product == "" {
		product = "rooms"
	}
	app := storage.NativeReturnApp(product, c.cfg.NativeConsumer)
	if app == "" {
		return nil, ErrIdentity
	}
	scheme := api.Esquema(app)
	switch key {
	case c.sessionKey():
		return auth.MigrarSecretoPrivado(key, scheme, func(raw []byte) error { _, err := c.decodeSession(raw); return err })
	case c.masterKey():
		if _, err := c.readSession(); err != nil {
			return nil, err
		}
		return auth.MigrarSecretoPrivado(key, scheme, func(raw []byte) error {
			if len(raw) != 32 {
				return ErrSecureStore
			}
			return nil
		})
	default:
		return nil, ErrSecureStore
	}
}
