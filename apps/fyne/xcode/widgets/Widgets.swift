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

    var descriptionKey: String { "app." + product + ".description" }

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
        // Precompute the private entry: expiry does not depend on the host app
        // opening, a network request or another WidgetKit refresh being granted.
        if let expiry = SytueWidgetTimeline.expiry(snapshot: snapshot, now: date) {
            entries.append(SytueWidgetEntry(date: expiry, snapshot: nil))
        }
        completion(Timeline(entries: entries, policy: .after(date.addingTimeInterval(900))))
    }
}

// Action widgets contain no account data. Their static timeline never opens the
// shared container and requires neither background work nor a signed-in account.
struct SytueWidgetActionProvider: TimelineProvider {
    func placeholder(in context: Context) -> SytueWidgetEntry {
        SytueWidgetEntry(date: Date(), snapshot: nil)
    }

    func getSnapshot(in context: Context, completion: @escaping (SytueWidgetEntry) -> Void) {
        completion(placeholder(in: context))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<SytueWidgetEntry>) -> Void) {
        completion(Timeline(entries: [placeholder(in: context)], policy: .never))
    }
}

private struct SytueWidgetCounterView: View {
    let counter: SytueWidgetCounter
    var compact = false

    var body: some View {
        counterContent
            .lineLimit(2)
            .minimumScaleFactor(0.8)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(counter.kind == "unread"
                ? Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                : Text(LocalizedStringKey(counter.localizationKey)))
            .accessibilityValue(counter.kind == "unread" ? Text("") : accessibleValue)
            .privacySensitive()
    }

    @ViewBuilder private var counterContent: some View {
        if compact {
            if counter.kind == "unread" {
                Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                    .font(.caption)
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 4) {
                    value.font(.headline)
                    Text(LocalizedStringKey(counter.localizationKey)).font(.caption)
                }
            }
        } else {
            VStack(alignment: .leading, spacing: 2) {
                if counter.kind == "unread" {
                    Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                        .font(.title2.weight(.semibold))
                } else {
                    value.font(.system(.largeTitle, design: .rounded).weight(.semibold))
                    Text(LocalizedStringKey(counter.localizationKey)).font(.caption).foregroundStyle(.secondary)
                }
            }
        }
    }

    private var value: Text {
        counter.kind == "storage_percent"
            ? Text(Double(counter.value) / 100, format: .percent.precision(.fractionLength(0)))
            : Text(counter.value, format: .number.notation(.compactName))
    }

    private var accessibleValue: Text {
        counter.kind == "storage_percent"
            ? Text(Double(counter.value) / 100, format: .percent.precision(.fractionLength(0)))
            : Text(counter.value, format: .number)
    }
}

private struct SytueWidgetActionLabel: View {
    let action: String

    var body: some View {
        Label(LocalizedStringKey(SytueWidgetActions.label(for: action)),
              systemImage: SytueWidgetActions.symbol(for: action))
    }
}

private struct SytueWidgetBackground: ViewModifier {
    @Environment(\.widgetFamily) var family

    @ViewBuilder private func legacy(_ content: Content) -> some View {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *),
           [.accessoryCircular, .accessoryRectangular, .accessoryInline].contains(family) {
            content
        } else {
            content.padding().background(Color.secondary.opacity(0.08))
        }
        #else
        content.padding().background(Color.secondary.opacity(0.08))
        #endif
    }

    func body(content: Content) -> some View {
        if #available(iOSApplicationExtension 17.0, macOSApplicationExtension 14.0, *) {
            content.containerBackground(.fill.tertiary, for: .widget)
        } else {
            legacy(content)
        }
    }
}

struct SytueWidgetView: View {
    @Environment(\.widgetFamily) var family
    let entry: SytueWidgetEntry
    let configuration = SytueWidgetConfiguration()

    private var header: some View {
        HStack(spacing: 8) {
            Image("Icon").resizable().scaledToFit().frame(width: 26, height: 26).accessibilityHidden(true)
            Text(configuration.name).font(.headline).lineLimit(1).minimumScaleFactor(0.8)
        }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            Spacer(minLength: 0)
            if let counters = entry.snapshot?.counters, !counters.isEmpty {
                HStack(alignment: .top, spacing: 16) {
                    ForEach(Array(counters.prefix(family == .systemSmall ? 1 : 3)), id: \.kind) { counter in
                        SytueWidgetCounterView(counter: counter)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            } else {
                // Missing, locked, expired and signed-out states share one neutral
                // presentation. It never invents zero counts or an account state.
                Text(LocalizedStringKey(configuration.descriptionKey))
                    .font(.caption).foregroundStyle(.secondary).lineLimit(3)
            }
            Spacer(minLength: 0)
            if family == .systemSmall {
                SytueWidgetActionLabel(action: "open").font(.caption.weight(.medium))
            } else {
                HStack(spacing: 16) {
                    ForEach(SytueWidgetActions.order.filter { configuration.actions[$0] != nil }, id: \.self) { action in
                        if let destination = configuration.actions[action] {
                            Link(destination: destination) {
                                SytueWidgetActionLabel(action: action)
                                    .font(.caption.weight(.medium))
                                    .lineLimit(1).minimumScaleFactor(0.8)
                            }
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    #if os(iOS)
    @available(iOSApplicationExtension 16.0, *)
    private var accessory: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(configuration.name).font(.headline).lineLimit(1)
            if let counter = entry.snapshot?.counters.first {
                SytueWidgetCounterView(counter: counter, compact: true)
            } else {
                SytueWidgetActionLabel(action: "open").font(.caption)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
    #endif

    @ViewBuilder var body: some View {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *), family == .accessoryRectangular {
            accessory.modifier(SytueWidgetBackground())
                .widgetURL(configuration.actions["open"])
        } else {
            content.modifier(SytueWidgetBackground())
                .widgetURL(configuration.actions["open"])
        }
        #else
        content.modifier(SytueWidgetBackground())
            .widgetURL(configuration.actions["open"])
        #endif
    }
}

struct SytueActionWidgetView: View {
    @Environment(\.widgetFamily) var family
    let action: String
    let configuration = SytueWidgetConfiguration()

    private var title: some View {
        SytueWidgetActionLabel(action: action).lineLimit(2).minimumScaleFactor(0.8)
    }

    @ViewBuilder private var content: some View {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *), family == .accessoryInline {
            SytueWidgetActionLabel(action: action)
        } else if #available(iOSApplicationExtension 16.0, *), family == .accessoryCircular {
            ZStack {
                AccessoryWidgetBackground()
                Image(systemName: SytueWidgetActions.symbol(for: action)).font(.title2)
            }
            .accessibilityLabel(Text(LocalizedStringKey(SytueWidgetActions.label(for: action))))
        } else if #available(iOSApplicationExtension 16.0, *), family == .accessoryRectangular {
            VStack(alignment: .leading, spacing: 2) {
                Text(configuration.name).font(.headline).lineLimit(1)
                title.font(.caption)
            }
        } else {
            regular
        }
        #else
        regular
        #endif
    }

    private var regular: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 8) {
                Image("Icon").resizable().scaledToFit().frame(width: 26, height: 26).accessibilityHidden(true)
                Text(configuration.name).font(.headline).lineLimit(1).minimumScaleFactor(0.8)
            }
            Spacer(minLength: 0)
            title.font(.title3.weight(.semibold))
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    var body: some View {
        content.modifier(SytueWidgetBackground())
            .widgetURL(configuration.actions[action])
    }
}

private func summaryFamilies() -> [WidgetFamily] {
    #if os(iOS)
    if #available(iOSApplicationExtension 16.0, *) {
        return [.systemSmall, .systemMedium, .accessoryRectangular]
    }
    #endif
    return [.systemSmall, .systemMedium]
}

private func actionFamilies() -> [WidgetFamily] {
    #if os(iOS)
    if #available(iOSApplicationExtension 16.0, *) {
        return [.systemSmall, .systemMedium, .accessoryCircular, .accessoryRectangular, .accessoryInline]
    }
    #endif
    return [.systemSmall, .systemMedium]
}

struct SytueSummaryWidget: Widget {
    private let configuration = SytueWidgetConfiguration()

    init() {}

    var body: some WidgetConfiguration {
        // Keep the installed kind stable when adding action widgets to the bundle.
        StaticConfiguration(kind: configuration.bundle + ".widgets", provider: SytueWidgetProvider()) { entry in
            SytueWidgetView(entry: entry)
        }
        .configurationDisplayName(Text(configuration.name))
        .description(Text(LocalizedStringKey(configuration.descriptionKey)))
        .supportedFamilies(summaryFamilies())
    }
}

private func actionConfiguration(_ action: String) -> some WidgetConfiguration {
    let configuration = SytueWidgetConfiguration()
    return StaticConfiguration(kind: configuration.bundle + ".widgets." + action, provider: SytueWidgetActionProvider()) { _ in
        SytueActionWidgetView(action: action)
    }
    .configurationDisplayName(Text(LocalizedStringKey(SytueWidgetActions.label(for: action))))
    .description(Text(configuration.name))
    .supportedFamilies(actionFamilies())
}

// WidgetKit requires a parameterless init for each widget type. Keep distinct
// types for the actions while sharing their provider, view and configuration.
struct SytueSearchWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("search")
    }
}

struct SytueComposeWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("compose")
    }
}

struct SytueCreateWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("create")
    }
}

@main
struct SytueWidgets: WidgetBundle {
    var body: some Widget {
        SytueSummaryWidget()
        // The signed target gets these flags from its FyneApp.toml. WidgetBundle
        // only permits availability conditions, not runtime metadata conditions.
        #if SYTUE_WIDGET_ACTION_SEARCH
        SytueSearchWidget()
        #endif
        #if SYTUE_WIDGET_ACTION_COMPOSE
        SytueComposeWidget()
        #endif
        #if SYTUE_WIDGET_ACTION_CREATE
        SytueCreateWidget()
        #endif
    }
}
