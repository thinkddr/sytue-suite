// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateArchivePlanKeepsHierarchyAndOmitsDuplicateAndHiddenTargets(t *testing.T) {
	r := queryFixture(t, 0)
	now := time.Now().UTC()
	root, parent := "root", "parent"
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &root, Name: "Folder", UpdatedAt: now})
	addQueryRow(t, r, "folder", "child", api.Folder{ID: "child", ParentID: &parent, Name: "Child", UpdatedAt: now})
	addQueryRow(t, r, "folder", "trash", api.Folder{ID: "trash", ParentID: &parent, Name: "Trash", UpdatedAt: now, DeletedAt: &now})
	addQueryRow(t, r, "file", "nested", api.File{ID: "nested", FolderID: "child", Name: "File.txt", SizeBytes: 7, UpdatedAt: now})
	addQueryRow(t, r, "file", "hidden", api.File{ID: "hidden", FolderID: "trash", Name: "Hidden.txt", SizeBytes: 100, UpdatedAt: now})
	addQueryRow(t, r, "file", "deleted", api.File{ID: "deleted", FolderID: parent, Name: "Deleted.txt", SizeBytes: 100, UpdatedAt: now, DeletedAt: &now})
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entries, total, err := tree.archivePlan(t.Context(), []string{"nested", "nested", "foreign"}, []string{parent, "child", parent, root})
	if err != nil || total != 7 || len(entries) != 1 || entries[0].name != "Folder/Child/File.txt" || entries[0].node.item.SourceID != "nested" {
		t.Fatal("private archive lost hierarchy or exposed duplicate/trash targets", entries, total, err)
	}
	entries, total, err = tree.archivePlan(t.Context(), []string{"foreign"}, nil)
	if err != nil || len(entries) != 0 || total != 0 {
		t.Fatal("foreign archive selection exposed a resource", entries, total, err)
	}
	if _, _, err := tree.archivePlan(t.Context(), nil, nil); !errors.Is(err, ErrSource) {
		t.Fatal("empty selection accepted", err)
	}
	if _, _, err := tree.archivePlan(t.Context(), make([]string, storage.MaxDriveSelectionItems+1), nil); !errors.Is(err, ErrSelectionTooLarge) {
		t.Fatal("oversized selection accepted", err)
	}
}

func TestArchiveNamesStayRelativeAndDoNotOverwriteOnCaseInsensitiveFilesystems(t *testing.T) {
	r := queryFixture(t, 0)
	for i, name := range []string{"../../escape.txt", `C:\temp\file.txt`, "Report.txt", "report.txt", "Folder"} {
		id := fmt.Sprintf("file_%d", i)
		addQueryRow(t, r, "file", id, api.File{ID: id, FolderID: "root", Name: name})
	}
	root := "root"
	addQueryRow(t, r, "folder", "folder", api.Folder{ID: "folder", ParentID: &root, Name: "Folder"})
	addQueryRow(t, r, "file", "nested", api.File{ID: "nested", FolderID: "folder", Name: "nested.txt"})
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := tree.archivePlan(t.Context(), []string{"file_0", "file_1", "file_2", "file_3", "file_4"}, []string{"folder"})
	if err != nil || len(entries) != 6 {
		t.Fatal(entries, err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		key := strings.ToLower(entry.name)
		if strings.HasPrefix(entry.name, "/") || strings.Contains(entry.name, "..") || strings.ContainsAny(entry.name, ":\\") || seen[key] || key == "folder" {
			t.Fatal("unsafe or colliding archive entry", entry.name)
		}
		seen[key] = true
	}
	if !seen["escape.txt"] || !seen["c_/temp/file.txt"] || !seen["folder/nested.txt"] {
		t.Fatal("archive sanitation lost files", seen)
	}
}

func TestPrivateArchiveBoundsFileCountBeforeStreaming(t *testing.T) {
	r := queryFixture(t, 0)
	root := "root"
	addQueryRow(t, r, "folder", "folder", api.Folder{ID: "folder", ParentID: &root, Name: "Folder"})
	for i := range storage.MaxDriveZipFiles + 1 {
		id := fmt.Sprintf("file_%d", i)
		addQueryRow(t, r, "file", id, api.File{ID: id, FolderID: "folder", Name: id})
	}
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tree.archivePlan(t.Context(), nil, []string{"folder"}); !errors.Is(err, ErrSelectionTooLarge) {
		t.Fatal("oversized archive reached streaming", err)
	}
}

func TestArchiveRejectsUnsafeNamesBeforeResolvingDirectories(t *testing.T) {
	for _, name := range []string{"/absolute/file", "../escape", "C:/file", `a\file`, ".", "a//b", strings.Repeat("a", 1<<16)} {
		if err := uniqueArchiveNames([]archiveEntry{{name: name}}); !errors.Is(err, ErrSource) {
			t.Fatal("unnormalized archive path accepted", name, err)
		}
	}
}

func TestArchiveUnicodeCaseAliasesRemainDistinct(t *testing.T) {
	entries := []archiveEntry{{name: "Σ.txt"}, {name: "ς.txt"}}
	if err := uniqueArchiveNames(entries); err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(entries[0].name, entries[1].name) {
		t.Fatal("Unicode case aliases would overwrite an archived file", entries)
	}
}

func TestClosingPrivateStreamCancelsItsProducer(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	r := &Repository{}
	body := r.stream(t.Context(), func() { close(finished) }, func(ctx context.Context, _ io.Writer) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started
	_ = body.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("closing the private stream did not cancel its network producer")
	}
}
