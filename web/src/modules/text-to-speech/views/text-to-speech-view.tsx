"use client"

import { useMemo } from "react"
import { useSearchParams } from "next/navigation"

import { TextInputPanel } from "../components/text-input-panel"
import {
  TextToSpeechForm,
  TTSFormValues,
  defaultTTSValues,
} from "../components/text-to-speech-form"
import { TTSVoicesProvider } from "../contexts/tts-voices-context"
import { useTTSVoices } from "../hooks/use-tts-voices"
import { VoicePreviewPlaceholder } from "../components/voice-preview-placeholder"
import { SettingsPanel } from "../components/settings-panel"

export const TextToSpeechView = ({ initialValues }: { initialValues?: Partial<TTSFormValues> }) => {
  const searchParams = useSearchParams()
  const voiceIdFromUrl = searchParams.get("voiceId") ?? undefined
  const { customVoices, systemVoices, allVoices } = useTTSVoices()

  const defaultValues = useMemo<TTSFormValues>(() => {
    const requestedVoiceId = initialValues?.voiceId ?? voiceIdFromUrl ?? ""
    const fallbackVoiceId = allVoices[0]?.id ?? ""
    const resolvedVoiceId =
      requestedVoiceId && allVoices.some((voice) => voice.id === requestedVoiceId)
        ? requestedVoiceId
        : fallbackVoiceId

    return {
      ...defaultTTSValues,
      ...initialValues,
      voiceId: resolvedVoiceId,
    }
  }, [allVoices, initialValues, voiceIdFromUrl])

  return (
    <TTSVoicesProvider value={{ customVoices, systemVoices, allVoices }}>
      <TextToSpeechForm
        key={defaultValues.voiceId || "tts-form"}
        defaultValues={defaultValues}>
        <div className="flex min-h-0 flex-1 overflow-hidden">
          <div className="flex min-h-0 flex-1 flex-col">
            <TextInputPanel />
            <VoicePreviewPlaceholder />
          </div>
          <SettingsPanel />
        </div>
      </TextToSpeechForm>
    </TTSVoicesProvider>
  )
}
