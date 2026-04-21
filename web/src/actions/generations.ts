"use server"

import { requireAccessTokenAction } from "@/actions/auth"
import { getSignedAudioUrl } from "@/lib/s3"

export type GenerationPlaybackUrlResult = {
  url: string
}

export async function getGenerationPlaybackUrlAction(
  s3ObjectKey: string,
): Promise<GenerationPlaybackUrlResult> {
  const normalizedKey = s3ObjectKey.trim().replace(/^\/+/, "")
  if (!normalizedKey) {
    throw new Error("s3ObjectKey is required")
  }

  await requireAccessTokenAction()

  const url = await getSignedAudioUrl(normalizedKey)
  if (!url) {
    throw new Error("Failed to sign generation audio url")
  }

  return { url }
}
