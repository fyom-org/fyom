import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { apiRequest } from '@/api/request';

export type SystemStatus =
  | 'unknown'
  | 'checking'
  | 'needs_setup'
  | 'initialized'
  | 'error';

export const useSystemStore = defineStore('system', () => {
  const status = ref<SystemStatus>('unknown');
  const checkedOnce = ref(false);

  const isSystemReady = computed(
    () => status.value === 'needs_setup' || status.value === 'initialized'
  );
  const needsSetup = computed(() => status.value === 'needs_setup');
  const isInitialized = computed(() => status.value === 'initialized');

  async function fetchSystemStatus(): Promise<void> {
    if (status.value === 'checking') return;
    status.value = 'checking';
    try {
      const res = await apiRequest.get<{ initialized: boolean }>('/system/status');
      status.value = res.data.initialized ? 'initialized' : 'needs_setup';
      checkedOnce.value = true;
    } catch {
      status.value = 'error';
      checkedOnce.value = true;
    }
  }

  function reset(): void {
    status.value = 'unknown';
    checkedOnce.value = false;
  }

  return {
    status,
    checkedOnce,
    isSystemReady,
    needsSetup,
    isInitialized,
    fetchSystemStatus,
    reset,
  };
});
