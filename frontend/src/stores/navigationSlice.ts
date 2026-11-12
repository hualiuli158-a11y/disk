import type { StateCreator } from 'zustand';
import type { ContentSortField, SearchKind, SortOrder } from '../services/api';
import type { Section } from '../features/files/FileList';
import type { WorkspaceStore } from './workspaceStore';

const SORT_STORAGE_KEY = 'qipan-content-sort';

function initialSort(): { field: ContentSortField; order: SortOrder } {
  try {
    const value = JSON.parse(localStorage.getItem(SORT_STORAGE_KEY) || 'null') as { field?: string; order?: string } | null;
    const field = value?.field;
    const order = value?.order;
    if ((field === 'name' || field === 'size' || field === 'created_at' || field === 'updated_at') && (order === 'asc' || order === 'desc')) return { field, order };
  } catch { /* Use defaults when persistence is unavailable. */ }
  return { field: 'name', order: 'asc' };
}

export interface NavigationSlice {
  section: Section;
  page: number;
  query: string;
  sort: { field: ContentSortField; order: SortOrder };
  searchKind: SearchKind;
  setSection: (section: Section) => void;
  setPage: (page: number) => void;
  setQuery: (query: string) => void;
  setSort: (sort: { field: ContentSortField; order: SortOrder }) => void;
  setSearchKind: (kind: SearchKind) => void;
  resetNavigation: () => void;
}

export const createNavigationSlice: StateCreator<WorkspaceStore, [], [], NavigationSlice> = set => ({
  section: 'files',
  page: 1,
  query: '',
  sort: initialSort(),
  searchKind: 'all',
  setSection: section => set({ section }),
  setPage: page => set({ page }),
  setQuery: query => set({ query }),
  setSort: sort => {
    try { localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify(sort)); } catch { /* Keep in-memory sorting. */ }
    set({ sort });
  },
  setSearchKind: searchKind => set({ searchKind }),
  resetNavigation: () => set({ section: 'files', page: 1, query: '', searchKind: 'all' }),
});
