<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useRouter } from 'vue-router';
import { getMediaList, getMediaByStatus } from '@/api/library';
import request from '@/api/request';
import MediaRow from '@/components/MediaRow.vue';

const router = useRouter();
const continueWatching = ref<unknown[]>([]);
const wantToWatch = ref<unknown[]>([]);
const recentlyAdded = ref<unknown[]>([]);
const libraries = ref<any[]>([]);
const loading = ref(true);
const isAdmin = computed(() => localStorage.getItem('role') === 'admin');

interface MediaItem {
  id: string;
  library_id?: string;
  [key: string]: unknown;
}

function getLibraryName(item: unknown): string {
  const media = item as MediaItem;
  if (!media?.library_id) return '';
  return libraries.value.find((l: any) => l.id === media.library_id)?.name || '';
}

const showLibraryTags = computed(() => libraries.value.length >= 2);

function onStatusChanged(id: string, newStatus: string) {
  const update = (arr: unknown[]) => {
    const items = arr as any[];
    const item = items.find((m: any) => m.id === id);
    if (item) item.user_status = newStatus;
  };
  update(continueWatching.value);
  update(wantToWatch.value);
  update(recentlyAdded.value);
}

onMounted(async () => {
  try {
    const [continueRes, wantRes, recentRes, libRes] = await Promise.all([
      fetch('/api/v1/library/continue', {
        headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
      }).then((r) => (r.ok ? r.json() : { data: [] })),
      getMediaByStatus('want_to_watch', 20),
      getMediaList(1, 20, { sort: 'created_desc' }),
      request.get('/libraries'),
    ]);
    continueWatching.value = continueRes.data || [];
    wantToWatch.value = wantRes?.items || [];
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
      <MediaRow
        title="Continue Watching"
        :items="continueWatching"
        @status-changed="onStatusChanged"
      />
      <MediaRow
        title="🔖 Want to Watch"
        :items="wantToWatch"
        @status-changed="onStatusChanged"
      />
      <MediaRow
        title="Recently Added"
        :items="recentlyAdded"
        :get-library-name="showLibraryTags ? getLibraryName : undefined"
        @status-changed="onStatusChanged"
      />
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