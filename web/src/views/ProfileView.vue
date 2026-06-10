<template>
  <div class="profile-view">
    <h2 class="page-title">Profile</h2>
    <div v-if="user" class="card">
      <div class="info-row">
        <span class="label">Username</span><span>{{ user.username }}</span>
      </div>
      <div class="info-row">
        <span class="label">Role</span><span class="badge">{{ user.role }}</span>
      </div>
    </div>

    <div class="card">
      <h3>Change Password</h3>
      <div class="field">
        <label>Current Password</label>
        <input v-model="oldPassword" type="password" />
      </div>
      <div class="field">
        <label>New Password</label>
        <input v-model="newPassword" type="password" />
      </div>
      <p v-if="msg" class="msg">{{ msg }}</p>
      <p v-if="error" class="error">{{ error }}</p>
      <button class="btn" @click="changePassword">Update Password</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import request from '@/api/request';

const user = ref<{ username: string; role: string } | null>(null);
const oldPassword = ref('');
const newPassword = ref('');
const msg = ref('');
const error = ref('');

onMounted(async () => {
  const res = await request.get('/auth/me');
  user.value = res.data;
});

async function changePassword() {
  error.value = '';
  msg.value = '';
  try {
    await request.put('/auth/me/password', {
      old_password: oldPassword.value,
      new_password: newPassword.value,
    });
    msg.value = 'Password updated';
    oldPassword.value = '';
    newPassword.value = '';
  } catch (e: unknown) {
    const err = e as { response?: { data?: { message?: string } } };
    error.value = err.response?.data?.message || 'Failed';
  }
}
</script>

<style scoped>
.page-title {
  font-size: 22px;
  color: #e0e0e0;
  margin-bottom: 20px;
}

.card {
  background: #1a1a2e;
  padding: 24px;
  border-radius: 8px;
  margin-bottom: 20px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
  color: #aaaacc;
  font-size: 14px;
}

.label {
  color: #8888aa;
}

.badge {
  background: #2a2a3e;
  padding: 2px 8px;
  border-radius: 4px;
  text-transform: capitalize;
  color: #6c63ff;
}

h3 {
  margin: 0 0 16px;
  color: #d0d0d0;
  font-size: 16px;
}

.field label {
  display: block;
  color: #8888aa;
  font-size: 13px;
  margin-bottom: 6px;
}

.field input {
  width: 100%;
  padding: 10px 12px;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 14px;
  box-sizing: border-box;
}

.field + .field {
  margin-top: 12px;
}

.btn {
  margin-top: 16px;
  background: #6c63ff;
  color: #fff;
  border: none;
  padding: 10px 24px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.btn:hover {
  background: #5a52e0;
}

.msg {
  color: #4caf50;
  font-size: 13px;
  margin-top: 12px;
}

.error {
  color: #ff6b6b;
  font-size: 13px;
  margin-top: 12px;
}
</style>
