"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"

import { useAuth } from "@/lib/auth-context"
import { voiceRpcClient } from "@/lib/rpc"
import { removeVoiceProviderKey } from "@/lib/audio-cache"

type DeleteVoiceInput = {
  id: string
}

export function useDeleteVoice() {
  const { accessToken } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (input: DeleteVoiceInput) => {
      if (!accessToken) {
        throw new Error("Unauthorized")
      }
      if (!input.id.trim()) {
        throw new Error("Voice id is required")
      }

      const response = await voiceRpcClient.deleteVoice({
        id: input.id,
      })

      if (!response.success) {
        throw new Error("Delete failed")
      }

      return response.success
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["voices"] })
    },
    onSettled: async (_data, _error, variables) => {
      if (variables?.id) {
        await removeVoiceProviderKey(variables.id)
      }
    },
  })
}
