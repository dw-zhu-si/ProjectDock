import { aiApi, directoryApi, githubApi, initializeSession, portApi, probeApi, projectApi, request } from "./api.js";
import { escapeHTML, parseHeaderLines, prettyBody } from "./format.js";
import { renderShell, renderSnapshot } from "./ui.js";
import { getIntlLocale, getLocale, localizeDOM, setLocalePreference, t } from "./i18n.js";

const app = document.querySelector("#app");
const toastRegion = document.querySelector("#toast-region");
const state = {
  snapshot: {
    projects: [], ports: [], reservations: [], audit: [], ignoredCount: 0,
    portPool: { summary: {}, allocations: [], suggestions: [], unassignedListeners: [] },
    generatedAt: new Date().toISOString(),
  },
  filters: { project: "", port: "" },
  activeView: location.hash.slice(1) || "dashboard",
  busy: false,
	refreshing: false,
	booted: false,
	snapshotSignature: "",
  scanCandidates: [],
  deletingProjectId: "",
};

const nativeDirectoryRequests = new Map();

let refreshTimer;
let refreshRun;
let pendingRefresh;

const aiProviderURLs = {
  openai: "https://api.openai.com/v1",
  dashscope: "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "dashscope-plan": "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
  local: "http://127.0.0.1:11434/v1",
};

window.projectDockReceiveNativeState = receiveNativeState;

async function boot() {
  try {
    await initializeSession();
    renderShell(app);
    bindEvents();
    postNativeMessage({ action: "setLocale", locale: getLocale() });
    requestNativeSettings();
    navigate(state.activeView);
    await refresh({ announce: false });
    app.setAttribute("aria-busy", "false");
	state.booted = true;
	scheduleRefresh();
  } catch (error) {
    renderBootError(error);
  }
}

function bindEvents() {
  document.addEventListener("click", handleClick);
  document.querySelector("#project-form").addEventListener("submit", saveProject);
  document.querySelector("#allocation-form").addEventListener("submit", allocatePort);
  document.querySelector("#reservation-form").addEventListener("submit", reservePort);
  document.querySelector("#probe-form").addEventListener("submit", sendProbe);
  document.querySelector("#ai-form").addEventListener("submit", saveAISettings);
  document.querySelector("#ai-form").elements.provider.addEventListener("change", selectAIProvider);
  document.querySelector("#ai-form").elements.baseUrl.addEventListener("input", syncAIProviderFromURL);
  document.querySelector("#github-form").addEventListener("submit", installGitHubProject);
  document.querySelector("#scan-form").addEventListener("submit", scanProjects);
  document.querySelector("#full-delete-form").addEventListener("submit", deleteProjectFiles);
  document.querySelector("#launch-at-login").addEventListener("change", toggleLaunchAtLogin);
  document.querySelector("#language-select").addEventListener("change", changeLanguage);
  document.querySelector("#project-search").addEventListener("input", (event) => {
    state.filters.project = event.target.value;
    renderSnapshot(state.snapshot, state.filters);
  });
  document.querySelector("#port-search").addEventListener("input", (event) => {
    state.filters.port = event.target.value;
    renderSnapshot(state.snapshot, state.filters);
  });
  window.addEventListener("hashchange", () => navigate(location.hash.slice(1) || "dashboard"));
	document.addEventListener("visibilitychange", () => {
		if (document.hidden) {
			window.clearTimeout(refreshTimer);
			return;
		}
		void pollNow();
	});
  const dropZone = document.querySelector("#project-drop-zone");
  ["dragenter", "dragover"].forEach((name) => dropZone.addEventListener(name, (event) => {
    event.preventDefault();
    dropZone.classList.add("drag-active");
  }));
  ["dragleave", "drop"].forEach((name) => dropZone.addEventListener(name, () => dropZone.classList.remove("drag-active")));
  dropZone.addEventListener("drop", handleFolderDrop);
  window.projectDockReceiveDroppedPaths = (paths) => importProjectPaths(paths, "Finder 拖放");
}

function changeLanguage(event) {
  if (!setLocalePreference(event.currentTarget.value)) return;
  location.reload();
}

async function handleClick(event) {
  const nav = event.target.closest("[data-nav]");
  if (nav) {
    navigate(nav.dataset.nav);
    return;
  }
  const editorTab = event.target.closest("[data-editor-tab]");
  if (editorTab) {
    switchEditorTab(editorTab.dataset.editorTab);
    return;
  }
  const trigger = event.target.closest("[data-action]");
  if (!trigger || trigger.disabled) return;
  const { action, project, port } = trigger.dataset;
  if (action === "refresh") return refresh();
  if (action === "open-login-items-settings") return postNativeMessage({ action: "openLoginItemsSettings" });
  if (action === "pick-folder") return pickFolder();
  if (action === "open-ai-settings") return openAISettings();
  if (action === "close-ai-dialog") return document.querySelector("#ai-dialog").close();
  if (action === "github-install") return openGitHubInstall();
  if (action === "close-github-dialog") return document.querySelector("#github-dialog").close();
  if (action === "pick-install-root") return pickDirectory("install", "#github-form", "installRoot");
  if (action === "scan-projects") return openScanDialog();
  if (action === "close-scan-dialog") return document.querySelector("#scan-dialog").close();
  if (action === "pick-scan-root") return pickDirectory("scan", "#scan-form", "root");
  if (action === "import-scan-selection") return importScanSelection();
  if (action === "close-delete-dialog") return document.querySelector("#delete-dialog").close();
  if (action === "delete-registration") return confirmRegistrationDelete();
  if (action === "reveal-full-delete") return revealFullDelete();
  if (action === "cancel-full-delete") return resetDeleteChoice();
  if (action === "new-project") return openProjectDialog();
  if (action === "close-project-dialog") return document.querySelector("#project-dialog").close();
  if (action === "close-log-dialog") return document.querySelector("#log-dialog").close();
  if (action === "edit-project") return openProjectDialog(project);
  if (action === "delete-project") return deleteProject(project);
  if (action === "start-project") return mutateProject(project, "start");
  if (action === "stop-project") return mutateProject(project, "stop");
  if (action === "logs") return showLogs(project);
  if (action === "unassign-port") return unassignPort(Number(port), project);
  if (action === "suggest-port") {
    document.querySelector("#allocation-form").elements.port.value = port;
    return;
  }
  if (action === "release-port") return releasePort(Number(port), project);
}

function nativeBridge() {
  return window.webkit?.messageHandlers?.projectDockNative;
}

function postNativeMessage(payload) {
  const bridge = nativeBridge();
  if (!bridge) return false;
  bridge.postMessage(payload);
  return true;
}

function requestNativeSettings() {
  const settings = document.querySelector("#native-settings");
  if (!settings || !nativeBridge()) return;
  settings.hidden = false;
  postNativeMessage({ action: "getLaunchAtLogin" });
}

function toggleLaunchAtLogin(event) {
  const toggle = event.currentTarget;
  const detail = document.querySelector("#launch-at-login-detail");
  toggle.disabled = true;
  if (detail) detail.textContent = toggle.checked ? t("正在开启…") : t("正在关闭…");
  if (!postNativeMessage({ action: "setLaunchAtLogin", enabled: toggle.checked })) {
    toggle.checked = !toggle.checked;
    toggle.disabled = false;
  }
}

function receiveNativeState(payload) {
	if (payload?.kind === "directoryPicker") {
		const pending = nativeDirectoryRequests.get(payload.requestId);
		if (!pending) return;
		nativeDirectoryRequests.delete(payload.requestId);
		if (payload.path) pending.resolve({ path: payload.path });
		else pending.reject(new Error(payload.error || "目录选择失败"));
		return;
	}
	if (payload?.kind !== "launchAtLogin") return;
  const settings = document.querySelector("#native-settings");
  const toggle = document.querySelector("#launch-at-login");
  const detail = document.querySelector("#launch-at-login-detail");
  const settingsLink = document.querySelector("#login-items-settings");
  if (!settings || !toggle || !detail || !settingsLink) return;
  settings.hidden = false;
  toggle.checked = Boolean(payload.selected);
  toggle.disabled = false;
  detail.textContent = payload.detail || t("状态暂不可用");
  settingsLink.hidden = payload.status !== "requiresApproval";
  if (payload.error) toast(`开机自动启动设置失败：${payload.error}`, "danger");
}

function navigate(view) {
  const allowed = ["dashboard", "projects", "ports", "api", "audit"];
  const next = allowed.includes(view) ? view : "dashboard";
  state.activeView = next;
  if (location.hash !== `#${next}`) history.replaceState(null, "", `#${next}`);
  document.querySelectorAll("[data-view]").forEach((section) => {
    const active = section.dataset.view === next;
    section.hidden = !active;
    section.classList.toggle("active", active);
  });
  document.querySelectorAll(".nav-item").forEach((item) => item.classList.toggle("active", item.dataset.nav === next));
  document.querySelector("#page-title").textContent = t({
    dashboard: "运行总览",
    projects: "项目管理",
    ports: "端口监控",
    api: "接口调试",
    audit: "操作审计",
  }[next]);
	if (state.booted) scheduleRefresh();
}

function refresh(options = {}) {
	pendingRefresh = mergeRefreshOptions(pendingRefresh, options);
	if (!refreshRun) {
		refreshRun = drainRefreshQueue();
	}
	return refreshRun;
}

function mergeRefreshOptions(current, requested) {
	const next = {
		quiet: requested.quiet ?? false,
		announce: requested.announce ?? true,
	};
	if (!current) return next;
	return {
		quiet: current.quiet && next.quiet,
		announce: current.announce || next.announce,
	};
}

async function drainRefreshQueue() {
	state.refreshing = true;
	try {
		while (pendingRefresh) {
			const options = pendingRefresh;
			pendingRefresh = undefined;
			await performRefresh(options);
		}
	} finally {
		state.refreshing = false;
		refreshRun = undefined;
	}
}

async function performRefresh({ quiet, announce }) {
  const syncLabel = document.querySelector("#sync-label");
  if (syncLabel) syncLabel.textContent = t("正在同步");
  try {
		const snapshot = await request("/api/snapshot");
		const signature = snapshotContentSignature(snapshot);
		state.snapshot = snapshot;
		applyCapabilities(snapshot.capabilities || {});
		if (signature !== state.snapshotSignature) {
			state.snapshotSignature = signature;
			renderSnapshot(state.snapshot, state.filters);
		}
    if (syncLabel) syncLabel.textContent = `${t("已同步")} · ${new Date().toLocaleTimeString(getIntlLocale(), { hour12: false })}`;
    if (announce) toast("数据已刷新", "success");
  } catch (error) {
    if (syncLabel) syncLabel.textContent = t("同步失败");
    if (!quiet) toast(error.message, "danger");
  }
}

function snapshotContentSignature(snapshot) {
	return JSON.stringify({
		projects: snapshot.projects,
		ports: snapshot.ports,
		reservations: snapshot.reservations,
		audit: snapshot.audit,
		ignoredCount: snapshot.ignoredCount,
		portPool: snapshot.portPool,
		capabilities: snapshot.capabilities,
	});
}

function applyCapabilities(capabilities) {
	if (!capabilities.appStore) return;
	document.querySelector('[data-nav="ports"]')?.setAttribute("hidden", "");
	document.querySelector('[data-view="ports"]')?.setAttribute("hidden", "");
	document.querySelector(".protocol-panel")?.setAttribute("hidden", "");
	document.querySelector("#dashboard-ports")?.closest(".panel")?.setAttribute("hidden", "");
	document.querySelector(".sidebar-note")?.setAttribute("hidden", "");
	document.querySelector('[data-action="reveal-full-delete"]')?.setAttribute("hidden", "");
	const status = document.querySelector(".sidebar-status div");
	if (status) status.innerHTML = `<strong>${t("Mac App Store 沙盒模式")}</strong><span>${t("仅访问用户授权目录")}</span>`;
	const dashboardCopy = document.querySelector("#dashboard-projects")?.closest(".panel")?.querySelector(".panel-heading p");
	if (dashboardCopy) dashboardCopy.textContent = t("查看已授权并登记的本地项目");
	const discoveryCopy = document.querySelector("#project-drop-zone > div > p:last-child");
	if (discoveryCopy) discoveryCopy.textContent = t("通过系统目录选择器授权后，可安全安装、扫描和登记项目。");
	const projectsCopy = document.querySelector("#projects-table")?.closest(".panel")?.previousElementSibling?.querySelector(".section-intro");
	if (projectsCopy) projectsCopy.textContent = t("这里显示已获目录授权并完成登记的项目；商店版不会执行项目启动命令。");
	const githubCopy = document.querySelector("#github-dialog .dialog-copy");
	if (githubCopy) githubCopy.textContent = t("ProjectDock 会通过 GitHub HTTPS 下载仓库 ZIP、调用已配置的 AI 分析项目，再登记到用户授权目录；不会自动执行依赖安装或模型生成的命令。");
	for (const name of ["ports", "launchPorts", "startCommand", "stopCommand"]) {
		document.querySelector(`#project-form [name="${name}"]`)?.closest("label")?.setAttribute("hidden", "");
	}
	if (state.activeView === "ports") navigate("projects");
}

function refreshInterval() {
	if (state.activeView === "api") return 30_000;
	if (state.activeView === "audit") return 15_000;
	return 5_000;
}

function scheduleRefresh() {
	window.clearTimeout(refreshTimer);
	if (!state.booted || document.hidden) return;
	refreshTimer = window.setTimeout(pollNow, refreshInterval());
}

async function pollNow() {
	if (!document.hidden && !state.busy) {
		await refresh({ quiet: true, announce: false });
	}
	scheduleRefresh();
}

function openProjectDialog(projectId = "") {
  const dialog = document.querySelector("#project-dialog");
  const form = document.querySelector("#project-form");
  const project = state.snapshot.projects.find((item) => item.id === projectId);
  form.reset();
  form.dataset.editing = project ? "true" : "false";
  form.dataset.syncMode = project?.syncMode || "manual";
  form.dataset.discoveredBy = project?.discoveredBy || "manual";
  form.dataset.projectCard = project?.projectCard || "";
  form.dataset.launchSource = project?.launchSource || "";
  form.dataset.originalWorkingDirectory = project?.workingDirectory || "";
  form.dataset.originalStartCommand = project?.startCommand || "";
  form.dataset.originalStopCommand = project?.stopCommand || "";
  form.dataset.lastSeenAt = project?.lastSeenAt || "0001-01-01T00:00:00Z";
  form.dataset.createdAt = project?.createdAt || "0001-01-01T00:00:00Z";
  document.querySelector("#project-dialog-title").textContent = t(project ? "编辑项目" : "登记项目");
  const idInput = form.elements.id;
  idInput.disabled = Boolean(project);
  if (project) {
    idInput.value = project.id;
    form.elements.name.value = project.name;
    form.elements.path.value = project.path;
    form.elements.source.value = project.source;
    form.elements.ports.value = project.ports.join(", ");
    form.elements.workingDirectory.value = project.workingDirectory || "";
    form.elements.launchPorts.value = (project.launchPorts || []).join(", ");
    form.elements.startCommand.value = project.startCommand;
    form.elements.stopCommand.value = project.stopCommand || "";
    form.elements.healthUrl.value = project.healthUrl || "";
  }
  dialog.showModal();
  form.elements[project ? "name" : "id"].focus();
}

async function saveProject(event) {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  const rawPorts = form.elements.ports.value.trim();
  const ports = rawPorts ? rawPorts.split(",").map((value) => Number(value.trim())) : [];
  if (ports.some((port) => !Number.isInteger(port) || port < 1 || port > 65535)) {
    toast("持久分配端口必须是 1-65535 之间的数字", "danger");
    return;
  }
  const payload = {
    id: form.elements.id.value,
    name: form.elements.name.value,
    path: form.elements.path.value,
    source: form.elements.source.value,
    syncMode: form.dataset.syncMode || "manual",
    discoveredBy: form.dataset.discoveredBy || "manual",
    projectCard: form.dataset.projectCard || "",
    workingDirectory: form.elements.workingDirectory.value,
    startCommand: form.elements.startCommand.value,
    stopCommand: form.elements.stopCommand.value,
    launchSource: lifecycleChanged(form) ? "manual" : (form.dataset.launchSource || ""),
    launchPorts: (form.elements.launchPorts.value || "").split(",").map((value) => value.trim()).filter(Boolean).map(Number).filter(Number.isInteger),
    ports,
    healthUrl: form.elements.healthUrl.value,
    lastSeenAt: form.dataset.lastSeenAt || "0001-01-01T00:00:00Z",
    createdAt: form.dataset.createdAt || "0001-01-01T00:00:00Z",
    updatedAt: "0001-01-01T00:00:00Z",
  };
  await runBusy(async () => {
    await projectApi.save(payload, form.dataset.editing === "true");
    document.querySelector("#project-dialog").close();
    toast("项目登记已保存", "success");
    await refresh({ announce: false });
  });
}

function lifecycleChanged(form) {
  return form.elements.workingDirectory.value.trim() !== (form.dataset.originalWorkingDirectory || "") ||
    form.elements.startCommand.value.trim() !== (form.dataset.originalStartCommand || "") ||
    form.elements.stopCommand.value.trim() !== (form.dataset.originalStopCommand || "");
}

async function mutateProject(projectId, action) {
  const project = state.snapshot.projects.find((item) => item.id === projectId);
  if (!project) return;
  if (action === "stop") {
    const detail = project.run.state === "external"
      ? "将执行该项目已登记的安全停止命令；不会直接终止未知 PID。"
      : "将停止由当前 ProjectDock 实例启动并持有运行令牌的进程。";
    if (!window.confirm(`停止“${project.name}”？${detail}`)) return;
  }
  await runBusy(async () => {
    if (action === "start") {
      await projectApi.start(projectId);
      toast(`${project.name} 已由 ProjectDock 启动`, "success");
    } else {
      await projectApi.stop(projectId);
      toast(`${project.name} 已停止`, "success");
    }
    await refresh({ announce: false });
  });
}

async function deleteProject(projectId) {
  const project = state.snapshot.projects.find((item) => item.id === projectId);
  if (!project) return;
  state.deletingProjectId = projectId;
  document.querySelector("#delete-dialog-title").textContent = `删除“${project.name}”`;
  document.querySelector("#delete-project-path").textContent = project.path;
  document.querySelector("#delete-confirm-label").textContent = project.name;
  document.querySelector("#full-delete-form").elements.confirmation.value = "";
  resetDeleteChoice();
  document.querySelector("#delete-dialog").showModal();
}

async function confirmRegistrationDelete() {
  const project = state.snapshot.projects.find((item) => item.id === state.deletingProjectId);
  if (!project) return;
  await runBusy(async () => {
    await projectApi.remove(project.id, false, "");
    document.querySelector("#delete-dialog").close();
    toast(`${project.name} 的登记已删除，项目文件未改动`, "success");
    await refresh({ announce: false });
  });
}

function revealFullDelete() {
  document.querySelector("#delete-choice").hidden = true;
  const form = document.querySelector("#full-delete-form");
  form.hidden = false;
  form.elements.confirmation.focus();
}

function resetDeleteChoice() {
  document.querySelector("#delete-choice").hidden = false;
  document.querySelector("#full-delete-form").hidden = true;
}

async function deleteProjectFiles(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const project = state.snapshot.projects.find((item) => item.id === state.deletingProjectId);
  if (!project || !form.reportValidity()) return;
  await runBusy(async () => {
    await projectApi.remove(project.id, true, form.elements.confirmation.value);
    document.querySelector("#delete-dialog").close();
    toast(`${project.name} 已彻底卸载，项目目录及登记均已删除`, "success");
    await refresh({ announce: false });
  });
}

async function openAISettings() {
  const githubDialog = document.querySelector("#github-dialog");
  if (githubDialog.open) githubDialog.close();
  const dialog = document.querySelector("#ai-dialog");
  const form = document.querySelector("#ai-form");
  dialog.showModal();
  document.querySelector("#ai-settings-status").textContent = t("正在读取配置…");
  try {
    const settings = await aiApi.get();
    form.elements.baseUrl.value = settings.baseUrl || "https://api.openai.com/v1";
    form.elements.provider.value = inferAIProvider(form.elements.baseUrl.value);
    form.elements.model.value = settings.model || "";
    form.elements.apiKey.value = "";
    document.querySelector("#ai-settings-status").textContent = describeAIStatus(settings);
  } catch (error) {
    document.querySelector("#ai-settings-status").textContent = error.message;
  }
}

function selectAIProvider(event) {
  const provider = event.currentTarget.value;
  const baseURL = aiProviderURLs[provider];
  if (!baseURL) return;
  const form = event.currentTarget.form;
  form.elements.baseUrl.value = baseURL;
  if (provider === "local" && !form.elements.model.value) form.elements.model.value = "qwen3:8b";
}

function syncAIProviderFromURL(event) {
  event.currentTarget.form.elements.provider.value = inferAIProvider(event.currentTarget.value);
}

function inferAIProvider(baseURL) {
  const normalized = String(baseURL || "").replace(/\/+$/, "");
  return Object.entries(aiProviderURLs).find(([, value]) => value === normalized)?.[0] || "custom";
}

async function saveAISettings(event) {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  await runBusy(async () => {
    await aiApi.save({
      baseUrl: form.elements.baseUrl.value,
      model: form.elements.model.value,
      apiKey: form.elements.apiKey.value,
    });
    form.elements.apiKey.value = "";
    const status = document.querySelector("#ai-settings-status");
    status.textContent = t("配置已保存，正在验证真实模型连接…");
    try {
      const verified = await aiApi.verify();
      status.textContent = describeAIStatus(verified);
      toast("AI 模型连接验证成功", "success");
    } catch (error) {
      status.textContent = `配置已保存，但连接验证失败：${error.message}`;
      toast(error.message, "danger");
    }
  });
}

function describeAIStatus(settings) {
  if (!settings.configured) return settings.requiresApiKey ? t("远程接口还需要 API 密钥。") : t("请填写接口地址和模型名称。");
  if (settings.usable) return `连接已验证，可使用 GitHub 安装${settings.verifiedAt ? ` · ${new Date(settings.verifiedAt).toLocaleString("zh-CN")}` : ""}`;
  if (settings.verificationStatus === "failed") return `连接验证失败：${settings.verificationMessage || "请重新验证配置"}`;
  return t("配置已保存但尚未验证；请点击“保存并验证连接”。");
}

async function openGitHubInstall() {
  const dialog = document.querySelector("#github-dialog");
  const status = document.querySelector("#github-ai-status");
  const installButton = document.querySelector("#github-install-submit");
  dialog.showModal();
  installButton.disabled = true;
  status.className = "inline-status neutral";
  status.textContent = "正在检查 AI 配置…";
  try {
    const settings = await aiApi.get();
    status.className = `inline-status ${settings.usable ? "success" : settings.verificationStatus === "failed" ? "danger" : "warning"}`;
    status.textContent = settings.usable ? `AI 连接已验证 · ${settings.model}` : describeAIStatus(settings);
    installButton.disabled = !settings.usable;
  } catch (error) {
    status.className = "inline-status danger";
    status.textContent = error.message;
    installButton.disabled = true;
  }
}

async function pickDirectory(purpose, formSelector, fieldName) {
  await runBusy(async () => {
	const result = state.snapshot.capabilities?.appStore && nativeBridge()
		? await pickNativeDirectory(purpose)
		: await directoryApi.pick(purpose);
    document.querySelector(formSelector).elements[fieldName].value = result.path;
  });
}

function pickNativeDirectory(purpose) {
	return new Promise((resolve, reject) => {
		const requestId = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
		nativeDirectoryRequests.set(requestId, { resolve, reject });
		if (!postNativeMessage({ action: "pickDirectory", purpose, requestId })) {
			nativeDirectoryRequests.delete(requestId);
			reject(new Error("原生目录选择器不可用"));
		}
	});
}

async function installGitHubProject(event) {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  await runBusy(async () => {
    const result = await githubApi.install({ url: form.elements.url.value, installRoot: form.elements.installRoot.value });
    document.querySelector("#github-dialog").close();
    toast(result.warning || `${result.repository.owner}/${result.repository.name} 已安装并登记`, result.warning ? "neutral" : "success");
    navigate("projects");
    await refresh({ announce: false });
  });
}

function openScanDialog() {
  state.scanCandidates = [];
  document.querySelector("#scan-results").innerHTML = `<p class="quiet-empty">${t("选择一个父目录开始扫描。")}</p>`;
  document.querySelector("#import-scan-selection").disabled = true;
  document.querySelector("#scan-dialog").showModal();
}

async function scanProjects(event) {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  await runBusy(async () => {
    const report = await projectApi.scan(form.elements.root.value);
    state.scanCandidates = report.candidates || [];
    renderScanCandidates(report);
  });
}

function renderScanCandidates(report) {
  const target = document.querySelector("#scan-results");
  if (!report.candidates.length) {
    target.innerHTML = `<p class="quiet-empty">没有发现带常见项目标记的目录。</p>`;
    localizeDOM(target);
    document.querySelector("#import-scan-selection").disabled = true;
    return;
  }
  target.innerHTML = `${report.truncated ? `<p class="scan-warning">结果已达到 250 个上限，请缩小扫描范围。</p>` : ""}<div class="scan-list">${report.candidates.map((candidate, index) => `
    <label class="scan-item ${candidate.manageable ? "" : "unavailable"}">
      <input type="checkbox" name="scanCandidate" value="${index}" ${candidate.manageable && !candidate.managed ? "checked" : "disabled"}>
      <span><strong>${escapeHTML(candidate.name)}</strong><small>${escapeHTML(candidate.path)}</small></span>
      <span class="scan-kind">${escapeHTML(candidate.kind)} · ${candidate.managed ? "已管理" : candidate.manageable ? escapeHTML(candidate.startCommand) : "未识别启动方式"}</span>
    </label>`).join("")}</div>`;
  localizeDOM(target);
  document.querySelector("#import-scan-selection").disabled = !report.candidates.some((candidate) => candidate.manageable && !candidate.managed);
}

async function importScanSelection() {
  const selected = [...document.querySelectorAll('input[name="scanCandidate"]:checked')].map((item) => state.scanCandidates[Number(item.value)]?.path).filter(Boolean);
  if (!selected.length) {
    toast("请至少选择一个可管理项目", "danger");
    return;
  }
  await runBusy(async () => {
    const report = await projectApi.importPaths(selected, "manual");
    document.querySelector("#scan-dialog").close();
    toast(`已导入 ${report.imported} 个可管理项目`, report.skipped ? "neutral" : "success");
    navigate("projects");
    await refresh({ announce: false });
  });
}

async function pickFolder() {
  await runBusy(async () => {
	const report = state.snapshot.capabilities?.appStore && nativeBridge()
		? await projectApi.importPaths([(await pickNativeDirectory("project")).path], "manual")
		: await projectApi.pickFolder();
    toast(`已添加 ${report.imported} 个项目文件夹`, "success");
    navigate("projects");
    await refresh({ announce: false });
  });
}

async function importProjectPaths(paths, label = "拖放") {
  const clean = [...new Set((paths || []).filter((path) => typeof path === "string" && path.trim()).map((path) => path.trim()))];
  if (!clean.length) {
    toast("没有识别到可导入的文件夹。普通浏览器可能无法读取 Finder 绝对路径，请使用“选择文件夹”或 ProjectDock 原生 APP。", "danger");
    return;
  }
  await runBusy(async () => {
    const report = await projectApi.importPaths(clean);
    toast(`${label}完成：添加或更新 ${report.imported} 个，跳过 ${report.skipped} 个`, report.skipped ? "neutral" : "success");
    navigate("projects");
    await refresh({ announce: false });
  });
}

function handleFolderDrop(event) {
  event.preventDefault();
  const paths = [];
  for (const type of ["text/uri-list", "text/plain"]) {
    const value = event.dataTransfer?.getData(type) || "";
    for (const line of value.split(/\r?\n/)) {
      const candidate = line.trim();
      if (!candidate || candidate.startsWith("#")) continue;
      try {
        const url = new URL(candidate);
        if (url.protocol === "file:") paths.push(decodeURIComponent(url.pathname));
      } catch {
        if (candidate.startsWith("/")) paths.push(candidate);
      }
    }
  }
  importProjectPaths(paths, "拖放");
}

async function showLogs(projectId) {
  await runBusy(async () => {
    const result = await projectApi.logs(projectId);
    const project = state.snapshot.projects.find((item) => item.id === projectId);
    document.querySelector("#log-dialog-title").textContent = `${project?.name || projectId} · 运行日志`;
    document.querySelector("#log-output").textContent = result.lines.length ? result.lines.join("\n") : t("当前没有可显示的受管运行日志。");
    document.querySelector("#log-dialog").showModal();
  });
}

async function reservePort(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const hours = Number(form.elements.ttl.value);
  const expiresAt = new Date(Date.now() + hours * 3_600_000).toISOString();
  await runBusy(async () => {
    await portApi.reserve({
      port: Number(form.elements.port.value),
      projectId: form.elements.projectId.value,
      owner: form.elements.owner.value,
      createdAt: "0001-01-01T00:00:00Z",
      expiresAt,
    });
    form.elements.port.value = "";
    toast("端口检查通过并已预留", "success");
    await refresh({ announce: false });
  });
}

async function allocatePort(event) {
  event.preventDefault();
  const form = event.currentTarget;
  await runBusy(async () => {
    await portApi.allocate({
      port: Number(form.elements.port.value),
      projectId: form.elements.projectId.value,
      owner: form.elements.owner.value,
    });
    form.elements.port.value = "";
    toast("端口已持久分配；项目未启动时仍会占用资源池", "success");
    await refresh({ announce: false });
  });
}

async function unassignPort(port, projectId) {
  if (!window.confirm(`取消项目“${projectId}”对端口 ${port} 的持久分配？这不会停止端口上的进程。`)) return;
  await runBusy(async () => {
    await portApi.unassign(port, projectId);
    toast(`端口 ${port} 已从项目资源中取消分配`, "success");
    await refresh({ announce: false });
  });
}

async function releasePort(port, projectId) {
  await runBusy(async () => {
    await portApi.release(port, projectId);
    toast(`端口 ${port} 的预留已释放`, "success");
    await refresh({ announce: false });
  });
}

function switchEditorTab(tab) {
  document.querySelectorAll("[data-editor-tab]").forEach((button) => {
    const active = button.dataset.editorTab === tab;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", String(active));
  });
  document.querySelectorAll("[data-editor-pane]").forEach((pane) => {
    const active = pane.dataset.editorPane === tab;
    pane.hidden = !active;
    pane.classList.toggle("active", active);
  });
}

async function sendProbe(event) {
  event.preventDefault();
  const form = event.currentTarget;
  let headers;
  try {
    headers = parseHeaderLines(form.elements.headers.value);
  } catch (error) {
    toast(error.message, "danger");
    return;
  }
  const resultTarget = document.querySelector("#probe-result");
  const metaTarget = document.querySelector("#probe-meta");
  resultTarget.className = "code-output loading-output";
  resultTarget.textContent = t("正在调用本地接口…");
  metaTarget.innerHTML = "";
  await runBusy(async () => {
    try {
      const response = await probeApi({
        method: form.elements.method.value,
        url: form.elements.url.value,
        headers,
        body: form.elements.body.value,
      });
      metaTarget.innerHTML = `<span class="response-status ${response.status < 400 ? "success" : "danger"}">${response.status}</span><span>${response.durationMs} ms</span>`;
      const headerText = Object.entries(response.headers).map(([name, values]) => `${name}: ${values.join(", ")}`).join("\n");
      resultTarget.className = "code-output";
      resultTarget.textContent = `${headerText}\n\n${prettyBody(response.body)}${response.truncated ? "\n\n[响应已截断]" : ""}`;
      toast("本地接口调用完成", "success");
      await refresh({ quiet: true, announce: false });
    } catch (error) {
      resultTarget.className = "code-output error-output";
      resultTarget.textContent = error.message;
      throw error;
    }
  });
}

async function runBusy(task) {
  if (state.busy) return;
  state.busy = true;
  document.body.classList.add("is-busy");
  try {
    await task();
  } catch (error) {
    toast(error.message, "danger");
  } finally {
    state.busy = false;
    document.body.classList.remove("is-busy");
  }
}

function toast(message, tone = "neutral") {
  const item = document.createElement("div");
  item.className = `toast ${tone}`;
  item.textContent = t(message);
  toastRegion.append(item);
  window.setTimeout(() => item.remove(), 3600);
}

function renderBootError(error) {
  app.innerHTML = `
    <main class="boot-screen">
      <p class="eyebrow">CONNECTION ERROR</p>
      <h1>无法连接本地控制台</h1>
      <p>${escapeHTML(error.message)}</p>
      <button id="retry-connection" class="button primary" type="button">重新连接</button>
    </main>
  `;
  document.querySelector("#retry-connection").addEventListener("click", () => location.reload());
  app.setAttribute("aria-busy", "false");
  localizeDOM(app);
}

boot();
