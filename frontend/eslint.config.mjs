import babelParser from '@babel/eslint-parser'
import js from '@eslint/js'
import reactHooks from 'eslint-plugin-react-hooks'
import globals from 'globals'

const sourceFiles = ['src/**/*.{js,jsx,ts,tsx}']
const toolingFiles = [
  'vite.config.ts',
  'scripts/generate-icons.mjs',
  'eslint.config.mjs',
  'lint.test.mjs',
]

export default [
  {
    name: 'generated-and-separate-tooling-tests',
    ignores: [
      'bindings/**',
      'dist/**',
      '**/node_modules/**',
      '*.test.mjs',
      '!lint.test.mjs',
    ],
  },
  {
    name: 'handwritten-code',
    files: [...sourceFiles, ...toolingFiles],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
    },
    linterOptions: {
      reportUnusedDisableDirectives: 'error',
    },
    rules: {
      ...js.configs.recommended.rules,
      eqeqeq: ['error', 'always', { null: 'ignore' }],
      'no-unreachable-loop': 'error',
      'no-var': 'error',
      'prefer-const': 'error',
    },
  },
  {
    name: 'browser-source',
    files: sourceFiles,
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
      },
    },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
    },
  },
  {
    name: 'node-tooling',
    files: toolingFiles,
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    name: 'typescript-syntax',
    files: ['src/**/*.{ts,tsx}', 'vite.config.ts'],
    languageOptions: {
      parser: babelParser,
      parserOptions: {
        requireConfigFile: false,
        babelOptions: {
          babelrc: false,
          configFile: false,
          parserOpts: { plugins: ['typescript'] },
        },
      },
    },
    // tsc understands type-only names, declaration merging, and overloads.
    rules: {
      'no-undef': 'off',
      'no-unused-vars': 'off',
      'no-redeclare': 'off',
      'no-dupe-class-members': 'off',
    },
  },
  {
    name: 'tsx-syntax',
    files: ['src/**/*.tsx'],
    languageOptions: {
      parserOptions: {
        babelOptions: {
          parserOpts: { plugins: ['typescript', 'jsx'] },
        },
      },
    },
  },
]
