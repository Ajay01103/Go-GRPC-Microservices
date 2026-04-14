"use client"

import { useEffect, useState } from "react"
import { GenerationJobStatus } from "@/gen/pb/generation_pb"

import {
  useGeneration,
  useGenerationPlaybackUrlWithCache,
} from "@/modules/text-to-speech/hooks/use-generations"
import { useTTSVoices } from "@/modules/text-to-speech/hooks/use-tts-voices"
import { TextInputPanel } from "@/modules/text-to-speech/components/text-input-panel"
import { SettingsPanel } from "@/modules/text-to-speech/components/settings-panel"
import {
  TextToSpeechForm,
  type TTSFormValues,
} from "@/modules/text-to-speech/components/text-to-speech-form"
import { TTSVoicesProvider } from "../contexts/tts-voices-context"
import { VoicePreviewPanel } from "../components/voice-preview-panel"
import { VoicePreviewPlaceholder } from "../components/voice-preview-placeholder"

interface TextToSpeechDetailViewProps {
  params: Promise<{ generationId: string }>
}

export function TextToSpeechDetailView({ params }: TextToSpeechDetailViewProps) {
  const [generationId, setGenerationId] = useState<string>("")

  // Await params once
  useEffect(() => {
    params.then(({ generationId }) => {
      setGenerationId(generationId)
    })
  }, [params])

  const { data: generation, isLoading: isGenerationLoading } = useGeneration(generationId)
  const {
    allVoices,
    customVoices,
    systemVoices,
    isLoading: isVoicesLoading,
  } = useTTSVoices()
  const hasGeneratedAudio = Boolean(generation?.s3ObjectKey?.trim())
  const {
    data: generatedAudioSrc,
    isLoading: isAudioSrcLoading,
    isError: isAudioSrcError,
  } = useGenerationPlaybackUrlWithCache(generation?.s3ObjectKey)

  if (isGenerationLoading || isVoicesLoading || !generation || !generationId) {
    return <div>Loading...</div>
  }

  const isActiveJob =
    generation.status === GenerationJobStatus.QUEUED ||
    generation.status === GenerationJobStatus.PROCESSING

  const statusLabel =
    generation.status === GenerationJobStatus.COMPLETED
      ? "Completed"
      : generation.status === GenerationJobStatus.FAILED
        ? "Failed"
        : generation.status === GenerationJobStatus.PROCESSING
          ? "Processing"
          : "Queued"

  const fallbackVoiceId = allVoices[0]?.id ?? ""

  // Requested voice may no longer exist (deleted); fall back to first available
  const resolvedVoiceId =
    generation.voiceId && allVoices.some((v) => v.id === generation.voiceId)
      ? generation.voiceId
      : fallbackVoiceId

  const defaultValues: TTSFormValues = {
    text: generation.text,
    voiceId: resolvedVoiceId,
    temperature: generation.temperature,
    exaggeration: generation.exaggeration,
    cfgWeight: generation.cfgWeight,
  }

  // Use the denormalized voiceName snapshot instead of a populated voice relation
  // so the preview always shows the voice name at the time of generation,
  // even if the voice was later renamed or deleted.
  const generationVoice = {
    id: generation.voiceId ?? undefined,
    name: generation.voiceName,
  }

  const showGeneratedAudioPreview =
    hasGeneratedAudio && Boolean(generatedAudioSrc) && !isAudioSrcError

  return (
    <TTSVoicesProvider value={{ customVoices, systemVoices, allVoices }}>
      <TextToSpeechForm
        key={generationId}
        defaultValues={defaultValues}>
        <div className="flex min-h-0 flex-1 overflow-hidden">
          <div className="flex min-h-0 flex-1 flex-col">
            <TextInputPanel generation={generation} />
            <div className="border-t px-4 py-3 text-sm lg:px-6">
              <div className="hidden lg:flex items-center justify-between gap-3">
                <span className="font-medium">Generation status</span>
                <span
                  className={
                    isActiveJob
                      ? "text-amber-500"
                      : generation.status === GenerationJobStatus.FAILED
                        ? "text-red-500"
                        : "text-emerald-500"
                  }>
                  {statusLabel}
                </span>
              </div>
              {generation.errorMessage ? (
                <p className="mt-2 text-sm text-red-500">{generation.errorMessage}</p>
              ) : null}
              {isActiveJob ? (
                <p className="mt-2 text-xs text-muted-foreground">
                  Audio will appear here automatically when processing completes.
                </p>
              ) : null}
            </div>
            {showGeneratedAudioPreview ? (
              <VoicePreviewPanel
                audioUrl={generatedAudioSrc ?? ""}
                voice={generationVoice}
                text={generation.text}
              />
            ) : (
              <VoicePreviewPlaceholder />
            )}
          </div>
          <SettingsPanel />
        </div>
      </TextToSpeechForm>
    </TTSVoicesProvider>
  )
}
