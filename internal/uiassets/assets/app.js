const state = {
  groups: [],
  proxies: {},
  selected: "",
  filter: "all",
  query: "",
  delaySort: false,
  delays: new Map(),
  token: localStorage.getItem("sboxkit.clashToken") || "",
};

const ids = {
  apiLine: document.getElementById("apiLine"),
  authBox: document.getElementById("authBox"),
  clearTokenBtn: document.getElementById("clearTokenBtn"),
  detailMeta: document.getElementById("detailMeta"),
  detailName: document.getElementById("detailName"),
  groupList: document.getElementById("groupList"),
  groupValue: document.getElementById("groupValue"),
  modeValue: document.getElementById("modeValue"),
  nodeList: document.getElementById("nodeList"),
  nodeValue: document.getElementById("nodeValue"),
  reloadBtn: document.getElementById("reloadBtn"),
  searchInput: document.getElementById("searchInput"),
  showCurrentBtn: document.getElementById("showCurrentBtn"),
  sortDelayBtn: document.getElementById("sortDelayBtn"),
  testAllBtn: document.getElementById("testAllBtn"),
  testCurrentBtn: document.getElementById("testCurrentBtn"),
  tokenInput: document.getElementById("tokenInput"),
  updatedAt: document.getElementById("updatedAt"),
  versionValue: document.getElementById("versionValue"),
};

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
  if (!state.selected || !state.groups.some((group) => group.name === state.selected)) {
    state.selected = state.groups[0]?.name || "";
  }
  ids.groupValue.textContent = String(state.groups.length);
  ids.nodeValue.textContent = String(countNodes(state.groups));
  render();
}

function groupCompare(a, b) {
  const rank = (name) => {
    const order = ["Proxy", "AI", "Streaming", "Direct", "SG-Auto", "HK-Auto", "Auto", "Fallback"];
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

function currentGroup() {
  return state.groups.find((group) => group.name === state.selected) || null;
}

function render() {
  renderGroups();
  renderNodes();
}

function renderGroups() {
  const groups = visibleGroups();
  if (!groups.length) {
    ids.groupList.innerHTML = "";
    ids.groupList.append(notice("No matching groups"));
    return;
  }
  ids.groupList.replaceChildren(...groups.map((group) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `group-item${group.name === state.selected ? " active" : ""}`;
    button.onclick = () => {
      state.selected = group.name;
      render();
    };

    const main = document.createElement("div");
    main.className = "group-main";
    main.append(textNode("div", "group-name", group.name));
    main.append(textNode("div", "group-now", group.now || "-"));

    const pill = textNode("div", "pill", String((group.all || []).length));
    button.append(main, pill);
    return button;
  }));
}

function renderNodes() {
  const group = currentGroup();
  if (!group) {
    ids.detailName.textContent = "No group selected";
    ids.detailMeta.textContent = "-";
    renderEmpty("No selector group available");
    return;
  }

  ids.detailName.textContent = group.name;
  ids.detailMeta.textContent = `${group.type || "group"} / current: ${group.now || "-"}`;
  const nodes = sortedNodes(group);
  if (!nodes.length) {
    renderEmpty("No nodes in this group");
    return;
  }
  ids.nodeList.replaceChildren(...nodes.map((name) => renderNode(group, name)));
}

function sortedNodes(group) {
  const nodes = [...(group.all || [])];
  if (!state.delaySort) return nodes;
  return nodes.sort((a, b) => {
    const ad = state.delays.get(a);
    const bd = state.delays.get(b);
    const av = ad && ad.ok ? ad.value : Number.MAX_SAFE_INTEGER;
    const bv = bd && bd.ok ? bd.value : Number.MAX_SAFE_INTEGER;
    return av - bv || a.localeCompare(b);
  });
}

function renderNode(group, name) {
  const row = document.createElement("div");
  const current = name === group.now;
  row.className = `node${current ? " current" : ""}`;

  const main = document.createElement("div");
  main.className = "node-main";
  main.append(textNode("div", "node-name", name));
  main.append(textNode("div", "node-meta", proxyType(name)));

  const delay = textNode("div", delayClass(name), delayText(name));
  const test = document.createElement("button");
  test.type = "button";
  test.textContent = "Test";
  test.disabled = state.delays.get(name)?.pending || false;
  test.onclick = () => testDelay(name);

  const use = document.createElement("button");
  use.type = "button";
  use.className = "use";
  use.textContent = current ? "Active" : "Use";
  use.disabled = current;
  use.onclick = () => switchNode(group.name, name);

  row.append(main, delay, test, use);
  return row;
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
  renderNodes();
  try {
    const data = await api(apiPath(["proxies", name, "delay"], {
      timeout: "5000",
      url: "https://www.gstatic.com/generate_204",
    }));
    state.delays.set(name, { ok: true, value: Number(data.delay) || 0 });
  } catch {
    state.delays.set(name, { ok: false, value: 0 });
  } finally {
    renderNodes();
  }
}

async function testNames(names) {
  ids.apiLine.textContent = `Testing ${names.length} node(s)`;
  setBusy(true);
  const queue = [...new Set(names)];
  const workers = Array.from({ length: Math.min(6, queue.length) }, async () => {
    while (queue.length) {
      const name = queue.shift();
      await testDelay(name);
    }
  });
  await Promise.all(workers);
  setBusy(false);
  ids.apiLine.textContent = "Latency test complete";
}

function setBusy(busy) {
  ids.testAllBtn.disabled = busy;
  ids.testCurrentBtn.disabled = busy;
}

function renderEmpty(message) {
  ids.nodeList.innerHTML = "";
  ids.nodeList.append(notice(message));
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
ids.testCurrentBtn.onclick = () => {
  const group = currentGroup();
  if (group) testNames(group.all || []);
};
ids.testAllBtn.onclick = () => {
  const names = visibleGroups().flatMap((group) => group.all || []);
  testNames(names);
};
ids.showCurrentBtn.onclick = () => {
  const group = currentGroup();
  if (!group || !group.now) return;
  state.query = group.now;
  ids.searchInput.value = state.query;
  render();
};
ids.sortDelayBtn.onclick = () => {
  state.delaySort = !state.delaySort;
  ids.sortDelayBtn.textContent = state.delaySort ? "Natural Sort" : "Sort Delay";
  renderNodes();
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
refresh();
setInterval(refresh, 20000);
