"use server"

import { createHash } from "node:crypto"
import { cookies } from "next/headers"
import { ConnectError } from "@connectrpc/connect"
import { authRpcClient } from "@/lib/rpc"
import { REFRESH_TOKEN_COOKIE_NAME } from "@/lib/auth-cookie"
import { createRefreshDPoPProof } from "@/lib/server-dpop"

const serverInflightRefresh = new Map<string, Promise<string | null>>()

function refreshDedupKey(refreshToken: string): string {
  return createHash("sha256").update(refreshToken).digest("hex")
}

function setRefreshCookie(cookieStore: Awaited<ReturnType<typeof cookies>>, refreshToken: string) {
  cookieStore.set(REFRESH_TOKEN_COOKIE_NAME, refreshToken, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 7 * 24 * 60 * 60,
  })
}

export async function setAuthCookies(refreshToken: string) {
  const cookieStore = await cookies()
  setRefreshCookie(cookieStore, refreshToken)
}

export async function refreshAccessTokenAction() {
  const cookieStore = await cookies()
  const refreshToken = cookieStore.get(REFRESH_TOKEN_COOKIE_NAME)?.value
  if (!refreshToken) return null

  const dedupKey = refreshDedupKey(refreshToken)
  const inFlight = serverInflightRefresh.get(dedupKey)
  if (inFlight) {
    return inFlight
  }

  const refreshPromise = (async () => {
    try {
      const firstProof = await createRefreshDPoPProof(cookieStore)
      let response
      try {
        response = await authRpcClient.refreshToken({
          refreshToken,
          dpopProof: firstProof,
        })
      } catch (error) {
        const nonce = getDPoPNonceFromError(error)
        if (!nonce) {
          throw error
        }

        const nonceBoundProof = await createRefreshDPoPProof(cookieStore, nonce)
        response = await authRpcClient.refreshToken({
          refreshToken,
          dpopProof: nonceBoundProof,
        })
      }

      // Update the cookie with the newly issued refresh token
      setRefreshCookie(cookieStore, response.refreshToken)

      return response.accessToken
    } catch (error) {
      console.error("Failed to refresh token", error)
      // Refresh failure should only clear local session state.
      // Logout remains an explicit user action.
      cookieStore.delete(REFRESH_TOKEN_COOKIE_NAME)
      return null
    } finally {
      serverInflightRefresh.delete(dedupKey)
    }
  })()

  serverInflightRefresh.set(dedupKey, refreshPromise)
  return refreshPromise
}

function getDPoPNonceFromError(error: unknown): string | null {
  if (!(error instanceof ConnectError)) {
    return null
  }

  return (
    error.metadata.get("dpop-nonce") ??
    error.metadata.get("Dpop-Nonce") ??
    error.metadata.get("DPoP-Nonce") ??
    null
  )
}

export async function requireAccessTokenAction(): Promise<string> {
  const token = await refreshAccessTokenAction()
  if (!token) {
    throw new Error("Unauthorized")
  }

  return token
}

export async function logoutAction() {
  const cookieStore = await cookies()
  const refreshToken = cookieStore.get(REFRESH_TOKEN_COOKIE_NAME)?.value

  if (refreshToken) {
    try {
      // Call the gRPC endpoint to immediately revoke the token in Redis
      await authRpcClient.logout({ refreshToken })
    } catch (error) {
      // Log but don't block logout — still clear the cookie
      console.error("RPC logout failed", error)
    }
  }

  cookieStore.delete(REFRESH_TOKEN_COOKIE_NAME)
}
