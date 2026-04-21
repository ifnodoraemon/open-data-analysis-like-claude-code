<template>
  <div v-if="open" class="datasource-drawer-overlay" @click.self="$emit('close')">
    <div class="datasource-drawer">
      <div class="drawer-header">
        <h3>Sources</h3>
        <button class="close-btn" @click="$emit('close')">&times;</button>
      </div>

      <div class="drawer-section">
        <h4>当前会话 Snapshots</h4>
        <div v-if="sessionSources.length === 0" class="empty-hint">暂无数据源</div>
        <div v-for="source in sessionSources" :key="source.source_id" class="source-card">
          <div class="source-name">{{ source.display_name }}</div>
          <div class="source-meta">
            <span class="badge" :class="source.source_type">{{ source.source_type }}</span>
            <span v-if="source.analysis_table_name" class="table-name">{{ source.analysis_table_name }}</span>
            <span v-if="source.row_count">{{ source.row_count.toLocaleString() }} rows</span>
            <span v-if="source.large_dataset" class="badge large">large dataset</span>
          </div>
          <div class="source-status">
            <span :class="['status', source.semantic_status]">{{ source.semantic_status }}</span>
            <span v-if="source.profile_mode" class="profile-mode">{{ source.profile_mode }}</span>
          </div>
        </div>
      </div>

      <div class="drawer-section">
        <h4>工作区 SQL Sources</h4>
        <div v-if="workspaceDataSources.length === 0" class="empty-hint">暂无 SQL 数据源</div>
        <div v-for="ds in workspaceDataSources" :key="ds.id" class="source-card">
          <div class="source-name">{{ ds.name }}</div>
          <div class="source-meta">
            <span class="badge postgres">{{ ds.source_type }}</span>
            <span :class="['status', ds.status]">{{ ds.status }}</span>
          </div>
        </div>
      </div>

      <div class="drawer-section">
        <h4>待确认语义项</h4>
        <div v-if="pendingProfiles.length === 0" class="empty-hint">无待确认项</div>
        <div v-for="p in pendingProfiles" :key="p.profile_id" class="source-card">
          <div class="source-name">{{ p.analysis_table_name }}</div>
          <span class="status needs_confirmation">{{ p.profile_status }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  open: Boolean,
  sessionSources: { type: Array, default: () => [] },
  workspaceDataSources: { type: Array, default: () => [] },
  pendingProfiles: { type: Array, default: () => [] }
})

defineEmits(['close'])
</script>

<style scoped>
.datasource-drawer-overlay {
  position: fixed; top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0,0,0,0.3); z-index: 1000;
  display: flex; justify-content: flex-end;
}
.datasource-drawer {
  width: 380px; height: 100vh; background: #fff;
  box-shadow: -2px 0 8px rgba(0,0,0,0.1); overflow-y: auto;
  padding: 16px;
}
.drawer-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.drawer-header h3 { margin: 0; font-size: 16px; }
.close-btn { background: none; border: none; font-size: 20px; cursor: pointer; }
.drawer-section { margin-bottom: 20px; }
.drawer-section h4 { font-size: 13px; color: #666; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.empty-hint { color: #999; font-size: 13px; }
.source-card { padding: 10px; border: 1px solid #eee; border-radius: 6px; margin-bottom: 8px; }
.source-name { font-weight: 500; font-size: 14px; margin-bottom: 4px; }
.source-meta { display: flex; gap: 8px; font-size: 12px; color: #666; flex-wrap: wrap; align-items: center; }
.source-status { margin-top: 4px; font-size: 12px; }
.badge { padding: 1px 6px; border-radius: 3px; font-size: 11px; }
.badge.file_upload { background: #e3f2fd; color: #1565c0; }
.badge.postgres_connection, .badge.postgres { background: #f3e5f5; color: #7b1fa2; }
.badge.large { background: #fff3e0; color: #e65100; }
.table-name { font-family: monospace; }
.status { font-size: 11px; }
.status.profiled, .status.active { color: #2e7d32; }
.status.draft, .status.needs_confirmation { color: #e65100; }
.status.confirmed { color: #1565c0; }
.status.failed, .status.invalid { color: #c62828; }
.profile-mode { color: #999; font-size: 11px; margin-left: 6px; }
</style>
