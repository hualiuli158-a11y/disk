import { useEffect, useRef, useState } from 'react';

interface Props {
  initialValue: string;
  className?: string;
  disabled?: boolean;
  selectStem?: boolean;
  onCommit: (value: string) => boolean | Promise<boolean>;
  onCancel: () => void;
}

export default function InlineNameInput({ initialValue, className = '', disabled = false, selectStem = false, onCommit, onCancel }: Props) {
  const [value, setValue] = useState(initialValue);
  const [pending, setPending] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const finishingRef = useRef(false);

  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    const dot = selectStem ? initialValue.lastIndexOf('.') : -1;
    if (dot > 0) input.setSelectionRange(0, dot);
    else input.select();
  }, [initialValue, selectStem]);

  const finish = async () => {
    if (finishingRef.current || disabled) return;
    finishingRef.current = true;
    setPending(true);
    let accepted = false;
    try { accepted = await onCommit(value); }
    catch { accepted = false; }
    if (!accepted) {
      finishingRef.current = false;
      setPending(false);
      window.setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 0);
    }
  };

  return <input
    ref={inputRef}
    className={className}
    value={value}
    disabled={disabled || pending}
    aria-label="名称"
    onChange={event => setValue(event.target.value)}
    onClick={event => event.stopPropagation()}
    onDoubleClick={event => event.stopPropagation()}
    onPointerDown={event => event.stopPropagation()}
    onBlur={() => void finish()}
    onKeyDown={event => {
      if (event.key === 'Enter') {
        event.preventDefault();
        event.stopPropagation();
        void finish();
      } else if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        finishingRef.current = true;
        onCancel();
      }
    }}
  />;
}
