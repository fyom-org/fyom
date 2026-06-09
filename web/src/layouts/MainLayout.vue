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
        <router-link to="/import">Import</router-link>
        <router-link to="/library">Library</router-link>
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
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const store = useUserStore()

function handleLogout() {
  store.logout()
  router.push({ name: 'login' })
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

.sidebar a {
  display: block;
  padding: 12px 20px;
  color: #8888aa;
  text-decoration: none;
  font-size: 14px;
}

.sidebar a:hover {
  color: #e0e0e0;
}

.sidebar a.router-link-active {
  color: #fff;
  border-left: 3px solid #6c63ff;
  padding-left: 17px;
}

.sidebar-spacer {
  flex: 1;
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}
</style>
