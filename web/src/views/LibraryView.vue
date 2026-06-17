<template>
  <div class="library-view">
    <router-link v-if="currentLibraryId && showBackLink" to="/library" class="back-link">
      {{ $t('library.backToAll') }}
    </router-link>

    <header class="library-header">
      <div>
        <h2 class="page-title">
          {{ pageTitle }}
        </h2>
        <p v-if="pageSubtitle" class="page-subtitle">
          {{ pageSubtitle }}
        </p>
      </div>
    </header>

    <section class="toolbar" aria-label="Library filters">
      <div class="search-wrap">
        <input
          v-model.trim="searchQuery"
          type="search"
          :placeholder="$t('library.searchPlaceholder')"
          class="search-input"
          autocomplete="off"
          :aria-label="$t('library.searchAriaLabel')"
          @input="onSearchInput"
        />
      </div>

      <select
        v-if="!activeStatus"
        v-model="activeType"
        class="mobile-filter-select"
        aria-label="Filter by media type"
        @change="resetAndFetch"
      >
        <option :value="TYPE_ALL">{{ $t('library.all') }}</option>
        <option :value="TYPE_MOVIE">{{ $t('library.movies') }}</option>
        <option :value="TYPE_SHOW">{{ $t('library.shows') }}</option>
      </select>

      <div v-if="!activeStatus" class="filter-group desktop-only" aria-label="Media type">
        <button
          type="button"
          :class="['filter-btn', { active: activeType === TYPE_ALL }]"
          @click="setType(TYPE_ALL)"
        >
          {{ $t('library.all') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', { active: activeType === TYPE_MOVIE }]"
          @click="setType(TYPE_MOVIE)"
        >
          {{ $t('library.movies') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', { active: activeType === TYPE_SHOW }]"
          @click="setType(TYPE_SHOW)"
        >
          {{ $t('library.shows') }}
        </button>
      </div>

      <div class="filter-group status-filter-group" aria-label="Watch status">
        <button
          type="button"
          :class="['filter-btn', { active: activeStatus === STATUS_ALL }]"
          @click="setStatus(STATUS_ALL)"
        >
          {{ $t('library.all') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', 'status-watching', { active: activeStatus === STATUS_WATCHING }]"
          @click="setStatus(STATUS_WATCHING)"
        >
          {{ $t('library.watching') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', 'status-want', { active: activeStatus === STATUS_WANT }]"
          @click="setStatus(STATUS_WANT)"
        >
          {{ $t('library.want') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', 'status-watched', { active: activeStatus === STATUS_WATCHED }]"
          @click="setStatus(STATUS_WATCHED)"
        >
          {{ $t('library.watched') }}
        </button>
        <button
          type="button"
          :class="['filter-btn', 'status-dropped', { active: activeStatus === STATUS_DROPPED }]"
          @click="setStatus(STATUS_DROPPED)"
        >
          {{ $t('library.dropped') }}
        </button>
      </div>

      <select
        v-if="!activeStatus"
        v-model="activeSort"
        class="sort-select"
        aria-label="Sort library"
        @change="resetAndFetch"
      >
        <option value="title_asc">{{ $t('library.titleAZ') }}</option>
        <option value="title_desc">Title Z-A</option>
        <option value="year_desc">{{ $t('library.newestFirst') }}</option>
        <option value="year_asc">Oldest First</option>
        <option value="rating_desc">{{ $t('library.topRated') }}</option>
        <option value="created_desc">{{ $t('library.recentlyAdded') }}</option>
      </select>
    </section>

    <section
      v-if="availableGenres.length > 0 && !activeStatus"
      class="genre-filter"
      aria-label="Genre filter"
    >
      <button
        type="button"
        :class="['genre-tag-btn', { active: !selectedGenre }]"
        @click="filterByGenre('')"
      >
        {{ $t('library.allGenres') }}
      </button>
      <button
        v-for="genre in availableGenres"
        :key="genre"
        type="button"
        :class="['genre-tag-btn', { active: selectedGenre === genre }]"
        @click="filterByGenre(genre)"
      >
        {{ genre }}
      </button>
    </section>

    <div class="meta-row">
      <div v-if="!loading && displayItems.length > 0" class="result-info">
        {{ resultLabel }}
      </div>

      <button v-if="hasActiveFilters" type="button" class="clear-filters-btn" @click="clearFilters">
        {{ $t('library.clearFilters') }}
      </button>
    </div>

    <div v-if="error" class="error-banner" role="alert">
      <span>{{ error }}</span>
      <button type="button" class="retry-btn" @click="retryFetch">{{ $t('library.retry') }}</button>
    </div>

    <div v-if="displayItems.length > 0" class="grid">
      <MediaCard
        v-for="media in displayItems"
        :key="media.id"
        :item="media"
        @status-changed="onStatusChanged"
      />
    </div>

    <div v-else-if="loading && !hasFetched" class="loading-state">{{ $t('library.loadingLibrary') }}</div>

    <div
      v-else-if="displayItems.length === 0 && !loading && hasFetched && !error"
      class="empty-state"
    >
      <p v-if="searchQuery">{{ $t('library.noResultsFor', { query: searchQuery }) }}</p>
      <p v-else-if="selectedGenre">No items found in "{{ selectedGenre }}".</p>
      <p v-else-if="activeStatus">No items with this status yet.</p>
      <p v-else>{{ $t('library.emptyLibrary') }}</p>
    </div>

    <div v-if="canLoadMore" class="load-more-wrap">
      <button type="button" class="load-more-btn" :disabled="loading" @click="fetchPage">
        {{ loading ? $t('common.loading') : $t('common.loadMore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { getMediaByStatus, getMediaList } from '@/api/library';
import { apiRequest } from '@/api/request';
import { getSafeApiErrorMessage, isRecord } from '@/lib/api/errors';
import { useLocaleFormat } from '@/composables/useLocaleFormat';
import MediaCard from '@/components/MediaCard.vue';

const { t } = useI18n();
const { formatNumber } = useLocaleFormat();

interface MediaItem {
  id: string;
  title: string;
  year?: number;
  poster_url?: string;
  user_status?: string;
  genres?: string[];
  [key: string]: unknown;
}

interface LibraryItem {
  id: string;
  name: string;
}

interface MediaListResponse {
  items?: MediaItem[];
  total?: number;
}

const PAGE_SIZE = 20;

const TYPE_ALL = 'movie,show';
const TYPE_MOVIE = 'movie';
const TYPE_SHOW = 'show';

const STATUS_ALL = '';
const STATUS_WATCHING = 'watching';
const STATUS_WANT = 'want_to_watch';
const STATUS_WATCHED = 'watched';
const STATUS_DROPPED = 'dropped';

const route = useRoute();

const currentLibraryId = computed(() => {
  return String(route.params.libraryId || route.query.library_id || '');
});

const libraryNameMap = ref<Record<string, string>>({});
const librariesCount = ref(0);

const currentLibraryName = computed(() => {
  if (!currentLibraryId.value) return 'Library';
  return libraryNameMap.value[currentLibraryId.value] || 'Library';
});

const showBackLink = computed(() => librariesCount.value >= 2);

const pageTitle = computed(() => {
  if (searchQuery.value) return t('library.searchResults');
  if (activeStatus.value) return statusTitle.value;
  return currentLibraryName.value;
});

const pageSubtitle = computed(() => {
  if (searchQuery.value && currentLibraryId.value) {
    return t('library.searchingIn', { libraryName: currentLibraryName.value });
  }

  if (activeStatus.value && currentLibraryId.value) {
    return currentLibraryName.value;
  }

  return '';
});

const statusTitle = computed(() => {
  switch (activeStatus.value) {
    case STATUS_WATCHING:
      return t('library.watching');
    case STATUS_WANT:
      return t('library.wantToWatch');
    case STATUS_WATCHED:
      return t('library.watched');
    case STATUS_DROPPED:
      return t('library.dropped');
    default:
      return currentLibraryName.value;
  }
});

const items = ref<MediaItem[]>([]);
const page = ref(1);
const total = ref(0);
const loading = ref(false);
const hasFetched = ref(false);
const allLoaded = ref(false);
const error = ref('');

const searchQuery = ref('');
const activeType = ref(TYPE_ALL);
const activeSort = ref('title_asc');
const activeStatus = ref(STATUS_ALL);
const selectedGenre = ref('');

let searchTimer: number | undefined;
let fetchGeneration = 0;

const availableGenres = computed(() => {
  const genres = new Set<string>();

  for (const item of items.value) {
    if (!Array.isArray(item.genres)) continue;

    for (const genre of item.genres) {
      if (typeof genre === 'string' && genre.trim()) {
        genres.add(genre.trim());
      }
    }
  }

  return Array.from(genres).sort((a, b) => a.localeCompare(b));
});

const displayItems = computed(() => {
  if (!selectedGenre.value) return items.value;

  return items.value.filter((item) => {
    return Array.isArray(item.genres) && item.genres.includes(selectedGenre.value);
  });
});

const resultLabel = computed(() => {
  const count =
    activeStatus.value || selectedGenre.value
      ? displayItems.value.length
      : total.value || displayItems.value.length;

  // Phase 11: manual plural selection with locale-aware number formatting.
  // We avoid vue-i18n's plural pipe syntax here because it auto-fills {n}
  // with the RAW count, overriding any formatted value we pass. By selecting
  // the key manually and using named-params interpolation, {n} receives the
  // formatNumber() output (e.g. "1,234" instead of "1234").
  // en: 0 → "no items", 1 → "1 item", N → "N items"
  // zh: 0 → "无项目", 1 → "1 项", N → "N 项" (no plural distinction)
  // ja: 0 → "アイテムなし", 1 → "1 件", N → "N 件" (no plural distinction)
  const n = formatNumber(count);
  if (count === 0) return t('library.resultCountZero');
  if (count === 1) return t('library.resultCountOne', { n });
  return t('library.resultCountMany', { n });
});

const hasActiveFilters = computed(() => {
  return Boolean(
    searchQuery.value ||
    selectedGenre.value ||
    activeStatus.value ||
    activeType.value !== TYPE_ALL ||
    activeSort.value !== 'title_asc'
  );
});

const canLoadMore = computed(() => {
  return !activeStatus.value && !allLoaded.value && items.value.length > 0;
});

onMounted(async () => {
  await loadLibraries();
  resetAndFetch();
});

onBeforeUnmount(() => {
  if (searchTimer) {
    window.clearTimeout(searchTimer);
  }

  fetchGeneration++;
});

watch(currentLibraryId, async () => {
  selectedGenre.value = '';
  await loadLibraries();
  resetAndFetch();
});

function onSearchInput() {
  if (searchTimer) {
    window.clearTimeout(searchTimer);
  }

  searchTimer = window.setTimeout(() => {
    selectedGenre.value = '';
    resetAndFetch();
  }, 300);
}

function setType(type: string) {
  if (activeType.value === type) return;

  activeType.value = type;
  selectedGenre.value = '';
  resetAndFetch();
}

function setStatus(status: string) {
  if (activeStatus.value === status) return;

  activeStatus.value = status;
  selectedGenre.value = '';
  resetAndFetch();
}

function filterByGenre(genre: string) {
  selectedGenre.value = genre;
}

function clearFilters() {
  searchQuery.value = '';
  activeType.value = TYPE_ALL;
  activeSort.value = 'title_asc';
  activeStatus.value = STATUS_ALL;
  selectedGenre.value = '';
  resetAndFetch();
}

function retryFetch() {
  error.value = '';
  resetAndFetch();
}

function resetAndFetch() {
  fetchGeneration++;
  items.value = [];
  page.value = 1;
  total.value = 0;
  allLoaded.value = false;
  hasFetched.value = false;
  fetchPage();
}

async function fetchPage() {
  if (loading.value || allLoaded.value) return;

  const generation = ++fetchGeneration;

  loading.value = true;
  error.value = '';

  try {
    const data = activeStatus.value ? await fetchStatusItems() : await fetchLibraryItems();

    if (generation !== fetchGeneration) return;

    const nextItems = Array.isArray(data.items) ? data.items : [];
    const nextTotal = Number.isFinite(data.total) ? Number(data.total) : 0;

    if (activeStatus.value) {
      items.value = nextItems;
      total.value = nextTotal || nextItems.length;
      allLoaded.value = true;
      return;
    }

    if (nextItems.length === 0) {
      allLoaded.value = true;
      total.value = nextTotal;
      return;
    }

    items.value = mergeUniqueItems(items.value, nextItems);
    total.value = nextTotal || items.value.length;

    if (items.value.length >= total.value) {
      allLoaded.value = true;
    } else {
      page.value += 1;
    }
  } catch (unknownError) {
    if (generation !== fetchGeneration) return;

    error.value = getSafeApiErrorMessage(unknownError, 'library.loadFailed');
    allLoaded.value = true;
  } finally {
    if (generation === fetchGeneration) {
      loading.value = false;
      hasFetched.value = true;
    }
  }
}

async function fetchLibraryItems(): Promise<MediaListResponse> {
  const params: Record<string, string | undefined> = {
    type: activeType.value,
    q: searchQuery.value || undefined,
    sort: activeSort.value,
    library_id: currentLibraryId.value || undefined,
  };

  return await getMediaList(page.value, PAGE_SIZE, params);
}

async function fetchStatusItems(): Promise<MediaListResponse> {
  const response = await getMediaByStatus(activeStatus.value, PAGE_SIZE);

  if (Array.isArray(response)) {
    return {
      items: response,
      total: response.length,
    };
  }

  return (
    response || {
      items: [],
      total: 0,
    }
  );
}

async function loadLibraries() {
  try {
    const response = (await apiRequest('/libraries')) as unknown;
    const libraries = normalizeLibrariesResponse(response);

    const nextMap: Record<string, string> = {};

    for (const library of libraries) {
      if (library.id && library.name) {
        nextMap[library.id] = library.name;
      }
    }

    libraryNameMap.value = nextMap;
    librariesCount.value = libraries.length;
  } catch {
    libraryNameMap.value = {};
    librariesCount.value = 0;
  }
}

function normalizeLibrariesResponse(response: unknown): LibraryItem[] {
  if (Array.isArray(response)) {
    return response.filter(isLibraryItem);
  }

  if (isRecord(response) && Array.isArray(response.data)) {
    return response.data.filter(isLibraryItem);
  }

  if (isRecord(response) && Array.isArray(response.items)) {
    return response.items.filter(isLibraryItem);
  }

  return [];
}

function isLibraryItem(value: unknown): value is LibraryItem {
  return isRecord(value) && typeof value.id === 'string' && typeof value.name === 'string';
}

function mergeUniqueItems(currentItems: MediaItem[], nextItems: MediaItem[]) {
  const seen = new Set(currentItems.map((item) => item.id));
  const merged = [...currentItems];

  for (const item of nextItems) {
    if (!item.id || seen.has(item.id)) continue;

    seen.add(item.id);
    merged.push(item);
  }

  return merged;
}

function onStatusChanged(id: string, newStatus: string) {
  const item = items.value.find((media) => media.id === id);

  if (!item) return;

  item.user_status = newStatus;

  if (activeStatus.value && newStatus !== activeStatus.value) {
    items.value = items.value.filter((media) => media.id !== id);
    total.value = Math.max(0, total.value - 1);
  }
}
</script>

<style scoped>
.library-view {
  width: 100%;
}

.library-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.page-title {
  margin: 0;
  color: var(--color-text);
  font-size: clamp(1.25rem, 3vw, 1.6rem);
  font-weight: 700;
  line-height: 1.2;
}

.page-subtitle {
  margin: 0.35rem 0 0;
  color: #777799;
  font-size: var(--font-size-sm);
}

.back-link {
  display: inline-block;
  margin-bottom: var(--spacing-sm);
  color: #777799;
  font-size: var(--font-size-sm);
  text-decoration: none;
  transition: color 0.15s ease;
}

.back-link:hover {
  color: #aaaacc;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.search-wrap {
  flex: 1 1 220px;
  min-width: 180px;
}

.search-input {
  width: 100%;
  box-sizing: border-box;
  padding: var(--spacing-sm) var(--spacing-md);
  color: var(--color-text);
  background: #0f0f1a;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  outline: none;
  font-size: var(--font-size-md);
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease;
}

.search-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgb(108 99 255 / 16%);
}

.search-input::placeholder {
  color: #555577;
}

.filter-group {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.status-filter-group {
  max-width: 100%;
}

.filter-btn {
  min-width: var(--touch-target);
  padding: var(--spacing-sm) var(--spacing-md);
  color: #8888aa;
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  cursor: pointer;
  font-size: var(--font-size-sm);
  white-space: nowrap;
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease,
    transform 0.15s ease;
}

.filter-btn:hover {
  color: #ccccee;
  border-color: #3a3a5e;
}

.filter-btn:active {
  transform: translateY(1px);
}

.filter-btn.active {
  color: #fff;
  background: var(--color-primary);
  border-color: var(--color-primary);
}

.filter-btn.active.status-watching {
  background: #6c63ff;
  border-color: #6c63ff;
}

.filter-btn.active.status-want {
  background: #2196f3;
  border-color: #2196f3;
}

.filter-btn.active.status-watched {
  background: #4caf50;
  border-color: #4caf50;
}

.filter-btn.active.status-dropped {
  background: #ff6b6b;
  border-color: #ff6b6b;
}

.sort-select,
.mobile-filter-select {
  min-width: 140px;
  padding: var(--spacing-sm) var(--spacing-md);
  color: #aaaacc;
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  outline: none;
  cursor: pointer;
  font-size: var(--font-size-sm);
}

.sort-select:focus,
.mobile-filter-select:focus {
  border-color: var(--color-primary);
}

.sort-select option,
.mobile-filter-select option {
  color: var(--color-text);
  background: #1a1a2e;
}

.mobile-filter-select {
  display: none;
  width: 100%;
}

.genre-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: var(--spacing-sm);
}

.genre-tag-btn {
  padding: 4px 10px;
  color: #8888aa;
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  cursor: pointer;
  font-size: var(--font-size-sm);
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease;
}

.genre-tag-btn:hover {
  color: #ccccee;
  border-color: #3a3a5e;
}

.genre-tag-btn.active {
  color: #fff;
  background: var(--color-primary);
  border-color: var(--color-primary);
}

.meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  min-height: 1.75rem;
  margin-bottom: var(--spacing-sm);
}

.result-info {
  color: #666688;
  font-size: var(--font-size-sm);
}

.clear-filters-btn {
  padding: 0;
  color: #8888aa;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: var(--font-size-sm);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.clear-filters-btn:hover {
  color: #ccccee;
}

.error-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 6px;
  font-size: var(--font-size-sm);
}

.retry-btn {
  flex: 0 0 auto;
  padding: 4px 10px;
  color: #fff;
  background: #5a2a2a;
  border: 1px solid #7a3a3a;
  border-radius: 4px;
  cursor: pointer;
  font-size: var(--font-size-sm);
}

.retry-btn:hover {
  background: #6a3030;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(130px, 100%), 1fr));
  gap: var(--spacing-md);
}

.empty-state,
.loading-state {
  padding: 5rem 1rem;
  color: #666688;
  text-align: center;
  font-size: var(--font-size-md);
}

.load-more-wrap {
  padding: 2.5rem 0;
  text-align: center;
}

.load-more-btn {
  min-width: var(--touch-target);
  padding: var(--spacing-sm) var(--spacing-lg);
  color: #aaaacc;
  background: #2a2a3e;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  cursor: pointer;
  font-size: var(--font-size-md);
  transition:
    background-color 0.15s ease,
    opacity 0.15s ease;
}

.load-more-btn:hover:not(:disabled) {
  background: #3a3a5e;
}

.load-more-btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

@media (max-width: 760px) {
  .toolbar {
    align-items: stretch;
  }

  .search-wrap {
    flex-basis: 100%;
  }

  .sort-select {
    flex: 1 1 100%;
  }

  .status-filter-group {
    width: 100%;
  }

  .status-filter-group .filter-btn {
    flex: 1 1 auto;
  }
}

@media (max-width: 600px) {
  .library-header {
    margin-bottom: var(--spacing-sm);
  }

  .mobile-filter-select {
    display: block;
  }

  .desktop-only {
    display: none;
  }

  .grid {
    grid-template-columns: repeat(auto-fill, minmax(105px, 1fr));
    gap: var(--spacing-sm);
  }

  .meta-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .error-banner {
    align-items: flex-start;
    flex-direction: column;
  }

  .retry-btn {
    width: 100%;
  }
}
</style>
