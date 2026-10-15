import type { FormEvent } from 'react';
import Icon from '../../components/Icon';

interface Props {
  mode: 'login' | 'register';
  username: string;
  password: string;
  busy: boolean;
  error: string;
  onUsernameChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onSwitchMode: () => void;
}

export default function AuthPage({ mode, username, password, busy, error, onUsernameChange, onPasswordChange, onSubmit, onSwitchMode }: Props) {
  return <div className="auth-page"><div className="auth-card">
    <div className="auth-brand"><span className="brand-mark"><Icon name="folder" size={21} /></span><span>栖盘</span></div>
    <h1>{mode === 'login' ? '欢迎回来' : '创建账号'}</h1>
    <form onSubmit={onSubmit}>
      <label className="sr-only" htmlFor="username">用户名</label><div className="auth-input"><Icon name="user" size={18} /><input id="username" autoComplete="username" required placeholder="用户名" value={username} onChange={event => onUsernameChange(event.target.value)} /></div>
      <label className="sr-only" htmlFor="password">密码</label><div className="auth-input"><Icon name="lock" size={18} /><input id="password" type="password" autoComplete={mode === 'login' ? 'current-password' : 'new-password'} aria-describedby={mode === 'register' ? 'password-help' : undefined} required placeholder="密码" value={password} onChange={event => onPasswordChange(event.target.value)} /></div>
      {mode === 'register' && <p id="password-help" className="auth-help">密码至少 8 个字符，最多 72 字节；无需特殊组合。</p>}
      {error && <p className="auth-error" role="alert">{error}</p>}
      <button type="submit" className="primary-button" disabled={busy}>{busy ? '请稍候…' : mode === 'login' ? '登录' : '注册并进入'}</button>
    </form>
    <button className="auth-switch" onClick={onSwitchMode}>{mode === 'login' ? '注册账号' : '返回登录'}</button>
  </div></div>;
}
