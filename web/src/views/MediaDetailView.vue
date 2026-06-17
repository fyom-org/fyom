<template>
  <main v-if="item" class="detail-view media-detail">
    <div class="backdrop" aria-hidden="true">
      <img
        v-if="!backdropFailed && backdropUrl"
        :src="backdropUrl"
        alt=""
        @error="backdropFailed = true"
      />

      <div class="backdrop-overlay"></div>

      <div v-if="hasProgress" class="backdrop-progress">
        <div
          class="backdrop-progress-fill"
          :style="{ transform: `scaleX(${progressPercent / 100})` }"
        ></div>
      </div>
    </div>

    <div class="content">
      <router-link :to="backTarget" class="back-link">
        {{ backLabel }}
      </router-link>

      <div v-if="error" class="error-banner" role="alert">
        <span>{{ error }}</span>
        <button type="button" class="error-action" @click="reloadCurrentMedia">{{ $t('library.retry') }}</button>
      </div>

      <section class="hero-section">
        <div class="poster-wrap">
          <img
            v-if="posterUrl && !posterFailed"
            class="poster"
            :src="posterUrl"
            :alt="$t('library.poster', { title: item.title })"
            @error="posterFailed = true"
          />

          <div v-else class="poster-fallback" aria-hidden="true">
            {{ posterInitial }}
          </div>
        </div>

        <div class="meta">
          <img
            v-if="logoUrl && !logoFailed"
            class="logo-image"
            :src="logoUrl"
            :alt="$t('library.logo', { title: item.title })"
            @error="logoFailed = true"
          />

          <h1 class="title" :class="{ 'with-logo': logoUrl && !logoFailed }">
            {{ item.title }}
          </h1>

          <p v-if="item.tagline" class="tagline">
            {{ item.tagline }}
          </p>

          <div class="facts">
            <span v-if="item.year">{{ item.year }}</span>

            <span v-if="item.mpaa" class="mpaa-badge">
              {{ item.mpaa }}
            </span>

            <span v-if="isFiniteNumber(item.rating)"> ★ {{ formatRating(item.rating) }} </span>

            <span v-if="isFiniteNumber(item.user_rating)" class="user-rating" title="User rating">
              ☆ {{ formatRating(item.user_rating) }}
            </span>

            <span v-if="item.duration">
              {{ formatDuration(item.duration) }}
            </span>

            <span class="type-badge">
              {{ typeLabel }}
            </span>
          </div>

          <div v-if="metadataChips.length > 0" class="movie-meta-row">
            <span
              v-for="chip in metadataChips"
              :key="chip.label + chip.value"
              class="meta-chip"
              :class="{ 'mpaa-chip': chip.kind === 'mpaa' }"
              :title="chip.label"
            >
              {{ chip.value }}
            </span>
          </div>

          <div v-if="dateChips.length > 0" class="movie-dates">
            <span v-for="chip in dateChips" :key="chip.label" class="date-chip">
              {{ chip.label }}: {{ chip.value }}
            </span>
          </div>

          <div v-if="item.genres?.length" class="genres">
            <span v-for="genre in item.genres" :key="genre" class="genre-tag">
              {{ genre }}
            </span>
          </div>

          <div v-if="canPlayItem" class="action-row">
            <router-link :to="`/play/${item.id}`" class="play-btn">
              <span class="play-text">{{ playLabel }}</span>
            </router-link>
          </div>

          <p v-if="hasProgress && canPlayItem" class="resume-info">
            {{ resumeLabel }}
          </p>

          <section class="status-row" aria-label="Media status">
            <span class="status-label">{{ $t('library.markAs') }}</span>

            <button
              v-for="status in statusOptions"
              :key="status.value"
              type="button"
              class="status-btn"
              :class="[status.className, { active: userStatus === status.value }]"
              :disabled="statusSaving"
              @click="setStatus(status.value)"
            >
              {{ status.label }}
            </button>

            <button
              v-if="userStatus !== STATUS_NONE"
              type="button"
              class="status-btn clear-btn"
              :disabled="statusSaving"
              @click="setStatus(STATUS_NONE)"
            >
              {{ $t('library.clear') }}
            </button>
          </section>

          <p v-if="statusError" class="status-error" role="alert">
            {{ statusError }}
          </p>
        </div>
      </section>

      <section v-if="item.overview" class="overview-section">
        <p class="overview" :class="{ collapsed: !overviewExpanded && overviewNeedsExpand }">
          {{ item.overview }}
        </p>

        <button
          v-if="overviewNeedsExpand"
          type="button"
          class="expand-btn"
          @click="overviewExpanded = !overviewExpanded"
        >
          {{ overviewExpanded ? $t('library.showLess') : $t('library.showMore') }}
        </button>
      </section>

      <section v-if="item.type === 'episode'" class="episode-detail-section">
        <div class="episode-detail-header">
          <router-link v-if="item.show_id" :to="`/media/${item.show_id}`" class="back-to-show">
            {{ $t('library.backToShow') }}
          </router-link>
        </div>

        <div class="episode-detail-meta">
          <h2 v-if="episodeCode" class="episode-detail-number">
            {{ episodeCode }}
          </h2>

          <span v-if="item.aired" class="episode-aired"> {{ $t('library.aired') }}{{ formatDateForLocale(item.aired, item.aired) }} </span>

          <span v-if="isFiniteNumber(item.rating)" class="episode-rating">
            ★ {{ formatRating(item.rating) }}
          </span>
        </div>

        <p v-if="item.overview" class="episode-plot">
          {{ item.overview }}
        </p>
      </section>

      <section v-if="visibleActors.length > 0" class="cast-section">
        <div class="section-header">
          <h3 class="section-subtitle">{{ $t('library.cast') }}</h3>
        </div>

        <div class="cast-list">
          <article
            v-for="actor in visibleActors"
            :key="actor.name + actor.role"
            class="cast-member"
          >
            <div class="cast-avatar" aria-hidden="true">
              {{ getInitial(actor.name) }}
            </div>

            <div class="cast-info">
              <span class="cast-name">{{ actor.name }}</span>
              <span v-if="actor.role" class="cast-role">{{ actor.role }}</span>
            </div>
          </article>
        </div>
      </section>

      <section v-if="visibleGuestStars.length > 0" class="guest-stars">
        <div class="section-header">
          <h3 class="section-subtitle">{{ $t('library.guestStars') }}</h3>
        </div>

        <div class="cast-grid">
          <article
            v-for="guest in visibleGuestStars"
            :key="guest.name + guest.role"
            class="cast-card"
          >
            <div class="cast-avatar large" aria-hidden="true">
              {{ getInitial(guest.name) }}
            </div>

            <div class="cast-name">{{ guest.name }}</div>
            <div v-if="guest.role" class="cast-role">{{ guest.role }}</div>
          </article>
        </div>
      </section>

      <section
        v-if="item.set_name || item.collection_number || item.set_overview"
        class="collection-section"
      >
        <h3 class="section-subtitle">{{ $t('library.collection') }}</h3>

        <div class="collection-info">
          <span v-if="item.set_name" class="collection-name">
            {{ item.set_name }}
          </span>

          <span v-if="item.collection_number" class="collection-number">
            {{ $t('library.tmdbCollectionId') }}{{ item.collection_number }}
          </span>
        </div>

        <p v-if="item.set_overview" class="collection-overview">
          {{ item.set_overview }}
        </p>
      </section>

      <EpisodeList v-if="item.type === 'show'" :show-id="item.id" class="episode-list" />
    </div>
  </main>

  <main v-else-if="loading" class="state-view">
    <div class="state-card">{{ $t('library.loadingMedia') }}</div>
  </main>

  <main v-else class="state-view">
    <div class="state-card">
      <h1>{{ $t('library.mediaNotFound') }}</h1>
      <p>{{ error || $t('library.mediaNotFoundDetail') }}</p>

      <div class="state-actions">
        <button type="button" class="state-btn" @click="reloadCurrentMedia">{{ $t('library.retry') }}</button>

        <router-link to="/library" class="state-link"> {{ $t('library.backToLibrary') }} </router-link>
      </div>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { getMediaDetail, setMediaStatus } from '@/api/library';
import EpisodeList from '@/components/EpisodeList.vue';
import { getSafeApiErrorMessage, isRecord } from '@/lib/api/errors';
import { resolveResourceUrl } from '@/lib/runtime/resource';
import { useLocaleFormat } from '@/composables/useLocaleFormat';

interface CastMember {
  name: string;
  role?: string;
}

interface MediaItem {
  id: string;
  title: string;
  type: 'movie' | 'show' | 'episode' | string;
  year?: number;
  poster_url?: string;
  backdrop_url?: string;
  logo_url?: string;
  tagline?: string;
  overview?: string;
  rating?: number;
  user_rating?: number;
  duration?: number;
  mpaa?: string;
  custom_rating?: string;
  original_title?: string;
  language?: string;
  country_code?: string;
  release_date?: string;
  end_date?: string;
  date_added?: string;
  playcount?: number;
  genres?: string[];
  actors?: CastMember[];
  guest_stars?: CastMember[];
  user_status?: MediaStatus;
  show_id?: string;
  season?: number;
  episode?: number;
  aired?: string;
  set_name?: string;
  collection_number?: string | number;
  set_overview?: string;
}

interface ProgressState {
  position: number;
  duration: number;
  finished: boolean;
}

interface MetadataChip {
  label: string;
  value: string;
  kind?: 'mpaa';
}

interface DateChip {
  label: string;
  value: string | number;
}

type MediaStatus = 'none' | 'want_to_watch' | 'watching' | 'watched' | 'dropped';

interface StatusOption {
  value: MediaStatus;
  label: string;
  className: string;
}

const STATUS_NONE: MediaStatus = 'none';
const STATUS_WANT: MediaStatus = 'want_to_watch';
const STATUS_WATCHING: MediaStatus = 'watching';
const STATUS_WATCHED: MediaStatus = 'watched';
const STATUS_DROPPED: MediaStatus = 'dropped';

// This endpoint is intentionally called with native fetch instead of apiRequest.
// Reason: progress can legitimately return 401/403 for non-playable or unauthorized
// progress records, and that must not trigger the global axios unauthorized handler.
const API_BASE_URL = normalizeBaseUrl(
  (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
);

const route = useRoute();
const { t } = useI18n();
// Phase 8: locale-aware duration formatting. Previously this view hand-rolled
// "1h 23m" strings which were English-only even when the UI was Japanese/Chinese.
// Phase 11: extended to also format date chips and play counts locale-aware.
const { formatDuration: formatDurationLocale, formatDate: formatDateForLocale, formatNumber } = useLocaleFormat();

const item = ref<MediaItem | null>(null);
const progress = ref<ProgressState | null>(null);

const loading = ref(true);
const error = ref('');
const statusError = ref('');
const statusSaving = ref(false);

const backdropFailed = ref(false);
const posterFailed = ref(false);
const logoFailed = ref(false);
const overviewExpanded = ref(false);

const userStatus = ref<MediaStatus>(STATUS_NONE);

let fetchGeneration = 0;

const statusOptions = computed<StatusOption[]>(() => [
  {
    value: STATUS_WANT,
    label: t('library.want'),
    className: 'want',
  },
  {
    value: STATUS_WATCHING,
    label: t('library.watching'),
    className: 'watching',
  },
  {
    value: STATUS_WATCHED,
    label: t('library.watched'),
    className: 'watched',
  },
  {
    value: STATUS_DROPPED,
    label: t('library.dropped'),
    className: 'dropped',
  },
]);

const posterUrl = computed(() => resolveResourceUrl(item.value?.poster_url));
const backdropUrl = computed(() => resolveResourceUrl(item.value?.backdrop_url));
const logoUrl = computed(() => resolveResourceUrl(item.value?.logo_url));

const posterInitial = computed(() => getInitial(item.value?.title || 'F'));

const canPlayItem = computed(() => {
  return item.value?.type !== 'show';
});

const typeLabel = computed(() => {
  switch (item.value?.type) {
    case 'movie':
      return t('library.movie');
    case 'show':
      return t('library.show');
    case 'episode':
      return t('library.episode');
    default:
      return item.value?.type || t('library.media');
  }
});

const backTarget = computed(() => {
  if (item.value?.type === 'episode' && item.value.show_id) {
    return `/media/${item.value.show_id}`;
  }

  return '/library';
});

const backLabel = computed(() => {
  if (item.value?.type === 'episode' && item.value.show_id) {
    return t('library.backToShow');
  }

  return t('library.backToLibrary');
});

const metadataChips = computed<MetadataChip[]>(() => {
  const media = item.value;
  if (!media) return [];

  const chips: MetadataChip[] = [];

  if (media.original_title) {
    chips.push({
      label: t('library.originalTitle'),
      value: media.original_title,
    });
  }

  if (media.language) {
    chips.push({
      label: t('library.language'),
      value: media.language,
    });
  }

  if (media.country_code) {
    chips.push({
      label: t('library.countryCode'),
      value: media.country_code,
    });
  }

  if (media.custom_rating) {
    chips.push({
      label: t('library.customRating'),
      value: media.custom_rating,
    });
  } else if (media.mpaa) {
    chips.push({
      label: t('library.mpaa'),
      value: media.mpaa,
      kind: 'mpaa',
    });
  }

  return chips;
});

const dateChips = computed<DateChip[]>(() => {
  const media = item.value;
  if (!media) return [];

  const chips: DateChip[] = [];

  if (media.release_date) {
    chips.push({
      label: t('library.released'),
      value: formatDateForLocale(media.release_date, media.release_date),
    });
  }

  if (media.end_date) {
    chips.push({
      label: t('library.end'),
      value: formatDateForLocale(media.end_date, media.end_date),
    });
  }

  if (media.date_added) {
    chips.push({
      label: t('library.added'),
      value: formatDateForLocale(media.date_added, media.date_added),
    });
  }

  if (media.playcount && media.playcount > 0) {
    chips.push({
      label: t('library.played'),
      value: t('library.playedCount', { n: formatNumber(media.playcount) }),
    });
  }

  return chips;
});

const visibleActors = computed(() => {
  return Array.isArray(item.value?.actors) ? item.value.actors.slice(0, 8) : [];
});

const visibleGuestStars = computed(() => {
  return Array.isArray(item.value?.guest_stars) ? item.value.guest_stars.slice(0, 12) : [];
});

const overviewNeedsExpand = computed(() => {
  return Boolean(item.value?.overview && item.value.overview.length > 280);
});

const hasProgress = computed(() => {
  return Boolean(
    progress.value &&
    progress.value.position > 0 &&
    progress.value.duration > 0 &&
    !progress.value.finished
  );
});

const progressPercent = computed(() => {
  if (!progress.value?.duration) return 0;

  return Math.min((progress.value.position / progress.value.duration) * 100, 100);
});

const resumeLabel = computed(() => {
  if (!hasProgress.value || !progress.value) return '';

  const position = formatDuration(progress.value.position);
  const duration = formatDuration(progress.value.duration);

  return t('library.resumeFrom', { position, duration });
});

const playLabel = computed(() => {
  if (progress.value?.finished) return t('library.playAgain');
  if (hasProgress.value) return t('library.resume');
  return t('library.play');
});

const episodeCode = computed(() => {
  const media = item.value;
  if (!media) return '';

  const parts: string[] = [];

  if (media.season !== undefined && media.season !== null) {
    parts.push(`S${String(media.season).padStart(2, '0')}`);
  }

  if (media.episode !== undefined && media.episode !== null) {
    parts.push(`E${String(media.episode).padStart(2, '0')}`);
  }

  return parts.join(' ');
});

watch(
  () => route.params.id,
  () => {
    void reloadCurrentMedia();
  },
  {
    immediate: true,
  }
);

async function reloadCurrentMedia(): Promise<void> {
  const id = route.params.id;

  if (typeof id !== 'string' || !id) {
    item.value = null;
    error.value = t('library.invalidMediaId');
    loading.value = false;
    return;
  }

  if (typeof window !== 'undefined') {
    window.scrollTo({
      top: 0,
      behavior: 'auto',
    });
  }

  await fetchMediaDetail(id);
}

async function fetchMediaDetail(id: string): Promise<void> {
  const generation = ++fetchGeneration;

  loading.value = true;
  error.value = '';
  statusError.value = '';
  item.value = null;
  progress.value = null;
  userStatus.value = STATUS_NONE;
  overviewExpanded.value = false;
  backdropFailed.value = false;
  posterFailed.value = false;
  logoFailed.value = false;

  try {
    const detail = await getMediaDetail(id);

    if (generation !== fetchGeneration) return;

    const normalizedItem = normalizeMediaItem(detail);

    item.value = normalizedItem;
    userStatus.value = normalizeStatus(normalizedItem.user_status);

    if (normalizedItem.type !== 'show') {
      await fetchProgressSafely(id, generation);
    }
  } catch (unknownError) {
    if (generation !== fetchGeneration) return;

    console.error('[fyom] fetch media detail failed:', unknownError);
    error.value = getSafeApiErrorMessage(unknownError, 'library.loadFailedDetail');
    item.value = null;
  } finally {
    if (generation === fetchGeneration) {
      loading.value = false;
    }
  }
}

async function fetchProgressSafely(id: string, generation: number): Promise<void> {
  const token = readPersistedToken();

  if (!token) {
    progress.value = null;
    return;
  }

  try {
    const response = await fetch(`${API_BASE_URL}/media/${encodeURIComponent(id)}/progress`, {
      method: 'GET',
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${token}`,
      },
    });

    if (generation !== fetchGeneration) return;

    // Progress is optional. Authorization failures here must not invalidate the
    // whole user session because some media records may not expose progress.
    if (response.status === 401 || response.status === 403 || response.status === 404) {
      progress.value = null;
      return;
    }

    if (!response.ok) {
      progress.value = null;
      return;
    }

    const payload = await response.json();
    const nextProgress = normalizeProgressResponse(payload);

    if (generation !== fetchGeneration) return;

    progress.value = nextProgress;
  } catch {
    if (generation !== fetchGeneration) return;

    progress.value = null;
  }
}

async function setStatus(status: MediaStatus): Promise<void> {
  if (!item.value || statusSaving.value || userStatus.value === status) return;

  const previousStatus = userStatus.value;

  statusSaving.value = true;
  statusError.value = '';
  userStatus.value = status;

  try {
    await setMediaStatus(item.value.id, status);

    if (item.value) {
      item.value.user_status = status;
    }
  } catch (unknownError) {
    console.error('[fyom] update media status failed:', unknownError);
    userStatus.value = previousStatus;

    if (item.value) {
      item.value.user_status = previousStatus;
    }

    statusError.value = getSafeApiErrorMessage(unknownError, 'library.statusUpdateFailed');
  } finally {
    statusSaving.value = false;
  }
}

function normalizeMediaItem(value: unknown): MediaItem {
  if (isRecord(value) && isRecord(value.data)) {
    return value.data as unknown as MediaItem;
  }

  return value as MediaItem;
}

function normalizeProgressResponse(value: unknown): ProgressState | null {
  const data = isRecord(value) && 'data' in value ? value.data : value;

  if (!isRecord(data)) return null;

  const position = Number(data.position);
  const duration = Number(data.duration);

  if (!Number.isFinite(position) || !Number.isFinite(duration)) {
    return null;
  }

  return {
    position,
    duration,
    finished: Boolean(data.finished),
  };
}

function normalizeStatus(status: unknown): MediaStatus {
  switch (status) {
    case STATUS_WANT:
    case STATUS_WATCHING:
    case STATUS_WATCHED:
    case STATUS_DROPPED:
    case STATUS_NONE:
      return status;
    default:
      return STATUS_NONE;
  }
}

function formatDuration(seconds: number): string {
  // Delegate to the locale-aware composable. The original returned English-only
  // "1h 23m"; the composable renders "1時間23分" / "1小时23分" / "1h 23m" per
  // the active locale. Empty-string contract for non-positive values is preserved.
  return formatDurationLocale(seconds);
}

function formatRating(value: number): string {
  return value.toFixed(1);
}

function getInitial(value: string): string {
  const trimmed = value.trim();

  if (!trimmed) return '?';

  return trimmed.slice(0, 1).toUpperCase();
}

function readPersistedToken(): string | null {
  if (typeof window === 'undefined') return null;

  return window.localStorage.getItem('token');
}

function normalizeBaseUrl(value: string): string {
  return value.replace(/\/+$/, '');
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}
</script>

<style scoped>
.detail-view {
  min-height: 100vh;
  color: #e0e0e0;
  background: #0f0f1a;
}

.backdrop {
  position: fixed;
  inset: 0 0 auto;
  z-index: 0;
  height: 100vh;
  overflow: hidden;
  background:
    radial-gradient(circle at top left, rgb(108 99 255 / 14%), transparent 30rem), #1a1a2e;
}

.backdrop img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(5px) brightness(0.32) saturate(1.08);
  transform: scale(1.03);
}

.backdrop-overlay {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(to bottom, rgb(15 15 26 / 20%) 0%, rgb(15 15 26 / 82%) 58%, #0f0f1a 100%),
    linear-gradient(
      to right,
      rgb(15 15 26 / 82%) 0%,
      rgb(15 15 26 / 22%) 52%,
      rgb(15 15 26 / 88%) 100%
    );
}

.backdrop-progress {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 4px;
  background: rgb(255 255 255 / 10%);
}

.backdrop-progress-fill {
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, #6c63ff, #2196f3);
  transform-origin: left;
  transition: transform 0.25s ease;
}

.content {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 1080px;
  box-sizing: border-box;
  margin: 0 auto;
  padding: 72px 24px 48px;
}

.back-link,
.back-to-show {
  display: inline-flex;
  align-items: center;
  margin-bottom: 18px;
  color: #aaaacc;
  font-size: 14px;
  text-decoration: none;
  transition: color 0.15s ease;
}

.back-link::before,
.back-to-show::before {
  content: '←';
  margin-right: 6px;
}

.back-link:hover,
.back-to-show:hover {
  color: #fff;
}

.error-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
  padding: 12px 14px;
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 10px;
  font-size: 13px;
}

.error-action {
  flex: 0 0 auto;
  padding: 6px 12px;
  color: #fff;
  background: #5a2a2a;
  border: 1px solid #7a3a3a;
  border-radius: 7px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
}

.error-action:hover {
  background: #6a3030;
}

.hero-section {
  display: grid;
  grid-template-columns: 190px minmax(0, 1fr);
  gap: 28px;
  align-items: start;
}

.poster-wrap {
  width: 190px;
}

.poster,
.poster-fallback {
  width: 190px;
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  box-shadow: 0 18px 46px rgb(0 0 0 / 55%);
}

.poster {
  display: block;
  object-fit: cover;
}

.poster-fallback {
  display: grid;
  place-items: center;
  color: #777799;
  background: linear-gradient(135deg, rgb(108 99 255 / 20%), rgb(33 150 243 / 10%)), #1a1a2e;
  border: 1px solid rgb(255 255 255 / 7%);
  font-size: 56px;
  font-weight: 900;
}

.meta {
  min-width: 0;
}

.logo-image {
  display: block;
  max-width: min(360px, 100%);
  max-height: 110px;
  margin-bottom: 14px;
  object-fit: contain;
  object-position: left center;
}

.title {
  margin: 0 0 12px;
  color: #f3f3ff;
  font-size: clamp(2rem, 5vw, 4rem);
  font-weight: 900;
  letter-spacing: -0.055em;
  line-height: 0.98;
}

.title.with-logo {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}

.tagline {
  max-width: 760px;
  margin: 0 0 14px;
  color: #9f99ff;
  font-size: 15px;
  font-style: italic;
  line-height: 1.55;
}

.facts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
  margin-bottom: 16px;
  color: #aaaacc;
  font-size: 14px;
}

.type-badge,
.mpaa-badge {
  display: inline-flex;
  align-items: center;
  min-height: 22px;
  box-sizing: border-box;
  padding: 2px 9px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.type-badge {
  color: #ccccee;
  background: #2a2a3e;
  text-transform: capitalize;
}

.mpaa-badge {
  color: #aaaacc;
  border: 1px solid #3a3a5e;
}

.user-rating {
  color: #ffaa00;
}

.movie-meta-row,
.movie-dates,
.genres {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.movie-meta-row {
  margin-top: 10px;
}

.movie-dates {
  margin-top: 8px;
}

.genres {
  margin-top: 12px;
}

.meta-chip,
.date-chip,
.genre-tag {
  display: inline-flex;
  align-items: center;
  color: #9999bb;
  background: rgb(26 26 46 / 70%);
  border: 1px solid #2a2a3e;
  border-radius: 999px;
  font-size: 12px;
}

.meta-chip,
.genre-tag {
  padding: 4px 10px;
}

.date-chip {
  padding: 3px 9px;
  color: #777799;
}

.meta-chip.mpaa-chip {
  color: #aaaacc;
  border-color: #3a3a5e;
  font-weight: 700;
}

.action-row {
  margin-top: 20px;
}

.play-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 48px;
  padding: 13px 34px;
  color: #fff;
  background: #6c63ff;
  border-radius: 12px;
  box-shadow: 0 10px 32px rgb(108 99 255 / 35%);
  font-size: 17px;
  font-weight: 900;
  letter-spacing: 0.01em;
  text-decoration: none;
  transition:
    background-color 0.15s ease,
    box-shadow 0.15s ease,
    transform 0.15s ease;
}

.play-btn::before {
  content: '▶';
  margin-right: 10px;
  font-size: 13px;
}

.play-btn:hover {
  background: #5a52e0;
  box-shadow: 0 12px 38px rgb(108 99 255 / 45%);
  transform: translateY(-1px);
}

.resume-info {
  margin: 9px 0 0;
  color: #9f99ff;
  font-size: 13px;
  font-weight: 600;
}

.status-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
}

.status-label {
  color: #666688;
  font-size: 12px;
  font-weight: 700;
}

.status-btn {
  min-height: 32px;
  padding: 6px 13px;
  color: #8888aa;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease,
    opacity 0.15s ease;
}

.status-btn:hover:not(:disabled) {
  color: #ccccee;
  border-color: #3a3a5e;
}

.status-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.status-btn.active {
  color: #fff;
}

.status-btn.active.want {
  background: #1565c0;
  border-color: #2196f3;
}

.status-btn.active.watching {
  background: #5a52e0;
  border-color: #6c63ff;
}

.status-btn.active.watched {
  background: #2e7d32;
  border-color: #4caf50;
}

.status-btn.active.dropped {
  background: #c62828;
  border-color: #ff6b6b;
}

.clear-btn {
  color: #666688;
}

.clear-btn:hover:not(:disabled) {
  color: #ff8f8f;
  border-color: #ff6b6b;
}

.status-error {
  margin: 8px 0 0;
  color: #ff8f8f;
  font-size: 12px;
}

.overview-section,
.cast-section,
.guest-stars,
.collection-section,
.episode-detail-section,
.episode-list {
  margin-top: 28px;
}

.overview {
  max-width: 760px;
  margin: 0;
  color: #b5b5d6;
  font-size: 15px;
  line-height: 1.75;
}

.overview.collapsed {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 4;
  line-clamp: 4;
}

.expand-btn {
  margin-top: 6px;
  padding: 4px 0;
  color: #8f89ff;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 13px;
  font-weight: 700;
}

.expand-btn:hover {
  color: #b4b0ff;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-subtitle {
  margin: 0 0 13px;
  color: #8888aa;
  font-size: 13px;
  font-weight: 900;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.cast-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 12px;
}

.cast-member {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 10px;
  background: rgb(26 26 46 / 55%);
  border: 1px solid rgb(255 255 255 / 5%);
  border-radius: 12px;
}

.cast-avatar {
  width: 38px;
  height: 38px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  color: #aaaacc;
  background: #2a2a3e;
  border-radius: 999px;
  font-size: 14px;
  font-weight: 900;
}

.cast-avatar.large {
  width: 48px;
  height: 48px;
  font-size: 17px;
}

.cast-info {
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.cast-name {
  color: #ddddef;
  font-size: 13px;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.cast-role {
  margin-top: 2px;
  color: #666688;
  font-size: 11px;
  line-height: 1.35;
}

.cast-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(112px, 1fr));
  gap: 12px;
}

.cast-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-height: 118px;
  gap: 7px;
  padding: 12px;
  text-align: center;
  background: rgb(26 26 46 / 55%);
  border: 1px solid rgb(255 255 255 / 5%);
  border-radius: 12px;
}

.episode-detail-section {
  padding: 16px;
  background: rgb(26 26 46 / 45%);
  border: 1px solid rgb(255 255 255 / 6%);
  border-radius: 14px;
}

.episode-detail-header {
  margin-bottom: 10px;
}

.episode-detail-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
  color: #8888aa;
  font-size: 13px;
}

.episode-detail-number {
  margin: 0;
  color: #ccccee;
  background: #2a2a3e;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 900;
}

.episode-aired {
  color: #777799;
}

.episode-rating {
  color: #ffaa00;
}

.episode-plot {
  max-width: 760px;
  margin: 12px 0 0;
  color: #9999bb;
  font-size: 14px;
  line-height: 1.7;
}

.collection-info {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 8px;
}

.collection-name {
  color: #ccccee;
  font-size: 15px;
  font-weight: 800;
}

.collection-number {
  color: #666688;
  font-size: 12px;
}

.collection-overview {
  max-width: 760px;
  margin: 0;
  color: #9999bb;
  font-size: 14px;
  line-height: 1.7;
}

.state-view {
  min-height: 100vh;
  display: grid;
  place-items: center;
  box-sizing: border-box;
  padding: 24px;
  color: #e0e0e0;
  background: #0f0f1a;
}

.state-card {
  width: 100%;
  max-width: 440px;
  padding: 28px;
  text-align: center;
  background: #1a1a2e;
  border: 1px solid rgb(255 255 255 / 6%);
  border-radius: 16px;
  box-shadow: 0 18px 48px rgb(0 0 0 / 35%);
}

.state-card h1 {
  margin: 0 0 8px;
  color: #f3f3ff;
  font-size: 22px;
}

.state-card p {
  margin: 0;
  color: #8888aa;
  font-size: 14px;
  line-height: 1.55;
}

.state-actions {
  display: flex;
  justify-content: center;
  gap: 10px;
  margin-top: 18px;
}

.state-btn,
.state-link {
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 9px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 800;
}

.state-btn {
  color: #fff;
  background: #6c63ff;
  border: 0;
  cursor: pointer;
}

.state-link {
  color: #ccccee;
  background: #2a2a3e;
  text-decoration: none;
}

.state-btn:hover {
  background: #5a52e0;
}

.state-link:hover {
  color: #fff;
}

@media (max-width: 760px) {
  .content {
    padding: 44px 16px 36px;
  }

  .hero-section {
    grid-template-columns: 1fr;
    gap: 20px;
  }

  .poster-wrap {
    width: 138px;
  }

  .poster,
  .poster-fallback {
    width: 138px;
    border-radius: 10px;
  }

  .poster-fallback {
    font-size: 42px;
  }

  .title {
    font-size: clamp(2rem, 11vw, 3rem);
  }

  .facts {
    gap: 8px 12px;
  }

  .play-btn {
    width: 100%;
  }

  .status-row {
    align-items: stretch;
  }

  .status-label {
    flex-basis: 100%;
  }

  .status-btn {
    flex: 1 1 auto;
  }

  .cast-list,
  .cast-grid {
    grid-template-columns: 1fr;
  }

  .error-banner {
    align-items: flex-start;
    flex-direction: column;
  }

  .error-action {
    width: 100%;
  }
}

@media (max-width: 480px) {
  .state-actions {
    flex-direction: column;
  }

  .state-btn,
  .state-link {
    width: 100%;
    box-sizing: border-box;
  }
}

@media (prefers-reduced-motion: reduce) {
  .backdrop-progress-fill,
  .back-link,
  .back-to-show,
  .error-action,
  .play-btn,
  .status-btn {
    transition: none;
  }

  .play-btn:hover {
    transform: none;
  }
}
</style>
