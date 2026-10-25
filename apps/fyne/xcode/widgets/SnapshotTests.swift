// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation
#if canImport(Glibc)
import Glibc
#else
import Darwin
#endif

@main
struct SnapshotContractTests {
    static func main() throws {
        let manager = FileManager.default
        let root = manager.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try manager.createDirectory(at: root.appendingPathComponent("widgets"), withIntermediateDirectories: true)
        defer { try? manager.removeItem(at: root) }
        let marker = root.appendingPathComponent("sesion.marca")
        let preferences = root.appendingPathComponent("ajustes.json")
        let snapshot = root.appendingPathComponent("widgets/eu.sytue.rooms.json")
        let now: Int64 = 1_800_000_000
        let valid: [String: Any] = [
            "version": 1, "product": "rooms", "generation": "opaque-revision",
            "updated_at": now - 10, "expires_at": now + 600, "active": true,
            "counters": [["kind": "unread", "value": 3]],
        ]
        var checked = 0

        func require(_ condition: Bool, _ description: String) {
            checked += 1
            if !condition {
                FileHandle.standardError.write(Data(("not ok - " + description + "\n").utf8))
                exit(1)
            }
        }
        let local = SytueWidgetActions.routes(product: "docs", scheme: "sytue-docs", declared: ["open", "search", "compose", "create", "https://example.test"])
        require(local.count == 2 && local["search"]?.absoluteString == "sytue-docs:/?action=search", "search opens only this app without query text")
        require(SytueWidgetActions.routes(product: "docs", scheme: "https", declared: ["search"]).isEmpty, "wrong scheme cannot become an external search destination")
        require(SytueWidgetActions.routes(product: "docs", scheme: "sytue-docs", declared: ["open"])["search"] == nil, "search must be declared by host metadata")
        require(SytueWidgetActions.routes(product: "mail", scheme: "sytue-mail", declared: ["compose", "search"]).count == 2, "mail keeps compose alongside search")
        require(SytueWidgetActions.routes(product: "calendar", scheme: "sytue-calendar", declared: ["create", "search"]).count == 2, "calendar keeps create alongside search")
        require(SytueWidgetActions.routes(product: "mail", scheme: "sytue-mail", declared: ["create"])["create"] == nil,
                "an event action cannot be exported by Mail")
        require(SytueWidgetActions.routes(product: "calendar", scheme: "sytue-calendar", declared: ["compose"])["compose"] == nil,
                "a compose action cannot be exported by Calendar")
        require(SytueWidgetActions.order == ["open", "compose", "create", "search"], "actions retain a predictable visual and VoiceOver order")
        require(SytueWidgetActions.label(for: "search") == "search.label"
                && SytueWidgetActions.symbol(for: "search") == "magnifyingglass", "search has a translated name alongside its symbol")
        func write(_ value: [String: Any]) throws {
            try JSONSerialization.data(withJSONObject: value).write(to: snapshot, options: .atomic)
        }
        func reset() throws {
            try Data("opaque-revision".utf8).write(to: marker, options: .atomic)
            try Data("{}".utf8).write(to: preferences, options: .atomic)
            try write(valid)
        }
        func load() -> SytueWidgetSnapshot? {
            SytueWidgetSnapshots.load(directory: root, bundle: "eu.sytue.rooms", product: "rooms", now: now)
        }
        func reject(_ key: String, _ value: Any) throws {
            try reset()
            var changed = valid
            changed[key] = value
            try write(changed)
            require(load() == nil, "reject invalid " + key)
        }

        try reset()
        require(load()?.counters.first?.value == 3, "current bounded counter is visible")
        let instant = Date(timeIntervalSince1970: TimeInterval(now))
        require(SytueWidgetTimeline.expiry(snapshot: load(), now: instant) == instant.addingTimeInterval(600),
                "timeline precomputes the private entry at snapshot expiry")
        require(SytueWidgetTimeline.expiry(snapshot: load(), now: instant.addingTimeInterval(600)) == nil,
                "expired snapshot cannot schedule a past timeline entry")
        require(SytueWidgetTimeline.expiry(snapshot: nil, now: instant) == nil, "private timeline never invents a refresh event")
        for percent in [Int64(0), 42, 100] {
            var storage = valid
            storage["counters"] = [["kind": "storage_percent", "value": percent]]
            try write(storage)
            require(load()?.counters.first?.value == percent, "bounded storage percentage is visible: \(percent)")
        }
        require(SytueWidgetCounter(kind: "storage_percent", value: 50).localizationKey == "widgets.storage",
                "storage percentage has a category label rather than a count label")
        require(SytueWidgetCounter(kind: "unread", value: 3).localizationKey == "notifications.unread",
                "unread keeps the canonical plural translation")
        for kind in ["calendars", "address_books"] {
            var collection = valid
            collection["counters"] = [["kind": kind, "value": 2]]
            try write(collection)
            require(load()?.counters.first?.value == 2, "complete collection count is visible: " + kind)
        }
        try reset()
        try manager.removeItem(at: preferences)
        require(load() != nil, "new settings profile can omit preferences")
        try reject("generation", "previous-account")
        try reject("product", "mail")
        try reject("version", 2)
        try reject("active", false)
        try reject("updated_at", now + 6)
        try reject("expires_at", now)
        try reject("expires_at", now + 1801)
        try reject("counters", [["kind": "message-body", "value": 3]])
        try reject("counters", [["kind": "unread", "value": -1]])
        try reject("counters", [["kind": "unread", "value": Int64(1_000_000_000_001)]])
        try reject("counters", [["kind": "storage_percent", "value": 101]])
        try reject("counters", [["kind": "storage_percent", "value": -1]])
        try reject("counters", [["kind": "unread", "value": 1], ["kind": "unread", "value": 2]])
        try reject("counters", [["kind": "unread", "value": 1, "text": "private message"]])
        try reject("access_token", "never-display-or-open")
        try reset()
        try Data("another-session".utf8).write(to: marker, options: .atomic)
        require(load() == nil, "reject snapshot from another session")
        try reset()
        try Data("{\"bloqueo\":true}".utf8).write(to: preferences, options: .atomic)
        require(load() == nil, "OS unlock preference keeps widget private")
        try Data("{\"bloqueo\":0}".utf8).write(to: preferences, options: .atomic)
        require(load() == nil, "invalid lock preference stays private")
        try Data("not-json".utf8).write(to: preferences, options: .atomic)
        require(load() == nil, "malformed settings stay private")
        try reset()
        try Data().write(to: marker, options: .atomic)
        require(load() == nil, "empty marker stays private")
        try reset()
        try Data(repeating: 32, count: 16_385).write(to: snapshot, options: .atomic)
        require(load() == nil, "oversized snapshot stays private")
        try reset()
        require(SytueWidgetSnapshots.load(directory: root, bundle: "../sesion.marca", product: "rooms", now: now) == nil,
                "untrusted bundle cannot choose another path")
        require(checked >= 22, "snapshot contract executes its cases")
        print("ok - \(checked) WidgetKit snapshot cases")
    }
}
