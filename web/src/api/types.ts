export interface Node {
  id: number
  name: string
  given_name: string
  ip_addresses: string[]
  online: boolean
  last_seen?: string
}

export interface NodeListResponse {
  nodes: Node[]
  count: number
}

export interface JoinTokenResponse {
  token: string
  expires_in: number
}

export interface ApiKeyInfo {
  id: string
  name: string
  key_prefix: string
  created_at: string
  last_used_at?: string
  expires_at?: string
}

export interface ApiKeyListResponse {
  api_keys: ApiKeyInfo[]
}

export interface CreateApiKeyRequest {
  name: string
  expires_in?: string
}

export interface CreateApiKeyResponse extends ApiKeyInfo {
  key: string
}
