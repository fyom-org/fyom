<template>
  <div class="dashboard">
    <div v-if="loading" class="dashboard-skeleton" aria-busy="true" aria-live="polite">
      <div v-for="row in 3" :key="row" class="skeleton-row">
        <div class="skeleton-title"></div>
        <div class="skeleton-cards">
          <div
            v-for="n in 6"
            :key="n"
            class="skeleton-card"
            :style="{ animationDelay: `${(row - 1) * 80 + n * 60}ms` }"
          >
            <div class="skeleton-poster"></div>
            <div class="skeleton-line"></div>
            <div class="skeleton-line short"></div>
          </div>
        </div>
      </div>
      <span class="sr-only">{{ $t('dashboard.loading') }}</span>
    </div>

    <div v-else-if="error" class="dashboard-state error-state">
      <p>{{ error }}</p>
    </div>

    <div v-else-if="libraries.length === 0" class="no-libraries">
      <p>{{ $t('dashboard.noLibraries') }}</p>
      <router-link v-if="isAdmin" to="/admin/libraries" class="setup-link">
        {{ $t('dashboard.createFirst') }}
      </router-link>
    </div>

    <template v-else>
      <TransitionGroup name="row-fade" tag="div">
        <MediaRow
          key="continue"
          :title="$t('dashboard.continueWatching')"
          :items="continueWatching"
          @status-changed="onStatusChanged"
        />

        <MediaRow key="want" :title="$t('dashboard.wantToWatch')" :items="wantToWatch" @status-changed="onStatusChanged" />

        <MediaRow
          key="recent"
          :title="$t('dashboard.recentlyAdded')"
          :items="recentlyAdded"
          :get-library-name="showLibraryTags ? getLibraryName : undefined"
          @status-changed="onStatusChanged"
        />
      </TransitionGroup>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
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

const { t } = useI18n();
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
  } catch (err: unknown) {
    const status =
      err && typeof err === 'object' && 'response' in err
        ? (err as { response?: { status?: number } }).response?.status
        : undefined;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[dashboard] loadDashboard failed:', err);
    error.value = t('dashboard.loadFailed');
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

/* Phase 11: skeleton loading state — three placeholder rows that mimic
   the real MediaRow layout (title + horizontal card scroll). The shimmer
   animation is staggered via inline animationDelay so cards fade in
   sequentially rather than all at once. */
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.skeleton-row {
  margin-bottom: 32px;
}

.skeleton-title {
  width: 180px;
  height: 20px;
  margin: 0 0 12px 4px;
  border-radius: 4px;
  background: linear-gradient(
    90deg,
    rgba(42, 42, 62, 0.6) 0%,
    rgba(60, 60, 88, 0.9) 50%,
    rgba(42, 42, 62, 0.6) 100%
  );
  background-size: 200% 100%;
  animation: shimmer 1.6s ease-in-out infinite;
}

.skeleton-cards {
  display: flex;
  gap: 16px;
  overflow: hidden;
}

.skeleton-card {
  flex-shrink: 0;
  width: 160px;
  opacity: 0;
  animation: skeleton-fade-in 0.5s ease forwards;
}

.skeleton-poster {
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  background: linear-gradient(
    90deg,
    rgba(42, 42, 62, 0.6) 0%,
    rgba(60, 60, 88, 0.9) 50%,
    rgba(42, 42, 62, 0.6) 100%
  );
  background-size: 200% 100%;
  animation: shimmer 1.6s ease-in-out infinite;
}

.skeleton-line {
  height: 10px;
  margin-top: 8px;
  border-radius: 3px;
  background: rgba(42, 42, 62, 0.7);
  width: 100%;
}

.skeleton-line.short {
  width: 60%;
}

@keyframes shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

@keyframes skeleton-fade-in {
  to {
    opacity: 1;
  }
}

/* Phase 11: row entrance transition — each MediaRow fades and slides up
   slightly when the dashboard content resolves. Respects reduced-motion. */
.row-fade-enter-active {
  transition: opacity 0.35s ease, transform 0.35s ease;
}

.row-fade-enter-from {
  opacity: 0;
  transform: translateY(12px);
}

@media (prefers-reduced-motion: reduce) {
  .skeleton-poster,
  .skeleton-title {
    animation: none;
    background: rgba(42, 42, 62, 0.7);
  }

  .skeleton-card {
    animation: none;
    opacity: 1;
  }

  .row-fade-enter-active {
    transition: none;
  }

  .row-fade-enter-from {
    transform: none;
  }
}
</style>
