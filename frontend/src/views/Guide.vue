<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import {Discount, Link, List, Memo, Star, TakeawayBox, User} from '@element-plus/icons-vue'
import TourCenterPanel from '@renderer/components/tour/TourCenterPanel.vue'
</script>
<template>
  <base-view>
    <template #default>
      <div class="guide-container">
        <!-- 左：向导控制中心 -->
        <div class="guide-center">
          <TourCenterPanel />
        </div>
        <!-- 右：功能说明 -->
        <el-scrollbar class="guide-docs">
          <div class="guide-docs-content">
            <div id="summary">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><Memo /></el-icon>
                <span class="guide-tabs-label-text">软件概述</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  LibrarySquirrel是一款用于在个人电脑中创建并维护一个基于标签进行检索的资源库的软件，从远程站点下载资源到本地资源库中，并提供标签式的检索服务是这个软件的主要功能。
                  <br>
                  如果还不熟悉各项功能，可在左侧「向导中心」启动引导向导，跟随提示逐步操作。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="site">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><Link /></el-icon>
                <span class="guide-tabs-label-text">站点</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  指远程的作品源，如bilibili、pixiv等站点，站点标签、站点作者、作品、插件都要基于站点才能创建。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="work">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><Link /></el-icon>
                <span class="guide-tabs-label-text">作品</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  作品是本软件的核心数据，它是图片、视频、音频、文本等资源与其相关信息的集合，下面的所有功能都是为了作品的下载、存储和检索而服务。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="task">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><List /></el-icon>
                <span class="guide-tabs-label-text">任务</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  任务用于创建作品，要创建任务，可在任务页面输入受支持的url（默认支持本地文件路径和pixiv作品的url）进行创建。在创建任务之后，点击开始任务，
                  等待任务完成后，作品就保存到资源库中了。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="localAuthor">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-local"><User /></el-icon>
                <span class="guide-tabs-label-text">本地作者</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  本地作者即是本地创建的作者，用于统一作者在不同站点的身份（可以理解为同一个作者在不同站点创建的账号），以实现使用单个作者即可检索到同一作者在不同站点发布的作品。要实现这
                  种检索方式，你只需要把本地作者与站点作者进行关联，此时在使用本地作者搜索时，包含与本地作者关联的站点作者的作品也会被搜索到。
                  <br>
                  （例如：本地作者LA关联了站点作者SA，作品W包含作者SA，此时搜索包含LA的作品时，无论作品W是否包含作者LA，作品W都会被搜索到）
                </p>
              </div>
              <el-divider />
            </div>
            <div id="siteAuthor">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-site"><User /></el-icon>
                <span class="guide-tabs-label-text">站点作者</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  站点作者即来自站点的作者，通常会在使用插件下载站点作品时被添加，它的主要功能就是直接作为作者进行检索和与本地作者进行关联。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="localTag">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-local"><Discount /></el-icon>
                <span class="guide-tabs-label-text">本地标签</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  本地标签即是本地创建的标签，用于统一具有相同含义的站点标签（这些站点标签可能是同一站点中含义重复或相近的站点标签，也可能是不同站点中含义重复或相近的站点标签），以实现使
                  用单个标签即可检索到使用不同站点标签而标签含义却相同的作品。要实现这种检索方式，你只需要把本地标签与站点标签进行关联，此时在使用本地标签搜索时，包含与本地标签关联的站点
                  标签的作品也会被搜索到。
                  <br>
                  （例如：本地标签LT关联了站点标签ST，作品W包含ST标签，此时搜索包含LT的作品时，无论作品W是否包含LT标签，作品W都会被搜索到）
                </p>
              </div>
              <el-divider />
            </div>
            <div id="siteTag">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-site"><Discount /></el-icon>
                <span class="guide-tabs-label-text">站点标签</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  站点标签即来自站点的标签，通常会在使用插件下载站点作品时被添加，它的主要功能就是直接作为标签进行检索和与本地标签进行关联。
                </p>
              </div>
              <el-divider />
            </div>
            <div id="collection">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><Star /></el-icon>
                <span class="guide-tabs-label-text">收藏</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  功能开发中
                </p>
              </div>
              <el-divider />
            </div>
            <div id="plugin">
              <span class="guide-tabs-label">
                <el-icon class="guide-tabs-label-icon-normal"><TakeawayBox /></el-icon>
                <span class="guide-tabs-label-text">插件</span>
              </span>
              <div class="guide-container-tabs-content">
                <p class="guide-text-indent">
                  插件用于扩展本软件对于不同站点的作品下载的支持，软件预置了用于从本地导入作品和pixiv下载作品的两个插件
                </p>
              </div>
              <el-divider />
            </div>
          </div>
        </el-scrollbar>
      </div>
    </template>
  </base-view>
</template>

<style scoped>
.guide-container {
  background: #ffffff;
  border-radius: 6px;
  display: flex;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
  gap: 10px;
}

.guide-center {
  width: 340px;
  flex-shrink: 0;
}

.guide-docs {
  flex: 1;
  min-width: 0;
}

.guide-docs-content {
  margin-right: 10px;
}

.guide-container-tabs-content {
  display: flex;
  flex-direction: column;
  background: #f4f4f4;
  border-radius: 6px;
  width: calc(100% - 20px);
  padding: 5px;
  margin: 5px;
}

.guide-tabs-label {
  display: flex;
  align-items: center;
}

.guide-tabs-label-icon-normal {
  border-radius: 50% / 50%;
  background-color: rgb(224, 229, 237);
  padding: 2px;
}

.guide-tabs-label-icon-local {
  border-radius: 50% / 50%;
  background-color: rgba(245, 108, 108, 0.7);
  color: white;
  padding: 2px;
}

.guide-tabs-label-icon-site {
  border-radius: 50% / 50%;
  background-color: rgba(64, 158, 255, 0.65);
  color: white;
  padding: 2px;
}

.guide-tabs-label-text {
  border-radius: 5px;
  padding-left: 2px;
}

.guide-text-indent {
  text-indent: 2em;
}
</style>
