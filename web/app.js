const COLORS = ["#00ADD8", "#A78BFA", "#3DDC97", "#FF8FAB", "#FFB547", "#5B8CFF", "#F472B6", "#2DD4BF"];
const NAMES = ["Ada", "Linus", "Grace", "Ken", "Rob", "Barbara", "Edsger", "Margaret"];
const SAMPLE_KEYS = ["color", "city", "lang", "score", "status", "theme"];
const SAMPLE_VALUES = {
  color: ["blue", "green", "orange", "purple"],
  city: ["Recife", "Lisbon", "Tokyo", "Berlin"],
  lang: ["Go", "Rust", "Python", "TypeScript"],
  score: ["42", "87", "100", "7"],
  status: ["online", "busy", "away"],
  theme: ["dark", "light"],
};
const MAX_ACTIVITY = 60;
const MAX_INBOX = 30;
const MAX_CLIENTS = 8;

const $ = (selector) => document.querySelector(selector);
const clientsEl = $("#clients");
const serverCard = $("#server-card");
const activityEl = $("#activity");
const storeBody = $("#store-body");
const template = $("#client-template");
const addButton = $("#add-client");
const autoTraffic = $("#auto-traffic");

const clients = new Map(); // client ID -> { el, name, color }
const store = new Map();
const lastRequester = new Map(); // key -> client ID of the latest get
const stats = { sent: 0, dropped: 0 };
let nameIndex = 0;
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

function pick(list) {
  return list[Math.floor(Math.random() * list.length)];
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function prepend(list, item, max) {
  list.prepend(item);
  while (list.children.length > max) list.lastElementChild.remove();
}

/* Packet animation */

function center(node) {
  const rect = node.getBoundingClientRect();
  return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
}

function flyPacket(fromEl, toEl, color, onArrive) {
  if (reducedMotion || !fromEl.isConnected || !toEl.isConnected) {
    onArrive?.();
    return;
  }
  const from = center(fromEl);
  const to = center(toEl);
  const packet = el("span", "packet");
  packet.style.setProperty("--packet", color);
  document.body.append(packet);

  const midX = (from.x + to.x) / 2;
  const midY = Math.min(from.y, to.y) - 60;
  const animation = packet.animate(
    [
      { transform: `translate(${from.x}px, ${from.y}px) scale(0.6)`, opacity: 0 },
      { transform: `translate(${midX}px, ${midY}px) scale(1)`, opacity: 1, offset: 0.5 },
      { transform: `translate(${to.x}px, ${to.y}px) scale(0.7)`, opacity: 0.9 },
    ],
    { duration: 650, easing: "cubic-bezier(0.45, 0, 0.25, 1)" },
  );
  animation.onfinish = () => {
    packet.remove();
    onArrive?.();
  };
}

function flash(node, className = "flash", duration = 500) {
  node.classList.remove(className);
  void node.offsetWidth; // Restart the animation.
  node.classList.add(className);
  setTimeout(() => node.classList.remove(className), duration);
}

/* Server panel */

function renderStats() {
  $("#stat-clients").textContent = clients.size;
  $("#stat-keys").textContent = store.size;
  $("#stat-sent").textContent = stats.sent;
  $("#stat-dropped").textContent = stats.dropped;
  $("#clients-empty")?.toggleAttribute("hidden", clients.size > 0);
  addButton.disabled = clients.size >= MAX_CLIENTS;
}

function renderStore(changedKey) {
  storeBody.replaceChildren();
  for (const [key, value] of [...store].sort(([a], [b]) => a.localeCompare(b))) {
    const row = el("tr", key === changedKey ? "changed" : "");
    row.append(el("td", "", key), el("td", "", value));
    row.title = `${key} = ${value}`;
    storeBody.append(row);
  }
  $("#store-empty").hidden = store.size > 0;
}

function clientLabel(id) {
  return clients.get(id)?.name ?? `#${id}`;
}

function logActivity(kind, parts) {
  const item = el("li");
  item.append(el("span", `tag ${kind}`, kind));
  const text = el("span");
  for (const part of parts) {
    text.append(typeof part === "string" ? document.createTextNode(part) : el("code", "", part.code));
  }
  item.append(text);
  prepend(activityEl, item, MAX_ACTIVITY);
}

function handleEvent(event) {
  const { kind, clientId, key, value, recipients } = event;
  const who = clientLabel(clientId);

  switch (kind) {
    case "connect":
      logActivity(kind, [`${who} connected`]);
      break;
    case "disconnect":
      logActivity(kind, [`${who} disconnected`]);
      break;
    case "put":
      store.set(key, value);
      renderStore(key);
      logActivity(kind, [`${who} set `, { code: key }, " = ", { code: value }]);
      break;
    case "get":
      lastRequester.set(key, clientId);
      stats.sent += recipients;
      logActivity(kind, [
        `${who} asked for `,
        { code: key },
        `, sent to ${recipients} client${recipients === 1 ? "" : "s"}`,
      ]);
      break;
    case "drop":
      stats.dropped += 1;
      logActivity(kind, [`${who}'s queue is full, message dropped`]);
      break;
  }
  renderStats();
}

/* Clients */

function addClient() {
  if (clients.size >= MAX_CLIENTS) return;
  const name = NAMES[nameIndex % NAMES.length];
  const color = COLORS[nameIndex % COLORS.length];
  nameIndex += 1;

  const node = template.content.firstElementChild.cloneNode(true);
  node.style.setProperty("--client", color);
  node.querySelector(".avatar").textContent = name[0];
  node.querySelector(".client-name").textContent = name;
  clientsEl.append(node);

  const id = window.kv.connect((clientId, line) => receive(clientId, line));
  clients.set(id, { el: node, name, color });
  node.querySelector(".client-status").textContent = `client #${id} · connected`;

  const form = node.querySelector(".command");
  const radios = form.querySelectorAll("input[type=radio]");
  radios.forEach((radio) => {
    radio.name = `op-${id}`;
    radio.addEventListener("change", () => form.classList.toggle("is-get", radio.value === "get" && radio.checked));
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const op = form.querySelector("input[type=radio]:checked").value;
    const key = form.querySelector(".key").value.trim();
    const value = form.querySelector(".value").value.trim();
    send(id, op, key, value);
  });
  node.querySelector(".disconnect").addEventListener("click", () => removeClient(id));

  renderStats();
  return id;
}

function removeClient(id) {
  const client = clients.get(id);
  if (!client) return;
  clients.delete(id);
  window.kv.disconnect(id);
  client.el.classList.add("leaving");
  client.el.addEventListener("animationend", () => client.el.remove(), { once: true });
  setTimeout(() => client.el.remove(), 400);
  renderStats();
}

// Keys and values cannot contain the separators of the protocol.
function clean(text) {
  return text.replace(/[,\r\n]/g, " ").trim();
}

function send(id, op, rawKey, rawValue) {
  const client = clients.get(id);
  const key = clean(rawKey);
  const value = rawValue.replace(/[\r\n]/g, " ").trim();
  if (!client || !key || (op === "put" && !value)) return;

  const line = op === "put" ? `put,${key},${value}` : `get,${key}`;
  flyPacket(client.el.querySelector(".send"), serverCard.querySelector(".server-icon"), client.color, () => {
    flash(serverCard, "pulse", 500);
    window.kv.send(id, line);
  });
}

function receive(id, line) {
  const client = clients.get(id);
  if (!client) return;
  const text = line.trimEnd();
  const key = text.split(",")[0];
  const requester = lastRequester.get(key);
  const from = requester === id ? "you asked" : `from ${clientLabel(requester)}`;

  flyPacket(serverCard.querySelector(".server-icon"), client.el.querySelector(".avatar"), "#A78BFA", () => {
    if (!client.el.isConnected) return;
    const item = el("li");
    item.append(el("span", "line", text), el("span", "from", from));
    item.title = text;
    prepend(client.el.querySelector(".inbox"), item, MAX_INBOX);
    flash(client.el);
  });
}

/* Auto traffic */

let trafficTimer = null;

function randomCommand() {
  const ids = [...clients.keys()];
  if (ids.length === 0) return;
  const id = pick(ids);
  const key = pick(SAMPLE_KEYS);
  if (Math.random() < 0.5 || !store.has(key)) {
    send(id, "put", key, pick(SAMPLE_VALUES[key]));
  } else {
    send(id, "get", key, "");
  }
}

autoTraffic.addEventListener("change", () => {
  clearInterval(trafficTimer);
  trafficTimer = autoTraffic.checked ? setInterval(randomCommand, 900) : null;
});

/* Start */

async function start() {
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch("kv.wasm"), go.importObject);
  window.addEventListener(
    "kv-ready",
    async () => {
      // Run after the current call returns, so that a new client already has a name.
      window.kv.onEvent = (event) => queueMicrotask(() => handleEvent(event));
      for (const [key, value] of Object.entries(await window.kv.snapshot())) store.set(key, value);
      $("#server-status").textContent = "Running in your browser";
      $("#clients-empty").textContent = "No clients. Add one to start.";
      addButton.addEventListener("click", addClient);

      // Start with a small scene: two clients and one stored value.
      const first = addClient();
      addClient();
      renderStore();
      setTimeout(() => send(first, "put", "lang", "Go"), 600);
      setTimeout(() => send(first, "get", "lang", ""), 1500);
    },
    { once: true },
  );
  go.run(result.instance);
}

start().catch((error) => {
  console.error(error);
  $("#server-status").textContent = "Could not start the server";
  $("#clients-empty").textContent = "Your browser could not load WebAssembly.";
});

renderStats();
renderStore();
