import type { StateCreator } from 'zustand';
import type { ContentItem } from '../services/api';
import type { Updater } from './types';
import { applyUpdater } from './types';
import type { WorkspaceStore } from './workspaceStore';

export interface PreviewSlice {
  tabs: ContentItem[];
  activeFileId: number | null;
  rightOpen: boolean;
  rightTab: 'outline' | 'details';
  setTabs: (update: Updater<ContentItem[]>) => void;
  setActiveFileId: (id: number | null) => void;
  setRightOpen: (open: boolean) => void;
  setRightTab: (tab: 'outline' | 'details') => void;
  resetPreview: () => void;
}

export type PanelSide = 'left' | 'right';

export const PANEL_DEFAULTS = { left: 252, right: 280 } as const;
const PANEL_BOUNDS = { left: [180, 400], right: [220, 600] } as const;
const PANEL_STORAGE_KEYS = { left: 'qipan-explorer-width', right: 'qipan-inspector-width' } as const;

export function clampPanel(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(Math.max(min, max), value));
}

export function readPanelPreference(side: PanelSide): number {
  const [min, max] = PANEL_BOUNDS[side];
  try {
    const stored = window.localStorage.getItem(PANEL_STORAGE_KEYS[side]);
    if (stored === null) return PANEL_DEFAULTS[side];
    const parsed = Number(stored);
    return Number.isFinite(parsed) ? Math.round(clampPanel(parsed, min, max)) : PANEL_DEFAULTS[side];
  } catch { return PANEL_DEFAULTS[side]; }
}

export function savePanelPreference(side: PanelSide, width: number): void {
  try { window.localStorage.setItem(PANEL_STORAGE_KEYS[side], String(width)); }
  catch { /* Layout remains usable if browser storage is disabled. */ }
}

export function resetPanelPreferences(): void {
  try {
    window.localStorage.removeItem(PANEL_STORAGE_KEYS.left);
    window.localStorage.removeItem(PANEL_STORAGE_KEYS.right);
  } catch { /* Layout remains usable if browser storage is disabled. */ }
}

export function calculatePanelLayout(viewport: number, leftPreference: number, rightPreference: number, rightOpen: boolean) {
  const showLeft = viewport > 720;
  const showRight = viewport > 980 && rightOpen;
  const rail = viewport <= 720 ? 54 : viewport <= 760 ? 49 : 62;
  const room = viewport - rail - 400 - (showLeft ? 8 : 0) - (showRight ? 8 : 0);
  const leftMax = Math.min(PANEL_BOUNDS.left[1], Math.max(PANEL_BOUNDS.left[0], room - (showRight ? PANEL_BOUNDS.right[0] : 0)));
  const left = showLeft ? clampPanel(leftPreference, PANEL_BOUNDS.left[0], leftMax) : 0;
  const rightMax = Math.min(PANEL_BOUNDS.right[1], Math.max(PANEL_BOUNDS.right[0], room - left));
  const right = showRight ? clampPanel(rightPreference, PANEL_BOUNDS.right[0], rightMax) : 0;
  return { left, right, leftMax, rightMax, showLeft, showRight };
}

export function defaultRightOpen(): boolean {
  return false;
}

export const createPreviewSlice: StateCreator<WorkspaceStore, [], [], PreviewSlice> = set => ({
  tabs: [],
  activeFileId: null,
  rightOpen: defaultRightOpen(),
  rightTab: 'details',
  setTabs: update => set(state => ({ tabs: applyUpdater(state.tabs, update) })),
  setActiveFileId: activeFileId => set({ activeFileId }),
  setRightOpen: rightOpen => set({ rightOpen }),
  setRightTab: rightTab => set({ rightTab }),
  resetPreview: () => set({ tabs: [], activeFileId: null, rightOpen: defaultRightOpen(), rightTab: 'details' }),
});
