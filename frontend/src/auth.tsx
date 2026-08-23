import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, setAccessToken } from './api'
import type { Role, User } from './types'

type AuthValue = { user: User | null; loading: boolean; login: (email: string, password: string) => Promise<void>; logout: () => Promise<void> }
const AuthContext = createContext<AuthValue | null>(null)
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null); const [loading, setLoading] = useState(true)
  useEffect(() => { api.refresh().then(token => { setAccessToken(token.access_token); setUser({ id: token.user_id, role: token.role }) }).catch(() => setAccessToken(null)).finally(() => setLoading(false)) }, [])
  const value = useMemo<AuthValue>(() => ({ user, loading, async login(email, password) { const token = await api.login(email, password); setAccessToken(token.access_token); setUser({ id: token.user_id, role: token.role }) }, async logout() { await api.logout().catch(() => undefined); setAccessToken(null); setUser(null) } }), [user, loading])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
export function useAuth() { const context = useContext(AuthContext); if (!context) throw new Error('useAuth must be inside AuthProvider'); return context }
export function isRole(role: Role | undefined, expected: Role) { return role === expected }
