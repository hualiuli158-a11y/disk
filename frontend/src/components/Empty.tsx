import type { ReactNode } from 'react';
import Icon from './Icon';

export default function Empty({ icon, title, text, action }: { icon: string; title: string; text?: string; action?: ReactNode }) {
  return <div className="empty-state"><div className="empty-icon"><Icon name={icon} size={27} /></div><h3>{title}</h3>{text && <p>{text}</p>}{action}</div>;
}
