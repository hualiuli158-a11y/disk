import { formatSize } from '../features/files/fileUtils';
import useAutoDismiss from './useAutoDismiss';

export interface UploadFailure {
  file: File;
  message: string;
}

export interface UploadProgress {
  folderId: number;
  total: number;
  completed: number;
  succeeded: number;
  failed: number;
  currentName: string | null;
  status: 'uploading' | 'complete' | 'cancelled';
  phase: 'transferring' | 'processing' | 'refreshing';
  failures: UploadFailure[];
  percent: number;
  currentPercent: number;
  transferredBytes: number;
  totalBytes: number;
}

export default function UploadStatus({ progress, onCancel, onClose, onLocate, onRetry }: {
  progress: UploadProgress;
  onCancel: () => void;
  onClose: () => void;
  onLocate: (folderId: number) => void;
  onRetry: (files: File[], folderId: number) => void;
}) {
  const active = progress.status === 'uploading';
  const dismissEvents = useAutoDismiss(!active, progress, onClose);
  const refreshing = active && progress.phase === 'refreshing';
  const title = !active
    ? progress.status === 'cancelled' ? '上传已取消' : progress.failed ? progress.succeeded ? '部分文件上传失败' : '上传失败' : '上传完成'
    : refreshing ? '正在同步文件列表' : progress.phase === 'processing' ? '服务器正在处理文件' : '正在上传文件';
  const description = refreshing
    ? '传输已结束，正在刷新列表，请稍候。'
    : active && progress.phase === 'processing'
      ? `已传输：${progress.currentName || '当前文件'}。等待服务器确认，上传尚未完成。`
      : active
        ? `正在传输：${progress.currentName || '当前文件'}`
        : progress.status === 'cancelled'
          ? `已停止后续上传，已成功 ${progress.succeeded} 个文件。`
          : progress.failed ? '部分文件未能上传，可展开查看失败原因。' : '文件已上传并更新列表。';

  return <section className="upload-status" role="region" aria-label="文件上传进度" {...dismissEvents}>
    <div className="upload-status-heading">
      <strong>{title}</strong>
      <button type="button" disabled={refreshing} onClick={active ? onCancel : onClose}>{active ? refreshing ? '请稍候' : '取消上传' : '关闭'}</button>
    </div>
    <p>已完成 {progress.completed} / {progress.total} 个 · 成功 {progress.succeeded} 个 · 失败 {progress.failed} 个</p>
    <div className="upload-transfer"><span>传输进度</span><strong>{formatSize(progress.transferredBytes)} / {formatSize(progress.totalBytes)} · {progress.percent}%</strong></div>
    <div className="upload-progress-track" role="progressbar" aria-label="文件传输进度" aria-valuenow={progress.percent} aria-valuemin={0} aria-valuemax={100} aria-valuetext={`已传输 ${formatSize(progress.transferredBytes)}，共 ${formatSize(progress.totalBytes)}`}>
      <span style={{ width: `${progress.percent}%` }} />
    </div>
    <p className="upload-current" role="status" title={progress.currentName || undefined}>{description}{active && progress.phase === 'transferring' && <span aria-hidden="true">（{progress.currentPercent}%）</span>}</p>
    {!!progress.failures.length && <details className="upload-failures" open={!active}><summary>查看失败文件（{progress.failures.length}）</summary><ul>{progress.failures.map((failure, index) => <li key={`${index}-${failure.file.name}`}><span>{failure.file.name}（{failure.message}）</span>{!active && <button type="button" onClick={() => onRetry([failure.file], progress.folderId)}>重试</button>}</li>)}</ul></details>}
    {!active && <div className="upload-status-actions">
      {progress.succeeded > 0 && <button type="button" onClick={() => onLocate(progress.folderId)}>查看所在文件夹</button>}
      {progress.failures.length > 1 && <button type="button" onClick={() => onRetry(progress.failures.map(failure => failure.file), progress.folderId)}>重试全部失败文件</button>}
    </div>}
  </section>;
}
