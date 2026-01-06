import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'
import type { ApiKeyInfo, CreateApiKeyRequest, CreateApiKeyResponse } from '../api/types'

export function useApiKeys() {
  const [apiKeys, setApiKeys] = useState<ApiKeyInfo[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchApiKeys = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const response = await api.getApiKeys()
      setApiKeys(response.api_keys || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchApiKeys()
  }, [fetchApiKeys])

  const createApiKey = useCallback(async (request: CreateApiKeyRequest): Promise<CreateApiKeyResponse> => {
    const response = await api.createApiKey(request)
    await fetchApiKeys()
    return response
  }, [fetchApiKeys])

  const deleteApiKey = useCallback(async (id: string) => {
    await api.deleteApiKey(id)
    await fetchApiKeys()
  }, [fetchApiKeys])

  return {
    apiKeys,
    isLoading,
    error,
    refetch: fetchApiKeys,
    createApiKey,
    deleteApiKey,
  }
}
