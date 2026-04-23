import { create } from "zustand";
import { snippet_entity, snippet_svc } from "../../wailsjs/go/models";
import {
  ListSnippetCategories,
  ListSnippets,
  CreateSnippet,
  UpdateSnippet,
  DeleteSnippet,
  DuplicateSnippet,
  RecordSnippetUse,
} from "../../wailsjs/go/app/App";

type Snippet = snippet_entity.Snippet;
type Category = snippet_svc.Category;

// Monotonic request counter for loadList. Rapid filter changes (typing in
// search / tag, drawer opens in PR 3) can overlap; a slow earlier response
// must not clobber the UI with stale data.
let loadListReqId = 0;

export type SnippetFilter = {
  categories: string[]; // empty = all
  keyword: string;
  tag: string;
};

interface SnippetState {
  categories: Category[];
  categoriesLoading: boolean;
  list: Snippet[];
  listLoading: boolean;
  filter: SnippetFilter;

  loadCategories: () => Promise<void>;
  loadList: () => Promise<void>;
  setFilter: (patch: Partial<SnippetFilter>) => void;

  create: (req: snippet_svc.CreateReq) => Promise<Snippet>;
  update: (req: snippet_svc.UpdateReq) => Promise<Snippet>;
  remove: (id: number) => Promise<void>;
  duplicate: (id: number) => Promise<Snippet>;
  recordUse: (id: number) => void;
}

export const useSnippetStore = create<SnippetState>()((set, get) => ({
  categories: [],
  categoriesLoading: false,
  list: [],
  listLoading: false,
  filter: { categories: [], keyword: "", tag: "" },

  loadCategories: async () => {
    set({ categoriesLoading: true });
    try {
      const cats = await ListSnippetCategories();
      set({ categories: cats ?? [], categoriesLoading: false });
    } catch (e) {
      set({ categoriesLoading: false });
      throw e;
    }
  },

  loadList: async () => {
    const myId = ++loadListReqId;
    const { filter } = get();
    set({ listLoading: true });
    try {
      // Wails generated class uses camelCase; management page lists everything
      // (AssetID omitted → pointer stays nil → include all, global and bound).
      const req: snippet_svc.ListReq = {
        categories: filter.categories,
        assetId: undefined,
        includeGlobal: true,
        keyword: filter.keyword,
        tag: filter.tag,
        limit: 0,
        offset: 0,
        orderBy: "",
      } as unknown as snippet_svc.ListReq;
      const items = await ListSnippets(req);
      if (myId !== loadListReqId) {
        // A newer request is in flight; let it own the state.
        return;
      }
      set({ list: items ?? [], listLoading: false });
    } catch (e) {
      if (myId === loadListReqId) set({ listLoading: false });
      throw e;
    }
  },

  setFilter: (patch) => {
    const cur = get().filter;
    const next = { ...cur, ...patch };
    if (
      cur.keyword === next.keyword &&
      cur.tag === next.tag &&
      cur.categories.length === next.categories.length &&
      cur.categories.every((c, i) => c === next.categories[i])
    ) {
      return; // no-op — avoid a redundant ListSnippets round-trip.
    }
    set({ filter: next });
    void get().loadList();
  },

  create: async (req) => {
    const s = await CreateSnippet(req);
    await get().loadList();
    return s;
  },

  update: async (req) => {
    const s = await UpdateSnippet(req);
    await get().loadList();
    return s;
  },

  remove: async (id) => {
    await DeleteSnippet(id);
    await get().loadList();
  },

  duplicate: async (id) => {
    const s = await DuplicateSnippet(id);
    await get().loadList();
    return s;
  },

  recordUse: (id) => {
    // Fire-and-forget telemetry; never surface errors to caller.
    void Promise.resolve()
      .then(() => RecordSnippetUse(id))
      .catch(() => {});
  },
}));
