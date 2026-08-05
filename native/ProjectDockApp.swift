import AppKit
import Foundation
import ServiceManagement
import WebKit
import WidgetKit

#if APP_STORE
private let projectDockPort = 43111
private let projectDockAppGroup = "L4G2HAQ5B5.com.zhusi.projectdock.store"
#else
private let projectDockPort = 43110
private let projectDockAppGroup = "L4G2HAQ5B5.com.zhusi.projectdock"
#endif
private let projectDockURL = URL(string: "http://127.0.0.1:\(projectDockPort)")!

private enum NativeL10n {
    private static let translations: [String: [String: String]] = [
        "zh-Hans": ["about": "关于 ProjectDock", "refreshWidget": "刷新桌面组件", "launchAtLogin": "开机自动启动", "quit": "退出 ProjectDock", "enabled": "已开启 · 登录后在后台运行", "approval": "等待在系统设置中允许", "disabled": "已关闭", "notFound": "已关闭 · 可在此开启", "unavailable": "状态暂不可用", "confirmAction": "请确认操作", "confirm": "确认", "cancel": "取消", "startupFailed": "ProjectDock 启动失败", "quitButton": "退出"],
        "zh-Hant": ["about": "關於 ProjectDock", "refreshWidget": "重新整理桌面小工具", "launchAtLogin": "登入時自動啟動", "quit": "結束 ProjectDock", "enabled": "已開啟 · 登入後在背景執行", "approval": "等待在系統設定中允許", "disabled": "已關閉", "notFound": "已關閉 · 可在此開啟", "unavailable": "狀態暫不可用", "confirmAction": "請確認操作", "confirm": "確認", "cancel": "取消", "startupFailed": "ProjectDock 啟動失敗", "quitButton": "結束"],
        "en": ["about": "About ProjectDock", "refreshWidget": "Refresh Widgets", "launchAtLogin": "Launch at Login", "quit": "Quit ProjectDock", "enabled": "Enabled · runs in the background after login", "approval": "Waiting for approval in System Settings", "disabled": "Disabled", "notFound": "Disabled · enable it here", "unavailable": "Status unavailable", "confirmAction": "Confirm Action", "confirm": "Confirm", "cancel": "Cancel", "startupFailed": "ProjectDock Failed to Start", "quitButton": "Quit"],
        "ja": ["about": "ProjectDock について", "refreshWidget": "ウィジェットを更新", "launchAtLogin": "ログイン時に起動", "quit": "ProjectDock を終了", "enabled": "有効 · ログイン後にバックグラウンドで実行", "approval": "システム設定での許可を待っています", "disabled": "無効", "notFound": "無効 · ここで有効にできます", "unavailable": "状態を取得できません", "confirmAction": "操作を確認", "confirm": "確認", "cancel": "キャンセル", "startupFailed": "ProjectDock の起動に失敗", "quitButton": "終了"],
        "ko": ["about": "ProjectDock 정보", "refreshWidget": "위젯 새로고침", "launchAtLogin": "로그인 시 실행", "quit": "ProjectDock 종료", "enabled": "켜짐 · 로그인 후 백그라운드에서 실행", "approval": "시스템 설정에서 승인을 기다리는 중", "disabled": "꺼짐", "notFound": "꺼짐 · 여기에서 켤 수 있음", "unavailable": "상태를 사용할 수 없음", "confirmAction": "작업 확인", "confirm": "확인", "cancel": "취소", "startupFailed": "ProjectDock 시작 실패", "quitButton": "종료"],
        "de": ["about": "Über ProjectDock", "refreshWidget": "Widgets aktualisieren", "launchAtLogin": "Bei Anmeldung starten", "quit": "ProjectDock beenden", "enabled": "Aktiviert · läuft nach Anmeldung im Hintergrund", "approval": "Wartet auf Erlaubnis in den Systemeinstellungen", "disabled": "Deaktiviert", "notFound": "Deaktiviert · hier aktivieren", "unavailable": "Status nicht verfügbar", "confirmAction": "Aktion bestätigen", "confirm": "Bestätigen", "cancel": "Abbrechen", "startupFailed": "ProjectDock konnte nicht gestartet werden", "quitButton": "Beenden"],
        "fr": ["about": "À propos de ProjectDock", "refreshWidget": "Actualiser les widgets", "launchAtLogin": "Lancer à l’ouverture de session", "quit": "Quitter ProjectDock", "enabled": "Activé · s’exécute en arrière-plan après connexion", "approval": "En attente d’autorisation dans Réglages Système", "disabled": "Désactivé", "notFound": "Désactivé · activer ici", "unavailable": "État indisponible", "confirmAction": "Confirmer l’action", "confirm": "Confirmer", "cancel": "Annuler", "startupFailed": "Échec du démarrage de ProjectDock", "quitButton": "Quitter"],
        "es": ["about": "Acerca de ProjectDock", "refreshWidget": "Actualizar widgets", "launchAtLogin": "Abrir al iniciar sesión", "quit": "Salir de ProjectDock", "enabled": "Activado · se ejecuta en segundo plano tras iniciar sesión", "approval": "Esperando permiso en Ajustes del Sistema", "disabled": "Desactivado", "notFound": "Desactivado · activar aquí", "unavailable": "Estado no disponible", "confirmAction": "Confirmar acción", "confirm": "Confirmar", "cancel": "Cancelar", "startupFailed": "ProjectDock no pudo iniciarse", "quitButton": "Salir"],
        "pt-BR": ["about": "Sobre o ProjectDock", "refreshWidget": "Atualizar widgets", "launchAtLogin": "Abrir ao iniciar sessão", "quit": "Sair do ProjectDock", "enabled": "Ativado · executa em segundo plano após iniciar sessão", "approval": "Aguardando permissão nos Ajustes do Sistema", "disabled": "Desativado", "notFound": "Desativado · ative aqui", "unavailable": "Status indisponível", "confirmAction": "Confirmar ação", "confirm": "Confirmar", "cancel": "Cancelar", "startupFailed": "Falha ao iniciar o ProjectDock", "quitButton": "Sair"],
        "ru": ["about": "О ProjectDock", "refreshWidget": "Обновить виджеты", "launchAtLogin": "Запускать при входе", "quit": "Выйти из ProjectDock", "enabled": "Включено · работает в фоне после входа", "approval": "Ожидание разрешения в настройках системы", "disabled": "Выключено", "notFound": "Выключено · включите здесь", "unavailable": "Статус недоступен", "confirmAction": "Подтвердите действие", "confirm": "Подтвердить", "cancel": "Отмена", "startupFailed": "Не удалось запустить ProjectDock", "quitButton": "Выйти"],
        "ar": ["about": "حول ProjectDock", "refreshWidget": "تحديث الأدوات", "launchAtLogin": "التشغيل عند تسجيل الدخول", "quit": "إنهاء ProjectDock", "enabled": "مفعّل · يعمل في الخلفية بعد تسجيل الدخول", "approval": "بانتظار السماح في إعدادات النظام", "disabled": "معطّل", "notFound": "معطّل · يمكن تفعيله هنا", "unavailable": "الحالة غير متاحة", "confirmAction": "تأكيد الإجراء", "confirm": "تأكيد", "cancel": "إلغاء", "startupFailed": "فشل تشغيل ProjectDock", "quitButton": "إنهاء"]
    ]

    static func normalize(_ value: String?) -> String {
        let raw = (value ?? "").replacingOccurrences(of: "_", with: "-")
        if raw.lowercased().hasPrefix("zh-hant") || raw.lowercased().hasPrefix("zh-tw") || raw.lowercased().hasPrefix("zh-hk") { return "zh-Hant" }
        if raw.lowercased().hasPrefix("zh") { return "zh-Hans" }
        if raw.lowercased().hasPrefix("pt") { return "pt-BR" }
        let short = raw.split(separator: "-").first.map(String.init)?.lowercased() ?? "en"
        return translations[short] == nil ? "en" : short
    }

    static func initialLocale() -> String {
        if let saved = UserDefaults(suiteName: projectDockAppGroup)?.string(forKey: "interfaceLocale") { return normalize(saved) }
        return normalize(Locale.preferredLanguages.first)
    }

    static func save(_ locale: String) {
        UserDefaults(suiteName: projectDockAppGroup)?.set(normalize(locale), forKey: "interfaceLocale")
    }

    static func text(_ key: String, locale: String) -> String {
        translations[normalize(locale)]?[key] ?? translations["en"]?[key] ?? key
    }
}

final class DropWebView: WKWebView {
    override init(frame: NSRect, configuration: WKWebViewConfiguration) {
        super.init(frame: frame, configuration: configuration)
        registerForDraggedTypes([.fileURL])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    override func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
        folderPaths(from: sender).isEmpty ? [] : .copy
    }

    override func draggingUpdated(_ sender: NSDraggingInfo) -> NSDragOperation {
        folderPaths(from: sender).isEmpty ? [] : .copy
    }

    override func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
        let paths = folderPaths(from: sender)
        guard !paths.isEmpty,
              let data = try? JSONSerialization.data(withJSONObject: paths),
              let json = String(data: data, encoding: .utf8)
        else {
            return false
        }
        evaluateJavaScript("window.projectDockReceiveDroppedPaths?.(\(json));")
        return true
    }

    private func folderPaths(from sender: NSDraggingInfo) -> [String] {
        let options: [NSPasteboard.ReadingOptionKey: Any] = [.urlReadingFileURLsOnly: true]
        let values = sender.draggingPasteboard.readObjects(forClasses: [NSURL.self], options: options) as? [URL] ?? []
        return values.compactMap { url in
            var isDirectory: ObjCBool = false
            guard url.isFileURL,
                  FileManager.default.fileExists(atPath: url.path, isDirectory: &isDirectory),
                  isDirectory.boolValue
            else {
                return nil
            }
            return url.standardizedFileURL.path
        }
    }
}

@main
final class ProjectDockApp: NSObject, NSApplicationDelegate, NSWindowDelegate, WKUIDelegate, WKScriptMessageHandler {
    private var window: NSWindow?
    private var webView: DropWebView?
    private var serviceProcess: Process?
    private var snapshotTimer: Timer?
    private var loginItemMenuItem: NSMenuItem?
    private var serviceReady = false
    private var isConnecting = false
    private var isTerminating = false
    private var shouldShowWindowWhenActivated = !ProcessInfo.processInfo.arguments.contains("--background")
    private var reconnectWorkItem: DispatchWorkItem?
    private var interfaceLocale = NativeL10n.initialLocale()

    private func localized(_ key: String) -> String { NativeL10n.text(key, locale: interfaceLocale) }

    static func main() {
        let application = NSApplication.shared
        let delegate = ProjectDockApp()
        application.delegate = delegate
        application.setActivationPolicy(.accessory)
        withExtendedLifetime(delegate) {
            application.run()
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        installMainMenu()
        connectToProjectDock()
    }

    func applicationDidBecomeActive(_ notification: Notification) {
        guard shouldShowWindowWhenActivated else {
            moveToBackgroundIfNoVisibleWindows()
            return
        }
        shouldShowWindowWhenActivated = false
        showWindow()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        shouldShowWindowWhenActivated = false
        showWindow()
        return true
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        for url in urls where url.scheme?.lowercased() == "projectdock" {
            handleProjectDockURL(url)
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        isTerminating = true
        reconnectWorkItem?.cancel()
        snapshotTimer?.invalidate()
        webView?.configuration.userContentController.removeScriptMessageHandler(forName: "projectDockNative")
        guard let serviceProcess, serviceProcess.isRunning else {
            return
        }
        serviceProcess.terminate()
    }

    private func installMainMenu() {
        let mainMenu = NSMenu()
        let appItem = NSMenuItem()
        mainMenu.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: localized("about"), action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        let refreshItem = NSMenuItem(title: localized("refreshWidget"), action: #selector(refreshWidgetFromMenu), keyEquivalent: "r")
        refreshItem.target = self
        appMenu.addItem(refreshItem)
        let loginItem = NSMenuItem(title: localized("launchAtLogin"), action: #selector(toggleLaunchAtLoginFromMenu), keyEquivalent: "")
        loginItem.target = self
        appMenu.addItem(loginItem)
        loginItemMenuItem = loginItem
        updateLoginItemControls()
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: localized("quit"), action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        NSApplication.shared.mainMenu = mainMenu
    }

    @objc
    private func refreshWidgetFromMenu() {
        refreshWidgetSnapshot()
    }

    @objc
    private func toggleLaunchAtLoginFromMenu() {
        setLaunchAtLogin(!loginItemSelected)
    }

    private func showWindow(route: String = "dashboard") {
        if window == nil {
            createWindow()
        }
        NSApplication.shared.setActivationPolicy(.regular)
        window?.makeKeyAndOrderFront(nil)
        NSApplication.shared.activate(ignoringOtherApps: true)
        if serviceReady {
            loadInterface(route: route)
        }
    }

    private func createWindow() {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .default()
        configuration.userContentController.add(self, name: "projectDockNative")
        let webView = DropWebView(frame: .zero, configuration: configuration)
        webView.setValue(false, forKey: "drawsBackground")
        webView.uiDelegate = self

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1280, height: 820),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = "ProjectDock"
        window.minSize = NSSize(width: 820, height: 620)
        window.isReleasedWhenClosed = false
        window.delegate = self
        window.center()
        window.contentView = webView
        self.window = window
        self.webView = webView
    }

    func windowWillClose(_ notification: Notification) {
        moveToBackgroundIfNoVisibleWindows()
    }

    private func moveToBackgroundIfNoVisibleWindows() {
        DispatchQueue.main.async {
            guard NSApplication.shared.windows.allSatisfy({ !$0.isVisible }) else {
                return
            }
            NSApplication.shared.setActivationPolicy(.accessory)
        }
    }

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.name == "projectDockNative",
              let payload = message.body as? [String: Any],
              let action = payload["action"] as? String
        else {
            return
        }
        switch action {
        case "getLaunchAtLogin":
            sendLoginItemState()
        case "setLaunchAtLogin":
            guard let enabled = payload["enabled"] as? Bool else {
                sendLoginItemState(error: "开机启动选项格式无效。")
                return
            }
            setLaunchAtLogin(enabled)
        case "openLoginItemsSettings":
            SMAppService.openSystemSettingsLoginItems()
        case "setLocale":
            guard let locale = payload["locale"] as? String else { return }
            interfaceLocale = NativeL10n.normalize(locale)
            NativeL10n.save(interfaceLocale)
            installMainMenu()
            sendLoginItemState()
            WidgetCenter.shared.reloadAllTimelines()
        case "pickDirectory":
            guard let requestID = payload["requestId"] as? String else { return }
            pickAuthorizedDirectory(requestID: requestID, purpose: payload["purpose"] as? String ?? "project")
        default:
            break
        }
    }

    private func pickAuthorizedDirectory(requestID: String, purpose: String) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = purpose == "install"
        panel.prompt = localized("confirm")
        switch purpose {
        case "install": panel.message = "选择 GitHub 项目的安装目录"
        case "scan": panel.message = "选择 ProjectDock 要扫描的父目录"
        default: panel.message = "选择要由 ProjectDock 管理的项目目录"
        }
        let completion: (NSApplication.ModalResponse) -> Void = { [weak self] response in
            guard let self else { return }
            guard response == .OK, let url = panel.url else {
                self.sendDirectoryPickerResult(requestID: requestID, error: "已取消选择。")
                return
            }
            do {
                let bookmark = try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
                self.authorizeDirectory(bookmark: bookmark, requestID: requestID)
            } catch {
                self.sendDirectoryPickerResult(requestID: requestID, error: error.localizedDescription)
            }
        }
        if let window {
            panel.beginSheetModal(for: window, completionHandler: completion)
        } else {
            completion(panel.runModal())
        }
    }

    private func authorizeDirectory(bookmark: Data, requestID: String) {
        URLSession.shared.dataTask(with: projectDockURL.appending(path: "api/session")) { [weak self] data, response, error in
            guard let self else { return }
            guard error == nil,
                  (response as? HTTPURLResponse)?.statusCode == 200,
                  let data,
                  let session = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let token = session["token"] as? String
            else {
                DispatchQueue.main.async { self.sendDirectoryPickerResult(requestID: requestID, error: "无法获取本地授权会话。") }
                return
            }
            var request = URLRequest(url: projectDockURL.appending(path: "api/directories/authorize"))
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.setValue(token, forHTTPHeaderField: "X-ProjectDock-Token")
            request.httpBody = try? JSONSerialization.data(withJSONObject: ["bookmark": bookmark.base64EncodedString()])
            URLSession.shared.dataTask(with: request) { data, response, error in
                var path: String?
                var message: String?
                if let data, let payload = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    path = payload["path"] as? String
                    message = (payload["error"] as? [String: Any])?["message"] as? String
                }
                DispatchQueue.main.async {
                    if error == nil, (response as? HTTPURLResponse)?.statusCode == 200, let path {
                        self.sendDirectoryPickerResult(requestID: requestID, path: path)
                    } else {
                        self.sendDirectoryPickerResult(requestID: requestID, error: message ?? error?.localizedDescription ?? "目录授权失败。")
                    }
                }
            }.resume()
        }.resume()
    }

    private func sendDirectoryPickerResult(requestID: String, path: String? = nil, error: String? = nil) {
        var payload: [String: Any] = ["kind": "directoryPicker", "requestId": requestID]
        if let path { payload["path"] = path }
        if let error { payload["error"] = error }
        guard let data = try? JSONSerialization.data(withJSONObject: payload),
              let json = String(data: data, encoding: .utf8)
        else { return }
        webView?.evaluateJavaScript("window.projectDockReceiveNativeState?.(\(json));")
    }

    private var loginItemSelected: Bool {
        switch SMAppService.mainApp.status {
        case .enabled, .requiresApproval:
            true
        case .notRegistered, .notFound:
            false
        @unknown default:
            false
        }
    }

    private func setLaunchAtLogin(_ enabled: Bool) {
        let service = SMAppService.mainApp
        do {
            if enabled {
                if service.status == .notRegistered || service.status == .notFound {
                    try service.register()
                }
            } else if service.status != .notRegistered {
                try service.unregister()
            }
            updateLoginItemControls()
            sendLoginItemState()
        } catch {
            updateLoginItemControls()
            sendLoginItemState(error: error.localizedDescription)
        }
    }

    private func updateLoginItemControls() {
        loginItemMenuItem?.state = loginItemSelected ? .on : .off
    }

    private func sendLoginItemState(error: String? = nil) {
        let status = SMAppService.mainApp.status
        let state: String
        let detail: String
        let enabled: Bool
        switch status {
        case .enabled:
            state = "enabled"
            detail = localized("enabled")
            enabled = true
        case .requiresApproval:
            state = "requiresApproval"
            detail = localized("approval")
            enabled = false
        case .notRegistered:
            state = "notRegistered"
            detail = localized("disabled")
            enabled = false
        case .notFound:
            state = "notFound"
            detail = localized("notFound")
            enabled = false
        @unknown default:
            state = "unknown"
            detail = localized("unavailable")
            enabled = false
        }
        var payload: [String: Any] = [
            "kind": "launchAtLogin",
            "status": state,
            "selected": loginItemSelected,
            "enabled": enabled,
            "detail": detail,
        ]
        if let error {
            payload["error"] = error
        }
        guard let data = try? JSONSerialization.data(withJSONObject: payload),
              let json = String(data: data, encoding: .utf8)
        else {
            return
        }
        webView?.evaluateJavaScript("window.projectDockReceiveNativeState?.(\(json));")
    }

    func webView(
        _ webView: WKWebView,
        runJavaScriptConfirmPanelWithMessage message: String,
        initiatedByFrame frame: WKFrameInfo,
        completionHandler: @escaping (Bool) -> Void
    ) {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = localized("confirmAction")
        alert.informativeText = message
        alert.addButton(withTitle: localized("confirm"))
        alert.addButton(withTitle: localized("cancel"))
        if let window {
            alert.beginSheetModal(for: window) { response in
                completionHandler(response == .alertFirstButtonReturn)
            }
        } else {
            completionHandler(alert.runModal() == .alertFirstButtonReturn)
        }
    }

    private func connectToProjectDock() {
        guard !isTerminating, !isConnecting else {
            return
        }
        isConnecting = true
        isProjectDockReady { [weak self] ready in
            guard let self else { return }
            if ready {
                self.isConnecting = false
                self.didConnectToService()
                return
            }
            do {
                if self.serviceProcess?.isRunning != true {
                    try self.launchBundledService()
                }
                self.waitForService(attemptsRemaining: 60)
            } catch {
                self.isConnecting = false
                self.presentStartupError(error.localizedDescription)
            }
        }
    }

    private func launchBundledService() throws {
        let candidates = ["projectctl", "projectctl-darwin-arm64"]
        guard let binary = candidates.compactMap({
            Bundle.main.url(forResource: $0, withExtension: nil)
        }).first else {
            throw NSError(
                domain: "ProjectDock",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "APP 内缺少 projectctl 运行文件。"]
            )
        }
        let process = Process()
        process.executableURL = binary
        process.arguments = [
            "serve", "--listen", "127.0.0.1:\(projectDockPort)", "--open=false",
            "--parent-pid", String(ProcessInfo.processInfo.processIdentifier),
        ]
#if APP_STORE
        process.arguments?.append("--app-store")
#endif
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        process.terminationHandler = { [weak self] terminatedProcess in
            DispatchQueue.main.async {
                guard let self, self.serviceProcess === terminatedProcess else {
                    return
                }
                self.serviceProcess = nil
                self.serviceReady = false
                self.snapshotTimer?.invalidate()
                if !self.isTerminating, !self.isConnecting {
                    self.scheduleReconnect()
                }
            }
        }
        try process.run()
        serviceProcess = process
    }

    private func waitForService(attemptsRemaining: Int) {
        isProjectDockReady { [weak self] ready in
            guard let self else { return }
            if ready {
                self.isConnecting = false
                self.didConnectToService()
                return
            }
            guard attemptsRemaining > 0 else {
                self.isConnecting = false
                self.presentStartupError("本地服务未能在 6 秒内启动。请确认 \(projectDockPort) 端口没有被其他程序占用。")
                return
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                self.waitForService(attemptsRemaining: attemptsRemaining - 1)
            }
        }
    }

    private func didConnectToService() {
        reconnectWorkItem?.cancel()
        reconnectWorkItem = nil
        serviceReady = true
        loadInterface(route: webView?.url?.fragment ?? "dashboard")
        refreshWidgetSnapshot()
        snapshotTimer?.invalidate()
        let timer = Timer.scheduledTimer(withTimeInterval: 60, repeats: true) { [weak self] _ in
            self?.refreshWidgetSnapshot()
        }
        // Widget refresh does not require an exact second. Tolerance lets macOS
        // coalesce this wake-up with other system work.
        timer.tolerance = 15
        snapshotTimer = timer
    }

    private func isProjectDockReady(completion: @escaping (Bool) -> Void) {
        var request = URLRequest(url: projectDockURL.appending(path: "api/health"))
        request.timeoutInterval = 0.8
        URLSession.shared.dataTask(with: request) { data, response, _ in
            let httpOK = (response as? HTTPURLResponse)?.statusCode == 200
            let serviceOK: Bool
            if let data,
               let payload = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                let expectedVersion = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String
                serviceOK = payload["service"] as? String == "projectdock"
                    && payload["version"] as? String == expectedVersion
            } else {
                serviceOK = false
            }
            DispatchQueue.main.async {
                completion(httpOK && serviceOK)
            }
        }.resume()
    }

    private func refreshWidgetSnapshot() {
        guard serviceReady else {
            connectToProjectDock()
            return
        }
        var request = URLRequest(url: projectDockURL.appending(path: "api/widget-snapshot"))
        request.cachePolicy = .reloadIgnoringLocalCacheData
        request.timeoutInterval = 3
        URLSession.shared.dataTask(with: request) { data, response, _ in
            guard let data,
                  (response as? HTTPURLResponse)?.statusCode == 200
            else {
                DispatchQueue.main.async { [weak self] in
                    self?.handleServiceUnavailable()
                }
                return
            }
            do {
                let changed = try ProjectDockSnapshotStore.saveIfChanged(data: data)
                if changed {
                    DispatchQueue.main.async {
                        WidgetCenter.shared.reloadTimelines(ofKind: ProjectDockWidgetKind.status)
                    }
                }
            } catch {
                NSLog("ProjectDock 桌面组件快照写入失败: \(error.localizedDescription)")
            }
        }.resume()
    }

    private func handleServiceUnavailable() {
        guard !isTerminating else {
            return
        }
        serviceReady = false
        snapshotTimer?.invalidate()
        connectToProjectDock()
    }

    private func scheduleReconnect() {
        reconnectWorkItem?.cancel()
        let workItem = DispatchWorkItem { [weak self] in
            self?.connectToProjectDock()
        }
        reconnectWorkItem = workItem
        DispatchQueue.main.asyncAfter(deadline: .now() + 1, execute: workItem)
    }

    private func handleProjectDockURL(_ url: URL) {
        let host = url.host?.lowercased() ?? ""
        if host == "refresh" {
            refreshWidgetSnapshot()
            moveToBackgroundIfNoVisibleWindows()
            return
        }
        let route = url.pathComponents.dropFirst().first ?? "dashboard"
        showWindow(route: route)
    }

    private func loadInterface(route: String = "dashboard") {
        var components = URLComponents(url: projectDockURL, resolvingAgainstBaseURL: false)
        components?.fragment = ["dashboard", "projects", "ports", "api", "audit"].contains(route) ? route : "dashboard"
        var request = URLRequest(url: components?.url ?? projectDockURL)
        request.cachePolicy = .reloadIgnoringLocalCacheData
        webView?.load(request)
    }

    private func presentStartupError(_ message: String) {
        let alert = NSAlert()
        alert.alertStyle = .critical
        alert.messageText = localized("startupFailed")
        alert.informativeText = message
        alert.addButton(withTitle: localized("quitButton"))
        alert.runModal()
        NSApplication.shared.terminate(nil)
    }
}
