import type {
  FullStats,
  Item,
  Owner,
  SearchResult,
  Settings,
  SetDetail,
  SetMeta,
  Stats,
  SyncStatus,
} from "./types";

async function req<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  health: () =>
    req<{ ok: boolean; cardCount: number }>("/api/health"),

  // Catalogue cache. runSync kicks off a background scrape (returns 202);
  // poll syncStatus until syncing is false.
  syncStatus: () => req<SyncStatus>("/api/sync"),
  runSync: () => req<{ started: boolean }>("/api/sync", { method: "POST" }),

  search: (name: string, page = 1, limit = 50) =>
    req<SearchResult>(
      `/api/search?name=${encodeURIComponent(name)}&page=${page}&limit=${limit}`,
    ),

  listCollection: (q = "", owner = 0) =>
    req<Item[]>(
      `/api/collection?q=${encodeURIComponent(q)}&owner=${owner}`,
    ),

  stats: () => req<Stats>("/api/collection/stats"),
  fullStats: () => req<FullStats>("/api/stats"),

  // Collection-by-set views.
  sets: () => req<SetMeta[]>("/api/sets"),
  setDetail: (code: string) =>
    req<SetDetail>(`/api/sets/${encodeURIComponent(code)}`),

  // App settings (collection goal, …) — stored server-side in SQLite.
  getSettings: () => req<Settings>("/api/settings"),
  updateSettings: (patch: Partial<Settings>) =>
    req<Settings>("/api/settings", { method: "PUT", body: JSON.stringify(patch) }),

  addItem: (payload: {
    cardId: string;
    ownerId?: number | null;
    quantity?: number;
    language?: string;
    notes?: string;
  }) =>
    req<Item>("/api/collection", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  // Bulk add: one request, one server-side transaction (all-or-nothing).
  addItemsBatch: (
    items: Array<{
      cardId: string;
      ownerId?: number | null;
      quantity?: number;
      language?: string;
      notes?: string;
    }>,
  ) =>
    req<Item[]>("/api/collection/batch", {
      method: "POST",
      body: JSON.stringify({ items }),
    }),

  updateItem: (
    id: number,
    patch: Partial<Pick<Item, "ownerId" | "quantity" | "language" | "notes">>,
  ) =>
    req<Item | { deleted: boolean }>(`/api/collection/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),

  deleteItem: (id: number) =>
    req<void>(`/api/collection/${id}`, { method: "DELETE" }),

  // owners
  listOwners: () => req<Owner[]>("/api/owners"),
  addOwner: (name: string) =>
    req<Owner>("/api/owners", { method: "POST", body: JSON.stringify({ name }) }),
  deleteOwner: (id: number) =>
    req<void>(`/api/owners/${id}`, { method: "DELETE" }),
};
