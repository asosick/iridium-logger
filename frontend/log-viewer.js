import {
  Virtualizer,
  elementScroll,
  observeElementOffset,
  observeElementRect,
} from "@tanstack/virtual-core";
import { FancyAnsi, stripAnsi } from "fancy-ansi";

const rowHeight = 28;
const ansi = new FancyAnsi();

class LogViewer {
  constructor(root) {
    this.root = root;
    this.entries = [];
    this.filtered = [];
    this.pending = [];
    this.paused = false;
    this.following = root.dataset.follow !== "false";
    this.maxEntries = clamp(number(root.dataset.maxEntries, 5000), 100, 50000);
    this.levels = parseLevels(root.dataset.levels);
    this.visibleLevels = new Set(this.levels);
    this.useAnsi = root.dataset.ansi !== "false";
    this.receivedThisSecond = 0;
    this.renderQueued = false;
    this.destroyed = false;
    this.abortController = new AbortController();

    this.viewport = root.querySelector("[data-log-viewport]");
    this.scrollSize = root.querySelector("[data-log-scroll-size]");
    this.rows = root.querySelector("[data-log-rows]");
    this.search = root.querySelector("[data-log-search]");
    this.levelControls = root.querySelector("[data-log-levels]");
    this.pauseButton = root.querySelector("[data-log-pause]");
    this.followButton = root.querySelector("[data-log-follow]");
    this.count = root.querySelector("[data-log-count]");
    this.rate = root.querySelector("[data-log-rate]");
    this.newButton = root.querySelector("[data-log-new]");

    root.style.height = root.dataset.height || "32rem";
    if (root.dataset.searchable === "false") this.search.closest(".ir-log-search").hidden = true;

    this.buildLevelControls();
    this.bind();
    this.followButton.classList.toggle("is-active", this.following);
    this.createVirtualizer();
    this.applyFilter();
    this.connect();
  }

  bind() {
    const options = { signal: this.abortController.signal };
    this.search.addEventListener("input", () => this.applyFilter(), options);
    this.pauseButton.addEventListener("click", () => this.togglePause(), options);
    this.followButton.addEventListener("click", () => this.setFollowing(!this.following), options);
    this.root.querySelector("[data-log-clear]").addEventListener("click", () => {
      this.entries = [];
      this.pending = [];
      this.applyFilter();
    }, options);
    this.newButton.addEventListener("click", () => {
      this.setFollowing(true);
      this.virtualizer.scrollToEnd();
    }, options);
    this.viewport.addEventListener("scroll", () => {
      const distanceFromEnd = this.viewport.scrollHeight - this.viewport.scrollTop - this.viewport.clientHeight;
      if (distanceFromEnd > 40 && this.following) {
        this.setFollowing(false);
      }
    }, options);
    this.rateTimer = window.setInterval(() => {
      this.rate.textContent = this.receivedThisSecond ? `${this.receivedThisSecond.toLocaleString()} lines/s` : "";
      this.receivedThisSecond = 0;
    }, 1000);
  }

  createVirtualizer() {
    this.virtualizer = new Virtualizer({
      count: 0,
      getScrollElement: () => this.viewport,
      estimateSize: () => rowHeight,
      scrollToFn: elementScroll,
      observeElementRect,
      observeElementOffset,
      overscan: 12,
      anchorTo: "end",
      followOnAppend: this.following,
      onChange: () => this.renderRows(),
    });
    this.unmountVirtualizer = this.virtualizer._didMount();
    this.virtualizer._willUpdate();
  }

  updateVirtualizer() {
    this.virtualizer.setOptions({
      ...this.virtualizer.options,
      count: this.filtered.length,
      followOnAppend: this.following,
    });
    this.virtualizer._willUpdate();
    this.renderRows();
    if (this.following) {
      requestAnimationFrame(() => {
        if (this.following) this.virtualizer.scrollToEnd();
      });
    }
  }

  buildLevelControls() {
    for (const level of this.levels) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "ir-log-level";
      button.dataset.level = level;
      button.ariaPressed = "true";
      button.textContent = level;
      button.addEventListener("click", () => {
        if (this.visibleLevels.has(level)) this.visibleLevels.delete(level);
        else this.visibleLevels.add(level);
        button.ariaPressed = String(this.visibleLevels.has(level));
        this.applyFilter();
      }, { signal: this.abortController.signal });
      this.levelControls.append(button);
    }
  }

  connect() {
    const endpoint = new URL(this.root.dataset.endpoint, document.baseURI);
    endpoint.searchParams.set("source", this.root.dataset.source);
    if (this.root.dataset.directory) endpoint.searchParams.set("directory", this.root.dataset.directory);
    endpoint.searchParams.set("lines", this.root.dataset.initialLines || "250");
    this.setStatus("connecting", "Connecting");
    this.eventSource = new EventSource(endpoint, { withCredentials: true });
    this.eventSource.addEventListener("ready", () => this.setStatus("live", "Live"));
    this.eventSource.addEventListener("line", (event) => {
      try {
        this.receive(JSON.parse(event.data));
      } catch {
        // A malformed event is isolated from the remaining stream.
      }
    });
    this.eventSource.addEventListener("problem", (event) => {
      try { this.setStatus("error", JSON.parse(event.data)); }
      catch { this.setStatus("error", "Stream unavailable"); }
    });
    this.eventSource.onerror = () => this.setStatus("connecting", "Reconnecting");
  }

  receive(raw) {
    const entry = parseEntry(raw);
    this.receivedThisSecond += 1;
    if (this.paused) {
      this.pending.push(entry);
      if (this.pending.length > this.maxEntries) this.pending.shift();
      this.updateNewButton();
      return;
    }
    this.entries.push(entry);
    trimToLimit(this.entries, this.maxEntries);
    this.scheduleFilter();
  }

  scheduleFilter() {
    if (this.renderQueued) return;
    this.renderQueued = true;
    requestAnimationFrame(() => {
      this.renderQueued = false;
      this.applyFilter();
    });
  }

  applyFilter() {
    const query = this.search.value.trim().toLocaleLowerCase();
    this.filtered = this.entries.filter((entry) => {
      const levelVisible = !entry.level || !this.levels.includes(entry.level) || this.visibleLevels.has(entry.level);
      return levelVisible && (!query || entry.plain.toLocaleLowerCase().includes(query));
    });
    this.count.textContent = `${this.filtered.length.toLocaleString()} ${this.filtered.length === 1 ? "entry" : "entries"}`;
    this.updateVirtualizer();
    this.updateNewButton();
  }

  renderRows() {
    if (this.destroyed) return;
    this.scrollSize.style.height = `${this.virtualizer.getTotalSize()}px`;
    const fragment = document.createDocumentFragment();
    for (const item of this.virtualizer.getVirtualItems()) {
      const entry = this.filtered[item.index];
      if (!entry) continue;
      const row = document.createElement("div");
      row.className = "ir-log-row";
      row.dataset.index = String(item.index);
      row.dataset.level = entry.level;
      row.style.transform = `translateY(${item.start}px)`;
      row.style.height = `${item.size}px`;
      row.title = entry.plain;

      const timestamp = document.createElement("time");
      timestamp.textContent = displayTime(entry.time);
      const level = document.createElement("span");
      level.className = "ir-log-row-level";
      level.textContent = entry.level || "LOG";
      const message = document.createElement("span");
      message.className = "ir-log-row-message";
      if (this.useAnsi) message.innerHTML = ansi.toHtml(entry.message || entry.raw);
      else message.textContent = stripAnsi(entry.message || entry.raw);
      row.append(timestamp, level, message);
      fragment.append(row);
    }
    this.rows.replaceChildren(fragment);
  }

  togglePause() {
    this.paused = !this.paused;
    this.pauseButton.ariaPressed = String(this.paused);
    this.pauseButton.textContent = this.paused ? "Resume" : "Pause";
    if (!this.paused && this.pending.length) {
      this.entries.push(...this.pending);
      this.pending = [];
      trimToLimit(this.entries, this.maxEntries);
      this.applyFilter();
    }
    this.updateNewButton();
  }

  setFollowing(value) {
    this.following = value;
    this.followButton.ariaPressed = String(value);
    this.followButton.classList.toggle("is-active", value);
    this.updateNewButton();
    this.updateVirtualizer();
  }

  updateNewButton() {
    const show = this.pending.length > 0 || (!this.following && this.entries.length > 0);
    this.newButton.hidden = !show;
    this.newButton.textContent = this.pending.length ? `${this.pending.length.toLocaleString()} new` : "Jump to latest";
  }

  setStatus(state, label) {
    const status = this.root.querySelector(".ir-log-status");
    status.dataset.status = state;
    status.querySelector("[data-status-label]").textContent = label;
  }

  destroy() {
    if (this.destroyed) return;
    this.destroyed = true;
    this.eventSource?.close();
    this.abortController.abort();
    this.unmountVirtualizer?.();
    window.clearInterval(this.rateTimer);
    this.root.removeAttribute("data-log-initialized");
    delete this.root.iridiumLogViewer;
  }
}

function parseEntry(raw) {
  const plain = stripAnsi(raw);
  const fields = {};
  const matcher = /(?:^|\s)([A-Za-z_][\w.-]*)=("(?:\\.|[^"\\])*"|[^\s]+)/g;
  let match;
  while ((match = matcher.exec(plain)) !== null) {
    let value = match[2];
    if (value.startsWith('"')) {
      try { value = JSON.parse(value); }
      catch { value = value.slice(1, -1); }
    }
    fields[match[1]] = value;
  }
  return {
    raw,
    plain,
    time: fields.time || fields.timestamp || "",
    level: normalizeLevel(fields.level || ""),
    message: fields.msg || fields.message || "",
  };
}

function normalizeLevel(level) {
  const normalized = level.toUpperCase();
  if (normalized === "WARNING") return "WARN";
  return normalized;
}

function displayTime(value) {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value.slice(0, 12);
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
  }).format(parsed);
}

function parseLevels(value = "") {
  return [...new Set(value.split(",").map(normalizeLevel).filter(Boolean))];
}

function number(value, fallback) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function clamp(value, minimum, maximum) {
  return Math.min(Math.max(value, minimum), maximum);
}

function trimToLimit(entries, limit) {
  if (entries.length > limit) entries.splice(0, entries.length - limit);
}

function initialize(root = document) {
  const candidates = root.matches?.("[data-ir-log-viewer]") ? [root] : root.querySelectorAll?.("[data-ir-log-viewer]") || [];
  for (const element of candidates) {
    if (element.dataset.logInitialized === "true") continue;
    element.dataset.logInitialized = "true";
    element.iridiumLogViewer = new LogViewer(element);
  }
}

function destroyWithin(root) {
  if (!root) return;
  const candidates = root.matches?.("[data-ir-log-viewer]") ? [root] : root.querySelectorAll?.("[data-ir-log-viewer]") || [];
  for (const element of candidates) element.iridiumLogViewer?.destroy();
}

document.addEventListener("DOMContentLoaded", () => initialize());
document.addEventListener("htmx:beforeSwap", (event) => {
  if (event.detail.shouldSwap === false) return;
  destroyWithin(event.detail.target);
});
document.addEventListener("htmx:afterSwap", (event) => initialize(event.detail.target));
document.addEventListener("htmx:afterSettle", (event) => initialize(event.detail.target));
document.addEventListener("htmx:beforeCleanupElement", (event) => {
  destroyWithin(event.detail.elt);
});

initialize();
