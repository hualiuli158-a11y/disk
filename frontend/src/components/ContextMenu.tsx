import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import Icon from './Icon';

export type ContextMenuEntry =
  | { kind: 'separator' }
  | {
      kind?: 'item';
      label: string;
      icon?: string;
      disabled?: boolean;
      danger?: boolean;
      checked?: boolean;
      action: () => void;
    };

interface Props {
  position: { x: number; y: number };
  items: ContextMenuEntry[];
  onClose: () => void;
}

export default function ContextMenu({ position, items, onClose }: Props) {
  const menuRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const [placed, setPlaced] = useState(position);

  useLayoutEffect(() => {
    const menu = menuRef.current;
    if (!menu) return;
    const rect = menu.getBoundingClientRect();
    const gap = 8;
    setPlaced({
      x: Math.max(gap, Math.min(position.x, window.innerWidth - rect.width - gap)),
      y: Math.max(gap, Math.min(position.y, window.innerHeight - rect.height - gap)),
    });
  }, [position, items]);

  useEffect(() => {
    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    menuRef.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
  }, []);

  useEffect(() => {
    const closeFromPointer = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) onClose();
    };
    const closeFromKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' || event.key === 'Tab') {
        event.preventDefault();
        event.stopPropagation();
        onClose();
        previousFocusRef.current?.focus();
        return;
      }
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
      const buttons = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]:not(:disabled)') || []);
      if (!buttons.length) return;
      event.preventDefault();
      event.stopPropagation();
      const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1
        : current < 0 ? event.key === 'ArrowDown' ? 0 : buttons.length - 1
        : (current + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
      buttons[next].focus();
    };
    const close = () => onClose();
    window.addEventListener('pointerdown', closeFromPointer, true);
    window.addEventListener('keydown', closeFromKey);
    window.addEventListener('resize', close);
    window.addEventListener('scroll', close, true);
    return () => {
      window.removeEventListener('pointerdown', closeFromPointer, true);
      window.removeEventListener('keydown', closeFromKey);
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', close, true);
    };
  }, [onClose]);

  return createPortal(<div ref={menuRef} className="context-menu" role="menu" aria-label="操作菜单" style={{ left: placed.x, top: placed.y }}>
    {items.map((item, index) => item.kind === 'separator'
      ? <div className="context-menu-separator" role="separator" key={`separator-${index}`} />
      : <button
          type="button"
          role="menuitem"
          key={`${item.label}-${index}`}
          className={item.danger ? 'danger' : ''}
          disabled={item.disabled}
          onClick={() => { item.action(); previousFocusRef.current?.focus(); onClose(); }}
        >
          <span className="context-menu-check">{item.checked ? <Icon name="check" size={15} /> : item.icon ? <Icon name={item.icon} size={16} /> : null}</span>
          <span>{item.label}</span>
        </button>)}
  </div>, document.body);
}
