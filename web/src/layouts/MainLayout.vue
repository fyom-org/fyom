<template>
  <div class="layout">
    <div class="body">
      <aside
        class="sidebar"
        :class="{
          'sidebar-mobile': isMobile,
          'sidebar-hidden': !sidebarOpen,
        }"
        aria-label="Main navigation"
      >
        <div class="sidebar-content">
          <router-link
            to="/"
            class="nav-link"
            exact-active-class="router-link-active"
            @click="handleNavClick"
          >
            Home
          </router-link>

          <router-link to="/library" class="nav-link" @click="handleNavClick">
            Library
          </router-link>

          <router-link to="/profile" class="nav-link" @click="handleNavClick">
            Profile
          </router-link>

          <router-link
            v-if="isAdmin"
            to="/admin/libraries"
            class="nav-link admin-link"
            @click="handleNavClick"
          >
            Admin
          </router-link>

          <div v-if="libraries.length >= 2" class="library-section">
            <div class="section-label">Libraries</div>

            <router-link
              v-for="library in libraries"
              :key="library.id"
              :to="`/library/${library.id}`"
              class="nav-link library-link"
              @click="handleNavClick"
            >
              <span class="library-icon" aria-hidden="true">
                {{ libraryIcon(library.type) }}
              </span>
              <span class="library-name">
                {{ library.name }}
              </span>
            </router-link>
          </div>
        </div>
      </aside>

      <div
        v-if="isMobile && sidebarOpen"
        class="sidebar-overlay"
        aria-hidden="true"
        @click="closeSidebar"
      ></div>

      <button
        type="button"
        class="floating-sidebar-toggle"
        :aria-expanded="sidebarOpen"
        aria-label="Toggle navigation"
        @click="toggleSidebar"
      >
        ☰
      </button>

      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { getLibraries, type Library } from '@/api/library';
import { useUserStore } from '@/stores/user';

const MOBILE_BREAKPOINT = 768;

const router = useRouter();
const userStore = useUserStore();

const libraries = ref<Library[]>([]);
const isMobile = ref(getInitialIsMobile());
const sidebarOpen = ref(!isMobile.value);
const loggingOut = ref(false);

const isAdmin = computed(() => userStore.isAdmin);

const username = computed(() => {
  return userStore.user?.username?.trim() || 'User';
});

const roleLabel = computed(() => {
  return userStore.user?.role || 'member';
});

onMounted(() => {
  window.addEventListener('resize', handleResize);

  void loadLibraries();
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
});

function getInitialIsMobile(): boolean {
  if (typeof window === 'undefined') return false;

  return window.innerWidth <= MOBILE_BREAKPOINT;
}

function toggleSidebar(): void {
  sidebarOpen.value = !sidebarOpen.value;
}

function closeSidebar(): void {
  sidebarOpen.value = false;
}

function handleNavClick(): void {
  if (isMobile.value) {
    closeSidebar();
  }
}

function handleResize(): void {
  const nextIsMobile = window.innerWidth <= MOBILE_BREAKPOINT;
  const changedMode = nextIsMobile !== isMobile.value;

  isMobile.value = nextIsMobile;

  if (changedMode) {
    sidebarOpen.value = !nextIsMobile;
  }
}

async function loadLibraries(): Promise<void> {
  try {
    libraries.value = await getLibraries();
  } catch {
    // Library switcher is optional. Main layout should not block navigation.
    libraries.value = [];
  }
}

function libraryIcon(type: string): string {
  switch (type) {
    case 'movie':
      return '🎬';
    case 'show':
      return '📺';
    default:
      return '📁';
  }
}
</script>

<style scoped>
.layout {
  height: 100vh;
  display: flex;
  flex-direction: column;
  color: var(--color-text);
  background: var(--color-bg);
}

.body {
  position: relative;
  flex: 1;
  display: flex;
  min-height: 0;
  overflow: hidden;
}

.sidebar {
  width: 180px;
  min-width: 180px;
  z-index: 100;
  display: flex;
  flex-shrink: 0;
  flex-direction: column;
  background: #1a1a2e;
  border-right: 1px solid var(--color-border);
  transition:
    width 0.2s ease,
    min-width 0.2s ease,
    transform 0.2s ease;
}

.sidebar-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.nav-link {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  box-sizing: border-box;
  padding: var(--spacing-xs) var(--spacing-sm);
  color: #8888aa;
  border-left: 2px solid transparent;
  border-radius: 0 6px 6px 0;
  text-decoration: none;
  font-size: var(--font-size-sm);
  line-height: 1.35;
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    border-color 0.15s ease;
}

.nav-link:hover {
  color: var(--color-text);
  background: #12121e;
}

.nav-link.router-link-active,
.nav-link.router-link-exact-active {
  color: #fff;
  background: #14142a;
  border-left-color: rgb(108 99 255 / 65%);
}

.admin-link {
  color: var(--color-primary);
}

.library-section {
  margin-top: var(--spacing-sm);
  padding-top: var(--spacing-sm);
  border-top: 1px solid #252542;
}

.section-label {
  margin-bottom: var(--spacing-xs);
  padding: 0 var(--spacing-sm);
  color: #555577;
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.library-link {
  min-width: 0;
}

.library-icon {
  flex: 0 0 auto;
  font-size: var(--font-size-sm);
}

.library-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-footer {
  padding: 12px var(--spacing-sm);
  border-top: 1px solid #252542;
}

.user-summary {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 8px;
  padding: 0 var(--spacing-sm);
}

.username {
  color: #aaaacc;
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

.floating-sidebar-toggle {
  position: fixed;
  left: 0;
  bottom: 0;
  z-index: 101;
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text);
  background: rgb(26 26 46 / 88%);
  border: 1px solid rgb(255 255 255 / 7%);
  border-left: 0;
  border-bottom: 0;
  border-radius: 0 6px 0 0;
  box-shadow: 2px 0 8px rgb(0 0 0 / 24%);
  cursor: pointer;
  font-size: 1.35rem;
  transition:
    background-color 0.15s ease,
    color 0.15s ease;
}

.floating-sidebar-toggle:hover {
  color: #fff;
  background: rgb(108 99 255 / 88%);
}

.content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: var(--spacing-lg);
}

@media (min-width: 769px) {
  .sidebar.sidebar-hidden {
    width: 0;
    min-width: 0;
    overflow: hidden;
    border-right: 0;
  }
}

.sidebar-mobile {
  position: fixed;
  top: 0;
  left: 0;
  width: min(260px, 82vw);
  height: 100%;
  transform: translateX(-100%);
}

.sidebar-mobile:not(.sidebar-hidden) {
  transform: translateX(0);
}

.sidebar-overlay {
  position: fixed;
  inset: 0;
  z-index: 99;
  background: rgb(0 0 0 / 55%);
}

@media (max-width: 768px) {
  .content {
    padding: var(--spacing-sm);
  }
}

@media (prefers-reduced-motion: reduce) {
  .sidebar,
  .nav-link,
  .floating-sidebar-toggle {
    transition: none;
  }
}
</style>
