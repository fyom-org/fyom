<template>
  <div class="modal-overlay" @click.self="preventClose">
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
      <div class="modal-header">
        <h2 id="modal-title">Password Change Required</h2>
      </div>
      <div class="modal-body">
        <p class="modal-message">
          You must change your password before continuing.
          This is required because your account was created with a temporary password.
        </p>

        <form @submit.prevent="handleSubmit" class="password-form">
          <div class="field">
            <label for="newPassword">New Password</label>
            <input
              id="newPassword"
              v-model="newPassword"
              type="password"
              required
              autocomplete="new-password"
              :disabled="loading"
            />
          </div>

          <div class="field">
            <label for="confirmPassword">Confirm New Password</label>
            <input
              id="confirmPassword"
              v-model="confirmPassword"
              type="password"
              required
              autocomplete="new-password"
              :disabled="loading"
            />
          </div>

          <p v-if="error" class="error">{{ error }}</p>

          <button type="submit" class="submit-btn" :disabled="loading">
            {{ loading ? 'Changing...' : 'Change Password' }}
          </button>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useUserStore } from '@/stores/user';

const store = useUserStore();

const newPassword = ref('');
const confirmPassword = ref('');
const loading = ref(false);
const error = ref('');

function preventClose() {
  // Intentionally empty - prevents closing by clicking overlay
}

async function handleSubmit() {
  error.value = '';

  if (!newPassword.value || !confirmPassword.value) {
    error.value = 'Fill all fields';
    return;
  }

  if (newPassword.value !== confirmPassword.value) {
    error.value = 'Passwords do not match';
    return;
  }

  if (newPassword.value.length < 8) {
    error.value = 'Password must be at least 8 characters';
    return;
  }

  loading.value = true;
  try {
    await store.updatePassword('', newPassword.value);
    // store.updatePassword will update the user state from backend response
    // which will clear requiresPasswordChange
  } catch (err) {
    console.error('[fyom] password change failed:', err);
    error.value = err instanceof Error ? err.message : 'Password change failed';
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
  animation: fadeIn 0.2s ease-out;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.modal {
  background: #1a1a2e;
  border-radius: 12px;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5);
  animation: slideUp 0.3s ease-out;
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.modal-header {
  padding: 24px 24px 0;
  border-bottom: 1px solid #2a2a3e;
  margin-bottom: 20px;
}

.modal-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #e0e0e0;
}

.modal-body {
  padding: 0 24px 24px;
}

.modal-message {
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.5;
  margin: 0 0 24px;
}

.password-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  outline: none;
  box-sizing: border-box;
}

.field input:focus {
  border-color: #6c63ff;
}

.field input:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.error {
  color: #ff6b6b;
  font-size: 13px;
  margin: 0;
}

.submit-btn {
  width: 100%;
  padding: 12px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  margin-top: 8px;
}

.submit-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.submit-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>