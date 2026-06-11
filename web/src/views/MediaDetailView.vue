<template>
  <div v-if="item" class="detail-view media-detail">
    <div class="backdrop">
      <img
        v-if="!backdropFailed && item.backdrop_url"
        :src="item.backdrop_url"
        @error="backdropFailed = true"
      />
      <div class="backdrop-overlay"></div>
      <div class="backdrop-progress" v-if="hasProgress">
        <div
          class="backdrop-progress-fill"
          :style="{ transform: `scaleX(${progressPercent / 100})` }"
        ></div>
      </div>
    </div>

    <div class="content">
      <router-link to="/library" class="back-link">&#8592; Back to Library</router-link>

      <div class="main-row">
        <img v-if="item.poster_url" class="poster" :src="item.poster_url" />

        <div class="meta">
          <img
            v-if="item.logo_url && !logoFailed"
            class="logo-image"
            :src="item.logo_url"
            @error="logoFailed = true"
          />
          <h1 class="title" :class="{ 'with-logo': item.logo_url && !logoFailed }">
            {{ item.title }}
          </h1>
          <p class="tagline" v-if="item.tagline">{{ item.tagline }}</p>
          <div class="facts">
            <span v-if="item.year">{{ item.year }}</span>
            <span class="mpaa-badge" v-if="item.mpaa">{{ item.mpaa }}</span>
            <span v-if="item.rating">&#9733; {{ item.rating.toFixed(1) }}</span>
            <span v-if="item.user_rating" class="user-rating" :title="'User Rating'"
              >&#9734; {{ item.user_rating.toFixed(1) }}</span
            >
            <span v-if="item.duration">{{ formatDuration(item.duration) }}</span>
            <span class="type-badge">{{ item.type }}</span>
            <h2 class="episode-detail-number">
              <span v-if="item.season"> S{{ String(item.season).padStart(2, '0') }} </span>
              <span v-if="item.episode"> E{{ String(item.episode).padStart(2, '0') }} </span>
            </h2>
          </div>

          <!-- Movie metadata row -->
          <div class="movie-meta-row">
            <span v-if="item.original_title" class="meta-chip" :title="'Original Title'">{{
              item.original_title
            }}</span>
            <span v-if="item.language" class="meta-chip" :title="'Language'">{{
              item.language
            }}</span>
            <span v-if="item.country_code" class="meta-chip" :title="'Country Code'">{{
              item.country_code
            }}</span>
            <span v-if="item.custom_rating" class="meta-chip" :title="'Custom Rating'">{{
              item.custom_rating
            }}</span>
            <span v-if="item.mpaa && !item.custom_rating" class="meta-chip mpaa-chip">{{
              item.mpaa
            }}</span>
          </div>

          <!-- Dates and additional info -->
          <div class="movie-dates">
            <span v-if="item.release_date" class="date-chip"
              >Released: {{ item.release_date }}</span
            >
            <span v-if="item.end_date" class="date-chip">End: {{ item.end_date }}</span>
            <span v-if="item.date_added" class="date-chip">Added: {{ item.date_added }}</span>
            <span v-if="item.playcount > 0" class="date-chip">Played: {{ item.playcount }}x</span>
          </div>
          <div class="genres" v-if="item.genres?.length">
            <span class="genre-tag" v-for="g in item.genres" :key="g">{{ g }}</span>
          </div>

          <div class="action-row" v-if="item.type !== 'show'">
            <router-link :to="`/play/${item.id}`" class="play-btn">
              <span class="play-text">{{ playLabel }}</span>
            </router-link>
          </div>
          <p class="resume-info" v-if="hasProgress && item.type !== 'show'">{{ resumeLabel }}</p>

          <div class="status-row">
            <span class="status-label">Mark as:</span>
            <button
              :class="['status-btn', { active: userStatus === 'want_to_watch' }]"
              @click="setStatus('want_to_watch')"
            >
              ✦ Want
            </button>
            <button
              :class="['status-btn', { active: userStatus === 'watching' }]"
              @click="setStatus('watching')"
            >
              ▶ Watching
            </button>
            <button
              :class="['status-btn', { active: userStatus === 'watched' }]"
              @click="setStatus('watched')"
            >
              ✓ Watched
            </button>
            <button
              :class="['status-btn', { active: userStatus === 'dropped' }]"
              @click="setStatus('dropped')"
            >
              ✕ Dropped
            </button>
            <button
              class="status-btn clear-btn"
              v-if="userStatus !== 'none'"
              @click="setStatus('none')"
            >
              ✕ Clear
            </button>
          </div>
        </div>
      </div>

      <div class="overview-section" v-if="item.overview">
        <p class="overview" :class="{ collapsed: !overviewExpanded }">
          {{ item.overview }}
        </p>
        <button
          class="expand-btn"
          v-if="overviewNeedsExpand"
          @click="overviewExpanded = !overviewExpanded"
        >
          {{ overviewExpanded ? 'Show less' : 'Show more' }}
        </button>
      </div>

      <div class="cast-section" v-if="item.actors?.length">
        <h3 class="section-subtitle">Cast</h3>
        <div class="cast-list">
          <div class="cast-member" v-for="a in item.actors.slice(0, 6)" :key="a.name">
            <div class="cast-avatar">{{ a.name?.[0] || '?' }}</div>
            <div class="cast-info">
              <span class="cast-name">{{ a.name }}</span>
              <span class="cast-role" v-if="a.role">{{ a.role }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Episode-specific section -->
      <div class="episode-detail-section" v-if="item.type === 'episode'">
        <div class="episode-detail-header">
          <router-link v-if="item.show_id" :to="`/media/${item.show_id}`" class="back-to-show">
            ← Back to Show
          </router-link>
        </div>
        <div class="episode-detail-meta">
          <span v-if="item.aired" class="episode-aired">Aired: {{ item.aired }}</span>
          <span v-if="item.rating" class="episode-rating"
            >&#9733; {{ item.rating.toFixed(1) }}</span
          >
        </div>
        <p class="episode-plot" v-if="item.overview">{{ item.overview }}</p>
      </div>

      <section class="guest-stars" v-if="item.guest_stars?.length">
        <h3 class="section-subtitle">Guest Stars</h3>
        <div class="cast-grid">
          <div class="cast-card" v-for="g in item.guest_stars" :key="g.name">
            <div class="cast-avatar">{{ g.name?.[0] || '?' }}</div>
            <div class="cast-name">{{ g.name }}</div>
            <div class="cast-role" v-if="g.role">{{ g.role }}</div>
          </div>
        </div>
      </section>
      <!-- Collection / Set info -->
      <section
        class="collection-section"
        v-if="item.set_name || item.collection_number || item.set_overview"
      >
        <h3 class="section-subtitle">Collection</h3>
        <div class="collection-info">
          <span class="collection-name" v-if="item.set_name">{{ item.set_name }}</span>
          <span class="collection-number" v-if="item.collection_number"
            >TMDb Collection ID: {{ item.collection_number }}</span
          >
        </div>
        <p class="collection-overview" v-if="item.set_overview">{{ item.set_overview }}</p>
      </section>
      <EpisodeList v-if="item.type === 'show'" :show-id="item.id" />
    </div>
  </div>

  <div v-else-if="loading" class="loading">Loading...</div>
  <div v-else class="loading">Not found</div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail, setMediaStatus } from '@/api/library';
import request from '@/api/request';
import EpisodeList from '@/components/EpisodeList.vue';

const route = useRoute();
const item = ref<any>(null);
const loading = ref(true);
const backdropFailed = ref(false);
const logoFailed = ref(false);
const progress = ref<any>(null);
const userStatus = ref('none');

const overviewExpanded = ref(false);
const overviewNeedsExpand = ref(false);

watch(
  item,
  (val) => {
    if (val?.overview && val.overview.length > 200) {
      overviewNeedsExpand.value = true;
    } else {
      overviewNeedsExpand.value = false;
    }
    if (val) userStatus.value = val.user_status || 'none';
  },
  { immediate: true }
);

async function fetchMediaDetail(id: string) {
  loading.value = true;
  backdropFailed.value = false;
  logoFailed.value = false;
  progress.value = null;
  userStatus.value = 'none';
  try {
    item.value = await getMediaDetail(id);
    try {
      const progressRes: any = await request.get(`/media/${id}/progress`);
      progress.value = progressRes.data;
    } catch {
      progress.value = null;
    }
  } finally {
    loading.value = false;
  }
}

const hasProgress = computed(
  () => progress.value && progress.value.position > 0 && !progress.value.finished
);

const resumeLabel = computed(() => {
  if (!hasProgress.value) return '';
  const pos = formatDuration(progress.value.position);
  const dur = formatDuration(progress.value.duration);
  return `Resume from ${pos} / ${dur}`;
});

const playLabel = computed(() => {
  if (progress.value?.finished) return '\u25b6 Play Again';
  if (hasProgress.value) return '\u25b6 Resume';
  return '\u25b6 Play';
});

const progressPercent = computed(() => {
  if (!progress.value || !progress.value.duration) return 0;
  return Math.min((progress.value.position / progress.value.duration) * 100, 100);
});

async function setStatus(status: string) {
  try {
    await setMediaStatus(item.value.id, status);
    userStatus.value = status;
    item.value.user_status = status;
  } catch {
    console.error('Failed to update status');
  }
}

// Watch for same-component navigation (e.g., show -> episode detail)
watch(
  () => route.params.id,
  (newId) => {
    if (newId) {
      item.value = null;
      window.scrollTo(0, 0);
      fetchMediaDetail(newId as string);
    }
  }
);

onMounted(async () => {
  await fetchMediaDetail(route.params.id as string);
});

function formatDuration(sec: number) {
  if (!sec) return '';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}
</script>

<style scoped>
.detail-view {
  min-height: 100vh;
  background: #0f0f1a;
}

.backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 100vh;
  overflow: hidden;
  background: #1a1a2e;
  z-index: 0;
}

.backdrop img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(4px) brightness(0.35);
}

.backdrop-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    to bottom,
    rgba(15, 15, 26, 0.1) 0%,
    rgba(15, 15, 26, 0.85) 60%,
    #0f0f1a 100%
  );
}

.backdrop-progress {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
}

.backdrop-progress-fill {
  height: 100%;
  background: #6c63ff;
  transition: transform 0.3s;
  transform-origin: left;
  width: 100%;
}

.content {
  position: relative;
  z-index: 1;
  max-width: 960px;
  margin: 0 auto;
  padding: 72px 24px 40px;
}

.back-link {
  color: #8888aa;
  font-size: 14px;
  text-decoration: none;
  display: inline-block;
  margin-bottom: 16px;
}

.back-link:hover {
  color: #e0e0e0;
}

.main-row {
  display: flex;
  gap: 24px;
}

.poster {
  width: 180px;
  min-width: 180px;
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  object-fit: cover;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
}

.meta {
  flex: 1;
}

.title {
  font-size: 28px;
  color: #e0e0e0;
  margin: 0 0 12px;
}

.title.with-logo {
  display: none;
}

.logo-image {
  max-width: 320px;
  max-height: 100px;
  margin-bottom: 12px;
  display: block;
}

.facts {
  display: flex;
  gap: 16px;
  color: #8888aa;
  font-size: 14px;
  margin-bottom: 20px;
  align-items: center;
}

.type-badge {
  background: #2a2a3e;
  padding: 2px 10px;
  border-radius: 4px;
  font-size: 12px;
  text-transform: capitalize;
}

.mpaa-badge {
  font-size: 11px;
  padding: 2px 6px;
  border: 1px solid #3a3a5e;
  border-radius: 3px;
  color: #8888aa;
  font-weight: 600;
}

.tagline {
  color: #6c63ff;
  font-size: 15px;
  font-style: italic;
  margin: 4px 0 0;
  font-weight: 300;
}

.genres {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}

.genre-tag {
  background: #1a1a2e;
  color: #8888aa;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  border: 1px solid #2a2a3e;
}

.genre-tag:hover {
  border-color: #3a3a3e;
}

.movie-meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.meta-chip {
  background: #1a1a2e;
  color: #9999bb;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  border: 1px solid #2a2a3e;
}

.meta-chip.mpaa-chip {
  color: #8888aa;
  font-weight: 600;
  border-color: #3a3a5e;
}

.movie-dates {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.date-chip {
  color: #666688;
  font-size: 12px;
  padding: 2px 8px;
  background: rgba(26, 26, 46, 0.5);
  border-radius: 3px;
}

.user-rating {
  color: #ffaa00;
  font-size: 14px;
}

.collection-section {
  margin-top: 24px;
}

.collection-info {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
}

.collection-name {
  color: #ccccee;
  font-size: 15px;
  font-weight: 600;
}

.collection-number {
  color: #666688;
  font-size: 12px;
}

.collection-overview {
  color: #9999bb;
  font-size: 14px;
  line-height: 1.7;
  margin: 0;
  max-width: 640px;
}
.section-subtitle {
  font-size: 14px;
  color: #8888aa;
  margin: 0 0 12px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.cast-section {
  margin-top: 24px;
}

.cast-list {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.cast-member {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cast-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: #2a2a3e;
  color: #666688;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 600;
}

.cast-info {
  display: flex;
  flex-direction: column;
}

.cast-name {
  color: #ccccee;
  font-size: 13px;
}

.cast-role {
  color: #555577;
  font-size: 11px;
}

.episode-detail-section {
  margin-top: 24px;
}

.episode-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.back-to-show {
  color: #6c63ff;
  font-size: 14px;
  text-decoration: none;
}

.back-to-show:hover {
  color: #8b83ff;
}

.episode-detail-number {
  background: #2a2a3e;
  padding: 2px 10px;
  border-radius: 4px;
  font-size: 12px;
  text-transform: capitalize;
}

.episode-detail-meta {
  display: flex;
  gap: 16px;
  color: #8888aa;
  font-size: 13px;
  margin-bottom: 12px;
}

.episode-aired {
  color: #666688;
}

.episode-rating {
  color: #ffaa00;
}

.episode-plot {
  color: #9999bb;
  font-size: 14px;
  line-height: 1.7;
  margin: 0;
  max-width: 640px;
}

.guest-stars {
  margin-top: 24px;
}

.cast-grid {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.cast-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  width: 80px;
}

.action-row {
  margin-top: 8px;
  margin-bottom: 0;
}

.play-btn {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  padding: 16px 40px;
  background: #6c63ff;
  color: #ffffff;
  border-radius: 12px;
  text-decoration: none;
  font-size: 18px;
  font-weight: 700;
  box-shadow: 0 4px 24px rgba(108, 99, 255, 0.4);
  transition: all 0.2s;
}

.play-btn:hover {
  background: #5a52e0;
  transform: translateY(-1px);
  box-shadow: 0 6px 32px rgba(108, 99, 255, 0.5);
}

.play-icon {
  font-size: 22px;
}

.play-text {
  letter-spacing: 0.5px;
}

.resume-info {
  color: #6c63ff;
  font-size: 13px;
  margin-top: 8px;
  margin-bottom: 0;
  font-weight: 500;
}

.status-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.status-label {
  color: #555577;
  font-size: 12px;
}

.status-btn {
  padding: 6px 14px;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 6px;
  color: #8888aa;
  cursor: pointer;
  font-size: 12px;
  transition: all 0.15s;
}

.status-btn:hover {
  border-color: #3a3a5e;
  color: #ccccee;
}

.status-btn.active {
  color: #fff;
}

.status-btn.active:first-of-type {
  background: #1565c0;
  border-color: #2196f3;
}
.status-btn.active:nth-of-type(2) {
  background: #5a52e0;
  border-color: #6c63ff;
}
.status-btn.active:nth-of-type(3) {
  background: #2e7d32;
  border-color: #4caf50;
}
.status-btn.active:nth-of-type(4) {
  background: #c62828;
  border-color: #ff6b6b;
}

.clear-btn {
  margin-left: 4px;
  color: #555577;
  border-color: #2a2a3e;
  font-size: 11px;
}

.clear-btn:hover {
  color: #ff6b6b;
  border-color: #ff6b6b;
}

.overview-section {
  margin-top: 16px;
}

.overview-section .overview {
  color: #9999bb;
  font-size: 14px;
  line-height: 1.7;
  margin: 0;
  max-width: 640px;
}

.overview-section .overview.collapsed {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
}

.expand-btn {
  background: none;
  border: none;
  color: #6c63ff;
  font-size: 13px;
  cursor: pointer;
  padding: 4px 0;
  margin-top: 4px;
}

.expand-btn:hover {
  color: #8b83ff;
}

.loading {
  text-align: center;
  padding: 80px;
  color: #666;
}
</style>
