<template>
  <div class="dashboard">
    <div v-if="loading" class="dashboard-state">
      <p>Loading dashboard...</p>
    </div>

    <div v-else-if="error" class="dashboard-state error-state">
      <p>{{ error }}</p>
    </div>

    <div class="no-libraries" v-else-if="libraries.length === 0">
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

      <MediaRow title="Want to Watch" :items="wantToWatch" @status-changed="onStatusChanged" />

      <MediaRow
        title="Recently Added"
        :items="recentlyAdded"
        :get-library-name="showLibraryTags ? getLibraryName : undefined"
        @status-changed="onStatus - changed"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { getMediaList, getMediaByStatus, getContinueWatching } from '@/api/library';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';
import { useUserStore } from '@/stores/user';
import MediaRow from '@/components/MediaRow.vue';

interface Library {
  id: string;
  name: string;
}

interface MediaItem {
  id: string;
  title?: string;
  library_id?: string;
  user_status?: string;
  [key: string]: unknown;
}

interface MediaListResponse {
  items: MediaItem[];
  total?: number;
}

const userStore = useUserStore();

const continueWatching = ref<MediaItem[]>([]);
const wantToWatch = ref<MediaItem[]>([]);
const recentlyAdded = ref<MediaItem[]>([]);
const libraries = ref<Library[]>([]);

const loading = ref(true);
const error = ref('');

const isAdmin = computed(() => userStore.isAdmin);
const showLibraryTags = computed(() => libraries.value.length >= 2);

function getLibraryName(item: MediaItem): string {
  if (!item?.library_id) return '';
  return libraries.value.find((library) => library.id === item.library_id)?.name || '';
}

function updateStatusInArray(items: MediaItem[], id: string, newStatus: string): MediaItem[] {
  return items.map((item) =>
    item.id === id
      ? {
          ...item,
          user_status: newStatus,
        }
      : item
  );
}

function onStatusChanged(id: string, newStatus: string): void {
  continueWatching.value = updateStatusInArray(continueWatching.value, id, newStatus);
  wantToWatch.value = updateStatusInArray(wantToWatch.value, id, newStatus);
  recentlyAdded.value = updateStatusInArray(recentlyAdded.value, id, newStatus);
}

async function loadLibraries(): Promise<Library[]> {
  const res = await authRequest.get<ApiEnvelope<Library[]>>('/libraries');
  return res.data.data || [];
}

async function loadDashboard(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const [continueRes, wantRes, recentRes, libraryList] = await Promise.all([
      getContinueWatching(),
      getMediaByStatus('want_to_watch', 20),
      getMediaList(1, 20, { sort: 'created_desc' }),
      loadLibraries(),
    ]);

    continueWatching.value = ((continueRes as MediaListResponse | undefined)?.items ||
      []) as MediaItem[];

    wantToWatch.value = ((wantRes as MediaListResponse | undefined)?.items || []) as MediaItem[];

    recentlyAdded.value = ((recentRes as MediaListResponse | undefined)?.items ||
      []) as MediaItem[];

    libraries.value = libraryList;
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[dashboard] loadDashboard failed:', err);
    error.value = 'Failed to load dashboard';
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void loadDashboard();
});
</script>

<style scoped>
.dashboard {
  padding: 0 24px 40px;
}

.dashboard-state {
  text-align: center;
  padding: 80px 20px;
  color: #555577;
}

.error-state {
  color: #ff6b6b;
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
