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
          <el-button type="success" :loading="gardenLoading" @click="addToGarden">🌱 加入我的花园</el-button>
        </div>
        <el-alert
          v-if="gardenResult"
          class="garden-result"
          :type="gardenResultAlertType"
          :title="gardenResultTitle"
          show-icon
          :closable="false"
        >
          <p v-if="gardenResult.water_plan_status === 'scheduled'">
            首次浇水提醒日期：{{ formatDate(gardenResult.first_water_date) }}
          </p>
          <p v-else>浇水计划待设置：无法识别该品种的浇水频率（{{ plant.water_frequency || '未填写' }}），可在花园中手动创建提醒。</p>
        </el-alert>
      </el-card>
    </div>

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
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getPlant } from '@/api/plant'
import { listPests } from '@/api/pest'
import { addGarden } from '@/api/garden'
import { useAuth } from '@/hooks/useAuth'
import ImageCarousel from '@/components/common/ImageCarousel.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import DiseaseCard from '@/components/common/DiseaseCard.vue'
import { PlantTypeMap, type PlantSpecies } from '@/constants/plant'
import { formatDate } from '@/utils/dateFormat'
import type { DiseasePest, GardenAddResult } from '@/types/api'

const route = useRoute()
const router = useRouter()
const { isLoggedIn } = useAuth()
const plant = ref<PlantSpecies | null>(null)
const pests = ref<DiseasePest[]>([])
const gardenLoading = ref(false)
const gardenResult = ref<GardenAddResult | null>(null)

const gardenResultAlertType = computed(() => {
  const r = gardenResult.value
  if (!r) return 'info'
  if (!r.created) return 'info'
  return r.water_plan_status === 'scheduled' ? 'success' : 'warning'
})
const gardenResultTitle = computed(() =>
  gardenResult.value?.created ? '入圃成功' : '该植物已在花园中，重复入圃已忽略',
)

onMounted(async () => {
  plant.value = await getPlant(route.params.id as string)
  pests.value = (await listPests({ plant_species_id: plant.value.id, page_size: 20 })).list
})

async function addToGarden() {
  if (!isLoggedIn.value) {
    ElMessage.warning('请先登录')
    router.push('/login')
    return
  }
  gardenLoading.value = true
  try {
    const res = await addGarden({ plant_species_id: plant.value!.id, nickname: plant.value!.name })
    gardenResult.value = res
    if (!res.created) {
      ElMessage.info('该植物已在花园中')
    } else if (res.water_plan_status === 'scheduled') {
      ElMessage.success('已加入我的花园')
    } else {
      ElMessage.warning('已加入我的花园，浇水计划待设置')
    }
  } finally {
    gardenLoading.value = false
  }
}
</script>

<style scoped>
.page { max-width: 1200px; margin: 0 auto; }
.detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-top: 16px; }
@media (max-width: 768px) { .detail-grid { grid-template-columns: 1fr; } }
.alias { color: #999; }
.desc { margin-top: 12px; line-height: 1.6; }
.actions { margin-top: 16px; display: flex; gap: 12px; }
.garden-result { margin-top: 16px; }
</style>
