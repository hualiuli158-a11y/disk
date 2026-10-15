import type { ContentItem, Folder } from '../../services/api';
import Empty from '../../components/Empty';
import Icon from '../../components/Icon';
import type { Heading } from '../preview/MarkdownPreview';
import { describeFile, formatDate, formatSize, iconForFileKind } from '../files/fileUtils';

interface Props {
  width?: number;
  tab: 'outline' | 'details';
  outline: Heading[];
  currentFile: ContentItem | null;
  activeFolder: Folder | null;
  onTabChange: (tab: 'outline' | 'details') => void;
  onClose: () => void;
  onDownload: (item: ContentItem) => void;
  onRename: (item: ContentItem) => void;
  onRemove: (item: ContentItem) => void;
}

export default function Inspector({ width, tab, outline, currentFile, activeFolder, onTabChange, onClose, onDownload, onRename, onRemove }: Props) {
  const descriptor = currentFile ? describeFile(currentFile.name) : null;
  return <aside className="inspector" style={width == null ? undefined : { width }}><div className="inspector-tabs"><button className={tab === 'details' ? 'active' : ''} onClick={() => onTabChange('details')}><Icon name="info" size={17} />详情</button><button className={tab === 'outline' ? 'active' : ''} onClick={() => onTabChange('outline')}><Icon name="outline" size={17} />大纲</button></div><button type="button" className="inspector-close" aria-label="关闭详情面板" onClick={onClose}><Icon name="close" size={18} /></button>
    {tab === 'outline' ? <div className="inspector-content">{outline.length ? <div className="outline-list">{outline.map(heading => <button key={heading.index} style={{ paddingLeft: `${16 + (heading.level - 1) * 13}px` }} onClick={() => document.getElementById(`heading-${currentFile?.id}-${heading.index}`)?.scrollIntoView({ behavior: 'smooth' })}>{heading.text}</button>)}</div> : <Empty icon="outline" title="暂无大纲" />}</div> : <div className="inspector-content"><div className="details-card"><div className="details-icon"><Icon name={descriptor ? iconForFileKind(descriptor.kind) : 'folder'} size={28} /></div><h3>{currentFile?.name || activeFolder?.name || '我的空间'}</h3>{currentFile && <div className="details-meta"><span>{formatSize(currentFile.size)}</span><span>{formatDate(currentFile.updated_at)}</span></div>}</div>{currentFile && <div className="details-actions"><button onClick={() => onDownload(currentFile)}><Icon name="download" size={17} />下载</button><button onClick={() => onRename(currentFile)}><Icon name="edit" size={17} />重命名</button><button className="danger" onClick={() => onRemove(currentFile)}><Icon name="trash" size={17} />移到回收站</button></div>}</div>}
  </aside>;
}
