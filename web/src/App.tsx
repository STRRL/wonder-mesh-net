import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuth } from './hooks/useAuth'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Nodes from './pages/Nodes'
import JoinToken from './pages/JoinToken'
import ApiKeys from './pages/ApiKeys'

function App() {
  const { isAuthenticated, isLoading, login } = useAuth()

  if (isLoading) {
    return (
      <div className="container" style={{ textAlign: 'center', paddingTop: '4rem' }}>
        <p>Loading...</p>
      </div>
    )
  }

  if (!isAuthenticated) {
    return (
      <div className="container" style={{ textAlign: 'center', paddingTop: '4rem' }}>
        <div className="card" style={{ maxWidth: '400px', margin: '0 auto' }}>
          <h1 style={{ marginBottom: '1rem' }}>Wonder Mesh Net</h1>
          <p style={{ marginBottom: '1.5rem', color: '#666' }}>
            Please sign in to manage your mesh network.
          </p>
          <button onClick={login}>Sign in with OIDC</button>
        </div>
      </div>
    )
  }

  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/nodes" element={<Nodes />} />
        <Route path="/token" element={<JoinToken />} />
        <Route path="/api-keys" element={<ApiKeys />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}

export default App
