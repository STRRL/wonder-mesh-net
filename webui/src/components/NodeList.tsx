import type { Node } from '../api/types'

interface NodeListProps {
  nodes: Node[]
}

function formatLastSeen(lastSeen?: string): string {
  if (!lastSeen) return 'Never'
  const date = new Date(lastSeen)
  return date.toLocaleString()
}

export default function NodeList({ nodes }: NodeListProps) {
  if (nodes.length === 0) {
    return (
      <div style={{ textAlign: 'center', padding: '2rem', color: '#666' }}>
        No nodes connected yet. Generate a join token and run{' '}
        <code style={{ backgroundColor: '#f5f5f5', padding: '0.25rem 0.5rem', borderRadius: '4px' }}>
          wonder worker join
        </code>{' '}
        on your machines.
      </div>
    )
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>IP Addresses</th>
          <th>Status</th>
          <th>Last Seen</th>
        </tr>
      </thead>
      <tbody>
        {nodes.map((node) => (
          <tr key={node.id}>
            <td>
              <strong>{node.given_name || node.name}</strong>
              {node.given_name && node.given_name !== node.name && (
                <span style={{ color: '#666', marginLeft: '0.5rem' }}>({node.name})</span>
              )}
            </td>
            <td>
              {node.ip_addresses.map((ip, idx) => (
                <span key={ip}>
                  <code style={{ backgroundColor: '#f5f5f5', padding: '0.125rem 0.375rem', borderRadius: '4px' }}>
                    {ip}
                  </code>
                  {idx < node.ip_addresses.length - 1 && ', '}
                </span>
              ))}
            </td>
            <td>
              <span style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: '0.5rem',
              }}>
                <span style={{
                  width: '8px',
                  height: '8px',
                  borderRadius: '50%',
                  backgroundColor: node.online ? '#28a745' : '#dc3545',
                }} />
                {node.online ? 'Online' : 'Offline'}
              </span>
            </td>
            <td>{formatLastSeen(node.last_seen)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
