import Icon from './Icon';

export default function Button({ icon, label, onClick, disabled = false }: { icon: string; label: string; onClick: () => void; disabled?: boolean }) {
  return <button className="icon-button" type="button" title={label} aria-label={label} onClick={onClick} disabled={disabled}><Icon name={icon} size={17} /></button>;
}
