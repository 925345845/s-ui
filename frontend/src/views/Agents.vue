<template>
  <v-dialog v-model="enroll.visible" width="min(640px, calc(100vw - 24px))">
    <v-card>
      <v-card-title class="text-center">{{ $t('agent.enroll') }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-alert type="info" variant="tonal" density="compact" class="mb-3">{{ $t('agent.note') }}</v-alert>
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

  <v-dialog v-model="detail.visible" width="min(920px, calc(100vw - 24px))">
    <v-card v-if="detail.node">
      <v-card-title class="text-center">{{ $t('agent.detail') }} · {{ detail.node.name }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-row dense>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.status') }}</div><v-chip size="small" :color="detail.node.online ? 'success' : 'default'" variant="tonal">{{ detail.node.online ? $t('online') : $t('agent.offline') }}</v-chip></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.connection') }}</div><strong>{{ connLabel(detail.node) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.uptime') }}</div><strong>{{ formatUptime(detail.node.report.uptime) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">CPU</div><strong>{{ percent(detail.node.report.cpu_percent) }} ({{ detail.node.report.cpu_cores || '-' }} cores)</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.memory') }}</div><strong>{{ usage(detail.node.report.memory) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.disk') }}</div><strong>{{ usage(detail.node.report.disk) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.load') }}</div><strong dir="ltr">{{ loadAvg(detail.node.report.load) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.netRate') }}</div><strong dir="ltr">↓ {{ rate(detail.node.report.net_rate?.recv) }} · ↑ {{ rate(detail.node.report.net_rate?.sent) }}</strong></v-col>
          <v-col cols="12" sm="6" md="4"><div class="metric-label">{{ $t('agent.processes') }}</div><strong>{{ detail.node.report.process_count ?? '-' }}</strong></v-col>
          <v-col cols="12"><div class="metric-label">{{ $t('agent.addresses') }}</div><div class="agent-muted" dir="ltr">{{ addressList(detail.node) }}</div></v-col>
        </v-row>
        <div class="metric-label mt-4">{{ $t('agent.history') }}</div>
        <div v-if="!detail.node.history?.length" class="agent-muted">{{ $t('noData') }}</div>
        <div v-else class="history-bars">
          <div v-for="(sample, idx) in detail.node.history.slice(-40)" :key="idx" class="history-bar"
            :title="`CPU ${percent(sample.cpu_percent)} / MEM ${percent(sample.mem_percent)}`"
            :style="{ height: Math.max(4, Math.min(100, sample.cpu_percent || 0)) + '%' }" />
        </div>

        <v-divider class="my-4" />
        <div class="metric-label">{{ $t('agent.control') }}</div>
        <v-alert v-if="!detail.node.controllable" type="warning" variant="tonal" density="compact" class="mb-3">{{ $t('agent.controlNeedWs') }}</v-alert>
        <div class="control-actions">
          <v-btn size="small" variant="tonal" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('report_now')">{{ $t('agent.cmdReportNow') }}</v-btn>
          <v-btn size="small" variant="tonal" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('ping')">{{ $t('agent.cmdPing') }}</v-btn>
          <v-btn size="small" color="primary" variant="tonal" :disabled="!detail.node.controllable" prepend-icon="mdi-console" @click="openTerminal(detail.node)">{{ $t('agent.terminal') }}</v-btn>
          <v-btn size="small" variant="tonal" color="warning" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('restart_xray')">{{ $t('agent.cmdRestartXray') }}</v-btn>
          <v-btn size="small" variant="tonal" color="warning" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('restart_singbox')">{{ $t('agent.cmdRestartSingBox') }}</v-btn>
          <v-btn size="small" variant="tonal" color="error" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('restart_agent')">{{ $t('agent.cmdRestartAgent') }}</v-btn>
        </div>
        <v-row dense class="mt-2">
          <v-col cols="12" sm="4">
            <v-text-field v-model.number="control.interval" type="number" min="5" max="300" :label="$t('agent.intervalSeconds')" density="compact" hide-details />
          </v-col>
          <v-col cols="12" sm="8" class="d-flex align-center">
            <v-btn size="small" variant="tonal" :disabled="!detail.node.controllable || control.loading" @click="sendCmd('set_interval', { seconds: control.interval })">{{ $t('agent.cmdSetInterval') }}</v-btn>
          </v-col>
          <v-col cols="12" sm="9">
            <v-text-field v-model="control.shell" :label="$t('agent.execCommand')" :placeholder="$t('agent.execPlaceholder')" density="compact" hide-details dir="ltr" @keyup.enter="runShell" />
          </v-col>
          <v-col cols="12" sm="3" class="d-flex align-center">
            <v-btn block color="primary" variant="tonal" :loading="control.loading" :disabled="!detail.node.controllable || !control.shell.trim()" @click="runShell">{{ $t('agent.sendCommand') }}</v-btn>
          </v-col>
        </v-row>
        <v-textarea v-if="control.lastOutput !== ''" class="mt-3" :model-value="control.lastOutput" :label="$t('agent.output')" readonly auto-grow rows="4" dir="ltr" hide-details />
        <div class="metric-label mt-4">{{ $t('agent.commandLog') }}</div>
        <div v-if="!detail.node.commands?.length" class="agent-muted">{{ $t('noData') }}</div>
        <v-list v-else density="compact" bg-color="transparent" class="command-log">
          <v-list-item v-for="item in detail.node.commands.slice().reverse().slice(0, 12)" :key="item.id">
            <v-list-item-title>
              <v-chip size="x-small" :color="item.ok ? 'success' : 'error'" variant="tonal" class="me-2">{{ item.ok ? 'OK' : 'ERR' }}</v-chip>
              {{ item.type }}
              <span class="agent-muted ms-2">{{ item.elapsed_ms }}ms</span>
            </v-list-item-title>
            <v-list-item-subtitle class="text-truncate" dir="ltr">{{ item.error || item.output || '-' }}</v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </v-card-text>
      <v-card-actions class="justify-center">
        <v-btn variant="outlined" @click="detail.visible = false">{{ $t('actions.close') }}</v-btn>
        <v-btn variant="tonal" :loading="control.loading" @click="refreshDetail">{{ $t('actions.update') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- Interactive terminal -->
  <v-dialog v-model="term.visible" width="min(960px, calc(100vw - 16px))" persistent scrim="true">
    <v-card class="term-card">
      <v-card-title class="d-flex align-center justify-space-between">
        <span>{{ $t('agent.terminal') }} · {{ term.nodeName }}</span>
        <div class="d-flex ga-2">
          <v-chip size="small" :color="term.connected ? 'success' : 'error'" variant="tonal">{{ term.connected ? $t('online') : $t('agent.offline') }}</v-chip>
          <v-btn icon="mdi-close" size="small" variant="text" @click="closeTerminal" />
        </div>
      </v-card-title>
      <v-divider />
      <div
        ref="termEl"
        class="term-screen"
        tabindex="0"
        @keydown="onTermKey"
        @paste="onTermPaste"
        @click="focusTerm"
      >{{ term.buffer }}</div>
      <v-card-text class="py-2">
        <div class="agent-muted">{{ $t('agent.terminalHint') }}</div>
      </v-card-text>
    </v-card>
  </v-dialog>

  <!-- Batch result -->
  <v-dialog v-model="batch.resultVisible" width="min(720px, calc(100vw - 24px))">
    <v-card>
      <v-card-title class="text-center">{{ $t('agent.batchResult') }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-list density="compact">
          <v-list-item v-for="item in batch.results" :key="item.node_id">
            <v-list-item-title>
              <v-chip size="x-small" :color="item.ok ? 'success' : 'error'" variant="tonal" class="me-2">{{ item.ok ? 'OK' : 'ERR' }}</v-chip>
              {{ item.name || ('#' + item.node_id) }}
            </v-list-item-title>
            <v-list-item-subtitle dir="ltr">{{ item.error || item.result?.output || item.result?.error || '-' }}</v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </v-card-text>
      <v-card-actions class="justify-center">
        <v-btn variant="outlined" @click="batch.resultVisible = false">{{ $t('actions.close') }}</v-btn>
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
    <v-btn variant="tonal" prepend-icon="mdi-checkbox-multiple-marked" @click="selectAllControllable">{{ $t('agent.selectOnline') }}</v-btn>
    <v-btn variant="text" @click="selected = []">{{ $t('agent.clearSelection') }}</v-btn>
  </div>

  <div v-if="selected.length" class="batch-bar">
    <strong>{{ $t('agent.batchSelected', { n: selected.length }) }}</strong>
    <v-btn size="small" variant="tonal" :loading="batch.loading" @click="batchCmd('report_now')">{{ $t('agent.cmdReportNow') }}</v-btn>
    <v-btn size="small" variant="tonal" :loading="batch.loading" @click="batchCmd('ping')">{{ $t('agent.cmdPing') }}</v-btn>
    <v-btn size="small" variant="tonal" color="warning" :loading="batch.loading" @click="batchCmd('restart_xray')">{{ $t('agent.cmdRestartXray') }}</v-btn>
    <v-btn size="small" variant="tonal" color="warning" :loading="batch.loading" @click="batchCmd('restart_singbox')">{{ $t('agent.cmdRestartSingBox') }}</v-btn>
    <v-btn size="small" variant="tonal" color="error" :loading="batch.loading" @click="batchCmd('restart_agent')">{{ $t('agent.cmdRestartAgent') }}</v-btn>
    <v-text-field v-model="batch.shell" density="compact" hide-details :placeholder="$t('agent.execPlaceholder')" style="max-width:220px" dir="ltr" />
    <v-btn size="small" color="primary" variant="tonal" :loading="batch.loading" :disabled="!batch.shell.trim()" @click="batchCmd('exec', { command: batch.shell.trim() })">{{ $t('agent.cmdExec') }}</v-btn>
  </div>

  <v-alert v-if="!loading && nodes.length === 0" type="info" variant="tonal">{{ $t('agent.noNodes') }}</v-alert>
  <template v-else>
    <div class="agent-table-wrap">
      <v-table density="comfortable" class="agent-table">
        <thead>
          <tr>
            <th style="width:42px"></th>
            <th>{{ $t('agent.name') }}</th>
            <th>{{ $t('agent.status') }}</th>
            <th>{{ $t('agent.connection') }}</th>
            <th>{{ $t('agent.platform') }}</th>
            <th>CPU</th>
            <th>{{ $t('agent.memory') }}</th>
            <th>{{ $t('agent.netRate') }}</th>
            <th>{{ $t('agent.cores') }}</th>
            <th>{{ $t('agent.lastSeen') }}</th>
            <th class="text-center">{{ $t('actions.action') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="node in nodes" :key="node.id" class="agent-row">
            <td @click.stop>
              <v-checkbox-btn :model-value="selected.includes(node.id)" :disabled="!node.controllable" @update:model-value="toggleSelect(node.id, $event)" />
            </td>
            <td @click="openDetail(node)">
              <div class="agent-name">{{ node.name }}</div>
              <div class="agent-muted" dir="ltr">{{ node.report.hostname || node.remote_ip || '-' }}</div>
            </td>
            <td @click="openDetail(node)"><v-chip size="small" :color="node.online ? 'success' : 'default'" variant="tonal">{{ node.online ? $t('online') : $t('agent.offline') }}</v-chip></td>
            <td @click="openDetail(node)"><v-chip size="x-small" variant="outlined">{{ connLabel(node) }}</v-chip></td>
            <td dir="ltr" @click="openDetail(node)">{{ platform(node) }}</td>
            <td @click="openDetail(node)">{{ percent(node.report.cpu_percent) }}</td>
            <td @click="openDetail(node)">{{ usage(node.report.memory) }}</td>
            <td dir="ltr" @click="openDetail(node)">↓ {{ rate(node.report.net_rate?.recv) }}</td>
            <td @click="openDetail(node)">
              <div class="core-status"><v-icon size="15" :color="node.report.cores?.singbox_running ? 'success' : 'default'" icon="mdi-circle" /> sing-box</div>
              <div class="core-status"><v-icon size="15" :color="node.report.cores?.xray_running ? 'success' : 'default'" icon="mdi-circle" /> Xray</div>
            </td>
            <td @click="openDetail(node)">{{ lastSeen(node.last_seen) }}</td>
            <td @click.stop>
              <div class="agent-actions">
                <v-btn icon="mdi-console" size="small" variant="text" :disabled="!node.controllable" :title="$t('agent.terminal')" @click="openTerminal(node)" />
                <v-btn icon="mdi-information-outline" size="small" variant="text" :title="$t('agent.detail')" @click="openDetail(node)" />
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
          <div class="d-flex align-center ga-2">
            <v-checkbox-btn :model-value="selected.includes(node.id)" :disabled="!node.controllable" @update:model-value="toggleSelect(node.id, $event)" @click.stop />
            <div class="agent-mobile-identity" @click="openDetail(node)">
              <div class="agent-name">{{ node.name }}</div>
              <div class="agent-muted" dir="ltr">{{ node.report.hostname || node.remote_ip || '-' }}</div>
            </div>
          </div>
          <v-chip size="small" :color="node.online ? 'success' : 'default'" variant="tonal">{{ node.online ? $t('online') : $t('agent.offline') }}</v-chip>
        </header>
        <div class="agent-mobile-metrics" @click="openDetail(node)">
          <div><span>{{ $t('agent.platform') }}</span><strong dir="ltr">{{ platform(node) }}</strong></div>
          <div><span>CPU</span><strong>{{ percent(node.report.cpu_percent) }}</strong></div>
          <div><span>{{ $t('agent.memory') }}</span><strong>{{ usage(node.report.memory) }}</strong></div>
          <div><span>{{ $t('agent.connection') }}</span><strong>{{ connLabel(node) }}</strong></div>
        </div>
        <div class="agent-mobile-footer" @click.stop>
          <div>
            <div class="core-status"><v-icon size="15" :color="node.report.cores?.singbox_running ? 'success' : 'default'" icon="mdi-circle" /> sing-box</div>
            <div class="core-status"><v-icon size="15" :color="node.report.cores?.xray_running ? 'success' : 'default'" icon="mdi-circle" /> Xray</div>
          </div>
          <div class="agent-actions">
            <v-btn icon="mdi-console" size="small" variant="text" :disabled="!node.controllable" @click="openTerminal(node)" />
            <v-btn icon="mdi-key-change" size="small" variant="text" @click="rotateNode(node)" />
            <v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" @click="askDelete(node)" />
          </div>
        </div>
      </section>
    </div>
  </template>
</template>

<script lang="ts" setup>
import { nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
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
  conn_mode?: string
  ws_connected?: boolean
  controllable?: boolean
  commands?: { id: string, type: string, ok: boolean, output?: string, error?: string, elapsed_ms?: number }[]
  report: {
    hostname?: string
    os?: string
    arch?: string
    uptime?: number
    cpu_percent?: number
    cpu_cores?: number
    memory?: Usage
    disk?: Usage
    net_rate?: { sent?: number, recv?: number }
    load?: { load1?: number, load5?: number, load15?: number }
    process_count?: number
    ipv4?: string[]
    ipv6?: string[]
    cores?: { singbox_running?: boolean, xray_running?: boolean, xray_version?: string }
    conn_mode?: string
  }
  history?: { time: number, cpu_percent: number, mem_percent: number }[]
}

const nodes = ref<AgentNode[]>([])
const loading = ref(false)
const selected = ref<number[]>([])
const enroll = reactive({ visible: false, loading: false, name: '', token: '', command: '' })
const removeDialog = reactive<{ visible: boolean, loading: boolean, node?: AgentNode }>({ visible: false, loading: false })
const detail = reactive<{ visible: boolean, node?: AgentNode }>({ visible: false })
const control = reactive({ loading: false, shell: '', interval: 15, lastOutput: '' })
const batch = reactive<{ loading: boolean, shell: string, results: any[], resultVisible: boolean }>({ loading: false, shell: '', results: [], resultVisible: false })
const term = reactive({ visible: false, connected: false, buffer: '', nodeName: '', nodeId: 0 })
const termEl = ref<HTMLElement | null>(null)
let termWs: WebSocket | null = null
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
  try {
    nodes.value = await api('api/agents') || []
    selected.value = selected.value.filter(id => nodes.value.some(n => n.id === id && n.controllable))
  } catch (error: any) {
    push.error({ message: error?.message || i18n.global.t('agent.loadFailed') })
  } finally { loading.value = false }
}

const openEnrollment = () => Object.assign(enroll, { visible: true, loading: false, name: '', token: '', command: '' })
const closeEnrollment = () => { enroll.visible = false; if (enroll.command) loadNodes() }

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

const openDetail = async (node: AgentNode) => {
  control.lastOutput = ''
  control.shell = ''
  try {
    detail.node = await api(`api/agents/${node.id}`)
  } catch {
    detail.node = node
  }
  detail.visible = true
}

const refreshDetail = async () => {
  if (!detail.node) return
  try { detail.node = await api(`api/agents/${detail.node.id}`) }
  catch (error: any) { push.error({ message: error?.message || i18n.global.t('agent.loadFailed') }) }
}

const sendCmd = async (type: string, args?: Record<string, any>) => {
  if (!detail.node) return
  control.loading = true
  try {
    const result = await api(`api/agents/${detail.node.id}/command`, {
      method: 'POST',
      body: JSON.stringify({ type, args: args || {} }),
    })
    control.lastOutput = [result?.output, result?.error].filter(Boolean).join('\n') || JSON.stringify(result)
    if (result?.ok) push.success({ message: i18n.global.t('agent.controlSuccess') })
    else push.error({ message: result?.error || i18n.global.t('agent.controlFailed') })
    await refreshDetail()
    if (type === 'report_now') await loadNodes()
  } catch (error: any) {
    control.lastOutput = error?.message || i18n.global.t('agent.controlFailed')
    push.error({ message: control.lastOutput })
  } finally { control.loading = false }
}

const runShell = () => { if (control.shell.trim()) sendCmd('exec', { command: control.shell.trim() }) }

const toggleSelect = (id: number, on: boolean | null) => {
  if (on) {
    if (!selected.value.includes(id)) selected.value.push(id)
  } else {
    selected.value = selected.value.filter(x => x !== id)
  }
}
const selectAllControllable = () => {
  selected.value = nodes.value.filter(n => n.controllable).map(n => n.id)
}

const batchCmd = async (type: string, args?: Record<string, any>) => {
  if (!selected.value.length) return
  batch.loading = true
  try {
    const results = await api('api/agents/batch-command', {
      method: 'POST',
      body: JSON.stringify({ ids: selected.value, type, args: args || {} }),
    })
    batch.results = results || []
    batch.resultVisible = true
    const ok = batch.results.filter((r: any) => r.ok).length
    push.success({ message: `${ok}/${batch.results.length} OK` })
    await loadNodes()
  } catch (error: any) {
    push.error({ message: error?.message || i18n.global.t('agent.controlFailed') })
  } finally { batch.loading = false }
}

const wsURL = (path: string) => {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const base = (document.querySelector('base')?.getAttribute('href') || (window as any).BASE_URL || '/').replace(/\/?$/, '/')
  return `${proto}//${location.host}${base}${path.replace(/^\//, '')}`
}

const openTerminal = async (node: AgentNode) => {
  if (!node.controllable) {
    push.error({ message: i18n.global.t('agent.controlNeedWs') })
    return
  }
  closeTerminal()
  term.visible = true
  term.connected = false
  term.buffer = ''
  term.nodeName = node.name
  term.nodeId = node.id
  await nextTick()
  focusTerm()
  const url = wsURL(`api/agents/${node.id}/terminal?cols=100&rows=30`)
  termWs = new WebSocket(url)
  termWs.onopen = () => { term.connected = true }
  termWs.onclose = () => { term.connected = false }
  termWs.onerror = () => {
    term.connected = false
    term.buffer += '\r\n[connection error]\r\n'
  }
  termWs.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data)
      if (msg.type === 'terminal_output' && msg.data) {
        term.buffer += atob(msg.data)
        if (term.buffer.length > 200000) term.buffer = term.buffer.slice(-150000)
        nextTick(() => {
          if (termEl.value) termEl.value.scrollTop = termEl.value.scrollHeight
        })
      } else if (msg.type === 'terminal_closed') {
        term.connected = false
        term.buffer += `\r\n[${msg.error || 'closed'}]\r\n`
      } else if (msg.type === 'terminal_opened') {
        term.connected = true
      }
    } catch { /* ignore */ }
  }
}

const closeTerminal = () => {
  if (termWs) {
    try { termWs.send(JSON.stringify({ type: 'close' })) } catch { /* */ }
    termWs.close()
    termWs = null
  }
  term.visible = false
  term.connected = false
}

const focusTerm = () => termEl.value?.focus()

const sendTermRaw = (text: string) => {
  if (!termWs || termWs.readyState !== WebSocket.OPEN) return
  termWs.send(JSON.stringify({ type: 'input', data: text }))
}

const onTermKey = (e: KeyboardEvent) => {
  if (!term.connected) return
  e.preventDefault()
  if (e.key === 'Enter') return sendTermRaw('\r')
  if (e.key === 'Backspace') return sendTermRaw('\x7f')
  if (e.key === 'Tab') return sendTermRaw('\t')
  if (e.key === 'Escape') return sendTermRaw('\x1b')
  if (e.key === 'ArrowUp') return sendTermRaw('\x1b[A')
  if (e.key === 'ArrowDown') return sendTermRaw('\x1b[B')
  if (e.key === 'ArrowRight') return sendTermRaw('\x1b[C')
  if (e.key === 'ArrowLeft') return sendTermRaw('\x1b[D')
  if (e.key === 'Home') return sendTermRaw('\x1b[H')
  if (e.key === 'End') return sendTermRaw('\x1b[F')
  if (e.key === 'Delete') return sendTermRaw('\x1b[3~')
  if (e.ctrlKey && e.key.length === 1) {
    const code = e.key.toLowerCase().charCodeAt(0) - 96
    if (code >= 1 && code <= 26) return sendTermRaw(String.fromCharCode(code))
  }
  if (e.key.length === 1) sendTermRaw(e.key)
}

const onTermPaste = (e: ClipboardEvent) => {
  e.preventDefault()
  const text = e.clipboardData?.getData('text') || ''
  if (text) sendTermRaw(text)
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
  try { await navigator.clipboard.writeText(value); push.success({ message: i18n.global.t('success') }) }
  catch { push.error({ message: i18n.global.t('failed') }) }
}

const platform = (node: AgentNode) => {
  const os = node.report.os || '-'
  const arch = node.report.arch || ''
  return arch ? `${os}/${arch}` : os
}
const percent = (value?: number) => value == null || Number.isNaN(value) ? '-' : `${value.toFixed(1)}%`
const usage = (value?: Usage) => {
  if (!value?.total) return '-'
  return `${((value.used || 0) / (1024 ** 3)).toFixed(1)} / ${(value.total / (1024 ** 3)).toFixed(1)} GiB`
}
const rate = (bytesPerSec?: number) => {
  if (bytesPerSec == null) return '-'
  if (bytesPerSec < 1024) return `${bytesPerSec} B/s`
  if (bytesPerSec < 1024 ** 2) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  return `${(bytesPerSec / (1024 ** 2)).toFixed(2)} MB/s`
}
const loadAvg = (load?: { load1?: number, load5?: number, load15?: number }) => {
  if (!load) return '-'
  return [load.load1, load.load5, load.load15].map(v => (v ?? 0).toFixed(2)).join(' / ')
}
const formatUptime = (seconds?: number) => {
  if (!seconds) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}
const connLabel = (node: AgentNode) => {
  if (node.ws_connected || node.controllable || node.conn_mode === 'ws' || node.report.conn_mode === 'ws') return i18n.global.t('agent.connWs')
  if (node.online) return i18n.global.t('agent.connHttp')
  return '-'
}
const addressList = (node: AgentNode) => {
  const items = [...(node.report.ipv4 || []), ...(node.report.ipv6 || [])]
  return items.length ? items.join(', ') : '-'
}
const lastSeen = (value: number) => {
  if (!value) return '-'
  const delta = Math.max(0, Math.floor(Date.now() / 1000 - value))
  if (delta < 60) return `${delta}s`
  if (delta < 3600) return `${Math.floor(delta / 60)}m`
  if (delta < 86400) return `${Math.floor(delta / 3600)}h`
  return new Date(value * 1000).toLocaleString()
}

onMounted(() => {
  loadNodes()
  refreshTimer = window.setInterval(loadNodes, 15000)
})
onBeforeUnmount(() => {
  if (refreshTimer) window.clearInterval(refreshTimer)
  closeTerminal()
})
</script>

<style scoped>
.agent-toolbar { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 16px; }
.batch-bar {
  display: flex; flex-wrap: wrap; gap: 8px; align-items: center;
  margin-bottom: 14px; padding: 10px 12px; border-radius: 14px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  background: rgba(var(--v-theme-primary), 0.06);
}
.agent-table-wrap { overflow-x: auto; border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); border-radius: 16px; }
.agent-table { min-width: 1120px; }
.agent-name { font-weight: 600; }
.agent-muted { opacity: 0.7; font-size: 0.85rem; word-break: break-all; }
.agent-actions { display: flex; justify-content: center; gap: 2px; }
.core-status { display: flex; align-items: center; gap: 6px; white-space: nowrap; }
.agent-mobile-list { display: none; gap: 12px; }
.agent-mobile-item { border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); border-radius: 16px; padding: 14px; display: grid; gap: 12px; }
.agent-mobile-header, .agent-mobile-footer { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
.agent-mobile-metrics { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.agent-mobile-metrics span, .metric-label { display: block; opacity: 0.7; font-size: 0.8rem; margin-bottom: 2px; }
.agent-row { cursor: pointer; }
.history-bars { display: flex; align-items: flex-end; gap: 3px; height: 80px; margin-top: 8px; padding: 8px; border-radius: 12px; background: rgba(var(--v-theme-surface-variant), 0.25); }
.history-bar { flex: 1; min-width: 4px; border-radius: 3px 3px 0 0; background: rgb(var(--v-theme-primary)); opacity: 0.85; }
.control-actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 8px 0 12px; }
.command-log { max-height: 220px; overflow: auto; border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity)); border-radius: 12px; }
.term-card { background: #0b1020 !important; color: #d7e0ff; }
.term-screen {
  height: min(62vh, 520px); overflow: auto; padding: 12px 14px;
  background: #070b16; color: #c8facc; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px; line-height: 1.35; white-space: pre-wrap; word-break: break-word; outline: none;
}
@media (max-width: 960px) {
  .agent-table-wrap { display: none; }
  .agent-mobile-list { display: grid; }
}
</style>
