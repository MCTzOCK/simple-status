/* simple-status — status page client.
 *
 * Fetches /api/v1/summary, renders the page from it and keeps it fresh.
 * All dynamic text is inserted via textContent, never innerHTML.
 */
"use strict";

const REFRESH_MS = 10_000;
const MAX_BARS = 45;

const root = document.getElementById("root");
const titleEl = document.getElementById("page-title");
const refreshDot = document.getElementById("refresh-dot");
const refreshLabel = document.getElementById("refresh-label");

let lastUpdated = null; // Date of the last successful fetch.

/* --- tiny DOM helpers ---------------------------------------------------- */

function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (key === "text") node.textContent = value;
    else if (key === "class") node.className = value;
    else node.setAttribute(key, value);
  }
  for (const child of children) {
    if (child !== null && child !== undefined) node.append(child);
  }
  return node;
}

/* --- formatting ----------------------------------------------------------- */

function fmtLatency(ms) {
  if (ms == null) return "—";
  if (ms < 1000) return `${Math.max(1, Math.round(ms))} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

function fmtUptime(pct) {
  if (pct == null) return "—";
  return pct >= 99.995 ? "100%" : `${pct.toFixed(2)}%`;
}

function fmtRelative(iso, now) {
  const seconds = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return new Date(iso).toLocaleDateString();
}

function fmtDownDuration(iso, now) {
  const minutes = Math.floor((now - new Date(iso).getTime()) / 60_000);
  if (minutes < 1) return "under a minute";
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

/* --- rendering ------------------------------------------------------------- */

const ICONS = {
  up: '<svg viewBox="0 0 24 24" fill="none" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>',
  down: '<svg viewBox="0 0 24 24" fill="none" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18M6 6l12 12"/></svg>',
  pending: '<svg viewBox="0 0 24 24" fill="none" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 12h8"/><circle cx="12" cy="12" r="9.2"/></svg>',
};

function renderHero(summary, now) {
  const counts = summary.counts;
  const state = summary.overall;
  const title =
    state === "down"
      ? `${counts.down} of ${counts.total} ${counts.down === 1 ? "service" : "services"} down`
      : state === "pending"
        ? "Checking services…"
        : "All Systems Operational";

  const sub =
    state === "pending"
      ? "Waiting for the first results to come in."
      : `${counts.up} operational · ${counts.down} down · updated `;

  const updated = el("span", { "data-reltime": summary.updated_at, text: "just now" });
  const subNode = el("p", { class: "hero-sub" });
  subNode.append(document.createTextNode(sub));
  if (state !== "pending") subNode.append(updated);

  const icon = el("div", { class: "hero-icon" });
  icon.innerHTML = ICONS[state]; // trusted constant, not user data

  return el(
    "section",
    { class: `hero hero--${state}` },
    icon,
    el("div", { class: "hero-text" }, el("p", { class: "hero-title", text: title }), subNode),
  );
}

function renderBeatBar(service) {
  const beats = el("div", { class: "beats" });
  const history = service.history.slice(-MAX_BARS);

  if (history.length === 0) {
    for (let i = 0; i < 24; i++) {
      beats.append(el("span", { class: "beat beat--pending", title: "no data yet" }));
    }
    return beats;
  }

  const maxDuration = Math.max(...history.map((h) => h.duration_ms), 1);
  for (const entry of history) {
    const label = `${new Date(entry.at).toLocaleTimeString()} · ${
      entry.ok ? fmtLatency(entry.duration_ms) : "failed"
    }`;
    const beat = el("span", {
      class: entry.ok ? "beat" : "beat beat--fail",
      title: label,
    });
    if (entry.ok) {
      // Faster responses render slightly translucent; failures are solid red.
      beat.style.opacity = (0.45 + 0.55 * (entry.duration_ms / maxDuration)).toFixed(2);
    }
    beats.append(beat);
  }
  return beats;
}

function renderCard(service, now) {
  const statusClass = `dot--${service.status}`;

  const stats = el("div", { class: "stats" },
    el("div", { class: "chips" },
      el("span", { class: "chip" }, "24h ", el("strong", { text: fmtUptime(service.uptime["24h"]) })),
      el("span", { class: "chip" }, "7d ", el("strong", { text: fmtUptime(service.uptime["7d"]) })),
      el("span", { class: "chip" }, "30d ", el("strong", { text: fmtUptime(service.uptime["30d"]) })),
    ),
    el("span", { class: "chip" }, "latency ",
      el("strong", { text: service.status === "pending" ? "—" : fmtLatency(service.last_latency_ms) })),
  );

  const card = el(
    "article",
    { class: "card" },
    el("div", { class: "card-head" },
      el("span", { class: `dot ${statusClass}`, title: service.status }),
      el("h3", { text: service.name }),
      el("span", { class: "badge", text: service.type }),
    ),
    renderBeatBar(service),
    stats,
  );

  if (service.status === "down") {
    const note = el("p", { class: "note note--down" });
    note.append(el("strong", { text: service.last_error || "check failed" }));
    if (service.down_since) {
      const meta = el("span", {
        class: "note-meta",
        "data-downtime": service.down_since,
        text: ` · down for ${fmtDownDuration(service.down_since, now)}`,
      });
      note.append(meta);
    }
    card.insertBefore(note, card.children[1]); // between header and beats
  }

  if (service.status === "pending") {
    const note = service.history.length === 0
      ? "waiting for the first check…"
      : "checks are failing — down status awaits confirmation";
    card.insertBefore(el("p", { class: "note note--pending", text: note }), card.children[1]);
  }

  return card;
}

function render(summary) {
  const now = Date.now();
  titleEl.textContent = summary.title;
  document.title = summary.title;

  const grouped = new Map(); // group name -> services, insertion ordered
  for (const service of summary.services) {
    const group = service.group || "";
    if (!grouped.has(group)) grouped.set(group, []);
    grouped.get(group).push(service);
  }
  const useGroupTitles = grouped.size > 1 || (grouped.size === 1 && !grouped.has(""));

  root.replaceChildren(renderHero(summary, now));
  for (const [group, services] of grouped) {
    if (useGroupTitles) {
      root.append(el("h2", { class: "group-title", text: group || "Other" }));
    }
    root.append(el("div", { class: "grid" }, ...services.map((s) => renderCard(s, now))));
  }
}

/* --- refresh loop ----------------------------------------------------------- */

async function fetchSummary() {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  try {
    const response = await fetch("/api/v1/summary", { signal: controller.signal });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return await response.json();
  } finally {
    clearTimeout(timer);
  }
}

async function refresh() {
  try {
    const summary = await fetchSummary();
    lastUpdated = new Date(summary.updated_at);
    render(summary);
    refreshDot.className = "refresh-dot refresh-dot--ok";
    refreshLabel.textContent = "updated just now";
  } catch {
    refreshDot.className = "refresh-dot refresh-dot--err";
    refreshLabel.textContent = "reconnecting…";
  }
}

/* Keep relative timestamps ("3m ago", "down for 12m") current between fetches. */
setInterval(() => {
  const now = Date.now();
  for (const node of document.querySelectorAll("[data-reltime]")) {
    node.textContent = fmtRelative(node.dataset.reltime, now);
  }
  for (const node of document.querySelectorAll("[data-downtime]")) {
    node.textContent = `· down for ${fmtDownDuration(node.dataset.downtime, now)}`;
  }
  if (lastUpdated) {
    refreshLabel.textContent = `updated ${fmtRelative(lastUpdated.toISOString(), now)}`;
  }
}, 1000);

document.addEventListener("visibilitychange", () => {
  if (!document.hidden) refresh();
});

refresh();
setInterval(refresh, REFRESH_MS);
