/**
 * Service Worker for audio caching
 * Provides network-level caching of audio files from S3
 * 
 * Install this service worker in your app's root layout or initialization code:
 * 
 * if ('serviceWorker' in navigator) {
 *   navigator.serviceWorker.register('/sw.js')
 * }
 */

const CACHE_NAME = "audio-cache-v1"
const CACHE_MAX_AGE = 30 * 24 * 60 * 60 * 1000 // 30 days

declare const self: ServiceWorkerGlobalScope

// Install event - set up the cache
self.addEventListener("install", (event: ExtendableEvent) => {
  console.log("[ServiceWorker] Installing...")
  self.skipWaiting()
})

// Activate event - clean up old caches
self.addEventListener("activate", (event: ExtendableEvent) => {
  console.log("[ServiceWorker] Activating...")
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            console.log(`[ServiceWorker] Deleting old cache: ${cacheName}`)
            return caches.delete(cacheName)
          }
        }),
      )
    }),
  )
  self.clients.claim()
})

// Fetch event - serve from cache or network
self.addEventListener("fetch", (event: FetchEvent) => {
  const { request } = event

  // Only cache GET requests for audio files
  if (request.method !== "GET") {
    return
  }

  // Check if this is an audio file request (from S3)
  const isAudioRequest =
    request.url.includes(".s3") ||
    request.url.includes("s3-") ||
    request.headers.get("accept")?.includes("audio")

  if (!isAudioRequest) {
    return
  }

  event.respondWith(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.match(request).then((cachedResponse) => {
        if (cachedResponse) {
          // Check if cache is still fresh
          const cachedTime = cachedResponse.headers.get("x-cached-time")
          if (cachedTime) {
            const cached = parseInt(cachedTime)
            const now = Date.now()
            if (now - cached < CACHE_MAX_AGE) {
              console.log(`[ServiceWorker] Serving from cache: ${request.url}`)
              return cachedResponse
            }
          }
        }

        // Fetch from network
        return fetch(request).then((response) => {
          // Don't cache non-successful responses
          if (!response || response.status !== 200 || response.type === "error") {
            return response
          }

          // Clone the response for caching
          const responseToCache = response.clone()
          const newResponse = new Response(responseToCache.body, {
            status: responseToCache.status,
            statusText: responseToCache.statusText,
            headers: new Headers(responseToCache.headers),
          })

          // Add timestamp header
          newResponse.headers.set("x-cached-time", String(Date.now()))

          cache.put(request, newResponse.clone())
          console.log(`[ServiceWorker] Cached: ${request.url}`)

          return response
        })
      })
    }),
  )
})
