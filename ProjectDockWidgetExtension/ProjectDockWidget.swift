import SwiftUI
import WidgetKit

private enum WidgetL10n {
#if APP_STORE
    private static let appGroup = "L4G2HAQ5B5.com.zhusi.projectdock.store"
#else
    private static let appGroup = "L4G2HAQ5B5.com.zhusi.projectdock"
#endif
    private static let locales = ["zh-Hans", "zh-Hant", "en", "ja", "ko", "de", "fr", "es", "pt-BR", "ru", "ar"]
    private static let values: [String: [String]] = [
        "displayName": ["ProjectDock 项目与端口", "ProjectDock 專案與連接埠", "ProjectDock Projects & Ports", "ProjectDock プロジェクトとポート", "ProjectDock 프로젝트 및 포트", "ProjectDock Projekte & Ports", "ProjectDock Projets et ports", "ProjectDock Proyectos y puertos", "ProjectDock Projetos e portas", "ProjectDock Проекты и порты", "مشاريع ومنافذ ProjectDock"],
        "description": ["查看本地项目、运行状态和持久端口资源池。", "查看本機專案、執行狀態和持久連接埠資源池。", "View local projects, runtime status, and persistent port allocations.", "ローカルプロジェクト、稼働状況、永続ポート割り当てを表示します。", "로컬 프로젝트, 실행 상태 및 영구 포트 할당을 확인합니다.", "Lokale Projekte, Laufzeitstatus und dauerhafte Portzuweisungen anzeigen.", "Affichez les projets locaux, leur état et les ports attribués.", "Vea proyectos locales, estado de ejecución y puertos persistentes.", "Veja projetos locais, status de execução e portas persistentes.", "Просмотр локальных проектов, состояния и назначенных портов.", "عرض المشاريع المحلية وحالة التشغيل وتخصيصات المنافذ."],
        "openApp": ["打开 APP 同步", "開啟 APP 同步", "Open app to sync", "アプリを開いて同期", "앱을 열어 동기화", "App zum Synchronisieren öffnen", "Ouvrir l’app pour synchroniser", "Abrir app para sincronizar", "Abrir app para sincronizar", "Открыть приложение для синхронизации", "افتح التطبيق للمزامنة"],
        "projects": ["项目", "專案", "Projects", "プロジェクト", "프로젝트", "Projekte", "Projets", "Proyectos", "Projetos", "Проекты", "المشاريع"],
        "running": ["运行", "執行", "Running", "実行中", "실행", "Läuft", "En cours", "En ejecución", "Em execução", "Работает", "قيد التشغيل"],
        "ports": ["端口", "連接埠", "Ports", "ポート", "포트", "Ports", "Ports", "Puertos", "Portas", "Порты", "المنافذ"],
        "allocated": ["已分配", "已分配", "Allocated", "割り当て済み", "할당됨", "Zugewiesen", "Attribués", "Asignados", "Alocadas", "Назначено", "مخصص"],
        "pending": ["待接入", "待接入", "Pending", "未設定", "대기", "Ausstehend", "En attente", "Pendientes", "Pendentes", "Ожидает", "قيد الانتظار"],
        "featured": ["重点项目", "重點專案", "Featured projects", "主要プロジェクト", "주요 프로젝트", "Wichtige Projekte", "Projets principaux", "Proyectos destacados", "Projetos em destaque", "Основные проекты", "المشاريع البارزة"],
        "noProjects": ["暂无项目", "暫無專案", "No projects", "プロジェクトなし", "프로젝트 없음", "Keine Projekte", "Aucun projet", "No hay proyectos", "Nenhum projeto", "Нет проектов", "لا توجد مشاريع"],
        "refresh": ["刷新", "重新整理", "Refresh", "更新", "새로고침", "Aktualisieren", "Actualiser", "Actualizar", "Atualizar", "Обновить", "تحديث"],
        "localConsole": ["本地开发控制台", "本機開發控制台", "Local development console", "ローカル開発コンソール", "로컬 개발 콘솔", "Lokale Entwicklungszentrale", "Console de développement locale", "Consola de desarrollo local", "Console de desenvolvimento local", "Локальная консоль разработки", "وحدة تحكم التطوير المحلية"],
        "synced": ["已同步", "已同步", "Synced", "同期済み", "동기화됨", "Synchronisiert", "Synchronisé", "Sincronizado", "Sincronizado", "Синхронизировано", "تمت المزامنة"],
        "stale": ["待刷新", "待重新整理", "Refresh needed", "更新が必要", "새로고침 필요", "Aktualisierung nötig", "Actualisation requise", "Requiere actualización", "Atualização necessária", "Требуется обновление", "يحتاج إلى تحديث"],
        "assignments": ["分配", "分配", "Assigned", "割り当て", "할당", "Zugewiesen", "Attribués", "Asignados", "Alocadas", "Назначено", "مخصص"],
        "active": ["活跃", "活躍", "Active", "アクティブ", "활성", "Aktiv", "Actifs", "Activos", "Ativas", "Активно", "نشط"],
        "temporary": ["临时", "臨時", "Temporary", "一時", "임시", "Temporär", "Temporaires", "Temporales", "Temporárias", "Временно", "مؤقت"],
        "pool": ["资源池", "資源池", "Port pool", "ポートプール", "포트 풀", "Port-Pool", "Pool de ports", "Grupo de puertos", "Pool de portas", "Пул портов", "مجموعة المنافذ"],
        "waiting": ["等待 APP 同步", "等待 APP 同步", "Waiting for app sync", "アプリの同期を待機中", "앱 동기화 대기 중", "Warten auf App-Synchronisierung", "En attente de synchronisation", "Esperando sincronización", "Aguardando sincronização", "Ожидание синхронизации", "بانتظار مزامنة التطبيق"],
        "noData": ["暂无数据", "暫無資料", "No data", "データなし", "데이터 없음", "Keine Daten", "Aucune donnée", "Sin datos", "Sem dados", "Нет данных", "لا توجد بيانات"],
        "listening": ["监听", "監聽", "Listening", "待受中", "수신 중", "Lauschend", "En écoute", "Escuchando", "Em escuta", "Слушает", "يستمع"],
        "conflict": ["冲突", "衝突", "Conflict", "競合", "충돌", "Konflikt", "Conflit", "Conflicto", "Conflito", "Конфликт", "تعارض"],
        "idle": ["空闲", "閒置", "Idle", "アイドル", "유휴", "Frei", "Inactif", "Inactivo", "Ocioso", "Свободен", "خامل"],
        "registered": ["已登记 · 启动待接入", "已登記 · 啟動待接入", "Registered · start pending", "登録済み · 起動未設定", "등록됨 · 시작 대기", "Registriert · Start ausstehend", "Enregistré · démarrage en attente", "Registrado · inicio pendiente", "Registrado · início pendente", "Зарегистрирован · запуск ожидает", "مسجل · التشغيل قيد الانتظار"],
        "unavailable": ["目录不可用", "目錄不可用", "Directory unavailable", "ディレクトリ利用不可", "디렉터리 사용 불가", "Ordner nicht verfügbar", "Dossier indisponible", "Directorio no disponible", "Diretório indisponível", "Каталог недоступен", "الدليل غير متاح"],
        "ready": ["可启动", "可啟動", "Ready", "起動可能", "시작 가능", "Bereit", "Prêt", "Listo", "Pronto", "Готов", "جاهز"],
        "local": ["本地", "本機", "Local", "ローカル", "로컬", "Lokal", "Local", "Local", "Local", "Локально", "محلي"]
    ]

    private static var locale: String {
        let stored = UserDefaults(suiteName: appGroup)?.string(forKey: "interfaceLocale") ?? Locale.preferredLanguages.first ?? "en"
        if stored.lowercased().hasPrefix("zh-hant") || stored.lowercased().hasPrefix("zh-tw") || stored.lowercased().hasPrefix("zh-hk") { return "zh-Hant" }
        if stored.lowercased().hasPrefix("zh") { return "zh-Hans" }
        if stored.lowercased().hasPrefix("pt") { return "pt-BR" }
        let short = stored.split(separator: "-").first.map(String.init)?.lowercased() ?? "en"
        return locales.contains(short) ? short : "en"
    }

    static func text(_ key: String) -> String {
        guard let row = values[key], let index = locales.firstIndex(of: locale), row.indices.contains(index) else { return key }
        return row[index]
    }
}

private func W(_ key: String) -> String { WidgetL10n.text(key) }

struct ProjectDockEntry: TimelineEntry {
    let date: Date
    let snapshot: ProjectDockWidgetSnapshot
}

struct ProjectDockProvider: TimelineProvider {
    func placeholder(in context: Context) -> ProjectDockEntry {
        ProjectDockEntry(
            date: .now,
            snapshot: ProjectDockWidgetSnapshot(
                schemaVersion: 2,
                updatedAt: ISO8601DateFormatter().string(from: .now),
                projectCount: 25,
                runningCount: 2,
                listeningPortCount: 12,
                allocatedPortCount: 6,
                activeAllocationCount: 3,
                temporaryReservationCount: 1,
                registeredOnlyCount: 18,
                projects: [
                    ProjectDockProjectSummary(name: "ProjectDock", state: "running", source: "codex", portCount: 1),
                    ProjectDockProjectSummary(name: "AI Workspace", state: "registered", source: "trae", portCount: 0),
                    ProjectDockProjectSummary(name: "DockVault", state: "ready", source: "codex", portCount: 2),
                ],
                allocations: [
                    ProjectDockAllocationSummary(port: 43110, projectName: "ProjectDock", state: "active"),
                    ProjectDockAllocationSummary(port: 4310, projectName: "DockVault", state: "idle"),
                    ProjectDockAllocationSummary(port: 4311, projectName: "DockVault", state: "active"),
                ]
            )
        )
    }

    func getSnapshot(in context: Context, completion: @escaping (ProjectDockEntry) -> Void) {
        completion(ProjectDockEntry(date: .now, snapshot: ProjectDockSnapshotStore.load()))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<ProjectDockEntry>) -> Void) {
        let entry = ProjectDockEntry(date: .now, snapshot: ProjectDockSnapshotStore.load())
        let next = Calendar.current.date(byAdding: .minute, value: 15, to: .now) ?? .now.addingTimeInterval(900)
        completion(Timeline(entries: [entry], policy: .after(next)))
    }
}

struct ProjectDockStatusWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: ProjectDockWidgetKind.status, provider: ProjectDockProvider()) { entry in
            ProjectDockWidgetView(entry: entry)
                .containerBackground(for: .widget) {
                    ProjectDockPalette.background
                }
        }
        .configurationDisplayName(W("displayName"))
        .description(W("description"))
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
        .contentMarginsDisabled()
    }
}

private struct ProjectDockWidgetView: View {
    @Environment(\.widgetFamily) private var family
    let entry: ProjectDockEntry

    var body: some View {
        Group {
            if family == .systemLarge {
                largeView
            } else if family == .systemMedium {
                mediumView
            } else {
                smallView
            }
        }
        .padding(14)
        .widgetURL(ProjectDockWidgetConfiguration.routeURL("projects"))
    }

    private var smallView: some View {
        VStack(alignment: .leading, spacing: 10) {
            smallHeader
            if entry.snapshot.isPlaceholder {
                Spacer()
                Label(W("openApp"), systemImage: "arrow.triangle.2.circlepath")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(ProjectDockPalette.muted)
                Spacer()
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 5) {
                    Text("\(entry.snapshot.projectCount)")
                        .font(.system(size: 34, weight: .bold, design: .rounded))
                        .foregroundStyle(ProjectDockPalette.ink)
                    Text(W("projects"))
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(ProjectDockPalette.muted)
                }
                HStack(spacing: 8) {
                    compactMetric(
                        value: entry.snapshot.runningCount,
                        label: W("running"),
                        icon: "play.fill",
                        color: ProjectDockPalette.green
                    )
                    compactMetric(
                        value: entry.snapshot.listeningPortCount,
                        label: W("ports"),
                        icon: "network",
                        color: ProjectDockPalette.cyan
                    )
                }
            }
            Spacer(minLength: 0)
            footer
        }
    }

    private var mediumView: some View {
        HStack(spacing: 14) {
            VStack(alignment: .leading, spacing: 10) {
                header
                HStack(spacing: 8) {
                    metric(value: entry.snapshot.projectCount, label: W("projects"), color: ProjectDockPalette.ink)
                    metric(value: entry.snapshot.runningCount, label: W("running"), color: ProjectDockPalette.green)
                    metric(value: entry.snapshot.listeningPortCount, label: W("ports"), color: ProjectDockPalette.cyan)
                }
                HStack(spacing: 10) {
                    Label("\(entry.snapshot.allocatedPortCount) \(W("allocated"))", systemImage: "square.stack.3d.up.fill")
                    Label("\(entry.snapshot.registeredOnlyCount) \(W("pending"))", systemImage: "checkmark.seal.fill")
                }
                .font(.system(size: 10, weight: .semibold))
                .foregroundStyle(ProjectDockPalette.muted)
                footer
            }
            .frame(maxWidth: 176, alignment: .leading)

            Rectangle()
                .fill(ProjectDockPalette.border)
                .frame(width: 1)

            VStack(alignment: .leading, spacing: 7) {
                Text(W("featured"))
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.muted)
                if entry.snapshot.projects.isEmpty {
                    Spacer()
                    Text(entry.snapshot.isPlaceholder ? W("waiting") : W("noProjects"))
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(ProjectDockPalette.muted)
                    Spacer()
                } else {
                    ForEach(entry.snapshot.projects.prefix(3)) { project in
                        projectRow(project)
                    }
                    Spacer(minLength: 0)
                }
                Link(destination: ProjectDockWidgetConfiguration.refreshURL) {
                    Label(W("refresh"), systemImage: "arrow.clockwise")
                        .font(.system(size: 10, weight: .bold))
                        .foregroundStyle(ProjectDockPalette.cyan)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var largeView: some View {
        ZStack(alignment: .topTrailing) {
            Circle()
                .fill(ProjectDockPalette.cyan.opacity(0.08))
                .frame(width: 180, height: 180)
                .blur(radius: 38)
                .offset(x: 72, y: -92)
                .accessibilityHidden(true)

            VStack(alignment: .leading, spacing: 10) {
                largeHeader
                largeOverview

                HStack(alignment: .top, spacing: 10) {
                    largeProjectPanel
                    largePortPanel
                }
                .frame(maxHeight: .infinity)

                largeFooter
            }
        }
        .dynamicTypeSize(.small ... .large)
    }

    private var largeHeader: some View {
        HStack(spacing: 8) {
            ZStack {
                RoundedRectangle(cornerRadius: 7)
                    .fill(ProjectDockPalette.cyan.opacity(0.15))
                Image(systemName: "shippingbox.fill")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.cyan)
            }
            .frame(width: 26, height: 26)

            VStack(alignment: .leading, spacing: 0) {
                Text("ProjectDock")
                    .font(.system(size: 12, weight: .bold, design: .rounded))
                    .foregroundStyle(ProjectDockPalette.ink)
                Text(W("localConsole"))
                    .font(.system(size: 8, weight: .medium))
                    .foregroundStyle(ProjectDockPalette.muted)
            }

            Spacer(minLength: 8)

            HStack(spacing: 5) {
                Circle()
                    .fill(snapshotIsStale ? ProjectDockPalette.amber : ProjectDockPalette.green)
                    .frame(width: 6, height: 6)
                Text(snapshotIsStale ? W("stale") : W("synced"))
                    .font(.system(size: 9, weight: .semibold))
                    .foregroundStyle(snapshotIsStale ? ProjectDockPalette.amber : ProjectDockPalette.muted)
                    .lineLimit(1)
            }
            .accessibilityElement(children: .combine)
        }
        .frame(height: 28)
    }

    private var largeOverview: some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .firstTextBaseline, spacing: 4) {
                    Text("\(entry.snapshot.projectCount)")
                        .font(.system(size: 30, weight: .bold, design: .rounded))
                        .foregroundStyle(ProjectDockPalette.ink)
                        .lineLimit(1)
                        .minimumScaleFactor(0.75)
                Text(W("projects"))
                        .font(.system(size: 10, weight: .bold))
                        .foregroundStyle(ProjectDockPalette.cyan)
                }
                Text(W("projects"))
                    .font(.system(size: 9, weight: .semibold))
                    .foregroundStyle(ProjectDockPalette.muted)
                    .lineLimit(1)
            }
            .frame(width: 72, alignment: .leading)

            Rectangle()
                .fill(ProjectDockPalette.border)
                .frame(width: 1, height: 40)

            HStack(spacing: 4) {
                overviewStat(
                    value: entry.snapshot.runningCount,
                    label: W("running"),
                    icon: "play.fill",
                    color: ProjectDockPalette.green
                )
                overviewStat(
                    value: entry.snapshot.allocatedPortCount,
                    label: W("assignments"),
                    icon: "square.stack.3d.up.fill",
                    color: ProjectDockPalette.cyan
                )
                overviewStat(
                    value: entry.snapshot.activeAllocationCount,
                    label: W("active"),
                    icon: "network",
                    color: ProjectDockPalette.green
                )
            }
            .frame(maxWidth: .infinity)
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 9)
        .background {
            RoundedRectangle(cornerRadius: 13)
                .fill(
                    LinearGradient(
                        colors: [
                            ProjectDockPalette.cyan.opacity(0.13),
                            ProjectDockPalette.panelElevated,
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
        }
        .overlay {
            RoundedRectangle(cornerRadius: 13)
                .stroke(ProjectDockPalette.cyan.opacity(0.18), lineWidth: 0.75)
        }
    }

    private var largeProjectPanel: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(spacing: 5) {
                Image(systemName: "square.grid.2x2")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.cyan)
                Text(W("projects"))
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.ink)
                Spacer(minLength: 4)
                Text("\(entry.snapshot.registeredOnlyCount) \(W("pending"))")
                    .font(.system(size: 8, weight: .medium))
                    .foregroundStyle(ProjectDockPalette.muted)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
            }

            Rectangle()
                .fill(ProjectDockPalette.border)
                .frame(height: 1)

            if entry.snapshot.projects.isEmpty {
                largeEmpty
            } else {
                ForEach(entry.snapshot.projects.prefix(4)) { project in
                    largeProjectRow(project)
                }
                Spacer(minLength: 0)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(ProjectDockPalette.panel, in: RoundedRectangle(cornerRadius: 12))
        .overlay {
            RoundedRectangle(cornerRadius: 12)
                .stroke(ProjectDockPalette.border, lineWidth: 0.6)
        }
    }

    private var largePortPanel: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(spacing: 5) {
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.cyan)
                Text(W("ports"))
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(ProjectDockPalette.ink)
                Spacer(minLength: 4)
                Text("\(entry.snapshot.temporaryReservationCount) \(W("temporary"))")
                    .font(.system(size: 8, weight: .medium))
                    .foregroundStyle(ProjectDockPalette.muted)
                    .lineLimit(1)
            }

            Rectangle()
                .fill(ProjectDockPalette.border)
                .frame(height: 1)

            if entry.snapshot.allocations.isEmpty {
                largeEmpty
            } else {
                ForEach(entry.snapshot.allocations.prefix(4)) { allocation in
                    largeAllocationRow(allocation)
                }
                Spacer(minLength: 0)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(ProjectDockPalette.panel, in: RoundedRectangle(cornerRadius: 12))
        .overlay {
            RoundedRectangle(cornerRadius: 12)
                .stroke(ProjectDockPalette.border, lineWidth: 0.6)
        }
    }

    private var largeFooter: some View {
        HStack(spacing: 8) {
            footer
            Spacer(minLength: 8)
            Link(destination: ProjectDockWidgetConfiguration.refreshURL) {
                Image(systemName: "arrow.clockwise")
                    .frame(width: 20, height: 20)
                    .background(ProjectDockPalette.panel, in: Circle())
                    .accessibilityLabel(W("refresh"))
            }
            Link(destination: ProjectDockWidgetConfiguration.routeURL("ports")) {
                HStack(spacing: 4) {
                    Text(W("pool"))
                    Image(systemName: "arrow.up.right")
                }
                .padding(.horizontal, 8)
                .frame(height: 20)
                .background(ProjectDockPalette.cyan.opacity(0.12), in: Capsule())
                .accessibilityLabel(W("pool"))
            }
        }
        .font(.system(size: 9, weight: .bold))
        .foregroundStyle(ProjectDockPalette.cyan)
        .frame(height: 20)
    }

    private func overviewStat(value: Int, label: String, icon: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: icon)
                    .font(.system(size: 8, weight: .bold))
                    .foregroundStyle(color)
                Text("\(value)")
                    .font(.system(size: 16, weight: .bold, design: .rounded))
                    .foregroundStyle(ProjectDockPalette.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(0.75)
            }
            Text(label)
                .font(.system(size: 8, weight: .semibold))
                .foregroundStyle(ProjectDockPalette.muted)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    private func largeProjectRow(_ project: ProjectDockProjectSummary) -> some View {
        HStack(spacing: 7) {
            ZStack {
                Circle()
                    .fill(stateColor(project.state).opacity(0.12))
                Image(systemName: stateIcon(project.state))
                    .font(.system(size: 8, weight: .bold))
                    .foregroundStyle(stateColor(project.state))
            }
            .frame(width: 22, height: 22)

            VStack(alignment: .leading, spacing: 1) {
                Text(project.name)
                    .font(.system(size: 9, weight: .semibold))
                    .foregroundStyle(ProjectDockPalette.ink)
                    .lineLimit(1)
                    .truncationMode(.tail)
                Text("\(sourceLabel(project.source)) · \(compactStateLabel(project.state))")
                    .font(.system(size: 7.5, weight: .medium))
                    .foregroundStyle(ProjectDockPalette.muted)
                    .lineLimit(1)
            }
        }
        .frame(minHeight: 25)
        .accessibilityElement(children: .combine)
    }

    private func largeAllocationRow(_ allocation: ProjectDockAllocationSummary) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 5) {
                Text(":\(allocation.port)")
                    .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                    .foregroundStyle(ProjectDockPalette.cyan)
                    .lineLimit(1)
                    .minimumScaleFactor(0.75)
                Spacer(minLength: 4)
                HStack(spacing: 3) {
                    Circle()
                        .fill(allocationStateColor(allocation.state))
                        .frame(width: 5, height: 5)
                    Text(allocationStateLabel(allocation.state))
                        .lineLimit(1)
                }
                .font(.system(size: 7.5, weight: .semibold))
                .foregroundStyle(ProjectDockPalette.muted)
            }
            Text(allocation.projectName)
                .font(.system(size: 8, weight: .medium))
                .foregroundStyle(ProjectDockPalette.ink.opacity(0.88))
                .lineLimit(1)
                .truncationMode(.tail)
        }
        .frame(minHeight: 25)
        .accessibilityElement(children: .combine)
    }

    private func allocationStateLabel(_ state: String) -> String {
        switch state {
        case "active": W("listening")
        case "conflict": W("conflict")
        default: W("idle")
        }
    }

    private func allocationStateColor(_ state: String) -> Color {
        switch state {
        case "active": ProjectDockPalette.green
        case "conflict": ProjectDockPalette.amber
        default: ProjectDockPalette.muted
        }
    }

    private func compactStateLabel(_ state: String) -> String {
        switch state {
        case "running": W("running")
        case "registered": W("pending")
        case "conflict": W("conflict")
        case "unavailable": W("unavailable")
        default: W("ready")
        }
    }

    private var header: some View {
        HStack(spacing: 7) {
            Image(systemName: "shippingbox.fill")
                .font(.system(size: 13, weight: .bold))
                .foregroundStyle(ProjectDockPalette.cyan)
            Text("PROJECTDOCK")
                .font(.system(size: 11, weight: .bold, design: .rounded))
                .foregroundStyle(ProjectDockPalette.ink)
            Spacer()
            Circle()
                .fill(snapshotIsStale ? ProjectDockPalette.amber : ProjectDockPalette.green)
                .frame(width: 7, height: 7)
                .accessibilityLabel(snapshotIsStale ? W("stale") : W("synced"))
        }
    }

    private var smallHeader: some View {
        HStack(spacing: 7) {
            Image(systemName: "shippingbox.fill")
                .font(.system(size: 13, weight: .bold))
                .foregroundStyle(ProjectDockPalette.cyan)
            Text("DOCK")
                .font(.system(size: 11, weight: .bold, design: .rounded))
                .foregroundStyle(ProjectDockPalette.ink)
            Spacer()
            Circle()
                .fill(snapshotIsStale ? ProjectDockPalette.amber : ProjectDockPalette.green)
                .frame(width: 7, height: 7)
                .accessibilityLabel(snapshotIsStale ? W("stale") : W("synced"))
        }
    }

    private var footer: some View {
        Group {
            if let updatedDate = entry.snapshot.updatedDate {
                HStack(spacing: 4) {
                    Text(snapshotIsStale ? W("stale") : W("synced"))
                    Text(updatedDate, style: .relative)
                }
            } else {
                Text(W("waiting"))
            }
        }
        .font(.system(size: 9, weight: .medium, design: .rounded))
        .foregroundStyle(snapshotIsStale ? ProjectDockPalette.amber : ProjectDockPalette.muted)
    }

    private var snapshotIsStale: Bool {
        guard let updatedDate = entry.snapshot.updatedDate else {
            return true
        }
        return entry.date.timeIntervalSince(updatedDate) > 300
    }

    private func compactMetric(value: Int, label: String, icon: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: icon)
                    .font(.system(size: 8, weight: .bold))
                Text("\(value)")
                    .font(.system(size: 14, weight: .bold, design: .rounded))
            }
            Text(label)
                .font(.system(size: 9, weight: .semibold))
        }
        .foregroundStyle(color)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 6)
        .padding(.vertical, 6)
        .background(ProjectDockPalette.panel, in: RoundedRectangle(cornerRadius: 8))
        .overlay {
            RoundedRectangle(cornerRadius: 8)
                .stroke(ProjectDockPalette.border, lineWidth: 0.5)
        }
    }

    private func metric(value: Int, label: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 1) {
            Text("\(value)")
                .font(.system(size: 23, weight: .bold, design: .rounded))
                .foregroundStyle(color)
            Text(label)
                .font(.system(size: 9, weight: .semibold))
                .foregroundStyle(ProjectDockPalette.muted)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var largeEmpty: some View {
        Text(entry.snapshot.isPlaceholder ? W("waiting") : W("noData"))
            .font(.system(size: 10, weight: .medium))
            .foregroundStyle(ProjectDockPalette.muted)
            .frame(maxWidth: .infinity, minHeight: 80, alignment: .center)
    }

    private func projectRow(_ project: ProjectDockProjectSummary) -> some View {
        HStack(spacing: 7) {
            Image(systemName: stateIcon(project.state))
                .font(.system(size: 9, weight: .bold))
                .foregroundStyle(stateColor(project.state))
                .frame(width: 12)
            VStack(alignment: .leading, spacing: 1) {
                Text(project.name)
                    .font(.system(size: 10, weight: .semibold))
                    .foregroundStyle(ProjectDockPalette.ink)
                    .lineLimit(1)
                Text("\(sourceLabel(project.source)) · \(stateLabel(project.state))")
                    .font(.system(size: 8, weight: .medium))
                    .foregroundStyle(ProjectDockPalette.muted)
                    .lineLimit(1)
            }
        }
    }

    private func stateLabel(_ state: String) -> String {
        switch state {
        case "running": W("running")
        case "registered": W("registered")
        case "conflict": W("conflict")
        case "unavailable": W("unavailable")
        default: W("ready")
        }
    }

    private func stateIcon(_ state: String) -> String {
        switch state {
        case "running": "play.circle.fill"
        case "registered": "checkmark.seal.fill"
        case "unavailable": "exclamationmark.triangle.fill"
        case "conflict": "exclamationmark.octagon.fill"
        default: "checkmark.circle.fill"
        }
    }

    private func stateColor(_ state: String) -> Color {
        switch state {
        case "running": ProjectDockPalette.green
        case "registered": ProjectDockPalette.muted
        case "unavailable": ProjectDockPalette.amber
        case "conflict": ProjectDockPalette.amber
        default: ProjectDockPalette.cyan
        }
    }

    private func sourceLabel(_ source: String) -> String {
        switch source.lowercased() {
        case "codex": "CODEX"
        case "trae": "TRAE"
        case "claude": "CLAUDE"
        default: W("local")
        }
    }
}

private enum ProjectDockPalette {
    static let background = Color(red: 0.035, green: 0.075, blue: 0.09)
    static let panel = Color.white.opacity(0.055)
    static let panelElevated = Color.white.opacity(0.075)
    static let border = Color.white.opacity(0.12)
    static let ink = Color(red: 0.92, green: 0.97, blue: 0.97)
    static let muted = Color(red: 0.56, green: 0.68, blue: 0.69)
    static let cyan = Color(red: 0.28, green: 0.86, blue: 0.83)
    static let green = Color(red: 0.46, green: 0.88, blue: 0.57)
    static let amber = Color(red: 0.98, green: 0.72, blue: 0.32)
}
