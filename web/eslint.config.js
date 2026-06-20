import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import pluginVue from 'eslint-plugin-vue';
import eslintConfigPrettier from 'eslint-config-prettier';
import globals from 'globals';

export default tseslint.config(
  // Ignore build output and dependencies
  {
    ignores: ['dist/**', 'node_modules/**'],
  },

  // Base JS rules
  js.configs.recommended,

  // TypeScript rules
  ...tseslint.configs.recommended,

  // Vue rules (includes vue-eslint-parser as the main parser)
  ...pluginVue.configs['flat/recommended'],

  // Tell vue-eslint-parser to use @typescript-eslint/parser for <script lang="ts"> blocks
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },

  // Project-specific rule overrides
  {
    languageOptions: {
      globals: {
        ...globals.browser,
      },
    },
    files: ['**/*.{ts,vue}'],
    rules: {
      // Single-word component names are used in this project (e.g. LibraryView)
      'vue/multi-word-component-names': 'off',
      // Allow unused vars prefixed with _ (matches Go convention used in backend)
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },

  // Node.js scripts: enable node globals and allow console/process usage
  {
    files: ['scripts/**/*.js'],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
  },

  // Test files: relaxed rules for mock/error construction
  {
    files: ['tests/**/*.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },

  // Disable ESLint formatting rules that conflict with Prettier (must be last)
  eslintConfigPrettier,
)
