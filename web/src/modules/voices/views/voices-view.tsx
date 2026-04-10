"use client"

import { useQueryState } from "nuqs"

import { useCurrentUser } from "@/hooks/use-current-user"
import { VoicesList } from "../components/voices-list"
import { voicesSearchParams } from "../lib/params"
import { VoicesToolbar } from "../components/voices-toolbar"
import { useVoices } from "../hooks/use-voices"

function VoicesContent() {
  const [query] = useQueryState("query", voicesSearchParams.query)
  const { data: currentUser } = useCurrentUser()

  const { data: customVoices = [], isPending: isCustomLoading } = useVoices({
    userId: currentUser?.userId ?? "",
    query,
  })

  const { data: systemVoices = [], isPending: isSystemLoading } = useVoices({
    userId: "SYSTEM",
    query,
  })

  return (
    <>
      <VoicesList
        title="Team Voices"
        voices={customVoices}
        isLoading={isCustomLoading}
      />
      <VoicesList
        title="Built-in Voices"
        voices={systemVoices}
        isLoading={isSystemLoading}
      />
    </>
  )
}

export function VoicesView() {
  return (
    <div className="flex-1 space-y-10 overflow-y-auto p-3 lg:p-6">
      <VoicesToolbar />
      <VoicesContent />
    </div>
  )
}
