const state = {
  groups: [],
  proxies: {},
  filter: "all",
  query: "",
  delays: new Map(),
  testingGroups: new Set(),
  collapsed: new Set(JSON.parse(localStorage.getItem("sboxkit.collapsedGroups") || "[]")),
  token: localStorage.getItem("sboxkit.clashToken") || "",
  theme: localStorage.getItem("sboxkit.theme") || preferredTheme(),
};

const ids = {
  apiLine: document.getElementById("apiLine"),
  authBox: document.getElementById("authBox"),
  clearTokenBtn: document.getElementById("clearTokenBtn"),
  groupBoard: document.getElementById("groupBoard"),
  groupValue: document.getElementById("groupValue"),
  modeValue: document.getElementById("modeValue"),
  reloadBtn: document.getElementById("reloadBtn"),
  searchInput: document.getElementById("searchInput"),
  testAllBtn: document.getElementById("testAllBtn"),
  themeBtn: document.getElementById("themeBtn"),
  tokenInput: document.getElementById("tokenInput"),
  updatedAt: document.getElementById("updatedAt"),
  versionValue: document.getElementById("versionValue"),
  nodeValue: document.getElementById("nodeValue"),
};

function preferredTheme() {
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme() {
  document.documentElement.dataset.theme = state.theme;
  ids.themeBtn.textContent = state.theme === "dark" ? "Light" : "Dark";
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (state.token) headers.set("Authorization", `Bearer ${state.token}`);
  if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...options, headers });
  if (response.status === 401) {
    ids.authBox.classList.add("visible");
    throw new Error("HTTP 401");
  }
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  if (response.status === 204) return null;
  return response.json();
}

function apiPath(parts, query) {
  const encoded = parts.map((part) => encodeURIComponent(part)).join("/");
  const qs = query ? `?${new URLSearchParams(query)}` : "";
  return `/${encoded}${qs}`;
}

async function refresh() {
  ids.reloadBtn.disabled = true;
  ids.apiLine.textContent = "Refreshing Clash API data";
  try {
    await Promise.all([loadVersion(), loadConfig(), loadProxies()]);
    ids.apiLine.textContent = "Clash API connected";
    ids.authBox.classList.remove("visible");
    ids.updatedAt.textContent = new Date().toLocaleTimeString();
  } catch (error) {
    ids.apiLine.textContent = `Clash API unavailable: ${error.message}`;
    if (!state.groups.length) renderEmpty("No proxy data available");
  } finally {
    ids.reloadBtn.disabled = false;
  }
}

async function loadVersion() {
  try {
    const data = await api("/version");
    ids.versionValue.textContent = data.version || "unknown";
  } catch {
    ids.versionValue.textContent = "unavailable";
  }
}

async function loadConfig() {
  try {
    const data = await api("/configs");
    ids.modeValue.textContent = data.mode || data.Mode || "-";
  } catch {
    ids.modeValue.textContent = "-";
  }
}

async function loadProxies() {
  const data = await api("/proxies");
  state.proxies = data.proxies || {};
  state.groups = Object.values(state.proxies)
    .filter((proxy) => Array.isArray(proxy.all) && proxy.all.length)
    .sort(groupCompare);
  ids.groupValue.textContent = String(state.groups.length);
  ids.nodeValue.textContent = String(countNodes(state.groups));
  renderGroups();
}

function groupCompare(a, b) {
  const rank = (name) => {
    const order = ["Proxy", "AI", "Streaming", "Direct", "Fallback", "Auto", "SG-Auto", "SG-Fallback", "HK-Auto", "HK-Fallback"];
    const index = order.indexOf(name);
    return index === -1 ? 100 : index;
  };
  return rank(a.name) - rank(b.name) || a.name.localeCompare(b.name);
}

function countNodes(groups) {
  const names = new Set();
  for (const group of groups) for (const name of group.all || []) names.add(name);
  return names.size;
}

function visibleGroups() {
  const query = state.query.trim().toLowerCase();
  return state.groups.filter((group) => {
    const type = String(group.type || "").toLowerCase();
    if (state.filter !== "all" && type !== state.filter) return false;
    if (!query) return true;
    const haystack = [group.name, group.now, ...(group.all || [])].join("\n").toLowerCase();
    return haystack.includes(query);
  });
}

function renderGroups() {
  const groups = visibleGroups();
  if (!groups.length) {
    renderEmpty("No matching groups");
    return;
  }
  ids.groupBoard.replaceChildren(...groups.map(renderGroup));
}

function renderGroup(group) {
  const card = document.createElement("article");
  const collapsed = state.collapsed.has(group.name);
  card.className = `group-card${collapsed ? " collapsed" : ""}`;

  const header = document.createElement("div");
  header.className = "group-head";

  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "collapse-btn";
  toggle.textContent = collapsed ? "+" : "-";
  toggle.onclick = () => toggleGroup(group.name);

  const title = document.createElement("button");
  title.type = "button";
  title.className = "group-title";
  title.onclick = () => toggleGroup(group.name);
  title.append(textNode("span", "group-name", group.name));
  title.append(textNode("span", "group-now", group.now || "-"));

  const meta = textNode("span", "pill", String((group.all || []).length));
  const test = document.createElement("button");
  test.type = "button";
  test.className = "group-test";
  test.textContent = state.testingGroups.has(group.name) ? "Testing" : "Test";
  test.disabled = state.testingGroups.has(group.name);
  test.onclick = () => testGroup(group);

  header.append(toggle, title, meta, test);
  card.append(header);

  if (!collapsed) {
    const nodes = document.createElement("div");
    nodes.className = "node-grid";
    nodes.replaceChildren(...(group.all || []).map((name) => renderNode(group, name)));
    card.append(nodes);
  }
  return card;
}

function renderNode(group, name) {
  const card = document.createElement("button");
  const current = name === group.now;
  card.type = "button";
  card.className = `node-card${current ? " current" : ""}`;
  card.disabled = current;
  card.onclick = () => switchNode(group.name, name);
  card.append(textNode("span", "node-name", name));
  card.append(textNode("span", "node-meta", proxyType(name)));
  card.append(textNode("span", delayClass(name), delayText(name)));
  return card;
}

function toggleGroup(name) {
  if (state.collapsed.has(name)) {
    state.collapsed.delete(name);
  } else {
    state.collapsed.add(name);
  }
  localStorage.setItem("sboxkit.collapsedGroups", JSON.stringify([...state.collapsed]));
  renderGroups();
}

function proxyType(name) {
  const proxy = state.proxies[name];
  if (!proxy) return "proxy";
  return proxy.type || "proxy";
}

function delayText(name) {
  const item = state.delays.get(name);
  if (!item) return "-";
  if (item.pending) return "testing";
  return item.ok ? `${item.value} ms` : "timeout";
}

function delayClass(name) {
  const item = state.delays.get(name);
  if (!item) return "delay";
  if (item.pending) return "delay";
  if (!item.ok) return "delay bad";
  if (item.value < 400) return "delay good";
  if (item.value < 1000) return "delay warn";
  return "delay bad";
}

async function switchNode(group, name) {
  ids.apiLine.textContent = `Switching ${group}`;
  try {
    await api(apiPath(["proxies", group]), {
      method: "PUT",
      body: JSON.stringify({ name }),
    });
    await loadProxies();
    ids.apiLine.textContent = `Selected ${name}`;
  } catch (error) {
    ids.apiLine.textContent = `Switch failed: ${error.message}`;
  }
}

async function testDelay(name) {
  state.delays.set(name, { pending: true, ok: true, value: 0 });
  renderGroups();
  try {
    const data = await api(apiPath(["proxies", name, "delay"], {
      timeout: "5000",
      url: "https://www.gstatic.com/generate_204",
    }));
    state.delays.set(name, { ok: true, value: Number(data.delay) || 0 });
  } catch {
    state.delays.set(name, { ok: false, value: 0 });
  } finally {
    renderGroups();
  }
}

async function testGroup(group) {
  state.testingGroups.add(group.name);
  renderGroups();
  await testNames(group.all || []);
  state.testingGroups.delete(group.name);
  renderGroups();
}

async function testNames(names) {
  const queue = [...new Set(names)];
  ids.apiLine.textContent = `Testing ${queue.length} node(s)`;
  setBusy(true);
  const workers = Array.from({ length: Math.min(6, queue.length) }, async () => {
    while (queue.length) {
      await testDelay(queue.shift());
    }
  });
  await Promise.all(workers);
  setBusy(false);
  ids.apiLine.textContent = "Latency test complete";
}

function setBusy(busy) {
  ids.testAllBtn.disabled = busy;
}

function renderEmpty(message) {
  ids.groupBoard.innerHTML = "";
  ids.groupBoard.append(notice(message));
}

function notice(message) {
  return textNode("div", "notice", message);
}

function textNode(tag, className, text) {
  const el = document.createElement(tag);
  el.className = className;
  el.textContent = text;
  return el;
}

ids.reloadBtn.onclick = refresh;
ids.testAllBtn.onclick = () => {
  const names = visibleGroups().flatMap((group) => group.all || []);
  testNames(names);
};
ids.themeBtn.onclick = () => {
  state.theme = state.theme === "dark" ? "light" : "dark";
  localStorage.setItem("sboxkit.theme", state.theme);
  applyTheme();
};
ids.searchInput.oninput = (event) => {
  state.query = event.target.value;
  renderGroups();
};
document.querySelectorAll("[data-filter]").forEach((button) => {
  button.onclick = () => {
    state.filter = button.dataset.filter;
    document.querySelectorAll("[data-filter]").forEach((item) => item.classList.toggle("active", item === button));
    renderGroups();
  };
});
ids.authBox.onsubmit = (event) => {
  event.preventDefault();
  state.token = ids.tokenInput.value.trim();
  localStorage.setItem("sboxkit.clashToken", state.token);
  refresh();
};
ids.clearTokenBtn.onclick = () => {
  state.token = "";
  ids.tokenInput.value = "";
  localStorage.removeItem("sboxkit.clashToken");
  refresh();
};

ids.tokenInput.value = state.token;
applyTheme();
refresh();
setInterval(refresh, 20000);
