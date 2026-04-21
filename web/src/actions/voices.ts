"use server"

import { requireAccessTokenAction } from "@/actions/auth"
import { VoiceCategory } from "@/gen/pb/voice_pb"
import { voiceRpcClient } from "@/lib/rpc"

const s3Bucket = process.env.S3_BUCKET?.trim() ?? ""

function resolveProviderKeyFromURL(rawURL: string): string {
  let key = ""

  try {
    const parsed = new URL(rawURL)
    key = decodeURIComponent(parsed.pathname.replace(/^\/+/, ""))
  } catch {
    key = decodeURIComponent(rawURL.split("?")[0].replace(/^\/+/, ""))
  }

  if (!key) {
    return ""
  }

  if (s3Bucket && key.startsWith(`${s3Bucket}/`)) {
    key = key.slice(s3Bucket.length + 1)
  }

  return key
}

export type VoicePlaybackUrlResult = {
  url: string
  expiresAtUnix: number
  providerKey: string
}

export type CreateVoiceInput = {
  name: string
  description?: string
  category: VoiceCategory
  language: string
  variant: string
  audioData: Uint8Array<ArrayBuffer>
  contentType?: string
}

export type CreateVoiceResult = {
  id: string
  name: string
  description: string
  category: VoiceCategory
  language: string
}

export async function getVoicePlaybackUrlAction(
  voiceId: string,
  accessToken?: string | null,
): Promise<VoicePlaybackUrlResult> {
  if (!voiceId) {
    throw new Error("voiceId is required")
  }

  const token = accessToken ?? (await requireAccessTokenAction())

  const response = await voiceRpcClient.getVoicePlaybackUrl(
    {
      voiceId,
    },
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  )

  const providerKey = resolveProviderKeyFromURL(response.url)

  if (!providerKey) {
    throw new Error("Failed to resolve voice provider key")
  }

  return {
    url: response.url,
    expiresAtUnix: Number(response.expiresAtUnix),
    providerKey,
  }
}

export async function createVoiceAction(
  input: CreateVoiceInput,
  accessToken?: string | null,
): Promise<CreateVoiceResult> {
  if (!input.name.trim()) {
    throw new Error("Name is required")
  }
  if (!input.language.trim()) {
    throw new Error("Language is required")
  }
  if (!input.audioData || input.audioData.length === 0) {
    throw new Error("Audio data is required")
  }

  const token = accessToken ?? (await requireAccessTokenAction())

  const response = await voiceRpcClient.createVoice(
    {
      name: input.name,
      description: input.description ?? "",
      category: input.category,
      language: input.language,
      audioData: input.audioData,
      contentType: input.contentType ?? "audio/wav",
    },
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  )

  if (!response.voice) {
    throw new Error("Failed to create voice")
  }

  return {
    id: response.voice.id,
    name: response.voice.name,
    description: response.voice.description,
    category: response.voice.category,
    language: response.voice.language,
  }
}
