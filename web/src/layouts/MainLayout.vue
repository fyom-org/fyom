<template>
  <div class="layout">
    <header class="header">
      <button class="sidebar-toggle" @click="toggleSidebar">☰</button>
      <span class="brand">fyom</span>
      <div class="header-right">
        <span class="username">{{ store.user?.username ?? '—' }}</span>
        <button class="btn-logout" @click="handleLogout">Logout</button>
      </div>
    </header>
    <div class="body">
      <aside
        class="sidebar"
        :class="{
          'sidebar-mobile': isMobile,
          'sidebar-hidden': !sidebarOpen,
        }"
      >
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
            @click="sidebarOpen = false"
          >
            <span class="library-icon">
              {{ lib.type === 'movie' ? '🎬' : lib.type === 'show' ? '📺' : '📁' }}
            </span>
            {{ lib.name }}
          </router-link>
        </div>
        <router-link
          v-if="isAdmin"
          to="/admin"
          class="nav-link admin-link"
          @click="sidebarOpen = false"
          >⚙ Admin</router-link
        >
        <div class="sidebar-spacer"></div>
        <router-link to="/profile" @click="sidebarOpen = false">Profile</router-link>
      </aside>
      <div
        class="sidebar-overlay"
        v-if="isMobile && sidebarOpen"
        @click="sidebarOpen = false"
      ></div>
      <main class="content">
        <router-view />
      </main>
    </div>
    <div class="mobile-nav" v-if="isMobile">
      <router-link to="/" class="mobile-nav-item">
        <span class="mobile-nav-icon">🏠</span>
        <span class="mobile-nav-label">Home</span>
      </router-link>
      <router-link to="/library" class="mobile-nav-item">
        <span class="mobile-nav-icon">📚</span>
        <span class="mobile-nav-label">Library</span>
      </router-link>
      <router-link to="/profile" class="mobile-nav-item">
        <span class="mobile-nav-icon">👤</span>
        <span class="mobile-nav-label">Profile</span>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue';
import { useRouter } from 'vue-router';
import { useUserStore } from '@/stores/user';
import request from '@/api/request';

const router = useRouter();
const store = useUserStore();
const libraries = ref<any[]>([]);
const isMobile = ref(window.innerWidth <= 768);
const sidebarOpen = ref(!isMobile.value); // Desktop: default open, Mobile: default closed

const isAdmin = computed(() => store.isAdmin);

const toggleSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value;
};

const handleResize = () => {
  isMobile.value = window.innerWidth <= 768;
  sidebarOpen.value = !isMobile.value; // Desktop: open, Mobile: closed
};

onMounted(async () => {
  window.addEventListener('resize', handleResize);
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

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
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
  background: var(--color-bg);
  color: var(--color-text);
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-md);
  height: var(--touch-target);
  background: #1e1e2e;
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.sidebar-toggle {
  background: none;
  border: none;
  color: var(--color-text);
  font-size: 1.5rem;
  cursor: pointer;
  padding: 0 var(--spacing-sm);
  margin-right: var(--spacing-sm);
}

.brand {
  font-size: var(--font-size-lg);
  font-weight: 700;
  letter-spacing: 0.5px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.username {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.btn-logout {
  background: var(--color-border);
  color: var(--color-text);
  border: none;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.btn-logout:hover {
  background: #52525b;
}

.body {
  display: flex;
  flex: 1;
  overflow: hidden;
  position: relative;
}

.sidebar {
  width: 200px;
  min-width: 180px;
  background: #1a1a2e;
  border-right: 1px solid var(--color-border);
  flex-shrink: 0;
  padding-top: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  transition: transform 0.3s ease;
  z-index: 100;
}

/* Desktop: Collapse via width */
@media (min-width: 769px) {
  .sidebar.sidebar-hidden {
    width: 0;
    min-width: 0;
    overflow: hidden;
    border-right: none;
  }
}

/* Mobile: Slide in/out */
.sidebar-mobile {
  position: fixed;
  top: 0;
  left: 0;
  height: 100%;
  transform: translateX(-100%);
}

.sidebar-hidden {
  transform: translateX(-100%);
}

.sidebar-mobile:not(.sidebar-hidden) {
  transform: translateX(0);
}

.sidebar-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 99;
}

.sidebar a,
.nav-link {
  display: block;
  padding: var(--spacing-sm) var(--spacing-md);
  color: #8888aa;
  text-decoration: none;
  font-size: var(--font-size-md);
}

.sidebar a:hover,
.nav-link:hover {
  color: var(--color-text);
}

.sidebar a.router-link-active,
.nav-link.router-link-active {
  color: #fff;
  background: #14142a;
  border-left: 2px solid rgba(108, 99, 255, 0.4);
  padding-left: calc(var(--spacing-md) - 2px);
}

.sidebar-spacer {
  flex: 1;
}

.admin-link {
  color: var(--color-primary) !important;
}

.library-section {
  margin-top: var(--spacing-sm);
  padding-top: var(--spacing-sm);
  border-top: 1px solid #1a1a2e;
}

.section-label {
  color: #444466;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 1px;
  padding: 0 var(--spacing-md);
  margin-bottom: var(--spacing-sm);
}

.library-icon {
  margin-right: var(--spacing-sm);
  font-size: var(--font-size-sm);
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-lg);
}

@media (max-width: 768px) {
  .content {
    padding: var(--spacing-sm);
  }
}

.mobile-nav {
  display: none;
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background: #1a1a2e;
  border-top: 1px solid var(--color-border);
  z-index: 100;
  padding: var(--spacing-sm) 0;
}

@media (max-width: 768px) {
  .mobile-nav {
    display: flex;
    justify-content: space-around;
  }

  .mobile-nav-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    color: var(--color-text-secondary);
    text-decoration: none;
    font-size: var(--font-size-sm);
  }

  .mobile-nav-item.router-link-active {
    color: var(--color-primary);
  }

  .mobile-nav-icon {
    font-size: var(--font-size-lg);
    margin-bottom: 2px;
  }
}
</style>
