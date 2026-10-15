import { useCallback, useMemo, useState } from 'react';

export type MarkdownSaveState = 'clean' | 'dirty' | 'saving' | 'conflict' | 'error';

export interface MarkdownDraft {
  fileId: number;
  content: string;
  savedContent: string;
  expectedSHA256: string;
  state: MarkdownSaveState;
  error?: string;
}

export function draftHasUnsavedChanges(draft: MarkdownDraft | null | undefined): boolean {
  return !!draft && draft.content !== draft.savedContent;
}

export function reconcileSavedDraft(current: MarkdownDraft | undefined, fileId: number, savedContent: string, sha256: string): MarkdownDraft {
  const latestContent = current?.content ?? savedContent;
  const changedDuringSave = latestContent !== savedContent;
  return {
    fileId,
    content: latestContent,
    savedContent,
    expectedSHA256: sha256,
    state: changedDuringSave ? 'dirty' : 'clean',
  };
}

export function editMarkdownDraft(current: MarkdownDraft, content: string): MarkdownDraft {
  const unsaved = content !== current.savedContent;
  return {
    ...current,
    content,
    // Keep the request in flight even if the user types again. Once it finishes,
    // reconcileSavedDraft will schedule the remaining edits against the new SHA.
    state: current.state === 'saving' ? 'saving'
      : current.state === 'conflict' && unsaved ? 'conflict'
      : unsaved ? 'dirty' : 'clean',
    error: current.state === 'conflict' && unsaved ? current.error : undefined,
  };
}

export default function useMarkdownDrafts() {
  const [drafts, setDrafts] = useState<Record<number, MarkdownDraft>>({});

  const ensureDraft = useCallback((fileId: number, content: string, sha256: string | null | undefined) => {
    if (!fileId || !sha256) return;
    setDrafts(previous => {
      const existing = previous[fileId];
      if (existing && draftHasUnsavedChanges(existing)) return previous;
      if (existing && existing.expectedSHA256 === sha256 && existing.savedContent === content) return previous;
      return {
        ...previous,
        [fileId]: {
          fileId,
          content,
          savedContent: content,
          expectedSHA256: sha256,
          state: 'clean',
        },
      };
    });
  }, []);

  const updateDraft = useCallback((fileId: number, content: string) => {
    setDrafts(previous => {
      const current = previous[fileId];
      if (!current || current.content === content) return previous;
      return { ...previous, [fileId]: editMarkdownDraft(current, content) };
    });
  }, []);

  const markSaving = useCallback((fileId: number) => {
    setDrafts(previous => previous[fileId] ? {
      ...previous,
      [fileId]: { ...previous[fileId], state: 'saving', error: undefined },
    } : previous);
  }, []);

  const markSaved = useCallback((fileId: number, savedContent: string, sha256: string) => {
    setDrafts(previous => ({
      ...previous,
      [fileId]: reconcileSavedDraft(previous[fileId], fileId, savedContent, sha256),
    }));
  }, []);

  const markConflict = useCallback((fileId: number, message: string) => {
    setDrafts(previous => previous[fileId] ? {
      ...previous,
      [fileId]: { ...previous[fileId], state: 'conflict', error: message },
    } : previous);
  }, []);

  const markError = useCallback((fileId: number, message: string) => {
    setDrafts(previous => previous[fileId] ? {
      ...previous,
      [fileId]: { ...previous[fileId], state: 'error', error: message },
    } : previous);
  }, []);

  const discardDraft = useCallback((fileId: number) => {
    setDrafts(previous => {
      if (!previous[fileId]) return previous;
      const next = { ...previous };
      delete next[fileId];
      return next;
    });
  }, []);

  const discardDrafts = useCallback((fileIds: Iterable<number>) => {
    setDrafts(previous => {
      const next = { ...previous };
      let changed = false;
      for (const fileId of fileIds) {
        if (!next[fileId]) continue;
        delete next[fileId];
        changed = true;
      }
      return changed ? next : previous;
    });
  }, []);

  const hasUnsavedDrafts = useMemo(
    () => Object.values(drafts).some(draftHasUnsavedChanges),
    [drafts],
  );

  return {
    drafts,
    ensureDraft,
    updateDraft,
    markSaving,
    markSaved,
    markConflict,
    markError,
    discardDraft,
    discardDrafts,
    hasUnsavedDrafts,
  };
}
