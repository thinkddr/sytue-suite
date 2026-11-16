// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/casttest"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

type privateCastFixture struct {
	st      *store.Store
	h       *pgtest.Harness
	source  *store.PrivateSource
	objects *casttest.Objects
	scope   storage.Scope
	video   domain.Video
	ctx     context.Context
}

func newPrivateCastFixture(t *testing.T) *privateCastFixture {
	t.Helper()
	st, h := montarConHarness(t)
	ctx := tenancy.Into(ctxCorto(t), tenancy.Tenant{Slug: "t1"})
	video := listo(ctx, t, st, "t1", "alice", domain.Visibility(model.PrivateVisibility), domain.StatusReady)
	prefix := video.ID.String() + "/"
	comoSuperusuario(ctx, t, h, `UPDATE videos SET original_key=$2,hls_key=$3,bytes_original=17,bytes_total=81,deleted_at=now()-interval '10 days' WHERE tenant='t1' AND id=$1`, video.ID, prefix+"original", prefix+"hls/master.m3u8")
	objects := &casttest.Objects{Bodies: map[string][]byte{prefix + "original": []byte("original / 漢字 é"), prefix + "hls/master.m3u8": []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXTINF:1,\nseg.m4s\n#EXT-X-ENDLIST\n"), prefix + "hls/seg.m4s": {0, 1, 2, 3, 4, 5}}}
	objects.Types = map[string]string{prefix + "original": "video/mp4; codecs=avc1.4d401f"}
	source, err := store.NewPrivateSource(st, objects)
	if err != nil {
		t.Fatal(err)
	}
	return &privateCastFixture{st: st, h: h, source: source, objects: objects, scope: storage.Scope{Tenant: "t1", Subject: "alice", Product: "cast"}, video: video, ctx: ctx}
}

func privateCastBlocked(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == "PST01"
}

func (f *privateCastFixture) freeze(t *testing.T) storage.AppMigrationState {
	t.Helper()
	state, err := f.source.Begin(f.ctx, f.scope, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestPrivateCastSourceRetainsExactMetadataTrashPlaylistsAndEveryMediaByte(t *testing.T) {
	f := newPrivateCastFixture(t)
	// Completed server work remains an exact historical row, including unknown
	// JSON numbers. Pending work is covered separately and cannot be frozen.
	comoSuperusuario(f.ctx, t, f.h, `INSERT INTO transcode_jobs(tenant,video_id,kind,status,payload)VALUES('t1',$1,'poster','done','{"unknown":900719925474099312345,"at_ms":1}')`, f.video.ID)
	state := f.freeze(t)
	if state.Mode != "frozen" || state.Rows != 8 || state.Blobs != 3 {
		t.Fatal("incomplete retained inventory", state.Mode, state.Rows, state.Blobs)
	}
	var cursor storage.LegacySourceCursor
	count, binaries := 0, 0
	foundTrash, foundPrecise, foundType := false, false, false
	for {
		page, err := f.source.Page(f.ctx, f.scope, uuid.MustParse(state.Epoch), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			actual, reader, err := f.source.Open(f.ctx, f.scope, uuid.MustParse(state.Epoch), row.Kind, row.ID)
			if err != nil || actual != row {
				t.Fatal("source descriptor changed", err)
			}
			raw, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			sum := sha256.Sum256(raw)
			if int64(len(raw)) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 {
				t.Fatal("source receipt did not authenticate exact bytes")
			}
			if row.Kind == model.BinaryKind {
				binaries++
			}
			if row.Kind == "video" {
				foundTrash = bytes.Contains(raw, []byte(`"deleted_at":`)) && bytes.Contains(raw, []byte(`"hls_qualities":`))
			}
			if row.Kind == "job" {
				foundPrecise = bytes.Contains(raw, []byte(`900719925474099312345`))
			}
			if row.Kind == "media" && bytes.Contains(raw, []byte("video/mp4; codecs=avc1.4d401f")) {
				foundType = true
			}
			clear(raw)
			count++
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if count != 8 || binaries != 3 || !foundTrash || !foundPrecise || !foundType {
		t.Fatal("historical originals or metadata omitted")
	}
	again, err := f.source.Begin(f.ctx, f.scope, uuid.MustParse(state.Epoch))
	if err != nil || !reflect.DeepEqual(again, state) {
		t.Fatal("exact epoch could not resume", err)
	}
	if _, err = f.source.Begin(f.ctx, f.scope, uuid.New()); !errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
		t.Fatal("foreign epoch replaced retained source", err)
	}
	rows, err := f.st.BorradosCaducados(f.ctx, 1, nil, 10)
	if err != nil || len(rows) != 0 {
		t.Fatal("GC scheduled frozen originals", err)
	}
	for index, pool := range []*pg.Pool{f.h.Pool, f.h.PoolSinRLS(t)} {
		for _, query := range []string{
			`UPDATE videos SET title='replacement' WHERE tenant='t1' AND owner_sub='alice'`,
			`DELETE FROM videos WHERE tenant='t1' AND owner_sub='alice'`,
			`INSERT INTO videos(tenant,owner_sub,kind,status,visibility,title)VALUES('t1','alice','upload','ready','private','new plaintext')`,
			`UPDATE transcode_jobs SET payload='{}' WHERE tenant='t1'`,
			`DELETE FROM cast_private_storage_snapshot WHERE tenant='t1' AND owner_sub='alice'`,
			`TRUNCATE cast_private_storage_snapshot`,
		} {
			var affected int64
			err := pool.InMaintenanceTx(f.ctx, func(tx pgx.Tx) error { tag, err := tx.Exec(f.ctx, query); affected = tag.RowsAffected(); return err })
			// An app role with no snapshot DELETE policy may see zero rows. The
			// BYPASSRLS role must still hit the physical trigger for every statement.
			if !privateCastBlocked(err) && (index == 1 || err != nil || affected != 0) {
				t.Fatal("old SQL writer escaped retained fence", query, err, affected)
			}
		}
	}
	down, err := fs.ReadFile(db.Migrations, "migrations/0013_private_storage.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	err = f.h.PoolSinRLS(t).InTx(f.ctx, func(tx pgx.Tx) error { _, err := tx.Exec(f.ctx, string(down)); return err })
	if !privateCastBlocked(err) {
		t.Fatal("rollback removed retained source authority", err)
	}
}

func TestPrivateCastSourceDoesNotFreezePublicSharedLiveBusyOrDamagedMedia(t *testing.T) {
	for _, mode := range []string{"org", "people-zero", "public", "unlisted", "live", "busy", "size", "missing-segment", "external-chat", "incoming-block"} {
		t.Run(mode, func(t *testing.T) {
			f := newPrivateCastFixture(t)
			switch mode {
			case "org", "public", "unlisted":
				comoSuperusuario(f.ctx, t, f.h, `UPDATE videos SET visibility=$2 WHERE tenant='t1' AND id=$1`, f.video.ID, mode)
			case "people-zero":
				comoSuperusuario(f.ctx, t, f.h, `UPDATE videos SET visibility='people' WHERE tenant='t1' AND id=$1`, f.video.ID)
			case "live":
				comoSuperusuario(f.ctx, t, f.h, `UPDATE videos SET kind='live',status='ready',live_room='still-server-signaled' WHERE tenant='t1' AND id=$1`, f.video.ID)
			case "busy":
				comoSuperusuario(f.ctx, t, f.h, `INSERT INTO transcode_jobs(tenant,video_id,kind)VALUES('t1',$1,'poster')`, f.video.ID)
			case "size":
				f.objects.BadSize = true
			case "missing-segment":
				delete(f.objects.Bodies, f.video.ID.String()+"/hls/seg.m4s")
			case "external-chat":
				other := listo(f.ctx, t, f.st, "other", "bob", domain.VisibilityOrg, domain.StatusReady)
				comoSuperusuario(f.ctx, t, f.h, `INSERT INTO chat_messages(tenant,video_id,author_sub,author_name,body)VALUES('other',$1,'alice','Alice','retained external contribution')`, other.ID)
			case "incoming-block":
				comoSuperusuario(f.ctx, t, f.h, `INSERT INTO person_blocks(blocker_sub,blocked_sub)VALUES('bob','alice')`)
			}
			if _, err := f.source.Begin(f.ctx, f.scope, uuid.New()); err == nil {
				t.Fatal("unsupported source was silently frozen")
			}
			state, err := f.source.State(f.ctx, f.scope)
			if err != nil || state.Mode != "legacy" || state.Rows != 0 {
				t.Fatal("failed capture published partial source", err)
			}
			if mode != "size" && mode != "missing-segment" && f.objects.Gets != 0 {
				t.Fatal("S3 opened before collaboration/live/work preflight")
			}
			var retained int
			if err := f.h.Pool.InMaintenanceTx(f.ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(f.ctx, `SELECT count(*) FROM videos WHERE tenant='t1' AND id=$1`, f.video.ID).Scan(&retained)
			}); err != nil || retained != 1 {
				t.Fatal("incompatible source was removed", err)
			}
		})
	}
}
