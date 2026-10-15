import hljs from 'highlight.js/lib/common';
import powershell from 'highlight.js/lib/languages/powershell';
hljs.registerLanguage('powershell', powershell);

const aliases: Record<string, string> = { tsx: 'typescript', jsx: 'javascript', yml: 'yaml', sh: 'bash', zsh: 'bash', md: 'markdown', h: 'c', hpp: 'cpp', cs: 'csharp', rs: 'rust', py: 'python', kt: 'kotlin', ps1: 'powershell', psm1: 'powershell', vue: 'xml', svelte: 'xml' };

export function codeLanguage(language: string): string {
  return aliases[language.toLowerCase()] || language.toLowerCase();
}

export function escapeCode(source: string): string {
  return source.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function highlightCode(source: string, language: string): string {
  const normalized = codeLanguage(language);
  // Do not run regex-based highlighting on multi-megabyte files.
  if (source.length > 200_000 || !hljs.getLanguage(normalized)) return escapeCode(source);
  try { return hljs.highlight(source, { language: normalized, ignoreIllegals: true }).value; }
  catch { return escapeCode(source); }
}
