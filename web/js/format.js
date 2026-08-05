import { getIntlLocale, t } from "./i18n.js";

export function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function formatDate(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat(getIntlLocale(), {
    month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
  }).format(date);
}

export function timeAgo(value) {
  if (!value) return t("刚刚");
  const diff = Date.now() - new Date(value).valueOf();
  if (diff < 10_000) return t("刚刚");
  if (diff < 60_000) return new Intl.RelativeTimeFormat(getIntlLocale(), { numeric: "auto" }).format(-Math.floor(diff / 1000), "second");
  if (diff < 3_600_000) return new Intl.RelativeTimeFormat(getIntlLocale(), { numeric: "auto" }).format(-Math.floor(diff / 60_000), "minute");
  return formatDate(value);
}

export function sourceLabel(source) {
  return t({
    codex: "Codex",
    trae: "TRAE",
    claude: "Claude",
    manual: "手动",
    "tri-agent": "三端注册表",
    projectdock: "ProjectDock",
    other: "其他",
  }[source] || source);
}

export function actionLabel(action) {
  return t({
    "project.upsert": "保存项目",
    "project.delete": "删除登记",
    "project.start": "启动项目",
    "project.stop": "停止项目",
    "project.external-stop": "停止外部项目",
    "project.exit": "项目退出",
    "project.sync": "同步项目",
    "project.scan": "扫描项目",
    "registry.sync": "同步三端项目",
    "port.allocate": "分配端口",
    "port.unassign": "取消分配",
    "port.reserve": "预留端口",
    "port.release": "释放端口",
    "api.probe": "接口调用",
	"settings.ai": "保存 AI 设置",
	"settings.ai.verify": "验证 AI 连接",
	"github.install": "安装 GitHub 项目",
  }[action] || action);
}

export function stateLabel(state) {
  return t({
    running: "运行中",
    starting: "启动中",
    stopping: "停止中",
    stopped: "已停止",
    external: "外部运行",
    conflict: "端口冲突",
  }[state] || state);
}

export function stateTone(state) {
  if (state === "running") return "success";
  if (state === "external") return "info";
  if (state === "conflict") return "danger";
  if (state === "starting" || state === "stopping") return "warning";
  return "neutral";
}

export function truncate(value, length = 80) {
  const text = String(value ?? "");
  return text.length > length ? `${text.slice(0, length - 1)}…` : text;
}

export function parseHeaderLines(raw) {
  const result = {};
  for (const line of raw.split("\n")) {
    if (!line.trim()) continue;
    const separator = line.indexOf(":");
    if (separator < 1) {
      throw new Error(`${t("请求头格式错误")}：${line}`);
    }
    const name = line.slice(0, separator).trim();
    const value = line.slice(separator + 1).trim();
    result[name] = value;
  }
  return result;
}

export function prettyBody(body) {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}
