<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { getMediaList } from '@/api/library';
import request from '@/api/request';
import MediaRow from '@/components/MediaRow.vue';

const continueWatching = ref<unknown[]>([]);
const recentlyAdded = ref<unknown[]>([]);
const libraries = ref<any[]>([]);

onMounted(async () => {
  try {
    const [continueRes, recentRes, libRes] = await Promise.all([
      fetch('/api/v1/library/continue', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      }).then((r) => (r.ok ? r.json() : { data: [] })),
      getMediaList(1, 20, { sort: 'created_desc' }),
      request.get('/libraries'),
    ]);
    continueWatching.value = continueRes.data || [];
    recentlyAdded.value = recentRes.items || [];
    libraries.value = (libRes as any).data || [];
  } catch {
    // silently ignore dashboard load errors
  }
});
</script>

<template>
  <div class="dashboard">
    <MediaRow title="Continue Watching" :items="continueWatching" />
    <MediaRow title="Recently Added" :items="recentlyAdded" />
  </div>
</template>

<style scoped>
.dashboard {
  padding: 0 24px 40px;
}
</style>
