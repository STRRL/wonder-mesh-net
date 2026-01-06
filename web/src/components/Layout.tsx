import { NavLink } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import type { ReactNode } from 'react'

interface LayoutProps {
  children: ReactNode
}

const navLinkStyle = ({ isActive }: { isActive: boolean }) => ({
  padding: '0.5rem 1rem',
  borderRadius: '4px',
  backgroundColor: isActive ? '#0066cc' : 'transparent',
  color: isActive ? 'white' : 'inherit',
  textDecoration: 'none',
})

export default function Layout({ children }: LayoutProps) {
  const { logout } = useAuth()

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <header style={{
        borderBottom: '1px solid #e0e0e0',
        padding: '1rem 2rem',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '2rem' }}>
          <h1 style={{ fontSize: '1.25rem', fontWeight: 600 }}>Wonder Mesh Net</h1>
          <nav style={{ display: 'flex', gap: '0.5rem' }}>
            <NavLink to="/" style={navLinkStyle} end>Dashboard</NavLink>
            <NavLink to="/nodes" style={navLinkStyle}>Nodes</NavLink>
            <NavLink to="/token" style={navLinkStyle}>Join Token</NavLink>
            <NavLink to="/api-keys" style={navLinkStyle}>API Keys</NavLink>
          </nav>
        </div>
        <button
          onClick={logout}
          style={{ backgroundColor: 'transparent', color: '#666', border: '1px solid #ccc' }}
        >
          Sign out
        </button>
      </header>
      <main style={{ flex: 1, padding: '2rem' }}>
        {children}
      </main>
    </div>
  )
}
