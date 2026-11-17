// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
)

type catalogPort struct {
	binding  Binding
	bindings int
	pages    map[string]map[string]ObjectPage
	calls    []string
	bind     func(int) (Binding, error)
	read     func(string, string) (ObjectPage, error)
}

func (p *catalogPort) StorageBindings(context.Context) (Binding, error) {
	p.bindings++
	if p.bind != nil {
		return p.bind(p.bindings)
	}
	return p.binding, nil
}

func (p *catalogPort) StorageObjectHistory(_ context.Context, room, cursor string, limit int) (ObjectPage, error) {
	if limit != 100 {
		panic("unexpected history page size")
	}
	p.calls = append(p.calls, room+"/"+cursor)
	if p.read != nil {
		return p.read(room, cursor)
	}
	return p.pages[room][cursor], nil
}

func catalogPortFixture() *catalogPort {
	binding := Binding{Scope: catalogScope, Target: PublicationTarget{RoomID: "!private:test", DeviceID: "DEVICE"}, Rooms: []string{"!private:test", "!retained:test"}}
	root := catalogEntry("root", "r0", "live")
	child := catalogEntry("child", "r1", "trash", root.Reference)
	child.Reference.RoomID = "!retained:test"
	return &catalogPort{binding: binding, pages: map[string]map[string]ObjectPage{
		"!private:test":  {"": {Next: "without-index"}, "without-index": {Entries: []RecordEntry{root}}},
		"!retained:test": {"": {Entries: []RecordEntry{child}}},
	}}
}

func TestLoadCatalogCoversEveryRoomAndKeepsExactOwnedHistory(t *testing.T) {
	p := catalogPortFixture()
	snapshot, err := LoadCatalog(t.Context(), p, catalogScope)
	if err != nil || len(snapshot.Entries) != 2 || len(snapshot.Heads) != 1 || snapshot.Heads[0].Commit.State != "trash" || len(p.calls) != 3 || p.bindings != 2 {
		t.Fatal("partial scan was accepted or history was lost", err, len(snapshot.Entries), p.calls, p.bindings)
	}
	ref := Reference{RoomID: "!private:test", EventID: "$root"}
	before := bytes.Clone(snapshot.Entries[ref].Record.Payload)
	p.pages["!private:test"]["without-index"].Entries[0].Record.Payload[0] ^= 1
	p.binding.Rooms[0] = "!foreign:test"
	if !bytes.Equal(snapshot.Entries[ref].Record.Payload, before) || snapshot.Binding.Rooms[0] != "!private:test" || !bytes.Contains(snapshot.Heads[0].Commit.Metadata, []byte("9007199254740993")) {
		t.Fatal("port buffers changed the retained projection")
	}
	child := snapshot.Entries[Reference{RoomID: "!retained:test", EventID: "$child"}]
	child.Record.Previous[0] = "$changed"
	if p.pages["!retained:test"][""].Entries[0].Record.Previous[0] != "$root" {
		t.Fatal("snapshot aliases the port's ancestry buffers")
	}
}

func TestLoadCatalogRejectsChangedRoutingAndBrokenPaginationWithoutPartialResult(t *testing.T) {
	for _, fault := range []string{"scope", "target", "retained-room", "late-change", "foreign-room", "duplicate", "repeat-cursor", "cursor-cycle", "parent-missing", "bad-record", "oversize", "port-error"} {
		t.Run(fault, func(t *testing.T) {
			p := catalogPortFixture()
			boom := errors.New("synthetic port interruption")
			switch fault {
			case "scope":
				p.binding.Scope.Product = "mail"
			case "target":
				p.binding.Target.DeviceID = ""
			case "retained-room":
				p.binding.Rooms = append(p.binding.Rooms, p.binding.Rooms[0])
			case "late-change":
				p.bind = func(call int) (Binding, error) {
					b := p.binding
					if call == 2 {
						b.Target.DeviceID = "ANOTHER"
					}
					return b, nil
				}
			case "foreign-room":
				p.pages["!private:test"]["without-index"].Entries[0].Reference.RoomID = "!foreign:test"
			case "duplicate":
				page := p.pages["!private:test"]["without-index"]
				page.Entries = append(page.Entries, page.Entries[0])
				p.pages["!private:test"]["without-index"] = page
			case "repeat-cursor":
				p.pages["!private:test"]["without-index"] = ObjectPage{Next: "without-index"}
			case "cursor-cycle":
				p.pages["!private:test"]["without-index"] = ObjectPage{Next: "second"}
				p.pages["!private:test"]["second"] = ObjectPage{Next: "without-index"}
			case "parent-missing":
				p.pages["!private:test"]["without-index"] = ObjectPage{}
			case "bad-record":
				p.pages["!private:test"]["without-index"].Entries[0].Record.Scope.Product = "mail"
			case "oversize":
				p.pages["!private:test"]["without-index"].Entries[0].Record.Payload = bytes.Repeat([]byte(" "), MaxRecordBytes+1)
			case "port-error":
				p.read = func(string, string) (ObjectPage, error) { return ObjectPage{}, boom }
			}
			snapshot, err := LoadCatalog(t.Context(), p, catalogScope)
			if err == nil || snapshot.Entries != nil || snapshot.Heads != nil || snapshot.Binding.Valid() {
				t.Fatal("invalid source became a partial projection", fault, err)
			}
			if fault == "port-error" && !errors.Is(err, boom) {
				t.Fatal("port error was masked", err)
			}
			if fault == "parent-missing" && !errors.Is(err, ErrIncompleteCatalog) {
				t.Fatal("incomplete history was masked", err)
			}
		})
	}
}

func TestLoadCatalogRechecksRoutingAndCancellationEvenWithAnEmptyCatalog(t *testing.T) {
	p := catalogPortFixture()
	p.pages = nil
	p.bind = func(call int) (Binding, error) {
		b := p.binding
		if call == 2 {
			b.Rooms = slices.Clone(b.Rooms)
			slices.Reverse(b.Rooms)
		}
		return b, nil
	}
	if s, err := LoadCatalog(t.Context(), p, catalogScope); err != nil || len(s.Entries) != 0 || len(s.Heads) != 0 {
		t.Fatal("valid empty/room ordering rejected", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	p.bind = func(int) (Binding, error) { cancel(); return p.binding, nil }
	if s, err := LoadCatalog(ctx, p, catalogScope); !errors.Is(err, context.Canceled) || s.Entries != nil {
		t.Fatal("cancelled scan succeeded", err)
	}
	calls := p.bindings
	if _, err := LoadCatalog(t.Context(), p, Scope{}); !errors.Is(err, ErrCatalogSource) || p.bindings != calls {
		t.Fatal("invalid expected scope reached reader", err)
	}
}
