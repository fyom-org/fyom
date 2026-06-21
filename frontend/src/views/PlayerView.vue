<template>
  <main class="player-view">
    <section v-if="error" class="error-state" role="alert">
      <h1>{{ $t('player.unableToPlay') }}</h1>
      <p>{{ error }}</p>

      <div class="error-actions">
        <button type="button" class="error-btn" @click="reloadCurrentMedia">
          {{ $t('player.retry') }}
        </button>

        <router-link v-if="mediaId" :to="`/media/${mediaId}`" class="error-link">
          {{ $t('player.backToDetails') }}
        </router-link>

        <router-link to="/library" class="error-link">
          {{ $t('player.backToLibrary') }}
        </router-link>
      </div>
    </section>

    <section v-else class="launcher-status">
      <div v-if="loading" class="loading">
        <span class="spinner" aria-hidden="true"></span>
        <span>{{ $t('player.loadingMedia') }}</span>
      </div>

      <div v-else-if="launched" class="launched-notice">
        <span class="icon" aria-hidden="true">▶</span>
        <span>{{ $t('player.playingExternally') }}</span>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { getApiErrorMessage, getMediaDetail } from '@/api/library';
import { isDesktopMode } from '@/lib/runtime/env';
import { resolveResourceUrl } from '@/lib/runtime/resource';

const route = useRoute();

const mediaId = computed(() => {
  const id = route.params.id;
  return typeof id === 'string' ? id : '';
});

const loading = ref(true);
const launched = ref(false);
const error = ref('');

async function launchExternalPlayback(): Promise<void> {
  const id = mediaId.value;
  if (!id) {
    error.value = 'No media ID provided.';
    loading.value = false;
    return;
  }

  try {
    const item = await getMediaDetail(id);
    const rawStreamUrl = item['stream_url'] as string | undefined;

    if (!rawStreamUrl) {
      error.value = 'No stream URL available for this media item.';
      loading.value = false;
      return;
    }

    // In desktop mode, resolve relative /api/v1/... URLs via same-origin.
    // In browser mode, the Vite proxy handles relative paths.
    const resolvedUrl = isDesktopMode()
      ? resolveResourceUrl(rawStreamUrl)
      : rawStreamUrl;

    // Open the presigned stream URL in a new tab/window.
    window.open(resolvedUrl as string, '_blank');

    launched.value = true;

    // Fire-and-forget: mark the media as played on the backend.
    // This is best-effort — failures are silently ignored.
    try {
      const { authRequest } = await import('@/api/request');
      await authRequest.put(`/media/${id}/progress`, { played: true });
    } catch {
      // Silently ignore progress reporting failures.
    }
  } catch (err) {
    console.error('[player] failed to launch external playback:', err);
    error.value = getApiErrorMessage(err, 'Failed to load media.');
  } finally {
    loading.value = false;
  }
}

function reloadCurrentMedia(): void {
  error.value = '';
  loading.value = true;
  launched.value = false;
  void launchExternalPlayback();
}

onMounted(() => {
  void launchExternalPlayback();
});
</script>

<style scoped>
.player-view {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
  padding: 24px;
}

.error-state {
  text-align: center;
  max-width: 480px;
}

.error-state h1 {
  font-size: 20px;
  margin-bottom: 8px;
}

.error-state p {
  color: var(--text-muted, #999);
  margin-bottom: 16px;
}

.error-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
}

.error-btn,
.error-link {
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 14px;
  text-decoration: none;
  cursor: pointer;
}

.error-btn {
  background: var(--accent, #3b82f6);
  color: #fff;
  border: none;
}

.error-link {
  background: transparent;
  color: var(--accent, #3b82f6);
  border: 1px solid var(--accent, #3b82f6);
}

.launcher-status {
  text-align: center;
}

.loading,
.launched-notice {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 16px;
}

.spinner {
  display: inline-block;
  width: 20px;
  height: 20px;
  border: 2px solid var(--text-muted, #999);
  border-top-color: var(--accent, #3b82f6);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.launched-notice .icon {
  font-size: 24px;
  color: var(--accent, #3b82f6);
}
</style>
