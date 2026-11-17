// SPDX-License-Identifier: Apache-2.0
package web

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

type attachmentView struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	CID    string `json:"cid,omitempty"`
	Inline bool   `json:"inline,omitempty"`
}
type messageView struct {
	Text        string           `json:"text"`
	HTML        string           `json:"html"`
	To          []string         `json:"to"`
	CC          []string         `json:"cc"`
	BCC         []string         `json:"bcc"`
	Subject     string           `json:"subject"`
	Attachments []attachmentView `json:"attachments"`
}

// Display metadata and a selected attachment share the maintained Go MIME
// reader. Unselected parts are drained; they never become JSON/base64 bodies.
func readMessageView(ctx context.Context, r *privateMail.Repository, id string, index int) (messageView, []byte, error) {
	var view messageView
	reader, err := r.Raw(ctx, id)
	if err != nil {
		return view, nil, err
	}
	var selected []byte
	count := 0
	parsed, readErr := eml.LeerConAdjuntos(ctx, reader, r.MessageByteLimit(), func(_ context.Context, part eml.Adjunto, body io.Reader) error {
		var size int64
		var err error
		if index == count {
			selected, err = io.ReadAll(body)
			size = int64(len(selected))
		} else {
			size, err = io.Copy(io.Discard, body)
		}
		view.Attachments = append(view.Attachments, attachmentView{Name: part.Nombre, Type: part.Tipo, Size: size, CID: part.CID, Inline: part.CID != ""})
		count++
		return err
	})
	if err = errors.Join(readErr, reader.Close()); err != nil {
		clear(selected)
		return messageView{}, nil, err
	}
	if index >= count {
		clear(selected)
		return messageView{}, nil, webport.ErrRejected
	}
	view.Text, view.HTML, view.To, view.CC, view.BCC, view.Subject = parsed.Texto, parsed.HTML, parsed.Para, parsed.Cc, parsed.Cco, parsed.Asunto
	return view, selected, nil
}
