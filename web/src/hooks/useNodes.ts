import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'
import type { Node } from '../api/types'

export function useNodes() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchNodes = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const response = await api.getNodes()
      setNodes(response.nodes || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchNodes()
  }, [fetchNodes])

  return {
    nodes,
    isLoading,
    error,
    refetch: fetchNodes,
  }
}
