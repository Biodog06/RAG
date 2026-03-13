<script setup lang="ts">
import { NModal, NScrollbar } from 'naive-ui';
import { VueMarkdownItProvider } from 'vue-markdown-shiki';
import FilePreview from '@/components/custom/file-preview.vue';
import ChatMessage from './chat-message.vue';

defineOptions({
  name: 'ChatList'
});

const chatStore = useChatStore();
const { list, previewVisible, previewFileName } = storeToRefs(chatStore);

const loading = ref(false);
const scrollbarRef = ref<InstanceType<typeof NScrollbar>>();

watch(() => [...list.value], scrollToBottom);

function scrollToBottom() {
  setTimeout(() => {
    scrollbarRef.value?.scrollBy({
      top: 999999999999999,
      behavior: 'auto'
    });
  }, 100);
}

const range = ref<[number, number]>([dayjs().subtract(7, 'day').valueOf(), dayjs().add(1, 'day').valueOf()]);

const params = computed(() => {
  return {
    start_date: dayjs(range.value[0]).format('YYYY-MM-DD'),
    end_date: dayjs(range.value[1]).format('YYYY-MM-DD')
  };
});

watch(params, () => {
  getList();
}, { immediate: true });

async function getList() {
  loading.value = true;
  const { error, data } = await request<Api.Chat.Message[]>({
    url: 'users/conversation',
    params: params.value
  });
  if (!error) {
    list.value = data;
  }
  loading.value = false;
}

onMounted(() => {
  chatStore.scrollToBottom = scrollToBottom;
});
</script>

<template>
  <div class="flex-1 flex-col h-full overflow-hidden">
    <NScrollbar ref="scrollbarRef" class="flex-1">
      <Teleport defer to="#header-extra">
        <div class="px-10">
          <NForm :model="params" label-placement="left" :show-feedback="false" inline>
            <NFormItem label="时间">
              <NDatePicker v-model:value="range" type="daterange" />
            </NFormItem>
          </NForm>
        </div>
      </Teleport>
      <NSpin :show="loading">
        <VueMarkdownItProvider>
          <ChatMessage v-for="(item, index) in list" :key="index" :msg="item" />
        </VueMarkdownItProvider>
      </NSpin>
    </NScrollbar>
    <!-- 文件预览弹窗 -->
    <NModal v-model:show="previewVisible" preset="card" title="文件预览" style="width: 80%; max-width: 1000px;">
      <FilePreview
        :file-name="previewFileName"
        :visible="previewVisible"
        @close="previewVisible = false"
      />
    </NModal>
  </div>
</template>

<style scoped lang="scss"></style>
