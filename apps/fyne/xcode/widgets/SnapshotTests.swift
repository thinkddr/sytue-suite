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
        let epochFile = root.appendingPathComponent("sesion.epoca")
        let preferences = root.appendingPathComponent("ajustes.json")
        let snapshot = root.appendingPathComponent("widgets/eu.sytue.rooms.json")
        let now: Int64 = 1_800_000_000
        // Formato 2: `generation` es la época (sesion.epoca), confirmada con la marca.
        let epoch = "0123456789abcdef0123456789abcdef"
        let valid: [String: Any] = [
            "version": 2, "product": "rooms", "generation": epoch,
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
        func writeEpoch(_ value: String, confirmed: String) throws {
            try Data((value + "\nhuella\nhuella-org\n" + confirmed + "\n").utf8).write(to: epochFile, options: .atomic)
        }
        func reset() throws {
            try Data("opaque-revision".utf8).write(to: marker, options: .atomic)
            try writeEpoch(epoch, confirmed: "opaque-revision")
            try Data("{}".utf8).write(to: preferences, options: .atomic)
            try? manager.removeItem(at: root.appendingPathComponent("sesion-0123.cerrada"))
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

        // Los casos comunes con Go y Java: apps/nucleo/widgets/testdata/contrato.tsv,
        // primer argumento.
        func contract() throws {
            guard CommandLine.arguments.count == 2 else {
                return require(false, "usage: snapshot apps/nucleo/widgets/testdata/contrato.tsv")
            }
            let text = try String(contentsOfFile: CommandLine.arguments[1], encoding: .utf8)
            var counts = [String: Int]()
            func field(_ value: String) -> String { value == "-" ? "" : value }
            func int(_ value: String) -> Int64 { Int64(value) ?? -1 }
            for line in text.components(separatedBy: "\n") where !line.isEmpty && !line.hasPrefix("#") {
                let c = line.components(separatedBy: "\t")
                counts[c[0], default: 0] += 1
                switch c[0] {
                case "visible":
                    let entorno = SytueWidgetEntorno(epoca: field(c[2]), confirmada: field(c[3]), marca: field(c[4]),
                                                     cerrada: c[5] == "1", bloqueo: c[6] == "1")
                    let object: [String: Any] = [
                        "version": int(c[7]), "product": "mail", "active": c[8] == "1", "generation": field(c[9]),
                        "updated_at": int(c[10]), "expires_at": int(c[11]), "counters": [["kind": "unread", "value": 3]],
                    ]
                    let decoded = try JSONDecoder().decode(SytueWidgetSnapshot.self,
                                                           from: JSONSerialization.data(withJSONObject: object))
                    require(decoded.isVisible(product: "mail", entorno: entorno, now: int(c[12])) == (c[13] == "1"),
                            "contract visible: " + c[1])
                case "rancio":
                    let object: [String: Any] = [
                        "version": 2, "product": "mail", "active": true, "generation": "g",
                        "updated_at": int(c[2]), "expires_at": int(c[2]) + 60, "counters": [[String: Any]](),
                    ]
                    let decoded = try JSONDecoder().decode(SytueWidgetSnapshot.self,
                                                           from: JSONSerialization.data(withJSONObject: object))
                    require(decoded.isStale(now: int(c[3])) == (c[4] == "1"), "contract stale: " + c[1])
                case "vigente":
                    var zone = Calendar(identifier: .gregorian)
                    guard let tz = TimeZone(identifier: c[2]) else { return require(false, "known zone " + c[2]) }
                    zone.timeZone = tz
                    let item = SytueWidgetItem(title: "x", subtitle: nil, badge: nil, color: nil, start_at: int(c[3]),
                                               end_at: int(c[4]), all_day: c[5] == "1", unread: nil, done: nil)
                    require(SytueWidgetTimeline.isCurrent(item, now: int(c[6]), calendar: zone) == (c[7] == "1"),
                            "contract current: " + c[1] + " in " + c[2])
                default:
                    require(false, "unknown contract case " + c[0])
                }
            }
            require((counts["visible"] ?? 0) >= 15 && (counts["rancio"] ?? 0) >= 4 && (counts["vigente"] ?? 0) >= 10,
                    "the contrato.tsv cases are all there")
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
        try reject("generation", "opaque-revision")
        try reject("product", "mail")
        try reject("version", 1)
        try reject("version", 3)
        try reject("active", false)
        try reject("updated_at", now + 6)
        try reject("expires_at", now)
        try reject("expires_at", now - 10 + 86401)
        try reset()
        var day = valid
        day["expires_at"] = now - 10 + 86400
        try write(day)
        require(load() != nil, "a snapshot lives up to 24 h")
        try reject("counters", [["kind": "message-body", "value": 3]])
        try reject("counters", [["kind": "unread", "value": -1]])
        try reject("counters", [["kind": "unread", "value": Int64(1_000_000_000_001)]])
        try reject("counters", [["kind": "storage_percent", "value": 101]])
        try reject("counters", [["kind": "storage_percent", "value": -1]])
        try reject("counters", [["kind": "unread", "value": 1], ["kind": "unread", "value": 2]])
        try reject("counters", [["kind": "unread", "value": 1, "text": "private message"]])
        try reject("access_token", "never-display-or-open")
        var rich = valid
        rich["items"] = [[
            "title": "Reunión de diseño",
            "subtitle": "Sala Norte",
            "badge": "10:00",
            "color": "#953be7",
            "start_at": now + 1800,
            "end_at": now + 3600,
            "unread": true,
        ]]
        rich["calendar"] = [
            "year": 2026,
            "month": 11,
            "busy_days": [3, 14, 28],
            "day_colors": ["3": "#953be7", "14": "#16734d"],
        ]
        try write(rich)
        let loadedRich = load()
        require(loadedRich?.visualItems.first?.title == "Reunión de diseño"
                && loadedRich?.calendar?.busyDays == [3, 14, 28]
                && loadedRich?.calendar?.dayColors["3"] == "#953be7",
                "rich visual items and calendar month grid are decoded")
        try reject("items", [["title": "enc:v1:secret-title"]])
        try reject("items", [["title": "Valid", "subtitle": "ENC:V1:secret-sub"]])
        try reject("items", [["title": "Valid", "url": "https://evil.example"]])
        try reject("items", [["title": "Valid", "color": "#953BE7"]])
        try reject("items", Array(repeating: ["title": "x"], count: 9))
        try reject("calendar", ["year": 2026, "month": 2, "busy_days": [29]])
        try reject("calendar", ["year": 2026, "month": 11, "busy_days": [14, 3]])
        try reject("calendar", ["year": 2026, "month": 11, "day_colors": ["03": "#953be7"]])
        try reject("calendar", ["year": 2026, "month": 11, "secret": "nope"])
        try reset()
        try Data("another-session".utf8).write(to: marker, options: .atomic)
        require(load() == nil, "a session changed by a build that does not know the epoch stays private")
        try writeEpoch(epoch, confirmed: "another-session")
        require(load() != nil, "a renewal (same epoch, new confirmed marker) keeps the snapshot visible")
        try writeEpoch("fedcba9876543210fedcba9876543210", confirmed: "another-session")
        require(load() == nil, "reject snapshot from another identity epoch")
        try reset()
        try Data("1\n".utf8).write(to: root.appendingPathComponent("sesion-0123.cerrada"), options: .atomic)
        require(load() == nil, "a closed session marker hides every snapshot")
        try reset()
        try manager.removeItem(at: epochFile)
        require(load() == nil, "without an identity epoch nothing is shown")
        try reset()
        try Data("short\n".utf8).write(to: epochFile, options: .atomic)
        require(load() == nil, "malformed epoch stays private")
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
        // La línea de tiempo `.atEnd`: precalculada, sin la entrada vacía de 24 h cuando la
        // lectura es protegida.
        try reset()
        var utc = Calendar(identifier: .gregorian)
        utc.timeZone = TimeZone(identifier: "UTC")!
        guard let current = load() else { return require(false, "fixture visible") }
        let start = Date(timeIntervalSince1970: TimeInterval(now))
        switch SytueWidgetTimeline.plan(for: .visible(current), now: start, calendar: utc) {
        case .entries(let dates, let expiry):
            // Caduca a los 10 min: ni la marca de los 15 min ni la medianoche caben.
            require(dates == [start] && expiry == start.addingTimeInterval(600),
                    "entries start now and never go past the expiry")
        default:
            require(false, "a visible snapshot gets precomputed entries")
        }
        var long = valid
        long["expires_at"] = now - 10 + 86400
        long["product"] = "calendar"
        long["items"] = [["title": "Daily", "start_at": now + 60, "end_at": now + 1200]]
        try write(long)
        if let calendarSnapshot = SytueWidgetSnapshots.load(directory: root, bundle: "eu.sytue.rooms", product: "calendar", now: now),
           case .entries(let dates, _) = SytueWidgetTimeline.plan(for: .visible(calendarSnapshot), now: start, calendar: utc) {
            let updated = Date(timeIntervalSince1970: TimeInterval(now - 10))
            require(dates.count <= SytueWidgetTimeline.maxEntries - 1, "at most 40 entries with the expiry")
            require(dates.first == start && dates == dates.sorted() && Set(dates).count == dates.count, "sorted, unique, from now")
            require([15, 30, 45, 60, 120].allSatisfy { dates.contains(updated.addingTimeInterval(TimeInterval($0 * 60))) },
                    "the «updated X ago» marks: 15, 30, 45 min, then hourly")
            require(dates.contains(Date(timeIntervalSince1970: TimeInterval(now + 1200))), "an entry when an event ends")
            require(dates.contains(utc.startOfDay(for: start).addingTimeInterval(86400)), "an entry at midnight")
            require(SytueWidgetTimeline.currentItems(calendarSnapshot, at: Date(timeIntervalSince1970: TimeInterval(now + 1200)),
                                                     calendar: utc).isEmpty, "rows that ended are trimmed at their entry")
            require(SytueWidgetTimeline.currentItems(calendarSnapshot, at: start, calendar: utc).count == 1, "current rows stay")
        } else {
            require(false, "the 24 h calendar snapshot is visible")
        }
        require(SytueWidgetTimeline.plan(for: .hidden, now: start, calendar: utc) == .empty, "nothing to show, no reloads")
        require(SytueWidgetTimeline.plan(for: .protected, now: start, calendar: utc) == .protected(retry: start.addingTimeInterval(300)),
                "a protected read retries in 5 min")
        try reset()
        if getuid() != 0 {
            // Lo que ve la extensión con NSFileProtectionComplete y el teléfono bloqueado.
            try manager.setAttributes([.posixPermissions: 0], ofItemAtPath: snapshot.path)
            if case .protected = SytueWidgetSnapshots.reading(directory: root, bundle: "eu.sytue.rooms", product: "rooms", now: now) {
                require(true, "an unreadable snapshot is protected, not missing")
            } else {
                require(false, "an unreadable snapshot is protected, not missing")
            }
            try manager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: snapshot.path)
        }
        try manager.removeItem(at: snapshot)
        if case .hidden = SytueWidgetSnapshots.reading(directory: root, bundle: "eu.sytue.rooms", product: "rooms", now: now) {
            require(true, "a missing snapshot is hidden")
        } else {
            require(false, "a missing snapshot is hidden")
        }
        try contract()
        require(checked >= 40, "snapshot contract executes its cases")
        print("ok - \(checked) WidgetKit snapshot cases")
    }
}
