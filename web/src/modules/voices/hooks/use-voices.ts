"use client"

import { useQuery } from "@tanstack/react-query"

import type { VoiceItemType } from "@/modules/voices/components/voice-card"
import { useAuth } from "@/lib/auth-context"
import { voiceRpcClient } from "@/lib/rpc"

interface UseVoicesParams {
  userId: string
  query: string
}

export function useVoices({ userId, query }: UseVoicesParams) {
  const { accessToken } = useAuth()

  return useQuery({
    queryKey: ["voices", userId, query, !!accessToken],
    enabled: Boolean(userId && accessToken),
    staleTime: 60 * 1000,
    queryFn: async (): Promise<VoiceItemType[]> => {
      if (!userId || !accessToken) return []
      const response = await voiceRpcClient.getAllVoices(
        {
          userId,
          query,
        },
      )
      return response.voices as VoiceItemType[]
    },
  })
}
