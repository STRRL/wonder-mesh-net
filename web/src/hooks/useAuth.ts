import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'

export function useAuth() {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    api.getNodes()
      .then(() => setIsAuthenticated(true))
      .catch(() => setIsAuthenticated(false))
      .finally(() => setIsLoading(false))
  }, [])

  const login = useCallback(() => {
    window.location.href = '/coordinator/oidc/login?redirect_to=/ui/'
  }, [])

  const logout = useCallback(() => {
    window.location.href = '/coordinator/oidc/logout'
  }, [])

  return {
    isAuthenticated: isAuthenticated ?? false,
    isLoading,
    login,
    logout,
  }
}
