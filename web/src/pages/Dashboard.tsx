import { Link } from 'react-router-dom'
import { useNodes } from '../hooks/useNodes'

export default function Dashboard() {
  const { nodes, isLoading, error } = useNodes()

  const onlineCount = nodes.filter(n => n.online).length
  const totalCount = nodes.length

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto' }}>
      <h2 style={{ marginBottom: '1.5rem' }}>Dashboard</h2>

      {error && <div className="error">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '1rem', marginBottom: '2rem' }}>
        <div className="card">
          <div style={{ color: '#666', fontSize: '0.875rem', marginBottom: '0.5rem' }}>Total Nodes</div>
          <div style={{ fontSize: '2rem', fontWeight: 600 }}>
            {isLoading ? '...' : totalCount}
          </div>
        </div>
        <div className="card">
          <div style={{ color: '#666', fontSize: '0.875rem', marginBottom: '0.5rem' }}>Online Nodes</div>
          <div style={{ fontSize: '2rem', fontWeight: 600, color: '#28a745' }}>
            {isLoading ? '...' : onlineCount}
          </div>
        </div>
        <div className="card">
          <div style={{ color: '#666', fontSize: '0.875rem', marginBottom: '0.5rem' }}>Offline Nodes</div>
          <div style={{ fontSize: '2rem', fontWeight: 600, color: totalCount - onlineCount > 0 ? '#dc3545' : 'inherit' }}>
            {isLoading ? '...' : totalCount - onlineCount}
          </div>
        </div>
      </div>

      <h3 style={{ marginBottom: '1rem' }}>Quick Actions</h3>
      <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
        <Link to="/token">
          <button>Generate Join Token</button>
        </Link>
        <Link to="/nodes">
          <button style={{ backgroundColor: '#6c757d' }}>View All Nodes</button>
        </Link>
        <Link to="/api-keys">
          <button style={{ backgroundColor: '#6c757d' }}>Manage API Keys</button>
        </Link>
      </div>
    </div>
  )
}
