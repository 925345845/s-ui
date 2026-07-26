<template>
  <v-dialog v-model="enroll.visible" width="min(640px, calc(100vw - 24px))">
    <v-card>
      <v-card-title class="text-center">{{ $t('agent.enroll') }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-text-field v-if="!enroll.command" v-model="enroll.name" :label="$t('agent.name')" maxlength="80" autofocus hide-details />
        <template v-else>
          <v-text-field :model-value="enroll.token" :label="$t('agent.token')" readonly dir="ltr" hide-details class="mb-3">
            <template #append-inner><v-btn icon="mdi-content-copy" size="small" variant="text" @click="copy(enroll.token)" /></template>
          </v-text-field>
          <v-textarea :model-value="enroll.command" :label="$t('agent.command')" readonly dir="ltr" rows="3" hide-details>
            <template #append-inner><v-btn icon="mdi-content-copy" size="small" variant="text" @click="copy(enroll.command)" /></template>
          </v-textarea>
        </template>
      </v-card-text>
      <v-card-actions class="justify-center">
        <v-btn variant="outlined" @click="closeEnrollment">{{ $t('actions.close') }}</v-btn>
        <v-btn v-if="!enroll.command" color="primary" variant="tonal" prepend-icon="mdi-link-plus" :loading="enroll.loading" :disabled="!enroll.name.trim()" @click="createNode">{{ $t('actions.add') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog v-model="removeDialog.visible" width="min(420px, calc(100vw - 24px))">
    <v-card>
      <v-card-title class="text-center">{{ $t('actions.del') }}</v-card-title>
      <v-card-text class="text-center">{{ removeDialog.node?.name }}</v-card-text>
      <v-card-actions class="justify-center">
        <v-btn variant="outlined" @click="removeDialog.visible = false">{{ $t('no') }}</v-btn>
        <v-btn color="error" variant="tonal" :loading="removeDialog.loading" @click="deleteNode">{{ $t('yes') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <div class="agent-toolbar">
    <v-btn color="primary" prepend-icon="mdi-server-plus" @click="openEnrollment">{{ $t('agent.enroll') }}</v-btn>
    <v-btn variant="tonal" prepend-icon="mdi-refresh" :loading="loading" @click="loadNodes">{{ $t('actions.update') }}</v-btn>
  </div>

  <v-alert v-if="!loading && nodes.length === 0" type="info" variant="tonal">{{ $t('agent.noNodes') }}</v-alert>
  <template v-else>
    <div class="agent-table-wrap">
      <v-table density="comfortable" class="agent-table">
        <thead>
          <tr>
            <th>{{ $t('agent.name') }}</th>
            <th>{{ $t('agent.status') }}</th>
            <th>{{ $t('agent.platform') }}</th>
            <th>CPU</th>
            <th>{{ $t('agent.memory') }}</th>
            <th>{{ $t('agent.disk') }}</th>
            <th>{{ $t('agent.cores') }}</th>
            <th>{{ $t('agent.lastSeen') }}</th>
            <th class="text-center">{{ $t('actions.action') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="node in nodes" :key="node.id">
            <td>
              <div class="agent-name">{{ node.name }}</div>
              <div class="agent-muted" dir="ltr">{{ node.report.hostname || node.remote_ip || '-' }}</div>
            </td>
            <td><v-chip size="small" :color="node.online ? 'success' : 'default'" variant="tonal">{{ node.online ? $t('online') : $t('agent.offline') }}</v-chip></td>
            <td dir="ltr">{{ platform(node) }}</td>
            <td>{{ percent(node.report.cpu_percent) }}</td>
            <td>{{ usage(node.report.memory) }}</td>
            <td>{{ usage(node.report.disk) }}</td>
            <td>
              <div class="core-status"><v-icon size="15" :color="node.report.cores?.singbox_running ? 'success' : 'default'" icon="mdi-circle" /> sing-box</div>
              <div class="core-status"><v-icon size="15" :color="node.report.cores?.xray_running ? 'success' : 'default'" icon="mdi-circle" /> Xray</div>
            </td>
            <td>{{ lastSeen(node.last_seen) }}</td>
            <td>
              <div class="agent-actions">
                <v-btn icon="mdi-key-change" size="small" variant="text" :title="$t('agent.rotate')" @click="rotateNode(node)" />
                <v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" :title="$t('actions.del')" @click="askDelete(node)" />
              </div>
            </td>
          </tr>
        </tbody>
      </v-table>
    </div>

    <div class="agent-mobile-list">
      <section v-for="node in nodes" :key="node.id" class="agent-mobile-item">
        <header class="agent-mobile-header">
          <div class="agent-mobile-identity">
            <div class="agent-name">{{ node.name }}</div>
            <div class="agent-muted" dir="ltr">{{ node.report.hostname || node.remote_ip || '-' }}</div>
          </div>
          <v-chip size="small" :color="node.online ? 'success' : 'default'" variant="tonal">{{ node.online ? $t('online') : $t('agent.offline') }}</v-chip>
        </header>
        <div class="agent-mobile-metrics">
          <div><span>{{ $t('agent.platform') }}</span><strong dir="ltr">{{ platform(node) }}</strong></div>
          <div><span>CPU</span><strong>{{ percent(node.report.cpu_percent) }}</strong></div>
          <div><span>{{ $t('agent.memory') }}</span><strong>{{ usage(node.report.memory) }}</strong></div>
          <div><span>{{ $t('agent.disk') }}</span><strong>{{ usage(node.report.disk) }}</strong></div>
        </div>
        <div class="agent-mobile-footer">
          <div>
            <div class="core-status"><v-icon size="15" :color="node.report.cores?.singbox_running ? 'success' : 'default'" icon="mdi-circle" /> sing-box</div>
            <div class="core-status"><v-icon size="15" :color="node.report.cores?.xray_running ? 'success' : 'default'" icon="mdi-circle" /> Xray</div>
          </div>
          <div class="agent-mobile-seen"><span>{{ $t('agent.lastSeen') }}</span>{{ lastSeen(node.last_seen) }}</div>
          <div class="agent-actions">
            <v-btn icon="mdi-key-change" size="small" variant="text" :title="$t('agent.rotate')" @click="rotateNode(node)" />
            <v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" :title="$t('actions.del')" @click="askDelete(node)" />
          </div>
        </div>
      </section>
    </div>
  </template>
</template>

<script lang="ts" setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { push } from 'notivue'
import { i18n } from '@/locales'

type Usage = { used?: number, total?: number }
type AgentNode = {
  id: number
  name: string
  last_seen: number
  remote_ip: string
  version: string
  online: boolean
  report: {
    hostname?: string
    os?: string
    arch?: string
    cpu_percent?: number
    memory?: Usage
    disk?: Usage
    cores?: { singbox_running?: boolean, xray_running?: boolean, xray_version?: string }
  }
}

const nodes = ref<AgentNode[]>([])
const loading = ref(false)
const enroll = reactive({ visible: false, loading: false, name: '', token: '', command: '' })
const removeDialog = reactive<{ visible: boolean, loading: boolean, node?: AgentNode }>({ visible: false, loading: false })
let refreshTimer: number | undefined

const api = async (path: string, options?: RequestInit) => {
  const response = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest', ...(options?.headers || {}) },
    ...options,
  })
  const result = await response.json()
  if (!response.ok || !result.success) throw new Error(result.msg || response.statusText)
  return result.obj
}

const loadNodes = async () => {
  loading.value = true
  try { nodes.value = await api('api/agents') || [] }
  catch (error: any) { push.error({ message: error?.message || i18n.global.t('agent.loadFailed') }) }
  finally { loading.value = false }
}

const openEnrollment = () => {
  Object.assign(enroll, { visible: true, loading: false, name: '', token: '', command: '' })
}

const closeEnrollment = () => {
  enroll.visible = false
  if (enroll.command) loadNodes()
}

const createNode = async () => {
  enroll.loading = true
  try {
    const result = await api('api/agents', { method: 'POST', body: JSON.stringify({ name: enroll.name.trim() }) })
    enroll.token = result.token
    enroll.command = result.command
  } catch (error: any) {
    push.error({ message: error?.message || i18n.global.t('agent.createFailed') })
  } finally { enroll.loading = false }
}

const rotateNode = async (node: AgentNode) => {
  try {
    const result = await api(`api/agents/${node.id}/rotate`, { method: 'POST', body: '{}' })
    Object.assign(enroll, { visible: true, loading: false, name: node.name, token: result.token, command: result.command })
  } catch (error: any) { push.error({ message: error?.message || i18n.global.t('agent.rotateFailed') }) }
}

const askDelete = (node: AgentNode) => Object.assign(removeDialog, { visible: true, loading: false, node })
const deleteNode = async () => {
  if (!removeDialog.node) return
  removeDialog.loading = true
  try {
    await api(`api/agents/${removeDialog.node.id}/delete`, { method: 'POST', body: '{}' })
    removeDialog.visible = false
    await loadNodes()
  } catch (error: any) { push.error({ message: error?.message || i18n.global.t('agent.deleteFailed') }) }
  finally { removeDialog.loading = false }
}

const copy = async (value: string) => {
  await navigator.clipboard.writeText(value)
  push.success({ message: i18n.global.t('copyToClipboard') })
}

const bytes = (value = 0) => {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / Math.pow(1024, index)).toFixed(index > 2 ? 1 : 0)} ${units[index]}`
}
const usage = (value?: Usage) => value?.total ? `${bytes(value.used)} / ${bytes(value.total)}` : '-'
const percent = (value = 0) => `${Math.max(0, value).toFixed(1)}%`
const platform = (node: AgentNode) => [node.report.os, node.report.arch].filter(Boolean).join(' / ') || '-'
const lastSeen = (value: number) => value ? new Date(value * 1000).toLocaleString() : '-'

onMounted(() => {
  loadNodes()
  refreshTimer = window.setInterval(loadNodes, 10000)
})
onBeforeUnmount(() => { if (refreshTimer) window.clearInterval(refreshTimer) })
</script>

<style scoped>
.agent-toolbar { display: flex; justify-content: center; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; }
.agent-table-wrap { width: 100%; overflow-x: auto; border: 1px solid rgba(var(--v-theme-on-surface), 0.1); border-radius: 8px; }
.agent-table { min-width: 980px; background: rgba(var(--v-theme-surface), 0.5) !important; }
.agent-name { font-weight: 600; }
.agent-muted { color: rgba(var(--v-theme-on-surface), 0.55); font-size: 12px; margin-top: 2px; }
.core-status { display: flex; align-items: center; gap: 5px; white-space: nowrap; font-size: 12px; }
.agent-actions { display: flex; justify-content: center; gap: 2px; }
.agent-mobile-list { display: none; }

@media (max-width: 720px) {
  .agent-table-wrap { display: none; }
  .agent-mobile-list { display: grid; gap: 10px; }
  .agent-mobile-item {
    overflow: hidden;
    border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
    border-radius: 8px;
    background: rgba(var(--v-theme-surface), 0.66);
    backdrop-filter: blur(14px);
  }
  .agent-mobile-header,
  .agent-mobile-footer { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 12px; }
  .agent-mobile-identity { min-width: 0; }
  .agent-mobile-identity .agent-muted { overflow-wrap: anywhere; }
  .agent-mobile-metrics { display: grid; grid-template-columns: 1fr 1fr; border-block: 1px solid rgba(var(--v-theme-on-surface), 0.08); }
  .agent-mobile-metrics > div { min-width: 0; padding: 10px 12px; }
  .agent-mobile-metrics span,
  .agent-mobile-seen span { display: block; margin-bottom: 3px; color: rgba(var(--v-theme-on-surface), 0.55); font-size: 11px; }
  .agent-mobile-metrics strong { display: block; overflow-wrap: anywhere; font-size: 12px; font-weight: 600; }
  .agent-mobile-seen { min-width: 0; font-size: 11px; text-align: center; }
}
</style>
