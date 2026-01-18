<template>
  <div class="container mx-auto px-4 py-8">
    <h1 class="text-3xl font-bold text-blue-600 mb-4">
      {{ title }}
    </h1>

    <div v-if="showContent" class="bg-white shadow-md rounded-lg p-6">
      <p class="text-gray-700 mb-4">{{ description }}</p>

      <div class="flex gap-4 mt-4">
        <button
          @click="handlePrimary"
          :class="['bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded', { 'opacity-50': isLoading }]"
          :disabled="isLoading"
        >
          {{ primaryButtonText }}
        </button>

        <button
          @click="handleSecondary"
          class="bg-gray-500 hover:bg-gray-700 text-white font-bold py-2 px-4 rounded"
        >
          {{ secondaryButtonText }}
        </button>
      </div>

      <ul v-for="item in items" :key="item.id" class="list-disc list-inside mt-4">
        <li class="text-gray-600">{{ item.name }}</li>
      </ul>
    </div>

    <div v-else class="text-center text-gray-500">
      <p>No content to display</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';

interface Item {
  id: number;
  name: string;
}

const title = ref('Vue Component with Tailwind');
const description = ref('This is a sample Vue 3 component using Composition API and Tailwind CSS');
const showContent = ref(true);
const isLoading = ref(false);

const items = ref<Item[]>([
  { id: 1, name: 'First item' },
  { id: 2, name: 'Second item' },
  { id: 3, name: 'Third item' },
]);

const primaryButtonText = computed(() => isLoading.value ? 'Loading...' : 'Primary Action');
const secondaryButtonText = ref('Secondary Action');

const handlePrimary = () => {
  isLoading.value = true;
  setTimeout(() => {
    isLoading.value = false;
  }, 2000);
};

const handleSecondary = () => {
  console.log('Secondary button clicked');
};
</script>

<style scoped>
.container {
  max-width: 1200px;
}
</style>
