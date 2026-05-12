<script setup lang="ts">
import ChatList from './modules/chat-list.vue';
import InputBox from './modules/input-box.vue';
</script>

<template>
  <div class="flex flex-col h-full gap-4 overflow-hidden p-4">
    <ChatList class="flex-1 overflow-hidden" />
    <InputBox class="flex-none" />
  </div>
</template>

<style scoped></style>
