<template>
  <div>
    <div class="toolbar">
      <div>
        <h2 class="page-title">安全巡检</h2>
        <p class="page-sub">工地安全巡检轮次与检查条目；风险轮次醒目标记</p>
      </div>
      <button class="btn" @click="openCreate">新增巡检</button>
    </div>

    <div class="card">
      <div class="filters">
        <label>
          工地筛选
          <select v-model="filterSiteId" @change="load">
            <option value="">全部工地</option>
            <option v-for="s in sites" :key="s.id" :value="String(s.id)">{{ s.name }}</option>
          </select>
        </label>
      </div>

      <table class="table">
        <thead>
          <tr>
            <th>巡检日期</th>
            <th>工地</th>
            <th>巡检人</th>
            <th>天气</th>
            <th>结论</th>
            <th>不合格项</th>
            <th>概述</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in list" :key="item.id" :class="{ 'row-risk': item.conclusion === 'risk' }">
            <td>{{ formatDate(item.roundDate) }}</td>
            <td>{{ item.site?.name || '-' }}</td>
            <td>{{ item.inspector }}</td>
            <td>{{ item.weather || '-' }}</td>
            <td>
              <span v-if="item.conclusion === 'risk'" class="badge badge-risk">⚠ 风险</span>
              <span v-else class="badge badge-ok">合格</span>
            </td>
            <td>
              <span v-if="failCount(item) > 0" class="badge badge-risk">{{ failCount(item) }} 项不合格</span>
              <span v-else>-</span>
            </td>
            <td class="summary-cell">{{ item.summary || '-' }}</td>
            <td>
              <button class="btn secondary small" @click="openEdit(item)">编辑</button>
              <button class="btn danger small" @click="remove(item)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!list.length" class="page-sub">暂无巡检记录</p>
      <p v-if="error" class="error">{{ error }}</p>
    </div>

    <div v-if="showModal" class="modal-mask" @click.self="showModal = false">
      <div class="modal modal-wide">
        <h3>
          {{ form.id ? '编辑巡检轮次' : '新增巡检轮次' }}
          <span v-if="form.conclusion === 'risk'" class="badge badge-risk title-badge">⚠ 风险</span>
        </h3>
        <div class="form-grid">
          <label>
            所属工地
            <select v-model.number="form.siteId">
              <option :value="0" disabled>请选择</option>
              <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </label>
          <label>
            巡检日期
            <input v-model="form.roundDate" type="date" />
          </label>
          <label>
            巡检人
            <input v-model="form.inspector" placeholder="必填" />
          </label>
          <label>
            天气
            <input v-model="form.weather" placeholder="晴 / 多云 / 雨…" />
          </label>
          <label class="full">
            结论
            <select v-model="form.conclusion">
              <option value="ok">合格（ok）</option>
              <option value="risk">风险（risk）</option>
            </select>
          </label>
          <label class="full">
            巡检概述
            <textarea v-model="form.summary" />
          </label>
        </div>

        <div class="items-head">
          <h4>检查条目（同轮条目编号唯一；结论由后端强校验）</h4>
          <button class="btn secondary small" @click="addItem">添加条目</button>
        </div>
        <table class="table items-table">
          <thead>
            <tr>
              <th class="col-code">条目编号</th>
              <th class="col-result">结果</th>
              <th>备注</th>
              <th class="col-act"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(it, idx) in form.items" :key="idx" :class="{ 'item-fail': it.result === 'fail' }">
              <td><input v-model="it.itemCode" placeholder="如 S-01" /></td>
              <td>
                <select v-model="it.result">
                  <option value="pass">合格 pass</option>
                  <option value="fail">不合格 fail</option>
                  <option value="na">不适用 na</option>
                </select>
              </td>
              <td><input v-model="it.comment" placeholder="现场情况说明" /></td>
              <td>
                <button class="btn danger small" @click="form.items.splice(idx, 1)">移除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="!form.items.length" class="page-sub">暂无条目，请至少添加一条</p>

        <p v-if="formError" class="error form-error">{{ formError }}</p>
        <div class="modal-actions">
          <button class="btn secondary" @click="showModal = false">取消</button>
          <button class="btn" @click="save">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api/http'

const route = useRoute()
const list = ref([])
const sites = ref([])
const filterSiteId = ref(route.query.siteId ? String(route.query.siteId) : '')
const error = ref('')
const formError = ref('')
const showModal = ref(false)

const form = reactive({
  id: null,
  siteId: 0,
  roundDate: '',
  inspector: '',
  weather: '',
  conclusion: 'ok',
  summary: '',
  items: []
})

function formatDate(v) {
  if (!v) return '-'
  return String(v).slice(0, 10)
}

function failCount(round) {
  return (round.items || []).filter((i) => i.result === 'fail').length
}

async function loadSites() {
  const { data } = await api.get('/sites')
  sites.value = data
}

async function load() {
  error.value = ''
  try {
    const params = {}
    if (filterSiteId.value) params.siteId = filterSiteId.value
    const { data } = await api.get('/safety-rounds', { params })
    list.value = data
  } catch (e) {
    error.value = e.response?.data?.error || '加载失败'
  }
}

function blankItem() {
  return { itemCode: '', result: 'pass', comment: '' }
}

function openCreate() {
  Object.assign(form, {
    id: null,
    siteId: filterSiteId.value ? Number(filterSiteId.value) : sites.value[0]?.id || 0,
    roundDate: '',
    inspector: '',
    weather: '',
    conclusion: 'ok',
    summary: '',
    items: [blankItem()]
  })
  formError.value = ''
  showModal.value = true
}

function openEdit(item) {
  Object.assign(form, {
    id: item.id,
    siteId: item.siteId,
    roundDate: formatDate(item.roundDate) === '-' ? '' : formatDate(item.roundDate),
    inspector: item.inspector || '',
    weather: item.weather || '',
    conclusion: item.conclusion,
    summary: item.summary || '',
    items: (item.items || []).map((i) => ({
      itemCode: i.itemCode,
      result: i.result,
      comment: i.comment || ''
    }))
  })
  if (!form.items.length) form.items.push(blankItem())
  formError.value = ''
  showModal.value = true
}

function addItem() {
  form.items.push(blankItem())
}

async function save() {
  formError.value = ''
  const payload = {
    siteId: form.siteId,
    roundDate: form.roundDate || null,
    inspector: form.inspector,
    weather: form.weather,
    conclusion: form.conclusion,
    summary: form.summary,
    // 仅提交条目编号非空的行，业务约束（必填/唯一/ok-fail）由后端强校验拦截
    items: form.items
      .filter((i) => i.itemCode.trim() !== '')
      .map((i) => ({ itemCode: i.itemCode.trim(), result: i.result, comment: i.comment }))
  }
  try {
    if (form.id) {
      await api.put(`/safety-rounds/${form.id}`, payload)
    } else {
      await api.post('/safety-rounds', payload)
    }
    showModal.value = false
    await load()
  } catch (e) {
    formError.value = e.response?.data?.error || '保存失败'
  }
}

async function remove(item) {
  if (!confirm('确认删除该巡检轮次及其全部条目？')) return
  try {
    await api.delete(`/safety-rounds/${item.id}`)
    await load()
  } catch (e) {
    alert(e.response?.data?.error || '删除失败')
  }
}

onMounted(async () => {
  await loadSites()
  await load()
})

// 从 Sites 页跳入（/safety?siteId=x）且已在本页时，响应筛选变化
watch(
  () => route.query.siteId,
  (v) => {
    filterSiteId.value = v ? String(v) : ''
    load()
  }
)
</script>

<style scoped>
.filters {
  display: flex;
  gap: 1rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}

.filters label {
  min-width: 220px;
}

.badge {
  display: inline-block;
  padding: 0.18rem 0.6rem;
  border-radius: 999px;
  font-size: 0.8rem;
  font-weight: 600;
  white-space: nowrap;
}

.badge-ok {
  background: #e3efe8;
  color: var(--ok);
}

.badge-risk {
  background: #fbe4df;
  color: var(--danger);
  border: 1px solid #e5a99d;
}

.row-risk {
  background: rgba(163, 59, 43, 0.07);
}

.row-risk td {
  border-bottom-color: #eccac2;
}

.summary-cell {
  max-width: 280px;
}

.modal-wide {
  width: min(820px, 100%);
}

.title-badge {
  margin-left: 0.6rem;
  vertical-align: middle;
}

.items-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 1.1rem 0 0.5rem;
}

.items-head h4 {
  margin: 0;
}

.items-table .col-code {
  width: 26%;
}

.items-table .col-result {
  width: 22%;
}

.items-table .col-act {
  width: 72px;
}

.item-fail td {
  background: rgba(163, 59, 43, 0.06);
}

.form-error {
  margin-top: 0.8rem;
  font-weight: 600;
}
</style>
