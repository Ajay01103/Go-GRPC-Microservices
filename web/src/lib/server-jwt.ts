import { jwtVerify, type JWTPayload } from "jose"

const JWT_ALGORITHM = "HS256"

export type RefreshTokenValidationResult =
  | { valid: true; payload: JWTPayload }
  | {
      valid: false
      reason: "missing-token" | "missing-secret" | "invalid-token" | "invalid-token-type"
    }

function getJwtSecret() {
  const secret = process.env.JWT_SECRET
  if (!secret) return null

  return new TextEncoder().encode(secret)
}

export async function validateRefreshToken(
  token: string | undefined,
): Promise<RefreshTokenValidationResult> {
  if (!token) {
    return { valid: false, reason: "missing-token" }
  }

  const secret = getJwtSecret()
  if (!secret) {
    return { valid: false, reason: "missing-secret" }
  }

  try {
    const { payload } = await jwtVerify(token, secret, {
      algorithms: [JWT_ALGORITHM],
      requiredClaims: ["exp", "iat", "sub", "token_type"],
    })

    if (payload.token_type !== "refresh") {
      return { valid: false, reason: "invalid-token-type" }
    }

    return { valid: true, payload }
  } catch {
    return { valid: false, reason: "invalid-token" }
  }
}
