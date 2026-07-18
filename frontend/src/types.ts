export interface Card {
  cardId: string;
  name: string;
  code: string;
  rarity: string;
  setName: string;
  imageSmall: string;
  imageLarge: string;
  raw?: unknown;
}

export interface SearchResult {
  page: number;
  limit: number;
  total: number;
  totalPages: number;
  cards: SetCard[];
}

export interface Owner {
  id: number;
  name: string;
  createdAt: string;
}

export type CardStatus = "owned" | "ordered" | "wishlist";

export interface Item {
  id: number;
  cardId: string;
  ownerId: number | null;
  ownerName?: string;
  quantity: number;
  language: string;
  notes: string;
  status: CardStatus;
  createdAt: string;
  updatedAt: string;
  card?: Card;
}

export interface OwnerStat {
  ownerId: number | null;
  name: string;
  uniqueCards: number;
  totalCards: number;
}

export interface Stats {
  uniqueCards: number;
  totalCards: number;
  byOwner: OwnerStat[];
}

export interface SetMeta {
  code: string;
  name: string;
  family: string;
  group: string;
  total: number;
  owned: number;
}

// Set families the collection goal can be scoped to.
export const FAMILIES: { value: string; label: string }[] = [
  { value: "main", label: "Séries principales" },
  { value: "eb", label: "Extra Boosters" },
  { value: "prb", label: "Premium Boosters" },
  { value: "deck", label: "Decks" },
  { value: "promo", label: "Promos & autres" },
];

export interface SetCard extends Card {
  owned: boolean; // has a physical copy
  ordered: boolean; // has an on-order copy
  wishlist: boolean; // wanted by someone
  quantity: number; // physical (owned) quantity
  inGoal: boolean; // counts toward the active collection goal
  items: Item[];
}

// Card statuses, with their UI label and short emoji marker.
export const STATUSES: { value: CardStatus; label: string; emoji: string }[] = [
  { value: "owned", label: "Possédée", emoji: "✓" },
  { value: "ordered", label: "Commandée", emoji: "🛒" },
  { value: "wishlist", label: "Wishlist", emoji: "❤" },
];

// Collection goal: which cards count toward set completion.
export type CollectionGoal = "complete" | "master" | "wizard";

export interface Settings {
  collectionGoal: CollectionGoal;
  families: string[];
}
export const COLLECTION_GOALS: {
  value: CollectionGoal;
  label: string;
  desc: string;
}[] = [
  {
    value: "complete",
    label: "Complete set",
    desc: "Cartes de base uniquement",
  },
  {
    value: "master",
    label: "Master set",
    desc: "Complete set + 1re parallèle (p1)",
  },
  {
    value: "wizard",
    label: "Wizard set",
    desc: "Master set + toutes les parallèles (p2, p3…)",
  },
];

export interface SetDetail {
  code: string;
  name: string;
  total: number;
  owned: number;
  cards: SetCard[];
}

// A set's missing (not-acquired) cards, for the "toutes les manquantes" view.
export interface MissingGroup {
  code: string;
  name: string;
  cards: SetCard[];
}

export interface StatBucket {
  label: string;
  owned?: number;
  total?: number;
  copies?: number;
}

export interface FullStats {
  goal: CollectionGoal;
  owned: number;
  copies: number;
  goalOwned: number;
  catalogueTotal: number;
  setsComplete: number;
  setsTotal: number;
  byGroup: StatBucket[];
  byOwner: StatBucket[];
  byLanguage: StatBucket[];
  byRarity: StatBucket[];
}

export interface SyncStatus {
  syncing: boolean;
  cardCount: number;
  lastSync: string;
  error: string;
}

// Languages One Piece cards are commonly printed in.
export const LANGUAGES = ["EN", "FR", "JP"];
