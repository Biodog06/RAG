import { useWebSocket } from '@vueuse/core';

export const useChatStore = defineStore(SetupStoreId.Chat, () => {
  const conversationId = ref<string>('');
  const input = ref<Api.Chat.Input>({ message: '' });

  const list = ref<Api.Chat.Message[]>([]);
  const tools = ref<any[]>([]);
  const loadingTools = ref(false);

  async function fetchTools() {
    loadingTools.value = true;
    const { data, error } = await fetchChatTools();
    if (!error) {
      tools.value = data || [];
    }
    loadingTools.value = false;
  }

  const store = useAuthStore();

  const {
    status: wsStatus,
    data: wsData,
    send: wsSend,
    open: wsOpen,
    close: wsClose
  } = useWebSocket(`/proxy-ws/chat/${store.token}`, {
    autoReconnect: true
  });

  const scrollToBottom = ref<null | (() => void)>(null);

  return {
    input,
    conversationId,
    list,
    tools,
    loadingTools,
    fetchTools,
    wsStatus,
    wsData,
    wsSend,
    wsOpen,
    wsClose,
    scrollToBottom
  };
});
