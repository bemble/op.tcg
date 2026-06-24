// WebAwesome styles (bundled by Vite -> fully offline). Includes default theme.
import "@awesome.me/webawesome/dist/styles/webawesome.css";

// Register only the components we use (keeps the bundle lean, all offline).
import "@awesome.me/webawesome/dist/components/button/button.js";
import "@awesome.me/webawesome/dist/components/input/input.js";
import "@awesome.me/webawesome/dist/components/textarea/textarea.js";
import "@awesome.me/webawesome/dist/components/select/select.js";
import "@awesome.me/webawesome/dist/components/option/option.js";
import "@awesome.me/webawesome/dist/components/card/card.js";
import "@awesome.me/webawesome/dist/components/badge/badge.js";
import "@awesome.me/webawesome/dist/components/spinner/spinner.js";
import "@awesome.me/webawesome/dist/components/tab-group/tab-group.js";
import "@awesome.me/webawesome/dist/components/tab/tab.js";
import "@awesome.me/webawesome/dist/components/tab-panel/tab-panel.js";
import "@awesome.me/webawesome/dist/components/callout/callout.js";
import "@awesome.me/webawesome/dist/components/dialog/dialog.js";

import "./icons"; // registers the offline Font Awesome "fa" <wa-icon> library
import "./app.css";
import { api } from "./api";
import {
  COLLECTION_GOALS,
  FAMILIES,
  LANGUAGES,
  type Card,
  type CollectionGoal,
  type Item,
  type Owner,
  type SetCard,
  type SetDetail,
  type SetMeta,
} from "./types";

const GOAL_VALUES = COLLECTION_GOALS.map((g) => g.value);

const app = document.querySelector<HTMLDivElement>("#app")!;

interface State {
  owners: Owner[];
  cardCount: number;
  goal: CollectionGoal;
  families: string[];
}
const state: State = { owners: [], cardCount: 0, goal: "master", families: ["main"] };

function esc(s: string): string {
  return (s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!,
  );
}

// Remote card-image hosts send `Cross-Origin-Resource-Policy: same-site`, so
// the browser refuses to render them cross-origin. Route through the backend
// proxy (/api/img) which re-serves them from our own origin. `width` maps to
// the proxy's ?w (omit for the server default; 0 = full-size source).
function proxied(src: string, width?: number): string {
  if (!src || src.startsWith("/")) return src;
  let url = `/api/img?u=${encodeURIComponent(src)}`;
  if (width !== undefined) url += `&w=${width}`;
  return url;
}

function imgTag(card: Card): string {
  const raw = card.imageSmall || card.imageLarge || "";
  if (!raw) return `<div class="card-img placeholder">Pas d'image</div>`;
  // data-full points at the full-size proxied image; a delegated click handler
  // opens it in a lightbox (see initLightbox).
  return `<img class="card-img" src="${esc(proxied(raw))}" alt="${esc(card.name)}" loading="lazy"
    data-full="${esc(proxied(raw, 0))}"
    onerror="this.replaceWith(Object.assign(document.createElement('div'),{className:'card-img placeholder',textContent:'Hors-ligne / pas d\\'image'}))" />`;
}

// A compact thumbnail for the list view. Carries data-full so the same
// lightbox handler zooms it, like the grid tiles.
function thumbTag(card: Card): string {
  const raw = card.imageSmall || card.imageLarge || "";
  if (!raw) return `<div class="list-thumb placeholder"></div>`;
  return `<img class="list-thumb" src="${esc(proxied(raw, 80))}" alt="${esc(card.name)}" loading="lazy"
    data-full="${esc(proxied(raw, 0))}" />`;
}

// ---- grid / list view toggle (shared by the set detail and search) ----

type ViewMode = "grid" | "list";
let viewMode: ViewMode = localStorage.getItem("viewMode") === "list" ? "list" : "grid";

function viewToggle(): string {
  return `
  <div class="view-toggle" role="group" aria-label="Affichage">
    <button class="vt-grid${viewMode === "grid" ? " active" : ""}" type="button" title="Vue grille" aria-label="Vue grille"><wa-icon library="fa" name="table-cells"></wa-icon></button>
    <button class="vt-list${viewMode === "list" ? " active" : ""}" type="button" title="Vue liste" aria-label="Vue liste"><wa-icon library="fa" name="list"></wa-icon></button>
  </div>`;
}

// wireViewToggle: clicking a toggle button switches mode, persists it, updates
// the button highlight, and repaints the caller's view.
function wireViewToggle(scope: ParentNode, repaint: () => void) {
  const set = (m: ViewMode) => {
    if (viewMode === m) return;
    viewMode = m;
    localStorage.setItem("viewMode", m);
    scope.querySelector(".vt-grid")?.classList.toggle("active", m === "grid");
    scope.querySelector(".vt-list")?.classList.toggle("active", m === "list");
    repaint();
  };
  scope.querySelector(".vt-grid")?.addEventListener("click", () => set("grid"));
  scope.querySelector(".vt-list")?.addEventListener("click", () => set("list"));
}

// initLightbox wires a single delegated click handler: clicking any card image
// opens it full-size in a dialog. One handler covers every tile in every tab,
// including ones rendered later.
let lightbox: HTMLElement | null = null;
function initLightbox() {
  document.addEventListener("click", (e) => {
    const img = (e.target as HTMLElement)?.closest?.("img[data-full]") as HTMLImageElement | null;
    if (!img) return;
    const full = img.getAttribute("data-full")!;
    if (!lightbox) {
      lightbox = document.createElement("wa-dialog");
      // No title for the lightbox, so drop the header entirely (otherwise it
      // leaves an empty bar). Light dismiss lets clicking the backdrop close it;
      // we add our own floating close button since the header one is gone.
      lightbox.setAttribute("without-header", "");
      lightbox.setAttribute("light-dismiss", "");
      lightbox.classList.add("lightbox");
      document.body.appendChild(lightbox);
    }
    // data-dialog="close" is what wa-dialog wires up to close itself.
    lightbox.innerHTML = `
      <wa-button class="lightbox-close" appearance="plain" data-dialog="close" aria-label="Fermer">×</wa-button>
      <img class="lightbox-img" src="${esc(full)}" alt="${esc(img.alt)}" />`;
    (lightbox as any).open = true;
  });
}

// Flags shown in front of language codes everywhere a language appears.
const LANG_FLAGS: Record<string, string> = { EN: "🇬🇧", FR: "🇫🇷", JP: "🇯🇵" };
function langLabel(code: string): string {
  const f = LANG_FLAGS[code];
  return f ? `${f} ${esc(code)}` : esc(code);
}
function langOptions(selected: string): string {
  return LANGUAGES.map(
    (v) => `<wa-option value="${v}"${v === selected ? " selected" : ""}>${langLabel(v)}</wa-option>`,
  ).join("");
}

// Owner <wa-option>s, with an "unspecified" entry mapped to value "".
function ownerOptions(selected: number | null): string {
  const sel = selected ?? "";
  const none = `<wa-option value=""${sel === "" ? " selected" : ""}>Non attribué</wa-option>`;
  return (
    none +
    state.owners
      .map(
        (o) =>
          `<wa-option value="${o.id}"${o.id === sel ? " selected" : ""}>${esc(o.name)}</wa-option>`,
      )
      .join("")
  );
}

function parseOwner(v: string): number | null {
  const n = parseInt(v, 10);
  return Number.isFinite(n) && n > 0 ? n : null;
}

function toast(message: string, variant: "success" | "danger" = "success") {
  const el = document.createElement("wa-callout");
  el.setAttribute("variant", variant);
  el.className = "toast";
  el.textContent = message;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 3200);
}

// ---------- shell ----------

async function boot() {
  initLightbox();
  app.innerHTML = `<div class="loading"><wa-spinner></wa-spinner></div>`;
  let sync;
  try {
    const [health, owners, status, settings] = await Promise.all([
      api.health(),
      api.listOwners(),
      api.syncStatus(),
      api.getSettings(),
    ]);
    state.cardCount = health.cardCount;
    state.owners = owners;
    if (GOAL_VALUES.includes(settings.collectionGoal)) state.goal = settings.collectionGoal;
    if (Array.isArray(settings.families) && settings.families.length) state.families = settings.families;
    sync = status;
  } catch (e) {
    app.innerHTML = `<wa-callout variant="danger">Backend injoignable : ${esc(
      (e as Error).message,
    )}</wa-callout>`;
    return;
  }
  renderShell();
  // A startup sync may already be running on the backend — surface it.
  if (sync.syncing) pollSync();
}

function renderShell() {
  app.innerHTML = `
    <header class="topbar">
      <h1><span class="app-logo" role="img" aria-label="op.tcg"></span>op.tcg</h1>
      <div id="stats" class="stats"></div>
    </header>
    <wa-tab-group>
      <wa-tab slot="nav" panel="collection">Ma collection</wa-tab>
      <wa-tab slot="nav" panel="stats">Statistiques</wa-tab>
      <wa-tab slot="nav" panel="search">Rechercher &amp; ajouter</wa-tab>
      <wa-tab slot="nav" panel="prefs">Préférences</wa-tab>
      <wa-tab-panel name="collection"><div id="collection"></div></wa-tab-panel>
      <wa-tab-panel name="stats"><div id="stats-page"></div></wa-tab-panel>
      <wa-tab-panel name="search"><div id="search"></div></wa-tab-panel>
      <wa-tab-panel name="prefs"><div id="prefs"></div></wa-tab-panel>
    </wa-tab-group>
  `;
  // Search & Préférences panels are route-independent — render once.
  renderSearch();
  renderPrefs();
  refreshStats();

  // Reflect tab clicks in the URL so browser back/forward work.
  const tg = document.querySelector("wa-tab-group");
  tg?.addEventListener("wa-tab-show", (e) => {
    const name = (e as CustomEvent).detail?.name as string | undefined;
    if (!name || currentRoute().tab === name) return; // already there (incl. sub-routes)
    navigate(name === "collection" ? "/collection" : `/${name}`);
  });
  // Clicking the "Ma collection" tab always returns to the sets overview, even
  // when already inside a set detail (/collection/OP16).
  tg?.querySelector('wa-tab[panel="collection"]')?.addEventListener("click", () => {
    navigate("/collection");
  });
  window.addEventListener("popstate", router);
  router(); // render the view for the current URL
}

// ---------- routing (History API / clean URLs, so back/forward work) ----------

const TABS = ["collection", "stats", "search", "prefs"];

function currentRoute(): { tab: string; set: string | null } {
  const parts = location.pathname.split("/").filter(Boolean);
  const tab = TABS.includes(parts[0]) ? parts[0] : "collection";
  return { tab, set: tab === "collection" ? parts[1] ?? null : null };
}

// Change the URL (adds a history entry), then render. pushState doesn't fire
// popstate, so we call router() directly; back/forward fire popstate -> router.
function navigate(path: string) {
  if (location.pathname !== path) history.pushState(null, "", path);
  router();
}

function activateTab(name: string) {
  const tg = document.querySelector("wa-tab-group") as any;
  if (tg && tg.active !== name) tg.active = name; // documented API: "active"
}

function router() {
  const { tab, set } = currentRoute();
  activateTab(tab);
  if (tab === "collection") {
    colSet = set;
    if (set) renderSetDetail(set);
    else renderSetsOverview();
  } else if (tab === "stats") {
    renderStats(); // re-fetch fresh each visit
  }
}

// Header badge: only the count of owned (distinct) cards. Everything else
// lives on the dedicated Statistiques tab.
async function refreshStats() {
  const host = document.querySelector<HTMLDivElement>("#stats");
  if (!host) return;
  try {
    const s = await api.stats();
    host.innerHTML = `<wa-badge variant="brand" pill>${s.uniqueCards} cartes possédées</wa-badge>`;
  } catch {
    host.innerHTML = "";
  }
}

// ---------- statistiques tab ----------

function statCard(label: string, value: string | number, sub = ""): string {
  return `
  <div class="stat-card">
    <div class="stat-num">${esc(String(value))}</div>
    <div class="stat-cap">${esc(label)}</div>
    ${sub ? `<div class="muted small">${esc(sub)}</div>` : ""}
  </div>`;
}

function statSection(title: string, body: string): string {
  return `<section class="stat-section"><h2>${esc(title)}</h2>${body}</section>`;
}

// Render a rarity label as a colored badge, OP.TCG-style:
//   base rarities (C/UC/R/SR/L/DON!!)  → black on white, black border
//   SEC / DON!! Gold                    → black on gold, black border
//   parallels P1/P2/P3/P4               → teal / purple / green / grey
function rarityBadge(label: string): string {
  let cls = "rb-white";
  if (label === "SEC" || label === "DON!! Gold") {
    cls = "rb-gold";
  } else {
    const m = /^P(\d+)$/.exec(label);
    if (m) cls = `rb-p${Math.min(Number(m[1]), 4)}`;
  }
  return `<span class="rarity-badge ${cls}">${esc(label)}</span>`;
}

async function renderStats() {
  const host = document.querySelector<HTMLDivElement>("#stats-page");
  if (!host) return;
  host.innerHTML = `<div class="loading"><wa-spinner></wa-spinner></div>`;
  let s;
  try {
    s = await api.fullStats();
  } catch (e) {
    host.innerHTML = errCallout(e);
    return;
  }

  const pct = s.catalogueTotal ? Math.round((s.goalOwned / s.catalogueTotal) * 100) : 0;
  const empty = `<p class="muted">Aucune carte pour l'instant.</p>`;

  // Defensive defaults so a missing array never throws / hides a section.
  const byGroup = s.byGroup ?? [];
  const byOwner = s.byOwner ?? [];
  const byLanguage = s.byLanguage ?? [];
  const byRarity = s.byRarity ?? [];

  const groups = byGroup
    .map((b) => {
      const p = b.total ? Math.round(((b.owned ?? 0) / b.total) * 100) : 0;
      const done = (b.total ?? 0) > 0 && (b.owned ?? 0) >= (b.total ?? 0);
      return `
      <div class="stat-row">
        <span class="stat-label">${esc(b.label)}</span>
        <span class="stat-val">${b.owned ?? 0}/${b.total ?? 0} · ${p}%</span>
      </div>
      <div class="progress"><div class="progress-bar${done ? " done" : ""}" style="width:${p}%"></div></div>`;
    })
    .join("");

  const owners = byOwner.length
    ? byOwner
        .map(
          (b) => `<div class="stat-row"><span class="stat-label">${esc(b.label)}</span>
            <span class="stat-val">${b.owned} cartes · ${b.copies} ex.</span></div>`,
        )
        .join("")
    : empty;

  const langs = byLanguage.length
    ? byLanguage
        .map(
          (b) => `<div class="stat-row"><span class="stat-label">${langLabel(b.label)}</span>
            <span class="stat-val">${b.copies} ex.</span></div>`,
        )
        .join("")
    : empty;

  const rarities = byRarity.length
    ? byRarity
        .map(
          (b) => `<div class="stat-row"><span class="stat-label">${rarityBadge(b.label)}</span>
            <span class="stat-val">${b.owned}</span></div>`,
        )
        .join("")
    : empty;

  const goalLabel = COLLECTION_GOALS.find((g) => g.value === s.goal)?.label ?? s.goal;
  host.innerHTML = `
    <div class="stat-goal">
      <span class="muted">Complétion selon l'objectif :</span>
      <wa-badge variant="brand" pill>${esc(goalLabel)}</wa-badge>
      <span class="muted small">(les répartitions ci-dessous couvrent toute la collection)</span>
    </div>
    <div class="stat-cards">
      ${statCard("Cartes possédées", s.owned)}
      ${statCard("Exemplaires", s.copies)}
      ${statCard("Complétion catalogue", `${pct}%`, `${s.goalOwned} / ${s.catalogueTotal}`)}
      ${statCard("Sets complétés", `${s.setsComplete} / ${s.setsTotal}`)}
    </div>
    ${statSection("Par famille", groups)}
    ${statSection("Par propriétaire", owners)}
    ${statSection("Par langue", langs)}
    ${statSection("Par rareté", rarities)}`;
}

// ---------- collection tab (browse by set, OP.TCG-style) ----------

// null = sets overview; a code = that set's detail.
let colSet: string | null = null;
let editMode = false; // off = browse/zoom only; on = add/edit/remove
let batchAdd = false; // select many cards, then add in one go (per-row in list, global owner/lang in grid)

// Set-detail display options (persisted in localStorage).
type OwnFilter = "all" | "owned" | "missing";
let ownFilter: OwnFilter = ((): OwnFilter => {
  const v = localStorage.getItem("ownFilter");
  return v === "owned" || v === "missing" ? v : "all";
})();
let goalOnly = localStorage.getItem("goalOnly") === "1";
let parallelsAtEnd = localStorage.getItem("parallelsAtEnd") === "1";
let showFilters = localStorage.getItem("showFilters") === "1"; // filters hidden by default

// Display rank when "parallèles à la fin" is on: standard, then DON!!, then parallels.
function cardSortRank(c: SetCard): number {
  if (c.rarity === "DON!!") return 1;
  return c.cardId !== c.code ? 2 : 0; // parallel vs standard base
}

// Owner filter: 0 = everyone. When set to an owner, cards that owner doesn't
// have are shown greyed (as missing), not hidden.
let setOwner = parseInt(localStorage.getItem("setOwner") || "0", 10) || 0;

// effectiveCard recomputes owned/quantity from the point of view of one owner
// (keeping items intact for the manage dialog). ownerId 0 = aggregate (all).
function effectiveCard(c: SetCard, ownerId: number): SetCard {
  if (!ownerId) return c;
  const mine = (c.items || []).filter((it) => it.ownerId === ownerId);
  const quantity = mine.reduce((n, it) => n + it.quantity, 0);
  return { ...c, owned: mine.length > 0, quantity };
}

function renderCollection() {
  if (colSet) renderSetDetail(colSet);
  else renderSetsOverview();
}

// Re-render the active collection view + the header stats.
function refreshCollection() {
  renderCollection();
  refreshStats();
}

function errCallout(e: unknown): string {
  return `<wa-callout variant="danger" class="span">${esc((e as Error).message)}</wa-callout>`;
}

async function renderSetsOverview() {
  colSet = null;
  const host = document.querySelector<HTMLDivElement>("#collection")!;
  host.innerHTML = `<div class="loading"><wa-spinner></wa-spinner></div>`;
  let sets;
  try {
    sets = await api.sets();
  } catch (e) {
    host.innerHTML = errCallout(e);
    return;
  }
  if (sets.length === 0) {
    host.innerHTML = `<wa-callout class="span">Catalogue vide. Va dans <strong>Préférences</strong> et synchronise le catalogue.</wa-callout>`;
    return;
  }
  // Sets arrive already ordered by family (OP, EB, PRB, ST, then others). Each
  // family is one card: a header (global completion ring + title + total) then
  // its sets as a plain list.
  // All families are listed; progress (ring + bars + counts) is shown only for
  // families included in the collection goal — others are browsable without it.
  let html = "";
  let lastFamily: string | null = null;
  for (const s of sets) {
    if (s.family !== lastFamily) {
      if (lastFamily !== null) html += `</div></div>`; // close list + section
      const inGoal = state.families.includes(s.family);
      const g = sets.filter((x) => x.family === s.family);
      const owned = g.reduce((n, x) => n + x.owned, 0);
      const total = g.reduce((n, x) => n + x.total, 0);
      const pct = total ? Math.round((owned / total) * 100) : 0;
      html += `<div class="set-section${inGoal ? "" : " off-goal"}">
        <div class="set-group-head">
          ${inGoal ? ring(pct) : ""}
          <span class="set-group-title">${esc(s.group)}</span>
          <span class="set-group-count muted">${owned}/${total}</span>
        </div>
        <div class="set-section-list">`;
      lastFamily = s.family;
    }
    html += setRow(s, state.families.includes(s.family));
  }
  if (lastFamily !== null) html += `</div></div>`;
  host.innerHTML = `<div class="sets-list">${html}</div>`;
  // Real links (right-click / new-tab work); a plain left-click navigates in-app.
  sets.forEach((s) =>
    host
      .querySelector(`.set-row[data-code="${cssId(s.code)}"]`)
      ?.addEventListener("click", (e) => {
        const me = e as MouseEvent;
        if (me.metaKey || me.ctrlKey || me.shiftKey || me.altKey || me.button !== 0) return;
        e.preventDefault();
        navigate(`/collection/${s.code}`);
      }),
  );
}

// A donut progress ring (pure CSS conic-gradient) showing a percentage.
function ring(pct: number): string {
  return `<span class="ring" style="--pct:${pct}%"><span class="ring-inner">${pct}<small>%</small></span></span>`;
}

// Split a set code like "OP01" into its family letters and number, for the
// stacked circular badge (OP / 01).
function splitCode(code: string): { alpha: string; num: string } {
  const m = /^([A-Za-z]+)(.*)$/.exec(code);
  return m ? { alpha: m[1], num: m[2] } : { alpha: code, num: "" };
}

function setRow(s: SetMeta, showProgress = true): string {
  const pct = s.total ? Math.round((s.owned / s.total) * 100) : 0;
  const done = showProgress && s.total > 0 && s.owned >= s.total;
  const { alpha, num } = splitCode(s.code);
  return `
  <a class="set-row${done ? " done" : ""}" data-code="${esc(s.code)}" href="/collection/${esc(s.code)}">
    <span class="set-badge"><b>${esc(alpha)}</b><span>${esc(num)}</span></span>
    <span class="set-row-main">
      <span class="set-name">${esc(s.name || s.code)}</span>
      ${showProgress ? `<div class="progress"><div class="progress-bar" style="width:${pct}%"></div></div>` : ""}
    </span>
    <span class="set-count">${s.owned}/${s.total}${done ? " ✓" : ""}</span>
  </a>`;
}

async function renderSetDetail(code: string) {
  colSet = code;
  const host = document.querySelector<HTMLDivElement>("#collection")!;
  host.innerHTML = `<div class="loading"><wa-spinner></wa-spinner></div>`;
  let detail;
  try {
    detail = await api.setDetail(code);
  } catch (e) {
    host.innerHTML = errCallout(e);
    return;
  }
  const pct = detail.total ? Math.round((detail.owned / detail.total) * 100) : 0;
  host.innerHTML = `
    <div class="set-detail-head">
      <wa-button id="set-back" appearance="outlined" size="small">← Sets</wa-button>
      <div class="set-detail-title">
        <strong>${esc(code)}</strong> ${esc(detail.name || "")}
        <span class="muted small" id="set-progress">· ${detail.owned}/${detail.total} (${pct}%)</span>
      </div>
      <div class="set-display-options">
        <span class="icon-group">
          <button type="button" id="edit-mode" class="icon-toggle edit-toggle${editMode ? " active" : ""}" title="Mode édition" aria-label="Mode édition" aria-pressed="${editMode}"><wa-icon library="fa" name="pen-to-square"></wa-icon></button>
          <button type="button" id="batch-mode" class="icon-toggle batch-toggle${batchAdd ? " active" : ""}" title="Ajout par lot" aria-label="Ajout par lot" aria-pressed="${batchAdd}"><wa-icon library="fa" name="list-check"></wa-icon></button>
        </span>
        <button type="button" id="filters-toggle" class="icon-toggle${showFilters ? " active" : ""}" title="Filtres" aria-label="Afficher/masquer les filtres" aria-pressed="${showFilters}"><wa-icon library="fa" name="filter"></wa-icon></button>
        <button type="button" id="par-end" class="icon-toggle${parallelsAtEnd ? " active" : ""}" title="Parallèles à la fin" aria-label="Parallèles à la fin" aria-pressed="${parallelsAtEnd}"><wa-icon library="fa" name="layer-group"></wa-icon></button>
        ${viewToggle()}
      </div>
    </div>
    <div class="set-filters${showFilters ? "" : " is-hidden"}" id="set-filters">
      ${
        state.owners.length
          ? `<wa-select id="set-owner" value="${setOwner}" size="small" style="min-width:150px">
        <wa-option value="0"${setOwner === 0 ? " selected" : ""}>Tous propriétaires</wa-option>
        ${state.owners.map((o) => `<wa-option value="${o.id}"${o.id === setOwner ? " selected" : ""}>${esc(o.name)}</wa-option>`).join("")}
      </wa-select>`
          : ""
      }
      <wa-select id="own-filter" value="${ownFilter}" size="small" style="min-width:140px">
        <wa-option value="all"${ownFilter === "all" ? " selected" : ""}>Toutes</wa-option>
        <wa-option value="owned"${ownFilter === "owned" ? " selected" : ""}>Possédées</wa-option>
        <wa-option value="missing"${ownFilter === "missing" ? " selected" : ""}>Manquantes</wa-option>
      </wa-select>
      <label class="set-toggle">
        <input type="checkbox" id="goal-only"${goalOnly ? " checked" : ""}/> Objectif seulement
      </label>
    </div>
    <div class="progress big"><div class="progress-bar" id="set-bar" style="width:${pct}%"></div></div>
    <div id="batch-bar"></div>
    <div id="set-grid" class="grid"></div>`;
  host.querySelector("#set-back")?.addEventListener("click", () => navigate("/collection"));
  host.querySelector("#filters-toggle")?.addEventListener("click", (e) => {
    showFilters = !showFilters;
    localStorage.setItem("showFilters", showFilters ? "1" : "0");
    const b = e.currentTarget as HTMLElement;
    b.classList.toggle("active", showFilters);
    b.setAttribute("aria-pressed", String(showFilters));
    host.querySelector("#set-filters")?.classList.toggle("is-hidden", !showFilters);
  });
  wireViewToggle(host, () => paintSetGrid(detail));
  host.querySelector("#set-owner")?.addEventListener("change", (e) => {
    setOwner = parseInt((e.target as HTMLInputElement).value, 10) || 0;
    localStorage.setItem("setOwner", String(setOwner));
    paintSetGrid(detail);
  });
  host.querySelector("#own-filter")?.addEventListener("change", (e) => {
    ownFilter = (e.target as HTMLInputElement).value as OwnFilter;
    localStorage.setItem("ownFilter", ownFilter);
    paintSetGrid(detail);
  });
  const toggleBtn = (id: string, set: (v: boolean) => void, get: () => boolean) =>
    host.querySelector(`#${id}`)?.addEventListener("click", (e) => {
      set(!get());
      const b = e.currentTarget as HTMLElement;
      b.classList.toggle("active", get());
      b.setAttribute("aria-pressed", String(get()));
      paintSetGrid(detail);
    });
  host.querySelector("#goal-only")?.addEventListener("change", (e) => {
    goalOnly = (e.target as HTMLInputElement).checked;
    localStorage.setItem("goalOnly", goalOnly ? "1" : "0");
    paintSetGrid(detail);
  });
  toggleBtn(
    "par-end",
    (v) => {
      parallelsAtEnd = v;
      localStorage.setItem("parallelsAtEnd", v ? "1" : "0");
    },
    () => parallelsAtEnd,
  );
  toggleBtn("edit-mode", (v) => (editMode = v), () => editMode);
  toggleBtn("batch-mode", (v) => (batchAdd = v), () => batchAdd);
  paintSetGrid(detail);
}

function paintSetGrid(detail: SetDetail) {
  const grid = document.querySelector<HTMLDivElement>("#set-grid");
  if (!grid) return;

  // Owner view: recompute owned/quantity from the chosen owner's perspective
  // (cards they lack stay visible but greyed). 0 / unknown owner = aggregate.
  const owner = setOwner && state.owners.some((o) => o.id === setOwner) ? setOwner : 0;
  let cards = detail.cards.map((c) => effectiveCard(c, owner));

  // Header progress reflects the (owner-scoped) in-goal completion.
  const ownedInGoal = cards.filter((c) => c.inGoal && c.owned).length;
  const pct = detail.total ? Math.round((ownedInGoal / detail.total) * 100) : 0;
  const prog = document.querySelector("#set-progress");
  if (prog) prog.textContent = `· ${ownedInGoal}/${detail.total} (${pct}%)`;
  const bar = document.querySelector<HTMLDivElement>("#set-bar");
  if (bar) bar.style.width = `${pct}%`;

  if (ownFilter === "owned") cards = cards.filter((c) => c.owned);
  else if (ownFilter === "missing") cards = cards.filter((c) => !c.owned);
  if (goalOnly) cards = cards.filter((c) => c.inGoal);
  if (parallelsAtEnd) {
    // Stable sort keeps the backend's code order within each group.
    cards = [...cards].sort((a, b) => cardSortRank(a) - cardSortRank(b));
  }
  const listView = viewMode === "list";
  const batch = batchAdd;
  // List rows are always actionable, so the "Mode édition" toggle is irrelevant
  // there — hidden via the .view-list class on the collection container.
  document.querySelector("#collection")?.classList.toggle("view-list", listView);

  // Grid batch uses global owner/language (set in the bar); list batch is
  // per-row.
  paintBatchBar(batch, !listView);

  grid.className = listView ? "card-list" : "grid";
  grid.innerHTML = cards
    .map(batch ? (listView ? setBatchRow : setBatchTile) : listView ? setCardRow : setCardTile)
    .join("");

  if (batch) {
    wireBatchRows();
    return;
  }
  // The image zooms via the global lightbox (like Search). In edit mode each
  // tile/row gets an explicit button to add/edit/remove.
  cards.forEach((c) =>
    grid
      .querySelector(`[data-id="${cssId(c.cardId)}"] .set-edit-btn`)
      ?.addEventListener("click", () => openCardDialog(c)),
  );
}

// ---- batch add (list view) ----

// A row mirroring the search list (per-row owner/lang/qty/notes), but the
// "Ajouter" button is replaced by a selection checkbox; a single bulk button
// at the top adds every checked row with its own settings.
function setBatchRow(c: SetCard): string {
  return `
  <div class="list-row batch-row ${c.owned ? "owned" : "missing"}" data-id="${esc(c.cardId)}">
    ${thumbTag(c)}
    <div class="list-main">
      <span class="list-code">${isDon(c) ? "" : esc(c.code) + " "}${cardRarityBadge(c)}${c.owned ? ` · ×${c.quantity}` : ""}</span>
      <span class="list-name" title="${esc(c.name)}">${esc(c.name)}</span>
    </div>
    <div class="list-actions add-grid">
      <wa-select class="owner" value="" style="min-width:120px">${ownerOptions(null)}</wa-select>
      <wa-select class="lang" value="EN" style="width:84px">${langOptions("EN")}</wa-select>
      <wa-input class="qty-in" type="number" min="1" value="1" style="width:64px"></wa-input>
      <wa-input class="note-in" placeholder="Commentaire" size="small" style="min-width:140px"></wa-input>
      <label class="batch-check" title="Sélectionner"><input type="checkbox" class="batch-cb"/></label>
    </div>
  </div>`;
}

// A grid tile in batch mode: the whole tile is a checkbox label (click to
// select); no per-card controls and no lightbox — owner/language are global.
function setBatchTile(c: SetCard): string {
  const alt = altLabel(c);
  const raw = c.imageSmall || c.imageLarge || "";
  const img = raw
    ? `<img class="card-img" src="${esc(proxied(raw))}" alt="${esc(c.name)}" loading="lazy"
        onerror="this.replaceWith(Object.assign(document.createElement('div'),{className:'card-img placeholder',textContent:'${esc(c.code)}'}))" />`
    : `<div class="card-img placeholder">${esc(c.code)}</div>`;
  return `
  <label class="tile set-tile batch-tile ${c.owned ? "owned" : "missing"}" data-id="${esc(c.cardId)}">
    <input type="checkbox" class="batch-cb"/>
    <div class="set-img-wrap">
      ${img}
      ${alt ? `<span class="alt-art">${esc(alt)}</span>` : ""}
      ${c.owned ? `<span class="qty-badge">×${c.quantity}</span><span class="own-check">✓</span>` : ""}
    </div>
    <div class="set-tile-name small" title="${esc(c.name)}">${isDon(c) ? `${cardRarityBadge(c)} ${esc(c.name)}` : `${esc(c.code)} ${cardRarityBadge(c)}`}</div>
  </label>`;
}

function paintBatchBar(active: boolean, global: boolean) {
  const bar = document.querySelector<HTMLDivElement>("#batch-bar");
  if (!bar) return;
  if (!active) {
    bar.innerHTML = "";
    return;
  }
  bar.innerHTML = `
    <div class="batch-bar">
      ${
        global
          ? `<wa-select id="batch-owner" value="" size="small" style="min-width:140px">${ownerOptions(null)}</wa-select>
             <wa-select id="batch-lang" value="EN" size="small" style="width:90px">${langOptions("EN")}</wa-select>`
          : ""
      }
      <wa-button id="batch-add" variant="brand" size="large" disabled>
        <wa-icon library="fa" name="plus" slot="start"></wa-icon><span id="batch-add-label">Ajouter la sélection (0)</span>
      </wa-button>
      <span class="muted small">${global ? "Règle propriétaire &amp; langue, puis coche les cartes." : "Coche les cartes, règle propriétaire / langue / quantité par ligne, puis valide."}</span>
    </div>`;
  bar.querySelector("#batch-add")?.addEventListener("click", () => runBatchAdd(global));
}

function wireBatchRows() {
  const grid = document.querySelector<HTMLDivElement>("#set-grid");
  if (!grid) return;
  grid
    .querySelectorAll<HTMLInputElement>(".batch-cb")
    .forEach((cb) => cb.addEventListener("change", updateBatchCount));
  updateBatchCount();
}

function updateBatchCount() {
  const grid = document.querySelector<HTMLDivElement>("#set-grid");
  const btn = document.querySelector("#batch-add") as any;
  if (!grid || !btn) return;
  const n = grid.querySelectorAll(".batch-cb:checked").length;
  btn.disabled = n === 0;
  const label = document.querySelector("#batch-add-label");
  if (label) label.textContent = `Ajouter la sélection (${n})`;
}

async function runBatchAdd(global: boolean) {
  const grid = document.querySelector<HTMLDivElement>("#set-grid");
  if (!grid) return;
  const checked = [...grid.querySelectorAll<HTMLInputElement>(".batch-cb:checked")];
  if (checked.length === 0) return;

  const btn = document.querySelector("#batch-add") as any;
  if (btn) {
    btn.loading = true;
    btn.disabled = true;
  }

  // Grid mode: one global owner/language for the whole selection. List mode:
  // each row carries its own owner/language/quantity/notes.
  const gOwner = global ? parseOwner((document.querySelector("#batch-owner") as any)?.value || "") : 0;
  const gLang = global ? (document.querySelector("#batch-lang") as any)?.value || "EN" : "EN";

  // One request, one server-side transaction (all-or-nothing).
  const items = checked.map((cb) => {
    const el = cb.closest("[data-id]") as HTMLElement;
    const cardId = el.getAttribute("data-id") || "";
    if (global) return { cardId, ownerId: gOwner, language: gLang, quantity: 1 };
    return {
      cardId,
      ownerId: parseOwner((el.querySelector(".owner") as any)?.value || ""),
      language: (el.querySelector(".lang") as any)?.value || "EN",
      quantity: parseInt((el.querySelector(".qty-in") as any)?.value, 10) || 1,
      notes: (el.querySelector(".note-in") as any)?.value || "",
    };
  });

  try {
    const added = await api.addItemsBatch(items);
    toast(`${added.length} carte(s) ajoutée(s)`);
  } catch (e) {
    toast((e as Error).message, "danger");
    if (btn) {
      btn.loading = false;
      btn.disabled = false;
    }
    return;
  }
  // Re-render the set (checkboxes reset, quantities refreshed); batch mode stays on.
  refreshCollection();
}

// CSS.escape for attribute selectors (card ids contain '_', safe but be robust).
function cssId(id: string): string {
  return (window.CSS && CSS.escape ? CSS.escape(id) : id);
}

// Alt-art / parallel marker: "" for the base card, else e.g. "P1" / "P2".
// DON!! cards have synthesized codes like "OP16-DON-698313"; hide that and
// show the (descriptive) name instead.
function isDon(c: Card): boolean {
  return c.rarity === "DON!!" || c.code.includes("-DON-");
}

function altLabel(c: Card): string {
  if (!c.cardId || c.cardId === c.code) return "";
  if (isDon(c)) return "GOLD"; // a DON whose id differs from its code is the gold variant
  const suffix = c.cardId.slice(c.code.length).replace(/^[_-]+/, "");
  return (suffix || "ALT").toUpperCase();
}

// Parallel level of a card (0 = base, 1+ = P1/P2…), mirroring the backend.
function parallelLevel(c: Card): number {
  if (!c.cardId || c.cardId === c.code || !c.cardId.startsWith(c.code)) return 0;
  const suffix = c.cardId.slice(c.code.length);
  const m = /(\d+)\s*$/.exec(suffix);
  return m ? Number(m[1]) : 0;
}

// Rarity bucket label, same logic as the stats "Par rareté" section:
// parallels → P1/P2…, gold DON → "DON!! Gold", else the base rarity.
function rarityBucketLabel(c: Card): string {
  const level = parallelLevel(c);
  if (c.rarity === "DON!!") return level > 0 ? "DON!! Gold" : "DON!!";
  if (level > 0) return "P" + level;
  return c.rarity || "";
}

// Colored rarity badge for a card (shared with the stats page styling).
function cardRarityBadge(c: Card): string {
  const label = rarityBucketLabel(c);
  return label ? rarityBadge(label) : "";
}

function setCardTile(c: SetCard): string {
  const btn = editMode
    ? `<wa-button class="set-edit-btn" size="small" appearance="outlined" variant="${c.owned ? "neutral" : "brand"}">${c.owned ? "Gérer" : "Ajouter"}</wa-button>`
    : "";
  const alt = altLabel(c);
  return `
  <div class="tile set-tile ${c.owned ? "owned" : "missing"}" data-id="${esc(c.cardId)}">
    <div class="set-img-wrap">
      ${imgTag(c)}
      ${alt ? `<span class="alt-art">${esc(alt)}</span>` : ""}
      ${c.owned ? `<span class="qty-badge">×${c.quantity}</span><span class="own-check">✓</span>` : ""}
    </div>
    <div class="set-tile-name small" title="${esc(c.name)}">${isDon(c) ? `${cardRarityBadge(c)} ${esc(c.name)}` : `${esc(c.code)} ${cardRarityBadge(c)} · ${esc(c.name)}`}</div>
    ${btn}
  </div>`;
}

function setCardRow(c: SetCard): string {
  // List rows are always actionable (no edit-mode gate), with an icon-only CTA:
  // ✏️ to manage an owned card, ➕ to add a missing one.
  const btn = `<wa-button class="set-edit-btn icon-btn" size="small" appearance="outlined" variant="${c.owned ? "neutral" : "brand"}" title="${c.owned ? "Gérer" : "Ajouter"}" aria-label="${c.owned ? "Gérer" : "Ajouter"}"><wa-icon library="fa" name="${c.owned ? "pen-to-square" : "plus"}"></wa-icon></wa-button>`;
  // Distinct named owners (drop unassigned copies), for the list view.
  const owners = [...new Set((c.items || []).map((it) => it.ownerName).filter(Boolean))];
  const ownersTag = owners.length
    ? ` · <span class="list-owners" title="${esc(owners.join(", "))}">👤 ${esc(owners.join(", "))}</span>`
    : "";
  return `
  <div class="list-row ${c.owned ? "owned" : "missing"}" data-id="${esc(c.cardId)}">
    ${thumbTag(c)}
    <div class="list-main">
      <span class="list-code">${isDon(c) ? "" : esc(c.code) + " "}${cardRarityBadge(c)}${c.owned ? ` · ×${c.quantity}` : ""}</span>
      <span class="list-name" title="${esc(c.name)}">${esc(c.name)}</span>
    </div>
    <div class="list-meta">${c.owned ? `<span class="own-tag">✓</span>` : ""}${ownersTag}</div>
    <div class="list-actions">${btn}</div>
  </div>`;
}

function openCardDialog(c: SetCard) {
  const dlg = document.createElement("wa-dialog");
  dlg.setAttribute("label", isDon(c) ? c.name : `${c.code} — ${c.name}`);
  const possessions = (c.items || [])
    .map(
      (it) => `
      <div class="poss-row" data-id="${it.id}">
        <span>${it.ownerName ? esc(it.ownerName) : "Non attribué"} · ${langLabel(it.language)} · ×${it.quantity}${it.notes ? ` · 💬 ${esc(it.notes)}` : ""}</span>
        <span class="poss-actions">
          <wa-button class="poss-edit" size="small" appearance="outlined">Éditer</wa-button>
          <wa-button class="poss-del" size="small" appearance="outlined" variant="danger">×</wa-button>
        </span>
      </div>`,
    )
    .join("");
  const imgSrc = c.imageLarge || c.imageSmall || "";
  const img = imgSrc
    ? `<img class="card-dialog-img" src="${esc(proxied(imgSrc, 360))}" alt="${esc(c.name)}"
         onerror="this.replaceWith(Object.assign(document.createElement('div'),{className:'card-dialog-img placeholder',textContent:'${esc(c.code)}'}))" />`
    : "";
  // Use real possessions (not the owner-filtered `owned`) so managing always
  // shows every owner's copies.
  const anyOwned = (c.items || []).length > 0;
  dlg.innerHTML = `
    ${img}
    ${anyOwned ? `<div class="poss-list">${possessions}</div><hr class="poss-sep"/>` : ""}
    <div class="form">
      <div class="muted small">${anyOwned ? "Ajouter un exemplaire" : "Ajouter à la collection"}</div>
      <label>Propriétaire
        <wa-select class="f-owner" value="">${ownerOptions(null)}</wa-select>
      </label>
      <label>Langue
        <wa-select class="f-lang" value="EN">${langOptions("EN")}</wa-select>
      </label>
      <label>Quantité
        <wa-input class="f-qty" type="number" min="1" value="1"></wa-input>
      </label>
      <label>Commentaire
        <wa-textarea class="f-notes" rows="2"></wa-textarea>
      </label>
    </div>
    <wa-button slot="footer" class="f-cancel" appearance="outlined">Fermer</wa-button>
    <wa-button slot="footer" class="f-add" variant="brand">Ajouter</wa-button>`;
  document.body.appendChild(dlg);
  (dlg as any).open = true;
  const close = () => {
    (dlg as any).open = false;
    setTimeout(() => dlg.remove(), 300);
  };
  dlg.querySelector(".f-cancel")?.addEventListener("click", close);
  dlg.querySelector(".f-add")?.addEventListener("click", async () => {
    const ownerId = parseOwner((dlg.querySelector(".f-owner") as any)?.value || "");
    const language = (dlg.querySelector(".f-lang") as any)?.value || "EN";
    const quantity = parseInt((dlg.querySelector(".f-qty") as any)?.value, 10) || 1;
    const notes = (dlg.querySelector(".f-notes") as any)?.value || "";
    try {
      await api.addItem({ cardId: c.cardId, ownerId, language, quantity, notes });
      close();
      refreshCollection();
      toast(`Ajouté : ${c.name}`);
    } catch (e) {
      toast((e as Error).message, "danger");
    }
  });
  (c.items || []).forEach((it) => {
    const row = dlg.querySelector(`.poss-row[data-id="${it.id}"]`);
    row?.querySelector(".poss-del")?.addEventListener("click", async () => {
      try {
        await api.deleteItem(it.id);
        close();
        refreshCollection();
      } catch (e) {
        toast((e as Error).message, "danger");
      }
    });
    row?.querySelector(".poss-edit")?.addEventListener("click", () => {
      it.card = c; // so the edit dialog can show the card name
      close();
      openEditDialog(it);
    });
  });
}

function openEditDialog(it: Item) {
  const dlg = document.createElement("wa-dialog");
  dlg.setAttribute("label", `Éditer — ${it.card?.name ?? ""}`);
  dlg.innerHTML = `
    <div class="form">
      <label>Propriétaire
        <wa-select class="f-owner" value="${it.ownerId ?? ""}">${ownerOptions(it.ownerId)}</wa-select>
      </label>
      <label>Langue
        <wa-select class="f-lang" value="${esc(it.language)}">${langOptions(it.language)}</wa-select>
      </label>
      <label>Quantité
        <wa-input class="f-qty" type="number" min="1" value="${it.quantity}"></wa-input>
      </label>
      <label>Commentaire
        <wa-textarea class="f-notes" rows="2">${esc(it.notes)}</wa-textarea>
      </label>
    </div>
    <wa-button slot="footer" class="f-cancel" appearance="outlined">Annuler</wa-button>
    <wa-button slot="footer" class="f-save" variant="brand">Enregistrer</wa-button>`;
  document.body.appendChild(dlg);
  (dlg as any).open = true;

  const close = () => {
    (dlg as any).open = false;
    setTimeout(() => dlg.remove(), 300);
  };
  dlg.querySelector(".f-cancel")?.addEventListener("click", close);
  dlg.querySelector(".f-save")?.addEventListener("click", async () => {
    const ownerId = parseOwner((dlg.querySelector(".f-owner") as any)?.value || "");
    const language = (dlg.querySelector(".f-lang") as any)?.value || it.language;
    const quantity = parseInt((dlg.querySelector(".f-qty") as any)?.value, 10) || 1;
    const notes = (dlg.querySelector(".f-notes") as any)?.value ?? "";
    try {
      await api.updateItem(it.id, { ownerId, language, quantity, notes });
      close();
      refreshCollection();
    } catch (e) {
      toast((e as Error).message, "danger");
    }
  });
}

// ---------- search tab ----------

function renderSearch() {
  const host = document.querySelector<HTMLDivElement>("#search")!;
  const emptyWarn =
    state.cardCount === 0
      ? `<wa-callout variant="warning" class="span">Le catalogue est vide. Va dans <strong>Préférences → Catalogue</strong> et clique sur <em>Synchroniser</em> pour le télécharger une fois (ensuite la recherche est instantanée et hors-ligne).</wa-callout>`
      : "";
  host.innerHTML = `
    ${emptyWarn}
    <p class="muted small">Recherche dans le catalogue local (${state.cardCount} cartes) — aucun appel API, aucun quota consommé. Laisse le champ vide pour tout parcourir.</p>
    <div class="toolbar">
      <wa-input id="s-name" placeholder="Nom ou code, ex. Luffy / OP01-001" style="flex:1"></wa-input>
      <wa-button id="s-go" variant="brand">Rechercher</wa-button>
      ${viewToggle()}
    </div>
    <div id="s-results" class="grid"></div>`;

  const go = () => doSearch();
  host.querySelector("#s-go")?.addEventListener("click", go);
  host.querySelector("#s-name")?.addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") go();
  });
  // Toggling the view re-paints the current results without re-querying.
  wireViewToggle(host, () => paintSearchResults());
}

// Last search results, kept so the grid/list toggle can repaint without refetching.
let lastSearch: Card[] = [];

async function doSearch() {
  const results = document.querySelector<HTMLDivElement>("#s-results")!;
  const name = (document.querySelector("#s-name") as any)?.value || "";
  results.innerHTML = `<div class="loading"><wa-spinner></wa-spinner></div>`;
  try {
    const res = await api.search(name, 1, 50);
    lastSearch = res.cards;
    paintSearchResults();
  } catch (e) {
    results.innerHTML = `<wa-callout variant="danger" class="span">${esc((e as Error).message)}</wa-callout>`;
  }
}

function paintSearchResults() {
  const results = document.querySelector<HTMLDivElement>("#s-results");
  if (!results) return;
  if (lastSearch.length === 0) {
    results.className = "grid";
    results.innerHTML = `<wa-callout class="span">Aucune carte trouvée.</wa-callout>`;
    return;
  }
  results.className = viewMode === "list" ? "card-list" : "grid";
  const render = viewMode === "list" ? searchRow : searchCard;
  results.innerHTML = lastSearch.map((c, i) => render(c, i)).join("");
  lastSearch.forEach((c, i) => wireSearchCard(c, i));
}

function searchCard(c: Card, i: number): string {
  return `
  <wa-card class="tile" data-i="${i}">
    ${imgTag(c)}
    <div class="tile-body">
      <div class="tile-name" title="${esc(c.name)}">${esc(c.name)}</div>
      <div class="muted small">${isDon(c) ? "" : esc(c.setName || c.code) + " "}${cardRarityBadge(c)}</div>
      <div class="add-grid">
        <wa-select class="owner" value="" style="min-width:120px">${ownerOptions(null)}</wa-select>
        <wa-select class="lang" value="EN" style="width:90px">${langOptions("EN")}</wa-select>
        <wa-input class="qty-in" type="number" min="1" value="1" style="width:70px"></wa-input>
      </div>
      <wa-input class="note-in" placeholder="Commentaire (optionnel)" size="small"></wa-input>
      <wa-button class="add" size="small" variant="brand">Ajouter</wa-button>
    </div>
  </wa-card>`;
}

function searchRow(c: Card, i: number): string {
  return `
  <div class="list-row" data-i="${i}">
    ${thumbTag(c)}
    <div class="list-main">
      <span class="list-name" title="${esc(c.name)}">${esc(c.name)}</span>
      <span class="list-meta">${isDon(c) ? "" : `${esc(c.code)}${c.setName ? " · " + esc(c.setName) : ""} `}${cardRarityBadge(c)}</span>
    </div>
    <div class="list-actions add-grid">
      <wa-select class="owner" value="" style="min-width:120px">${ownerOptions(null)}</wa-select>
      <wa-select class="lang" value="EN" style="width:84px">${langOptions("EN")}</wa-select>
      <wa-input class="qty-in" type="number" min="1" value="1" style="width:64px"></wa-input>
      <wa-input class="note-in" placeholder="Commentaire" size="small" style="min-width:140px"></wa-input>
      <wa-button class="add icon-btn" size="small" variant="brand" title="Ajouter" aria-label="Ajouter"><wa-icon library="fa" name="plus"></wa-icon></wa-button>
    </div>
  </div>`;
}

function wireSearchCard(c: Card, i: number) {
  const el = document.querySelector<HTMLElement>(`#s-results [data-i="${i}"]`);
  if (!el) return;
  el.querySelector(".add")?.addEventListener("click", async () => {
    const ownerId = parseOwner((el.querySelector(".owner") as any)?.value || "");
    const language = (el.querySelector(".lang") as any)?.value || "EN";
    const quantity = parseInt((el.querySelector(".qty-in") as any)?.value, 10) || 1;
    const notes = (el.querySelector(".note-in") as any)?.value || "";
    try {
      await api.addItem({ cardId: c.cardId, ownerId, language, quantity, notes });
      const who = ownerId ? state.owners.find((o) => o.id === ownerId)?.name : "collection";
      toast(`Ajouté : ${c.name} → ${who}`);
      refreshStats();
    } catch (e) {
      toast((e as Error).message, "danger");
    }
  });
}

// ---------- preferences tab ----------

function renderPrefs() {
  const host = document.querySelector<HTMLDivElement>("#prefs")!;
  host.innerHTML = `
    <h2>Objectif de collection</h2>
    <p class="muted">Définit quelles cartes comptent dans la progression affichée sur la page
    « Ma collection ». La grille montre toujours toutes les cartes ; seuls les compteurs changent.</p>
    <wa-select id="goal-select" value="${state.goal}" style="max-width:420px">
      ${COLLECTION_GOALS.map(
        (g) =>
          `<wa-option value="${g.value}"${g.value === state.goal ? " selected" : ""}>${esc(g.label)} — ${esc(g.desc)}</wa-option>`,
      ).join("")}
    </wa-select>

    <p class="muted" style="margin-top:1rem">Familles de sets à inclure dans l'objectif (aperçu collection &amp; statistiques) :</p>
    <div class="family-list">
      ${FAMILIES.map(
        (f) =>
          `<label class="set-toggle"><input type="checkbox" class="family-cb" value="${f.value}"${state.families.includes(f.value) ? " checked" : ""}/> ${esc(f.label)}</label>`,
      ).join("")}
    </div>

    <h2 style="margin-top:2rem">Catalogue One Piece</h2>
    <p class="muted">La recherche lit un catalogue local (aucune connexion, instantané).
    Une synchronisation récupère tout le catalogue depuis le site officiel One Piece
    (tous les sets) ; ça prend ~30 s. À relancer de temps en temps quand de
    nouvelles séries sortent.</p>
    <div id="catalogue"></div>

    <h2 style="margin-top:2rem">Co-propriétaires de la collection</h2>
    <p class="muted">Les noms ajoutés ici sont proposés dans le menu déroulant « Propriétaire » de chaque carte.
    Supprimer un propriétaire ne supprime pas ses cartes : elles repassent simplement en « Non attribué ».</p>
    <div class="toolbar">
      <wa-input id="owner-name" placeholder="Nom (ex. Pierre)" style="flex:1"></wa-input>
      <wa-button id="owner-add" variant="brand">Ajouter</wa-button>
    </div>
    <div id="owner-list" class="owner-list"></div>`;

  const add = async () => {
    const input = document.querySelector("#owner-name") as any;
    const name = (input?.value || "").trim();
    if (!name) return;
    try {
      await api.addOwner(name);
      input.value = "";
      await reloadOwners();
    } catch (e) {
      toast((e as Error).message, "danger");
    }
  };
  host.querySelector("#owner-add")?.addEventListener("click", add);
  host.querySelector("#owner-name")?.addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") add();
  });
  host.querySelector("#goal-select")?.addEventListener("change", async (e) => {
    const v = (e.target as any).value as CollectionGoal;
    if (!GOAL_VALUES.includes(v) || v === state.goal) return;
    try {
      const s = await api.updateSettings({ collectionGoal: v });
      state.goal = s.collectionGoal;
      const label = COLLECTION_GOALS.find((g) => g.value === state.goal)?.label ?? state.goal;
      toast(`Objectif : ${label}`);
      // Collection progression depends on the goal — refresh it (if mounted).
      if (colSet) renderSetDetail(colSet);
      else renderSetsOverview();
    } catch (err) {
      toast((err as Error).message, "danger");
    }
  });
  host.querySelectorAll(".family-cb").forEach((cb) =>
    cb.addEventListener("change", async () => {
      const families = [...host.querySelectorAll<HTMLInputElement>(".family-cb:checked")].map(
        (el) => el.value,
      );
      try {
        const s = await api.updateSettings({ families });
        state.families = s.families;
        // Collection overview & stats depend on this — refresh the collection view.
        if (colSet) renderSetDetail(colSet);
        else renderSetsOverview();
      } catch (err) {
        toast((err as Error).message, "danger");
      }
    }),
  );
  renderOwnerList();
  renderCatalogue();
}

// ---------- catalogue sync ----------

function fmtDate(iso: string): string {
  if (!iso) return "jamais";
  const d = new Date(iso);
  return isNaN(d.getTime()) ? iso : d.toLocaleString("fr-FR");
}

async function renderCatalogue() {
  const host = document.querySelector<HTMLDivElement>("#catalogue");
  if (!host) return;
  let status;
  try {
    status = await api.syncStatus();
  } catch (e) {
    host.innerHTML = `<wa-callout variant="danger">${esc((e as Error).message)}</wa-callout>`;
    return;
  }
  state.cardCount = status.cardCount;

  const errWarn = status.error
    ? `<wa-callout variant="danger">Dernière synchro en échec : ${esc(status.error)}</wa-callout>`
    : "";
  host.innerHTML = `
    ${errWarn}
    <div class="catalogue-row">
      <div>
        <div><strong>${status.cardCount}</strong> cartes en cache</div>
        <div class="muted small">Dernière synchro : ${fmtDate(status.lastSync)}</div>
      </div>
      <wa-button id="sync-btn" variant="brand"${status.syncing ? " loading disabled" : ""}>
        ${status.syncing ? "Synchronisation…" : "Synchroniser le catalogue"}
      </wa-button>
    </div>`;

  host.querySelector("#sync-btn")?.addEventListener("click", syncCatalogue);

  // If a sync is already running (e.g. started elsewhere), keep watching it.
  if (status.syncing) pollSync();
}

// Global banner shown while the backend is (re)building the catalogue.
function showSyncBanner() {
  if (document.querySelector("#sync-banner")) return;
  const el = document.createElement("div");
  el.id = "sync-banner";
  el.className = "sync-banner";
  el.innerHTML = `<wa-spinner></wa-spinner><span>Synchronisation de la base de données…</span>`;
  document.body.appendChild(el);
}
function hideSyncBanner() {
  document.querySelector("#sync-banner")?.remove();
}

let syncPolling = false;
async function pollSync() {
  if (syncPolling) return;
  syncPolling = true;
  showSyncBanner();
  try {
    // Poll until the backend reports the scrape is finished.
    for (;;) {
      await new Promise((r) => setTimeout(r, 1500));
      const st = await api.syncStatus();
      if (!st.syncing) {
        state.cardCount = st.cardCount;
        if (st.error) toast(`Échec de la synchro : ${st.error}`, "danger");
        else toast(`Catalogue synchronisé : ${st.cardCount} cartes`);
        renderCatalogue();
        renderSearch();
        return;
      }
    }
  } catch (e) {
    toast((e as Error).message, "danger");
  } finally {
    syncPolling = false;
    hideSyncBanner();
  }
}

async function syncCatalogue() {
  const btn = document.querySelector<HTMLButtonElement>("#sync-btn");
  if (btn) {
    (btn as any).loading = true;
    (btn as any).disabled = true;
    btn.textContent = "Synchronisation… (récupère tous les sets, ~30s)";
  }
  try {
    await api.runSync(); // 202 — runs in the background
    pollSync();
  } catch (e) {
    toast((e as Error).message, "danger");
    renderCatalogue();
  }
}

function renderOwnerList() {
  const list = document.querySelector<HTMLDivElement>("#owner-list");
  if (!list) return;
  if (state.owners.length === 0) {
    list.innerHTML = `<wa-callout>Aucun propriétaire pour l'instant.</wa-callout>`;
    return;
  }
  list.innerHTML = state.owners
    .map(
      (o) => `
      <div class="owner-row" data-id="${o.id}">
        <span>${esc(o.name)}</span>
        <wa-button class="owner-del" size="small" appearance="outlined" variant="danger">Supprimer</wa-button>
      </div>`,
    )
    .join("");
  state.owners.forEach((o) => {
    list
      .querySelector(`.owner-row[data-id="${o.id}"] .owner-del`)
      ?.addEventListener("click", async () => {
        try {
          await api.deleteOwner(o.id);
          await reloadOwners();
        } catch (e) {
          toast((e as Error).message, "danger");
        }
      });
  });
}

// Reload owners everywhere they appear (prefs list, filters, selects).
async function reloadOwners() {
  state.owners = await api.listOwners();
  renderOwnerList();
  renderCollection();
  refreshStats();
}

boot();
