<template>
  <div class="admin-layout">
    <aside class="admin-sidebar" :aria-label="$t('nav.adminNavigation')">
      <div class="brand">
        <router-link to="/admin/libraries" class="brand-link"> {{ $t('nav.fyomAdmin') }} </router-link>
      </div>

      <nav class="admin-nav">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to" class="nav-link">
          {{ item.label }}
        </router-link>
      </nav>

      <div class="sidebar-bottom">
        <router-link to="/" class="nav-link back-link"> {{ $t('nav.backToLibrary') }} </router-link>

        <div class="user-info">
          <div class="user-meta">
            <span class="username" :title="username">
              {{ username }}
            </span>
            <span class="role">
              {{ roleLabel }}
            </span>
          </div>

          <button type="button" class="logout-btn" :disabled="loggingOut" @click="handleLogout">
            {{ loggingOut ? $t('nav.signingOut') : $t('nav.logout') }}
          </button>
        </div>
      </div>
    </aside>

    <main class="admin-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useUserStore } from '@/stores/user';

interface AdminNavItem {
  label: string;
  to: string;
}

const router = useRouter();
const userStore = useUserStore();
const { t } = useI18n();

const loggingOut = ref(false);

const navItems = computed<AdminNavItem[]>(() => [
  {
    label: t('nav.libraries'),
    to: '/admin/libraries',
  },
  {
    label: t('nav.accessControl'),
    to: '/admin/permissions',
  },
  {
    label: t('nav.media'),
    to: '/admin/media',
  },
  {
    label: t('nav.missingItems'),
    to: '/admin/missing',
  },
  {
    label: t('nav.providers'),
    to: '/admin/providers',
  },
  {
    label: t('nav.system'),
    to: '/admin/system',
  },
  {
    label: t('nav.settings'),
    to: '/admin/settings',
  },
]);

const username = computed(() => {
  return userStore.user?.username?.trim() || t('nav.admin');
});

const roleLabel = computed(() => {
  return userStore.user?.role || 'admin';
});

async function handleLogout(): Promise<void> {
  if (loggingOut.value) return;

  loggingOut.value = true;

  try {
    userStore.logout();

    await router.replace({
      path: '/login',
    });
  } finally {
    loggingOut.value = false;
  }
}
</script>

<style scoped>
.admin-layout {
  min-height: 100vh;
  display: flex;
  color: #e0e0e0;
  background: #0a0a14;
}

.admin-sidebar {
  width: 230px;
  min-width: 230px;
  display: flex;
  flex-direction: column;
  padding: 20px 0;
  background: #0e0e1a;
  border-right: 1px solid #1a1a2e;
}

.brand {
  padding: 0 20px;
  margin-bottom: 24px;
}

.brand-link {
  color: #6c63ff;
  font-size: 16px;
  font-weight: 900;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  text-decoration: none;
}

.brand-link:hover {
  color: #8f89ff;
}

.admin-nav {
  flex: 1;
}

.nav-link {
  display: block;
  padding: 10px 20px;
  color: #777799;
  text-decoration: none;
  font-size: 13px;
  line-height: 1.35;
  border-left: 2px solid transparent;
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease;
}

.nav-link:hover {
  color: #aaaacc;
  background: #12121e;
}

.nav-link.router-link-active,
.nav-link.router-link-exact-active {
  color: #ffffff;
  background: #14142a;
  border-left-color: rgb(108 99 255 / 65%);
}

.sidebar-bottom {
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid #1a1a2e;
}

.back-link {
  color: #666688;
  font-size: 12px;
}

.back-link::before {
  content: '← ';
}

.user-info {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 20px 0;
}

.user-meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.username {
  max-width: 120px;
  color: #777799;
  font-size: 12px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.role {
  color: #555577;
  font-size: 11px;
  text-transform: capitalize;
}

.logout-btn {
  flex: 0 0 auto;
  padding: 4px 0;
  color: #777799;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
  transition:
    color 0.15s ease,
    opacity 0.15s ease;
}

.logout-btn:hover:not(:disabled) {
  color: #ff8f8f;
}

.logout-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.admin-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 32px;
}

@media (max-width: 820px) {
  .admin-layout {
    flex-direction: column;
  }

  .admin-sidebar {
    width: 100%;
    min-width: 0;
    padding: 14px 0;
    border-right: 0;
    border-bottom: 1px solid #1a1a2e;
  }

  .brand {
    margin-bottom: 12px;
  }

  .admin-nav {
    display: flex;
    flex-wrap: wrap;
    padding: 0 12px;
  }

  .nav-link {
    flex: 1 1 auto;
    padding: 9px 10px;
    border-left: 0;
    border-bottom: 2px solid transparent;
    text-align: center;
  }

  .nav-link.router-link-active,
  .nav-link.router-link-exact-active {
    border-left-color: transparent;
    border-bottom-color: rgb(108 99 255 / 65%);
  }

  .sidebar-bottom {
    margin-top: 12px;
  }

  .user-info {
    padding: 10px 20px 0;
  }

  .username {
    max-width: 220px;
  }

  .admin-content {
    padding: 20px;
  }
}

@media (max-width: 520px) {
  .admin-nav {
    flex-direction: column;
  }

  .nav-link {
    text-align: left;
  }

  .user-info {
    align-items: flex-start;
    flex-direction: column;
  }

  .logout-btn {
    width: 100%;
    text-align: left;
  }

  .admin-content {
    padding: 16px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .nav-link,
  .brand-link,
  .logout-btn {
    transition: none;
  }
}
</style>
