import App from "./App.vue";
import { createApp } from "vue";
import * as Vue from "vue";
import { createPinia } from "pinia";
import Element from "element-plus";
import { elementIconRegister } from "./plugins/elementIcon";
import router from "./router";
import "element-plus/dist/index.css";
import "./styles/el-tag-mimic.css";
import "./styles/rounded-borders.css";
import "./styles/scroll-text-left.css";
import "./styles/scroll-text-center.css";
import "./styles/z-axis-layers.css";
import clickOutSide from "./directives/clickOutSide";
import elSelectBottomed from "./directives/elSelectBottomed";
import elScrollbarBottomed from "./directives/elScrollbarBottomed";
import { initListener } from "./MainIpcListener";
import { initBuiltinMenus } from "./composables/useBuiltinMenus";
import { setRouterInstance } from "./store/SlotRegistryStore";
import lodash from "lodash";

const app = createApp(App);
const pinia = createPinia();
app.use(Element);
app.use(pinia);
app.use(router);

// 全局注册 el-icon
elementIconRegister(app);

// 注册点击外部事件的自定义指令
app.directive("clickOutSide", clickOutSide);

// 注册el-select触底的自定义指令
app.directive("elSelectBottomed", elSelectBottomed);

// 注册el-scrollbar触底的自定义指令
app.directive("elScrollbarBottomed", elScrollbarBottomed);

// 暴露 router 实例到全局
app.config.globalProperties.$router = router;
(window as any)["__vueRouter__"] = router;

// 设置 router 实例到 store
setRouterInstance(router);

// 构建插件的上下文对象
const pluginContext = {
  // --- Vue Core ---
  vue: Vue,

  // --- Globals (从 app 实例提取) ---
  globals: {
    $message: app.config.globalProperties.$message,
    $notify: app.config.globalProperties.$notify,
    $confirm: app.config.globalProperties.$confirm,
    $alert: app.config.globalProperties.$alert,
    $router: router,
  },

  // --- Third-party Libs ---
  libs: {
    lodash: lodash,
  },

  // --- Custom Business Logic ---
  custom: {},
};

// 插件的上下文暴露到 window
(window as any)["__PLUGIN_CTX__"] = pluginContext;

app.mount("#app");

// 初始化内置菜单
initBuiltinMenus();

// 初始化 IPC 监听器
initListener();
