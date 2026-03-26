"use server"

import { cookies } from "next/headers"
import { rpcClient } from "@/lib/rpc"

export async function setAuthCookies(refreshToken: string) {
  const cookieStore = await cookies()
  cookieStore.set("refreshToken", refreshToken, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 7 * 24 * 60 * 60, // 7 days in seconds
  })
}

export async function refreshAccessTokenAction() {
  const cookieStore = await cookies()
  const refreshToken = cookieStore.get("refreshToken")?.value
  if (!refreshToken) return null

  try {
    const response = await rpcClient.refreshToken({ refreshToken })

    // Update the cookie with the newly issued refresh token
    cookieStore.set("refreshToken", response.refreshToken, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 7 * 24 * 60 * 60,
    })

    return response.accessToken
  } catch (error) {
    console.error("Failed to refresh token", error)
    cookieStore.delete("refreshToken")
    return null
  }
}

export async function logoutAction() {
  const cookieStore = await cookies()
  const refreshToken = cookieStore.get("refreshToken")?.value

  if (refreshToken) {
    try {
      // Call the gRPC endpoint to immediately revoke the token in Redis
      await rpcClient.logout({ refreshToken })
    } catch (error) {
      // Log but don't block logout — still clear the cookie
      console.error("RPC logout failed", error)
    }
  }

  cookieStore.delete("refreshToken")
}
