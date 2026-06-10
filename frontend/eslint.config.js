import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import vueParser from 'vue-eslint-parser'
import globals from 'globals'

export default tseslint.config(
  // 全局忽略
  {
    ignores: [
      'node_modules',
      'dist',
      'bindings/**',           // Wails 自动生成的 TypeScript bindings
      'frontend/**',           // 嵌套的 Wails 生成的 frontend 目录
    ],
  },

  // JS 推荐规则
  js.configs.recommended,

  // 浏览器全局变量（console、setTimeout、window 等）
  {
    languageOptions: {
      globals: {
        ...globals.browser,
      },
    },
  },

  // TypeScript 推荐规则
  ...tseslint.configs.recommended,

  // Vue 3 推荐规则（flat config）
  ...pluginVue.configs['flat/recommended'],

  // Vue 文件：使用 vue-eslint-parser 解析，<script> 块委托给 TypeScript 解析器
  {
    files: ['**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tseslint.parser,
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.vue'],
      },
    },
  },

  // TypeScript 文件配置
  {
    files: ['**/*.ts'],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },

  // 规则放宽（适配现有代码库风格）
  {
    rules: {
      // 允许 _ 前缀的未使用变量
      '@typescript-eslint/no-unused-vars': ['warn', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        caughtErrorsIgnorePattern: '^_',
      }],

      // 允许 any 类型（匹配 tsconfig noImplicitAny: false）
      '@typescript-eslint/no-explicit-any': 'off',

      // 允许 require 风格导入
      '@typescript-eslint/no-require-imports': 'off',

      // 允许非空断言
      '@typescript-eslint/no-non-null-assertion': 'off',

      // 允许多词组件名以外的命名（项目已有单词组件名）
      'vue/multi-word-component-names': 'off',

      // 不强制 prop 默认值
      'vue/require-default-prop': 'off',

      // 允许 v-html
      'vue/no-v-html': 'off',
    },
  },
)
