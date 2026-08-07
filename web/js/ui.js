import {
  actionLabel,
  escapeHTML,
  formatDate,
  sourceLabel,
  stateLabel,
  stateTone,
  timeAgo,
  truncate,
} from "./format.js";
import { getLocalePreference, localizeDOM, supportedLocales, t } from "./i18n.js";

let currentCapabilities = {};

export function renderShell(root) {
  root.innerHTML = `
    <div class="app-shell">
      <aside class="sidebar">
        <a class="brand" href="#dashboard" data-nav="dashboard" aria-label="ProjectDock 首页">
          <span class="brand-mark" aria-hidden="true"><i></i><i></i><i></i></span>
          <span><strong>ProjectDock</strong><small>本地项目总控台</small></span>
        </a>
        <nav class="primary-nav" aria-label="主导航">
          ${navButton("dashboard", "总览", "01")}
          ${navButton("projects", "项目", "02", "project-count")}
          ${navButton("ports", "端口", "03", "port-count")}
          ${navButton("api", "接口调试", "04")}
          ${navButton("audit", "操作审计", "05")}
        </nav>
        <div id="native-settings" class="sidebar-setting" hidden>
          <label class="launch-toggle" for="launch-at-login">
            <span><strong>开机自动启动</strong><small id="launch-at-login-detail">读取系统状态…</small></span>
            <input id="launch-at-login" type="checkbox" role="switch" aria-describedby="launch-at-login-detail">
          </label>
          <button id="login-items-settings" class="text-button login-settings-link" type="button" data-action="open-login-items-settings" hidden>前往系统设置允许</button>
        </div>
        <label class="sidebar-language" for="language-select">
          <span>语言</span>
          <select id="language-select" aria-label="语言"></select>
        </label>
        <div class="sidebar-status">
          <span class="status-dot" aria-hidden="true"></span>
          <div><strong>独立本机模式</strong><span>仅限 127.0.0.1</span></div>
        </div>
        <div class="sidebar-note">
          <span>端口使用协议</span>
          <code>check → allocate → start</code>
        </div>
      </aside>

      <main class="main">
        <header class="topbar">
          <div>
            <p class="eyebrow">LOCAL DEVELOPMENT OPERATIONS</p>
            <h1 id="page-title">运行总览</h1>
          </div>
          <div class="topbar-actions">
            <span class="sync-state"><i></i><span id="sync-label">正在同步</span></span>
            <button class="button secondary" type="button" data-action="refresh">立即刷新</button>
            <button class="button secondary" type="button" data-action="open-model-settings">AI 设置</button>
            <button class="button primary" type="button" data-action="new-project">登记项目</button>
          </div>
        </header>

        <section class="view active" data-view="dashboard" aria-labelledby="page-title">
          <div id="metrics" class="metrics" aria-label="运行指标"></div>
          <div class="dashboard-grid">
            <section class="panel span-7">
              ${panelHeader("项目运行态", "从这里启动或停止已登记项目", "projects")}
              <div id="dashboard-projects"></div>
            </section>
            <section class="panel span-5">
              ${panelHeader("端口雷达", "持久分配及真实监听，5 秒刷新", "ports")}
              <div id="dashboard-ports"></div>
            </section>
            <section class="panel span-7">
              ${panelHeader("最近操作", "结构化记录，不含敏感请求头", "audit")}
              <div id="dashboard-audit"></div>
            </section>
            <section class="panel span-5 protocol-panel">
              <p class="panel-kicker">开发工具接入</p>
              <h2>启动前先过端口门禁</h2>
              <ol class="protocol-steps">
                <li><span>1</span><div><strong>检查</strong><code>projectctl port check 5173 --project demo</code></div></li>
                <li><span>2</span><div><strong>分配</strong><code>projectctl port allocate 5173 --project demo --owner codex</code></div></li>
                <li><span>3</span><div><strong>启动</strong><p>分配在项目停止后仍保留，直到显式取消。</p></div></li>
              </ol>
            </section>
          </div>
        </section>

        <section class="view" data-view="projects" hidden>
          <section id="project-drop-zone" class="project-drop-zone" aria-label="拖放项目文件夹">
            <div><p class="panel-kicker">PROJECT DISCOVERY & INSTALL</p><h2>安装或发现本地项目</h2><p>可粘贴 GitHub 仓库地址、扫描父目录，或直接添加现有文件夹。彻底卸载仅在二次确认后删除精确项目目录。</p></div>
            <div class="import-actions"><button class="button secondary" type="button" data-action="github-install">GitHub 安装</button><button class="button secondary" type="button" data-action="scan-projects">扫描本地项目</button><button class="button primary" type="button" data-action="pick-folder">选择文件夹</button></div>
          </section>
          <div class="section-toolbar">
            <div><p class="section-intro">这里只显示路径可用且已接入安全启动方式的项目；登记与自动识别不会直接执行命令。</p></div>
            <label class="search-field"><span class="sr-only">搜索项目</span><input id="project-search" type="search" placeholder="搜索名称、路径或来源"></label>
          </div>
          <section class="panel table-panel"><div id="projects-table"></div></section>
        </section>

        <section class="view" data-view="ports" hidden>
          <div class="section-toolbar">
            <div><p class="section-intro">持久分配在项目未启动时同样占用资源池；真实 LISTEN 状态永远优先。</p></div>
            <label class="search-field"><span class="sr-only">筛选端口</span><input id="port-search" type="search" placeholder="端口、PID、进程名"></label>
          </div>
          <div id="port-pool-metrics" class="metrics pool-metrics" aria-label="端口资源池指标"></div>
          <div class="ports-layout">
            <section class="panel span-8">
              ${panelHeader("持久分配", "项目停止后仍保留，显式取消后才释放")}
              <div id="allocations-table"></div>
            </section>
            <aside class="span-4">
              <section class="panel compact-panel">
                <p class="panel-kicker">PORT RESOURCE POOL</p>
                <h2>分配端口</h2>
                <form id="allocation-form" class="stack-form">
                  <label>端口<input name="port" type="number" min="1" max="65535" placeholder="例如 5173" required></label>
                  <label>项目<select name="projectId" id="allocation-project" required><option value="">选择项目</option></select></label>
                  <label>使用方<select name="owner" required><option value="codex">Codex</option><option value="trae">TRAE</option><option value="claude">Claude</option><option value="manual">手动</option></select></label>
                  <button class="button primary full" type="submit">检查并持久分配</button>
                </form>
                <div class="suggestions"><span>空闲建议</span><div id="port-suggestions"></div></div>
              </section>
            </aside>
            <section class="panel span-8">
              ${panelHeader("实时监听", "来自本机 lsof 的 TCP LISTEN；未分配监听也会单独统计")}
              <div id="ports-table"></div>
            </section>
            <aside class="span-4">
              <section class="panel compact-panel">
                <p class="panel-kicker">TEMPORARY RESERVATION</p>
                <h2>临时预留</h2>
                <form id="reservation-form" class="stack-form">
                  <label>端口<input name="port" type="number" min="1" max="65535" placeholder="例如 5173" required></label>
                  <label>项目<select name="projectId" id="reservation-project" required><option value="">选择项目</option></select></label>
                  <label>使用方<select name="owner" required><option value="codex">Codex</option><option value="trae">TRAE</option><option value="claude">Claude</option><option value="manual">手动</option></select></label>
                  <label>有效期<select name="ttl"><option value="1">1 小时</option><option value="4" selected>4 小时</option><option value="12">12 小时</option><option value="24">24 小时</option></select></label>
                  <button class="button secondary full" type="submit">创建短时预留</button>
                </form>
              </section>
              <section class="panel compact-panel reservations-panel">
                ${panelHeader("当前预留", "到期会自动失效")}
                <div id="reservations-list"></div>
              </section>
            </aside>
          </div>
        </section>

        <section class="view" data-view="api" hidden>
          <div class="api-layout">
            <section class="panel api-request">
              <div class="panel-heading"><div><p class="panel-kicker">LOCAL API WORKBENCH</p><h2>构造请求</h2></div><span class="local-only">仅回环地址</span></div>
              <form id="probe-form" class="probe-form">
                <div class="request-line">
                  <label class="sr-only" for="probe-method">请求方法</label>
                  <select id="probe-method" name="method"><option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option><option>HEAD</option></select>
                  <label class="sr-only" for="probe-url">接口地址</label>
                  <input id="probe-url" name="url" type="url" value="${escapeHTML(window.location.origin)}/api/health" required>
                  <button class="button primary" type="submit">发送请求</button>
                </div>
                <div class="editor-tabs" role="tablist" aria-label="请求内容">
                  <button type="button" class="editor-tab active" data-editor-tab="body" role="tab" aria-selected="true">Body</button>
                  <button type="button" class="editor-tab" data-editor-tab="headers" role="tab" aria-selected="false">Headers</button>
                </div>
                <label class="editor-pane active" data-editor-pane="body"><span class="sr-only">请求正文</span><textarea name="body" spellcheck="false" placeholder='{"name":"demo"}'></textarea></label>
                <label class="editor-pane" data-editor-pane="headers" hidden><span class="sr-only">请求头，每行一个</span><textarea name="headers" spellcheck="false" placeholder="Content-Type: application/json"></textarea></label>
                <p class="form-hint">Authorization、Cookie、代理头与非本机地址会被安全门禁拒绝。</p>
              </form>
            </section>
            <section class="panel api-response">
              <div class="panel-heading"><div><p class="panel-kicker">RESPONSE</p><h2>响应</h2></div><div id="probe-meta"></div></div>
              <div id="probe-result" class="code-output empty-output">发送请求后在这里查看状态、耗时、响应头和正文。</div>
            </section>
          </div>
        </section>

        <section class="view" data-view="audit" hidden>
          <div class="section-toolbar">
            <p class="section-intro">最近 200 条本地操作。URL 查询参数与敏感响应头不会进入记录。</p>
            <button class="button secondary" type="button" data-action="refresh">刷新审计</button>
          </div>
          <section class="panel table-panel"><div id="audit-table"></div></section>
        </section>
      </main>
    </div>

    <dialog id="project-dialog" class="dialog">
      <form id="project-form" method="dialog" autocomplete="off">
        <div class="dialog-header"><div><p class="panel-kicker">PROJECT REGISTRY</p><h2 id="project-dialog-title">登记项目</h2></div><button class="icon-button" type="button" data-action="close-project-dialog" aria-label="关闭">×</button></div>
        <div class="form-grid">
          <label>项目 ID<input name="id" pattern="[a-z0-9][a-z0-9_-]{1,47}" placeholder="例如 my-web-app" required><small>保存后作为稳定标识</small></label>
          <label>项目名称<input name="name" placeholder="例如 官网重构" required></label>
          <label class="full-field">绝对路径<input name="path" placeholder="/Users/name/Projects/my-app" required></label>
          <label>来源工具<select name="source"><option value="codex">Codex</option><option value="trae">TRAE</option><option value="claude">Claude</option><option value="tri-agent">三端注册表</option><option value="manual">手动</option><option value="other">其他</option></select></label>
          <label>持久分配端口<input name="ports" placeholder="5173, 3000"><small>未启动时也占用资源池；多个端口用逗号分隔</small></label>
          <label>项目内工作目录（可选）<input name="workingDirectory" placeholder="例如 web 或 apps/admin"><small>必须是项目根目录内的相对路径</small></label>
          <label>检测到的运行端口<input name="launchPorts" readonly placeholder="自动识别"><small>用于运行态识别；持久占用请在端口资源池分配</small></label>
          <label class="full-field">启动命令（可选）<input name="startCommand" placeholder="npm run dev -- --host 127.0.0.1"><small>自动识别只写入配置，不会自动执行</small></label>
          <label class="full-field">停止命令（可选）<input name="stopCommand" placeholder="docker compose stop 或 make stop"><small>外部运行项目只会执行此命令，绝不直接终止未知 PID</small></label>
          <label class="full-field">健康地址（可选）<input name="healthUrl" type="url" placeholder="http://127.0.0.1:5173/health"></label>
        </div>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="close-project-dialog">取消</button><button class="button primary" type="submit">保存项目</button></div>
      </form>
    </dialog>

    <dialog id="log-dialog" class="dialog log-dialog">
      <div class="dialog-header"><div><p class="panel-kicker">MANAGED PROCESS LOG</p><h2 id="log-dialog-title">运行日志</h2></div><button class="icon-button" type="button" data-action="close-log-dialog" aria-label="关闭">×</button></div>
      <pre id="log-output" class="log-output"></pre>
    </dialog>

    <dialog id="ai-dialog" class="dialog">
      <form id="ai-form" method="dialog" autocomplete="off">
        <div class="dialog-header"><div><p class="panel-kicker">AI MODEL SETTINGS</p><h2>AI 模型配置</h2></div><button class="icon-button" type="button" data-action="close-ai-dialog" aria-label="关闭">×</button></div>
        <p class="dialog-copy">用于分析 GitHub 项目的技术栈和人工安装步骤。API 密钥只保存到 macOS 钥匙串，不写入 ProjectDock 注册表或审计日志。</p>
        <div class="form-grid">
          <label>模型供应商<select name="provider"><option value="custom">通用兼容接口</option><option value="dashscope">阿里云百炼（标准 API）</option><option value="dashscope-plan">阿里云百炼（Coding Plan）</option><option value="local">本机模型</option></select><small>选择后自动填写对应接口；密钥必须由同一供应商签发</small></label>
          <label>模型名称<input name="model" placeholder="例如 gpt-5-mini 或 qwen-plus" required></label>
          <label class="full-field">兼容模型接口基础地址<input name="baseUrl" type="url" placeholder="https://example.com/v1" required><small>只允许 HTTPS；本机模型可使用 127.0.0.1 / localhost 的 HTTP 地址</small></label>
          <label>API 密钥<input name="apiKey" type="password" autocomplete="new-password" placeholder="已保存时可留空"><small>远程接口需要密钥；本机回环模型可留空</small></label>
        </div>
        <p id="ai-settings-status" class="form-hint">正在读取配置…</p>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="close-ai-dialog">取消</button><button class="button primary" type="submit">保存并验证连接</button></div>
      </form>
    </dialog>

    <dialog id="github-dialog" class="dialog">
      <form id="github-form" method="dialog" autocomplete="off">
        <div class="dialog-header"><div><p class="panel-kicker">GITHUB LOCAL INSTALL</p><h2>从 GitHub 安装项目</h2></div><button class="icon-button" type="button" data-action="close-github-dialog" aria-label="关闭">×</button></div>
        <p class="dialog-copy">ProjectDock 会浅克隆仓库、调用已配置的 AI 分析项目，再安全登记本地启动方式；不会自动执行依赖安装或模型生成的命令。</p>
        <div class="form-grid">
          <label class="full-field">GitHub 仓库网址<input name="url" type="url" placeholder="https://github.com/owner/repository" required></label>
          <label class="full-field">项目安装目录<div class="input-action"><input name="installRoot" placeholder="/Users/name/Projects" required><button class="mini-button" type="button" data-action="pick-install-root">选择…</button></div><small>项目将安装到“安装目录/仓库名称”</small></label>
        </div>
        <div id="github-ai-status" class="inline-status neutral">正在检查 AI 配置…</div>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="open-model-settings">配置 AI</button><button id="github-install-submit" class="button primary" type="submit" disabled>安装并登记</button></div>
      </form>
    </dialog>

    <dialog id="scan-dialog" class="dialog wide-dialog">
      <form id="scan-form" method="dialog" autocomplete="off">
        <div class="dialog-header"><div><p class="panel-kicker">LOCAL PROJECT SCAN</p><h2>扫描可管理项目</h2></div><button class="icon-button" type="button" data-action="close-scan-dialog" aria-label="关闭">×</button></div>
        <p class="dialog-copy">最多向下扫描 5 层，自动忽略依赖、构建产物、隐藏目录和符号链接。只勾选识别到安全启动方式的项目。</p>
        <label class="full-field">扫描父目录<div class="input-action"><input name="root" placeholder="/Users/name/Projects" required><button class="mini-button" type="button" data-action="pick-scan-root">选择…</button><button class="mini-button accent" type="submit">开始扫描</button></div></label>
        <div id="scan-results" class="scan-results"><p class="quiet-empty">选择一个父目录开始扫描。</p></div>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="close-scan-dialog">取消</button><button id="import-scan-selection" class="button primary" type="button" data-action="import-scan-selection" disabled>导入所选项目</button></div>
      </form>
    </dialog>

    <dialog id="delete-dialog" class="dialog">
      <div class="dialog-header"><div><p class="panel-kicker">REMOVE PROJECT</p><h2 id="delete-dialog-title">删除项目</h2></div><button class="icon-button" type="button" data-action="close-delete-dialog" aria-label="关闭">×</button></div>
      <p id="delete-project-path" class="danger-path"></p>
      <div id="delete-choice" class="delete-choice">
        <button class="delete-option" type="button" data-action="delete-registration"><strong>仅移除登记</strong><span>保留磁盘上的全部项目文件，并忽略后续自动同步。</span></button>
        <button class="delete-option danger" type="button" data-action="reveal-full-delete"><strong>彻底卸载项目</strong><span>永久删除上方精确目录内的全部内容，同时移除登记。</span></button>
      </div>
      <form id="full-delete-form" hidden>
        <div class="danger-callout"><strong>此操作不可撤销</strong><p>请输入完整项目名称 <code id="delete-confirm-label"></code> 继续。ProjectDock 不会删除父目录或符号链接目标。</p></div>
        <label>项目名称确认<input name="confirmation" autocomplete="off" required></label>
        <div class="dialog-actions"><button class="button secondary" type="button" data-action="cancel-full-delete">返回</button><button class="button danger-button" type="submit">彻底卸载并删除全部内容</button></div>
      </form>
    </dialog>
  `;
  const languageSelect = root.querySelector("#language-select");
  languageSelect.innerHTML = supportedLocales.map(({ code, label }) => `<option value="${code}">${code === "auto" ? `${t("跟随系统")} · ${label}` : label}</option>`).join("");
  languageSelect.value = getLocalePreference();
  root.querySelectorAll("input:not([autocomplete])").forEach((input) => input.setAttribute("autocomplete", "off"));
  localizeDOM(root);
}

function navButton(view, label, index, countId = "") {
  return `<button class="nav-item${view === "dashboard" ? " active" : ""}" type="button" data-nav="${view}"><span class="nav-index">${index}</span><span>${label}</span>${countId ? `<b id="${countId}">0</b>` : ""}</button>`;
}

function panelHeader(title, subtitle, target = "") {
  return `<div class="panel-heading"><div><h2>${title}</h2><p>${subtitle}</p></div>${target ? `<button class="text-button" type="button" data-nav="${target}">查看全部 →</button>` : ""}</div>`;
}

export function renderSnapshot(snapshot, filters = {}) {
	currentCapabilities = snapshot.capabilities || {};
  document.querySelector("#project-count").textContent = snapshot.projects.length;
  document.querySelector("#port-count").textContent = new Set(snapshot.ports.map((item) => item.port)).size;
  renderMetrics(snapshot);
  renderProjects(snapshot.projects, filters.project || "");
  renderPortPool(snapshot.portPool || { summary: {}, allocations: [], suggestions: [] }, snapshot.projects, filters.port || "");
  renderPorts(snapshot.ports, filters.port || "");
  renderReservations(snapshot.reservations, snapshot.projects);
  renderAudit(snapshot.audit);
  renderDashboard(snapshot);
  localizeDOM(document.querySelector(".main"));
}

function renderMetrics(snapshot) {
  const running = snapshot.projects.filter((project) => ["running", "starting", "stopping", "external"].includes(project.run.state)).length;
  const pool = snapshot.portPool?.summary || {};
  const uniqueListeningPorts = new Set(snapshot.ports.map((item) => item.port)).size;
  let metrics = [
    ["已登记项目", snapshot.projects.length, t("{running} 个运行 · {ignored} 个已忽略", { running, ignored: snapshot.ignoredCount || 0 }), running ? "success" : "neutral"],
    ["监听端口", uniqueListeningPorts, "本机唯一 TCP LISTEN", "info"],
    ["持久分配", pool.allocated || 0, t("{count} 个正在监听", { count: pool.activeAllocations || 0 }), pool.activeAllocations ? "info" : "success"],
		["最近同步", timeAgo(snapshot.generatedAt), "自动刷新间隔 5 秒", "neutral"],
  ];
	if (currentCapabilities.portMonitoring === false) {
		metrics = [metrics[0], ["目录授权", "安全作用域", "仅访问系统选择的目录", "success"], metrics[3]];
	}
  document.querySelector("#metrics").innerHTML = metrics.map(([label, value, detail, tone]) => `
    <article class="metric">
      <span>${escapeHTML(label)}</span>
      <strong class="${tone}">${escapeHTML(value)}</strong>
      <small>${escapeHTML(detail)}</small>
    </article>
  `).join("");
}

function renderProjects(projects, filter) {
  const normalized = filter.trim().toLowerCase();
  const visible = projects.filter((project) => [project.name, project.path, project.source, project.id].join(" ").toLowerCase().includes(normalized));
  const html = visible.length ? `
    <div class="data-table projects-data-table" role="table" aria-label="项目列表">
      <div class="table-row table-head" role="row"><span>项目</span><span>来源</span><span>分配端口</span><span>状态</span><span class="align-right">操作</span></div>
      ${visible.map(projectRow).join("")}
    </div>
  ` : emptyState(
	currentCapabilities.projectLifecycle === false ? "没有匹配的已授权项目" : "没有匹配的可启动项目",
	projects.length ? "换个关键词试试。" : (currentCapabilities.projectLifecycle === false ? "选择一个项目目录，即可安全登记和扫描。" : "添加具备明确启动方式的项目后，会在这里显示。"),
	"new-project", "登记项目",
  );
  document.querySelector("#projects-table").innerHTML = html;
  const select = document.querySelector("#reservation-project");
  const current = select.value;
  select.innerHTML = `<option value="">选择项目</option>${projects.map((project) => `<option value="${escapeHTML(project.id)}">${escapeHTML(project.name)}</option>`).join("")}`;
  select.value = current;
  const allocationSelect = document.querySelector("#allocation-project");
  const allocationCurrent = allocationSelect.value;
  allocationSelect.innerHTML = `<option value="">选择项目</option>${projects.map((project) => `<option value="${escapeHTML(project.id)}">${escapeHTML(project.name)}</option>`).join("")}`;
  allocationSelect.value = allocationCurrent;
}

function projectRow(project) {
  const managedActive = ["running", "starting", "stopping"].includes(project.run.state);
  const active = managedActive || project.run.state === "external";
  const status = projectStatus(project);
  const lifecycle = projectLifecycleAction(project);
  const portMarkup = project.ports.length
    ? project.ports.map((port) => `<code title="持久分配">:${port}</code>`).join("")
    : (project.launchPorts?.length ? project.launchPorts.map((port) => `<code class="detected-port" title="检测端口">~${port}</code>`).join("") : "—");
  return `
    <div class="table-row" role="row">
      <div class="project-cell"><span class="project-monogram">${escapeHTML(project.name.slice(0, 1).toUpperCase())}</span><div><strong>${escapeHTML(project.name)}</strong><small title="${escapeHTML(project.path)}">${escapeHTML(truncate(project.path, 56))}</small></div></div>
      <span><span class="source-badge source-${escapeHTML(project.source)}">${escapeHTML(sourceLabel(project.source))}</span><small class="sync-mode-label">${project.syncMode === "auto" ? "自动同步" : "手动登记"}</small></span>
      <span class="ports-inline">${portMarkup}</span>
      <span><span class="state-badge ${status.tone}"><i></i>${escapeHTML(status.label)}</span>${project.run.pid ? `<small class="pid-label">PID ${project.run.pid}</small>` : ""}</span>
      <span class="row-actions align-right">
		${currentCapabilities.projectLifecycle === false ? "" : `<button class="mini-button" type="button" data-action="logs" data-project="${escapeHTML(project.id)}">日志</button>`}
        <button class="mini-button" type="button" data-action="edit-project" data-project="${escapeHTML(project.id)}" ${managedActive ? "disabled" : ""}>编辑</button>
        <button class="mini-button" type="button" data-action="delete-project" data-project="${escapeHTML(project.id)}" ${active ? "disabled" : ""}>删除</button>
		${currentCapabilities.projectLifecycle === false ? "" : `<button class="mini-button ${lifecycle.tone}" type="button" data-action="${lifecycle.action}" data-project="${escapeHTML(project.id)}" ${lifecycle.disabled ? `disabled title="${escapeHTML(lifecycle.title)}"` : ""}>${escapeHTML(lifecycle.label)}</button>`}
      </span>
    </div>
  `;
}

function renderPortPool(pool, projects, filter) {
  const summary = pool.summary || {};
  const metrics = [
    ["持久分配", summary.allocated || 0, "未启动也占用", "success"],
    ["活跃分配", summary.activeAllocations || 0, "已有真实监听", "info"],
    ["临时预留", summary.temporaryReserved || 0, "到期自动释放", "neutral"],
    ["未分配监听", summary.unassignedListeners || 0, "仅监控，不接管", summary.unassignedListeners ? "warning" : "success"],
  ];
  document.querySelector("#port-pool-metrics").innerHTML = metrics.map(([label, value, detail, tone]) => `
    <article class="metric"><span>${escapeHTML(label)}</span><strong class="${tone}">${value}</strong><small>${escapeHTML(detail)}</small></article>
  `).join("");
  const normalized = filter.trim().toLowerCase();
  const allocations = (pool.allocations || []).filter((item) => [
    item.port, item.projectId, item.projectName, item.listener?.process || "", item.listener?.pid || "",
  ].join(" ").toLowerCase().includes(normalized));
  document.querySelector("#allocations-table").innerHTML = allocations.length ? `
    <div class="data-table allocation-data-table" role="table" aria-label="持久端口分配">
      <div class="table-row table-head" role="row"><span>端口</span><span>项目</span><span>状态</span><span>真实进程</span><span></span></div>
      ${allocations.map((item) => `
        <div class="table-row" role="row">
          <strong class="port-number">:${item.port}</strong>
          <span><strong>${escapeHTML(item.projectName)}</strong><small>${escapeHTML(item.projectId)}</small></span>
          <span class="state-badge ${item.state === "active" ? "success" : item.state === "conflict" ? "danger" : "neutral"}"><i></i>${item.state === "active" ? "监听中" : item.state === "conflict" ? "冲突" : "已分配 · 未启动"}</span>
          <span>${item.listener ? `${escapeHTML(item.listener.process || "未知")} · PID ${item.listener.pid}` : "—"}</span>
          <button class="mini-button" type="button" data-action="unassign-port" data-port="${item.port}" data-project="${escapeHTML(item.projectId)}">取消分配</button>
        </div>
      `).join("")}
    </div>
  ` : `<p class="quiet-empty">暂无持久分配；右侧选择项目和端口后即可加入资源池。</p>`;
  document.querySelector("#port-suggestions").innerHTML = (pool.suggestions || []).slice(0, 10).map((port) => `
    <button class="suggestion-chip" type="button" data-action="suggest-port" data-port="${port}">:${port}</button>
  `).join("") || `<span class="quiet-empty">当前范围没有空闲建议</span>`;
}

function renderPorts(ports, filter) {
  const normalized = filter.trim().toLowerCase();
  const visible = ports.filter((listener) => [listener.port, listener.pid, listener.process, listener.address].join(" ").toLowerCase().includes(normalized));
  document.querySelector("#ports-table").innerHTML = visible.length ? `
    <div class="data-table port-data-table" role="table" aria-label="监听端口">
      <div class="table-row table-head" role="row"><span>端口</span><span>进程</span><span>PID</span><span>监听地址</span></div>
      ${visible.map((listener) => `
        <div class="table-row" role="row"><strong class="port-number">${listener.port}</strong><span>${escapeHTML(listener.process || "未知")}</span><code>${listener.pid}</code><code title="${escapeHTML(listener.address)}">${escapeHTML(truncate(listener.address, 42))}</code></div>
      `).join("")}
    </div>
  ` : emptyState("没有匹配的监听端口", ports.length ? "清除筛选条件查看全部。" : "当前 lsof 没有返回 TCP LISTEN 结果。");
}

function renderReservations(reservations) {
  document.querySelector("#reservations-list").innerHTML = reservations.length ? reservations.map((reservation) => `
    <article class="reservation-item">
      <div><strong>:${reservation.port}</strong><span>${escapeHTML(reservation.projectId)} · ${escapeHTML(sourceLabel(reservation.owner))}</span></div>
      <button class="icon-button small" type="button" data-action="release-port" data-port="${reservation.port}" data-project="${escapeHTML(reservation.projectId)}" aria-label="释放 ${reservation.port} 端口">×</button>
      <small>到期 ${formatDate(reservation.expiresAt)}</small>
    </article>
  `).join("") : `<p class="quiet-empty">暂无有效预留</p>`;
}

function renderAudit(audit) {
  const rows = audit.length ? audit.map((event) => `
    <div class="table-row" role="row">
      <span>${formatDate(event.timestamp)}</span>
      <strong>${escapeHTML(actionLabel(event.action))}</strong>
      <span>${escapeHTML(event.projectId || "—")}</span>
      <span>${event.port || "—"}</span>
      <span><span class="state-badge ${event.status === "success" ? "success" : "danger"}"><i></i>${event.status === "success" ? "成功" : "失败"}</span></span>
    </div>
  `).join("") : "";
  document.querySelector("#audit-table").innerHTML = rows ? `<div class="data-table audit-data-table" role="table" aria-label="操作审计"><div class="table-row table-head" role="row"><span>时间</span><span>操作</span><span>项目</span><span>端口</span><span>结果</span></div>${rows}</div>` : emptyState("还没有操作记录", "登记项目、预留端口或调用接口后会在这里出现。");
}

function renderDashboard(snapshot) {
  const projectTarget = document.querySelector("#dashboard-projects");
  projectTarget.innerHTML = snapshot.projects.length ? `<div class="dashboard-list">${snapshot.projects.slice(0, 5).map((project) => `
    <article class="dashboard-project">
      <span class="project-monogram">${escapeHTML(project.name.slice(0, 1).toUpperCase())}</span>
      <div><strong>${escapeHTML(project.name)}</strong><small>${project.syncMode === "auto" ? t("自动同步") : escapeHTML(sourceLabel(project.source))} · ${project.ports.length ? `:${project.ports.join(", :")}` : t("未声明端口")}</small></div>
      <span class="state-badge ${projectStatus(project).tone}"><i></i>${escapeHTML(projectStatus(project).label)}</span>
      ${dashboardLifecycleButton(project)}
    </article>
  `).join("")}</div>` : emptyState("还没有登记项目", "先添加一个本地项目。", "new-project", "登记项目");

  const allocations = snapshot.portPool?.allocations || [];
  document.querySelector("#dashboard-ports").innerHTML = allocations.length ? `<div class="port-radar">${allocations.slice(0, 8).map((item) => `
    <div><strong>:${item.port}</strong><span>${escapeHTML(item.projectName)}</span><code>${item.state === "active" ? "监听中" : "已分配"}</code></div>
  `).join("")}</div>${allocations.length > 8 ? `<p class="list-footnote">另有 ${allocations.length - 8} 个持久分配</p>` : ""}` : `<p class="quiet-empty">当前没有持久端口分配</p>`;

  document.querySelector("#dashboard-audit").innerHTML = snapshot.audit.length ? `<div class="timeline">${snapshot.audit.slice(0, 6).map((event) => `
    <div><i class="${event.status === "success" ? "success" : "danger"}"></i><span><strong>${escapeHTML(actionLabel(event.action))}</strong><small>${escapeHTML(event.projectId || (event.port ? `${t("端口")} ${event.port}` : t("本地操作")))}</small></span><time>${timeAgo(event.timestamp)}</time></div>
  `).join("")}</div>` : `<p class="quiet-empty">暂无操作记录</p>`;
}

function projectStatus(project) {
  if (project.run.state === "running" || project.run.state === "starting" || project.run.state === "stopping") {
    return { label: stateLabel(project.run.state), tone: stateTone(project.run.state) };
  }
  if (project.run.state === "external") return { label: t("外部运行"), tone: "info" };
  if (project.run.state === "conflict") return { label: t("端口冲突"), tone: "danger" };
  if (!project.pathAvailable) return { label: t("路径不可用"), tone: "danger" };
	if (currentCapabilities.projectLifecycle === false) return { label: t("已授权 · 已登记"), tone: "success" };
  if (project.launchSource === "archived") return { label: t("已归档 · 仅登记"), tone: "neutral" };
  if (!project.configuredToStart) return { label: t("已登记 · 启动未接入"), tone: "neutral" };
  return { label: stateLabel(project.run.state), tone: stateTone(project.run.state) };
}

function projectLifecycleAction(project) {
  if (project.canStop) {
    return { action: "stop-project", label: "停止", tone: "danger", disabled: false, title: "" };
  }
  if (project.run.state === "external") {
    return { action: "", label: "外部运行", tone: "", disabled: true, title: "未登记安全停止命令，不会终止未知 PID" };
  }
  if (project.run.state === "conflict") {
    return { action: "", label: "端口冲突", tone: "", disabled: true, title: project.run.message || "检测端口被占用" };
  }
  if (project.readyToStart) {
    return { action: "start-project", label: "启动", tone: "accent", disabled: false, title: "" };
  }
  if (!project.pathAvailable) {
    return { action: "", label: "路径不可用", tone: "", disabled: true, title: "项目路径不可访问" };
  }
  if (project.launchSource === "archived") {
    return { action: "", label: "已归档", tone: "", disabled: true, title: "归档项目默认只登记，不自动接入启动" };
  }
  if (!project.configuredToStart) {
    return { action: "edit-project", label: "配置启动", tone: "accent", disabled: false, title: "" };
  }
  return { action: "", label: "不可启动", tone: "", disabled: true, title: project.run.message || "当前状态不可启动" };
}

function dashboardLifecycleButton(project) {
	if (currentCapabilities.projectLifecycle === false) return "";
  const lifecycle = projectLifecycleAction(project);
  return `<button class="mini-button ${lifecycle.tone}" type="button" data-action="${lifecycle.action}" data-project="${escapeHTML(project.id)}" ${lifecycle.disabled ? `disabled title="${escapeHTML(lifecycle.title)}"` : ""}>${escapeHTML(lifecycle.label)}</button>`;
}

function emptyState(title, body, action = "", label = "") {
  return `<div class="empty-state"><span aria-hidden="true">＋</span><h3>${escapeHTML(title)}</h3><p>${escapeHTML(body)}</p>${action ? `<button class="button secondary" type="button" data-action="${action}">${escapeHTML(label)}</button>` : ""}</div>`;
}
