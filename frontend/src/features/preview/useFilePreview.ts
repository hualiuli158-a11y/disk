import { useCallback, useEffect, useState } from 'react';
import type { ContentItem } from '../../services/api';
import { userErrorMessage } from '../../services/userMessages';
import { describeFile, previewVersionKey } from '../files/fileUtils';
import type { Preview } from '../files/fileUtils';
import { isPreviewAbort, loadFilePreview, previewLoadingState } from './previewLoader';

export interface UseFilePreviewResult {
  preview: Preview | null;
  reload: () => void;
}

export default function useFilePreview(file: ContentItem | null): UseFilePreviewResult {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [reloadSequence, setReloadSequence] = useState(0);
  const versionKey = file ? previewVersionKey(file) : '';

  useEffect(() => {
    if (!file) { setPreview(null); return; }
    const selected = file;
    const controller = new AbortController();
    let release = () => {};
    setPreview(previewLoadingState(selected));

    void loadFilePreview(selected, controller.signal).then(result => {
      if (controller.signal.aborted) {
        result.release();
        return;
      }
      release = result.release;
      setPreview(result.preview);
    }).catch(cause => {
      if (controller.signal.aborted || isPreviewAbort(cause)) return;
      setPreview({
        fileId: selected.id,
        status: 'unavailable',
        descriptor: describeFile(selected.name),
        reason: 'load-error',
        message: userErrorMessage(cause),
      });
    });

    return () => {
      controller.abort();
      release();
    };
    // versionKey deliberately folds every server-visible file version field into one stable dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [versionKey, reloadSequence]);

  const reload = useCallback(() => {
    setPreview(null);
    setReloadSequence(sequence => sequence + 1);
  }, []);
  return { preview, reload };
}
