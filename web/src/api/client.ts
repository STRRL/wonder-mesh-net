import type {
  NodeListResponse,
  JoinTokenResponse,
  ApiKeyListResponse,
  CreateApiKeyRequest,
  CreateApiKeyResponse,
} from './types'

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

class ApiClient {
  private async fetch<T>(path: string, options?: RequestInit): Promise<T> {
    const response = await fetch(path, {
      ...options,
      credentials: 'include',
    })

    if (response.status === 401) {
      window.location.href = '/coordinator/oidc/login?redirect_to=/ui/'
      throw new ApiError(401, 'Unauthorized')
    }

    if (!response.ok) {
      const text = await response.text()
      throw new ApiError(response.status, text || `API error: ${response.status}`)
    }

    if (response.status === 204) {
      return undefined as T
    }

    return response.json()
  }

  async getNodes(): Promise<NodeListResponse> {
    return this.fetch('/coordinator/api/v1/nodes')
  }

  async createJoinToken(): Promise<JoinTokenResponse> {
    return this.fetch('/coordinator/api/v1/join-token', { method: 'POST' })
  }

  async getApiKeys(): Promise<ApiKeyListResponse> {
    return this.fetch('/coordinator/api/v1/api-keys')
  }

  async createApiKey(request: CreateApiKeyRequest): Promise<CreateApiKeyResponse> {
    return this.fetch('/coordinator/api/v1/api-keys', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    })
  }

  async deleteApiKey(id: string): Promise<void> {
    return this.fetch(`/coordinator/api/v1/api-keys/${id}`, {
      method: 'DELETE',
    })
  }
}

export const api = new ApiClient()
export { ApiError }
