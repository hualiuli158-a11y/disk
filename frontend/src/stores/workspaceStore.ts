import { create } from 'zustand';
import { createFeedbackSlice } from './feedbackSlice';
import type { FeedbackSlice } from './feedbackSlice';
import { createNavigationSlice } from './navigationSlice';
import type { NavigationSlice } from './navigationSlice';
import { createPreviewSlice } from './previewSlice';
import type { PreviewSlice } from './previewSlice';

export type WorkspaceStore = PreviewSlice & NavigationSlice & FeedbackSlice;

export const useWorkspaceStore = create<WorkspaceStore>()((...args) => ({
  ...createPreviewSlice(...args),
  ...createNavigationSlice(...args),
  ...createFeedbackSlice(...args),
}));
