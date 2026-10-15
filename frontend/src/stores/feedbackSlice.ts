import type { StateCreator } from 'zustand';
import type { WorkspaceStore } from './workspaceStore';

export interface FeedbackSlice {
  error: string;
  errorRevision: number;
  notice: string;
  setError: (error: string) => void;
  setNotice: (notice: string) => void;
  clearFeedback: () => void;
}

export const createFeedbackSlice: StateCreator<WorkspaceStore, [], [], FeedbackSlice> = set => ({
  error: '',
  errorRevision: 0,
  notice: '',
  setError: error => set(state => ({ error, errorRevision: state.errorRevision + 1 })),
  setNotice: notice => set({ notice }),
  clearFeedback: () => set({ error: '', notice: '' }),
});
