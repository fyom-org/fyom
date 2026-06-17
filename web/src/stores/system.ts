import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { apiRequest } from '@/api/request';
import type { ApiEnvelope, SystemStatusData } from '@/api/types';

export type SystemStatus = 'unknown' | 'checking' | 'initialized' | 'error';

export const useSystemStore = defineStore('system', () => {
  const status = ref<SystemStatus>('unknown');
  const checkedOnce = ref(false);

  const isInitialized = computed(() => status.value === 'initialized');

  async function fetchSystemStatus(): Promise<void> {
    if (status.value === 'checking') return;
    status.value = 'checking';
    try {
      const res = await apiRequest.get<ApiEnvelope<SystemStatusData>>('/system/status');
      status.value = res.data.data.initialized ? 'initialized' : 'error';
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
    isInitialized,
    fetchSystemStatus,
    reset,
  };
});
