"use strict";

/* ============================================================
   sboxkit web console
   Two views: (1) login / port screen  (2) node dashboard.
   Talks to the sing-box Clash API: /version /configs /proxies
   /proxies/{group} (PUT) /proxies/{name}/delay
   ============================================================ */

const LS = {
  host: "sboxkit.host",
  port: "sboxkit.port",
  secret: "sboxkit.secret",
  remember: "sboxkit.autoConnect",
  theme: "sboxkit.theme",
  collapsed: "sboxkit.collapsedGroups",
};

const DELAY = { url: "https://www.google.com/generate_204", timeout: 5000 };
const GOOD_MS = 400;
const WARN_MS = 1000;
const CONCURRENCY = 6;
const REFRESH_MS = 15000;

const state = {
  base: "",
  secret: "",
  proxies: {},
  groups: [],
  filter: "all",
  query: "",
  delays: new Map(),          // node name -> {pending, ok, value}
  testing: new Set(),         // group names currently under test
  collapsed: new Set(load(LS.collapsed, [])),
  theme: localStorage.getItem(LS.theme) || preferredTheme(),
  refreshTimer: null,
};

const el = (id) => document.getElementById(id);
const ui = {
  loginView: el("loginView"), dashView: el("dashView"),
  loginForm: el("loginForm"), loginMsg: el("loginMsg"), loginVersion: el("loginVersion"),
  host: el("hostInput"), port: el("portInput"), secret: el("secretInput"), remember: el("rememberInput"),
  connChip: el("connChip"), connText: el("connText"),
  versionValue: el("versionValue"), modeValue: el("modeValue"),
  groupValue: el("groupValue"), nodeValue: el("nodeValue"),
  search: el("searchInput"), board: el("groupBoard"),
  refreshBtn: el("refreshBtn"), testAllBtn: el("testAllBtn"),
  themeBtn: el("themeBtn"), loginThemeBtn: el("loginThemeBtn"),
  disconnectBtn: el("disconnectBtn"),
  expandAllBtn: el("expandAllBtn"), collapseAllBtn: el("collapseAllBtn"),
  toast: el("toast"),
};

/* ---------- small helpers ---------- */
function load(key, fallback) {
  try { return JSON.parse(localStorage.getItem(key)) ?? fallback; } catch { return fallback; }
}
function preferredTheme() {
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
function applyTheme() {
  document.documentElement.dataset.theme = state.theme;
}
function toggleTheme() {
  state.theme = state.theme === "dark" ? "light" : "dark";
  localStorage.setItem(LS.theme, state.theme);
  applyTheme();
}
let toastTimer = null;
function toast(message, isError) {
  ui.toast.textContent = message;
  ui.toast.classList.toggle("error", !!isError);
  ui.toast.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => ui.toast.classList.remove("show"), 2600);
}
function textNode(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text != null) node.textContent = text;
  return node;
}

/* ---------- API layer ---------- */
async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (state.secret) headers.set("Authorization", `Bearer ${state.secret}`);
  if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const res = await fetch(state.base + path, { ...options, headers });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  if (res.status === 204) return null;
  return res.json();
}
function apiPath(parts, query) {
  const encoded = parts.map(encodeURIComponent).join("/");
  const qs = query ? `?${new URLSearchParams(query)}` : "";
  return `/${encoded}${qs}`;
}
function baseFrom(host, port) {
  const h = (host || "127.0.0.1").trim();
  const p = (port || "9090").trim();
  return `http://${h}:${p}`;
}

/* ============================================================
   LOGIN VIEW
   ============================================================ */
function initLogin() {
  ui.host.value = localStorage.getItem(LS.host) || location.hostname || "127.0.0.1";
  ui.port.value = localStorage.getItem(LS.port) || (location.port || "9090");
  ui.secret.value = localStorage.getItem(LS.secret) || "";
  ui.remember.checked = localStorage.getItem(LS.remember) === "1";

  ui.loginForm.addEventListener("submit", (e) => {
    e.preventDefault();
    connect(ui.host.value, ui.port.value, ui.secret.value, ui.remember.checked);
  });
}

async function connect(host, port, secret, remember) {
  const base = baseFrom(host, port);
  setLoginMsg("Connecting…", "pending");
  state.base = base;
  state.secret = (secret || "").trim();
  try {
    const version = await api("/version");
    // success — persist connection prefs
    localStorage.setItem(LS.host, (host || "").trim());
    localStorage.setItem(LS.port, (port || "").trim());
    localStorage.setItem(LS.secret, state.secret);
    localStorage.setItem(LS.remember, remember ? "1" : "0");
    ui.versionValue.textContent = version.version || "unknown";
    ui.loginVersion.textContent = version.version || "connected";
    showDashboard();
  } catch (err) {
    setLoginMsg(loginError(err, base), "error");
  }
}

function loginError(err, base) {
  const msg = String(err.message || err);
  if (msg.includes("401") || msg.includes("403")) return "Secret rejected — check the API secret.";
  if (msg.startsWith("HTTP")) return `Controller responded ${msg} at ${base}.`;
  return `Cannot reach controller at ${base}.`;
}
function setLoginMsg(text, cls) {
  ui.loginMsg.textContent = text;
  ui.loginMsg.className = `login-msg ${cls || ""}`.trim();
}

/* ============================================================
   VIEW SWITCHING
   ============================================================ */
function showDashboard() {
  ui.loginView.hidden = true;
  ui.dashView.hidden = false;
  setLoginMsg("", "");
  refresh();
  clearInterval(state.refreshTimer);
  state.refreshTimer = setInterval(() => refresh(true), REFRESH_MS);
}

function disconnect() {
  clearInterval(state.refreshTimer);
  state.groups = [];
  state.proxies = {};
  state.delays.clear();
  ui.dashView.hidden = true;
  ui.loginView.hidden = false;
  ui.secret.value = state.secret;
  setLoginMsg("Disconnected.", "pending");
}

/* ============================================================
   DASHBOARD DATA
   ============================================================ */
async function refresh(silent) {
  ui.refreshBtn.disabled = true;
  try {
    await Promise.all([loadVersion(), loadConfig(), loadProxies()]);
    setConn("ok", "connected");
  } catch (err) {
    setConn("down", "connection lost");
    if (!silent) toast(`Refresh failed: ${err.message}`, true);
    if (!state.groups.length) renderEmpty("No proxy data available.");
  } finally {
    ui.refreshBtn.disabled = false;
  }
}
async function loadVersion() {
  try { ui.versionValue.textContent = (await api("/version")).version || "unknown"; }
  catch { ui.versionValue.textContent = "–"; }
}
async function loadConfig() {
  try {
    const cfg = await api("/configs");
    ui.modeValue.textContent = cfg.mode || cfg.Mode || "–";
  } catch { ui.modeValue.textContent = "–"; }
}
async function loadProxies() {
  const data = await api("/proxies");
  state.proxies = data.proxies || {};
  state.groups = Object.values(state.proxies)
    .filter((p) => Array.isArray(p.all) && p.all.length)
    .sort(groupCompare);
  ui.groupValue.textContent = String(state.groups.length);
  ui.nodeValue.textContent = String(countNodes(state.groups));
  renderGroups();
}
function setConn(status, text) {
  ui.connChip.className = `conn-chip ${status === "ok" ? "" : status}`.trim();
  ui.connText.textContent = text;
}
function groupCompare(a, b) {
  const order = ["Proxy", "AI", "Streaming", "SG-Auto", "HK-Auto", "Auto", "Direct", "Fallback"];
  const rank = (n) => { const i = order.indexOf(n); return i === -1 ? 100 : i; };
  return rank(a.name) - rank(b.name) || a.name.localeCompare(b.name);
}
function countNodes(groups) {
  const names = new Set();
  for (const g of groups) for (const n of g.all || []) names.add(n);
  return names.size;
}

/* ============================================================
   RENDERING
   ============================================================ */
function visibleGroups() {
  const q = state.query.trim().toLowerCase();
  return state.groups.filter((g) => {
    const type = String(g.type || "").toLowerCase();
    if (state.filter !== "all" && type !== state.filter) return false;
    if (!q) return true;
    return [g.name, g.now, ...(g.all || [])].join("\n").toLowerCase().includes(q);
  });
}

function renderGroups() {
  const groups = visibleGroups();
  if (!groups.length) { renderEmpty(state.query ? "No groups match your filter." : "No proxy groups."); return; }
  ui.board.replaceChildren(...groups.map(renderGroup));
}

function renderGroup(group) {
  const collapsed = state.collapsed.has(group.name);
  const card = document.createElement("article");
  card.className = `group-card${collapsed ? " collapsed" : ""}`;

  // header (click toggles collapse)
  const head = document.createElement("div");
  head.className = "group-head";
  head.onclick = (e) => { if (!e.target.closest("button")) toggleGroup(group.name); };

  head.append(textNode("span", "chevron"));

  const headings = document.createElement("div");
  headings.className = "group-headings";
  const nameRow = document.createElement("div");
  nameRow.className = "group-name-row";
  nameRow.append(textNode("span", "group-name", group.name));
  const type = String(group.type || "").toLowerCase();
  nameRow.append(textNode("span", `type-badge ${type}`, type || "group"));
  headings.append(nameRow);
  const now = document.createElement("div");
  now.className = "group-now";
  now.append(textNode("b", null, "→"), textNode("span", null, group.now || "—"));
  headings.append(now);
  head.append(headings);

  const actions = document.createElement("div");
  actions.className = "group-head-actions";
  actions.append(textNode("span", "count-pill", String((group.all || []).length)));
  const testBtn = document.createElement("button");
  testBtn.type = "button";
  testBtn.className = "group-test-btn";
  const testing = state.testing.has(group.name);
  testBtn.disabled = testing;
  testBtn.append(textNode("span", "ico ico-bolt"), textNode("span", null, testing ? "Testing…" : "Test"));
  testBtn.onclick = () => testGroup(group);
  actions.append(testBtn);
  head.append(actions);
  card.append(head);

  // body
  const body = document.createElement("div");
  body.className = "group-body";
  const selectable = type === "selector";
  body.replaceChildren(...(group.all || []).map((name) => renderNode(group, name, selectable)));
  card.append(body);
  return card;
}

function renderNode(group, name, selectable) {
  const current = name === group.now;
  const card = document.createElement("button");
  card.type = "button";
  card.className = `node-card${current ? " current" : ""}${selectable ? "" : " readonly"}`;
  if (!selectable) card.disabled = true;
  else card.onclick = () => switchNode(group.name, name);

  const top = document.createElement("div");
  top.className = "node-top";
  top.append(textNode("span", "node-name", name));
  if (current) top.append(textNode("span", "node-check"));
  card.append(top);

  const bottom = document.createElement("div");
  bottom.className = "node-bottom";
  bottom.append(textNode("span", "node-type", proxyType(name)));
  const d = document.createElement("span");
  d.className = delayClass(name);
  d.append(textNode("span", "lat-dot"), textNode("span", null, delayText(name)));
  bottom.append(d);
  card.append(bottom);
  return card;
}

function renderEmpty(message) {
  ui.board.replaceChildren(textNode("div", "empty-note", message));
}

function proxyType(name) {
  const p = state.proxies[name];
  return p && p.type ? String(p.type).toLowerCase() : "proxy";
}
function delayText(name) {
  const d = state.delays.get(name);
  if (!d) return "—";
  if (d.pending) return "…";
  return d.ok ? `${d.value} ms` : "timeout";
}
function delayClass(name) {
  const d = state.delays.get(name);
  if (!d) return "delay";
  if (d.pending) return "delay pending";
  if (!d.ok) return "delay bad";
  if (d.value < GOOD_MS) return "delay good";
  if (d.value < WARN_MS) return "delay warn";
  return "delay bad";
}

/* ---------- collapse ---------- */
function toggleGroup(name) {
  if (state.collapsed.has(name)) state.collapsed.delete(name);
  else state.collapsed.add(name);
  persistCollapsed();
  renderGroups();
}
function persistCollapsed() {
  localStorage.setItem(LS.collapsed, JSON.stringify([...state.collapsed]));
}
function setAllCollapsed(collapsed) {
  state.collapsed = collapsed ? new Set(state.groups.map((g) => g.name)) : new Set();
  persistCollapsed();
  renderGroups();
}

/* ============================================================
   ACTIONS: switch + speed test
   ============================================================ */
async function switchNode(group, name) {
  try {
    await api(apiPath(["proxies", group]), { method: "PUT", body: JSON.stringify({ name }) });
    // optimistic update then reload
    if (state.proxies[group]) state.proxies[group].now = name;
    renderGroups();
    await loadProxies();
    toast(`${group} → ${name}`);
  } catch (err) {
    toast(`Switch failed: ${err.message}`, true);
    refresh(true);
  }
}

async function testDelay(name) {
  state.delays.set(name, { pending: true });
  renderGroups();
  try {
    const data = await api(apiPath(["proxies", name, "delay"], { timeout: DELAY.timeout, url: DELAY.url }));
    state.delays.set(name, { ok: true, value: Number(data.delay) || 0 });
  } catch {
    state.delays.set(name, { ok: false, value: 0 });
  }
  renderGroups();
}

async function testNames(names) {
  const queue = [...new Set(names)].filter((n) => !isBuiltin(n));
  if (!queue.length) return;
  const workers = Array.from({ length: Math.min(CONCURRENCY, queue.length) }, async () => {
    while (queue.length) await testDelay(queue.shift());
  });
  await Promise.all(workers);
}

function isBuiltin(name) {
  const p = state.proxies[name];
  const t = p && String(p.type || "").toLowerCase();
  return name === "DIRECT" || name === "BLOCK" || name === "REJECT" || t === "direct" || t === "block" || t === "reject";
}

async function testGroup(group) {
  if (state.testing.has(group.name)) return;
  state.testing.add(group.name);
  renderGroups();
  toast(`Testing ${group.name}…`);
  await testNames(group.all || []);
  state.testing.delete(group.name);
  renderGroups();
  toast(`${group.name} test complete`);
}

async function testAllVisible() {
  const names = visibleGroups().flatMap((g) => g.all || []);
  if (!names.length) return;
  ui.testAllBtn.disabled = true;
  toast(`Testing ${new Set(names).size} nodes…`);
  await testNames(names);
  ui.testAllBtn.disabled = false;
  toast("Latency test complete");
}

/* ============================================================
   WIRING
   ============================================================ */
function wireDashboard() {
  ui.refreshBtn.onclick = () => refresh();
  ui.testAllBtn.onclick = testAllVisible;
  ui.themeBtn.onclick = toggleTheme;
  ui.loginThemeBtn.onclick = toggleTheme;
  ui.disconnectBtn.onclick = disconnect;
  ui.expandAllBtn.onclick = () => setAllCollapsed(false);
  ui.collapseAllBtn.onclick = () => setAllCollapsed(true);
  ui.search.oninput = (e) => { state.query = e.target.value; renderGroups(); };
  document.querySelectorAll("[data-filter]").forEach((btn) => {
    btn.onclick = () => {
      state.filter = btn.dataset.filter;
      document.querySelectorAll("[data-filter]").forEach((b) => b.classList.toggle("active", b === btn));
      renderGroups();
    };
  });
}

function boot() {
  applyTheme();
  initLogin();
  wireDashboard();
  // Auto-connect only when the user opted in on a previous session.
  if (localStorage.getItem(LS.remember) === "1" && localStorage.getItem(LS.host) != null) {
    connect(
      localStorage.getItem(LS.host),
      localStorage.getItem(LS.port),
      localStorage.getItem(LS.secret),
      true,
    );
  }
}

boot();
