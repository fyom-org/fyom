<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useRouter } from 'vue-router';
import { getMediaList } from '@/api/library';
import request from '@/api/request';
import MediaRow from '@/components/MediaRow.vue';

const router = useRouter();
const continueWatching = ref<unknown[]>([]);
const recentlyAdded = ref<unknown[]>([]);
const libraries = ref<any[]>([]);
const loading = ref(true);
const isAdmin = computed(() => localStorage.getItem('role') === 'admin');

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
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="dashboard">
    <div class="no-libraries" v-if="!loading && libraries.length === 0">
      <p>No libraries yet.</p>
      <router-link to="/admin/libraries" class="setup-link" v-if="isAdmin">
        Create your first library →
      </router-link>
    </div>
    <template v-else>
      <MediaRow title="Continue Watching" :items="continueWatching" />
      <MediaRow title="Recently Added" :items="recentlyAdded" />
    </template>
  </div>
</template>

<style scoped>
.dashboard {
  padding: 0 24px 40px;
}

.no-libraries {
  text-align: center;
  padding: 80px 20px;
  color: #555577;
}

.no-libraries p {
  font-size: 16px;
  margin: 0 0 16px;
}

.setup-link {
  color: #6c63ff;
  text-decoration: none;
  font-size: 14px;
}

.setup-link:hover {
  color: #8b83ff;
}
</style>
