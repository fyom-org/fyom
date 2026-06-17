<template>
  <div v-if="seasons.length" class="episode-list">
    <div v-for="s in seasons" :key="s.season" class="season-group">
      <button class="season-header" @click="toggleSeason(s.season)">
        <span class="season-name">
          {{ s.season === 0 ? $t('library.specials') : $t('library.season', { n: s.season }) }}
        </span>
        <span class="season-count">{{ s.episodes.length }} {{ $t('library.episodeCount', s.episodes.length) }}</span>
        <span class="chevron" :class="{ expanded: isSeasonExpanded(s.season) }">&#8249;</span>
      </button>

      <div class="season-episodes" v-if="isSeasonExpanded(s.season)">
        <div
          v-for="ep in s.episodes"
          :key="ep.id"
          class="episode-row"
        >
          <router-link :to="`/media/${ep.id}`" class="ep-label">{{ epLabel(ep) }}</router-link>
          <router-link :to="`/media/${ep.id}`" class="ep-title-link">{{ ep.title }}</router-link>
          <span v-if="ep.duration" class="ep-duration">{{ formatDuration(ep.duration) }}</span>
          <router-link :to="`/play/${ep.id}`" class="ep-play" @click.stop>&#9654;</router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { getEpisodes } from '@/api/library';
import { useLocaleFormat } from '@/composables/useLocaleFormat';

// Phase 8: locale-aware duration formatting. Previously this component
// hand-rolled "1h 23m" strings which were English-only.
const { formatDuration } = useLocaleFormat();

const props = defineProps<{ showId: string }>();

interface Episode {
  id: string;
  title: string;
  season: number;
  episode: number;
  duration: number;
  poster_url?: string;
  stream_url?: string;
}

const episodes = ref<Episode[]>([]);
const expandedSeasons = ref(new Set<number>());

// Read user preference: default expanded or collapsed
const defaultCollapsed = (() => {
  try {
    return localStorage.getItem('seasons_collapsed_default') === 'true';
  } catch {
    return false;
  }
})();

onMounted(async () => {
  try {
    episodes.value = await getEpisodes(props.showId);
    // Pre-expand all seasons by default
    if (!defaultCollapsed) {
      const allSeasons = new Set<number>();
      for (const ep of episodes.value) {
        allSeasons.add(ep.season ?? 0);
      }
      expandedSeasons.value = allSeasons;
    }
  } catch {
    episodes.value = [];
  }
});

const seasons = computed(() => {
  const map: Record<number, Episode[]> = {};
  for (const ep of episodes.value) {
    const s = ep.season ?? 0;
    if (!map[s]) map[s] = [];
    map[s].push(ep);
  }
  return Object.entries(map)
    .sort(([a], [b]) => Number(a) - Number(b))
    .map(([s, eps]) => ({ season: Number(s), episodes: eps }));
});

function toggleSeason(season: number) {
  const s = expandedSeasons.value;
  if (s.has(season)) s.delete(season);
  else s.add(season);
  expandedSeasons.value = new Set(s);
}

function isSeasonExpanded(season: number) {
  return expandedSeasons.value.has(season);
}

function epLabel(ep: Episode) {
  const s = ep.season ?? 0;
  const e = ep.episode ?? 0;
  return `${s}\u00d7${String(e).padStart(2, '0')}`;
}

</script>

<style scoped>
.episode-list {
  margin-top: 32px;
}

.season-group {
  margin-bottom: 4px;
}

.season-header {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: #14142a;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  color: #c0c0d0;
  font-size: 15px;
  text-align: left;
  transition: background 0.15s;
}

.season-header:hover {
  background: #1a1a32;
}

.season-name {
  font-weight: 600;
  flex: 1;
}

.season-count {
  color: #555577;
  font-size: 13px;
}

.chevron {
  color: #555577;
  font-size: 18px;
  transition: transform 0.2s;
  transform: rotate(-90deg);
}

.chevron.expanded {
  transform: rotate(90deg);
}

.season-episodes {
  padding: 4px 0 4px 16px;
}

.episode-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  text-decoration: none;
  color: #9999bb;
  font-size: 14px;
  transition: background 0.15s;
}

.episode-row:hover {
  background: #1a1a32;
  color: #e0e0e0;
}

.ep-label {
  color: #6c63ff;
  font-weight: 600;
  min-width: 48px;
  font-size: 13px;
  text-decoration: none;
}

.ep-label:hover {
  color: #8b83ff;
}

.ep-title-link {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: #9999bb;
  text-decoration: none;
}

.ep-title-link:hover {
  color: #e0e0e0;
}

.ep-duration {
  color: #555577;
  font-size: 13px;
}

.ep-play {
  color: #6c63ff;
  text-decoration: none;
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 4px;
  transition: background 0.15s;
}

.ep-play:hover {
  background: #2a2a3e;
}
</style>
