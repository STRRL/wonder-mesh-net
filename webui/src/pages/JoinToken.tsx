import { useJoinToken } from '../hooks/useJoinToken'
import CopyButton from '../components/CopyButton'

export default function JoinToken() {
  const { token, isLoading, error, generateToken, clearToken } = useJoinToken()

  const expiresInHours = token ? Math.round(token.expires_in / 3600) : 0

  return (
    <div style={{ maxWidth: '600px', margin: '0 auto' }}>
      <h2 style={{ marginBottom: '1.5rem' }}>Join Token</h2>

      {error && <div className="error">{error}</div>}

      <div className="card">
        {!token ? (
          <div style={{ textAlign: 'center' }}>
            <p style={{ marginBottom: '1.5rem', color: '#666' }}>
              Generate a join token to add new nodes to your mesh network.
              The token will be valid for 8 hours.
            </p>
            <button onClick={generateToken} disabled={isLoading}>
              {isLoading ? 'Generating...' : 'Generate Token'}
            </button>
          </div>
        ) : (
          <div>
            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
                Your Join Token
              </label>
              <div style={{ position: 'relative' }}>
                <textarea
                  readOnly
                  value={token.token}
                  style={{
                    width: '100%',
                    minHeight: '100px',
                    fontFamily: 'monospace',
                    fontSize: '0.875rem',
                    resize: 'vertical',
                  }}
                />
              </div>
            </div>

            <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', flexWrap: 'wrap' }}>
              <CopyButton text={token.token} />
              <button onClick={clearToken} style={{ backgroundColor: '#6c757d' }}>
                Clear
              </button>
              <span style={{ color: '#666', fontSize: '0.875rem' }}>
                Expires in {expiresInHours} hours
              </span>
            </div>

            <div style={{ marginTop: '1.5rem', padding: '1rem', backgroundColor: '#f5f5f5', borderRadius: '4px' }}>
              <p style={{ marginBottom: '0.5rem', fontWeight: 500 }}>Usage:</p>
              <code style={{ display: 'block', wordBreak: 'break-all' }}>
                wonder worker join --coordinator-url {window.location.origin} {'<token>'}
              </code>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
