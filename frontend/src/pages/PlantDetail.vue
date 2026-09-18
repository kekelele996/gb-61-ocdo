<template>
  <div class="page" v-if="plant">
    <el-page-header @back="$router.back()" :content="plant.name" />
    <div class="detail-grid">
      <div>
        <ImageCarousel :image-urls="plant.image_urls" />
      </div>
      <el-card>
        <h1>{{ plant.name }} <el-tag>{{ PlantTypeMap[plant.type] }}</el-tag></h1>
        <p class="alias" v-if="plant.alias">别名：{{ plant.alias }}</p>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="科属">{{ plant.family }} · {{ plant.genus }}</el-descriptions-item>
          <el-descriptions-item label="原产地">{{ plant.origin }}</el-descriptions-item>
          <el-descriptions-item label="适宜温度">{{ plant.temp_min }}°C ~ {{ plant.temp_max }}°C</el-descriptions-item>
          <el-descriptions-item label="光照">{{ plant.light_requirement }}</el-descriptions-item>
          <el-descriptions-item label="浇水频率">{{ plant.water_frequency }}</el-descriptions-item>
        </el-descriptions>
        <p class="desc">{{ plant.description }}</p>
        <div class="actions">
          <FavoriteButton target-type="plant" :target-id="plant.id" />
          <el-button
            type="success"
            :loading="gardenLoading"
            :disabled="alreadyInGarden"
            @click="addToGarden"
          >{{ alreadyInGarden ? '🌱 已在我的花园' : '🌱 加入我的花园' }}</el-button>
        </div>
        <el-alert
          v-if="alreadyInGarden && enrolled"
          type="success"
          :closable="false"
          class="in-garden-tip"
          show-icon
        >
          <template #title>
            已在花园 · {{ enrolled.watering_plan_text }} · 首次浇水提醒：
            <strong>{{ enrolled.first_watering_date ? formatDate(enrolled.first_watering_date) : '待设置' }}</strong>
          </template>
        </el-alert>
      </el-card>
    </div>

    <el-dialog
      v-model="resultVisible"
      :title="enrolledDuplicated ? '该植物已在花园' : '入圃成功'"
      width="440px"
    >
      <el-descriptions v-if="enrolled" :column="1" border size="small">
        <el-descriptions-item label="植物">{{ enrolled.plant_name || plant?.name }}</el-descriptions-item>
        <el-descriptions-item label="品种浇水频率">
          {{ enrolled.watering_frequency_text || plant?.water_frequency || '未知' }}
        </el-descriptions-item>
        <el-descriptions-item label="养护计划">
          <el-tag :type="enrolled.first_watering_date ? 'success' : 'info'">
            {{ enrolled.watering_plan_text }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="首次浇水提醒日期">
          <span v-if="enrolled.first_watering_date">{{ formatDate(enrolled.first_watering_date) }}</span>
          <span v-else class="pending-text">待设置（不影响入圃，可稍后在花园中手动添加提醒）</span>
        </el-descriptions-item>
      </el-descriptions>
      <p class="result-tip">
        {{ enrolledDuplicated
          ? '同一植物只保留一条入圃记录，浇水提醒未重复创建。'
          : '花园记录与浇水提醒已一起生效，刷新或重复提交也只会保留这一条。' }}
      </p>
      <template #footer>
        <el-button @click="resultVisible = false">知道了</el-button>
        <el-button type="primary" @click="goGarden">去我的花园</el-button>
      </template>
    </el-dialog>

    <section v-if="pests.length">
      <h2>关联病虫害</h2>
      <el-row :gutter="16">
        <el-col v-for="p in pests" :key="p.id" :xs="24" :sm="12" :md="8">
          <DiseaseCard :pest="p" />
        </el-col>
      </el-row>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getPlant } from '@/api/plant'
import { listPests } from '@/api/pest'
import { addGarden, listGardens } from '@/api/garden'
import { useAuth } from '@/hooks/useAuth'
import ImageCarousel from '@/components/common/ImageCarousel.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import DiseaseCard from '@/components/common/DiseaseCard.vue'
import { PlantTypeMap, type PlantSpecies } from '@/constants/plant'
import { formatDate } from '@/utils/dateFormat'
import type { DiseasePest, GardenItem } from '@/types/api'

const route = useRoute()
const router = useRouter()
const { isLoggedIn } = useAuth()
const plant = ref<PlantSpecies | null>(null)
const pests = ref<DiseasePest[]>([])
const gardenLoading = ref(false)
const alreadyInGarden = ref(false)
const enrolled = ref<GardenItem | null>(null)
const enrolledDuplicated = ref(false)
const resultVisible = ref(false)

onMounted(async () => {
  plant.value = await getPlant(route.params.id as string)
  pests.value = (await listPests({ plant_species_id: plant.value.id, page_size: 20 })).list
  if (isLoggedIn.value) {
    try {
      const items = await listGardens()
      enrolled.value = items.find((g) => g.plant_species_id === plant.value!.id) ?? null
      alreadyInGarden.value = enrolled.value !== null
    } catch {
      // 花园预载失败不影响品种详情浏览
    }
  }
})

async function addToGarden() {
  if (!isLoggedIn.value) {
    ElMessage.warning('请先登录')
    router.push('/login')
    return
  }
  // 并发/连点去抖：进行中或已入圃直接忽略，保证一次生效。
  if (gardenLoading.value || alreadyInGarden.value) return
  gardenLoading.value = true
  try {
    const result = await addGarden({ plant_species_id: plant.value!.id, nickname: plant.value!.name })
    enrolledDuplicated.value = result.duplicated
    enrolled.value = result
    alreadyInGarden.value = true
    resultVisible.value = true
    ElMessage.success(result.duplicated ? '该植物已在花园中' : '已加入我的花园')
  } finally {
    gardenLoading.value = false
  }
}

function goGarden() {
  resultVisible.value = false
  router.push('/garden')
}
</script>

<style scoped>
.page { max-width: 1200px; margin: 0 auto; }
.detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-top: 16px; }
@media (max-width: 768px) { .detail-grid { grid-template-columns: 1fr; } }
.alias { color: #999; }
.desc { margin-top: 12px; line-height: 1.6; }
.actions { margin-top: 16px; display: flex; gap: 12px; }
.in-garden-tip { margin-top: 12px; }
.pending-text { color: #909399; }
.result-tip { margin: 12px 0 0; color: #606266; font-size: 13px; line-height: 1.6; }
</style>
