/**
 * Service Worker registration and management
 */

/**
 * Register the audio caching service worker
 * Call this once on app initialization (e.g., in root layout or after auth)
 */
export async function registerAudioCacheServiceWorker(): Promise<void> {
  if (typeof window === "undefined" || !("serviceWorker" in navigator)) {
    console.log("[AudioSW] Service Workers not supported")
    return
  }

  try {
    const registration = await navigator.serviceWorker.register("/sw.js", {
      scope: "/",
    })
    console.log("[AudioSW] Service Worker registered successfully:", registration)
  } catch (error) {
    console.warn("[AudioSW] Failed to register Service Worker:", error)
  }
}

/**
 * Unregister the audio caching service worker
 * Useful for cleanup or testing
 */
export async function unregisterAudioCacheServiceWorker(): Promise<void> {
  if (typeof window === "undefined" || !("serviceWorker" in navigator)) {
    return
  }

  try {
    const registration = await navigator.serviceWorker.ready
    await registration.unregister()
    console.log("[AudioSW] Service Worker unregistered successfully")
  } catch (error) {
    console.warn("[AudioSW] Failed to unregister Service Worker:", error)
  }
}

/**
 * Get service worker registration
 */
export async function getAudioCacheServiceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
  if (typeof window === "undefined" || !("serviceWorker" in navigator)) {
    return null
  }

  try {
    return await navigator.serviceWorker.ready
  } catch (error) {
    console.warn("[AudioSW] Failed to get Service Worker registration:", error)
    return null
  }
}
