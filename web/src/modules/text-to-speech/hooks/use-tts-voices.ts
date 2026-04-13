"use client"

import { useMemo } from "react"

import { useCurrentUser } from "@/hooks/use-current-user"
import { useVoices } from "@/modules/voices/hooks/use-voices"

export function useTTSVoices() {
  const { data: currentUser } = useCurrentUser()

  const { data: customVoices = [], isPending: isCustomVoicesPending } = useVoices({
    userId: currentUser?.userId ?? "",
    query: "",
  })

  const { data: systemVoices = [], isPending: isSystemVoicesPending } = useVoices({
    userId: "SYSTEM",
    query: "",
  })

  const allVoices = useMemo(() => [...customVoices, ...systemVoices], [customVoices, systemVoices])

  return {
    customVoices,
    systemVoices,
    allVoices,
    isLoading: isCustomVoicesPending || isSystemVoicesPending,
  }
}
