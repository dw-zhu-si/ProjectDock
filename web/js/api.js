let mutationToken = "";

export async function initializeSession() {
  const response = await request("/api/session");
  mutationToken = response.token;
}

export async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  const method = (options.method || "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD") {
    headers.set("X-ProjectDock-Token", mutationToken);
  }
  const response = await fetch(path, {
    ...options,
    method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload?.error?.message || `请求失败（HTTP ${response.status}）`);
  }
  return payload;
}

export const projectApi = {
  save(project, editing = false) {
    const path = editing ? `/api/projects/${encodeURIComponent(project.id)}` : "/api/projects";
    return request(path, { method: editing ? "PUT" : "POST", body: project });
  },
  remove(id, removeFiles = false, confirmation = "") {
    return request(`/api/projects/${encodeURIComponent(id)}/delete`, { method: "POST", body: { removeFiles, confirmation } });
  },
  start(id) {
    return request(`/api/projects/${encodeURIComponent(id)}/start`, { method: "POST", body: {} });
  },
  stop(id) {
    return request(`/api/projects/${encodeURIComponent(id)}/stop`, { method: "POST", body: {} });
  },
  logs(id) {
    return request(`/api/projects/${encodeURIComponent(id)}/logs?limit=250`);
  },
  importPaths(paths, source = "manual") {
    return request("/api/projects/import", { method: "POST", body: { paths, source } });
  },
  pickFolder() {
    return request("/api/projects/pick", { method: "POST", body: {} });
  },
  scan(root) {
    return request("/api/projects/scan", { method: "POST", body: { root } });
  },
};

export const directoryApi = {
  pick(purpose) {
    return request("/api/directories/pick", { method: "POST", body: { purpose } });
  },
};

export const aiApi = {
  get() {
    return request("/api/settings/ai");
  },
  save(settings) {
    return request("/api/settings/ai", { method: "PUT", body: settings });
  },
  verify() {
    return request("/api/settings/ai/verify", { method: "POST", body: {} });
  },
};

export const githubApi = {
  install(input) {
    return request("/api/github/install", { method: "POST", body: input });
  },
};

export const portApi = {
  allocate(allocation) {
    return request("/api/ports/allocations", { method: "POST", body: allocation });
  },
  unassign(port, projectId) {
    return request(`/api/ports/allocations/${port}?project=${encodeURIComponent(projectId)}`, { method: "DELETE" });
  },
  reserve(reservation) {
    return request("/api/reservations", { method: "POST", body: reservation });
  },
  release(port, projectId) {
    return request(`/api/reservations/${port}?project=${encodeURIComponent(projectId)}`, { method: "DELETE" });
  },
};

export function probeApi(payload) {
  return request("/api/probe", { method: "POST", body: payload });
}
