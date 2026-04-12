/**
 * LocalTag API - Wails bindings wrapper
 * Bridges the frontend API calls to Wails backend
 */

import { App } from "../../../bindings/github.com/library-squirrel/wails";

// Re-export types from bindings for convenience
export type { LocalTagQueryDTO } from "../../../bindings/github.com/library-squirrel/wails/internal/localTag/models";
export type { LocalTag, SelectItem } from "../../../bindings/github.com/library-squirrel/wails/internal/model/models";

/**
 * LocalTag API wrapper for Wails bindings
 */
export const LocalTagApi = {
  save: (tag: any): Promise<number> => {
    return App.LocalTagSave(tag);
  },

  deleteById: (id: number): Promise<void> => {
    return App.LocalTagDeleteById(id);
  },

  updateById: (tag: any): Promise<void> => {
    return App.LocalTagUpdateById(tag);
  },

  getById: (id: number): Promise<any> => {
    return App.LocalTagGetById(id);
  },

  queryPage: (query: any): Promise<any> => {
    return App.LocalTagQueryPage(query);
  },

  queryDTOPage: (query: any): Promise<any> => {
    return App.LocalTagQueryDTOPage(query);
  },

  getTree: (rootId?: number, depth?: number): Promise<any> => {
    return App.LocalTagGetTree(rootId ?? 0, depth ?? 1);
  },

  listSelectItems: (query?: any): Promise<any> => {
    return App.LocalTagListSelectItems(query);
  },

  querySelectItemPage: (query: any): Promise<any> => {
    return App.LocalTagQuerySelectItemPage(query);
  },

  listByWorkId: (workId: number): Promise<any> => {
    return App.LocalTagListByWorkId(workId);
  },

  querySelectItemPageByWorkId: (query: any, workId: number): Promise<any> => {
    return App.LocalTagQuerySelectItemPageByWorkId(query, workId);
  },
};
