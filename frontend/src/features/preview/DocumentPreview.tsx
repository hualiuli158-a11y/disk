import { useCallback, useEffect, useState } from 'react';
import type { ContentItem } from '../../services/api';
import { describeFile } from '../files/fileUtils';
import type { Preview } from '../files/fileUtils';
import DocumentToolbar from './DocumentToolbar';
import MarkdownWorkspace from './MarkdownWorkspace';
import MediaViewer from './MediaViewer';
import PreviewRenderer from './PreviewRenderer';
import FileHistoryPanel from './FileHistoryPanel';
import type { MarkdownDraft } from './useMarkdownDrafts';

interface MediaNavigation {
  hasPrevious: boolean;
  hasNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
}

interface Props {
  file: ContentItem;
  preview: Preview | null;
  draft?: MarkdownDraft;
  mediaNavigation?: MediaNavigation;
  onDraftChange: (content: string) => void;
  onSave: (createVersion?: boolean) => void;
  onDiscardDraftAndReload: () => void;
  onDownload: (item: ContentItem) => void;
  onReload: () => void;
  onRestoreVersion: (versionId: number) => Promise<boolean>;
}

export default function DocumentPreview({ file, preview, draft, mediaNavigation, onDraftChange, onSave, onDiscardDraftAndReload, onDownload, onReload, onRestoreVersion }: Props) {
  const [viewerOpen, setViewerOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [findRequest, setFindRequest] = useState({ fileId: file.id, sequence: 0 });
  const [viewMode, setViewMode] = useState<{ fileId: number; mode: 'edit' | 'preview' }>({ fileId: file.id, mode: 'preview' });
  const markdownMode = viewMode.fileId === file.id ? viewMode.mode : 'preview';
  const toggleMarkdownMode = () => setViewMode({ fileId: file.id, mode: markdownMode === 'edit' ? 'preview' : 'edit' });
  const [decodeError, setDecodeError] = useState('');
  const previewUrl = preview?.fileId === file.id && preview.status === 'ready' ? preview.url : undefined;
  const closeViewer = useCallback(() => setViewerOpen(false), []);
  const fileStrategy = preview?.fileId === file.id ? preview.descriptor.preview : describeFile(file.name).preview;
  // Keep the same editor mounted when a successful save refreshes file metadata.
  const descriptor = preview?.fileId === file.id ? preview.descriptor : describeFile(file.name);
  const editableText = fileStrategy === 'markdown' || descriptor.kind === 'text';
  const markdownReady = editableText && draft?.fileId === file.id;
  const mediaKind = fileStrategy === 'image' || fileStrategy === 'video' ? fileStrategy : null;

  useEffect(() => { setDecodeError(''); }, [file.id, previewUrl]);
  useEffect(() => {
    if (mediaKind) return;
    setViewerOpen(false);
  }, [mediaKind]);

  return <div className="document-page">
    <DocumentToolbar file={file} draft={markdownReady ? draft : undefined} mode={markdownMode} onToggleMode={toggleMarkdownMode} onReload={onReload} onDownload={() => onDownload(file)} onHistory={() => setHistoryOpen(true)} onFind={markdownReady ? () => setFindRequest(current => ({ fileId: file.id, sequence: current.sequence + 1 })) : undefined} />
    {draft?.state === 'conflict' && <div className="editor-notice conflict"><div><strong>远端文件已发生变化</strong><span>{draft.error || '当前草稿没有被覆盖。请先复制草稿，或放弃草稿并重新加载。'}</span></div><div><button type="button" onClick={() => void navigator.clipboard.writeText(draft.content)}>复制草稿</button><button type="button" className="danger" onClick={onDiscardDraftAndReload}>放弃草稿并重新加载</button></div></div>}
    {draft?.state === 'error' && <div className="editor-notice error"><div><strong>保存失败</strong><span>{draft.error || '请检查网络后重试。'}</span></div><button type="button" onClick={() => onSave()}>重试</button></div>}
    {markdownReady
      ? <MarkdownWorkspace key={file.id} fileId={file.id} draft={draft} plainText={fileStrategy !== 'markdown'} mode={markdownMode} findRequest={findRequest.fileId === file.id ? findRequest.sequence : 0} onDraftChange={onDraftChange} onSave={onSave} />
      : editableText && preview?.status === 'ready' && !!(preview.sha256 || file.sha256)
        ? <div className="editor-loading" role="status">正在准备编辑器…</div>
        : <PreviewRenderer file={file} preview={preview} decodeError={decodeError} onDecodeError={setDecodeError}
            onDownload={() => onDownload(file)} onOpenViewer={() => setViewerOpen(true)} />}
    {viewerOpen && mediaKind && <MediaViewer kind={mediaKind} name={file.name} url={previewUrl} loading={!previewUrl}
      hasPrevious={mediaNavigation?.hasPrevious} hasNext={mediaNavigation?.hasNext}
      onPrevious={mediaNavigation?.onPrevious} onNext={mediaNavigation?.onNext} onClose={closeViewer} />}
    {historyOpen && <FileHistoryPanel file={file} onClose={() => setHistoryOpen(false)} onRestore={onRestoreVersion} />}
  </div>;
}
