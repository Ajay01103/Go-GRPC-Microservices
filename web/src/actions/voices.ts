"use server"

import { refreshAccessTokenAction } from "@/actions/auth"
import { voiceRpcClient } from "@/lib/rpc"

export type VoicePlaybackUrlResult = {
  url: string
  expiresAtUnix: number
}

export async function getVoicePlaybackUrlAction(
  voiceId: string,
  accessToken?: string | null,
): Promise<VoicePlaybackUrlResult> {
  if (!voiceId) {
    throw new Error("voiceId is required")
  }

  const token = accessToken ?? (await refreshAccessTokenAction())
  if (!token) {
    throw new Error("Unauthorized")
  }

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

  return {
    url: response.url,
    expiresAtUnix: Number(response.expiresAtUnix),
  }
}
