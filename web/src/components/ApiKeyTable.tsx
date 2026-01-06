import { useState, useCallback } from 'react'
import type { ApiKeyInfo } from '../api/types'

interface ApiKeyTableProps {
  apiKeys: ApiKeyInfo[]
  onDelete: (id: string) => Promise<void>
}

function formatDate(dateString?: string): string {
  if (!dateString) return 'Never'
  const date = new Date(dateString)
  return date.toLocaleString()
}

export default function ApiKeyTable({ apiKeys, onDelete }: ApiKeyTableProps) {
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [confirmId, setConfirmId] = useState<string | null>(null)

  const handleDelete = useCallback(async (id: string) => {
    if (confirmId !== id) {
      setConfirmId(id)
      return
    }

    setDeletingId(id)
    try {
      await onDelete(id)
    } finally {
      setDeletingId(null)
      setConfirmId(null)
    }
  }, [confirmId, onDelete])

  const cancelConfirm = useCallback(() => {
    setConfirmId(null)
  }, [])

  if (apiKeys.length === 0) {
    return (
      <div style={{ textAlign: 'center', padding: '2rem', color: '#666' }}>
        No API keys created yet. Create one to allow third-party integrations.
      </div>
    )
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Key Prefix</th>
          <th>Created</th>
          <th>Last Used</th>
          <th>Expires</th>
          <th style={{ width: '120px' }}>Actions</th>
        </tr>
      </thead>
      <tbody>
        {apiKeys.map((key) => (
          <tr key={key.id}>
            <td><strong>{key.name}</strong></td>
            <td>
              <code style={{ backgroundColor: '#f5f5f5', padding: '0.125rem 0.375rem', borderRadius: '4px' }}>
                {key.key_prefix}...
              </code>
            </td>
            <td>{formatDate(key.created_at)}</td>
            <td>{formatDate(key.last_used_at)}</td>
            <td>{key.expires_at ? formatDate(key.expires_at) : 'Never'}</td>
            <td>
              {confirmId === key.id ? (
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <button
                    onClick={() => handleDelete(key.id)}
                    disabled={deletingId === key.id}
                    style={{ backgroundColor: '#dc3545', padding: '0.25rem 0.5rem', fontSize: '0.875rem' }}
                  >
                    {deletingId === key.id ? '...' : 'Confirm'}
                  </button>
                  <button
                    onClick={cancelConfirm}
                    style={{ backgroundColor: '#6c757d', padding: '0.25rem 0.5rem', fontSize: '0.875rem' }}
                  >
                    Cancel
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => handleDelete(key.id)}
                  style={{ backgroundColor: '#dc3545', padding: '0.25rem 0.5rem', fontSize: '0.875rem' }}
                >
                  Delete
                </button>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
