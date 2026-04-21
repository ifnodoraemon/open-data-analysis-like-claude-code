import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useDataSourceStore = defineStore('dataSource', () => {
  const sessionSources = ref([])
  const workspaceDataSources = ref([])
  const semanticProfileSummaries = ref([])
  const semanticProfileDetails = ref({})
  const loading = ref(false)

  async function fetchSessionSources(sessionId) {
    if (!sessionId) return
    loading.value = true
    try {
      const res = await fetch(`/api/sessions/${sessionId}/sources`, { headers: getAuthHeaders() })
      if (res.ok) {
        const data = await res.json()
        sessionSources.value = data.sources || []
      }
    } finally {
      loading.value = false
    }
  }

  async function fetchWorkspaceDataSources() {
    loading.value = true
    try {
      const res = await fetch('/api/data-sources', { headers: getAuthHeaders() })
      if (res.ok) {
        const data = await res.json()
        workspaceDataSources.value = data.data_sources || []
      }
    } finally {
      loading.value = false
    }
  }

  async function fetchProfileDetail(profileId) {
    if (!profileId) return
    loading.value = true
    try {
      const res = await fetch(`/api/semantic-profiles/${profileId}`, { headers: getAuthHeaders() })
      if (res.ok) {
        const data = await res.json()
        semanticProfileDetails.value[profileId] = data
      }
    } finally {
      loading.value = false
    }
  }

  async function confirmProfile(profileId, scope, overrides) {
    const res = await fetch(`/api/semantic-profiles/${profileId}/confirm`, {
      method: 'POST',
      headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ scope, overrides })
    })
    return res.ok
  }

  async function createPostgresSource(name, config) {
    const res = await fetch('/api/data-sources', {
      method: 'POST',
      headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, source_type: 'postgres_connection', postgres: config })
    })
    if (res.ok) {
      await fetchWorkspaceDataSources()
      return true
    }
    return false
  }

  async function testConnection(sourceId) {
    const res = await fetch(`/api/data-sources/${sourceId}/test`, {
      method: 'POST',
      headers: getAuthHeaders()
    })
    return res.ok ? await res.json() : { success: false, message: '请求失败' }
  }

  async function importFromSource(sourceId, sessionId, schemaName, objectName) {
    const res = await fetch(`/api/data-sources/${sourceId}/import`, {
      method: 'POST',
      headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionId, schema_name: schemaName, object_name: objectName })
    })
    if (res.ok) {
      await fetchSessionSources(sessionId)
      return await res.json()
    }
    return null
  }

  function getAuthHeaders() {
    const token = localStorage.getItem('token')
    return token ? { Authorization: `Bearer ${token}` } : {}
  }

  return {
    sessionSources,
    workspaceDataSources,
    semanticProfileSummaries,
    semanticProfileDetails,
    loading,
    fetchSessionSources,
    fetchWorkspaceDataSources,
    fetchProfileDetail,
    confirmProfile,
    createPostgresSource,
    testConnection,
    importFromSource
  }
})
