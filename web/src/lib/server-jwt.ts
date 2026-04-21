import { jwtVerify, type JWTPayload, importJWK, decodeProtectedHeader } from "jose"

type JsonWebKey = Parameters<typeof importJWK>[0]

type CachedJwkEntry = {
  key: JsonWebKey
  observedAt: number
}

const JWKS_REFRESH_INTERVAL_MS = 60 * 60 * 1000
const JWKS_RETENTION_MS = 8 * 24 * 60 * 60 * 1000

const jwksCache = new Map<string, CachedJwkEntry>()
let lastJwksFetchAt = 0

export type RefreshTokenValidationResult =
  | { valid: true; payload: JWTPayload }
  | {
      valid: false
      reason:
        | "missing-token"
        | "invalid-token"
        | "invalid-token-type"
        | "invalid-algorithm"
        | "jwks-fetch-failed"
    }

export async function validateRefreshToken(
  token: string | undefined,
): Promise<RefreshTokenValidationResult> {
  if (!token) {
    return { valid: false, reason: "missing-token" }
  }

  try {
    // Decode header to check algorithm without verifying yet
    const header = decodeProtectedHeader(token)
    const algorithm = header.alg as string
    const kid = header.kid as string

    if (!algorithm) {
      return { valid: false, reason: "invalid-token" }
    }

    if (algorithm !== "RS256") {
      return { valid: false, reason: "invalid-algorithm" }
    }

    if (!kid) {
      return { valid: false, reason: "invalid-token" }
    }

    const jwksUrl =
      process.env.AUTH_SERVICE_JWKS_URL ||
      process.env.NEXT_PUBLIC_AUTH_JWKS_URL ||
      "http://localhost:50051/.well-known/jwks.json"

    const jwkKey = await resolveRefreshTokenJwk(jwksUrl, kid)
    if ("reason" in jwkKey) {
      return { valid: false, reason: jwkKey.reason }
    }

    try {
      const publicKey = await importJWK(jwkKey.key as any, "RS256")

      const { payload } = await jwtVerify(token, publicKey, {
        algorithms: ["RS256"],
        requiredClaims: ["exp", "iat", "sub", "token_type"],
      })

      if (payload.token_type !== "refresh") {
        return { valid: false, reason: "invalid-token-type" }
      }

      return { valid: true, payload }
    } catch {
      return { valid: false, reason: "invalid-token" }
    }
  } catch {
    return { valid: false, reason: "invalid-token" }
  }
}

type JwkResolutionResult =
  | { key: JsonWebKey }
  | { reason: "invalid-token" | "jwks-fetch-failed" }

async function resolveRefreshTokenJwk(
  jwksUrl: string,
  kid: string,
): Promise<JwkResolutionResult> {
  const now = Date.now()
  pruneExpiredJwksEntries(now)

  const cachedEntry = jwksCache.get(kid)
  if (cachedEntry && now - lastJwksFetchAt < JWKS_REFRESH_INTERVAL_MS) {
    return { key: cachedEntry.key }
  }

  const refreshResult = await refreshJwksCache(jwksUrl, now)
  const refreshedEntry = jwksCache.get(kid)
  if (refreshedEntry) {
    return { key: refreshedEntry.key }
  }

  if (cachedEntry) {
    return { key: cachedEntry.key }
  }

  if (!refreshResult.ok) {
    return { reason: refreshResult.reason }
  }

  return { reason: "invalid-token" }
}

async function refreshJwksCache(
  jwksUrl: string,
  now: number,
): Promise<{ ok: true } | { ok: false; reason: "jwks-fetch-failed" }> {
  try {
    const response = await fetch(jwksUrl, { cache: "no-store" })
    if (!response.ok) {
      return { ok: false, reason: "jwks-fetch-failed" }
    }

    const jwks = await response.json()
    const keys = Array.isArray(jwks?.keys) ? jwks.keys : []

    for (const key of keys) {
      if (!key || typeof key.kid !== "string") continue
      if (key.alg !== "RS256" || key.kty !== "RSA") continue

      jwksCache.set(key.kid, {
        key,
        observedAt: now,
      })
    }

    lastJwksFetchAt = now
    pruneExpiredJwksEntries(now)
    return { ok: true }
  } catch {
    return { ok: false, reason: "jwks-fetch-failed" }
  }
}

function pruneExpiredJwksEntries(now: number) {
  for (const [kid, entry] of jwksCache.entries()) {
    if (now - entry.observedAt > JWKS_RETENTION_MS) {
      jwksCache.delete(kid)
    }
  }
}
