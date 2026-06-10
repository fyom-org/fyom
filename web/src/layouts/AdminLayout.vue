<template>
  <div class="admin-layout">
    <aside class="admin-sidebar">
      <div class="brand">fyom admin</div>
      <nav class="admin-nav">
        <router-link to="/admin/import" class="nav-link">Import</router-link>
        <router-link to="/admin/providers" class="nav-link">Providers</router-link>
        <router-link to="/admin/system" class="nav-link">System</router-link>
      </nav>
      <div class="sidebar-bottom">
        <router-link to="/" class="nav-link back-link">← Back to Library</router-link>
        <div class="user-info">
          <span class="username">{{ username }}</span>
          <button class="logout-btn" @click="logout">Logout</button>
        </div>
      </div>
    </aside>
    <main class="admin-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';

const router = useRouter();
const username = ref('');

onMounted(async () => {
  try {
    const res = await fetch('/api/v1/auth/me', {
      headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
    });
    if (res.ok) {
      const data = await res.json();
      username.value = data.data?.username || '';
    }
  } catch {
    // ignore
  }
});

function logout() {
  localStorage.removeItem('token');
  localStorage.removeItem('role');
  router.push('/login');
}
</script>

<style scoped>
.admin-layout {
  display: flex;
  min-height: 100vh;
  background: #0a0a14;
}
.admin-sidebar {
  width: 220px;
  background: #0e0e1a;
  border-right: 1px solid #1a1a2e;
  display: flex;
  flex-direction: column;
  padding: 20px 0;
}
.brand {
  font-size: 16px;
  font-weight: 800;
  color: #6c63ff;
  padding: 0 20px;
  margin-bottom: 24px;
  letter-spacing: 1px;
  text-transform: uppercase;
}
.admin-nav {
  flex: 1;
}
.nav-link {
  display: block;
  padding: 10px 20px;
  color: #666688;
  text-decoration: none;
  font-size: 13px;
  transition: all 0.15s;
}
.nav-link:hover {
  color: #aaaacc;
  background: #12121e;
}
.nav-link.router-link-active {
  color: #6c63ff;
  border-left: 3px solid #6c63ff;
  padding-left: 17px;
  background: #12121e;
}
.sidebar-bottom {
  border-top: 1px solid #1a1a2e;
  padding-top: 12px;
  margin-top: auto;
}
.back-link {
  color: #555577 !important;
  font-size: 12px;
}
.user-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 20px;
}
.username {
  color: #555577;
  font-size: 12px;
}
.logout-btn {
  background: none;
  border: none;
  color: #555577;
  font-size: 12px;
  cursor: pointer;
}
.logout-btn:hover {
  color: #ff6b6b;
}
.admin-content {
  flex: 1;
  overflow-y: auto;
  padding: 32px;
}
</style>
