<script setup lang="ts">
import { ref } from 'vue'
import lodash from 'lodash'
import Site from '@renderer/model/model/entity/Site.ts'
import RankedSiteAuthor from '@renderer/model/model/domain/RankedSiteAuthor.ts'
import LocalTag from '@renderer/model/model/entity/LocalTag.ts'
import WorkFullDTO from '@renderer/model/model/dto/WorkFullDTO.ts'

// 变量
// 接口
const apis = {
  workSaveWork: window.api.workSaveWork,
  testTransactionTest: window.api.testTransactionTest
}
const site = ref(new Site())
const siteAuthor = ref(new RankedSiteAuthor())
const localTag = ref(new LocalTag())

// 方法
function saveWork() {
  const workFullDTO = new WorkFullDTO()
  workFullDTO.site = lodash.cloneDeep(site.value)
  const siteAuthors: RankedSiteAuthor[] = []
  siteAuthors.push(lodash.cloneDeep(siteAuthor.value))
  workFullDTO.siteAuthors = siteAuthors
  workFullDTO.localTags = lodash.cloneDeep([localTag.value])
  apis.workSaveWork(workFullDTO)
  // apis.testTransactionTest()
}
</script>

<template>
  <el-dialog>
    <el-form>
      <el-row>
        <el-col>
          <el-form-item label="站点域名">
            <el-input v-model="site.id"></el-input>
          </el-form-item>
        </el-col>
        <el-col>
          <el-form-item label="站点名称">
            <el-input v-model="site.siteName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="站点作者id">
            <el-input v-model="siteAuthor.siteAuthorId"></el-input>
          </el-form-item>
        </el-col>
        <el-col>
          <el-form-item label="站点作者名称">
            <el-input v-model="siteAuthor.authorName"></el-input>
          </el-form-item>
        </el-col>
        <el-col>
          <el-form-item label="站点作者所在站点Id">
            <el-input v-model="siteAuthor.siteId"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="本地标签id">
            <el-input v-model="localTag.id"></el-input>
          </el-form-item>
        </el-col>
        <el-col>
          <el-form-item label="本地标签名称">
            <el-input v-model="localTag.localTagName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>
    <el-button @click="saveWork">保存</el-button>
  </el-dialog>
</template>

<style scoped></style>
