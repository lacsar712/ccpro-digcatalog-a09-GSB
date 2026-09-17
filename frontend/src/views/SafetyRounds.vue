<template>
  <div>
    <div class="toolbar">
      <div>
        <h2 class="page-title">工地安全巡检</h2>
        <p class="page-sub">按轮次登记巡检条目；结论与不合格项由后端强校验</p>
      </div>
      <button class="btn" @click="openCreate">新增巡检轮次</button>
    </div>

    <div class="card">
      <label class="filter">
        按工地筛选
        <select v-model="filterSiteId" @change="onFilterChange">
          <option value="">全部工地</option>
          <option v-for="s in sites" :key="s.id" :value="String(s.id)">{{ s.name }}</option>
        </select>
      </label>
      <table class="table">
        <thead>
          <tr>
            <th>巡检日期</th>
            <th>工地</th>
            <th>巡检人</th>
            <th>天气</th>
            <th>结论</th>
            <th>不合格项</th>
            <th>摘要</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in list" :key="r.id" :class="{ 'row-risk': r.conclusion === 'risk' }">
            <td>{{ fmtDate(r.roundDate) }}</td>
            <td>{{ r.site?.name || '-' }}</td>
            <td>{{ r.inspector }}</td>
            <td>{{ r.weather || '-' }}</td>
            <td>
              <span v-if="r.conclusion === 'risk'" class="badge badge-risk">⚠ 风险 RISK</span>
              <span v-else class="badge badge-ok">合格 OK</span>
            </td>
            <td>
              <span v-if="failCount(r) > 0" class="badge badge-risk">{{ failCount(r) }} 项</span>
              <span v-else class="muted">0</span>
            </td>
            <td class="summary-cell">{{ r.summary || '-' }}</td>
            <td>
              <button class="btn secondary small" @click="openEdit(r)">编辑条目</button>
              <button class="btn danger small" @click="remove(r)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!list.length" class="page-sub">暂无巡检记录</p>
      <p v-if="error" class="error">{{ error }}</p>
    </div>

    <div v-if="showModal" class="modal-mask" @click.self="showModal = false">
      <div class="modal modal-wide">
        <h3>{{ form.id ? '编辑巡检轮次' : '新增巡检轮次' }}</h3>
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
            <input v-model="form.weather" placeholder="如 晴 / 阵雨" />
          </label>
          <label class="full">
            巡检结论
            <span class="conclusion">
              <label class="inline">
                <input type="radio" value="ok" v-model="form.conclusion" />
                <span class="badge badge-ok">合格 OK</span>
              </label>
              <label class="inline">
                <input type="radio" value="risk" v-model="form.conclusion" />
                <span class="badge badge-risk">⚠ 风险 RISK</span>
              </label>
              <span class="hint">有不合格项时结论必须为风险；合格轮不允许出现不合格项（后端强校验）</span>
            </span>
          </label>
          <label class="full">
            巡检摘要
            <textarea v-model="form.summary" />
          </label>
        </div>

        <div class="items-head">
          <h4>巡检条目（{{ form.items.length }}）</h4>
          <button class="btn secondary small" @click="addItem">新增条目</button>
        </div>
        <table class="table items-table">
          <thead>
            <tr>
              <th style="width: 130px">条目编号</th>
              <th style="width: 150px">结果</th>
              <th>备注</th>
              <th style="width: 70px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(it, idx) in form.items" :key="it._uid" :class="{ 'item-fail': it.result === 'fail' }">
              <td><input v-model="it.itemCode" placeholder="如 S01" /></td>
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
        <p v-if="!form.items.length" class="page-sub">尚无条目，请至少添加一条巡检条目</p>

        <p v-if="formError" class="error">{{ formError }}</p>
        <div class="modal-actions">
          <button class="btn secondary" @click="showModal = false">取消</button>
          <button class="btn" @click="save">保存轮次</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api/http'

const route = useRoute()
const router = useRouter()

const list = ref([])
const sites = ref([])
const filterSiteId = ref(String(route.query.siteId || ''))
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

let itemSeq = 0
function makeItem(src = {}) {
  return { _uid: ++itemSeq, id: null, itemCode: '', result: 'pass', comment: '', ...src }
}

function fmtDate(s) {
  return s ? String(s).slice(0, 10) : '-'
}

function failCount(round) {
  return (round.items || []).filter((i) => i.result === 'fail').length
}

function todayStr() {
  const d = new Date()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
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

function onFilterChange() {
  const query = filterSiteId.value ? { siteId: filterSiteId.value } : {}
  router.replace({ name: 'safety-rounds', query })
  load()
}

function resetForm() {
  Object.assign(form, {
    id: null,
    siteId: filterSiteId.value ? Number(filterSiteId.value) : sites.value[0]?.id || 0,
    roundDate: todayStr(),
    inspector: '',
    weather: '',
    conclusion: 'ok',
    summary: '',
    items: []
  })
}

function openCreate() {
  resetForm()
  addItem()
  formError.value = ''
  showModal.value = true
}

function openEdit(round) {
  Object.assign(form, {
    id: round.id,
    siteId: round.siteId,
    roundDate: fmtDate(round.roundDate) === '-' ? '' : fmtDate(round.roundDate),
    inspector: round.inspector,
    weather: round.weather || '',
    conclusion: round.conclusion,
    summary: round.summary || '',
    items: (round.items || []).map((i) =>
      makeItem({
        id: i.id,
        itemCode: i.itemCode,
        result: i.result,
        comment: i.comment || ''
      })
    )
  })
  formError.value = ''
  showModal.value = true
}

function addItem() {
  form.items.push(makeItem())
}

async function save() {
  formError.value = ''
  const payload = {
    siteId: Number(form.siteId) || 0,
    roundDate: form.roundDate || null,
    inspector: form.inspector,
    weather: form.weather,
    conclusion: form.conclusion,
    summary: form.summary,
    items: form.items.map((i) => ({
      id: i.id ?? null,
      itemCode: i.itemCode,
      result: i.result,
      comment: i.comment
    }))
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

async function remove(round) {
  const tip = `确认删除 ${fmtDate(round.roundDate)} 对「${round.site?.name || ''}」的巡检轮次及其全部条目？`
  if (!confirm(tip)) return
  try {
    await api.delete(`/safety-rounds/${round.id}`)
    await load()
  } catch (e) {
    alert(e.response?.data?.error || '删除失败')
  }
}

onMounted(async () => {
  await loadSites()
  await load()
})
</script>

<style scoped>
.filter {
  max-width: 260px;
  margin-bottom: 1rem;
}

.badge {
  display: inline-block;
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  font-size: 0.8rem;
  font-weight: 700;
  white-space: nowrap;
}

.badge-ok {
  background: #e3f0e8;
  color: var(--ok);
}

.badge-risk {
  background: #fbeae7;
  color: var(--danger);
  border: 1px solid var(--danger);
  animation: pulse-risk 1.6s ease-in-out infinite;
}

.row-risk {
  background: rgba(163, 59, 43, 0.06);
}

.row-risk td:first-child {
  border-left: 4px solid var(--danger);
}

.summary-cell {
  max-width: 260px;
}

.muted {
  color: var(--muted);
}

@keyframes pulse-risk {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(163, 59, 43, 0.35);
  }
  50% {
    box-shadow: 0 0 0 5px rgba(163, 59, 43, 0);
  }
}

.modal-wide {
  width: min(880px, 100%);
  max-height: 90vh;
  overflow-y: auto;
}

.conclusion {
  display: flex;
  align-items: center;
  gap: 1.2rem;
  flex-wrap: wrap;
  margin-top: 0.35rem;
}

.inline {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 0.4rem;
  color: var(--text);
}

.hint {
  color: var(--muted);
  font-size: 0.82rem;
}

.items-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 1.25rem 0 0.6rem;
}

.items-head h4 {
  margin: 0;
}

.items-table .item-fail {
  background: rgba(163, 59, 43, 0.07);
}
</style>
