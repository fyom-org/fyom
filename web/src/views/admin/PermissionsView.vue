<template>
  <div class="admin-page">
    <h1>Library Access</h1>
    <p class="hint">
      Control which users can view which libraries. Admins always see all libraries.
    </p>

    <div v-if="loading" class="loading">Loading...</div>
    <p v-else-if="error" class="error-text">{{ error }}</p>

    <template v-else-if="users.length > 0 && libraries.length > 0">
      <div class="permissions-matrix">
        <table>
          <thead>
            <tr>
              <th class="user-header">User</th>
              <th v-for="lib in libraries" :key="lib.id" class="lib-header">
                {{ lib.name }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.id">
              <td class="user-cell">{{ user.username }}</td>

              <td v-for="lib in libraries" :key="lib.id" class="perm-cell">
                <button
                  :class="['perm-toggle', { active: canView(user.id, lib.id) }]"
                  :disabled="pendingKey === makePermissionKey(user.id, lib.id)"
                  @click="togglePermission(user.id, lib.id)"
                >
                  {{
                    pendingKey === makePermissionKey(user.id, lib.id)
                      ? '...'
                      : canView(user.id, lib.id)
                        ? '✓'
                        : '✗'
                  }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <p v-else class="empty">No users or libraries to configure.</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

interface PermissionEntry {
  user_id: string;
  username: string;
  library_id: string;
  library_name?: string;
  can_view: boolean;
}

interface Library {
  id: string;
  name: string;
}

interface MatrixUser {
  id: string;
  username: string;
}

const permissions = ref<PermissionEntry[]>([]);
const libraries = ref<Library[]>([]);
const loading = ref(true);
const error = ref('');
const pendingKey = ref('');

const users = computed<MatrixUser[]>(() => {
  const userMap = new Map<string, string>();

  for (const entry of permissions.value) {
    if (!userMap.has(entry.user_id)) {
      userMap.set(entry.user_id, entry.username);
    }
  }

  return Array.from(userMap.entries())
    .map(([id, username]) => ({ id, username }))
    .sort((a, b) => a.username.localeCompare(b.username));
});

const permissionMap = computed<Record<string, boolean>>(() => {
  const map: Record<string, boolean> = {};

  for (const entry of permissions.value) {
    map[makePermissionKey(entry.user_id, entry.library_id)] = entry.can_view;
  }

  return map;
});

function makePermissionKey(userId: string, libraryId: string): string {
  return `${userId}::${libraryId}`;
}

function canView(userId: string, libraryId: string): boolean {
  return permissionMap.value[makePermissionKey(userId, libraryId)] === true;
}

async function loadPermissionsData(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const [permRes, libRes] = await Promise.all([
      authRequest.get<ApiEnvelope<PermissionEntry[]>>('/admin/permissions'),
      authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries'),
    ]);

    permissions.value = permRes.data.data || [];
    libraries.value = (libRes.data.data || []).slice().sort((a, b) => a.name.localeCompare(b.name));
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[permissions] loadPermissionsData failed:', err);
    error.value = 'Failed to load library permissions';
  } finally {
    loading.value = false;
  }
}

async function togglePermission(userId: string, libraryId: string): Promise<void> {
  const key = makePermissionKey(userId, libraryId);
  const current = canView(userId, libraryId);
  const next = !current;

  pendingKey.value = key;

  try {
    await authRequest.put<ApiEnvelope<null>>('/admin/permissions', {
      user_id: userId,
      library_id: libraryId,
      can_view: next,
    });

    const idx = permissions.value.findIndex(
      (entry) => entry.user_id === userId && entry.library_id === libraryId
    );

    if (idx >= 0) {
      permissions.value[idx] = {
        ...permissions.value[idx],
        can_view: next,
      };
      permissions.value = [...permissions.value];
    } else {
      const username = users.value.find((u) => u.id === userId)?.username || '';
      const libraryName = libraries.value.find((l) => l.id === libraryId)?.name || '';

      permissions.value = [
        ...permissions.value,
        {
          user_id: userId,
          username,
          library_id: libraryId,
          library_name: libraryName,
          can_view: next,
        },
      ];
    }
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[permissions] togglePermission failed:', err);
    error.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to update permission';
  } finally {
    pendingKey.value = '';
  }
}

onMounted(() => {
  void loadPermissionsData();
});
</script>

<style scoped>
h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0 0 8px;
}

.hint {
  color: #555577;
  font-size: 13px;
  margin-bottom: 24px;
}

.loading {
  color: #555577;
}

.error-text {
  color: #ff6b6b;
  font-size: 13px;
  margin-bottom: 16px;
}

.permissions-matrix {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th {
  text-align: left;
  padding: 10px 12px;
  color: #666688;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid #1a1a2e;
}

.lib-header {
  text-align: center;
  min-width: 100px;
}

td {
  padding: 8px 12px;
  border-bottom: 1px solid #0e0e1a;
}

.user-cell {
  color: #ccccee;
  font-size: 14px;
  white-space: nowrap;
}

.perm-cell {
  text-align: center;
}

.perm-toggle {
  width: 36px;
  height: 36px;
  border-radius: 6px;
  border: 1px solid #1a1a2e;
  background: #0a0a14;
  color: #555577;
  cursor: pointer;
  font-size: 16px;
  transition: all 0.15s;
}

.perm-toggle.active {
  background: #1a3a1a;
  color: #4caf50;
  border-color: #2a4a2a;
}

.perm-toggle:hover:not(:disabled) {
  border-color: #2a2a3e;
}

.perm-toggle:disabled {
  opacity: 0.6;
  cursor: wait;
}

.empty {
  color: #555577;
  text-align: center;
  padding: 40px 0;
}
</style>
