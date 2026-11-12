import { useCallback, useEffect, useState } from 'react';
import { userErrorMessage } from '../../services/userMessages';
import type { ContentItem, FileVersion } from '../../services/api';
import { api } from '../../services/api';
import Icon from '../../components/Icon';
import { formatSize } from '../files/fileUtils';

const SOURCE_LABELS: Record<FileVersion['source'], string> = {
  upload: '初始上传',
  edit: '编辑快照',
  restore: '历史恢复',
};

function formatVersionTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? '时间未知' : date.toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' });
}

export default function FileHistoryPanel({ file, onClose, onRestore }: {
  file: ContentItem;
  onClose: () => void;
  onRestore: (versionId: number) => Promise<boolean>;
}) {
  const [versions, setVersions] = useState<FileVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try { setVersions((await api.fileVersions(file.id)).items); }
    catch (cause) { setError(userErrorMessage(cause)); }
    finally { setLoading(false); }
  }, [file]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  async function downloadVersion(version: FileVersion) {
    setBusyId(version.id);
    setError('');
    try {
      const blob = await api.downloadFileVersion(file.id, version.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${file.name}.version-${version.id}`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
    } catch (cause) { setError(userErrorMessage(cause)); }
    finally { setBusyId(null); }
  }

  async function restoreVersion(version: FileVersion) {
    if (version.current || !window.confirm(`将「${file.name}」恢复到 ${formatVersionTime(version.created_at)} 的内容？当前未创建快照的内容会被覆盖；已有历史快照会保留。`)) return;
    setBusyId(version.id);
    setError('');
    try {
      if (await onRestore(version.id)) await load();
    } finally { setBusyId(null); }
  }

  return <div className="history-backdrop" role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <aside className="history-panel" role="dialog" aria-modal="true" aria-label={`${file.name} 的版本历史`}>
      <header><div><span className="history-icon"><Icon name="clock" size={19} /></span><div><h2>版本历史</h2><p>{file.name}</p></div></div><button type="button" aria-label="关闭版本历史" onClick={onClose}><Icon name="close" size={18} /></button></header>
      {error && <div className="history-error" role="alert">{error}</div>}
      <p className="history-policy">自动保存不生成历史。按 Ctrl / ⌘ + S 保存一个版本快照。</p>
      <div className="history-list">
        {loading ? <div className="history-loading"><span className="loading-dot" />正在加载版本…</div> : versions.length ? versions.map(version => <article key={version.id} className={`history-version ${version.current ? 'current' : ''}`}>
          <div className="history-timeline"><span /></div>
          <div className="history-version-body"><div className="history-version-heading"><strong>{formatVersionTime(version.created_at)}</strong>{version.current && <span>当前版本</span>}</div>
            <p>{SOURCE_LABELS[version.source] || version.source} · {formatSize(version.size)}</p><code>{version.sha256.slice(0, 12)}</code>
            <div className="history-version-actions"><button type="button" disabled={busyId != null} onClick={() => void downloadVersion(version)}><Icon name="download" size={15} />下载</button>{!version.current && <button type="button" disabled={busyId != null} onClick={() => void restoreVersion(version)}><Icon name="clock" size={15} />恢复此版本</button>}</div>
          </div>
        </article>) : <div className="history-loading">暂无版本记录</div>}
      </div>
    </aside>
  </div>;
}
