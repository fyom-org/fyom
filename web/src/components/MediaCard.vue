<template>
  <router-link v-slot="{ navigate }" :to="`/media/${item.id}`" custom>
    <div
      class="media-card"
      role="link"
      tabindex="0"
      @click="navigate"
      @mouseenter="onHover"
      @mouseleave="onLeave"
    >
      <div class="poster-wrap">
        <img v-if="item.poster_url" :src="item.poster_url" :alt="item.title" loading="lazy" />
        <div v-else class="poster-fallback">{{ item.title?.[0] ?? '?' }}</div>
        <div
          class="status-icon"
          v-if="item.user_status && item.user_status !== 'none'"
          :class="item.user_status"
          @click.stop="cycleStatus"
        >
          {{ statusEmoji }}
        </div>
        <span class="library-tag" v-if="libraryName">{{ libraryName }}</span>
        <div v-if="hovered" class="hover-overlay">
          <button class="play-icon" @click.stop="play">▶</button>
        </div>
        <div v-if="item.position && item.duration && item.position > 0" class="progress-bar">
          <div
            class="progress-fill"
            :style="{ transform: `scaleX(${progressPercent / 100})` }"
          ></div>
        </div>
      </div>
      <div class="info">
        <span class="title">{{ item.title }}</span>
        <div class="meta-row">
          <span v-if="item.year" class="year">{{ item.year }}</span>
          <span v-if="item.type" class="type-badge">{{ item.type }}</span>
        </div>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import {
  setMediaStatus,
  STATUS_NONE,
  STATUS_WANT,
  STATUS_WATCHING,
  STATUS_WATCHED,
  STATUS_DROPPED,
} from '@/api/library';

const props = defineProps<{
  item: {
    id: string;
    title: string;
    year?: number;
    type?: string;
    poster_url?: string;
    position?: number;
    duration?: number;
    user_status?: string;
  };
  libraryName?: string;
}>();
const emit = defineEmits<{
  (e: 'status-changed', id: string, status: string): void;
}>();

const router = useRouter();
const hovered = ref(false);
const onHover = () => {
  hovered.value = true;
};
const onLeave = () => {
  hovered.value = false;
};
const play = () => {
  router.push(`/play/${props.item.id}`);
};
const progressPercent = computed(() => {
  if (!props.item.position || !props.item.duration) return 0;
  return Math.min((props.item.position / props.item.duration) * 100, 100);
});

const statusEmoji = computed(() => {
  switch (props.item.user_status) {
    case STATUS_WANT:
      return '🔖';
    case STATUS_WATCHING:
      return '▶';
    case STATUS_WATCHED:
      return '✓';
    case STATUS_DROPPED:
      return '✕';
    default:
      return '';
  }
});

const STATUS_CYCLE = [STATUS_NONE, STATUS_WANT, STATUS_WATCHING, STATUS_WATCHED, STATUS_DROPPED];

async function cycleStatus() {
  const current = props.item.user_status || STATUS_NONE;
  const idx = STATUS_CYCLE.indexOf(current);
  const next = STATUS_CYCLE[(idx + 1) % STATUS_CYCLE.length];
  try {
    await setMediaStatus(props.item.id, next);
    emit('status-changed', props.item.id, next);
  } catch {
    console.error('Failed to update status');
  }
}
</script>

<style scoped>
.media-card {
  cursor: pointer;
  position: relative;
  transition: transform 0.2s ease;
  transform-origin: center;
}

@media (hover: none) {
  .media-card:hover {
    transform: none;
  }
}

.poster-wrap {
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  overflow: hidden;
  background: #2a2a3e;
  position: relative;
}

.poster-wrap img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.poster-fallback {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 3rem;
  color: #555577;
}

.status-icon {
  position: absolute;
  top: var(--spacing-sm);
  left: var(--spacing-sm);
  width: var(--touch-target);
  height: var(--touch-target);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-md);
  z-index: 2;
  cursor: pointer;
  transition: all 0.15s;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
}

.status-icon.want_to_watch {
  background: rgba(33, 150, 243, 0.7);
}

.status-icon.watching {
  background: rgba(108, 99, 255, 0.7);
}

.status-icon.watched {
  background: rgba(76, 175, 80, 0.7);
}

.status-icon.dropped {
  background: rgba(255, 107, 107, 0.7);
}

.status-icon:hover {
  transform: scale(1.15);
}

.library-tag {
  position: absolute;
  top: var(--spacing-sm);
  right: var(--spacing-sm);
  background: rgba(0, 0, 0, 0.7);
  color: #8888aa;
  font-size: var(--font-size-sm);
  padding: 2px 6px;
  border-radius: 3px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  z-index: 2;
}

.hover-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
  border-radius: 8px;
}

@media (hover: hover) {
  .media-card:hover .hover-overlay {
    opacity: 1;
  }
  .media-card:hover {
    transform: scale(1.08);
    z-index: 10;
  }
}

.play-icon {
  width: var(--touch-target);
  height: var(--touch-target);
  border-radius: 50%;
  background: rgba(108, 99, 255, 0.9);
  color: #fff;
  border: none;
  font-size: 1.5rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
}

.play-icon::before,
.play-icon::after {
  display: none;
}

.progress-bar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 0 0 8px 8px;
}

.progress-fill {
  height: 100%;
  background: var(--color-primary);
  transition: transform 0.3s;
  transform-origin: left;
  width: 100%;
}

.info {
  margin-top: var(--spacing-sm);
}

.title {
  font-size: var(--font-size-sm);
  color: var(--color-text);
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: 2px;
}

.year {
  font-size: var(--font-size-sm);
  color: #777799;
}

.type-badge {
  font-size: var(--font-size-sm);
  background: #2a2a3e;
  padding: 1px 6px;
  border-radius: 3px;
  text-transform: capitalize;
  color: #777799;
}
</style>
