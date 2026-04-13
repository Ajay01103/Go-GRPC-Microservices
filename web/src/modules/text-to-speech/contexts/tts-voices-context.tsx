"use client"

import { createContext, useContext } from "react"
import type { PlainMessage } from "@bufbuild/protobuf"

import type { VoiceItem } from "@/gen/pb/voice_pb"

export type TTSVoiceItem = PlainMessage<VoiceItem>

interface TTSVoicesContextValue {
  customVoices: TTSVoiceItem[]
  systemVoices: TTSVoiceItem[]
  allVoices: TTSVoiceItem[]
}

const TTSVoicesContext = createContext<TTSVoicesContextValue | null>(null)

export function TTSVoicesProvider({
  children,
  value,
}: {
  children: React.ReactNode
  value: TTSVoicesContextValue
}) {
  return <TTSVoicesContext.Provider value={value}>{children}</TTSVoicesContext.Provider>
}

export function useTTSVoices() {
  const context = useContext(TTSVoicesContext)

  if (!context) {
    throw new Error("useTTSVoices must be used within a TTSVoicesProvider")
  }

  return context
}
