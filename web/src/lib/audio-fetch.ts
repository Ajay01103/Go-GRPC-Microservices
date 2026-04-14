/**
 * Audio fetching utility with caching support
 * Handles downloading audio from signed URLs and caching them locally
 */

import { getCachedAudio, cacheAudio } from "./audio-cache"

interface FetchAudioOptions {
  url?: string
  key: string // S3 object key for cache identification
  signal?: AbortSignal
}

/**
 * Fetch audio from URL with caching support
 * Checks cache first, then fetches from URL if not cached
 * @param url Signed S3 URL
 * @param key S3 object key for cache identification
 * @param signal AbortSignal for cancellation
 * @returns Blob URL for use with audio elements
 */
export async function fetchAudioWithCache({
  url,
  key,
  signal,
}: FetchAudioOptions): Promise<string> {
  if (!url || !key) {
    throw new Error("URL and key are required")
  }

  // Try to get from cache first
  const cachedBlob = await getCachedAudio(key)
  if (cachedBlob) {
    console.debug(`[AudioCache] Serving from cache: ${key}`)
    return URL.createObjectURL(cachedBlob)
  }

  console.debug(`[AudioCache] Fetching from S3: ${key}`)

  // Fetch from S3
  const response = await fetch(url, { signal })
  if (!response.ok) {
    throw new Error(`Failed to fetch audio: ${response.statusText}`)
  }

  const blob = await response.blob()

  // Cache the blob for future use
  await cacheAudio(key, blob)
    .then(() => console.debug(`[AudioCache] Cached: ${key}`))
    .catch((error) => console.warn(`[AudioCache] Failed to cache: ${key}`, error))

  return URL.createObjectURL(blob)
}

/**
 * Prefetch audio and cache it without creating a blob URL
 * Useful for preloading frequently accessed audio
 */
export async function prefetchAudio(url: string, key: string): Promise<void> {
  if (!url || !key) return

  // Check if already cached
  const cached = await getCachedAudio(key)
  if (cached) {
    console.debug(`[AudioCache] Already cached: ${key}`)
    return
  }

  console.debug(`[AudioCache] Prefetching: ${key}`)

  try {
    const response = await fetch(url)
    if (!response.ok) {
      throw new Error(`Failed to prefetch audio: ${response.statusText}`)
    }

    const blob = await response.blob()
    await cacheAudio(key, blob)
    console.debug(`[AudioCache] Prefetched and cached: ${key}`)
  } catch (error) {
    console.warn(`[AudioCache] Prefetch failed for ${key}:`, error)
  }
}
