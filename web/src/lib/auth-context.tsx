"use client"

import React, { createContext, useContext, useState, ReactNode, useEffect } from "react"
import { refreshAccessTokenAction } from "@/actions/auth"

interface AuthContextType {
  accessToken: string | null
  setAccessToken: (token: string | null) => void
  isLoadingAuth: boolean
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [accessToken, setAccessToken] = useState<string | null>(null)
  const [isLoadingAuth, setIsLoadingAuth] = useState(true)

  useEffect(() => {
    async function initAuth() {
      try {
        const token = await refreshAccessTokenAction()
        if (token) {
          setAccessToken(token)
        }
      } catch (error) {
        console.error("Failed to restore session", error)
      } finally {
        setIsLoadingAuth(false)
      }
    }

    if (!accessToken) {
      initAuth()
    } else {
      setIsLoadingAuth(false)
    }
  }, [accessToken])

  return (
    <AuthContext.Provider value={{ accessToken, setAccessToken, isLoadingAuth }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider")
  }
  return context
}
