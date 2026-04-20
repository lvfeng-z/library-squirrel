import IPage from "@renderer/model/util/IPage.ts";
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";

/**
 * 创建一个指定类型的副本
 */
export function copyPage<NewData, NewQuery>(source: IPage<unknown, unknown>): IPage<NewData, NewQuery> {
    const result = new Page<NewData, NewQuery>()
    result.pageNumber = source.pageNumber
    result.pageSize = source.pageSize
    result.pageCount = source.pageCount
    result.dataCount = source.dataCount
    result.currentCount = source.currentCount

    return result
}