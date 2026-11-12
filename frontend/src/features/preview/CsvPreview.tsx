import { useMemo } from 'react';
import { MAX_CSV_PREVIEW_ROWS, parseDelimitedText } from './csvParser';

export default function CsvPreview({ content, fileName }: { content: string; fileName: string }) {
  const delimiter: ',' | '\t' = fileName.toLowerCase().endsWith('.tsv') ? '\t' : ',';
  const table = useMemo(() => parseDelimitedText(content, delimiter), [content, delimiter]);
  const [header, ...body] = table.rows;

  if (!header) return <div className="table-preview-empty">空表格</div>;

  return <div className="table-preview-wrap">
    <table className="table-preview">
      <thead><tr>{header.map((cell, index) => <th key={index}>{cell || `第 ${index + 1} 列`}</th>)}</tr></thead>
      <tbody>{body.map((row, rowIndex) => <tr key={rowIndex}>{header.map((_, columnIndex) => <td key={columnIndex}>{row[columnIndex] || ''}</td>)}</tr>)}</tbody>
    </table>
    {table.truncated && <div className="table-preview-limit">仅显示前 {MAX_CSV_PREVIEW_ROWS} 行</div>}
  </div>;
}
