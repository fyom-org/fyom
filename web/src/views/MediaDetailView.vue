<template>
  <div v-if="item" class="detail-view">
    <div class="backdrop">
      <img
        v-if="!backdropFailed && item.backdrop_url"
        :src="item.backdrop_url"
        @error="backdropFailed = true"
      />
      <div class="backdrop-overlay"></div>
      <div class="backdrop-progress" v-if="hasProgress">
        <div class="backdrop-progress-fill" :style="{ width: progressPercent + '%' }"></div>
      </div>
    </div>

    <div class="content">
      <router-link to="/library" class="back-link">&#8592; Back to Library</router-link>

      <div class="main-row">
        <img v-if="item.poster_url" class="poster" :src="item.poster_url" />

        <div class="meta">
          <h1 class="title">{{ item.title }}</h1>
          <div class="facts">
            <span v-if="item.year">{{ item.year }}</span>
            <span v-if="item.rating">&#9733; {{ item.rating.toFixed(1) }}</span>
            <span v-if="item.duration">{{ formatDuration(item.duration) }}</span>
            <span class="type-badge">{{ item.type }}</span>
          </div>

          <div class="action-row" v-if="item.type !== 'show'">
            <router-link :to="`/play/${item.id}`" class="play-btn">
              <span class="play-icon">&#9654;</span>
              <span class="play-text">{{ playLabel }}</span>
            </router-link>
          </div>
          <p class="resume-info" v-if="hasProgress && item.type !== 'show'">{{ resumeLabel }}</p>
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

      <EpisodeList v-if="item.type === 'show'" :show-id="item.id" />
    </div>
  </div>

  <div v-else-if="loading" class="loading">Loading...</div>
  <div v-else class="loading">Not found</div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail } from '@/api/library';
import request from '@/api/request';
import EpisodeList from '@/components/EpisodeList.vue';

const route = useRoute();
const item = ref<any>(null);
const loading = ref(true);
const backdropFailed = ref(false);
const progress = ref<any>(null);

const overviewExpanded = ref(false);
const overviewNeedsExpand = ref(false);

watch(item, (val) => {
  if (val?.overview && val.overview.length > 200) {
    overviewNeedsExpand.value = true;
  } else {
    overviewNeedsExpand.value = false;
  }
});

const hasProgress = computed(() =>
  progress.value && progress.value.position > 0 && !progress.value.finished
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

onMounted(async () => {
  try {
    item.value = await getMediaDetail(route.params.id as string);
    try {
      const progressRes: any = await request.get(`/media/${route.params.id}/progress`);
      progress.value = progressRes.data;
    } catch {
      progress.value = null;
    }
  } finally {
    loading.value = false;
  }
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
  position: relative;
  height: 300px;
  overflow: hidden;
  background: #1a1a2e;
}

.backdrop img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(2px) brightness(0.6);
}

.backdrop-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, rgba(15, 15, 26, 0.3), #0f0f1a);
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
  transition: width 0.3s;
}

.content {
  max-width: 960px;
  margin: 0 auto;
  padding: 0 24px 40px;
  margin-top: -80px;
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
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
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
