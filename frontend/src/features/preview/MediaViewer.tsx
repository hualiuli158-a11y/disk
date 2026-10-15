import { useEffect, useRef, useState } from 'react';
import Icon from '../../components/Icon';

interface Props {
  kind: 'image' | 'video';
  name: string;
  url?: string;
  loading?: boolean;
  hasPrevious?: boolean;
  hasNext?: boolean;
  onPrevious?: () => void;
  onNext?: () => void;
  onClose: () => void;
}

interface Offset { x: number; y: number }

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export default function MediaViewer({ kind, name, url, loading = false, hasPrevious = false, hasNext = false, onPrevious, onNext, onClose }: Props) {
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState<Offset>({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const drag = useRef<{ x: number; y: number; baseX: number; baseY: number } | null>(null);

  const reset = () => {
    drag.current = null;
    setDragging(false);
    setScale(1);
    setOffset({ x: 0, y: 0 });
  };

  const zoom = (factor: number) => {
    setScale(current => {
      const next = clamp(current * factor, 1, 8);
      if (next === 1) setOffset({ x: 0, y: 0 });
      return next;
    });
  };

  useEffect(reset, [url]);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (kind === 'image' && (event.key === '+' || event.key === '=')) zoom(1.2);
      if (kind === 'image' && event.key === '-') zoom(1 / 1.2);
      if (kind === 'image' && event.key === '0') reset();
      if (event.key === 'ArrowLeft' && hasPrevious) onPrevious?.();
      if (event.key === 'ArrowRight' && hasNext) onNext?.();
    };
    window.addEventListener('keydown', handleKey);
    return () => {
      window.removeEventListener('keydown', handleKey);
      document.body.style.overflow = previousOverflow;
    };
  }, [hasNext, hasPrevious, kind, onClose, onNext, onPrevious]);

  return <div className="media-viewer-overlay" role="dialog" aria-modal="true" aria-label={`查看 ${name}`} onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <div className="media-viewer-toolbar">
      <span className="media-viewer-name">{name}</span>
      <div className="media-viewer-actions">
        {kind === 'image' && <>
          <button type="button" title="缩小" aria-label="缩小" onClick={() => zoom(1 / 1.2)}><Icon name="minus" size={18} /></button>
          <button type="button" title="恢复适应窗口" aria-label="恢复适应窗口" onClick={reset}><Icon name="fit" size={18} /></button>
          <button type="button" title="放大" aria-label="放大" onClick={() => zoom(1.2)}><Icon name="plus" size={18} /></button>
          <span className="media-viewer-scale">{Math.round(scale * 100)}%</span>
        </>}
        <button type="button" title="关闭" aria-label="关闭" onClick={onClose}><Icon name="close" size={18} /></button>
      </div>
    </div>
    <div className="media-viewer-stage" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
      {loading || !url ? <span className="loading-dot light" role="status" aria-label="正在加载媒体" /> : kind === 'image' ? <img
        src={url}
        alt={name}
        draggable={false}
        style={{ transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})`, cursor: scale > 1 ? (dragging ? 'grabbing' : 'grab') : 'default' }}
        onWheel={event => { event.preventDefault(); zoom(event.deltaY < 0 ? 1.12 : 1 / 1.12); }}
        onDoubleClick={() => { if (scale > 1) reset(); else zoom(2); }}
        onPointerDown={event => {
          if (scale <= 1 || event.button !== 0) return;
          event.preventDefault();
          event.currentTarget.setPointerCapture(event.pointerId);
          drag.current = { x: event.clientX, y: event.clientY, baseX: offset.x, baseY: offset.y };
          setDragging(true);
        }}
        onPointerMove={event => {
          if (!drag.current) return;
          setOffset({ x: drag.current.baseX + event.clientX - drag.current.x, y: drag.current.baseY + event.clientY - drag.current.y });
        }}
        onPointerUp={event => {
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
          drag.current = null;
          setDragging(false);
        }}
        onPointerCancel={() => { drag.current = null; setDragging(false); }}
      /> : <video src={url} controls autoPlay={false} />}
      <button type="button" className="media-viewer-nav previous" disabled={!hasPrevious} aria-label="上一个媒体" title="上一个媒体（←）" onClick={event => { event.stopPropagation(); onPrevious?.(); }}><Icon name="chevron" size={28} /></button>
      <button type="button" className="media-viewer-nav next" disabled={!hasNext} aria-label="下一个媒体" title="下一个媒体（→）" onClick={event => { event.stopPropagation(); onNext?.(); }}><Icon name="chevron" size={28} /></button>
    </div>
  </div>;
}
