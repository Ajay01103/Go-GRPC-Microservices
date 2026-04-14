"use client"

import { useCallback, useEffect, useState } from "react"
import { clearAudioCache, getAudioCacheSize, removeCachedAudio } from "@/lib/audio-cache"

interface UseAudioCacheReturn {
  cacheSize: number
  formattedCacheSize: string
  clearCache: () => Promise<void>
  isClearing: boolean
  isLoading: boolean
}

/**
 * Hook for managing audio cache
 * Provides utilities to check cache size and clear cache
 */
export function useAudioCache(): UseAudioCacheReturn {
  const [cacheSize, setCacheSize] = useState(0)
  const [isLoading, setIsLoading] = useState(true)
  const [isClearing, setIsClearing] = useState(false)

  // Load initial cache size
  useEffect(() => {
    const loadCacheSize = async () => {
      try {
        const size = await getAudioCacheSize()
        setCacheSize(size)
      } catch (error) {
        console.warn("Failed to get cache size:", error)
      } finally {
        setIsLoading(false)
      }
    }

    loadCacheSize()
  }, [])

  const handleClearCache = useCallback(async () => {
    setIsClearing(true)
    try {
      await clearAudioCache()
      setCacheSize(0)
    } catch (error) {
      console.error("Failed to clear cache:", error)
    } finally {
      setIsClearing(false)
    }
  }, [])

  // Format cache size to human-readable format
  const formattedCacheSize = formatBytes(cacheSize)

  return {
    cacheSize,
    formattedCacheSize,
    clearCache: handleClearCache,
    isClearing,
    isLoading,
  }
}

/**
 * Format bytes to human-readable format
 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"

  const k = 1024
  const sizes = ["B", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i]
}

/**
 * Hook to remove a specific cached audio
 */
export function useRemoveCachedAudio() {
  return useCallback(async (key: string) => {
    try {
      await removeCachedAudio(key)
      console.debug(`Removed cached audio: ${key}`)
    } catch (error) {
      console.error(`Failed to remove cached audio ${key}:`, error)
    }
  }, [])
}
