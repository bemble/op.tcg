// Minimal i18n: a flat FR/EN dictionary + a t() helper with {param} interpolation.
// Language is detected from the browser, overridable in Préférences, persisted.

export type Lang = "fr" | "en";
export const LANGS: { value: Lang; label: string }[] = [
  { value: "fr", label: "Français" },
  { value: "en", label: "English" },
];

type Entry = { fr: string; en: string };

const dict: Record<string, Entry> = {
  // shell / tabs
  "tab.collection": { fr: "Ma collection", en: "My collection" },
  "tab.stats": { fr: "Statistiques", en: "Statistics" },
  "tab.missing": { fr: "Manquantes", en: "Missing" },
  "tab.search": { fr: "Rechercher & ajouter", en: "Search & add" },
  "tab.tracking": { fr: "Suivi", en: "Tracking" },
  "tab.prefs": { fr: "Préférences", en: "Preferences" },
  "header.owned": { fr: "{n} cartes possédées", en: "{n} cards owned" },
  "err.backend": { fr: "Backend injoignable : {msg}", en: "Backend unreachable: {msg}" },

  // view toggle
  "view.display": { fr: "Affichage", en: "Display" },
  "view.grid": { fr: "Vue grille", en: "Grid view" },
  "view.list": { fr: "Vue liste", en: "List view" },

  // generic actions
  "action.add": { fr: "Ajouter", en: "Add" },
  "action.close": { fr: "Fermer", en: "Close" },
  "action.cancel": { fr: "Annuler", en: "Cancel" },
  "action.save": { fr: "Enregistrer", en: "Save" },
  "action.edit": { fr: "Éditer", en: "Edit" },
  "action.delete": { fr: "Supprimer", en: "Delete" },
  "common.noImage": { fr: "Pas d'image", en: "No image" },
  "common.unassigned": { fr: "Non attribué", en: "Unassigned" },
  "common.comment": { fr: "Commentaire", en: "Comment" },
  "common.select": { fr: "Sélectionner", en: "Select" },
  "common.owner": { fr: "Propriétaire", en: "Owner" },
  "common.language": { fr: "Langue", en: "Language" },
  "common.quantity": { fr: "Quantité", en: "Quantity" },
  "common.status": { fr: "Statut", en: "Status" },
  "common.name": { fr: "Nom", en: "Name" },
  "common.code": { fr: "Code", en: "Code" },
  "common.rarity": { fr: "Rareté", en: "Rarity" },
  "common.backToSets": { fr: "← Sets", en: "← Sets" },

  // statuses
  "status.owned": { fr: "Possédée", en: "Owned" },
  "status.ordered": { fr: "Commandée", en: "Ordered" },
  "status.wishlist": { fr: "Wishlist", en: "Wishlist" },

  // filters
  "filter.all": { fr: "Toutes", en: "All" },
  "filter.owned": { fr: "Possédées", en: "Owned" },
  "filter.ordered": { fr: "Commandées", en: "Ordered" },
  "filter.wishlist": { fr: "Wishlist", en: "Wishlist" },
  "filter.missing": { fr: "Manquantes", en: "Missing" },

  // families
  "family.main": { fr: "Séries principales", en: "Main series" },
  "family.eb": { fr: "Extra Boosters", en: "Extra Boosters" },
  "family.prb": { fr: "Premium Boosters", en: "Premium Boosters" },
  "family.deck": { fr: "Decks", en: "Decks" },
  "family.promo": { fr: "Promos & autres", en: "Promos & others" },

  // collection goals
  "goal.complete": { fr: "Complete set", en: "Complete set" },
  "goal.complete.desc": { fr: "Cartes de base uniquement", en: "Base cards only" },
  "goal.master": { fr: "Master set", en: "Master set" },
  "goal.master.desc": { fr: "Complete set + 1re parallèle (p1)", en: "Complete set + 1st parallel (p1)" },
  "goal.wizard": { fr: "Wizard set", en: "Wizard set" },
  "goal.wizard.desc": { fr: "Master set + toutes les parallèles (p2, p3…)", en: "Master set + all parallels (p2, p3…)" },

  // collection: set detail toolbar
  "toolbar.batchAdd": { fr: "Ajout par lot", en: "Batch add" },
  "toolbar.bulkEdit": { fr: "Édition groupée (langue)", en: "Bulk edit (language)" },
  "toolbar.filters": { fr: "Afficher/masquer les filtres", en: "Show/hide filters" },
  "toolbar.parallelsLast": { fr: "Parallèles à la fin", en: "Parallels last" },
  "filters.allOwners": { fr: "Tous propriétaires", en: "All owners" },
  "filters.goalOnly": { fr: "Objectif seulement", en: "Goal only" },
  "collection.emptyCatalogue": {
    fr: "Catalogue vide. Va dans <strong>Préférences</strong> et synchronise le catalogue.",
    en: "Empty catalogue. Go to <strong>Preferences</strong> and sync the catalogue.",
  },

  // missing view
  "missing.title": { fr: "Cartes manquantes", en: "Missing cards" },
  "missing.empty": { fr: "Aucune carte manquante 🎉", en: "No missing cards 🎉" },

  // batch add / bulk edit
  "batch.addSelection": { fr: "Ajouter la sélection ({n})", en: "Add selection ({n})" },
  "batch.added": { fr: "{n} carte(s) ajoutée(s)", en: "{n} card(s) added" },
  "bulk.nothingToEdit": { fr: "Aucun exemplaire à éditer ici.", en: "Nothing to edit here." },
  "bulk.newLanguage": { fr: "Nouvelle langue :", en: "New language:" },
  "bulk.changeLanguage": { fr: "Changer la langue ({n})", en: "Change language ({n})" },
  "bulk.done": { fr: "{n} exemplaire(s) mis en {lang}", en: "{n} copy(ies) set to {lang}" },

  // card dialog
  "dialog.addCopy": { fr: "Ajouter un exemplaire", en: "Add a copy" },
  "dialog.addOrTrack": { fr: "Ajouter / suivre cette carte", en: "Add / track this card" },
  "dialog.editTitle": { fr: "Éditer — {name}", en: "Edit — {name}" },
  "toast.added": { fr: "{status} : {name}", en: "{status}: {name}" },

  // tracking
  "tracking.ordered": { fr: "Commandées", en: "Ordered" },
  "tracking.wishlist": { fr: "Wishlist", en: "Wishlist" },
  "tracking.none": { fr: "Aucune carte.", en: "No cards." },

  // stats
  "stats.completionByGoal": { fr: "Complétion selon l'objectif :", en: "Completion by goal:" },
  "stats.breakdownNote": {
    fr: "(les répartitions ci-dessous couvrent toute la collection)",
    en: "(the breakdowns below cover the whole collection)",
  },
  "stats.cardsOwned": { fr: "Cartes possédées", en: "Cards owned" },
  "stats.copies": { fr: "Exemplaires", en: "Copies" },
  "stats.catalogueCompletion": { fr: "Complétion catalogue", en: "Catalogue completion" },
  "stats.setsCompleted": { fr: "Sets complétés", en: "Sets completed" },
  "stats.byFamily": { fr: "Par famille", en: "By family" },
  "stats.byOwner": { fr: "Par propriétaire", en: "By owner" },
  "stats.byLanguage": { fr: "Par langue", en: "By language" },
  "stats.byRarity": { fr: "Par rareté", en: "By rarity" },
  "stats.ownerRow": { fr: "{owned} cartes · {copies} ex.", en: "{owned} cards · {copies} copies" },
  "stats.copiesShort": { fr: "{copies} ex.", en: "{copies} copies" },

  // search
  "search.emptyCatalogueWarn": {
    fr: "Le catalogue est vide. Va dans <strong>Préférences → Catalogue</strong> et clique sur <em>Synchroniser</em> pour le télécharger une fois (ensuite la recherche est instantanée et hors-ligne).",
    en: "The catalogue is empty. Go to <strong>Preferences → Catalogue</strong> and click <em>Sync</em> to download it once (search is then instant and offline).",
  },
  "search.hint": {
    fr: "Recherche dans le catalogue local ({n} cartes) — aucun appel API, aucun quota consommé. Laisse le champ vide pour tout parcourir.",
    en: "Search the local catalogue ({n} cards) — no API call, no quota. Leave empty to browse everything.",
  },
  "search.placeholder": { fr: "Nom ou code, ex. Luffy / OP01-001", en: "Name or code, e.g. Luffy / OP01-001" },
  "search.button": { fr: "Rechercher", en: "Search" },
  "search.none": { fr: "Aucune carte trouvée.", en: "No cards found." },

  // preferences
  "prefs.goalTitle": { fr: "Objectif de collection", en: "Collection goal" },
  "prefs.goalDesc": {
    fr: "Définit quelles cartes comptent dans la progression affichée sur la page « Ma collection ». La grille montre toujours toutes les cartes ; seuls les compteurs changent.",
    en: "Defines which cards count toward the progress shown on “My collection”. The grid always shows every card; only the counters change.",
  },
  "prefs.families": {
    fr: "Familles de sets à inclure dans l'objectif (aperçu collection & statistiques) :",
    en: "Set families to include in the goal (collection overview & statistics):",
  },
  "prefs.catalogueTitle": { fr: "Catalogue One Piece", en: "One Piece catalogue" },
  "prefs.catalogueDesc": {
    fr: "La recherche lit un catalogue local (aucune connexion, instantané). Une synchronisation récupère tout le catalogue depuis le site officiel One Piece (tous les sets) ; ça prend ~30 s. À relancer de temps en temps quand de nouvelles séries sortent.",
    en: "Search reads a local catalogue (offline, instant). A sync fetches the whole catalogue from the official One Piece site (all sets); it takes ~30 s. Re-run it occasionally when new sets release.",
  },
  "prefs.addMissingTitle": { fr: "Ajouter une carte manquante", en: "Add a missing card" },
  "prefs.addMissingDesc": {
    fr: "Certaines promos (arts alternatifs, Tin Pack Set…) ne figurent pas dans la base officielle One Piece et n'apparaissent donc pas. Colle l'URL TCGplayer du produit pour l'ajouter au catalogue (immédiat, sans resynchronisation).",
    en: "Some promos (alternate arts, Tin Pack Sets…) aren't in the official One Piece database and don't show up. Paste the product's TCGplayer URL to add it to the catalogue (instant, no resync).",
  },
  "prefs.manualSummary": {
    fr: "Saisie manuelle (carte hors TCGplayer, ex. Cardmarket)",
    en: "Manual entry (card not on TCGplayer, e.g. Cardmarket)",
  },
  "prefs.manualDesc": {
    fr: "Pour une carte absente de TCGplayer. Le <strong>code</strong> détermine le set (ex. <code>OP08-043</code> → un parallèle si le code existe déjà, <code>P</code> pour une promo). L'image est optionnelle (URL directe d'une image).",
    en: "For a card missing from TCGplayer. The <strong>code</strong> decides the set (e.g. <code>OP08-043</code> → a parallel if the code already exists, <code>P</code> for a promo). The image is optional (a direct image URL).",
  },
  "prefs.imageUrl": { fr: "URL de l'image (optionnel)", en: "Image URL (optional)" },
  "prefs.sourceUrl": { fr: "URL source (optionnel, ex. Cardmarket)", en: "Source URL (optional, e.g. Cardmarket)" },
  "prefs.coOwnersTitle": { fr: "Co-propriétaires de la collection", en: "Collection co-owners" },
  "prefs.coOwnersDesc": {
    fr: "Les noms ajoutés ici sont proposés dans le menu déroulant « Propriétaire » de chaque carte. Supprimer un propriétaire ne supprime pas ses cartes : elles repassent simplement en « Non attribué ».",
    en: "Names added here appear in each card's “Owner” dropdown. Removing an owner doesn't delete their cards — they simply become “Unassigned”.",
  },
  "prefs.ownerPlaceholder": { fr: "Nom (ex. Pierre)", en: "Name (e.g. Pierre)" },
  "prefs.noOwners": { fr: "Aucun propriétaire pour l'instant.", en: "No co-owners yet." },
  "prefs.noCurated": { fr: "Aucune carte ajoutée manuellement.", en: "No manually-added cards." },
  "prefs.langTitle": { fr: "Langue de l'interface", en: "Interface language" },
  "toast.nameCodeRequired": { fr: "Nom et code requis", en: "Name and code required" },
  "toast.cardAdded": { fr: "Ajoutée : {code} — {name}", en: "Added: {code} — {name}" },
  "toast.goalSet": { fr: "Objectif : {label}", en: "Goal: {label}" },

  // catalogue sync
  "sync.cached": { fr: "{n} cartes en cache", en: "{n} cards cached" },
  "sync.last": { fr: "Dernière synchro : {date}", en: "Last sync: {date}" },
  "sync.never": { fr: "jamais", en: "never" },
  "sync.button": { fr: "Synchroniser le catalogue", en: "Sync catalogue" },
  "sync.syncing": { fr: "Synchronisation…", en: "Syncing…" },
  "sync.banner": { fr: "Synchronisation de la base de données…", en: "Syncing the database…" },
  "sync.done": { fr: "Catalogue synchronisé : {n} cartes", en: "Catalogue synced: {n} cards" },
  "sync.progress": { fr: "Synchronisation… (récupère tous les sets, ~30s)", en: "Syncing… (fetching all sets, ~30s)" },
  "sync.failed": { fr: "Dernière synchro en échec : {msg}", en: "Last sync failed: {msg}" },
};

const KEY = "lang";
let lang: Lang = detect();

function detect(): Lang {
  const saved = localStorage.getItem(KEY);
  if (saved === "fr" || saved === "en") return saved;
  return (navigator.language || "").toLowerCase().startsWith("fr") ? "fr" : "en";
}

export function getLang(): Lang {
  return lang;
}

export function setLang(l: Lang) {
  lang = l;
  localStorage.setItem(KEY, l);
  document.documentElement.lang = l;
}

// Apply the detected language to <html lang> at startup.
export function initLang() {
  document.documentElement.lang = lang;
}

export function t(key: string, params?: Record<string, string | number>): string {
  const entry = dict[key];
  let s = entry ? entry[lang] : key;
  if (params) {
    for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  }
  return s;
}
