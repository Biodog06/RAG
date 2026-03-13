<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useMessage } from 'naive-ui';
import { fetchChatTools } from '@/service/api/chat';

interface Tool {
  name: string;
  description: string;
  isGenerated: boolean;
}

const chatStore = useChatStore();
const { tools, loadingTools: loading } = storeToRefs(chatStore);

async function fetchTools() {
  await chatStore.fetchTools();
}

onMounted(() => {
  fetchTools();
});

defineExpose({ refresh: fetchTools });
</script>

<template>
  <div class="p-4">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-bold">可用工具列表</h2>
      <n-button type="primary" :loading="loading" @click="fetchTools">刷新</n-button>
    </div>
    
    <n-spin :show="loading">
      <div v-if="tools.length === 0" class="text-center py-10 text-gray-400">
        暂无可用工具
      </div>
      <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <n-card v-for="tool in tools" :key="tool.name" :title="tool.name" size="small" hoverable>
          <template #header-extra>
            <n-tag :type="tool.isGenerated ? 'info' : 'success'" size="small">
              {{ tool.isGenerated ? '动态生成' : '内置' }}
            </n-tag>
          </template>
          <p class="text-gray-600 mb-2">{{ tool.description }}</p>
          <div class="text-xs text-gray-400">
            ID: {{ tool.name }}
          </div>
        </n-card>
      </div>
    </n-spin>
  </div>
</template>

<style scoped></style>
