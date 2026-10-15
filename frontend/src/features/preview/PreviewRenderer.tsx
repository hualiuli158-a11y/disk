import { lazy, Suspense, type ReactNode } from 'react';
import type { ContentItem } from '../../services/api';
import Empty from '../../components/Empty';
import Icon from '../../components/Icon';
import type { Preview } from '../files/fileUtils';
import CsvPreview from './CsvPreview';
const MarkdownPreview = lazy(() => import('./MarkdownPreview'));
const CodePreview = lazy(() => import('./CodePreview'));

export default function PreviewRenderer({ file, preview, decodeError, onDecodeError, onDownload, onOpenViewer }: {
  file: ContentItem;
  preview: Preview | null;
  decodeError: string;
  onDecodeError: (message: string) => void;
  onDownload: () => void;
  onOpenViewer: () => void;
}) {
  let content: ReactNode;
  if (!preview || preview.fileId !== file.id || preview.status === 'loading') {
    content = <div className="editor-loading"><span className="loading-dot" role="status" aria-label="正在加载预览" /></div>;
  } else if (preview.status === 'unavailable') {
    content = <Empty icon={preview.reason === 'load-error' ? 'info' : 'file'} title="预览不可用" text={preview.message} action={<button className="primary-button" onClick={onDownload}>下载文件</button>} />;
  } else if (decodeError) {
    content = <Empty icon="info" title="浏览器无法解码" text={decodeError} action={<button className="primary-button" onClick={onDownload}>下载文件</button>} />;
  } else {
    const strategy = preview.descriptor.preview;
    if (strategy === 'markdown') content = <Suspense fallback={<div className="editor-loading" role="status">正在排版…</div>}><MarkdownPreview content={preview.text || ''} fileId={String(file.id)} /></Suspense>;
    else if (strategy === 'text') content = <Suspense fallback={<div className="editor-loading" role="status">正在加载代码…</div>}><CodePreview key={file.id} content={preview.text || ''} extension={preview.descriptor.ext} /></Suspense>;
    else if (strategy === 'csv') content = <CsvPreview fileName={file.name} content={preview.text || ''} />;
    else if (strategy === 'image' && preview.url) content = <button type="button" className="media-preview media-preview-button" onClick={onOpenViewer} title="打开图片查看器"><img src={preview.url} alt={file.name} onError={() => onDecodeError('当前浏览器不支持这种图片编码。')} /></button>;
    else if (strategy === 'video' && preview.url) content = <div className="media-preview media-preview-video"><video src={preview.url} controls preload="metadata" onError={() => onDecodeError('当前浏览器不支持这种视频编码。')} /><button type="button" className="media-expand-button" onClick={onOpenViewer} title="全屏查看" aria-label="全屏查看"><Icon name="external" size={17} /></button></div>;
    else if (strategy === 'audio' && preview.url) content = <div className="audio-preview"><div className="audio-preview-icon"><Icon name="audio" size={30} /></div><strong>{file.name}</strong><audio src={preview.url} controls preload="metadata" onError={() => onDecodeError('当前浏览器不支持这种音频编码。')} /></div>;
    else if (strategy === 'pdf' && preview.url) content = <object className="pdf-preview" title={file.name} data={preview.url} type="application/pdf"><Empty icon="pdf" title="PDF 预览不可用" action={<button className="primary-button" onClick={onDownload}>下载文件</button>} /></object>;
    else content = <Empty icon="file" title="无法在线预览" action={<button className="primary-button" onClick={onDownload}>下载文件</button>} />;
  }

  return <div className={`document-content preview preview-${preview?.descriptor.preview || 'loading'}`}><div className="preview-pane"><div className="paper">{content}</div></div></div>;
}
