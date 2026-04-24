import { createPrivateKey, createPublicKey, generateKeyPairSync, randomUUID } from "node:crypto"
import { SignJWT } from "jose"

const DPOP_PRIVATE_JWK_COOKIE = "dpop_private_jwk"
const DPOP_REFRESH_HTU = "/auth.AuthService/RefreshToken"

type DPoPPrivateJWK = {
  kty: "OKP"
  crv: "Ed25519"
  x: string
  d: string
}

type CookieStoreLike = {
  get(name: string): { value: string } | undefined
  set(
    name: string,
    value: string,
    options?: {
      httpOnly?: boolean
      secure?: boolean
      sameSite?: "lax" | "strict" | "none"
      path?: string
      maxAge?: number
    },
  ): void
}

function encodeJwkCookie(jwk: DPoPPrivateJWK): string {
  return Buffer.from(JSON.stringify(jwk), "utf8").toString("base64url")
}

function decodeJwkCookie(raw: string | undefined): DPoPPrivateJWK | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(Buffer.from(raw, "base64url").toString("utf8")) as DPoPPrivateJWK
    if (parsed?.kty !== "OKP" || parsed?.crv !== "Ed25519" || !parsed?.x || !parsed?.d) {
      return null
    }
    return parsed
  } catch {
    return null
  }
}

function getOrCreatePrivateJWK(cookieStore: CookieStoreLike): DPoPPrivateJWK {
  const existing = decodeJwkCookie(cookieStore.get(DPOP_PRIVATE_JWK_COOKIE)?.value)
  if (existing) {
    return existing
  }

  const { privateKey } = generateKeyPairSync("ed25519")
  const privateJwk = privateKey.export({ format: "jwk" }) as DPoPPrivateJWK

  cookieStore.set(DPOP_PRIVATE_JWK_COOKIE, encodeJwkCookie(privateJwk), {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 30 * 24 * 60 * 60,
  })

  return privateJwk
}

export async function createRefreshDPoPProof(
  cookieStore: CookieStoreLike,
  nonce?: string,
): Promise<string> {
  const privateJwk = getOrCreatePrivateJWK(cookieStore)
  const privateKey = createPrivateKey({ format: "jwk", key: privateJwk })
  const publicJwk = createPublicKey(privateKey).export({ format: "jwk" }) as {
    kty: "OKP"
    crv: "Ed25519"
    x: string
  }

  const payload: Record<string, string | number> = {
    htm: "POST",
    htu: DPOP_REFRESH_HTU,
    iat: Math.floor(Date.now() / 1000),
    jti: randomUUID(),
  }
  if (nonce) {
    payload.nonce = nonce
  }

  return new SignJWT(payload)
    .setProtectedHeader({
      typ: "dpop+jwt",
      alg: "EdDSA",
      jwk: {
        kty: "OKP",
        crv: "Ed25519",
        x: publicJwk.x,
      },
    })
    .sign(privateKey)
}
