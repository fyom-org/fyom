<template>
  <div class="layout">
    <header class="header">
      <span class="brand">fyom</span>
      <div class="header-right">
        <span class="username">{{ store.user?.username ?? '—' }}</span>
        <button class="btn-logout" @click="handleLogout">Logout</button>
      </div>
    </header>
    <div class="body">
      <aside class="sidebar">
        <router-link to="/">Home</router-link>
        <router-link to="/library">Library</router-link>
        <!-- Library switcher — only shows when 2+ libraries exist -->
        <div class="library-section" v-if="libraries.length >= 2">
          <div class="section-label">Libraries</div>
          <router-link
            v-for="lib in libraries"
            :key="lib.id"
            :to="`/library/${lib.id}`"
            class="nav-link library-link"
          >
            <span class="library-icon">
              {{ lib.type === 'movie' ? '🎬' : lib.type === 'show' ? '📺' : '📁' }}
            </span>
            {{ lib.name }}
          </router-link>
        </div>
        <router-link v-if="isAdmin" to="/admin" class="nav-link admin-link">⚙ Admin</router-link>
        <div class="sidebar-spacer"></div>
        <router-link to="/profile">Profile</router-link>
      </aside>
      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useUserStore } from '@/stores/user';
import request from '@/api/request';

const router = useRouter();
const store = useUserStore();
const libraries = ref<any[]>([]);

const isAdmin = computed(() => store.isAdmin);

onMounted(async () => {
  try {
    if (store.isLoggedIn && !store.user) {
      await store.fetchMe();
    }
  } catch {
    // ignore
  }
  try {
    const res: any = await request.get('/libraries');
    libraries.value = res.data || [];
  } catch {
    // ignore
  }
});

function handleLogout() {
  store.logout();
  router.push({ name: 'login' });
}
</script>

<style scoped>
.layout {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #18181b;
  color: #e4e4e7;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 48px;
  background: #27272a;
  border-bottom: 1px solid #3f3f46;
  flex-shrink: 0;
}

.brand {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.5px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.username {
  font-size: 13px;
  color: #a1a1aa;
}

.btn-logout {
  background: #3f3f46;
  color: #e4e4e7;
  border: none;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
}

.btn-logout:hover {
  background: #52525b;
}

.body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.sidebar {
  width: 200px;
  min-width: 180px;
  background: #1a1a2e;
  border-right: 1px solid #3f3f46;
  flex-shrink: 0;
  padding-top: 24px;
  display: flex;
  flex-direction: column;
}

.sidebar a,
.nav-link {
  display: block;
  padding: 12px 20px;
  color: #8888aa;
  text-decoration: none;
  font-size: 14px;
}

.sidebar a:hover,
.nav-link:hover {
  color: #e0e0e0;
}

.sidebar a.router-link-active,
.nav-link.router-link-active {
  color: #fff;
  background: #14142a;
  border-left: 2px solid rgba(108, 99, 255, 0.4);
  padding-left: 18px;
}

.sidebar-spacer {
  flex: 1;
}

.admin-link {
  color: #6c63ff !important;
}

.library-section {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #1a1a2e;
}

.section-label {
  color: #444466;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 1px;
  padding: 0 20px;
  margin-bottom: 8px;
}

.library-icon {
  margin-right: 6px;
  font-size: 13px;
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}
</style>