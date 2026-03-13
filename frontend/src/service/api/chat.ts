import { request } from '../request';

/**
 * 获取可用工具列表
 */
export function fetchChatTools() {
  return request<any[]>({
    url: '/chat/tools',
    method: 'get'
  });
}
