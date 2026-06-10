<template>
  <div class="admin-page">
    <h1>Library Access</h1>
    <p class="hint">Control which users can view which libraries. Admins always see all libraries.</p>

    <div v-if="loading" class="loading">Loading...</div>

    <template v-else-if="users.length > 0 && libraries.length > 0">
      <div class="permissions-matrix">
        <table>
          <thead>
            <tr>
              <th class="user-header">User</th>
              <th v-for="lib in libraries" :key="lib.id" class="lib-header">{{ lib.name }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.id">
              <td class="user-cell">{{ user.username }}</td>
              <td v-for="lib in libraries" :key="lib.id" class="perm-cell">
                <button
                  :class="['perm-toggle', { active: canView(user.id, lib.id) }]"
                  @click="togglePermission(user.id, lib.id, canView(user.id, lib.id))"
                >
                  {{ canView(user.id, lib.id) ? '&#10003;' : '&#10007;' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <p class="empty" v-else>No users or libraries to configure.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import request from '@/api/request';

const permissions = ref<any[]>([]);
const libraries = ref<any[]>([]);
const users = ref<any[]>([]);
const loading = ref(true);

onMounted(async () => {
  try {
    const [permRes, libRes] = await Promise.all([
      request.get('/admin/permissions'),
      request.get('/admin/libraries'),
    ]);
    permissions.value = (permRes as any).data || [];
    libraries.value = (libRes as any).data || [];

    const userMap = new Map<string, string>();
    for (const p of permissions.value) {
      if (!userMap.has(p.user_id)) {
        userMap.set(p.user_id, p.username);
      }
    }
    users.value = Array.from(userMap.entries()).map(([id, name]) => ({
      id, username: name,
    }));
  } catch {
    // ignore
  } finally {
    loading.value = false;
  }
});

function canView(userId: string, libraryId: string) {
  const p = permissions.value.find(
    (x: any) => x.user_id === userId && x.library_id === libraryId
  );
  return p?.can_view ?? false;
}

async function togglePermission(userId: string, libraryId: string, current: boolean) {
  try {
    await request.put('/admin/permissions', {
      user_id: userId,
      library_id: libraryId,
      can_view: !current,
    });
    const idx = permissions.value.findIndex(
      (x: any) => x.user_id === userId && x.library_id === libraryId
    );
    if (idx >= 0) {
      permissions.value[idx].can_view = !current;
    } else {
      const username = users.value.find((u: any) => u.id === userId)?.username || '';
      const libName = libraries.value.find((l: any) => l.id === libraryId)?.name || '';
      permissions.value.push({
        user_id: userId, username,
        library_id: libraryId, library_name: libName,
        can_view: true,
      });
    }
  } catch {
    // ignore
  }
}
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

.perm-toggle:hover {
  border-color: #2a2a3e;
}

.empty {
  color: #555577;
  text-align: center;
  padding: 40px 0;
}
</style>
