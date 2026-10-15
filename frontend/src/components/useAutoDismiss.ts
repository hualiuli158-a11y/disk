import { useEffect, useRef, useState } from 'react';
import type { FocusEvent } from 'react';

export default function useAutoDismiss(enabled: boolean, resetKey: unknown, onDismiss: () => void, delay = 5000) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const latest = useRef({ enabled, resetKey, onDismiss });
  latest.current = { enabled, resetKey, onDismiss };

  useEffect(() => {
    if (!enabled) { setHovered(false); setFocused(false); }
  }, [enabled]);

  useEffect(() => {
    if (!enabled || hovered || focused) return;
    const timer = window.setTimeout(() => {
      if (latest.current.enabled && latest.current.resetKey === resetKey) latest.current.onDismiss();
    }, delay);
    return () => window.clearTimeout(timer);
  }, [enabled, resetKey, delay, hovered, focused]);

  return {
    onMouseEnter: () => setHovered(true),
    onMouseLeave: () => setHovered(false),
    onFocusCapture: () => setFocused(true),
    onBlurCapture: (event: FocusEvent<HTMLElement>) => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setFocused(false);
    },
  };
}
