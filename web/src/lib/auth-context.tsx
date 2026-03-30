"use client"

import {
  createContext, useContext, useState,
  useEffect, useRef, useCallback, ReactNode
} from "react"
import { refreshAccessTokenAction } from "@/actions/auth"

interface AuthContextType {
  accessToken: string | null
  isAuthenticated: boolean
  isLoadingAuth: boolean
  setAccessToken: (token: string | null) => void
}

function parseJwtExp(token: string): number | null {
  try {
    const payload = JSON.parse(atob(token.split(".")[1]))
    return payload.exp ?? null
  } catch {
    return null
  }
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [accessToken, setAccessTokenState] = useState<string | null>(null)
  const [isLoadingAuth, setIsLoadingAuth] = useState(true)
  const silentRefreshRef = useRef<NodeJS.Timeout | null>(null)

  const scheduleRefresh = useCallback((token: string) => {
    const exp = parseJwtExp(token)
    if (!exp) return

    const refreshInMs = exp * 1000 - Date.now() - 30_000 // 30s before expiry
    if (silentRefreshRef.current) clearTimeout(silentRefreshRef.current)

    silentRefreshRef.current = setTimeout(async () => {
      const newToken = await refreshAccessTokenAction()
      if (newToken) {
        setAccessTokenState(newToken)
        scheduleRefresh(newToken)
      } else {
        setAccessTokenState(null)
      }
    }, Math.max(refreshInMs, 0))
  }, [])

  const setAccessToken = useCallback((token: string | null) => {
    setAccessTokenState(token)
    if (token) {
      scheduleRefresh(token)
    } else {
      if (silentRefreshRef.current) clearTimeout(silentRefreshRef.current)
    }
  }, [scheduleRefresh])

  // Run once on mount only
  useEffect(() => {
    async function initAuth() {
      try {
        const token = await refreshAccessTokenAction()
        if (token) setAccessToken(token)
      } catch (err) {
        console.error("Failed to restore session", err)
      } finally {
        setIsLoadingAuth(false)
      }
    }

    initAuth()

    return () => {
      if (silentRefreshRef.current) clearTimeout(silentRefreshRef.current)
    }
  }, []) // ← empty, intentional

  return (
    <AuthContext.Provider value={{
      accessToken,
      isAuthenticated: accessToken !== null,
      isLoadingAuth,
      setAccessToken,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error("useAuth must be used within an AuthProvider")
  return context
}
