"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"

import { VoiceCategory } from "@/gen/pb/voice_pb"
import { useAuth } from "@/lib/auth-context"
import { voiceRpcClient } from "@/lib/rpc"

type UseCreateVoiceInput = {
  name: string
  description?: string
  language: string
  variant: string
  audioData: Uint8Array<ArrayBuffer>
  contentType?: string
  category: string
}

function mapCategory(category: string): VoiceCategory {
  switch (category) {
    case "NARRATION":
      return VoiceCategory.NARRATION
    case "CHARACTER":
    case "CHARACTERS":
      return VoiceCategory.CHARACTER
    default:
      return VoiceCategory.GENERAL
  }
}

function mapVariant(variant: string) {
  switch (variant) {
    case "MALE":
      return 1 // VoiceVariant.MALE
    case "FEMALE":
      return 2 // VoiceVariant.FEMALE
    case "NEUTRAL":
      return 3 // VoiceVariant.NEUTRAL
    default:
      return 3 // VoiceVariant.NEUTRAL
  }
}

export function useCreateVoice() {
  const { accessToken } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (input: UseCreateVoiceInput) => {
      if (!accessToken) {
        throw new Error("Unauthorized")
      }

      if (!input.name.trim()) {
        throw new Error("Name is required")
      }
      if (!input.language.trim()) {
        throw new Error("Language is required")
      }
      if (!input.audioData || input.audioData.length === 0) {
        throw new Error("Audio data is required")
      }

      const response = await voiceRpcClient.createVoice(
        {
          name: input.name,
          description: input.description ?? "",
          category: mapCategory(input.category),
          language: input.language,
          variant: mapVariant(input.variant),
          audioData: input.audioData,
          contentType: input.contentType ?? "audio/wav",
        },
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
          },
        },
      )

      if (!response.voice) {
        throw new Error("Failed to create voice")
      }

      return response.voice
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["voices"] })
    },
  })
}
