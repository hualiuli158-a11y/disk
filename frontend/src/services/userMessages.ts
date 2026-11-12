import { ApiError } from './api';

export type MessageContext = 'general' | 'login' | 'register';

export function userErrorMessage(cause: unknown, context: MessageContext = 'general'): string {
  if (cause instanceof ApiError) {
    if (context === 'login' && cause.status === 401) return '用户名或密码不正确，请重新输入。';
    if (context === 'register' && (cause.code === 20003 || cause.status === 409)) return '这个用户名已经被使用，请换一个。';
    if (cause.code === 10001 || cause.message === '入参错误') {
      if (context === 'register') return '注册信息有误，请检查用户名和密码要求。';
      if (context === 'login') return '请输入正确的用户名和密码。';
      return '提交的信息有误，请检查后重试。';
    }
    if (cause.status === 429) return '操作太频繁，请稍后再试。';
    if (cause.status >= 500) return '服务暂时不可用，请稍后重试。';
    return cause.message;
  }
  if (cause instanceof TypeError) return '网络连接失败，请检查网络后重试。';
  return '操作失败，请稍后重试。';
}
