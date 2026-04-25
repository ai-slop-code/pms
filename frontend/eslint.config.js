import js from '@eslint/js'
import globals from 'globals'
import tsParser from '@typescript-eslint/parser'
import tsPlugin from '@typescript-eslint/eslint-plugin'
import vuePlugin from 'eslint-plugin-vue'
import vueParser from 'vue-eslint-parser'
import prettier from 'eslint-config-prettier'

export default [
  {
    ignores: ['dist/**', 'node_modules/**', 'coverage/**'],
  },
  js.configs.recommended,
  {
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: 'module',
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
  },
  // TypeScript files.
  {
    files: ['**/*.ts', '**/*.tsx'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 2023,
        sourceType: 'module',
      },
      globals: {
        ...globals.browser,
      },
    },
    plugins: {
      '@typescript-eslint': tsPlugin,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/no-explicit-any': 'warn',
      'no-unused-vars': 'off',
      // TS handles the DOM lib types; don't double-check with ESLint's no-undef.
      'no-undef': 'off',
    },
  },
  // Vue single-file components.
  ...vuePlugin.configs['flat/recommended'],
  {
    files: ['**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tsParser,
        ecmaVersion: 2023,
        sourceType: 'module',
        extraFileExtensions: ['.vue'],
      },
    },
    plugins: {
      '@typescript-eslint': tsPlugin,
    },
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      'no-unused-vars': 'off',
      'no-undef': 'off',
      'vue/multi-word-component-names': 'off',
      // We use the 5.3+ shorthand, keep enforcement predictable.
      'vue/attributes-order': 'warn',
      // Vue 3 TS SFCs with defineProps<{ ariaLabel: string }> only accept the
      // camelCase form in <template>. Leave prop-name casing to authors; the
      // camelCase↔kebab binding is enforced by the compiler anyway.
      'vue/attribute-hyphenation': 'off',
      'vue/v-on-event-hyphenation': 'off',
      'vue/html-self-closing': [
        'warn',
        {
          html: { void: 'never', normal: 'any', component: 'always' },
        },
      ],
    },
  },
  // Project-wide rules (apply after plugin presets so they win).
  {
    rules: {
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'no-debugger': 'warn',
      // Block regressions on Phase 2 §3.4 — no native dialogs.
      'no-restricted-globals': [
        'error',
        { name: 'confirm', message: 'Use useConfirm() from @/composables/useConfirm instead.' },
        { name: 'prompt', message: 'Use a UiDialog with inputs instead of window.prompt.' },
        { name: 'alert', message: 'Use useToast() or UiInlineBanner instead of window.alert.' },
      ],
      'no-restricted-syntax': [
        'error',
        {
          selector: "MemberExpression[object.name='window'][property.name='confirm']",
          message: 'Use useConfirm() from @/composables/useConfirm instead of window.confirm.',
        },
        {
          selector: "MemberExpression[object.name='window'][property.name='prompt']",
          message: 'Use a UiDialog with inputs instead of window.prompt.',
        },
        {
          selector: "MemberExpression[object.name='window'][property.name='alert']",
          message: 'Use useToast() or UiInlineBanner instead of window.alert.',
        },
      ],
    },
  },
  // Tests & config files allow looser rules.
  {
    files: ['**/*.spec.ts', '**/*.test.ts', '**/vitest.setup.ts', 'vite.config.ts', 'eslint.config.js'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      'no-restricted-globals': 'off',
      'no-restricted-syntax': 'off',
      'vue/one-component-per-file': 'off',
    },
  },
  prettier,
]
