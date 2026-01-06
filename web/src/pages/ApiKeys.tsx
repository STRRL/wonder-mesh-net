import { useState, useCallback, type FormEvent } from 'react'
import { useApiKeys } from '../hooks/useApiKeys'
import ApiKeyTable from '../components/ApiKeyTable'
import CopyButton from '../components/CopyButton'
import type { CreateApiKeyResponse } from '../api/types'

export default function ApiKeys() {
  const { apiKeys, isLoading, error, createApiKey, deleteApiKey } = useApiKeys()
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [newKeyName, setNewKeyName] = useState('')
  const [newKeyExpiry, setNewKeyExpiry] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [newlyCreatedKey, setNewlyCreatedKey] = useState<CreateApiKeyResponse | null>(null)

  const handleCreate = useCallback(async (e: FormEvent) => {
    e.preventDefault()
    if (!newKeyName.trim()) return

    setIsCreating(true)
    setCreateError(null)
    try {
      const response = await createApiKey({
        name: newKeyName.trim(),
        expires_in: newKeyExpiry || undefined,
      })
      setNewlyCreatedKey(response)
      setNewKeyName('')
      setNewKeyExpiry('')
      setShowCreateForm(false)
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Unknown error occurred')
    } finally {
      setIsCreating(false)
    }
  }, [newKeyName, newKeyExpiry, createApiKey])

  const dismissNewKey = useCallback(() => {
    setNewlyCreatedKey(null)
  }, [])

  return (
    <div style={{ maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
        <h2>API Keys</h2>
        <button onClick={() => setShowCreateForm(!showCreateForm)}>
          {showCreateForm ? 'Cancel' : 'Create API Key'}
        </button>
      </div>

      {error && <div className="error">{error}</div>}

      {newlyCreatedKey && (
        <div className="success" style={{ marginBottom: '1rem' }}>
          <div style={{ marginBottom: '0.5rem' }}>
            <strong>API Key Created!</strong> Make sure to copy it now. You won't be able to see it again.
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', flexWrap: 'wrap' }}>
            <code style={{ backgroundColor: 'rgba(0,0,0,0.1)', padding: '0.5rem', borderRadius: '4px', wordBreak: 'break-all' }}>
              {newlyCreatedKey.key}
            </code>
            <CopyButton text={newlyCreatedKey.key} />
            <button onClick={dismissNewKey} style={{ backgroundColor: '#6c757d' }}>
              Dismiss
            </button>
          </div>
        </div>
      )}

      {showCreateForm && (
        <div className="card" style={{ marginBottom: '1rem' }}>
          <form onSubmit={handleCreate}>
            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
                Name <span style={{ color: '#dc3545' }}>*</span>
              </label>
              <input
                type="text"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
                placeholder="e.g., zeabur-integration"
                style={{ width: '100%' }}
                required
              />
            </div>
            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
                Expires In (optional)
              </label>
              <input
                type="text"
                value={newKeyExpiry}
                onChange={(e) => setNewKeyExpiry(e.target.value)}
                placeholder="e.g., 720h (30 days), leave empty for no expiry"
                style={{ width: '100%' }}
              />
              <small style={{ color: '#666', display: 'block', marginTop: '0.25rem' }}>
                Format: 1h, 24h, 720h, etc.
              </small>
            </div>
            {createError && <div className="error">{createError}</div>}
            <button type="submit" disabled={isCreating || !newKeyName.trim()}>
              {isCreating ? 'Creating...' : 'Create'}
            </button>
          </form>
        </div>
      )}

      <div className="card">
        {isLoading && apiKeys.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '2rem' }}>Loading...</div>
        ) : (
          <ApiKeyTable apiKeys={apiKeys} onDelete={deleteApiKey} />
        )}
      </div>
    </div>
  )
}
