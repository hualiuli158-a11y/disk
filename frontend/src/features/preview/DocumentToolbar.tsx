import type { ContentItem } from '../../services/api';
import Button from '../../components/Button';
import Icon from '../../components/Icon';
import FileGlyph from '../files/FileGlyph';
import type { MarkdownDraft } from './useMarkdownDrafts';

const SAVE_LABELS: Record<MarkdownDraft['state'], string> = {
  clean: '已保存',
  dirty: '等待自动保存',
  saving: '保存中…',
  conflict: '存在冲突',
  error: '保存失败',
};

export default function DocumentToolbar({ file, draft, mode, onToggleMode, onReload, onDownload, onHistory, onFind }: {
  file: ContentItem;
  draft?: MarkdownDraft;
  mode?: 'edit' | 'preview';
  onToggleMode: () => void;
  onReload: () => void;
  onDownload: () => void;
  onHistory: () => void;
  onFind?: () => void;
}) {
  return <div className="document-toolbar">
    <div className="document-meta"><FileGlyph item={file} /><span>{file.name}</span>{draft && <span className={`save-state ${draft.state}`} role="status">{SAVE_LABELS[draft.state]}</span>}</div>
    <div className="document-actions">
      {onFind && <Button icon="search" label="在文档中查找" onClick={onFind} />}
      {draft && <button type="button" className="save-button" onClick={onToggleMode}><Icon name={mode === 'preview' ? 'edit' : 'eye'} size={16} />{mode === 'preview' ? '编辑' : '预览'}</button>}
      <Button icon="clock" label="版本历史" onClick={onHistory} />
      <Button icon="clock" label="重新加载预览" onClick={onReload} />
      <Button icon="download" label="下载文件" onClick={onDownload} />
    </div>
  </div>;
}
