<template>
  <main class="admin-page">
    <header class="page-header">
      <div>
        <h1>{{ $t('admin.permissions.title') }}</h1>
        <p class="hint">
          {{ $t('admin.permissions.subtitle') }}
        </p>
      </div>

      <button type="button" class="reload-btn" :disabled="loading" @click="loadPermissionsData">
        {{ loading ? $t('admin.permissions.loading') : $t('admin.permissions.reload') }}
      </button>
    </header>

    <div v-if="error" class="error-banner" role="alert">
      <span>{{ error }}</span>
      <button type="button" class="error-action" @click="clearError">{{ $t('common.dismiss') }}</button>
    </div>

    <div v-if="loading" class="loading">{{ $t('admin.permissions.loadingPermissions') }}</div>

    <template v-else-if="users.length > 0 && libraries.length > 0">
      <div class="summary-row">
        <span>{{ users.length }} {{ $t('admin.permissions.userCount', users.length) }}</span>
        <span>{{ libraries.length }} {{ $t('admin.permissions.libraryCount', libraries.length) }}</span>
      </div>

      <div class="permissions-matrix">
        <table>
          <thead>
            <tr>
              <th class="user-header">{{ $t('admin.permissions.userHeader') }}</th>
              <th
                v-for="library in libraries"
                :key="library.id"
                class="lib-header"
                :title="library.name"
              >
                {{ library.name }}
              </th>
            </tr>
          </thead>

          <tbody>
            <tr v-for="user in users" :key="user.id">
              <td class="user-cell">
                <div class="user-name-row">
                  <span class="user-name">{{ user.username }}</span>
                  <span v-if="isPrivilegedUser(user)" class="role-badge">
                    {{ user.role }}
                  </span>
                </div>
              </td>

              <td v-for="library in libraries" :key="library.id" class="perm-cell">
                <button
                  type="button"
                  class="perm-toggle"
                  :class="{
                    active: canView(user.id, library.id) || isPrivilegedUser(user),
                    locked: isPrivilegedUser(user),
                  }"
                  :disabled="isToggleDisabled(user, library.id)"
                  :aria-label="permissionAriaLabel(user, library)"
                  @click="togglePermission(user, library)"
                >
                  <span v-if="isPending(user.id, library.id)">...</span>
                  <span v-else-if="isPrivilegedUser(user)">{{ $t('admin.permissions.all') }}</span>
                  <span v-else-if="canView(user.id, library.id)">{{ $t('admin.permissions.yes') }}</span>
                  <span v-else>{{ $t('admin.permissions.no') }}</span>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <p v-if="statusMessage" class="status-message" role="status">
        {{ statusMessage }}
      </p>
    </template>

    <section v-else class="empty">
      <p>{{ $t('admin.permissions.noConfig') }}</p>
      <button type="button" class="reload-btn" @click="loadPermissionsData">{{ $t('admin.permissions.reload') }}</button>
    </section>


  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { authRequest } from '@/api/request';
import {
  getSafeApiErrorMessage,
  isUnauthorizedOrForbidden,
  isRecord,
} from '@/lib/api/errors';
import type { ApiEnvelope } from '@/api/types';

const { t } = useI18n();

interface PermissionEntry {
  user_id: string;
  username: string;
  role?: string;
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
  role?: string;
}

interface PermissionUpdatePayload {
  user_id: string;
  library_id: string;
  can_view: boolean;
}

const permissions = ref<PermissionEntry[]>([]);
const libraries = ref<Library[]>([]);
const loading = ref(true);
const error = ref('');
const statusMessage = ref('');
const pendingKeys = ref<Set<string>>(new Set());

const users = computed<MatrixUser[]>(() => {
  const userMap = new Map<string, MatrixUser>();

  for (const entry of permissions.value) {
    if (!entry.user_id) continue;

    if (!userMap.has(entry.user_id)) {
      userMap.set(entry.user_id, {
        id: entry.user_id,
        username: entry.username || t('admin.permissions.unknownUser'),
        role: entry.role,
      });
    }
  }

  return Array.from(userMap.values()).sort((a, b) => {
    return a.username.localeCompare(b.username);
  });
});

const permissionMap = computed<Record<string, boolean>>(() => {
  const map: Record<string, boolean> = {};

  for (const entry of permissions.value) {
    map[makePermissionKey(entry.user_id, entry.library_id)] = entry.can_view === true;
  }

  return map;
});

onMounted(() => {
  void loadPermissionsData();
});

function makePermissionKey(userId: string, libraryId: string): string {
  return `${userId}::${libraryId}`;
}

function canView(userId: string, libraryId: string): boolean {
  return permissionMap.value[makePermissionKey(userId, libraryId)] === true;
}

function isPending(userId: string, libraryId: string): boolean {
  return pendingKeys.value.has(makePermissionKey(userId, libraryId));
}

function isPrivilegedUser(user: MatrixUser): boolean {
  const role = user.role?.toLowerCase();

  return role === 'admin' || role === 'owner';
}

function isToggleDisabled(user: MatrixUser, libraryId: string): boolean {
  return isPrivilegedUser(user) || isPending(user.id, libraryId);
}

function permissionAriaLabel(user: MatrixUser, library: Library): string {
  if (isPrivilegedUser(user)) {
    return t('admin.permissions.ariaHasAccess', {
      username: user.username,
      role: user.role || 'admin',
      library: library.name,
    });
  }

  return canView(user.id, library.id)
    ? t('admin.permissions.ariaRevoke', { username: user.username, library: library.name })
    : t('admin.permissions.ariaGrant', { username: user.username, library: library.name });
}

function clearError(): void {
  error.value = '';
}

async function loadPermissionsData(): Promise<void> {
  loading.value = true;
  error.value = '';
  statusMessage.value = '';
  pendingKeys.value = new Set();

  try {
    const [permissionResponse, libraryResponse] = await Promise.all([
      authRequest.get<ApiEnvelope<PermissionEntry[]>>('/admin/permissions', {
        authFailureMode: 'forbidden',
      }),
      authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries', {
        authFailureMode: 'forbidden',
      }),
    ]);

    permissions.value = normalizePermissions(permissionResponse.data);
    libraries.value = normalizeLibraries(libraryResponse.data);
  } catch (unknownError) {
    if (isUnauthorizedOrForbidden(unknownError)) {
      error.value = t('admin.permissions.noPermission');
      return;
    }

    console.error('[fyom] load permissions failed:', unknownError);
    error.value = getSafeApiErrorMessage(unknownError, 'admin.permissions.loadFailed');
  } finally {
    loading.value = false;
  }
}

async function togglePermission(user: MatrixUser, library: Library): Promise<void> {
  if (isPrivilegedUser(user)) return;

  const key = makePermissionKey(user.id, library.id);

  if (pendingKeys.value.has(key)) return;

  const current = canView(user.id, library.id);
  const next = !current;
  const previousPermissions = permissions.value.slice();

  statusMessage.value = '';
  error.value = '';
  addPendingKey(key);

  applyPermissionLocally(user, library, next);

  try {
    await updatePermission({
      user_id: user.id,
      library_id: library.id,
      can_view: next,
    });

    statusMessage.value = next
      ? t('admin.permissions.granted', { username: user.username, library: library.name })
      : t('admin.permissions.revoked', { username: user.username, library: library.name });
  } catch (unknownError) {
    permissions.value = previousPermissions;

    if (isUnauthorizedOrForbidden(unknownError)) {
      error.value = t('admin.permissions.updateNoPermission');
      return;
    }

    console.error('[fyom] update permission failed:', unknownError);
    error.value = getSafeApiErrorMessage(unknownError, 'admin.permissions.updateFailed');
  } finally {
    removePendingKey(key);
  }
}

async function updatePermission(payload: PermissionUpdatePayload): Promise<void> {
  await authRequest.put<ApiEnvelope<null>>('/admin/permissions', payload, {
    authFailureMode: 'forbidden',
  });
}

function applyPermissionLocally(user: MatrixUser, library: Library, canViewNext: boolean): void {
  const index = permissions.value.findIndex((entry) => {
    return entry.user_id === user.id && entry.library_id === library.id;
  });

  if (index >= 0) {
    const nextPermissions = permissions.value.slice();

    nextPermissions[index] = {
      ...nextPermissions[index],
      can_view: canViewNext,
    };

    permissions.value = nextPermissions;
    return;
  }

  permissions.value = [
    ...permissions.value,
    {
      user_id: user.id,
      username: user.username,
      role: user.role,
      library_id: library.id,
      library_name: library.name,
      can_view: canViewNext,
    },
  ];
}

function addPendingKey(key: string): void {
  const next = new Set(pendingKeys.value);

  next.add(key);
  pendingKeys.value = next;
}

function removePendingKey(key: string): void {
  const next = new Set(pendingKeys.value);

  next.delete(key);
  pendingKeys.value = next;
}

function normalizePermissions(
  value: ApiEnvelope<PermissionEntry[]> | PermissionEntry[] | unknown
): PermissionEntry[] {
  const data = unwrapUnknownEnvelope(value);

  if (!Array.isArray(data)) return [];

  return data
    .filter(isRecord)
    .map(
      (entry): PermissionEntry => ({
        user_id: toStringValue(entry.user_id),
        username: toStringValue(entry.username) || t('admin.permissions.unknownUser'),
        role: typeof entry.role === 'string' ? entry.role : undefined,
        library_id: toStringValue(entry.library_id),
        library_name: typeof entry.library_name === 'string' ? entry.library_name : undefined,
        can_view: entry.can_view === true,
      })
    )
    .filter((entry) => entry.user_id && entry.library_id);
}

function normalizeLibraries(value: ApiEnvelope<Library[]> | Library[] | unknown): Library[] {
  const data = unwrapUnknownEnvelope(value);

  if (!Array.isArray(data)) return [];

  return data
    .filter(isRecord)
    .map(
      (entry): Library => ({
        id: toStringValue(entry.id),
        name: toStringValue(entry.name) || t('admin.permissions.untitledLibrary'),
      })
    )
    .filter((library) => library.id)
    .sort((a, b) => a.name.localeCompare(b.name));
}

function unwrapUnknownEnvelope(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }

  return value;
}

function toStringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}
</script>

<style scoped>
.admin-page {
  width: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 20px;
}

h1 {
  margin: 0 0 8px;
  color: #e0e0e0;
  font-size: 24px;
  font-weight: 800;
}

h2 {
  margin: 0 0 8px;
  color: #dadaf0;
  font-size: 15px;
  font-weight: 800;
}

.hint {
  max-width: 720px;
  margin: 0;
  color: #666688;
  font-size: 13px;
  line-height: 1.5;
}

.reload-btn,
.error-action {
  min-height: 34px;
  padding: 7px 12px;
  color: #fff;
  background: #6c63ff;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 800;
}

.reload-btn:hover:not(:disabled),
.error-action:hover {
  background: #5a52e0;
}

.reload-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.loading {
  padding: 40px 0;
  color: #555577;
  text-align: center;
  font-size: 14px;
}

.error-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  padding: 12px 14px;
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 10px;
  font-size: 13px;
}

.summary-row {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
  color: #666688;
  font-size: 12px;
}

.permissions-matrix {
  overflow-x: auto;
  background: #10101b;
  border: 1px solid #1a1a2e;
  border-radius: 12px;
}

table {
  width: 100%;
  min-width: 640px;
  border-collapse: collapse;
}

th {
  padding: 11px 12px;
  color: #777799;
  border-bottom: 1px solid #1a1a2e;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-align: left;
  text-transform: uppercase;
}

.user-header {
  min-width: 180px;
}

.lib-header {
  min-width: 110px;
  max-width: 180px;
  overflow: hidden;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

td {
  padding: 9px 12px;
  border-bottom: 1px solid #0e0e1a;
}

tbody tr:last-child td {
  border-bottom: 0;
}

.user-cell {
  color: #ccccee;
  font-size: 14px;
  white-space: nowrap;
}

.user-name-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-name {
  overflow: hidden;
  text-overflow: ellipsis;
}

.role-badge {
  flex: 0 0 auto;
  padding: 2px 7px;
  color: #8f89ff;
  background: rgb(108 99 255 / 12%);
  border: 1px solid rgb(108 99 255 / 22%);
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
  text-transform: capitalize;
}

.perm-cell {
  text-align: center;
}

.perm-toggle {
  min-width: 46px;
  height: 34px;
  padding: 0 9px;
  color: #8888aa;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 7px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 800;
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease,
    opacity 0.15s ease;
}

.perm-toggle.active {
  color: #7bd88f;
  background: #142515;
  border-color: #2a4a2a;
}

.perm-toggle.locked {
  color: #8f89ff;
  background: rgb(108 99 255 / 10%);
  border-color: rgb(108 99 255 / 22%);
  cursor: not-allowed;
}

.perm-toggle:hover:not(:disabled) {
  color: #ccccee;
  border-color: #2a2a3e;
}

.perm-toggle.active:hover:not(:disabled) {
  color: #9ff0ad;
  border-color: #3b6a3b;
}

.perm-toggle:disabled {
  cursor: wait;
  opacity: 0.65;
}

.status-message {
  margin: 12px 0 0;
  color: #7bd88f;
  font-size: 13px;
}

.empty {
  padding: 42px 0;
  color: #555577;
  text-align: center;
  font-size: 14px;
}

@media (max-width: 720px) {
  .page-header,
  .error-banner {
    flex-direction: column;
  }

  .reload-btn,
  .error-action {
    width: 100%;
  }
}
</style>
