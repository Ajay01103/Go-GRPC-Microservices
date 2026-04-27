"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"

import { VoiceCategory } from "@/gen/pb/voice_pb"
import { useAuth } from "@/lib/auth-context"
import { voiceRpcClient } from "@/lib/rpc"

type UpdateVoiceInput = {
  id: string
  name: string
  description?: string
  language: string
  category: "GENERAL" | "NARRATION" | "CHARACTER"
}

function mapCategory(category: UpdateVoiceInput["category"]): VoiceCategory {
  switch (category) {
    case "NARRATION":
      return VoiceCategory.NARRATION
    case "CHARACTER":
      return VoiceCategory.CHARACTER
    default:
      return VoiceCategory.GENERAL
  }
}

export function useUpdateVoice() {
  const { accessToken } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (input: UpdateVoiceInput) => {
      if (!accessToken) {
        throw new Error("Unauthorized")
      }
      if (!input.id.trim()) {
        throw new Error("Voice id is required")
      }
      if (!input.name.trim()) {
        throw new Error("Name is required")
      }
      if (!input.language.trim()) {
        throw new Error("Language is required")
      }

      const response = await voiceRpcClient.updateVoice(
        {
          id: input.id,
          name: input.name,
          description: input.description ?? "",
          category: mapCategory(input.category),
          language: input.language,
        },
      )

      if (!response.voice) {
        throw new Error("Update failed")
      }

      return response.voice
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["voices"] })
    },
  })
}
