import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import { resolve } from "path";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), wails("./bindings")],
  resolve: {
    alias: {
      // 完整版（含运行时模板编译器）：插件前端扩展 vueSource 形态需在运行时编译
      // .vue 模板（useSlotSyncListener 的 loadVueSourceComponent），runtime-only
      // 构建中 compile 为告警桩、编译产物为空
      "vue": "vue/dist/vue.esm-bundler.js",
      "@renderer": resolve("src"),
      "@bindings": resolve("bindings"),
      "@apis": resolve("src/apis")
    },
  },
  server: {
    host: "127.0.0.1",
  },
});
