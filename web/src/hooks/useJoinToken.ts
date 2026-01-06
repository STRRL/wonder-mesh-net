import { useState, useCallback } from 'react'
import { api } from '../api/client'
import type { JoinTokenResponse } from '../api/types'

export function useJoinToken() {
  const [token, setToken] = useState<JoinTokenResponse | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const generateToken = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const response = await api.createJoinToken()
      setToken(response)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred')
    } finally {
      setIsLoading(false)
    }
  }, [])

  const clearToken = useCallback(() => {
    setToken(null)
    setError(null)
  }, [])

  return {
    token,
    isLoading,
    error,
    generateToken,
    clearToken,
  }
}
