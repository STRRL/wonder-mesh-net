import { useNodes } from '../hooks/useNodes'
import NodeList from '../components/NodeList'

export default function Nodes() {
  const { nodes, isLoading, error, refetch } = useNodes()

  return (
    <div style={{ maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
        <h2>Nodes</h2>
        <button onClick={refetch} disabled={isLoading}>
          {isLoading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      {error && <div className="error">{error}</div>}

      <div className="card">
        {isLoading && nodes.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '2rem' }}>Loading...</div>
        ) : (
          <NodeList nodes={nodes} />
        )}
      </div>
    </div>
  )
}
