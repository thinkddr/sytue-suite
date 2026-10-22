// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation
import SwiftUI
import WidgetKit

struct SytueWidgetConfiguration {
    let bundle: String
    let product: String
    let name: String
    let group: String
    let actions: [String: URL]

    init(bundle source: Bundle = .main) {
        let metadata = source.infoDictionary ?? [:]
        bundle = metadata["SytueWidgetBundle"] as? String ?? ""
        product = metadata["SytueWidgetProduct"] as? String ?? ""
        name = metadata["CFBundleDisplayName"] as? String ?? "Sytue"
        group = metadata["SytueWidgetGroup"] as? String ?? ""
        let scheme = metadata["SytueWidgetScheme"] as? String ?? ""
        actions = SytueWidgetActions.routes(product: product, scheme: scheme,
                                          declared: metadata["SytueWidgetActions"] as? [String] ?? [])
    }

    func snapshot(at date: Date) -> SytueWidgetSnapshot? {
        guard !group.isEmpty,
              let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group) else { return nil }
        return SytueWidgetSnapshots.load(directory: container.appendingPathComponent("sytue"), bundle: bundle,
                                         product: product, now: Int64(date.timeIntervalSince1970))
    }
}

struct SytueWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: SytueWidgetSnapshot?
}

struct SytueWidgetProvider: TimelineProvider {
    let configuration = SytueWidgetConfiguration()

    func placeholder(in context: Context) -> SytueWidgetEntry {
        SytueWidgetEntry(date: Date(), snapshot: nil)
    }

    func getSnapshot(in context: Context, completion: @escaping (SytueWidgetEntry) -> Void) {
        let date = Date()
        completion(SytueWidgetEntry(date: date, snapshot: context.isPreview ? nil : configuration.snapshot(at: date)))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<SytueWidgetEntry>) -> Void) {
        let date = Date()
        let snapshot = configuration.snapshot(at: date)
        var entries = [SytueWidgetEntry(date: date, snapshot: snapshot)]
        // A precomputed private entry removes stale counts even when the host
        // cannot fetch another snapshot. WidgetKit controls the exact schedule.
        if let snapshot {
            entries.append(SytueWidgetEntry(date: Date(timeIntervalSince1970: TimeInterval(snapshot.expires_at)), snapshot: nil))
        }
        completion(Timeline(entries: entries, policy: .after(date.addingTimeInterval(900))))
    }
}

private struct SytueWidgetCounterView: View {
    let counter: SytueWidgetCounter

    private var label: String {
        switch counter.kind {
        case "files", "events", "documents", "contacts": return "dataflow.item." + counter.kind
        default: return "widgets." + counter.kind
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            if counter.kind == "unread" {
                Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                    .font(.title2.weight(.semibold))
            } else {
                Text(counter.value, format: .number.notation(.compactName))
                    .font(.system(.largeTitle, design: .rounded).weight(.semibold))
                Text(LocalizedStringKey(label)).font(.caption).foregroundStyle(.secondary)
            }
        }
        .minimumScaleFactor(0.7)
        .privacySensitive()
    }
}

struct SytueWidgetView: View {
    @Environment(\.widgetFamily) var family
    let entry: SytueWidgetEntry
    let configuration = SytueWidgetConfiguration()

    private var content: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image("Icon").resizable().scaledToFit().frame(width: 26, height: 26).accessibilityHidden(true)
                Text(configuration.name).font(.headline).lineLimit(1).minimumScaleFactor(0.75)
            }
            Spacer(minLength: 0)
            if let counters = entry.snapshot?.counters, !counters.isEmpty {
                HStack(alignment: .top, spacing: 16) {
                    ForEach(Array(counters.prefix(family == .systemSmall ? 1 : 3)), id: \.kind) { counter in
                        SytueWidgetCounterView(counter: counter)
                    }
                }
            }
            Spacer(minLength: 0)
            if family == .systemSmall {
                Label("widgets.open", systemImage: "arrow.up.right").font(.caption.weight(.medium))
            } else if let open = configuration.actions["open"] {
                HStack(spacing: 16) {
                Link(destination: open) {
                    Label("widgets.open", systemImage: "arrow.up.right").font(.caption.weight(.medium))
                }
                if let compose = configuration.actions["compose"] {
                    Link(destination: compose) {
                        Label("widgets.compose", systemImage: "square.and.pencil").font(.caption.weight(.medium))
                    }
                }
                if let create = configuration.actions["create"] {
                    Link(destination: create) {
                        Label("widgets.create", systemImage: "calendar.badge.plus").font(.caption.weight(.medium))
                    }
                }
                if let search = configuration.actions["search"] {
                    Link(destination: search) {
                        Label("search.label", systemImage: "magnifyingglass")
                            .labelStyle(.iconOnly).font(.caption.weight(.medium))
                    }
                }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(configuration.actions["open"])
    }

    @ViewBuilder var body: some View {
        if #available(iOSApplicationExtension 17.0, macOSApplicationExtension 14.0, *) {
            content.containerBackground(.fill.tertiary, for: .widget)
        } else {
            content.padding().background(Color.secondary.opacity(0.08))
        }
    }
}

@main
struct SytueWidgets: Widget {
    private let configuration = SytueWidgetConfiguration()

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: configuration.bundle + ".widgets", provider: SytueWidgetProvider()) { entry in
            SytueWidgetView(entry: entry)
        }
        .configurationDisplayName(Text(configuration.name))
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}
