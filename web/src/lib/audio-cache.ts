/**
 * IndexedDB-based audio cache management
 * Stores audio blobs locally to reduce S3 roundtrips and improve load times
 */

const DB_NAME = "audioCache"
const STORE_NAME = "audioBlobs"
const PROVIDER_KEY_INDEX_STORE = "voiceProviderKeys"
const DB_VERSION = 3
const CACHE_CAP_BYTES = 200 * 1024 * 1024

interface CacheEntry {
  key: string // S3 object key
  blob: Blob
  timestamp: number
  lastAccessed: number
  sizeBytes: number
}

interface VoiceProviderKeyEntry {
  voiceId: string
  providerKey: string
  updatedAt: number
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
      const previousVersion = event.oldVersion

      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, { keyPath: "key" })
        store.createIndex("timestamp", "timestamp", { unique: false })
        store.createIndex("lastAccessed", "lastAccessed", { unique: false })
      }

      if (!db.objectStoreNames.contains(PROVIDER_KEY_INDEX_STORE)) {
        db.createObjectStore(PROVIDER_KEY_INDEX_STORE, { keyPath: "voiceId" })
      }

      if (previousVersion < 2) {
        const transaction = (event.target as IDBOpenDBRequest).transaction
        if (!transaction) {
          return
        }

        const store = transaction.objectStore(STORE_NAME)
        if (!store.indexNames.contains("lastAccessed")) {
          store.createIndex("lastAccessed", "lastAccessed", { unique: false })
        }
      }

      if (previousVersion < 3) {
        if (!db.objectStoreNames.contains(PROVIDER_KEY_INDEX_STORE)) {
          db.createObjectStore(PROVIDER_KEY_INDEX_STORE, { keyPath: "voiceId" })
        }
      }
    }
  })
}

function normalizeEntrySize(entry: CacheEntry): number {
  return entry.sizeBytes > 0 ? entry.sizeBytes : entry.blob.size
}

async function enforceCacheCap(db: IDBDatabase): Promise<void> {
  const entries = await new Promise<CacheEntry[]>((resolve) => {
    const transaction = db.transaction([STORE_NAME], "readonly")
    const store = transaction.objectStore(STORE_NAME)
    const request = store.getAll()

    request.onsuccess = () => resolve((request.result as CacheEntry[]) ?? [])
    request.onerror = () => resolve([])
  })

  let totalSize = entries.reduce((sum, entry) => sum + normalizeEntrySize(entry), 0)
  if (totalSize <= CACHE_CAP_BYTES) {
    return
  }

  const evictionOrder = [...entries].sort(
    (a, b) => (a.lastAccessed || a.timestamp || 0) - (b.lastAccessed || b.timestamp || 0),
  )

  await new Promise<void>((resolve, reject) => {
    const transaction = db.transaction([STORE_NAME], "readwrite")
    const store = transaction.objectStore(STORE_NAME)

    for (const entry of evictionOrder) {
      if (totalSize <= CACHE_CAP_BYTES) {
        break
      }
      totalSize -= normalizeEntrySize(entry)
      store.delete(entry.key)
    }

    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error)
    transaction.onabort = () => reject(transaction.error)
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
      const transaction = db.transaction([STORE_NAME], "readwrite")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.get(key)

      request.onsuccess = () => {
        const entry = request.result as CacheEntry | undefined
        if (!entry?.blob) {
          resolve(null)
          return
        }

        store.put({
          ...entry,
          timestamp: entry.timestamp || Date.now(),
          lastAccessed: Date.now(),
          sizeBytes: normalizeEntrySize(entry),
        } satisfies CacheEntry)

        resolve(entry.blob)
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
    const now = Date.now()
    const entry: CacheEntry = {
      key,
      blob,
      timestamp: now,
      lastAccessed: now,
      sizeBytes: blob.size,
    }

    await new Promise<void>((resolve, reject) => {
      const transaction = db.transaction([STORE_NAME], "readwrite")
      const store = transaction.objectStore(STORE_NAME)
      const request = store.put(entry)

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })

    await enforceCacheCap(db)
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
      const transaction = db.transaction(
        [STORE_NAME, PROVIDER_KEY_INDEX_STORE],
        "readwrite",
      )
      const audioStore = transaction.objectStore(STORE_NAME)
      const providerStore = transaction.objectStore(PROVIDER_KEY_INDEX_STORE)
      const audioRequest = audioStore.clear()
      const providerRequest = providerStore.clear()

      audioRequest.onsuccess = () => undefined
      providerRequest.onsuccess = () => resolve()
      audioRequest.onerror = () => reject(audioRequest.error)
      providerRequest.onerror = () => reject(providerRequest.error)
    })
  } catch (error) {
    console.warn("Failed to clear audio cache:", error)
  }
}

export async function getVoiceProviderKey(voiceId: string): Promise<string | null> {
  if (!voiceId || typeof window === "undefined") return null

  try {
    const db = await initDB()
    return new Promise((resolve) => {
      const transaction = db.transaction([PROVIDER_KEY_INDEX_STORE], "readonly")
      const store = transaction.objectStore(PROVIDER_KEY_INDEX_STORE)
      const request = store.get(voiceId)

      request.onsuccess = () => {
        const entry = request.result as VoiceProviderKeyEntry | undefined
        const providerKey = entry?.providerKey?.trim() || ""
        resolve(providerKey || null)
      }
      request.onerror = () => resolve(null)
    })
  } catch (error) {
    console.warn("Failed to read voice provider key:", error)
    return null
  }
}

export async function setVoiceProviderKey(
  voiceId: string,
  providerKey: string,
): Promise<void> {
  if (!voiceId || !providerKey || typeof window === "undefined") return

  try {
    const db = await initDB()
    const entry: VoiceProviderKeyEntry = {
      voiceId,
      providerKey,
      updatedAt: Date.now(),
    }

    await new Promise<void>((resolve, reject) => {
      const transaction = db.transaction([PROVIDER_KEY_INDEX_STORE], "readwrite")
      const store = transaction.objectStore(PROVIDER_KEY_INDEX_STORE)
      const request = store.put(entry)

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })
  } catch (error) {
    console.warn("Failed to persist voice provider key:", error)
  }
}

/**
 * Remove a persisted voiceId -> providerKey mapping.
 */
export async function removeVoiceProviderKey(voiceId: string): Promise<void> {
  if (!voiceId || typeof window === "undefined") return

  try {
    const db = await initDB()
    return new Promise((resolve, reject) => {
      const transaction = db.transaction([PROVIDER_KEY_INDEX_STORE], "readwrite")
      const store = transaction.objectStore(PROVIDER_KEY_INDEX_STORE)
      const request = store.delete(voiceId)

      request.onsuccess = () => resolve()
      request.onerror = () => reject(request.error)
    })
  } catch (error) {
    console.warn("Failed to remove voice provider key:", error)
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
        const totalSize = entries.reduce(
          (sum, entry) => sum + normalizeEntrySize(entry),
          0,
        )
        resolve(totalSize)
      }
      request.onerror = () => resolve(0)
    })
  } catch (error) {
    console.warn("Failed to get cache size:", error)
    return 0
  }
}
