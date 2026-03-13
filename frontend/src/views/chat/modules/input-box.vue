<script setup lang="ts">
const chatStore = useChatStore();
const { input, list, wsStatus, wsData } = storeToRefs(chatStore);

const latestMessage = computed(() => {
  return list.value[list.value.length - 1] ?? {};
});

const isSending = computed(() => {
  return (
    latestMessage.value?.role === 'assistant' && ['loading', 'pending'].includes(latestMessage.value?.status || '')
  );
});

const sendable = computed(
  () => (!input.value.message && !isSending) || ['CLOSED', 'CONNECTING'].includes(wsStatus.value)
);

watch(wsData, val => {
  if (!val) return;
  console.log('Received WS message:', val);
  try {
    const data = JSON.parse(val);
    const assistant = list.value[list.value.length - 1];
    if (!assistant || assistant.role !== 'assistant') {
      console.warn('No assistant message at the end of the list');
      return;
    }

    if (data.type === 'completion' && data.status === 'finished' && assistant.status !== 'error') {
      assistant.status = 'finished';
      console.log('--- Dialogue Finished: Triggering Tool List Refresh ---');
      chatStore.fetchTools();
    }
    if (data.error) {
      assistant.status = 'error';
      window.$message?.error(data.error);
    } else if (data.chunk) {
      assistant.status = 'loading';
      assistant.content += data.chunk;
    }
  } catch (err) {
    console.error('Failed to parse WS message:', err);
  }
});

const handleSend = async () => {
  console.log('handleSend called, input:', input.value.message);
  //  判断是否正在发送, 如果发送中，则停止ai继续响应
  if (isSending.value) {
    console.log('Currently sending, triggering stop');
    const { error, data } = await request<Api.Chat.Token>({ url: 'chat/websocket-token', baseURL: 'proxy-api' });
    if (error) return;

    chatStore.wsSend(JSON.stringify({ type: 'stop', _internal_cmd_token: data.cmdToken }));

    list.value[list.value.length - 1].status = 'finished';
    if (!latestMessage.value.content) list.value.pop();
    return;
  }

  if (!input.value.message) return;

  const newUserMsg = {
    content: input.value.message,
    role: 'user' as const,
    timestamp: new Date().toISOString()
  };
  console.log('Pushing user message to list:', newUserMsg);
  list.value.push(newUserMsg);
  
  console.log('Sending message via WS:', input.value.message);
  chatStore.wsSend(input.value.message);
  
  const newAssistantMsg = {
    content: '',
    role: 'assistant' as const,
    status: 'pending' as const,
    timestamp: new Date().toISOString()
  };
  console.log('Pushing assistant message to list:', newAssistantMsg);
  list.value.push(newAssistantMsg);
  
  input.value.message = '';
};

const inputRef = ref();
// 手动插入换行符（确保所有浏览器兼容）
const insertNewline = () => {
  const textarea = inputRef.value;
  const start = textarea.selectionStart;
  const end = textarea.selectionEnd;

  // 在光标位置插入换行符
  input.value.message = `${input.value.message.substring(0, start)}\n${input.value.message.substring(end)}`;

  // 更新光标位置（在插入的换行符之后）
  nextTick(() => {
    textarea.selectionStart = start + 1;
    textarea.selectionEnd = start + 1;
    textarea.focus(); // 确保保持焦点
  });
};

// ctrl + enter 换行
// enter 发送
const handShortcut = (e: KeyboardEvent) => {
  if (e.key === 'Enter') {
    e.preventDefault();

    if (!e.shiftKey && !e.ctrlKey) {
      handleSend();
    } else insertNewline();
  }
};
</script>

<template>
  <div class="relative w-full b-1 b-#1c1c1c20 bg-#fff p-4 card-wrapper dark:bg-#1c1c1c">
    <textarea
      ref="inputRef"
      v-model.trim="input.message"
      placeholder="给 派聪明 发送消息"
      class="min-h-10 w-full cursor-text resize-none b-none bg-transparent color-#333 caret-[rgb(var(--primary-color))] outline-none dark:color-#f1f1f1"
      @keydown="handShortcut"
    />
    <div class="flex items-center justify-between pt-2">
      <div class="flex items-center text-18px color-gray-500">
        <NText class="text-14px">连接状态：</NText>
        <icon-eos-icons:loading v-if="wsStatus === 'CONNECTING'" class="color-yellow" />
        <icon-fluent:plug-connected-checkmark-20-filled v-else-if="wsStatus === 'OPEN'" class="color-green" />
        <icon-tabler:plug-connected-x v-else class="color-red" />
      </div>
      <NButton :disabled="sendable" strong circle type="primary" @click="handleSend">
        <template #icon>
          <icon-material-symbols:stop-rounded v-if="isSending" />
          <icon-guidance:send v-else />
        </template>
      </NButton>
    </div>
  </div>
</template>

<style scoped></style>
