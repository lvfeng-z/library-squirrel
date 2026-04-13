import GotoPageConfig from "@renderer/model/util/GotoPageConfig";
import { getRouterInstance } from "@renderer/store/SlotRegistryStore";

export function askGotoPage(config: GotoPageConfig) {
  const router = getRouterInstance();
  if (!router) {
    console.error("Router instance not found");
    return;
  }

  if (config.params) {
    router.push({
      path: config.path,
      query: config.query,
      params: config.params,
    });
  } else if (config.query) {
    router.push({
      path: config.path,
      query: config.query,
    });
  } else {
    router.push(config.path!);
  }
}
