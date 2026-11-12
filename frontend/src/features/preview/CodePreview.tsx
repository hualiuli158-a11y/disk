import { useMemo, useState } from 'react';
import { codeLanguage, highlightCode } from './highlightCode';

export default function CodePreview({ content, extension }: { content: string; extension?: string }) {
  const [wrap, setWrap] = useState(false);
  const [copied, setCopied] = useState(false);
  const html = useMemo(() => highlightCode(content, extension || ''), [content, extension]);
  const lines = content.split('\n').length;
  return <div className="source-viewer">
    <div className="source-toolbar"><span>{codeLanguage(extension || 'text')}<i />{lines.toLocaleString()} 行</span><div>
      <button type="button" aria-pressed={wrap} onClick={() => setWrap(value => !value)}>自动换行</button>
      <button type="button" onClick={async () => { try { await navigator.clipboard.writeText(content); setCopied(true); } catch { setCopied(false); } }}>{copied ? '已复制' : '复制代码'}</button>
    </div></div>
    <div className={`source-scroll ${wrap ? 'wrap' : ''}`}>
      {lines <= 10000 && !wrap && <pre className="source-gutter" aria-hidden="true">{Array.from({ length: lines }, (_, index) => index + 1).join('\n')}</pre>}
      <pre className="source-code"><code dangerouslySetInnerHTML={{ __html: html }} /></pre>
    </div>
  </div>;
}
