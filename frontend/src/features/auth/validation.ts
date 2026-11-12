const encoder = new TextEncoder();

export function registrationError(username: string, password: string): string | null {
  if (!username.trim()) return '请输入用户名。';
  if (encoder.encode(username.trim()).length > 100) return '用户名太长，请缩短后重试。';
  if (Array.from(password).length < 8) return '密码至少需要 8 个字符。';
  if (encoder.encode(password).length > 72) return '密码太长，请缩短后重试（最多 72 字节）。';
  if (!password.trim()) return '密码不能只包含空格。';
  return null;
}
