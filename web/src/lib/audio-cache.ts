/**
 * IndexedDB-based audio cache management
 * Stores audio blobs locally to reduce S3 roundtrips and improve load times
 */

const DB_NAME = "audioCache"
const STORE_NAME = "audioBlobs"
const DB_VERSION = 1

interface CacheEntry {
  key: string // S3 object key
  blob: Blob
  timestamp: number // For cache invalidation
}

let dbInstance: IDBDatabase | null = null

async function initDB(): Promise<IDBDatabase> {
  if (dbInstance) return dbInstance

  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)

    request.onerror = () => reject(request.error)
    request.onsuccess = () => {
      dbInstance = request.result
      resolve(dbInstance)
    }

    request.onupgradeneeded = (event) => {
      const db = (event.target as IDBOpenDBRequest).result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, { keyPath: "key" })
        store.createIndex("timestamp", "timestamp", { unique: false })
      }
    }
  })
}

/**
 * Get cached audio blob
 * @param key S3 object key
 * @returns Blob if found, null otherwise
 */
export async function getCachedAudio(key: string): Promise<Blob | null> {
  if (!key || typeof window === "undefined") return null

  try {
    const db = await initDB()
    return new Promise((resolve) => {
      const transaction = db.transaction([STORE_NAME], "readonly")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.get(key)

      request.onsuccess = () => {
        const entry = request.result as CacheEntry | undefined
        resolve(entry?.blob || null)
      }
      request.onerror = () => resolve(null)
    })
  } catch (error) {
    console.warn("Failed to get cached audio:", error)
    return null
  }
}

/**
 * Cache audio blob
 * @param key S3 object key
 * @param blob Audio blob to cache
 */
export async function cacheAudio(key: string, blob: Blob): Promise<void> {
  if (!key || !blob || typeof window === "undefined") return

  try {
    const db = await initDB()
    const entry: CacheEntry = {
      key,
      blob,
      timestamp: Date.now(),
    }

    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], "readwrite")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.put(entry)

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })
  } catch (error) {
    console.warn("Failed to cache audio:", error)
  }
}

/**
 * Clear all cached audio
 */
export async function clearAudioCache(): Promise<void> {
  if (typeof window === "undefined") return

  try {
    const db = await initDB()
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], "readwrite")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.clear()

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })
  } catch (error) {
    console.warn("Failed to clear audio cache:", error)
  }
}

/**
 * Remove specific cached audio
 * @param key S3 object key
 */
export async function removeCachedAudio(key: string): Promise<void> {
  if (!key || typeof window === "undefined") return

  try {
    const db = await initDB()
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], "readwrite")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.delete(key)

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })
  } catch (error) {
    console.warn("Failed to remove cached audio:", error)
  }
}

/**
 * Get cache size in bytes
 */
export async function getAudioCacheSize(): Promise<number> {
  if (typeof window === "undefined") return 0

  try {
    const db = await initDB()
    return new Promise((resolve) => {
      const transaction = db.transaction([STORE_NAME], "readonly")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.getAll()

      request.onsuccess = () => {
        const entries = request.result as CacheEntry[]
        const totalSize = entries.reduce((sum, entry) => sum + entry.blob.size, 0)
        resolve(totalSize)
      }
      request.onerror = () => resolve(0)
    })
  } catch (error) {
    console.warn("Failed to get cache size:", error)
    return 0
  }
}
