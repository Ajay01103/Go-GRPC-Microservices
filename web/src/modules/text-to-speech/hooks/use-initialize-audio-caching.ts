"use client"

import { useEffect } from "react"
import { registerAudioCacheServiceWorker } from "@/lib/service-worker"

/**
 * Hook to initialize audio caching system
 * Should be called once in the root layout or after authentication
 *
 * Usage:
 * ```
 * function RootLayout({ children }) {
 *   useInitializeAudioCaching()
 *   return <>{children}</>
 * }
 * ```
 */
export function useInitializeAudioCaching(): void {
  useEffect(() => {
    // Register service worker for network-level audio caching
    registerAudioCacheServiceWorker().catch((error) => {
      console.warn("Failed to initialize audio caching:", error)
    })

    return () => {
      // Cleanup if needed (typically not required for service workers)
    }
  }, [])
}
