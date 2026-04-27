"use client"

import { z } from "zod"
import { toast } from "sonner"
import { useRouter } from "next/navigation"
import { formOptions } from "@tanstack/react-form"
import { useQueryClient } from "@tanstack/react-query"

import { useAppForm } from "@/hooks/use-app-form"
import { voiceRpcClient } from "@/lib/rpc"

import { useTTSVoices } from "../contexts/tts-voices-context"
import { useGenerateSpeechMutation } from "../hooks/use-generations"

const ttsFormSchema = z.object({
  text: z.string().min(1, "Please enter some text"),
  voiceId: z.string().min(1, "Please select a voice"),
  temperature: z.number(),
  exaggeration: z.number(),
  cfgWeight: z.number(),
})

export type TTSFormValues = z.infer<typeof ttsFormSchema>

export const defaultTTSValues: TTSFormValues = {
  text: "",
  voiceId: "",
  // Conservative defaults for more natural pacing with most cloned voices.
  temperature: 0.6,
  exaggeration: 0.35,
  cfgWeight: 0.35,
}

function resolveLanguageID(language: string): string {
  const normalized = language.trim().toLowerCase()
  if (!normalized) {
    return "en"
  }

  const firstSegment = normalized.split(/[-_]/)[0]
  return firstSegment || "en"
}

export const ttsFormOptions = formOptions({
  defaultValues: defaultTTSValues,
})

export function TextToSpeechForm({
  children,
  defaultValues,
}: {
  children: React.ReactNode
  defaultValues?: TTSFormValues
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { allVoices } = useTTSVoices()
  const generateSpeechMutation = useGenerateSpeechMutation()

  const form = useAppForm({
    ...ttsFormOptions,
    defaultValues: defaultValues ?? defaultTTSValues,
    validators: {
      onSubmit: ttsFormSchema,
    },
    onSubmit: async ({ value }) => {
      try {
        const selectedVoice = allVoices.find((voice) => voice.id === value.voiceId)

        if (!selectedVoice) {
          throw new Error("Please select a valid voice")
        }

        const playback = await voiceRpcClient.getVoicePlaybackUrl({
          voiceId: selectedVoice.id,
        })
        const providerKey = playback.providerKey.trim()

        if (!providerKey) {
          throw new Error("Failed to resolve voice provider key")
        }

        const data = await generateSpeechMutation.mutateAsync({
          text: value.text.trim(),
          voiceId: selectedVoice.id,
          voiceName: selectedVoice.name,
          voiceKey: providerKey,
          languageId: resolveLanguageID(selectedVoice.language),
          temperature: value.temperature,
          exaggeration: value.exaggeration,
          cfgWeight: value.cfgWeight,
        })

        toast.success("Audio generated successfully!")
        await queryClient.invalidateQueries({ queryKey: ["generations"] })
        router.push(`/text-to-speech/${data.generationId}`)
      } catch (error) {
        const message =
          error instanceof Error ? error.message : "Failed to generate audio"

        if (message === "SUBSCRIPTION_REQUIRED") {
          toast.error("Subscription required")
          return
        }

        toast.error(message)
      }
    },
  })

  return <form.AppForm>{children}</form.AppForm>
}
