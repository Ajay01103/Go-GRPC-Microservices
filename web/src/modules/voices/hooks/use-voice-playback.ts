"use client"

import { useQuery, type QueryClient } from "@tanstack/react-query"

import { voiceRpcClient } from "@/lib/rpc"

const PLAYBACK_REFRESH_BUFFER_SECONDS = 20

export type VoicePlaybackResult = {
  url: string
  expiresAtUnix: number
  providerKey: string
}

function isPlaybackFresh(playback: VoicePlaybackResult | undefined): boolean {
  if (!playback) {
    return false
  }
  const nowUnix = Math.floor(Date.now() / 1000)
  return playback.expiresAtUnix > nowUnix + PLAYBACK_REFRESH_BUFFER_SECONDS
}

async function requestVoicePlayback(voiceId: string): Promise<VoicePlaybackResult> {
  const response = await voiceRpcClient.getVoicePlaybackUrl({ voiceId })
  const providerKey = response.providerKey.trim()

  if (!providerKey) {
    throw new Error("Missing provider key for voice playback")
  }

  return {
    url: response.url,
    expiresAtUnix: Number(response.expiresAtUnix),
    providerKey,
  }
}

export function getVoicePlaybackQueryOptions(voiceId: string) {
  return {
    queryKey: ["voice-playback", voiceId],
    queryFn: async () => requestVoicePlayback(voiceId),
    staleTime: 60 * 1000,
  }
}

export function useVoicePlaybackQuery(voiceId: string, enabled = true) {
  return useQuery({
    ...getVoicePlaybackQueryOptions(voiceId),
    enabled: Boolean(enabled && voiceId),
  })
}

export async function getVoicePlayback(
  queryClient: QueryClient,
  voiceId: string,
): Promise<VoicePlaybackResult> {
  const cached = queryClient.getQueryData<VoicePlaybackResult>([
    "voice-playback",
    voiceId,
  ])
  if (cached && isPlaybackFresh(cached)) {
    return cached
  }

  return queryClient.fetchQuery(getVoicePlaybackQueryOptions(voiceId))
}
