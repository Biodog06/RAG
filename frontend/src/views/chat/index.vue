<script setup lang="ts">
import { ref } from 'vue';
import { NCard, NTabs, NTabPane } from 'naive-ui';
import ChatList from './modules/chat-list.vue';
import InputBox from './modules/input-box.vue';
import ToolList from './modules/tool-list.vue';

const activeTab = ref('chat');
const toolListRef = ref<any>(null);

function handleTabChange(value: string) {
  activeTab.value = value;
  if (value === 'tools' && toolListRef.value) {
    toolListRef.value.refresh();
  }
}
</script>

<template>
  <n-card :bordered="false" class="h-full">
    <n-tabs v-model:value="activeTab" type="segment" animated @update:value="handleTabChange">
      <n-tab-pane name="chat" tab="AI 聊天">
        <div class="flex-col gap-4 mt-4">
          <ChatList />
          <InputBox />
        </div>
      </n-tab-pane>
      <n-tab-pane name="tools" tab="工具管理">
        <ToolList ref="toolListRef" />
      </n-tab-pane>
    </n-tabs>
  </n-card>
</template>

<style scoped></style>
