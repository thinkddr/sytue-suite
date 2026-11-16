// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const (
	publicationCoveragePath = "/_synapse/client/sytue/publication"
	publicationCoverageKind = publication.OriginCoverageKind
)

// PublicationCoverage is a fresh, authenticated witness from the configured
// single-process origin. The v3 contract recognizes registered collaborative
// resources as managed, including before their first event. It does not prove
// source admission or past deployments and is never a durable drain certificate.
type PublicationCoverage struct{ server, instance string }

func (p PublicationCoverage) SameProcess(other PublicationCoverage) bool {
	return p.server != "" && p.instance != "" && p == other
}

// RequirePublicationCoverage fails closed when only Synapse's generic health
// endpoint is available. A random challenge prevents a cached successful reply
// from authorizing a device handoff after the admission module disappears.
func (c *Client) RequirePublicationCoverage(ctx context.Context) (PublicationCoverage, error) {
	unavailable := domain.ErrMatrixUnavailable
	if !c.configured() || !canonicalCoverageHex(c.cfg.PublicationSecret) {
		return PublicationCoverage{}, unavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return PublicationCoverage{}, unavailable
	}
	challenge := hex.EncodeToString(nonce[:])
	raw, err := json.Marshal(struct {
		Version int    `json:"v"`
		Nonce   string `json:"nonce"`
	}{1, challenge})
	if err != nil {
		return PublicationCoverage{}, unavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.InternalURL+publicationCoveragePath, bytes.NewReader(raw))
	if err != nil {
		return PublicationCoverage{}, unavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.PublicationSecret)
	req.Header.Set("Cache-Control", "no-store")
	res, err := c.http.Do(req)
	if err != nil {
		return PublicationCoverage{}, unavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return PublicationCoverage{}, unavailable
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(body) > 4096 {
		return PublicationCoverage{}, unavailable
	}
	var witness struct {
		Version  int    `json:"v"`
		Server   string `json:"server"`
		Coverage string `json:"coverage"`
		Instance string `json:"instance"`
		Nonce    string `json:"nonce"`
		MAC      string `json:"mac"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&witness) != nil || decoder.Decode(new(any)) != io.EOF || witness.Version != 1 || witness.Server != c.cfg.ServerName || witness.Coverage != publicationCoverageKind || witness.Nonce != challenge || !canonicalCoverageHex(witness.Instance) || !canonicalCoverageHex(witness.MAC) {
		return PublicationCoverage{}, unavailable
	}
	transcript := "sytue-publication-coverage-v1\n" + witness.Server + "\n" + witness.Coverage + "\n" + witness.Instance + "\n" + challenge
	mac := hmac.New(sha256.New, []byte(c.cfg.PublicationSecret))
	_, _ = mac.Write([]byte(transcript))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(witness.MAC)) {
		return PublicationCoverage{}, unavailable
	}
	return PublicationCoverage{server: witness.Server, instance: witness.Instance}, nil
}

func canonicalCoverageHex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}
