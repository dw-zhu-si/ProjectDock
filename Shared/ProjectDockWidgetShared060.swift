import Foundation

enum ProjectDockWidgetConfiguration {
#if APP_STORE
    static let appGroupIdentifier = "L4G2HAQ5B5.com.zhusi.projectdock.store"
#else
    static let appGroupIdentifier = "L4G2HAQ5B5.com.zhusi.projectdock"
#endif
    static let snapshotFilename = "projectdock-widget-snapshot.json"

    static func snapshotURL(fileManager: FileManager = .default) -> URL? {
        fileManager
            .containerURL(forSecurityApplicationGroupIdentifier: appGroupIdentifier)?
            .appendingPathComponent(snapshotFilename)
    }

    static func routeURL(_ route: String) -> URL {
        URL(string: "projectdock://open/\(route)")!
    }

    static let refreshURL = URL(string: "projectdock://refresh")!
}

enum ProjectDockWidgetKind {
    static let status = "ProjectDockStatusWidget"
}

struct ProjectDockProjectSummary: Codable, Hashable, Identifiable {
    let name: String
    let state: String
    let source: String
    let portCount: Int

    var id: String {
        "\(name)|\(source)|\(state)"
    }
}

struct ProjectDockAllocationSummary: Codable, Hashable, Identifiable {
    let port: Int
    let projectName: String
    let state: String

    var id: String {
        "\(port)|\(projectName)"
    }
}

struct ProjectDockWidgetSnapshot: Codable, Hashable {
    let schemaVersion: Int
    let updatedAt: String
    let projectCount: Int
    let runningCount: Int
    let listeningPortCount: Int
    let allocatedPortCount: Int
    let activeAllocationCount: Int
    let temporaryReservationCount: Int
    let registeredOnlyCount: Int
    let projects: [ProjectDockProjectSummary]
    let allocations: [ProjectDockAllocationSummary]

    var updatedDate: Date? {
        ProjectDockDateParser.parse(updatedAt)
    }

    var isPlaceholder: Bool {
        schemaVersion == 0
    }

    static let placeholder = ProjectDockWidgetSnapshot(
        schemaVersion: 0,
        updatedAt: "",
        projectCount: 0,
        runningCount: 0,
        listeningPortCount: 0,
        allocatedPortCount: 0,
        activeAllocationCount: 0,
        temporaryReservationCount: 0,
        registeredOnlyCount: 0,
        projects: [],
        allocations: []
    )
}

enum ProjectDockDateParser {
    static func parse(_ value: String) -> Date? {
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractional.date(from: value) {
            return date
        }
        return ISO8601DateFormatter().date(from: value)
    }
}

enum ProjectDockSnapshotStore {
	private static let unchangedRefreshInterval: TimeInterval = 4 * 60

    static func load(fileManager: FileManager = .default) -> ProjectDockWidgetSnapshot {
        guard let url = ProjectDockWidgetConfiguration.snapshotURL(fileManager: fileManager),
              let data = try? Data(contentsOf: url),
              let snapshot = try? JSONDecoder().decode(ProjectDockWidgetSnapshot.self, from: data),
              snapshot.schemaVersion == 2
        else {
            return .placeholder
        }
        return snapshot
    }

	@discardableResult
	static func saveIfChanged(data: Data, fileManager: FileManager = .default) throws -> Bool {
		let snapshot = try JSONDecoder().decode(ProjectDockWidgetSnapshot.self, from: data)
        guard snapshot.schemaVersion == 2,
              snapshot.projectCount >= 0,
              snapshot.runningCount >= 0,
              snapshot.listeningPortCount >= 0,
              snapshot.allocatedPortCount >= 0,
              snapshot.activeAllocationCount >= 0,
              snapshot.temporaryReservationCount >= 0,
              snapshot.registeredOnlyCount >= 0,
              snapshot.projects.count <= 6,
              snapshot.allocations.count <= 8,
              snapshot.updatedDate != nil
		else {
			throw CocoaError(.fileReadCorruptFile)
		}
		let existing = load(fileManager: fileManager)
		if !existing.isPlaceholder,
		   existing.hasSameContent(as: snapshot),
		   let updatedDate = existing.updatedDate,
		   Date().timeIntervalSince(updatedDate) < unchangedRefreshInterval {
			return false
		}
		guard let url = ProjectDockWidgetConfiguration.snapshotURL(fileManager: fileManager) else {
            throw CocoaError(.fileNoSuchFile)
        }
        try fileManager.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
		try data.write(to: url, options: .atomic)
		try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
		return true
	}
}

private extension ProjectDockWidgetSnapshot {
	func hasSameContent(as other: ProjectDockWidgetSnapshot) -> Bool {
		schemaVersion == other.schemaVersion &&
		projectCount == other.projectCount &&
		runningCount == other.runningCount &&
		listeningPortCount == other.listeningPortCount &&
		allocatedPortCount == other.allocatedPortCount &&
		activeAllocationCount == other.activeAllocationCount &&
		temporaryReservationCount == other.temporaryReservationCount &&
		registeredOnlyCount == other.registeredOnlyCount &&
		projects == other.projects &&
		allocations == other.allocations
	}
}
